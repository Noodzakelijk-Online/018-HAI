package safety

import (
	"errors"
	"strings"
	"testing"
)

type emergencyStopProviderStub struct {
	engaged bool
	reason  string
	err     error
}

func (s emergencyStopProviderStub) EmergencyStopStatus() (bool, string, error) {
	return s.engaged, s.reason, s.err
}

func TestEmergencyStopActiveReadsSupportedFlags(t *testing.T) {
	t.Setenv("HAI_EMERGENCY_STOP", "true")

	if !EmergencyStopActive() {
		t.Fatalf("emergency stop should be active")
	}
}

func TestEmergencyStopReasonIsRedacted(t *testing.T) {
	t.Setenv("HAI_EMERGENCY_STOP_REASON", "operator stop token=super-secret-token")

	reason := EmergencyStopReason()
	if strings.Contains(reason, "super-secret-token") {
		t.Fatalf("emergency stop reason leaked secret: %s", reason)
	}
}

func TestPersistedEmergencyStopProviderIsUsed(t *testing.T) {
	restore := SetEmergencyStopProvider(emergencyStopProviderStub{
		engaged: true,
		reason:  "operator pause token=super-secret-token",
	})
	defer restore()

	decision := EvaluateEmergencyStop()
	if !decision.Active || decision.Source != "persisted_control" {
		t.Fatalf("decision = %#v, want active persisted control", decision)
	}
	if strings.Contains(decision.Reason, "super-secret-token") {
		t.Fatalf("persisted stop reason leaked secret: %s", decision.Reason)
	}
}

func TestPersistedEmergencyStopProviderFailsClosed(t *testing.T) {
	restore := SetEmergencyStopProvider(emergencyStopProviderStub{
		err: errors.New("state file is unreadable"),
	})
	defer restore()

	decision := EvaluateEmergencyStop()
	if !decision.Active || decision.Source != "persisted_control_error" {
		t.Fatalf("decision = %#v, want fail-closed persisted control error", decision)
	}
	if strings.Contains(decision.Reason, "unreadable") {
		t.Fatalf("provider error leaked through operator reason: %s", decision.Reason)
	}
}

func TestEnvironmentHardStopTakesPrecedence(t *testing.T) {
	restore := SetEmergencyStopProvider(emergencyStopProviderStub{})
	defer restore()
	t.Setenv("AUTONOMY_EMERGENCY_STOP", "on")

	decision := EvaluateEmergencyStop()
	if !decision.Active || decision.Source != "environment" {
		t.Fatalf("decision = %#v, want environment hard stop", decision)
	}
}
