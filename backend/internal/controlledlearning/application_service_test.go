package controlledlearning

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestApprovalFailsClosedWithoutPromoter(t *testing.T) {
	t.Parallel()
	repository := NewMemoryRepository()
	service, err := NewService(
		repositoryWithoutApplications{Repository: repository},
		func() time.Time { return fixedNow },
		sequenceIDs(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	evidence, err := service.RecordOutcome(
		context.Background(),
		verifiedOutcomeRequest("no-promoter-evidence"),
	)
	if err != nil {
		t.Fatalf("RecordOutcome: %v", err)
	}
	proposal, err := service.Propose(
		context.Background(),
		proposalRequest(evidence.ID),
	)
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	_, err = service.Decide(context.Background(), approvedDecision(proposal))
	if !errors.Is(err, ErrPromoterUnavailable) {
		t.Fatalf("approval error = %v, want ErrPromoterUnavailable", err)
	}
	stored, err := repository.GetProposal(context.Background(), "robert", proposal.ID)
	if err != nil {
		t.Fatalf("GetProposal: %v", err)
	}
	if stored.Status != ProposalReviewRequired || stored.Revision != 1 {
		t.Fatalf("proposal advanced without promoter: %#v", stored)
	}
	if len(repository.applications) != 0 || len(repository.decisions[scopedKey("robert", proposal.ID)]) != 0 {
		t.Fatal("approval without promoter persisted application or decision")
	}
}

type repositoryWithoutApplications struct {
	Repository
}

func TestApproveAppliesDurablyAndReplayIsIdempotent(t *testing.T) {
	t.Parallel()
	service, promoter := newTestServiceWithPromoter(t)
	evidence, _ := service.RecordOutcome(
		context.Background(),
		verifiedOutcomeRequest("durable-apply-evidence"),
	)
	proposal, _ := service.Propose(
		context.Background(),
		proposalRequest(evidence.ID),
	)
	request := approvedDecision(proposal)
	request.IdempotencyKey = "apply-proposal-once"

	first, err := service.DecideAndApply(context.Background(), request)
	if err != nil {
		t.Fatalf("DecideAndApply: %v", err)
	}
	if first.Proposal.Status != ProposalApproved || first.Proposal.Revision != 2 {
		t.Fatalf("approved proposal = %#v", first.Proposal)
	}
	if first.Application == nil ||
		first.Application.Status != ApplicationApplied ||
		first.Application.AppliedVersion != proposal.ProposedVersion ||
		first.Application.RollbackToken == "" ||
		first.Application.ResultDigest == "" ||
		len(first.Application.Evidence) != 1 {
		t.Fatalf("application evidence = %#v", first.Application)
	}
	second, err := service.DecideAndApply(context.Background(), request)
	if err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}
	if second.Application == nil || second.Application.ID != first.Application.ID {
		t.Fatalf("replay application = %#v, want %s", second.Application, first.Application.ID)
	}
	applyCalls, _, _ := promoter.calls()
	if applyCalls != 1 {
		t.Fatalf("promoter apply calls = %d, want 1", applyCalls)
	}
	events, err := service.ListApplicationEvents(
		context.Background(),
		"robert",
		first.Application.ID,
	)
	if err != nil {
		t.Fatalf("ListApplicationEvents: %v", err)
	}
	if len(events) != 2 ||
		events[0].Kind != ApplicationEventReserved ||
		events[1].Kind != ApplicationEventApplied {
		t.Fatalf("application events = %#v", events)
	}
	decisions, err := service.repository.ListDecisions(
		context.Background(),
		"robert",
		proposal.ID,
	)
	if err != nil || len(decisions) != 1 ||
		decisions[0].ApplicationID != first.Application.ID {
		t.Fatalf("application-bound decision = %#v, err=%v", decisions, err)
	}

	conflicting := request
	conflicting.Rationale = "A different approval intent must not reuse the application."
	if _, err := service.DecideAndApply(context.Background(), conflicting); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting replay error = %v", err)
	}
}

func TestFailedApplicationRetriesSameDurableIdentity(t *testing.T) {
	t.Parallel()
	service, promoter := newTestServiceWithPromoter(t)
	promoter.applyErr = errTestPromotion
	evidence, _ := service.RecordOutcome(
		context.Background(),
		verifiedOutcomeRequest("retry-apply-evidence"),
	)
	proposal, _ := service.Propose(
		context.Background(),
		proposalRequest(evidence.ID),
	)
	request := approvedDecision(proposal)
	request.IdempotencyKey = "retry-same-application"

	first, err := service.DecideAndApply(context.Background(), request)
	if !errors.Is(err, ErrApplicationFailed) {
		t.Fatalf("first apply error = %v", err)
	}
	if first.Application == nil || first.Application.Status != ApplicationFailed {
		t.Fatalf("failed application = %#v", first.Application)
	}
	storedProposal, _ := service.repository.GetProposal(
		context.Background(),
		"robert",
		proposal.ID,
	)
	if storedProposal.Status != ProposalReviewRequired || storedProposal.Revision != 1 {
		t.Fatalf("failed apply advanced proposal: %#v", storedProposal)
	}

	second, err := service.DecideAndApply(context.Background(), request)
	if err != nil {
		t.Fatalf("retry apply: %v", err)
	}
	if second.Application == nil ||
		second.Application.ID != first.Application.ID ||
		second.Application.Attempt != 2 ||
		second.Application.Status != ApplicationApplied {
		t.Fatalf("retried application = %#v", second.Application)
	}
	applyCalls, _, _ := promoter.calls()
	if applyCalls != 2 {
		t.Fatalf("promoter apply calls = %d, want 2", applyCalls)
	}
	events, _ := service.ListApplicationEvents(
		context.Background(),
		"robert",
		first.Application.ID,
	)
	want := []ApplicationEventKind{
		ApplicationEventReserved,
		ApplicationEventFailed,
		ApplicationEventAttemptStarted,
		ApplicationEventApplied,
	}
	if len(events) != len(want) {
		t.Fatalf("event count = %d, want %d: %#v", len(events), len(want), events)
	}
	for index := range want {
		if events[index].Kind != want[index] {
			t.Fatalf("event[%d] = %q, want %q", index, events[index].Kind, want[index])
		}
	}
}

func TestProtectedProposalCreatesReviewHandoffWithoutApplying(t *testing.T) {
	t.Parallel()
	service, promoter := newTestServiceWithPromoter(t)
	evidence, _ := service.RecordOutcome(
		context.Background(),
		verifiedOutcomeRequest("protected-handoff-evidence"),
	)
	request := proposalRequest(evidence.ID)
	request.Target = TargetApprovalPolicy
	request.Method = MethodPolicyVersioning
	request.IdempotencyKey = "protected-handoff-proposal"
	proposal, err := service.Propose(context.Background(), request)
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if _, err := service.Decide(
		context.Background(),
		approvedDecision(proposal),
	); !errors.Is(err, ErrProtectedTarget) {
		t.Fatalf("ordinary protected approval error = %v", err)
	}
	decision := approvedDecision(proposal)
	decision.Kind = DecisionEscalateGovernance
	decision.GovernanceReference = "governance-review-42"
	decision.Rationale = "Create an independent protected-policy review case."
	decision.IdempotencyKey = "protected-handoff-once"
	first, err := service.DecideAndApply(context.Background(), decision)
	if err != nil {
		t.Fatalf("governance handoff: %v", err)
	}
	if first.Proposal.Status != ProposalGovernanceReview ||
		first.Application == nil ||
		first.Application.Status != ApplicationHandoffReady ||
		first.Application.AppliedVersion != "" ||
		first.Application.HandoffReference == "" {
		t.Fatalf("protected handoff result = %#v", first)
	}
	if _, err := service.DecideAndApply(context.Background(), decision); err != nil {
		t.Fatalf("idempotent protected handoff replay: %v", err)
	}
	applyCalls, handoffCalls, rollbackCalls := promoter.calls()
	if applyCalls != 0 || handoffCalls != 1 || rollbackCalls != 0 {
		t.Fatalf(
			"promoter calls apply=%d handoff=%d rollback=%d",
			applyCalls,
			handoffCalls,
			rollbackCalls,
		)
	}
	if _, err := service.Rollback(context.Background(), RollbackRequest{
		OwnerIdentity: "robert", ApplicationID: first.Application.ID,
		ActorIdentity: "robert", HumanConfirmed: true,
		Rationale:       "A handoff must not be treated as an applied change.",
		ExpectedVersion: proposal.ProposedVersion,
	}); !errors.Is(err, ErrRollbackUnavailable) {
		t.Fatalf("protected handoff rollback error = %v", err)
	}
}

func TestRollbackPreservesVersionProvenanceAndIsIdempotent(t *testing.T) {
	t.Parallel()
	service, promoter := newTestServiceWithPromoter(t)
	evidence, _ := service.RecordOutcome(
		context.Background(),
		verifiedOutcomeRequest("rollback-evidence"),
	)
	proposal, _ := service.Propose(
		context.Background(),
		proposalRequest(evidence.ID),
	)
	approved, err := service.DecideAndApply(
		context.Background(),
		approvedDecision(proposal),
	)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	request := RollbackRequest{
		OwnerIdentity:   "robert",
		ApplicationID:   approved.Application.ID,
		IdempotencyKey:  "rollback-once",
		ActorIdentity:   "robert",
		HumanConfirmed:  true,
		Rationale:       "The post-apply evaluation regressed.",
		ExpectedVersion: proposal.ProposedVersion,
	}
	rolledBack, err := service.Rollback(context.Background(), request)
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if rolledBack.Status != ApplicationRolledBack ||
		rolledBack.AppliedVersion != proposal.ProposedVersion ||
		rolledBack.RestoredVersion != proposal.CurrentVersion ||
		rolledBack.RollbackIntentDigest == "" ||
		len(rolledBack.RollbackEvidence) != 1 ||
		rolledBack.RolledBackAt.IsZero() {
		t.Fatalf("rollback provenance = %#v", rolledBack)
	}
	replayed, err := service.Rollback(context.Background(), request)
	if err != nil || replayed.ID != rolledBack.ID {
		t.Fatalf("rollback replay = %#v, err=%v", replayed, err)
	}
	_, _, rollbackCalls := promoter.calls()
	if rollbackCalls != 1 {
		t.Fatalf("promoter rollback calls = %d, want 1", rollbackCalls)
	}
	events, _ := service.ListApplicationEvents(
		context.Background(),
		"robert",
		rolledBack.ID,
	)
	if len(events) != 4 ||
		events[2].Kind != ApplicationEventRollbackStarted ||
		events[3].Kind != ApplicationEventRolledBack {
		t.Fatalf("rollback event history = %#v", events)
	}

	wrongVersion := request
	wrongVersion.ExpectedVersion = "unexpected-version"
	if _, err := service.Rollback(context.Background(), wrongVersion); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("wrong-version rollback error = %v", err)
	}
}

func approvedDecision(proposal LearningProposal) DecideRequest {
	return DecideRequest{
		OwnerIdentity:    proposal.OwnerIdentity,
		ProposalID:       proposal.ID,
		ExpectedRevision: proposal.Revision,
		Kind:             DecisionApprove,
		ActorIdentity:    proposal.OwnerIdentity,
		HumanConfirmed:   true,
		Rationale:        "Verified evidence supports this bounded learning change.",
	}
}
