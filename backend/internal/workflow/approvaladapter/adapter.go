package approvaladapter

import (
	"context"
	"errors"
	"reflect"

	"automation-hub-backend/internal/executionapproval"
	"automation-hub-backend/internal/workflow"
)

var ErrRepositoryRequired = errors.New("workflow approval decision repository is required")

// Repository adapts workflow's cycle-free storage projection to the execution
// approval resolver contract.
type Repository struct {
	repository workflow.ApprovalDecisionRepository
}

var _ executionapproval.WorkflowApprovalDecisionRepository = (*Repository)(nil)

func New(
	repository workflow.ApprovalDecisionRepository,
) (*Repository, error) {
	if isNilRepository(repository) {
		return nil, ErrRepositoryRequired
	}
	return &Repository{repository: repository}, nil
}

func (r *Repository) FindWorkflowApprovalDecision(
	ctx context.Context,
	ownerIdentity string,
	decisionID string,
) (*executionapproval.WorkflowApprovalDecisionRecord, error) {
	if r == nil || isNilRepository(r.repository) {
		return nil, ErrRepositoryRequired
	}
	record, err := r.repository.FindApprovalDecisionForOwner(
		ctx,
		ownerIdentity,
		decisionID,
	)
	if err != nil || record == nil {
		return nil, err
	}
	return &executionapproval.WorkflowApprovalDecisionRecord{
		DecisionID:    record.DecisionID,
		WorkflowID:    record.WorkflowID,
		OwnerIdentity: record.OwnerIdentity,
		DecisionType:  record.DecisionType,
		Decision:      record.Decision,
		Reason:        record.Reason,
		ActionBinding: record.ActionBinding,
		Approved:      record.Approved,
		Actor:         record.Actor,
		CreatedAt:     record.CreatedAt,
	}, nil
}

func isNilRepository(repository workflow.ApprovalDecisionRepository) bool {
	if repository == nil {
		return true
	}
	value := reflect.ValueOf(repository)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
