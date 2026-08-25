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
