package agentregistry

import "errors"

var (
	ErrNotFound          = errors.New("agent not found")
	ErrConflict          = errors.New("agent revision conflict")
	ErrAlreadyExists     = errors.New("agent already exists")
	ErrNoEligibleAgent   = errors.New("no eligible agent")
	ErrInvalidTransition = errors.New("invalid agent lifecycle transition")
	ErrAssignmentExists  = errors.New("assignment already exists")
)
