package workflowtask

import (
	"testing"

	"automation-hub-backend/internal/task"
	"automation-hub-backend/internal/workflow"
)

type fakeWorkflowTaskRunner struct {
	seen workflow.TaskRunRequest
}

type capturingTaskService struct {
	request task.IntakeRequest
}

func (s *capturingTaskService) Plan(task.IntakeRequest) (*task.CompletionPlan, error) {
	return nil, nil
}

func (s *capturingTaskService) Run(request task.IntakeRequest) (*task.CompletionPlan, error) {
	s.request = request
	return &task.CompletionPlan{
		ID:               "task-plan-1",
		CompletionStatus: "validated",
		ValidationResult: task.ValidationResult{Passed: true, Status: "verified"},
	}, nil
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

func TestRunnerPassesPursuitAndWorkflowContextToTaskEngine(t *testing.T) {
	tasks := &capturingTaskService{}
	runner := NewRunner(tasks)

	result, err := runner.RunWorkflowTask(workflow.TaskRunRequest{
		OwnerIdentity: "alice",
		PursuitID:     "pursuit-1",
		WorkflowID:    "workflow-1",
		Request:       "Advance the governed workflow.",
		ProjectKey:    "018-hai",
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
}
