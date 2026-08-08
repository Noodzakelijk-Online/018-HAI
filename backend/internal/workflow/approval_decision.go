package workflow

import (
	"context"
	"time"
)

// ApprovalDecisionRecord is the immutable projection of one workflow decision
// and its owning workflow. It intentionally contains only authority evidence
// needed by the execution-approval boundary.
type ApprovalDecisionRecord struct {
	DecisionID    string
	WorkflowID    string
	OwnerIdentity string
	DecisionType  string
	Decision      string
	Reason        string
	ActionBinding string
	Approved      bool
	Actor         string
	CreatedAt     time.Time
}

// ApprovalDecisionRepository resolves one decision by its durable identifier
// while applying owner identity inside the storage lookup.
type ApprovalDecisionRepository interface {
	FindApprovalDecisionForOwner(
		context.Context,
		string,
		string,
	) (*ApprovalDecisionRecord, error)
}

var _ ApprovalDecisionRepository = (*GormRepository)(nil)
