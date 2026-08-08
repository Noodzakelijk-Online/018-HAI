package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"

	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var _ ApprovalDecisionRepository = (*fakeWorkflowRepo)(nil)

func TestMemoryWorkflowApprovalDecisionLookupIsOwnerScopedAndExact(t *testing.T) {
	repo := newFakeWorkflowRepo()
	aliceWorkflow := createApprovalWorkflow(t, repo, "alice")
	bobWorkflow := createApprovalWorkflow(t, repo, "bob")
	binding := "automation-action:automation.script.execute:" + strings.Repeat("a", 64)

	aliceDecision := createApprovalDecision(t, repo, aliceWorkflow.ID, "alice", binding, true)
	bobDecision := createApprovalDecision(t, repo, bobWorkflow.ID, "bob", binding, true)

	record, err := repo.FindApprovalDecisionForOwner(
		context.Background(),
		"alice",
		aliceDecision.ID.String(),
	)
	if err != nil {
		t.Fatalf("FindWorkflowApprovalDecision: %v", err)
	}
	if record.DecisionID != aliceDecision.ID.String() ||
		record.WorkflowID != aliceWorkflow.ID.String() ||
		record.OwnerIdentity != "alice" ||
		record.DecisionType != "approval" ||
		record.Decision != "approved" ||
		record.ActionBinding != binding ||
		!record.Approved ||
		record.Actor != "alice" ||
		record.CreatedAt.IsZero() {
		t.Fatalf("approval projection = %#v", record)
	}

	for _, test := range []struct {
		name       string
		owner      string
		decisionID string
	}{
		{name: "cross owner Alice decision", owner: "bob", decisionID: aliceDecision.ID.String()},
		{name: "cross owner Bob decision", owner: "alice", decisionID: bobDecision.ID.String()},
		{name: "invented decision", owner: "alice", decisionID: uuid.NewString()},
		{name: "malformed decision", owner: "alice", decisionID: "not-a-uuid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			found, lookupErr := repo.FindApprovalDecisionForOwner(
				context.Background(),
				test.owner,
				test.decisionID,
			)
			if found != nil || !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				t.Fatalf("lookup = %#v, %v, want nil record-not-found", found, lookupErr)
			}
		})
	}
}

func TestMemoryWorkflowApprovalDecisionLookupReturnsRejectedDecisionForPolicyValidation(t *testing.T) {
	repo := newFakeWorkflowRepo()
	workflow := createApprovalWorkflow(t, repo, "alice")
	digest := strings.Repeat("c", 64)
	decision := createApprovalDecision(
		t,
		repo,
		workflow.ID,
		"alice",
		"automation-action:automation.api.mutate:"+digest,
		false,
	)

	record, err := repo.FindApprovalDecisionForOwner(
		context.Background(),
		"alice",
		decision.ID.String(),
	)
	if err != nil {
		t.Fatalf("FindWorkflowApprovalDecision: %v", err)
	}
	if record.Approved || record.Decision != "rejected" {
		t.Fatalf("rejected decision projection = %#v", record)
	}

}

func TestMemoryWorkflowApprovalDecisionLookupHonorsCancellation(t *testing.T) {
	repo := newFakeWorkflowRepo()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	record, err := repo.FindApprovalDecisionForOwner(ctx, "alice", uuid.NewString())
	if record != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled lookup = %#v, %v", record, err)
	}
}

func createApprovalWorkflow(
	t *testing.T,
	repo *fakeWorkflowRepo,
	ownerIdentity string,
) *models.WorkflowItem {
	t.Helper()
	item, err := repo.CreateItem(&models.WorkflowItem{
		ID:               uuid.New(),
		OwnerIdentity:    ownerIdentity,
		Title:            "Approval workflow for " + ownerIdentity,
		CurrentState:     StateReady,
		TaskType:         "administrative",
		RiskLevel:        "high",
		AutonomyLevel:    "approve_before_execute",
		RequiresApproval: true,
		ApprovalStatus:   "approved",
	})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	return item
}

func createApprovalDecision(
	t *testing.T,
	repo *fakeWorkflowRepo,
	workflowID uuid.UUID,
	actor string,
	binding string,
	approved bool,
) *models.WorkflowDecision {
	t.Helper()
	decisionValue := "rejected"
	if approved {
		decisionValue = "approved"
	}
	decision, err := repo.CreateDecision(&models.WorkflowDecision{
		ID:           uuid.New(),
		WorkflowID:   workflowID,
		DecisionType: "approval",
		Decision:     decisionValue,
		Reason:       "owner reviewed the exact action",
		RuleApplied:  binding,
		Approved:     approved,
		Actor:        actor,
	})
	if err != nil {
		t.Fatalf("CreateDecision: %v", err)
	}
	return decision
}
