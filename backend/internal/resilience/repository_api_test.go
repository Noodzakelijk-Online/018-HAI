package resilience

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestMemoryRepositoryIsolationDefensiveCopiesAndIdempotency(t *testing.T) {
	ctx := context.Background()
	repository := NewMemoryRepository(4)
	key := strings.Repeat("b", 64)
	record := IdempotencyRecord{
		ContractVersion: ContractVersion, Scope: testScope, WorkID: "work-1",
		IdempotencyKey: key, PayloadHash: testPayload, RecordedAt: testNow,
	}
	stored, created, err := repository.CreateIdempotency(ctx, record)
	if err != nil || !created {
		t.Fatalf("CreateIdempotency: created=%v err=%v", created, err)
	}
	stored.WorkID = "mutated"
	again, created, err := repository.CreateIdempotency(ctx, record)
	if err != nil || created || again.WorkID != record.WorkID {
		t.Fatalf("idempotent create: record=%+v created=%v err=%v", again, created, err)
	}

	conflict := record
	conflict.PayloadHash = strings.Repeat("c", 64)
	if _, _, err := repository.CreateIdempotency(ctx, conflict); !errors.Is(err, ErrStateConflict) {
		t.Fatalf("payload rebinding error=%v, want ErrStateConflict", err)
	}

	otherOwner := Scope{OwnerID: "owner-2", WorkspaceID: testScope.WorkspaceID}
	otherWorkspace := Scope{OwnerID: testScope.OwnerID, WorkspaceID: "workspace-2"}
	for _, scope := range []Scope{otherOwner, otherWorkspace} {
		if _, err := repository.LookupIdempotency(ctx, scope, key); !errors.Is(err, ErrStateNotFound) {
			t.Fatalf("cross-scope lookup %v error=%v", scope, err)
		}
	}

	lease := repositoryTestLease(t, testScope, "worker-1", testNow)
	if err := repository.CompareAndSwapLease(ctx, testScope, lease.WorkID, nil, lease); err != nil {
		t.Fatal(err)
	}
	read, err := repository.GetLease(ctx, testScope, lease.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	read.WorkerID = "mutated"
	readAgain, err := repository.GetLease(ctx, testScope, lease.WorkID)
	if err != nil || readAgain.WorkerID != "worker-1" {
		t.Fatalf("repository returned mutable alias: lease=%+v err=%v", readAgain, err)
	}
	if _, err := repository.GetLease(ctx, otherOwner, lease.WorkID); !errors.Is(err, ErrStateNotFound) {
		t.Fatalf("cross-owner lease error=%v", err)
	}
}

func TestMemoryRepositoryRejectsStaleLeaseHeartbeatAndCircuitFences(t *testing.T) {
	ctx := context.Background()
	repository := NewMemoryRepository()
	lease := repositoryTestLease(t, testScope, "worker-1", testNow)
	if err := repository.CompareAndSwapLease(ctx, testScope, lease.WorkID, nil, lease); err != nil {
		t.Fatal(err)
	}
	staleSnapshot := *cloneLease(&lease)
	renewal, err := DecideLeaseHeartbeat(lease, LeaseHeartbeat{
		ContractVersion: ContractVersion, Scope: testScope, WorkID: lease.WorkID,
		WorkerID: lease.WorkerID, Generation: lease.Generation,
		ObservedAt: testNow.Add(10 * time.Second), TTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CompareAndSwapLease(ctx, testScope, lease.WorkID, &lease, *renewal.Lease); err != nil {
		t.Fatal(err)
	}
	staleRenewal, err := DecideLeaseHeartbeat(staleSnapshot, LeaseHeartbeat{
		ContractVersion: ContractVersion, Scope: testScope, WorkID: lease.WorkID,
		WorkerID: lease.WorkerID, Generation: lease.Generation,
		ObservedAt: testNow.Add(20 * time.Second), TTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CompareAndSwapLease(ctx, testScope, lease.WorkID, &staleSnapshot, *staleRenewal.Lease); !errors.Is(err, ErrStaleFence) {
		t.Fatalf("stale lease CAS error=%v", err)
	}

	premature := repositoryTestLease(t, testScope, "worker-2", testNow.Add(30*time.Second))
	premature.Generation = lease.Generation + 1
	if err := repository.CompareAndSwapLease(ctx, testScope, lease.WorkID, renewal.Lease, premature); !errors.Is(err, ErrStaleFence) {
		t.Fatalf("premature generation advance error=%v", err)
	}

	heartbeat := WorkerHeartbeat{ContractVersion: ContractVersion, Scope: testScope, WorkerID: "worker-1", Sequence: 1, ObservedAt: testNow}
	if err := repository.CompareAndSwapWorkerHeartbeat(ctx, testScope, heartbeat.WorkerID, nil, heartbeat); err != nil {
		t.Fatal(err)
	}
	nextHeartbeat := heartbeat
	nextHeartbeat.Sequence = 2
	nextHeartbeat.ObservedAt = testNow.Add(time.Second)
	if err := repository.CompareAndSwapWorkerHeartbeat(ctx, testScope, heartbeat.WorkerID, &heartbeat, nextHeartbeat); err != nil {
		t.Fatal(err)
	}
	replayed := heartbeat
	replayed.Sequence = 3
	replayed.ObservedAt = testNow.Add(2 * time.Second)
	if err := repository.CompareAndSwapWorkerHeartbeat(ctx, testScope, heartbeat.WorkerID, &heartbeat, replayed); !errors.Is(err, ErrStaleFence) {
		t.Fatalf("stale heartbeat CAS error=%v", err)
	}

	circuit, err := NewCircuitState(testScope, "provider-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CompareAndSwapCircuit(ctx, testScope, circuit.CircuitID, 0, circuit); err != nil {
		t.Fatal(err)
	}
	nextCircuit, err := AfterCircuitAttempt(testScope, circuit, AttemptFailed, testNow, CircuitPolicy{
		FailureThreshold: 2, OpenDuration: time.Minute, MaxHalfOpenProbes: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CompareAndSwapCircuit(ctx, testScope, circuit.CircuitID, circuit.Revision, nextCircuit.State); err != nil {
		t.Fatal(err)
	}
	if err := repository.CompareAndSwapCircuit(ctx, testScope, circuit.CircuitID, circuit.Revision, nextCircuit.State); !errors.Is(err, ErrStaleFence) {
		t.Fatalf("stale circuit revision error=%v", err)
	}
}

func TestMemoryRepositoryBoundsEventAndRecoveryHistories(t *testing.T) {
	ctx := context.Background()
	repository := NewMemoryRepository(2)
	previousHash := ""
	for sequence := uint64(1); sequence <= 5; sequence++ {
		event := ControlEvent{
			ContractVersion: ContractVersion, Scope: testScope, Type: "test.recorded",
			SubjectID: "work-1", OccurredAt: testNow.Add(time.Duration(sequence) * time.Second),
			Sequence: sequence, PreviousHash: previousHash,
		}
		hash, err := EventHash(event)
		if err != nil {
			t.Fatal(err)
		}
		if err := repository.AppendEvent(ctx, EventRecord{Event: event, Hash: hash, Authority: advisoryBoundary()}); err != nil {
			t.Fatal(err)
		}
		previousHash = hash
	}
	events, err := repository.ListEvents(ctx, testScope, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Event.Sequence != 4 || events[1].Event.Sequence != 5 {
		t.Fatalf("bounded events=%+v", events)
	}
	events[0].Event.SubjectID = "mutated"
	eventsAgain, _ := repository.ListEvents(ctx, testScope, 2)
	if eventsAgain[0].Event.SubjectID != "work-1" {
		t.Fatal("event history returned mutable aliases")
	}

	request := RecoveryRequest{
		ContractVersion: ContractVersion, Scope: testScope, WorkID: "work-1", Now: testNow,
		HeartbeatMaxAge: time.Minute, RetryPolicy: testRetryPolicy(),
	}
	decision, err := DecideRecovery(request)
	if err != nil {
		t.Fatal(err)
	}
	for sequence := uint64(1); sequence <= 4; sequence++ {
		request.Now = testNow.Add(time.Duration(sequence) * time.Second)
		decision, err = DecideRecovery(request)
		if err != nil {
			t.Fatal(err)
		}
		record := RecoveryRecord{
			ContractVersion: ContractVersion, Scope: testScope, WorkID: "work-1", Sequence: sequence,
			RequestedAt: request.Now, Request: request, Decision: decision, Authority: advisoryBoundary(),
		}
		if err := repository.AppendRecovery(ctx, sequence-1, record); err != nil {
			t.Fatal(err)
		}
	}
	recoveries, err := repository.ListRecoveries(ctx, testScope, "work-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recoveries) != 2 || recoveries[0].Sequence != 3 || recoveries[1].Sequence != 4 {
		t.Fatalf("bounded recoveries=%+v", recoveries)
	}
}

func TestCurrentInventoryIsNotTruncatedByHistoryRetention(t *testing.T) {
	ctx := context.Background()
	repository := NewMemoryRepository(2)
	for index := 0; index < 101; index++ {
		workID := fmt.Sprintf("work-%03d", index)
		key, err := DeriveIdempotencyKey(testScope, "sync", workID, testPayload)
		if err != nil {
			t.Fatal(err)
		}
		decision, err := DecideLease(LeaseRequest{
			ContractVersion: ContractVersion, Scope: testScope, WorkID: workID,
			IdempotencyKey: key, PayloadHash: testPayload, WorkerID: "worker-1",
			Now: testNow, TTL: time.Minute,
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := repository.CompareAndSwapLease(ctx, testScope, workID, nil, *decision.Lease); err != nil {
			t.Fatal(err)
		}
	}
	leases, err := repository.ListLeases(ctx, testScope, MaxHistoryLimit)
	if err != nil || len(leases) != 101 {
		t.Fatalf("current lease inventory len=%d err=%v", len(leases), err)
	}
}

func TestRepositoryRejectsForgedAdvisoryDecisionsAndCircuitTransitions(t *testing.T) {
	ctx := context.Background()
	repository := NewMemoryRepository()
	decision, err := DecideRetry(testScope, "work-1", 1, testFailure(FailureTransient), testRetryPolicy(), testNow)
	if err != nil {
		t.Fatal(err)
	}
	decision.Reason = "forged recommendation"
	retry := RetryRecord{
		ContractVersion: ContractVersion, Scope: testScope, WorkID: "work-1", Sequence: 1,
		RequestedAt: testNow, Policy: testRetryPolicy(), Decision: decision, Authority: advisoryBoundary(),
	}
	if err := repository.AppendRetry(ctx, 0, retry); !errors.Is(err, ErrStateConflict) {
		t.Fatalf("forged retry decision error=%v", err)
	}

	circuit, err := NewCircuitState(testScope, "provider-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CompareAndSwapCircuit(ctx, testScope, circuit.CircuitID, 0, circuit); err != nil {
		t.Fatal(err)
	}
	openedAt := testNow
	retryAfter := testNow.Add(time.Minute)
	forged := circuit
	forged.Revision++
	forged.Phase = CircuitHalfOpen
	forged.ProbesInFlight = 1
	forged.OpenedAt = &openedAt
	forged.RetryAfter = &retryAfter
	if err := repository.CompareAndSwapCircuit(ctx, testScope, circuit.CircuitID, circuit.Revision, forged); !errors.Is(err, ErrStaleFence) {
		t.Fatalf("forged circuit transition error=%v", err)
	}
}

func repositoryTestLease(t *testing.T, scope Scope, workerID string, now time.Time) WorkLease {
	t.Helper()
	key, err := DeriveIdempotencyKey(scope, "sync", "source-1", testPayload)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := DecideLease(LeaseRequest{
		ContractVersion: ContractVersion, Scope: scope, WorkID: "work-1",
		IdempotencyKey: key, PayloadHash: testPayload, WorkerID: workerID, Now: now, TTL: time.Minute,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return *decision.Lease
}
