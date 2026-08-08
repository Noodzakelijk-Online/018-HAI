package executionauth

import "errors"

var (
	ErrNotFound                 = errors.New("execution authorization record not found")
	ErrIdempotencyConflict      = errors.New("execution authorization idempotency conflict")
	ErrAlreadyConsumed          = errors.New("execution authorization was already consumed")
	ErrAlreadyExercised         = errors.New("execution authorization final effect was already exercised")
	ErrNotAuthorized            = errors.New("execution authorization did not permit execution")
	ErrFinalEffectMismatch      = errors.New("execution authorization final effect binding does not match")
	ErrPolicyUnavailable        = errors.New("execution authorization policy is unavailable")
	ErrAuthorizationChanged     = errors.New("execution authorization evidence changed before consumption")
	ErrSourceEvidenceUnverified = errors.New("source evidence could not be independently verified")
)
