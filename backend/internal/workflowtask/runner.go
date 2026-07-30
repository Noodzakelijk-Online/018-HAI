package workflowtask

import (
	"fmt"
	"strings"
	"sync"

	"automation-hub-backend/internal/automation"
	"automation-hub-backend/internal/task"
	"automation-hub-backend/internal/workflow"

	"github.com/google/uuid"
)

type DeferredRunner struct {
	mu       sync.RWMutex
	delegate workflow.TaskRunner
}

type Runner struct {
	service                 task.Service
	approvalBindingPreparer automation.WorkflowApprovalBindingPreparer
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

func (r *DeferredRunner) PrepareWorkflowApprovalBinding(request workflow.WorkflowApprovalBindingRequest) (string, error) {
	r.mu.RLock()
	delegate := r.delegate
	r.mu.RUnlock()
	if delegate == nil {
		return "", fmt.Errorf("workflow task runner is not initialized")
	}
	preparer, ok := delegate.(workflow.ApprovalBindingPreparer)
	if !ok {
		return "", fmt.Errorf("workflow task runner cannot prepare automation approval bindings")
	}
	return preparer.PrepareWorkflowApprovalBinding(request)
}

func NewRunner(service task.Service, preparers ...automation.WorkflowApprovalBindingPreparer) *Runner {
	runner := &Runner{service: service}
	for _, preparer := range preparers {
		if preparer != nil {
			runner.approvalBindingPreparer = preparer
			break
		}
	}
	return runner
}

func DefaultRunner() (*Runner, error) {
	service, err := task.DefaultService()
	if err != nil {
		return nil, err
	}
	return NewRunner(service), nil
}

func (r *Runner) PrepareWorkflowApprovalBinding(request workflow.WorkflowApprovalBindingRequest) (string, error) {
	if r == nil || r.approvalBindingPreparer == nil {
		return "", fmt.Errorf("automation approval binding preparer is not configured")
	}
	request.OwnerIdentity = strings.TrimSpace(request.OwnerIdentity)
	request.WorkflowID = strings.TrimSpace(request.WorkflowID)
	request.AutomationID = strings.TrimSpace(request.AutomationID)
	request.Request = strings.TrimSpace(request.Request)
	request.ProjectKey = strings.TrimSpace(request.ProjectKey)
	if request.OwnerIdentity == "" {
		return "", fmt.Errorf("workflow approval owner identity is required")
	}
	if _, err := uuid.Parse(request.WorkflowID); err != nil {
		return "", fmt.Errorf("workflow approval workflow ID must be a UUID")
	}
	automationID, err := uuid.Parse(request.AutomationID)
	if err != nil || automationID == uuid.Nil {
		return "", fmt.Errorf("workflow approval automation ID must be a UUID")
	}
	if request.Request == "" {
		return "", fmt.Errorf("workflow approval request is required")
	}
	executionTask := task.ExecutionTask(task.IntakeRequest{
		OwnerIdentity:  request.OwnerIdentity,
		WorkflowID:     request.WorkflowID,
		Request:        request.Request,
		ProjectKey:     request.ProjectKey,
		AutomationID:   request.AutomationID,
		ExecuteAllowed: true,
		HumanApproved:  true,
	})
	return r.approvalBindingPreparer.PrepareWorkflowApprovalBinding(
		automationID,
		automation.TaskLaunchRequest{
			OwnerIdentity: request.OwnerIdentity,
			Task:          executionTask,
			ProjectKey:    request.ProjectKey,
		},
	)
}

func (r *Runner) RunWorkflowTask(request workflow.TaskRunRequest) (*workflow.TaskRunResult, error) {
	request, err := normalizeWorkflowTaskRequest(request)
	if err != nil {
		return nil, err
	}
	plan, err := r.service.Run(task.IntakeRequest{
		OwnerIdentity:    request.OwnerIdentity,
		PursuitID:        request.PursuitID,
		WorkflowID:       request.WorkflowID,
		Request:          request.Request,
		ProjectKey:       request.ProjectKey,
		AutomationID:     request.AutomationID,
		ExecuteAllowed:   true,
		HumanApproved:    request.HumanApproved,
		ApprovalNote:     request.ApprovalNote,
		ApprovalSourceID: request.ApprovalSourceID,
	})
	if err != nil {
		return nil, err
	}
	frameworkSelection, err := frameworkSelectionFromPlan(plan)
	if err != nil {
		return nil, fmt.Errorf("workflow task framework selection: %w", err)
	}
	result := &workflow.TaskRunResult{
		PlanID:             plan.ID,
		CompletionStatus:   plan.CompletionStatus,
		Passed:             plan.ValidationResult.Passed,
		ReviewRequired:     plan.CompletionStatus == "review_required",
		FailureReason:      strings.Join(plan.ValidationResult.Failures, "; "),
		FrameworkSelection: frameworkSelection,
	}
	if plan.ExecutionResult != nil {
		result.VerificationStatus = plan.ExecutionResult.VerificationStatus
		result.Output = plan.ExecutionResult.Output
		if plan.ExecutionResult.ToolExecution != nil {
			tool := plan.ExecutionResult.ToolExecution
			launchEventID := strings.TrimSpace(tool.LaunchEventID)
			if strings.EqualFold(strings.TrimSpace(tool.Status), "completed") && launchEventID == "" {
				return nil, fmt.Errorf("completed workflow external action has no immutable launch-event evidence")
			}
			if launchEventID != "" {
				eventID, parseErr := uuid.Parse(launchEventID)
				if parseErr != nil || eventID == uuid.Nil {
					return nil, fmt.Errorf("workflow external action has an invalid launch-event evidence ID")
				}
				result.RuntimeEvidenceURI = "automation-launch://" + eventID.String()
				result.RuntimeEvidenceLabel = firstNonEmpty(
					tool.RuntimeType,
					tool.LaunchType,
					tool.Target,
					"controlled runtime launch",
				)
				result.RuntimeRouteTrace = tool.RuntimeRouteTrace
			}
			result.ExternalActionExecuted = strings.EqualFold(strings.TrimSpace(tool.Status), "completed")
		}
		result.ApprovalRequired = plan.ExecutionResult.ToolExecution != nil && plan.ExecutionResult.ToolExecution.RequiresApproval
		if plan.ExecutionResult.BlockedReason != "" {
			result.FailureReason = plan.ExecutionResult.BlockedReason
		}
	}
	if result.VerificationStatus == "" {
		result.VerificationStatus = plan.ValidationResult.Status
	}
	return result, nil
}

func normalizeWorkflowTaskRequest(request workflow.TaskRunRequest) (workflow.TaskRunRequest, error) {
	request.OwnerIdentity = strings.TrimSpace(request.OwnerIdentity)
	request.WorkflowID = strings.TrimSpace(request.WorkflowID)
	request.Request = strings.TrimSpace(request.Request)
	request.ProjectKey = strings.TrimSpace(request.ProjectKey)
	request.AutomationID = strings.TrimSpace(request.AutomationID)
	request.ApprovalNote = strings.TrimSpace(request.ApprovalNote)
	request.ApprovalSourceID = strings.TrimSpace(request.ApprovalSourceID)
	if request.OwnerIdentity == "" {
		return workflow.TaskRunRequest{}, fmt.Errorf("workflow task owner identity is required")
	}
	if request.WorkflowID == "" {
		return workflow.TaskRunRequest{}, fmt.Errorf("workflow task workflow ID is required")
	}
	if request.Request == "" {
		return workflow.TaskRunRequest{}, fmt.Errorf("workflow task request is required")
	}
	if !request.HumanApproved {
		request.ApprovalNote = ""
		request.ApprovalSourceID = ""
		return request, nil
	}
	const prefix = "workflow-decision:"
	if !strings.HasPrefix(request.ApprovalSourceID, prefix) {
		return workflow.TaskRunRequest{}, fmt.Errorf("approved workflow task requires workflow-decision provenance")
	}
	decisionID, err := uuid.Parse(strings.TrimPrefix(request.ApprovalSourceID, prefix))
	if err != nil || decisionID == uuid.Nil {
		return workflow.TaskRunRequest{}, fmt.Errorf("approved workflow task requires a valid workflow decision UUID")
	}
	return request, nil
}

func frameworkSelectionFromPlan(plan *task.CompletionPlan) (*workflow.FrameworkSelectionProvenance, error) {
	if plan == nil {
		return nil, fmt.Errorf("task engine returned no plan")
	}
	if plan.FrameworkDecision == nil {
		return nil, fmt.Errorf("task plan %q has no framework selection decision", strings.TrimSpace(plan.ID))
	}
	decision := plan.FrameworkDecision
	provenance := &workflow.FrameworkSelectionProvenance{
		SelectionDecisionID:       strings.TrimSpace(decision.ID),
		TaskPlanID:                strings.TrimSpace(decision.TaskPlanID),
		CatalogVersion:            strings.TrimSpace(decision.CatalogVersion),
		CatalogDigest:             strings.TrimSpace(decision.CatalogDigest),
		SelectorAlgorithmVersion:  strings.TrimSpace(decision.SelectorAlgorithmVersion),
		EffectivePreferenceDigest: strings.TrimSpace(decision.EffectivePreferenceDigest),
		ConstitutionVersion:       decision.ConstitutionVersion,
		ConstitutionDigest:        strings.TrimSpace(decision.ConstitutionDigest),
		ConstitutionSource:        strings.TrimSpace(decision.ConstitutionSource),
	}
	if err := provenance.Validate(plan.ID); err != nil {
		return nil, err
	}
	return provenance, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
