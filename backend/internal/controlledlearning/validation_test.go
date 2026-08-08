package controlledlearning

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

func TestNewServiceRequiresRepository(t *testing.T) {
	t.Parallel()
	if _, err := NewService(nil, nil, nil); err == nil {
		t.Fatal("nil repository unexpectedly accepted")
	}
}

func TestOutcomeValidationBoundaries(t *testing.T) {
	t.Parallel()
	service := newTestService(t)
	tests := []struct {
		name   string
		mutate func(*RecordOutcomeRequest)
	}{
		{"missing owner", func(request *RecordOutcomeRequest) { request.OwnerIdentity = "" }},
		{"missing idempotency", func(request *RecordOutcomeRequest) { request.IdempotencyKey = "" }},
		{"missing operation", func(request *RecordOutcomeRequest) { request.OperationID = "" }},
		{"missing summary", func(request *RecordOutcomeRequest) { request.Summary = "" }},
		{"invalid status", func(request *RecordOutcomeRequest) { request.Status = "unknown" }},
		{"invalid verification", func(request *RecordOutcomeRequest) { request.Verification = "unknown" }},
		{"future outcome", func(request *RecordOutcomeRequest) { request.OccurredAt = fixedNow.Add(time.Hour) }},
		{"too many tags", func(request *RecordOutcomeRequest) { request.Tags = repeatedStrings(101, "tag") }},
		{"duplicate source", func(request *RecordOutcomeRequest) { request.Sources = append(request.Sources, request.Sources[0]) }},
		{"missing source time", func(request *RecordOutcomeRequest) { request.Sources[0].RetrievedAt = time.Time{} }},
		{"relative source URI", func(request *RecordOutcomeRequest) { request.Sources[0].URI = "/local/report" }},
		{"duplicate criterion", func(request *RecordOutcomeRequest) { request.Criteria = append(request.Criteria, request.Criteria[0]) }},
		{"duplicate metric", func(request *RecordOutcomeRequest) { request.Metrics = append(request.Metrics, request.Metrics[0]) }},
		{"invalid metric direction", func(request *RecordOutcomeRequest) { request.Metrics[0].Direction = "sideways" }},
		{"non-finite metric", func(request *RecordOutcomeRequest) { request.Metrics[0].Actual = math.Inf(1) }},
		{"unknown evidence basis", func(request *RecordOutcomeRequest) { request.Basis = "unknown" }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := verifiedOutcomeRequest("invalid-" + test.name)
			test.mutate(&request)
			if _, err := service.RecordOutcome(context.Background(), request); err == nil {
				t.Fatal("invalid outcome unexpectedly accepted")
			}
		})
	}
}

func TestProposalValidationBoundaries(t *testing.T) {
	t.Parallel()
	service := newTestService(t)
	evidence, err := service.RecordOutcome(context.Background(), verifiedOutcomeRequest("proposal-validation-evidence"))
	if err != nil {
		t.Fatalf("RecordOutcome: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*ProposeRequest)
	}{
		{"missing owner", func(request *ProposeRequest) { request.OwnerIdentity = "" }},
		{"missing idempotency", func(request *ProposeRequest) { request.IdempotencyKey = "" }},
		{"invalid method", func(request *ProposeRequest) { request.Method = "unknown" }},
		{"invalid target", func(request *ProposeRequest) { request.Target = "unknown" }},
		{"missing title", func(request *ProposeRequest) { request.Title = "" }},
		{"same version", func(request *ProposeRequest) { request.ProposedVersion = request.CurrentVersion }},
		{"missing rollback", func(request *ProposeRequest) { request.RollbackPlan = "" }},
		{"missing evidence", func(request *ProposeRequest) { request.EvidenceIDs = nil }},
		{"too much evidence", func(request *ProposeRequest) { request.EvidenceIDs = repeatedStrings(101, "evidence") }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := proposalRequest(evidence.ID)
			request.IdempotencyKey = "invalid-" + test.name
			test.mutate(&request)
			if _, err := service.Propose(context.Background(), request); err == nil {
				t.Fatal("invalid proposal unexpectedly accepted")
			}
		})
	}
}

func TestDecisionValidationBoundaries(t *testing.T) {
	t.Parallel()
	service := newTestService(t)
	evidence, _ := service.RecordOutcome(context.Background(), verifiedOutcomeRequest("decision-validation-evidence"))
	proposal, _ := service.Propose(context.Background(), proposalRequest(evidence.ID))
	base := DecideRequest{
		OwnerIdentity: "robert", ProposalID: proposal.ID, ExpectedRevision: 1,
		Kind: DecisionApprove, ActorIdentity: "robert", HumanConfirmed: true,
		Rationale: "bounded improvement is supported by evidence",
	}
	tests := []struct {
		name   string
		mutate func(*DecideRequest)
	}{
		{"missing owner", func(request *DecideRequest) { request.OwnerIdentity = "" }},
		{"missing proposal", func(request *DecideRequest) { request.ProposalID = "" }},
		{"invalid revision", func(request *DecideRequest) { request.ExpectedRevision = 0 }},
		{"missing actor", func(request *DecideRequest) { request.ActorIdentity = "" }},
		{"actor owner mismatch", func(request *DecideRequest) { request.ActorIdentity = "mallory" }},
		{"not confirmed", func(request *DecideRequest) { request.HumanConfirmed = false }},
		{"missing rationale", func(request *DecideRequest) { request.Rationale = "" }},
		{"unknown decision", func(request *DecideRequest) { request.Kind = "unknown" }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := base
			test.mutate(&request)
			if _, err := service.Decide(context.Background(), request); err == nil {
				t.Fatal("invalid decision unexpectedly accepted")
			}
		})
	}
}

func TestProtectedGovernanceEscalationRequiresReference(t *testing.T) {
	t.Parallel()
	service := newTestService(t)
	evidence, _ := service.RecordOutcome(context.Background(), verifiedOutcomeRequest("governance-evidence"))
	request := proposalRequest(evidence.ID)
	request.Target = TargetSafetyBoundary
	request.Method = MethodPolicyVersioning
	proposal, err := service.Propose(context.Background(), request)
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	_, err = service.Decide(context.Background(), DecideRequest{
		OwnerIdentity: "robert", ProposalID: proposal.ID, ExpectedRevision: 1,
		Kind: DecisionEscalateGovernance, ActorIdentity: "robert", HumanConfirmed: true,
		Rationale: "separate governance must review this protected change",
	})
	if err == nil || !strings.Contains(err.Error(), "governance reference") {
		t.Fatalf("missing governance reference error = %v", err)
	}
}

func TestMemoryRepositoryContractErrorsAndCancellation(t *testing.T) {
	t.Parallel()
	repository := NewMemoryRepository()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := repository.ListOutcomes(ctx, OutcomeQuery{OwnerIdentity: "robert"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("ListOutcomes cancellation error = %v", err)
	}
	if _, err := repository.ListProposals(context.Background(), ProposalQuery{}); !errors.Is(err, ErrOwnerScopeViolation) {
		t.Fatalf("ownerless ListProposals error = %v", err)
	}
	if _, err := repository.GetProposal(context.Background(), "", "proposal"); !errors.Is(err, ErrOwnerScopeViolation) {
		t.Fatalf("ownerless GetProposal error = %v", err)
	}
	if _, err := repository.ListDecisions(context.Background(), "robert", "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing decision history error = %v", err)
	}
}

func repeatedStrings(count int, value string) []string {
	result := make([]string, count)
	for index := range result {
		result[index] = value + string(rune(index+1))
	}
	return result
}
