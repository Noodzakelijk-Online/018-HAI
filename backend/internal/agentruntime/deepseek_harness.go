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

// deepSeekHarnessAdapter executes DeepSeek Harness only through its documented
// non-interactive headless profile. HAI remains the authority for identity,
// approvals, workspace limits, cancellation, verification, and audit.
type deepSeekHarnessAdapter struct {
	enabled       bool
	executable    string
	workspace     string
	workspaceRoot string
	profile       string
	timeout       time.Duration
	envAllow      []string
	outputLimit   int64
}

func (*deepSeekHarnessAdapter) RuntimeID() string { return "deepseek-harness" }

func newDeepSeekHarnessAdapterFromEnv() *deepSeekHarnessAdapter {
	return &deepSeekHarnessAdapter{
		enabled:       envEnabled("DEEPSEEK_HARNESS_ENABLED"),
		executable:    firstNonEmpty(os.Getenv("DEEPSEEK_HARNESS_EXECUTABLE"), "dsh"),
		workspace:     strings.TrimSpace(os.Getenv("DEEPSEEK_HARNESS_WORKSPACE")),
		workspaceRoot: strings.TrimSpace(os.Getenv("AGENT_RUNTIME_WORKSPACE_ROOT")),
		profile:       firstNonEmpty(strings.TrimSpace(os.Getenv("DEEPSEEK_HARNESS_PROFILE")), "headless"),
		timeout:       time.Duration(boundedIntEnv("DEEPSEEK_HARNESS_TIMEOUT_SECONDS", defaultTimeoutSeconds, 1, 900)) * time.Second,
		envAllow:      csvValues(os.Getenv("DEEPSEEK_HARNESS_ENV_ALLOWLIST")),
		outputLimit:   int64(boundedIntEnv("AGENT_RUNTIME_OUTPUT_LIMIT_BYTES", defaultOutputLimit, 4096, maxOutputLimit)),
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
	if strings.TrimSpace(a.profile) != "headless" {
		missing = append(missing, "DEEPSEEK_HARNESS_PROFILE=headless")
	}
	workspaceReason := a.workspaceBlockedReason()
	return Info{
		ID:                   "deepseek-harness",
		Name:                 "DeepSeek Harness",
		Type:                 "deepseek_harness",
		Enabled:              a.enabled,
		Configured:           len(missing) == 0 && workspaceReason == "",
		ExecutionEnabled:     a.enabled && len(missing) == 0 && workspaceReason == "",
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
	if strings.TrimSpace(a.profile) != "headless" {
		health.Status = "blocked"
		health.Reason = "DEEPSEEK_HARNESS_PROFILE must remain headless"
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
	probeTimeout := a.timeout
	if probeTimeout > 10*time.Second {
		probeTimeout = 10 * time.Second
	}
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	// `--help` is handled by the documented headless profile without accepting
	// a task. It validates the exact command grammar without creating a session
	// or running model, tool, or workspace operations.
	probe := exec.CommandContext(probeCtx, path, "--profile", "headless", "--help")
	probe.Dir = a.workspace
	probe.Env = safeEnvironment(a.envAllow, map[string]string{
		"DSH_HOME":     filepath.Join(a.workspace, ".hai-dsh"),
		"TERMINAL_CWD": a.workspace,
	})
	var stderr bytes.Buffer
	probe.Stderr = &limitedWriter{writer: &stderr, remaining: a.outputLimit / 4}
	if err := probe.Run(); err != nil {
		health.Status = "unavailable"
		if probeCtx.Err() == context.DeadlineExceeded {
			health.Reason = "DeepSeek Harness headless profile probe timed out"
		} else {
			health.Reason = safety.RedactSecrets(strings.TrimSpace(stderr.String()))
			if health.Reason == "" {
				health.Reason = "DeepSeek Harness does not support the required headless profile"
			}
		}
		return health
	}
	health.Status = "ready"
	health.Reason = "DeepSeek Harness headless profile and dedicated workspace are available: " + filepath.Base(path)
	health.LatencyMs = time.Since(started).Milliseconds()
	return health
}

func (a *deepSeekHarnessAdapter) ListSkills(context.Context) []Skill {
	return []Skill{{
		ID:               "deepseek-harness:headless",
		RuntimeID:        "deepseek-harness",
		Name:             "Headless agent session",
		Category:         "agent_harness",
		RiskLevel:        "high",
		ApprovalRequired: true,
		ExecutionMode:    "approved_headless_cli",
		Source:           "DEEPSEEK_HARNESS_PROFILE",
		Description:      "A bounded DeepSeek Harness headless session in a dedicated HAI workspace. HAI does not expose the Web UI, ACP server, or plugin installation through this adapter.",
		Tags:             []string{"deepseek", "harness", "headless", "approval-required"},
	}}
}

func (a *deepSeekHarnessAdapter) ExecuteTask(parent context.Context, task Task) Result {
	started := time.Now()
	if result, blocked := emergencyStopResult("deepseek-harness"); blocked {
		return result
	}
	if strings.TrimSpace(a.profile) != "headless" {
		return Result{RuntimeID: "deepseek-harness", Status: "blocked", Message: "DEEPSEEK_HARNESS_PROFILE must remain headless", ExitCode: -1}
	}
	if reason := a.workspaceBlockedReason(); reason != "" {
		return Result{RuntimeID: "deepseek-harness", Status: "blocked", Message: reason, ExitCode: -1}
	}
	ctx, cancel := context.WithTimeout(parent, a.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, a.executable, "--profile", "headless", task.Prompt)
	cmd.Dir = a.workspace
	cmd.Env = safeEnvironment(a.envAllow, map[string]string{
		"HAI_RUNTIME_TASK_ID": task.ID,
		"HAI_PROJECT_KEY":     task.ProjectKey,
		"TERMINAL_CWD":        a.workspace,
		// The Harness persists profiles and one-shot sessions under DSH_HOME.
		// Keep that state within the already-approved runtime workspace rather
		// than allowing its default user-profile location to escape HAI's scope.
		"DSH_HOME": filepath.Join(a.workspace, ".hai-dsh"),
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
	status, message, exitCode := "completed", "DeepSeek Harness completed the approved headless task", 0
	if err != nil {
		status = "failed"
		message = safety.RedactSecrets(strings.TrimSpace(stderr.String()))
		if message == "" {
			message = "DeepSeek Harness process failed without diagnostic output"
		}
		exitCode = -1
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
			"server-side HAI approval and final-effect proof verified",
			"DeepSeek Harness invoked through the documented headless CLI without shell interpolation",
			"HAI did not launch the DeepSeek Harness Web UI, ACP server, or install plugins",
			"dedicated workspace, timeout, output limit, environment allowlist, and secret redaction enforced by HAI",
		},
	}
}

func (a *deepSeekHarnessAdapter) StopTask(_ context.Context, taskID string) StopResult {
	return unsupportedStopTask("deepseek-harness", taskID, "DeepSeek Harness tasks are bounded CLI processes; HAI cancels the owner-bound context but does not persist a remote session handle")
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
		"headless agent session",
		"plugin-based runtime architecture",
		"operator-configured model routing",
		"workspace file and command operations subject to DeepSeek Harness policy",
	}
}

func (a *deepSeekHarnessAdapter) architecture() []string {
	return []string{
		"HAI workflow approval queue and final-effect proof",
		"HAI agent-runtime registry",
		"DeepSeek Harness documented headless CLI profile",
		"DeepSeek Harness operator-configured model and permission policy",
		"HAI source-grounded verification and audit log",
	}
}

func (a *deepSeekHarnessAdapter) controls() []string {
	return []string{
		"disabled by default through DEEPSEEK_HARNESS_ENABLED",
		"only the documented headless profile is accepted",
		"server-side HAI approval and a durable final-effect proof required before every task",
		"dedicated workspace must remain under AGENT_RUNTIME_WORKSPACE_ROOT",
		"invoked without shell interpolation, Web UI, ACP server, or plugin installation",
		"bounded timeout and output capture with secret redaction",
		"environment inheritance limited to DEEPSEEK_HARNESS_ENV_ALLOWLIST plus HAI task metadata",
		"DeepSeek Harness permission prompts and model credentials remain operator-managed",
	}
}
