package controlledlearning

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)

func TestRecordVerifiedOutcomeReconcilesCriteriaAndDrift(t *testing.T) {
	t.Parallel()
	service := newTestService(t)
	record, err := service.RecordOutcome(context.Background(), verifiedOutcomeRequest("outcome-1"))
	if err != nil {
		t.Fatalf("RecordOutcome: %v", err)
	}
	if record.ProtocolVersion != ProtocolVersion || record.EvidenceDigest == "" {
		t.Fatalf("missing versioned evidence metadata: %#v", record)
	}
	if record.Reconciliation.Status != ReconciliationDiverged {
		t.Fatalf("reconciliation status = %q", record.Reconciliation.Status)
	}
	if len(record.Reconciliation.FailedCriteria) != 1 ||
		record.Reconciliation.FailedCriteria[0] != "verified_result" {
		t.Fatalf("failed criteria = %#v", record.Reconciliation.FailedCriteria)
	}
	if len(record.Reconciliation.DriftSignals) != 1 ||
		record.Reconciliation.DriftSignals[0] != "latency_ms" {
		t.Fatalf("drift signals = %#v", record.Reconciliation.DriftSignals)
	}
	assertContainsMethod(t, record.Reconciliation.SuggestedMethods, MethodRootCauseAnalysis)
	assertContainsMethod(t, record.Reconciliation.SuggestedMethods, MethodDriftDetection)
}

func TestRecordOutcomeRejectsUnsupportedOrUnprovenEvidence(t *testing.T) {
	t.Parallel()
	service := newTestService(t)
	tests := []struct {
		name   string
		mutate func(*RecordOutcomeRequest)
	}{
		{"schema only", func(request *RecordOutcomeRequest) { request.Verification = VerificationSchemaValidated }},
		{"no provenance", func(request *RecordOutcomeRequest) { request.Sources = nil }},
		{"future evidence", func(request *RecordOutcomeRequest) { request.Sources[0].RetrievedAt = fixedNow.Add(time.Hour) }},
		{"unknown source reference", func(request *RecordOutcomeRequest) { request.Criteria[0].SourceIDs = []string{"missing"} }},
		{"credential URL", func(request *RecordOutcomeRequest) { request.Sources[0].URI = "https://user:secret@example.com/report" }},
		{"secret query", func(request *RecordOutcomeRequest) {
			request.Sources[0].URI = "https://example.com/report?access_token=secret"
		}},
		{"missing occurred time", func(request *RecordOutcomeRequest) { request.OccurredAt = time.Time{} }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := verifiedOutcomeRequest("outcome-" + test.name)
			test.mutate(&request)
			if _, err := service.RecordOutcome(context.Background(), request); err == nil {
				t.Fatal("RecordOutcome unexpectedly succeeded")
			}
		})
	}
}

func TestSupportedLearningMethodsCoverSection37(t *testing.T) {
	t.Parallel()
	methods := SupportedLearningMethods()
	if len(methods) != 30 {
		t.Fatalf("supported learning method count = %d, want 30", len(methods))
	}
	seen := map[LearningMethod]struct{}{}
	for _, method := range methods {
		if !validLearningMethod(method) {
			t.Fatalf("catalog contains invalid method %q", method)
		}
		if _, exists := seen[method]; exists {
			t.Fatalf("duplicate method %q", method)
		}
		seen[method] = struct{}{}
	}
	methods[0] = "mutated"
	if SupportedLearningMethods()[0] == "mutated" {
		t.Fatal("learning method catalog exposed shared mutable state")
	}
}

func TestHumanCorrectionRequiresExplicitHumanConfirmation(t *testing.T) {
	t.Parallel()
	service := newTestService(t)
	request := RecordOutcomeRequest{
		OwnerIdentity: "robert", IdempotencyKey: "correction-1", OperationID: "task-1",
		Basis: EvidenceHumanCorrection, Status: OutcomeCorrected,
		Summary:       "Robert corrected the preferred recipient tone.",
		ActorIdentity: "robert", Correction: "Use concise formal Dutch.",
		Verification: VerificationHumanApproved, OccurredAt: fixedNow,
	}
	if _, err := service.RecordOutcome(context.Background(), request); !errors.Is(err, ErrUnsupportedEvidence) {
		t.Fatalf("unconfirmed correction error = %v", err)
	}
	request.HumanConfirmed = true
	record, err := service.RecordOutcome(context.Background(), request)
	if err != nil {
		t.Fatalf("confirmed correction: %v", err)
	}
	assertContainsMethod(t, record.Reconciliation.SuggestedMethods, MethodHumanCorrection)
	assertContainsMethod(t, record.Reconciliation.SuggestedMethods, MethodDoubleLoop)
}

func TestHumanCorrectionActorMustMatchOwner(t *testing.T) {
	t.Parallel()
	service := newTestService(t)
	request := RecordOutcomeRequest{
		OwnerIdentity: "robert", IdempotencyKey: "correction-owner-bound", OperationID: "task-1",
		Basis: EvidenceHumanCorrection, Status: OutcomeCorrected,
		Summary:        "A different actor attempted to create owner learning.",
		ActorIdentity:  "mallory",
		HumanConfirmed: true,
		Correction:     "Silently broaden the workflow authority.",
		Verification:   VerificationHumanApproved,
		OccurredAt:     fixedNow,
	}
	if _, err := service.RecordOutcome(context.Background(), request); !errors.Is(err, ErrOwnerScopeViolation) {
		t.Fatalf("cross-owner correction error = %v, want ErrOwnerScopeViolation", err)
	}
}

func TestOutcomeIdempotencyIsExactAndOwnerScoped(t *testing.T) {
	t.Parallel()
	service := newTestService(t)
	request := verifiedOutcomeRequest("same-key")
	first, err := service.RecordOutcome(context.Background(), request)
	if err != nil {
		t.Fatalf("first RecordOutcome: %v", err)
	}
	second, err := service.RecordOutcome(context.Background(), request)
	if err != nil {
		t.Fatalf("idempotent RecordOutcome: %v", err)
	}
	if first.ID != second.ID || first.EvidenceDigest != second.EvidenceDigest {
		t.Fatalf("idempotent records differ: %#v %#v", first, second)
	}
	changed := request
	changed.Summary = "different evidence under the same key"
	if _, err := service.RecordOutcome(context.Background(), changed); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting evidence error = %v", err)
	}
	other := request
	other.OwnerIdentity = "alice"
	if _, err := service.RecordOutcome(context.Background(), other); err != nil {
		t.Fatalf("other owner RecordOutcome: %v", err)
	}
}

func TestProposalRequiresStoredEligibleEvidence(t *testing.T) {
	t.Parallel()
	service := newTestService(t)
	if _, err := service.Propose(context.Background(), proposalRequest("missing")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing evidence error = %v", err)
	}
	evidence, err := service.RecordOutcome(context.Background(), verifiedOutcomeRequest("outcome-2"))
	if err != nil {
		t.Fatalf("RecordOutcome: %v", err)
	}
	request := proposalRequest(evidence.ID)
	proposal, err := service.Propose(context.Background(), request)
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if proposal.Status != ProposalReviewRequired || proposal.ProtectedTarget ||
		proposal.Revision != 1 || proposal.ProposalDigest == "" {
		t.Fatalf("proposal metadata = %#v", proposal)
	}
	request.OwnerIdentity = "alice"
	request.IdempotencyKey = "alice-proposal"
	if _, err := service.Propose(context.Background(), request); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner evidence error = %v", err)
	}
}

func TestProposalAndDecisionRejectTamperedPersistentRecords(t *testing.T) {
	t.Parallel()
	repository := NewMemoryRepository()
	service, err := NewService(repository, func() time.Time { return fixedNow }, sequenceIDs())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	evidence, _ := service.RecordOutcome(context.Background(), verifiedOutcomeRequest("integrity-evidence"))
	key := scopedKey("robert", evidence.ID)
	repository.outcomes[key] = cloneOutcome(evidence)
	tampered := repository.outcomes[key]
	tampered.Summary = "tampered after persistence"
	repository.outcomes[key] = tampered
	if _, err := service.Propose(context.Background(), proposalRequest(evidence.ID)); !errors.Is(err, ErrIntegrityViolation) {
		t.Fatalf("tampered evidence error = %v", err)
	}

	repository.outcomes[key] = cloneOutcome(evidence)
	proposal, err := service.Propose(context.Background(), proposalRequest(evidence.ID))
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	proposalKey := scopedKey("robert", proposal.ID)
	tamperedProposal := repository.proposals[proposalKey]
	tamperedProposal.ProposedChange = "silently broaden authority"
	repository.proposals[proposalKey] = tamperedProposal
	if _, err := service.Decide(context.Background(), DecideRequest{
		OwnerIdentity: "robert", ProposalID: proposal.ID, ExpectedRevision: 1,
		Kind: DecisionApprove, ActorIdentity: "robert", HumanConfirmed: true,
		Rationale: "this must not approve altered persistence",
	}); !errors.Is(err, ErrIntegrityViolation) {
		t.Fatalf("tampered proposal error = %v", err)
	}
}

func TestOrdinaryProposalNeedsExplicitHumanDecision(t *testing.T) {
	t.Parallel()
	service := newTestService(t)
	evidence, err := service.RecordOutcome(context.Background(), verifiedOutcomeRequest("outcome-3"))
	if err != nil {
		t.Fatalf("RecordOutcome: %v", err)
	}
	proposal, err := service.Propose(context.Background(), proposalRequest(evidence.ID))
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	base := DecideRequest{
		OwnerIdentity: "robert", ProposalID: proposal.ID, ExpectedRevision: proposal.Revision,
		Kind: DecisionApprove, ActorIdentity: "robert", Rationale: "Evidence supports a bounded prompt revision.",
	}
	if _, err := service.Decide(context.Background(), base); err == nil {
		t.Fatal("unconfirmed approval unexpectedly succeeded")
	}
	base.HumanConfirmed = true
	approved, err := service.Decide(context.Background(), base)
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if approved.Status != ProposalApproved || approved.Revision != 2 {
		t.Fatalf("approved proposal = %#v", approved)
	}
	decisions, err := service.repository.ListDecisions(context.Background(), "robert", proposal.ID)
	if err != nil || len(decisions) != 1 || decisions[0].DecisionDigest == "" {
		t.Fatalf("decision history = %#v, err=%v", decisions, err)
	}
}

func TestDecisionActorMustMatchOwner(t *testing.T) {
	t.Parallel()
	service := newTestService(t)
	evidence, err := service.RecordOutcome(context.Background(), verifiedOutcomeRequest("decision-owner-bound"))
	if err != nil {
		t.Fatalf("RecordOutcome: %v", err)
	}
	proposal, err := service.Propose(context.Background(), proposalRequest(evidence.ID))
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	_, err = service.Decide(context.Background(), DecideRequest{
		OwnerIdentity: "robert", ProposalID: proposal.ID, ExpectedRevision: proposal.Revision,
		Kind: DecisionApprove, ActorIdentity: "reviewer", HumanConfirmed: true,
		Rationale: "Attempt to approve another owner's proposal.",
	})
	if !errors.Is(err, ErrOwnerScopeViolation) {
		t.Fatalf("cross-owner decision error = %v, want ErrOwnerScopeViolation", err)
	}
	decisions, listErr := service.repository.ListDecisions(context.Background(), "robert", proposal.ID)
	if listErr != nil || len(decisions) != 0 {
		t.Fatalf("cross-owner decision was persisted: %#v, err=%v", decisions, listErr)
	}
}

func TestProposalIdempotencyAndListFiltering(t *testing.T) {
	t.Parallel()
	repository := NewMemoryRepository()
	service, err := NewService(repository, func() time.Time { return fixedNow }, sequenceIDs())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	evidence, _ := service.RecordOutcome(context.Background(), verifiedOutcomeRequest("proposal-list-evidence"))
	request := proposalRequest(evidence.ID)
	first, err := service.Propose(context.Background(), request)
	if err != nil {
		t.Fatalf("first Propose: %v", err)
	}
	second, err := service.Propose(context.Background(), request)
	if err != nil || second.ID != first.ID {
		t.Fatalf("idempotent proposal = %#v, err=%v", second, err)
	}
	changed := request
	changed.Title = "different proposal"
	if _, err := service.Propose(context.Background(), changed); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting proposal error = %v", err)
	}
	list, err := repository.ListProposals(context.Background(), ProposalQuery{
		OwnerIdentity: "robert", Status: ProposalReviewRequired, Limit: 1,
	})
	if err != nil || len(list) != 1 || list[0].ID != first.ID {
		t.Fatalf("proposal list = %#v, err=%v", list, err)
	}
	list[0].EvidenceIDs[0] = "mutated"
	stored, _ := repository.GetProposal(context.Background(), "robert", first.ID)
	if stored.EvidenceIDs[0] == "mutated" {
		t.Fatal("proposal list exposed mutable repository state")
	}
	otherOwner, err := repository.ListProposals(context.Background(), ProposalQuery{OwnerIdentity: "alice"})
	if err != nil || len(otherOwner) != 0 {
		t.Fatalf("other owner proposals = %#v, err=%v", otherOwner, err)
	}
}

func TestReviewTransitionsAndProtectedPolicyMethodValidation(t *testing.T) {
	t.Parallel()
	service := newTestService(t)
	evidence, _ := service.RecordOutcome(context.Background(), verifiedOutcomeRequest("transition-evidence"))
	request := proposalRequest(evidence.ID)
	request.Method = MethodPolicyVersioning
	if _, err := service.Propose(context.Background(), request); err == nil {
		t.Fatal("policy versioning against a prompt unexpectedly succeeded")
	}
	request.Method = MethodPromptVersioning
	proposal, err := service.Propose(context.Background(), request)
	if err != nil {
		t.Fatalf("prompt version proposal: %v", err)
	}
	changed, err := service.Decide(context.Background(), DecideRequest{
		OwnerIdentity: "robert", ProposalID: proposal.ID, ExpectedRevision: 1,
		Kind: DecisionRequestChanges, ActorIdentity: "robert", HumanConfirmed: true,
		Rationale: "add a stronger held-out evaluation",
	})
	if err != nil || changed.Status != ProposalChangesRequested {
		t.Fatalf("request changes = %#v, err=%v", changed, err)
	}
	rejected, err := service.Decide(context.Background(), DecideRequest{
		OwnerIdentity: "robert", ProposalID: proposal.ID, ExpectedRevision: 2,
		Kind: DecisionReject, ActorIdentity: "robert", HumanConfirmed: true,
		Rationale: "the revision no longer has sufficient benefit",
	})
	if err != nil || rejected.Status != ProposalRejected {
		t.Fatalf("reject = %#v, err=%v", rejected, err)
	}
	if _, err := service.Decide(context.Background(), DecideRequest{
		OwnerIdentity: "robert", ProposalID: proposal.ID, ExpectedRevision: 3,
		Kind: DecisionReject, ActorIdentity: "robert", HumanConfirmed: true,
		Rationale: "duplicate terminal decision",
	}); !errors.Is(err, ErrInvalidStateChange) {
		t.Fatalf("terminal decision error = %v", err)
	}
}

func TestProtectedTargetsCannotBeApprovedByLearningFlow(t *testing.T) {
	t.Parallel()
	for _, target := range []TargetKind{
		TargetConstitution, TargetPermission, TargetSafetyBoundary,
		TargetApprovalPolicy, TargetAutonomyPolicy, TargetProviderBudget,
		TargetMandate, TargetExecutionPolicy,
	} {
		target := target
		t.Run(string(target), func(t *testing.T) {
			t.Parallel()
			service := newTestService(t)
			evidence, err := service.RecordOutcome(context.Background(), verifiedOutcomeRequest("evidence-"+string(target)))
			if err != nil {
				t.Fatalf("RecordOutcome: %v", err)
			}
			request := proposalRequest(evidence.ID)
			request.IdempotencyKey = "proposal-" + string(target)
			request.Target = target
			proposal, err := service.Propose(context.Background(), request)
			if err != nil {
				t.Fatalf("Propose: %v", err)
			}
			if proposal.Status != ProposalGovernanceRequired || !proposal.ProtectedTarget {
				t.Fatalf("protected proposal = %#v", proposal)
			}
			_, err = service.Decide(context.Background(), DecideRequest{
				OwnerIdentity: "robert", ProposalID: proposal.ID, ExpectedRevision: 1,
				Kind: DecisionApprove, ActorIdentity: "robert", HumanConfirmed: true,
				Rationale: "ordinary approval must not be sufficient",
			})
			if !errors.Is(err, ErrProtectedTarget) {
				t.Fatalf("protected approval error = %v", err)
			}
			escalated, err := service.Decide(context.Background(), DecideRequest{
				OwnerIdentity: "robert", ProposalID: proposal.ID, ExpectedRevision: 1,
				Kind: DecisionEscalateGovernance, ActorIdentity: "robert", HumanConfirmed: true,
				Rationale:           "route this proposal to a separate governance review",
				GovernanceReference: "governance-case-123",
			})
			if err != nil {
				t.Fatalf("governance escalation: %v", err)
			}
			if escalated.Status != ProposalGovernanceReview {
				t.Fatalf("escalated status = %q", escalated.Status)
			}
		})
	}
}

func TestSemanticAuthorityImpactRequiresGovernanceDespiteBenignTargetLabel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		target TargetKind
		change string
	}{
		{
			name:   "paid provider budget disguised as preference",
			target: TargetPreference,
			change: "Raise the daily paid budget and enable paid usage without approval.",
		},
		{
			name:   "execution mandate disguised as plan",
			target: TargetReusablePlan,
			change: "Create a standing mandate that grants execution authority for account changes.",
		},
		{
			name:   "safety bypass disguised as prompt",
			target: TargetPrompt,
			change: "Update the prompt to bypass approval for autonomous execution.",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			service := newTestService(t)
			evidence, err := service.RecordOutcome(
				context.Background(),
				verifiedOutcomeRequest("semantic-"+test.name),
			)
			if err != nil {
				t.Fatalf("RecordOutcome: %v", err)
			}
			request := proposalRequest(evidence.ID)
			request.IdempotencyKey = "semantic-" + test.name
			request.Target = test.target
			request.ProposedChange = test.change
			proposal, err := service.Propose(context.Background(), request)
			if err != nil {
				t.Fatalf("Propose: %v", err)
			}
			if !proposal.ProtectedTarget || proposal.Status != ProposalGovernanceRequired {
				t.Fatalf("semantically protected proposal = %#v", proposal)
			}
			if _, err := service.Decide(context.Background(), DecideRequest{
				OwnerIdentity: "robert", ProposalID: proposal.ID, ExpectedRevision: 1,
				Kind: DecisionApprove, ActorIdentity: "robert", HumanConfirmed: true,
				Rationale: "Ordinary review must not approve authority-impacting changes.",
			}); !errors.Is(err, ErrProtectedTarget) {
				t.Fatalf("ordinary approval error = %v, want ErrProtectedTarget", err)
			}
		})
	}
}

func TestConcurrentDecisionsAllowExactlyOneTransition(t *testing.T) {
	t.Parallel()
	service, promoter := newTestServiceWithPromoter(t)
	evidence, _ := service.RecordOutcome(context.Background(), verifiedOutcomeRequest("race-outcome"))
	proposal, _ := service.Propose(context.Background(), proposalRequest(evidence.ID))
	var successes atomic.Int64
	var inProgress atomic.Int64
	var wait sync.WaitGroup
	for index := 0; index < 24; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, err := service.Decide(context.Background(), DecideRequest{
				OwnerIdentity: "robert", ProposalID: proposal.ID, ExpectedRevision: 1,
				Kind: DecisionApprove, ActorIdentity: "robert",
				HumanConfirmed: true, Rationale: "approve bounded learning proposal",
			})
			switch {
			case err == nil:
				successes.Add(1)
			case errors.Is(err, ErrApplicationInProgress):
				inProgress.Add(1)
			default:
				t.Errorf("unexpected decision error: %v", err)
			}
		}(index)
	}
	wait.Wait()
	if successes.Load()+inProgress.Load() != 24 {
		t.Fatalf("successes=%d in-progress=%d", successes.Load(), inProgress.Load())
	}
	applyCalls, _, _ := promoter.calls()
	if applyCalls != 1 {
		t.Fatalf("promoter apply calls = %d, want 1", applyCalls)
	}
	decisions, err := service.repository.ListDecisions(context.Background(), "robert", proposal.ID)
	if err != nil || len(decisions) != 1 {
		t.Fatalf("decisions = %d, err = %v; want one durable decision", len(decisions), err)
	}
}

func TestRepositoryReturnsDefensiveCopiesAndBoundedQueries(t *testing.T) {
	t.Parallel()
	repository := NewMemoryRepository()
	service, err := NewService(repository, func() time.Time { return fixedNow }, sequenceIDs())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	first, _ := service.RecordOutcome(context.Background(), verifiedOutcomeRequest("copy-1"))
	for index := 2; index <= 5; index++ {
		request := verifiedOutcomeRequest(fmt.Sprintf("copy-%d", index))
		request.OperationID = fmt.Sprintf("operation-%d", index)
		_, _ = service.RecordOutcome(context.Background(), request)
	}
	first.Tags[0] = "mutated"
	stored, err := repository.GetOutcome(context.Background(), "robert", first.ID)
	if err != nil {
		t.Fatalf("GetOutcome: %v", err)
	}
	if stored.Tags[0] == "mutated" {
		t.Fatal("repository exposed mutable outcome state")
	}
	list, err := repository.ListOutcomes(context.Background(), OutcomeQuery{OwnerIdentity: "robert", Limit: 2})
	if err != nil || len(list) != 2 {
		t.Fatalf("bounded outcome list len=%d err=%v", len(list), err)
	}
	if _, err := repository.GetOutcome(context.Background(), "alice", first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner lookup error = %v", err)
	}
}

func TestMetricDirectionsAndValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		metric MetricResult
		drift  bool
	}{
		{MetricResult{Name: "exact-ok", Expected: 10, Actual: 11, Tolerance: 1, Direction: MetricExact}, false},
		{MetricResult{Name: "exact-drift", Expected: 10, Actual: 12, Tolerance: 1, Direction: MetricExact}, true},
		{MetricResult{Name: "minimum-ok", Expected: 10, Actual: 9, Tolerance: 1, Direction: MetricAtLeast}, false},
		{MetricResult{Name: "minimum-drift", Expected: 10, Actual: 8, Tolerance: 1, Direction: MetricAtLeast}, true},
		{MetricResult{Name: "maximum-ok", Expected: 10, Actual: 11, Tolerance: 1, Direction: MetricAtMost}, false},
		{MetricResult{Name: "maximum-drift", Expected: 10, Actual: 12, Tolerance: 1, Direction: MetricAtMost}, true},
	}
	for _, test := range tests {
		if got := metricDrifted(test.metric); got != test.drift {
			t.Errorf("metricDrifted(%s) = %v, want %v", test.metric.Name, got, test.drift)
		}
	}
	if err := validateMetrics([]MetricResult{{
		Name: "bad", Expected: 1, Actual: 1, Tolerance: -1, Direction: MetricExact,
	}}); err == nil {
		t.Fatal("negative metric tolerance unexpectedly accepted")
	}
}

func verifiedOutcomeRequest(key string) RecordOutcomeRequest {
	return RecordOutcomeRequest{
		OwnerIdentity: "robert", IdempotencyKey: key, OperationID: "task-123",
		ProjectKey: "hai", DomainPackIDs: []string{"work_venture", "work_venture"},
		Basis: EvidenceVerifiedOutcome, Status: OutcomeFailed,
		Summary:      "The execution completed but failed one verified success criterion.",
		Verification: VerificationTestPassed,
		Sources: []SourceReference{{
			ID: "test-report", Kind: "test_report", URI: "local://reports/task-123",
			RetrievedAt: fixedNow, ContentHash: "sha256:abc",
		}},
		Criteria: []CriterionResult{{
			ID: "verified_result", Description: "The external result matches the request.",
			Passed: false, SourceIDs: []string{"test-report"},
		}},
		Metrics: []MetricResult{{
			Name: "latency_ms", Expected: 100, Actual: 150, Tolerance: 10,
			Direction: MetricAtMost, Unit: "ms",
		}},
		Tags:       []string{"test", "test"},
		OccurredAt: fixedNow,
	}
}

func proposalRequest(evidenceID string) ProposeRequest {
	return ProposeRequest{
		OwnerIdentity: "robert", IdempotencyKey: "proposal-1",
		Method: MethodEvalDriven, Target: TargetPrompt,
		Title:          "Improve validation prompt",
		Hypothesis:     "A stricter success-criteria prompt will reduce false completion.",
		ProposedChange: "Require explicit evidence per success criterion.",
		CurrentVersion: "1.0.0", ProposedVersion: "1.1.0",
		RollbackPlan:   "Restore prompt version 1.0.0.",
		EvaluationPlan: "Shadow-test against the recorded failure and ten held-out tasks.",
		EvidenceIDs:    []string{evidenceID},
	}
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	service, _ := newTestServiceWithPromoter(t)
	return service
}

func sequenceIDs() func() string {
	var sequence atomic.Int64
	return func() string {
		return fmt.Sprintf("id-%04d", sequence.Add(1))
	}
}

func assertContainsMethod(t *testing.T, values []LearningMethod, expected LearningMethod) {
	t.Helper()
	for _, value := range values {
		if value == expected {
			return
		}
	}
	t.Fatalf("methods %#v do not contain %q", values, expected)
}
