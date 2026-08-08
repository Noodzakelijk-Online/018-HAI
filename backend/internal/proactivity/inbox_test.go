package proactivity

import (
	"context"
	"testing"
	"time"
)

func TestInboxHonorsCurrentOwnerFeedbackWithoutAuthority(t *testing.T) {
	now := time.Date(2026, time.August, 5, 8, 0, 0, 0, time.UTC)
	service := newService(NewMemoryRepository(), func() time.Time { return now })
	owner := "owner-inbox"
	if _, _, err := service.RecordPolicy(context.Background(), owner, "policy", DefaultPreferences(owner)); err != nil {
		t.Fatal(err)
	}
	signals := []OpenLoopSignal{
		testSignal(owner, "ready", "ready-loop", now),
		testSignal(owner, "snoozed", "snoozed-loop", now),
		testSignal(owner, "dismissed", "dismissed-loop", now),
		testSignal(owner, "resumed", "resumed-loop", now),
	}
	if _, _, err := service.RecordSignals(context.Background(), owner, "signals", signals); err != nil {
		t.Fatal(err)
	}
	batch, _, err := service.EvaluateStored(context.Background(), owner, EvaluateStoredRequest{IdempotencyKey: "evaluate", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	byKey := make(map[string]Decision)
	for _, decision := range batch.Result.Decisions {
		byKey[decision.OpenLoopKey] = decision
	}
	record := func(key string, action FeedbackAction, until *time.Time) {
		decision := byKey[key]
		if _, _, recordErr := service.RecordFeedback(context.Background(), owner, FeedbackRequest{
			IdempotencyKey: "feedback-" + key + "-" + string(action), SignalID: decision.SignalID,
			OpenLoopKey: key, SignalDigest: decision.SignalDigest, Action: action,
			Reason: "Owner set the current attention preference.", SnoozedUntil: until,
		}); recordErr != nil {
			t.Fatal(recordErr)
		}
	}
	snoozedUntil := now.Add(24 * time.Hour)
	record("snoozed-loop", FeedbackSnooze, &snoozedUntil)
	record("dismissed-loop", FeedbackDismiss, nil)
	record("resumed-loop", FeedbackSuppress, nil)
	now = now.Add(time.Microsecond)
	record("resumed-loop", FeedbackResume, nil)

	inbox, err := service.Inbox(context.Background(), owner, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox.Items) != 2 || inbox.Snoozed != 1 || inbox.Dismissed != 1 || inbox.Suppressed != 0 {
		t.Fatalf("unexpected inbox: %#v", inbox)
	}
	if inbox.Authority != InboxAuthority || inbox.CanExecute {
		t.Fatalf("inbox gained authority: %#v", inbox)
	}
	for _, item := range inbox.Items {
		if item.CanExecute || item.DeliveryAuthorized || item.ExecutionAuthorized || item.Authority != InboxAuthority {
			t.Fatalf("inbox item gained authority: %#v", item)
		}
	}
}

func TestInboxRejectsInvalidLimits(t *testing.T) {
	service := NewService(NewMemoryRepository())
	for _, limit := range []int{0, maxAdvisoryLimit + 1} {
		if _, err := service.Inbox(context.Background(), "owner-a", limit); err != ErrInvalidLimit {
			t.Fatalf("limit %d error = %v", limit, err)
		}
	}
}
