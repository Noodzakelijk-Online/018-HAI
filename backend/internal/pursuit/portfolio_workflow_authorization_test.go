package pursuit

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/executionauth"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/plangraph"
	"automation-hub-backend/internal/resourceplanner"
	"automation-hub-backend/internal/workflow"

	"github.com/google/uuid"
)

var _ PortfolioWorkflowEffectApprovalRepository = (*GormRepository)(nil)
var _ PortfolioWorkflowEffectApprovalRepository = (*portfolioAcceptanceFakeRepository)(nil)

func TestPortfolioWorkflowEffectAuthorizationIsExactAndNonExecuting(t *testing.T) {
	svc, repo, item, decision := approvedPortfolioWorkflowFixture(t)
	authorizer := &portfolioWorkflowAuthorizerSpy{}
	configured, err := WithPortfolioWorkflowEffectAuthorization(svc, authorizer)
	if err != nil {
		t.Fatal(err)
	}
	svc = configured.(*service)

	workflowCount := len(repo.workflows)
	attemptCount := len(repo.taskAttempts)
	settlementCount := len(repo.resourceSettlements)
	decisionCount := len(repo.executionProposalDecisions[item.ID])
	result, err := svc.AuthorizePortfolioWorkflowEffectForOwner(
		context.Background(), "alice", "alice", item.ID,
		PortfolioWorkflowEffectAuthorizationRequest{
			ExpectedItemDigest: item.RecordDigest, ExpectedDecisionDigest: decision.RecordDigest,
			Confirmation: PortfolioWorkflowEffectConfirmation,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if authorizer.calls != 1 {
		t.Fatalf("authorization calls=%d, want one", authorizer.calls)
	}
	wanted := authorizer.request
	if wanted.OwnerIdentity != "alice" || wanted.ActorIdentity != "alice" ||
		wanted.ActorKind != executionauth.ActorHuman || wanted.Action != PortfolioWorkflowEffectAction ||
		wanted.Stage != executionauth.StageExecution || wanted.ResourceType != PortfolioWorkflowEffectResourceType ||
		wanted.ResourceID != item.ID.String() || wanted.ToolID != PortfolioWorkflowEffectToolID ||
		wanted.RuntimeID != PortfolioWorkflowEffectRuntimeID || wanted.RequestedAutonomy != 6 ||
		wanted.RequiredAuthority != 1 || !wanted.Reversible || wanted.EstimatedCostEUR != 0 ||
		wanted.ApprovalSourceID != PortfolioWorkflowEffectApprovalSourcePrefix+decision.ID.String() ||
		wanted.ApprovalBindingDigest == "" || wanted.ApprovalBindingDigest != wanted.EffectDigest {
		t.Fatalf("authorization request escaped fixed contract: %#v", wanted)
	}
	if result.Authority != PortfolioWorkflowEffectAuthority || result.CanExecute ||
		result.Effect.EffectDigest != wanted.EffectDigest || result.Receipt.ID == uuid.Nil ||
		result.Receipt.Outcome != executionauth.OutcomeAuthorized {
		t.Fatalf("authorization result=%#v", result)
	}
	if len(repo.workflows) != workflowCount || len(repo.taskAttempts) != attemptCount ||
		len(repo.resourceSettlements) != settlementCount ||
		len(repo.executionProposalDecisions[item.ID]) != decisionCount {
		t.Fatal("receipt issuance crossed the execution or persistence boundary")
	}
}

func TestPortfolioWorkflowEffectAuthorizationFailsClosedBeforePolicyEvaluation(t *testing.T) {
	svc, repo, item, decision := approvedPortfolioWorkflowFixture(t)
	authorizer := &portfolioWorkflowAuthorizerSpy{}
	svc.portfolioWorkflowAuthorizer = authorizer
	valid := PortfolioWorkflowEffectAuthorizationRequest{
		ExpectedItemDigest: item.RecordDigest, ExpectedDecisionDigest: decision.RecordDigest,
		Confirmation: PortfolioWorkflowEffectConfirmation,
	}

	tests := []struct {
		name  string
		owner string
		actor string
		input PortfolioWorkflowEffectAuthorizationRequest
	}{
		{"foreign owner", "mallory", "mallory", valid},
		{"actor mismatch", "alice", "mallory", valid},
		{"item digest mismatch", "alice", "alice", withPortfolioItemDigest(valid, strings.Repeat("f", 64))},
		{"decision digest mismatch", "alice", "alice", withPortfolioDecisionDigest(valid, strings.Repeat("e", 64))},
		{"confirmation mismatch", "alice", "alice", withPortfolioConfirmation(valid, "AUTHORIZE")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := svc.AuthorizePortfolioWorkflowEffectForOwner(
				context.Background(), test.owner, test.actor, item.ID, test.input,
			); err == nil {
				t.Fatal("expected fail-closed authorization error")
			}
		})
	}
	if authorizer.calls != 0 {
		t.Fatalf("invalid evidence reached policy evaluator %d times", authorizer.calls)
	}

	changed := repo.pursuits[item.PursuitID]
	changed.NextRecommendedAction = "A materially different next action"
	repo.pursuits[item.PursuitID] = changed
	if _, err := svc.AuthorizePortfolioWorkflowEffectForOwner(
		context.Background(), "alice", "alice", item.ID, valid,
	); err == nil || !strings.Contains(strings.ToLower(err.Error()), "fresh proposal") {
		t.Fatalf("stale pursuit error=%v", err)
	}
	if authorizer.calls != 0 {
		t.Fatal("stale pursuit evidence reached policy evaluator")
	}
}

func TestPortfolioWorkflowEffectAuthorizationRejectsRevokedExpiredAndSettledApproval(t *testing.T) {
	t.Run("revoked", func(t *testing.T) {
		svc, _, item, approved := approvedPortfolioWorkflowFixture(t)
		svc.portfolioWorkflowAuthorizer = &portfolioWorkflowAuthorizerSpy{}
		revoked, err := svc.DecidePortfolioExecutionProposalItemForOwner(
			"alice", "alice", item.ID, PortfolioExecutionProposalDecisionRequest{
				ExpectedItemDigest: item.RecordDigest, Decision: PortfolioExecutionDecisionRevoked,
				Reason:       "Withdraw before a concrete effect is authorized.",
				Confirmation: PortfolioExecutionDecisionRevokeConfirmation,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		_, err = svc.AuthorizePortfolioWorkflowEffectForOwner(
			context.Background(), "alice", "alice", item.ID,
			PortfolioWorkflowEffectAuthorizationRequest{
				ExpectedItemDigest:     item.RecordDigest,
				ExpectedDecisionDigest: revoked.Decision.RecordDigest,
				Confirmation:           PortfolioWorkflowEffectConfirmation,
			},
		)
		if !errors.Is(err, ErrPortfolioWorkflowApprovalUnavailable) {
			t.Fatalf("revoked approval error=%v; approved=%s", err, approved.ID)
		}
	})

	t.Run("expired", func(t *testing.T) {
		svc, repo, item, decision := approvedPortfolioWorkflowFixture(t)
		svc.portfolioWorkflowAuthorizer = &portfolioWorkflowAuthorizerSpy{}
		records := repo.executionProposalDecisions[item.ID]
		expired := time.Now().UTC().Add(-time.Minute)
		records[len(records)-1].ExpiresAt = &expired
		records[len(records)-1].RecordDigest, _ = digestPortfolioExecutionDecision(&records[len(records)-1])
		repo.executionProposalDecisions[item.ID] = records
		_, err := svc.AuthorizePortfolioWorkflowEffectForOwner(
			context.Background(), "alice", "alice", item.ID,
			PortfolioWorkflowEffectAuthorizationRequest{
				ExpectedItemDigest:     item.RecordDigest,
				ExpectedDecisionDigest: records[len(records)-1].RecordDigest,
				Confirmation:           PortfolioWorkflowEffectConfirmation,
			},
		)
		if !errors.Is(err, ErrPortfolioWorkflowApprovalStale) &&
			!errors.Is(err, ErrPortfolioWorkflowApprovalInvalid) {
			t.Fatalf("expired approval error=%v; original=%s", err, decision.ID)
		}
	})

	t.Run("settled", func(t *testing.T) {
		svc, repo, item, decision := approvedPortfolioWorkflowFixture(t)
		svc.portfolioWorkflowAuthorizer = &portfolioWorkflowAuthorizerSpy{}
		repo.resourceSettlements[item.ReservationID] = models.PursuitResourceReservationSettlement{
			ID: uuid.New(), ReservationID: item.ReservationID, PursuitID: item.PursuitID,
			OwnerIdentity: "alice", Disposition: ResourceReservationReleased,
			Actor: "alice", Reason: "Reservation no longer available", SettledAt: time.Now().UTC(),
		}
		_, err := svc.AuthorizePortfolioWorkflowEffectForOwner(
			context.Background(), "alice", "alice", item.ID,
			PortfolioWorkflowEffectAuthorizationRequest{
				ExpectedItemDigest: item.RecordDigest, ExpectedDecisionDigest: decision.RecordDigest,
				Confirmation: PortfolioWorkflowEffectConfirmation,
			},
		)
		if !errors.Is(err, ErrPortfolioWorkflowApprovalInvalid) {
			t.Fatalf("settled approval error=%v", err)
		}
	})
}

func TestPortfolioWorkflowEffectApprovalResolverRevalidatesExactBindingAndOwner(t *testing.T) {
	_, repo, _, decision := approvedPortfolioWorkflowFixture(t)
	resolver, err := NewPortfolioWorkflowEffectApprovalResolver(repo)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := repo.LoadPortfolioWorkflowEffectApprovalSnapshot(context.Background(), "alice", decision.ID)
	if err != nil {
		t.Fatal(err)
	}
	effect, err := buildPortfolioWorkflowEffect(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	approval, err := resolver.Resolve(context.Background(), "alice", effect.ApprovalSource, effect.EffectDigest)
	if err != nil || approval.DecisionID != decision.ID.String() || approval.BindingDigest != effect.EffectDigest {
		t.Fatalf("approval=%#v err=%v", approval, err)
	}
	if _, err := resolver.Resolve(context.Background(), "mallory", effect.ApprovalSource, effect.EffectDigest); !errors.Is(err, ErrPortfolioWorkflowApprovalUnavailable) {
		t.Fatalf("foreign owner error=%v", err)
	}
	if _, err := resolver.Resolve(context.Background(), "alice", effect.ApprovalSource, strings.Repeat("a", 64)); !errors.Is(err, ErrPortfolioWorkflowBindingMismatch) {
		t.Fatalf("binding mismatch error=%v", err)
	}
}

func TestPortfolioWorkflowEffectAuthorizationRevalidatesAndBindsAcceptedPlan(t *testing.T) {
	svc, repo, item, decision, reference, planResolver := approvedPortfolioWorkflowFixtureWithCoordinationPlan(t)
	authorizer := &portfolioWorkflowAuthorizerSpy{}
	svc.portfolioWorkflowAuthorizer = authorizer
	planResolver.calls = 0

	result, err := svc.AuthorizePortfolioWorkflowEffectForOwner(
		context.Background(), "alice", "alice", item.ID,
		PortfolioWorkflowEffectAuthorizationRequest{
			ExpectedItemDigest: item.RecordDigest, ExpectedDecisionDigest: decision.RecordDigest,
			Confirmation: PortfolioWorkflowEffectConfirmation,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if planResolver.calls != 1 {
		t.Fatalf("plan revalidation calls=%d, want one before policy authorization", planResolver.calls)
	}
	if result.CanExecute || result.Effect.CoordinationPlanID != reference.PlanID.String() ||
		result.Effect.CoordinationPlanRevision != reference.Revision ||
		result.Effect.CoordinationPlanDigest != reference.Digest ||
		result.Effect.CoordinationPlanNodeID != reference.NodeID {
		t.Fatalf("plan-bound authorization result=%#v", result)
	}
	wantedURI := portfolioWorkflowCoordinationPlanURI(result.Effect)
	if authorizer.request.Facts["coordinationPlanId"] != reference.PlanID.String() ||
		authorizer.request.Facts["coordinationPlanRevision"] != "1" ||
		authorizer.request.Facts["coordinationPlanDigest"] != reference.Digest ||
		authorizer.request.Facts["coordinationPlanNodeId"] != reference.NodeID ||
		!containsPortfolioString(authorizer.request.SourceReferences, wantedURI) {
		t.Fatalf("authorization plan evidence facts=%#v sources=%#v", authorizer.request.Facts, authorizer.request.SourceReferences)
	}
	resolverWithoutPlan, err := NewPortfolioWorkflowEffectApprovalResolver(repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolverWithoutPlan.Resolve(
		context.Background(), "alice", result.Effect.ApprovalSource, result.Effect.EffectDigest,
	); err == nil || !strings.Contains(err.Error(), "validation is unavailable") {
		t.Fatalf("missing plan resolver error=%v", err)
	}

	resolver, err := NewPortfolioWorkflowEffectApprovalResolver(repo, planResolver)
	if err != nil {
		t.Fatal(err)
	}
	planResolver.calls = 0
	approval, err := resolver.Resolve(
		context.Background(), "alice", result.Effect.ApprovalSource, result.Effect.EffectDigest,
	)
	if err != nil || approval.BindingDigest != result.Effect.EffectDigest || planResolver.calls != 1 {
		t.Fatalf("canonical resolver approval=%#v calls=%d err=%v", approval, planResolver.calls, err)
	}
	planResolver.err = plangraph.ErrReferenceStale
	if _, err := resolver.Resolve(
		context.Background(), "alice", result.Effect.ApprovalSource, result.Effect.EffectDigest,
	); err == nil || !errors.Is(err, plangraph.ErrReferenceStale) {
		t.Fatalf("stale coordination plan resolver error=%v", err)
	}
}

func TestPortfolioWorkflowEffectPreservesLegacyUnboundDigest(t *testing.T) {
	_, repo, _, decision := approvedPortfolioWorkflowFixture(t)
	snapshot, err := repo.LoadPortfolioWorkflowEffectApprovalSnapshot(context.Background(), "alice", decision.ID)
	if err != nil {
		t.Fatal(err)
	}
	effect, err := buildPortfolioWorkflowEffect(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	wanted, err := digestPortfolioPayload(struct {
		ContractVersion                                                            int
		ProposalItemID, ProposalItemDigest, StateDigest                            string
		DecisionID, DecisionDigest, Action, Stage, ResourceType, ResourceID        string
		ProjectKey, Domain, ToolID, RuntimeID, Risk, ActionSummary, ApprovalSource string
		Reversible                                                                 bool
		EstimatedCostMicros                                                        int64
	}{
		1, snapshot.Item.ID.String(), snapshot.Item.RecordDigest, snapshot.Item.StateDigest,
		snapshot.Decision.ID.String(), snapshot.Decision.RecordDigest,
		effect.Action, effect.Stage, effect.ResourceType, effect.ResourceID,
		effect.ProjectKey, effect.Domain, effect.ToolID, effect.RuntimeID, effect.Risk,
		effect.ActionSummary, effect.ApprovalSource, effect.Reversible, effect.EstimatedCost,
	})
	if err != nil {
		t.Fatal(err)
	}
	if effect.CoordinationPlanID != "" || effect.EffectDigest != wanted {
		t.Fatalf("legacy effect=%#v digest=%s, want %s", effect, effect.EffectDigest, wanted)
	}
}

func TestPortfolioWorkflowEffectFirstConsumptionRevalidatesAndPropagatesAcceptedPlan(t *testing.T) {
	svc, repo, item, decision, reference, planResolver := approvedPortfolioWorkflowFixtureWithCoordinationPlan(t)
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
	execute := PortfolioWorkflowEffectExecutionRequest{
		AuthorizationReceiptID: authorized.Receipt.ID.String(),
		ExpectedItemDigest:     item.RecordDigest, ExpectedDecisionDigest: decision.RecordDigest,
		Confirmation: PortfolioWorkflowEffectExecutionConfirmation,
	}
	planResolver.err = plangraph.ErrReferenceStale
	if _, err := svc.ExecutePortfolioWorkflowEffectForOwner(
		context.Background(), "alice", "alice", item.ID, execute,
	); err == nil || !errors.Is(err, plangraph.ErrReferenceStale) {
		t.Fatalf("first consumption stale-plan error=%v", err)
	}
	if executor.consumeCalls != 0 || workflowIntake.createCount != 0 {
		t.Fatalf("stale plan crossed effect boundary: consumes=%d creates=%d", executor.consumeCalls, workflowIntake.createCount)
	}

	planResolver.err = nil
	result, err := svc.ExecutePortfolioWorkflowEffectForOwner(
		context.Background(), "alice", "alice", item.ID, execute,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.CanExecute || executor.consumeCalls != 1 || workflowIntake.createCount != 1 ||
		workflowIntake.last.CoordinationPlan != reference {
		t.Fatalf("plan-bound execution result=%#v intake=%#v", result, workflowIntake.last)
	}
}

func TestPortfolioWorkflowEffectConsumedRecoveryDoesNotRequireLatestAcceptedPlan(t *testing.T) {
	svc, repo, item, decision, _, planResolver := approvedPortfolioWorkflowFixtureWithCoordinationPlan(t)
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
	execute := PortfolioWorkflowEffectExecutionRequest{
		AuthorizationReceiptID: authorized.Receipt.ID.String(),
		ExpectedItemDigest:     item.RecordDigest, ExpectedDecisionDigest: decision.RecordDigest,
		Confirmation: PortfolioWorkflowEffectExecutionConfirmation,
	}
	first, err := svc.ExecutePortfolioWorkflowEffectForOwner(
		context.Background(), "alice", "alice", item.ID, execute,
	)
	if err != nil {
		t.Fatal(err)
	}
	planResolver.err = plangraph.ErrReferenceStale
	planResolver.calls = 0
	recovered, err := svc.ExecutePortfolioWorkflowEffectForOwner(
		context.Background(), "alice", "alice", item.ID, execute,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !recovered.Replayed || recovered.WorkflowID != first.WorkflowID ||
		executor.consumeCalls != 1 || workflowIntake.createCount != 1 || planResolver.calls != 0 {
		t.Fatalf("consumed recovery result=%#v consumes=%d creates=%d planChecks=%d", recovered, executor.consumeCalls, workflowIntake.createCount, planResolver.calls)
	}
}

func TestPortfolioWorkflowEffectExecutionConsumesOnceAndCreatesReviewGatedWorkflow(t *testing.T) {
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
	request := PortfolioWorkflowEffectExecutionRequest{
		AuthorizationReceiptID: authorized.Receipt.ID.String(),
		ExpectedItemDigest:     item.RecordDigest, ExpectedDecisionDigest: decision.RecordDigest,
		Confirmation: PortfolioWorkflowEffectExecutionConfirmation,
	}
	result, err := svc.ExecutePortfolioWorkflowEffectForOwner(
		context.Background(), "alice", "alice", item.ID, request,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Authority != PortfolioWorkflowEffectExecutionAuthority || result.CanExecute ||
		result.Replayed || result.Receipt.ID != authorized.Receipt.ID ||
		result.Consumption.Consumer != PortfolioWorkflowEffectConsumer ||
		result.WorkflowID == uuid.Nil || result.WorkflowState != workflow.StateNeedsApproval {
		t.Fatalf("execution result=%#v", result)
	}
	if executor.consumeCalls != 1 || workflowIntake.createCount != 1 ||
		workflowIntake.last.RequiresReview != true ||
		workflowIntake.last.SourceType != PortfolioWorkflowEffectSourceType ||
		workflowIntake.last.SourceID != authorized.Receipt.ID.String() {
		t.Fatalf("executor=%#v intake=%#v", executor, workflowIntake)
	}
	if len(repo.resourceSettlements) != 0 {
		t.Fatal("workflow creation settled the resource reservation")
	}
	linkCount := len(repo.links)
	activityCount := len(repo.activity[item.PursuitID])

	replayed, err := svc.ExecutePortfolioWorkflowEffectForOwner(
		context.Background(), "alice", "alice", item.ID, request,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || replayed.WorkflowID != result.WorkflowID ||
		executor.consumeCalls != 1 || workflowIntake.createCount != 1 ||
		len(repo.links) != linkCount || len(repo.activity[item.PursuitID]) != activityCount {
		t.Fatalf("replay duplicated effect: result=%#v consumes=%d creates=%d", replayed, executor.consumeCalls, workflowIntake.createCount)
	}
}

func TestPortfolioWorkflowEffectExecutionRecoversMissingAuditWithoutReplayingIntake(t *testing.T) {
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
	request := PortfolioWorkflowEffectExecutionRequest{
		AuthorizationReceiptID: authorized.Receipt.ID.String(),
		ExpectedItemDigest:     item.RecordDigest, ExpectedDecisionDigest: decision.RecordDigest,
		Confirmation: PortfolioWorkflowEffectExecutionConfirmation,
	}
	first, err := svc.ExecutePortfolioWorkflowEffectForOwner(
		context.Background(), "alice", "alice", item.ID, request,
	)
	if err != nil {
		t.Fatal(err)
	}
	retained := make([]models.PursuitActivity, 0, len(repo.activity[item.PursuitID]))
	for _, activity := range repo.activity[item.PursuitID] {
		if activity.EventType != PortfolioWorkflowEffectEvent {
			retained = append(retained, activity)
		}
	}
	repo.activity[item.PursuitID] = retained

	recovered, err := svc.ExecutePortfolioWorkflowEffectForOwner(
		context.Background(), "alice", "alice", item.ID, request,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !recovered.Replayed || recovered.WorkflowID != first.WorkflowID || workflowIntake.createCount != 1 {
		t.Fatalf("recovery replayed intake: result=%#v creates=%d", recovered, workflowIntake.createCount)
	}
	effectEvents := 0
	for _, activity := range repo.activity[item.PursuitID] {
		if activity.EventType == PortfolioWorkflowEffectEvent {
			effectEvents++
		}
	}
	if effectEvents != 1 {
		t.Fatalf("recovered effect audit count=%d, want one", effectEvents)
	}
}

func TestPortfolioWorkflowEffectExecutionRejectsMismatchedOrDifferentlyConsumedReceipt(t *testing.T) {
	t.Run("different receipt", func(t *testing.T) {
		svc, _, item, decision := approvedPortfolioWorkflowFixture(t)
		executor := newPortfolioWorkflowExecutorFake()
		workflowIntake := newPortfolioWorkflowIntakeFake(svc.repo.(*portfolioAcceptanceFakeRepository).fakeRepo)
		svc.portfolioWorkflowAuthorizer = executor
		svc.portfolioWorkflowExecutor = executor
		svc.workflowService = workflowIntake
		_, err := svc.AuthorizePortfolioWorkflowEffectForOwner(
			context.Background(), "alice", "alice", item.ID,
			PortfolioWorkflowEffectAuthorizationRequest{
				ExpectedItemDigest: item.RecordDigest, ExpectedDecisionDigest: decision.RecordDigest,
				Confirmation: PortfolioWorkflowEffectConfirmation,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		_, err = svc.ExecutePortfolioWorkflowEffectForOwner(
			context.Background(), "alice", "alice", item.ID,
			PortfolioWorkflowEffectExecutionRequest{
				AuthorizationReceiptID: uuid.NewString(),
				ExpectedItemDigest:     item.RecordDigest, ExpectedDecisionDigest: decision.RecordDigest,
				Confirmation: PortfolioWorkflowEffectExecutionConfirmation,
			},
		)
		if err == nil || workflowIntake.createCount != 0 {
			t.Fatalf("mismatched receipt error=%v creates=%d", err, workflowIntake.createCount)
		}
	})

	t.Run("different consumption target", func(t *testing.T) {
		svc, _, item, decision := approvedPortfolioWorkflowFixture(t)
		executor := newPortfolioWorkflowExecutorFake()
		workflowIntake := newPortfolioWorkflowIntakeFake(svc.repo.(*portfolioAcceptanceFakeRepository).fakeRepo)
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
		executor.consumption = &executionauth.Consumption{
			ReceiptID: authorized.Receipt.ID, OwnerIdentity: "alice",
			Consumer: "other-consumer", ExecutionTarget: "workflow-intake:" + strings.Repeat("f", 64),
			ReceiptDigest: authorized.Receipt.DecisionDigest, ConsumedAt: time.Now().UTC(),
		}
		_, err = svc.ExecutePortfolioWorkflowEffectForOwner(
			context.Background(), "alice", "alice", item.ID,
			PortfolioWorkflowEffectExecutionRequest{
				AuthorizationReceiptID: authorized.Receipt.ID.String(),
				ExpectedItemDigest:     item.RecordDigest, ExpectedDecisionDigest: decision.RecordDigest,
				Confirmation: PortfolioWorkflowEffectExecutionConfirmation,
			},
		)
		if err == nil || !strings.Contains(err.Error(), "different effect") || workflowIntake.createCount != 0 {
			t.Fatalf("different consumption error=%v creates=%d", err, workflowIntake.createCount)
		}
	})
}

func TestPortfolioWorkflowEffectExecutionRecoversExactConsumedEffectAfterApprovalExpires(t *testing.T) {
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
	target, err := portfolioWorkflowExecutionTarget(authorized.Effect.EffectDigest)
	if err != nil {
		t.Fatal(err)
	}
	executor.consumption = &executionauth.Consumption{
		ReceiptID: authorized.Receipt.ID, OwnerIdentity: "alice",
		Consumer: PortfolioWorkflowEffectConsumer, ExecutionTarget: target,
		ReceiptDigest: authorized.Receipt.DecisionDigest, ConsumedAt: time.Now().UTC(),
	}
	records := repo.executionProposalDecisions[item.ID]
	expired := time.Now().UTC().Add(-time.Minute)
	records[len(records)-1].ExpiresAt = &expired
	repo.executionProposalDecisions[item.ID] = records

	result, err := svc.ExecutePortfolioWorkflowEffectForOwner(
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
	if !result.Replayed || result.WorkflowID == uuid.Nil || workflowIntake.createCount != 1 || executor.consumeCalls != 0 {
		t.Fatalf("recovery result=%#v creates=%d consumes=%d", result, workflowIntake.createCount, executor.consumeCalls)
	}
}

func approvedPortfolioWorkflowFixture(
	t *testing.T,
) (*service, *portfolioAcceptanceFakeRepository, models.PursuitPortfolioExecutionProposalItem, models.PursuitPortfolioExecutionProposalDecision) {
	t.Helper()
	svc, repo, accepted, _ := acceptedExecutionProposalFixture(
		t, "high", "approve_before_execute", "Draft a source-grounded response",
	)
	proposal, err := svc.PreparePortfolioExecutionProposalsForOwner(
		"alice", "alice", accepted.Allocation.ID,
		PortfolioExecutionProposalRequest{
			ExpectedAllocationDigest: accepted.Allocation.RecordDigest,
			Confirmation:             PortfolioExecutionProposalConfirmation,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	item := proposal.Items[0]
	approved, err := svc.DecidePortfolioExecutionProposalItemForOwner(
		"alice", "alice", item.ID, PortfolioExecutionProposalDecisionRequest{
			ExpectedItemDigest: item.RecordDigest, Decision: PortfolioExecutionDecisionApproved,
			Reason:       "Owner reviewed this exact immutable action.",
			Confirmation: PortfolioExecutionDecisionApproveConfirmation,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return svc, repo, item, *approved.Decision
}

type mutablePortfolioAcceptedPlanResolver struct {
	owner     string
	reference plangraph.AcceptedRevisionReference
	binding   plangraph.AcceptedRevisionBinding
	err       error
	calls     int
}

func (r *mutablePortfolioAcceptedPlanResolver) ResolveAccepted(
	ctx context.Context,
	ownerIdentity string,
	reference plangraph.AcceptedRevisionReference,
) (*plangraph.AcceptedRevisionBinding, error) {
	r.calls++
	if ctx == nil {
		return nil, errors.New("context is required")
	}
	if r.err != nil {
		return nil, r.err
	}
	if ownerIdentity != r.owner || reference != r.reference {
		return nil, plangraph.ErrReferenceInvalid
	}
	binding := r.binding
	binding.Nodes = append([]plangraph.Node(nil), r.binding.Nodes...)
	return &binding, nil
}

func approvedPortfolioWorkflowFixtureWithCoordinationPlan(
	t *testing.T,
) (*service, *portfolioAcceptanceFakeRepository, models.PursuitPortfolioExecutionProposalItem, models.PursuitPortfolioExecutionProposalDecision, plangraph.AcceptedRevisionReference, *mutablePortfolioAcceptedPlanResolver) {
	t.Helper()
	repo := newPortfolioAcceptanceFakeRepository()
	svc := &service{repo: repo}
	pursuit, err := svc.Create(CreateRequest{
		OwnerIdentity: "alice", Title: "Plan-bound execution proposal fixture",
		RiskLevel: "high", AutonomyLevel: "approve_before_execute",
		NextRecommendedAction: "Draft a source-grounded response",
		ResourceLimits:        models.PursuitResourceLimits{MaxEffortHours: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	reference := plangraph.AcceptedRevisionReference{
		PlanID: uuid.New(), Revision: 1, Digest: strings.Repeat("c", 64),
		NodeID: "pursuit-" + pursuit.ID.String(),
	}
	planResolver := &mutablePortfolioAcceptedPlanResolver{
		owner: "alice", reference: reference,
		binding: plangraph.AcceptedRevisionBinding{
			PlanID: reference.PlanID, Revision: reference.Revision, Digest: reference.Digest,
			NodeID: reference.NodeID, AcceptedAt: time.Now().UTC(), CanExecute: false,
			Node:  plangraph.Node{ID: reference.NodeID, Bindings: plangraph.Bindings{PursuitID: pursuit.ID.String()}},
			Nodes: []plangraph.Node{{ID: reference.NodeID, Bindings: plangraph.Bindings{PursuitID: pursuit.ID.String()}}},
		},
	}
	svc.acceptedPlanResolver = planResolver
	now := time.Now().UTC().Truncate(time.Minute)
	planning := PortfolioPlanningRequest{
		PlanID: "plan-bound-execution-" + uuid.NewString(), AsOf: now,
		HorizonStart: now, HorizonEnd: now.Add(time.Hour),
		DurationMode: resourceplanner.ExpectedDuration,
		Availability: []PortfolioCapacityWindow{{Start: now, End: now.Add(time.Hour)}},
		Pursuits: []PortfolioPursuitPlanningInput{{
			PursuitID: pursuit.ID, Duration: portfolioDuration(10, 20, 30), Factors: portfolioFactors(75),
		}},
		Budget:           resourceplanner.Budget{MaxCostMicros: portfolioInt64(0)},
		CoordinationPlan: reference,
	}
	planned, err := svc.PlanPortfolioForOwner("alice", planning)
	if err != nil || planned.Decision == nil {
		t.Fatalf("plan=%#v err=%v", planned, err)
	}
	accepted, err := svc.AcceptPortfolioAllocationForOwner("alice", "alice", PortfolioAllocationAcceptanceRequest{
		PlanningRequest: planning, ExpectedDecisionDigest: planned.Decision.DecisionDigest,
		Confirmation: PortfolioAllocationConfirmation,
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := svc.PreparePortfolioExecutionProposalsForOwner(
		"alice", "alice", accepted.Allocation.ID,
		PortfolioExecutionProposalRequest{
			ExpectedAllocationDigest: accepted.Allocation.RecordDigest,
			Confirmation:             PortfolioExecutionProposalConfirmation,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	item := proposal.Items[0]
	approved, err := svc.DecidePortfolioExecutionProposalItemForOwner(
		"alice", "alice", item.ID, PortfolioExecutionProposalDecisionRequest{
			ExpectedItemDigest: item.RecordDigest, Decision: PortfolioExecutionDecisionApproved,
			Reason:       "Owner reviewed this exact immutable plan-bound action.",
			Confirmation: PortfolioExecutionDecisionApproveConfirmation,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	planResolver.calls = 0
	return svc, repo, item, *approved.Decision, reference, planResolver
}

func containsPortfolioString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func (r *portfolioAcceptanceFakeRepository) LoadPortfolioWorkflowEffectApprovalSnapshot(
	ctx context.Context,
	ownerIdentity string,
	decisionID uuid.UUID,
) (*PortfolioWorkflowEffectApprovalSnapshot, error) {
	if ctx == nil {
		return nil, errors.New("context is required")
	}
	for itemID, records := range r.executionProposalDecisions {
		for _, decision := range records {
			if decision.ID != decisionID || decision.OwnerIdentity != ownerIdentity {
				continue
			}
			snapshot, err := r.LoadPortfolioExecutionProposalDecisionSnapshot(ownerIdentity, itemID)
			if err != nil || snapshot == nil {
				return nil, err
			}
			copyDecision := decision
			return &PortfolioWorkflowEffectApprovalSnapshot{
				Allocation: snapshot.Allocation, Proposal: snapshot.Proposal, Item: snapshot.Item,
				AllocationItem: snapshot.AllocationItem, Pursuit: snapshot.Pursuit,
				Decision: copyDecision, LatestDecision: snapshot.LatestDecision,
				Settled: snapshot.Settled,
			}, nil
		}
	}
	return nil, nil
}

type portfolioWorkflowAuthorizerSpy struct {
	request executionauth.Request
	calls   int
	err     error
}

type portfolioWorkflowExecutorFake struct {
	request        executionauth.Request
	receipt        executionauth.Receipt
	consumption    *executionauth.Consumption
	authorizeCalls int
	consumeCalls   int
}

func newPortfolioWorkflowExecutorFake() *portfolioWorkflowExecutorFake {
	return &portfolioWorkflowExecutorFake{}
}

func (f *portfolioWorkflowExecutorFake) Authorize(
	_ context.Context,
	request executionauth.Request,
) (executionauth.Receipt, error) {
	f.authorizeCalls++
	f.request = request
	if f.receipt.ID == uuid.Nil {
		f.receipt = executionauth.Receipt{
			ID: uuid.New(), ContractVersion: executionauth.ContractVersion,
			OwnerIdentity: request.OwnerIdentity, IdempotencyKey: request.IdempotencyKey,
			ActorIdentity: request.ActorIdentity, ActorKind: request.ActorKind,
			TaskID: request.TaskID, Action: request.Action, Stage: request.Stage,
			ResourceType: request.ResourceType, ResourceID: request.ResourceID,
			ProjectKey: request.ProjectKey, Domain: request.Domain, RuntimeID: request.RuntimeID,
			ApprovalSourceID: request.ApprovalSourceID, EffectDigest: request.EffectDigest,
			Outcome: executionauth.OutcomeAuthorized, Reason: "all policy boundaries passed",
			RequestDigest: strings.Repeat("a", 64), DecisionDigest: strings.Repeat("b", 64),
			RequiredAuthority: request.RequiredAuthority, RequestedAutonomy: request.RequestedAutonomy,
			EffectiveAutonomy: request.RequestedAutonomy, Risk: request.Risk,
			Reversible: request.Reversible, EstimatedCostEUR: request.EstimatedCostEUR,
			EvaluatedAt: time.Now().UTC(),
		}
	}
	return f.receipt, nil
}

func (f *portfolioWorkflowExecutorFake) AuthorizeAndConsume(
	ctx context.Context,
	request executionauth.Request,
	consumer, target string,
) (executionauth.Receipt, error) {
	f.consumeCalls++
	receipt, err := f.Authorize(ctx, request)
	if err != nil {
		return executionauth.Receipt{}, err
	}
	if f.consumption != nil {
		return receipt, executionauth.ErrAlreadyConsumed
	}
	f.consumption = &executionauth.Consumption{
		ReceiptID: receipt.ID, OwnerIdentity: receipt.OwnerIdentity,
		Consumer: consumer, ExecutionTarget: target,
		ReceiptDigest: receipt.DecisionDigest, ConsumedAt: time.Now().UTC(),
	}
	return receipt, nil
}

func (f *portfolioWorkflowExecutorFake) Get(
	_ context.Context,
	owner string,
	id uuid.UUID,
) (executionauth.Receipt, error) {
	if f.receipt.ID != id || f.receipt.OwnerIdentity != owner {
		return executionauth.Receipt{}, executionauth.ErrNotFound
	}
	return f.receipt, nil
}

func (f *portfolioWorkflowExecutorFake) GetConsumption(
	_ context.Context,
	owner string,
	id uuid.UUID,
) (executionauth.Consumption, error) {
	if f.consumption == nil || f.consumption.OwnerIdentity != owner || f.consumption.ReceiptID != id {
		return executionauth.Consumption{}, executionauth.ErrNotFound
	}
	return *f.consumption, nil
}

type portfolioWorkflowIntakeFake struct {
	last        workflow.IntakeRequest
	record      *workflow.WorkflowRecord
	createCount int
	repo        *fakeRepo
}

func newPortfolioWorkflowIntakeFake(repo *fakeRepo) *portfolioWorkflowIntakeFake {
	return &portfolioWorkflowIntakeFake{repo: repo}
}

func (f *portfolioWorkflowIntakeFake) Intake(request workflow.IntakeRequest) (*workflow.WorkflowRecord, error) {
	f.last = request
	if f.record != nil {
		return f.record, nil
	}
	f.createCount++
	f.record = &workflow.WorkflowRecord{Item: models.WorkflowItem{
		ID: uuid.New(), OwnerIdentity: request.OwnerIdentity,
		Title: request.Input, Description: request.Input, ProjectKey: request.ProjectKey,
		CurrentState: workflow.StateNeedsApproval, RiskLevel: "high",
		RequiresApproval: true, ApprovalStatus: "pending",
		SourceType: request.SourceType, SourceID: request.SourceID, SourceURI: request.SourceURI,
	}}
	if f.repo != nil {
		f.repo.workflows[f.record.Item.ID] = f.record.Item
	}
	return f.record, nil
}

func (s *portfolioWorkflowAuthorizerSpy) Authorize(
	_ context.Context,
	request executionauth.Request,
) (executionauth.Receipt, error) {
	s.calls++
	s.request = request
	if s.err != nil {
		return executionauth.Receipt{}, s.err
	}
	return executionauth.Receipt{
		ID: uuid.New(), ContractVersion: executionauth.ContractVersion,
		OwnerIdentity: request.OwnerIdentity, IdempotencyKey: request.IdempotencyKey,
		ActorIdentity: request.ActorIdentity, ActorKind: request.ActorKind,
		TaskID: request.TaskID, Action: request.Action, Stage: request.Stage,
		ResourceType: request.ResourceType, ResourceID: request.ResourceID,
		ProjectKey: request.ProjectKey, Domain: request.Domain, RuntimeID: request.RuntimeID,
		ApprovalSourceID: request.ApprovalSourceID, EffectDigest: request.EffectDigest,
		Outcome: executionauth.OutcomeAuthorized, Reason: "all policy boundaries passed",
		RequiredAuthority: request.RequiredAuthority, RequestedAutonomy: request.RequestedAutonomy,
		EffectiveAutonomy: request.RequestedAutonomy, Risk: request.Risk,
		Reversible: request.Reversible, EstimatedCostEUR: request.EstimatedCostEUR,
		EvaluatedAt: time.Now().UTC(),
	}, nil
}

func withPortfolioItemDigest(
	request PortfolioWorkflowEffectAuthorizationRequest,
	digest string,
) PortfolioWorkflowEffectAuthorizationRequest {
	request.ExpectedItemDigest = digest
	return request
}

func withPortfolioDecisionDigest(
	request PortfolioWorkflowEffectAuthorizationRequest,
	digest string,
) PortfolioWorkflowEffectAuthorizationRequest {
	request.ExpectedDecisionDigest = digest
	return request
}

func withPortfolioConfirmation(
	request PortfolioWorkflowEffectAuthorizationRequest,
	confirmation string,
) PortfolioWorkflowEffectAuthorizationRequest {
	request.Confirmation = confirmation
	return request
}
