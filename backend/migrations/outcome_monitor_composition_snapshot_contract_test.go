package migrations

import (
	"strings"
	"testing"
)

func TestOutcomeMonitorCompositionSnapshotMigrationContract(t *testing.T) {
	upBytes, err := Files.ReadFile("pre/0051_outcome_monitor_composition_snapshot.up.sql")
	if err != nil {
		t.Fatalf("read up migration: %v", err)
	}
	up := string(upBytes)
	for _, fragment := range []string{
		"ADD COLUMN snapshot_status character varying(24)",
		"ADD COLUMN composer_version character varying(120)",
		"ADD COLUMN snapshot_captured_at timestamp with time zone",
		"ADD COLUMN outcome_revision bigint",
		"ADD COLUMN outcome_audit_digest character(64)",
		"ADD COLUMN policy_idempotency_key text",
		"ADD COLUMN policy_payload_digest character(64)",
		"ADD COLUMN policy_recorded_at timestamp with time zone",
		"ADD COLUMN signal_watermark_at timestamp with time zone",
		"ADD COLUMN signal_watermark_key text",
		"ADD COLUMN decision_watermark_at timestamp with time zone",
		"ADD COLUMN decision_watermark_key text",
		"ADD COLUMN feedback_watermark_at timestamp with time zone",
		"ADD COLUMN feedback_watermark_id uuid",
		"ADD COLUMN feedback_watermark_digest character(64)",
		"ADD COLUMN attention_snapshot jsonb",
		"ADD COLUMN attention_snapshot_digest character(64)",
		"ADD COLUMN snapshot_digest character(64)",
		"ALTER TABLE public.outcome_monitor_composition_attempts",
		"snapshot_status IN ('pinned', 'legacy_unpinned')",
		"ambient-outcome-attention-v2",
		"ambient-monitor-composer/pre-0051-unknown",
		"CREATE OR REPLACE FUNCTION public.hai_outcome_monitor_attention_snapshot_json",
		"CREATE OR REPLACE FUNCTION public.hai_outcome_monitor_composition_snapshot_digest",
		"CREATE OR REPLACE FUNCTION public.hai_outcome_monitor_composition_binding_digest",
		"CREATE OR REPLACE FUNCTION public.hai_validate_outcome_monitor_attention_snapshot",
		"CREATE OR REPLACE FUNCTION public.hai_pin_outcome_monitor_composition_snapshot",
		"octet_length(p_attention_snapshot::text) NOT BETWEEN 2 AND 131072",
		"signal_count NOT BETWEEN 0 AND 512",
		"decision_count NOT BETWEEN 0 AND 2048",
		"feedback_count NOT BETWEEN 0 AND 2048",
		"'count', 'windowDigest'",
		"'{inputDigest}'",
		"'{signals,cursor,ordinal}'",
		"'{decisions,cursor,ordinal}'",
		"'{feedback,cursor,feedbackId}'",
		"attention_captured_at < scoped_run.completed_at",
		"attention_captured_at > NEW.created_at",
		"revision = NEW.outcome_revision",
		"audit_digest = NEW.outcome_audit_digest",
		"stored snapshot_digest must equal the exact Go composition snapshot digest",
		"FOREIGN KEY (owner_identity, policy_idempotency_key)",
		"FOREIGN KEY (owner_identity, feedback_watermark_id)",
		"UNIQUE (owner_identity, workspace_key, delivery_id, snapshot_digest)",
		"FOREIGN KEY (owner_identity, workspace_key, delivery_id, snapshot_digest)",
		"NEW.snapshot_digest IS DISTINCT FROM delivery_record.snapshot_digest",
		"composition delivery binding must equal the exact Go snapshot-bound binding digest",
		"NEW.snapshot_digest",
		"UPDATE public.outcome_monitor_composition_attempts AS attempt",
		"ALTER COLUMN snapshot_digest SET NOT NULL",
		"CREATE OR REPLACE FUNCTION public.hai_enqueue_outcome_monitor_composition_delivery",
		"observation.recorded_at <= NEW.completed_at",
		"'legacy_unpinned', 'ambient-monitor-composer/pre-0051-unknown'",
		"'dead_lettered', 1, 0, 0, 5, 30, 3600, NULL",
		"'snapshot_unavailable', NEW.completed_at",
		"CREATE TRIGGER trg_outcome_monitor_composition_delivery_0051_snapshot_insert",
		"CREATE TRIGGER trg_outcome_monitor_composition_delivery_0051_snapshot_immutable",
		"CREATE TRIGGER trg_outcome_monitor_composition_delivery_0051_snapshot_no_truncate",
		"outcome monitor composition snapshot pins are immutable",
		"outcome monitor composition snapshots cannot be deleted",
		"outcome monitor composition snapshots cannot be truncated",
		"Exact Go CompositionSnapshot.SnapshotDigest",
		"Immutable copy of the delivery CompositionSnapshot.SnapshotDigest",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("migration missing %q", fragment)
		}
	}

	for _, forbidden := range []string{
		"hai_resolve_outcome_monitor_composition_snapshot",
		"ambient-monitor-composer/v1",
		"legacy-unpinned-v1",
		"ORDER BY revision DESC, recorded_at DESC, idempotency_key DESC",
		"COALESCE(NEW.snapshot_captured_at",
		"COALESCE(NEW.outcome_revision",
		"composition_binding_v1",
	} {
		if strings.Contains(up, forbidden) {
			t.Errorf("migration retains forbidden snapshot behavior %q", forbidden)
		}
	}
	if strings.Contains(strings.ToUpper(up), " ON DELETE CASCADE") {
		t.Error("snapshot migration must not cascade-delete immutable history")
	}

	digestStart := strings.Index(up, "CREATE OR REPLACE FUNCTION public.hai_outcome_monitor_composition_snapshot_digest")
	if digestStart < 0 {
		t.Fatal("cannot locate Go-compatible snapshot digest function")
	}
	digestEnd := strings.Index(up[digestStart:], "CREATE OR REPLACE FUNCTION public.hai_validate_outcome_monitor_attention_snapshot")
	if digestEnd < 0 {
		t.Fatal("cannot locate Go-compatible snapshot digest function")
	}
	digestBody := up[digestStart : digestStart+digestEnd]
	for _, field := range []string{
		"contractVersion", "status", "composerVersion", "capturedAt",
		"outcomeRevision", "outcomeAuditDigest", "attention", "snapshotDigest",
	} {
		if !strings.Contains(digestBody, field) {
			t.Errorf("Go-compatible snapshot digest does not bind %q", field)
		}
	}
	for _, nested := range []string{
		"ownerIdentity", "policy", "signals", "decisions", "feedback",
		"cursor", "ordinal", "count", "windowDigest", "inputDigest",
	} {
		if !strings.Contains(up, nested) {
			t.Errorf("attention snapshot contract does not bind %q", nested)
		}
	}

	downBytes, err := Files.ReadFile("pre/0051_outcome_monitor_composition_snapshot.down.sql")
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}
	down := string(downBytes)
	for _, fragment := range []string{
		"refusing to roll back non-empty outcome monitor composition snapshot ledgers",
		"CREATE OR REPLACE FUNCTION public.hai_enqueue_outcome_monitor_composition_delivery",
		"CREATE OR REPLACE FUNCTION public.hai_validate_outcome_monitor_composition_attempt_insert",
		"CREATE OR REPLACE FUNCTION public.hai_validate_outcome_monitor_composition_delivery_insert",
		"CREATE OR REPLACE FUNCTION public.hai_validate_outcome_monitor_composition_delivery_write",
		"ADD CONSTRAINT chk_outcome_monitor_composition_delivery_attempt_projection",
		"DROP CONSTRAINT IF EXISTS fk_outcome_monitor_composition_attempt_snapshot",
		"DROP COLUMN IF EXISTS snapshot_digest",
		"DROP COLUMN IF EXISTS attention_snapshot",
		"DROP COLUMN IF EXISTS snapshot_status",
		"DROP FUNCTION IF EXISTS public.hai_pin_outcome_monitor_composition_snapshot",
		"DROP FUNCTION IF EXISTS public.hai_outcome_monitor_composition_snapshot_digest",
		"DROP FUNCTION IF EXISTS public.hai_outcome_monitor_composition_binding_digest",
	} {
		if !strings.Contains(down, fragment) {
			t.Errorf("rollback missing %q", fragment)
		}
	}
	if strings.Contains(strings.ToUpper(down), " CASCADE") {
		t.Error("rollback must not use CASCADE")
	}
}
