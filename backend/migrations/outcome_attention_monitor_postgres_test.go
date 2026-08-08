package migrations_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/infra"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestOutcomeAttentionMonitorMigrationLifecycle(t *testing.T) {
	db := openIsolatedMigrationDatabase(t)
	const version = "pre/0049_outcome_attention_monitor"
	files := migrationFilesThrough(t, version)
	if _, err := infra.ApplyMigrations(db, files, "pre"); err != nil {
		t.Fatalf("apply pre migrations: %v", err)
	}
	for _, table := range []string{
		"outcome_observation_records",
		"outcome_monitor_targets",
		"outcome_monitor_commands",
		"outcome_monitor_runs",
	} {
		var count int64
		if err := db.Raw("SELECT count(*) FROM public." + table).Scan(&count).Error; err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("isolated %s contains %d records", table, count)
		}
	}

	if err := infra.RollbackMigration(db, files, "pre", version); err != nil {
		t.Fatalf("roll back empty monitor migration: %v", err)
	}
	if outcomeMonitorRelationExists(t, db, "outcome_monitor_targets") {
		t.Fatal("outcome_monitor_targets still exists after empty rollback")
	}
	if count, err := infra.ApplyMigrations(db, files, "pre"); err != nil || count != 1 {
		t.Fatalf("reapply monitor migration = (%d, %v), want (1, nil)", count, err)
	}

	owner := "outcome-monitor-" + uuid.NewString()
	otherOwner := "outcome-monitor-" + uuid.NewString()
	observationID := uuid.New()
	requestDigest := strings.Repeat("a", 64)
	recordDigest := strings.Repeat("b", 64)
	sourceDigest := strings.Repeat("c", 64)
	observedAt := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Microsecond)
	recordedAt := observedAt.Add(time.Minute)
	if err := db.Exec(`
		INSERT INTO public.outcome_observation_records (
			observation_id, owner_identity, workspace_key, outcome_key, indicator_key,
			source_kind, source_key, source_digest, numeric_value, unit,
			idempotency_key, request_digest, record_digest, observed_at, recorded_at
		) VALUES (?, ?, 'personal-os', 'weekly-progress', 'verified-completions',
			'workflow', 'workflow-summary:2026-W32', ?, 7.25, 'count',
			'observation-1', ?, ?, ?, ?)`,
		observationID, owner, sourceDigest, requestDigest, recordDigest, observedAt, recordedAt).Error; err != nil {
		t.Fatalf("insert observation: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO public.outcome_observation_records (
			observation_id, owner_identity, workspace_key, outcome_key, indicator_key,
			source_kind, source_key, source_digest, numeric_value, unit,
			idempotency_key, request_digest, record_digest, observed_at, recorded_at,
			authority, can_execute
		) VALUES (?, ?, 'personal-os', 'unsafe', 'execution',
			'workflow', 'unsafe-source', ?, 1, 'count',
			'unsafe-observation', ?, ?, ?, ?, 'advisory_monitor_only', true)`,
		uuid.New(), owner, strings.Repeat("d", 64), strings.Repeat("e", 64),
		strings.Repeat("f", 64), observedAt, recordedAt).Error; err == nil {
		t.Fatal("authority-bearing observation was accepted")
	}

	targetID := uuid.New()
	createdAt := time.Now().UTC().Truncate(time.Microsecond)
	if err := db.Exec(`
		INSERT INTO public.outcome_monitor_targets (
			target_id, owner_identity, workspace_key, outcome_key, indicator_key,
			source_kind, cadence_seconds, next_run_at, created_at, updated_at
		) VALUES (?, ?, 'personal-os', 'weekly-progress', 'verified-completions',
			'workflow', 3600, ?, ?, ?)`,
		targetID, owner, createdAt, createdAt, createdAt).Error; err != nil {
		t.Fatalf("insert target: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO public.outcome_monitor_commands (
			owner_identity, workspace_key, operation, idempotency_key, request_digest,
			target_id, result_revision, result_lease_generation, result_enabled,
			result_next_run_at, result_updated_at, recorded_at
		) VALUES (?, 'personal-os', 'create_target', 'create-target-1', ?, ?, 1, 0, true, ?, ?, ?)`,
		owner, strings.Repeat("8", 64), targetID, createdAt, createdAt, createdAt).Error; err != nil {
		t.Fatalf("insert target command: %v", err)
	}
	if err := db.Exec(`
		UPDATE public.outcome_monitor_targets
		SET owner_identity = ?, revision = 2, updated_at = ?
		WHERE target_id = ?`, otherOwner, createdAt.Add(time.Second), targetID).Error; err == nil ||
		!strings.Contains(err.Error(), "owner and scope are immutable") {
		t.Fatalf("owner mutation error = %v", err)
	}
	if err := db.Exec(`
		UPDATE public.outcome_monitor_targets
		SET revision = 3, updated_at = ?
		WHERE target_id = ?`, createdAt.Add(time.Second), targetID).Error; err == nil ||
		!strings.Contains(err.Error(), "advance revision") {
		t.Fatalf("revision skip error = %v", err)
	}

	claimID := uuid.New()
	leaseOwner := "worker/region-eu/node-" + strings.Repeat("a", 80)
	claimedAt := createdAt.Add(2 * time.Second)
	leaseUntil := claimedAt.Add(10 * time.Minute)
	if err := db.Exec(`
		UPDATE public.outcome_monitor_targets
		SET lease_id = ?, lease_owner = ?, lease_until = ?, revision = 2, updated_at = ?
		WHERE target_id = ?`, claimID, leaseOwner, leaseUntil, claimedAt, targetID).Error; err != nil {
		t.Fatalf("claim target: %v", err)
	}

	startedAt := claimedAt.Add(time.Second)
	completedAt := startedAt.Add(time.Second)
	runID := uuid.New()
	runRecordDigest := strings.Repeat("1", 64)
	if err := db.Exec(`
		INSERT INTO public.outcome_monitor_runs (
			run_id, target_id, owner_identity, workspace_key, target_revision, claim_id, claimed_at,
			status, observation_count, signal_count, idempotency_key,
			request_digest, record_digest, started_at, completed_at
		) VALUES (?, ?, ?, 'personal-os', 2, ?, ?, 'succeeded', 1, 1, 'run-1', ?, ?, ?, ?)`,
		runID, targetID, owner, claimID, claimedAt, strings.Repeat("2", 64),
		runRecordDigest, startedAt, completedAt).Error; err != nil {
		t.Fatalf("insert monitor run: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO public.outcome_monitor_runs (
			run_id, target_id, owner_identity, workspace_key, target_revision, claim_id, claimed_at,
			status, observation_count, signal_count, idempotency_key,
			request_digest, record_digest, started_at, completed_at
		) VALUES (?, ?, ?, 'personal-os', 2, ?, ?, 'succeeded', 0, 0, 'wrong-owner-run', ?, ?, ?, ?)`,
		uuid.New(), targetID, otherOwner, claimID, claimedAt, strings.Repeat("3", 64),
		strings.Repeat("4", 64), startedAt, completedAt).Error; err == nil ||
		!strings.Contains(err.Error(), "current owner-scoped claim") {
		t.Fatalf("cross-owner run error = %v", err)
	}
	if err := db.Exec(`
		INSERT INTO public.outcome_monitor_runs (
			run_id, target_id, owner_identity, workspace_key, target_revision, claim_id, claimed_at,
			status, error_message_redacted, error_was_redacted, idempotency_key,
			request_digest, record_digest, started_at, completed_at
		) VALUES (?, ?, ?, 'personal-os', 2, ?, ?, 'failed', 'provider request failed', false,
			'unredacted-run', ?, ?, ?, ?)`,
		uuid.New(), targetID, owner, claimID, claimedAt, strings.Repeat("5", 64),
		strings.Repeat("6", 64), startedAt, completedAt).Error; err == nil {
		t.Fatal("failed run without an explicit redaction guarantee was accepted")
	}

	settledAt := completedAt.Add(time.Second)
	if err := db.Exec(`
		UPDATE public.outcome_monitor_targets
		SET lease_id = NULL, lease_owner = NULL, lease_until = NULL, last_run_at = ?,
			last_result = 'succeeded', last_digest = ?, next_run_at = ?,
			revision = 3, updated_at = ?
		WHERE target_id = ?`, completedAt, strings.Repeat("7", 64),
		settledAt.Add(time.Hour), settledAt, targetID).Error; err == nil ||
		!strings.Contains(err.Error(), "does not match immutable run history") {
		t.Fatalf("forged run projection error = %v", err)
	}
	if err := db.Exec(`
		UPDATE public.outcome_monitor_targets
		SET lease_id = NULL, lease_owner = NULL, lease_until = NULL, last_run_at = ?,
			last_result = 'succeeded', last_digest = ?, next_run_at = ?,
			revision = 3, updated_at = ?
		WHERE target_id = ?`, completedAt, runRecordDigest,
		settledAt.Add(time.Hour), settledAt, targetID).Error; err != nil {
		t.Fatalf("settle target: %v", err)
	}

	workspaceTwo := "personal-os-2"
	if err := db.Exec(`
		INSERT INTO public.outcome_observation_records (
			observation_id, owner_identity, workspace_key, outcome_key, indicator_key,
			source_kind, source_key, source_digest, numeric_value, unit,
			idempotency_key, request_digest, record_digest, observed_at, recorded_at
		) VALUES (?, ?, ?, 'weekly-progress', 'verified-completions',
			'workflow', 'workflow-summary:2026-W32', ?, 7.25, 'count',
			'observation-1', ?, ?, ?, ?)`,
		uuid.New(), owner, workspaceTwo, sourceDigest, requestDigest, recordDigest, observedAt, recordedAt).Error; err != nil {
		t.Fatalf("cross-workspace observation idempotency should not conflict: %v", err)
	}
	workspaceTargetID := uuid.New()
	workspaceClaimID := uuid.New()
	if err := db.Exec(`
		INSERT INTO public.outcome_monitor_targets (
			target_id, owner_identity, workspace_key, outcome_key, indicator_key,
			source_kind, cadence_seconds, next_run_at, created_at, updated_at
		) VALUES (?, ?, ?, 'weekly-progress', 'verified-completions',
			'workflow', 3600, ?, ?, ?)`,
		workspaceTargetID, owner, workspaceTwo, createdAt, createdAt, createdAt).Error; err != nil {
		t.Fatalf("insert cross-workspace target: %v", err)
	}
	if err := db.Exec(`
		UPDATE public.outcome_monitor_targets
		SET lease_id = ?, lease_owner = ?, lease_until = ?, revision = 2, updated_at = ?
		WHERE target_id = ?`, workspaceClaimID, leaseOwner, leaseUntil, claimedAt, workspaceTargetID).Error; err != nil {
		t.Fatalf("claim cross-workspace target: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO public.outcome_monitor_runs (
			run_id, target_id, owner_identity, workspace_key, target_revision, claim_id, claimed_at,
			status, observation_count, signal_count, idempotency_key,
			request_digest, record_digest, started_at, completed_at
		) VALUES (?, ?, ?, ?, 2, ?, ?, 'succeeded', 1, 1, 'run-1', ?, ?, ?, ?)`,
		uuid.New(), workspaceTargetID, owner, workspaceTwo, workspaceClaimID, claimedAt,
		strings.Repeat("2", 64), runRecordDigest, startedAt, completedAt).Error; err != nil {
		t.Fatalf("cross-workspace run idempotency should not conflict: %v", err)
	}

	disabledTargetID := uuid.New()
	disabledClaimID := uuid.New()
	if err := db.Exec(`
		INSERT INTO public.outcome_monitor_targets (
			target_id, owner_identity, workspace_key, outcome_key, indicator_key,
			source_kind, cadence_seconds, next_run_at, created_at, updated_at
		) VALUES (?, ?, 'personal-os', 'governed-disable', 'open-loops',
			'workflow', 3600, ?, ?, ?)`, disabledTargetID, owner, createdAt, createdAt, createdAt).Error; err != nil {
		t.Fatalf("insert governed-disable target: %v", err)
	}
	if err := db.Exec(`
		UPDATE public.outcome_monitor_targets
		SET lease_id = ?, lease_owner = ?, lease_until = ?, revision = 2, updated_at = ?
		WHERE target_id = ?`, disabledClaimID, leaseOwner, leaseUntil, claimedAt, disabledTargetID).Error; err != nil {
		t.Fatalf("claim governed-disable target: %v", err)
	}
	disabledAt := claimedAt.Add(time.Second)
	if err := db.Exec(`
		UPDATE public.outcome_monitor_targets
		SET enabled = false, lease_id = NULL, lease_owner = NULL, lease_until = NULL,
			revision = 3, updated_at = ?
		WHERE target_id = ?`, disabledAt, disabledTargetID).Error; err != nil {
		t.Fatalf("governed disable should revoke active lease: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO public.outcome_monitor_runs (
			run_id, target_id, owner_identity, workspace_key, target_revision, claim_id, claimed_at,
			status, observation_count, signal_count, idempotency_key,
			request_digest, record_digest, started_at, completed_at
		) VALUES (?, ?, ?, 'personal-os', 2, ?, ?, 'succeeded', 0, 0,
			'stale-disabled-run', ?, ?, ?, ?)`,
		uuid.New(), disabledTargetID, owner, disabledClaimID, claimedAt,
		strings.Repeat("9", 64), strings.Repeat("a", 64), startedAt, completedAt).Error; err == nil ||
		!strings.Contains(err.Error(), "current owner-scoped claim") {
		t.Fatalf("stale worker run after governed disable error = %v", err)
	}

	for label, statement := range map[string]string{
		"observation update":   `UPDATE public.outcome_observation_records SET numeric_value = 8 WHERE observation_id = '` + observationID.String() + `'`,
		"observation delete":   `DELETE FROM public.outcome_observation_records WHERE observation_id = '` + observationID.String() + `'`,
		"observation truncate": `TRUNCATE public.outcome_observation_records`,
		"run update":           `UPDATE public.outcome_monitor_runs SET observation_count = 2 WHERE run_id = '` + runID.String() + `'`,
		"run delete":           `DELETE FROM public.outcome_monitor_runs WHERE run_id = '` + runID.String() + `'`,
		"run truncate":         `TRUNCATE public.outcome_monitor_runs`,
		"command update":       `UPDATE public.outcome_monitor_commands SET result_enabled = false WHERE idempotency_key = 'create-target-1'`,
		"command delete":       `DELETE FROM public.outcome_monitor_commands WHERE idempotency_key = 'create-target-1'`,
		"command truncate":     `TRUNCATE public.outcome_monitor_commands`,
	} {
		if err := db.Exec(statement).Error; err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Errorf("%s error = %v", label, err)
		}
	}
	if err := db.Exec(`DELETE FROM public.outcome_monitor_targets WHERE target_id = ?`, targetID).Error; err == nil ||
		!strings.Contains(err.Error(), "must be disabled") {
		t.Fatalf("target delete error = %v", err)
	}

	rollbackFixture := errors.New("rollback fixture")
	err := db.Transaction(func(tx *gorm.DB) error {
		err := infra.RollbackMigration(tx, files, "pre", version)
		if err == nil || !strings.Contains(err.Error(), "refusing to roll back non-empty") {
			t.Fatalf("non-empty rollback error = %v", err)
		}
		return rollbackFixture
	})
	if !errors.Is(err, rollbackFixture) {
		t.Fatalf("rollback fixture transaction = %v", err)
	}
	for _, relation := range []string{
		"outcome_observation_records",
		"outcome_monitor_targets",
		"outcome_monitor_commands",
		"outcome_monitor_runs",
	} {
		if !outcomeMonitorRelationExists(t, db, relation) {
			t.Fatalf("refused rollback removed %s", relation)
		}
	}
}

func outcomeMonitorRelationExists(t *testing.T, db *gorm.DB, relation string) bool {
	t.Helper()
	var exists bool
	if err := db.Raw(`SELECT to_regclass('public.' || ?) IS NOT NULL`, relation).Row().Scan(&exists); err != nil {
		t.Fatalf("check relation %s: %v", relation, err)
	}
	return exists
}
