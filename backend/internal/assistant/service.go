package assistant

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"automation-hub-backend/internal/agentcycle"
	"automation-hub-backend/internal/task"

	"github.com/google/uuid"
)

type TaskEngine interface {
	Plan(request task.IntakeRequest) (*task.CompletionPlan, error)
	Run(request task.IntakeRequest) (*task.CompletionPlan, error)
}

type AgentCycleRunner interface {
	Run(request agentcycle.RunRequest) *agentcycle.RunResult
}

type CommandRequest struct {
	Message         string   `json:"message"`
	ProjectKey      string   `json:"projectKey,omitempty"`
	AutomationID    string   `json:"automationId,omitempty"`
	SuccessCriteria []string `json:"successCriteria,omitempty"`
	ExecuteAllowed  bool     `json:"executeAllowed,omitempty"`
	RunCycle        bool     `json:"runCycle,omitempty"`
	SkipSourceSync  bool     `json:"skipSourceSync,omitempty"`
	SkipAmbient     bool     `json:"skipAmbient,omitempty"`
}

type CommandAction struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
}

type CommandResult struct {
	ID             string                `json:"id"`
	CreatedAt      time.Time             `json:"createdAt"`
	Intent         string                `json:"intent"`
	Summary        string                `json:"summary"`
	NextAction     string                `json:"nextAction"`
	SafetySummary  string                `json:"safetySummary"`
	Actions        []CommandAction       `json:"actions"`
	ReviewRequired bool                  `json:"reviewRequired"`
	Plan           *task.CompletionPlan  `json:"plan,omitempty"`
	AgentCycle     *agentcycle.RunResult `json:"agentCycle,omitempty"`
}

type Service struct {
	tasks TaskEngine
	cycle AgentCycleRunner
	mu    sync.Mutex
	logs  []CommandResult
}

func NewService(tasks TaskEngine, cycle AgentCycleRunner) *Service {
	return &Service{tasks: tasks, cycle: cycle}
}

func (s *Service) Command(request CommandRequest) (*CommandResult, error) {
	message := strings.TrimSpace(request.Message)
	if message == "" {
		message = "Run the HAI autonomous maintenance cycle and surface the next best action."
		request.RunCycle = true
	}

	intent := classifyIntent(message, request)
	result := &CommandResult{
		ID:            uuid.NewString(),
		CreatedAt:     time.Now().UTC(),
		Intent:        intent,
		Actions:       []CommandAction{},
		SafetySummary: "Assistant commands are routed through existing HAI engines; risky execution remains blocked by task risk gates, workflow approval gates, emergency stop, and runtime safety controls.",
	}

	if s.tasks == nil {
		return nil, fmt.Errorf("task engine is not configured")
	}

	taskRequest := task.IntakeRequest{
		Request:         message,
		ProjectKey:      request.ProjectKey,
		AutomationID:    request.AutomationID,
		SuccessCriteria: request.SuccessCriteria,
		ExecuteAllowed:  request.ExecuteAllowed,
	}

	var plan *task.CompletionPlan
	var err error
	if request.ExecuteAllowed {
		plan, err = s.tasks.Run(taskRequest)
		result.record("task success engine", err, "ran safe allowed task steps")
	} else {
		plan, err = s.tasks.Plan(taskRequest)
		result.record("task planner", err, "created completion-first plan")
	}
	if err != nil {
		return result, err
	}
	result.Plan = plan

	if shouldRunCycle(message, request, intent) {
		if s.cycle == nil {
			result.record("agent cycle", fmt.Errorf("agent cycle service is not configured"), "agent cycle unavailable")
		} else {
			cycle := s.cycle.Run(agentcycle.RunRequest{
				Trigger:        "assistant." + intent,
				Limit:          8,
				SkipSourceSync: request.SkipSourceSync,
				SkipAmbient:    request.SkipAmbient,
			})
			result.AgentCycle = cycle
			result.record("agent cycle", nil, cycleSummary(cycle))
		}
	}

	result.ReviewRequired = planRequiresReview(result.Plan) || cycleRequiresReview(result.AgentCycle)
	result.NextAction = deriveNextAction(result)
	result.Summary = deriveSummary(result)
	s.addLog(*result)
	return result, nil
}

func (s *Service) Logs() []CommandResult {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	copied := make([]CommandResult, len(s.logs))
	copy(copied, s.logs)
	return copied
}

func (s *Service) addLog(result CommandResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = append([]CommandResult{result}, s.logs...)
	if len(s.logs) > 50 {
		s.logs = s.logs[:50]
	}
}

func (r *CommandResult) record(name string, err error, summary string) {
	status := "completed"
	if err != nil {
		status = "failed"
		summary = err.Error()
	}
	r.Actions = append(r.Actions, CommandAction{Name: name, Status: status, Summary: summary})
}

func classifyIntent(message string, request CommandRequest) string {
	text := strings.ToLower(message)
	if request.RunCycle || shouldTriggerOperatingCycle(text) {
		if request.ExecuteAllowed {
			return "execute_and_cycle"
		}
		return "plan_and_cycle"
	}
	if request.ExecuteAllowed {
		return "execute_safe_steps"
	}
	return "plan"
}

func shouldRunCycle(message string, request CommandRequest, intent string) bool {
	if request.RunCycle {
		return true
	}
	text := strings.ToLower(message)
	return strings.Contains(intent, "cycle") ||
		shouldTriggerOperatingCycle(text)
}

func shouldTriggerOperatingCycle(text string) bool {
	return containsAny(text,
		"ambient",
		"agent cycle",
		"autonomous cycle",
		"open loop",
		"open loops",
		"follow-up",
		"follow up",
		"clear blocker",
		"clear blockers",
		"sync source",
		"sync sources",
		"what is stuck",
		"what's stuck",
		"pursuit",
		"pursuits",
		"long-running goal",
		"long running goal",
		"robert decision",
		"robert decisions",
		"needs robert",
		"need robert",
		"what needs me",
		"needs me",
		"decision queue",
		"yes/no queue",
		"yes no queue",
		"va-ready",
		"va ready",
		"system-ready",
		"system ready",
		"ready to move",
		"stale pursuit",
		"stuck pursuit",
	)
}

func deriveSummary(result *CommandResult) string {
	if result == nil {
		return ""
	}
	if result.Plan != nil && result.AgentCycle != nil {
		if len(result.AgentCycle.PursuitDecisions) > 0 {
			first := result.AgentCycle.PursuitDecisions[0]
			return fmt.Sprintf(
				"HAI prepared a task plan and found %d Robert-only pursuit decision(s). Top decision: %s",
				len(result.AgentCycle.PursuitDecisions),
				firstNonEmpty(first.Decision.Recommended, first.NextAction, first.Pursuit.Title),
			)
		}
		if state := result.AgentCycle.PursuitState; state != nil {
			return fmt.Sprintf(
				"HAI converted the command into a task success plan, refreshed pursuit operating state, and found %d item(s) needing attention in the %s lane.",
				state.AttentionTotal,
				firstNonEmpty(state.PrimaryLane, "monitor"),
			)
		}
		return "HAI converted the command into a task success plan and ran the autonomous maintenance cycle."
	}
	if result.Plan != nil {
		if result.Plan.RiskAssessment.ApprovalRequired {
			return "HAI prepared the command but left risky execution behind an approval gate."
		}
		return "HAI converted the command into a completion-first task plan."
	}
	if result.AgentCycle != nil {
		if state := result.AgentCycle.PursuitState; state != nil {
			return fmt.Sprintf("HAI refreshed pursuit operating state and found %d item(s) needing attention in the %s lane.", state.AttentionTotal, firstNonEmpty(state.PrimaryLane, "monitor"))
		}
		return "HAI ran the autonomous maintenance cycle."
	}
	return "HAI could not route the command to an engine."
}

func deriveNextAction(result *CommandResult) string {
	if result == nil {
		return ""
	}
	if result.AgentCycle != nil && strings.TrimSpace(result.AgentCycle.NextAction) != "" && result.AgentCycle.NextAction != "no immediate human action; continue scheduled monitoring" {
		return result.AgentCycle.NextAction
	}
	if result.Plan != nil {
		if strings.TrimSpace(result.Plan.ValidationResult.NextAction) != "" {
			return result.Plan.ValidationResult.NextAction
		}
		if result.Plan.RiskAssessment.ApprovalRequired && !result.Plan.RiskAssessment.ApprovalGranted {
			return "review approval queue before execution"
		}
	}
	if result.AgentCycle != nil && strings.TrimSpace(result.AgentCycle.NextAction) != "" {
		return result.AgentCycle.NextAction
	}
	return "continue scheduled monitoring"
}

func planRequiresReview(plan *task.CompletionPlan) bool {
	if plan == nil {
		return false
	}
	return plan.RiskAssessment.ApprovalRequired ||
		plan.ReviewQueueItem != nil ||
		strings.Contains(strings.ToLower(plan.CompletionStatus), "review")
}

func cycleRequiresReview(cycle *agentcycle.RunResult) bool {
	if cycle == nil {
		return false
	}
	if len(cycle.Errors) > 0 {
		return true
	}
	if cycle.Dashboard != nil {
		if len(cycle.Dashboard.ApprovalItems) > 0 || len(cycle.Dashboard.BlockedItems) > 0 {
			return true
		}
	}
	if len(cycle.PursuitDecisions) > 0 {
		return true
	}
	if cycle.PursuitState != nil {
		return cycle.PursuitState.NeedsRobert > 0 ||
			cycle.PursuitState.PlanningNeeded > 0 ||
			cycle.PursuitState.ReviewDue > 0 ||
			cycle.PursuitState.Stuck > 0
	}
	if cycle.PursuitBrief != nil {
		return cycle.PursuitBrief.NeedsRobert > 0 ||
			cycle.PursuitBrief.PlanningNeeded > 0 ||
			cycle.PursuitBrief.ReviewDue > 0 ||
			cycle.PursuitBrief.Stuck > 0
	}
	return false
}

func cycleSummary(cycle *agentcycle.RunResult) string {
	if cycle == nil {
		return "agent cycle returned no result"
	}
	parts := []string{"status " + cycle.Status}
	if cycle.SourceSync != nil {
		parts = append(parts, fmt.Sprintf("sources %d/%d completed", cycle.SourceSync.Completed, cycle.SourceSync.Checked))
	}
	if cycle.Workflows != nil {
		parts = append(parts, fmt.Sprintf("workflows checked %d", cycle.Workflows.Checked))
	}
	if cycle.AmbientScan != nil {
		parts = append(parts, fmt.Sprintf("ambient opportunities %d", cycle.AmbientScan.OpportunitiesFound))
	}
	if cycle.PursuitState != nil {
		parts = append(parts, fmt.Sprintf("pursuits lane %s attention %d Robert %d", cycle.PursuitState.PrimaryLane, cycle.PursuitState.AttentionTotal, cycle.PursuitState.NeedsRobert))
		if len(cycle.PursuitDecisions) > 0 {
			parts = append(parts, fmt.Sprintf("top decision %s", firstNonEmpty(cycle.PursuitDecisions[0].Decision.Recommended, cycle.PursuitDecisions[0].NextAction, cycle.PursuitDecisions[0].Pursuit.Title)))
		}
		return strings.Join(parts, "; ")
	}
	if cycle.PursuitBrief != nil {
		parts = append(parts, fmt.Sprintf("pursuits Robert %d planning %d stuck %d", cycle.PursuitBrief.NeedsRobert, cycle.PursuitBrief.PlanningNeeded, cycle.PursuitBrief.Stuck))
	}
	return strings.Join(parts, "; ")
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
