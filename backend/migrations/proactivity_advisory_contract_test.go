package migrations

import (
	"strings"
	"testing"
)

func TestProactivityAdvisoryMigrationIsScopedImmutableAndReversible(t *testing.T) {
	t.Parallel()

	upBytes, err := Files.ReadFile("pre/0020_proactivity_advisory.up.sql")
	if err != nil {
		t.Fatalf("read proactivity advisory migration: %v", err)
	}
	downBytes, err := Files.ReadFile("pre/0020_proactivity_advisory.down.sql")
	if err != nil {
		t.Fatalf("read proactivity advisory rollback: %v", err)
	}
	up := strings.ToLower(string(upBytes))
	down := strings.ToLower(string(downBytes))

	for _, fragment := range []string{
		"create table public.proactivity_idempotency",
		"create table public.proactivity_policy_records",
		"create table public.proactivity_signal_batches",
		"create table public.proactivity_signal_records",
		"create table public.proactivity_decision_batches",
		"create table public.proactivity_decision_records",
		"primary key (owner_identity, idempotency_key)",
		"unique (owner_identity, idempotency_key, record_kind, payload_digest)",
		"foreign key (owner_identity, idempotency_key, record_kind, payload_digest)",
		"foreign key (owner_identity, batch_idempotency_key)",
		"jsonb_array_length(payload) = signal_count",
		"jsonb_array_length(payload #> '{result,decisions}') = decision_count",
		"executionauthorized}' = 'false'",
		"deliveryauthorized}' = 'false'",
		"authoritygranted}' = 'false'",
		"proactivity signal batch child count is inconsistent",
		"proactivity decision batch child payload is inconsistent",
		"proactivity advisory records are append-only",
		"before update or delete on public.proactivity_idempotency",
		"before truncate on public.proactivity_decision_records",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("proactivity advisory up migration missing %q", fragment)
		}
	}
	if strings.Contains(up, " cascade") || strings.Contains(down, " cascade") {
		t.Fatal("proactivity advisory migration must not use CASCADE")
	}
	for _, fragment := range []string{
		"refusing to roll back non-empty proactivity advisory tables",
		"drop table if exists public.proactivity_decision_records",
		"drop table if exists public.proactivity_decision_batches",
		"drop table if exists public.proactivity_signal_records",
		"drop table if exists public.proactivity_signal_batches",
		"drop table if exists public.proactivity_policy_records",
		"drop table if exists public.proactivity_idempotency",
		"drop function if exists public.hai_reject_proactivity_mutation()",
	} {
		if !strings.Contains(down, fragment) {
			t.Errorf("proactivity advisory down migration missing %q", fragment)
		}
	}
}

func TestProactivityAdvisoryMigrationProtectsEveryLedgerTable(t *testing.T) {
	t.Parallel()
	contents, err := Files.ReadFile("pre/0020_proactivity_advisory.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := strings.ToLower(string(contents))
	for _, table := range []string{
		"proactivity_idempotency",
		"proactivity_policy_records",
		"proactivity_signal_batches",
		"proactivity_signal_records",
		"proactivity_decision_batches",
		"proactivity_decision_records",
	} {
		for _, suffix := range []string{"_immutable", "_no_truncate"} {
			if !strings.Contains(up, "trg_"+table+suffix) {
				t.Errorf("%s is missing %s protection", table, suffix)
			}
		}
	}
}
