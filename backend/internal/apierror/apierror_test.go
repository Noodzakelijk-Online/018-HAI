package apierror

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

func TestHTTPStatusMapping(t *testing.T) {
	cases := map[Code]int{
		CodeBadRequest:  http.StatusBadRequest,
		CodeNotFound:    http.StatusNotFound,
		CodeConflict:    http.StatusConflict,
		CodeValidation:  http.StatusUnprocessableEntity,
		CodeRateLimited: http.StatusTooManyRequests,
		CodeInternal:    http.StatusInternalServerError,
		Code("unknown"): http.StatusInternalServerError, // fail-safe default
	}
	for code, want := range cases {
		if got := code.HTTPStatus(); got != want {
			t.Fatalf("%s status = %d, want %d", code, got, want)
		}
	}
}

func TestErrorStringAndDetails(t *testing.T) {
	err := New(CodeValidation, "content is required").WithDetail("content", "must not be empty")
	if err.Error() != "validation_failed: content is required" {
		t.Fatalf("Error() = %q", err.Error())
	}
	if err.Details["content"] != "must not be empty" {
		t.Fatalf("detail missing: %+v", err.Details)
	}
	if err.HTTPStatus() != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d", err.HTTPStatus())
	}
}

func TestEnvelopeSerialization(t *testing.T) {
	raw, _ := json.Marshal(New(CodeNotFound, "memory not found").Envelope())
	var round struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &round); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if round.Error.Code != "not_found" || round.Error.Message != "memory not found" {
		t.Fatalf("envelope round-trip wrong: %+v", round)
	}
}

func TestPublicMessageDoesNotExposeUnexpectedErrorDetails(t *testing.T) {
	if got := PublicMessage(errors.New(`database password=real-secret at C:\\private`), "Service is unavailable"); got != "Service is unavailable" {
		t.Fatalf("unexpected error message = %q", got)
	}
	if got := PublicMessage(New(CodeValidation, "name is required"), "Service is unavailable"); got != "name is required" {
		t.Fatalf("structured error message = %q", got)
	}
}
