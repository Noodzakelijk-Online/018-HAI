package migrations

import (
	"strings"
	"testing"
)

func TestWorkflowReminderActivationLedgerIsInternalOwnerBoundAndAppendOnly(t *testing.T) {
	upBytes, err := Files.ReadFile("pre/0046_workflow_reminder_activation_ledger.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := string(upBytes)
	for _, fragment := range []string{
		"CREATE TABLE public.workflow_reminder_activation_requests",
		"CREATE TABLE public.workflow_reminder_activation_decisions",
		"UNIQUE (owner_identity, idempotency_key)",
		"activation_kind = 'internal_notification'",
		"authority = 'reminder_activation_request_only'",
		"authority = 'reminder_activation_decision_only'",
		"PREPARE INTERNAL REMINDER ONLY",
		"APPROVE INTERNAL REMINDER PREPARATION",
		"reminder activation request does not match a current owner-scoped reminder",
		"workflow_record.owner_identity IS NULL",
		"reminder activation decision must extend the current chain tip",
		"only the latest approved reminder preparation can be revoked",
		"uq_workflow_reminder_activation_decision_chain_link",
		"workflow reminder activation ledgers are append-only",
		"BEFORE TRUNCATE ON public.workflow_reminder_activation_requests",
		"BEFORE TRUNCATE ON public.workflow_reminder_activation_decisions",
	} {
		if !strings.Contains(up, fragment) {
			t.Fatalf("up migration missing %q", fragment)
		}
	}
	for _, forbidden := range []string{"calendar_event", "email_send", "message_send", "execution_job", "COALESCE(NULLIF(workflow_record.owner_identity, ''), 'system')"} {
		if strings.Contains(up, forbidden) {
			t.Fatalf("non-executing reminder ledger unexpectedly contains %q", forbidden)
		}
	}

	downBytes, err := Files.ReadFile("pre/0046_workflow_reminder_activation_ledger.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	down := string(downBytes)
	for _, fragment := range []string{
		"refusing to remove non-empty workflow reminder activation ledgers",
		"DROP TABLE IF EXISTS public.workflow_reminder_activation_decisions",
		"DROP TABLE IF EXISTS public.workflow_reminder_activation_requests",
	} {
		if !strings.Contains(down, fragment) {
			t.Fatalf("down migration missing %q", fragment)
		}
	}
}
