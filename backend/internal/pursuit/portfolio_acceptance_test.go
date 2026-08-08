package pursuit

import (
	"automation-hub-backend/internal/identity"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/rbac"
	"automation-hub-backend/internal/resourceplanner"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestPortfolioAllocationAcceptanceIsFreshIdempotentAndNonExecutable(t *testing.T) {
	repo := newPortfolioAcceptanceFakeRepository()
	svc := &service{repo: repo}
	lowRisk, err := svc.Create(CreateRequest{
		OwnerIdentity: "alice", Title: "Prepare evidence inventory",
		RiskLevel: "low", AutonomyLevel: "autonomous_safe",
		ResourceLimits: models.PursuitResourceLimits{MaxEffortHours: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	highRisk, err := svc.Create(CreateRequest{
		OwnerIdentity: "alice", Title: "Draft legal response",
		RiskLevel: "high", AutonomyLevel: "approve_before_execute",
		ResourceLimits: models.PursuitResourceLimits{MaxEffortHours: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	asOf := time.Now().UTC().Truncate(time.Minute)
	planning := PortfolioPlanningRequest{
		PlanID: "acceptance-plan-1", AsOf: asOf,
		HorizonStart: asOf, HorizonEnd: asOf.Add(4 * time.Hour),
		DurationMode: resourceplanner.ConservativeDuration,
		Availability: []PortfolioCapacityWindow{{Start: asOf, End: asOf.Add(4 * time.Hour)}},
		Pursuits: []PortfolioPursuitPlanningInput{
			{PursuitID: lowRisk.ID, Duration: portfolioDuration(30, 45, 60), Factors: portfolioFactors(70)},
			{PursuitID: highRisk.ID, Duration: portfolioDuration(30, 45, 60), Factors: portfolioFactors(80)},
		},
		Budget: resourceplanner.Budget{MaxCostMicros: portfolioInt64(0)},
	}
	proposal, err := svc.PlanPortfolioForOwner("alice", planning)
	if err != nil || proposal.Decision == nil {
		t.Fatalf("PlanPortfolioForOwner result=%#v err=%v", proposal, err)
	}
	request := PortfolioAllocationAcceptanceRequest{
		PlanningRequest: planning, ExpectedDecisionDigest: proposal.Decision.DecisionDigest,
		Confirmation: PortfolioAllocationConfirmation,
	}

	result, err := svc.AcceptPortfolioAllocationForOwner("alice", "alice", request)
	if err != nil {
		t.Fatalf("AcceptPortfolioAllocationForOwner: %v", err)
	}
	if result.Authority != "allocation_only" || result.CanExecute || result.Replayed || result.Allocation == nil {
		t.Fatalf("acceptance authority = %#v", result)
	}
	if result.Allocation.Status != "accepted_needs_approval" || len(result.Items) != 2 {
		t.Fatalf("accepted allocation = %#v items=%#v", result.Allocation, result.Items)
	}
	if len(repo.portfolioReservations) != 2 || len(repo.portfolioActivities) != 2 {
		t.Fatalf("durable portfolio side effects reservations=%d activities=%d", len(repo.portfolioReservations), len(repo.portfolioActivities))
	}
	for _, item := range result.Items {
		if item.DurationMinutes != 60 || item.ReservationID == uuid.Nil || item.RecordDigest == "" {
			t.Fatalf("allocation item = %#v", item)
		}
		if item.PursuitID == highRisk.ID && !item.RequiresApproval {
			t.Fatalf("high-risk item lost approval boundary: %#v", item)
		}
	}
	for _, pursuitID := range []uuid.UUID{lowRisk.ID, highRisk.ID} {
		stored, findErr := repo.FindByID(pursuitID)
		if findErr != nil || stored.Status != StatusActive || stored.PriorityScore != 50 {
			t.Fatalf("acceptance mutated pursuit %s: %#v err=%v", pursuitID, stored, findErr)
		}
	}

	replay, err := svc.AcceptPortfolioAllocationForOwner("alice", "alice", request)
	if err != nil || !replay.Replayed || replay.Allocation.ID != result.Allocation.ID {
		t.Fatalf("acceptance replay=%#v err=%v", replay, err)
	}
	if len(repo.portfolioReservations) != 2 || len(repo.portfolioActivities) != 2 {
		t.Fatalf("replay duplicated records reservations=%d activities=%d", len(repo.portfolioReservations), len(repo.portfolioActivities))
	}
}

func TestPortfolioAllocationAcceptanceFailsClosedForUnconfirmedStaleAndChangedPlans(t *testing.T) {
	repo := newPortfolioAcceptanceFakeRepository()
	svc := &service{repo: repo}
	pursuit, err := svc.Create(CreateRequest{OwnerIdentity: "alice", Title: "Bounded task", RiskLevel: "low", AutonomyLevel: "autonomous_safe"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Minute)
	planning := PortfolioPlanningRequest{
		PlanID: "acceptance-plan-2", AsOf: now, HorizonStart: now, HorizonEnd: now.Add(2 * time.Hour),
		Availability: []PortfolioCapacityWindow{{Start: now, End: now.Add(2 * time.Hour)}},
		Pursuits:     []PortfolioPursuitPlanningInput{{PursuitID: pursuit.ID, Duration: portfolioDuration(10, 20, 30), Factors: portfolioFactors(60)}},
	}
	proposal, err := svc.PlanPortfolioForOwner("alice", planning)
	if err != nil {
		t.Fatal(err)
	}
	request := PortfolioAllocationAcceptanceRequest{PlanningRequest: planning, ExpectedDecisionDigest: proposal.Decision.DecisionDigest}
	if _, err := svc.AcceptPortfolioAllocationForOwner("alice", "alice", request); err == nil || !strings.Contains(err.Error(), "confirmation") {
		t.Fatalf("missing confirmation returned %v", err)
	}
	request.Confirmation = PortfolioAllocationConfirmation
	if _, err := svc.AcceptPortfolioAllocationForOwner("alice", "mallory", request); err == nil || !strings.Contains(err.Error(), "authenticated owner") {
		t.Fatalf("actor mismatch returned %v", err)
	}
	request.PlanningRequest.AsOf = now.Add(-portfolioAllocationFreshness - time.Minute)
	if _, err := svc.AcceptPortfolioAllocationForOwner("alice", "alice", request); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale proposal returned %v", err)
	}
	request.PlanningRequest = planning
	request.PlanningRequest.Pursuits[0].Factors = portfolioFactors(90)
	if _, err := svc.AcceptPortfolioAllocationForOwner("alice", "alice", request); err == nil || !strings.Contains(err.Error(), "changed during acceptance") {
		t.Fatalf("changed proposal returned %v", err)
	}
	if len(repo.portfolioAllocations) != 0 || len(repo.portfolioReservations) != 0 {
		t.Fatalf("failed acceptance mutated durable state")
	}
}

func TestPortfolioAllocationAcceptanceHandlerUsesVerifiedOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newPortfolioAcceptanceFakeRepository()
	svc := &service{repo: repo}
	pursuit, err := svc.Create(CreateRequest{OwnerIdentity: "alice", Title: "Handler allocation", RiskLevel: "low", AutonomyLevel: "autonomous_safe"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Minute)
	planning := PortfolioPlanningRequest{
		PlanID: "handler-acceptance", AsOf: now, HorizonStart: now, HorizonEnd: now.Add(time.Hour),
		Availability: []PortfolioCapacityWindow{{Start: now, End: now.Add(time.Hour)}},
		Pursuits:     []PortfolioPursuitPlanningInput{{PursuitID: pursuit.ID, Duration: portfolioDuration(10, 15, 20), Factors: portfolioFactors(60)}},
	}
	proposal, err := svc.PlanPortfolioForOwner("alice", planning)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(PortfolioAllocationAcceptanceRequest{
		PlanningRequest: planning, ExpectedDecisionDigest: proposal.Decision.DecisionDigest,
		Confirmation: PortfolioAllocationConfirmation,
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
	router.POST("/pursuits/portfolio-plan/accept", NewHandler(svc).AcceptPortfolioAllocation)
	response := httptest.NewRecorder()
	httpRequest := httptest.NewRequest(http.MethodPost, "/pursuits/portfolio-plan/accept", strings.NewReader(string(payload)))
	httpRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, httpRequest)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"authority":"allocation_only"`) || !strings.Contains(response.Body.String(), `"canExecute":false`) {
		t.Fatalf("acceptance handler status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestPortfolioAllocationHistoryIsOwnerScopedBoundedAndNonExecutable(t *testing.T) {
	repo := newPortfolioAcceptanceFakeRepository()
	svc := &service{repo: repo}
	pursuit, err := svc.Create(CreateRequest{OwnerIdentity: "alice", Title: "History evidence", RiskLevel: "low", AutonomyLevel: "autonomous_safe"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Minute)
	planning := PortfolioPlanningRequest{
		PlanID: "history-allocation", AsOf: now, HorizonStart: now, HorizonEnd: now.Add(time.Hour),
		Availability: []PortfolioCapacityWindow{{Start: now, End: now.Add(time.Hour)}},
		Pursuits:     []PortfolioPursuitPlanningInput{{PursuitID: pursuit.ID, Duration: portfolioDuration(10, 15, 20), Factors: portfolioFactors(60)}},
	}
	proposal, err := svc.PlanPortfolioForOwner("alice", planning)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := svc.AcceptPortfolioAllocationForOwner("alice", "alice", PortfolioAllocationAcceptanceRequest{
		PlanningRequest: planning, ExpectedDecisionDigest: proposal.Decision.DecisionDigest,
		Confirmation: PortfolioAllocationConfirmation,
	})
	if err != nil {
		t.Fatal(err)
	}
	history, err := svc.PortfolioAllocationHistoryForOwner("alice", 10)
	if err != nil || len(history) != 1 {
		t.Fatalf("owner history=%#v err=%v", history, err)
	}
	if history[0].Allocation.ID != accepted.Allocation.ID || history[0].Authority != "allocation_only" || history[0].CanExecute || history[0].Replayed || len(history[0].Items) != 1 {
		t.Fatalf("history authority/evidence=%#v", history[0])
	}
	foreign, err := svc.PortfolioAllocationHistoryForOwner("mallory", 10)
	if err != nil || len(foreign) != 0 {
		t.Fatalf("foreign history=%#v err=%v", foreign, err)
	}
	if _, err := svc.PortfolioAllocationHistoryForOwner("alice", 101); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("unbounded history error=%v", err)
	}
}

func TestPortfolioAllocationHistoryHandlerUsesVerifiedOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newPortfolioAcceptanceFakeRepository()
	svc := &service{repo: repo}
	allocationID := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)
	allocation := models.PursuitPortfolioAllocation{
		ID: allocationID, OwnerIdentity: "alice", PlanID: "handler-history",
		RequestDigest: strings.Repeat("a", 64), DecisionDigest: strings.Repeat("b", 64),
		Status:       PortfolioAllocationAccepted,
		DurationMode: "expected", HorizonStart: now, HorizonEnd: now.Add(time.Hour),
		Actor: "alice", Confirmation: PortfolioAllocationConfirmation, AcceptedAt: now,
	}
	allocationDigest, err := digestPortfolioAllocation(&allocation)
	if err != nil {
		t.Fatal(err)
	}
	allocation.RecordDigest = allocationDigest
	item := models.PursuitPortfolioAllocationItem{
		ID: uuid.New(), AllocationID: allocationID, PursuitID: uuid.New(), OwnerIdentity: "alice",
		ScheduledStart: now, ScheduledEnd: now.Add(time.Hour), DurationMinutes: 60,
		ReservationID: uuid.New(), CreatedAt: now,
	}
	itemDigest, err := digestPortfolioAllocationItem(allocation.PlanID, item)
	if err != nil {
		t.Fatal(err)
	}
	item.RecordDigest = itemDigest
	repo.portfolioAllocations["alice:handler-history"] = allocation
	repo.portfolioItems[allocationID] = []models.PursuitPortfolioAllocationItem{item}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "alice")
		c.Set(identity.ContextRoleKey, string(rbac.RoleOwner))
		c.Next()
	})
	router.GET("/pursuits/portfolio-allocations", NewHandler(svc).PortfolioAllocationHistory)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/pursuits/portfolio-allocations?limit=10", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"planId":"handler-history"`) || !strings.Contains(response.Body.String(), `"authority":"allocation_only"`) || !strings.Contains(response.Body.String(), `"canExecute":false`) {
		t.Fatalf("history handler status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestPortfolioAllocationHistoryRejectsTamperedEvidence(t *testing.T) {
	repo := newPortfolioAcceptanceFakeRepository()
	svc := &service{repo: repo}
	allocationID := uuid.New()
	repo.portfolioAllocations["alice:tampered-history"] = models.PursuitPortfolioAllocation{
		ID: allocationID, OwnerIdentity: "alice", PlanID: "tampered-history",
		RequestDigest: strings.Repeat("a", 64), DecisionDigest: strings.Repeat("b", 64),
		RecordDigest: strings.Repeat("c", 64), Status: PortfolioAllocationAccepted,
		DurationMode: "expected", HorizonStart: time.Now().UTC(), HorizonEnd: time.Now().UTC().Add(time.Hour),
		Actor: "alice", Confirmation: PortfolioAllocationConfirmation, AcceptedAt: time.Now().UTC(),
	}
	repo.portfolioItems[allocationID] = []models.PursuitPortfolioAllocationItem{{
		ID: uuid.New(), AllocationID: allocationID, PursuitID: uuid.New(), OwnerIdentity: "alice",
		ScheduledStart: time.Now().UTC(), ScheduledEnd: time.Now().UTC().Add(time.Hour), DurationMinutes: 60,
		ReservationID: uuid.New(), RecordDigest: strings.Repeat("d", 64), CreatedAt: time.Now().UTC(),
	}}
	if _, err := svc.PortfolioAllocationHistoryForOwner("alice", 10); err == nil || !strings.Contains(err.Error(), "digest verification failed") {
		t.Fatalf("tampered history error=%v", err)
	}
}

type portfolioAcceptanceFakeRepository struct {
	*fakeRepo
	portfolioAllocations        map[string]models.PursuitPortfolioAllocation
	portfolioItems              map[uuid.UUID][]models.PursuitPortfolioAllocationItem
	portfolioReservations       map[uuid.UUID]models.PursuitResourceReservation
	portfolioActivities         []models.PursuitActivity
	executionProposals          map[string]models.PursuitPortfolioExecutionProposal
	executionProposalItems      map[uuid.UUID][]models.PursuitPortfolioExecutionProposalItem
	executionProposalActivities []models.PursuitActivity
	executionProposalDecisions  map[uuid.UUID][]models.PursuitPortfolioExecutionProposalDecision
	executionDecisionActivities []models.PursuitActivity
}

func newPortfolioAcceptanceFakeRepository() *portfolioAcceptanceFakeRepository {
	return &portfolioAcceptanceFakeRepository{
		fakeRepo:                    newFakeRepo(),
		portfolioAllocations:        map[string]models.PursuitPortfolioAllocation{},
		portfolioItems:              map[uuid.UUID][]models.PursuitPortfolioAllocationItem{},
		portfolioReservations:       map[uuid.UUID]models.PursuitResourceReservation{},
		portfolioActivities:         []models.PursuitActivity{},
		executionProposals:          map[string]models.PursuitPortfolioExecutionProposal{},
		executionProposalItems:      map[uuid.UUID][]models.PursuitPortfolioExecutionProposalItem{},
		executionProposalActivities: []models.PursuitActivity{},
		executionProposalDecisions:  map[uuid.UUID][]models.PursuitPortfolioExecutionProposalDecision{},
		executionDecisionActivities: []models.PursuitActivity{},
	}
}

func (r *portfolioAcceptanceFakeRepository) FindPortfolioAllocationForOwner(ownerIdentity, planID string) (*models.PursuitPortfolioAllocation, []models.PursuitPortfolioAllocationItem, error) {
	existing, ok := r.portfolioAllocations[ownerIdentity+":"+planID]
	if !ok {
		return nil, nil, nil
	}
	items := append([]models.PursuitPortfolioAllocationItem(nil), r.portfolioItems[existing.ID]...)
	return &existing, items, nil
}

func (r *portfolioAcceptanceFakeRepository) ListPortfolioAllocationsForOwner(ownerIdentity string, limit int) ([]models.PursuitPortfolioAllocation, []models.PursuitPortfolioAllocationItem, error) {
	allocations := []models.PursuitPortfolioAllocation{}
	for _, allocation := range r.portfolioAllocations {
		if allocation.OwnerIdentity == ownerIdentity {
			allocations = append(allocations, allocation)
		}
	}
	sort.Slice(allocations, func(left, right int) bool {
		if !allocations[left].AcceptedAt.Equal(allocations[right].AcceptedAt) {
			return allocations[left].AcceptedAt.After(allocations[right].AcceptedAt)
		}
		return allocations[left].ID.String() > allocations[right].ID.String()
	})
	if len(allocations) > limit {
		allocations = allocations[:limit]
	}
	items := []models.PursuitPortfolioAllocationItem{}
	for _, allocation := range allocations {
		items = append(items, r.portfolioItems[allocation.ID]...)
	}
	return allocations, items, nil
}

func (r *portfolioAcceptanceFakeRepository) SavePortfolioAllocation(
	allocation *models.PursuitPortfolioAllocation,
	items []models.PursuitPortfolioAllocationItem,
	reservations []models.PursuitResourceReservation,
	activities []models.PursuitActivity,
) (*models.PursuitPortfolioAllocation, []models.PursuitPortfolioAllocationItem, bool, error) {
	key := allocation.OwnerIdentity + ":" + allocation.PlanID
	if existing, ok := r.portfolioAllocations[key]; ok {
		if existing.RequestDigest != allocation.RequestDigest || existing.DecisionDigest != allocation.DecisionDigest || existing.RecordDigest != allocation.RecordDigest {
			return nil, nil, false, fmt.Errorf("portfolio allocation plan id was already used for a different decision")
		}
		storedItems := append([]models.PursuitPortfolioAllocationItem(nil), r.portfolioItems[existing.ID]...)
		return &existing, storedItems, false, nil
	}
	if len(items) == 0 || len(items) != len(reservations) || len(items) != len(activities) {
		return nil, nil, false, fmt.Errorf("incomplete portfolio allocation transaction")
	}
	for index := range items {
		if items[index].AllocationID != allocation.ID || items[index].OwnerIdentity != allocation.OwnerIdentity ||
			reservations[index].OwnerIdentity != allocation.OwnerIdentity || activities[index].PursuitID != items[index].PursuitID {
			return nil, nil, false, fmt.Errorf("portfolio allocation ownership mismatch")
		}
	}
	r.portfolioAllocations[key] = *allocation
	r.portfolioItems[allocation.ID] = append([]models.PursuitPortfolioAllocationItem(nil), items...)
	for index := range reservations {
		r.portfolioReservations[reservations[index].ID] = reservations[index]
		r.resourceReservations[resourceReservationFakeKey(
			reservations[index].OwnerIdentity,
			reservations[index].PursuitID,
			reservations[index].OperationID,
		)] = reservations[index]
		r.portfolioActivities = append(r.portfolioActivities, activities[index])
	}
	stored := *allocation
	return &stored, append([]models.PursuitPortfolioAllocationItem(nil), items...), true, nil
}
