package controlledlearning

import "errors"

var (
	ErrNotFound              = errors.New("controlled learning record not found")
	ErrIdempotencyConflict   = errors.New("controlled learning idempotency conflict")
	ErrRevisionConflict      = errors.New("controlled learning revision conflict")
	ErrProtectedTarget       = errors.New("protected policy target requires separate governance")
	ErrUnsupportedEvidence   = errors.New("learning evidence is not verified or human-confirmed")
	ErrInvalidStateChange    = errors.New("invalid controlled learning state change")
	ErrOwnerScopeViolation   = errors.New("controlled learning owner scope violation")
	ErrIntegrityViolation    = errors.New("controlled learning integrity violation")
	ErrPromoterUnavailable   = errors.New("controlled learning promoter is unavailable")
	ErrApplicationInProgress = errors.New("controlled learning application is in progress")
	ErrApplicationFailed     = errors.New("controlled learning application failed")
	ErrRollbackUnavailable   = errors.New("controlled learning rollback is unavailable")
)
