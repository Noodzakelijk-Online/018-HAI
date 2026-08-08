package migrations

import (
	"strings"
	"testing"
)

func TestProactivityAttentionFeedbackMigrationIsImmutableAndNonExecuting(t *testing.T) {
	upBytes, err := Files.ReadFile("pre/0048_proactivity_attention_feedback.up.sql")
	if err != nil {
		t.Fatalf("read up migration: %v", err)
	}
	up := string(upBytes)
	for _, fragment := range []string{
		"CREATE TABLE public.proactivity_feedback_records",
		"attention_feedback_only",
		"can_execute = false",
		"delivery_authorized = false",
		"execution_authorized = false",
		"proactivity feedback source decision is stale or unavailable",
		"proactivity feedback chain tip does not match",
		"BEFORE UPDATE OR DELETE",
		"BEFORE TRUNCATE",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("migration missing %q", fragment)
		}
	}

	downBytes, err := Files.ReadFile("pre/0048_proactivity_attention_feedback.down.sql")
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}
	down := string(downBytes)
	if !strings.Contains(down, "refusing to roll back non-empty proactivity attention feedback ledger") {
		t.Error("rollback must preserve non-empty feedback evidence")
	}
	if strings.Contains(strings.ToUpper(down), " CASCADE") {
		t.Error("rollback must not use CASCADE")
	}
}
