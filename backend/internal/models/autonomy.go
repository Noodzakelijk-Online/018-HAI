package models

import (
	"time"

	"github.com/google/uuid"
)

type AutonomyWorldState struct {
	ID                uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	WorkflowID        uuid.UUID `gorm:"type:uuid;index;not null" json:"workflowId"`
	Attempt           int       `gorm:"index;not null" json:"attempt"`
	ObservationType   string    `gorm:"type:varchar(80);index;not null" json:"observationType"`
	State             string    `gorm:"type:varchar(80);index;not null" json:"state"`
	Snapshot          string    `gorm:"type:text;not null" json:"snapshot"`
	Confidence        float64   `json:"confidence"`
	Uncertainty       float64   `json:"uncertainty"`
	SourceRevision    string    `gorm:"type:varchar(64);index" json:"sourceRevision,omitempty"`
	ObservedAt        time.Time `gorm:"index;not null" json:"observedAt"`
	StaleAfter        time.Time `gorm:"index;not null" json:"staleAfter"`
	Partial           bool      `gorm:"index" json:"partial"`
	RequiresReobserve bool      `gorm:"index" json:"requiresReobserve"`
	CreatedAt         time.Time `json:"createdAt"`
}

type AutonomyActionTrace struct {
	ID                  uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	WorkflowID          uuid.UUID  `gorm:"type:uuid;index;not null" json:"workflowId"`
	WorldStateID        *uuid.UUID `gorm:"type:uuid;index" json:"worldStateId,omitempty"`
	Attempt             int        `gorm:"index;not null" json:"attempt"`
	InterfaceType       string     `gorm:"type:varchar(80);index;not null" json:"interfaceType"`
	ActionType          string     `gorm:"type:varchar(120);index;not null" json:"actionType"`
	ActionPayload       string     `gorm:"type:text" json:"actionPayload,omitempty"`
	Status              string     `gorm:"type:varchar(50);index;not null" json:"status"`
	PolicyDecision      string     `gorm:"type:varchar(50);index;not null" json:"policyDecision"`
	PolicyReason        string     `gorm:"type:text" json:"policyReason,omitempty"`
	RequiresApproval    bool       `gorm:"index" json:"requiresApproval"`
	ApprovalRecorded    bool       `gorm:"index" json:"approvalRecorded"`
	ExecutionVerified   bool       `gorm:"index" json:"executionVerified"`
	VerificationStatus  string     `gorm:"type:varchar(80);index" json:"verificationStatus,omitempty"`
	ExternalSideEffects bool       `gorm:"index" json:"externalSideEffects"`
	StartedAt           time.Time  `gorm:"index;not null" json:"startedAt"`
	CompletedAt         *time.Time `gorm:"index" json:"completedAt,omitempty"`
	LatencyMilliseconds int64      `json:"latencyMilliseconds"`
	ResultSummary       string     `gorm:"type:text" json:"resultSummary,omitempty"`
	CreatedAt           time.Time  `json:"createdAt"`
	UpdatedAt           time.Time  `json:"updatedAt"`
}

type AutonomyEvaluation struct {
	ID                        uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	WorkflowID                uuid.UUID `gorm:"type:uuid;index;not null" json:"workflowId"`
	ActionTraceID             uuid.UUID `gorm:"type:uuid;index;not null" json:"actionTraceId"`
	Attempt                   int       `gorm:"index;not null" json:"attempt"`
	RawCompletion             bool      `gorm:"index" json:"rawCompletion"`
	ExecutionBasedCorrectness bool      `gorm:"index" json:"executionBasedCorrectness"`
	CompletionUnderPolicy     bool      `gorm:"index" json:"completionUnderPolicy"`
	PartialCompletion         bool      `gorm:"index" json:"partialCompletion"`
	PolicyCompliant           bool      `gorm:"index" json:"policyCompliant"`
	RiskViolation             bool      `gorm:"index" json:"riskViolation"`
	InvalidAction             bool      `gorm:"index" json:"invalidAction"`
	HumanIntervention         bool      `gorm:"index" json:"humanIntervention"`
	RecoveryAttempt           bool      `gorm:"index" json:"recoveryAttempt"`
	Recovered                 bool      `gorm:"index" json:"recovered"`
	RetryCount                int       `json:"retryCount"`
	LatencyMilliseconds       int64     `json:"latencyMilliseconds"`
	FailureMode               string    `gorm:"type:varchar(120);index" json:"failureMode,omitempty"`
	CreatedAt                 time.Time `gorm:"index" json:"createdAt"`
}

type AutonomyStressRun struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	Passed    int       `json:"passed"`
	Failed    int       `json:"failed"`
	Results   string    `gorm:"type:text;not null" json:"results"`
	CreatedAt time.Time `gorm:"index" json:"createdAt"`
}
