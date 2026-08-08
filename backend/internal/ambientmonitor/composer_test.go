package ambientmonitor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/lifeontology"
	"automation-hub-backend/internal/outcomeevaluation"
	"automation-hub-backend/internal/proactivity"
)

type composerHistory struct {
	records []ObservationRecord
	err     error
	owner   string
	space   string
	target  string
	calls   int
}

func (h *composerHistory) ListObservationsAt(_ context.Context, owner, workspace, target string, cutoff time.Time, limit int) ([]ObservationRecord, error) {
	h.calls++
	if h.owner != "" && (owner != h.owner || workspace != h.space || target != h.target) {
		return nil, ErrScopeViolation
	}
	if limit != compositionHistoryLimit {
		return nil, errors.New("unexpected history bound")
	}
	if cutoff.IsZero() {
		return nil, errors.New("missing immutable history cutoff")
	}
	if h.err != nil {
		return nil, h.err
	}
	result := make([]ObservationRecord, 0, len(h.records))
	for _, record := range h.records {
		if !record.RecordedAt.After(cutoff) {
			result = append(result, record)
		}
	}
	return result, nil
}

func TestComposerReplaysEveryDownstreamWriteAndPopulatesOwnerInbox(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond).Add(-time.Minute)
	scope := Scope{OwnerID: "owner-robert", WorkspaceID: "workspace-hai"}
	outcomeService, proactivityService, outcome := composerServices(t, scope, now)

	current := composerObservation(t, scope, "target-attention", outcome.ID, "indicator-open-loops", SourceWorkflowOpenLoopCount, 4, now.Add(-20*time.Second), now.Add(-10*time.Second), "a")
	outside := composerObservation(t, scope, current.TargetID, outcome.ID, current.IndicatorID, current.SourceKind, 99, outcome.Window.Start.Add(-time.Hour), outcome.Window.Start.Add(-30*time.Minute), "b")
	signal := composerSignal(t, current, current.RecordedAt)
	mapped := outcomeObservation(current)
	if len(mapped.Sources) != 1 || mapped.Sources[0].URI != "hai://ambient-monitor/observations/obs-a" ||
		mapped.Sources[0].ContentDigest != current.SourceDigest || mapped.Sources[0].Status != outcomeevaluation.SourceSupported {
		t.Fatalf("unsafe outcome provenance mapping: %#v", mapped.Sources)
	}
	history := &composerHistory{records: []ObservationRecord{outside, current}, owner: scope.OwnerID, space: scope.WorkspaceID, target: current.TargetID}
	composer := NewComposer(history, outcomeService, proactivityService)
	signal = pinComposerSignal(t, composer, signal)

	if _, err := composer.Compose(t.Context(), signal); err != nil {
		t.Fatalf("Compose() error = %v", err)
	}
	if _, err := composer.Compose(t.Context(), signal); err != nil {
		t.Fatalf("Compose() replay error = %v", err)
	}
	if history.calls != 3 {
		t.Fatalf("history calls = %d, want one capture read plus one immutable read per replay", history.calls)
	}

	evaluations, err := outcomeService.Evaluations(t.Context(), scope.OwnerID, scope.WorkspaceID, outcome.ID)
	if err != nil || len(evaluations) != 1 {
		t.Fatalf("evaluations = (%d, %v), want one replay-safe record", len(evaluations), err)
	}
	evaluation := evaluations[0]
	if err := evaluation.Evaluation.ValidateNoAuthority(); err != nil {
		t.Fatalf("evaluation grants authority: %v", err)
	}
	if len(evaluation.Evaluation.Indicators) != 1 || len(evaluation.Evaluation.Indicators[0].Effective) != 1 ||
		evaluation.Evaluation.Indicators[0].Effective[0].ObservationID != current.ID {
		t.Fatalf("evaluation did not filter to the one in-window observation: %#v", evaluation.Evaluation.Indicators)
	}
	if source := evaluation.Evaluation.Indicators[0].Effective[0].SourceIDs; len(source) != 1 || source[0] != current.ID {
		t.Fatalf("effective source ids = %#v", source)
	}

	policies, err := proactivityService.PolicyHistory(t.Context(), scope.OwnerID, 10)
	if err != nil || len(policies) != 1 {
		t.Fatalf("policies = (%d, %v), want one default policy", len(policies), err)
	}
	signals, err := proactivityService.Signals(t.Context(), scope.OwnerID, 10)
	if err != nil || len(signals) != 1 {
		t.Fatalf("signals = (%d, %v), want one replay-safe signal", len(signals), err)
	}
	if !signals[0].Signal.Sensitive || !signals[0].Signal.HumanReviewRequired || signals[0].Signal.OwnerIdentity != scope.OwnerID {
		t.Fatalf("unsafe or incorrectly scoped signal: %#v", signals[0].Signal)
	}
	if len(signals[0].Signal.Evidence) != 1 || signals[0].Signal.Evidence[0].Digest != evaluation.RecordDigest {
		t.Fatalf("signal is not bound to evaluation evidence: %#v", signals[0].Signal.Evidence)
	}
	decisions, err := proactivityService.Decisions(t.Context(), scope.OwnerID, 10)
	if err != nil || len(decisions) != 1 {
		t.Fatalf("decisions = (%d, %v), want one replay-safe decision", len(decisions), err)
	}
	if decisions[0].Decision.ExecutionAuthorized || decisions[0].Decision.DeliveryAuthorized || decisions[0].Decision.AuthorityGranted {
		t.Fatalf("decision grants authority: %#v", decisions[0].Decision)
	}
	inbox, err := proactivityService.Inbox(t.Context(), scope.OwnerID, 10)
	if err != nil || len(inbox.Items) != 1 || inbox.CanExecute || inbox.Items[0].CanExecute {
		t.Fatalf("owner inbox = (%#v, %v), want one advisory item", inbox, err)
	}
}

func TestComposerRejectsCrossScopeObservationBeforeAnyDownstreamWrite(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond).Add(-time.Minute)
	scope := Scope{OwnerID: "owner-a", WorkspaceID: "workspace-a"}
	outcomeService, proactivityService, outcome := composerServices(t, scope, now)
	current := composerObservation(t, scope, "target-a", outcome.ID, "indicator-open-loops", SourceWorkflowOpenLoopCount, 2, now.Add(-20*time.Second), now.Add(-10*time.Second), "c")
	signal := composerSignal(t, current, current.RecordedAt)

	leaked := current
	leaked.Scope.OwnerID = "owner-b"
	leaked.RecordDigest = mustObservationDigest(t, leaked)
	composer := NewComposer(&composerHistory{records: []ObservationRecord{leaked}}, outcomeService, proactivityService)
	if _, err := composer.CaptureSnapshot(t.Context(), signal); !errors.Is(err, ErrScopeViolation) {
		t.Fatalf("cross-scope CaptureSnapshot() error = %v, want %v", err, ErrScopeViolation)
	}
	if evaluations, err := outcomeService.Evaluations(t.Context(), scope.OwnerID, scope.WorkspaceID, outcome.ID); err != nil || len(evaluations) != 0 {
		t.Fatalf("cross-scope input created evaluations: (%#v, %v)", evaluations, err)
	}
	if _, err := proactivityService.CurrentPolicy(t.Context(), scope.OwnerID); !errors.Is(err, proactivity.ErrNotFound) {
		t.Fatalf("cross-scope input created a policy: %v", err)
	}
	if signals, err := proactivityService.Signals(t.Context(), scope.OwnerID, 10); err != nil || len(signals) != 0 {
		t.Fatalf("cross-scope input created signals: (%#v, %v)", signals, err)
	}
}

func TestComposerRejectsUnknownIndicatorAndRedactsDownstreamErrors(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond).Add(-time.Minute)
	scope := Scope{OwnerID: "owner-a", WorkspaceID: "workspace-a"}
	outcomeService, proactivityService, outcome := composerServices(t, scope, now)
	current := composerObservation(t, scope, "target-a", outcome.ID, "indicator-unknown", SourceWorkflowOpenLoopCount, 1, now.Add(-20*time.Second), now.Add(-10*time.Second), "d")
	signal := composerSignal(t, current, current.RecordedAt)
	composer := NewComposer(&composerHistory{records: []ObservationRecord{current}}, outcomeService, proactivityService)
	if _, err := composer.CaptureSnapshot(t.Context(), signal); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown-indicator Compose() error = %v, want %v", err, ErrInvalidInput)
	}

	current = composerObservation(t, scope, "target-a", outcome.ID, "indicator-open-loops", SourceWorkflowOpenLoopCount, 1, now.Add(-20*time.Second), now.Add(-10*time.Second), "e")
	signal = composerSignal(t, current, current.RecordedAt)
	composer = NewComposer(&composerHistory{records: []ObservationRecord{current}}, outcomeService, proactivityService)
	signal = pinComposerSignal(t, composer, signal)
	composer.history = &composerHistory{err: errors.New("Bearer super-secret-provider-token")}
	_, err := composer.Compose(t.Context(), signal)
	if !errors.Is(err, ErrSinkFailed) || strings.Contains(strings.ToLower(err.Error()), "bearer") || strings.Contains(strings.ToLower(err.Error()), "secret") {
		t.Fatalf("unsafe downstream error = %q", err)
	}
}

func TestComposerRequestsMonitorShutdownAfterOutcomeWindow(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond).Add(-4 * time.Hour)
	scope := Scope{OwnerID: "owner-expiry", WorkspaceID: "workspace-expiry"}
	outcomeService, proactivityService, outcome := composerServices(t, scope, now)
	finishedAt := outcome.Window.End.Add(time.Minute)
	current := composerObservation(
		t, scope, "target-expiry", outcome.ID, "indicator-open-loops",
		SourceWorkflowOpenLoopCount, 1, outcome.Window.End.Add(-time.Minute), finishedAt, "f",
	)
	signal := composerSignal(t, current, finishedAt)
	composer := NewComposer(
		&composerHistory{records: []ObservationRecord{current}},
		outcomeService,
		proactivityService,
	)
	signal = pinComposerSignal(t, composer, signal)
	result, err := composer.Compose(t.Context(), signal)
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}
	if !result.DisableTarget {
		t.Fatal("closed outcome window did not request monitor shutdown")
	}
}

func TestComposerUsesPinnedOutcomeAndAttentionAfterCurrentStateAdvances(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond).Add(-time.Minute)
	scope := Scope{OwnerID: "owner-replay", WorkspaceID: "workspace-replay"}
	outcomeService, proactivityService, outcome := composerServices(t, scope, now)
	current := composerObservation(t, scope, "target-replay", outcome.ID, "indicator-open-loops", SourceWorkflowOpenLoopCount, 3, now.Add(-20*time.Second), now.Add(-10*time.Second), "9")
	history := &composerHistory{records: []ObservationRecord{current}, owner: scope.OwnerID, space: scope.WorkspaceID, target: current.TargetID}
	composer := NewComposer(history, outcomeService, proactivityService)
	signal := pinComposerSignal(t, composer, composerSignal(t, current, current.RecordedAt))

	updated := outcome
	updated.Statement = "A later outcome definition that must not affect the queued replay."
	if revision, created, err := outcomeService.StoreOutcome(t.Context(), scope.OwnerID, scope.WorkspaceID, outcome.ID, outcomeevaluation.StoreOutcomeRequest{
		IdempotencyKey: "store-later-outcome-revision", ExpectedRevision: signal.Snapshot.OutcomeRevision, Outcome: updated,
	}); err != nil || !created || revision.Revision <= signal.Snapshot.OutcomeRevision {
		t.Fatalf("later StoreOutcome() = (%+v, %v, %v)", revision, created, err)
	}
	laterPolicy := proactivity.DefaultPreferences(scope.OwnerID)
	laterPolicy.MinimumConfidence = 0.99
	if _, created, err := proactivityService.RecordPolicy(t.Context(), scope.OwnerID, "later-policy", laterPolicy); err != nil || !created {
		t.Fatalf("later RecordPolicy() = (%v, %v)", created, err)
	}

	if _, err := composer.Compose(t.Context(), signal); err != nil {
		t.Fatalf("Compose() exact replay error = %v", err)
	}
	evaluations, err := outcomeService.Evaluations(t.Context(), scope.OwnerID, scope.WorkspaceID, outcome.ID)
	if err != nil || len(evaluations) != 1 || evaluations[0].OutcomeRevision != signal.Snapshot.OutcomeRevision {
		t.Fatalf("evaluation revision = (%+v, %v), want pinned revision %d", evaluations, err, signal.Snapshot.OutcomeRevision)
	}
	decisions, err := proactivityService.Decisions(t.Context(), scope.OwnerID, 10)
	if err != nil || len(decisions) != 1 {
		t.Fatalf("decisions = (%+v, %v), want one result from the pinned pre-update policy", decisions, err)
	}
}

func composerServices(t *testing.T, scope Scope, now time.Time) (*outcomeevaluation.Service, *proactivity.Service, outcomeevaluation.IntendedOutcome) {
	t.Helper()
	outcomeRepository := outcomeevaluation.NewMemoryRepository()
	outcomeService := outcomeevaluation.NewService(outcomeRepository)
	proactivityService := proactivity.NewService(proactivity.NewMemoryRepository())
	window := outcomeevaluation.LongitudinalWindow{Start: now.Add(-2 * time.Hour), End: now.Add(2 * time.Hour)}
	outcome := outcomeevaluation.IntendedOutcome{
		ID: "outcome-attention", Scope: outcomeevaluation.Scope{OwnerID: scope.OwnerID, WorkspaceID: scope.WorkspaceID},
		Statement: "Reduce verified open loops through source-supported review.", LifeDomain: lifeontology.DomainPersonalAdmin, Window: window,
		Indicators: []outcomeevaluation.Indicator{{
			ID: "indicator-open-loops", Name: "Open loops", Unit: "count", Direction: outcomeevaluation.DirectionLower,
			TargetValue: 0, TargetTolerance: 0, TrendThresholdPerDay: 0.1, RegressionThreshold: 1, MinimumObservations: 2,
			Baseline: outcomeevaluation.Baseline{
				ID: "baseline-open-loops", Scope: outcomeevaluation.Scope{OwnerID: scope.OwnerID, WorkspaceID: scope.WorkspaceID},
				Value: 5, ObservedAt: window.Start.Add(-time.Minute), Verification: outcomeevaluation.VerificationUnverified,
			},
		}},
	}
	if _, created, err := outcomeService.StoreOutcome(t.Context(), scope.OwnerID, scope.WorkspaceID, outcome.ID, outcomeevaluation.StoreOutcomeRequest{
		IdempotencyKey: "store-composer-outcome", Outcome: outcome,
	}); err != nil || !created {
		t.Fatalf("StoreOutcome() = created %v, err %v", created, err)
	}
	return outcomeService, proactivityService, outcome
}

func composerObservation(t *testing.T, scope Scope, targetID, outcomeID, indicatorID string, kind SourceKind, value float64, observedAt, recordedAt time.Time, digestByte string) ObservationRecord {
	t.Helper()
	record := ObservationRecord{
		ContractVersion: ContractVersion, ID: "obs-" + digestByte, Scope: scope, TargetID: targetID,
		OutcomeID: outcomeID, IndicatorID: indicatorID, SourceKind: kind, Value: value,
		ObservedAt: observedAt, RecordedAt: recordedAt, SourceDigest: strings.Repeat(digestByte, 64), Authority: advisoryAuthority(),
	}
	record.RecordDigest = mustObservationDigest(t, record)
	return record
}

func composerSignal(t *testing.T, observation ObservationRecord, finishedAt time.Time) AdvisorySignal {
	t.Helper()
	run := MonitorRun{
		ContractVersion: ContractVersion, ID: "run-" + strings.TrimPrefix(observation.ID, "obs-"), Scope: observation.Scope,
		TargetID: observation.TargetID, OutcomeID: observation.OutcomeID, IndicatorID: observation.IndicatorID,
		SourceKind: observation.SourceKind, LeaseGeneration: 1, Status: RunCompleted,
		StartedAt: observation.RecordedAt.Add(-10 * time.Second), FinishedAt: finishedAt,
		ObservationID: observation.ID, ObservationDigest: observation.RecordDigest,
		IdempotencyDigest: strings.Repeat("f", 64), Authority: advisoryAuthority(),
	}
	var err error
	run.RecordDigest, err = runDigest(run)
	if err != nil {
		t.Fatal(err)
	}
	return AdvisorySignal{Observation: observation, Run: run, Authority: advisoryAuthority()}
}

func pinComposerSignal(t *testing.T, composer *Composer, signal AdvisorySignal) AdvisorySignal {
	t.Helper()
	snapshot, err := composer.CaptureSnapshot(t.Context(), signal)
	if err != nil {
		t.Fatalf("CaptureSnapshot() error = %v", err)
	}
	signal.Snapshot = snapshot
	return signal
}

func mustObservationDigest(t *testing.T, record ObservationRecord) string {
	t.Helper()
	digest, err := immutableObservationDigest(record)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
