package pursuit

import (
	"automation-hub-backend/internal/identity"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/plangraph"
	"automation-hub-backend/internal/rbac"
	"automation-hub-backend/migrations"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var _ pursuitPortfolioExecutionProposalDecisionRepository = (*GormRepository)(nil)
var _ pursuitPortfolioExecutionProposalDecisionRepository = (*portfolioAcceptanceFakeRepository)(nil)

func TestPortfolioExecutionProposalDecisionMigrationContract(t *testing.T) {
	upBytes, err := migrations.Files.ReadFile("pre/0040_pursuit_portfolio_execution_proposal_decisions.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	downBytes, err := migrations.Files.ReadFile("pre/0040_pursuit_portfolio_execution_proposal_decisions.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := string(upBytes)
	for _, required := range []string{
		"pursuit_portfolio_execution_proposal_decisions", "approval_decision_only",
		PortfolioExecutionDecisionApproveConfirmation, PortfolioExecutionDecisionRejectConfirmation,
		PortfolioExecutionDecisionClarifyConfirmation, PortfolioExecutionDecisionRevokeConfirmation,
		"append-only", "previous_decision_id", "expires_at",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("decision migration is missing %q", required)
		}
	}
	if strings.Contains(strings.ToUpper(string(downBytes)), " CASCADE") {
		t.Fatal("decision rollback must fail closed without CASCADE")
	}
}

func TestPortfolioExecutionProposalDecisionIsAppendOnlyBoundedAndNonExecutable(t *testing.T) {
	svc, repo, accepted, _ := acceptedExecutionProposalFixture(t, "high", "approve_before_execute", "Draft a source-grounded response")
	proposal, err := svc.PreparePortfolioExecutionProposalsForOwner("alice", "alice", accepted.Allocation.ID, PortfolioExecutionProposalRequest{
		ExpectedAllocationDigest: accepted.Allocation.RecordDigest,
		Confirmation:             PortfolioExecutionProposalConfirmation,
	})
	if err != nil {
		t.Fatal(err)
	}
	item := proposal.Items[0]
	request := PortfolioExecutionProposalDecisionRequest{
		ExpectedItemDigest: item.RecordDigest, Decision: PortfolioExecutionDecisionApproved,
		Reason:       "I reviewed the exact proposed action and its recorded safety gates.",
		Confirmation: PortfolioExecutionDecisionApproveConfirmation,
	}
	result, err := svc.DecidePortfolioExecutionProposalItemForOwner("alice", "alice", item.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision == nil || result.Decision.Decision != PortfolioExecutionDecisionApproved ||
		result.Authority != PortfolioExecutionDecisionAuthority || result.CanExecute || result.Replayed ||
		result.Decision.ExpiresAt == nil || !result.Decision.ExpiresAt.After(result.Decision.DecidedAt) {
		t.Fatalf("approval decision result=%#v", result)
	}
	if len(repo.executionProposalDecisions[item.ID]) != 1 || len(repo.executionDecisionActivities) != 1 ||
		len(repo.workflows) != 0 || len(repo.taskAttempts) != 0 {
		t.Fatalf("decision crossed execution boundary: decisions=%d activities=%d workflows=%d attempts=%d",
			len(repo.executionProposalDecisions[item.ID]), len(repo.executionDecisionActivities), len(repo.workflows), len(repo.taskAttempts))
	}
	replay, err := svc.DecidePortfolioExecutionProposalItemForOwner("alice", "alice", item.ID, request)
	if err != nil || !replay.Replayed || replay.Decision.ID != result.Decision.ID || len(repo.executionProposalDecisions[item.ID]) != 1 {
		t.Fatalf("decision replay=%#v err=%v", replay, err)
	}

	request.Decision = PortfolioExecutionDecisionRevoked
	request.Reason = "Withdraw this approval before any concrete effect is authorized."
	request.Confirmation = PortfolioExecutionDecisionRevokeConfirmation
	revoked, err := svc.DecidePortfolioExecutionProposalItemForOwner("alice", "alice", item.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	if revoked.Decision.Decision != PortfolioExecutionDecisionRevoked || revoked.Decision.PreviousDecisionID == nil ||
		*revoked.Decision.PreviousDecisionID != result.Decision.ID || revoked.Decision.ExpiresAt != nil || revoked.CanExecute {
		t.Fatalf("revocation=%#v", revoked)
	}
	history, err := svc.PortfolioExecutionProposalDecisionHistoryForOwner("alice", item.ID, 10)
	if err != nil || len(history.Decisions) != 2 || history.Decisions[0].Decision != PortfolioExecutionDecisionRevoked ||
		history.Authority != PortfolioExecutionDecisionAuthority || history.CanExecute {
		t.Fatalf("history=%#v err=%v", history, err)
	}
}

func TestPortfolioExecutionProposalDecisionFailsClosedForStaleBlockedAndOwnerBoundaries(t *testing.T) {
	svc, repo, accepted, pursuit := acceptedExecutionProposalFixture(t, "low", "autonomous_safe", "Prepare the evidence inventory")
	proposal, err := svc.PreparePortfolioExecutionProposalsForOwner("alice", "alice", accepted.Allocation.ID, PortfolioExecutionProposalRequest{
		ExpectedAllocationDigest: accepted.Allocation.RecordDigest,
		Confirmation:             PortfolioExecutionProposalConfirmation,
	})
	if err != nil {
		t.Fatal(err)
	}
	item := proposal.Items[0]
	request := PortfolioExecutionProposalDecisionRequest{
		ExpectedItemDigest: item.RecordDigest, Decision: PortfolioExecutionDecisionApproved,
		Reason: "Reviewed", Confirmation: PortfolioExecutionDecisionApproveConfirmation,
	}
	if _, err := svc.DecidePortfolioExecutionProposalItemForOwner("alice", "mallory", item.ID, request); err == nil || !strings.Contains(err.Error(), "authenticated owner") {
		t.Fatalf("actor boundary error=%v", err)
	}
	if _, err := svc.DecidePortfolioExecutionProposalItemForOwner("mallory", "mallory", item.ID, request); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("owner boundary error=%v", err)
	}
	request.ExpectedItemDigest = strings.Repeat("f", 64)
	if _, err := svc.DecidePortfolioExecutionProposalItemForOwner("alice", "alice", item.ID, request); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("item digest boundary error=%v", err)
	}
	request.ExpectedItemDigest = item.RecordDigest
	request.Confirmation = "APPROVE"
	if _, err := svc.DecidePortfolioExecutionProposalItemForOwner("alice", "alice", item.ID, request); err == nil || !strings.Contains(err.Error(), "exact confirmation") {
		t.Fatalf("confirmation boundary error=%v", err)
	}
	request.Confirmation = PortfolioExecutionDecisionApproveConfirmation
	changed := repo.pursuits[pursuit.ID]
	changed.NextRecommendedAction = "A materially different action"
	repo.pursuits[pursuit.ID] = changed
	if _, err := svc.DecidePortfolioExecutionProposalItemForOwner("alice", "alice", item.ID, request); err == nil || !strings.Contains(err.Error(), "fresh proposal") {
		t.Fatalf("stale-state boundary error=%v", err)
	}
	if len(repo.executionProposalDecisions[item.ID]) != 0 {
		t.Fatal("failed decisions mutated durable state")
	}

	blockedSvc, _, blockedAccepted, _ := acceptedExecutionProposalFixture(t, "high", "approve_before_execute", "Draft the legal response")
	blockedRepo := blockedSvc.repo.(*portfolioAcceptanceFakeRepository)
	blockedPursuitID := blockedAccepted.Items[0].PursuitID
	blockedPursuit := blockedRepo.pursuits[blockedPursuitID]
	blockedPursuit.Status = StatusBlocked
	blockedRepo.pursuits[blockedPursuitID] = blockedPursuit
	blockedProposal, err := blockedSvc.PreparePortfolioExecutionProposalsForOwner("alice", "alice", blockedAccepted.Allocation.ID, PortfolioExecutionProposalRequest{
		ExpectedAllocationDigest: blockedAccepted.Allocation.RecordDigest,
		Confirmation:             PortfolioExecutionProposalConfirmation,
	})
	if err != nil {
		t.Fatal(err)
	}
	blockedItem := blockedProposal.Items[0]
	request.ExpectedItemDigest = blockedItem.RecordDigest
	if _, err := blockedSvc.DecidePortfolioExecutionProposalItemForOwner("alice", "alice", blockedItem.ID, request); err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("blocked-item decision error=%v", err)
	}
}

func TestPortfolioExecutionProposalDecisionRevalidatesCoordinationPlanOnlyForApproval(t *testing.T) {
	testCases := []struct {
		name             string
		decision         string
		confirmation     string
		requiresApproval bool
	}{
		{name: "approval", decision: PortfolioExecutionDecisionApproved, confirmation: PortfolioExecutionDecisionApproveConfirmation},
		{name: "rejection", decision: PortfolioExecutionDecisionRejected, confirmation: PortfolioExecutionDecisionRejectConfirmation},
		{name: "clarification", decision: PortfolioExecutionDecisionNeedsClarification, confirmation: PortfolioExecutionDecisionClarifyConfirmation},
		{name: "revocation", decision: PortfolioExecutionDecisionRevoked, confirmation: PortfolioExecutionDecisionRevokeConfirmation, requiresApproval: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			svc, repo, accepted, pursuit := acceptedExecutionProposalFixture(t, "high", "approve_before_execute", "Prepare governed evidence")
			reference := plangraph.AcceptedRevisionReference{
				PlanID: uuid.New(), Revision: 4, Digest: strings.Repeat("a", 64), NodeID: "portfolio-node",
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

			stale := false
			calls := 0
			svc.acceptedPlanResolver = pursuitAcceptedPlanResolverFunc(func(
				_ context.Context,
				owner string,
				got plangraph.AcceptedRevisionReference,
			) (*plangraph.AcceptedRevisionBinding, error) {
				calls++
				if owner != "alice" || got != reference {
					t.Fatalf("resolver owner/reference = %q %#v", owner, got)
				}
				if stale {
					return nil, fmt.Errorf("accepted revision is stale")
				}
				return &plangraph.AcceptedRevisionBinding{
					PlanID: reference.PlanID, Revision: reference.Revision, Digest: reference.Digest,
					NodeID: reference.NodeID, Nodes: []plangraph.Node{{
						ID: "pursuit-node", Bindings: plangraph.Bindings{PursuitID: pursuit.ID.String()},
					}}, CanExecute: false,
				}, nil
			})

			proposal, err := svc.PreparePortfolioExecutionProposalsForOwner("alice", "alice", allocation.ID, PortfolioExecutionProposalRequest{
				ExpectedAllocationDigest: allocation.RecordDigest,
				Confirmation:             PortfolioExecutionProposalConfirmation,
			})
			if err != nil {
				t.Fatal(err)
			}
			item := proposal.Items[0]
			if testCase.requiresApproval {
				approved, err := svc.DecidePortfolioExecutionProposalItemForOwner("alice", "alice", item.ID, PortfolioExecutionProposalDecisionRequest{
					ExpectedItemDigest: item.RecordDigest, Decision: PortfolioExecutionDecisionApproved,
					Reason: "Approve before testing stale-plan revocation.", Confirmation: PortfolioExecutionDecisionApproveConfirmation,
				})
				if err != nil || approved.Decision == nil {
					t.Fatalf("prerequisite approval=%#v err=%v", approved, err)
				}
			}
			callsBeforeDecision := calls
			stale = true
			result, err := svc.DecidePortfolioExecutionProposalItemForOwner("alice", "alice", item.ID, PortfolioExecutionProposalDecisionRequest{
				ExpectedItemDigest: item.RecordDigest, Decision: testCase.decision,
				Reason: "Exercise the proposal decision boundary.", Confirmation: testCase.confirmation,
			})
			if testCase.decision == PortfolioExecutionDecisionApproved {
				if err == nil || !strings.Contains(err.Error(), "accepted revision is stale") {
					t.Fatalf("stale approval result=%#v err=%v", result, err)
				}
				if calls != callsBeforeDecision+1 || len(repo.executionProposalDecisions[item.ID]) != 0 {
					t.Fatalf("stale approval calls=%d decisions=%d", calls, len(repo.executionProposalDecisions[item.ID]))
				}
				return
			}
			if err != nil || result == nil || result.Decision == nil || result.Decision.Decision != testCase.decision {
				t.Fatalf("non-approval result=%#v err=%v", result, err)
			}
			if calls != callsBeforeDecision {
				t.Fatalf("%s unexpectedly revalidated stale coordination plan: calls %d -> %d", testCase.decision, callsBeforeDecision, calls)
			}
		})
	}
}

func TestPortfolioExecutionProposalDecisionHandlerUsesVerifiedOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _, accepted, _ := acceptedExecutionProposalFixture(t, "high", "approve_before_execute", "Prepare a reviewed response")
	proposal, err := svc.PreparePortfolioExecutionProposalsForOwner("alice", "alice", accepted.Allocation.ID, PortfolioExecutionProposalRequest{
		ExpectedAllocationDigest: accepted.Allocation.RecordDigest,
		Confirmation:             PortfolioExecutionProposalConfirmation,
	})
	if err != nil {
		t.Fatal(err)
	}
	item := proposal.Items[0]
	payload, _ := json.Marshal(PortfolioExecutionProposalDecisionRequest{
		ExpectedItemDigest: item.RecordDigest, Decision: PortfolioExecutionDecisionApproved,
		Reason: "Owner reviewed the immutable proposal.", Confirmation: PortfolioExecutionDecisionApproveConfirmation,
	})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "alice")
		c.Set(identity.ContextRoleKey, string(rbac.RoleOwner))
		c.Next()
	})
	handler := NewHandler(svc)
	router.POST("/proposal-items/:itemId/decisions", handler.DecidePortfolioExecutionProposalItem)
	router.GET("/proposal-items/:itemId/decisions", handler.PortfolioExecutionProposalDecisionHistory)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/proposal-items/"+item.ID.String()+"/decisions", strings.NewReader(string(payload)))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"authority":"approval_decision_only"`) ||
		!strings.Contains(response.Body.String(), `"canExecute":false`) {
		t.Fatalf("decision handler status=%d body=%s", response.Code, response.Body.String())
	}
	history := httptest.NewRecorder()
	router.ServeHTTP(history, httptest.NewRequest(http.MethodGet, "/proposal-items/"+item.ID.String()+"/decisions?limit=10", nil))
	if history.Code != http.StatusOK || !strings.Contains(history.Body.String(), `"decisions":[`) ||
		!strings.Contains(history.Body.String(), `"canExecute":false`) {
		t.Fatalf("decision history status=%d body=%s", history.Code, history.Body.String())
	}
}

func (r *portfolioAcceptanceFakeRepository) LoadPortfolioExecutionProposalDecisionSnapshot(
	ownerIdentity string,
	itemID uuid.UUID,
) (*portfolioExecutionProposalDecisionSnapshot, error) {
	var item *models.PursuitPortfolioExecutionProposalItem
	var proposal *models.PursuitPortfolioExecutionProposal
	for _, candidate := range r.executionProposals {
		if candidate.OwnerIdentity != ownerIdentity {
			continue
		}
		for _, candidateItem := range r.executionProposalItems[candidate.ID] {
			if candidateItem.ID == itemID && candidateItem.OwnerIdentity == ownerIdentity {
				itemCopy := candidateItem
				proposalCopy := candidate
				item, proposal = &itemCopy, &proposalCopy
				break
			}
		}
	}
	if item == nil || proposal == nil {
		return nil, nil
	}
	var allocation models.PursuitPortfolioAllocation
	for _, candidate := range r.portfolioAllocations {
		if candidate.ID == proposal.AllocationID && candidate.OwnerIdentity == ownerIdentity &&
			candidate.RecordDigest == proposal.AllocationRecordDigest {
			allocation = candidate
			break
		}
	}
	var allocationItem models.PursuitPortfolioAllocationItem
	for _, candidate := range r.portfolioItems[proposal.AllocationID] {
		if candidate.ID == item.AllocationItemID && candidate.OwnerIdentity == ownerIdentity {
			allocationItem = candidate
			break
		}
	}
	pursuit, ok := r.pursuits[item.PursuitID]
	if allocation.ID == uuid.Nil || allocationItem.ID == uuid.Nil || !ok || pursuit.OwnerIdentity != ownerIdentity {
		return nil, fmt.Errorf("decision source evidence is unavailable")
	}
	records := r.executionProposalDecisions[itemID]
	var latest *models.PursuitPortfolioExecutionProposalDecision
	if len(records) > 0 {
		copyRecord := records[len(records)-1]
		latest = &copyRecord
	}
	_, settled := r.resourceSettlements[item.ReservationID]
	return &portfolioExecutionProposalDecisionSnapshot{
		Allocation: allocation, Proposal: *proposal, Item: *item, AllocationItem: allocationItem,
		Pursuit: pursuit, Settled: settled, LatestDecision: latest,
	}, nil
}

func (r *portfolioAcceptanceFakeRepository) SavePortfolioExecutionProposalDecision(
	snapshot *portfolioExecutionProposalDecisionSnapshot,
	decision *models.PursuitPortfolioExecutionProposalDecision,
	activity models.PursuitActivity,
) (*models.PursuitPortfolioExecutionProposalDecision, bool, error) {
	current, err := r.LoadPortfolioExecutionProposalDecisionSnapshot(decision.OwnerIdentity, decision.ProposalItemID)
	if err != nil || current == nil {
		return nil, false, fmt.Errorf("decision source is unavailable")
	}
	if current.LatestDecision != nil && current.LatestDecision.RequestDigest == decision.RequestDigest {
		copyRecord := *current.LatestDecision
		return &copyRecord, false, nil
	}
	currentPrevious := uuid.Nil
	if current.LatestDecision != nil {
		currentPrevious = current.LatestDecision.ID
	}
	wantedPrevious := uuid.Nil
	if decision.PreviousDecisionID != nil {
		wantedPrevious = *decision.PreviousDecisionID
	}
	if currentPrevious != wantedPrevious {
		return nil, false, fmt.Errorf("decision chain changed")
	}
	if err := validatePortfolioExecutionDecisionEvidence(decision.OwnerIdentity, snapshot.Item, decision); err != nil {
		return nil, false, err
	}
	r.executionProposalDecisions[decision.ProposalItemID] = append(r.executionProposalDecisions[decision.ProposalItemID], *decision)
	r.executionDecisionActivities = append(r.executionDecisionActivities, activity)
	copyRecord := *decision
	return &copyRecord, true, nil
}

func (r *portfolioAcceptanceFakeRepository) ListPortfolioExecutionProposalDecisions(
	ownerIdentity string,
	itemID uuid.UUID,
	limit int,
) ([]models.PursuitPortfolioExecutionProposalDecision, error) {
	records := append([]models.PursuitPortfolioExecutionProposalDecision(nil), r.executionProposalDecisions[itemID]...)
	filtered := make([]models.PursuitPortfolioExecutionProposalDecision, 0, len(records))
	for index := len(records) - 1; index >= 0 && len(filtered) < limit; index-- {
		if records[index].OwnerIdentity == ownerIdentity {
			filtered = append(filtered, records[index])
		}
	}
	return filtered, nil
}
