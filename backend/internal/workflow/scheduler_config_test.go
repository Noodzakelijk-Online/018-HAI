package workflow

import "testing"

func TestLegacyFallbackRequiresExplicitOptIn(t *testing.T) {
	t.Setenv("DURABLE_SCHEDULER_LEGACY_FALLBACK_ENABLED", "")
	if legacyFallbackEnabled() {
		t.Fatal("legacy fallback must be disabled by default")
	}
	t.Setenv("DURABLE_SCHEDULER_LEGACY_FALLBACK_ENABLED", "yes")
	if !legacyFallbackEnabled() {
		t.Fatal("legacy fallback should accept explicit opt-in")
	}
}
