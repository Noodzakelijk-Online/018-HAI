package task

import (
	"fmt"
	"strings"

	"automation-hub-backend/internal/automation"

	"github.com/google/uuid"
)

type automationLauncher interface {
	Launch(id uuid.UUID) (*automation.LaunchResult, error)
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
	result, err := e.launcher.Launch(id)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("automation runtime returned no launch result")
	}
	return &ToolExecutionResult{
		AutomationID:     result.AutomationID.String(),
		RuntimeType:      result.RuntimeType,
		LaunchType:       result.LaunchType,
		Target:           result.Target,
		Status:           result.Status,
		Message:          result.Message,
		Output:           result.Output,
		ExitCode:         result.ExitCode,
		DurationMs:       result.DurationMs,
		RequiresApproval: result.RequiresApproval,
		AuditEvents:      append([]string{}, result.AuditEvents...),
		ExecutedAt:       result.LaunchedAt,
	}, nil
}
