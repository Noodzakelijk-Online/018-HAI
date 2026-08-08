package proactivity

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestServiceRecordsAndReplaysOwnerAdvisoryState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	clock := time.Date(2026, time.August, 1, 13, 0, 0, 0, time.UTC)
	service := newService(NewMemoryRepository(), func() time.Time { return clock })

	policy, created, err := service.RecordPolicy(ctx, "owner-a", "policy-1", DefaultPreferences("owner-a"))
	if err != nil || !created || policy.OwnerIdentity != "owner-a" {
		t.Fatalf("record policy: created=%v policy=%#v err=%v", created, policy, err)
	}
	clock = clock.Add(time.Minute)
	replayedPolicy, created, err := service.RecordPolicy(ctx, "owner-a", "policy-1", DefaultPreferences("owner-a"))
	if err != nil || created || replayedPolicy.RecordedAt != policy.RecordedAt {
		t.Fatalf("replay policy: created=%v policy=%#v err=%v", created, replayedPolicy, err)
	}

	signal := testSignal("owner-a", "signal-a", "loop-a", clock)
	signal.Title = "Review api_key=top-secret-value"
	signal.Summary = "Authorization: Bearer hidden-token-value"
	storedSignals, created, err := service.RecordSignals(ctx, "owner-a", "signals-1", []OpenLoopSignal{signal})
	if err != nil || !created {
		t.Fatalf("record signals: created=%v err=%v", created, err)
	}
	encodedSignals, _ := json.Marshal(storedSignals)
	if strings.Contains(string(encodedSignals), "top-secret-value") || strings.Contains(string(encodedSignals), "hidden-token-value") || !strings.Contains(string(encodedSignals), redactedValue) {
		t.Fatalf("signal redaction failed: %s", encodedSignals)
	}
	clock = clock.Add(time.Minute)
	replayedSignals, created, err := service.RecordSignals(ctx, "owner-a", "signals-1", []OpenLoopSignal{signal})
	if err != nil || created || !reflect.DeepEqual(replayedSignals, storedSignals) {
		t.Fatalf("replay signals: created=%v signals=%#v err=%v", created, replayedSignals, err)
	}

	evaluationTime := clock.Add(time.Minute)
	batch, created, err := service.EvaluateStored(ctx, "owner-a", EvaluateStoredRequest{IdempotencyKey: "decisions-1", Now: evaluationTime})
	if err != nil || !created || len(batch.Result.Decisions) != 1 {
		t.Fatalf("evaluate: created=%v batch=%#v err=%v", created, batch, err)
	}
	assertNoAuthority(t, batch.Result.Decisions[0])
	clock = clock.Add(time.Hour)
	replayedBatch, created, err := service.EvaluateStored(ctx, "owner-a", EvaluateStoredRequest{IdempotencyKey: "decisions-1", Now: evaluationTime})
	if err != nil || created || !reflect.DeepEqual(replayedBatch, batch) {
		t.Fatalf("replay evaluation: created=%v batch=%#v err=%v", created, replayedBatch, err)
	}
	if _, _, err := service.EvaluateStored(ctx, "owner-a", EvaluateStoredRequest{IdempotencyKey: "decisions-1", Now: evaluationTime.Add(time.Second)}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("evaluation idempotency conflict error = %v", err)
	}
}

func TestServiceEnforcesOwnerIsolationAcrossAllRecords(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, time.August, 1, 14, 0, 0, 0, time.UTC)
	service := newService(NewMemoryRepository(), func() time.Time { return now })

	if _, _, err := service.RecordPolicy(ctx, "owner-a", "policy-cross", DefaultPreferences("owner-b")); err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("cross-owner policy error = %v", err)
	}
	if _, _, err := service.RecordSignals(ctx, "owner-a", "signals-cross", []OpenLoopSignal{testSignal("owner-b", "signal-a", "loop-a", now)}); err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("cross-owner signal error = %v", err)
	}
	if _, _, err := service.RecordPolicy(ctx, "owner-a", "shared-key", DefaultPreferences("owner-a")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.RecordPolicy(ctx, "owner-b", "shared-key", DefaultPreferences("owner-b")); err != nil {
		t.Fatalf("idempotency keys should be owner scoped: %v", err)
	}
	ownerBPolicies, err := service.PolicyHistory(ctx, "owner-b", 10)
	if err != nil || len(ownerBPolicies) != 1 || ownerBPolicies[0].OwnerIdentity != "owner-b" {
		t.Fatalf("owner-b policy history = %#v, err=%v", ownerBPolicies, err)
	}
	ownerBSignals, err := service.Signals(ctx, "owner-b", 10)
	if err != nil || len(ownerBSignals) != 0 {
		t.Fatalf("owner-b signals = %#v, err=%v", ownerBSignals, err)
	}
}

func TestServiceRequiresPersistedPolicyBeforeEvaluation(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 1, 15, 0, 0, 0, time.UTC)
	service := newService(NewMemoryRepository(), func() time.Time { return now })
	_, _, err := service.EvaluateStored(context.Background(), "owner-a", EvaluateStoredRequest{IdempotencyKey: "evaluate-empty", Now: now})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("evaluation without policy error = %v", err)
	}
}
