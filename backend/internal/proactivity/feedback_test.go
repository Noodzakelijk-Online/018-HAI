package proactivity

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestFeedbackIsOwnerScopedReplayableChainedAndNonExecuting(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	clock := time.Date(2026, time.August, 5, 9, 0, 0, 0, time.UTC)
	service := newService(NewMemoryRepository(), func() time.Time { return clock })
	if _, _, err := service.RecordPolicy(ctx, "owner-a", "feedback-policy", DefaultPreferences("owner-a")); err != nil {
		t.Fatal(err)
	}
	signal := testSignal("owner-a", "feedback-signal", "feedback-loop", clock)
	if _, _, err := service.RecordSignals(ctx, "owner-a", "feedback-signals", []OpenLoopSignal{signal}); err != nil {
		t.Fatal(err)
	}
	batch, _, err := service.EvaluateStored(ctx, "owner-a", EvaluateStoredRequest{
		IdempotencyKey: "feedback-evaluate", Now: clock,
	})
	if err != nil || len(batch.Result.Decisions) != 1 {
		t.Fatalf("evaluate: batch=%#v err=%v", batch, err)
	}
	decision := batch.Result.Decisions[0]
	request := FeedbackRequest{
		IdempotencyKey: "feedback-dismiss", SignalID: decision.SignalID,
		OpenLoopKey: decision.OpenLoopKey, SignalDigest: decision.SignalDigest,
		Action: FeedbackDismiss, Reason: "This exact signal is not useful now.",
	}
	record, created, err := service.RecordFeedback(ctx, "owner-a", request)
	if err != nil || !created {
		t.Fatalf("record feedback: created=%v record=%#v err=%v", created, record, err)
	}
	if record.Authority != FeedbackAuthority || record.CanExecute || record.DeliveryAuthorized || record.ExecutionAuthorized {
		t.Fatalf("feedback granted authority: %#v", record)
	}
	clock = clock.Add(time.Minute)
	replay, created, err := service.RecordFeedback(ctx, "owner-a", request)
	if err != nil || created || replay.RecordDigest != record.RecordDigest || replay.ID != record.ID {
		t.Fatalf("feedback replay: created=%v replay=%#v err=%v", created, replay, err)
	}
	changed := request
	changed.Action = FeedbackSuppress
	if _, _, err := service.RecordFeedback(ctx, "owner-a", changed); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("changed replay error = %v", err)
	}

	resume := request
	resume.IdempotencyKey = "feedback-resume"
	resume.Action = FeedbackResume
	resume.Reason = "Resume proactive attention for this open loop."
	resumed, created, err := service.RecordFeedback(ctx, "owner-a", resume)
	if err != nil || !created || resumed.PreviousRecordDigest != record.RecordDigest {
		t.Fatalf("resume feedback: created=%v record=%#v err=%v", created, resumed, err)
	}
	history, err := service.Feedback(ctx, "owner-a", 10)
	if err != nil || len(history) != 2 || history[0].Action != FeedbackResume || history[1].Action != FeedbackDismiss {
		t.Fatalf("feedback history = %#v, err=%v", history, err)
	}
	other, err := service.Feedback(ctx, "owner-b", 10)
	if err != nil || len(other) != 0 {
		t.Fatalf("cross-owner feedback = %#v, err=%v", other, err)
	}
}

func TestAttentionControlsSuppressOnlyTheirIntendedScope(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 5, 10, 0, 0, 0, time.UTC)
	signal := testSignal("owner-a", "signal-a", "loop-a", now)
	initial, err := Evaluate(testRequest("owner-a", now, []OpenLoopSignal{signal}))
	if err != nil {
		t.Fatal(err)
	}
	digest := initial.Decisions[0].SignalDigest

	tests := []struct {
		name       string
		control    AttentionControl
		mutate     func(*OpenLoopSignal)
		want       Outcome
		wantReason string
		wantNext   bool
	}{
		{
			name: "indefinite suppression", control: AttentionControl{OpenLoopKey: "loop-a", SignalDigest: digest, Action: FeedbackSuppress, RecordedAt: now.Add(-time.Minute)},
			want: OutcomeSuppress, wantReason: "owner suppressed",
		},
		{
			name: "active snooze", control: AttentionControl{OpenLoopKey: "loop-a", SignalDigest: digest, Action: FeedbackSnooze, SnoozedUntil: timePointer(now.Add(time.Hour)), RecordedAt: now.Add(-time.Minute)},
			want: OutcomeSuppress, wantReason: "owner snoozed", wantNext: true,
		},
		{
			name: "expired snooze", control: AttentionControl{OpenLoopKey: "loop-a", SignalDigest: digest, Action: FeedbackSnooze, SnoozedUntil: timePointer(now.Add(-time.Second)), RecordedAt: now.Add(-time.Hour)},
			want: initial.Decisions[0].Outcome,
		},
		{
			name: "exact revision dismissal", control: AttentionControl{OpenLoopKey: "loop-a", SignalDigest: digest, Action: FeedbackDismiss, RecordedAt: now.Add(-time.Minute)},
			want: OutcomeSuppress, wantReason: "exact signal revision",
		},
		{
			name: "changed revision survives dismissal", control: AttentionControl{OpenLoopKey: "loop-a", SignalDigest: digest, Action: FeedbackDismiss, RecordedAt: now.Add(-time.Minute)},
			mutate: func(value *OpenLoopSignal) { value.Summary = "new source-backed information" },
			want:   initial.Decisions[0].Outcome,
		},
		{
			name: "resume restores policy", control: AttentionControl{OpenLoopKey: "loop-a", SignalDigest: digest, Action: FeedbackResume, RecordedAt: now.Add(-time.Minute)},
			want: initial.Decisions[0].Outcome,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := signal
			if test.mutate != nil {
				test.mutate(&candidate)
			}
			request := testRequest("owner-a", now, []OpenLoopSignal{candidate})
			request.Controls = []AttentionControl{test.control}
			result, evaluateErr := Evaluate(request)
			if evaluateErr != nil {
				t.Fatal(evaluateErr)
			}
			decision := result.Decisions[0]
			if decision.Outcome != test.want || (test.wantReason != "" && !containsReason(decision.Reasons, test.wantReason)) || (decision.NextEligibleAt != nil) != test.wantNext {
				t.Fatalf("decision = %#v", decision)
			}
			assertNoAuthority(t, decision)
		})
	}
}

func TestFeedbackValidationRejectsSecretAndInvalidSnooze(t *testing.T) {
	now := time.Date(2026, time.August, 5, 11, 0, 0, 0, time.UTC)
	request := FeedbackRequest{
		IdempotencyKey: "feedback-validation", SignalID: "signal-a", OpenLoopKey: "loop-a",
		SignalDigest: strings.Repeat("a", 64), Action: FeedbackSnooze,
		Reason: "Authorization: Bearer forbidden-secret", SnoozedUntil: timePointer(now.Add(time.Hour)),
	}
	if _, err := normalizeFeedbackRequest(request, now); err == nil || !strings.Contains(err.Error(), "secret") {
		t.Fatalf("secret feedback error = %v", err)
	}
	request.Reason = "Snooze this signal."
	request.SnoozedUntil = timePointer(now.Add(time.Minute))
	if _, err := normalizeFeedbackRequest(request, now); err == nil || !strings.Contains(err.Error(), "5 minutes") {
		t.Fatalf("short snooze error = %v", err)
	}
}

func timePointer(value time.Time) *time.Time { return &value }
