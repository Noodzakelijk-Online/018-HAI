// Package executioncontract defines a transport-neutral, versioned boundary
// for governed execution attempts. A valid envelope describes authority and
// scope; it does not itself grant approval or prove that execution occurred.
package executioncontract

import "time"

const (
	CurrentSchemaVersion = "hai.execution-contract/v1"
	RedactedValue        = "[redacted]"
)

type ExecutionMode string

const (
	ModePlanOnly  ExecutionMode = "plan_only"
	ModeRecommend ExecutionMode = "recommend"
	ModeDraft     ExecutionMode = "draft"
	ModeExecute   ExecutionMode = "execute"
)

type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

type ResourceOperation string

const (
	OperationRead          ResourceOperation = "read"
	OperationCreate        ResourceOperation = "create"
	OperationUpdate        ResourceOperation = "update"
	OperationDelete        ResourceOperation = "delete"
	OperationExecute       ResourceOperation = "execute"
	OperationSend          ResourceOperation = "send"
	OperationPublish       ResourceOperation = "publish"
	OperationTransact      ResourceOperation = "transact"
	OperationAccountChange ResourceOperation = "account_change"
)

type ActionScope struct {
	Name                string        `json:"name"`
	Purpose             string        `json:"purpose"`
	Mode                ExecutionMode `json:"mode"`
	Risk                RiskLevel     `json:"risk"`
	RequiresApproval    bool          `json:"requiresApproval"`
	AllowedTools        []string      `json:"allowedTools,omitempty"`
	ProhibitedActions   []string      `json:"prohibitedActions"`
	ExpectedSideEffects []string      `json:"expectedSideEffects,omitempty"`
}

// ResourceScope grants only the listed operations against one exact resource
// or one explicitly named resource prefix. Wildcards are never valid.
type ResourceScope struct {
	Kind       string              `json:"kind"`
	Identifier string              `json:"identifier"`
	Operations []ResourceOperation `json:"operations"`
}

type PolicyReference struct {
	ID             string `json:"id"`
	Version        string `json:"version"`
	DecisionID     string `json:"decisionId"`
	DecisionDigest string `json:"decisionDigest"`
}

// ApprovalReference points to approval evidence held by an external approval
// service. This package validates shape and expiry, not authenticity or use.
type ApprovalReference struct {
	ID          string    `json:"id"`
	GrantedBy   string    `json:"grantedBy"`
	ScopeDigest string    `json:"scopeDigest"`
	GrantedAt   time.Time `json:"grantedAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

type EvidenceKind string

const (
	EvidenceSource       EvidenceKind = "source"
	EvidenceSchema       EvidenceKind = "schema"
	EvidenceTest         EvidenceKind = "test"
	EvidenceCalculation  EvidenceKind = "calculation"
	EvidenceHumanReview  EvidenceKind = "human_review"
	EvidenceExecution    EvidenceKind = "execution_receipt"
	EvidenceVerification EvidenceKind = "verification"
)

type EvidenceRequirement struct {
	ID           string       `json:"id"`
	Kind         EvidenceKind `json:"kind"`
	Description  string       `json:"description"`
	MinimumCount int          `json:"minimumCount"`
	Verifier     string       `json:"verifier"`
	Required     bool         `json:"required"`
}

type SourceProvenance struct {
	SourceID      string    `json:"sourceId"`
	SourceType    string    `json:"sourceType"`
	SourceVersion string    `json:"sourceVersion"`
	URI           string    `json:"uri,omitempty"`
	ContentDigest string    `json:"contentDigest"`
	RetrievedAt   time.Time `json:"retrievedAt"`
	Authority     string    `json:"authority"`
}

// Envelope binds an execution attempt to owner, scope, policy, approval, and
// evidence constraints. ContractDigest covers every field except itself.
type Envelope struct {
	SchemaVersion        string                `json:"schemaVersion"`
	ContractDigest       string                `json:"contractDigest"`
	OwnerID              string                `json:"ownerId"`
	RunID                string                `json:"runId"`
	AttemptID            string                `json:"attemptId"`
	ParentAttemptID      string                `json:"parentAttemptId,omitempty"`
	ParentContractDigest string                `json:"parentContractDigest,omitempty"`
	AttemptNumber        uint32                `json:"attemptNumber"`
	CorrelationID        string                `json:"correlationId"`
	IdempotencyKey       string                `json:"idempotencyKey"`
	TraceID              string                `json:"traceId"`
	CreatedAt            time.Time             `json:"createdAt"`
	Deadline             time.Time             `json:"deadline"`
	Action               ActionScope           `json:"action"`
	Resources            []ResourceScope       `json:"resources,omitempty"`
	PolicyReferences     []PolicyReference     `json:"policyReferences"`
	ApprovalReferences   []ApprovalReference   `json:"approvalReferences,omitempty"`
	AutonomyCeiling      int                   `json:"autonomyCeiling"`
	EvidenceRequirements []EvidenceRequirement `json:"evidenceRequirements"`
	SourceProvenance     []SourceProvenance    `json:"sourceProvenance"`
	RedactedMetadata     map[string]string     `json:"redactedMetadata,omitempty"`
}

// ChildAttempt describes the values that must be new for a child execution
// attempt and the optional restrictions it adds to the parent contract.
type ChildAttempt struct {
	AttemptID            string
	IdempotencyKey       string
	CreatedAt            time.Time
	Deadline             time.Time
	Action               *ActionScope
	Resources            []ResourceScope
	AutonomyCeiling      *int
	EvidenceRequirements []EvidenceRequirement
	SourceProvenance     []SourceProvenance
	RedactedMetadata     map[string]string
}

// SafeLogProjection intentionally excludes raw owner IDs, resource
// identifiers, source URIs, approval actors, and provenance payloads.
type SafeLogProjection struct {
	SchemaVersion      string              `json:"schemaVersion"`
	ContractDigest     string              `json:"contractDigest"`
	OwnerRef           string              `json:"ownerRef"`
	RunID              string              `json:"runId"`
	AttemptID          string              `json:"attemptId"`
	ParentAttemptID    string              `json:"parentAttemptId,omitempty"`
	AttemptNumber      uint32              `json:"attemptNumber"`
	CorrelationID      string              `json:"correlationId"`
	TraceID            string              `json:"traceId"`
	Deadline           time.Time           `json:"deadline"`
	Action             string              `json:"action"`
	Mode               ExecutionMode       `json:"mode"`
	Risk               RiskLevel           `json:"risk"`
	AutonomyCeiling    int                 `json:"autonomyCeiling"`
	ResourceKinds      []string            `json:"resourceKinds,omitempty"`
	ResourceOperations []ResourceOperation `json:"resourceOperations,omitempty"`
	PolicyIDs          []string            `json:"policyIds"`
	ApprovalCount      int                 `json:"approvalCount"`
	EvidenceIDs        []string            `json:"evidenceIds"`
	SourceIDs          []string            `json:"sourceIds"`
	Metadata           map[string]string   `json:"metadata,omitempty"`
}
