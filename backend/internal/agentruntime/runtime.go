package agentruntime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"automation-hub-backend/internal/safety"
)

const (
	defaultTimeoutSeconds = 120
	defaultOutputLimit     = 64 * 1024
	maxOutputLimit         = 1024 * 1024
	maxTaskPromptBytes     = 50 * 1024
)

type Task struct {
	ID            string
	Prompt        string
	ProjectKey    string
	HumanApproved bool
}

type Result struct {
	RuntimeID   string   `json:"runtimeId"`
	Status      string   `json:"status"`
	Message     string   `json:"message,omitempty"`
	Output      string   `json:"output,omitempty"`
	ExitCode    int      `json:"exitCode"`
	DurationMs  int64    `json:"durationMs"`
	AuditEvents []string `json:"auditEvents"`
}

type Health struct {
	RuntimeID string    `json:"runtimeId"`
	Status    string    `json:"status"`
	Reason    string    `json:"reason"`
	CheckedAt time.Time `json:"checkedAt"`
	LatencyMs int64     `json:"latencyMs"`
}

type Info struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	Type                 string   `json:"type"`
	Enabled              bool     `json:"enabled"`
	Configured           bool     `json:"configured"`
	ExecutionEnabled     bool     `json:"executionEnabled"`
	RequiresApproval     bool     `json:"requiresApproval"`
	ReadOnlyDefault      bool     `json:"readOnlyDefault"`
	Capabilities         []string `json:"capabilities"`
	MissingConfiguration []string `json:"missingConfiguration,omitempty"`
	Endpoint             string   `json:"endpoint,omitempty"`
}

type Adapter interface {
	Info() Info
	HealthCheck(context.Context) Health
	ExecuteTask(context.Context, Task) Result
}

type Registry struct {
	adapters map[string]Adapter
}

func NewRegistry(adapters ...Adapter) *Registry {
	registry := &Registry{adapters: map[string]Adapter{}}
	for _, adapter := range adapters {
		if adapter == nil {
			continue
		}
		registry.adapters[adapter.Info().ID] = adapter
	}
	return registry
}

func DefaultRegistry() *Registry {
	return NewRegistry(newHermesAdapterFromEnv(), newOdysseusAdapterFromEnv())
}

func (r *Registry) List() []Info {
	result := []Info{}
	for _, id := range []string{"hermes", "odysseus"} {
		if adapter := r.adapters[id]; adapter != nil {
			result = append(result, adapter.Info())
		}
	}
	return result
}

func (r *Registry) Health(ctx context.Context) []Health {
	result := []Health{}
	for _, id := range []string{"hermes", "odysseus"} {
		if adapter := r.adapters[id]; adapter != nil {
			result = append(result, adapter.HealthCheck(ctx))
		}
	}
	return result
}

func (r *Registry) Execute(ctx context.Context, runtimeID string, task Task) Result {
	runtimeID = strings.ToLower(strings.TrimSpace(runtimeID))
	adapter := r.adapters[runtimeID]
	if adapter == nil {
		return Result{
			RuntimeID:   runtimeID,
			Status:      "blocked",
			Message:     fmt.Sprintf("agent runtime %q is not registered", runtimeID),
			ExitCode:    -1,
			AuditEvents: []string{"runtime registry lookup failed"},
		}
	}
	info := adapter.Info()
	if !info.Enabled || !info.Configured || !info.ExecutionEnabled {
		return Result{
			RuntimeID:   runtimeID,
			Status:      "blocked",
			Message:     "agent runtime is disabled or incomplete; review runtime registry configuration",
			ExitCode:    -1,
			AuditEvents: []string{"runtime registry policy blocked execution"},
		}
	}
	if info.RequiresApproval && !task.HumanApproved {
		return Result{
			RuntimeID:   runtimeID,
			Status:      "blocked",
			Message:     "agent runtime execution requires a server-side human approval record",
			ExitCode:    -1,
			AuditEvents: []string{"agent approval gate blocked execution"},
		}
	}
	if strings.TrimSpace(task.Prompt) == "" {
		return Result{
			RuntimeID:   runtimeID,
			Status:      "blocked",
			Message:     "agent runtime task prompt is required",
			ExitCode:    -1,
			AuditEvents: []string{"empty agent task rejected"},
		}
	}
	if len(task.Prompt) > maxTaskPromptBytes {
		return Result{
			RuntimeID:   runtimeID,
			Status:      "blocked",
			Message:     "agent runtime task prompt exceeds the 50 KiB execution boundary; store large context in HAI memory or connected sources instead",
			ExitCode:    -1,
			AuditEvents: []string{"oversized agent task rejected"},
		}
	}
	return adapter.ExecuteTask(ctx, task)
}

type hermesAdapter struct {
	enabled     bool
	executable  string
	workspace   string
	workspaceRoot string
	maxTurns    int
	timeout     time.Duration
	toolsets    []string
	envAllow    []string
	outputLimit int64
}

func newHermesAdapterFromEnv() *hermesAdapter {
	return &hermesAdapter{
		enabled:     envEnabled("HERMES_AGENT_ENABLED"),
		executable:  firstNonEmpty(os.Getenv("HERMES_EXECUTABLE"), "hermes"),
		workspace:   strings.TrimSpace(os.Getenv("HERMES_WORKSPACE")),
		workspaceRoot: strings.TrimSpace(os.Getenv("AGENT_RUNTIME_WORKSPACE_ROOT")),
		maxTurns:    boundedIntEnv("HERMES_MAX_TURNS", 20, 1, 100),
		timeout:     time.Duration(boundedIntEnv("HERMES_TIMEOUT_SECONDS", defaultTimeoutSeconds, 1, 900)) * time.Second,
		toolsets:    csvValues(os.Getenv("HERMES_TOOLSETS")),
		envAllow:    csvValues(os.Getenv("HERMES_ENV_ALLOWLIST")),
		outputLimit: int64(boundedIntEnv("AGENT_RUNTIME_OUTPUT_LIMIT_BYTES", defaultOutputLimit, 4096, maxOutputLimit)),
	}
}

func (a *hermesAdapter) Info() Info {
	missing := []string{}
	if strings.TrimSpace(a.executable) == "" {
		missing = append(missing, "HERMES_EXECUTABLE")
	}
	if strings.TrimSpace(a.workspace) == "" {
		missing = append(missing, "HERMES_WORKSPACE")
	}
	if strings.TrimSpace(a.workspaceRoot) == "" {
		missing = append(missing, "AGENT_RUNTIME_WORKSPACE_ROOT")
	}
	workspaceReason := a.workspaceBlockedReason()
	return Info{
		ID:                   "hermes",
		Name:                 "Hermes Agent",
		Type:                 "hermes",
		Enabled:              a.enabled,
		Configured:           len(missing) == 0 && workspaceReason == "",
		ExecutionEnabled:     a.enabled && len(missing) == 0 && workspaceReason == "",
		RequiresApproval:     true,
		ReadOnlyDefault:      true,
		Capabilities:         []string{"single-query agent", "skills", "MCP tools", "checkpoints", "local workspace"},
		MissingConfiguration: missing,
		Endpoint:             a.executable,
	}
}

func (a *hermesAdapter) HealthCheck(ctx context.Context) Health {
	started := time.Now()
	health := Health{RuntimeID: "hermes", Status: "disabled", CheckedAt: time.Now().UTC()}
	if !a.enabled {
		health.Reason = "HERMES_AGENT_ENABLED is false"
		return health
	}
	if strings.TrimSpace(a.workspace) == "" {
		health.Status = "blocked"
		health.Reason = "HERMES_WORKSPACE is required"
		return health
	}
	if reason := a.workspaceBlockedReason(); reason != "" {
		health.Status = "blocked"
		health.Reason = reason
		return health
	}
	if stat, err := os.Stat(a.workspace); err != nil || !stat.IsDir() {
		health.Status = "blocked"
		health.Reason = "Hermes workspace is not an accessible directory"
		return health
	}
	path, err := exec.LookPath(a.executable)
	if err != nil {
		health.Status = "unavailable"
		health.Reason = "Hermes executable was not found"
		return health
	}
	health.Status = "ready"
	health.Reason = "Hermes executable and workspace are available: " + filepath.Base(path)
	health.LatencyMs = time.Since(started).Milliseconds()
	return health
}

func (a *hermesAdapter) ExecuteTask(parent context.Context, task Task) Result {
	started := time.Now()
	if reason := a.workspaceBlockedReason(); reason != "" {
		return Result{RuntimeID: "hermes", Status: "blocked", Message: reason, ExitCode: -1}
	}
	ctx, cancel := context.WithTimeout(parent, a.timeout)
	defer cancel()

	args := []string{"chat", "-q", task.Prompt, "-Q", "--source", "tool", "--max-turns", strconv.Itoa(a.maxTurns), "--checkpoints"}
	if len(a.toolsets) > 0 {
		args = append(args, "--toolsets", strings.Join(a.toolsets, ","))
	}
	cmd := exec.CommandContext(ctx, a.executable, args...)
	cmd.Dir = a.workspace
	cmd.Env = safeEnvironment(a.envAllow, map[string]string{
		"HAI_RUNTIME_TASK_ID": task.ID,
		"HAI_PROJECT_KEY":     task.ProjectKey,
		"TERMINAL_CWD":        a.workspace,
	})
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &limitedWriter{writer: &stdout, remaining: a.outputLimit}
	cmd.Stderr = &limitedWriter{writer: &stderr, remaining: a.outputLimit / 4}
	err := cmd.Run()
	output := trimAndRedact(stdout.String(), a.outputLimit)
	message := "Hermes completed the approved agent task"
	exitCode := 0
	status := "completed"
	if err != nil {
		status = "failed"
		message = safety.RedactSecrets(strings.TrimSpace(stderr.String()))
		if message == "" {
			message = err.Error()
		}
		exitCode = -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		if ctx.Err() == context.DeadlineExceeded {
			status = "blocked"
			message = "Hermes execution exceeded the configured timeout and was stopped"
		}
	}
	return Result{
		RuntimeID:  "hermes",
		Status:     status,
		Message:    message,
		Output:     output,
		ExitCode:   exitCode,
		DurationMs: time.Since(started).Milliseconds(),
		AuditEvents: []string{
			"server-side approval verified",
			"Hermes invoked without shell interpolation or --yolo",
			"filesystem checkpoints enabled",
			"bounded runtime output captured",
		},
	}
}

func (a *hermesAdapter) workspaceBlockedReason() string {
	if strings.TrimSpace(a.workspace) == "" || strings.TrimSpace(a.workspaceRoot) == "" {
		return ""
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
		return "Hermes workspace is invalid"
	}
	workspace, err = filepath.EvalSymlinks(workspace)
	if err != nil {
		return "Hermes workspace is not accessible"
	}
	relative, err := filepath.Rel(root, workspace)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) || filepath.IsAbs(relative) {
		return "Hermes workspace must stay inside AGENT_RUNTIME_WORKSPACE_ROOT"
	}
	return ""
}

type odysseusAdapter struct {
	enabled     bool
	baseURL     string
	token       string
	sessionID   string
	workspace   string
	timeout     time.Duration
	outputLimit int64
	allowedHost map[string]bool
}

func newOdysseusAdapterFromEnv() *odysseusAdapter {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("ODYSSEUS_BASE_URL")), "/")
	return &odysseusAdapter{
		enabled:     envEnabled("ODYSSEUS_AGENT_ENABLED"),
		baseURL:     baseURL,
		token:       strings.TrimSpace(os.Getenv("ODYSSEUS_API_TOKEN")),
		sessionID:   strings.TrimSpace(os.Getenv("ODYSSEUS_AGENT_SESSION_ID")),
		workspace:   strings.TrimSpace(os.Getenv("ODYSSEUS_AGENT_WORKSPACE")),
		timeout:     time.Duration(boundedIntEnv("ODYSSEUS_AGENT_TIMEOUT_SECONDS", defaultTimeoutSeconds, 1, 900)) * time.Second,
		outputLimit: int64(boundedIntEnv("AGENT_RUNTIME_OUTPUT_LIMIT_BYTES", defaultOutputLimit, 4096, maxOutputLimit)),
		allowedHost: csvMap(firstNonEmpty(os.Getenv("AGENT_RUNTIME_ALLOWED_HOSTS"), "localhost,127.0.0.1,::1,host.docker.internal,odysseus")),
	}
}

func (a *odysseusAdapter) Info() Info {
	missing := []string{}
	if a.baseURL == "" {
		missing = append(missing, "ODYSSEUS_BASE_URL")
	}
	if a.sessionID == "" {
		missing = append(missing, "ODYSSEUS_AGENT_SESSION_ID")
	}
	return Info{
		ID:                   "odysseus",
		Name:                 "Odysseus Workspace Agent",
		Type:                 "odysseus",
		Enabled:              a.enabled,
		Configured:           len(missing) == 0 && a.validBaseURL() == "",
		ExecutionEnabled:     a.enabled && len(missing) == 0 && a.validBaseURL() == "",
		RequiresApproval:     true,
		ReadOnlyDefault:      true,
		Capabilities:         []string{"agent mode", "scoped API tokens", "documents", "email", "calendar", "memory", "MCP tools"},
		MissingConfiguration: missing,
		Endpoint:             safety.RedactURL(a.baseURL),
	}
}

func (a *odysseusAdapter) validBaseURL() string {
	parsed, err := url.Parse(a.baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "ODYSSEUS_BASE_URL must be an absolute HTTP or HTTPS URL"
	}
	host := strings.ToLower(parsed.Hostname())
	if !a.allowedHost["*"] && !a.allowedHost[host] {
		return "Odysseus host is not in AGENT_RUNTIME_ALLOWED_HOSTS"
	}
	ip := net.ParseIP(host)
	if ip != nil && (ip.IsUnspecified() || ip.IsLinkLocalUnicast()) {
		return "Odysseus endpoint uses blocked address space"
	}
	return ""
}

func (a *odysseusAdapter) HealthCheck(parent context.Context) Health {
	started := time.Now()
	health := Health{RuntimeID: "odysseus", Status: "disabled", CheckedAt: time.Now().UTC()}
	if !a.enabled {
		health.Reason = "ODYSSEUS_AGENT_ENABLED is false"
		return health
	}
	if reason := a.validBaseURL(); reason != "" {
		health.Status = "blocked"
		health.Reason = reason
		return health
	}
	ctx, cancel := context.WithTimeout(parent, minDuration(a.timeout, 10*time.Second))
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/api/codex/capabilities", nil)
	a.authorize(req)
	resp, err := noRedirectClient(a.timeout).Do(req)
	if err != nil {
		health.Status = "unavailable"
		health.Reason = safety.RedactSecrets(err.Error())
		return health
	}
	defer resp.Body.Close()
	health.LatencyMs = time.Since(started).Milliseconds()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		health.Status = "auth_required"
		health.Reason = "Odysseus rejected the configured API token"
		return health
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		health.Status = "unavailable"
		health.Reason = fmt.Sprintf("Odysseus capabilities returned HTTP %d", resp.StatusCode)
		return health
	}
	health.Status = "ready"
	health.Reason = "Odysseus scoped capabilities API is reachable"
	return health
}

func (a *odysseusAdapter) ExecuteTask(parent context.Context, task Task) Result {
	started := time.Now()
	if reason := a.validBaseURL(); reason != "" {
		return Result{RuntimeID: "odysseus", Status: "blocked", Message: reason, ExitCode: -1}
	}
	ctx, cancel := context.WithTimeout(parent, a.timeout)
	defer cancel()
	form := url.Values{}
	form.Set("message", task.Prompt)
	form.Set("session", a.sessionID)
	form.Set("mode", "agent")
	form.Set("allow_bash", "false")
	form.Set("allow_web_search", "false")
	form.Set("use_web", "false")
	form.Set("use_research", "false")
	if a.workspace != "" {
		form.Set("workspace", a.workspace)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/api/chat_stream", strings.NewReader(form.Encode()))
	if err != nil {
		return runtimeFailure("odysseus", started, err.Error())
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", "018-HAI-Agent-Runtime/1.0")
	a.authorize(req)
	resp, err := noRedirectClient(a.timeout).Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return Result{RuntimeID: "odysseus", Status: "blocked", Message: "Odysseus execution exceeded the configured timeout and was stopped", ExitCode: -1, DurationMs: time.Since(started).Milliseconds()}
		}
		return runtimeFailure("odysseus", started, err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Result{
			RuntimeID:  "odysseus",
			Status:     "failed",
			Message:    fmt.Sprintf("Odysseus returned HTTP %d: %s", resp.StatusCode, trimAndRedact(string(body), 4096)),
			ExitCode:   resp.StatusCode,
			DurationMs: time.Since(started).Milliseconds(),
		}
	}
	output, streamErr := readOdysseusStream(resp.Body, a.outputLimit)
	if streamErr != nil {
		return runtimeFailure("odysseus", started, streamErr.Error())
	}
	if strings.TrimSpace(output) == "" {
		return Result{RuntimeID: "odysseus", Status: "failed", Message: "Odysseus completed without a final response", ExitCode: -1, DurationMs: time.Since(started).Milliseconds()}
	}
	return Result{
		RuntimeID:  "odysseus",
		Status:     "completed",
		Message:    "Odysseus completed the approved agent task",
		Output:     output,
		ExitCode:   0,
		DurationMs: time.Since(started).Milliseconds(),
		AuditEvents: []string{
			"server-side approval verified",
			"Odysseus agent mode invoked with bash and web search disabled",
			"configured session and scoped API token used",
			"bounded SSE output captured",
		},
	}
}

func (a *odysseusAdapter) authorize(req *http.Request) {
	if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}
}

func readOdysseusStream(reader io.Reader, limit int64) (string, error) {
	scanner := bufio.NewScanner(io.LimitReader(reader, limit))
	scanner.Buffer(make([]byte, 64*1024), int(minInt64(limit, 1024*1024)))
	var output strings.Builder
	done := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			done = true
			break
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			continue
		}
		if delta, ok := event["delta"].(string); ok {
			output.WriteString(delta)
		}
		if message, ok := event["error"].(string); ok && strings.TrimSpace(message) != "" {
			return "", fmt.Errorf("Odysseus stream error: %s", safety.RedactSecrets(message))
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	if !done {
		return "", fmt.Errorf("Odysseus stream ended before completion or exceeded the configured output limit")
	}
	return trimAndRedact(output.String(), limit), nil
}

type limitedWriter struct {
	writer    io.Writer
	remaining int64
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	original := len(p)
	if w.remaining <= 0 {
		return original, nil
	}
	if int64(len(p)) > w.remaining {
		p = p[:w.remaining]
	}
	_, err := w.writer.Write(p)
	w.remaining -= int64(len(p))
	return original, err
}

func safeEnvironment(allow []string, additions map[string]string) []string {
	env := []string{}
	for _, key := range append([]string{"PATH", "HOME", "USERPROFILE", "SYSTEMROOT", "WINDIR", "TEMP", "TMP"}, allow...) {
		if value, ok := os.LookupEnv(key); ok && validEnvKey(key) {
			env = append(env, key+"="+value)
		}
	}
	for key, value := range additions {
		if validEnvKey(key) && strings.TrimSpace(value) != "" {
			env = append(env, key+"="+value)
		}
	}
	return env
}

func runtimeFailure(runtimeID string, started time.Time, message string) Result {
	return Result{
		RuntimeID:  runtimeID,
		Status:     "failed",
		Message:    safety.RedactSecrets(message),
		ExitCode:   -1,
		DurationMs: time.Since(started).Milliseconds(),
	}
}

func noRedirectClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func csvValues(value string) []string {
	result := []string{}
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func csvMap(value string) map[string]bool {
	result := map[string]bool{}
	for _, item := range csvValues(value) {
		result[strings.ToLower(item)] = true
	}
	return result
}

func envEnabled(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "true", "1", "yes":
		return true
	default:
		return false
	}
}

func intEnv(name string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func boundedIntEnv(name string, fallback, minimum, maximum int) int {
	value := intEnv(name, fallback)
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func validEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for index, r := range key {
		letter := r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z'
		digit := r >= '0' && r <= '9'
		if letter || index > 0 && (digit || r == '_') {
			continue
		}
		return false
	}
	return true
}

func trimAndRedact(value string, limit int64) string {
	value = strings.TrimSpace(value)
	if int64(len(value)) > limit {
		value = value[:limit]
	}
	return strings.TrimSpace(safety.RedactSecrets(value))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func minDuration(left, right time.Duration) time.Duration {
	if left < right {
		return left
	}
	return right
}

func minInt64(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}
