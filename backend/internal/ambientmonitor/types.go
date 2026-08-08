// Package ambientmonitor provides an owner-scoped, advisory-only scheduler for
// observing deterministic outcome indicators. It records what collectors see
// and may submit those records to downstream composition, but it cannot send,
// execute, schedule, authorize, mutate workflows, or change learned state.
package ambientmonitor

import (
	"context"
	"errors"
	"time"
)

const (
	ContractVersion = 1
	AuthorityLabel  = "advisory_monitor_only"
)

var (
	ErrInvalidInput          = errors.New("invalid ambient monitor input")
	ErrScopeViolation        = errors.New("ambient monitor owner or workspace scope violation")
	ErrNotFound              = errors.New("ambient monitor record not found")
	ErrIdempotencyConflict   = errors.New("ambient monitor idempotency conflict")
	ErrLeaseLost             = errors.New("ambient monitor lease is no longer owned")
	ErrRepositoryUnavailable = errors.New("ambient monitor repository unavailable")
	ErrCollectorUnavailable  = errors.New("ambient monitor collector unavailable")
	ErrCollectorFailed       = errors.New("ambient monitor collector failed")
	ErrSinkFailed            = errors.New("ambient monitor advisory sink failed")
	ErrSnapshotUnavailable   = errors.New("ambient monitor composition snapshot unavailable")
)

type SourceKind string

const (
	SourceWorkflowOpenLoopCount           SourceKind = "workflow_open_loop_count"
	SourceWorkflowVerifiedCompletionCount SourceKind = "workflow_verified_completion_count"
	SourceOverdueCommitmentCount          SourceKind = "overdue_commitment_count"
)

type RunStatus string

const (
	RunCompleted RunStatus = "completed"
	RunFailed    RunStatus = "failed"
)

// AuthorityControl is attached to every externally returned domain record.
// Every capability flag is intentionally false. The monitor supplies evidence
// to other governed components; it never grants operational authority.
type AuthorityControl struct {
	Label               string `json:"label"`
	CanExecute          bool   `json:"canExecute"`
	CanDeliver          bool   `json:"canDeliver"`
	CanNotify           bool   `json:"canNotify"`
	CanWriteCalendar    bool   `json:"canWriteCalendar"`
	CanMutateWorkflow   bool   `json:"canMutateWorkflow"`
	CanAuthorizeMandate bool   `json:"canAuthorizeMandate"`
	CanMutateLearning   bool   `json:"canMutateLearning"`
}

type Scope struct {
	OwnerID     string `json:"ownerId"`
	WorkspaceID string `json:"workspaceId"`
}

type Lease struct {
	WorkerID   string    `json:"workerId"`
	Generation uint64    `json:"generation"`
	ClaimedAt  time.Time `json:"claimedAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

func (l Lease) Active() bool {
	return l.WorkerID != "" && l.Generation > 0 && !l.ClaimedAt.IsZero() && !l.ExpiresAt.IsZero()
}

// MonitorTarget is mutable scheduling state scoped to one owner, workspace,
// intended outcome, and indicator. SourceKind is a closed enum; targets never
// contain SQL, scripts, expressions, URLs, or arbitrary tool instructions.
type MonitorTarget struct {
	ContractVersion int              `json:"contractVersion"`
	ID              string           `json:"id"`
	Scope           Scope            `json:"scope"`
	OutcomeID       string           `json:"outcomeId"`
	IndicatorID     string           `json:"indicatorId"`
	SourceKind      SourceKind       `json:"sourceKind"`
	Enabled         bool             `json:"enabled"`
	Cadence         time.Duration    `json:"cadence"`
	NextRunAt       time.Time        `json:"nextRunAt"`
	Lease           Lease            `json:"lease"`
	CreatedAt       time.Time        `json:"createdAt"`
	UpdatedAt       time.Time        `json:"updatedAt"`
	Authority       AuthorityControl `json:"authority"`
}

// CollectedObservation is the only value a Collector may return. The source
// digest must identify the deterministic source snapshot used for the count.
type CollectedObservation struct {
	Value        float64   `json:"value"`
	ObservedAt   time.Time `json:"observedAt"`
	SourceDigest string    `json:"sourceDigest"`
}

// ObservationRecord is immutable once accepted by Repository.Complete.
type ObservationRecord struct {
	ContractVersion int              `json:"contractVersion"`
	ID              string           `json:"id"`
	Scope           Scope            `json:"scope"`
	TargetID        string           `json:"targetId"`
	OutcomeID       string           `json:"outcomeId"`
	IndicatorID     string           `json:"indicatorId"`
	SourceKind      SourceKind       `json:"sourceKind"`
	Value           float64          `json:"value"`
	ObservedAt      time.Time        `json:"observedAt"`
	RecordedAt      time.Time        `json:"recordedAt"`
	SourceDigest    string           `json:"sourceDigest"`
	RecordDigest    string           `json:"recordDigest"`
	Authority       AuthorityControl `json:"authority"`
}

// MonitorRun is an immutable lease-fenced receipt for one collection attempt.
type MonitorRun struct {
	ContractVersion   int              `json:"contractVersion"`
	ID                string           `json:"id"`
	Scope             Scope            `json:"scope"`
	TargetID          string           `json:"targetId"`
	OutcomeID         string           `json:"outcomeId"`
	IndicatorID       string           `json:"indicatorId"`
	SourceKind        SourceKind       `json:"sourceKind"`
	LeaseGeneration   uint64           `json:"leaseGeneration"`
	Status            RunStatus        `json:"status"`
	StartedAt         time.Time        `json:"startedAt"`
	FinishedAt        time.Time        `json:"finishedAt"`
	ObservationID     string           `json:"observationId,omitempty"`
	ObservationDigest string           `json:"observationDigest,omitempty"`
	FailureCode       string           `json:"failureCode,omitempty"`
	FailureSummary    string           `json:"failureSummary,omitempty"`
	IdempotencyDigest string           `json:"idempotencyDigest"`
	RecordDigest      string           `json:"recordDigest"`
	Authority         AuthorityControl `json:"authority"`
}

type Completion struct {
	Observation ObservationRecord   `json:"observation"`
	Run         MonitorRun          `json:"run"`
	Composition CompositionDelivery `json:"composition"`
	Created     bool                `json:"created"`
	Composed    bool                `json:"composed"`
	Authority   AuthorityControl    `json:"authority"`
}

type AdvisorySignal struct {
	Observation ObservationRecord   `json:"observation"`
	Run         MonitorRun          `json:"run"`
	Snapshot    CompositionSnapshot `json:"snapshot"`
	Authority   AuthorityControl    `json:"authority"`
}

// CompositionResult may govern only the monitor's own lifecycle. It carries
// no authority over tasks, workflows, delivery, calendars, mandates, or
// learned state.
type CompositionResult struct {
	DisableTarget bool `json:"disableTarget"`
}

// Collector is a read-only adapter. Implementations must return the same
// numeric value, observation time, and source digest for the same source
// snapshot. It has no mutation or delivery methods by design.
type Collector interface {
	Collect(context.Context, MonitorTarget) (CollectedObservation, error)
}

// Sink accepts source-backed advisory evidence for outcome/proactivity
// composition. Implementations must use the run digest idempotently. This
// interface deliberately exposes no task, notification, calendar, workflow,
// mandate, or learning mutation operation.
type Sink interface {
	Compose(context.Context, AdvisorySignal) (CompositionResult, error)
}

// SnapshotProvider captures the exact immutable decision context before the
// collection transaction enqueues its composition delivery. Production sinks
// must implement this interface; legacy sinks remain readable but their queue
// rows are explicitly marked unpinned and cannot claim deterministic replay.
type SnapshotProvider interface {
	CaptureSnapshot(context.Context, AdvisorySignal) (CompositionSnapshot, error)
}

type RegisterTargetRequest struct {
	IdempotencyKey string        `json:"idempotencyKey"`
	Scope          Scope         `json:"scope"`
	TargetID       string        `json:"targetId"`
	OutcomeID      string        `json:"outcomeId"`
	IndicatorID    string        `json:"indicatorId"`
	SourceKind     SourceKind    `json:"sourceKind"`
	Enabled        bool          `json:"enabled"`
	Cadence        time.Duration `json:"cadence"`
	FirstRunAt     time.Time     `json:"firstRunAt"`
	RequestedAt    time.Time     `json:"requestedAt"`
}

type SetEnabledRequest struct {
	IdempotencyKey string    `json:"idempotencyKey"`
	Scope          Scope     `json:"scope"`
	TargetID       string    `json:"targetId"`
	Enabled        bool      `json:"enabled"`
	RequestedAt    time.Time `json:"requestedAt"`
}

type ClaimDueRequest struct {
	Scope         Scope         `json:"scope"`
	WorkerID      string        `json:"workerId"`
	Now           time.Time     `json:"now"`
	LeaseDuration time.Duration `json:"leaseDuration"`
	Limit         int           `json:"limit"`
}

type CompleteRequest struct {
	IdempotencyKey  string               `json:"idempotencyKey"`
	Scope           Scope                `json:"scope"`
	TargetID        string               `json:"targetId"`
	WorkerID        string               `json:"workerId"`
	LeaseGeneration uint64               `json:"leaseGeneration"`
	Collected       CollectedObservation `json:"collected"`
	CompletedAt     time.Time            `json:"completedAt"`
}

type FailRequest struct {
	IdempotencyKey  string    `json:"idempotencyKey"`
	Scope           Scope     `json:"scope"`
	TargetID        string    `json:"targetId"`
	WorkerID        string    `json:"workerId"`
	LeaseGeneration uint64    `json:"leaseGeneration"`
	FailureCode     string    `json:"failureCode"`
	FailureSummary  string    `json:"failureSummary"`
	FailedAt        time.Time `json:"failedAt"`
}

type ProcessClaimRequest struct {
	IdempotencyKey  string    `json:"idempotencyKey"`
	Scope           Scope     `json:"scope"`
	TargetID        string    `json:"targetId"`
	WorkerID        string    `json:"workerId"`
	LeaseGeneration uint64    `json:"leaseGeneration"`
	CompletedAt     time.Time `json:"completedAt"`
}

type ProcessDueRequest struct {
	Scope         Scope         `json:"scope"`
	WorkerID      string        `json:"workerId"`
	Now           time.Time     `json:"now"`
	LeaseDuration time.Duration `json:"leaseDuration"`
	Limit         int           `json:"limit"`
}

type ProcessFailure struct {
	TargetID string `json:"targetId"`
	Code     string `json:"code"`
}

type ProcessDueResult struct {
	Claimed      int                       `json:"claimed"`
	Completions  []Completion              `json:"completions"`
	Failures     []ProcessFailure          `json:"failures"`
	Compositions ProcessCompositionsResult `json:"compositions"`
	Authority    AuthorityControl          `json:"authority"`
}
