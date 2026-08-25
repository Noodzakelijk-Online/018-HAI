// hai-dsh-bridge is a deliberately narrow Windows host worker for the
// DeepSeek Harness adapter. It has no listener: it only polls HAI's separate
// loopback-only gateway and executes leased, already-approved jobs.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	maxOutputBytes                = 16 * 1024
	pollInterval                  = 2 * time.Second
	executionConfirmationInterval = 2 * time.Second
)

var (
	errEmergencyStop = errors.New("host runtime execution is blocked by emergency stop")
	errStaleLease    = errors.New("host runtime lease is no longer valid")
)

type config struct {
	baseURL      *url.URL
	token        string
	executable   string
	version      string
	workspace    string
	stateDir     string
	workspaceKey string
	timeout      time.Duration
	envAllow     []string
}

type lease struct {
	Job struct {
		ID           string `json:"id"`
		RuntimeID    string `json:"runtimeId"`
		Prompt       string `json:"prompt"`
		WorkspaceKey string `json:"workspaceKey"`
	} `json:"job"`
	Token string `json:"leaseToken"`
}

type completion struct {
	LeaseToken string `json:"leaseToken"`
	ExitCode   int    `json:"exitCode"`
	Output     string `json:"output"`
	Error      string `json:"error"`
}

type confirmRequest struct {
	LeaseToken string `json:"leaseToken"`
}

func main() {
	configuration, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "hai-dsh-bridge:", err)
		os.Exit(2)
	}
	if err := verifyVersion(context.Background(), configuration); err != nil {
		fmt.Fprintln(os.Stderr, "hai-dsh-bridge:", err)
		os.Exit(2)
	}
	if err := run(context.Background(), configuration); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "hai-dsh-bridge:", err)
		os.Exit(1)
	}
}

func loadConfig() (config, error) {
	rawURL := strings.TrimSpace(os.Getenv("HAI_HOST_RUNTIME_BRIDGE_URL"))
	endpoint, err := url.Parse(rawURL)
	if err != nil || !isLoopbackURL(endpoint) {
		return config{}, errors.New("HAI_HOST_RUNTIME_BRIDGE_URL must be an http loopback URL without credentials, query, or fragment")
	}
	configuration := config{
		baseURL:      endpoint,
		token:        strings.TrimSpace(os.Getenv("HAI_HOST_RUNTIME_BRIDGE_TOKEN")),
		executable:   firstNonEmpty(os.Getenv("DEEPSEEK_HARNESS_EXECUTABLE"), "dsh"),
		version:      strings.TrimSpace(os.Getenv("DEEPSEEK_HARNESS_VERSION")),
		workspace:    strings.TrimSpace(os.Getenv("DEEPSEEK_HARNESS_WORKSPACE")),
		stateDir:     strings.TrimSpace(os.Getenv("DEEPSEEK_HARNESS_STATE_DIR")),
		workspaceKey: firstNonEmpty(os.Getenv("DEEPSEEK_HARNESS_WORKSPACE_KEY"), "deepseek-harness"),
		timeout:      boundedSeconds(os.Getenv("DEEPSEEK_HARNESS_TIMEOUT_SECONDS"), 120),
		envAllow:     csvValues(os.Getenv("DEEPSEEK_HARNESS_ENV_ALLOWLIST")),
	}
	if len(configuration.token) < 32 || configuration.version == "" || configuration.workspaceKey == "" {
		return config{}, errors.New("bridge token, pinned Harness version, and workspace key are required")
	}
	if _, err := exec.LookPath(configuration.executable); err != nil {
		return config{}, fmt.Errorf("DeepSeek Harness executable is unavailable: %w", err)
	}
	workspace, stateDir, err := validatedWorkspace(configuration.workspace, configuration.stateDir)
	if err != nil {
		return config{}, err
	}
	configuration.workspace, configuration.stateDir = workspace, stateDir
	return configuration, nil
}

func run(ctx context.Context, configuration config) error {
	client := &http.Client{Timeout: 15 * time.Second}
	for {
		leased, found, err := requestLease(ctx, client, configuration)
		if err != nil {
			return err
		}
		if !found {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(pollInterval):
			}
			continue
		}
		if leased.Job.RuntimeID != "deepseek-harness" || leased.Job.WorkspaceKey != configuration.workspaceKey || invalidPrompt(leased.Job.Prompt) != "" {
			if err := submitCompletion(ctx, client, configuration, leased, completion{LeaseToken: leased.Token, ExitCode: -1, Error: "leased job violates the configured Windows host policy"}); err != nil {
				return err
			}
			continue
		}
		if err := confirmLease(ctx, client, configuration, leased); err != nil {
			result := completion{
				LeaseToken: leased.Token,
				ExitCode:   -1,
				Error:      "Windows host execution was blocked before DeepSeek Harness started: " + bridgeError(err),
			}
			if submitErr := submitCompletion(ctx, client, configuration, leased, result); submitErr != nil {
				return submitErr
			}
			continue
		}
		result := executeWithLeaseMonitor(ctx, client, configuration, leased)
		result.LeaseToken = leased.Token
		if err := submitCompletion(ctx, client, configuration, leased, result); err != nil {
			return err
		}
	}
}

func confirmLease(ctx context.Context, client *http.Client, configuration config, leased lease) error {
	payload, err := json.Marshal(confirmRequest{LeaseToken: leased.Token})
	if err != nil {
		return err
	}
	request, err := newRequest(ctx, configuration, http.MethodPost, "/api/v1/host-runtime/leases/"+url.PathEscape(leased.Job.ID)+"/confirm", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("confirm host runtime lease: %w", err)
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusNoContent:
		return nil
	case http.StatusLocked:
		return errEmergencyStop
	case http.StatusConflict:
		return errStaleLease
	default:
		return fmt.Errorf("confirm host runtime lease: gateway returned %s", response.Status)
	}
}

// executeWithLeaseMonitor keeps the final execution gate live while DSH is
// running. A lease can become unsafe after the process starts when the owner
// activates emergency stop, so a launch-only check is insufficient. Any
// failed confirmation stops the local process rather than letting host work
// continue without HAI's current approval state.
func executeWithLeaseMonitor(parent context.Context, client *http.Client, configuration config, leased lease) completion {
	return monitorExecution(parent, executionConfirmationInterval, func(checkContext context.Context) error {
		return confirmLease(checkContext, client, configuration, leased)
	}, func(executionContext context.Context) completion {
		return execute(executionContext, configuration, leased.Job.Prompt)
	})
}

func monitorExecution(parent context.Context, interval time.Duration, confirm func(context.Context) error, launch func(context.Context) completion) completion {
	if interval <= 0 {
		interval = executionConfirmationInterval
	}
	executionContext, cancel := context.WithCancel(parent)
	defer cancel()
	completed := make(chan completion, 1)
	go func() {
		completed <- launch(executionContext)
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case result := <-completed:
			return result
		case <-parent.Done():
			return <-completed
		case <-ticker.C:
			confirmationContext, confirmationCancel := context.WithTimeout(parent, 15*time.Second)
			err := confirm(confirmationContext)
			confirmationCancel()
			if err == nil {
				continue
			}
			cancel()
			result := <-completed
			result.ExitCode = -1
			result.Error = executionStoppedReason(err)
			return result
		}
	}
}

func executionStoppedReason(err error) string {
	switch {
	case errors.Is(err, errEmergencyStop):
		return "DeepSeek Harness execution was stopped because HAI emergency stop is active"
	case errors.Is(err, errStaleLease):
		return "DeepSeek Harness execution was stopped because its HAI execution lease is no longer valid"
	default:
		return "DeepSeek Harness execution was stopped because HAI could not reconfirm the execution lease: " + bridgeError(err)
	}
}

func requestLease(ctx context.Context, client *http.Client, configuration config) (lease, bool, error) {
	request, err := newRequest(ctx, configuration, http.MethodPost, "/api/v1/host-runtime/leases", nil)
	if err != nil {
		return lease{}, false, err
	}
	response, err := client.Do(request)
	if err != nil {
		return lease{}, false, fmt.Errorf("request host runtime lease: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent {
		return lease{}, false, nil
	}
	if response.StatusCode != http.StatusOK {
		return lease{}, false, fmt.Errorf("request host runtime lease: gateway returned %s", response.Status)
	}
	var leased lease
	if err := json.NewDecoder(io.LimitReader(response.Body, 64*1024)).Decode(&leased); err != nil || leased.Token == "" || leased.Job.ID == "" {
		return lease{}, false, errors.New("request host runtime lease: invalid gateway response")
	}
	return leased, true, nil
}

func submitCompletion(ctx context.Context, client *http.Client, configuration config, leased lease, result completion) error {
	payload, err := json.Marshal(result)
	if err != nil {
		return err
	}
	request, err := newRequest(ctx, configuration, http.MethodPost, "/api/v1/host-runtime/leases/"+url.PathEscape(leased.Job.ID)+"/complete", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("submit host runtime completion: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("submit host runtime completion: gateway returned %s", response.Status)
	}
	return nil
}

func bridgeError(err error) string {
	message := strings.TrimSpace(err.Error())
	if len(message) > 256 {
		message = strings.TrimSpace(message[:256])
	}
	if message == "" {
		return "confirmation failed"
	}
	return message
}

func newRequest(ctx context.Context, configuration config, method, requestPath string, body io.Reader) (*http.Request, error) {
	endpoint := configuration.baseURL.ResolveReference(&url.URL{Path: requestPath})
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+configuration.token)
	return request, nil
}

func execute(parent context.Context, configuration config, prompt string) completion {
	if reason := invalidPrompt(prompt); reason != "" {
		return completion{ExitCode: -1, Error: reason}
	}
	ctx, cancel := context.WithTimeout(parent, configuration.timeout)
	defer cancel()
	command := exec.CommandContext(ctx, configuration.executable, "--profile", "headless", prompt)
	command.Dir = configuration.workspace
	command.Env = safeEnvironment(configuration.envAllow, map[string]string{
		"DSH_HOME":     configuration.stateDir,
		"TERMINAL_CWD": configuration.workspace,
	})
	var stdout, stderr limitedBuffer
	stdout.remaining, stderr.remaining = maxOutputBytes, maxOutputBytes/4
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	if err == nil {
		return completion{ExitCode: 0, Output: stdout.String()}
	}
	if ctx.Err() == context.DeadlineExceeded {
		return completion{ExitCode: -1, Output: stdout.String(), Error: "DeepSeek Harness execution exceeded the configured timeout and was stopped"}
	}
	exitCode := -1
	if exitError, ok := err.(*exec.ExitError); ok {
		exitCode = exitError.ExitCode()
	}
	diagnostic := strings.TrimSpace(stderr.String())
	if diagnostic == "" {
		diagnostic = "DeepSeek Harness process failed without diagnostic output"
	}
	return completion{ExitCode: exitCode, Output: stdout.String(), Error: diagnostic}
}

func verifyVersion(ctx context.Context, configuration config) error {
	probe, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	command := exec.CommandContext(probe, configuration.executable, "--version")
	command.Dir = configuration.workspace
	command.Env = safeEnvironment(configuration.envAllow, map[string]string{"DSH_HOME": configuration.stateDir})
	output, err := command.Output()
	if err != nil {
		return errors.New("DeepSeek Harness version probe failed")
	}
	if !strings.Contains(string(output), configuration.version) {
		return fmt.Errorf("DeepSeek Harness version mismatch: expected %q", configuration.version)
	}
	return nil
}

func validatedWorkspace(workspace, stateDir string) (string, string, error) {
	workspace, err := filepath.EvalSymlinks(filepath.Clean(workspace))
	if err != nil {
		return "", "", errors.New("DeepSeek Harness workspace is not an accessible directory")
	}
	info, err := os.Stat(workspace)
	if err != nil || !info.IsDir() {
		return "", "", errors.New("DeepSeek Harness workspace is not an accessible directory")
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return "", "", errors.New("DeepSeek Harness state directory cannot be created")
	}
	stateDir, err = filepath.EvalSymlinks(filepath.Clean(stateDir))
	if err != nil {
		return "", "", errors.New("DeepSeek Harness state directory is not accessible")
	}
	relative, err := filepath.Rel(workspace, stateDir)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) || filepath.IsAbs(relative) {
		return "", "", errors.New("DeepSeek Harness state directory must stay inside the configured workspace")
	}
	return workspace, stateDir, nil
}

func invalidPrompt(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" || len(prompt) > 50*1024 || strings.HasPrefix(prompt, "-") || prompt == "web" || prompt == "plugin" || strings.ContainsRune(prompt, 0) {
		return "leased task prompt violates the DeepSeek Harness execution policy"
	}
	return ""
}

func isLoopbackURL(endpoint *url.URL) bool {
	if endpoint == nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return false
	}
	host := endpoint.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	return net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
}

func boundedSeconds(value string, fallback int) time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || seconds < 1 || seconds > 900 {
		seconds = fallback
	}
	return time.Duration(seconds) * time.Second
}

func csvValues(value string) []string {
	result := []string{}
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" && validEnvKey(item) {
			result = append(result, item)
		}
	}
	return result
}

func safeEnvironment(allowlist []string, fixed map[string]string) []string {
	// The host worker has a dedicated DSH_HOME inside its approved workspace.
	// Do not pass Windows profile locations through to the child process: they
	// can contain browser sessions, SSH material, cloud credentials, and other
	// unrelated user state that the approved job was never authorized to use.
	allowed := map[string]bool{
		"COMSPEC": true, "PATH": true, "PATHEXT": true, "SYSTEMROOT": true,
		"TEMP": true, "TMP": true, "WINDIR": true,
	}
	for _, key := range allowlist {
		allowed[strings.ToUpper(key)] = true
	}
	fixedValues := make(map[string]string, len(fixed))
	for key := range fixed {
		normalized := strings.ToUpper(key)
		allowed[normalized] = true
		fixedValues[normalized] = fixed[key]
	}
	values := make(map[string]string, len(allowed))
	for _, pair := range os.Environ() {
		key, _, found := strings.Cut(pair, "=")
		normalized := strings.ToUpper(key)
		if found && allowed[normalized] {
			// Fixed runtime values must not be shadowed by an ambient value.
			if _, fixed := fixedValues[normalized]; !fixed {
				values[normalized] = pair
			}
		}
	}
	for key, value := range fixedValues {
		values[key] = key + "=" + value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, values[key])
	}
	return result
}

func validEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for index, character := range key {
		letter := character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z'
		digit := character >= '0' && character <= '9'
		if letter || index > 0 && (digit || character == '_') {
			continue
		}
		return false
	}
	return true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

type limitedBuffer struct {
	buffer    bytes.Buffer
	remaining int
}

func (b *limitedBuffer) Write(value []byte) (int, error) {
	if b.remaining > 0 {
		count := len(value)
		if count > b.remaining {
			count = b.remaining
		}
		_, _ = b.buffer.Write(value[:count])
		b.remaining -= count
	}
	return len(value), nil
}

func (b *limitedBuffer) String() string { return strings.TrimSpace(b.buffer.String()) }
