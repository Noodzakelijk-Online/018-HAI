package operations

import "testing"

func TestOperationStateMachineValidTransitions(t *testing.T) {
	valid := [][2]OperationStatus{
		{StatusNew, StatusClassified},
		{StatusClassified, StatusReady},
		{StatusClassified, StatusAwaitingApproval},
		{StatusClassified, StatusBlocked},
		{StatusReady, StatusRunning},
		{StatusAwaitingApproval, StatusApproved},
		{StatusApproved, StatusRunning},
		{StatusRunning, StatusVerifying},
		{StatusRunning, StatusInterrupted},
		{StatusVerifying, StatusCompleted},
		{StatusCompleted, StatusArchived},
		{StatusFailed, StatusReady},
		{StatusWaitingExternal, StatusReady},
	}
	for _, tr := range valid {
		if !CanTransition(tr[0], tr[1]) {
			t.Fatalf("expected %s -> %s to be allowed", tr[0], tr[1])
		}
		if got, err := Transition(tr[0], tr[1]); err != nil || got != tr[1] {
			t.Fatalf("Transition(%s,%s) = %s,%v", tr[0], tr[1], got, err)
		}
	}
}

func TestOperationStateMachineInvalidTransitions(t *testing.T) {
	invalid := [][2]OperationStatus{
		{StatusNew, StatusRunning},        // must classify first
		{StatusNew, StatusCompleted},      // cannot skip the pipeline
		{StatusClassified, StatusRunning}, // low-risk must go ready->running
		{StatusAwaitingApproval, StatusRunning}, // must be approved first
		{StatusCompleted, StatusRunning},  // terminal-ish
		{StatusArchived, StatusReady},     // archived is terminal
	}
	for _, tr := range invalid {
		if CanTransition(tr[0], tr[1]) {
			t.Fatalf("expected %s -> %s to be illegal", tr[0], tr[1])
		}
		if _, err := Transition(tr[0], tr[1]); err == nil {
			t.Fatalf("Transition(%s,%s) should error", tr[0], tr[1])
		}
	}
}

func TestExternalRunRequiresApprovalPath(t *testing.T) {
	// An external action can only reach running via approved (not directly from
	// awaiting_approval or ready-with-approval-required). The state machine
	// enforces awaiting_approval -> approved -> running.
	if CanTransition(StatusAwaitingApproval, StatusRunning) {
		t.Fatalf("awaiting_approval must not jump straight to running")
	}
	if !CanTransition(StatusAwaitingApproval, StatusApproved) || !CanTransition(StatusApproved, StatusRunning) {
		t.Fatalf("approval path awaiting_approval -> approved -> running must exist")
	}
}

func TestArchivedIsTerminal(t *testing.T) {
	if !StatusArchived.IsTerminal() {
		t.Fatalf("archived must be terminal")
	}
	if len(NextStatuses(StatusArchived)) != 0 {
		t.Fatalf("archived must have no next statuses")
	}
	if len(NextStatuses(StatusNew)) == 0 {
		t.Fatalf("new must have a next status")
	}
}
