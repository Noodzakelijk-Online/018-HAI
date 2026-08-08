package pursuit

import (
	"automation-hub-backend/internal/lifeops"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/plangraph"
	"automation-hub-backend/internal/resourceplanner"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type PortfolioCapacityWindow struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type PortfolioPursuitPlanningInput struct {
	PursuitID      uuid.UUID                            `json:"pursuitId"`
	Duration       resourceplanner.DurationEstimate     `json:"duration"`
	EstimatedUsage resourceplanner.Usage                `json:"estimatedUsage"`
	Factors        lifeops.PriorityFactors              `json:"factors"`
	Calibration    *PortfolioEstimateCalibrationBinding `json:"calibration,omitempty"`
	Optional       bool                                 `json:"optional,omitempty"`
}

type PortfolioPlanningRequest struct {
	PlanID           string                              `json:"planId"`
	AsOf             time.Time                           `json:"asOf"`
	HorizonStart     time.Time                           `json:"horizonStart"`
	HorizonEnd       time.Time                           `json:"horizonEnd"`
	DurationMode     resourceplanner.DurationMode        `json:"durationMode,omitempty"`
	Availability     []PortfolioCapacityWindow           `json:"availability"`
	Pursuits         []PortfolioPursuitPlanningInput     `json:"pursuits"`
	Budget           resourceplanner.Budget              `json:"budget"`
	ApprovalPolicy   resourceplanner.ApprovalPolicy      `json:"approvalPolicy"`
	CoordinationPlan plangraph.AcceptedRevisionReference `json:"coordinationPlan,omitempty"`
}

type PortfolioPriority struct {
	PursuitID        uuid.UUID                    `json:"pursuitId"`
	Title            string                       `json:"title"`
	Score            int                          `json:"score"`
	Band             string                       `json:"band"`
	Factors          lifeops.PriorityFactors      `json:"factors"`
	Contributions    []lifeops.FactorContribution `json:"contributions"`
	Reasons          []string                     `json:"reasons"`
	AlgorithmVersion string                       `json:"algorithmVersion"`
}

type PortfolioExclusion struct {
	PursuitID uuid.UUID `json:"pursuitId"`
	Title     string    `json:"title"`
	Code      string    `json:"code"`
	Reason    string    `json:"reason"`
}

type PortfolioPlanningResult struct {
	PlanID             string                                       `json:"planId"`
	AsOf               time.Time                                    `json:"asOf"`
	Status             string                                       `json:"status"`
	PursuitsConsidered int                                          `json:"pursuitsConsidered"`
	PursuitsPlanned    int                                          `json:"pursuitsPlanned"`
	Priorities         []PortfolioPriority                          `json:"priorities"`
	Exclusions         []PortfolioExclusion                         `json:"exclusions"`
	Decision           *resourceplanner.Decision                    `json:"decision,omitempty"`
	Capacity           *PortfolioCapacityAssessment                 `json:"capacity,omitempty"`
	Calibrations       []PortfolioEstimateCalibrationRecommendation `json:"calibrations,omitempty"`
	Authority          string                                       `json:"authority"`
	CanExecute         bool                                         `json:"canExecute"`
	CoordinationPlan   *plangraph.AcceptedRevisionBinding           `json:"coordinationPlan,omitempty"`
}

// PlanPortfolioForOwner compares explicit estimates across the authenticated
// owner's open pursuits. It is advisory only: it does not update priorities,
// reserve capacity, consume approval, or enqueue work.
func (s *service) PlanPortfolioForOwner(ownerIdentity string, request PortfolioPlanningRequest) (*PortfolioPlanningResult, error) {
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	if ownerIdentity == "" {
		return nil, fmt.Errorf("an authenticated owner identity is required for portfolio planning")
	}
	request.PlanID = strings.TrimSpace(request.PlanID)
	if !validPortfolioPlanID(request.PlanID) {
		return nil, fmt.Errorf("portfolio plan id must contain 1 to 96 opaque identifier characters")
	}
	request.AsOf = request.AsOf.UTC().Truncate(time.Minute)
	request.HorizonStart = request.HorizonStart.UTC().Truncate(time.Minute)
	request.HorizonEnd = request.HorizonEnd.UTC().Truncate(time.Minute)
	if request.AsOf.IsZero() || request.HorizonStart.IsZero() || !request.HorizonEnd.After(request.HorizonStart) {
		return nil, fmt.Errorf("a valid asOf and planning horizon are required")
	}
	if request.AsOf.After(request.HorizonEnd) {
		return nil, fmt.Errorf("asOf must not be after the planning horizon")
	}
	if request.HorizonEnd.Sub(request.HorizonStart) > 10*365*24*time.Hour {
		return nil, fmt.Errorf("portfolio planning horizon exceeds 10 years")
	}
	if len(request.Availability) == 0 {
		return nil, fmt.Errorf("at least one explicit owner-capacity window is required")
	}
	if len(request.Availability) > 5000 {
		return nil, fmt.Errorf("portfolio availability may contain at most 5000 windows")
	}
	for index := range request.Availability {
		request.Availability[index].Start = request.Availability[index].Start.UTC().Truncate(time.Minute)
		request.Availability[index].End = request.Availability[index].End.UTC().Truncate(time.Minute)
		window := request.Availability[index]
		if window.Start.Before(request.HorizonStart) || window.End.After(request.HorizonEnd) || !window.End.After(window.Start) {
			return nil, fmt.Errorf("portfolio availability window %d is outside the planning horizon", index)
		}
	}
	if len(request.Pursuits) == 0 || len(request.Pursuits) > 500 {
		return nil, fmt.Errorf("portfolio pursuits must contain 1 to 500 explicit estimates")
	}
	coordinationPlan, err := s.resolvePortfolioCoordinationPlan(ownerIdentity, request.Pursuits, request.CoordinationPlan)
	if err != nil {
		return nil, err
	}

	open, err := s.ListForOwner(ownerIdentity, false)
	if err != nil {
		return nil, err
	}
	byID := make(map[uuid.UUID]models.Pursuit, len(open))
	for _, item := range open {
		if !pursuitClosed(item) {
			byID[item.ID] = item
		}
	}
	inputs := make(map[uuid.UUID]PortfolioPursuitPlanningInput, len(request.Pursuits))
	for _, input := range request.Pursuits {
		if input.PursuitID == uuid.Nil {
			return nil, fmt.Errorf("portfolio pursuit id is required")
		}
		if _, duplicate := inputs[input.PursuitID]; duplicate {
			return nil, fmt.Errorf("portfolio pursuit %s is duplicated", input.PursuitID)
		}
		if _, visible := byID[input.PursuitID]; !visible {
			return nil, fmt.Errorf("portfolio pursuit is unavailable")
		}
		inputs[input.PursuitID] = input
	}

	result := &PortfolioPlanningResult{
		PlanID: request.PlanID, AsOf: request.AsOf.UTC(), Status: "needs_input",
		PursuitsConsidered: len(byID), Priorities: []PortfolioPriority{},
		Exclusions: []PortfolioExclusion{}, Authority: "advisory_only", CanExecute: false,
		CoordinationPlan: coordinationPlan,
	}
	for id, item := range byID {
		if _, estimated := inputs[id]; !estimated {
			result.Exclusions = append(result.Exclusions, portfolioExclusion(item, "estimate_required", "an explicit duration, usage, and priority-factor estimate is required"))
		}
	}
	result.Calibrations, err = s.portfolioCalibrationRecommendations(ownerIdentity, inputs, byID)
	if err != nil {
		return nil, err
	}

	capacityInputs := make(map[uuid.UUID]models.Pursuit, len(inputs))
	for id := range inputs {
		capacityInputs[id] = byID[id]
	}
	capacity, availability, capacityBlocksPlanning, err := s.applyPortfolioCapacity(ownerIdentity, request, result, capacityInputs)
	if err != nil {
		return nil, err
	}
	if capacityBlocksPlanning {
		sort.Slice(result.Exclusions, func(i, j int) bool {
			if result.Exclusions[i].Code != result.Exclusions[j].Code {
				return result.Exclusions[i].Code < result.Exclusions[j].Code
			}
			return result.Exclusions[i].PursuitID.String() < result.Exclusions[j].PursuitID.String()
		})
		return result, nil
	}
	tasks := make([]resourceplanner.Task, 0, len(inputs))
	for pursuitID, input := range inputs {
		item := byID[pursuitID]
		if code, reason := portfolioPursuitBlocker(item, request.AsOf); code != "" {
			result.Exclusions = append(result.Exclusions, portfolioExclusion(item, code, reason))
			continue
		}
		usage, usageErr := s.ResourceUsageForOwner(ownerIdentity, pursuitID)
		if usageErr != nil || item.ResourceLimits.MaxEffortHours > 0 || item.ResourceLimits.MaxSpendEUR > 0 {
			if usageErr != nil || usage == nil || !usage.Available {
				result.Exclusions = append(result.Exclusions, portfolioExclusion(item, "resource_ledger_unavailable", "resource usage and active reservations cannot be verified"))
				continue
			}
		}
		if usage != nil {
			if usage.EffortLimitHours > 0 && float64(input.Duration.PessimisticMinutes) > usage.EffortRemainingHours*60+0.000001 {
				result.Exclusions = append(result.Exclusions, portfolioExclusion(item, "effort_ceiling_conflict", "the pessimistic duration exceeds the pursuit's remaining effort capacity"))
				continue
			}
			if usage.SpendLimitEUR > 0 && float64(input.EstimatedUsage.CostMicros) > usage.SpendRemainingEUR*1_000_000+0.5 {
				result.Exclusions = append(result.Exclusions, portfolioExclusion(item, "spend_ceiling_conflict", "the estimated cost exceeds the pursuit's remaining spend capacity"))
				continue
			}
		}

		assessment, assessmentErr := lifeops.EvaluatePriority(lifeops.PriorityAssessmentRequest{
			OwnerIdentity: ownerIdentity, EntityType: "pursuit", EntityID: pursuitID.String(),
			Title: item.Title, Deadline: item.TargetAt, Factors: input.Factors, Capacity: capacity,
			SourceLabel: "pursuit:portfolio_plan", SourceURI: "hai://pursuits/" + pursuitID.String(),
		}, request.AsOf)
		if assessmentErr != nil {
			return nil, fmt.Errorf("assess portfolio pursuit %s: %w", pursuitID, assessmentErr)
		}
		result.Priorities = append(result.Priorities, PortfolioPriority{
			PursuitID: pursuitID, Title: item.Title, Score: assessment.Score, Band: assessment.Band,
			Factors: assessment.Factors, Contributions: assessment.Contributions,
			Reasons: assessment.Reasons, AlgorithmVersion: assessment.AlgorithmVersion,
		})

		dependencies, dependencyCode, dependencyReason := portfolioDependencies(item, byID, inputs)
		if dependencyCode != "" {
			result.Exclusions = append(result.Exclusions, portfolioExclusion(item, dependencyCode, dependencyReason))
			continue
		}
		task := resourceplanner.Task{
			ID: pursuitID.String(), Duration: input.Duration, Dependencies: dependencies,
			Resources:      []resourceplanner.ResourceRequirement{{ResourceID: "owner-capacity", CapacityUnits: 1}},
			EstimatedUsage: input.EstimatedUsage, Priority: assessment.Score, Optional: input.Optional,
			Approval: resourceplanner.TaskApproval{Required: portfolioApprovalRequired(item), Reasons: portfolioApprovalReasons(item)},
		}
		if item.TargetAt != nil && item.TargetAt.After(request.HorizonStart) && !item.TargetAt.After(request.HorizonEnd) {
			target := item.TargetAt.UTC()
			task.Deadline = &target
			task.DeadlineKind = resourceplanner.HardDeadline
		}
		tasks = append(tasks, task)
	}

	sort.Slice(result.Priorities, func(i, j int) bool {
		if result.Priorities[i].Score != result.Priorities[j].Score {
			return result.Priorities[i].Score > result.Priorities[j].Score
		}
		return result.Priorities[i].PursuitID.String() < result.Priorities[j].PursuitID.String()
	})
	sort.Slice(result.Exclusions, func(i, j int) bool {
		if result.Exclusions[i].Code != result.Exclusions[j].Code {
			return result.Exclusions[i].Code < result.Exclusions[j].Code
		}
		return result.Exclusions[i].PursuitID.String() < result.Exclusions[j].PursuitID.String()
	})
	if len(tasks) == 0 {
		return result, nil
	}
	decision, err := resourceplanner.New().Plan(resourceplanner.Request{
		OwnerIdentity: ownerIdentity, PlanID: request.PlanID, AsOf: request.AsOf,
		HorizonStart: request.HorizonStart, HorizonEnd: request.HorizonEnd,
		DurationMode: request.DurationMode, Tasks: tasks, Availability: availability,
		Budget: request.Budget, ApprovalPolicy: request.ApprovalPolicy,
	})
	if err != nil {
		return nil, fmt.Errorf("plan pursuit portfolio: %w", err)
	}
	result.Decision = &decision
	result.PursuitsPlanned = len(tasks)
	result.Status = string(decision.Feasibility)
	return result, nil
}

func portfolioPursuitBlocker(item models.Pursuit, asOf time.Time) (string, string) {
	if item.TargetAt != nil && item.TargetAt.Before(asOf.UTC()) {
		return "target_overdue", "the pursuit target has passed and requires review before new allocation"
	}
	for _, stop := range item.StopConditions {
		if strings.EqualFold(stop.Status, StopTriggered) {
			return "stop_condition_triggered", firstNonEmpty(stop.Reason, stop.Description)
		}
	}
	if strings.EqualFold(item.Status, StatusBlocked) {
		return "pursuit_blocked", "the pursuit is blocked and must be unblocked before capacity is allocated"
	}
	if strings.EqualFold(item.Status, StatusWaiting) {
		return "pursuit_waiting", "the pursuit is waiting for input and cannot consume owner capacity yet"
	}
	return "", ""
}

func portfolioDependencies(item models.Pursuit, open map[uuid.UUID]models.Pursuit, inputs map[uuid.UUID]PortfolioPursuitPlanningInput) ([]string, string, string) {
	result := []string{}
	for _, dependency := range item.Dependencies {
		if oneOf(strings.ToLower(strings.TrimSpace(dependency.Status)), DependencySatisfied, DependencyWaived) {
			continue
		}
		if dependency.RelatedPursuitID == "" {
			return nil, "external_dependency_unresolved", firstNonEmpty(dependency.Reason, dependency.Label)
		}
		dependencyID, err := uuid.Parse(dependency.RelatedPursuitID)
		if err != nil {
			return nil, "dependency_reference_invalid", "a related pursuit dependency is not a valid identifier"
		}
		if _, exists := open[dependencyID]; !exists {
			return nil, "dependency_unavailable", "a required pursuit is not open or visible"
		}
		if _, estimated := inputs[dependencyID]; !estimated {
			return nil, "dependency_estimate_required", "a required pursuit has no portfolio estimate"
		}
		result = append(result, dependencyID.String())
	}
	sort.Strings(result)
	return result, "", ""
}

func portfolioApprovalRequired(item models.Pursuit) bool {
	return oneOf(strings.ToLower(strings.TrimSpace(item.RiskLevel)), "high", "critical") ||
		oneOf(strings.ToLower(strings.TrimSpace(item.AutonomyLevel)), "manual", "suggest", "approve_before_execute")
}

func portfolioApprovalReasons(item models.Pursuit) []string {
	if !portfolioApprovalRequired(item) {
		return nil
	}
	return []string{"pursuit risk or autonomy policy requires operator review before execution"}
}

func portfolioExclusion(item models.Pursuit, code, reason string) PortfolioExclusion {
	return PortfolioExclusion{PursuitID: item.ID, Title: item.Title, Code: code, Reason: strings.TrimSpace(reason)}
}

func validPortfolioPlanID(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > 96 {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || character == '-' || character == '_' || character == '.') {
			return false
		}
	}
	return true
}
