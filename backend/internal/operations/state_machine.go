package operations

import (
	"fmt"

	"automation-hub-backend/internal/statemachine"
)

// operationMachine encodes the Operation state machine from §8, reusing the
// Phase-1 generic transition validator (`internal/statemachine`). The prompt's
// FSM includes a "rejected" node not present in the status enum; a rejection is
// modelled as awaiting_approval -> dismissed (dismissed -> archived), and
// "later" is a self-hold in awaiting_approval (tracked via NextReviewAt), not a
// separate state.
var operationMachine = newOperationMachine()

func newOperationMachine() *statemachine.Machine {
	st := func(s OperationStatus) statemachine.State { return statemachine.State(s) }
	return statemachine.New(map[statemachine.State][]statemachine.State{
		st(StatusNew):              {st(StatusClassified)},
		st(StatusClassified):       {st(StatusReady), st(StatusDrafting), st(StatusAwaitingApproval), st(StatusBlocked), st(StatusWaitingExternal)},
		st(StatusDrafting):         {st(StatusDraftReady)},
		st(StatusDraftReady):       {st(StatusAwaitingApproval), st(StatusCompleted)},
		st(StatusReady):            {st(StatusRunning)},
		st(StatusAwaitingApproval): {st(StatusApproved), st(StatusDismissed), st(StatusBlocked)},
		st(StatusApproved):         {st(StatusRunning)},
		st(StatusRunning):          {st(StatusVerifying), st(StatusFailed), st(StatusInterrupted)},
		st(StatusInterrupted):      {st(StatusAwaitingApproval), st(StatusBlocked), st(StatusCompleted)},
		st(StatusVerifying):        {st(StatusCompleted), st(StatusFailed), st(StatusAwaitingApproval)},
		st(StatusFailed):           {st(StatusReady), st(StatusBlocked)},
		st(StatusWaitingExternal):  {st(StatusReady)},
		st(StatusCompleted):        {st(StatusArchived)},
		st(StatusDismissed):        {st(StatusArchived)},
		st(StatusBlocked):          {st(StatusArchived)},
		st(StatusArchived):         {},
	})
}

// CanTransition reports whether an Operation may move from one status to another.
func CanTransition(from, to OperationStatus) bool {
	return operationMachine.CanTransition(statemachine.State(from), statemachine.State(to))
}

// Transition returns `to` when the move is allowed, else an error and `from`.
func Transition(from, to OperationStatus) (OperationStatus, error) {
	if !to.IsValid() {
		return from, fmt.Errorf("invalid target status: %q", to)
	}
	if !CanTransition(from, to) {
		return from, fmt.Errorf("illegal operation transition: %s -> %s", from, to)
	}
	return to, nil
}

// NextStatuses returns the sorted allowed next statuses from a given status.
func NextStatuses(from OperationStatus) []OperationStatus {
	next := operationMachine.Next(statemachine.State(from))
	out := make([]OperationStatus, 0, len(next))
	for _, s := range next {
		out = append(out, OperationStatus(s))
	}
	return out
}
