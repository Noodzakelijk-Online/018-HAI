package migrations

import (
	"strings"
	"testing"
)

func TestOutcomeAttentionMonitorMigrationContract(t *testing.T) {
	upBytes, err := Files.ReadFile("pre/0049_outcome_attention_monitor.up.sql")
	if err != nil {
		t.Fatalf("read up migration: %v", err)
	}
	up := string(upBytes)
	for _, fragment := range []string{
		"CREATE TABLE public.outcome_observation_records",
		"CREATE TABLE public.outcome_monitor_targets",
		"CREATE TABLE public.outcome_monitor_commands",
		"CREATE TABLE public.outcome_monitor_runs",
		"lease_owner character varying(255)",
		"result_lease_generation bigint NOT NULL",
		"result_lease_id uuid",
		"result_lease_owner character varying(255)",
		"result_lease_until timestamp with time zone",
		"PRIMARY KEY (owner_identity, workspace_key, operation, idempotency_key)",
		"UNIQUE (owner_identity, workspace_key, idempotency_key)",
		"UNIQUE (owner_identity, workspace_key, record_digest)",
		"FOREIGN KEY (owner_identity, workspace_key, target_id)",
		"authority = 'advisory_monitor_only'",
		"can_execute = false",
		"delivery_authorized = false",
		"execution_authorized = false",
		"uq_outcome_monitor_target_owner_scope",
		"idx_outcome_monitor_targets_due_claim",
		"idx_outcome_observation_scope_history",
		"idx_outcome_monitor_runs_target_history",
		"outcome monitor target owner and scope are immutable",
		"outcome monitor target update must advance revision and time exactly once",
		"outcome monitor run projection does not match immutable run history",
		"outcome monitor run does not match the current owner-scoped claim",
		"BEFORE UPDATE OR DELETE ON public.outcome_observation_records",
		"BEFORE TRUNCATE ON public.outcome_observation_records",
		"BEFORE UPDATE OR DELETE ON public.outcome_monitor_runs",
		"BEFORE TRUNCATE ON public.outcome_monitor_runs",
		"BEFORE UPDATE OR DELETE ON public.outcome_monitor_commands",
		"BEFORE TRUNCATE ON public.outcome_monitor_commands",
		"ON UPDATE RESTRICT ON DELETE RESTRICT",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("migration missing %q", fragment)
		}
	}
	if strings.Contains(strings.ToUpper(up), " ON DELETE CASCADE") {
		t.Error("migration must not cascade-delete outcome monitor history")
	}

	downBytes, err := Files.ReadFile("pre/0049_outcome_attention_monitor.down.sql")
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}
	down := string(downBytes)
	if !strings.Contains(down, "refusing to roll back non-empty outcome attention monitor tables") {
		t.Error("rollback must refuse to discard monitor schedules or evidence")
	}
	if strings.Contains(strings.ToUpper(down), " CASCADE") {
		t.Error("rollback must not use CASCADE")
	}
	if strings.Index(down, "DROP TABLE IF EXISTS public.outcome_monitor_runs") >
		strings.Index(down, "DROP TABLE IF EXISTS public.outcome_monitor_targets") {
		t.Error("rollback must remove run history before its restricted target")
	}
	if strings.Index(down, "DROP TABLE IF EXISTS public.outcome_monitor_commands") >
		strings.Index(down, "DROP TABLE IF EXISTS public.outcome_monitor_targets") {
		t.Error("rollback must remove command history before its restricted target")
	}
}
