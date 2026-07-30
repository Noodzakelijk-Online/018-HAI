package workflowgraph

import (
	"errors"
	"testing"
	"time"
)

func TestEvaluateNextConditionIsDeterministic(t *testing.T) {
	definition := Definition{
		SchemaVersion: CurrentDefinitionSchemaVersion,
		ID:            "condition",
		Version:       1,
		EntryNodeID:   "check",
		MaxRunSteps:   10,
		Nodes: []Node{
			{
				ID:   "check",
				Type: NodeCondition,
				Condition: &ConditionConfig{
					Field: "approved", Operator: ConditionEqual, Value: "yes",
				},
			},
			{ID: "accepted", Type: NodeTerminal, Terminal: &TerminalConfig{Result: TerminalCompleted}},
			{ID: "declined", Type: NodeTerminal, Terminal: &TerminalConfig{Result: TerminalFailed}},
		},
		Edges: []Edge{
			{ID: "false", From: "check", To: "declined", Outcome: OutcomeFalse},
			{ID: "true", From: "check", To: "accepted", Outcome: OutcomeTrue},
		},
	}
	run := runFor(definition, "check")

	decision, err := EvaluateNext(definition, run, Evaluation{
		NodeID: "check", Variables: map[string]string{"approved": "yes"},
	})
	if err != nil {
		t.Fatalf("EvaluateNext() error = %v", err)
	}
	if len(decision.Next) != 1 || decision.Next[0].NodeID != "accepted" {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestEvaluateNextHumanApprovalWaitsThenAdvances(t *testing.T) {
	definition := approvalDefinition()
	run := runFor(definition, "approval")

	waitingDecision, err := EvaluateNext(definition, run, Evaluation{NodeID: "approval"})
	if err != nil {
		t.Fatalf("pending approval error = %v", err)
	}
	if waitingDecision.Disposition != DispositionWait || waitingDecision.ResultingRun != RunWaiting {
		t.Fatalf("pending decision = %#v", waitingDecision)
	}

	approvedDecision, err := EvaluateNext(definition, run, Evaluation{
		NodeID: "approval", Approval: ApprovalApproved,
	})
	if err != nil {
		t.Fatalf("approved decision error = %v", err)
	}
	if len(approvedDecision.Next) != 1 || approvedDecision.Next[0].NodeID != "done" {
		t.Fatalf("approved decision = %#v", approvedDecision)
	}
}

func TestEvaluateNextTimerRequiresExplicitClockAndEntryTime(t *testing.T) {
	definition := timerDefinition()
	enteredAt := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	run := runFor(definition, "timer")
	run.NodeStates["timer"] = NodeRunState{Status: NodeWaiting, EnteredAt: enteredAt}

	_, err := EvaluateNext(definition, run, Evaluation{NodeID: "timer"})
	if err == nil {
		t.Fatal("timer without explicit time accepted")
	}

	decision, err := EvaluateNext(definition, run, Evaluation{
		NodeID: "timer", Now: enteredAt.Add(4 * time.Minute),
	})
	if err != nil {
		t.Fatalf("waiting timer error = %v", err)
	}
	if decision.Disposition != DispositionWait {
		t.Fatalf("timer decision = %#v", decision)
	}

	decision, err = EvaluateNext(definition, run, Evaluation{
		NodeID: "timer", Now: enteredAt.Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("elapsed timer error = %v", err)
	}
	if len(decision.Next) != 1 || decision.Next[0].NodeID != "done" {
		t.Fatalf("elapsed timer decision = %#v", decision)
	}
}

func TestEvaluateNextParallelSplitUsesStableOrder(t *testing.T) {
	definition := parallelDefinition()
	run := runFor(definition, "split")

	decision, err := EvaluateNext(definition, run, Evaluation{NodeID: "split"})
	if err != nil {
		t.Fatalf("EvaluateNext() error = %v", err)
	}
	if len(decision.Next) != 2 ||
		decision.Next[0].NodeID != "branch-a" ||
		decision.Next[1].NodeID != "branch-b" {
		t.Fatalf("parallel next = %#v", decision.Next)
	}
}

func TestEvaluateNextParallelJoinWaitsForAllBranches(t *testing.T) {
	definition := parallelDefinition()
	run := runFor(definition, "join")
	run.NodeStates["branch-a"] = NodeRunState{Status: NodeSucceeded}
	run.NodeStates["branch-b"] = NodeRunState{Status: NodeActive}

	decision, err := EvaluateNext(definition, run, Evaluation{NodeID: "join"})
	if err != nil {
		t.Fatalf("waiting join error = %v", err)
	}
	if decision.Disposition != DispositionWait {
		t.Fatalf("join decision = %#v", decision)
	}

	run.NodeStates["branch-b"] = NodeRunState{Status: NodeSucceeded}
	decision, err = EvaluateNext(definition, run, Evaluation{NodeID: "join"})
	if err != nil {
		t.Fatalf("ready join error = %v", err)
	}
	if len(decision.Next) != 1 || decision.Next[0].NodeID != "done" {
		t.Fatalf("ready join decision = %#v", decision)
	}
}

func TestEvaluateNextEnforcesTraversalAndRunStepBounds(t *testing.T) {
	definition := linearDefinition()
	definition.Edges[0].MaxTraversals = 1
	run := runFor(definition, "start")
	run.EdgeTraversals["finish"] = 1

	_, err := EvaluateNext(definition, run, Evaluation{NodeID: "start"})
	if !errors.Is(err, ErrTraversalLimit) {
		t.Fatalf("traversal error = %v", err)
	}

	run.EdgeTraversals["finish"] = 0
	run.Steps = definition.MaxRunSteps
	_, err = EvaluateNext(definition, run, Evaluation{NodeID: "start"})
	if !errors.Is(err, ErrRunStepLimit) {
		t.Fatalf("run step error = %v", err)
	}
}

func TestEvaluateNextTerminalMapsResult(t *testing.T) {
	definition := linearDefinition()
	run := runFor(definition, "done")

	decision, err := EvaluateNext(definition, run, Evaluation{NodeID: "done"})
	if err != nil {
		t.Fatalf("EvaluateNext() error = %v", err)
	}
	if decision.Disposition != DispositionComplete || decision.ResultingRun != RunCompleted {
		t.Fatalf("terminal decision = %#v", decision)
	}
}

func approvalDefinition() Definition {
	return Definition{
		SchemaVersion: CurrentDefinitionSchemaVersion,
		ID:            "approval",
		Version:       1,
		EntryNodeID:   "approval",
		MaxRunSteps:   10,
		Nodes: []Node{
			{ID: "approval", Type: NodeHumanApproval},
			{ID: "done", Type: NodeTerminal, Terminal: &TerminalConfig{Result: TerminalCompleted}},
			{ID: "rejected", Type: NodeTerminal, Terminal: &TerminalConfig{Result: TerminalCancelled}},
		},
		Edges: []Edge{
			{ID: "approved", From: "approval", To: "done", Outcome: OutcomeApproved},
			{ID: "rejected", From: "approval", To: "rejected", Outcome: OutcomeRejected},
		},
	}
}

func timerDefinition() Definition {
	return Definition{
		SchemaVersion: CurrentDefinitionSchemaVersion,
		ID:            "timer",
		Version:       1,
		EntryNodeID:   "timer",
		MaxRunSteps:   10,
		Nodes: []Node{
			{ID: "timer", Type: NodeTimer, Timer: &TimerConfig{After: 5 * time.Minute}},
			{ID: "done", Type: NodeTerminal, Terminal: &TerminalConfig{Result: TerminalCompleted}},
		},
		Edges: []Edge{{ID: "elapsed", From: "timer", To: "done", Outcome: OutcomeElapsed}},
	}
}
