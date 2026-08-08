package ambientmonitor

import (
	"fmt"
	"strings"
	"time"

	"automation-hub-backend/internal/proactivity"
)

const (
	defaultCompositionMaxAttempts = 5
	compositionRetryBase          = time.Minute
	compositionRetryMaximum       = time.Hour
	compositionSnapshotVersion    = 1
	currentComposerVersion        = "ambient-outcome-attention-v2"
)

type CompositionSnapshotStatus string

const (
	CompositionSnapshotPinned         CompositionSnapshotStatus = "pinned"
	CompositionSnapshotLegacyUnpinned CompositionSnapshotStatus = "legacy_unpinned"
)

// CompositionSnapshot pins the immutable decision context captured before a
// monitor completion is committed. Cursors identify the final record included
// in each append-only history; an empty cursor means that history was empty.
// A retry must resolve these exact references instead of reading newer state.
type CompositionSnapshot struct {
	ContractVersion    int                            `json:"contractVersion"`
	Status             CompositionSnapshotStatus      `json:"status"`
	ComposerVersion    string                         `json:"composerVersion"`
	CapturedAt         time.Time                      `json:"capturedAt"`
	OutcomeRevision    int64                          `json:"outcomeRevision,omitempty"`
	OutcomeAuditDigest string                         `json:"outcomeAuditDigest,omitempty"`
	Attention          proactivity.EvaluationSnapshot `json:"attention"`
	SnapshotDigest     string                         `json:"snapshotDigest"`
}

type CompositionStatus string

const (
	CompositionPending      CompositionStatus = "pending"
	CompositionSucceeded    CompositionStatus = "succeeded"
	CompositionDeadLettered CompositionStatus = "dead_lettered"
)

type CompositionAttemptStatus string

const (
	CompositionAttemptSucceeded CompositionAttemptStatus = "succeeded"
	CompositionAttemptFailed    CompositionAttemptStatus = "failed"
)

// CompositionDelivery is the durable outbox projection for one immutable
// monitor run. It contains only advisory evidence and cannot grant effect
// authority. Mutable fields are revisioned and lease-fenced by Repository.
type CompositionDelivery struct {
	ContractVersion   int                 `json:"contractVersion"`
	ID                string              `json:"id"`
	Scope             Scope               `json:"scope"`
	TargetID          string              `json:"targetId"`
	RunID             string              `json:"runId"`
	RunDigest         string              `json:"runDigest"`
	ObservationID     string              `json:"observationId"`
	ObservationDigest string              `json:"observationDigest"`
	Snapshot          CompositionSnapshot `json:"snapshot"`
	Status            CompositionStatus   `json:"status"`
	Revision          uint64              `json:"revision"`
	AttemptCount      int                 `json:"attemptCount"`
	MaxAttempts       int                 `json:"maxAttempts"`
	NextAttemptAt     time.Time           `json:"nextAttemptAt"`
	Lease             Lease               `json:"lease"`
	LastAttemptAt     time.Time           `json:"lastAttemptAt,omitempty"`
	LastFailureCode   string              `json:"lastFailureCode,omitempty"`
	CreatedAt         time.Time           `json:"createdAt"`
	UpdatedAt         time.Time           `json:"updatedAt"`
	CompletedAt       time.Time           `json:"completedAt,omitempty"`
	BindingDigest     string              `json:"bindingDigest"`
	Authority         AuthorityControl    `json:"authority"`
}

// CompositionAttempt is append-only evidence for one leased sink invocation.
type CompositionAttempt struct {
	ContractVersion int                      `json:"contractVersion"`
	ID              string                   `json:"id"`
	Scope           Scope                    `json:"scope"`
	DeliveryID      string                   `json:"deliveryId"`
	TargetID        string                   `json:"targetId"`
	RunID           string                   `json:"runId"`
	RunDigest       string                   `json:"runDigest"`
	SnapshotDigest  string                   `json:"snapshotDigest"`
	AttemptNumber   int                      `json:"attemptNumber"`
	LeaseGeneration uint64                   `json:"leaseGeneration"`
	WorkerID        string                   `json:"workerId"`
	Status          CompositionAttemptStatus `json:"status"`
	FailureCode     string                   `json:"failureCode,omitempty"`
	StartedAt       time.Time                `json:"startedAt"`
	FinishedAt      time.Time                `json:"finishedAt"`
	RequestDigest   string                   `json:"requestDigest"`
	RecordDigest    string                   `json:"recordDigest"`
	Authority       AuthorityControl         `json:"authority"`
}

type ProcessCompositionsRequest struct {
	Scope         Scope         `json:"scope"`
	WorkerID      string        `json:"workerId"`
	Now           time.Time     `json:"now"`
	LeaseDuration time.Duration `json:"leaseDuration"`
	Limit         int           `json:"limit"`
}

type CompositionFailure struct {
	DeliveryID string `json:"deliveryId"`
	Code       string `json:"code"`
	Retrying   bool   `json:"retrying"`
}

type ProcessCompositionsResult struct {
	Claimed   int                   `json:"claimed"`
	Succeeded int                   `json:"succeeded"`
	Failures  []CompositionFailure  `json:"failures"`
	Records   []CompositionDelivery `json:"records"`
	Authority AuthorityControl      `json:"authority"`
}

func initialCompositionDelivery(observation ObservationRecord, run MonitorRun, snapshot CompositionSnapshot) (CompositionDelivery, error) {
	if run.Status != RunCompleted || run.ObservationID != observation.ID || run.ObservationDigest != observation.RecordDigest {
		return CompositionDelivery{}, fmt.Errorf("%w: composition source binding is invalid", ErrInvalidInput)
	}
	id := "cmp-" + strings.TrimPrefix(run.ID, "run-")
	delivery := CompositionDelivery{
		ContractVersion: ContractVersion,
		ID:              id, Scope: run.Scope, TargetID: run.TargetID,
		RunID: run.ID, RunDigest: run.RecordDigest,
		ObservationID: observation.ID, ObservationDigest: observation.RecordDigest,
		Snapshot: snapshot,
		Status:   CompositionPending, Revision: 1,
		AttemptCount: 0, MaxAttempts: defaultCompositionMaxAttempts,
		NextAttemptAt: snapshot.CapturedAt, CreatedAt: snapshot.CapturedAt, UpdatedAt: snapshot.CapturedAt,
		Authority: advisoryAuthority(),
	}
	if snapshot.Status == CompositionSnapshotLegacyUnpinned {
		delivery.Status = CompositionDeadLettered
		delivery.CompletedAt = snapshot.CapturedAt
		delivery.LastFailureCode = "snapshot_unavailable"
	}
	digest, err := exactDigest("composition_binding", struct {
		Scope                                                            Scope
		ID, TargetID, RunID, RunDigest, ObservationID, ObservationDigest string
		SnapshotDigest                                                   string
	}{delivery.Scope, delivery.ID, delivery.TargetID, delivery.RunID, delivery.RunDigest, delivery.ObservationID, delivery.ObservationDigest, delivery.Snapshot.SnapshotDigest})
	if err != nil {
		return CompositionDelivery{}, err
	}
	delivery.BindingDigest = digest
	return validateCompositionDelivery(delivery)
}

func validateCompositionDelivery(value CompositionDelivery) (CompositionDelivery, error) {
	if value.ContractVersion != ContractVersion {
		return CompositionDelivery{}, fmt.Errorf("%w: unsupported composition contract version", ErrInvalidInput)
	}
	scope, err := validateScope(value.Scope)
	if err != nil {
		return CompositionDelivery{}, err
	}
	value.Scope = scope
	for name, item := range map[string]string{
		"composition id": value.ID, "target id": value.TargetID, "run id": value.RunID,
		"observation id": value.ObservationID,
	} {
		if err := validateIdentifier(name, item); err != nil {
			return CompositionDelivery{}, err
		}
	}
	if value.Snapshot, err = validateCompositionSnapshot(value.Snapshot); err != nil {
		return CompositionDelivery{}, err
	}
	if value.Snapshot.Status == CompositionSnapshotPinned && value.Snapshot.Attention.OwnerIdentity != scope.OwnerID {
		return CompositionDelivery{}, ErrScopeViolation
	}
	for name, item := range map[string]string{
		"run digest": value.RunDigest, "observation digest": value.ObservationDigest,
		"composition binding digest": value.BindingDigest,
	} {
		if _, err := validateDigest(name, item); err != nil {
			return CompositionDelivery{}, err
		}
	}
	if value.Revision < 1 || value.AttemptCount < 0 || value.MaxAttempts < 1 || value.MaxAttempts > 20 || value.AttemptCount > value.MaxAttempts {
		return CompositionDelivery{}, fmt.Errorf("%w: composition counters are invalid", ErrInvalidInput)
	}
	if value.CreatedAt, err = validateTime("composition creation time", value.CreatedAt); err != nil {
		return CompositionDelivery{}, err
	}
	if value.UpdatedAt, err = validateTime("composition update time", value.UpdatedAt); err != nil || value.UpdatedAt.Before(value.CreatedAt) {
		return CompositionDelivery{}, fmt.Errorf("%w: composition update time is invalid", ErrInvalidInput)
	}
	if err := validateLease(value.Lease); err != nil {
		return CompositionDelivery{}, err
	}
	switch value.Status {
	case CompositionPending:
		if value.NextAttemptAt, err = validateTime("composition next attempt time", value.NextAttemptAt); err != nil || !value.CompletedAt.IsZero() || (value.AttemptCount >= value.MaxAttempts) {
			return CompositionDelivery{}, fmt.Errorf("%w: pending composition state is invalid", ErrInvalidInput)
		}
	case CompositionSucceeded, CompositionDeadLettered:
		if value.CompletedAt, err = validateTime("composition completion time", value.CompletedAt); err != nil || value.CompletedAt.Before(value.CreatedAt) || value.Lease.Active() {
			return CompositionDelivery{}, fmt.Errorf("%w: terminal composition state is invalid", ErrInvalidInput)
		}
	default:
		return CompositionDelivery{}, fmt.Errorf("%w: composition status is invalid", ErrInvalidInput)
	}
	if value.AttemptCount == 0 {
		if !value.LastAttemptAt.IsZero() || (value.LastFailureCode != "" && !(value.Status == CompositionDeadLettered && value.LastFailureCode == "snapshot_unavailable")) {
			return CompositionDelivery{}, fmt.Errorf("%w: unattempted composition has attempt metadata", ErrInvalidInput)
		}
	} else {
		if value.LastAttemptAt, err = validateTime("composition last attempt time", value.LastAttemptAt); err != nil {
			return CompositionDelivery{}, err
		}
		if value.LastFailureCode != "" {
			if err := validateIdentifier("composition failure code", value.LastFailureCode); err != nil {
				return CompositionDelivery{}, err
			}
		}
	}
	if err := validateAuthority(value.Authority); err != nil {
		return CompositionDelivery{}, err
	}
	return value, nil
}

func validateCompositionSnapshot(value CompositionSnapshot) (CompositionSnapshot, error) {
	if value.ContractVersion != compositionSnapshotVersion {
		return CompositionSnapshot{}, fmt.Errorf("%w: unsupported composition snapshot version", ErrInvalidInput)
	}
	if err := validateIdentifier("composer version", value.ComposerVersion); err != nil {
		return CompositionSnapshot{}, err
	}
	var err error
	if value.CapturedAt, err = validateTime("composition snapshot capture time", value.CapturedAt); err != nil {
		return CompositionSnapshot{}, err
	}
	if _, err := validateDigest("composition snapshot digest", value.SnapshotDigest); err != nil {
		return CompositionSnapshot{}, err
	}
	switch value.Status {
	case CompositionSnapshotPinned:
		if value.ComposerVersion != currentComposerVersion || value.OutcomeRevision < 1 {
			return CompositionSnapshot{}, fmt.Errorf("%w: pinned composition snapshot identity is invalid", ErrInvalidInput)
		}
		if _, err := validateDigest("outcome audit digest", value.OutcomeAuditDigest); err != nil {
			return CompositionSnapshot{}, err
		}
		if value.Attention.OwnerIdentity == "" {
			return CompositionSnapshot{}, fmt.Errorf("%w: pinned attention snapshot owner is missing", ErrInvalidInput)
		}
		if !value.Attention.CapturedAt.Equal(value.CapturedAt) {
			return CompositionSnapshot{}, fmt.Errorf("%w: pinned attention capture time does not match composition snapshot", ErrInvalidInput)
		}
		if value.Attention.InputDigest == "" {
			return CompositionSnapshot{}, fmt.Errorf("%w: pinned attention input digest is missing", ErrInvalidInput)
		}
		if err := proactivity.VerifyEvaluationSnapshot(value.Attention.OwnerIdentity, value.Attention); err != nil {
			return CompositionSnapshot{}, fmt.Errorf("%w: pinned attention snapshot failed verification", ErrInvalidInput)
		}
	case CompositionSnapshotLegacyUnpinned:
		if value.OutcomeRevision != 0 || value.OutcomeAuditDigest != "" || value.Attention.OwnerIdentity != "" || value.Attention.InputDigest != "" {
			return CompositionSnapshot{}, fmt.Errorf("%w: legacy composition snapshot cannot claim pinned context", ErrInvalidInput)
		}
	default:
		return CompositionSnapshot{}, fmt.Errorf("%w: composition snapshot status is invalid", ErrInvalidInput)
	}
	expected, err := compositionSnapshotDigest(value)
	if err != nil || expected != value.SnapshotDigest {
		return CompositionSnapshot{}, fmt.Errorf("%w: composition snapshot integrity check failed", ErrInvalidInput)
	}
	return value, nil
}

func compositionSnapshotDigest(value CompositionSnapshot) (string, error) {
	value.SnapshotDigest = ""
	return exactDigest("composition_snapshot", value)
}

func legacyCompositionSnapshot(at time.Time) (CompositionSnapshot, error) {
	value := CompositionSnapshot{
		ContractVersion: compositionSnapshotVersion,
		Status:          CompositionSnapshotLegacyUnpinned,
		ComposerVersion: "ambient-monitor-composer/pre-0051-unknown",
		CapturedAt:      at,
	}
	var err error
	value.SnapshotDigest, err = compositionSnapshotDigest(value)
	if err != nil {
		return CompositionSnapshot{}, err
	}
	return validateCompositionSnapshot(value)
}

func validateCompositionAttempt(value CompositionAttempt) (CompositionAttempt, error) {
	if value.ContractVersion != ContractVersion || value.AttemptNumber < 1 || value.LeaseGeneration < 1 {
		return CompositionAttempt{}, fmt.Errorf("%w: composition attempt contract is invalid", ErrInvalidInput)
	}
	scope, err := validateScope(value.Scope)
	if err != nil {
		return CompositionAttempt{}, err
	}
	value.Scope = scope
	for name, item := range map[string]string{
		"attempt id": value.ID, "composition id": value.DeliveryID, "target id": value.TargetID,
		"run id": value.RunID, "worker id": value.WorkerID,
	} {
		if err := validateIdentifier(name, item); err != nil {
			return CompositionAttempt{}, err
		}
	}
	for name, item := range map[string]string{"run digest": value.RunDigest, "snapshot digest": value.SnapshotDigest, "request digest": value.RequestDigest, "record digest": value.RecordDigest} {
		if _, err := validateDigest(name, item); err != nil {
			return CompositionAttempt{}, err
		}
	}
	if value.StartedAt, err = validateTime("composition attempt start", value.StartedAt); err != nil {
		return CompositionAttempt{}, err
	}
	if value.FinishedAt, err = validateTime("composition attempt finish", value.FinishedAt); err != nil || value.FinishedAt.Before(value.StartedAt) {
		return CompositionAttempt{}, fmt.Errorf("%w: composition attempt time is invalid", ErrInvalidInput)
	}
	switch value.Status {
	case CompositionAttemptSucceeded:
		if value.FailureCode != "" {
			return CompositionAttempt{}, fmt.Errorf("%w: successful composition attempt has a failure", ErrInvalidInput)
		}
	case CompositionAttemptFailed:
		if err := validateIdentifier("composition failure code", value.FailureCode); err != nil {
			return CompositionAttempt{}, err
		}
	default:
		return CompositionAttempt{}, fmt.Errorf("%w: composition attempt status is invalid", ErrInvalidInput)
	}
	if err := validateAuthority(value.Authority); err != nil {
		return CompositionAttempt{}, err
	}
	expectedRequest, err := compositionAttemptRequestDigest(value)
	if err != nil || expectedRequest != value.RequestDigest {
		return CompositionAttempt{}, fmt.Errorf("%w: composition attempt request digest mismatch", ErrInvalidInput)
	}
	expectedRecord, err := compositionAttemptDigest(value)
	if err != nil || expectedRecord != value.RecordDigest {
		return CompositionAttempt{}, fmt.Errorf("%w: composition attempt record digest mismatch", ErrInvalidInput)
	}
	return value, nil
}

func compositionRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := compositionRetryBase
	for n := 1; n < attempt && delay < compositionRetryMaximum; n++ {
		delay *= 2
		if delay > compositionRetryMaximum {
			delay = compositionRetryMaximum
		}
	}
	return delay
}
