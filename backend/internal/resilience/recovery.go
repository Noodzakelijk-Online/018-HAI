package resilience

import "fmt"

// DecideRecovery combines durable liveness, lease, circuit, and retry state
// into a recommendation. It never contacts a worker or mutates supplied state.
func DecideRecovery(request RecoveryRequest) (RecoveryDecision, error) {
	if err := validateContract(request.ContractVersion); err != nil {
		return RecoveryDecision{}, err
	}
	if err := validateScope(request.Scope); err != nil {
		return RecoveryDecision{}, err
	}
	if err := validateID("work id", request.WorkID); err != nil {
		return RecoveryDecision{}, err
	}
	if err := validateTime("recovery decision time", request.Now); err != nil {
		return RecoveryDecision{}, err
	}
	if err := validateHeartbeatAge(request.HeartbeatMaxAge); err != nil {
		return RecoveryDecision{}, err
	}
	if err := validateRetryPolicy(request.RetryPolicy); err != nil {
		return RecoveryDecision{}, err
	}
	decision := RecoveryDecision{Authority: advisoryBoundary()}

	if request.Circuit != nil {
		if err := validateCircuitState(*request.Circuit); err != nil {
			return RecoveryDecision{}, err
		}
		if err := requireSameScope(request.Scope, request.Circuit.Scope); err != nil {
			return RecoveryDecision{}, err
		}
		if request.Circuit.Phase == CircuitOpen && request.Now.Before(*request.Circuit.RetryAfter) {
			notBefore := request.Circuit.RetryAfter.UTC()
			decision.Action = RecoveryHoldCircuitOpen
			decision.NotBefore = &notBefore
			decision.Reason = "circuit open interval has not elapsed"
			return decision, nil
		}
	}

	if request.Lease != nil {
		if err := validateLease(*request.Lease); err != nil {
			return RecoveryDecision{}, err
		}
		if err := requireSameScope(request.Scope, request.Lease.Scope); err != nil {
			return RecoveryDecision{}, err
		}
		if request.Lease.WorkID != request.WorkID {
			return RecoveryDecision{}, fmt.Errorf("resilience: recovery lease work id does not match request")
		}
		if request.Lease.State == LeaseActive && !request.Now.Before(request.Lease.ExpiresAt) {
			decision.Action = RecoveryReclaimLease
			decision.Reason = "active lease has expired"
			return decision, nil
		}
	}

	heartbeat, err := AssessHeartbeat(request.Scope, request.Heartbeat, request.Now, request.HeartbeatMaxAge)
	if err != nil {
		return RecoveryDecision{}, err
	}
	if request.Lease != nil && request.Lease.State == LeaseActive && request.Heartbeat != nil && request.Heartbeat.WorkerID != request.Lease.WorkerID {
		return RecoveryDecision{}, fmt.Errorf("resilience: recovery heartbeat worker does not match lease worker")
	}
	if request.Lease != nil && request.Lease.State == LeaseActive && heartbeat.Status != HeartbeatHealthy {
		decision.Action = RecoveryReclaimLease
		decision.Reason = "active lease worker heartbeat is stale or missing"
		return decision, nil
	}

	if request.Failure != nil {
		retry, retryErr := DecideRetry(request.Scope, request.WorkID, request.AttemptsCompleted, *request.Failure, request.RetryPolicy, request.Now)
		if retryErr != nil {
			return RecoveryDecision{}, retryErr
		}
		if retry.Disposition == RetryDeadLetter {
			decision.Action = RecoveryDeadLetter
			decision.DeadLetterClass = retry.DeadLetterClass
			decision.Reason = retry.Reason
			return decision, nil
		}
		decision.Action = RecoveryScheduleRetry
		decision.NotBefore = retry.RetryAt
		decision.Reason = retry.Reason
		return decision, nil
	}

	if request.Lease != nil && request.Lease.State == LeaseActive && heartbeat.Status == HeartbeatHealthy {
		decision.Action = RecoveryWaitWorker
		decision.Reason = "lease and worker heartbeat remain healthy"
		return decision, nil
	}
	decision.Action = RecoveryManualReview
	decision.Reason = "insufficient durable evidence for automatic recovery recommendation"
	return decision, nil
}
