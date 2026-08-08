package resilience

import (
	"strings"
	"testing"
	"time"
)

func TestEventHashIsDeterministic(t *testing.T) {
	event := ControlEvent{
		ContractVersion: ContractVersion,
		Scope:           testScope,
		Type:            "lease.reclaimed",
		SubjectID:       "work-1",
		OccurredAt:      time.Date(2026, 7, 31, 14, 0, 0, 123, time.FixedZone("CEST", 2*60*60)),
		Sequence:        7,
		PreviousHash:    strings.Repeat("d", 64),
		Attributes:      map[string]string{"worker": "worker-2", "generation": "2"},
	}
	reordered := event
	reordered.OccurredAt = event.OccurredAt.UTC()
	reordered.Attributes = map[string]string{"generation": "2", "worker": "worker-2"}
	changed := reordered
	changed.Attributes = map[string]string{"generation": "3", "worker": "worker-2"}

	tests := []struct {
		name      string
		candidate ControlEvent
		equal     bool
	}{
		{name: "same event", candidate: event, equal: true},
		{name: "map order and timezone do not matter", candidate: reordered, equal: true},
		{name: "attribute change matters", candidate: changed, equal: false},
	}
	want, err := EventHash(event)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, hashErr := EventHash(tt.candidate)
			if hashErr != nil {
				t.Fatalf("EventHash: %v", hashErr)
			}
			if (got == want) != tt.equal {
				t.Fatalf("hash equality=%v, want %v\ngot  %s\nwant %s", got == want, tt.equal, got, want)
			}
			if len(got) != 64 {
				t.Fatalf("hash length=%d, want 64", len(got))
			}
		})
	}
}

func TestOwnerAndWorkspaceIsolationFailsClosed(t *testing.T) {
	otherOwner := Scope{OwnerID: "owner-2", WorkspaceID: testScope.WorkspaceID}
	otherWorkspace := Scope{OwnerID: testScope.OwnerID, WorkspaceID: "workspace-2"}
	lease := mustLease(t)
	heartbeat := WorkerHeartbeat{ContractVersion: ContractVersion, Scope: testScope, WorkerID: "worker-1", Sequence: 1, ObservedAt: testNow}
	work := WorkDescriptor{ContractVersion: ContractVersion, Scope: testScope, WorkID: "work-1", IdempotencyKey: lease.IdempotencyKey, PayloadHash: testPayload, CreatedAt: testNow}
	idempotency, _ := DecideIdempotency(work, nil)
	circuit, _ := NewCircuitState(testScope, "provider-a")

	for _, scope := range []Scope{otherOwner, otherWorkspace} {
		name := scope.OwnerID + "/" + scope.WorkspaceID
		t.Run(name, func(t *testing.T) {
			tests := []struct {
				name string
				call func() error
			}{
				{name: "lease", call: func() error {
					request := testLeaseRequest("worker-2", lease.ExpiresAt)
					request.Scope = scope
					_, err := DecideLease(request, &lease)
					return err
				}},
				{name: "lease heartbeat", call: func() error {
					_, err := DecideLeaseHeartbeat(lease, LeaseHeartbeat{ContractVersion: ContractVersion, Scope: scope, WorkID: lease.WorkID, WorkerID: lease.WorkerID, Generation: lease.Generation, ObservedAt: testNow.Add(time.Second), TTL: time.Minute})
					return err
				}},
				{name: "worker heartbeat", call: func() error {
					next := heartbeat
					next.Scope = scope
					next.Sequence++
					next.ObservedAt = next.ObservedAt.Add(time.Second)
					_, err := DecideWorkerHeartbeat(next, &heartbeat)
					return err
				}},
				{name: "idempotency", call: func() error {
					candidate := work
					candidate.Scope = scope
					_, err := DecideIdempotency(candidate, idempotency.Record)
					return err
				}},
				{name: "circuit", call: func() error {
					_, err := BeforeCircuitAttempt(scope, circuit, testNow, CircuitPolicy{FailureThreshold: 2, OpenDuration: time.Minute, MaxHalfOpenProbes: 1})
					return err
				}},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					if err := tt.call(); err == nil {
						t.Fatal("expected owner/workspace mismatch to fail closed")
					}
				})
			}
		})
	}
}

func TestSecretRedactionAndStructuredRejection(t *testing.T) {
	privateKey := "-----BEGIN PRIVATE KEY-----\nabc123\n-----END PRIVATE KEY-----"
	tests := []struct {
		name  string
		input string
	}{
		{name: "key value", input: "request failed password=hunter2"},
		{name: "bearer", input: "Authorization: Bearer abcdefghijklmnop"},
		{name: "private key", input: privateKey},
		{name: "credential prefix", input: "upstream returned sk-proj-abcdefghijklmnop"},
		{name: "url user info", input: "https://user:hunter2@example.test/path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failure, err := NewFailure("upstream-error", FailureTransient, tt.input)
			if err != nil {
				t.Fatalf("NewFailure: %v", err)
			}
			if failure.Message == tt.input || strings.Contains(failure.Message, "hunter2") || strings.Contains(failure.Message, "abcdefghijklmnop") || strings.Contains(failure.Message, "abc123") {
				t.Fatalf("secret was not redacted: %q", failure.Message)
			}
		})
	}

	baseEvent := ControlEvent{
		ContractVersion: ContractVersion,
		Scope:           testScope,
		Type:            "retry.scheduled",
		SubjectID:       "work-1",
		OccurredAt:      testNow,
		Sequence:        1,
		Attributes:      map[string]string{"reason": "temporary outage"},
	}
	rejections := []struct {
		name   string
		modify func(*ControlEvent)
	}{
		{name: "sensitive key", modify: func(event *ControlEvent) { event.Attributes = map[string]string{"api_token": "redacted-or-not"} }},
		{name: "secret value", modify: func(event *ControlEvent) { event.Attributes = map[string]string{"reason": "token=super-secret"} }},
		{name: "secret owner", modify: func(event *ControlEvent) { event.Scope.OwnerID = "token=super-secret" }},
		{name: "malformed previous hash", modify: func(event *ControlEvent) { event.PreviousHash = "not-a-hash" }},
		{name: "noncanonical attribute key", modify: func(event *ControlEvent) { event.Attributes = map[string]string{" reason ": "outage"} }},
	}
	for _, tt := range rejections {
		t.Run("reject "+tt.name, func(t *testing.T) {
			event := baseEvent
			tt.modify(&event)
			if _, err := EventHash(event); err == nil {
				t.Fatal("expected secret-bearing or invalid event to fail closed")
			}
		})
	}
}

func TestFailClosedValidation(t *testing.T) {
	request := testLeaseRequest("worker-1", testNow)
	tests := []struct {
		name   string
		modify func(*LeaseRequest)
	}{
		{name: "missing owner", modify: func(value *LeaseRequest) { value.Scope.OwnerID = "" }},
		{name: "unsupported contract", modify: func(value *LeaseRequest) { value.ContractVersion++ }},
		{name: "invalid payload hash", modify: func(value *LeaseRequest) { value.PayloadHash = "abc" }},
		{name: "zero time", modify: func(value *LeaseRequest) { value.Now = time.Time{} }},
		{name: "zero ttl", modify: func(value *LeaseRequest) { value.TTL = 0 }},
		{name: "secret worker id", modify: func(value *LeaseRequest) { value.WorkerID = "token=worker-secret" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := request
			tt.modify(&candidate)
			decision, err := DecideLease(candidate, nil)
			if err == nil {
				t.Fatalf("expected validation error, got %+v", decision)
			}
			if decision.Lease != nil || decision.Authority.CanExecute || decision.Authority.GrantsAuthority {
				t.Fatalf("invalid input returned actionable decision: %+v", decision)
			}
		})
	}
}
