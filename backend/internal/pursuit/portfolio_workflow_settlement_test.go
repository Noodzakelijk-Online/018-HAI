package pursuit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/controlledlearning"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/workflow"

	"github.com/google/uuid"
)

func TestPortfolioWorkflowSettlementRequiresVerifiedReceiptBoundCompletionAndReplays(t *testing.T) {
	svc, repo, item, execution := completedPortfolioWorkflowFixture(t, "verified")
	request := PortfolioWorkflowSettlementRequest{
		WorkflowID: execution.WorkflowID.String(), ExpectedItemDigest: item.RecordDigest,
		ActualEffortMinutes: 12, ActualCostMicros: 0,
		Confirmation: PortfolioWorkflowSettlementConfirmation,
	}

	result, err := svc.SettlePortfolioWorkflowForOwner(
		context.Background(), "alice", "alice", item.ID, request,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Replayed || result.PursuitID != item.PursuitID ||
		result.ProposalItemID != item.ID || result.ReservationID != item.ReservationID ||
		result.WorkflowID != execution.WorkflowID || result.Disposition != ResourceReservationConsumed ||
		result.ActualEffortMinutes != 12 || result.VerificationStatus != "verified" ||
		result.Authority != PortfolioWorkflowSettlementAuthority || result.CanExecute ||
		result.ResourceUsage == nil || !strings.Contains(result.EvidenceURI, "workflow-completion-attestations") {
		t.Fatalf("settlement result=%#v", result)
	}
	settled, ok := repo.resourceSettlements[item.ReservationID]
	if !ok || settled.ActualEffortMinutes != 12 || settled.Disposition != ResourceReservationConsumed ||
		settled.EvidenceURI != result.EvidenceURI {
		t.Fatalf("stored settlement=%#v", settled)
	}
	archived := repo.workflows[execution.WorkflowID]
	archived.Archived = true
	archived.CurrentState = workflow.StateArchived
	repo.workflows[execution.WorkflowID] = archived

	replayed, err := svc.SettlePortfolioWorkflowForOwner(
		context.Background(), "alice", "alice", item.ID, request,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || len(repo.resourceSettlements) != 1 || replayed.EvidenceURI != result.EvidenceURI {
		t.Fatalf("settlement replay=%#v count=%d", replayed, len(repo.resourceSettlements))
	}

	changed := request
	changed.ActualEffortMinutes++
	if _, err := svc.SettlePortfolioWorkflowForOwner(
		context.Background(), "alice", "alice", item.ID, changed,
	); err == nil || !strings.Contains(err.Error(), "different outcome") {
		t.Fatalf("changed settlement replay error=%v", err)
	}
}

func TestPortfolioWorkflowSettlementRecordsEvidenceWithoutInventingAReviewProposal(t *testing.T) {
	svc, repo, item, execution := completedPortfolioWorkflowFixture(t, "test_passed")
	learningRepository := controlledlearning.NewMemoryRepository()
	learningService, err := controlledlearning.NewService(learningRepository, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.controlledLearning = learningService
	request := PortfolioWorkflowSettlementRequest{
		WorkflowID: execution.WorkflowID.String(), ExpectedItemDigest: item.RecordDigest,
		ActualEffortMinutes: 12, ActualCostMicros: 250000,
		Confirmation: PortfolioWorkflowSettlementConfirmation,
	}

	result, err := svc.SettlePortfolioWorkflowForOwner(
		context.Background(), "alice", "alice", item.ID, request,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.LearningStatus != PortfolioLearningRecorded || result.LearningOutcomeID == "" ||
		result.LearningReviewRequired || result.LearningProposalID != "" ||
		result.LearningProposalStatus != controlledlearning.EstimateCalibrationInsufficientEvidence ||
		result.LearningSampleCount != 1 || result.LearningNewEvidenceCount != 1 || result.LearningDriftDetected ||
		result.CompletionAttestationID == uuid.Nil ||
		result.SettlementProofID == uuid.Nil || result.SettlementProofDigest == "" {
		t.Fatalf("settlement learning result=%#v", result)
	}

	replayed, err := svc.SettlePortfolioWorkflowForOwner(
		context.Background(), "alice", "alice", item.ID, request,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || replayed.LearningOutcomeID != result.LearningOutcomeID {
		t.Fatalf("learning replay=%#v", replayed)
	}
	outcomes, err := learningRepository.ListOutcomes(context.Background(), controlledlearning.OutcomeQuery{
		OwnerIdentity: "alice", OperationID: execution.WorkflowID.String(), Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 1 {
		t.Fatalf("learning outcomes=%d, want one idempotent record", len(outcomes))
	}
	outcome := outcomes[0]
	if outcome.Basis != controlledlearning.EvidenceVerifiedOutcome ||
		outcome.Verification != controlledlearning.VerificationTestPassed ||
		outcome.Status != controlledlearning.OutcomeSucceeded || len(outcome.Sources) != 3 ||
		len(outcome.Criteria) != 3 || len(outcome.Metrics) != 2 {
		t.Fatalf("learning outcome=%#v", outcome)
	}
	reservation, err := repo.FindResourceReservationByID("alice", item.PursuitID, item.ReservationID)
	if err != nil {
		t.Fatal(err)
	}
	metrics := map[string]controlledlearning.MetricResult{}
	for _, metric := range outcome.Metrics {
		metrics[metric.Name] = metric
	}
	if metrics["portfolio_effort_minutes"].Expected != float64(reservation.EstimatedEffortMinutes) ||
		metrics["portfolio_effort_minutes"].Actual != 12 ||
		metrics["portfolio_cost_micros"].Expected != float64(reservation.EstimatedCostMicros) ||
		metrics["portfolio_cost_micros"].Actual != 250000 {
		t.Fatalf("learning metrics=%#v reservation=%#v", outcome.Metrics, reservation)
	}
}

func TestThirdComparableSettlementCreatesReviewRequiredCalibrationProposal(t *testing.T) {
	svc, repo, item, execution := completedPortfolioWorkflowFixture(t, "verified")
	learningRepository := controlledlearning.NewMemoryRepository()
	learningService, err := controlledlearning.NewService(learningRepository, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.controlledLearning = learningService
	svc.portfolioCalibration = learningService
	pursuitRecord, err := repo.FindByID(item.PursuitID)
	if err != nil || pursuitRecord == nil {
		t.Fatalf("FindByID result=%#v err=%v", pursuitRecord, err)
	}
	scope := portfolioCalibrationScope(pursuitRecord.ProjectKey, item.PursuitID)
	for index := 0; index < 2; index++ {
		recordComparableSettlementEvidence(t, learningService, scope, index)
	}
	result, err := svc.SettlePortfolioWorkflowForOwner(
		context.Background(), "alice", "alice", item.ID,
		PortfolioWorkflowSettlementRequest{
			WorkflowID: execution.WorkflowID.String(), ExpectedItemDigest: item.RecordDigest,
			ActualEffortMinutes: 12, ActualCostMicros: 250000,
			Confirmation: PortfolioWorkflowSettlementConfirmation,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.LearningStatus != PortfolioLearningRecorded || !result.LearningReviewRequired ||
		result.LearningProposalStatus != string(controlledlearning.ProposalReviewRequired) ||
		result.LearningProposalID == "" || result.LearningSampleCount != 3 ||
		result.LearningNewEvidenceCount != 3 || result.LearningDriftDetected {
		t.Fatalf("third-settlement learning result=%#v", result)
	}
	proposals, err := learningRepository.ListProposals(context.Background(), controlledlearning.ProposalQuery{
		OwnerIdentity: "alice", Status: controlledlearning.ProposalReviewRequired, Limit: 10,
	})
	if err != nil || len(proposals) != 1 || proposals[0].ID != result.LearningProposalID ||
		proposals[0].Target != controlledlearning.TargetPlanningEstimateCalibration {
		t.Fatalf("calibration proposals=%#v err=%v", proposals, err)
	}
}

func TestPortfolioWorkflowSettlementReplayRecoversLearningWithoutRepeatingAccounting(t *testing.T) {
	svc, repo, item, execution := completedPortfolioWorkflowFixture(t, "verified")
	svc.controlledLearning = failingPortfolioLearningRecorder{err: errors.New("temporary ledger outage")}
	request := PortfolioWorkflowSettlementRequest{
		WorkflowID: execution.WorkflowID.String(), ExpectedItemDigest: item.RecordDigest,
		ActualEffortMinutes: 8, Confirmation: PortfolioWorkflowSettlementConfirmation,
	}
	result, err := svc.SettlePortfolioWorkflowForOwner(
		context.Background(), "alice", "alice", item.ID, request,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.LearningStatus != PortfolioLearningFailed || result.LearningReviewRequired ||
		len(repo.resourceSettlements) != 1 {
		t.Fatalf("failed learning settlement=%#v accounting=%d", result, len(repo.resourceSettlements))
	}

	learningRepository := controlledlearning.NewMemoryRepository()
	learningService, err := controlledlearning.NewService(learningRepository, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.controlledLearning = learningService
	replayed, err := svc.SettlePortfolioWorkflowForOwner(
		context.Background(), "alice", "alice", item.ID, request,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || replayed.LearningStatus != PortfolioLearningRecorded ||
		replayed.LearningOutcomeID == "" || len(repo.resourceSettlements) != 1 {
		t.Fatalf("recovered learning=%#v accounting=%d", replayed, len(repo.resourceSettlements))
	}
}

type failingPortfolioLearningRecorder struct {
	err error
}

func recordComparableSettlementEvidence(
	t *testing.T,
	service *controlledlearning.Service,
	scope string,
	index int,
) {
	t.Helper()
	occurredAt := time.Now().UTC().Add(-time.Duration(index+2) * time.Hour).Truncate(time.Second)
	sourceID := fmt.Sprintf("prior-settlement-%d", index)
	_, err := service.RecordOutcome(context.Background(), controlledlearning.RecordOutcomeRequest{
		OwnerIdentity: "alice", IdempotencyKey: sourceID, OperationID: sourceID,
		ProjectKey: scope, Basis: controlledlearning.EvidenceVerifiedOutcome,
		Status: controlledlearning.OutcomeSucceeded, Summary: "A prior verified portfolio settlement was reconciled.",
		Verification: controlledlearning.VerificationVerified,
		Sources: []controlledlearning.SourceReference{{
			ID: sourceID, Kind: "portfolio_workflow_settlement_proof",
			URI:         "hai://test/portfolio-settlements/" + sourceID,
			RetrievedAt: occurredAt, ContentHash: strings.Repeat("d", 64),
		}},
		Criteria: []controlledlearning.CriterionResult{{
			ID: "settled", Description: "The workflow settlement was verified.", Passed: true,
			SourceIDs: []string{sourceID},
		}},
		Metrics: []controlledlearning.MetricResult{
			{Name: "portfolio_effort_minutes", Expected: 10, Actual: 20, Direction: controlledlearning.MetricExact, Unit: "minutes"},
			{Name: "portfolio_cost_micros", Expected: 100000, Actual: 200000, Direction: controlledlearning.MetricExact, Unit: "EUR_micros"},
		},
		Tags: []string{"portfolio-settlement", "outcome-reconciliation"}, OccurredAt: occurredAt,
	})
	if err != nil {
		t.Fatalf("RecordOutcome(%s): %v", sourceID, err)
	}
}

func (recorder failingPortfolioLearningRecorder) RecordOutcome(
	context.Context,
	controlledlearning.RecordOutcomeRequest,
) (controlledlearning.OutcomeRecord, error) {
	return controlledlearning.OutcomeRecord{}, recorder.err
}

func TestPortfolioWorkflowSettlementRejectsMutableCompletionWithoutAttestation(t *testing.T) {
	svc, repo, item, execution := completedPortfolioWorkflowFixture(t, "verified")
	delete(repo.workflowAttestations, execution.WorkflowID)
	request := PortfolioWorkflowSettlementRequest{
		WorkflowID: execution.WorkflowID.String(), ExpectedItemDigest: item.RecordDigest,
		ActualEffortMinutes: 2, Confirmation: PortfolioWorkflowSettlementConfirmation,
	}
	if _, err := svc.SettlePortfolioWorkflowForOwner(context.Background(), "alice", "alice", item.ID, request); err == nil ||
		!strings.Contains(err.Error(), "immutable verified completion") {
		t.Fatalf("forged mutable completion error=%v", err)
	}
	if len(repo.resourceSettlements) != 0 || len(repo.portfolioProofs) != 0 {
		t.Fatal("forged completion changed accounting")
	}
}

func TestPortfolioWorkflowSettlementRejectsInsufficientVerificationStatuses(t *testing.T) {
	for _, status := range []string{"schema_validated", "source_supported", "human_approved", "needs_review", "unsupported"} {
		t.Run(status, func(t *testing.T) {
			svc, repo, item, execution := completedPortfolioWorkflowFixture(t, status)
			request := PortfolioWorkflowSettlementRequest{
				WorkflowID: execution.WorkflowID.String(), ExpectedItemDigest: item.RecordDigest,
				ActualEffortMinutes: 2, Confirmation: PortfolioWorkflowSettlementConfirmation,
			}
			if _, err := svc.SettlePortfolioWorkflowForOwner(context.Background(), "alice", "alice", item.ID, request); err == nil {
				t.Fatalf("verification status %q was accepted", status)
			}
			if len(repo.resourceSettlements) != 0 {
				t.Fatal("rejected verification status changed accounting")
			}
		})
	}
}

func TestPortfolioWorkflowSettlementRejectsReceiptLinkedToMultipleWorkflows(t *testing.T) {
	svc, repo, item, execution := completedPortfolioWorkflowFixture(t, "verified")
	links, err := repo.FindLinks(item.PursuitID)
	if err != nil {
		t.Fatal(err)
	}
	receiptURI := ""
	for _, link := range links {
		if link.LinkID == execution.WorkflowID.String() && link.Relationship == PortfolioWorkflowEffectRelationship {
			receiptURI = link.SourceURI
		}
	}
	conflictingID := uuid.New()
	repo.workflows[conflictingID] = models.WorkflowItem{ID: conflictingID, OwnerIdentity: "alice", SourceType: PortfolioWorkflowEffectSourceType}
	if _, err := repo.CreateLink(&models.PursuitLink{
		PursuitID: item.PursuitID, LinkType: LinkWorkflow, LinkID: conflictingID.String(),
		Relationship: PortfolioWorkflowEffectRelationship, SourceURI: receiptURI,
	}); err != nil {
		t.Fatal(err)
	}
	request := PortfolioWorkflowSettlementRequest{
		WorkflowID: execution.WorkflowID.String(), ExpectedItemDigest: item.RecordDigest,
		ActualEffortMinutes: 2, Confirmation: PortfolioWorkflowSettlementConfirmation,
	}
	if _, err := svc.SettlePortfolioWorkflowForOwner(context.Background(), "alice", "alice", item.ID, request); err == nil ||
		!strings.Contains(err.Error(), "conflicting workflows") {
		t.Fatalf("duplicate receipt workflow error=%v", err)
	}
}

func TestPortfolioWorkflowSettlementFailsClosedBeforeVerifiedCompletion(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*models.WorkflowItem, *PortfolioWorkflowSettlementRequest)
	}{
		{
			name: "workflow not completed",
			mutate: func(item *models.WorkflowItem, _ *PortfolioWorkflowSettlementRequest) {
				item.CurrentState = workflow.StateReady
				item.CompletedAt = nil
			},
		},
		{
			name: "verification requires review",
			mutate: func(item *models.WorkflowItem, _ *PortfolioWorkflowSettlementRequest) {
				item.VerificationStatus = "needs_review"
			},
		},
		{
			name: "workflow has no durable task plan",
			mutate: func(item *models.WorkflowItem, _ *PortfolioWorkflowSettlementRequest) {
				item.LastTaskPlanID = ""
			},
		},
		{
			name: "different linked workflow",
			mutate: func(_ *models.WorkflowItem, request *PortfolioWorkflowSettlementRequest) {
				request.WorkflowID = uuid.NewString()
			},
		},
		{
			name: "changed proposal item digest",
			mutate: func(_ *models.WorkflowItem, request *PortfolioWorkflowSettlementRequest) {
				request.ExpectedItemDigest = strings.Repeat("f", 64)
			},
		},
		{
			name: "missing exact confirmation",
			mutate: func(_ *models.WorkflowItem, request *PortfolioWorkflowSettlementRequest) {
				request.Confirmation = "SETTLE"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc, repo, proposalItem, execution := completedPortfolioWorkflowFixture(t, "test_passed")
			workflowItem := repo.workflows[execution.WorkflowID]
			request := PortfolioWorkflowSettlementRequest{
				WorkflowID: execution.WorkflowID.String(), ExpectedItemDigest: proposalItem.RecordDigest,
				ActualEffortMinutes: 4, Confirmation: PortfolioWorkflowSettlementConfirmation,
			}
			test.mutate(&workflowItem, &request)
			repo.workflows[execution.WorkflowID] = workflowItem
			if _, err := svc.SettlePortfolioWorkflowForOwner(
				context.Background(), "alice", "alice", proposalItem.ID, request,
			); err == nil {
				t.Fatal("expected settlement to fail closed")
			}
			if len(repo.resourceSettlements) != 0 {
				t.Fatalf("failed settlement wrote %d records", len(repo.resourceSettlements))
			}
		})
	}
}

func TestPortfolioWorkflowSettlementRejectsCrossOwnerAndMismatchedReceipt(t *testing.T) {
	svc, repo, item, execution := completedPortfolioWorkflowFixture(t, "verified")
	request := PortfolioWorkflowSettlementRequest{
		WorkflowID: execution.WorkflowID.String(), ExpectedItemDigest: item.RecordDigest,
		ActualEffortMinutes: 3, Confirmation: PortfolioWorkflowSettlementConfirmation,
	}
	if _, err := svc.SettlePortfolioWorkflowForOwner(
		context.Background(), "mallory", "mallory", item.ID, request,
	); err == nil {
		t.Fatal("cross-owner settlement was accepted")
	}

	workflowItem := repo.workflows[execution.WorkflowID]
	workflowItem.SourceID = uuid.NewString()
	repo.workflows[execution.WorkflowID] = workflowItem
	if _, err := svc.SettlePortfolioWorkflowForOwner(
		context.Background(), "alice", "alice", item.ID, request,
	); err == nil || !strings.Contains(err.Error(), "invalid portfolio authorization receipt") {
		t.Fatalf("mismatched receipt error=%v", err)
	}
	if len(repo.resourceSettlements) != 0 {
		t.Fatal("mismatched receipt settled resources")
	}
}

func completedPortfolioWorkflowFixture(
	t *testing.T,
	verificationStatus string,
) (*service, *portfolioAcceptanceFakeRepository, models.PursuitPortfolioExecutionProposalItem, *PortfolioWorkflowEffectExecutionResult) {
	t.Helper()
	svc, repo, item, decision := approvedPortfolioWorkflowFixture(t)
	executor := newPortfolioWorkflowExecutorFake()
	workflowIntake := newPortfolioWorkflowIntakeFake(repo.fakeRepo)
	svc.portfolioWorkflowAuthorizer = executor
	svc.portfolioWorkflowExecutor = executor
	svc.workflowService = workflowIntake
	authorized, err := svc.AuthorizePortfolioWorkflowEffectForOwner(
		context.Background(), "alice", "alice", item.ID,
		PortfolioWorkflowEffectAuthorizationRequest{
			ExpectedItemDigest: item.RecordDigest, ExpectedDecisionDigest: decision.RecordDigest,
			Confirmation: PortfolioWorkflowEffectConfirmation,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	execution, err := svc.ExecutePortfolioWorkflowEffectForOwner(
		context.Background(), "alice", "alice", item.ID,
		PortfolioWorkflowEffectExecutionRequest{
			AuthorizationReceiptID: authorized.Receipt.ID.String(),
			ExpectedItemDigest:     item.RecordDigest, ExpectedDecisionDigest: decision.RecordDigest,
			Confirmation: PortfolioWorkflowEffectExecutionConfirmation,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	workflowItem := repo.workflows[execution.WorkflowID]
	completedAt := time.Now().UTC().Truncate(time.Second)
	workflowItem.CurrentState = workflow.StateCompleted
	workflowItem.CompletedAt = &completedAt
	workflowItem.VerificationStatus = verificationStatus
	workflowItem.LastTaskPlanID = "verified-plan-" + execution.WorkflowID.String()
	repo.workflows[execution.WorkflowID] = workflowItem
	repo.workflowAttestations[execution.WorkflowID] = models.WorkflowCompletionAttestation{
		ID: uuid.New(), WorkflowID: execution.WorkflowID, OwnerIdentity: "alice",
		TaskPlanID: workflowItem.LastTaskPlanID, CompletionStatus: workflow.StateCompleted,
		VerificationStatus: verificationStatus, RuntimeEvidenceURI: "hai://test/runtime/" + execution.WorkflowID.String(),
		RuntimeEvidenceDigest: strings.Repeat("a", 64), ResultDigest: strings.Repeat("b", 64),
		RecordDigest: strings.Repeat("c", 64), CompletedAt: completedAt, CreatedAt: completedAt,
	}
	return svc, repo, item, execution
}
