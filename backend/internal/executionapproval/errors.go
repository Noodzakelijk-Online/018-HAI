package executionapproval

import "errors"

var (
	ErrInvalidRequest      = errors.New("invalid execution approval request")
	ErrInvalidReference    = errors.New("invalid task review approval reference")
	ErrApprovalUnavailable = errors.New("durable task review approval is unavailable")
	ErrBindingMismatch     = errors.New("task review approval binding digest does not match")
	ErrInvalidDecision     = errors.New("durable task review approval decision is invalid")
	ErrStaleApproval       = errors.New("durable task review approval is stale")
	ErrFutureApproval      = errors.New("durable task review approval timestamp is in the future")
)
