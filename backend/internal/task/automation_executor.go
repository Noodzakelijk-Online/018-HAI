package task

import (
	"fmt"
	"strings"

	"automation-hub-backend/internal/automation"

	"github.com/google/uuid"
)

type automationLauncher interface {
	Launch(id uuid.UUID) (*automation.LaunchResult, error)
	LaunchTask(id uuid.UUID, request automation.TaskLaunchRequest) (*automation.LaunchResult, error)
}

type AutomationToolExecutor struct {
	launcher automationLauncher
}

func NewAutomationToolExecutor(launcher automationLauncher) *AutomationToolExecutor {
	return &AutomationToolExecutor{launcher: launcher}
}

func (e *AutomationToolExecutor) Execute(request ToolExecutionRequest) (*ToolExecutionResult, error) {
	if e == nil || e.launcher == nil {
		return nil, fmt.Errorf("automation runtime launcher is not configured")
	}
	id, err := uuid.Parse(strings.TrimSpace(request.AutomationID))
	if err != nil {
		return nil, fmt.Errorf("automationId must be a valid UUID")
	}
	result, err := e.launcher.LaunchTask(id, automation.TaskLaunchRequest{
		OwnerIdentity: request.OwnerIdentity,
		Task:          request.Task,
		ProjectKey:    request.ProjectKey,
		HumanApproved: request.HumanApproved,
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("automation runtime returned no launch result")
	}
	return &ToolExecutionResult{
		AutomationID:      result.AutomationID.String(),
		LaunchEventID:     uuidStringOrEmpty(result.LaunchEventID),
		RuntimeType:       result.RuntimeType,
		LaunchType:        result.LaunchType,
		Target:            result.Target,
		Status:            result.Status,
		Message:           result.Message,
		Output:            result.Output,
		RuntimeRouteTrace: copyAutomationRuntimeRouteTrace(result.RuntimeRouteTrace),
		ExitCode:          result.ExitCode,
		DurationMs:        result.DurationMs,
		RequiresApproval:  result.RequiresApproval,
		AuditEvents:       append([]string{}, result.AuditEvents...),
		ExecutedAt:        result.LaunchedAt,
	}, nil
}

func uuidStringOrEmpty(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}
