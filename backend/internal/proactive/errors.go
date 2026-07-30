package proactive

import "errors"

var (
	ErrNotFound            = errors.New("not found")
	ErrAlreadyExists       = errors.New("already exists")
	ErrConflict            = errors.New("revision conflict")
	ErrInvalidTransition   = errors.New("invalid proposal transition")
	ErrIdempotencyConflict = errors.New("idempotency key reused with different signal")
	ErrScheduleExhausted   = errors.New("retry and escalation schedule exhausted")
)
