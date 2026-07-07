package statemachine

import "testing"

func workflowMachine() *Machine {
	return New(map[State][]State{
		"intake":   {"planned", "rejected"},
		"planned":  {"awaiting_approval", "rejected"},
		"awaiting_approval": {"executing", "rejected"},
		"executing": {"done", "failed"},
		"done":     {},
		"rejected": {},
		"failed":   {"planned"}, // failed work can be replanned
	})
}

func TestAllowedTransitions(t *testing.T) {
	m := workflowMachine()
	if !m.CanTransition("intake", "planned") {
		t.Fatalf("intake -> planned should be allowed")
	}
	if to, ok := m.Transition("executing", "done"); !ok || to != "done" {
		t.Fatalf("executing -> done failed: %v %v", to, ok)
	}
}

func TestIllegalTransitionsAreBlocked(t *testing.T) {
	m := workflowMachine()
	if m.CanTransition("intake", "done") {
		t.Fatalf("intake -> done must be illegal")
	}
	to, ok := m.Transition("done", "planned")
	if ok || to != "done" {
		t.Fatalf("transition from terminal state should be blocked and stay put, got %v %v", to, ok)
	}
}

func TestTerminalAndNext(t *testing.T) {
	m := workflowMachine()
	if !m.IsTerminal("done") || m.IsTerminal("intake") {
		t.Fatalf("terminal detection wrong")
	}
	next := m.Next("planned")
	if len(next) != 2 || next[0] != "awaiting_approval" || next[1] != "rejected" {
		t.Fatalf("Next(planned) = %v, want sorted [awaiting_approval rejected]", next)
	}
}
