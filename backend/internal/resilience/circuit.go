package resilience

import (
	"fmt"
	"time"
)

// NewCircuitState returns the initial durable state for a scoped circuit.
func NewCircuitState(scope Scope, circuitID string) (CircuitState, error) {
	if err := validateScope(scope); err != nil {
		return CircuitState{}, err
	}
	if err := validateID("circuit id", circuitID); err != nil {
		return CircuitState{}, err
	}
	return CircuitState{
		ContractVersion: ContractVersion,
		Scope:           scope,
		CircuitID:       circuitID,
		Phase:           CircuitClosed,
		Revision:        1,
	}, nil
}

// BeforeCircuitAttempt recommends whether an executor should consider a normal
// attempt or bounded probe. It does not authorize either action.
func BeforeCircuitAttempt(scope Scope, current CircuitState, now time.Time, policy CircuitPolicy) (CircuitDecision, error) {
	if err := validateCircuitInput(scope, current, now, policy); err != nil {
		return CircuitDecision{}, err
	}
	next := cloneCircuit(current)
	decision := CircuitDecision{State: next, Authority: advisoryBoundary()}
	switch current.Phase {
	case CircuitClosed:
		decision.Recommendation = CircuitRecommendAttempt
		decision.Reason = "circuit is closed"
	case CircuitOpen:
		if now.Before(*current.RetryAfter) {
			decision.Recommendation = CircuitRecommendBlock
			decision.Reason = "circuit remains open until retry-after"
			return decision, nil
		}
		next.Phase = CircuitHalfOpen
		next.ProbesInFlight = 1
		next.Revision++
		decision.State = next
		decision.Recommendation = CircuitRecommendProbe
		decision.Reason = "open interval elapsed; one half-open probe is recommended"
	case CircuitHalfOpen:
		if current.ProbesInFlight >= policy.MaxHalfOpenProbes {
			decision.Recommendation = CircuitRecommendBlock
			decision.Reason = "half-open probe limit is already in flight"
			return decision, nil
		}
		next.ProbesInFlight++
		next.Revision++
		decision.State = next
		decision.Recommendation = CircuitRecommendProbe
		decision.Reason = "bounded half-open probe is available"
	}
	return decision, nil
}

// AfterCircuitAttempt recommends the next durable circuit state after an
// observed result. A half-open failure reopens the circuit; a success closes it.
func AfterCircuitAttempt(scope Scope, current CircuitState, outcome AttemptOutcome, now time.Time, policy CircuitPolicy) (CircuitDecision, error) {
	if err := validateCircuitInput(scope, current, now, policy); err != nil {
		return CircuitDecision{}, err
	}
	if outcome != AttemptSucceeded && outcome != AttemptFailed {
		return CircuitDecision{}, fmt.Errorf("resilience: attempt outcome is unsupported")
	}
	if current.Phase == CircuitOpen {
		return CircuitDecision{}, fmt.Errorf("resilience: an attempt result cannot be recorded while circuit is open")
	}
	next := cloneCircuit(current)
	next.Revision++
	decision := CircuitDecision{Authority: advisoryBoundary()}
	if outcome == AttemptSucceeded {
		next.Phase = CircuitClosed
		next.ConsecutiveFailures = 0
		next.ProbesInFlight = 0
		next.OpenedAt = nil
		next.RetryAfter = nil
		decision.State = next
		decision.Recommendation = CircuitRecommendAttempt
		decision.Reason = "successful observation closes and resets the circuit"
		return decision, nil
	}

	nextFailureCount := incrementFailureCount(current.ConsecutiveFailures)
	if current.Phase == CircuitHalfOpen || current.ConsecutiveFailures >= policy.FailureThreshold-1 {
		openedAt := now.UTC()
		retryAfter := openedAt.Add(policy.OpenDuration)
		next.Phase = CircuitOpen
		next.ConsecutiveFailures = nextFailureCount
		next.ProbesInFlight = 0
		next.OpenedAt = &openedAt
		next.RetryAfter = &retryAfter
		decision.State = next
		decision.Recommendation = CircuitRecommendBlock
		decision.Reason = "failure threshold reached; circuit should open"
		return decision, nil
	}
	next.ConsecutiveFailures = nextFailureCount
	decision.State = next
	decision.Recommendation = CircuitRecommendAttempt
	decision.Reason = "failure recorded below circuit threshold"
	return decision, nil
}

func validateCircuitInput(scope Scope, state CircuitState, now time.Time, policy CircuitPolicy) error {
	if err := validateTime("circuit decision time", now); err != nil {
		return err
	}
	if err := validateCircuitPolicy(policy); err != nil {
		return err
	}
	if err := validateCircuitState(state); err != nil {
		return err
	}
	if state.Phase == CircuitClosed && state.ConsecutiveFailures >= policy.FailureThreshold {
		return fmt.Errorf("resilience: closed circuit failure count meets or exceeds threshold")
	}
	if state.Phase == CircuitHalfOpen && state.ProbesInFlight > policy.MaxHalfOpenProbes {
		return fmt.Errorf("resilience: half-open probes exceed configured limit")
	}
	if state.OpenedAt != nil && now.Before(*state.OpenedAt) {
		return fmt.Errorf("resilience: circuit decision time predates the open interval")
	}
	return requireSameScope(scope, state.Scope)
}

func validateCircuitState(state CircuitState) error {
	if err := validateContract(state.ContractVersion); err != nil {
		return err
	}
	if err := validateScope(state.Scope); err != nil {
		return err
	}
	if err := validateID("circuit id", state.CircuitID); err != nil {
		return err
	}
	if state.Revision == 0 {
		return fmt.Errorf("resilience: circuit revision must be positive")
	}
	switch state.Phase {
	case CircuitClosed:
		if state.OpenedAt != nil || state.RetryAfter != nil || state.ProbesInFlight != 0 {
			return fmt.Errorf("resilience: closed circuit carries open-state fields")
		}
	case CircuitOpen:
		if state.OpenedAt == nil || state.RetryAfter == nil || !state.RetryAfter.After(*state.OpenedAt) || state.ProbesInFlight != 0 {
			return fmt.Errorf("resilience: open circuit timestamps are inconsistent")
		}
	case CircuitHalfOpen:
		if state.OpenedAt == nil || state.RetryAfter == nil || !state.RetryAfter.After(*state.OpenedAt) {
			return fmt.Errorf("resilience: half-open circuit is missing its open interval")
		}
	default:
		return fmt.Errorf("resilience: circuit phase is unsupported")
	}
	return nil
}

func cloneCircuit(state CircuitState) CircuitState {
	copyState := state
	if state.OpenedAt != nil {
		openedAt := *state.OpenedAt
		copyState.OpenedAt = &openedAt
	}
	if state.RetryAfter != nil {
		retryAfter := *state.RetryAfter
		copyState.RetryAfter = &retryAfter
	}
	return copyState
}

func incrementFailureCount(current uint32) uint32 {
	if current == ^uint32(0) {
		return current
	}
	return current + 1
}
