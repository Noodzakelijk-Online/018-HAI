package resilience

import (
	"strings"
	"testing"
	"time"
)

func testRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxAttempts: 3, BaseDelay: time.Second, Multiplier: 2, MaxDelay: 10 * time.Second}
}

func testFailure(class FailureClass) Failure {
	return Failure{Code: "worker-failure", Class: class, Message: "worker returned a bounded failure"}
}

func TestRetryAndDeadLetterClassification(t *testing.T) {
	tests := []struct {
		name       string
		class      FailureClass
		attempts   uint32
		want       RetryDisposition
		wantDelay  time.Duration
		deadLetter DeadLetterClass
	}{
		{name: "first transient retry", class: FailureTransient, attempts: 1, want: RetrySchedule, wantDelay: time.Second},
		{name: "second rate limit retry", class: FailureRateLimited, attempts: 2, want: RetrySchedule, wantDelay: 2 * time.Second},
		{name: "retry exhausted", class: FailureTransient, attempts: 3, want: RetryDeadLetter, deadLetter: DeadLetterRetryExhausted},
		{name: "permanent", class: FailurePermanent, attempts: 1, want: RetryDeadLetter, deadLetter: DeadLetterPermanent},
		{name: "invalid", class: FailureInvalidWork, attempts: 1, want: RetryDeadLetter, deadLetter: DeadLetterInvalid},
		{name: "unauthorized", class: FailureUnauthorized, attempts: 1, want: RetryDeadLetter, deadLetter: DeadLetterUnauthorized},
		{name: "security", class: FailureSecurity, attempts: 1, want: RetryDeadLetter, deadLetter: DeadLetterSecurity},
		{name: "unknown fails closed", class: FailureUnknown, attempts: 1, want: RetryDeadLetter, deadLetter: DeadLetterUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := DecideRetry(testScope, "work-1", tt.attempts, testFailure(tt.class), testRetryPolicy(), testNow)
			if err != nil {
				t.Fatalf("DecideRetry: %v", err)
			}
			if decision.Disposition != tt.want || decision.DeadLetterClass != tt.deadLetter {
				t.Fatalf("decision=%+v, want disposition=%s deadLetter=%s", decision, tt.want, tt.deadLetter)
			}
			if tt.wantDelay > 0 && (decision.RetryAt == nil || !decision.RetryAt.Equal(testNow.Add(tt.wantDelay))) {
				t.Fatalf("retryAt=%v, want %v", decision.RetryAt, testNow.Add(tt.wantDelay))
			}
			assertAdvisoryOnly(t, decision.Authority)
		})
	}
}

func TestRetryDelayCapsWithoutOverflow(t *testing.T) {
	policy := RetryPolicy{MaxAttempts: 1000, BaseDelay: time.Second, Multiplier: 100, MaxDelay: time.Hour}
	for _, attempts := range []uint32{3, 20, 999} {
		decision, err := DecideRetry(testScope, "work-1", attempts, testFailure(FailureTransient), policy, testNow)
		if err != nil {
			t.Fatalf("attempts %d: %v", attempts, err)
		}
		if decision.RetryAt == nil || !decision.RetryAt.Equal(testNow.Add(time.Hour)) {
			t.Fatalf("attempts %d retryAt=%v, want capped hour", attempts, decision.RetryAt)
		}
	}
}

func TestCircuitTransitions(t *testing.T) {
	policy := CircuitPolicy{FailureThreshold: 2, OpenDuration: time.Minute, MaxHalfOpenProbes: 1}
	closed, err := NewCircuitState(testScope, "provider-a")
	if err != nil {
		t.Fatal(err)
	}
	oneFailure, err := AfterCircuitAttempt(testScope, closed, AttemptFailed, testNow, policy)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := AfterCircuitAttempt(testScope, oneFailure.State, AttemptFailed, testNow.Add(time.Second), policy)
	if err != nil {
		t.Fatal(err)
	}
	halfOpen, err := BeforeCircuitAttempt(testScope, opened.State, *opened.State.RetryAfter, policy)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name          string
		state         CircuitState
		before        bool
		outcome       AttemptOutcome
		now           time.Time
		wantPhase     CircuitPhase
		wantRecommend CircuitRecommendation
	}{
		{name: "closed recommends attempt", state: closed, before: true, now: testNow, wantPhase: CircuitClosed, wantRecommend: CircuitRecommendAttempt},
		{name: "failure below threshold stays closed", state: closed, outcome: AttemptFailed, now: testNow, wantPhase: CircuitClosed, wantRecommend: CircuitRecommendAttempt},
		{name: "threshold failure opens", state: oneFailure.State, outcome: AttemptFailed, now: testNow.Add(time.Second), wantPhase: CircuitOpen, wantRecommend: CircuitRecommendBlock},
		{name: "open before retry blocks", state: opened.State, before: true, now: opened.State.RetryAfter.Add(-time.Nanosecond), wantPhase: CircuitOpen, wantRecommend: CircuitRecommendBlock},
		{name: "open at retry becomes half open", state: opened.State, before: true, now: *opened.State.RetryAfter, wantPhase: CircuitHalfOpen, wantRecommend: CircuitRecommendProbe},
		{name: "half open probe limit blocks", state: halfOpen.State, before: true, now: halfOpen.State.RetryAfter.Add(time.Second), wantPhase: CircuitHalfOpen, wantRecommend: CircuitRecommendBlock},
		{name: "half open success closes", state: halfOpen.State, outcome: AttemptSucceeded, now: halfOpen.State.RetryAfter.Add(time.Second), wantPhase: CircuitClosed, wantRecommend: CircuitRecommendAttempt},
		{name: "half open failure reopens", state: halfOpen.State, outcome: AttemptFailed, now: halfOpen.State.RetryAfter.Add(time.Second), wantPhase: CircuitOpen, wantRecommend: CircuitRecommendBlock},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var decision CircuitDecision
			var decisionErr error
			if tt.before {
				decision, decisionErr = BeforeCircuitAttempt(testScope, tt.state, tt.now, policy)
			} else {
				decision, decisionErr = AfterCircuitAttempt(testScope, tt.state, tt.outcome, tt.now, policy)
			}
			if decisionErr != nil {
				t.Fatalf("circuit decision: %v", decisionErr)
			}
			if decision.State.Phase != tt.wantPhase || decision.Recommendation != tt.wantRecommend {
				t.Fatalf("decision=%+v, want phase=%s recommendation=%s", decision, tt.wantPhase, tt.wantRecommend)
			}
			assertAdvisoryOnly(t, decision.Authority)
		})
	}
}

func TestRecoveryDecisions(t *testing.T) {
	lease := mustLease(t)
	heartbeat := WorkerHeartbeat{ContractVersion: ContractVersion, Scope: testScope, WorkerID: lease.WorkerID, Sequence: 1, ObservedAt: testNow}
	openCircuit, _ := NewCircuitState(testScope, "provider-a")
	openedAt := testNow
	retryAfter := testNow.Add(time.Minute)
	openCircuit.Phase = CircuitOpen
	openCircuit.OpenedAt = &openedAt
	openCircuit.RetryAfter = &retryAfter
	failure := testFailure(FailureTransient)
	permanent := testFailure(FailurePermanent)
	base := RecoveryRequest{
		ContractVersion:   ContractVersion,
		Scope:             testScope,
		WorkID:            lease.WorkID,
		Now:               testNow.Add(10 * time.Second),
		Lease:             &lease,
		Heartbeat:         &heartbeat,
		HeartbeatMaxAge:   time.Minute,
		AttemptsCompleted: 1,
		RetryPolicy:       testRetryPolicy(),
	}
	tests := []struct {
		name   string
		modify func(*RecoveryRequest)
		want   RecoveryAction
	}{
		{name: "healthy worker waits", want: RecoveryWaitWorker},
		{name: "expired lease reclaimed", modify: func(r *RecoveryRequest) { r.Now = lease.ExpiresAt }, want: RecoveryReclaimLease},
		{name: "stale heartbeat reclaimed", modify: func(r *RecoveryRequest) {
			longLease := lease
			longLease.ExpiresAt = testNow.Add(10 * time.Minute)
			r.Lease = &longLease
			r.Now = testNow.Add(2 * time.Minute)
		}, want: RecoveryReclaimLease},
		{name: "open circuit held", modify: func(r *RecoveryRequest) { r.Circuit = &openCircuit }, want: RecoveryHoldCircuitOpen},
		{name: "transient failure retried", modify: func(r *RecoveryRequest) { r.Lease = nil; r.Failure = &failure }, want: RecoveryScheduleRetry},
		{name: "permanent failure dead lettered", modify: func(r *RecoveryRequest) { r.Lease = nil; r.Failure = &permanent }, want: RecoveryDeadLetter},
		{name: "insufficient evidence reviewed", modify: func(r *RecoveryRequest) { r.Lease = nil }, want: RecoveryManualReview},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := base
			if tt.modify != nil {
				tt.modify(&request)
			}
			decision, decisionErr := DecideRecovery(request)
			if decisionErr != nil {
				t.Fatalf("DecideRecovery: %v", decisionErr)
			}
			if decision.Action != tt.want {
				t.Fatalf("action=%s, want %s (%+v)", decision.Action, tt.want, decision)
			}
			assertAdvisoryOnly(t, decision.Authority)
		})
	}
}

func TestRecoveryRejectsHeartbeatFromDifferentWorker(t *testing.T) {
	lease := mustLease(t)
	heartbeat := WorkerHeartbeat{ContractVersion: ContractVersion, Scope: testScope, WorkerID: "worker-2", Sequence: 1, ObservedAt: testNow}
	_, err := DecideRecovery(RecoveryRequest{
		ContractVersion: ContractVersion,
		Scope:           testScope,
		WorkID:          lease.WorkID,
		Now:             testNow.Add(time.Second),
		Lease:           &lease,
		Heartbeat:       &heartbeat,
		HeartbeatMaxAge: time.Minute,
		RetryPolicy:     testRetryPolicy(),
	})
	if err == nil {
		t.Fatal("expected heartbeat from a different worker to fail closed")
	}
}

func TestRetryRejectsUnredactedFailure(t *testing.T) {
	failure := testFailure(FailureTransient)
	failure.Message = "Authorization: Bearer " + strings.Repeat("x", 24)
	if _, err := DecideRetry(testScope, "work-1", 1, failure, testRetryPolicy(), testNow); err == nil {
		t.Fatal("expected unredacted failure to be rejected")
	}
}
