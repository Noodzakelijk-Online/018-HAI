package workflowtask

import (
	"errors"
	"strings"
	"testing"

	"automation-hub-backend/internal/automation"
	"automation-hub-backend/internal/frameworkregistry"
	"automation-hub-backend/internal/task"
	"automation-hub-backend/internal/workflow"

	"github.com/google/uuid"
)

type fakeWorkflowTaskRunner struct {
	seen workflow.TaskRunRequest
}

type capturingTaskService struct {
	request task.IntakeRequest
	plan    *task.CompletionPlan
	err     error
	runs    int
}

type capturingApprovalBindingPreparer struct {
	automationID uuid.UUID
	request      automation.TaskLaunchRequest
	binding      string
	err          error
}

func (p *capturingApprovalBindingPreparer) PrepareWorkflowApprovalBinding(id uuid.UUID, request automation.TaskLaunchRequest) (string, error) {
	p.automationID = id
	p.request = request
	return p.binding, p.err
}

func (s *capturingTaskService) Plan(task.IntakeRequest) (*task.CompletionPlan, error) {
	return nil, nil
}

func (s *capturingTaskService) Run(request task.IntakeRequest) (*task.CompletionPlan, error) {
	s.runs++
	s.request = request
	if s.err != nil {
		return nil, s.err
	}
	if s.plan != nil {
		return s.plan, nil
	}
	return validWorkflowCompletionPlan("task-plan-1"), nil
}

func (s *capturingTaskService) Logs() []task.CompletionPlan { return nil }

func (s *capturingTaskService) ReviewQueue() []task.ReviewQueueItem { return nil }

func (s *capturingTaskService) ResolveReviewItem(string, task.ApprovalDecision) (*task.ReviewResolutionResult, error) {
	return nil, nil
}

func (r *fakeWorkflowTaskRunner) RunWorkflowTask(request workflow.TaskRunRequest) (*workflow.TaskRunResult, error) {
	r.seen = request
	return &workflow.TaskRunResult{
		PlanID:             "plan-1",
		CompletionStatus:   "completed",
		VerificationStatus: "verified",
		Passed:             true,
	}, nil
}

func TestDeferredRunnerRequiresInitializedDelegate(t *testing.T) {
	runner := NewDeferredRunner()

	if _, err := runner.RunWorkflowTask(workflow.TaskRunRequest{Request: "advance workflow"}); err == nil {
		t.Fatalf("expected error before delegate is initialized")
	}
}

func TestDeferredRunnerDelegatesAfterInitialization(t *testing.T) {
	runner := NewDeferredRunner()
	delegate := &fakeWorkflowTaskRunner{}
	runner.Set(delegate)

	result, err := runner.RunWorkflowTask(workflow.TaskRunRequest{
		WorkflowID: "workflow-1",
		Request:    "advance workflow",
		ProjectKey: "018-hai",
	})
	if err != nil {
		t.Fatalf("RunWorkflowTask returned error: %v", err)
	}
	if result == nil || !result.Passed || result.VerificationStatus != "verified" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if delegate.seen.WorkflowID != "workflow-1" || delegate.seen.ProjectKey != "018-hai" {
		t.Fatalf("request was not delegated intact: %#v", delegate.seen)
	}
}

func TestRunnerPreparesBindingForExactTaskEngineExecutionIdentity(t *testing.T) {
	tasks := &capturingTaskService{}
	preparer := &capturingApprovalBindingPreparer{
		binding: "automation-action:script:" + strings.Repeat("d", 64),
	}
	runner := NewRunner(tasks, preparer)
	workflowID := uuid.NewString()
	automationID := uuid.New()
	request := workflow.WorkflowApprovalBindingRequest{
		OwnerIdentity: " alice ",
		WorkflowID:    workflowID,
		AutomationID:  automationID.String(),
		Request:       " Email the lawyer with the approved evidence summary. ",
		ProjectKey:    " 018-hai ",
	}

	binding, err := runner.PrepareWorkflowApprovalBinding(request)
	if err != nil {
		t.Fatalf("PrepareWorkflowApprovalBinding: %v", err)
	}
	if binding != preparer.binding || preparer.automationID != automationID {
		t.Fatalf("binding = %q automation = %s", binding, preparer.automationID)
	}
	expectedTask := task.ExecutionTask(task.IntakeRequest{
		OwnerIdentity:  "alice",
		WorkflowID:     workflowID,
		Request:        "Email the lawyer with the approved evidence summary.",
		ProjectKey:     "018-hai",
		AutomationID:   automationID.String(),
		ExecuteAllowed: true,
		HumanApproved:  true,
	})
	if preparer.request.OwnerIdentity != "alice" ||
		preparer.request.ProjectKey != "018-hai" ||
		preparer.request.Task != expectedTask {
		t.Fatalf("prepared exact action request = %#v, expected task %q", preparer.request, expectedTask)
	}
}

func TestDeferredRunnerDelegatesApprovalBindingPreparation(t *testing.T) {
	preparer := &capturingApprovalBindingPreparer{
		binding: "automation-action:script:" + strings.Repeat("e", 64),
	}
	delegate := NewRunner(&capturingTaskService{}, preparer)
	runner := NewDeferredRunner()
	runner.Set(delegate)

	binding, err := runner.PrepareWorkflowApprovalBinding(workflow.WorkflowApprovalBindingRequest{
		OwnerIdentity: "alice",
		WorkflowID:    uuid.NewString(),
		AutomationID:  uuid.NewString(),
		Request:       "Run the approved script.",
	})
	if err != nil {
		t.Fatalf("PrepareWorkflowApprovalBinding: %v", err)
	}
	if binding != preparer.binding {
		t.Fatalf("binding = %q, want %q", binding, preparer.binding)
	}
}

func TestRunnerPassesPursuitAndWorkflowContextToTaskEngine(t *testing.T) {
	tasks := &capturingTaskService{}
	runner := NewRunner(tasks)
	approvalSourceID := "workflow-decision:" + uuid.NewString()

	result, err := runner.RunWorkflowTask(workflow.TaskRunRequest{
		OwnerIdentity:    "alice",
		PursuitID:        "pursuit-1",
		WorkflowID:       "workflow-1",
		Request:          "Advance the governed workflow.",
		ProjectKey:       "018-hai",
		HumanApproved:    true,
		ApprovalSourceID: approvalSourceID,
	})
	if err != nil {
		t.Fatalf("RunWorkflowTask returned error: %v", err)
	}
	if result == nil || !result.Passed {
		t.Fatalf("unexpected result: %#v", result)
	}
	if tasks.request.PursuitID != "pursuit-1" || tasks.request.WorkflowID != "workflow-1" || tasks.request.OwnerIdentity != "alice" {
		t.Fatalf("task context = %#v", tasks.request)
	}
	if !tasks.request.HumanApproved || tasks.request.ApprovalSourceID != approvalSourceID {
		t.Fatalf("workflow approval provenance was not delegated intact: %#v", tasks.request)
	}
	if result.FrameworkSelection == nil ||
		result.FrameworkSelection.SelectionDecisionID == "" ||
		result.FrameworkSelection.TaskPlanID != result.PlanID ||
		result.FrameworkSelection.CatalogDigest != strings.Repeat("a", 64) ||
		result.FrameworkSelection.ConstitutionDigest != strings.Repeat("c", 64) {
		t.Fatalf("framework selection provenance = %#v", result.FrameworkSelection)
	}
}

func TestRunnerRejectsForgedWorkflowApprovalBeforeTaskEngine(t *testing.T) {
	for _, test := range []struct {
		name             string
		owner            string
		workflowID       string
		approvalSourceID string
		expectedError    string
	}{
		{
			name:             "ownerless",
			workflowID:       "workflow-1",
			approvalSourceID: "workflow-decision:" + uuid.NewString(),
			expectedError:    "owner identity is required",
		},
		{
			name:             "missing workflow",
			owner:            "alice",
			approvalSourceID: "workflow-decision:" + uuid.NewString(),
			expectedError:    "workflow ID is required",
		},
		{
			name:          "caller controlled flag only",
			owner:         "alice",
			workflowID:    "workflow-1",
			expectedError: "workflow-decision provenance",
		},
		{
			name:             "wrong provenance type",
			owner:            "alice",
			workflowID:       "workflow-1",
			approvalSourceID: "task-review:" + uuid.NewString(),
			expectedError:    "workflow-decision provenance",
		},
		{
			name:             "malformed decision",
			owner:            "alice",
			workflowID:       "workflow-1",
			approvalSourceID: "workflow-decision:forged",
			expectedError:    "valid workflow decision UUID",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			tasks := &capturingTaskService{}
			result, err := NewRunner(tasks).RunWorkflowTask(workflow.TaskRunRequest{
				OwnerIdentity:    test.owner,
				WorkflowID:       test.workflowID,
				Request:          "Perform reviewed work",
				HumanApproved:    true,
				ApprovalSourceID: test.approvalSourceID,
			})
			if err == nil || !strings.Contains(err.Error(), test.expectedError) {
				t.Fatalf("error = %v, want %q", err, test.expectedError)
			}
			if result != nil || tasks.runs != 0 {
				t.Fatalf("forged approval reached task engine: result=%#v runs=%d", result, tasks.runs)
			}
		})
	}
}

func TestRunnerStripsStaleApprovalMetadataFromUnapprovedTask(t *testing.T) {
	tasks := &capturingTaskService{}
	_, err := NewRunner(tasks).RunWorkflowTask(workflow.TaskRunRequest{
		OwnerIdentity:    " alice ",
		WorkflowID:       " workflow-1 ",
		Request:          " Continue safe planning ",
		HumanApproved:    false,
		ApprovalNote:     "stale approval",
		ApprovalSourceID: "workflow-decision:" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("RunWorkflowTask: %v", err)
	}
	if tasks.runs != 1 {
		t.Fatalf("task runs = %d, want 1", tasks.runs)
	}
	if tasks.request.HumanApproved || tasks.request.ApprovalNote != "" || tasks.request.ApprovalSourceID != "" {
		t.Fatalf("stale approval metadata reached task engine: %#v", tasks.request)
	}
	if tasks.request.OwnerIdentity != "alice" || tasks.request.WorkflowID != "workflow-1" {
		t.Fatalf("workflow identity was not normalized: %#v", tasks.request)
	}
}

func TestRunnerRejectsCompletedExternalActionWithoutImmutableEvidence(t *testing.T) {
	plan := validWorkflowCompletionPlan("task-plan-without-launch-evidence")
	plan.ExecutionResult = &task.ExecutionResult{
		VerificationStatus: "verified",
		ToolExecution: &task.ToolExecutionResult{
			Status: "completed",
		},
	}
	tasks := &capturingTaskService{plan: plan}

	result, err := NewRunner(tasks).RunWorkflowTask(workflow.TaskRunRequest{
		OwnerIdentity: "alice",
		WorkflowID:    "workflow-1",
		Request:       "Run the approved external action",
	})
	if err == nil || !strings.Contains(err.Error(), "no immutable launch-event evidence") {
		t.Fatalf("error = %v, want immutable evidence failure", err)
	}
	if result != nil {
		t.Fatalf("unaudited external completion was returned: %#v", result)
	}
}

func TestRunnerRejectsMissingFrameworkSelectionInsteadOfReportingCompletion(t *testing.T) {
	tasks := &capturingTaskService{plan: &task.CompletionPlan{
		ID:               "task-plan-without-selection",
		CompletionStatus: "validated",
		ValidationResult: task.ValidationResult{Passed: true, Status: "verified"},
	}}
	runner := NewRunner(tasks)

	result, err := runner.RunWorkflowTask(workflow.TaskRunRequest{
		OwnerIdentity: "alice",
		WorkflowID:    "workflow-1",
		Request:       "Advance workflow",
	})
	if err == nil || !strings.Contains(err.Error(), "no framework selection decision") {
		t.Fatalf("error = %v, want missing framework selection failure", err)
	}
	if result != nil {
		t.Fatalf("failed framework selection was returned as a result: %#v", result)
	}
}

func TestRunnerPropagatesFrameworkSelectionFailureWithoutCompletionResult(t *testing.T) {
	tasks := &capturingTaskService{err: errors.New("select planning frameworks: selector unavailable")}
	runner := NewRunner(tasks)

	result, err := runner.RunWorkflowTask(workflow.TaskRunRequest{
		OwnerIdentity: "alice",
		WorkflowID:    "workflow-1",
		Request:       "Advance workflow",
	})
	if err == nil || !strings.Contains(err.Error(), "selector unavailable") {
		t.Fatalf("error = %v, want selector failure", err)
	}
	if result != nil {
		t.Fatalf("selection failure was described as complete: %#v", result)
	}
}

func validWorkflowCompletionPlan(planID string) *task.CompletionPlan {
	return &task.CompletionPlan{
		ID:               planID,
		CompletionStatus: "validated",
		ValidationResult: task.ValidationResult{Passed: true, Status: "verified"},
		FrameworkDecision: &frameworkregistry.SelectionDecision{
			ID:                        uuid.NewString(),
			TaskPlanID:                planID,
			CatalogVersion:            "v1",
			CatalogDigest:             strings.Repeat("a", 64),
			SelectorAlgorithmVersion:  "chief-of-staff-v1",
			EffectivePreferenceDigest: strings.Repeat("b", 64),
			ConstitutionVersion:       1,
			ConstitutionDigest:        strings.Repeat("c", 64),
			ConstitutionSource:        "builtin-robert-constitution-v1:v1",
		},
	}
}
