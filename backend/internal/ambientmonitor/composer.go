package ambientmonitor

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"automation-hub-backend/internal/outcomeevaluation"
	"automation-hub-backend/internal/proactivity"
)

const compositionHistoryLimit = maxHistory

// ObservationHistory is the only monitor persistence capability required by
// the composer. It can read immutable observations, but cannot alter monitor
// targets, leases, runs, or source systems.
type ObservationHistory interface {
	ListObservationsAt(context.Context, string, string, string, time.Time, int) ([]ObservationRecord, error)
}

// OutcomeEvaluator exposes only the owner-scoped outcome operations needed to
// turn source-backed monitor observations into one advisory evaluation.
type OutcomeEvaluator interface {
	GetOutcome(context.Context, string, string, string) (outcomeevaluation.OutcomeRevision, error)
	ResolveOutcomeRevision(context.Context, string, string, string, int64, string) (outcomeevaluation.OutcomeRevision, error)
	CreateEvaluation(context.Context, string, string, string, outcomeevaluation.CreateEvaluationRequest) (outcomeevaluation.EvaluationRecord, bool, error)
}

// ProactivityAdvisor exposes advisory persistence and evaluation only. It has
// no delivery, execution, task, workflow, calendar, mandate, or learning API.
type ProactivityAdvisor interface {
	CurrentPolicy(context.Context, string) (proactivity.PolicyRecord, error)
	RecordPolicy(context.Context, string, string, proactivity.Preferences) (proactivity.PolicyRecord, bool, error)
	RecordSignals(context.Context, string, string, []proactivity.OpenLoopSignal) ([]proactivity.SignalRecord, bool, error)
	CaptureEvaluationSnapshot(context.Context, string, time.Time) (proactivity.EvaluationSnapshot, error)
	EvaluateStoredSnapshot(context.Context, string, proactivity.EvaluateStoredSnapshotRequest) (proactivity.DecisionBatch, bool, error)
}

// Composer adapts immutable monitor evidence into the existing outcome and
// proactivity engines. Every downstream write is keyed by the immutable run
// digest, making a sink retry an exact replay instead of a duplicate action.
type Composer struct {
	history     ObservationHistory
	outcomes    OutcomeEvaluator
	proactivity ProactivityAdvisor
}

var _ Sink = (*Composer)(nil)
var _ SnapshotProvider = (*Composer)(nil)

func NewComposer(history ObservationHistory, outcomes OutcomeEvaluator, advisor ProactivityAdvisor) *Composer {
	return &Composer{history: history, outcomes: outcomes, proactivity: advisor}
}

func (c *Composer) CaptureSnapshot(ctx context.Context, signal AdvisorySignal) (CompositionSnapshot, error) {
	if c == nil || isTypedNil(c.outcomes) || isTypedNil(c.proactivity) {
		return CompositionSnapshot{}, compositionError("composition dependencies unavailable", ErrRepositoryUnavailable)
	}
	if err := validateCompositionSource(signal); err != nil {
		return CompositionSnapshot{}, err
	}
	scope := signal.Run.Scope
	outcomeRevision, err := c.outcomes.GetOutcome(ctx, scope.OwnerID, scope.WorkspaceID, signal.Run.OutcomeID)
	if err != nil {
		return CompositionSnapshot{}, sanitizeCompositionError("load outcome snapshot", err)
	}
	if outcomeRevision.Outcome.ID != signal.Run.OutcomeID ||
		outcomeRevision.Outcome.Scope.OwnerID != scope.OwnerID ||
		outcomeRevision.Outcome.Scope.WorkspaceID != scope.WorkspaceID {
		return CompositionSnapshot{}, ErrScopeViolation
	}
	if !outcomeHasIndicator(outcomeRevision.Outcome, signal.Run.IndicatorID) {
		return CompositionSnapshot{}, compositionError("target indicator is not defined by outcome", ErrInvalidInput)
	}
	records, err := c.history.ListObservationsAt(ctx, scope.OwnerID, scope.WorkspaceID, signal.Run.TargetID, signal.Run.FinishedAt, compositionHistoryLimit)
	if err != nil {
		return CompositionSnapshot{}, sanitizeCompositionError("load monitor observations for snapshot", err)
	}
	currentPresent := false
	for _, record := range records {
		if record.ID == signal.Observation.ID && record.RecordDigest == signal.Observation.RecordDigest {
			currentPresent = true
			break
		}
	}
	if !currentPresent {
		records = append(records, signal.Observation)
	}
	if _, currentFound, historyErr := composeObservations(signal, outcomeRevision.Outcome, records); historyErr != nil {
		return CompositionSnapshot{}, historyErr
	} else if !currentFound {
		return CompositionSnapshot{}, compositionError("completed observation is absent from immutable history", ErrInvalidInput)
	}
	policy, err := c.ensureDefaultPolicy(ctx, scope.OwnerID)
	if err != nil {
		return CompositionSnapshot{}, err
	}
	capturedAt := signal.Run.FinishedAt.UTC()
	if policy.RecordedAt.After(capturedAt) {
		capturedAt = policy.RecordedAt.UTC()
	}
	attention, err := c.proactivity.CaptureEvaluationSnapshot(ctx, scope.OwnerID, capturedAt)
	if err != nil {
		return CompositionSnapshot{}, sanitizeCompositionError("capture owner attention snapshot", err)
	}
	capturedAt = attention.CapturedAt.UTC()
	value := CompositionSnapshot{
		ContractVersion:    compositionSnapshotVersion,
		Status:             CompositionSnapshotPinned,
		ComposerVersion:    currentComposerVersion,
		CapturedAt:         capturedAt,
		OutcomeRevision:    outcomeRevision.Revision,
		OutcomeAuditDigest: outcomeRevision.AuditDigest,
		Attention:          attention,
	}
	value.SnapshotDigest, err = compositionSnapshotDigest(value)
	if err != nil {
		return CompositionSnapshot{}, compositionError("digest composition snapshot", ErrInvalidInput)
	}
	return validateCompositionSnapshot(value)
}

func (c *Composer) Compose(ctx context.Context, signal AdvisorySignal) (CompositionResult, error) {
	if c == nil || isTypedNil(c.history) || isTypedNil(c.outcomes) || isTypedNil(c.proactivity) {
		return CompositionResult{}, compositionError("composition dependencies unavailable", ErrRepositoryUnavailable)
	}
	if err := validateCompositionSignal(signal); err != nil {
		return CompositionResult{}, err
	}

	scope := signal.Run.Scope
	outcomeRevision, err := c.outcomes.ResolveOutcomeRevision(ctx, scope.OwnerID, scope.WorkspaceID, signal.Run.OutcomeID, signal.Snapshot.OutcomeRevision, signal.Snapshot.OutcomeAuditDigest)
	if err != nil {
		return CompositionResult{}, sanitizeCompositionError("load outcome", err)
	}
	if outcomeRevision.Outcome.ID != signal.Run.OutcomeID ||
		outcomeRevision.Outcome.Scope.OwnerID != scope.OwnerID ||
		outcomeRevision.Outcome.Scope.WorkspaceID != scope.WorkspaceID {
		return CompositionResult{}, ErrScopeViolation
	}
	if !outcomeHasIndicator(outcomeRevision.Outcome, signal.Run.IndicatorID) {
		return CompositionResult{}, compositionError("target indicator is not defined by outcome", ErrInvalidInput)
	}

	records, err := c.history.ListObservationsAt(ctx, scope.OwnerID, scope.WorkspaceID, signal.Run.TargetID, signal.Run.FinishedAt, compositionHistoryLimit)
	if err != nil {
		return CompositionResult{}, sanitizeCompositionError("load monitor observations", err)
	}
	observations, currentFound, err := composeObservations(signal, outcomeRevision.Outcome, records)
	if err != nil {
		return CompositionResult{}, err
	}
	if !currentFound {
		return CompositionResult{}, compositionError("completed observation is absent from immutable history", ErrInvalidInput)
	}

	evaluationRecord, _, err := c.outcomes.CreateEvaluation(ctx, scope.OwnerID, scope.WorkspaceID, signal.Run.OutcomeID, outcomeevaluation.CreateEvaluationRequest{
		IdempotencyKey:     compositionKey("ambient-evaluation", signal.Run.RecordDigest),
		OutcomeRevision:    outcomeRevision.Revision,
		OutcomeAuditDigest: outcomeRevision.AuditDigest,
		Observations:       observations,
		AsOf:               signal.Run.FinishedAt,
	})
	if err != nil {
		return CompositionResult{}, sanitizeCompositionError("create outcome evaluation", err)
	}
	if evaluationRecord.Evaluation.Scope.OwnerID != scope.OwnerID ||
		evaluationRecord.Evaluation.Scope.WorkspaceID != scope.WorkspaceID ||
		evaluationRecord.Evaluation.OutcomeID != signal.Run.OutcomeID {
		return CompositionResult{}, ErrScopeViolation
	}
	if err := evaluationRecord.Evaluation.ValidateNoAuthority(); err != nil {
		return CompositionResult{}, compositionError("outcome evaluation violated advisory authority", ErrInvalidInput)
	}

	openLoop := proactivitySignal(signal, outcomeRevision.Outcome, evaluationRecord)
	recordedSignals, _, err := c.proactivity.RecordSignals(ctx, scope.OwnerID,
		compositionKey("ambient-signal", signal.Run.RecordDigest), []proactivity.OpenLoopSignal{openLoop})
	if err != nil {
		return CompositionResult{}, sanitizeCompositionError("record proactivity signal", err)
	}
	if len(recordedSignals) != 1 || recordedSignals[0].OwnerIdentity != scope.OwnerID || recordedSignals[0].Signal.OwnerIdentity != scope.OwnerID {
		return CompositionResult{}, ErrScopeViolation
	}
	batch, _, err := c.proactivity.EvaluateStoredSnapshot(ctx, scope.OwnerID, proactivity.EvaluateStoredSnapshotRequest{
		IdempotencyKey:    compositionKey("ambient-decision", signal.Run.RecordDigest),
		Now:               signal.Snapshot.CapturedAt,
		Snapshot:          signal.Snapshot.Attention,
		AdditionalSignals: []proactivity.OpenLoopSignal{openLoop},
	})
	if err != nil {
		return CompositionResult{}, sanitizeCompositionError("evaluate owner attention", err)
	}
	if batch.OwnerIdentity != scope.OwnerID || batch.Result.OwnerIdentity != scope.OwnerID {
		return CompositionResult{}, ErrScopeViolation
	}
	for _, decision := range batch.Result.Decisions {
		if decision.OwnerIdentity != scope.OwnerID || decision.ExecutionAuthorized || decision.DeliveryAuthorized || decision.AuthorityGranted {
			return CompositionResult{}, ErrScopeViolation
		}
	}
	return CompositionResult{
		DisableTarget: !signal.Run.FinishedAt.Before(outcomeRevision.Outcome.Window.End),
	}, nil
}

func (c *Composer) ensureDefaultPolicy(ctx context.Context, owner string) (proactivity.PolicyRecord, error) {
	if record, err := c.proactivity.CurrentPolicy(ctx, owner); err == nil {
		if record.OwnerIdentity != owner || record.Policy.OwnerIdentity != owner {
			return proactivity.PolicyRecord{}, ErrScopeViolation
		}
		return record, nil
	} else if !errors.Is(err, proactivity.ErrNotFound) {
		return proactivity.PolicyRecord{}, sanitizeCompositionError("load proactivity policy", err)
	}
	digest, err := exactDigest("ambient_default_policy", owner)
	if err != nil {
		return proactivity.PolicyRecord{}, compositionError("derive default policy key", ErrInvalidInput)
	}
	record, _, err := c.proactivity.RecordPolicy(ctx, owner,
		compositionKey("ambient-policy", digest), proactivity.DefaultPreferences(owner))
	if err != nil {
		return proactivity.PolicyRecord{}, sanitizeCompositionError("record default proactivity policy", err)
	}
	if record.OwnerIdentity != owner || record.Policy.OwnerIdentity != owner {
		return proactivity.PolicyRecord{}, ErrScopeViolation
	}
	return record, nil
}

func validateCompositionSignal(signal AdvisorySignal) error {
	if err := validateCompositionSource(signal); err != nil {
		return err
	}
	snapshot, err := validateCompositionSnapshot(signal.Snapshot)
	if err != nil {
		return compositionError("composition snapshot is invalid", ErrSnapshotUnavailable)
	}
	if snapshot.Status != CompositionSnapshotPinned {
		return ErrSnapshotUnavailable
	}
	if snapshot.Attention.OwnerIdentity != signal.Run.Scope.OwnerID {
		return ErrScopeViolation
	}
	return nil
}

func validateCompositionSource(signal AdvisorySignal) error {
	if err := validateAuthority(signal.Authority); err != nil {
		return compositionError("signal authority is invalid", ErrInvalidInput)
	}
	owner, workspace, target := signal.Run.Scope.OwnerID, signal.Run.Scope.WorkspaceID, signal.Run.TargetID
	if err := validateRunRecord(signal.Run, owner, workspace, target); err != nil {
		return sanitizeCompositionValidation(err)
	}
	if err := validateObservationRecord(signal.Observation, owner, workspace, target); err != nil {
		return sanitizeCompositionValidation(err)
	}
	if signal.Run.Status != RunCompleted || signal.Observation.Scope != signal.Run.Scope ||
		signal.Observation.OutcomeID != signal.Run.OutcomeID || signal.Observation.IndicatorID != signal.Run.IndicatorID ||
		signal.Observation.SourceKind != signal.Run.SourceKind || signal.Run.ObservationID != signal.Observation.ID ||
		signal.Run.ObservationDigest != signal.Observation.RecordDigest {
		return ErrScopeViolation
	}
	if !signal.Observation.RecordedAt.Equal(signal.Run.FinishedAt) || signal.Observation.ObservedAt.After(signal.Observation.RecordedAt) {
		return compositionError("observation and run chronology is invalid", ErrInvalidInput)
	}
	observationDigest, err := immutableObservationDigest(signal.Observation)
	if err != nil || observationDigest != signal.Observation.RecordDigest {
		return compositionError("observation integrity check failed", ErrInvalidInput)
	}
	monitorRunDigest, err := runDigest(signal.Run)
	if err != nil || monitorRunDigest != signal.Run.RecordDigest {
		return compositionError("run integrity check failed", ErrInvalidInput)
	}
	return nil
}

func composeObservations(signal AdvisorySignal, outcome outcomeevaluation.IntendedOutcome, records []ObservationRecord) ([]outcomeevaluation.Observation, bool, error) {
	result := make([]outcomeevaluation.Observation, 0, len(records))
	seenIDs := make(map[string]struct{}, len(records))
	seenDigests := make(map[string]struct{}, len(records))
	currentFound := false
	for _, record := range records {
		if err := validateObservationRecord(record, signal.Run.Scope.OwnerID, signal.Run.Scope.WorkspaceID, signal.Run.TargetID); err != nil {
			return nil, false, sanitizeCompositionValidation(err)
		}
		if record.OutcomeID != signal.Run.OutcomeID || record.IndicatorID != signal.Run.IndicatorID || record.SourceKind != signal.Run.SourceKind {
			return nil, false, ErrScopeViolation
		}
		digest, err := immutableObservationDigest(record)
		if err != nil || digest != record.RecordDigest {
			return nil, false, compositionError("observation history integrity check failed", ErrInvalidInput)
		}
		if _, exists := seenIDs[record.ID]; exists {
			return nil, false, compositionError("observation history contains a duplicate id", ErrInvalidInput)
		}
		if _, exists := seenDigests[record.RecordDigest]; exists {
			return nil, false, compositionError("observation history contains a duplicate digest", ErrInvalidInput)
		}
		seenIDs[record.ID] = struct{}{}
		seenDigests[record.RecordDigest] = struct{}{}
		if record.ID == signal.Observation.ID && record.RecordDigest == signal.Observation.RecordDigest {
			currentFound = true
		}
		if record.ObservedAt.Before(outcome.Window.Start) || record.ObservedAt.After(outcome.Window.End) ||
			record.ObservedAt.After(signal.Run.FinishedAt) || record.RecordedAt.After(signal.Run.FinishedAt) {
			continue
		}
		result = append(result, outcomeObservation(record))
	}
	sort.Slice(result, func(i, j int) bool {
		if !result[i].ObservedAt.Equal(result[j].ObservedAt) {
			return result[i].ObservedAt.Before(result[j].ObservedAt)
		}
		return result[i].ID < result[j].ID
	})
	return result, currentFound, nil
}

func outcomeObservation(record ObservationRecord) outcomeevaluation.Observation {
	return outcomeevaluation.Observation{
		ID:           record.ID,
		Scope:        outcomeevaluation.Scope{OwnerID: record.Scope.OwnerID, WorkspaceID: record.Scope.WorkspaceID},
		IndicatorID:  record.IndicatorID,
		Value:        record.Value,
		ObservedAt:   record.ObservedAt,
		RecordedAt:   record.RecordedAt,
		Verification: outcomeevaluation.VerificationSourceSupported,
		Sources: []outcomeevaluation.SourceReference{{
			ID: record.ID, URI: monitorObservationURI(record.ID), ContentDigest: record.SourceDigest,
			RetrievedAt: record.RecordedAt, Status: outcomeevaluation.SourceSupported,
		}},
		Attribution: outcomeevaluation.Attribution{
			Method: outcomeevaluation.AttributionCorrelation, Confidence: 1,
			Rationale: "Deterministic advisory monitor observation from an immutable source snapshot.",
		},
	}
}

func proactivitySignal(signal AdvisorySignal, outcome outcomeevaluation.IntendedOutcome, record outcomeevaluation.EvaluationRecord) proactivity.OpenLoopSignal {
	state := record.Evaluation.State
	status, risk, impact, urgency, confidence, title := proactivityState(state)
	var deadline *time.Time
	if status == proactivity.StatusOpen && outcome.Window.End.After(record.Evaluation.AsOf) {
		value := outcome.Window.End.UTC()
		deadline = &value
	}
	return proactivity.OpenLoopSignal{
		ContractVersion:     proactivity.ContractVersion,
		ID:                  compositionKey("ambient-monitor", signal.Run.RecordDigest),
		OwnerIdentity:       signal.Run.Scope.OwnerID,
		OpenLoopKey:         stableOpenLoopKey(signal.Run),
		Title:               title,
		Summary:             "The latest source-supported outcome monitor evaluation is " + strings.ReplaceAll(string(state), "_", " ") + ".",
		Status:              status,
		Risk:                risk,
		ObservedAt:          record.Evaluation.AsOf,
		LastActivityAt:      signal.Run.FinishedAt,
		Deadline:            deadline,
		StaleAfter:          24 * time.Hour,
		Impact:              impact,
		Urgency:             urgency,
		Confidence:          confidence,
		Sensitive:           true,
		HumanReviewRequired: record.Evaluation.ReviewRequired,
		Evidence: []proactivity.EvidenceReference{{
			ID: record.Evaluation.ID, Kind: "outcome_evaluation", Digest: record.RecordDigest,
			ObservedAt: record.Evaluation.AsOf,
		}},
	}
}

func proactivityState(state outcomeevaluation.OutcomeState) (proactivity.SignalStatus, proactivity.RiskLevel, float64, float64, float64, string) {
	switch state {
	case outcomeevaluation.OutcomeAchieved:
		return proactivity.StatusResolved, proactivity.RiskLow, 0.2, 0.1, 0.95, "Outcome achieved"
	case outcomeevaluation.OutcomeOnTrack:
		return proactivity.StatusOpen, proactivity.RiskLow, 0.55, 0.4, 0.95, "Outcome is on track"
	case outcomeevaluation.OutcomeRegression:
		return proactivity.StatusOpen, proactivity.RiskHigh, 0.95, 0.9, 0.95, "Review outcome regression"
	case outcomeevaluation.OutcomeReviewRequired:
		return proactivity.StatusOpen, proactivity.RiskMedium, 0.8, 0.7, 0.9, "Review outcome evidence"
	default:
		return proactivity.StatusOpen, proactivity.RiskMedium, 0.65, 0.55, 0.85, "Collect outcome evidence"
	}
}

func outcomeHasIndicator(outcome outcomeevaluation.IntendedOutcome, indicatorID string) bool {
	for _, indicator := range outcome.Indicators {
		if indicator.ID == indicatorID {
			return true
		}
	}
	return false
}

func immutableObservationDigest(record ObservationRecord) (string, error) {
	return exactDigest("observation_record", struct {
		Scope                            Scope
		TargetID, OutcomeID, IndicatorID string
		SourceKind                       SourceKind
		Value                            float64
		ObservedAt, RecordedAt           time.Time
		SourceDigest                     string
	}{record.Scope, record.TargetID, record.OutcomeID, record.IndicatorID, record.SourceKind, record.Value, record.ObservedAt, record.RecordedAt, record.SourceDigest})
}

func stableOpenLoopKey(run MonitorRun) string {
	digest, err := exactDigest("ambient_open_loop", struct {
		Scope       Scope
		TargetID    string
		OutcomeID   string
		IndicatorID string
	}{run.Scope, run.TargetID, run.OutcomeID, run.IndicatorID})
	if err != nil {
		return compositionKey("outcome-monitor", run.RecordDigest)
	}
	return compositionKey("outcome-monitor", digest)
}

func monitorObservationURI(observationID string) string {
	return (&url.URL{Scheme: "hai", Host: "ambient-monitor", Path: "/observations/" + url.PathEscape(observationID)}).String()
}

func compositionKey(prefix, digest string) string {
	return prefix + "-" + strings.TrimSpace(digest)
}

func sanitizeCompositionValidation(err error) error {
	if errors.Is(err, ErrScopeViolation) {
		return ErrScopeViolation
	}
	return compositionError("composition input is invalid", ErrInvalidInput)
}

func sanitizeCompositionError(stage string, err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, ErrScopeViolation), errors.Is(err, outcomeevaluation.ErrScopeViolation), errors.Is(err, proactivity.ErrOwnerScopeViolation):
		return ErrScopeViolation
	default:
		return compositionError(stage, ErrSinkFailed)
	}
}

func compositionError(message string, kind error) error {
	return fmt.Errorf("%w: %s", kind, message)
}
