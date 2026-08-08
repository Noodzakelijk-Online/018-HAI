package controlledlearning

import (
	"context"
	"time"
)

const applicationLeaseDuration = 2 * time.Minute

type ApplicationMode string

const (
	ApplicationModeApply            ApplicationMode = "apply"
	ApplicationModeProtectedHandoff ApplicationMode = "protected_handoff"
)

type ApplicationStatus string

const (
	ApplicationApplying         ApplicationStatus = "applying"
	ApplicationApplied          ApplicationStatus = "applied"
	ApplicationHandoffPending   ApplicationStatus = "handoff_pending"
	ApplicationHandoffReady     ApplicationStatus = "handoff_ready"
	ApplicationFailed           ApplicationStatus = "failed"
	ApplicationRollbackApplying ApplicationStatus = "rollback_applying"
	ApplicationRolledBack       ApplicationStatus = "rolled_back"
	ApplicationRollbackFailed   ApplicationStatus = "rollback_failed"
)

type ApplicationEventKind string

const (
	ApplicationEventReserved        ApplicationEventKind = "reserved"
	ApplicationEventAttemptStarted  ApplicationEventKind = "attempt_started"
	ApplicationEventApplied         ApplicationEventKind = "applied"
	ApplicationEventHandoffReady    ApplicationEventKind = "handoff_ready"
	ApplicationEventFailed          ApplicationEventKind = "failed"
	ApplicationEventRollbackStarted ApplicationEventKind = "rollback_started"
	ApplicationEventRolledBack      ApplicationEventKind = "rolled_back"
	ApplicationEventRollbackFailed  ApplicationEventKind = "rollback_failed"
)

type ApplicationEvidence struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	URI        string    `json:"uri"`
	Digest     string    `json:"digest"`
	RecordedAt time.Time `json:"recordedAt"`
}

type ApplicationRecord struct {
	ID                   string                `json:"id"`
	ProtocolVersion      string                `json:"protocolVersion"`
	OwnerIdentity        string                `json:"ownerIdentity"`
	ProposalID           string                `json:"proposalId"`
	ProposalRevision     int64                 `json:"proposalRevision"`
	ProposalDigest       string                `json:"proposalDigest"`
	DecisionID           string                `json:"decisionId,omitempty"`
	DecisionDigest       string                `json:"decisionDigest,omitempty"`
	IdempotencyKey       string                `json:"idempotencyKey"`
	IntentDigest         string                `json:"intentDigest"`
	Mode                 ApplicationMode       `json:"mode"`
	Status               ApplicationStatus     `json:"status"`
	Target               TargetKind            `json:"target"`
	ProtectedTarget      bool                  `json:"protectedTarget"`
	ApplierID            string                `json:"applierId"`
	CurrentVersion       string                `json:"currentVersion"`
	ProposedVersion      string                `json:"proposedVersion"`
	AppliedVersion       string                `json:"appliedVersion,omitempty"`
	RollbackPlan         string                `json:"rollbackPlan"`
	RollbackToken        string                `json:"rollbackToken,omitempty"`
	RollbackIntentDigest string                `json:"rollbackIntentDigest,omitempty"`
	RestoredVersion      string                `json:"restoredVersion,omitempty"`
	GovernanceReference  string                `json:"governanceReference,omitempty"`
	HandoffReference     string                `json:"handoffReference,omitempty"`
	Evidence             []ApplicationEvidence `json:"evidence,omitempty"`
	RollbackEvidence     []ApplicationEvidence `json:"rollbackEvidence,omitempty"`
	Attempt              int                   `json:"attempt"`
	LeaseExpiresAt       time.Time             `json:"leaseExpiresAt,omitempty"`
	LastErrorCode        string                `json:"lastErrorCode,omitempty"`
	DefinitionDigest     string                `json:"definitionDigest"`
	ResultDigest         string                `json:"resultDigest,omitempty"`
	CreatedAt            time.Time             `json:"createdAt"`
	UpdatedAt            time.Time             `json:"updatedAt"`
	CompletedAt          time.Time             `json:"completedAt,omitempty"`
	RolledBackAt         time.Time             `json:"rolledBackAt,omitempty"`
}

type ApplicationEvent struct {
	ID                string                `json:"id"`
	ApplicationID     string                `json:"applicationId"`
	OwnerIdentity     string                `json:"ownerIdentity"`
	ProposalID        string                `json:"proposalId"`
	Attempt           int                   `json:"attempt"`
	Kind              ApplicationEventKind  `json:"kind"`
	Status            ApplicationStatus     `json:"status"`
	Version           string                `json:"version,omitempty"`
	Reference         string                `json:"reference,omitempty"`
	ErrorCode         string                `json:"errorCode,omitempty"`
	Evidence          []ApplicationEvidence `json:"evidence,omitempty"`
	ApplicationDigest string                `json:"applicationDigest"`
	EventDigest       string                `json:"eventDigest"`
	OccurredAt        time.Time             `json:"occurredAt"`
}

type ApplicationQuery struct {
	OwnerIdentity string
	ProposalID    string
	Status        ApplicationStatus
	Limit         int
}

type PromotionRequest struct {
	ApplicationID   string
	IdempotencyKey  string
	OwnerIdentity   string
	ProposalID      string
	ProposalDigest  string
	Target          TargetKind
	CurrentVersion  string
	ProposedVersion string
	ProposedChange  string
	RollbackPlan    string
	EvidenceIDs     []string
}

type PromotionResult struct {
	AppliedVersion string
	RollbackToken  string
	Evidence       []ApplicationEvidence
}

type ProtectedHandoffRequest struct {
	ApplicationID       string
	IdempotencyKey      string
	OwnerIdentity       string
	ProposalID          string
	ProposalDigest      string
	Target              TargetKind
	CurrentVersion      string
	ProposedVersion     string
	ProposedChange      string
	RollbackPlan        string
	EvidenceIDs         []string
	GovernanceReference string
}

type ProtectedHandoffResult struct {
	HandoffReference string
	Evidence         []ApplicationEvidence
}

type PromotionRollbackRequest struct {
	ApplicationID  string
	IdempotencyKey string
	OwnerIdentity  string
	ProposalID     string
	Target         TargetKind
	AppliedVersion string
	RestoreVersion string
	RollbackPlan   string
	RollbackToken  string
}

type PromotionRollbackResult struct {
	RestoredVersion string
	Evidence        []ApplicationEvidence
}

// ProposalPromoter is the only boundary allowed to turn an approved learning
// proposal into effective behavior. Implementations must honor the supplied
// idempotency key: retrying a request may return the original result, but must
// not apply or roll back the same change twice.
//
// HandoffProtected must create review work only. It must never apply the
// protected proposal or weaken its independent governance gate.
type ProposalPromoter interface {
	ID() string
	Apply(context.Context, PromotionRequest) (PromotionResult, error)
	HandoffProtected(context.Context, ProtectedHandoffRequest) (ProtectedHandoffResult, error)
	Rollback(context.Context, PromotionRollbackRequest) (PromotionRollbackResult, error)
}

type ApplicationRepository interface {
	AcquireApplication(context.Context, ApplicationRecord) (ApplicationRecord, bool, error)
	CompleteApplication(
		context.Context,
		string,
		string,
		int,
		ReviewDecision,
		ProposalStatus,
		ApplicationCompletion,
	) (LearningProposal, ApplicationRecord, error)
	FailApplication(context.Context, string, string, int, string, time.Time) (ApplicationRecord, error)
	GetApplication(context.Context, string, string) (ApplicationRecord, error)
	GetProposalApplication(context.Context, string, string, int64, ApplicationMode) (ApplicationRecord, error)
	ListApplications(context.Context, ApplicationQuery) ([]ApplicationRecord, error)
	ListApplicationEvents(context.Context, string, string) ([]ApplicationEvent, error)
	AcquireRollback(context.Context, string, string, string, time.Time, time.Time) (ApplicationRecord, bool, error)
	CompleteRollback(
		context.Context,
		string,
		string,
		int,
		PromotionRollbackResult,
		time.Time,
	) (ApplicationRecord, error)
	FailRollback(context.Context, string, string, int, string, time.Time) (ApplicationRecord, error)
}

type ApplicationCompletion struct {
	Status           ApplicationStatus
	AppliedVersion   string
	RollbackToken    string
	HandoffReference string
	Evidence         []ApplicationEvidence
	ResultDigest     string
	CompletedAt      time.Time
}

type DecisionResult struct {
	Proposal    LearningProposal   `json:"proposal"`
	Application *ApplicationRecord `json:"application,omitempty"`
}

type RollbackRequest struct {
	OwnerIdentity   string
	ApplicationID   string
	IdempotencyKey  string
	ActorIdentity   string
	HumanConfirmed  bool
	Rationale       string
	ExpectedVersion string
}
