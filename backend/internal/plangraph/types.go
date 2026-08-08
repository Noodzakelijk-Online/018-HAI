package plangraph

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusDraft    Status = "draft"
	StatusAccepted Status = "accepted"
)

type NodeStatus string

const (
	NodePlanned       NodeStatus = "planned"
	NodeReady         NodeStatus = "ready"
	NodeBlocked       NodeStatus = "blocked"
	NodeWaiting       NodeStatus = "waiting"
	NodeNeedsApproval NodeStatus = "needs_approval"
	NodeCompleted     NodeStatus = "completed"
	NodeFailed        NodeStatus = "failed"
)

type Risk string

const (
	RiskLow    Risk = "low"
	RiskMedium Risk = "medium"
	RiskHigh   Risk = "high"
)

type ApprovalState string

const (
	ApprovalNotRequired ApprovalState = "not_required"
	ApprovalRequired    ApprovalState = "required"
	ApprovalGranted     ApprovalState = "granted"
	ApprovalRejected    ApprovalState = "rejected"
)

// Bindings preserve the operational records a plan node was derived from.
// They do not authorize execution against any of those records.
type Bindings struct {
	PursuitID  string `json:"pursuitId,omitempty"`
	WorkflowID string `json:"workflowId,omitempty"`
	TaskID     string `json:"taskId,omitempty"`
	AgentID    string `json:"agentId,omitempty"`
}

type Node struct {
	ID               string        `json:"id"`
	Type             string        `json:"type"`
	Title            string        `json:"title"`
	Owner            string        `json:"owner"`
	Status           NodeStatus    `json:"status"`
	EstimatedMinutes int           `json:"estimatedMinutes"`
	EstimatedCostEUR float64       `json:"estimatedCostEur"`
	EarliestStart    *time.Time    `json:"earliestStart,omitempty"`
	Deadline         *time.Time    `json:"deadline,omitempty"`
	Risk             Risk          `json:"risk"`
	ApprovalState    ApprovalState `json:"approvalState"`
	FrameworkDigest  string        `json:"frameworkDigest,omitempty"`
	EvidenceDigest   string        `json:"evidenceDigest,omitempty"`
	Bindings         Bindings      `json:"bindings"`
}

type Edge struct {
	ID         string `json:"id"`
	From       string `json:"from"`
	To         string `json:"to"`
	Type       string `json:"type"`
	LagMinutes int    `json:"lagMinutes,omitempty"`
}

type RepairProvenance struct {
	Reason           string    `json:"reason"`
	Trigger          string    `json:"trigger"`
	PreviousRevision uint64    `json:"previousRevision"`
	PreviousDigest   string    `json:"previousDigest"`
	CreatedBy        string    `json:"createdBy"`
	CreatedAt        time.Time `json:"createdAt"`
}

// AcceptedRevisionReference identifies one exact node in one immutable plan
// revision. It is planning provenance only and never grants tool or runtime
// authority.
type AcceptedRevisionReference struct {
	PlanID   uuid.UUID `json:"planId"`
	Revision uint64    `json:"revision"`
	Digest   string    `json:"digest"`
	NodeID   string    `json:"nodeId"`
}

func (reference AcceptedRevisionReference) IsZero() bool {
	return reference.PlanID == uuid.Nil && reference.Revision == 0 &&
		strings.TrimSpace(reference.Digest) == "" && strings.TrimSpace(reference.NodeID) == ""
}

// AcceptedRevisionBinding is the verified, owner-scoped result of resolving a
// reference. CanExecute is deliberately always false.
type AcceptedRevisionBinding struct {
	PlanID     uuid.UUID `json:"planId"`
	Revision   uint64    `json:"revision"`
	Digest     string    `json:"digest"`
	NodeID     string    `json:"nodeId"`
	PlanTitle  string    `json:"planTitle"`
	Node       Node      `json:"node"`
	Nodes      []Node    `json:"nodes"`
	Edges      []Edge    `json:"edges"`
	AcceptedAt time.Time `json:"acceptedAt"`
	CanExecute bool      `json:"canExecute"`
}

// Plan is one immutable revision. A plan is advisory: CanExecute is always
// false and accepting it never grants runtime or tool authority.
type Plan struct {
	ID             uuid.UUID         `json:"id"`
	OwnerIdentity  string            `json:"-"`
	Title          string            `json:"title"`
	Status         Status            `json:"status"`
	Revision       uint64            `json:"revision"`
	Digest         string            `json:"digest"`
	ParentRevision uint64            `json:"parentRevision,omitempty"`
	ParentDigest   string            `json:"parentDigest,omitempty"`
	IdempotencyKey string            `json:"idempotencyKey,omitempty"`
	RequestDigest  string            `json:"requestDigest,omitempty"`
	Nodes          []Node            `json:"nodes"`
	Edges          []Edge            `json:"edges"`
	Repair         *RepairProvenance `json:"repair,omitempty"`
	CreatedBy      string            `json:"createdBy"`
	CreatedAt      time.Time         `json:"createdAt"`
	AcceptedAt     *time.Time        `json:"acceptedAt,omitempty"`
	CanExecute     bool              `json:"canExecute"`
}

type PreviewRequest struct {
	PlanID         uuid.UUID `json:"planId,omitempty"`
	IdempotencyKey string    `json:"idempotencyKey"`
	Title          string    `json:"title"`
	Nodes          []Node    `json:"nodes"`
	Edges          []Edge    `json:"edges"`
	CreatedBy      string    `json:"createdBy"`
}

type AcceptRequest struct {
	ExpectedRevision uint64 `json:"expectedRevision"`
	ExpectedDigest   string `json:"expectedDigest"`
	AcceptedBy       string `json:"acceptedBy"`
}

type ReplanRequest struct {
	ExpectedRevision uint64 `json:"expectedRevision"`
	ExpectedDigest   string `json:"expectedDigest"`
	IdempotencyKey   string `json:"idempotencyKey"`
	Title            string `json:"title"`
	Nodes            []Node `json:"nodes"`
	Edges            []Edge `json:"edges"`
	Reason           string `json:"reason"`
	Trigger          string `json:"trigger"`
	CreatedBy        string `json:"createdBy"`
}
