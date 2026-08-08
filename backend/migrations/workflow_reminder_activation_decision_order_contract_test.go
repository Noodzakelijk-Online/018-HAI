package migrations

import (
	"strings"
	"testing"
)

func TestWorkflowReminderActivationDecisionOrderMigrationContract(t *testing.T) {
	upBytes, err := Files.ReadFile("pre/0047_workflow_reminder_activation_decision_order.up.sql")
	if err != nil {
		t.Fatalf("read decision-order migration: %v", err)
	}
	up := string(upBytes)
	for _, required := range []string{
		"validate_workflow_reminder_activation_decision_order_insert",
		"workflow_reminder_activation_decisions_validate_order_insert",
		"NEW.decided_at <= latest_record.decided_at",
		"reminder activation decision time must advance after the current chain tip",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("decision-order migration is missing %q", required)
		}
	}
	downBytes, err := Files.ReadFile("pre/0047_workflow_reminder_activation_decision_order.down.sql")
	if err != nil {
		t.Fatalf("read decision-order rollback: %v", err)
	}
	down := string(downBytes)
	for _, required := range []string{
		"DROP TRIGGER IF EXISTS workflow_reminder_activation_decisions_validate_order_insert",
		"DROP FUNCTION IF EXISTS public.validate_workflow_reminder_activation_decision_order_insert",
	} {
		if !strings.Contains(down, required) {
			t.Fatalf("decision-order rollback is missing %q", required)
		}
	}
}
