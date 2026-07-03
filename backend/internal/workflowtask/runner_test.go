package workflowtask

import (
	"testing"

	"automation-hub-backend/internal/workflow"
)

type fakeWorkflowTaskRunner struct {
	seen workflow.TaskRunRequest
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
