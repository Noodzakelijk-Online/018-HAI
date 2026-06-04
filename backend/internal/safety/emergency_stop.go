package safety

import (
	"os"
	"strings"
)

const defaultEmergencyStopReason = "emergency stop is active; autonomous execution is blocked until the stop is cleared"

func EmergencyStopActive() bool {
	return truthyEnv("HAI_EMERGENCY_STOP") ||
		truthyEnv("AUTONOMY_EMERGENCY_STOP") ||
		truthyEnv("EMERGENCY_STOP")
}

func EmergencyStopReason() string {
	reason := strings.TrimSpace(os.Getenv("HAI_EMERGENCY_STOP_REASON"))
	if reason != "" {
		return RedactSecrets(reason)
	}
	return defaultEmergencyStopReason
}

func truthyEnv(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on", "enabled":
		return true
	default:
		return false
	}
}
