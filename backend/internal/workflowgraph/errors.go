package workflowgraph

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrDefinitionNotFound = errors.New("workflow graph definition not found")
	ErrRunNotFound        = errors.New("workflow graph run not found")
	ErrRevisionConflict   = errors.New("workflow graph run revision conflict")
	ErrNodeNotActive      = errors.New("workflow graph node is not active")
	ErrOutcomeRequired    = errors.New("workflow graph outcome is required")
	ErrNoMatchingEdge     = errors.New("workflow graph has no matching edge")
	ErrTraversalLimit     = errors.New("workflow graph edge traversal limit reached")
	ErrRunStepLimit       = errors.New("workflow graph run step limit reached")
)

type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	return "invalid workflow graph: " + strings.Join(e.Problems, "; ")
}

func validationProblem(format string, args ...any) error {
	return &ValidationError{Problems: []string{fmt.Sprintf(format, args...)}}
}
