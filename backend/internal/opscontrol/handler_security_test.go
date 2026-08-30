package opscontrol

import (
	"errors"
	"strings"
	"testing"
)

func TestPublicControlErrorDoesNotExposeUnexpectedPersistenceDetails(t *testing.T) {
	err := errors.New(`write failed: password=control-secret at C:\\private`)
	message := publicControlError(err)
	for _, forbidden := range []string{"password", "control-secret", "C:\\\\private"} {
		if strings.Contains(strings.ToLower(message), strings.ToLower(forbidden)) {
			t.Fatalf("message leaked %q: %s", forbidden, message)
		}
	}
	if message != "safety-control request could not be completed" {
		t.Fatalf("message = %q", message)
	}
}

func TestPublicControlErrorKeepsActionableSafetyStates(t *testing.T) {
	if got := publicControlError(ErrControlPersistence); got != "safety-control state could not be persisted" {
		t.Fatalf("persistence message = %q", got)
	}
	if got := publicControlError(ErrAutonomyModeStateChanged); got != "safety-control state changed; refresh and retry" {
		t.Fatalf("concurrent message = %q", got)
	}
}

func TestPublicControlReasonCodeDoesNotExposeAuthorizationDetails(t *testing.T) {
	err := controlAuthorizationFailureFor(
		"control.authorization.execution_denied",
		errors.New("provider rejected signature=super-secret"),
	)
	if got := publicControlReasonCode(err); got != "control.authorization.execution_denied" {
		t.Fatalf("reason code = %q", got)
	}
	if strings.Contains(publicControlReasonCode(err), "secret") {
		t.Fatal("reason code leaked internal authorization detail")
	}
}
