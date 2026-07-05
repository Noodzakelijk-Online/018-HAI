// Package apierror defines a small, stable catalog of API error codes and a
// consistent JSON error envelope. Handlers can map failures to a known code so
// clients and the troubleshooting guide share one vocabulary.
package apierror

import "net/http"

// Code is a stable, machine-readable error identifier.
type Code string

const (
	CodeBadRequest   Code = "bad_request"
	CodeUnauthorized Code = "unauthorized"
	CodeForbidden    Code = "forbidden"
	CodeNotFound     Code = "not_found"
	CodeConflict     Code = "conflict"
	CodeValidation   Code = "validation_failed"
	CodeRateLimited  Code = "rate_limited"
	CodeUnavailable  Code = "unavailable"
	CodeInternal     Code = "internal_error"
)

// HTTPStatus maps a code to its HTTP status. Unknown codes default to 500 so a
// missing mapping fails safe rather than leaking a 200.
func (c Code) HTTPStatus() int {
	switch c {
	case CodeBadRequest:
		return http.StatusBadRequest
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeForbidden:
		return http.StatusForbidden
	case CodeNotFound:
		return http.StatusNotFound
	case CodeConflict:
		return http.StatusConflict
	case CodeValidation:
		return http.StatusUnprocessableEntity
	case CodeRateLimited:
		return http.StatusTooManyRequests
	case CodeUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// Error is a structured API error carrying a code, a human message, and
// optional field-level details.
type Error struct {
	Code    Code              `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

// New builds an error with the given code and message.
func New(code Code, message string) *Error {
	return &Error{Code: code, Message: message}
}

// WithDetail attaches a field-level detail and returns the error for chaining.
func (e *Error) WithDetail(field, detail string) *Error {
	if e.Details == nil {
		e.Details = map[string]string{}
	}
	e.Details[field] = detail
	return e
}

// Error implements the error interface.
func (e *Error) Error() string {
	return string(e.Code) + ": " + e.Message
}

// HTTPStatus returns the HTTP status for this error's code.
func (e *Error) HTTPStatus() int { return e.Code.HTTPStatus() }

// Envelope is the top-level shape returned to clients: {"error": {...}}.
type Envelope struct {
	Error *Error `json:"error"`
}

// Envelope wraps the error for JSON serialization.
func (e *Error) Envelope() Envelope { return Envelope{Error: e} }
