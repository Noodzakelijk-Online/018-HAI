package automation

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestApprovalProofBindsActionAndConsumesOnce(t *testing.T) {
	now := time.Date(2026, time.July, 30, 10, 0, 0, 0, time.UTC)
	service := newApprovalProofTestService(t, func() time.Time { return now })
	automationID := uuid.New()
	digest := approvalTestDigest("approved action")
	sourceID := "task-review:review-1"

	proof, err := service.Issue(ApprovalProofIssueRequest{
		OwnerIdentity:    "alice",
		AutomationID:     automationID,
		ActionDigest:     digest,
		Scope:            ApprovalScopeScript,
		ApprovalSourceID: sourceID,
	})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	expected := ApprovalProofExpectation{
		OwnerIdentity:    "alice",
		AutomationID:     automationID,
		ActionDigest:     digest,
		Scope:            ApprovalScopeScript,
		ApprovalSourceID: sourceID,
	}
	if err := service.VerifyAndConsume(proof, expected); err != nil {
		t.Fatalf("VerifyAndConsume: %v", err)
	}
	if err := service.VerifyAndConsume(proof, expected); !errors.Is(err, ErrApprovalProofConsumed) {
		t.Fatalf("replay error = %v, want ErrApprovalProofConsumed", err)
	}
}

func TestApprovalProofAllowsOnlyOneConcurrentConsumer(t *testing.T) {
	service := newApprovalProofTestService(t, time.Now)
	request := ApprovalProofIssueRequest{
		OwnerIdentity:    "alice",
		AutomationID:     uuid.New(),
		ActionDigest:     approvalTestDigest("single consumer"),
		Scope:            ApprovalScopeDocker,
		ApprovalSourceID: "task-review:review-1",
	}
	proof, err := service.Issue(request)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	expected := ApprovalProofExpectation{
		OwnerIdentity:    request.OwnerIdentity,
		AutomationID:     request.AutomationID,
		ActionDigest:     request.ActionDigest,
		Scope:            request.Scope,
		ApprovalSourceID: request.ApprovalSourceID,
	}

	var successes atomic.Int32
	var unexpected atomic.Int32
	var wait sync.WaitGroup
	for i := 0; i < 16; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			err := service.VerifyAndConsume(proof, expected)
			if err == nil {
				successes.Add(1)
				return
			}
			if !errors.Is(err, ErrApprovalProofConsumed) {
				unexpected.Add(1)
			}
		}()
	}
	wait.Wait()
	if successes.Load() != 1 || unexpected.Load() != 0 {
		t.Fatalf("concurrent consumption successes=%d unexpectedErrors=%d, want 1/0", successes.Load(), unexpected.Load())
	}
}

func TestApprovalProofFailsClosedOnBindingMismatch(t *testing.T) {
	now := time.Date(2026, time.July, 30, 10, 0, 0, 0, time.UTC)
	automationID := uuid.New()
	digest := approvalTestDigest("approved action")
	base := ApprovalProofExpectation{
		OwnerIdentity:    "alice",
		AutomationID:     automationID,
		ActionDigest:     digest,
		Scope:            ApprovalScopeScript,
		ApprovalSourceID: "task-review:review-1",
	}
	tests := []struct {
		name   string
		mutate func(*ApprovalProofExpectation)
	}{
		{name: "owner", mutate: func(value *ApprovalProofExpectation) { value.OwnerIdentity = "bob" }},
		{name: "automation", mutate: func(value *ApprovalProofExpectation) { value.AutomationID = uuid.New() }},
		{name: "action digest", mutate: func(value *ApprovalProofExpectation) { value.ActionDigest = approvalTestDigest("different action") }},
		{name: "scope", mutate: func(value *ApprovalProofExpectation) { value.Scope = ApprovalScopeDocker }},
		{name: "approval source", mutate: func(value *ApprovalProofExpectation) { value.ApprovalSourceID = "task-review:review-2" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := newApprovalProofTestService(t, func() time.Time { return now })
			proof, err := service.Issue(ApprovalProofIssueRequest{
				OwnerIdentity:    base.OwnerIdentity,
				AutomationID:     base.AutomationID,
				ActionDigest:     base.ActionDigest,
				Scope:            base.Scope,
				ApprovalSourceID: base.ApprovalSourceID,
			})
			if err != nil {
				t.Fatalf("Issue: %v", err)
			}
			expected := base
			test.mutate(&expected)
			if err := service.VerifyAndConsume(proof, expected); !errors.Is(err, ErrApprovalProofInvalid) {
				t.Fatalf("mismatch error = %v, want ErrApprovalProofInvalid", err)
			}
			if err := service.VerifyAndConsume(proof, base); err != nil {
				t.Fatalf("mismatch consumed proof or corrupted valid binding: %v", err)
			}
		})
	}
}

func TestApprovalProofRejectsTamperingAndExpiry(t *testing.T) {
	now := time.Date(2026, time.July, 30, 10, 0, 0, 0, time.UTC)
	service := newApprovalProofTestService(t, func() time.Time { return now })
	request := ApprovalProofIssueRequest{
		OwnerIdentity:    "alice",
		AutomationID:     uuid.New(),
		ActionDigest:     approvalTestDigest("approved action"),
		Scope:            ApprovalScopeAPIMutate,
		ApprovalSourceID: "workflow-review:workflow-1",
		TTL:              time.Minute,
	}
	expected := ApprovalProofExpectation{
		OwnerIdentity:    request.OwnerIdentity,
		AutomationID:     request.AutomationID,
		ActionDigest:     request.ActionDigest,
		Scope:            request.Scope,
		ApprovalSourceID: request.ApprovalSourceID,
	}

	tampered, err := service.Issue(request)
	if err != nil {
		t.Fatalf("Issue tampered proof: %v", err)
	}
	tampered.Signature = "tampered"
	if err := service.VerifyAndConsume(tampered, expected); !errors.Is(err, ErrApprovalProofInvalid) {
		t.Fatalf("tamper error = %v, want ErrApprovalProofInvalid", err)
	}

	expired, err := service.Issue(request)
	if err != nil {
		t.Fatalf("Issue expiring proof: %v", err)
	}
	now = now.Add(time.Minute)
	if err := service.VerifyAndConsume(expired, expected); !errors.Is(err, ErrApprovalProofExpired) {
		t.Fatalf("expiry error = %v, want ErrApprovalProofExpired", err)
	}
}

func TestApprovalProofRequiresCompleteTrustedBinding(t *testing.T) {
	service := newApprovalProofTestService(t, time.Now)
	valid := ApprovalProofIssueRequest{
		OwnerIdentity:    "alice",
		AutomationID:     uuid.New(),
		ActionDigest:     approvalTestDigest("approved action"),
		Scope:            ApprovalScopeDocker,
		ApprovalSourceID: "task-review:review-1",
	}
	tests := []struct {
		name   string
		mutate func(*ApprovalProofIssueRequest)
	}{
		{name: "owner", mutate: func(value *ApprovalProofIssueRequest) { value.OwnerIdentity = "" }},
		{name: "automation", mutate: func(value *ApprovalProofIssueRequest) { value.AutomationID = uuid.Nil }},
		{name: "digest", mutate: func(value *ApprovalProofIssueRequest) { value.ActionDigest = "" }},
		{name: "scope", mutate: func(value *ApprovalProofIssueRequest) { value.Scope = "" }},
		{name: "source", mutate: func(value *ApprovalProofIssueRequest) { value.ApprovalSourceID = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := valid
			test.mutate(&request)
			if _, err := service.Issue(request); err == nil {
				t.Fatalf("expected incomplete %s binding to be rejected", test.name)
			}
		})
	}
}

func newApprovalProofTestService(t *testing.T, now func() time.Time) ApprovalProofService {
	t.Helper()
	secret := make([]byte, 32)
	for i := range secret {
		secret[i] = byte(i + 1)
	}
	service, err := NewInMemoryApprovalProofService(secret, now)
	if err != nil {
		t.Fatalf("NewInMemoryApprovalProofService: %v", err)
	}
	return service
}

func approvalTestDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
