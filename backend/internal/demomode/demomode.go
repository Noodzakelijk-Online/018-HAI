// Package demomode makes the run mode explicit and impossible to confuse with
// production. Demo/test modes are clearly labelled and must never perform real
// external side effects.
package demomode

import "strings"

// Mode is the operating mode of the system.
type Mode string

const (
	Production Mode = "production"
	Demo       Mode = "demo"
	Test       Mode = "test"
)

// Parse maps arbitrary input to a Mode, defaulting to Production so an unknown
// or empty value fails safe toward the strictest behaviour.
func Parse(value string) Mode {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "demo":
		return Demo
	case "test":
		return Test
	default:
		return Production
	}
}

// IsProduction reports whether the mode is production.
func (m Mode) IsProduction() bool { return m == Production }

// AllowsRealSideEffects reports whether real external side effects are permitted.
// Only production may perform them.
func (m Mode) AllowsRealSideEffects() bool { return m == Production }

// Label returns a UI banner label; production has no banner.
func (m Mode) Label() string {
	switch m {
	case Demo:
		return "[DEMO — no real actions are performed]"
	case Test:
		return "[TEST — no real actions are performed]"
	default:
		return ""
	}
}
