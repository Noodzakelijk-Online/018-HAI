package proactivity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestMemoryRepositoryIsOwnerScopedIdempotentAndDefensive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, time.August, 1, 10, 0, 0, 0, time.UTC)
	repository := NewMemoryRepository()
	record := PolicyRecord{
		ContractVersion: ContractVersion,
		OwnerIdentity:   "owner-a",
		Policy:          DefaultPreferences("owner-a"),
		RecordedAt:      now,
	}
	digest := strings.Repeat("a", 64)
	stored, created, err := repository.RecordPolicy(ctx, "owner-a", "policy-1", digest, record)
	if err != nil || !created {
		t.Fatalf("record policy: created=%v err=%v", created, err)
	}
	stored.Policy.Channels[0].Enabled = false
	current, err := repository.CurrentPolicy(ctx, "owner-a")
	if err != nil {
		t.Fatal(err)
	}
	if !current.Policy.Channels[0].Enabled {
		t.Fatal("repository exposed mutable policy storage")
	}

	replayed, created, err := repository.RecordPolicy(ctx, "owner-a", "policy-1", digest, record)
	if err != nil || created || replayed.RecordedAt != now {
		t.Fatalf("policy replay: created=%v record=%#v err=%v", created, replayed, err)
	}
	if _, _, err := repository.RecordPolicy(ctx, "owner-a", "policy-1", strings.Repeat("b", 64), record); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("idempotency conflict error = %v", err)
	}

	crossOwner := record
	crossOwner.OwnerIdentity = "owner-b"
	crossOwner.Policy.OwnerIdentity = "owner-b"
	if _, _, err := repository.RecordPolicy(ctx, "owner-a", "policy-cross", strings.Repeat("c", 64), crossOwner); !errors.Is(err, ErrOwnerScopeViolation) {
		t.Fatalf("cross-owner policy error = %v", err)
	}
	if _, err := repository.CurrentPolicy(ctx, "owner-b"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other owner current policy error = %v", err)
	}
}

func TestMemoryRepositoryBoundsAllHistories(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, time.August, 1, 11, 0, 0, 0, time.UTC)
	repository := NewMemoryRepository()

	for index := 0; index < MaxPolicyHistory+5; index++ {
		policy := DefaultPreferences("owner-a")
		policy.Cooldown = time.Duration(index) * time.Minute
		record := PolicyRecord{ContractVersion: ContractVersion, OwnerIdentity: "owner-a", Policy: policy, RecordedAt: now.Add(time.Duration(index) * time.Second)}
		if _, _, err := repository.RecordPolicy(ctx, "owner-a", fmt.Sprintf("policy-%d", index), testDigest(index+1), record); err != nil {
			t.Fatal(err)
		}
	}
	policies, err := repository.ListPolicies(ctx, "owner-a", MaxPolicyHistory+100)
	if err != nil || len(policies) != MaxPolicyHistory {
		t.Fatalf("policy history length = %d, err=%v", len(policies), err)
	}

	for index := 0; index < MaxSignalHistory+5; index++ {
		signal := testSignal("owner-a", fmt.Sprintf("signal-%d", index), fmt.Sprintf("loop-%d", index), now)
		record := SignalRecord{ContractVersion: ContractVersion, OwnerIdentity: "owner-a", Signal: signal, RecordedAt: now}
		if _, _, err := repository.RecordSignals(ctx, "owner-a", fmt.Sprintf("signals-%d", index), testDigest(1000+index), []SignalRecord{record}); err != nil {
			t.Fatal(err)
		}
	}
	signals, err := repository.ListSignals(ctx, "owner-a", MaxSignalHistory+100)
	if err != nil || len(signals) != MaxSignalHistory {
		t.Fatalf("signal history length = %d, err=%v", len(signals), err)
	}

	remaining := MaxDecisionHistory + 5
	for batchIndex := 0; remaining > 0; batchIndex++ {
		count := min(MaxSignals, remaining)
		decisions := make([]Decision, count)
		for index := range decisions {
			id := batchIndex*MaxSignals + index
			decisions[index] = Decision{
				ContractVersion: ContractVersion,
				OwnerIdentity:   "owner-a",
				SignalID:        fmt.Sprintf("decision-signal-%d", id),
				OpenLoopKey:     fmt.Sprintf("decision-loop-%d", id),
				SignalDigest:    testDigest(5000 + id),
				Outcome:         OutcomeSuppress,
				DecidedAt:       now,
			}
		}
		batch := DecisionBatch{
			ContractVersion: ContractVersion,
			OwnerIdentity:   "owner-a",
			Result: EvaluationResult{
				ContractVersion: ContractVersion,
				OwnerIdentity:   "owner-a",
				DecidedAt:       now,
				TimeZone:        "UTC",
				Decisions:       decisions,
			},
			RecordedAt: now,
		}
		if _, _, err := repository.RecordDecisionBatch(ctx, "owner-a", fmt.Sprintf("decisions-%d", batchIndex), testDigest(9000+batchIndex), batch); err != nil {
			t.Fatal(err)
		}
		remaining -= count
	}
	decisions, err := repository.ListDecisions(ctx, "owner-a", MaxDecisionHistory+100)
	if err != nil || len(decisions) != MaxDecisionHistory {
		t.Fatalf("decision history length = %d, err=%v", len(decisions), err)
	}
}

func TestMemoryRepositoryRejectsAuthorityBearingDecisions(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 1, 12, 0, 0, 0, time.UTC)
	decision := Decision{
		ContractVersion:     ContractVersion,
		OwnerIdentity:       "owner-a",
		SignalID:            "signal-a",
		OpenLoopKey:         "loop-a",
		SignalDigest:        strings.Repeat("a", 64),
		Outcome:             OutcomeNotify,
		ExecutionAuthorized: true,
		DecidedAt:           now,
	}
	batch := DecisionBatch{
		ContractVersion: ContractVersion,
		OwnerIdentity:   "owner-a",
		Result: EvaluationResult{
			ContractVersion: ContractVersion,
			OwnerIdentity:   "owner-a",
			DecidedAt:       now,
			TimeZone:        "UTC",
			Decisions:       []Decision{decision},
		},
		RecordedAt: now,
	}
	_, _, err := NewMemoryRepository().RecordDecisionBatch(context.Background(), "owner-a", "unsafe", strings.Repeat("d", 64), batch)
	if !errors.Is(err, ErrOwnerScopeViolation) {
		t.Fatalf("authority-bearing decision error = %v", err)
	}
}

func testDigest(value int) string {
	return fmt.Sprintf("%064x", value)
}
