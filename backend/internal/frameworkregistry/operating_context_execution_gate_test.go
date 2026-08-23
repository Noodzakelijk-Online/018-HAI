package frameworkregistry

import "testing"

func TestBuildCoordinationPlanBlocksExecutionForUnverifiedRequiredSpecialist(t *testing.T) {
	plan, err := buildCoordinationPlan(SelectionRequest{ExecuteRequested: true, PreferredCoordinationMode: "hierarchical"}, []AgentCard{
		{
			ID:       "hai_task_engine",
			Role:     "coordinator",
			Verified: true,
		},
		{
			ID:       "legal_specialist",
			Role:     "legal_specialist",
			Status:   "required_unassigned",
			Verified: false,
		},
	})
	if err != nil {
		t.Fatalf("buildCoordinationPlan: %v", err)
	}
	if plan.Mode != "blocked_pending_assignment" {
		t.Fatalf("mode = %q, want blocked_pending_assignment", plan.Mode)
	}
	if plan.Rationale == "" {
		t.Fatal("expected an operator-visible explanation for the blocked state")
	}
}

func TestBuildCoordinationPlanDoesNotDowngradeRequestedSpecialistTeam(t *testing.T) {
	plan, err := buildCoordinationPlan(SelectionRequest{ExecuteRequested: true, PreferredCoordinationMode: "parallel_specialists"}, []AgentCard{
		{ID: "hai_task_engine", Role: "coordinator", Verified: true},
		{ID: "evidence_reviewer", Role: "evidence_reviewer", Status: "required_unassigned"},
	})
	if err != nil {
		t.Fatalf("buildCoordinationPlan: %v", err)
	}
	if plan.Mode != "blocked_pending_assignment" {
		t.Fatalf("mode = %q, want blocked_pending_assignment", plan.Mode)
	}
}
