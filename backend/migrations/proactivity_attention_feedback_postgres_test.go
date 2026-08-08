package migrations_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/proactivity"
	"github.com/google/uuid"
)

func TestProactivityAttentionFeedbackMigrationLifecycle(t *testing.T) {
	db := openProactivityMigrationDatabase(t)
	files := migrationFilesThrough(t, "pre/0048_proactivity_attention_feedback")
	if _, err := infra.ApplyMigrations(db, files, "pre"); err != nil {
		t.Fatalf("apply pre migrations: %v", err)
	}
	var existing int64
	if err := db.Raw(`SELECT count(*) FROM public.proactivity_feedback_records`).Scan(&existing).Error; err != nil {
		t.Fatalf("count feedback records: %v", err)
	}
	if existing != 0 {
		t.Skipf("dedicated destructive test database required; feedback ledger contains %d records", existing)
	}

	ctx := context.Background()
	owner := "migration-feedback-" + uuid.NewString()
	service := proactivity.NewService(proactivity.NewPostgresRepository(db))
	if _, _, err := service.RecordPolicy(ctx, owner, "policy-1", proactivity.DefaultPreferences(owner)); err != nil {
		t.Fatalf("record policy: %v", err)
	}
	now := time.Now().UTC().Add(-time.Minute)
	if _, _, err := service.RecordSignals(ctx, owner, "signals-1", []proactivity.OpenLoopSignal{{
		ContractVersion: proactivity.ContractVersion,
		OwnerIdentity:   owner,
		ID:              "feedback-signal",
		OpenLoopKey:     "feedback-loop",
		Title:           "Review source-backed work",
		Summary:         "An open loop requires owner attention.",
		Status:          proactivity.StatusOpen,
		Risk:            proactivity.RiskMedium,
		ObservedAt:      now,
		LastActivityAt:  now,
		StaleAfter:      24 * time.Hour,
		Impact:          0.8,
		Urgency:         0.7,
		Confidence:      0.9,
	}}); err != nil {
		t.Fatalf("record signal: %v", err)
	}
	batch, _, err := service.EvaluateStored(ctx, owner, proactivity.EvaluateStoredRequest{
		IdempotencyKey: "evaluate-1",
		Now:            time.Now().UTC(),
	})
	if err != nil || len(batch.Result.Decisions) != 1 {
		t.Fatalf("evaluate signal: batch=%#v err=%v", batch, err)
	}
	decision := batch.Result.Decisions[0]
	firstRequest := proactivity.FeedbackRequest{
		IdempotencyKey: "dismiss-1",
		SignalID:       decision.SignalID,
		OpenLoopKey:    decision.OpenLoopKey,
		SignalDigest:   decision.SignalDigest,
		Action:         proactivity.FeedbackDismiss,
		Reason:         "Dismiss this exact revision.",
	}
	first, created, err := service.RecordFeedback(ctx, owner, firstRequest)
	if err != nil || !created {
		t.Fatalf("record first feedback: created=%v record=%#v err=%v", created, first, err)
	}
	if first.CanExecute || first.DeliveryAuthorized || first.ExecutionAuthorized || first.Authority != proactivity.FeedbackAuthority {
		t.Fatalf("feedback gained authority: %#v", first)
	}

	resumeRequest := firstRequest
	resumeRequest.IdempotencyKey = "resume-1"
	resumeRequest.Action = proactivity.FeedbackResume
	resumeRequest.Reason = "Resume owner attention."
	second, created, err := service.RecordFeedback(ctx, owner, resumeRequest)
	if err != nil || !created || second.PreviousRecordDigest != first.RecordDigest {
		t.Fatalf("record successor: created=%v record=%#v err=%v", created, second, err)
	}

	// Replaying the older command after a successor exists must return the exact
	// durable record without re-entering the append-chain trigger.
	replay, created, err := service.RecordFeedback(ctx, owner, firstRequest)
	if err != nil || created || replay.ID != first.ID || replay.RecordDigest != first.RecordDigest {
		t.Fatalf("replay first feedback after successor: created=%v record=%#v err=%v", created, replay, err)
	}
	history, err := service.Feedback(ctx, owner, 10)
	if err != nil || len(history) != 2 || history[0].ID != second.ID || history[1].ID != first.ID {
		t.Fatalf("feedback history = %#v, err=%v", history, err)
	}
	other, err := service.Feedback(ctx, "other-owner", 10)
	if err != nil || len(other) != 0 {
		t.Fatalf("cross-owner feedback = %#v, err=%v", other, err)
	}

	if err := db.Exec(`UPDATE public.proactivity_feedback_records SET recorded_at = recorded_at WHERE owner_identity = ?`, owner).Error; err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("immutable update error = %v", err)
	}
	if err := db.Exec(`DELETE FROM public.proactivity_feedback_records WHERE owner_identity = ?`, owner).Error; err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("immutable delete error = %v", err)
	}
	if err := db.Exec(`TRUNCATE public.proactivity_feedback_records`).Error; err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("truncate protection error = %v", err)
	}
}
