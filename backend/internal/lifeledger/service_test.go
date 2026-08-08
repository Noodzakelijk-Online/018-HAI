package lifeledger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"

	"automation-hub-backend/internal/lifeontology"
)

var ledgerTestNow = time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)

func TestCommitmentLifecycleIsRevisionedIdempotentAndOwnerScoped(t *testing.T) {
	repository := NewMemoryRepository()
	service, err := NewService(repository, func() time.Time { return ledgerTestNow })
	if err != nil {
		t.Fatal(err)
	}
	request := commitmentRequest("owner-a", "legal/case/answer", 0, CommitmentProposed, "commitment-create")
	created, err := service.RecordCommitment(context.Background(), request)
	if err != nil || !created.Created || created.Record.Revision != 1 || !created.Record.LocalOnly {
		t.Fatalf("create commitment = %#v err=%v", created, err)
	}
	replayed, err := service.RecordCommitment(context.Background(), request)
	if err != nil || replayed.Created || replayed.Record.ID != created.Record.ID {
		t.Fatalf("idempotent replay = %#v err=%v", replayed, err)
	}
	conflict := request
	conflict.Title = "Different request under the same key"
	if _, err := service.RecordCommitment(context.Background(), conflict); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("idempotency conflict = %v", err)
	}

	active := commitmentRequest("owner-a", request.CommitmentKey, 1, CommitmentActive, "commitment-active")
	activeResult, err := service.RecordCommitment(context.Background(), active)
	if err != nil || activeResult.Record.Revision != 2 {
		t.Fatalf("activate commitment = %#v err=%v", activeResult, err)
	}
	fulfilled := commitmentRequest("owner-a", request.CommitmentKey, 2, CommitmentFulfilled, "commitment-fulfilled")
	fulfilledResult, err := service.RecordCommitment(context.Background(), fulfilled)
	if err != nil || fulfilledResult.Record.Revision != 3 {
		t.Fatalf("fulfill commitment = %#v err=%v", fulfilledResult, err)
	}
	reopen := commitmentRequest("owner-a", request.CommitmentKey, 3, CommitmentActive, "commitment-reopen")
	if _, err := service.RecordCommitment(context.Background(), reopen); err == nil {
		t.Fatal("terminal fulfilled commitment was reopened")
	}
	if _, err := service.GetCommitment(context.Background(), "owner-b", request.CommitmentKey); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner lookup = %v", err)
	}
	history, err := service.CommitmentHistory(context.Background(), "owner-a", request.CommitmentKey, 10)
	if err != nil || len(history) != 3 || history[0].Revision != 1 || history[2].Revision != 3 {
		t.Fatalf("history = %#v err=%v", history, err)
	}
}

func TestCostsKeepEstimateSeparateFromIncurredAndRequireExistingCommitment(t *testing.T) {
	repository := NewMemoryRepository()
	service, _ := NewService(repository, func() time.Time { return ledgerTestNow })
	commitment := commitmentRequest("owner-a", "project/vendor-contract", 0, CommitmentActive, "vendor-contract")
	if _, err := service.RecordCommitment(context.Background(), commitment); err != nil {
		t.Fatal(err)
	}
	estimate := costRequest("owner-a", CostEstimate, "estimate-1")
	estimate.CommitmentKey = commitment.CommitmentKey
	estimateResult, err := service.RecordCost(context.Background(), estimate)
	if err != nil || !estimateResult.Created || estimateResult.Record.Kind != CostEstimate {
		t.Fatalf("estimate = %#v err=%v", estimateResult, err)
	}
	incurred := costRequest("owner-a", CostIncurred, "incurred-1")
	incurred.CommitmentKey = commitment.CommitmentKey
	incurredResult, err := service.RecordCost(context.Background(), incurred)
	if err != nil || incurredResult.Record.Kind != CostIncurred || incurredResult.Record.ID == estimateResult.Record.ID {
		t.Fatalf("incurred = %#v err=%v", incurredResult, err)
	}
	missing := costRequest("owner-a", CostPaid, "missing-commitment")
	missing.Verification = VerificationHumanConfirmed
	missing.CommitmentKey = "does-not-exist"
	if _, err := service.RecordCost(context.Background(), missing); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing commitment error = %v", err)
	}
	invalid := costRequest("owner-a", CostPaid, "zero")
	invalid.Verification = VerificationHumanConfirmed
	invalid.AmountMinor = 0
	if _, err := service.RecordCost(context.Background(), invalid); err == nil {
		t.Fatal("zero cost was accepted")
	}
	records, err := service.ListCosts(context.Background(), "owner-a", 10)
	kinds := map[CostKind]int{}
	for _, record := range records {
		kinds[record.Kind]++
	}
	if err != nil || len(records) != 2 || kinds[CostEstimate] != 1 || kinds[CostIncurred] != 1 {
		t.Fatalf("cost records = %#v err=%v", records, err)
	}
	other, err := service.ListCosts(context.Background(), "owner-b", 10)
	if err != nil || len(other) != 0 {
		t.Fatalf("cross-owner costs = %#v err=%v", other, err)
	}
}

func TestCostVerificationStrengthMatchesFinancialEventKind(t *testing.T) {
	tests := []struct {
		kind         CostKind
		verification VerificationStatus
		allowed      bool
	}{
		{CostEstimate, VerificationNeedsReview, true},
		{CostEstimate, VerificationSourceSupported, true},
		{CostEstimate, VerificationHumanConfirmed, true},
		{CostEstimate, VerificationVerified, true},
		{CostEstimate, VerificationDisputed, false},
		{CostIncurred, VerificationNeedsReview, false},
		{CostIncurred, VerificationSourceSupported, true},
		{CostIncurred, VerificationHumanConfirmed, true},
		{CostIncurred, VerificationVerified, true},
		{CostIncurred, VerificationDisputed, false},
		{CostPaid, VerificationNeedsReview, false},
		{CostPaid, VerificationSourceSupported, false},
		{CostPaid, VerificationHumanConfirmed, true},
		{CostPaid, VerificationVerified, true},
		{CostPaid, VerificationDisputed, false},
		{CostRefund, VerificationNeedsReview, false},
		{CostRefund, VerificationSourceSupported, false},
		{CostRefund, VerificationHumanConfirmed, true},
		{CostRefund, VerificationVerified, true},
		{CostRefund, VerificationDisputed, false},
	}

	for index, test := range tests {
		t.Run(string(test.kind)+"/"+string(test.verification), func(t *testing.T) {
			repository := NewMemoryRepository()
			service, err := NewService(repository, func() time.Time { return ledgerTestNow })
			if err != nil {
				t.Fatal(err)
			}
			request := costRequest("owner-a", test.kind, fmt.Sprintf("verification-%d", index))
			request.Verification = test.verification

			result, err := service.RecordCost(t.Context(), request)
			if test.allowed {
				if err != nil || !result.Created || result.Record.Verification != test.verification {
					t.Fatalf("allowed cost = %#v err=%v", result, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("weak verification %q was accepted for %q", test.verification, test.kind)
			}
			records, listErr := service.ListCosts(t.Context(), "owner-a", 10)
			if listErr != nil || len(records) != 0 {
				t.Fatalf("rejected cost was persisted: records=%#v err=%v", records, listErr)
			}
		})
	}
}

func TestLedgerProjectsAdvisoryCommitmentAndCostContext(t *testing.T) {
	repository := NewMemoryRepository()
	service, _ := NewService(repository, func() time.Time { return ledgerTestNow })
	graph := lifeontology.NewService(lifeontology.NewMemoryRepository(), func() time.Time { return ledgerTestNow })
	if _, err := service.WithProjection(graph); err != nil {
		t.Fatal(err)
	}
	commitment := commitmentRequest("owner-a", "case/commitment", 0, CommitmentActive, "projected-commitment")
	commitment.ProjectKey = "case-project"
	commitmentResult, err := service.RecordCommitment(context.Background(), commitment)
	if err != nil || commitmentResult.Record.LifeGraph == nil || commitmentResult.Record.LifeGraphWarning != "" {
		t.Fatalf("commitment projection = %#v err=%v", commitmentResult.Record, err)
	}
	if !commitmentResult.Record.LifeGraph.AdvisoryOnly || commitmentResult.Record.LifeGraph.CanExecute || commitmentResult.Record.LifeGraph.GrantsAuthority {
		t.Fatalf("commitment graph crossed authority boundary: %#v", commitmentResult.Record.LifeGraph)
	}
	cost := costRequest("owner-a", CostPaid, "projected-cost")
	cost.Verification = VerificationHumanConfirmed
	cost.CommitmentKey = commitment.CommitmentKey
	cost.ProjectKey = commitment.ProjectKey
	costResult, err := service.RecordCost(context.Background(), cost)
	if err != nil || costResult.Record.LifeGraph == nil || len(costResult.Record.LifeGraph.Relations) != 2 {
		t.Fatalf("cost projection = %#v err=%v", costResult.Record, err)
	}
	entities, err := graph.QueryEntities(context.Background(), "owner-a", lifeontology.EntityQuery{AllowLocalOnly: true, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	counts := map[lifeontology.EntityType]int{}
	for _, entity := range entities {
		counts[entity.Type]++
	}
	if counts[lifeontology.EntityCommitment] != 1 || counts[lifeontology.EntityCost] != 1 || counts[lifeontology.EntityProject] != 1 {
		t.Fatalf("projected entity counts = %#v", counts)
	}
}

func commitmentRequest(owner, key string, expected uint64, status CommitmentStatus, idempotency string) RecordCommitmentRequest {
	return RecordCommitmentRequest{
		OwnerIdentity: owner, CommitmentKey: key, ExpectedRevision: expected,
		Domain: lifeontology.DomainLegalGovernment, Title: "Provide the verified response",
		Summary: "An evidence-backed commitment with explicit lifecycle state.", Status: status,
		Counterparty: "External party", ProjectKey: "case-project",
		Verification: VerificationSourceSupported, Evidence: []EvidenceReference{ledgerEvidence(idempotency)},
		IdempotencyKey: idempotency, ObservedAt: ledgerTestNow.Add(-time.Minute),
	}
}

func costRequest(owner string, kind CostKind, idempotency string) RecordCostRequest {
	return RecordCostRequest{
		OwnerIdentity: owner, Domain: lifeontology.DomainFinancial,
		Title: "Documented cost", Summary: "Source-backed cost event.", Kind: kind,
		AmountMinor: 1250, Currency: "eur", Verification: VerificationSourceSupported,
		Evidence: []EvidenceReference{ledgerEvidence(idempotency)}, IdempotencyKey: idempotency,
		ObservedAt: ledgerTestNow.Add(-time.Minute),
	}
}

func ledgerEvidence(seed string) EvidenceReference {
	sum := sha256.Sum256([]byte(seed))
	return EvidenceReference{
		ID: "evidence-" + seed, URI: "hai://tests/" + seed,
		ContentDigest: hex.EncodeToString(sum[:]), Authority: "integration-test",
		ObservedAt: ledgerTestNow.Add(-2 * time.Minute), Verification: VerificationSourceSupported,
	}
}
