package ambientmonitor

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCompositionQueueClaimsOnceAndFencesStaleWorker(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 5, 12, 0, 0, 0, time.UTC)
	current := now
	repository := NewMemoryRepository()
	service := newService(repository, &deterministicCollector{value: CollectedObservation{Value: 3, ObservedAt: now, SourceDigest: strings.Repeat("a", 64)}}, &recordingSink{}, func() time.Time { return current })
	scope := Scope{OwnerID: "owner-compose", WorkspaceID: "workspace-compose"}
	target, _, err := service.RegisterTarget(t.Context(), testRegisterRequest(scope, now))
	if err != nil {
		t.Fatal(err)
	}
	claims, err := service.ClaimDue(t.Context(), ClaimDueRequest{Scope: scope, WorkerID: "collector-worker", Now: now, LeaseDuration: time.Minute, Limit: 1})
	if err != nil || len(claims) != 1 {
		t.Fatalf("claim = (%+v,%v)", claims, err)
	}
	completion, err := service.ProcessClaim(t.Context(), ProcessClaimRequest{IdempotencyKey: "durable-composition", Scope: scope, TargetID: target.ID, WorkerID: "collector-worker", LeaseGeneration: claims[0].Lease.Generation, CompletedAt: now})
	if err != nil || completion.Composition.Status != CompositionPending {
		t.Fatalf("completion = (%+v,%v)", completion, err)
	}

	type result struct {
		items []CompositionDelivery
		err   error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, worker := range []string{"composition-a", "composition-b"} {
		worker := worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			items, err := repository.ClaimDueCompositions(t.Context(), scope.OwnerID, scope.WorkspaceID, worker, now, time.Minute, 1)
			results <- result{items, err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	var claimed CompositionDelivery
	count := 0
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if len(result.items) == 1 {
			claimed = result.items[0]
			count++
		}
	}
	if count != 1 {
		t.Fatalf("composition claims=%d, want one", count)
	}
	staleAttempt, err := newCompositionAttempt(claimed, claimed.Lease.WorkerID, claimed.Lease.ClaimedAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	staleAttempt.Status = CompositionAttemptSucceeded
	staleAttempt.RecordDigest, err = compositionAttemptDigest(staleAttempt)
	if err != nil {
		t.Fatal(err)
	}
	current = claimed.Lease.ExpiresAt.Add(time.Second)
	if recovered, err := service.RecoverExpiredCompositionLeases(t.Context(), scope, current); err != nil || recovered != 1 {
		t.Fatalf("recover=(%d,%v)", recovered, err)
	}
	if _, _, err := repository.CompleteComposition(t.Context(), scope.OwnerID, scope.WorkspaceID, claimed.ID, claimed.Lease.WorkerID, claimed.Lease.Generation, staleAttempt, staleAttempt.FinishedAt); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale completion error=%v", err)
	}
}

func TestCompositionQueueDeadLettersAfterBoundedFailures(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 5, 13, 0, 0, 0, time.UTC)
	current := now
	sink := &recordingSink{err: errors.New("unavailable")}
	service := newService(NewMemoryRepository(), &deterministicCollector{value: CollectedObservation{Value: 1, ObservedAt: now, SourceDigest: strings.Repeat("b", 64)}}, sink, func() time.Time { return current })
	scope := Scope{OwnerID: "owner-dead-letter", WorkspaceID: "workspace-dead-letter"}
	if _, _, err := service.RegisterTarget(t.Context(), testRegisterRequest(scope, now)); err != nil {
		t.Fatal(err)
	}
	result, err := service.ProcessDue(t.Context(), ProcessDueRequest{Scope: scope, WorkerID: "monitor-worker", Now: now, LeaseDuration: time.Minute, Limit: 1})
	if err != nil || len(result.Completions) != 1 {
		t.Fatalf("initial pass=(%+v,%v)", result, err)
	}
	var latest CompositionDelivery
	for attempt := 1; attempt <= defaultCompositionMaxAttempts; attempt++ {
		items, err := service.Compositions(t.Context(), scope, result.Completions[0].Run.TargetID, 10)
		if err != nil || len(items) != 1 {
			t.Fatalf("list=(%+v,%v)", items, err)
		}
		latest = items[0]
		if latest.Status == CompositionDeadLettered {
			break
		}
		current = latest.NextAttemptAt.Add(time.Second)
		processed, err := service.ProcessCompositions(t.Context(), ProcessCompositionsRequest{Scope: scope, WorkerID: "retry-worker", Now: current, LeaseDuration: time.Minute, Limit: 1})
		if err != nil || len(processed.Failures) != 1 {
			t.Fatalf("retry %d=(%+v,%v)", attempt, processed, err)
		}
	}
	items, err := service.Compositions(t.Context(), scope, result.Completions[0].Run.TargetID, 10)
	if err != nil {
		t.Fatal(err)
	}
	latest = items[0]
	if latest.Status != CompositionDeadLettered || latest.AttemptCount != defaultCompositionMaxAttempts || latest.Lease.Active() {
		t.Fatalf("dead letter=%+v", latest)
	}
	attempts, err := service.CompositionAttempts(t.Context(), scope, latest.ID, 10)
	if err != nil || len(attempts) != defaultCompositionMaxAttempts {
		t.Fatalf("attempts=(%+v,%v)", attempts, err)
	}
	for _, attempt := range attempts {
		assertAdvisoryControl(t, attempt.Authority)
	}
}

func TestCompositionQueueIsOwnerAndWorkspaceScoped(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 5, 14, 0, 0, 0, time.UTC)
	service := newService(NewMemoryRepository(), &deterministicCollector{value: CollectedObservation{Value: 1, ObservedAt: now, SourceDigest: strings.Repeat("c", 64)}}, &recordingSink{}, func() time.Time { return now })
	scope := Scope{OwnerID: "owner-a", WorkspaceID: "workspace-a"}
	target, _, err := service.RegisterTarget(t.Context(), testRegisterRequest(scope, now))
	if err != nil {
		t.Fatal(err)
	}
	claims, err := service.ClaimDue(t.Context(), ClaimDueRequest{Scope: scope, WorkerID: "worker-a", Now: now, LeaseDuration: time.Minute, Limit: 1})
	if err != nil || len(claims) != 1 {
		t.Fatal(err)
	}
	completion, err := service.ProcessClaim(t.Context(), ProcessClaimRequest{IdempotencyKey: "scope-composition", Scope: scope, TargetID: target.ID, WorkerID: "worker-a", LeaseGeneration: claims[0].Lease.Generation, CompletedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	foreign := Scope{OwnerID: "owner-b", WorkspaceID: "workspace-a"}
	if _, err := service.Composition(t.Context(), foreign, completion.Composition.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign composition error=%v", err)
	}
	if items, err := service.Compositions(t.Context(), foreign, target.ID, 10); err != nil || len(items) != 0 {
		t.Fatalf("foreign list=(%+v,%v)", items, err)
	}
}
