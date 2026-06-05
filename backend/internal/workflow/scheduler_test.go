package workflow

import (
	"testing"
	"time"
)

func TestWorkflowSchedulerRunsOpenLoopsBeforeRunnableItems(t *testing.T) {
	service := &fakeScheduledWorkflowService{
		openLoopResult: &OpenLoopRunSummary{Checked: 1, Triggered: 1},
		runDueResult:   &WorkflowRunSummary{Checked: 1, Completed: 1},
	}
	scheduler := NewScheduler(service, time.Minute, 3)

	scheduler.runOnce()

	if got := service.calls; len(got) != 2 || got[0] != "open-loops" || got[1] != "run-due" {
		t.Fatalf("calls = %#v, want open-loops then run-due", got)
	}
	if service.openLoopLimit != 3 || service.runDueLimit != 3 {
		t.Fatalf("limits = open-loop %d run-due %d, want 3", service.openLoopLimit, service.runDueLimit)
	}
}

func TestWorkflowSchedulerCanDisableOpenLoopPass(t *testing.T) {
	t.Setenv("WORKFLOW_OPEN_LOOP_SCHEDULER_ENABLED", "false")
	service := &fakeScheduledWorkflowService{runDueResult: &WorkflowRunSummary{Checked: 1}}
	scheduler := NewScheduler(service, time.Minute, 2)

	scheduler.runOnce()

	if got := service.calls; len(got) != 1 || got[0] != "run-due" {
		t.Fatalf("calls = %#v, want only run-due", got)
	}
}

func TestWorkflowSchedulerLimitDefaultsAndCaps(t *testing.T) {
	t.Setenv("WORKFLOW_SCHEDULER_RUN_LIMIT", "75")
	if got := schedulerLimit(); got != 50 {
		t.Fatalf("capped limit = %d, want 50", got)
	}
	t.Setenv("WORKFLOW_SCHEDULER_RUN_LIMIT", "bad")
	if got := schedulerLimit(); got != 5 {
		t.Fatalf("fallback limit = %d, want 5", got)
	}
}

type fakeScheduledWorkflowService struct {
	calls          []string
	openLoopLimit  int
	runDueLimit    int
	openLoopResult *OpenLoopRunSummary
	runDueResult   *WorkflowRunSummary
}

func (s *fakeScheduledWorkflowService) RunDue(request RunDueRequest) (*WorkflowRunSummary, error) {
	s.calls = append(s.calls, "run-due")
	s.runDueLimit = request.Limit
	if s.runDueResult != nil {
		return s.runDueResult, nil
	}
	return &WorkflowRunSummary{}, nil
}

func (s *fakeScheduledWorkflowService) RunDueOpenLoops(request RunDueRequest) (*OpenLoopRunSummary, error) {
	s.calls = append(s.calls, "open-loops")
	s.openLoopLimit = request.Limit
	if s.openLoopResult != nil {
		return s.openLoopResult, nil
	}
	return &OpenLoopRunSummary{}, nil
}
