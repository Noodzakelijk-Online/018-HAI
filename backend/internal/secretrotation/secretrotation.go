// Package secretrotation implements a pure, age-based secret-rotation policy:
// given when a secret was last rotated and the current time, decide whether it
// is due for rotation. No I/O.
package secretrotation

import (
	"math"
	"time"
)

// Policy defines the maximum age before a secret must be rotated.
type Policy struct {
	MaxAgeDays int `json:"maxAgeDays"` // <= 0 disables rotation requirements
}

// Enabled reports whether the policy enforces rotation.
func (p Policy) Enabled() bool { return p.MaxAgeDays > 0 }

// Due reports whether a secret last rotated at lastRotated is due for rotation
// at time now.
func (p Policy) Due(lastRotated, now time.Time) bool {
	if !p.Enabled() {
		return false
	}
	deadline := lastRotated.AddDate(0, 0, p.MaxAgeDays)
	return !now.Before(deadline)
}

// DaysUntilDue returns how many whole days remain before rotation is due; 0 when
// already due, and -1 when the policy is disabled.
func (p Policy) DaysUntilDue(lastRotated, now time.Time) int {
	if !p.Enabled() {
		return -1
	}
	deadline := lastRotated.AddDate(0, 0, p.MaxAgeDays)
	remaining := deadline.Sub(now).Hours() / 24
	if remaining <= 0 {
		return 0
	}
	return int(math.Ceil(remaining))
}
