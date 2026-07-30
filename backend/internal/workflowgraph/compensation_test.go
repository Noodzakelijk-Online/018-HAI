package workflowgraph

import (
	"testing"
	"time"
)

func TestPlanCancellationCompensatesCompletedActionsInReverseOrder(t *testing.T) {
	definition := compensationDefinition()
	firstCompleted := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	secondCompleted := firstCompleted.Add(time.Minute)
	run := runFor(definition, "pending", "action-2")
	run.ID = "cancel-me"
	run.NodeStates = map[string]NodeRunState{
		"action-1": {Status: NodeSucceeded, CompletedAt: &firstCompleted},
		"action-2": {Status: NodeSucceeded, CompletedAt: &secondCompleted},
	}

	plan, err := PlanCancellation(definition, run, "operator requested cancellation")
	if err != nil {
		t.Fatalf("PlanCancellation() error = %v", err)
	}
	if !plan.RequiresCompensation || plan.TargetStatus != RunCancelled {
		t.Fatalf("plan = %#v", plan)
	}
	if len(plan.CancelActiveNodeIDs) != 2 ||
		plan.CancelActiveNodeIDs[0] != "action-2" ||
		plan.CancelActiveNodeIDs[1] != "pending" {
		t.Fatalf("cancel active nodes = %#v", plan.CancelActiveNodeIDs)
	}
	if len(plan.CompensationSteps) != 2 ||
		plan.CompensationSteps[0].CompletedNodeID != "action-2" ||
		plan.CompensationSteps[0].CompensationNodeID != "undo-2" ||
		plan.CompensationSteps[1].CompletedNodeID != "action-1" {
		t.Fatalf("compensation steps = %#v", plan.CompensationSteps)
	}
}

func TestPlanCompensationSkipsCompletedCompensator(t *testing.T) {
	definition := compensationDefinition()
	completedAt := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	run := runFor(definition)
	run.NodeStates = map[string]NodeRunState{
		"action-1": {Status: NodeSucceeded, CompletedAt: &completedAt},
		"undo-1":   {Status: NodeCompensated},
	}

	plan, err := PlanCompensation(definition, run, RunFailed, "downstream failure")
	if err != nil {
		t.Fatalf("PlanCompensation() error = %v", err)
	}
	if plan.RequiresCompensation || len(plan.CompensationSteps) != 0 {
		t.Fatalf("plan = %#v", plan)
	}
}

func compensationDefinition() Definition {
	return Definition{
		SchemaVersion: CurrentDefinitionSchemaVersion,
		ID:            "compensation",
		Version:       1,
		EntryNodeID:   "action-1",
		MaxRunSteps:   20,
		Nodes: []Node{
			{ID: "action-1", Type: NodeAction, CompensationNodeID: "undo-1"},
			{ID: "action-2", Type: NodeAction, CompensationNodeID: "undo-2"},
			{ID: "pending", Type: NodeWait},
			{ID: "undo-1", Type: NodeCompensation},
			{ID: "undo-2", Type: NodeCompensation},
			{ID: "done", Type: NodeTerminal, Terminal: &TerminalConfig{Result: TerminalCompleted}},
		},
		Edges: []Edge{
			{ID: "next-action", From: "action-1", To: "action-2"},
			{ID: "wait", From: "action-2", To: "pending"},
			{ID: "resume", From: "pending", To: "done", Outcome: "resume"},
		},
	}
}
