package resilience

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestServiceConcurrentIdempotencyAndLeaseCAS(t *testing.T) {
	repository := NewMemoryRepository(100)
	fixed := time.Date(2026, 8, 1, 11, 0, 0, 0, time.UTC)
	service := newServiceWithClock(repository, func() time.Time { return fixed })
	const callers = 16
	type registrationResult struct {
		decision IdempotencyDecision
		err      error
	}
	registrations := make(chan registrationResult, callers)
	var group sync.WaitGroup
	for index := 0; index < callers; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			decision, err := service.RegisterWork(context.Background(), "owner-a", "workspace-a", WorkRegistrationInput{
				WorkID: "work-" + strconv.Itoa(index), Operation: "sync", SourceRef: "source-a", PayloadHash: strings.Repeat("a", 64),
			})
			registrations <- registrationResult{decision: decision, err: err}
		}(index)
	}
	group.Wait()
	close(registrations)
	accepted := 0
	canonical := ""
	for result := range registrations {
		if result.err != nil {
			t.Fatalf("concurrent registration: %v", result.err)
		}
		if result.decision.Disposition == IdempotencyAccept {
			accepted++
			canonical = result.decision.CanonicalWorkID
		}
	}
	if accepted != 1 || canonical == "" {
		t.Fatalf("accepted=%d canonical=%q, want one canonical registration", accepted, canonical)
	}

	key, err := DeriveIdempotencyKey(Scope{OwnerID: "owner-a", WorkspaceID: "workspace-a"}, "sync", "source-a", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	type leaseResult struct {
		advisory LeaseAdvisory
		err      error
	}
	leases := make(chan leaseResult, callers)
	for index := 0; index < callers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			advisory, err := service.AcquireLease(context.Background(), "owner-a", "workspace-a", LeaseAcquireInput{
				WorkID: canonical, WorkerID: "worker-a", IdempotencyKey: key, PayloadHash: strings.Repeat("a", 64), TTL: time.Minute,
			})
			leases <- leaseResult{advisory: advisory, err: err}
		}()
	}
	group.Wait()
	close(leases)
	grants := 0
	duplicates := 0
	for result := range leases {
		if result.err != nil {
			t.Fatalf("concurrent acquire: %v", result.err)
		}
		if result.advisory.Lease == nil {
			t.Fatal("canonical concurrent acquire returned no lease decision")
		}
		switch result.advisory.Lease.Disposition {
		case LeaseGrant:
			grants++
		case LeaseDuplicate:
			duplicates++
		default:
			t.Fatalf("unexpected lease disposition %q", result.advisory.Lease.Disposition)
		}
	}
	if grants != 1 || duplicates != callers-1 {
		t.Fatalf("grants=%d duplicates=%d", grants, duplicates)
	}
}

func steppingClock(start time.Time) func() time.Time {
	var mu sync.Mutex
	next := start
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		value := next
		next = next.Add(time.Millisecond)
		return value
	}
}

func TestServiceAdvisoryLifecyclePreservesFencingAndAuditChain(t *testing.T) {
	now := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	clockNow := now
	repository := NewMemoryRepository(100)
	service := newServiceWithClock(repository, func() time.Time { return clockNow })
	ctx := context.Background()
	owner, workspace := "owner-a", "workspace-a"
	hash := strings.Repeat("a", 64)
	key := strings.Repeat("b", 64)

	acquired, err := service.AcquireLease(ctx, owner, workspace, LeaseAcquireInput{
		WorkID: "work-1", WorkerID: "worker-1", IdempotencyKey: key,
		PayloadHash: hash, TTL: 10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}
	if acquired.Lease == nil || acquired.Lease.Disposition != LeaseGrant || acquired.Lease.Lease.Generation != 1 {
		t.Fatalf("unexpected acquire result: %#v", acquired)
	}
	assertAdvisoryOnly(t, acquired.Authority)
	assertAdvisoryOnly(t, acquired.Lease.Authority)

	duplicate, err := service.AcquireLease(ctx, owner, workspace, LeaseAcquireInput{
		WorkID: "work-1", WorkerID: "worker-1", IdempotencyKey: key,
		PayloadHash: hash, TTL: 10 * time.Minute,
	})
	if err != nil || duplicate.Idempotency.Disposition != IdempotencyDuplicate || duplicate.Lease == nil || duplicate.Lease.Disposition != LeaseDuplicate {
		t.Fatalf("duplicate acquire = %#v, %v", duplicate, err)
	}

	clockNow = now.Add(time.Minute)
	renewed, err := service.HeartbeatLease(ctx, owner, workspace, LeaseHeartbeatInput{WorkID: "work-1", WorkerID: "worker-1", Generation: 1, TTL: 10 * time.Minute})
	if err != nil || renewed.Disposition != LeaseRenew || renewed.Lease.Generation != 1 {
		t.Fatalf("renew lease = %#v, %v", renewed, err)
	}
	if _, err := service.HeartbeatLease(ctx, owner, workspace, LeaseHeartbeatInput{WorkID: "work-1", WorkerID: "worker-1", Generation: 0, TTL: 10 * time.Minute}); err == nil {
		t.Fatal("stale fencing token was accepted")
	}

	heartbeat, err := service.RecordWorkerHeartbeat(ctx, owner, workspace, WorkerHeartbeatInput{WorkerID: "worker-1", Sequence: 1})
	if err != nil || heartbeat.Sequence != 1 {
		t.Fatalf("worker heartbeat = %#v, %v", heartbeat, err)
	}
	if _, err := service.RecordWorkerHeartbeat(ctx, owner, workspace, WorkerHeartbeatInput{WorkerID: "worker-1", Sequence: 1}); err == nil {
		t.Fatal("replayed worker heartbeat was accepted")
	}

	retry, err := service.AdviseRetry(ctx, owner, workspace, RetryAdvisoryInput{
		WorkID: "work-1", AttemptsCompleted: 1, FailureCode: "network_timeout",
		FailureClass: FailureTransient, FailureMessage: "request failed token=super-secret-value",
		Policy: RetryPolicy{MaxAttempts: 3, BaseDelay: time.Second, Multiplier: 2, MaxDelay: time.Minute},
	})
	if err != nil {
		t.Fatalf("advise retry: %v", err)
	}
	if strings.Contains(retry.Decision.Failure.Message, "super-secret-value") || !strings.Contains(retry.Decision.Failure.Message, "[REDACTED]") {
		t.Fatalf("failure was not redacted: %q", retry.Decision.Failure.Message)
	}
	assertAdvisoryOnly(t, retry.Authority)

	policy := CircuitPolicy{FailureThreshold: 2, OpenDuration: 5 * time.Minute, MaxHalfOpenProbes: 1}
	first, err := service.ObserveCircuit(ctx, owner, workspace, CircuitObservationInput{CircuitID: "provider-a", Outcome: AttemptFailed, Policy: policy})
	if err != nil || first.State.Phase != CircuitClosed {
		t.Fatalf("first circuit failure = %#v, %v", first, err)
	}
	second, err := service.ObserveCircuit(ctx, owner, workspace, CircuitObservationInput{CircuitID: "provider-a", Outcome: AttemptFailed, Policy: policy})
	if err != nil || second.State.Phase != CircuitOpen || second.Recommendation != CircuitRecommendBlock {
		t.Fatalf("second circuit failure = %#v, %v", second, err)
	}
	assertAdvisoryOnly(t, second.Authority)

	clockNow = now.Add(2 * time.Minute)
	recovery, err := service.AdviseRecovery(ctx, owner, workspace, RecoveryAdvisoryInput{
		WorkID: "work-1", WorkerID: "worker-1", CircuitID: "provider-a",
		HeartbeatMaxAge: 5 * time.Minute, AttemptsCompleted: 1,
		RetryPolicy: RetryPolicy{MaxAttempts: 3, BaseDelay: time.Second, Multiplier: 2, MaxDelay: time.Minute},
	})
	if err != nil || recovery.Decision.Action != RecoveryHoldCircuitOpen {
		t.Fatalf("recovery = %#v, %v", recovery, err)
	}
	assertAdvisoryOnly(t, recovery.Authority)
	assertAdvisoryOnly(t, recovery.Decision.Authority)

	status, err := service.Status(ctx, owner, workspace)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.LeaseCount != 1 || status.WorkerCount != 1 || status.RetryCount != 1 || status.CircuitCount != 1 || status.RecoveryCount != 1 {
		t.Fatalf("unexpected status: %#v", status)
	}
	assertAdvisoryOnly(t, status.Authority)

	events, err := service.ListEvents(ctx, owner, workspace, 100)
	if err != nil || len(events) < 7 {
		t.Fatalf("events = %d, %v", len(events), err)
	}
	for index, record := range events {
		hash, hashErr := EventHash(record.Event)
		if hashErr != nil || hash != record.Hash {
			t.Fatalf("event %d hash invalid: %v", index, hashErr)
		}
		if index == 0 {
			if record.Event.Sequence != 1 || record.Event.PreviousHash != "" {
				t.Fatalf("invalid first event: %#v", record)
			}
		} else if record.Event.Sequence != events[index-1].Event.Sequence+1 || record.Event.PreviousHash != events[index-1].Hash {
			t.Fatalf("event chain broken at %d", index)
		}
		assertAdvisoryOnly(t, record.Authority)
	}
}

func TestServiceOwnerAndWorkspaceIsolation(t *testing.T) {
	repository := NewMemoryRepository()
	service := newServiceWithClock(repository, func() time.Time { return time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC) })
	_, err := service.AcquireLease(context.Background(), "owner-a", "workspace-a", LeaseAcquireInput{
		WorkID: "work-1", WorkerID: "worker-1", IdempotencyKey: strings.Repeat("a", 64), PayloadHash: strings.Repeat("b", 64), TTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("seed lease: %v", err)
	}
	for _, scope := range [][2]string{{"owner-b", "workspace-a"}, {"owner-a", "workspace-b"}} {
		if _, err := service.GetLease(context.Background(), scope[0], scope[1], "work-1"); !errorsIs(err, ErrStateNotFound) {
			t.Fatalf("cross-scope get returned %v", err)
		}
	}
}

func errorsIs(err, target error) bool {
	return err != nil && (err == target || strings.Contains(err.Error(), target.Error()))
}

func TestAdvisoryRecordsNeverSerializeExecutionAuthority(t *testing.T) {
	value := Status{ContractVersion: ContractVersion, Scope: Scope{OwnerID: "owner", WorkspaceID: "workspace"}, GeneratedAt: time.Now(), Authority: advisoryBoundary()}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"canExecute":true`, `"grantsAuthority":true`, `"consumesApproval":true`, `"dispatchesWork":true`} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("authority leaked in %s", encoded)
		}
	}
}
