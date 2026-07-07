// Package statemachine provides a small, pure finite-state-machine helper:
// declare allowed transitions once, then validate or apply them. It performs no
// I/O so workflow/task state rules can be defined and tested in isolation.
package statemachine

import "sort"

// State is a named state.
type State string

// Machine holds the allowed transitions between states.
type Machine struct {
	transitions map[State]map[State]bool
	terminal    map[State]bool
}

// New builds a machine from an adjacency map of from -> allowed next states.
func New(transitions map[State][]State) *Machine {
	m := &Machine{
		transitions: map[State]map[State]bool{},
		terminal:    map[State]bool{},
	}
	for from, tos := range transitions {
		set := map[State]bool{}
		for _, to := range tos {
			set[to] = true
		}
		m.transitions[from] = set
		if len(tos) == 0 {
			m.terminal[from] = true
		}
	}
	return m
}

// CanTransition reports whether from -> to is an allowed transition.
func (m *Machine) CanTransition(from, to State) bool {
	next, ok := m.transitions[from]
	return ok && next[to]
}

// Transition returns to when the transition is allowed, otherwise it returns
// from and ok=false so callers never advance into an illegal state.
func (m *Machine) Transition(from, to State) (State, bool) {
	if m.CanTransition(from, to) {
		return to, true
	}
	return from, false
}

// IsTerminal reports whether a state has no outgoing transitions.
func (m *Machine) IsTerminal(state State) bool {
	return m.terminal[state]
}

// Next returns the sorted allowed next states from a given state.
func (m *Machine) Next(from State) []State {
	set := m.transitions[from]
	out := make([]State, 0, len(set))
	for to := range set {
		out = append(out, to)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
