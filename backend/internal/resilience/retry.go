package resilience

import (
	"fmt"
	"math"
	"time"
)

// DecideRetry classifies a failed attempt as a delayed retry or dead letter.
// attemptsCompleted includes the attempt that produced failure.
func DecideRetry(scope Scope, workID string, attemptsCompleted uint32, failure Failure, policy RetryPolicy, now time.Time) (RetryDecision, error) {
	if err := validateScope(scope); err != nil {
		return RetryDecision{}, err
	}
	if err := validateID("work id", workID); err != nil {
		return RetryDecision{}, err
	}
	if err := validateTime("retry decision time", now); err != nil {
		return RetryDecision{}, err
	}
	if attemptsCompleted == 0 {
		return RetryDecision{}, fmt.Errorf("resilience: attempts completed must be positive")
	}
	if err := validateFailure(failure); err != nil {
		return RetryDecision{}, err
	}
	if err := validateRetryPolicy(policy); err != nil {
		return RetryDecision{}, err
	}

	decision := RetryDecision{
		Scope:             scope,
		WorkID:            workID,
		AttemptsCompleted: attemptsCompleted,
		Failure:           failure,
		Authority:         advisoryBoundary(),
	}
	if class, terminal := deadLetterForFailure(failure.Class); terminal {
		decision.Disposition = RetryDeadLetter
		decision.DeadLetterClass = class
		decision.Reason = "failure class is not retryable"
		return decision, nil
	}
	if attemptsCompleted >= policy.MaxAttempts {
		decision.Disposition = RetryDeadLetter
		decision.DeadLetterClass = DeadLetterRetryExhausted
		decision.Reason = "retry attempt limit exhausted"
		return decision, nil
	}
	delay := retryDelay(policy, attemptsCompleted)
	retryAt := now.UTC().Add(delay)
	decision.Disposition = RetrySchedule
	decision.RetryAt = &retryAt
	decision.Reason = "retryable failure remains within attempt limit"
	return decision, nil
}

func deadLetterForFailure(class FailureClass) (DeadLetterClass, bool) {
	switch class {
	case FailureTransient, FailureRateLimited:
		return "", false
	case FailurePermanent:
		return DeadLetterPermanent, true
	case FailureInvalidWork:
		return DeadLetterInvalid, true
	case FailureUnauthorized:
		return DeadLetterUnauthorized, true
	case FailureSecurity:
		return DeadLetterSecurity, true
	case FailureUnknown:
		return DeadLetterUnknown, true
	default:
		return DeadLetterUnknown, true
	}
}

func retryDelay(policy RetryPolicy, attemptsCompleted uint32) time.Duration {
	delay := uint64(policy.BaseDelay)
	capDelay := uint64(policy.MaxDelay)
	for exponent := uint32(1); exponent < attemptsCompleted; exponent++ {
		if delay >= capDelay {
			return policy.MaxDelay
		}
		if delay > math.MaxUint64/uint64(policy.Multiplier) {
			return policy.MaxDelay
		}
		delay *= uint64(policy.Multiplier)
		if delay >= capDelay {
			return policy.MaxDelay
		}
	}
	return time.Duration(delay)
}
