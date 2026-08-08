package resourceplanner

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPlanIsDeterministicAndSchedulesDependenciesWithinCapacity(t *testing.T) {
	request := baseRequest()
	request.Tasks = []Task{
		{
			ID: "prepare", Duration: duration(45, 60, 90), Deadline: timePtr(at(12, 0)), Priority: 80,
			Resources:      []ResourceRequirement{{ResourceID: "operator", CapacityUnits: 1}},
			EstimatedUsage: Usage{InputTokens: 100, OutputTokens: 50, ToolCalls: 1},
		},
		{
			ID: "deliver", Duration: duration(50, 60, 75), Deadline: timePtr(at(13, 0)), Priority: 100,
			Dependencies: []string{"prepare"}, Resources: []ResourceRequirement{{ResourceID: "operator", CapacityUnits: 1}},
			EstimatedUsage: Usage{InputTokens: 200, OutputTokens: 100, ToolCalls: 1},
		},
		{
			ID: "triage", Duration: duration(30, 60, 60), Deadline: timePtr(at(11, 0)), Priority: 50,
			Resources: []ResourceRequirement{{ResourceID: "operator", CapacityUnits: 1}},
		},
	}
	request.Availability = []CapacityWindow{{ResourceID: "operator", Start: at(9, 0), End: at(17, 0), CapacityUnits: 1}}
	request.Budget = Budget{MaxInputTokens: int64Ptr(300), MaxOutputTokens: int64Ptr(150), MaxToolCalls: int64Ptr(2)}

	planner := New()
	first, err := planner.Plan(request)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	request.Tasks[0], request.Tasks[2] = request.Tasks[2], request.Tasks[0]
	second, err := planner.Plan(request)
	if err != nil {
		t.Fatalf("reordered plan failed: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("normalized decision changed with input order:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if first.Feasibility != Feasible || first.InputDigest == "" || len(first.InputDigest) != 64 || len(first.DecisionDigest) != 64 {
		t.Fatalf("unexpected decision identity or feasibility: %#v", first)
	}
	if first.Authority != "advisory_only" || first.CanExecute || first.GrantsAuthority {
		t.Fatalf("planner must not grant authority or execute: %#v", first)
	}
	assertSchedule(t, first.Scheduled, "triage", at(9, 0), at(10, 0))
	assertSchedule(t, first.Scheduled, "prepare", at(10, 0), at(11, 0))
	deliver := assertSchedule(t, first.Scheduled, "deliver", at(11, 0), at(12, 0))
	if deliver.DependencySlackMinutes != 0 || deliver.NetworkSlackMinutes != 60 || deliver.Critical {
		t.Fatalf("unexpected dependency/network slack: %#v", deliver)
	}
	if !first.Budget.WithinInputTokenLimit || !first.Budget.WithinOutputTokenLimit || !first.Budget.WithinToolCallLimit {
		t.Fatalf("configured budget should be satisfied: %#v", first.Budget)
	}
	for index, entry := range first.Audit {
		if entry.Sequence != index+1 {
			t.Fatalf("audit sequence is not contiguous: %#v", first.Audit)
		}
	}
}

func TestCapacityWindowsAllowBoundedParallelWorkAndDelayExcess(t *testing.T) {
	request := baseRequest()
	request.Tasks = []Task{
		{ID: "a", Duration: duration(60, 120, 120), Priority: 100, Resources: requirement("worker", 1)},
		{ID: "b", Duration: duration(60, 120, 120), Priority: 90, Resources: requirement("worker", 1)},
		{ID: "c", Duration: duration(60, 120, 120), Priority: 80, Resources: requirement("worker", 1)},
	}
	request.Availability = []CapacityWindow{{ResourceID: "worker", Start: at(9, 0), End: at(17, 0), CapacityUnits: 2}}
	decision, err := New().Plan(request)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	assertSchedule(t, decision.Scheduled, "a", at(9, 0), at(11, 0))
	assertSchedule(t, decision.Scheduled, "b", at(9, 0), at(11, 0))
	assertSchedule(t, decision.Scheduled, "c", at(11, 0), at(13, 0))
	if decision.Feasibility != Feasible || len(decision.CriticalBlockers) != 0 {
		t.Fatalf("capacity-two schedule should be feasible: %#v", decision)
	}
}

func TestOverlappingWindowsUseMaximumRatherThanInflatingCapacity(t *testing.T) {
	request := baseRequest()
	request.Tasks = []Task{
		{ID: "a", Duration: duration(60, 60, 60), Priority: 100, Resources: requirement("worker", 1)},
		{ID: "b", Duration: duration(60, 60, 60), Priority: 90, Resources: requirement("worker", 1)},
	}
	request.Availability = []CapacityWindow{
		{ResourceID: "worker", Start: at(9, 0), End: at(12, 0), CapacityUnits: 1},
		{ResourceID: "worker", Start: at(9, 0), End: at(12, 0), CapacityUnits: 1},
	}
	decision, err := New().Plan(request)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	assertSchedule(t, decision.Scheduled, "a", at(9, 0), at(10, 0))
	assertSchedule(t, decision.Scheduled, "b", at(10, 0), at(11, 0))
}

func TestPlanNeverSchedulesBeforeAsOf(t *testing.T) {
	request := baseRequest()
	request.AsOf = at(10, 15)
	request.Tasks = []Task{{ID: "current", Duration: duration(30, 30, 30), Priority: 50}}
	decision, err := New().Plan(request)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	assertSchedule(t, decision.Scheduled, "current", at(10, 15), at(10, 45))
}

func TestConservativeModeUsesPessimisticDuration(t *testing.T) {
	request := baseRequest()
	request.DurationMode = ConservativeDuration
	request.Tasks = []Task{{ID: "uncertain", Duration: duration(30, 60, 120), Priority: 50}}
	decision, err := New().Plan(request)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	entry := assertSchedule(t, decision.Scheduled, "uncertain", at(9, 0), at(11, 0))
	if entry.DurationEstimateBasis != "pessimistic" || entry.PlannedDurationMinutes != 120 || entry.DurationUncertaintyPct != 150 {
		t.Fatalf("conservative estimate not preserved: %#v", entry)
	}
}

func TestHardDeadlineAndUnavailableCapacityProduceBlockersAndStagedFallbacks(t *testing.T) {
	request := baseRequest()
	request.Tasks = []Task{
		{ID: "deadline", Duration: duration(60, 60, 60), Deadline: timePtr(at(9, 30)), Priority: 100},
		{ID: "capacity", Duration: duration(60, 60, 60), Priority: 90, Resources: requirement("specialist", 2)},
	}
	request.Availability = []CapacityWindow{{ResourceID: "specialist", Start: at(12, 0), End: at(12, 30), CapacityUnits: 1}}
	decision, err := New().Plan(request)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	if decision.Feasibility != Infeasible || !hasBlocker(decision, "deadline_dependency_conflict", "deadline") || !hasBlocker(decision, "resource_capacity_unavailable", "capacity") {
		t.Fatalf("expected deadline and capacity blockers: %#v", decision.CriticalBlockers)
	}
	if !hasFallback(decision, 2, "rebalance_capacity") || !hasFallback(decision, 4, "review_deadlines") {
		t.Fatalf("expected ordered capacity and deadline fallbacks: %#v", decision.FallbackStages)
	}
	if !reflect.DeepEqual(decision.UnscheduledTaskIDs, []string{"capacity", "deadline"}) {
		t.Fatalf("unexpected unscheduled task ids: %#v", decision.UnscheduledTaskIDs)
	}
}

func TestHardZeroBudgetBlocksAndReviewThresholdDoesNotGrantOverride(t *testing.T) {
	request := baseRequest()
	request.Tasks = []Task{{
		ID: "paid", Duration: duration(30, 30, 30), Priority: 50, Optional: true,
		EstimatedUsage: Usage{CostMicros: 1, InputTokens: 10, OutputTokens: 5, ToolCalls: 1},
		Approval:       TaskApproval{Required: true, Reasons: []string{"external financial effect"}},
	}}
	request.Budget = Budget{MaxCostMicros: int64Ptr(0)}
	request.ApprovalPolicy = ApprovalPolicy{CostThresholdMicros: int64Ptr(0)}
	decision, err := New().Plan(request)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	if decision.Feasibility != Infeasible || decision.Budget.WithinCostLimit || !hasBlocker(decision, "cost_budget_exceeded", "") {
		t.Fatalf("zero paid budget must block estimated paid usage: %#v", decision)
	}
	if !hasApproval(decision, "cost_threshold_review", "") || !hasApproval(decision, "task_approval_required", "paid") {
		t.Fatalf("expected explicit review flags: %#v", decision.ApprovalFlags)
	}
	if !hasFallback(decision, 3, "reduce_scope_or_review_budget") || !hasFallback(decision, 5, "human_review") {
		t.Fatalf("expected budget and review stages: %#v", decision.FallbackStages)
	}
	if !fallbackAffects(decision, 3, "paid") {
		t.Fatalf("budget fallback should identify optional scope: %#v", decision.FallbackStages)
	}
	if decision.CanExecute || decision.GrantsAuthority {
		t.Fatal("approval flag must not become execution authority")
	}
}

func TestSoftDeadlineMissIsReviewableButNotHardInfeasibility(t *testing.T) {
	request := baseRequest()
	request.Tasks = []Task{{
		ID: "soft", Duration: duration(120, 120, 120), Deadline: timePtr(at(10, 0)), DeadlineKind: SoftDeadline, Priority: 50,
	}}
	request.ApprovalPolicy.SoftDeadlineMiss = true
	decision, err := New().Plan(request)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	entry := assertSchedule(t, decision.Scheduled, "soft", at(9, 0), at(11, 0))
	if decision.Feasibility != FeasibleWithApprovals || !hasAdvisory(decision, "soft_deadline_missed", "soft") || !hasApproval(decision, "soft_deadline_miss_review", "soft") {
		t.Fatalf("soft miss should be scheduled and reviewable: %#v", decision)
	}
	if entry.DeadlineSlackMinutes == nil || *entry.DeadlineSlackMinutes != -60 || !entry.Critical {
		t.Fatalf("soft deadline slack was not exposed: %#v", entry)
	}
}

func TestMissingAndCyclicDependenciesAreAuditableAndPropagate(t *testing.T) {
	request := baseRequest()
	request.Tasks = []Task{
		{ID: "missing", Duration: duration(30, 30, 30), Dependencies: []string{"not-present"}},
		{ID: "cycle-a", Duration: duration(30, 30, 30), Dependencies: []string{"cycle-b"}},
		{ID: "cycle-b", Duration: duration(30, 30, 30), Dependencies: []string{"cycle-a"}},
		{ID: "downstream", Duration: duration(30, 30, 30), Dependencies: []string{"missing"}},
	}
	decision, err := New().Plan(request)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	if decision.Feasibility != Infeasible || !hasBlocker(decision, "missing_dependency", "missing") || !hasBlocker(decision, "dependency_cycle", "cycle-a") || !hasBlocker(decision, "dependency_cycle", "cycle-b") || !hasBlocker(decision, "dependency_unavailable", "downstream") {
		t.Fatalf("dependency failures not fully represented: %#v", decision.CriticalBlockers)
	}
	if !hasFallback(decision, 1, "repair_prerequisites") {
		t.Fatalf("missing prerequisite fallback: %#v", decision.FallbackStages)
	}
}

func TestApprovalAndUncertaintyFlagsYieldFeasibleWithApprovals(t *testing.T) {
	request := baseRequest()
	request.Tasks = []Task{{
		ID: "review", Duration: duration(30, 60, 120), Priority: 50,
		Approval: TaskApproval{Required: true, Reasons: []string{"Robert must decide"}},
	}}
	request.ApprovalPolicy.UncertaintyThreshold = 100
	decision, err := New().Plan(request)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	if decision.Feasibility != FeasibleWithApprovals || !hasApproval(decision, "duration_uncertainty_review", "review") || !hasApproval(decision, "task_approval_required", "review") {
		t.Fatalf("approval-only plan should remain feasible but gated: %#v", decision)
	}
}

func TestValidationRejectsAmbiguousOrUnsafeInputs(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Request)
		message string
	}{
		{"missing asOf", func(request *Request) { request.AsOf = time.Time{} }, "asOf"},
		{"unsafe id", func(request *Request) { request.Tasks[0].ID = "person@example.com" }, "opaque id"},
		{"negative budget", func(request *Request) { request.Budget.MaxCostMicros = int64Ptr(-1) }, "cannot be negative"},
		{"invalid estimate", func(request *Request) { request.Tasks[0].Duration.PessimisticMinutes = 1 }, "duration estimate"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := baseRequest()
			request.Tasks = []Task{{ID: "task", Duration: duration(10, 20, 30)}}
			test.mutate(&request)
			_, err := New().Plan(request)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("expected %q validation error, got %v", test.message, err)
			}
		})
	}
}

func TestDecisionDigestChangesWhenPlanningInputChanges(t *testing.T) {
	request := baseRequest()
	request.Tasks = []Task{{ID: "task", Duration: duration(10, 20, 30)}}
	first, err := New().Plan(request)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	request.Tasks[0].Duration.ExpectedMinutes = 21
	second, err := New().Plan(request)
	if err != nil {
		t.Fatalf("changed plan failed: %v", err)
	}
	if first.InputDigest == second.InputDigest || first.DecisionDigest == second.DecisionDigest {
		t.Fatalf("material input change did not change audit digests: %#v %#v", first, second)
	}
}

func TestPlanDoesNotMutateCallerInput(t *testing.T) {
	request := baseRequest()
	request.Tasks = []Task{
		{ID: "z", Duration: duration(10, 20, 30), Dependencies: []string{"a"}, Resources: []ResourceRequirement{{ResourceID: "z-resource", CapacityUnits: 1}, {ResourceID: "a-resource", CapacityUnits: 1}}},
		{ID: "a", Duration: duration(10, 20, 30)},
	}
	request.Availability = []CapacityWindow{
		{ResourceID: "z-resource", Start: at(10, 0), End: at(12, 0), CapacityUnits: 1},
		{ResourceID: "a-resource", Start: at(9, 0), End: at(12, 0), CapacityUnits: 1},
	}
	original := cloneRequest(request)
	if _, err := New().Plan(request); err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	if !reflect.DeepEqual(request, original) {
		t.Fatalf("planner mutated caller input:\ninput=%#v\noriginal=%#v", request, original)
	}
}

func baseRequest() Request {
	return Request{
		OwnerIdentity: "owner@example.test", WorkspaceID: "workspace-001",
		PlanID: "plan-001", AsOf: at(8, 0), HorizonStart: at(9, 0), HorizonEnd: at(17, 0),
		DurationMode: ExpectedDuration,
	}
}

func TestPlanBindsOwnerScopeWithoutExposingIdentity(t *testing.T) {
	request := baseRequest()
	request.Tasks = []Task{{ID: "bounded", Duration: duration(5, 10, 15)}}
	first, err := New().Plan(request)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	request.OwnerIdentity = "another@example.test"
	second, err := New().Plan(request)
	if err != nil {
		t.Fatalf("second plan failed: %v", err)
	}
	if first.OwnerScopeDigest == "" || first.OwnerScopeDigest == second.OwnerScopeDigest || first.InputDigest == second.InputDigest {
		t.Fatalf("owner scope was not bound: first=%#v second=%#v", first, second)
	}
}

func TestPlanRejectsMissingOwnerAndSecretPlanningText(t *testing.T) {
	request := baseRequest()
	request.Tasks = []Task{{ID: "bounded", Duration: duration(5, 10, 15)}}
	request.OwnerIdentity = ""
	if _, err := New().Plan(request); err == nil || !strings.Contains(err.Error(), "owner identity") {
		t.Fatalf("missing owner should fail closed, got %v", err)
	}
	request = baseRequest()
	request.Tasks = []Task{{ID: "bounded", Duration: DurationEstimate{OptimisticMinutes: 5, ExpectedMinutes: 10, PessimisticMinutes: 15, Basis: "token=secret-value"}}}
	if _, err := New().Plan(request); err == nil || !strings.Contains(err.Error(), "secret material") {
		t.Fatalf("secret planning text should fail closed, got %v", err)
	}
}

func at(hour, minute int) time.Time {
	return time.Date(2026, time.July, 31, hour, minute, 0, 0, time.FixedZone("CEST", 2*60*60))
}

func duration(optimistic, expected, pessimistic int64) DurationEstimate {
	return DurationEstimate{OptimisticMinutes: optimistic, ExpectedMinutes: expected, PessimisticMinutes: pessimistic, Basis: "bounded test estimate"}
}

func requirement(resourceID string, units int64) []ResourceRequirement {
	return []ResourceRequirement{{ResourceID: resourceID, CapacityUnits: units}}
}

func timePtr(value time.Time) *time.Time { return &value }
func int64Ptr(value int64) *int64        { return &value }

func assertSchedule(t *testing.T, schedule []ScheduledTask, id string, start, end time.Time) ScheduledTask {
	t.Helper()
	start, end = minuteUTC(start), minuteUTC(end)
	for _, entry := range schedule {
		if entry.TaskID == id {
			if !entry.Start.Equal(start) || !entry.End.Equal(end) {
				t.Fatalf("task %s schedule=%s-%s, want %s-%s", id, entry.Start, entry.End, start, end)
			}
			return entry
		}
	}
	t.Fatalf("task %s not found in schedule: %#v", id, schedule)
	return ScheduledTask{}
}

func hasBlocker(decision Decision, code, taskID string) bool {
	for _, blocker := range decision.CriticalBlockers {
		if blocker.Code == code && (taskID == "" || blocker.TaskID == taskID) {
			return true
		}
	}
	return false
}

func hasAdvisory(decision Decision, code, taskID string) bool {
	for _, advisory := range decision.Advisories {
		if advisory.Code == code && (taskID == "" || advisory.TaskID == taskID) {
			return true
		}
	}
	return false
}

func hasApproval(decision Decision, code, taskID string) bool {
	for _, flag := range decision.ApprovalFlags {
		if flag.Code == code && (taskID == "" || flag.TaskID == taskID) {
			return true
		}
	}
	return false
}

func hasFallback(decision Decision, stage int, code string) bool {
	for _, fallback := range decision.FallbackStages {
		if fallback.Stage == stage && fallback.Code == code {
			return true
		}
	}
	return false
}

func fallbackAffects(decision Decision, stage int, taskID string) bool {
	for _, fallback := range decision.FallbackStages {
		if fallback.Stage == stage && containsString(fallback.AffectedTaskIDs, taskID) {
			return true
		}
	}
	return false
}
