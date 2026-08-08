package resilience

import (
	"strings"
	"testing"
	"time"
)

var (
	testNow     = time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	testScope   = Scope{OwnerID: "owner-1", WorkspaceID: "workspace-1"}
	testPayload = strings.Repeat("a", 64)
)

func testLeaseRequest(worker string, now time.Time) LeaseRequest {
	key, err := DeriveIdempotencyKey(testScope, "sync", "source-1", testPayload)
	if err != nil {
		panic(err)
	}
	return LeaseRequest{
		ContractVersion: ContractVersion,
		Scope:           testScope,
		WorkID:          "work-1",
		IdempotencyKey:  key,
		PayloadHash:     testPayload,
		WorkerID:        worker,
		Now:             now,
		TTL:             time.Minute,
	}
}

func mustLease(t *testing.T) WorkLease {
	t.Helper()
	decision, err := DecideLease(testLeaseRequest("worker-1", testNow), nil)
	if err != nil {
		t.Fatalf("DecideLease: %v", err)
	}
	return *decision.Lease
}

func TestDecideLeaseExpiryAndFencing(t *testing.T) {
	lease := mustLease(t)
	tests := []struct {
		name       string
		now        time.Time
		worker     string
		want       LeaseDisposition
		generation uint64
	}{
		{name: "active other worker is busy", now: lease.ExpiresAt.Add(-time.Nanosecond), worker: "worker-2", want: LeaseBusy, generation: 1},
		{name: "same worker duplicate does not renew", now: lease.ExpiresAt.Add(-time.Second), worker: "worker-1", want: LeaseDuplicate, generation: 1},
		{name: "exact expiry is reclaimable", now: lease.ExpiresAt, worker: "worker-2", want: LeaseReclaim, generation: 2},
		{name: "past expiry is reclaimable", now: lease.ExpiresAt.Add(time.Second), worker: "worker-2", want: LeaseReclaim, generation: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := testLeaseRequest(tt.worker, tt.now)
			decision, err := DecideLease(request, &lease)
			if err != nil {
				t.Fatalf("DecideLease: %v", err)
			}
			if decision.Disposition != tt.want || decision.Lease.Generation != tt.generation {
				t.Fatalf("decision=%+v, want disposition=%s generation=%d", decision, tt.want, tt.generation)
			}
			assertAdvisoryOnly(t, decision.Authority)
		})
	}
	request := testLeaseRequest("worker-2", lease.AcquiredAt.Add(-time.Nanosecond))
	if _, err := DecideLease(request, &lease); err == nil {
		t.Fatal("expected clock regression to fail closed")
	}
}

func TestDecideLeaseHeartbeatFencesStaleWorkers(t *testing.T) {
	lease := mustLease(t)
	tests := []struct {
		name      string
		modify    func(*LeaseHeartbeat)
		wantError bool
	}{
		{name: "matching heartbeat renews"},
		{name: "stale generation", modify: func(h *LeaseHeartbeat) { h.Generation-- }, wantError: true},
		{name: "wrong worker", modify: func(h *LeaseHeartbeat) { h.WorkerID = "worker-2" }, wantError: true},
		{name: "late heartbeat", modify: func(h *LeaseHeartbeat) { h.ObservedAt = lease.ExpiresAt }, wantError: true},
		{name: "older heartbeat", modify: func(h *LeaseHeartbeat) { h.ObservedAt = lease.LastHeartbeatAt.Add(-time.Second) }, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			heartbeat := LeaseHeartbeat{
				ContractVersion: ContractVersion,
				Scope:           testScope,
				WorkID:          lease.WorkID,
				WorkerID:        lease.WorkerID,
				Generation:      lease.Generation,
				ObservedAt:      lease.LastHeartbeatAt.Add(30 * time.Second),
				TTL:             time.Minute,
			}
			if tt.modify != nil {
				tt.modify(&heartbeat)
			}
			decision, err := DecideLeaseHeartbeat(lease, heartbeat)
			if (err != nil) != tt.wantError {
				t.Fatalf("error=%v, wantError=%v", err, tt.wantError)
			}
			if !tt.wantError {
				if decision.Disposition != LeaseRenew || !decision.Lease.ExpiresAt.Equal(heartbeat.ObservedAt.Add(heartbeat.TTL)) {
					t.Fatalf("unexpected renewal: %+v", decision)
				}
				assertAdvisoryOnly(t, decision.Authority)
			}
		})
	}
}

func TestLeaseReleaseRejectsExpiredWorker(t *testing.T) {
	lease := mustLease(t)
	tests := []struct {
		name      string
		now       time.Time
		wantError bool
	}{
		{name: "inside lease", now: lease.ExpiresAt.Add(-time.Nanosecond)},
		{name: "at expiry", now: lease.ExpiresAt, wantError: true},
		{name: "after expiry", now: lease.ExpiresAt.Add(time.Second), wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := DecideLeaseRelease(lease, testScope, lease.WorkerID, lease.Generation, tt.now)
			if (err != nil) != tt.wantError {
				t.Fatalf("error=%v, wantError=%v", err, tt.wantError)
			}
			if !tt.wantError && decision.Disposition != LeaseRelease {
				t.Fatalf("unexpected release: %+v", decision)
			}
		})
	}
}

func TestIdempotencyDecisions(t *testing.T) {
	request := testLeaseRequest("worker-1", testNow)
	work := WorkDescriptor{
		ContractVersion: ContractVersion,
		Scope:           request.Scope,
		WorkID:          request.WorkID,
		IdempotencyKey:  request.IdempotencyKey,
		PayloadHash:     request.PayloadHash,
		CreatedAt:       request.Now,
	}
	accepted, err := DecideIdempotency(work, nil)
	if err != nil {
		t.Fatalf("accept new work: %v", err)
	}
	if accepted.Disposition != IdempotencyAccept || accepted.Record == nil {
		t.Fatalf("unexpected accept decision: %+v", accepted)
	}

	tests := []struct {
		name      string
		work      WorkDescriptor
		record    IdempotencyRecord
		want      IdempotencyDisposition
		wantError bool
	}{
		{name: "same key and payload is duplicate", work: work, record: *accepted.Record, want: IdempotencyDuplicate},
		{name: "duplicate resolves to original work", work: func() WorkDescriptor { value := work; value.WorkID = "work-replayed"; return value }(), record: *accepted.Record, want: IdempotencyDuplicate},
		{name: "same key different payload fails closed", work: func() WorkDescriptor { value := work; value.PayloadHash = strings.Repeat("b", 64); return value }(), record: *accepted.Record, wantError: true},
		{name: "unrelated key record fails closed", work: func() WorkDescriptor { value := work; value.IdempotencyKey = strings.Repeat("c", 64); return value }(), record: *accepted.Record, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, decisionErr := DecideIdempotency(tt.work, &tt.record)
			if (decisionErr != nil) != tt.wantError {
				t.Fatalf("error=%v, wantError=%v", decisionErr, tt.wantError)
			}
			if !tt.wantError && decision.Disposition != tt.want {
				t.Fatalf("disposition=%s, want %s", decision.Disposition, tt.want)
			}
		})
	}
}

func TestWorkerHeartbeatLivenessAndReplay(t *testing.T) {
	heartbeat := WorkerHeartbeat{
		ContractVersion: ContractVersion,
		Scope:           testScope,
		WorkerID:        "worker-1",
		Sequence:        1,
		ObservedAt:      testNow,
	}
	tests := []struct {
		name      string
		heartbeat *WorkerHeartbeat
		now       time.Time
		want      HeartbeatStatus
		wantError bool
	}{
		{name: "missing", heartbeat: nil, now: testNow, want: HeartbeatMissing},
		{name: "fresh", heartbeat: &heartbeat, now: testNow.Add(10 * time.Second), want: HeartbeatHealthy},
		{name: "boundary is healthy", heartbeat: &heartbeat, now: testNow.Add(time.Minute), want: HeartbeatHealthy},
		{name: "older than boundary is stale", heartbeat: &heartbeat, now: testNow.Add(time.Minute + time.Nanosecond), want: HeartbeatStale},
		{name: "future observation fails closed", heartbeat: &heartbeat, now: testNow.Add(-time.Second), wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := AssessHeartbeat(testScope, tt.heartbeat, tt.now, time.Minute)
			if (err != nil) != tt.wantError {
				t.Fatalf("error=%v, wantError=%v", err, tt.wantError)
			}
			if !tt.wantError && decision.Status != tt.want {
				t.Fatalf("status=%s, want %s", decision.Status, tt.want)
			}
		})
	}

	replays := []struct {
		name      string
		sequence  uint64
		observed  time.Time
		wantError bool
	}{
		{name: "monotonic", sequence: 2, observed: testNow.Add(time.Second)},
		{name: "same sequence", sequence: 1, observed: testNow.Add(time.Second), wantError: true},
		{name: "older timestamp", sequence: 2, observed: testNow.Add(-time.Second), wantError: true},
	}
	for _, tt := range replays {
		t.Run("replay "+tt.name, func(t *testing.T) {
			next := heartbeat
			next.Sequence = tt.sequence
			next.ObservedAt = tt.observed
			_, err := DecideWorkerHeartbeat(next, &heartbeat)
			if (err != nil) != tt.wantError {
				t.Fatalf("error=%v, wantError=%v", err, tt.wantError)
			}
		})
	}
}

func assertAdvisoryOnly(t *testing.T, authority AuthorityBoundary) {
	t.Helper()
	if authority.Mode != AuthorityAdvisoryOnly || authority.CanExecute || authority.GrantsAuthority || authority.ConsumesApproval || authority.DispatchesWork {
		t.Fatalf("decision granted authority: %+v", authority)
	}
}
