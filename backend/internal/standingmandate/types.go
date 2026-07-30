package standingmandate

import (
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusDraft   Status = "draft"
	StatusActive  Status = "active"
	StatusRevoked Status = "revoked"
)

type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

type ApprovalMode string

const (
	ApprovalNever             ApprovalMode = "never"
	ApprovalAlways            ApprovalMode = "always"
	ApprovalAtOrAboveAutonomy ApprovalMode = "at_or_above_autonomy"
	ApprovalForRiskOrAction   ApprovalMode = "for_risk_or_action"
)

type StopEffect string

const (
	StopDeny            StopEffect = "deny"
	StopRequireApproval StopEffect = "require_approval"
)

type StopOperator string

const (
	StopEquals         StopOperator = "equals"
	StopNotEquals      StopOperator = "not_equals"
	StopPresent        StopOperator = "present"
	StopAbsent         StopOperator = "absent"
	StopGreaterOrEqual StopOperator = "greater_or_equal"
	StopLessOrEqual    StopOperator = "less_or_equal"
)

type DecisionOutcome string

const (
	DecisionAuthorized       DecisionOutcome = "authorized"
	DecisionRequiresApproval DecisionOutcome = "requires_approval"
	DecisionDenied           DecisionOutcome = "denied"
)

// ResourceScope bounds a mandate to a resource type. An empty IDs list means
// all resources of that exact type; wildcard resource types and IDs are not
// accepted.
type ResourceScope struct {
	Type string   `json:"type"`
	IDs  []string `json:"ids,omitempty"`
}

// Scope is an AND of its populated fields. A mandate may contain multiple
// scopes, which are evaluated as OR alternatives.
type Scope struct {
	ID          string          `json:"id"`
	Actions     []string        `json:"actions"`
	Resources   []ResourceScope `json:"resources,omitempty"`
	Projects    []string        `json:"projects,omitempty"`
	Domains     []string        `json:"domains,omitempty"`
	Tools       []string        `json:"tools,omitempty"`
	MaximumRisk RiskLevel       `json:"maximumRisk,omitempty"`
}

type ApprovalPolicy struct {
	Mode                      ApprovalMode `json:"mode"`
	AutonomyThreshold         int          `json:"autonomyThreshold,omitempty"`
	RiskLevels                []RiskLevel  `json:"riskLevels,omitempty"`
	Actions                   []string     `json:"actions,omitempty"`
	ApproverRoles             []string     `json:"approverRoles,omitempty"`
	MaximumEvidenceAgeSeconds int          `json:"maximumEvidenceAgeSeconds,omitempty"`
}

// StopCondition is evaluated against ActionRequest.Facts. Missing required
// facts fail closed and trigger the configured effect.
type StopCondition struct {
	ID            string       `json:"id"`
	Description   string       `json:"description"`
	FactKey       string       `json:"factKey"`
	Operator      StopOperator `json:"operator"`
	ExpectedValue string       `json:"expectedValue,omitempty"`
	Required      bool         `json:"required"`
	Effect        StopEffect   `json:"effect"`
}

// StandingMandate is a durable, versioned grant of bounded authority. The
// Revision field supports optimistic concurrency in persistent repositories.
type StandingMandate struct {
	ID               uuid.UUID       `json:"id"`
	OwnerIdentity    string          `json:"ownerIdentity"`
	Name             string          `json:"name"`
	Purpose          string          `json:"purpose"`
	Status           Status          `json:"status"`
	Version          string          `json:"version"`
	Revision         uint64          `json:"revision"`
	Scopes           []Scope         `json:"scopes"`
	AutonomyCeiling  int             `json:"autonomyCeiling"`
	ApprovalPolicy   ApprovalPolicy  `json:"approvalPolicy"`
	StopConditions   []StopCondition `json:"stopConditions,omitempty"`
	SourceReferences []string        `json:"sourceReferences,omitempty"`
	CreatedBy        string          `json:"createdBy"`
	CreatedAt        time.Time       `json:"createdAt"`
	UpdatedAt        time.Time       `json:"updatedAt"`
	ActivatedAt      *time.Time      `json:"activatedAt,omitempty"`
	ExpiresAt        *time.Time      `json:"expiresAt,omitempty"`
	RevokedAt        *time.Time      `json:"revokedAt,omitempty"`
	RevokedBy        string          `json:"revokedBy,omitempty"`
	RevocationReason string          `json:"revocationReason,omitempty"`
}

type CreateRequest struct {
	OwnerIdentity    string
	Name             string
	Purpose          string
	Version          string
	Scopes           []Scope
	AutonomyCeiling  int
	ApprovalPolicy   ApprovalPolicy
	StopConditions   []StopCondition
	SourceReferences []string
	CreatedBy        string
	ExpiresAt        *time.Time
}

type ActionRequest struct {
	OwnerIdentity            string            `json:"ownerIdentity"`
	ActorIdentity            string            `json:"actorIdentity"`
	Action                   string            `json:"action"`
	ResourceType             string            `json:"resourceType"`
	ResourceID               string            `json:"resourceId,omitempty"`
	ProjectKey               string            `json:"projectKey,omitempty"`
	Domain                   string            `json:"domain,omitempty"`
	ToolID                   string            `json:"toolId,omitempty"`
	Risk                     RiskLevel         `json:"risk"`
	RequestedAutonomy        int               `json:"requestedAutonomy"`
	UpstreamApprovalRequired bool              `json:"upstreamApprovalRequired"`
	Facts                    map[string]string `json:"facts,omitempty"`
	Approval                 *ApprovalEvidence `json:"approval,omitempty"`
	SourceReferences         []string          `json:"sourceReferences,omitempty"`
	RequestedAt              time.Time         `json:"requestedAt"`
}

// ApprovalEvidence is an action-bound approval fact. Signature validation and
// single-use consumption remain the responsibility of the integration layer.
type ApprovalEvidence struct {
	ID            string    `json:"id"`
	ApprovedBy    string    `json:"approvedBy"`
	ApproverRoles []string  `json:"approverRoles,omitempty"`
	ActionDigest  string    `json:"actionDigest"`
	ApprovedAt    time.Time `json:"approvedAt"`
	ExpiresAt     time.Time `json:"expiresAt"`
	Revoked       bool      `json:"revoked"`
	Source        string    `json:"source"`
}

type DecisionTrace struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type TriggeredStop struct {
	ConditionID string     `json:"conditionId"`
	Effect      StopEffect `json:"effect"`
	Reason      string     `json:"reason"`
}

// DecisionEvidence is sufficient to reproduce and audit the policy decision.
// The digest covers the normalized request, mandate snapshot, and result.
type DecisionEvidence struct {
	RequestDigest      string          `json:"requestDigest"`
	MandateDigest      string          `json:"mandateDigest"`
	DecisionDigest     string          `json:"decisionDigest"`
	MandateRevision    uint64          `json:"mandateRevision"`
	MatchedScopeIDs    []string        `json:"matchedScopeIds,omitempty"`
	TriggeredStops     []TriggeredStop `json:"triggeredStops,omitempty"`
	ApprovalEvidenceID string          `json:"approvalEvidenceId,omitempty"`
	SourceReferences   []string        `json:"sourceReferences,omitempty"`
	Trace              []DecisionTrace `json:"trace"`
}

type AuthorizationDecision struct {
	ID                uuid.UUID        `json:"id"`
	MandateID         uuid.UUID        `json:"mandateId"`
	OwnerIdentity     string           `json:"ownerIdentity"`
	ActorIdentity     string           `json:"actorIdentity"`
	Action            string           `json:"action"`
	Outcome           DecisionOutcome  `json:"outcome"`
	Reason            string           `json:"reason"`
	EffectiveAutonomy int              `json:"effectiveAutonomy"`
	ApprovalRequired  bool             `json:"approvalRequired"`
	ApprovalSatisfied bool             `json:"approvalSatisfied"`
	EvaluatedAt       time.Time        `json:"evaluatedAt"`
	Evidence          DecisionEvidence `json:"evidence"`
}
