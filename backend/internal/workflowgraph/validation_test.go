package workflowgraph

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateDefinitionAcceptsParallelGraph(t *testing.T) {
	if err := ValidateDefinition(parallelDefinition()); err != nil {
		t.Fatalf("ValidateDefinition() error = %v", err)
	}
}

func TestValidateDefinitionRequiresEveryCycleToCrossBoundedEdge(t *testing.T) {
	definition := Definition{
		SchemaVersion: CurrentDefinitionSchemaVersion,
		ID:            "retry",
		Version:       1,
		EntryNodeID:   "first",
		MaxRunSteps:   20,
		Nodes: []Node{
			{ID: "first", Type: NodeAction},
			{ID: "retry", Type: NodeAction},
			{ID: "done", Type: NodeTerminal, Terminal: &TerminalConfig{Result: TerminalCompleted}},
		},
		Edges: []Edge{
			{ID: "to-retry", From: "first", To: "retry"},
			{ID: "loop", From: "retry", To: "first", Outcome: "again", MaxTraversals: 2},
			{ID: "finish", From: "retry", To: "done", Outcome: "done"},
		},
	}

	if err := ValidateDefinition(definition); err != nil {
		t.Fatalf("bounded graph rejected: %v", err)
	}

	definition.Edges[1].MaxTraversals = 0
	err := ValidateDefinition(definition)
	if err == nil || !strings.Contains(err.Error(), "no explicit traversal bound") {
		t.Fatalf("unbounded cycle error = %v", err)
	}
}

func TestValidateDefinitionRejectsUnreachableNode(t *testing.T) {
	definition := linearDefinition()
	definition.Nodes = append(definition.Nodes, Node{ID: "orphan", Type: NodeTerminal, Terminal: &TerminalConfig{Result: TerminalFailed}})

	err := ValidateDefinition(definition)
	if err == nil || !strings.Contains(err.Error(), `node "orphan" is unreachable`) {
		t.Fatalf("unreachable node error = %v", err)
	}
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error type = %T, want *ValidationError", err)
	}
}

func TestValidateDefinitionRejectsIncorrectParallelJoin(t *testing.T) {
	definition := parallelDefinition()
	for index := range definition.Edges {
		if definition.Edges[index].ID == "b-join" {
			definition.Edges[index].To = "done"
		}
	}

	err := ValidateDefinition(definition)
	if err == nil {
		t.Fatal("incorrect join accepted")
	}
	if !strings.Contains(err.Error(), "cannot reach join") ||
		!strings.Contains(err.Error(), "requires at least two incoming branches") {
		t.Fatalf("join validation error = %v", err)
	}
}

func TestValidateDefinitionRejectsAmbiguousOutcomes(t *testing.T) {
	definition := linearDefinition()
	definition.Nodes = append(
		definition.Nodes[:1],
		Node{ID: "failed", Type: NodeTerminal, Terminal: &TerminalConfig{Result: TerminalFailed}},
		definition.Nodes[1],
	)
	definition.Edges = []Edge{
		{ID: "one", From: "start", To: "done", Outcome: OutcomeDefault},
		{ID: "two", From: "start", To: "failed", Outcome: OutcomeDefault},
	}

	err := ValidateDefinition(definition)
	if err == nil || !strings.Contains(err.Error(), `duplicate outcome "default"`) {
		t.Fatalf("ambiguous outcome error = %v", err)
	}
}

func linearDefinition() Definition {
	return Definition{
		SchemaVersion: CurrentDefinitionSchemaVersion,
		ID:            "linear",
		Version:       1,
		EntryNodeID:   "start",
		MaxRunSteps:   10,
		Nodes: []Node{
			{ID: "start", Type: NodeAction},
			{ID: "done", Type: NodeTerminal, Terminal: &TerminalConfig{Result: TerminalCompleted}},
		},
		Edges: []Edge{{ID: "finish", From: "start", To: "done"}},
	}
}

func parallelDefinition() Definition {
	return Definition{
		SchemaVersion: CurrentDefinitionSchemaVersion,
		ID:            "parallel",
		Version:       1,
		EntryNodeID:   "split",
		MaxRunSteps:   20,
		Nodes: []Node{
			{ID: "split", Type: NodeParallelSplit},
			{ID: "branch-a", Type: NodeAction},
			{ID: "branch-b", Type: NodeAction},
			{ID: "join", Type: NodeParallelJoin, Join: &JoinConfig{SplitNodeID: "split", Mode: JoinAll}},
			{ID: "done", Type: NodeTerminal, Terminal: &TerminalConfig{Result: TerminalCompleted}},
		},
		Edges: []Edge{
			{ID: "split-b", From: "split", To: "branch-b", Order: 20},
			{ID: "split-a", From: "split", To: "branch-a", Order: 10},
			{ID: "a-join", From: "branch-a", To: "join"},
			{ID: "b-join", From: "branch-b", To: "join"},
			{ID: "joined", From: "join", To: "done"},
		},
	}
}

func runFor(definition Definition, active ...string) Run {
	return Run{
		SchemaVersion:     CurrentRunSchemaVersion,
		ID:                "run-1",
		DefinitionID:      definition.ID,
		DefinitionVersion: definition.Version,
		Status:            RunRunning,
		ActiveNodeIDs:     active,
		NodeStates:        map[string]NodeRunState{},
		EdgeTraversals:    map[string]uint32{},
	}
}
