package pursuit

import (
	"automation-hub-backend/internal/controlledlearning"
	"automation-hub-backend/internal/lifeops"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/resourceplanner"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type fakePortfolioCapacityReader struct {
	snapshot *lifeops.CapacitySnapshot
	err      error
}

type fakePortfolioCalibrationReader struct {
	latest *controlledlearning.AppliedEstimateCalibration
	exact  *controlledlearning.AppliedEstimateCalibration
	err    error
}

func (reader fakePortfolioCalibrationReader) LatestAppliedEstimateCalibration(
	context.Context,
	string,
	string,
) (*controlledlearning.AppliedEstimateCalibration, error) {
	return reader.latest, reader.err
}

func (reader fakePortfolioCalibrationReader) AppliedEstimateCalibration(
	context.Context,
	string,
	string,
	string,
) (*controlledlearning.AppliedEstimateCalibration, error) {
	if reader.err != nil {
		return nil, reader.err
	}
	return reader.exact, nil
}

func withPortfolioCalibrationReader(value Service, reader PortfolioCalibrationReader) Service {
	if concrete, ok := value.(*service); ok {
		concrete.portfolioCalibration = reader
	}
	return value
}

func (reader fakePortfolioCapacityReader) LatestCapacity(string) (*lifeops.CapacitySnapshot, error) {
	return reader.snapshot, reader.err
}

func TestPortfolioPlanRanksSchedulesAndExplainsWithoutGrantingAuthority(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	asOf := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)
	prerequisite, err := service.Create(CreateRequest{
		OwnerIdentity: "alice", Title: "Secure required evidence", PriorityScore: 10,
		RiskLevel: "low", AutonomyLevel: "autonomous_safe",
	})
	if err != nil {
		t.Fatal(err)
	}
	dependent, err := service.Create(CreateRequest{
		OwnerIdentity: "alice", Title: "Prepare formal response", PriorityScore: 10,
		RiskLevel: "high", AutonomyLevel: "approve_before_execute",
		TargetAt: asOf.Add(7 * time.Hour).Format(time.RFC3339),
		Dependencies: []models.PursuitDependency{{
			ID: "evidence", Label: "Evidence is ready", Status: DependencyPending,
			RelatedPursuitID: prerequisite.ID.String(),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	unestimated, err := service.Create(CreateRequest{OwnerIdentity: "alice", Title: "Needs an estimate"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := service.Create(CreateRequest{OwnerIdentity: "bob", Title: "Bob private pursuit"})
	if err != nil {
		t.Fatal(err)
	}
	activityCounts := map[uuid.UUID]int{
		prerequisite.ID: len(repo.activity[prerequisite.ID]),
		dependent.ID:    len(repo.activity[dependent.ID]),
		unestimated.ID:  len(repo.activity[unestimated.ID]),
	}

	request := PortfolioPlanningRequest{
		PlanID: "portfolio-2026-08-04", AsOf: asOf,
		HorizonStart: asOf, HorizonEnd: asOf.Add(8 * time.Hour),
		DurationMode: resourceplanner.ExpectedDuration,
		Availability: []PortfolioCapacityWindow{{Start: asOf, End: asOf.Add(8 * time.Hour)}},
		Pursuits: []PortfolioPursuitPlanningInput{
			{PursuitID: prerequisite.ID, Duration: portfolioDuration(30, 60, 90), Factors: portfolioFactors(70)},
			{PursuitID: dependent.ID, Duration: portfolioDuration(30, 60, 90), Factors: portfolioFactors(80)},
		},
		Budget: resourceplanner.Budget{MaxCostMicros: portfolioInt64(0)},
	}
	first, err := service.PlanPortfolioForOwner("alice", request)
	if err != nil {
		t.Fatalf("PlanPortfolioForOwner: %v", err)
	}
	second, err := service.PlanPortfolioForOwner("alice", request)
	if err != nil {
		t.Fatalf("deterministic replay: %v", err)
	}
	if first.Decision == nil || first.Decision.DecisionDigest == "" || first.Authority != "advisory_only" || first.CanExecute || first.Decision.CanExecute || first.Decision.GrantsAuthority {
		t.Fatalf("portfolio authority boundary = %#v", first)
	}
	if first.Decision.DecisionDigest != second.Decision.DecisionDigest || !reflect.DeepEqual(first.Decision.Scheduled, second.Decision.Scheduled) {
		t.Fatalf("portfolio replay changed: first=%#v second=%#v", first.Decision, second.Decision)
	}
	if len(first.Priorities) != 2 || len(first.Priorities[0].Contributions) != 25 || first.Priorities[0].PursuitID != dependent.ID {
		t.Fatalf("multi-factor priorities = %#v", first.Priorities)
	}
	prerequisiteSchedule := portfolioScheduled(t, first.Decision.Scheduled, prerequisite.ID)
	dependentSchedule := portfolioScheduled(t, first.Decision.Scheduled, dependent.ID)
	if dependentSchedule.Start.Before(prerequisiteSchedule.End) {
		t.Fatalf("dependency order violated: prerequisite=%#v dependent=%#v", prerequisiteSchedule, dependentSchedule)
	}
	if !portfolioHasExclusion(first.Exclusions, unestimated.ID, "estimate_required") {
		t.Fatalf("missing-estimate exclusion = %#v", first.Exclusions)
	}
	if first.PursuitsConsidered != 3 || first.PursuitsPlanned != 2 {
		t.Fatalf("portfolio counts = %#v", first)
	}
	for id, before := range activityCounts {
		if len(repo.activity[id]) != before {
			t.Fatalf("advisory planning mutated pursuit %s activity", id)
		}
	}

	foreignRequest := request
	foreignRequest.Pursuits = []PortfolioPursuitPlanningInput{{PursuitID: foreign.ID, Duration: portfolioDuration(5, 10, 15), Factors: portfolioFactors(50)}}
	if _, err := service.PlanPortfolioForOwner("alice", foreignRequest); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("cross-owner portfolio input returned %v", err)
	}
}

func TestPortfolioPlanExcludesUnresolvedAndResourceBoundPursuits(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	asOf := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)
	bounded, err := service.Create(CreateRequest{
		OwnerIdentity: "alice", Title: "Bounded work",
		RiskLevel: "low", AutonomyLevel: "autonomous_safe",
		ResourceLimits: models.PursuitResourceLimits{MaxEffortHours: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendResourceEventForOwner("alice", bounded.ID, AppendPursuitResourceEventRequest{
		Kind: ResourceKindEffort, EffortHours: 0.75, Note: "Previously recorded portfolio effort",
		IdempotencyKey: "portfolio-existing-effort", Actor: "alice",
	}); err != nil {
		t.Fatal(err)
	}
	external, err := service.Create(CreateRequest{
		OwnerIdentity: "alice", Title: "Waiting on external evidence",
		RiskLevel: "low", AutonomyLevel: "autonomous_safe",
		Dependencies: []models.PursuitDependency{{ID: "external", Label: "External reply", Status: DependencyPending}},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.PlanPortfolioForOwner("alice", PortfolioPlanningRequest{
		PlanID: "portfolio-exclusions", AsOf: asOf, HorizonStart: asOf, HorizonEnd: asOf.Add(8 * time.Hour),
		Availability: []PortfolioCapacityWindow{{Start: asOf, End: asOf.Add(8 * time.Hour)}},
		Pursuits: []PortfolioPursuitPlanningInput{
			{PursuitID: bounded.ID, Duration: portfolioDuration(20, 25, 30), Factors: portfolioFactors(50)},
			{PursuitID: external.ID, Duration: portfolioDuration(20, 25, 30), Factors: portfolioFactors(50)},
		},
	})
	if err != nil {
		t.Fatalf("PlanPortfolioForOwner: %v", err)
	}
	if result.Decision != nil || result.Status != "needs_input" || result.PursuitsPlanned != 0 {
		t.Fatalf("fully excluded portfolio = %#v", result)
	}
	if !portfolioHasExclusion(result.Exclusions, bounded.ID, "effort_ceiling_conflict") ||
		!portfolioHasExclusion(result.Exclusions, external.ID, "external_dependency_unresolved") {
		t.Fatalf("portfolio exclusions = %#v", result.Exclusions)
	}
}

func TestPortfolioPlanValidatesEnvelopeEvenWhenNoPursuitCanBeScheduled(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	asOf := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)
	blocked, err := service.Create(CreateRequest{OwnerIdentity: "alice", Title: "Blocked work", Status: StatusBlocked})
	if err != nil {
		t.Fatal(err)
	}
	request := PortfolioPlanningRequest{
		PlanID: "invalid plan id", AsOf: asOf, HorizonStart: asOf, HorizonEnd: asOf.Add(time.Hour),
		Availability: []PortfolioCapacityWindow{{Start: asOf, End: asOf.Add(time.Hour)}},
		Pursuits:     []PortfolioPursuitPlanningInput{{PursuitID: blocked.ID, Duration: portfolioDuration(10, 15, 20), Factors: portfolioFactors(50)}},
	}
	if _, err := service.PlanPortfolioForOwner("alice", request); err == nil || !strings.Contains(err.Error(), "plan id") {
		t.Fatalf("invalid portfolio envelope returned %v", err)
	}
	request.PlanID = "  valid-plan  "
	trimmed, err := service.PlanPortfolioForOwner("alice", request)
	if err != nil || trimmed.PlanID != "valid-plan" {
		t.Fatalf("trimmed portfolio id result=%#v err=%v", trimmed, err)
	}
	request.PlanID = "valid-plan"
	request.Availability[0].End = request.HorizonEnd.Add(time.Minute)
	if _, err := service.PlanPortfolioForOwner("alice", request); err == nil || !strings.Contains(err.Error(), "availability window") {
		t.Fatalf("out-of-horizon availability returned %v", err)
	}
}

func TestPortfolioPlanHandlerUsesAuthenticatedOwnerAndReturnsAdvisoryDecision(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newFakeRepo()
	service := NewService(repo, nil)
	pursuit, err := service.Create(CreateRequest{OwnerIdentity: "alice", Title: "Handler portfolio", RiskLevel: "low", AutonomyLevel: "autonomous_safe"})
	if err != nil {
		t.Fatal(err)
	}
	asOf := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)
	payload, err := json.Marshal(PortfolioPlanningRequest{
		PlanID: "handler-plan", AsOf: asOf, HorizonStart: asOf, HorizonEnd: asOf.Add(time.Hour),
		Availability: []PortfolioCapacityWindow{{Start: asOf, End: asOf.Add(time.Hour)}},
		Pursuits:     []PortfolioPursuitPlanningInput{{PursuitID: pursuit.ID, Duration: portfolioDuration(10, 15, 20), Factors: portfolioFactors(60)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(identity.ContextSubjectKey, "alice"); c.Next() })
	router.POST("/pursuits/portfolio-plan", NewHandler(service).PlanPortfolio)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/pursuits/portfolio-plan", strings.NewReader(string(payload)))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"authority":"advisory_only"`) {
		t.Fatalf("portfolio handler status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestPortfolioPlanOffersApprovedCalibrationWithoutChangingExplicitEstimate(t *testing.T) {
	repo := newFakeRepo()
	asOf := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)
	calibration := portfolioAppliedCalibration("project:hai")
	service := withPortfolioCalibrationReader(
		NewService(repo, nil),
		fakePortfolioCalibrationReader{latest: calibration, exact: calibration},
	)
	pursuit, err := service.Create(CreateRequest{
		OwnerIdentity: "alice", Title: "Calibrated work", ProjectKey: "hai",
		RiskLevel: "low", AutonomyLevel: "autonomous_safe",
	})
	if err != nil {
		t.Fatal(err)
	}
	request := portfolioRequest(asOf, pursuit.ID)
	request.Pursuits[0].EstimatedUsage.CostMicros = 2_000_000
	result, err := service.PlanPortfolioForOwner("alice", request)
	if err != nil {
		t.Fatalf("PlanPortfolioForOwner: %v", err)
	}
	if result.Decision == nil || len(result.Decision.Scheduled) != 1 ||
		result.Decision.Scheduled[0].PlannedDurationMinutes != 60 {
		t.Fatalf("explicit estimate was changed by advisory calibration: %#v", result.Decision)
	}
	if len(result.Calibrations) != 1 || result.Calibrations[0].Status != "available" ||
		result.Calibrations[0].Applied || result.Calibrations[0].SuggestedExpectedMinutes != 90 ||
		result.Calibrations[0].SuggestedCostMicros != 2_500_000 {
		t.Fatalf("optional calibration recommendation = %#v", result.Calibrations)
	}
}

func TestPortfolioPlanBindsExactApprovedCalibrationOnlyAfterExplicitUse(t *testing.T) {
	repo := newFakeRepo()
	asOf := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)
	calibration := portfolioAppliedCalibration("project:hai")
	service := withPortfolioCalibrationReader(
		NewService(repo, nil),
		fakePortfolioCalibrationReader{latest: calibration, exact: calibration},
	)
	pursuit, err := service.Create(CreateRequest{OwnerIdentity: "alice", Title: "Bound estimate", ProjectKey: "hai"})
	if err != nil {
		t.Fatal(err)
	}
	request := portfolioRequest(asOf, pursuit.ID)
	sourceDuration := request.Pursuits[0].Duration
	sourceUsage := request.Pursuits[0].EstimatedUsage
	sourceUsage.CostMicros = 2_000_000
	request.Pursuits[0].Duration = portfolioDuration(45, 90, 135)
	request.Pursuits[0].EstimatedUsage = sourceUsage
	request.Pursuits[0].EstimatedUsage.CostMicros = 2_500_000
	request.Pursuits[0].Calibration = &PortfolioEstimateCalibrationBinding{
		ScopeKey: calibration.ScopeKey, ProposalID: calibration.ProposalID,
		ProposalVersion: calibration.ProposalVersion, ApplicationID: calibration.ApplicationID,
		EvidenceDigest: calibration.EvidenceDigest, SourceDuration: sourceDuration,
		SourceEstimatedUsage: sourceUsage,
	}
	result, err := service.PlanPortfolioForOwner("alice", request)
	if err != nil {
		t.Fatalf("PlanPortfolioForOwner: %v", err)
	}
	if len(result.Calibrations) != 1 || result.Calibrations[0].Status != "bound" ||
		!result.Calibrations[0].Applied || result.Decision == nil ||
		result.Decision.Scheduled[0].PlannedDurationMinutes != 90 {
		t.Fatalf("bound calibration result = %#v", result)
	}

	request.Pursuits[0].Duration.ExpectedMinutes++
	if _, err := service.PlanPortfolioForOwner("alice", request); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("changed bound estimate returned %v", err)
	}
}

func TestPortfolioPlanFailsClosedForUnverifiableBindingButNotOptionalSuggestion(t *testing.T) {
	repo := newFakeRepo()
	asOf := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)
	service := withPortfolioCalibrationReader(
		NewService(repo, nil),
		fakePortfolioCalibrationReader{err: errors.New("learning ledger offline")},
	)
	pursuit, err := service.Create(CreateRequest{OwnerIdentity: "alice", Title: "Ledger unavailable", ProjectKey: "hai"})
	if err != nil {
		t.Fatal(err)
	}
	request := portfolioRequest(asOf, pursuit.ID)
	result, err := service.PlanPortfolioForOwner("alice", request)
	if err != nil || len(result.Calibrations) != 1 || result.Calibrations[0].Status != "unavailable" || result.Decision == nil {
		t.Fatalf("optional lookup result=%#v err=%v", result, err)
	}
	request.Pursuits[0].Calibration = &PortfolioEstimateCalibrationBinding{ProposalVersion: "calibration:v1"}
	if _, err := service.PlanPortfolioForOwner("alice", request); err == nil || !strings.Contains(err.Error(), "verify portfolio calibration binding") {
		t.Fatalf("unverifiable binding returned %v", err)
	}
}

func TestPortfolioPlanRequiresDurableCapacityWhenReaderIsAttached(t *testing.T) {
	repo := newFakeRepo()
	service := WithPortfolioCapacity(NewService(repo, nil), fakePortfolioCapacityReader{err: lifeops.ErrNotFound})
	asOf := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)
	service = withPortfolioCapacityClock(service, func() time.Time { return asOf })
	pursuit, err := service.Create(CreateRequest{
		OwnerIdentity: "alice", Title: "Owner-scoped plan", RiskLevel: "low", AutonomyLevel: "autonomous_safe",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.PlanPortfolioForOwner("alice", portfolioRequest(asOf, pursuit.ID))
	if err != nil {
		t.Fatalf("PlanPortfolioForOwner: %v", err)
	}
	if result.Decision != nil || result.Status != "capacity_required" || result.Capacity == nil || result.Capacity.Status != PortfolioCapacityMissing {
		t.Fatalf("missing-capacity boundary = %#v", result)
	}
	if !portfolioHasExclusion(result.Exclusions, pursuit.ID, "capacity_snapshot_required") {
		t.Fatalf("missing-capacity exclusions = %#v", result.Exclusions)
	}
}

func TestPortfolioPlanRejectsStaleReviewAndUnavailableCapacity(t *testing.T) {
	asOf := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		mutate     func(*lifeops.CapacitySnapshot)
		wantStatus string
		wantCode   string
	}{
		{
			name: "stale", mutate: func(snapshot *lifeops.CapacitySnapshot) {
				snapshot.CapturedAt = asOf.Add(-25 * time.Hour)
			}, wantStatus: "capacity_stale", wantCode: "capacity_snapshot_stale",
		},
		{
			name: "review required", mutate: func(snapshot *lifeops.CapacitySnapshot) {
				snapshot.NeedsReview = true
			}, wantStatus: "capacity_review_required", wantCode: "capacity_review_required",
		},
		{
			name: "low confidence", mutate: func(snapshot *lifeops.CapacitySnapshot) {
				snapshot.Confidence = 0.59
			}, wantStatus: "capacity_review_required", wantCode: "capacity_review_required",
		},
		{
			name: "unavailable", mutate: func(snapshot *lifeops.CapacitySnapshot) {
				snapshot.Status = lifeops.CapacityUnavailable
			}, wantStatus: "capacity_unavailable", wantCode: "capacity_unavailable",
		},
		{
			name: "owner mismatch", mutate: func(snapshot *lifeops.CapacitySnapshot) {
				snapshot.OwnerIdentity = "bob"
			}, wantStatus: "capacity_review_required", wantCode: "capacity_owner_mismatch",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newFakeRepo()
			snapshot := freshPortfolioCapacity(asOf)
			test.mutate(snapshot)
			service := WithPortfolioCapacity(NewService(repo, nil), fakePortfolioCapacityReader{snapshot: snapshot})
			service = withPortfolioCapacityClock(service, func() time.Time { return asOf })
			pursuit, err := service.Create(CreateRequest{OwnerIdentity: "alice", Title: "Capacity-bound work"})
			if err != nil {
				t.Fatal(err)
			}
			result, err := service.PlanPortfolioForOwner("alice", portfolioRequest(asOf, pursuit.ID))
			if err != nil {
				t.Fatalf("PlanPortfolioForOwner: %v", err)
			}
			if result.Decision != nil || result.Status != test.wantStatus || result.Capacity == nil {
				t.Fatalf("capacity result = %#v", result)
			}
			if !portfolioHasExclusion(result.Exclusions, pursuit.ID, test.wantCode) {
				t.Fatalf("capacity exclusions = %#v", result.Exclusions)
			}
		})
	}
}

func TestPortfolioPlanAppliesFreshCapacityToTimeAndPriority(t *testing.T) {
	repo := newFakeRepo()
	asOf := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)
	snapshot := freshPortfolioCapacity(asOf)
	snapshot.Status = lifeops.CapacityConstrained
	snapshot.TimeAvailableMinutes = 90
	snapshot.CurrentLoad = 20
	snapshot.Signals.Energy = 40
	service := WithPortfolioCapacity(NewService(repo, nil), fakePortfolioCapacityReader{snapshot: snapshot})
	service = withPortfolioCapacityClock(service, func() time.Time { return asOf })
	pursuit, err := service.Create(CreateRequest{
		OwnerIdentity: "alice", Title: "Constrained owner plan", RiskLevel: "low", AutonomyLevel: "autonomous_safe",
	})
	if err != nil {
		t.Fatal(err)
	}
	request := portfolioRequest(asOf, pursuit.ID)
	request.Availability = []PortfolioCapacityWindow{
		{Start: asOf, End: asOf.Add(2 * time.Hour)},
		{Start: asOf.Add(time.Hour), End: asOf.Add(4 * time.Hour)},
	}
	result, err := service.PlanPortfolioForOwner("alice", request)
	if err != nil {
		t.Fatalf("PlanPortfolioForOwner: %v", err)
	}
	if result.Decision == nil || result.Capacity == nil || result.Capacity.Status != PortfolioCapacityApplied || result.Capacity.AppliedMinutes != 90 {
		t.Fatalf("applied capacity = %#v", result)
	}
	if len(result.Priorities) != 1 || result.Priorities[0].Factors.AvailableCapacity != 50 || result.Priorities[0].Factors.EnergyFit != 40 {
		t.Fatalf("capacity-overridden factors = %#v", result.Priorities)
	}
	if len(result.Decision.Scheduled) != 1 || int(result.Decision.Scheduled[0].End.Sub(result.Decision.Scheduled[0].Start)/time.Minute) > 90 {
		t.Fatalf("capacity-bounded schedule = %#v", result.Decision.Scheduled)
	}
}

func TestPortfolioPlanFailsClosedWhenCapacityLedgerFails(t *testing.T) {
	repo := newFakeRepo()
	asOf := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)
	service := WithPortfolioCapacity(NewService(repo, nil), fakePortfolioCapacityReader{err: errors.New("ledger offline")})
	pursuit, err := service.Create(CreateRequest{OwnerIdentity: "alice", Title: "Ledger-bound work"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PlanPortfolioForOwner("alice", portfolioRequest(asOf, pursuit.ID)); err == nil || !strings.Contains(err.Error(), "capacity ledger") {
		t.Fatalf("capacity ledger failure returned %v", err)
	}
}

func freshPortfolioCapacity(asOf time.Time) *lifeops.CapacitySnapshot {
	return &lifeops.CapacitySnapshot{
		ID: uuid.New(), OwnerIdentity: "alice", Status: lifeops.CapacityAvailable,
		Signals:              lifeops.CapacitySignals{Energy: 80, AttentionQuality: 80, ConfidenceReadiness: 80},
		TimeAvailableMinutes: 240, ConcurrentWorkLimit: 1, CurrentLoad: 20, PlanningStepLimit: 8,
		SourceLabel: "owner check-in", CapturedAt: asOf.Add(-time.Hour), Confidence: 0.9, Fresh: true,
	}
}

func portfolioRequest(asOf time.Time, pursuitID uuid.UUID) PortfolioPlanningRequest {
	return PortfolioPlanningRequest{
		PlanID: "capacity-bound-plan", AsOf: asOf, HorizonStart: asOf, HorizonEnd: asOf.Add(8 * time.Hour),
		Availability: []PortfolioCapacityWindow{{Start: asOf, End: asOf.Add(8 * time.Hour)}},
		Pursuits: []PortfolioPursuitPlanningInput{{
			PursuitID: pursuitID, Duration: portfolioDuration(30, 60, 90), Factors: portfolioFactors(99),
		}},
	}
}

func portfolioFactors(value int) lifeops.PriorityFactors {
	return lifeops.PriorityFactors{
		Importance: value, Urgency: value, HumanNeedAffected: value, DeadlinePressure: value,
		CostOfDelay: value, ExpectedValue: value, HarmAvoided: value, ProbabilityOfSuccess: value,
		Effort: 100 - value, Duration: 100 - value, Dependencies: 100 - value,
		Reversibility: value, Risk: value, LegalObligation: value, RelationshipConsequences: value,
		AvailableCapacity: value, EnergyFit: value, OpportunityCost: 100 - value,
		StrategicAlignment: value, LearningValue: value, CompoundingValue: value, Staleness: value,
		CommitmentAge: value, PeopleBlocked: value, Delegability: value,
	}
}

func portfolioDuration(optimistic, expected, pessimistic int64) resourceplanner.DurationEstimate {
	return resourceplanner.DurationEstimate{
		OptimisticMinutes: optimistic, ExpectedMinutes: expected, PessimisticMinutes: pessimistic,
		Basis: "operator-supplied bounded portfolio estimate",
	}
}

func portfolioInt64(value int64) *int64 { return &value }

func portfolioAppliedCalibration(scope string) *controlledlearning.AppliedEstimateCalibration {
	return &controlledlearning.AppliedEstimateCalibration{
		EstimateCalibrationDefinition: controlledlearning.EstimateCalibrationDefinition{
			Kind: "portfolio_estimate_calibration", Version: 1,
			AlgorithmVersion: "portfolio-estimate-median-mad-v1", ScopeKey: scope,
			SampleCount: 5, CostSampleCount: 5, EffortMultiplier: 1.5, CostMultiplier: 1.25,
			EffortDispersion: 0.1, CostDispersion: 0.05, Confidence: 0.7,
			EvidenceDigest:  "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			ObservedFrom:    time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC),
			ObservedThrough: time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC),
		},
		ProposalID: "calibration-proposal", ProposalVersion: "portfolio-estimate-calibration:v1",
		ApplicationID: "calibration-application", AppliedAt: time.Date(2026, 8, 2, 9, 0, 0, 0, time.UTC),
	}
}

func portfolioScheduled(t *testing.T, scheduled []resourceplanner.ScheduledTask, pursuitID uuid.UUID) resourceplanner.ScheduledTask {
	t.Helper()
	for _, item := range scheduled {
		if item.TaskID == pursuitID.String() {
			return item
		}
	}
	t.Fatalf("pursuit %s was not scheduled: %#v", pursuitID, scheduled)
	return resourceplanner.ScheduledTask{}
}

func portfolioHasExclusion(items []PortfolioExclusion, pursuitID uuid.UUID, code string) bool {
	for _, item := range items {
		if item.PursuitID == pursuitID && item.Code == code {
			return true
		}
	}
	return false
}
