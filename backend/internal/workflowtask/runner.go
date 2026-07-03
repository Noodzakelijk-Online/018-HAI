package workflowtask

import (
	"fmt"
	"strings"
	"sync"

	"automation-hub-backend/internal/task"
	"automation-hub-backend/internal/workflow"
)

type DeferredRunner struct {
	mu       sync.RWMutex
	delegate workflow.TaskRunner
}

type Runner struct {
	service task.Service
}

func NewDeferredRunner() *DeferredRunner {
	return &DeferredRunner{}
}

func (r *DeferredRunner) Set(delegate workflow.TaskRunner) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.delegate = delegate
}

func (r *DeferredRunner) RunWorkflowTask(request workflow.TaskRunRequest) (*workflow.TaskRunResult, error) {
	r.mu.RLock()
	delegate := r.delegate
	r.mu.RUnlock()
	if delegate == nil {
		return nil, fmt.Errorf("workflow task runner is not initialized")
	}
	return delegate.RunWorkflowTask(request)
}

func NewRunner(service task.Service) *Runner {
	return &Runner{service: service}
}

func DefaultRunner() (*Runner, error) {
	service, err := task.DefaultService()
	if err != nil {
		return nil, err
	}
	return NewRunner(service), nil
}

func (r *Runner) RunWorkflowTask(request workflow.TaskRunRequest) (*workflow.TaskRunResult, error) {
	plan, err := r.service.Run(task.IntakeRequest{
		Request:        request.Request,
		ProjectKey:     request.ProjectKey,
		AutomationID:   request.AutomationID,
		ExecuteAllowed: true,
		HumanApproved:  request.HumanApproved,
		ApprovalNote:   request.ApprovalNote,
	})
	if err != nil {
		return nil, err
	}
	result := &workflow.TaskRunResult{
		PlanID:           plan.ID,
		CompletionStatus: plan.CompletionStatus,
		Passed:           plan.ValidationResult.Passed,
		ReviewRequired:   plan.CompletionStatus == "review_required",
		FailureReason:    strings.Join(plan.ValidationResult.Failures, "; "),
	}
	if plan.ExecutionResult != nil {
		result.VerificationStatus = plan.ExecutionResult.VerificationStatus
		result.Output = plan.ExecutionResult.Output
		result.ExternalActionExecuted = plan.ExecutionResult.ToolExecution != nil && plan.ExecutionResult.ToolExecution.Status == "completed"
		if plan.ExecutionResult.ToolExecution != nil && strings.TrimSpace(plan.ExecutionResult.ToolExecution.LaunchEventID) != "" {
			tool := plan.ExecutionResult.ToolExecution
			result.RuntimeEvidenceURI = "automation-launch://" + strings.TrimSpace(tool.LaunchEventID)
			result.RuntimeEvidenceLabel = firstNonEmpty(
				tool.RuntimeType,
				tool.LaunchType,
				tool.Target,
				"controlled runtime launch",
			)
			result.RuntimeRouteTrace = tool.RuntimeRouteTrace
		}
		if plan.ExecutionResult.BlockedReason != "" {
			result.FailureReason = plan.ExecutionResult.BlockedReason
		}
	}
	if result.VerificationStatus == "" {
		result.VerificationStatus = plan.ValidationResult.Status
	}
	return result, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
