package agentruntime

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// deepSeekHarnessAdapter is a preview-readiness integration. DeepSeek currently
// documents a Web UI launch and explicitly warns that the developer preview may
// introduce compatibility-breaking changes. HAI therefore never invents a
// headless task grammar or invokes the Harness until a stable, documented,
// non-interactive contract is available and independently validated.
type deepSeekHarnessAdapter struct {
	enabled       bool
	executable    string
	workspace     string
	workspaceRoot string
}

func (*deepSeekHarnessAdapter) RuntimeID() string { return "deepseek-harness" }

func newDeepSeekHarnessAdapterFromEnv() *deepSeekHarnessAdapter {
	return &deepSeekHarnessAdapter{
		enabled:       envEnabled("DEEPSEEK_HARNESS_ENABLED"),
		executable:    firstNonEmpty(os.Getenv("DEEPSEEK_HARNESS_EXECUTABLE"), "dsh"),
		workspace:     strings.TrimSpace(os.Getenv("DEEPSEEK_HARNESS_WORKSPACE")),
		workspaceRoot: strings.TrimSpace(os.Getenv("AGENT_RUNTIME_WORKSPACE_ROOT")),
	}
}

func (a *deepSeekHarnessAdapter) Info() Info {
	missing := []string{}
	if strings.TrimSpace(a.executable) == "" {
		missing = append(missing, "DEEPSEEK_HARNESS_EXECUTABLE")
	}
	if strings.TrimSpace(a.workspace) == "" {
		missing = append(missing, "DEEPSEEK_HARNESS_WORKSPACE")
	}
	if strings.TrimSpace(a.workspaceRoot) == "" {
		missing = append(missing, "AGENT_RUNTIME_WORKSPACE_ROOT")
	}
	workspaceReason := a.workspaceBlockedReason()
	return Info{
		ID:                   "deepseek-harness",
		Name:                 "DeepSeek Harness",
		Type:                 "deepseek_harness",
		Enabled:              a.enabled,
		Configured:           len(missing) == 0 && workspaceReason == "",
		ExecutionEnabled:     false,
		RequiresApproval:     true,
		ReadOnlyDefault:      true,
		Capabilities:         a.capabilities(),
		Architecture:         a.architecture(),
		Controls:             a.controls(),
		MissingConfiguration: missing,
		Endpoint:             a.executable,
	}
}

func (a *deepSeekHarnessAdapter) HealthCheck(ctx context.Context) Health {
	started := time.Now()
	health := Health{RuntimeID: "deepseek-harness", Status: "disabled", CheckedAt: time.Now().UTC()}
	if !a.enabled {
		health.Reason = "DEEPSEEK_HARNESS_ENABLED is false"
		return health
	}
	if strings.TrimSpace(a.workspace) == "" {
		health.Status = "blocked"
		health.Reason = "DEEPSEEK_HARNESS_WORKSPACE is required"
		return health
	}
	if reason := a.workspaceBlockedReason(); reason != "" {
		health.Status = "blocked"
		health.Reason = reason
		return health
	}
	if stat, err := os.Stat(a.workspace); err != nil || !stat.IsDir() {
		health.Status = "blocked"
		health.Reason = "DeepSeek Harness workspace is not an accessible directory"
		return health
	}
	path, err := exec.LookPath(a.executable)
	if err != nil {
		health.Status = "unavailable"
		health.Reason = "DeepSeek Harness executable was not found"
		return health
	}
	health.Status = "blocked"
	health.Reason = "DeepSeek Harness developer preview is installed at " + filepath.Base(path) + "; HAI execution remains disabled until a stable documented non-interactive contract is validated"
	health.LatencyMs = time.Since(started).Milliseconds()
	return health
}

func (a *deepSeekHarnessAdapter) ListSkills(context.Context) []Skill {
	return []Skill{{
		ID:               "deepseek-harness:preview-readiness",
		RuntimeID:        "deepseek-harness",
		Name:             "Preview readiness check",
		Category:         "agent_harness",
		RiskLevel:        "high",
		ApprovalRequired: true,
		ExecutionMode:    "preview_readiness_only",
		Source:           "DEEPSEEK_HARNESS_EXECUTABLE",
		Description:      "Checks that the operator-selected DeepSeek Harness executable and dedicated workspace exist. HAI does not execute preview sessions, launch the Web UI, ACP server, or install plugins.",
		Tags:             []string{"deepseek", "harness", "preview", "readiness-only"},
	}}
}

func (a *deepSeekHarnessAdapter) ExecuteTask(parent context.Context, task Task) Result {
	started := time.Now()
	if result, blocked := emergencyStopResult("deepseek-harness"); blocked {
		return result
	}
	if reason := a.workspaceBlockedReason(); reason != "" {
		return Result{RuntimeID: "deepseek-harness", Status: "blocked", Message: reason, ExitCode: -1}
	}
	return Result{
		RuntimeID:  "deepseek-harness",
		Status:     "blocked",
		Message:    "DeepSeek Harness is a developer preview; HAI will not execute an undocumented non-interactive command contract",
		ExitCode:   -1,
		DurationMs: time.Since(started).Milliseconds(),
		AuditEvents: []string{
			"DeepSeek Harness execution blocked because the published developer-preview documentation does not establish a stable non-interactive task contract",
			"HAI did not launch the Web UI, ACP server, execute a task, or install plugins",
			"operator-selected executable and dedicated workspace remain inspectable for readiness only",
		},
	}
}

func (a *deepSeekHarnessAdapter) StopTask(_ context.Context, taskID string) StopResult {
	return unsupportedStopTask("deepseek-harness", taskID, "DeepSeek Harness execution is disabled while the upstream remains an incompatible developer preview")
}

func (a *deepSeekHarnessAdapter) workspaceBlockedReason() string {
	if strings.TrimSpace(a.workspace) == "" {
		return "DEEPSEEK_HARNESS_WORKSPACE is required"
	}
	if strings.TrimSpace(a.workspaceRoot) == "" {
		return "agent runtime workspace root is required"
	}
	root, err := filepath.Abs(filepath.Clean(a.workspaceRoot))
	if err != nil {
		return "agent runtime workspace root is invalid"
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "agent runtime workspace root is not accessible"
	}
	workspace, err := filepath.Abs(filepath.Clean(a.workspace))
	if err != nil {
		return "DeepSeek Harness workspace is invalid"
	}
	workspace, err = filepath.EvalSymlinks(workspace)
	if err != nil {
		return "DeepSeek Harness workspace is not accessible"
	}
	relative, err := filepath.Rel(root, workspace)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) || filepath.IsAbs(relative) {
		return "DeepSeek Harness workspace must stay inside AGENT_RUNTIME_WORKSPACE_ROOT"
	}
	return ""
}

func (a *deepSeekHarnessAdapter) capabilities() []string {
	return []string{
		"preview installation readiness",
		"plugin-based runtime architecture",
		"operator-configured model routing",
		"operator-managed Web UI and plugin architecture, not executed by HAI",
	}
}

func (a *deepSeekHarnessAdapter) architecture() []string {
	return []string{
		"HAI workflow approval queue and final-effect proof",
		"HAI agent-runtime registry",
		"DeepSeek Harness published developer-preview capability boundary",
		"operator-managed model and permission policy",
		"HAI source-grounded verification and audit log",
	}
}

func (a *deepSeekHarnessAdapter) controls() []string {
	return []string{
		"disabled by default through DEEPSEEK_HARNESS_ENABLED",
		"no DeepSeek Harness task execution until a stable documented non-interactive contract is independently validated",
		"dedicated workspace must remain under AGENT_RUNTIME_WORKSPACE_ROOT",
		"does not launch the Web UI, ACP server, execute a task, or install plugins",
		"DeepSeek Harness permission prompts and model credentials remain operator-managed",
	}
}
