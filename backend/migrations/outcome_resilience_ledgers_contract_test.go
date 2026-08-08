package migrations

import (
	"strings"
	"testing"
)

func TestOutcomeResilienceMigrationIsScopedBoundedAndAdvisory(t *testing.T) {
	t.Parallel()

	upBytes, err := Files.ReadFile("pre/0023_outcome_resilience_ledgers.up.sql")
	if err != nil {
		t.Fatalf("read outcome and resilience migration: %v", err)
	}
	downBytes, err := Files.ReadFile("pre/0023_outcome_resilience_ledgers.down.sql")
	if err != nil {
		t.Fatalf("read outcome and resilience rollback: %v", err)
	}
	up := strings.ToLower(string(upBytes))
	down := strings.ToLower(string(downBytes))

	for _, fragment := range []string{
		"create table public.outcome_evaluation_outcome_revisions",
		"create table public.outcome_evaluation_evaluations",
		"create table public.outcome_evaluation_corrections",
		"primary key (owner_identity, workspace_id, outcome_id, revision)",
		"foreign key (owner_identity, workspace_id, outcome_id, outcome_revision)",
		"outcome evaluation history is append-only",
		"create table public.resilience_idempotency_records",
		"create table public.resilience_leases",
		"create table public.resilience_worker_heartbeats",
		"create table public.resilience_circuits",
		"create table public.resilience_retry_records",
		"create table public.resilience_recovery_records",
		"create table public.resilience_event_records",
		"primary key (owner_id, workspace_id, idempotency_key)",
		"primary key (owner_id, workspace_id, work_id)",
		"lease_state in ('active', 'released')",
		"circuit_phase in ('closed', 'open', 'half_open')",
		"contract_version = 1",
		"octet_length(payload::text) between 2 and 1048576",
		"resilience history is append-only",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("0023 up migration missing %q", fragment)
		}
	}

	for _, forbidden := range []string{
		"dispatch_authorized",
		"execution_authorized",
		"approval_consumed",
		"authority_granted",
	} {
		if strings.Contains(up, forbidden) {
			t.Errorf("0023 migration must not persist authority field %q", forbidden)
		}
	}
	if strings.Contains(up, " cascade") || strings.Contains(down, " cascade") {
		t.Fatal("0023 migration must not use CASCADE")
	}

	for _, fragment := range []string{
		"refusing to roll back non-empty outcome or resilience ledgers",
		"drop table if exists public.resilience_event_records",
		"drop table if exists public.resilience_idempotency_records",
		"drop table if exists public.outcome_evaluation_corrections",
		"drop table if exists public.outcome_evaluation_outcome_revisions",
		"drop function if exists public.hai_reject_resilience_history_mutation()",
		"drop function if exists public.hai_reject_outcome_evaluation_mutation()",
	} {
		if !strings.Contains(down, fragment) {
			t.Errorf("0023 rollback missing %q", fragment)
		}
	}
}
