package pursuit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"automation-hub-backend/internal/executionauth"
	"automation-hub-backend/internal/identity"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/plangraph"
	"automation-hub-backend/internal/rbac"
	"automation-hub-backend/internal/workflow"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var _ pursuitPortfolioDispatchRepository = (*GormRepository)(nil)
var _ pursuitPortfolioDispatchRepository = (*portfolioDispatchFakeRepository)(nil)
var _ pursuitPortfolioDispatchCoordinationRepository = (*GormRepository)(nil)
var _ pursuitPortfolioDispatchCoordinationRepository = (*portfolioDispatchFakeRepository)(nil)

func TestPortfolioDispatchCoordinatesApprovedItemsWithoutGrantingDownstreamAuthority(t *testing.T) {
	svc, baseRepo, item, decision := approvedPortfolioWorkflowFixture(t)
	repo := newPortfolioDispatchFakeRepository(baseRepo)
	svc.repo = repo
	executor := newPortfolioWorkflowExecutorFake()
	intake := newPortfolioWorkflowIntakeFake(baseRepo.fakeRepo)
	svc.portfolioWorkflowAuthorizer = executor
	svc.portfolioWorkflowExecutor = executor
	svc.workflowService = intake

	preview, err := svc.PortfolioDispatchCoordinationForOwner(context.Background(), "alice", item.ProposalID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Authority != PortfolioCoordinationAuthority || preview.CanExecute || preview.Eligible != 1 ||
		len(preview.Items) != 1 || !preview.Items[0].Selectable || preview.Items[0].Decision == nil ||
		preview.Items[0].Decision.ID != decision.ID {
		t.Fatalf("coordination preview=%#v", preview)
	}
	proposal := repo.proposalByID(item.ProposalID)
	request := PortfolioDispatchRequest{
		ExpectedProposalDigest: proposal.RecordDigest,
		Items: []PortfolioDispatchItemRequest{{
			ProposalItemID: item.ID.String(), ExpectedItemDigest: item.RecordDigest,
			ExpectedDecisionDigest: decision.RecordDigest,
		}},
		Confirmation: PortfolioDispatchConfirmation,
	}
	result, err := svc.DispatchPortfolioWorkflowsForOwner(
		context.Background(), "alice", "alice", item.ProposalID, request,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Authority != PortfolioDispatchAuthority || result.CanExecute || result.Resumed ||
		result.Status != "workflows_created" || result.Created != 1 || len(result.Items) != 1 ||
		result.Items[0].Outcome != PortfolioDispatchOutcomeWorkflowCreated ||
		result.Items[0].WorkflowID == nil || result.Items[0].WorkflowState != workflow.StateNeedsApproval {
		t.Fatalf("dispatch result=%#v", result)
	}
	if intake.createCount != 1 || executor.consumeCalls != 1 || len(repo.dispatchRuns) != 1 || len(repo.dispatchResults[result.Run.ID]) != 1 {
		t.Fatalf("intake=%d consume=%d runs=%d results=%d", intake.createCount, executor.consumeCalls, len(repo.dispatchRuns), len(repo.dispatchResults[result.Run.ID]))
	}

	replay, err := svc.DispatchPortfolioWorkflowsForOwner(
		context.Background(), "alice", "alice", item.ProposalID, request,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Resumed || len(replay.Items) != 1 || replay.Items[0].ID != result.Items[0].ID ||
		intake.createCount != 1 || executor.consumeCalls != 1 || len(repo.dispatchRuns) != 1 ||
		len(repo.dispatchResults[result.Run.ID]) != 1 {
		t.Fatalf("exact replay crossed idempotency boundary: %#v", replay)
	}
}

func TestPortfolioDispatchRevalidatesAcceptedPlanBeforeNonTerminalDispatch(t *testing.T) {
	svc, baseRepo, item, decision, planState := boundApprovedPortfolioDispatchFixture(t)
	repo := newPortfolioDispatchFakeRepository(baseRepo)
	svc.repo = repo
	executor := newPortfolioWorkflowExecutorFake()
	intake := newPortfolioWorkflowIntakeFake(baseRepo.fakeRepo)
	svc.portfolioWorkflowAuthorizer = executor
	svc.portfolioWorkflowExecutor = executor
	svc.workflowService = intake
	planState.stale = true

	proposal := repo.proposalByID(item.ProposalID)
	result, err := svc.DispatchPortfolioWorkflowsForOwner(
		context.Background(), "alice", "alice", item.ProposalID,
		PortfolioDispatchRequest{
			ExpectedProposalDigest: proposal.RecordDigest,
			Items: []PortfolioDispatchItemRequest{{
				ProposalItemID: item.ID.String(), ExpectedItemDigest: item.RecordDigest,
				ExpectedDecisionDigest: decision.RecordDigest,
			}},
			Confirmation: PortfolioDispatchConfirmation,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if planState.calls != 1 || result.Status != "needs_review" || result.NeedsReview != 1 ||
		len(result.Items) != 1 || result.Items[0].Outcome != PortfolioDispatchOutcomeStale ||
		!strings.Contains(result.Items[0].Message, "accepted revision is stale") ||
		executor.authorizeCalls != 0 || executor.consumeCalls != 0 || intake.createCount != 0 {
		t.Fatalf("stale coordination dispatch escaped fail-closed boundary: result=%#v calls=%d", result, planState.calls)
	}
}

func TestPortfolioDispatchTerminalReplaySurvivesLaterReplan(t *testing.T) {
	svc, baseRepo, item, decision, planState := boundApprovedPortfolioDispatchFixture(t)
	repo := newPortfolioDispatchFakeRepository(baseRepo)
	svc.repo = repo
	executor := newPortfolioWorkflowExecutorFake()
	intake := newPortfolioWorkflowIntakeFake(baseRepo.fakeRepo)
	svc.portfolioWorkflowAuthorizer = executor
	svc.portfolioWorkflowExecutor = executor
	svc.workflowService = intake
	proposal := repo.proposalByID(item.ProposalID)
	request := PortfolioDispatchRequest{
		ExpectedProposalDigest: proposal.RecordDigest,
		Items: []PortfolioDispatchItemRequest{{
			ProposalItemID: item.ID.String(), ExpectedItemDigest: item.RecordDigest,
			ExpectedDecisionDigest: decision.RecordDigest,
		}},
		Confirmation: PortfolioDispatchConfirmation,
	}

	created, err := svc.DispatchPortfolioWorkflowsForOwner(
		context.Background(), "alice", "alice", item.ProposalID, request,
	)
	if err != nil {
		t.Fatal(err)
	}
	if planState.calls < 1 || created.Created != 1 || created.Items[0].Outcome != PortfolioDispatchOutcomeWorkflowCreated {
		t.Fatalf("initial coordinated dispatch=%#v calls=%d", created, planState.calls)
	}
	initialCalls := planState.calls
	planState.stale = true
	replay, err := svc.DispatchPortfolioWorkflowsForOwner(
		context.Background(), "alice", "alice", item.ProposalID, request,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Resumed || replay.Items[0].ID != created.Items[0].ID ||
		planState.calls != initialCalls || executor.consumeCalls != 1 || intake.createCount != 1 ||
		len(repo.dispatchResults[created.Run.ID]) != 1 {
		t.Fatalf("terminal dispatch was not durably replayable after replan: replay=%#v calls=%d", replay, planState.calls)
	}
}

func TestPortfolioDispatchBatchCoordinationRestoresCurrentEvidenceWithoutAuthority(t *testing.T) {
	svc, baseRepo, item, decision := approvedPortfolioWorkflowFixture(t)
	repo := newPortfolioDispatchFakeRepository(baseRepo)
	svc.repo = repo

	results, err := svc.PortfolioDispatchCoordinationBatchForOwner(
		context.Background(), "alice", []uuid.UUID{item.ProposalID},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Proposal.ID != item.ProposalID ||
		results[0].Authority != PortfolioCoordinationAuthority || results[0].CanExecute ||
		results[0].Eligible != 1 || len(results[0].Items) != 1 ||
		!results[0].Items[0].Selectable || results[0].Items[0].Decision == nil ||
		results[0].Items[0].Decision.ID != decision.ID ||
		results[0].Freshness.Status != PortfolioCoordinationFreshnessCurrent ||
		!results[0].Freshness.RevalidationRequired || results[0].Freshness.CheckedAt.IsZero() {
		t.Fatalf("batch coordination=%#v", results)
	}
	if _, err := svc.PortfolioDispatchCoordinationBatchForOwner(
		context.Background(), "alice", []uuid.UUID{item.ProposalID, item.ProposalID},
	); err == nil || !strings.Contains(err.Error(), "unique") {
		t.Fatalf("duplicate proposal ids error=%v", err)
	}
	if _, err := svc.PortfolioDispatchCoordinationBatchForOwner(
		context.Background(), "alice", []uuid.UUID{item.ProposalID, uuid.New()},
	); err == nil || !strings.Contains(err.Error(), "unavailable to this owner") {
		t.Fatalf("mixed owner scope error=%v", err)
	}
	if len(repo.dispatchRuns) != 0 || len(repo.dispatchResults) != 0 {
		t.Fatal("read-only batch coordination mutated dispatch history")
	}
}

func TestPortfolioDispatchNeverCreatesApprovalAuthority(t *testing.T) {
	svc, baseRepo, accepted, _ := acceptedExecutionProposalFixture(
		t, "high", "approve_before_execute", "Draft a source-grounded response",
	)
	proposalResult, err := svc.PreparePortfolioExecutionProposalsForOwner(
		"alice", "alice", accepted.Allocation.ID,
		PortfolioExecutionProposalRequest{
			ExpectedAllocationDigest: accepted.Allocation.RecordDigest,
			Confirmation:             PortfolioExecutionProposalConfirmation,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	item := proposalResult.Items[0]
	repo := newPortfolioDispatchFakeRepository(baseRepo)
	svc.repo = repo
	executor := newPortfolioWorkflowExecutorFake()
	intake := newPortfolioWorkflowIntakeFake(baseRepo.fakeRepo)
	svc.portfolioWorkflowAuthorizer = executor
	svc.portfolioWorkflowExecutor = executor
	svc.workflowService = intake

	result, err := svc.DispatchPortfolioWorkflowsForOwner(
		context.Background(), "alice", "alice", item.ProposalID,
		PortfolioDispatchRequest{
			ExpectedProposalDigest: proposalResult.Proposal.RecordDigest,
			Items: []PortfolioDispatchItemRequest{{
				ProposalItemID: item.ID.String(), ExpectedItemDigest: item.RecordDigest,
				ExpectedDecisionDigest: strings.Repeat("a", 64),
			}},
			Confirmation: PortfolioDispatchConfirmation,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "needs_review" || result.NeedsReview != 1 ||
		result.Items[0].Outcome != PortfolioDispatchOutcomeNeedsApproval ||
		executor.authorizeCalls != 0 || executor.consumeCalls != 0 || intake.createCount != 0 ||
		len(baseRepo.executionProposalDecisions[item.ID]) != 0 {
		t.Fatalf("unapproved dispatch escaped review boundary: %#v", result)
	}
}

func TestPortfolioDispatchRecordsPolicyDenialWithoutClaimingAReceiptOrWorkflow(t *testing.T) {
	svc, baseRepo, item, decision := approvedPortfolioWorkflowFixture(t)
	repo := newPortfolioDispatchFakeRepository(baseRepo)
	svc.repo = repo
	executor := newPortfolioWorkflowExecutorFake()
	svc.portfolioWorkflowAuthorizer = portfolioDispatchDeniedAuthorizer{delegate: executor}
	svc.portfolioWorkflowExecutor = executor
	svc.workflowService = newPortfolioWorkflowIntakeFake(baseRepo.fakeRepo)
	proposal := repo.proposalByID(item.ProposalID)

	result, err := svc.DispatchPortfolioWorkflowsForOwner(
		context.Background(), "alice", "alice", item.ProposalID,
		PortfolioDispatchRequest{
			ExpectedProposalDigest: proposal.RecordDigest,
			Items: []PortfolioDispatchItemRequest{{
				ProposalItemID: item.ID.String(), ExpectedItemDigest: item.RecordDigest,
				ExpectedDecisionDigest: decision.RecordDigest,
			}},
			Confirmation: PortfolioDispatchConfirmation,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "partial_failure" || result.Failed != 1 ||
		result.Items[0].Outcome != PortfolioDispatchOutcomeFailed ||
		result.Items[0].AuthorizationReceiptID != nil || result.Items[0].WorkflowID != nil ||
		executor.consumeCalls != 0 {
		t.Fatalf("denied authorization was represented as executable: %#v", result)
	}
}

func TestPortfolioDispatchFailsClosedBeforePersistence(t *testing.T) {
	svc, baseRepo, item, decision := approvedPortfolioWorkflowFixture(t)
	repo := newPortfolioDispatchFakeRepository(baseRepo)
	svc.repo = repo
	proposal := repo.proposalByID(item.ProposalID)
	valid := PortfolioDispatchRequest{
		ExpectedProposalDigest: proposal.RecordDigest,
		Items: []PortfolioDispatchItemRequest{{
			ProposalItemID: item.ID.String(), ExpectedItemDigest: item.RecordDigest,
			ExpectedDecisionDigest: decision.RecordDigest,
		}},
		Confirmation: PortfolioDispatchConfirmation,
	}
	tests := []struct {
		name    string
		owner   string
		actor   string
		request PortfolioDispatchRequest
	}{
		{"foreign owner", "mallory", "mallory", valid},
		{"actor mismatch", "alice", "mallory", valid},
		{"wrong confirmation", "alice", "alice", withDispatchConfirmation(valid, "DISPATCH")},
		{"wrong proposal digest", "alice", "alice", withDispatchProposalDigest(valid, strings.Repeat("f", 64))},
		{"empty selection", "alice", "alice", withDispatchItems(valid, nil)},
		{"duplicate selection", "alice", "alice", withDispatchItems(valid, []PortfolioDispatchItemRequest{valid.Items[0], valid.Items[0]})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := svc.DispatchPortfolioWorkflowsForOwner(
				context.Background(), test.owner, test.actor, item.ProposalID, test.request,
			); err == nil {
				t.Fatal("expected fail-closed dispatch error")
			}
		})
	}
	if len(repo.dispatchRuns) != 0 || len(repo.dispatchResults) != 0 {
		t.Fatal("invalid dispatch evidence reached durable storage")
	}
}

func TestPortfolioDispatchHandlerRequiresOwnerApprovalAndVerifiedIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, baseRepo, item, decision := approvedPortfolioWorkflowFixture(t)
	repo := newPortfolioDispatchFakeRepository(baseRepo)
	svc.repo = repo
	executor := newPortfolioWorkflowExecutorFake()
	svc.portfolioWorkflowAuthorizer = executor
	svc.portfolioWorkflowExecutor = executor
	svc.workflowService = newPortfolioWorkflowIntakeFake(baseRepo.fakeRepo)
	proposal := repo.proposalByID(item.ProposalID)
	payload, err := json.Marshal(PortfolioDispatchRequest{
		ExpectedProposalDigest: proposal.RecordDigest,
		Items: []PortfolioDispatchItemRequest{{
			ProposalItemID: item.ID.String(), ExpectedItemDigest: item.RecordDigest,
			ExpectedDecisionDigest: decision.RecordDigest,
		}},
		Confirmation: PortfolioDispatchConfirmation,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name       string
		subject    string
		role       rbac.Role
		wantStatus int
	}{
		{"owner", "alice", rbac.RoleOwner, http.StatusCreated},
		{"viewer", "alice", rbac.RoleViewer, http.StatusForbidden},
		{"missing identity", "", rbac.RoleOwner, http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				if test.subject != "" {
					c.Set(identity.ContextSubjectKey, test.subject)
				}
				c.Set(identity.ContextRoleKey, string(test.role))
				c.Next()
			})
			router.POST("/pursuits/portfolio-execution-proposals/:proposalId/dispatch", NewHandler(svc).DispatchPortfolioWorkflows)
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost,
				"/pursuits/portfolio-execution-proposals/"+item.ProposalID.String()+"/dispatch",
				strings.NewReader(string(payload)))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status=%d body=%s want=%d", response.Code, response.Body.String(), test.wantStatus)
			}
		})
	}
}

func TestPortfolioDispatchCoordinationBatchHandlerIsOwnerScopedAndReadOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, baseRepo, item, _ := approvedPortfolioWorkflowFixture(t)
	repo := newPortfolioDispatchFakeRepository(baseRepo)
	svc.repo = repo
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "alice")
		c.Set(identity.ContextRoleKey, string(rbac.RoleOwner))
		c.Next()
	})
	router.GET("/pursuits/portfolio-execution-proposals/coordination", NewHandler(svc).PortfolioDispatchCoordinationBatch)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet,
		"/pursuits/portfolio-execution-proposals/coordination?proposalIds="+item.ProposalID.String(), nil)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"authority":"coordination_preview_only"`) ||
		!strings.Contains(response.Body.String(), `"canExecute":false`) ||
		!strings.Contains(response.Body.String(), `"dispatchRuns":[]`) ||
		!strings.Contains(response.Body.String(), `"status":"current_coordination_snapshot"`) ||
		!strings.Contains(response.Body.String(), `"revalidationRequired":true`) {
		t.Fatalf("batch coordination handler status=%d body=%s", response.Code, response.Body.String())
	}

	duplicate := httptest.NewRecorder()
	duplicateRequest := httptest.NewRequest(http.MethodGet,
		"/pursuits/portfolio-execution-proposals/coordination?proposalIds="+item.ProposalID.String()+","+item.ProposalID.String(), nil)
	router.ServeHTTP(duplicate, duplicateRequest)
	if duplicate.Code != http.StatusBadRequest || !strings.Contains(duplicate.Body.String(), "unique") {
		t.Fatalf("duplicate batch status=%d body=%s", duplicate.Code, duplicate.Body.String())
	}

	acceptedIDs := make([]string, 0, PortfolioDispatchMaxItems)
	for range PortfolioDispatchMaxItems {
		acceptedIDs = append(acceptedIDs, uuid.NewString())
	}
	acceptedBoundary := httptest.NewRecorder()
	acceptedBoundaryRequest := httptest.NewRequest(http.MethodGet,
		"/pursuits/portfolio-execution-proposals/coordination?proposalIds="+strings.Join(acceptedIDs, ","), nil)
	router.ServeHTTP(acceptedBoundary, acceptedBoundaryRequest)
	if acceptedBoundary.Code != http.StatusNotFound {
		t.Fatalf("maximum-size batch status=%d body=%s, want owner-scoped lookup 404", acceptedBoundary.Code, acceptedBoundary.Body.String())
	}

	rejectedIDs := append(append([]string{}, acceptedIDs...), uuid.NewString())
	rejectedBoundary := httptest.NewRecorder()
	rejectedBoundaryRequest := httptest.NewRequest(http.MethodGet,
		"/pursuits/portfolio-execution-proposals/coordination?proposalIds="+strings.Join(rejectedIDs, ","), nil)
	router.ServeHTTP(rejectedBoundary, rejectedBoundaryRequest)
	if rejectedBoundary.Code != http.StatusBadRequest || !strings.Contains(rejectedBoundary.Body.String(), "between 1 and 20") {
		t.Fatalf("oversized batch status=%d body=%s", rejectedBoundary.Code, rejectedBoundary.Body.String())
	}
	if len(repo.dispatchRuns) != 0 || len(repo.dispatchResults) != 0 {
		t.Fatal("batch handler mutated dispatch state")
	}
}

type portfolioDispatchFakeRepository struct {
	*portfolioAcceptanceFakeRepository
	dispatchRuns    map[string]models.PursuitPortfolioDispatchRun
	dispatchResults map[uuid.UUID][]models.PursuitPortfolioDispatchItemResult
}

type portfolioDispatchDeniedAuthorizer struct {
	delegate *portfolioWorkflowExecutorFake
}

type portfolioDispatchPlanState struct {
	reference plangraph.AcceptedRevisionReference
	pursuitID uuid.UUID
	stale     bool
	calls     int
}

func (s *portfolioDispatchPlanState) ResolveAccepted(
	_ context.Context,
	ownerIdentity string,
	reference plangraph.AcceptedRevisionReference,
) (*plangraph.AcceptedRevisionBinding, error) {
	s.calls++
	if ownerIdentity != "alice" || reference != s.reference {
		return nil, fmt.Errorf("unexpected coordination plan scope")
	}
	if s.stale {
		return nil, fmt.Errorf("accepted revision is stale")
	}
	return &plangraph.AcceptedRevisionBinding{
		PlanID: reference.PlanID, Revision: reference.Revision, Digest: reference.Digest,
		NodeID: reference.NodeID, Node: plangraph.Node{ID: reference.NodeID},
		Nodes: []plangraph.Node{{
			ID: "pursuit-node", Bindings: plangraph.Bindings{PursuitID: s.pursuitID.String()},
		}},
		CanExecute: false,
	}, nil
}

func boundApprovedPortfolioDispatchFixture(
	t *testing.T,
) (*service, *portfolioAcceptanceFakeRepository, models.PursuitPortfolioExecutionProposalItem, models.PursuitPortfolioExecutionProposalDecision, *portfolioDispatchPlanState) {
	t.Helper()
	svc, repo, accepted, pursuit := acceptedExecutionProposalFixture(
		t, "high", "approve_before_execute", "Draft a source-grounded response",
	)
	reference := plangraph.AcceptedRevisionReference{
		PlanID: uuid.New(), Revision: 7, Digest: strings.Repeat("7", 64), NodeID: "portfolio-dispatch",
	}
	allocation := *accepted.Allocation
	allocation.CoordinationPlanID = &reference.PlanID
	allocation.CoordinationPlanRevision = reference.Revision
	allocation.CoordinationPlanDigest = reference.Digest
	allocation.CoordinationPlanNodeID = reference.NodeID
	var err error
	allocation.RecordDigest, err = digestPortfolioAllocation(&allocation)
	if err != nil {
		t.Fatal(err)
	}
	repo.portfolioAllocations[allocation.OwnerIdentity+":"+allocation.PlanID] = allocation
	accepted.Allocation = &allocation
	state := &portfolioDispatchPlanState{reference: reference, pursuitID: pursuit.ID}
	svc.acceptedPlanResolver = state
	proposal, err := svc.PreparePortfolioExecutionProposalsForOwner(
		"alice", "alice", allocation.ID,
		PortfolioExecutionProposalRequest{
			ExpectedAllocationDigest: allocation.RecordDigest,
			Confirmation:             PortfolioExecutionProposalConfirmation,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	item := proposal.Items[0]
	approved, err := svc.DecidePortfolioExecutionProposalItemForOwner(
		"alice", "alice", item.ID,
		PortfolioExecutionProposalDecisionRequest{
			ExpectedItemDigest: item.RecordDigest, Decision: PortfolioExecutionDecisionApproved,
			Reason:       "Owner reviewed this exact immutable action.",
			Confirmation: PortfolioExecutionDecisionApproveConfirmation,
		},
	)
	if err != nil || approved.Decision == nil {
		t.Fatalf("approve coordinated portfolio item=%#v err=%v", approved, err)
	}
	state.calls = 0
	return svc, repo, item, *approved.Decision, state
}

func (a portfolioDispatchDeniedAuthorizer) Authorize(
	ctx context.Context,
	request executionauth.Request,
) (executionauth.Receipt, error) {
	receipt, err := a.delegate.Authorize(ctx, request)
	receipt.Outcome = executionauth.OutcomeDenied
	receipt.Reason = "Emergency stop policy denied the effect."
	return receipt, err
}

func newPortfolioDispatchFakeRepository(base *portfolioAcceptanceFakeRepository) *portfolioDispatchFakeRepository {
	return &portfolioDispatchFakeRepository{
		portfolioAcceptanceFakeRepository: base,
		dispatchRuns:                      map[string]models.PursuitPortfolioDispatchRun{},
		dispatchResults:                   map[uuid.UUID][]models.PursuitPortfolioDispatchItemResult{},
	}
}

func (r *portfolioDispatchFakeRepository) proposalByID(id uuid.UUID) models.PursuitPortfolioExecutionProposal {
	for _, proposal := range r.executionProposals {
		if proposal.ID == id {
			return proposal
		}
	}
	return models.PursuitPortfolioExecutionProposal{}
}

func (r *portfolioDispatchFakeRepository) LoadPortfolioDispatchProposal(
	ownerIdentity string,
	proposalID uuid.UUID,
) (*portfolioDispatchProposalEvidence, error) {
	proposal := r.proposalByID(proposalID)
	if proposal.ID == uuid.Nil || proposal.OwnerIdentity != ownerIdentity {
		return nil, nil
	}
	var allocation models.PursuitPortfolioAllocation
	for _, record := range r.portfolioAllocations {
		if record.ID == proposal.AllocationID && record.OwnerIdentity == ownerIdentity && record.RecordDigest == proposal.AllocationRecordDigest {
			allocation = record
			break
		}
	}
	if allocation.ID == uuid.Nil {
		return nil, fmt.Errorf("portfolio dispatch parent allocation evidence is unavailable or changed")
	}
	items := append([]models.PursuitPortfolioExecutionProposalItem(nil), r.executionProposalItems[proposalID]...)
	allocationItems := append([]models.PursuitPortfolioAllocationItem(nil), r.portfolioItems[allocation.ID]...)
	return &portfolioDispatchProposalEvidence{
		Allocation: allocation, AllocationItems: allocationItems,
		Proposal: proposal, Items: items,
	}, nil
}

func (r *portfolioDispatchFakeRepository) LoadPortfolioDispatchCoordinationEvidence(
	ctx context.Context,
	ownerIdentity string,
	proposalIDs []uuid.UUID,
	runLimit int,
) (map[uuid.UUID]portfolioDispatchCoordinationEvidence, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context is required")
	}
	result := make(map[uuid.UUID]portfolioDispatchCoordinationEvidence)
	for _, proposalID := range proposalIDs {
		proposalEvidence, err := r.LoadPortfolioDispatchProposal(ownerIdentity, proposalID)
		if err != nil {
			return nil, err
		}
		if proposalEvidence == nil {
			continue
		}
		runs, err := r.ListPortfolioDispatchRuns(ownerIdentity, proposalID, runLimit)
		if err != nil {
			return nil, err
		}
		latestRecords, err := r.ListLatestPortfolioDispatchItemResults(ownerIdentity, proposalID)
		if err != nil {
			return nil, err
		}
		evidence := portfolioDispatchCoordinationEvidence{
			Allocation: proposalEvidence.Allocation,
			Proposal:   proposalEvidence.Proposal, Items: proposalEvidence.Items, DispatchRuns: runs,
			LatestDispatch:    make(map[uuid.UUID]models.PursuitPortfolioDispatchItemResult),
			ApprovalSnapshots: make(map[uuid.UUID]*PortfolioWorkflowEffectApprovalSnapshot),
		}
		for _, latest := range latestRecords {
			evidence.LatestDispatch[latest.ProposalItemID] = latest
		}
		for _, item := range proposalEvidence.Items {
			current, err := r.LoadPortfolioExecutionProposalDecisionSnapshot(ownerIdentity, item.ID)
			if err != nil || current == nil {
				return nil, firstPortfolioError(err, fmt.Errorf("portfolio coordination source evidence is incomplete"))
			}
			snapshot := &PortfolioWorkflowEffectApprovalSnapshot{
				Allocation: current.Allocation,
				Proposal:   current.Proposal, Item: current.Item,
				AllocationItem: current.AllocationItem, Pursuit: current.Pursuit,
				Settled: current.Settled,
			}
			if current.LatestDecision != nil {
				decisionCopy := *current.LatestDecision
				snapshot.Decision = decisionCopy
				snapshot.LatestDecision = &decisionCopy
			}
			evidence.ApprovalSnapshots[item.ID] = snapshot
		}
		result[proposalID] = evidence
	}
	return result, nil
}

func (r *portfolioDispatchFakeRepository) FindOrCreatePortfolioDispatchRun(
	wanted *models.PursuitPortfolioDispatchRun,
) (*models.PursuitPortfolioDispatchRun, bool, error) {
	key := wanted.OwnerIdentity + ":" + wanted.RequestDigest
	if existing, ok := r.dispatchRuns[key]; ok {
		if existing.ProposalID != wanted.ProposalID || existing.ProposalDigest != wanted.ProposalDigest ||
			existing.SelectedItemsDigest != wanted.SelectedItemsDigest || existing.Actor != wanted.Actor ||
			existing.Confirmation != wanted.Confirmation {
			return nil, false, fmt.Errorf("portfolio dispatch request digest already exists with different immutable evidence")
		}
		copy := existing
		return &copy, false, nil
	}
	r.dispatchRuns[key] = *wanted
	copy := *wanted
	return &copy, true, nil
}

func (r *portfolioDispatchFakeRepository) ListPortfolioDispatchRuns(
	ownerIdentity string,
	proposalID uuid.UUID,
	limit int,
) ([]models.PursuitPortfolioDispatchRun, error) {
	result := []models.PursuitPortfolioDispatchRun{}
	for _, record := range r.dispatchRuns {
		if record.OwnerIdentity == ownerIdentity && record.ProposalID == proposalID {
			result = append(result, record)
		}
	}
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (r *portfolioDispatchFakeRepository) ListPortfolioDispatchItemResults(
	ownerIdentity string,
	runID uuid.UUID,
) ([]models.PursuitPortfolioDispatchItemResult, error) {
	result := []models.PursuitPortfolioDispatchItemResult{}
	for _, record := range r.dispatchResults[runID] {
		if record.OwnerIdentity == ownerIdentity {
			result = append(result, record)
		}
	}
	return result, nil
}

func (r *portfolioDispatchFakeRepository) ListLatestPortfolioDispatchItemResults(
	ownerIdentity string,
	proposalID uuid.UUID,
) ([]models.PursuitPortfolioDispatchItemResult, error) {
	latest := map[uuid.UUID]models.PursuitPortfolioDispatchItemResult{}
	for _, records := range r.dispatchResults {
		for _, record := range records {
			if record.OwnerIdentity != ownerIdentity || record.ProposalID != proposalID {
				continue
			}
			current, found := latest[record.ProposalItemID]
			if !found || record.AttemptedAt.After(current.AttemptedAt) ||
				(record.AttemptedAt.Equal(current.AttemptedAt) && record.AttemptNumber > current.AttemptNumber) {
				latest[record.ProposalItemID] = record
			}
		}
	}
	result := make([]models.PursuitPortfolioDispatchItemResult, 0, len(latest))
	for _, record := range latest {
		result = append(result, record)
	}
	return result, nil
}

func (r *portfolioDispatchFakeRepository) AppendPortfolioDispatchItemResult(
	wanted *models.PursuitPortfolioDispatchItemResult,
) (*models.PursuitPortfolioDispatchItemResult, bool, error) {
	records := r.dispatchResults[wanted.DispatchRunID]
	for index := len(records) - 1; index >= 0; index-- {
		if records[index].ProposalItemID == wanted.ProposalItemID && portfolioDispatchOutcomeTerminal(records[index].Outcome) {
			copy := records[index]
			return &copy, false, nil
		}
	}
	wanted.AttemptNumber = 1
	for _, record := range records {
		if record.ProposalItemID == wanted.ProposalItemID && record.AttemptNumber >= wanted.AttemptNumber {
			wanted.AttemptNumber = record.AttemptNumber + 1
		}
	}
	var err error
	wanted.RecordDigest, err = digestPortfolioDispatchItemResult(wanted)
	if err != nil {
		return nil, false, err
	}
	records = append(records, *wanted)
	r.dispatchResults[wanted.DispatchRunID] = records
	copy := *wanted
	return &copy, true, nil
}

func withDispatchConfirmation(request PortfolioDispatchRequest, confirmation string) PortfolioDispatchRequest {
	request.Confirmation = confirmation
	return request
}

func withDispatchProposalDigest(request PortfolioDispatchRequest, digest string) PortfolioDispatchRequest {
	request.ExpectedProposalDigest = digest
	return request
}

func withDispatchItems(request PortfolioDispatchRequest, items []PortfolioDispatchItemRequest) PortfolioDispatchRequest {
	request.Items = items
	return request
}
