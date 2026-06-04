package safety

import (
	"strings"
	"testing"
)

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
