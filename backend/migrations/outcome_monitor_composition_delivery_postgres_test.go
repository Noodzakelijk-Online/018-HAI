package migrations_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/infra"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestOutcomeMonitorCompositionDeliveryMigrationLifecycle(t *testing.T) {
	db := openIsolatedMigrationDatabase(t)
	const version = "pre/0050_outcome_monitor_composition_delivery"
	files := migrationFilesThrough(t, version)
	if _, err := infra.ApplyMigrations(db, files, "pre"); err != nil {
		t.Fatalf("apply pre migrations: %v", err)
	}
	for _, table := range []string{
		"outcome_monitor_composition_deliveries",
		"outcome_monitor_composition_attempts",
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
		t.Fatalf("roll back empty composition migration: %v", err)
	}
	if outcomeMonitorRelationExists(t, db, "outcome_monitor_composition_deliveries") {
		t.Fatal("composition delivery queue still exists after empty rollback")
	}
	// This run exists before 0050 and proves the migration closes the crash
	// window by backfilling it before normal trigger-based enqueueing begins.
	owner := "composition-owner-" + uuid.NewString()
	workspace := "personal-os"
	fixture := insertCompositionMonitorRun(t, db, owner, workspace, "pre0050-backfill")
	if count, err := infra.ApplyMigrations(db, files, "pre"); err != nil || count != 1 {
		t.Fatalf("reapply composition migration = (%d, %v), want (1, nil)", count, err)
	}

	type deliveryProjection struct {
		DeliveryID         uuid.UUID
		TargetID           uuid.UUID
		RunID              uuid.UUID
		RunDigest          string
		ObservationID      *uuid.UUID
		ObservationDigest  *string
		Status             string
		Revision           int64
		LeaseGeneration    int64
		AttemptCount       int
		MaxAttempts        int
		BindingDigest      string
		CanExecute         bool
		DeliveryAuthorized bool
	}
	var delivery deliveryProjection
	if err := db.Raw(`
		SELECT delivery_id, target_id, run_id, run_digest, observation_id,
		       observation_digest, status, revision, lease_generation,
		       attempt_count, max_attempts, binding_digest, can_execute, delivery_authorized
		FROM public.outcome_monitor_composition_deliveries
		WHERE owner_identity = ? AND workspace_key = ? AND run_id = ?`,
		owner, workspace, fixture.runID).Scan(&delivery).Error; err != nil {
		t.Fatalf("read auto-enqueued delivery: %v", err)
	}
	if delivery.DeliveryID != fixture.runID || delivery.RunID != fixture.runID ||
		delivery.TargetID != fixture.targetID || delivery.RunDigest != fixture.runDigest ||
		delivery.ObservationID == nil || *delivery.ObservationID != fixture.observationID ||
		delivery.ObservationDigest == nil || *delivery.ObservationDigest != fixture.observationDigest ||
		delivery.Status != "pending" || delivery.Revision != 1 || delivery.LeaseGeneration != 0 ||
		delivery.AttemptCount != 0 || delivery.MaxAttempts != 5 ||
		delivery.BindingDigest != digest(strings.Join([]string{
			"composition_binding_v1", owner, workspace, fixture.runID.String(),
			fixture.targetID.String(), fixture.runDigest, fixture.observationID.String(),
			fixture.observationDigest,
		}, "|")) ||
		delivery.CanExecute || delivery.DeliveryAuthorized {
		t.Fatalf("unexpected initial delivery projection: %+v", delivery)
	}

	otherOwner := "composition-owner-" + uuid.NewString()
	if err := db.Exec(`
		INSERT INTO public.outcome_monitor_composition_deliveries (
			delivery_id, owner_identity, workspace_key, target_id, run_id,
			run_digest, status, revision, lease_generation, attempt_count,
			max_attempts, base_backoff_seconds, max_backoff_seconds,
			next_attempt_at, created_at, updated_at, binding_digest
		) VALUES (?, ?, ?, ?, ?, ?, 'pending', 1, 0, 0, 5, 30, 3600, ?, ?, ?, ?)`,
		uuid.New(), otherOwner, workspace, fixture.targetID, fixture.runID,
		fixture.runDigest, fixture.completedAt, fixture.completedAt, fixture.completedAt,
		digest("cross-owner-binding")).Error; err == nil {
		t.Fatal("cross-owner composition delivery was accepted")
	}
	if err := db.Exec(`
		UPDATE public.outcome_monitor_composition_deliveries
		SET can_execute = true, revision = 2, updated_at = ?
		WHERE delivery_id = ?`, fixture.completedAt.Add(time.Second), delivery.DeliveryID).Error; err == nil ||
		!strings.Contains(err.Error(), "capabilities are immutable") {
		t.Fatalf("authority mutation error = %v", err)
	}

	worker := "composition-worker/eu-1"
	claimOne := uuid.New()
	claimedOneAt := fixture.completedAt.Add(time.Second)
	leaseOneUntil := claimedOneAt.Add(10 * time.Minute)
	if err := db.Exec(`
		UPDATE public.outcome_monitor_composition_deliveries
		SET lease_id = ?, lease_owner = ?, lease_until = ?, lease_generation = 3,
			revision = 2, updated_at = ? WHERE delivery_id = ?`,
		claimOne, worker, leaseOneUntil, claimedOneAt, delivery.DeliveryID).Error; err == nil {
		t.Fatal("lease generation greater than revision was accepted")
	}
	if err := db.Exec(`
		UPDATE public.outcome_monitor_composition_deliveries
		SET lease_id = ?, lease_owner = ?, lease_until = ?, lease_generation = 1,
			revision = 2, updated_at = ?
		WHERE delivery_id = ?`, claimOne, worker, leaseOneUntil, claimedOneAt,
		delivery.DeliveryID).Error; err != nil {
		t.Fatalf("claim first composition attempt: %v", err)
	}

	failedAttemptID := uuid.New()
	failedFinishedAt := claimedOneAt.Add(time.Second)
	if err := db.Exec(`
		INSERT INTO public.outcome_monitor_composition_attempts (
			attempt_id, delivery_id, owner_identity, workspace_key, target_id, run_id,
			run_digest, attempt_number, queue_revision, lease_generation, claim_id, worker_id, status,
			failure_code,
			started_at, finished_at, request_digest, record_digest
		) VALUES (?, ?, ?, ?, ?, ?, ?, 1, 1, 1, ?, ?, 'failed', 'stale_claim',
			?, ?, ?, ?)`,
		failedAttemptID, delivery.DeliveryID, owner, workspace, fixture.targetID,
		fixture.runID, fixture.runDigest, claimOne, worker,
		claimedOneAt, failedFinishedAt, digest("stale-request"), digest("stale-record")).Error; err == nil ||
		!strings.Contains(err.Error(), "fenced lease") {
		t.Fatalf("stale queue revision error = %v", err)
	}

	failedAttemptID = uuid.New()
	failedRecordDigest := digest("failed-record-1")
	if err := db.Exec(`
		INSERT INTO public.outcome_monitor_composition_attempts (
			attempt_id, delivery_id, owner_identity, workspace_key, target_id, run_id,
			run_digest, attempt_number, lease_generation, claim_id, worker_id, status,
			failure_code,
			started_at, finished_at, request_digest, record_digest
		) VALUES (?, ?, ?, ?, ?, ?, ?, 1, 1, ?, ?, 'failed', 'provider_unavailable',
			?, ?, ?, ?)`,
		failedAttemptID, delivery.DeliveryID, owner, workspace, fixture.targetID,
		fixture.runID, fixture.runDigest, claimOne, worker,
		claimedOneAt, failedFinishedAt, digest("failed-request-1"), failedRecordDigest).Error; err != nil {
		t.Fatalf("insert failed attempt receipt: %v", err)
	}
	settledOneAt := failedFinishedAt.Add(time.Second)
	nextAttemptAt := settledOneAt.Add(30 * time.Second)
	if err := db.Exec(`
		UPDATE public.outcome_monitor_composition_deliveries
		SET lease_id = NULL, lease_owner = NULL, lease_until = NULL,
			attempt_count = 1, next_attempt_at = ?, last_attempt_at = ?,
			last_failure_code = 'provider_unavailable', revision = 3, updated_at = ?
		WHERE delivery_id = ?`, nextAttemptAt, failedFinishedAt, settledOneAt,
		delivery.DeliveryID).Error; err != nil {
		t.Fatalf("settle failed attempt for retry: %v", err)
	}
	var storedQueueRevision int64
	if err := db.Raw(`SELECT queue_revision FROM public.outcome_monitor_composition_attempts
		WHERE attempt_id = ?`, failedAttemptID).Scan(&storedQueueRevision).Error; err != nil || storedQueueRevision != 2 {
		t.Fatalf("trigger-derived attempt queue revision = (%d, %v), want (2, nil)", storedQueueRevision, err)
	}

	claimTwo := uuid.New()
	claimedTwoAt := nextAttemptAt.Add(time.Second)
	leaseTwoUntil := claimedTwoAt.Add(10 * time.Minute)
	if err := db.Exec(`
		UPDATE public.outcome_monitor_composition_deliveries
		SET lease_id = ?, lease_owner = ?, lease_until = ?, lease_generation = 1,
			revision = 4, updated_at = ? WHERE delivery_id = ?`,
		claimTwo, worker, leaseTwoUntil, claimedTwoAt, delivery.DeliveryID).Error; err == nil ||
		!strings.Contains(err.Error(), "claim must fence") {
		t.Fatalf("non-monotonic reclaim error = %v", err)
	}
	if err := db.Exec(`
		UPDATE public.outcome_monitor_composition_deliveries
		SET lease_id = ?, lease_owner = ?, lease_until = ?, lease_generation = 2,
			revision = 4, updated_at = ?
		WHERE delivery_id = ?`, claimTwo, worker, leaseTwoUntil, claimedTwoAt,
		delivery.DeliveryID).Error; err != nil {
		t.Fatalf("claim second composition attempt: %v", err)
	}
	successAttemptID := uuid.New()
	successFinishedAt := claimedTwoAt.Add(time.Second)
	if err := db.Exec(`
		INSERT INTO public.outcome_monitor_composition_attempts (
			attempt_id, delivery_id, owner_identity, workspace_key, target_id, run_id,
			run_digest, attempt_number, queue_revision, lease_generation, claim_id, worker_id, status,
			started_at, finished_at, request_digest, record_digest
		) VALUES (?, ?, ?, ?, ?, ?, ?, 2, 4, 2, ?, ?, 'succeeded', ?, ?, ?, ?)`,
		successAttemptID, delivery.DeliveryID, owner, workspace, fixture.targetID,
		fixture.runID, fixture.runDigest, claimTwo, worker,
		claimedTwoAt, successFinishedAt, digest("success-request-2"),
		digest("success-record-2")).Error; err != nil {
		t.Fatalf("insert successful attempt receipt: %v", err)
	}
	settledTwoAt := successFinishedAt.Add(time.Second)
	if err := db.Exec(`
		UPDATE public.outcome_monitor_composition_deliveries
		SET status = 'succeeded', lease_id = NULL, lease_owner = NULL, lease_until = NULL,
			attempt_count = 2, next_attempt_at = NULL, last_attempt_at = ?,
			last_failure_code = NULL, completed_at = ?, revision = 5, updated_at = ?
		WHERE delivery_id = ?`, successFinishedAt, successFinishedAt, settledTwoAt,
		delivery.DeliveryID).Error; err != nil {
		t.Fatalf("settle successful composition delivery: %v", err)
	}
	if err := db.Exec(`
		UPDATE public.outcome_monitor_composition_deliveries
		SET revision = 6, updated_at = ? WHERE delivery_id = ?`,
		settledTwoAt.Add(time.Second), delivery.DeliveryID).Error; err == nil ||
		!strings.Contains(err.Error(), "terminal composition delivery") {
		t.Fatalf("terminal transition error = %v", err)
	}

	deadFixture := insertCompositionMonitorRun(t, db, owner, workspace, "dead-letter")
	deadClaim := uuid.New()
	deadClaimedAt := deadFixture.completedAt.Add(time.Second)
	deadLeaseUntil := deadClaimedAt.Add(10 * time.Minute)
	if err := db.Exec(`
		UPDATE public.outcome_monitor_composition_deliveries
		SET lease_id = ?, lease_owner = ?, lease_until = ?, lease_generation = 1,
			revision = 2, updated_at = ? WHERE delivery_id = ?`,
		deadClaim, worker, deadLeaseUntil, deadClaimedAt, deadFixture.runID).Error; err != nil {
		t.Fatalf("claim dead-letter fixture: %v", err)
	}
	deadAttemptID := uuid.New()
	deadFinishedAt := deadClaimedAt.Add(time.Second)
	if err := db.Exec(`
		INSERT INTO public.outcome_monitor_composition_attempts (
			attempt_id, delivery_id, owner_identity, workspace_key, target_id, run_id,
			run_digest, attempt_number, queue_revision, lease_generation, claim_id, worker_id, status,
			failure_code,
			started_at, finished_at, request_digest, record_digest
		) VALUES (?, ?, ?, ?, ?, ?, ?, 1, 2, 1, ?, ?, 'failed', 'invalid_composition',
			?, ?, ?, ?)`,
		deadAttemptID, deadFixture.runID, owner, workspace, deadFixture.targetID,
		deadFixture.runID, deadFixture.runDigest, deadClaim, worker,
		deadClaimedAt, deadFinishedAt, digest("dead-request"), digest("dead-record")).Error; err != nil {
		t.Fatalf("insert dead-letter attempt: %v", err)
	}
	deadSettledAt := deadFinishedAt.Add(time.Second)
	if err := db.Exec(`
		UPDATE public.outcome_monitor_composition_deliveries
		SET status = 'dead_lettered', lease_id = NULL, lease_owner = NULL, lease_until = NULL,
			attempt_count = 1, next_attempt_at = NULL, last_attempt_at = ?,
			last_failure_code = 'invalid_composition', completed_at = ?,
			revision = 3, updated_at = ? WHERE delivery_id = ?`,
		deadFinishedAt, deadFinishedAt, deadSettledAt, deadFixture.runID).Error; err != nil {
		t.Fatalf("dead-letter composition delivery: %v", err)
	}

	for label, statement := range map[string]string{
		"attempt update":    `UPDATE public.outcome_monitor_composition_attempts SET failure_code = 'changed' WHERE attempt_id = '` + failedAttemptID.String() + `'`,
		"attempt delete":    `DELETE FROM public.outcome_monitor_composition_attempts WHERE attempt_id = '` + failedAttemptID.String() + `'`,
		"attempt truncate":  `TRUNCATE public.outcome_monitor_composition_attempts`,
		"delivery delete":   `DELETE FROM public.outcome_monitor_composition_deliveries WHERE delivery_id = '` + delivery.DeliveryID.String() + `'`,
		"delivery truncate": `TRUNCATE public.outcome_monitor_composition_deliveries`,
	} {
		if err := db.Exec(statement).Error; err == nil {
			t.Errorf("%s was accepted", label)
		}
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
}

type compositionRunFixture struct {
	targetID          uuid.UUID
	runID             uuid.UUID
	runDigest         string
	observationID     uuid.UUID
	observationDigest string
	completedAt       time.Time
}

func insertCompositionMonitorRun(
	t *testing.T,
	db *gorm.DB,
	owner string,
	workspace string,
	label string,
) compositionRunFixture {
	t.Helper()
	createdAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	targetID := uuid.New()
	if err := db.Exec(`
		INSERT INTO public.outcome_monitor_targets (
			target_id, owner_identity, workspace_key, outcome_key, indicator_key,
			source_kind, cadence_seconds, next_run_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, 'verified-completions', 'workflow', 3600, ?, ?, ?)`,
		targetID, owner, workspace, "outcome-"+label, createdAt, createdAt, createdAt).Error; err != nil {
		t.Fatalf("insert target %s: %v", label, err)
	}
	claimID := uuid.New()
	claimedAt := createdAt.Add(time.Second)
	leaseUntil := claimedAt.Add(30 * time.Minute)
	if err := db.Exec(`
		UPDATE public.outcome_monitor_targets
		SET lease_id = ?, lease_owner = 'monitor-worker/eu-1', lease_until = ?,
			revision = 2, updated_at = ? WHERE target_id = ?`,
		claimID, leaseUntil, claimedAt, targetID).Error; err != nil {
		t.Fatalf("claim target %s: %v", label, err)
	}
	startedAt := claimedAt.Add(time.Second)
	completedAt := startedAt.Add(2 * time.Second)
	observationID := uuid.New()
	observationDigest := digest("observation-record-" + label)
	if err := db.Exec(`
		INSERT INTO public.outcome_observation_records (
			observation_id, owner_identity, workspace_key, outcome_key, indicator_key,
			source_kind, source_key, source_digest, numeric_value, unit,
			idempotency_key, request_digest, record_digest, observed_at, recorded_at
		) VALUES (?, ?, ?, ?, 'verified-completions', 'workflow', ?, ?, 1, 'count',
			?, ?, ?, ?, ?)`, observationID, owner, workspace, "outcome-"+label,
		"source-"+label, digest("source-"+label), "observation-"+label,
		digest("observation-request-"+label), observationDigest,
		startedAt, startedAt.Add(time.Second)).Error; err != nil {
		t.Fatalf("insert observation %s: %v", label, err)
	}
	runID := uuid.New()
	runDigest := digest("run-record-" + label)
	if err := db.Exec(`
		INSERT INTO public.outcome_monitor_runs (
			run_id, target_id, owner_identity, workspace_key, target_revision,
			claim_id, claimed_at, status, observation_count, signal_count,
			idempotency_key, request_digest, record_digest, started_at, completed_at
		) VALUES (?, ?, ?, ?, 2, ?, ?, 'succeeded', 1, 0, ?, ?, ?, ?, ?)`,
		runID, targetID, owner, workspace, claimID, claimedAt, "run-"+label,
		digest("run-request-"+label), runDigest, startedAt, completedAt).Error; err != nil {
		t.Fatalf("insert monitor run %s: %v", label, err)
	}
	return compositionRunFixture{
		targetID:          targetID,
		runID:             runID,
		runDigest:         runDigest,
		observationID:     observationID,
		observationDigest: observationDigest,
		completedAt:       completedAt,
	}
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
