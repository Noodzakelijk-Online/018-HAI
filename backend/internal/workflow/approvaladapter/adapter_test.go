package approvaladapter

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/executionapproval"
	"automation-hub-backend/internal/workflow"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestAdapterMakesWorkflowRepositoryUsableByApprovalResolver(t *testing.T) {
	digest := strings.Repeat("a", 64)
	decisionID := uuid.NewString()
	workflowID := uuid.NewString()
	storage := &stubRepository{
		records: map[string]workflow.ApprovalDecisionRecord{
			decisionID: {
				DecisionID:    decisionID,
				WorkflowID:    workflowID,
				OwnerIdentity: "alice",
				DecisionType:  "approval",
				Decision:      "approved",
				Reason:        "Alice approved the exact action",
				ActionBinding: "automation-action:automation.script.execute:" + digest,
				Approved:      true,
				Actor:         "alice",
				CreatedAt:     time.Now().UTC(),
			},
		},
	}
	repository, err := New(storage)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	resolver, err := executionapproval.NewWorkflowApprovalResolver(repository)
	if err != nil {
		t.Fatalf("NewWorkflowApprovalResolver: %v", err)
	}

	approval, err := resolver.Resolve(
		context.Background(),
		"alice",
		"workflow-decision:"+decisionID,
		digest,
	)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if approval.DecisionID != decisionID ||
		approval.ApprovedBy != "alice" ||
		approval.BindingDigest != digest {
		t.Fatalf("resolved approval = %#v", approval)
	}
	if storage.calls != 1 ||
		storage.lastOwner != "alice" ||
		storage.lastDecisionID != decisionID {
		t.Fatalf("storage lookup = calls %d, owner %q, decision %q",
			storage.calls,
			storage.lastOwner,
			storage.lastDecisionID,
		)
	}
}

func TestAdapterAndResolverRejectCrossOwnerInventedAndRejectedDecisions(t *testing.T) {
	digest := strings.Repeat("b", 64)
	approvedID := uuid.NewString()
	rejectedID := uuid.NewString()
	storage := &stubRepository{
		records: map[string]workflow.ApprovalDecisionRecord{
			approvedID: approvalRecord(approvedID, "alice", digest, true),
			rejectedID: approvalRecord(rejectedID, "alice", digest, false),
		},
	}
	repository, err := New(storage)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	resolver, err := executionapproval.NewWorkflowApprovalResolver(repository)
	if err != nil {
		t.Fatalf("NewWorkflowApprovalResolver: %v", err)
	}

	for _, test := range []struct {
		name     string
		owner    string
		sourceID string
	}{
		{
			name:     "cross owner",
			owner:    "bob",
			sourceID: "workflow-decision:" + approvedID,
		},
		{
			name:     "invented",
			owner:    "alice",
			sourceID: "workflow-decision:" + uuid.NewString(),
		},
		{
			name:     "rejected",
			owner:    "alice",
			sourceID: "workflow-decision:" + rejectedID,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, resolveErr := resolver.Resolve(
				context.Background(),
				test.owner,
				test.sourceID,
				digest,
			)
			if !errors.Is(resolveErr, executionapproval.ErrWorkflowApprovalUnavailable) {
				t.Fatalf("Resolve error = %v", resolveErr)
			}
		})
	}
}

func TestAdapterPreservesStorageErrorsAndValidatesConstruction(t *testing.T) {
	if _, err := New(nil); !errors.Is(err, ErrRepositoryRequired) {
		t.Fatalf("New(nil) error = %v", err)
	}
	var typedNil *stubRepository
	if _, err := New(typedNil); !errors.Is(err, ErrRepositoryRequired) {
		t.Fatalf("New(typed nil) error = %v", err)
	}

	storage := &stubRepository{err: context.Canceled}
	repository, err := New(storage)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	record, err := repository.FindWorkflowApprovalDecision(
		context.Background(),
		"alice",
		uuid.NewString(),
	)
	if record != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("lookup = %#v, %v", record, err)
	}

	var nilAdapter *Repository
	if _, err := nilAdapter.FindWorkflowApprovalDecision(
		context.Background(),
		"alice",
		uuid.NewString(),
	); !errors.Is(err, ErrRepositoryRequired) {
		t.Fatalf("nil adapter error = %v", err)
	}
}

func approvalRecord(
	decisionID string,
	ownerIdentity string,
	digest string,
	approved bool,
) workflow.ApprovalDecisionRecord {
	decision := "approved"
	if !approved {
		decision = "rejected"
	}
	return workflow.ApprovalDecisionRecord{
		DecisionID:    decisionID,
		WorkflowID:    uuid.NewString(),
		OwnerIdentity: ownerIdentity,
		DecisionType:  "approval",
		Decision:      decision,
		Reason:        "owner decision",
		ActionBinding: "automation-action:automation.script.execute:" + digest,
		Approved:      approved,
		Actor:         ownerIdentity,
		CreatedAt:     time.Now().UTC(),
	}
}

type stubRepository struct {
	records        map[string]workflow.ApprovalDecisionRecord
	err            error
	calls          int
	lastOwner      string
	lastDecisionID string
}

var _ workflow.ApprovalDecisionRepository = (*stubRepository)(nil)

func (r *stubRepository) FindApprovalDecisionForOwner(
	ctx context.Context,
	ownerIdentity string,
	decisionID string,
) (*workflow.ApprovalDecisionRecord, error) {
	r.calls++
	r.lastOwner = ownerIdentity
	r.lastDecisionID = decisionID
	if r.err != nil {
		return nil, r.err
	}
	record, ok := r.records[decisionID]
	if !ok || record.OwnerIdentity != ownerIdentity {
		return nil, gorm.ErrRecordNotFound
	}
	copied := record
	return &copied, nil
}
