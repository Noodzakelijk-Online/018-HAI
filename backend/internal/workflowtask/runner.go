package workflowtask

import (
	"automation-hub-backend/internal/task"
	"automation-hub-backend/internal/workflow"
	"strings"
)

type Runner struct {
	service task.Service
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
		if plan.ExecutionResult.BlockedReason != "" {
			result.FailureReason = plan.ExecutionResult.BlockedReason
		}
	}
	if result.VerificationStatus == "" {
		result.VerificationStatus = plan.ValidationResult.Status
	}
	return result, nil
}
