// Package autonomypolicy is the HAI Phase 2 autonomy policy engine (§10.13). It
// classifies an operation's risk (Robert taxonomy, §25) and decides how HAI may
// act on it, reusing the Phase-1 autonomy gate. It performs no I/O and is
// deterministic, so decisions are auditable.
package autonomypolicy

import (
	"fmt"
	"strings"
)

// Mode is the background autonomy mode (§13).
type Mode string

const (
	ModePaused           Mode = "paused"
	ModeReadOnly         Mode = "read_only"
	ModeDraftOnly        Mode = "draft_only"
	ModeApprovalRequired Mode = "approval_required"
	ModeAutonomousSafe   Mode = "autonomous_safe"
	ModeEmergencyStopped Mode = "emergency_stopped"
)

func (m Mode) String() string { return string(m) }

func (m Mode) IsValid() bool {
	switch m {
	case ModePaused, ModeReadOnly, ModeDraftOnly, ModeApprovalRequired,
		ModeAutonomousSafe, ModeEmergencyStopped:
		return true
	}
	return false
}

// AllowsBackgroundProcessing reports whether the loop should process at all.
func (m Mode) AllowsBackgroundProcessing() bool {
	return m != ModePaused && m != ModeEmergencyStopped
}

// ParseMode parses a background mode, defaulting to autonomous_safe on empty.
func ParseMode(v string) (Mode, error) {
	s := strings.ToLower(strings.TrimSpace(v))
	if s == "" {
		return ModeAutonomousSafe, nil
	}
	m := Mode(s)
	if !m.IsValid() {
		return "", fmt.Errorf("invalid background mode: %q", v)
	}
	return m, nil
}
