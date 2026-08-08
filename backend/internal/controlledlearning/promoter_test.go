package controlledlearning

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

const testEvidenceDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type testPromoter struct {
	mu            sync.Mutex
	applyCalls    int
	handoffCalls  int
	rollbackCalls int
	applyErr      error
	handoffErr    error
	rollbackErr   error
	evidenceAt    time.Time
}

func (promoter *testPromoter) ID() string {
	return "controlled-learning-test-promoter"
}

func (promoter *testPromoter) evidenceTime() time.Time {
	if promoter.evidenceAt.IsZero() {
		return fixedNow
	}
	return promoter.evidenceAt
}

func (promoter *testPromoter) Apply(
	_ context.Context,
	request PromotionRequest,
) (PromotionResult, error) {
	promoter.mu.Lock()
	defer promoter.mu.Unlock()
	promoter.applyCalls++
	if promoter.applyErr != nil {
		err := promoter.applyErr
		promoter.applyErr = nil
		return PromotionResult{}, err
	}
	return PromotionResult{
		AppliedVersion: request.ProposedVersion,
		RollbackToken:  "rollback:" + request.ApplicationID,
		Evidence: []ApplicationEvidence{{
			ID:         "apply-receipt",
			Kind:       "test_receipt",
			URI:        "local://controlled-learning/apply/" + request.ApplicationID,
			Digest:     testEvidenceDigest,
			RecordedAt: promoter.evidenceTime(),
		}},
	}, nil
}

func (promoter *testPromoter) HandoffProtected(
	_ context.Context,
	request ProtectedHandoffRequest,
) (ProtectedHandoffResult, error) {
	promoter.mu.Lock()
	defer promoter.mu.Unlock()
	promoter.handoffCalls++
	if promoter.handoffErr != nil {
		err := promoter.handoffErr
		promoter.handoffErr = nil
		return ProtectedHandoffResult{}, err
	}
	return ProtectedHandoffResult{
		HandoffReference: "governance-handoff:" + request.ApplicationID,
		Evidence: []ApplicationEvidence{{
			ID:         "handoff-receipt",
			Kind:       "governance_receipt",
			URI:        "local://controlled-learning/handoff/" + request.ApplicationID,
			Digest:     testEvidenceDigest,
			RecordedAt: promoter.evidenceTime(),
		}},
	}, nil
}

func (promoter *testPromoter) Rollback(
	_ context.Context,
	request PromotionRollbackRequest,
) (PromotionRollbackResult, error) {
	promoter.mu.Lock()
	defer promoter.mu.Unlock()
	promoter.rollbackCalls++
	if promoter.rollbackErr != nil {
		err := promoter.rollbackErr
		promoter.rollbackErr = nil
		return PromotionRollbackResult{}, err
	}
	return PromotionRollbackResult{
		RestoredVersion: request.RestoreVersion,
		Evidence: []ApplicationEvidence{{
			ID:         "rollback-receipt",
			Kind:       "test_receipt",
			URI:        "local://controlled-learning/rollback/" + request.ApplicationID,
			Digest:     testEvidenceDigest,
			RecordedAt: promoter.evidenceTime(),
		}},
	}, nil
}

func (promoter *testPromoter) calls() (int, int, int) {
	promoter.mu.Lock()
	defer promoter.mu.Unlock()
	return promoter.applyCalls, promoter.handoffCalls, promoter.rollbackCalls
}

func newTestServiceWithPromoter(t *testing.T) (*Service, *testPromoter) {
	t.Helper()
	promoter := &testPromoter{}
	service, err := NewServiceWithPromoter(
		NewMemoryRepository(),
		promoter,
		func() time.Time { return fixedNow },
		sequenceIDs(),
	)
	if err != nil {
		t.Fatalf("NewServiceWithPromoter: %v", err)
	}
	return service, promoter
}

func configureTestPromoter(
	t *testing.T,
	service *Service,
	evidenceAt ...time.Time,
) (*Service, *testPromoter) {
	t.Helper()
	promoter := &testPromoter{}
	if len(evidenceAt) > 0 {
		promoter.evidenceAt = evidenceAt[0]
	}
	configured, err := service.WithPromoter(promoter)
	if err != nil {
		t.Fatalf("WithPromoter: %v", err)
	}
	return configured, promoter
}

var errTestPromotion = errors.New("test promotion failed")
