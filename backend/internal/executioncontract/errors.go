package executioncontract

import (
	"fmt"
	"strings"
)

type ErrorCategory string

const (
	ErrorValidation            ErrorCategory = "validation"
	ErrorPolicyDenied          ErrorCategory = "policy_denied"
	ErrorApprovalRequired      ErrorCategory = "approval_required"
	ErrorDeadlineExceeded      ErrorCategory = "deadline_exceeded"
	ErrorScopeViolation        ErrorCategory = "scope_violation"
	ErrorConflict              ErrorCategory = "conflict"
	ErrorDependencyUnavailable ErrorCategory = "dependency_unavailable"
	ErrorRateLimited           ErrorCategory = "rate_limited"
	ErrorTransient             ErrorCategory = "transient"
	ErrorPermanent             ErrorCategory = "permanent"
	ErrorCancelled             ErrorCategory = "cancelled"
	ErrorUnknown               ErrorCategory = "unknown"
)

// ExecutionError is safe to move across process boundaries. Message and
// Details must contain only operator-safe, redacted values.
type ExecutionError struct {
	Code      string            `json:"code"`
	Category  ErrorCategory     `json:"category"`
	Message   string            `json:"message"`
	Retryable bool              `json:"retryable"`
	Details   map[string]string `json:"details,omitempty"`
}

func (e ExecutionError) Error() string {
	if strings.TrimSpace(e.Code) == "" {
		return string(e.Category)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func ValidateExecutionError(value ExecutionError) error {
	if !validErrorCategory(value.Category) {
		return fmt.Errorf("execution error category %q is invalid", value.Category)
	}
	if err := validateIdentifier("execution error code", value.Code, 3, 96); err != nil {
		return err
	}
	if strings.TrimSpace(value.Message) == "" || len(value.Message) > 512 {
		return fmt.Errorf("execution error message must be between 1 and 512 characters")
	}
	if containsSecretText(value.Message) {
		return fmt.Errorf("execution error message contains secret material")
	}
	for key, detail := range value.Details {
		if err := validateMetadataEntry(key, detail); err != nil {
			return fmt.Errorf("execution error detail: %w", err)
		}
	}
	if value.Retryable && !retryableCategory(value.Category) {
		return fmt.Errorf("execution error category %q cannot be retryable", value.Category)
	}
	return nil
}

func validErrorCategory(value ErrorCategory) bool {
	switch value {
	case ErrorValidation,
		ErrorPolicyDenied,
		ErrorApprovalRequired,
		ErrorDeadlineExceeded,
		ErrorScopeViolation,
		ErrorConflict,
		ErrorDependencyUnavailable,
		ErrorRateLimited,
		ErrorTransient,
		ErrorPermanent,
		ErrorCancelled,
		ErrorUnknown:
		return true
	default:
		return false
	}
}

func retryableCategory(value ErrorCategory) bool {
	switch value {
	case ErrorConflict, ErrorDependencyUnavailable, ErrorRateLimited, ErrorTransient, ErrorUnknown:
		return true
	default:
		return false
	}
}
