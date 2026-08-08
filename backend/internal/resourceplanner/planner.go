package resourceplanner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// Planner is stateless. Plan always returns an advisory decision and never
// invokes a tool, calendar, runtime, approval service, or external system.
type Planner struct{}

func New() *Planner { return &Planner{} }

type scheduledAllocation struct {
	taskID     string
	resourceID string
	start      time.Time
	end        time.Time
	units      int64
}

// Plan validates and normalizes a bounded request, builds a deterministic
// dependency-aware schedule, and returns an auditable advisory decision.
func (p *Planner) Plan(request Request) (Decision, error) {
	normalized, err := normalizeAndValidate(request)
	if err != nil {
		return Decision{}, err
	}
	inputDigest, err := canonicalDigest(normalized)
	if err != nil {
		return Decision{}, fmt.Errorf("digest normalized request: %w", err)
	}

	decision := Decision{
		PlanID:           normalized.PlanID,
		OwnerScopeDigest: normalized.OwnerScopeDigest,
		WorkspaceID:      normalized.WorkspaceID,
		AlgorithmVersion: AlgorithmVersion,
		AsOf:             normalized.AsOf,
		InputDigest:      inputDigest,
		Scheduled:        []ScheduledTask{},
		Audit:            []AuditEntry{},
		Authority:        "advisory_only",
		CanExecute:       false,
		GrantsAuthority:  false,
	}
	appendAudit(&decision, "request_normalized", normalized.PlanID, "validated deterministic planning input")

	tasks := make(map[string]Task, len(normalized.Tasks))
	for _, task := range normalized.Tasks {
		tasks[task.ID] = task
	}
	order, graphBlockers := dependencyOrder(normalized.Tasks, tasks)
	decision.CriticalBlockers = append(decision.CriticalBlockers, graphBlockers...)
	appendAudit(&decision, "dependency_graph_assessed", normalized.PlanID, fmt.Sprintf("ordered %d of %d tasks", len(order), len(normalized.Tasks)))

	usage, err := aggregateUsage(normalized.Tasks)
	if err != nil {
		return Decision{}, err
	}
	decision.Budget = assessBudget(usage, normalized.Budget)
	decision.CriticalBlockers = append(decision.CriticalBlockers, budgetBlockers(decision.Budget)...)
	appendAudit(&decision, "budget_assessed", normalized.PlanID, budgetAuditDetail(decision.Budget))

	decision.ApprovalFlags = approvalFlags(normalized.Tasks, usage, normalized.ApprovalPolicy)
	scheduled := make(map[string]ScheduledTask, len(normalized.Tasks))
	allocations := make([]scheduledAllocation, 0)
	blockedTask := blockerTaskSet(decision.CriticalBlockers)
	planningStart := normalized.HorizonStart
	if normalized.AsOf.After(planningStart) {
		planningStart = normalized.AsOf
	}

	for _, taskID := range order {
		task := tasks[taskID]
		if blockedTask[taskID] {
			decision.UnscheduledTaskIDs = append(decision.UnscheduledTaskIDs, taskID)
			appendAudit(&decision, "task_unscheduled", taskID, "dependency graph validation blocked scheduling")
			continue
		}
		dependencyReady, dependencyEnd, dependencyBlocker := dependencyReadiness(task, scheduled, tasks, planningStart)
		if !dependencyReady {
			decision.CriticalBlockers = append(decision.CriticalBlockers, dependencyBlocker)
			blockedTask[taskID] = true
			decision.UnscheduledTaskIDs = append(decision.UnscheduledTaskIDs, taskID)
			appendAudit(&decision, "task_unscheduled", taskID, dependencyBlocker.Code)
			continue
		}

		earliest := planningStart
		if task.EarliestStart != nil && task.EarliestStart.After(earliest) {
			earliest = *task.EarliestStart
		}
		if dependencyEnd.After(earliest) {
			earliest = dependencyEnd
		}
		duration, basis := selectedDuration(task.Duration, normalized.DurationMode)
		latestEnd := normalized.HorizonEnd
		if task.Deadline != nil && task.DeadlineKind == HardDeadline && task.Deadline.Before(latestEnd) {
			latestEnd = *task.Deadline
		}
		if earliest.Add(time.Duration(duration) * time.Minute).After(latestEnd) {
			blocker := Blocker{
				Code: "deadline_dependency_conflict", TaskID: taskID,
				Detail: "dependency completion or earliest start leaves insufficient time before the hard limit", BlocksFeasibility: true,
			}
			decision.CriticalBlockers = append(decision.CriticalBlockers, blocker)
			blockedTask[taskID] = true
			decision.UnscheduledTaskIDs = append(decision.UnscheduledTaskIDs, taskID)
			appendAudit(&decision, "task_unscheduled", taskID, blocker.Code)
			continue
		}

		start, reasonResource, ok := findSlot(task, earliest, latestEnd, duration, normalized.Availability, allocations)
		if !ok {
			code := "schedule_capacity_conflict"
			detail := "no simultaneous capacity slot satisfies the task duration and hard limits"
			if reasonResource != "" && !resourceHasCandidateWindow(reasonResource, task, duration, normalized.Availability) {
				code = "resource_capacity_unavailable"
				detail = "required resource has no availability window with sufficient capacity and duration"
			}
			blocker := Blocker{Code: code, TaskID: taskID, ResourceID: reasonResource, Detail: detail, BlocksFeasibility: true}
			decision.CriticalBlockers = append(decision.CriticalBlockers, blocker)
			blockedTask[taskID] = true
			decision.UnscheduledTaskIDs = append(decision.UnscheduledTaskIDs, taskID)
			appendAudit(&decision, "task_unscheduled", taskID, code)
			continue
		}

		end := start.Add(time.Duration(duration) * time.Minute)
		entry := ScheduledTask{
			TaskID: taskID, Start: start, End: end, PlannedDurationMinutes: duration,
			DependencySlackMinutes: minutesBetween(dependencyEnd, start),
			Allocations:            allocationsFor(task),
			Dependencies:           append([]string(nil), task.Dependencies...),
			DurationEstimateBasis:  basis,
			DurationUncertaintyPct: uncertaintyPercent(task.Duration),
		}
		if task.Deadline != nil {
			slack := minutesBetween(end, *task.Deadline)
			entry.DeadlineSlackMinutes = &slack
			if slack < 0 && task.DeadlineKind == SoftDeadline {
				decision.Advisories = append(decision.Advisories, Blocker{
					Code: "soft_deadline_missed", TaskID: taskID,
					Detail: "the advisory schedule ends after a soft deadline", BlocksFeasibility: false,
				})
				if normalized.ApprovalPolicy.SoftDeadlineMiss {
					decision.ApprovalFlags = append(decision.ApprovalFlags, ApprovalFlag{
						Code: "soft_deadline_miss_review", TaskID: taskID,
						Reason: "the proposed schedule misses a soft deadline", Mandatory: true,
					})
				}
			}
		}
		scheduled[taskID] = entry
		for _, requirement := range task.Resources {
			allocations = append(allocations, scheduledAllocation{taskID: taskID, resourceID: requirement.ResourceID, start: start, end: end, units: requirement.CapacityUnits})
		}
		appendAudit(&decision, "task_scheduled", taskID, fmt.Sprintf("scheduled for %d minutes", duration))
	}

	for _, task := range normalized.Tasks {
		if _, exists := scheduled[task.ID]; exists || containsString(decision.UnscheduledTaskIDs, task.ID) {
			continue
		}
		decision.UnscheduledTaskIDs = append(decision.UnscheduledTaskIDs, task.ID)
	}
	sort.Strings(decision.UnscheduledTaskIDs)
	decision.Scheduled = computeSlack(order, tasks, scheduled, normalized.HorizonEnd)
	sortBlockers(decision.CriticalBlockers)
	sortBlockers(decision.Advisories)
	sortApprovalFlags(decision.ApprovalFlags)
	issues := append(append([]Blocker(nil), decision.CriticalBlockers...), decision.Advisories...)
	decision.ReplanReasons = buildReplanReasons(issues)
	decision.FallbackStages = buildFallbackStages(issues, decision.ApprovalFlags, normalized.Tasks)
	decision.Feasibility = determineFeasibility(decision.CriticalBlockers, decision.ApprovalFlags)
	appendAudit(&decision, "decision_completed", normalized.PlanID, fmt.Sprintf("%s with %d scheduled and %d unscheduled tasks", decision.Feasibility, len(decision.Scheduled), len(decision.UnscheduledTaskIDs)))

	decision.DecisionDigest, err = decisionDigest(decision)
	if err != nil {
		return Decision{}, fmt.Errorf("digest planning decision: %w", err)
	}
	return decision, nil
}

func dependencyOrder(taskList []Task, tasks map[string]Task) ([]string, []Blocker) {
	indegree := make(map[string]int, len(taskList))
	dependents := make(map[string][]string, len(taskList))
	blockers := make([]Blocker, 0)
	for _, task := range taskList {
		indegree[task.ID] = 0
	}
	for _, task := range taskList {
		for _, dependency := range task.Dependencies {
			if _, exists := tasks[dependency]; !exists {
				blockers = append(blockers, Blocker{
					Code: "missing_dependency", TaskID: task.ID,
					Detail: "a declared dependency is absent from the planning request", BlocksFeasibility: true,
				})
				continue
			}
			indegree[task.ID]++
			dependents[dependency] = append(dependents[dependency], task.ID)
		}
	}
	ready := make([]string, 0)
	for id, count := range indegree {
		if count == 0 {
			ready = append(ready, id)
		}
	}
	order := make([]string, 0, len(taskList))
	for len(ready) > 0 {
		sort.Slice(ready, func(i, j int) bool { return taskBefore(tasks[ready[i]], tasks[ready[j]]) })
		id := ready[0]
		ready = ready[1:]
		order = append(order, id)
		sort.Strings(dependents[id])
		for _, dependent := range dependents[id] {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				ready = append(ready, dependent)
			}
		}
	}
	if len(order) != len(taskList) {
		for _, task := range taskList {
			if indegree[task.ID] > 0 {
				blockers = append(blockers, Blocker{
					Code: "dependency_cycle", TaskID: task.ID,
					Detail: "the task participates in a dependency cycle", BlocksFeasibility: true,
				})
			}
		}
	}
	sortBlockers(blockers)
	return order, blockers
}

func taskBefore(left, right Task) bool {
	leftDeadline, rightDeadline := time.Time{}, time.Time{}
	if left.Deadline != nil {
		leftDeadline = *left.Deadline
	}
	if right.Deadline != nil {
		rightDeadline = *right.Deadline
	}
	if leftDeadline.IsZero() != rightDeadline.IsZero() {
		return !leftDeadline.IsZero()
	}
	if !leftDeadline.Equal(rightDeadline) {
		return leftDeadline.Before(rightDeadline)
	}
	if left.Priority != right.Priority {
		return left.Priority > right.Priority
	}
	return left.ID < right.ID
}

func dependencyReadiness(task Task, scheduled map[string]ScheduledTask, tasks map[string]Task, horizonStart time.Time) (bool, time.Time, Blocker) {
	end := horizonStart
	for _, dependency := range task.Dependencies {
		if _, exists := tasks[dependency]; !exists {
			return false, end, Blocker{Code: "missing_dependency", TaskID: task.ID, Detail: "a declared dependency is absent from the planning request", BlocksFeasibility: true}
		}
		entry, exists := scheduled[dependency]
		if !exists {
			return false, end, Blocker{Code: "dependency_unavailable", TaskID: task.ID, Detail: "a dependency could not be scheduled", BlocksFeasibility: true}
		}
		if entry.End.After(end) {
			end = entry.End
		}
	}
	return true, end, Blocker{}
}

func selectedDuration(estimate DurationEstimate, mode DurationMode) (int64, string) {
	if mode == ConservativeDuration {
		return estimate.PessimisticMinutes, "pessimistic"
	}
	return estimate.ExpectedMinutes, "expected"
}

func findSlot(task Task, earliest, latestEnd time.Time, duration int64, windows []CapacityWindow, allocations []scheduledAllocation) (time.Time, string, bool) {
	if len(task.Resources) == 0 {
		end := earliest.Add(time.Duration(duration) * time.Minute)
		return earliest, "", !end.After(latestEnd)
	}
	candidates := []time.Time{earliest}
	for _, window := range windows {
		if !window.Start.Before(earliest) && window.Start.Before(latestEnd) {
			candidates = append(candidates, window.Start)
		}
	}
	for _, allocation := range allocations {
		if !allocation.end.Before(earliest) && allocation.end.Before(latestEnd) {
			candidates = append(candidates, allocation.end)
		}
	}
	candidates = uniqueTimes(candidates)
	for _, candidate := range candidates {
		if candidate.Before(earliest) {
			continue
		}
		end := candidate.Add(time.Duration(duration) * time.Minute)
		if end.After(latestEnd) {
			continue
		}
		fits := true
		for _, requirement := range task.Resources {
			if !capacityAvailable(requirement, candidate, end, windows, allocations) {
				fits = false
				break
			}
		}
		if fits {
			return candidate, "", true
		}
	}
	for _, requirement := range task.Resources {
		if !resourceHasCandidateWindow(requirement.ResourceID, task, duration, windows) {
			return time.Time{}, requirement.ResourceID, false
		}
	}
	return time.Time{}, task.Resources[0].ResourceID, false
}

func capacityAvailable(requirement ResourceRequirement, start, end time.Time, windows []CapacityWindow, allocations []scheduledAllocation) bool {
	boundaries := []time.Time{start, end}
	for _, window := range windows {
		if window.ResourceID != requirement.ResourceID || !intervalsOverlap(start, end, window.Start, window.End) {
			continue
		}
		if window.Start.After(start) && window.Start.Before(end) {
			boundaries = append(boundaries, window.Start)
		}
		if window.End.After(start) && window.End.Before(end) {
			boundaries = append(boundaries, window.End)
		}
	}
	for _, allocation := range allocations {
		if allocation.resourceID != requirement.ResourceID || !intervalsOverlap(start, end, allocation.start, allocation.end) {
			continue
		}
		if allocation.start.After(start) && allocation.start.Before(end) {
			boundaries = append(boundaries, allocation.start)
		}
		if allocation.end.After(start) && allocation.end.Before(end) {
			boundaries = append(boundaries, allocation.end)
		}
	}
	boundaries = uniqueTimes(boundaries)
	for index := 0; index < len(boundaries)-1; index++ {
		segmentStart := boundaries[index]
		if !boundaries[index+1].After(segmentStart) {
			continue
		}
		capacity := int64(0)
		for _, window := range windows {
			if window.ResourceID == requirement.ResourceID && !segmentStart.Before(window.Start) && segmentStart.Before(window.End) && window.CapacityUnits > capacity {
				capacity = window.CapacityUnits
			}
		}
		used := int64(0)
		for _, allocation := range allocations {
			if allocation.resourceID == requirement.ResourceID && !segmentStart.Before(allocation.start) && segmentStart.Before(allocation.end) {
				used += allocation.units
			}
		}
		if capacity-used < requirement.CapacityUnits {
			return false
		}
	}
	return true
}

func resourceHasCandidateWindow(resourceID string, task Task, duration int64, windows []CapacityWindow) bool {
	units := int64(0)
	for _, requirement := range task.Resources {
		if requirement.ResourceID == resourceID {
			units = requirement.CapacityUnits
			break
		}
	}
	for _, window := range windows {
		if window.ResourceID == resourceID && window.CapacityUnits >= units && minutesBetween(window.Start, window.End) >= duration {
			return true
		}
	}
	return false
}

func intervalsOverlap(leftStart, leftEnd, rightStart, rightEnd time.Time) bool {
	return leftStart.Before(rightEnd) && rightStart.Before(leftEnd)
}

func uniqueTimes(values []time.Time) []time.Time {
	sort.Slice(values, func(i, j int) bool { return values[i].Before(values[j]) })
	result := make([]time.Time, 0, len(values))
	for _, value := range values {
		if len(result) == 0 || !result[len(result)-1].Equal(value) {
			result = append(result, value)
		}
	}
	return result
}

func allocationsFor(task Task) []Allocation {
	result := make([]Allocation, 0, len(task.Resources))
	for _, requirement := range task.Resources {
		result = append(result, Allocation{ResourceID: requirement.ResourceID, CapacityUnits: requirement.CapacityUnits})
	}
	return result
}

func computeSlack(order []string, tasks map[string]Task, scheduled map[string]ScheduledTask, horizonEnd time.Time) []ScheduledTask {
	latestFinish := make(map[string]time.Time, len(scheduled))
	dependents := make(map[string][]string, len(scheduled))
	for id, entry := range scheduled {
		latest := horizonEnd
		if tasks[id].Deadline != nil && tasks[id].Deadline.Before(latest) {
			latest = *tasks[id].Deadline
		}
		latestFinish[id] = latest
		for _, dependency := range entry.Dependencies {
			if _, exists := scheduled[dependency]; exists {
				dependents[dependency] = append(dependents[dependency], id)
			}
		}
	}
	for index := len(order) - 1; index >= 0; index-- {
		id := order[index]
		if _, exists := scheduled[id]; !exists {
			continue
		}
		for _, dependent := range dependents[id] {
			dependentEntry := scheduled[dependent]
			candidate := latestFinish[dependent].Add(-time.Duration(dependentEntry.PlannedDurationMinutes) * time.Minute)
			if candidate.Before(latestFinish[id]) {
				latestFinish[id] = candidate
			}
		}
	}
	result := make([]ScheduledTask, 0, len(scheduled))
	for _, id := range order {
		entry, exists := scheduled[id]
		if !exists {
			continue
		}
		entry.NetworkSlackMinutes = minutesBetween(entry.End, latestFinish[id])
		entry.Critical = entry.NetworkSlackMinutes <= 0
		result = append(result, entry)
	}
	return result
}

func aggregateUsage(tasks []Task) (Usage, error) {
	var total Usage
	for _, task := range tasks {
		var ok bool
		if total.CostMicros, ok = safeAdd(total.CostMicros, task.EstimatedUsage.CostMicros); !ok {
			return Usage{}, fmt.Errorf("aggregate cost estimate overflows int64")
		}
		if total.InputTokens, ok = safeAdd(total.InputTokens, task.EstimatedUsage.InputTokens); !ok {
			return Usage{}, fmt.Errorf("aggregate input-token estimate overflows int64")
		}
		if total.OutputTokens, ok = safeAdd(total.OutputTokens, task.EstimatedUsage.OutputTokens); !ok {
			return Usage{}, fmt.Errorf("aggregate output-token estimate overflows int64")
		}
		if total.ToolCalls, ok = safeAdd(total.ToolCalls, task.EstimatedUsage.ToolCalls); !ok {
			return Usage{}, fmt.Errorf("aggregate tool-call estimate overflows int64")
		}
	}
	return total, nil
}

func safeAdd(left, right int64) (int64, bool) {
	if right > 0 && left > math.MaxInt64-right {
		return 0, false
	}
	return left + right, true
}

func assessBudget(usage Usage, budget Budget) BudgetAssessment {
	return BudgetAssessment{
		Estimated: usage, Limits: budget,
		WithinCostLimit:        within(usage.CostMicros, budget.MaxCostMicros),
		WithinInputTokenLimit:  within(usage.InputTokens, budget.MaxInputTokens),
		WithinOutputTokenLimit: within(usage.OutputTokens, budget.MaxOutputTokens),
		WithinToolCallLimit:    within(usage.ToolCalls, budget.MaxToolCalls),
	}
}

func within(value int64, limit *int64) bool { return limit == nil || value <= *limit }

func budgetBlockers(assessment BudgetAssessment) []Blocker {
	checks := []struct {
		within bool
		code   string
		detail string
	}{
		{assessment.WithinCostLimit, "cost_budget_exceeded", "estimated cost exceeds the hard cost budget"},
		{assessment.WithinInputTokenLimit, "input_token_budget_exceeded", "estimated input tokens exceed the hard token budget"},
		{assessment.WithinOutputTokenLimit, "output_token_budget_exceeded", "estimated output tokens exceed the hard token budget"},
		{assessment.WithinToolCallLimit, "tool_call_budget_exceeded", "estimated tool calls exceed the hard tool budget"},
	}
	result := make([]Blocker, 0)
	for _, check := range checks {
		if !check.within {
			result = append(result, Blocker{Code: check.code, Detail: check.detail, BlocksFeasibility: true})
		}
	}
	return result
}

func approvalFlags(tasks []Task, usage Usage, policy ApprovalPolicy) []ApprovalFlag {
	flags := make([]ApprovalFlag, 0)
	for _, task := range tasks {
		if task.Approval.Required {
			flags = append(flags, ApprovalFlag{Code: "task_approval_required", TaskID: task.ID, Reason: strings.Join(task.Approval.Reasons, "; "), Mandatory: true})
		}
		threshold := policy.UncertaintyThreshold
		if task.UncertaintyReviewPct > 0 {
			threshold = task.UncertaintyReviewPct
		}
		if threshold > 0 && uncertaintyPercent(task.Duration) >= threshold {
			flags = append(flags, ApprovalFlag{Code: "duration_uncertainty_review", TaskID: task.ID, Reason: "duration uncertainty meets the configured review threshold", Mandatory: true})
		}
	}
	thresholds := []struct {
		value     int64
		threshold *int64
		code      string
		reason    string
	}{
		{usage.CostMicros, policy.CostThresholdMicros, "cost_threshold_review", "estimated cost exceeds the review threshold"},
		{usage.InputTokens, policy.InputTokenThreshold, "input_token_threshold_review", "estimated input tokens exceed the review threshold"},
		{usage.OutputTokens, policy.OutputTokenThreshold, "output_token_threshold_review", "estimated output tokens exceed the review threshold"},
		{usage.ToolCalls, policy.ToolCallThreshold, "tool_call_threshold_review", "estimated tool calls exceed the review threshold"},
	}
	for _, threshold := range thresholds {
		if threshold.threshold != nil && threshold.value > *threshold.threshold {
			flags = append(flags, ApprovalFlag{Code: threshold.code, Reason: threshold.reason, Mandatory: true})
		}
	}
	sortApprovalFlags(flags)
	return flags
}

func uncertaintyPercent(estimate DurationEstimate) int64 {
	return (estimate.PessimisticMinutes - estimate.OptimisticMinutes) * 100 / estimate.ExpectedMinutes
}

func buildReplanReasons(blockers []Blocker) []ReplanReason {
	type group struct {
		tasks     map[string]struct{}
		resources map[string]struct{}
	}
	groups := map[string]*group{}
	for _, blocker := range blockers {
		entry := groups[blocker.Code]
		if entry == nil {
			entry = &group{tasks: map[string]struct{}{}, resources: map[string]struct{}{}}
			groups[blocker.Code] = entry
		}
		if blocker.TaskID != "" {
			entry.tasks[blocker.TaskID] = struct{}{}
		}
		if blocker.ResourceID != "" {
			entry.resources[blocker.ResourceID] = struct{}{}
		}
	}
	codes := make([]string, 0, len(groups))
	for code := range groups {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	result := make([]ReplanReason, 0, len(codes))
	for _, code := range codes {
		detail, change := replanGuidance(code)
		result = append(result, ReplanReason{
			Code: code, TaskIDs: sortedKeys(groups[code].tasks), ResourceIDs: sortedKeys(groups[code].resources),
			Detail: detail, SuggestedChange: change,
		})
	}
	return result
}

func replanGuidance(code string) (string, string) {
	switch code {
	case "missing_dependency", "dependency_cycle", "dependency_unavailable":
		return "the dependency graph cannot produce an executable order", "repair or satisfy prerequisites, then create a new decision"
	case "resource_capacity_unavailable", "schedule_capacity_conflict":
		return "declared availability cannot carry the proposed work", "add a reviewed capacity window, reduce duration, or move work"
	case "deadline_dependency_conflict", "soft_deadline_missed":
		return "the schedule conflicts with a declared deadline", "review scope, ordering, capacity, or deadline"
	case "cost_budget_exceeded", "input_token_budget_exceeded", "output_token_budget_exceeded", "tool_call_budget_exceeded":
		return "estimated usage exceeds a hard budget", "reduce scope or obtain a separately authorized budget change before replanning"
	default:
		return "the plan has an unresolved constraint", "review the constraint and submit a revised planning request"
	}
}

func buildFallbackStages(blockers []Blocker, approvals []ApprovalFlag, tasks []Task) []FallbackStage {
	stageCodes := map[int]map[string]struct{}{}
	stageTasks := map[int]map[string]struct{}{}
	add := func(stage int, code, taskID string) {
		if stageCodes[stage] == nil {
			stageCodes[stage] = map[string]struct{}{}
			stageTasks[stage] = map[string]struct{}{}
		}
		stageCodes[stage][code] = struct{}{}
		if taskID != "" {
			stageTasks[stage][taskID] = struct{}{}
		}
	}
	for _, blocker := range blockers {
		switch blocker.Code {
		case "missing_dependency", "dependency_cycle", "dependency_unavailable":
			add(1, blocker.Code, blocker.TaskID)
		case "resource_capacity_unavailable", "schedule_capacity_conflict":
			add(2, blocker.Code, blocker.TaskID)
		case "cost_budget_exceeded", "input_token_budget_exceeded", "output_token_budget_exceeded", "tool_call_budget_exceeded":
			add(3, blocker.Code, blocker.TaskID)
		case "deadline_dependency_conflict", "soft_deadline_missed":
			add(4, blocker.Code, blocker.TaskID)
		default:
			add(5, blocker.Code, blocker.TaskID)
		}
	}
	if len(approvals) > 0 {
		for _, approval := range approvals {
			add(5, approval.Code, approval.TaskID)
		}
	}
	if _, hasBudgetFallback := stageCodes[3]; hasBudgetFallback {
		for _, task := range tasks {
			if task.Optional {
				stageTasks[3][task.ID] = struct{}{}
			}
		}
	}
	descriptions := map[int]struct {
		code, description string
		requiresApproval  bool
	}{
		1: {"repair_prerequisites", "Repair missing, failed, or cyclic prerequisites before scheduling dependent work.", false},
		2: {"rebalance_capacity", "Rebalance declared availability, capacity, ordering, or duration estimates.", false},
		3: {"reduce_scope_or_review_budget", "Reduce optional scope first; any budget increase requires separate authorization.", true},
		4: {"review_deadlines", "Review scope and capacity before proposing a deadline change.", true},
		5: {"human_review", "Route unresolved risk and mandatory approval flags to an authorized reviewer.", true},
	}
	result := make([]FallbackStage, 0, len(stageCodes))
	for stage := 1; stage <= 5; stage++ {
		if len(stageCodes[stage]) == 0 {
			continue
		}
		definition := descriptions[stage]
		result = append(result, FallbackStage{
			Stage: stage, Code: definition.code, Description: definition.description,
			TriggeredBy: sortedKeys(stageCodes[stage]), AffectedTaskIDs: sortedKeys(stageTasks[stage]), RequiresApproval: definition.requiresApproval,
		})
	}
	return result
}

func determineFeasibility(blockers []Blocker, approvals []ApprovalFlag) Feasibility {
	for _, blocker := range blockers {
		if blocker.BlocksFeasibility {
			return Infeasible
		}
	}
	if len(approvals) > 0 {
		return FeasibleWithApprovals
	}
	return Feasible
}

func blockerTaskSet(blockers []Blocker) map[string]bool {
	result := map[string]bool{}
	for _, blocker := range blockers {
		if blocker.BlocksFeasibility && blocker.TaskID != "" {
			result[blocker.TaskID] = true
		}
	}
	return result
}

func budgetAuditDetail(assessment BudgetAssessment) string {
	if assessment.WithinCostLimit && assessment.WithinInputTokenLimit && assessment.WithinOutputTokenLimit && assessment.WithinToolCallLimit {
		return "all configured hard budgets are satisfied"
	}
	return "one or more configured hard budgets are exceeded"
}

func appendAudit(decision *Decision, code, subject, detail string) {
	decision.Audit = append(decision.Audit, AuditEntry{Sequence: len(decision.Audit) + 1, Code: code, Subject: subject, Detail: detail})
}

func sortBlockers(blockers []Blocker) {
	sort.SliceStable(blockers, func(i, j int) bool {
		if blockers[i].Code != blockers[j].Code {
			return blockers[i].Code < blockers[j].Code
		}
		if blockers[i].TaskID != blockers[j].TaskID {
			return blockers[i].TaskID < blockers[j].TaskID
		}
		return blockers[i].ResourceID < blockers[j].ResourceID
	})
}

func sortApprovalFlags(flags []ApprovalFlag) {
	sort.SliceStable(flags, func(i, j int) bool {
		if flags[i].Code != flags[j].Code {
			return flags[i].Code < flags[j].Code
		}
		return flags[i].TaskID < flags[j].TaskID
	})
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func canonicalDigest(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func decisionDigest(decision Decision) (string, error) {
	decision.DecisionDigest = ""
	return canonicalDigest(decision)
}
