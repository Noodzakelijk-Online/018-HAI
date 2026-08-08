package pursuit

import (
	"automation-hub-backend/internal/identity"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/rbac"
	"automation-hub-backend/internal/resourceplanner"
	"automation-hub-backend/migrations"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var _ pursuitPortfolioExecutionProposalRepository = (*GormRepository)(nil)
var _ pursuitPortfolioExecutionProposalRepository = (*portfolioAcceptanceFakeRepository)(nil)
var _ pursuitPortfolioExecutionProposalHistoryRepository = (*GormRepository)(nil)
var _ pursuitPortfolioExecutionProposalHistoryRepository = (*portfolioAcceptanceFakeRepository)(nil)

func TestPortfolioExecutionProposalMigrationContract(t *testing.T) {
	upBytes, err := migrations.Files.ReadFile("pre/0039_pursuit_portfolio_execution_proposals.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	downBytes, err := migrations.Files.ReadFile("pre/0039_pursuit_portfolio_execution_proposals.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := string(upBytes)
	for _, required := range []string{
		"pursuit_portfolio_execution_proposals",
		"pursuit_portfolio_execution_proposal_items",
		"proposal_only",
		"PREPARE EXECUTION PROPOSALS",
		"append-only",
		"allocation_record_digest",
		"snapshot_digest",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("migration is missing %q", required)
		}
	}
	if strings.Contains(strings.ToUpper(string(downBytes)), " CASCADE") {
		t.Fatal("rollback must fail closed without CASCADE")
	}
}

func TestPortfolioExecutionProposalIsImmutableProposalOnlyAndReplaysExactState(t *testing.T) {
	svc, repo, accepted, pursuit := acceptedExecutionProposalFixture(t, "low", "autonomous_safe", "Collect the verified source records")
	request := PortfolioExecutionProposalRequest{
		ExpectedAllocationDigest: accepted.Allocation.RecordDigest,
		Confirmation:             PortfolioExecutionProposalConfirmation,
	}
	result, err := svc.PreparePortfolioExecutionProposalsForOwner("alice", "alice", accepted.Allocation.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Proposal == nil || result.Proposal.Status != PortfolioExecutionProposalPrepared ||
		result.Authority != PortfolioExecutionProposalAuthority || result.CanExecute || result.Replayed || len(result.Items) != 1 ||
		result.Freshness.Status != PortfolioExecutionProposalFreshnessPrepared || !result.Freshness.RevalidationRequired ||
		result.Freshness.CheckedAt.IsZero() || !strings.Contains(result.Freshness.Reason, "separate revalidation") {
		t.Fatalf("proposal-only result = %#v", result)
	}
	item := result.Items[0]
	if item.Status != PortfolioExecutionProposalItemProposed || item.RequiresApproval || len(item.ApprovalReasons) != 0 ||
		len(item.BlockedReasons) != 0 || item.ReservationID != accepted.Items[0].ReservationID || item.RecordDigest == "" {
		t.Fatalf("proposal item = %#v", item)
	}
	if len(repo.executionProposals) != 1 || len(repo.executionProposalActivities) != 1 ||
		len(repo.workflows) != 0 || len(repo.taskAttempts) != 0 {
		t.Fatalf("proposal crossed execution boundary: proposals=%d activities=%d workflows=%d attempts=%d",
			len(repo.executionProposals), len(repo.executionProposalActivities), len(repo.workflows), len(repo.taskAttempts))
	}
	stored, err := repo.FindByID(pursuit.ID)
	if err != nil || stored.Status != StatusActive || stored.NextRecommendedAction != "Collect the verified source records" {
		t.Fatalf("proposal mutated pursuit: %#v err=%v", stored, err)
	}

	replay, err := svc.PreparePortfolioExecutionProposalsForOwner("alice", "alice", accepted.Allocation.ID, request)
	if err != nil || !replay.Replayed || replay.Proposal.ID != result.Proposal.ID || len(repo.executionProposals) != 1 ||
		replay.Freshness.Status != PortfolioExecutionProposalFreshnessRecovered || !replay.Freshness.RevalidationRequired {
		t.Fatalf("exact replay=%#v err=%v proposals=%d", replay, err, len(repo.executionProposals))
	}

	changed := repo.pursuits[pursuit.ID]
	changed.NextRecommendedAction = "Review the source records with Robert"
	repo.pursuits[pursuit.ID] = changed
	newSnapshot, err := svc.PreparePortfolioExecutionProposalsForOwner("alice", "alice", accepted.Allocation.ID, request)
	if err != nil || newSnapshot.Replayed || newSnapshot.Proposal.SnapshotDigest == result.Proposal.SnapshotDigest || len(repo.executionProposals) != 2 {
		t.Fatalf("changed-state proposal=%#v err=%v proposals=%d", newSnapshot, err, len(repo.executionProposals))
	}
}

func TestPortfolioExecutionProposalSurfacesApprovalAndBlockingReasons(t *testing.T) {
	svc, repo, accepted, pursuit := acceptedExecutionProposalFixture(t, "high", "approve_before_execute", "Draft the legal response")
	request := PortfolioExecutionProposalRequest{
		ExpectedAllocationDigest: accepted.Allocation.RecordDigest,
		Confirmation:             PortfolioExecutionProposalConfirmation,
	}
	result, err := svc.PreparePortfolioExecutionProposalsForOwner("alice", "alice", accepted.Allocation.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Proposal.Status != PortfolioExecutionProposalPreparedNeedsApproval ||
		result.Items[0].Status != PortfolioExecutionProposalItemNeedsApproval || !result.Items[0].RequiresApproval ||
		len(result.Items[0].ApprovalReasons) < 2 || result.CanExecute {
		t.Fatalf("approval proposal = %#v", result)
	}

	changed := repo.pursuits[pursuit.ID]
	changed.Status = StatusBlocked
	changed.StopConditions = []models.PursuitStopCondition{{ID: "legal-review", Description: "Legal evidence must be reviewed", Status: "triggered", TriggeredAt: timePointer(time.Now().UTC())}}
	repo.pursuits[pursuit.ID] = changed
	blocked, err := svc.PreparePortfolioExecutionProposalsForOwner("alice", "alice", accepted.Allocation.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Proposal.Status != PortfolioExecutionProposalPreparedBlocked || blocked.Items[0].Status != PortfolioExecutionProposalItemBlocked ||
		len(blocked.Items[0].BlockedReasons) < 2 || blocked.CanExecute {
		t.Fatalf("blocked proposal = %#v", blocked)
	}

	settlement := models.PursuitResourceReservationSettlement{ReservationID: accepted.Items[0].ReservationID}
	repo.resourceSettlements[settlement.ReservationID] = settlement
	settled, err := svc.PreparePortfolioExecutionProposalsForOwner("alice", "alice", accepted.Allocation.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	if !containsPortfolioReason(settled.Items[0].BlockedReasons, "already settled") {
		t.Fatalf("settled reservation was not blocked: %#v", settled.Items[0])
	}
}

func TestPortfolioExecutionProposalFailsClosedAcrossOwnerConfirmationAndDigestBoundaries(t *testing.T) {
	svc, repo, accepted, _ := acceptedExecutionProposalFixture(t, "low", "autonomous_safe", "Prepare the evidence index")
	request := PortfolioExecutionProposalRequest{ExpectedAllocationDigest: accepted.Allocation.RecordDigest}
	if _, err := svc.PreparePortfolioExecutionProposalsForOwner("alice", "alice", accepted.Allocation.ID, request); err == nil || !strings.Contains(err.Error(), "confirmation") {
		t.Fatalf("missing confirmation error=%v", err)
	}
	request.Confirmation = PortfolioExecutionProposalConfirmation
	if _, err := svc.PreparePortfolioExecutionProposalsForOwner("alice", "mallory", accepted.Allocation.ID, request); err == nil || !strings.Contains(err.Error(), "authenticated owner") {
		t.Fatalf("actor boundary error=%v", err)
	}
	request.ExpectedAllocationDigest = strings.Repeat("f", 64)
	if _, err := svc.PreparePortfolioExecutionProposalsForOwner("alice", "alice", accepted.Allocation.ID, request); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("digest boundary error=%v", err)
	}
	request.ExpectedAllocationDigest = accepted.Allocation.RecordDigest
	if _, err := svc.PreparePortfolioExecutionProposalsForOwner("mallory", "mallory", accepted.Allocation.ID, request); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("owner boundary error=%v", err)
	}
	if len(repo.executionProposals) != 0 || len(repo.executionProposalActivities) != 0 {
		t.Fatal("failed proposal preparation mutated durable state")
	}
}

func TestPortfolioExecutionProposalHandlerUsesVerifiedOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _, accepted, _ := acceptedExecutionProposalFixture(t, "low", "autonomous_safe", "Prepare a source-backed draft")
	payload, err := json.Marshal(PortfolioExecutionProposalRequest{
		ExpectedAllocationDigest: accepted.Allocation.RecordDigest,
		Confirmation:             PortfolioExecutionProposalConfirmation,
	})
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "alice")
		c.Set(identity.ContextRoleKey, string(rbac.RoleOwner))
		c.Next()
	})
	router.POST("/pursuits/portfolio-allocations/:allocationId/execution-proposals", NewHandler(svc).PreparePortfolioExecutionProposals)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/pursuits/portfolio-allocations/"+accepted.Allocation.ID.String()+"/execution-proposals", strings.NewReader(string(payload)))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"authority":"proposal_only"`) ||
		!strings.Contains(response.Body.String(), `"canExecute":false`) {
		t.Fatalf("proposal handler status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestPortfolioExecutionProposalHistoryRestoresLatestOwnerScopedSnapshot(t *testing.T) {
	svc, repo, accepted, pursuit := acceptedExecutionProposalFixture(t, "low", "autonomous_safe", "Prepare the evidence index")
	request := PortfolioExecutionProposalRequest{
		ExpectedAllocationDigest: accepted.Allocation.RecordDigest,
		Confirmation:             PortfolioExecutionProposalConfirmation,
	}
	first, err := svc.PreparePortfolioExecutionProposalsForOwner("alice", "alice", accepted.Allocation.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	changed := repo.pursuits[pursuit.ID]
	changed.NextRecommendedAction = "Review the recovered evidence index"
	repo.pursuits[pursuit.ID] = changed
	latest, err := svc.PreparePortfolioExecutionProposalsForOwner("alice", "alice", accepted.Allocation.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	results, err := svc.PortfolioExecutionProposalHistoryForOwner("alice", []uuid.UUID{accepted.Allocation.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Proposal.ID != latest.Proposal.ID ||
		results[0].Proposal.ID == first.Proposal.ID || !results[0].Replayed ||
		results[0].Authority != PortfolioExecutionProposalAuthority || results[0].CanExecute ||
		results[0].Freshness.Status != PortfolioExecutionProposalFreshnessRecovered ||
		!results[0].Freshness.RevalidationRequired || results[0].Freshness.CheckedAt.IsZero() ||
		!strings.Contains(results[0].Freshness.Reason, "does not prove current approval") ||
		len(results[0].Items) != 1 || results[0].Items[0].ActionSummary != "Review the recovered evidence index" {
		t.Fatalf("restored proposal history = %#v", results)
	}
	foreign, err := svc.PortfolioExecutionProposalHistoryForOwner("mallory", []uuid.UUID{accepted.Allocation.ID})
	if err != nil || len(foreign) != 0 {
		t.Fatalf("foreign history = %#v err=%v", foreign, err)
	}
	if _, err := svc.PortfolioExecutionProposalHistoryForOwner("alice", []uuid.UUID{accepted.Allocation.ID, accepted.Allocation.ID}); err == nil || !strings.Contains(err.Error(), "unique") {
		t.Fatalf("duplicate allocation ids were accepted: %v", err)
	}
}

func TestPortfolioExecutionProposalHistoryHandlerValidatesBoundedAllocationIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _, accepted, _ := acceptedExecutionProposalFixture(t, "low", "autonomous_safe", "Prepare a source-backed draft")
	_, err := svc.PreparePortfolioExecutionProposalsForOwner("alice", "alice", accepted.Allocation.ID, PortfolioExecutionProposalRequest{
		ExpectedAllocationDigest: accepted.Allocation.RecordDigest,
		Confirmation:             PortfolioExecutionProposalConfirmation,
	})
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "alice")
		c.Set(identity.ContextRoleKey, string(rbac.RoleOwner))
		c.Next()
	})
	router.GET("/pursuits/portfolio-execution-proposals", NewHandler(svc).PortfolioExecutionProposalHistory)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/pursuits/portfolio-execution-proposals?allocationIds="+accepted.Allocation.ID.String(), nil)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"authority":"proposal_only"`) ||
		!strings.Contains(response.Body.String(), `"canExecute":false`) ||
		!strings.Contains(response.Body.String(), `"status":"recovered_snapshot"`) ||
		!strings.Contains(response.Body.String(), `"revalidationRequired":true`) {
		t.Fatalf("history handler status=%d body=%s", response.Code, response.Body.String())
	}

	invalid := httptest.NewRecorder()
	router.ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "/pursuits/portfolio-execution-proposals?allocationIds=invalid", nil))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid history status=%d body=%s", invalid.Code, invalid.Body.String())
	}
}

func acceptedExecutionProposalFixture(
	t *testing.T,
	riskLevel, autonomyLevel, nextAction string,
) (*service, *portfolioAcceptanceFakeRepository, *PortfolioAllocationAcceptanceResult, *models.Pursuit) {
	t.Helper()
	repo := newPortfolioAcceptanceFakeRepository()
	svc := &service{repo: repo}
	pursuit, err := svc.Create(CreateRequest{
		OwnerIdentity: "alice", Title: "Execution proposal fixture",
		RiskLevel: riskLevel, AutonomyLevel: autonomyLevel, NextRecommendedAction: nextAction,
		ResourceLimits: models.PursuitResourceLimits{MaxEffortHours: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Minute)
	planning := PortfolioPlanningRequest{
		PlanID: "execution-proposal-" + uuid.NewString(), AsOf: now,
		HorizonStart: now, HorizonEnd: now.Add(time.Hour),
		DurationMode: resourceplanner.ExpectedDuration,
		Availability: []PortfolioCapacityWindow{{Start: now, End: now.Add(time.Hour)}},
		Pursuits: []PortfolioPursuitPlanningInput{{
			PursuitID: pursuit.ID, Duration: portfolioDuration(10, 20, 30), Factors: portfolioFactors(75),
		}},
		Budget: resourceplanner.Budget{MaxCostMicros: portfolioInt64(0)},
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
	return svc, repo, accepted, pursuit
}

func timePointer(value time.Time) *time.Time { return &value }

func containsPortfolioReason(reasons []string, fragment string) bool {
	for _, reason := range reasons {
		if strings.Contains(strings.ToLower(reason), strings.ToLower(fragment)) {
			return true
		}
	}
	return false
}

func (r *portfolioAcceptanceFakeRepository) LoadPortfolioExecutionProposalSnapshot(
	ownerIdentity string,
	allocationID uuid.UUID,
) (*portfolioExecutionProposalSnapshot, error) {
	var allocation *models.PursuitPortfolioAllocation
	for _, record := range r.portfolioAllocations {
		if record.ID == allocationID && record.OwnerIdentity == ownerIdentity {
			copyRecord := record
			allocation = &copyRecord
			break
		}
	}
	if allocation == nil {
		return nil, nil
	}
	allocationItems := append([]models.PursuitPortfolioAllocationItem(nil), r.portfolioItems[allocationID]...)
	pursuits := map[uuid.UUID]models.Pursuit{}
	settled := map[uuid.UUID]struct{}{}
	for _, item := range allocationItems {
		if record, ok := r.pursuits[item.PursuitID]; ok && record.OwnerIdentity == ownerIdentity {
			pursuits[item.PursuitID] = record
		}
		if _, ok := r.resourceSettlements[item.ReservationID]; ok {
			settled[item.ReservationID] = struct{}{}
		}
	}
	return &portfolioExecutionProposalSnapshot{
		Allocation: allocation, AllocationItems: allocationItems,
		Pursuits: pursuits, SettledReservationIDs: settled,
	}, nil
}

func (r *portfolioAcceptanceFakeRepository) FindPortfolioExecutionProposalForSnapshot(
	ownerIdentity string,
	allocationID uuid.UUID,
	snapshotDigest string,
) (*models.PursuitPortfolioExecutionProposal, []models.PursuitPortfolioExecutionProposalItem, error) {
	record, ok := r.executionProposals[ownerIdentity+":"+allocationID.String()+":"+snapshotDigest]
	if !ok {
		return nil, nil, nil
	}
	items := append([]models.PursuitPortfolioExecutionProposalItem(nil), r.executionProposalItems[record.ID]...)
	return &record, items, nil
}

func (r *portfolioAcceptanceFakeRepository) ListLatestPortfolioExecutionProposals(
	ownerIdentity string,
	allocationIDs []uuid.UUID,
) ([]models.PursuitPortfolioExecutionProposal, map[uuid.UUID][]models.PursuitPortfolioExecutionProposalItem, error) {
	requested := make(map[uuid.UUID]struct{}, len(allocationIDs))
	for _, allocationID := range allocationIDs {
		requested[allocationID] = struct{}{}
	}
	latest := make(map[uuid.UUID]models.PursuitPortfolioExecutionProposal, len(allocationIDs))
	for _, proposal := range r.executionProposals {
		if proposal.OwnerIdentity != ownerIdentity {
			continue
		}
		if _, ok := requested[proposal.AllocationID]; !ok {
			continue
		}
		prior, exists := latest[proposal.AllocationID]
		if !exists || proposal.PreparedAt.After(prior.PreparedAt) ||
			(proposal.PreparedAt.Equal(prior.PreparedAt) && proposal.ID.String() > prior.ID.String()) {
			latest[proposal.AllocationID] = proposal
		}
	}
	proposals := make([]models.PursuitPortfolioExecutionProposal, 0, len(latest))
	itemsByProposal := make(map[uuid.UUID][]models.PursuitPortfolioExecutionProposalItem, len(latest))
	for _, allocationID := range allocationIDs {
		proposal, exists := latest[allocationID]
		if !exists {
			continue
		}
		proposals = append(proposals, proposal)
		itemsByProposal[proposal.ID] = append([]models.PursuitPortfolioExecutionProposalItem(nil), r.executionProposalItems[proposal.ID]...)
	}
	return proposals, itemsByProposal, nil
}

func (r *portfolioAcceptanceFakeRepository) SavePortfolioExecutionProposal(
	proposal *models.PursuitPortfolioExecutionProposal,
	items []models.PursuitPortfolioExecutionProposalItem,
	activities []models.PursuitActivity,
) (*models.PursuitPortfolioExecutionProposal, []models.PursuitPortfolioExecutionProposalItem, bool, error) {
	if err := validatePortfolioExecutionProposalAggregate(proposal, items, activities); err != nil {
		return nil, nil, false, err
	}
	key := proposal.OwnerIdentity + ":" + proposal.AllocationID.String() + ":" + proposal.SnapshotDigest
	if existing, ok := r.executionProposals[key]; ok {
		if existing.RecordDigest != proposal.RecordDigest {
			return nil, nil, false, fmt.Errorf("different proposal parent evidence")
		}
		storedItems := append([]models.PursuitPortfolioExecutionProposalItem(nil), r.executionProposalItems[existing.ID]...)
		return &existing, storedItems, false, nil
	}
	r.executionProposals[key] = *proposal
	r.executionProposalItems[proposal.ID] = append([]models.PursuitPortfolioExecutionProposalItem(nil), items...)
	r.executionProposalActivities = append(r.executionProposalActivities, activities...)
	stored := *proposal
	return &stored, append([]models.PursuitPortfolioExecutionProposalItem(nil), items...), true, nil
}
