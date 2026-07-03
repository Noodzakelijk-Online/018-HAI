package task

import (
	"testing"
	"time"

	"automation-hub-backend/internal/automation"
	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
)

func TestAutomationToolExecutorMapsControlledLaunchResult(t *testing.T) {
	id := uuid.New()
	launchEventID := uuid.New()
	launchedAt := time.Now().UTC()
	executor := NewAutomationToolExecutor(&fakeAutomationLauncher{
		result: &automation.LaunchResult{
			AutomationID:  id,
			LaunchEventID: launchEventID,
			RuntimeType:   "script",
			LaunchType:    "script",
			Target:        "verify-project.sh",
			Status:        "completed",
			Message:       "script completed",
			Output:        "tests passed",
			RuntimeRouteTrace: &models.AutomationRuntimeRouteTrace{
				RuntimeID:         "openclaw",
				Intent:            "code_review",
				ExecutionMode:     "read_only",
				RiskLevel:         "medium",
				RecommendedSkills: []string{"autoreview", "gitcrawl"},
				BlockedSurfaces:   []string{"external_message_sending"},
			},
			ExitCode:    0,
			DurationMs:  42,
			AuditEvents: []string{"script executed without shell"},
			LaunchedAt:  launchedAt,
		},
	})

	result, err := executor.Execute(ToolExecutionRequest{AutomationID: id.String(), Task: "Run tests"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status != "completed" || result.Output != "tests passed" {
		t.Fatalf("result = %#v, want completed runtime evidence", result)
	}
	if result.AutomationID != id.String() || result.LaunchEventID != launchEventID.String() || !result.ExecutedAt.Equal(launchedAt) {
		t.Fatalf("runtime identity was not preserved: %#v", result)
	}
	if result.RuntimeRouteTrace == nil || result.RuntimeRouteTrace.RuntimeID != "openclaw" || len(result.RuntimeRouteTrace.RecommendedSkills) != 2 {
		t.Fatalf("runtime route trace was not preserved: %#v", result.RuntimeRouteTrace)
	}
}

func TestAutomationToolExecutorRejectsInvalidAutomationID(t *testing.T) {
	executor := NewAutomationToolExecutor(&fakeAutomationLauncher{})
	if _, err := executor.Execute(ToolExecutionRequest{AutomationID: "not-a-uuid"}); err == nil {
		t.Fatalf("expected invalid automation ID to be rejected")
	}
}

type fakeAutomationLauncher struct {
	result *automation.LaunchResult
	err    error
}

func (f *fakeAutomationLauncher) Launch(id uuid.UUID) (*automation.LaunchResult, error) {
	return f.result, f.err
}

func (f *fakeAutomationLauncher) LaunchTask(id uuid.UUID, request automation.TaskLaunchRequest) (*automation.LaunchResult, error) {
	return f.result, f.err
}
