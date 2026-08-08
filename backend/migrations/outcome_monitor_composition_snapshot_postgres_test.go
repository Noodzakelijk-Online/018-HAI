package migrations_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/proactivity"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const compositionSnapshotMigration = "pre/0051_outcome_monitor_composition_snapshot"

type pinnedCompositionSnapshot struct {
	ContractVersion    int                            `json:"contractVersion"`
	Status             string                         `json:"status"`
	ComposerVersion    string                         `json:"composerVersion"`
	CapturedAt         time.Time                      `json:"capturedAt"`
	OutcomeRevision    int64                          `json:"outcomeRevision,omitempty"`
	OutcomeAuditDigest string                         `json:"outcomeAuditDigest,omitempty"`
	Attention          proactivity.EvaluationSnapshot `json:"attention"`
	SnapshotDigest     string                         `json:"snapshotDigest"`
}

func TestOutcomeMonitorCompositionSnapshotMigrationLifecycle(t *testing.T) {
	db := openIsolatedMigrationDatabase(t)
	files := migrationFilesThrough(t, compositionSnapshotMigration)
	if _, err := infra.ApplyMigrations(db, files, "pre"); err != nil {
		t.Fatalf("apply migrations through 0051: %v", err)
	}
	if err := infra.RollbackMigration(db, files, "pre", compositionSnapshotMigration); err != nil {
		t.Fatalf("roll back empty 0051 migration: %v", err)
	}
	if snapshotColumnExists(t, db, "outcome_monitor_composition_deliveries", "snapshot_status") ||
		snapshotColumnExists(t, db, "outcome_monitor_composition_attempts", "snapshot_digest") {
		t.Fatal("0051 columns remain after empty rollback")
	}

	workspace := "personal-os"
	legacyOwner := "snapshot-legacy-" + uuid.NewString()
	legacyCompletedAt := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Microsecond)
	legacyOutcomeDigest := digest("legacy-outcome-audit")
	insertSnapshotOutcomeRevision(t, db, legacyOwner, workspace, "outcome-legacy", 4, legacyOutcomeDigest, legacyCompletedAt.Add(-time.Minute))
	legacy := insertSnapshotMonitorRun(t, db, legacyOwner, workspace, "legacy", legacyCompletedAt, nil)
	legacyAttemptID := insertPre0051CompositionAttempt(t, db, legacy, legacyOwner, workspace)

	if count, err := infra.ApplyMigrations(db, files, "pre"); err != nil || count != 1 {
		t.Fatalf("apply snapshot migration with legacy rows = (%d, %v), want (1, nil)", count, err)
	}

	legacyProjection := loadSnapshotProjection(t, db, legacy.runID)
	legacyDigest := goCompositionSnapshotDigest(t, pinnedCompositionSnapshot{
		ContractVersion: 1,
		Status:          "legacy_unpinned",
		ComposerVersion: "ambient-monitor-composer/pre-0051-unknown",
		CapturedAt:      legacyCompletedAt,
	})
	legacyBinding := goCompositionBindingDigest(t, legacyOwner, workspace, legacy, legacyDigest)
	if legacyProjection.SnapshotStatus != "legacy_unpinned" ||
		legacyProjection.ComposerVersion != "ambient-monitor-composer/pre-0051-unknown" ||
		!legacyProjection.SnapshotCapturedAt.Equal(legacyCompletedAt) ||
		legacyProjection.Status != "dead_lettered" ||
		!legacyProjection.LastFailureCode.Valid || legacyProjection.LastFailureCode.String != "snapshot_unavailable" ||
		!legacyProjection.CompletedAt.Valid || legacyProjection.NextAttemptAt.Valid ||
		legacyProjection.BindingDigest != legacyBinding ||
		legacyProjection.OutcomeRevision.Valid || legacyProjection.OutcomeAuditDigest.Valid ||
		legacyProjection.PolicyIdempotencyKey.Valid || legacyProjection.AttentionSnapshot.Valid ||
		legacyProjection.AttentionSnapshotDigest.Valid || legacyProjection.SnapshotDigest != legacyDigest {
		t.Fatalf("legacy row fabricated unavailable snapshot pins: %+v", legacyProjection)
	}
	var legacyAttemptDigest string
	if err := db.Raw(`SELECT snapshot_digest FROM public.outcome_monitor_composition_attempts
		WHERE attempt_id = ?`, legacyAttemptID).Scan(&legacyAttemptDigest).Error; err != nil || legacyAttemptDigest != legacyDigest {
		t.Fatalf("legacy attempt digest = (%q, %v), want %q", legacyAttemptDigest, err, legacyDigest)
	}

	triggerOwner := "snapshot-trigger-" + uuid.NewString()
	triggerCompletedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	triggerOnly := insertSnapshotMonitorRun(t, db, triggerOwner, workspace, "trigger-only", triggerCompletedAt, nil)
	triggerProjection := loadSnapshotProjection(t, db, triggerOnly.runID)
	triggerDigest := goCompositionSnapshotDigest(t, pinnedCompositionSnapshot{
		ContractVersion: 1,
		Status:          "legacy_unpinned",
		ComposerVersion: "ambient-monitor-composer/pre-0051-unknown",
		CapturedAt:      triggerCompletedAt,
	})
	triggerBinding := goCompositionBindingDigest(t, triggerOwner, workspace, triggerOnly, triggerDigest)
	if triggerProjection.Status != "dead_lettered" ||
		!triggerProjection.LastFailureCode.Valid || triggerProjection.LastFailureCode.String != "snapshot_unavailable" ||
		triggerProjection.NextAttemptAt.Valid || !triggerProjection.CompletedAt.Valid ||
		triggerProjection.SnapshotStatus != "legacy_unpinned" ||
		triggerProjection.SnapshotDigest != triggerDigest ||
		triggerProjection.BindingDigest != triggerBinding {
		t.Fatalf("trigger-only legacy delivery remains retryable or is not canonically bound: %+v", triggerProjection)
	}
	if err := db.Exec(`UPDATE public.outcome_monitor_composition_deliveries
		SET lease_id = ?, lease_owner = 'forbidden-retry', lease_until = ?,
			lease_generation = lease_generation + 1, revision = revision + 1,
			updated_at = updated_at + interval '1 microsecond'
		WHERE delivery_id = ?`, uuid.New(), triggerCompletedAt.Add(time.Minute), triggerOnly.runID).Error; err == nil {
		t.Fatal("trigger-only legacy delivery accepted a retry claim")
	}

	owner := "snapshot-pinned-" + uuid.NewString()
	service := proactivity.NewService(proactivity.NewPostgresRepository(db))
	policy, _, err := service.RecordPolicy(context.Background(), owner, "policy-exact", proactivity.DefaultPreferences(owner))
	if err != nil {
		t.Fatalf("record exact policy: %v", err)
	}
	signalObservedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	if _, _, err := service.RecordSignals(context.Background(), owner, "signals-exact", []proactivity.OpenLoopSignal{{
		ContractVersion: proactivity.ContractVersion,
		OwnerIdentity:   owner,
		ID:              "snapshot-signal",
		OpenLoopKey:     "snapshot-open-loop",
		Title:           "Review immutable composition snapshot",
		Summary:         "Source-backed signal for exact bounded snapshot coverage.",
		Status:          proactivity.StatusOpen,
		Risk:            proactivity.RiskMedium,
		ObservedAt:      signalObservedAt,
		LastActivityAt:  signalObservedAt,
		StaleAfter:      24 * time.Hour,
		Impact:          0.8,
		Urgency:         0.7,
		Confidence:      0.9,
	}}); err != nil {
		t.Fatalf("record exact signal: %v", err)
	}
	batch, _, err := service.EvaluateStored(context.Background(), owner, proactivity.EvaluateStoredRequest{
		IdempotencyKey: "decisions-exact",
		Now:            time.Now().UTC(),
	})
	if err != nil || len(batch.Result.Decisions) != 1 {
		t.Fatalf("record exact decision: batch=%#v err=%v", batch, err)
	}
	decision := batch.Result.Decisions[0]
	feedback, _, err := service.RecordFeedback(context.Background(), owner, proactivity.FeedbackRequest{
		IdempotencyKey: "feedback-exact",
		SignalID:       decision.SignalID,
		OpenLoopKey:    decision.OpenLoopKey,
		SignalDigest:   decision.SignalDigest,
		Action:         proactivity.FeedbackAccept,
		Reason:         "Exercise the exact immutable feedback cursor.",
	})
	if err != nil {
		t.Fatalf("record exact feedback: %v", err)
	}

	capturedAt := time.Now().UTC().Truncate(time.Microsecond)
	attention, err := service.CaptureEvaluationSnapshot(context.Background(), owner, capturedAt)
	if err != nil {
		t.Fatalf("capture exact EvaluationSnapshot: %v", err)
	}
	if err := proactivity.VerifyEvaluationSnapshot(owner, attention); err != nil {
		t.Fatalf("verify exact EvaluationSnapshot: %v", err)
	}
	if !attention.Policy.RecordedAt.Equal(policy.RecordedAt) {
		t.Fatalf("snapshot policy recordedAt = %s, want %s", attention.Policy.RecordedAt, policy.RecordedAt)
	}
	if attention.Signals.Count != 1 || attention.Signals.Cursor == nil ||
		attention.Decisions.Count != 1 || attention.Decisions.Cursor == nil ||
		attention.Feedback.Count != 1 || attention.Feedback.Cursor == nil ||
		attention.Feedback.Cursor.RecordDigest != feedback.RecordDigest {
		t.Fatalf("unexpected bounded EvaluationSnapshot: %#v", attention)
	}

	runCompletedAt := capturedAt.Add(-5 * time.Second)
	outcomeAuditDigest := digest("pinned-outcome-audit")
	insertSnapshotOutcomeRevision(t, db, owner, workspace, "outcome-pinned", 7, outcomeAuditDigest, runCompletedAt.Add(-time.Minute))
	pinned := pinnedCompositionSnapshot{
		ContractVersion:    1,
		Status:             "pinned",
		ComposerVersion:    "ambient-outcome-attention-v2",
		CapturedAt:         capturedAt,
		OutcomeRevision:    7,
		OutcomeAuditDigest: outcomeAuditDigest,
		Attention:          attention,
	}
	pinned.SnapshotDigest = goCompositionSnapshotDigest(t, pinned)
	pinnedRun := insertSnapshotMonitorRun(t, db, owner, workspace, "pinned", runCompletedAt, &pinned)
	pinnedProjection := loadSnapshotProjection(t, db, pinnedRun.runID)
	pinnedBinding := goCompositionBindingDigest(t, owner, workspace, pinnedRun, pinned.SnapshotDigest)
	if pinnedProjection.SnapshotStatus != "pinned" ||
		pinnedProjection.ComposerVersion != "ambient-outcome-attention-v2" ||
		!pinnedProjection.SnapshotCapturedAt.Equal(capturedAt) ||
		!pinnedProjection.OutcomeRevision.Valid || pinnedProjection.OutcomeRevision.Int64 != 7 ||
		!pinnedProjection.OutcomeAuditDigest.Valid || pinnedProjection.OutcomeAuditDigest.String != outcomeAuditDigest ||
		!pinnedProjection.PolicyIdempotencyKey.Valid || pinnedProjection.PolicyIdempotencyKey.String != "policy-exact" ||
		!pinnedProjection.AttentionSnapshotDigest.Valid || pinnedProjection.AttentionSnapshotDigest.String != attention.InputDigest ||
		pinnedProjection.BindingDigest != pinnedBinding ||
		pinnedProjection.SnapshotDigest != pinned.SnapshotDigest {
		t.Fatalf("unexpected pinned snapshot: %+v", pinnedProjection)
	}
	var sqlDigest string
	if err := db.Raw(`SELECT public.hai_outcome_monitor_composition_snapshot_digest(
		'pinned', composer_version, snapshot_captured_at, outcome_revision,
		outcome_audit_digest, attention_snapshot)
		FROM public.outcome_monitor_composition_deliveries WHERE run_id = ?`, pinnedRun.runID).Scan(&sqlDigest).Error; err != nil || sqlDigest != pinned.SnapshotDigest {
		t.Fatalf("SQL/Go snapshot digest = (%q, %v), want %q", sqlDigest, err, pinned.SnapshotDigest)
	}

	forged := pinned
	forged.OutcomeRevision = 1
	forged.OutcomeAuditDigest = digest("forged-outcome-audit")
	forged.Attention.Signals.Count++
	forged.SnapshotDigest = goCompositionSnapshotDigest(t, forged)
	if err := proactivity.VerifyEvaluationSnapshot(owner, forged.Attention); err == nil {
		t.Fatal("Go verification accepted a forged attention count")
	}
	insertSnapshotOutcomeRevision(t, db, owner, workspace, "outcome-forged", 1, forged.OutcomeAuditDigest, runCompletedAt.Add(-time.Minute))
	if _, err := tryInsertSnapshotMonitorRun(db, owner, workspace, "forged", runCompletedAt, &forged); err == nil ||
		!strings.Contains(err.Error(), "signal count or cursor is not exact") {
		t.Fatalf("forged bounded snapshot error = %v", err)
	}

	claimID, worker := claimSnapshotDelivery(t, db, pinnedRun, owner, workspace, capturedAt.Add(time.Second))
	wrongAttemptID := uuid.New()
	if err := insertSnapshotAttempt(db, wrongAttemptID, pinnedRun, owner, workspace, claimID, worker, digest("wrong-snapshot")); err == nil {
		t.Fatal("attempt with a different replay snapshot was accepted")
	}
	attemptID := uuid.New()
	if err := insertSnapshotAttempt(db, attemptID, pinnedRun, owner, workspace, claimID, worker, pinned.SnapshotDigest); err != nil {
		t.Fatalf("insert exact snapshot attempt: %v", err)
	}
	var attemptDigest string
	if err := db.Raw(`SELECT snapshot_digest FROM public.outcome_monitor_composition_attempts
		WHERE attempt_id = ?`, attemptID).Scan(&attemptDigest).Error; err != nil || attemptDigest != pinned.SnapshotDigest {
		t.Fatalf("attempt snapshot digest = (%q, %v), want %q", attemptDigest, err, pinned.SnapshotDigest)
	}

	for label, statement := range map[string]string{
		"delivery snapshot mutation": `UPDATE public.outcome_monitor_composition_deliveries SET snapshot_digest = '` + digest("changed") + `', revision = revision + 1, updated_at = updated_at + interval '1 microsecond' WHERE run_id = '` + pinnedRun.runID.String() + `'`,
		"attempt snapshot mutation":  `UPDATE public.outcome_monitor_composition_attempts SET snapshot_digest = '` + digest("changed") + `' WHERE attempt_id = '` + attemptID.String() + `'`,
		"attempt delete":             `DELETE FROM public.outcome_monitor_composition_attempts WHERE attempt_id = '` + attemptID.String() + `'`,
		"attempt truncate":           `TRUNCATE public.outcome_monitor_composition_attempts`,
		"delivery delete":            `DELETE FROM public.outcome_monitor_composition_deliveries WHERE run_id = '` + pinnedRun.runID.String() + `'`,
		"delivery truncate":          `TRUNCATE public.outcome_monitor_composition_deliveries`,
	} {
		if err := db.Exec(statement).Error; err == nil {
			t.Errorf("%s was accepted", label)
		}
	}

	rollbackFixture := errors.New("rollback fixture")
	err = db.Transaction(func(tx *gorm.DB) error {
		err := infra.RollbackMigration(tx, files, "pre", compositionSnapshotMigration)
		if err == nil || !strings.Contains(err.Error(), "refusing to roll back non-empty") {
			t.Fatalf("non-empty rollback error = %v", err)
		}
		return rollbackFixture
	})
	if !errors.Is(err, rollbackFixture) {
		t.Fatalf("rollback fixture transaction = %v", err)
	}
}

type snapshotProjection struct {
	Status                  string
	LastFailureCode         sql.NullString
	NextAttemptAt           sql.NullTime
	CompletedAt             sql.NullTime
	BindingDigest           string
	SnapshotStatus          string
	ComposerVersion         string
	SnapshotCapturedAt      time.Time
	OutcomeRevision         sql.NullInt64
	OutcomeAuditDigest      sql.NullString
	PolicyIdempotencyKey    sql.NullString
	AttentionSnapshot       sql.NullString
	AttentionSnapshotDigest sql.NullString
	SnapshotDigest          string
}

func loadSnapshotProjection(t *testing.T, db *gorm.DB, runID uuid.UUID) snapshotProjection {
	t.Helper()
	var row snapshotProjection
	if err := db.Raw(`
		SELECT status, last_failure_code, next_attempt_at, completed_at, binding_digest,
		       snapshot_status, composer_version, snapshot_captured_at,
		       outcome_revision, outcome_audit_digest, policy_idempotency_key,
		       attention_snapshot::text AS attention_snapshot,
		       attention_snapshot_digest, snapshot_digest
		FROM public.outcome_monitor_composition_deliveries WHERE run_id = ?`, runID).Scan(&row).Error; err != nil {
		t.Fatalf("load snapshot %s: %v", runID, err)
	}
	return row
}

func goCompositionSnapshotDigest(t *testing.T, value pinnedCompositionSnapshot) string {
	t.Helper()
	value.SnapshotDigest = ""
	payload := struct {
		Operation string                    `json:"operation"`
		Value     pinnedCompositionSnapshot `json:"value"`
	}{Operation: "composition_snapshot", Value: value}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal Go composition snapshot: %v", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func goCompositionBindingDigest(t *testing.T, owner, workspace string, fixture compositionRunFixture, snapshotDigest string) string {
	t.Helper()
	return goCompositionBindingDigestForFixture(owner, workspace, fixture, snapshotDigest)
}

func goCompositionBindingDigestForFixture(owner, workspace string, fixture compositionRunFixture, snapshotDigest string) string {
	type bindingScope struct {
		OwnerID     string `json:"ownerId"`
		WorkspaceID string `json:"workspaceId"`
	}
	type bindingValue struct {
		Scope             bindingScope
		ID                string
		TargetID          string
		RunID             string
		RunDigest         string
		ObservationID     string
		ObservationDigest string
		SnapshotDigest    string
	}
	payload := struct {
		Operation string       `json:"operation"`
		Value     bindingValue `json:"value"`
	}{
		Operation: "composition_binding",
		Value: bindingValue{
			Scope:             bindingScope{OwnerID: owner, WorkspaceID: workspace},
			ID:                "cmp-" + strings.ReplaceAll(fixture.runID.String(), "-", ""),
			TargetID:          fixture.targetID.String(),
			RunID:             "run-" + strings.ReplaceAll(fixture.runID.String(), "-", ""),
			RunDigest:         fixture.runDigest,
			ObservationID:     "obs-" + strings.ReplaceAll(fixture.observationID.String(), "-", ""),
			ObservationDigest: fixture.observationDigest,
			SnapshotDigest:    snapshotDigest,
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("marshal Go composition binding: %v", err))
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func insertSnapshotOutcomeRevision(t *testing.T, db *gorm.DB, owner, workspace, outcomeID string, revision int64, auditDigest string, recordedAt time.Time) {
	t.Helper()
	if err := db.Exec(`
		INSERT INTO public.outcome_evaluation_outcome_revisions (
			owner_identity, workspace_id, outcome_id, revision, idempotency_key,
			request_digest, audit_digest, recorded_at, payload
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, '{}'::jsonb)`, owner, workspace, outcomeID,
		revision, "outcome-revision-"+outcomeID, digest("outcome-request-"+outcomeID),
		auditDigest, recordedAt).Error; err != nil {
		t.Fatalf("insert outcome revision %s: %v", outcomeID, err)
	}
}

func insertSnapshotMonitorRun(t *testing.T, db *gorm.DB, owner, workspace, label string, completedAt time.Time, snapshot *pinnedCompositionSnapshot) compositionRunFixture {
	t.Helper()
	fixture, err := tryInsertSnapshotMonitorRun(db, owner, workspace, label, completedAt, snapshot)
	if err != nil {
		t.Fatalf("insert snapshot run %s: %v", label, err)
	}
	return fixture
}

func tryInsertSnapshotMonitorRun(db *gorm.DB, owner, workspace, label string, completedAt time.Time, snapshot *pinnedCompositionSnapshot) (compositionRunFixture, error) {
	createdAt := completedAt.Add(-10 * time.Second)
	targetID, claimID := uuid.New(), uuid.New()
	if err := db.Exec(`
		INSERT INTO public.outcome_monitor_targets (
			target_id, owner_identity, workspace_key, outcome_key, indicator_key,
			source_kind, cadence_seconds, next_run_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, 'verified-completions', 'workflow', 3600, ?, ?, ?)`,
		targetID, owner, workspace, "outcome-"+label, createdAt, createdAt, createdAt).Error; err != nil {
		return compositionRunFixture{}, err
	}
	claimedAt := createdAt.Add(time.Second)
	if err := db.Exec(`UPDATE public.outcome_monitor_targets
		SET lease_id = ?, lease_owner = 'snapshot-worker/eu-1', lease_until = ?,
			revision = 2, updated_at = ? WHERE target_id = ?`, claimID,
		completedAt.Add(time.Minute), claimedAt, targetID).Error; err != nil {
		return compositionRunFixture{}, err
	}
	startedAt := claimedAt.Add(time.Second)
	observationID, runID := uuid.New(), uuid.New()
	observationDigest, runDigest := digest("snapshot-observation-"+label), digest("snapshot-run-"+label)
	bindingDigest := digest(strings.Join([]string{
		"composition_binding_v1", owner, workspace, runID.String(), targetID.String(),
		runDigest, observationID.String(), observationDigest,
	}, "|"))
	fixture := compositionRunFixture{targetID: targetID, runID: runID, runDigest: runDigest,
		observationID: observationID, observationDigest: observationDigest, completedAt: completedAt}
	if snapshot != nil {
		bindingDigest = goCompositionBindingDigestForFixture(owner, workspace, fixture, snapshot.SnapshotDigest)
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		if snapshot != nil {
			if err := tx.Exec(`SELECT set_config('hai.outcome_monitor_pinned_enqueue', 'on', true)`).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec(`
			INSERT INTO public.outcome_observation_records (
				observation_id, owner_identity, workspace_key, outcome_key, indicator_key,
				source_kind, source_key, source_digest, numeric_value, unit,
				idempotency_key, request_digest, record_digest, observed_at, recorded_at
			) VALUES (?, ?, ?, ?, 'verified-completions', 'workflow', ?, ?, 1, 'count',
				?, ?, ?, ?, ?)`, observationID, owner, workspace, "outcome-"+label,
			targetID.String(), digest("snapshot-source-"+label), "observation-"+label,
			digest("snapshot-observation-request-"+label), observationDigest,
			startedAt, completedAt).Error; err != nil {
			return err
		}
		if err := tx.Exec(`
			INSERT INTO public.outcome_monitor_runs (
				run_id, target_id, owner_identity, workspace_key, target_revision,
				claim_id, claimed_at, status, observation_count, signal_count,
				idempotency_key, request_digest, record_digest, started_at, completed_at
			) VALUES (?, ?, ?, ?, 2, ?, ?, 'succeeded', 1, 0, ?, ?, ?, ?, ?)`,
			runID, targetID, owner, workspace, claimID, claimedAt, "run-"+label,
			digest("snapshot-run-request-"+label), runDigest, startedAt, completedAt).Error; err != nil {
			return err
		}
		if snapshot == nil {
			return nil
		}
		attentionJSON, err := json.Marshal(snapshot.Attention)
		if err != nil {
			return err
		}
		var signalAt, decisionAt, feedbackAt any
		var signalKey, decisionKey, feedbackID, feedbackDigest any
		if cursor := snapshot.Attention.Signals.Cursor; cursor != nil {
			signalAt, signalKey = cursor.RecordedAt, fmt.Sprintf(`["%s", %d]`, cursor.IdempotencyKey, cursor.Ordinal)
		}
		if cursor := snapshot.Attention.Decisions.Cursor; cursor != nil {
			decisionAt, decisionKey = cursor.RecordedAt, fmt.Sprintf(`["%s", %d]`, cursor.IdempotencyKey, cursor.Ordinal)
		}
		if cursor := snapshot.Attention.Feedback.Cursor; cursor != nil {
			feedbackAt, feedbackID, feedbackDigest = cursor.RecordedAt, cursor.FeedbackID, cursor.RecordDigest
		}
		return tx.Exec(`
			INSERT INTO public.outcome_monitor_composition_deliveries (
				delivery_id, owner_identity, workspace_key, target_id, run_id, run_digest,
				observation_id, observation_digest, status, revision, lease_generation,
				attempt_count, max_attempts, base_backoff_seconds, max_backoff_seconds,
				next_attempt_at, created_at, updated_at, binding_digest,
				snapshot_status, composer_version, snapshot_captured_at,
				outcome_revision, outcome_audit_digest,
				policy_idempotency_key, policy_payload_digest, policy_recorded_at,
				signal_watermark_at, signal_watermark_key,
				decision_watermark_at, decision_watermark_key,
				feedback_watermark_at, feedback_watermark_id, feedback_watermark_digest,
				attention_snapshot, attention_snapshot_digest, snapshot_digest
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending', 1, 0, 0, 5, 30, 3600,
				?, ?, ?, ?,
				?, ?, ?,
				?, ?,
				?, ?, ?,
				?, ?,
				?, ?,
				?, ?, ?,
				CAST(? AS jsonb), ?, ?)`,
			runID, owner, workspace, targetID, runID, runDigest, observationID, observationDigest,
			snapshot.CapturedAt, snapshot.CapturedAt, snapshot.CapturedAt, bindingDigest,
			snapshot.Status, snapshot.ComposerVersion, snapshot.CapturedAt,
			snapshot.OutcomeRevision, snapshot.OutcomeAuditDigest,
			snapshot.Attention.Policy.IdempotencyKey, snapshot.Attention.Policy.PayloadDigest,
			snapshot.Attention.Policy.RecordedAt, signalAt, signalKey, decisionAt, decisionKey,
			feedbackAt, feedbackID, feedbackDigest, string(attentionJSON),
			snapshot.Attention.InputDigest, snapshot.SnapshotDigest).Error
	})
	return fixture, err
}

func insertPre0051CompositionAttempt(t *testing.T, db *gorm.DB, fixture compositionRunFixture, owner, workspace string) uuid.UUID {
	t.Helper()
	claimID, worker := claimSnapshotDelivery(t, db, fixture, owner, workspace, fixture.completedAt.Add(time.Second))
	attemptID := uuid.New()
	if err := db.Exec(`
		INSERT INTO public.outcome_monitor_composition_attempts (
			attempt_id, delivery_id, owner_identity, workspace_key, target_id, run_id,
			run_digest, attempt_number, queue_revision, lease_generation, claim_id,
			worker_id, status, started_at, finished_at, request_digest, record_digest
		) VALUES (?, ?, ?, ?, ?, ?, ?, 1, 2, 1, ?, ?, 'succeeded', ?, ?, ?, ?)`,
		attemptID, fixture.runID, owner, workspace, fixture.targetID, fixture.runID,
		fixture.runDigest, claimID, worker, fixture.completedAt.Add(time.Second),
		fixture.completedAt.Add(2*time.Second), digest("legacy-attempt-request"),
		digest("legacy-attempt-record")).Error; err != nil {
		t.Fatalf("insert pre-0051 composition attempt: %v", err)
	}
	return attemptID
}

func claimSnapshotDelivery(t *testing.T, db *gorm.DB, fixture compositionRunFixture, owner, workspace string, claimedAt time.Time) (uuid.UUID, string) {
	t.Helper()
	claimID, worker := uuid.New(), "snapshot-attempt-worker/eu-1"
	if err := db.Exec(`UPDATE public.outcome_monitor_composition_deliveries
		SET lease_id = ?, lease_owner = ?, lease_until = ?, lease_generation = 1,
			revision = 2, updated_at = ?
		WHERE owner_identity = ? AND workspace_key = ? AND delivery_id = ?`,
		claimID, worker, claimedAt.Add(30*time.Second), claimedAt,
		owner, workspace, fixture.runID).Error; err != nil {
		t.Fatalf("claim snapshot delivery: %v", err)
	}
	return claimID, worker
}

func insertSnapshotAttempt(db *gorm.DB, attemptID uuid.UUID, fixture compositionRunFixture, owner, workspace string, claimID uuid.UUID, worker, snapshotDigest string) error {
	startedAt := fixture.completedAt.Add(6 * time.Second)
	return db.Exec(`
		INSERT INTO public.outcome_monitor_composition_attempts (
			attempt_id, delivery_id, owner_identity, workspace_key, target_id, run_id,
			run_digest, snapshot_digest, attempt_number, queue_revision, lease_generation,
			claim_id, worker_id, status, started_at, finished_at, request_digest, record_digest
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, 2, 1, ?, ?, 'succeeded', ?, ?, ?, ?)`,
		attemptID, fixture.runID, owner, workspace, fixture.targetID, fixture.runID,
		fixture.runDigest, snapshotDigest, claimID, worker, startedAt, startedAt.Add(time.Second),
		digest("attempt-request-"+attemptID.String()), digest("attempt-record-"+attemptID.String())).Error
}

func snapshotColumnExists(t *testing.T, db *gorm.DB, table, column string) bool {
	t.Helper()
	var exists bool
	if err := db.Raw(`SELECT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = ? AND column_name = ?)`, table, column).Scan(&exists).Error; err != nil {
		t.Fatalf("check %s.%s: %v", table, column, err)
	}
	return exists
}
