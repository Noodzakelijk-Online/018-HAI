package migrations

import (
	"strings"
	"testing"
)

func TestOutcomeMonitorCompositionDeliveryMigrationContract(t *testing.T) {
	upBytes, err := Files.ReadFile("pre/0050_outcome_monitor_composition_delivery.up.sql")
	if err != nil {
		t.Fatalf("read up migration: %v", err)
	}
	up := string(upBytes)
	for _, fragment := range []string{
		"CREATE TABLE public.outcome_monitor_composition_deliveries",
		"CREATE TABLE public.outcome_monitor_composition_attempts",
		"delivery_id uuid PRIMARY KEY",
		"target_id uuid NOT NULL",
		"run_id uuid NOT NULL",
		"run_digest character(64) NOT NULL",
		"observation_id uuid",
		"observation_digest character(64)",
		"binding_digest character(64) NOT NULL",
		"status IN ('pending', 'succeeded', 'dead_lettered')",
		"revision bigint NOT NULL DEFAULT 1",
		"lease_generation bigint NOT NULL DEFAULT 0",
		"attempt_count integer NOT NULL DEFAULT 0",
		"max_attempts integer NOT NULL DEFAULT 5",
		"base_backoff_seconds integer NOT NULL DEFAULT 30",
		"max_backoff_seconds integer NOT NULL DEFAULT 3600",
		"next_attempt_at timestamp with time zone",
		"lease_id uuid",
		"lease_owner character varying(255)",
		"lease_until timestamp with time zone",
		"last_attempt_at timestamp with time zone",
		"last_failure_code character varying(80)",
		"attempt_id uuid PRIMARY KEY",
		"claim_id uuid NOT NULL",
		"worker_id character varying(255) NOT NULL",
		"status IN ('succeeded', 'failed')",
		"failure_code character varying(80)",
		"started_at timestamp with time zone NOT NULL",
		"finished_at timestamp with time zone NOT NULL",
		"request_digest character(64) NOT NULL",
		"record_digest character(64) NOT NULL",
		"lease_generation <= revision",
		"UNIQUE (owner_identity, workspace_key, run_id)",
		"FOREIGN KEY (owner_identity, workspace_key, target_id)",
		"FOREIGN KEY (owner_identity, workspace_key, run_id)",
		"FOREIGN KEY (owner_identity, workspace_key, observation_id)",
		"FOREIGN KEY (owner_identity, workspace_key, delivery_id)",
		"composition delivery update must advance revision and time exactly once",
		"composition attempt does not match the current owner-scoped fenced lease",
		"composition delivery settlement does not match immutable attempt receipt",
		"BEFORE UPDATE OR DELETE ON public.outcome_monitor_composition_attempts",
		"BEFORE TRUNCATE ON public.outcome_monitor_composition_attempts",
		"BEFORE DELETE ON public.outcome_monitor_composition_deliveries",
		"BEFORE TRUNCATE ON public.outcome_monitor_composition_deliveries",
		"CREATE CONSTRAINT TRIGGER trg_outcome_monitor_run_enqueue_composition_delivery",
		"DEFERRABLE INITIALLY DEFERRED",
		"ON CONFLICT (delivery_id) DO NOTHING",
		"CREATE EXTENSION IF NOT EXISTS pgcrypto",
		"encode(digest(concat_ws('|',",
		"'composition_binding_v1'",
		"authority = 'advisory_monitor_only'",
		"can_execute = false",
		"delivery_authorized = false",
		"notification_authorized = false",
		"external_effects_authorized = false",
		"learning_mutation_authorized = false",
		"ON UPDATE RESTRICT ON DELETE RESTRICT",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("migration missing %q", fragment)
		}
	}
	if strings.Contains(strings.ToUpper(up), " ON DELETE CASCADE") {
		t.Error("migration must not cascade-delete composition queue or receipts")
	}
	downBytes, err := Files.ReadFile("pre/0050_outcome_monitor_composition_delivery.down.sql")
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}
	down := string(downBytes)
	if !strings.Contains(down, "refusing to roll back non-empty outcome monitor composition delivery ledgers") {
		t.Error("rollback must refuse to discard queue or attempt evidence")
	}
	if strings.Contains(strings.ToUpper(down), " CASCADE") {
		t.Error("rollback must not use CASCADE")
	}
	if strings.Index(down, "DROP TABLE IF EXISTS public.outcome_monitor_composition_attempts") >
		strings.Index(down, "DROP TABLE IF EXISTS public.outcome_monitor_composition_deliveries") {
		t.Error("rollback must remove attempt receipts before their delivery queue")
	}
}
