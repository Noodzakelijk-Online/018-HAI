package assistant

import (
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/agentcycle"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/pursuit"
	"automation-hub-backend/internal/task"
	"automation-hub-backend/internal/workflow"

	"github.com/google/uuid"
)

func TestCommandPlansAndRunsCycleForBlockerRequest(t *testing.T) {
	tasks := &fakeTaskEngine{}
	cycle := &fakeAgentCycleRunner{}
	service := NewService(tasks, cycle)

	result, err := service.Command(CommandRequest{
		Message:        "Clear blockers and follow up on open loops for 018-HAI.",
		ProjectKey:     "018-HAI",
		ExecuteAllowed: true,
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if tasks.runCalls != 1 || tasks.planCalls != 0 {
		t.Fatalf("task calls plan=%d run=%d, want run only", tasks.planCalls, tasks.runCalls)
	}
	if cycle.calls != 1 {
		t.Fatalf("agent cycle calls = %d, want 1", cycle.calls)
	}
	if result.Plan == nil || result.AgentCycle == nil {
		t.Fatalf("result did not include both plan and cycle: %#v", result)
	}
	if result.Intent != "execute_and_cycle" {
		t.Fatalf("intent = %q, want execute_and_cycle", result.Intent)
	}
	if result.NextAction != "review approval queue" {
		t.Fatalf("next action = %q, want agent-cycle next action", result.NextAction)
	}
}

func TestCommandRoutesPursuitDecisionPromptThroughAgentCycle(t *testing.T) {
	tasks := &fakeTaskEngine{}
	pursuitID := uuid.New()
	cycle := &fakeAgentCycleRunner{
		result: &agentcycle.RunResult{
			Status:      "completed",
			StartedAt:   time.Now().UTC(),
			CompletedAt: time.Now().UTC(),
			PursuitState: &agentcycle.PursuitOperatingState{
				OperatingMode:  "decision_queue",
				PrimaryLane:    "robert",
				PrimaryAction:  "Review Robert-only pursuit decisions.",
				NeedsRobert:    2,
				AttentionTotal: 2,
			},
			PursuitDecisions: []pursuit.PursuitDashboardDecision{
				{
					Pursuit: models.Pursuit{
						ID:    pursuitID,
						Title: "OpenClaw recovery",
					},
					Decision: pursuit.PursuitDecision{
						ID:               "pursuit:" + pursuitID.String() + ":next-action",
						DecisionType:     "pursuit_next_action",
						Status:           "pending",
						Recommended:      "Approve recovery workflow.",
						YesLabel:         "Create workflow",
						NoLabel:          "Revise goal",
						RequiresApproval: true,
					},
					NextAction: "Approve recovery workflow.",
				},
			},
			NextAction:    "Review Robert-only pursuit decisions.",
			SafetySummary: "approval gates remain enforced",
		},
	}
	service := NewService(tasks, cycle)

	result, err := service.Command(CommandRequest{
		Message:    "What pursuits need Robert right now?",
		ProjectKey: "018-HAI",
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if tasks.planCalls != 1 || tasks.runCalls != 0 {
		t.Fatalf("task calls plan=%d run=%d, want plan only", tasks.planCalls, tasks.runCalls)
	}
	if cycle.calls != 1 {
		t.Fatalf("agent cycle calls = %d, want 1", cycle.calls)
	}
	if result.Intent != "plan_and_cycle" {
		t.Fatalf("intent = %q, want plan_and_cycle", result.Intent)
	}
	if result.AgentCycle == nil || result.AgentCycle.PursuitState == nil {
		t.Fatalf("result missing pursuit operating state: %#v", result.AgentCycle)
	}
	if !result.ReviewRequired {
		t.Fatalf("Robert-only pursuit decisions should require review: %#v", result)
	}
	if result.NextAction != "Review Robert-only pursuit decisions." {
		t.Fatalf("next action = %q", result.NextAction)
	}
	if !strings.Contains(result.Summary, "Robert-only pursuit decision") || !strings.Contains(result.Summary, "Approve recovery workflow.") {
		t.Fatalf("summary does not expose top pursuit decision: %q", result.Summary)
	}
}

func TestCommandPlanOnlyForOrdinaryRequest(t *testing.T) {
	tasks := &fakeTaskEngine{}
	cycle := &fakeAgentCycleRunner{}
	service := NewService(tasks, cycle)

	result, err := service.Command(CommandRequest{
		Message:    "Plan the next implementation step for the dashboard.",
		ProjectKey: "018-HAI",
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if tasks.planCalls != 1 || tasks.runCalls != 0 {
		t.Fatalf("task calls plan=%d run=%d, want plan only", tasks.planCalls, tasks.runCalls)
	}
	if cycle.calls != 0 {
		t.Fatalf("agent cycle calls = %d, want 0", cycle.calls)
	}
	if result.Plan == nil || result.AgentCycle != nil {
		t.Fatalf("unexpected result engines: %#v", result)
	}
	if result.Intent != "plan" {
		t.Fatalf("intent = %q, want plan", result.Intent)
	}
}

func TestCommandPlanningSuggestsPursuitsWithoutCreatingWorkflowWork(t *testing.T) {
	tasks := &fakeTaskEngine{}
	pursuitID := uuid.New()
	router := &fakePursuitCommandRouter{matches: []pursuit.MatchCandidate{{
		Pursuit: models.Pursuit{ID: pursuitID, Title: "Dashboard recovery"},
		Score:   0.82,
		Reasons: []string{"project key matches"},
	}}}
	service := NewService(tasks, nil, router)

	result, err := service.Command(CommandRequest{
		Message:    "Plan the next dashboard recovery step.",
		ProjectKey: "018-HAI",
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if tasks.planCalls != 1 || tasks.runCalls != 0 {
		t.Fatalf("task calls plan=%d run=%d, want plan only", tasks.planCalls, tasks.runCalls)
	}
	if router.matchCalls != 1 || router.routeCalls != 0 || router.intakeCalls != 0 {
		t.Fatalf("pursuit router calls match=%d route=%d intake=%d, want match only", router.matchCalls, router.routeCalls, router.intakeCalls)
	}
	if result.Pursuit == nil || result.Pursuit.Mode != "suggested" || len(result.Pursuit.Matches) != 1 {
		t.Fatalf("pursuit context = %#v", result.Pursuit)
	}
}

func TestCommandExecutionQueuesPursuitWorkflowInsteadOfDirectTaskRun(t *testing.T) {
	tasks := &fakeTaskEngine{}
	cycle := &fakeAgentCycleRunner{}
	pursuitID := uuid.New()
	router := &fakePursuitCommandRouter{routed: &pursuit.RoutedIntakeResult{
		Mode:             "candidate_created",
		CreatedCandidate: true,
		PursuitID:        pursuitID,
		Message:          "pursuit candidate created from source-derived workflow",
		Detail:           &pursuit.PursuitDetail{Pursuit: models.Pursuit{ID: pursuitID, Title: "Local runtime recovery"}},
	}}
	service := NewService(tasks, cycle, router)

	result, err := service.Command(CommandRequest{
		Message:        "Run the local runtime recovery workflow safely.",
		ProjectKey:     "018-HAI",
		ExecuteAllowed: true,
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if tasks.planCalls != 1 || tasks.runCalls != 0 {
		t.Fatalf("task calls plan=%d run=%d, want governed plan only", tasks.planCalls, tasks.runCalls)
	}
	if router.routeCalls != 1 || router.lastRoute.SourceType != "assistant_command" || router.lastRoute.SourceID == "" {
		t.Fatalf("expected deduplicated assistant intake: %#v", router.lastRoute)
	}
	if cycle.calls != 0 {
		t.Fatalf("cycle calls = %d, want 0 because only an explicit cycle may process unrelated ready workflows", cycle.calls)
	}
	if result.Pursuit == nil || !result.Pursuit.ExecutionQueued || result.Pursuit.PursuitID != pursuitID.String() {
		t.Fatalf("pursuit context = %#v", result.Pursuit)
	}
}

func TestCommandPlanningWithinSelectedPursuitDoesNotCreateWorkflow(t *testing.T) {
	tasks := &fakeTaskEngine{}
	pursuitID := uuid.New()
	router := &fakePursuitCommandRouter{}
	service := NewService(tasks, nil, router)

	result, err := service.Command(CommandRequest{
		Message:   "Plan the evidence review.",
		PursuitID: pursuitID.String(),
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if router.matchCalls != 0 || router.routeCalls != 0 || router.intakeCalls != 0 {
		t.Fatalf("planning should not create workflow work: match=%d route=%d intake=%d", router.matchCalls, router.routeCalls, router.intakeCalls)
	}
	if result.Pursuit == nil || result.Pursuit.PursuitID != pursuitID.String() || result.Pursuit.Mode != "selected" {
		t.Fatalf("pursuit context = %#v", result.Pursuit)
	}
}

func TestCommandCanRunCycleOnlyFromExplicitFlag(t *testing.T) {
	tasks := &fakeTaskEngine{}
	cycle := &fakeAgentCycleRunner{}
	service := NewService(tasks, cycle)

	result, err := service.Command(CommandRequest{
		Message:  "Run an autonomous cycle.",
		RunCycle: true,
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if tasks.planCalls != 1 {
		t.Fatalf("explicit cycle should still create an auditable plan, planCalls=%d", tasks.planCalls)
	}
	if cycle.calls != 1 {
		t.Fatalf("agent cycle calls = %d, want 1", cycle.calls)
	}
	if result.SafetySummary == "" {
		t.Fatalf("expected safety summary")
	}
}

func TestCommandStoresBoundedRecentLogs(t *testing.T) {
	tasks := &fakeTaskEngine{}
	service := NewService(tasks, nil)

	for i := 0; i < 55; i++ {
		if _, err := service.Command(CommandRequest{Message: "Plan dashboard step"}); err != nil {
			t.Fatalf("Command %d: %v", i, err)
		}
	}

	logs := service.Logs()
	if len(logs) != 50 {
		t.Fatalf("logs = %d, want bounded 50", len(logs))
	}
	if logs[0].Summary == "" || logs[0].CreatedAt.IsZero() {
		t.Fatalf("newest log missing summary/timestamp: %#v", logs[0])
	}
}

type fakeTaskEngine struct {
	planCalls int
	runCalls  int
}

func (f *fakeTaskEngine) Plan(request task.IntakeRequest) (*task.CompletionPlan, error) {
	f.planCalls++
	return fakePlan(request, "planned"), nil
}

func (f *fakeTaskEngine) Run(request task.IntakeRequest) (*task.CompletionPlan, error) {
	f.runCalls++
	return fakePlan(request, "validated"), nil
}

func fakePlan(request task.IntakeRequest, status string) *task.CompletionPlan {
	return &task.CompletionPlan{
		ID:               "task-1",
		CreatedAt:        time.Now().UTC(),
		Request:          request.Request,
		ProjectKey:       request.ProjectKey,
		CompletionStatus: status,
		RiskAssessment: task.RiskAssessment{
			ApprovalRequired: false,
			AllowedNow:       request.ExecuteAllowed,
		},
		ValidationResult: task.ValidationResult{
			NextAction: "continue with validated task plan",
		},
	}
}

type fakeAgentCycleRunner struct {
	calls  int
	result *agentcycle.RunResult
}

type fakePursuitCommandRouter struct {
	matchCalls  int
	routeCalls  int
	intakeCalls int
	matches     []pursuit.MatchCandidate
	routed      *pursuit.RoutedIntakeResult
	lastRoute   pursuit.IntakeRequest
}

func (f *fakePursuitCommandRouter) Match(request pursuit.MatchRequest) ([]pursuit.MatchCandidate, error) {
	f.matchCalls++
	return f.matches, nil
}

func (f *fakePursuitCommandRouter) RouteIntake(request pursuit.IntakeRequest) (*pursuit.RoutedIntakeResult, error) {
	f.routeCalls++
	f.lastRoute = request
	if f.routed != nil {
		return f.routed, nil
	}
	return &pursuit.RoutedIntakeResult{Mode: "candidate_created"}, nil
}

func (f *fakePursuitCommandRouter) Intake(id uuid.UUID, request pursuit.IntakeRequest) (*pursuit.PursuitDetail, error) {
	f.intakeCalls++
	return &pursuit.PursuitDetail{Pursuit: models.Pursuit{ID: id, Title: "Selected pursuit"}}, nil
}

func (f *fakeAgentCycleRunner) Run(request agentcycle.RunRequest) *agentcycle.RunResult {
	f.calls++
	if f.result != nil {
		result := *f.result
		result.Trigger = request.Trigger
		return &result
	}
	return &agentcycle.RunResult{
		Trigger:     request.Trigger,
		Status:      "completed",
		StartedAt:   time.Now().UTC(),
		CompletedAt: time.Now().UTC(),
		Dashboard: &workflow.WorkflowDashboard{
			ApprovalItems: []models.WorkflowItem{{ID: uuid.New()}},
		},
		NextAction:    "review approval queue",
		SafetySummary: "approval gates remain enforced",
	}
}
