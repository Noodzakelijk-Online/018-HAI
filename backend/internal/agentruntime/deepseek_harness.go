package agentruntime

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"automation-hub-backend/internal/safety"
)

// deepSeekHarnessAdapter invokes only the upstream documented headless profile.
// DeepSeek still labels the harness a developer preview, so execution remains
// separately opt-in and stays behind HAI's final-effect proof boundary.
type deepSeekHarnessAdapter struct {
	enabled          bool
	executionEnabled bool
	executable       string
	workspace        string
	workspaceRoot    string
	stateDir         string
	timeout          time.Duration
	outputLimit      int64
	envAllow         []string
}

func (*deepSeekHarnessAdapter) RuntimeID() string { return "deepseek-harness" }

func newDeepSeekHarnessAdapterFromEnv() *deepSeekHarnessAdapter {
	return &deepSeekHarnessAdapter{
		enabled:          envEnabled("DEEPSEEK_HARNESS_ENABLED"),
		executionEnabled: envEnabled("DEEPSEEK_HARNESS_EXECUTION_ENABLED"),
		executable:       firstNonEmpty(os.Getenv("DEEPSEEK_HARNESS_EXECUTABLE"), "dsh"),
		workspace:        strings.TrimSpace(os.Getenv("DEEPSEEK_HARNESS_WORKSPACE")),
		workspaceRoot:    strings.TrimSpace(os.Getenv("AGENT_RUNTIME_WORKSPACE_ROOT")),
		stateDir:         strings.TrimSpace(os.Getenv("DEEPSEEK_HARNESS_STATE_DIR")),
		timeout:          time.Duration(boundedIntEnv("DEEPSEEK_HARNESS_TIMEOUT_SECONDS", defaultTimeoutSeconds, 1, 900)) * time.Second,
		outputLimit:      int64(boundedIntEnv("AGENT_RUNTIME_OUTPUT_LIMIT_BYTES", defaultOutputLimit, 4096, maxOutputLimit)),
		envAllow:         csvValues(os.Getenv("DEEPSEEK_HARNESS_ENV_ALLOWLIST")),
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
	if strings.TrimSpace(a.stateDir) == "" {
		missing = append(missing, "DEEPSEEK_HARNESS_STATE_DIR")
	}
	workspaceReason := a.workspaceBlockedReason()
	return Info{
		ID:                   "deepseek-harness",
		Name:                 "DeepSeek Harness",
		Type:                 "deepseek_harness",
		Enabled:              a.enabled,
		Configured:           len(missing) == 0 && workspaceReason == "" && a.stateDirBlockedReason() == "",
		ExecutionEnabled:     a.enabled && a.executionEnabled && len(missing) == 0 && workspaceReason == "" && a.stateDirBlockedReason() == "",
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
	if reason := a.stateDirBlockedReason(); reason != "" {
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
	if !a.executionEnabled {
		health.Status = "blocked"
		health.Reason = "DEEPSEEK_HARNESS_EXECUTION_ENABLED is false; headless execution remains opt-in while upstream is a developer preview"
		return health
	}
	health.Status = "ready"
	health.Reason = "DeepSeek Harness headless profile is available at " + filepath.Base(path) + "; every task still needs HAI approval and a final-effect proof"
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
		ExecutionMode:    "approved_headless_task",
		Source:           "DEEPSEEK_HARNESS_EXECUTABLE",
		Description:      "Runs only DeepSeek Harness' documented one-shot headless profile after HAI approval. HAI never launches the Web UI or ACP server and never installs plugins.",
		Tags:             []string{"deepseek", "harness", "headless", "approval-gated"},
	}}
}

func (a *deepSeekHarnessAdapter) ExecuteTask(parent context.Context, task Task) Result {
	started := time.Now()
	if result, blocked := emergencyStopResult("deepseek-harness"); blocked {
		return result
	}
	if !a.enabled {
		return Result{RuntimeID: "deepseek-harness", Status: "blocked", Message: "DEEPSEEK_HARNESS_ENABLED is false", ExitCode: -1}
	}
	if !a.executionEnabled {
		return Result{RuntimeID: "deepseek-harness", Status: "blocked", Message: "DEEPSEEK_HARNESS_EXECUTION_ENABLED is false", ExitCode: -1}
	}
	if reason := a.workspaceBlockedReason(); reason != "" {
		return Result{RuntimeID: "deepseek-harness", Status: "blocked", Message: reason, ExitCode: -1}
	}
	if reason := a.stateDirBlockedReason(); reason != "" {
		return Result{RuntimeID: "deepseek-harness", Status: "blocked", Message: reason, ExitCode: -1}
	}

	ctx, cancel := context.WithTimeout(parent, a.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, a.executable, "--profile", "headless", task.Prompt)
	cmd.Dir = a.workspace
	cmd.Env = safeEnvironment(a.envAllow, map[string]string{
		"DSH_HOME":            a.stateDir,
		"HAI_RUNTIME_TASK_ID": task.ID,
		"HAI_PROJECT_KEY":     task.ProjectKey,
		"TERMINAL_CWD":        a.workspace,
	})
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &limitedWriter{writer: &stdout, remaining: a.outputLimit}
	cmd.Stderr = &limitedWriter{writer: &stderr, remaining: a.outputLimit / 4}
	if result, blocked := emergencyStopResult("deepseek-harness"); blocked {
		return result
	}
	err := cmd.Run()
	output := trimAndRedact(stdout.String(), a.outputLimit)
	message := "DeepSeek Harness completed the approved headless task"
	status, exitCode := "completed", 0
	if err != nil {
		status, exitCode = "failed", -1
		message = safety.RedactSecrets(strings.TrimSpace(stderr.String()))
		if message == "" {
			message = "DeepSeek Harness process failed without diagnostic output"
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		if ctx.Err() == context.DeadlineExceeded {
			status = "blocked"
			message = "DeepSeek Harness execution exceeded the configured timeout and was stopped"
		}
	}
	return Result{
		RuntimeID:  "deepseek-harness",
		Status:     status,
		Message:    message,
		Output:     output,
		ExitCode:   exitCode,
		DurationMs: time.Since(started).Milliseconds(),
		AuditEvents: []string{
			"server-side approval and final-effect proof verified by HAI before adapter execution",
			"DeepSeek Harness invoked only through documented --profile headless without shell interpolation",
			"Web UI, ACP server, browser control, and plugin installation were not invoked",
			"dedicated workspace, state directory, timeout, output limit, environment allowlist, and secret redaction enforced by HAI",
		},
	}
}

func (a *deepSeekHarnessAdapter) StopTask(_ context.Context, taskID string) StopResult {
	return unsupportedStopTask("deepseek-harness", taskID, "DeepSeek Harness tasks are bounded headless CLI processes; HAI does not persist an upstream session handle for external stop yet")
}

func (a *deepSeekHarnessAdapter) stateDirBlockedReason() string {
	if strings.TrimSpace(a.stateDir) == "" {
		return "DEEPSEEK_HARNESS_STATE_DIR is required"
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
	stateDir, err := filepath.Abs(filepath.Clean(a.stateDir))
	if err != nil {
		return "DeepSeek Harness state directory is invalid"
	}
	if info, err := os.Lstat(stateDir); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "DeepSeek Harness state directory must not be a symbolic link"
		}
		if !info.IsDir() {
			return "DeepSeek Harness state directory must be a directory"
		}
		stateDir, err = filepath.EvalSymlinks(stateDir)
		if err != nil {
			return "DeepSeek Harness state directory is not accessible"
		}
	} else if !os.IsNotExist(err) {
		return "DeepSeek Harness state directory cannot be inspected"
	}
	parent := filepath.Dir(stateDir)
	parent, err = filepath.EvalSymlinks(parent)
	if err != nil {
		return "DeepSeek Harness state directory parent is not accessible"
	}
	stateDir = filepath.Join(parent, filepath.Base(stateDir))
	relative, err := filepath.Rel(root, stateDir)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) || filepath.IsAbs(relative) {
		return "DeepSeek Harness state directory must stay inside AGENT_RUNTIME_WORKSPACE_ROOT"
	}
	return ""
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
		"documented one-shot headless execution",
		"plugin-based runtime architecture",
		"operator-configured model routing",
		"operator-managed model routing and plugin architecture",
	}
}

func (a *deepSeekHarnessAdapter) architecture() []string {
	return []string{
		"HAI workflow approval queue and final-effect proof",
		"HAI agent-runtime registry",
		"DeepSeek Harness documented headless profile capability boundary",
		"operator-managed model and permission policy",
		"HAI source-grounded verification and audit log",
	}
}

func (a *deepSeekHarnessAdapter) controls() []string {
	return []string{
		"disabled by default through DEEPSEEK_HARNESS_ENABLED and DEEPSEEK_HARNESS_EXECUTION_ENABLED",
		"server-side HAI approval and final-effect proof required before every headless task",
		"dedicated workspace must remain under AGENT_RUNTIME_WORKSPACE_ROOT",
		"dedicated state directory must remain under AGENT_RUNTIME_WORKSPACE_ROOT",
		"does not launch the Web UI, ACP server, control a browser, or install plugins",
		"timeout, output limit, environment allowlist, and secret redaction remain enforced by HAI",
		"DeepSeek Harness permission prompts and model credentials remain operator-managed",
	}
}
