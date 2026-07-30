// Package grype exposes a deliberately narrow vulnerability-evidence boundary.
// It scans named, operator-allowlisted read-only snapshots with a locally
// supplied advisory database and returns aggregate counts only. It never
// exposes a package, version, CVE, path, source file, raw report, or fix.
package grype

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	enabledEnv       = "HAI_GRYPE_ENABLED"
	runnerURLEnv     = "HAI_GRYPE_RUNNER_URL"
	tokenEnv         = "HAI_GRYPE_RUNNER_TOKEN"
	workspacesEnv    = "HAI_GRYPE_WORKSPACES"
	timeoutEnv       = "HAI_GRYPE_TIMEOUT_SECONDS"
	maxResponseBytes = 32 << 10
	maxWorkspaces    = 8
	defaultTimeout   = 120 * time.Second
)

var (
	ErrNotConfigured = errors.New("local Grype runner is not configured")
	ErrUnavailable   = errors.New("local Grype runner is unavailable")
	ErrWorkspace     = errors.New("workspace is not approved for vulnerability scanning")
	workspacePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
	severityPattern  = regexp.MustCompile(`^(critical|high|medium|low|negligible|unknown)$`)
)

type Status struct {
	Enabled      bool     `json:"enabled"`
	Configured   bool     `json:"configured"`
	Provider     string   `json:"provider"`
	Endpoint     string   `json:"endpoint,omitempty"`
	Workspaces   []string `json:"workspaces"`
	ConfigError  string   `json:"configError,omitempty"`
	Capabilities []string `json:"capabilities"`
	Restrictions []string `json:"restrictions"`
	Scope        string   `json:"scope"`
}

type ProbeResult struct {
	Reachable bool      `json:"reachable"`
	Engine    string    `json:"engine"`
	CheckedAt time.Time `json:"checkedAt"`
	Scope     string    `json:"scope"`
}

type SeverityCount struct {
	Severity string `json:"severity"`
	Count    int    `json:"count"`
}

type ScanResult struct {
	Status             string          `json:"status"`
	Engine             string          `json:"engine"`
	WorkspaceID        string          `json:"workspaceId"`
	VulnerabilityCount int             `json:"vulnerabilityCount"`
	FixAvailableCount  int             `json:"fixAvailableCount"`
	Severities         []SeverityCount `json:"severities"`
	DurationMS         int64           `json:"durationMs"`
	ResultDigest       string          `json:"resultDigest"`
	Scope              string          `json:"scope"`
}

type Config struct {
	Enabled    bool
	RunnerURL  string
	Token      string
	Workspaces []string
	Timeout    time.Duration
}

type Service interface {
	Status() Status
	Probe(context.Context) (*ProbeResult, error)
	Scan(context.Context, string) (*ScanResult, error)
}

type service struct {
	enabled    bool
	runnerURL  *url.URL
	token      string
	workspaces map[string]bool
	configErr  string
	client     *http.Client
	now        func() time.Time
}

func ConfigFromEnv() Config {
	timeout := defaultTimeout
	if raw := strings.TrimSpace(os.Getenv(timeoutEnv)); raw != "" {
		if seconds, err := time.ParseDuration(raw + "s"); err == nil && seconds >= 15*time.Second && seconds <= 5*time.Minute {
			timeout = seconds
		}
	}
	return Config{Enabled: strings.EqualFold(strings.TrimSpace(os.Getenv(enabledEnv)), "true"), RunnerURL: os.Getenv(runnerURLEnv), Token: os.Getenv(tokenEnv), Workspaces: parseWorkspaces(os.Getenv(workspacesEnv)), Timeout: timeout}
}

func DefaultService() Service { return NewService(ConfigFromEnv(), nil) }

func NewService(config Config, client *http.Client) Service {
	if config.Timeout < 15*time.Second || config.Timeout > 5*time.Minute {
		config.Timeout = defaultTimeout
	}
	if client == nil {
		client = &http.Client{Timeout: config.Timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }, Transport: &http.Transport{Proxy: nil}}
	}
	s := &service{enabled: config.Enabled, token: strings.TrimSpace(config.Token), workspaces: map[string]bool{}, client: client, now: time.Now}
	for _, workspace := range config.Workspaces {
		s.workspaces[workspace] = true
	}
	if s.enabled {
		s.runnerURL, s.configErr = parseRunnerURL(config.RunnerURL)
		if s.configErr == "" && len(s.token) < 16 {
			s.configErr = tokenEnv + " must contain a separate local-only token with at least 16 characters"
		}
		if s.configErr == "" && (len(s.workspaces) == 0 || len(s.workspaces) > maxWorkspaces || !validWorkspaceConfig(config.Workspaces)) {
			s.configErr = workspacesEnv + " requires one to eight valid, distinct reviewed snapshot names"
		}
	}
	return s
}

func (s *service) Status() Status {
	workspaces := make([]string, 0, len(s.workspaces))
	for workspace := range s.workspaces {
		workspaces = append(workspaces, workspace)
	}
	sort.Strings(workspaces)
	status := Status{
		Enabled: s.enabled, Configured: s.configured(), Provider: "Grype local aggregate vulnerability scanner", Workspaces: workspaces, ConfigError: s.configErr,
		Capabilities: []string{"operator-triggered aggregate vulnerability evidence for an explicitly allowlisted local snapshot", "redacted total, severity counts, fix-availability count, duration, and result digest"},
		Restrictions: []string{"no caller-selected path, source content, package, version, CVE, advisory, file path, raw report, cloud upload, external network, write operation, remediation, or scheduled scan", "the runner uses a separately mounted local advisory database with upstream update checks disabled", "a scan result cannot approve an action, verify task completion, alter a dependency, or automatically block a workflow"},
		Scope:        "Owner-triggered local vulnerability evidence only. HAI returns aggregate redacted metadata from a configured snapshot; findings require human review in the original workspace and do not become source facts or execution decisions.",
	}
	if s.runnerURL != nil {
		status.Endpoint = s.runnerURL.String()
	}
	return status
}

func (s *service) Probe(ctx context.Context) (*ProbeResult, error) {
	if !s.configured() {
		return nil, ErrNotConfigured
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.endpoint("/healthz").String(), nil)
	if err != nil {
		return nil, ErrUnavailable
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "HAI-Grype/1.0")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer response.Body.Close()
	var body struct {
		Status     string `json:"status"`
		Engine     string `json:"engine"`
		Configured bool   `json:"configured"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&body) != nil || body.Status != "ok" || !body.Configured || !validEngine(body.Engine) {
		return nil, ErrUnavailable
	}
	return &ProbeResult{Reachable: true, Engine: body.Engine, CheckedAt: s.now().UTC(), Scope: "Runner reachability only. The probe does not scan a workspace, contact an advisory service, or read source content."}, nil
}

func (s *service) Scan(ctx context.Context, workspaceID string) (*ScanResult, error) {
	if !s.configured() {
		return nil, ErrNotConfigured
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if !s.workspaces[workspaceID] {
		return nil, ErrWorkspace
	}
	body, _ := json.Marshal(map[string]string{"workspaceId": workspaceID})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint("/v1/scan").String(), bytes.NewReader(body))
	if err != nil {
		return nil, ErrUnavailable
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "HAI-Grype/1.0")
	request.Header.Set("X-HAI-Grype-Token", s.token)
	response, err := s.client.Do(request)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, ErrUnavailable
	}
	var result ScanResult
	if json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes)).Decode(&result) != nil || !validResult(result, workspaceID) {
		return nil, ErrUnavailable
	}
	result.Scope = s.Status().Scope
	return &result, nil
}

func (s *service) configured() bool { return s.enabled && s.configErr == "" && s.runnerURL != nil }

func (s *service) endpoint(path string) *url.URL {
	endpoint := *s.runnerURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + path
	return &endpoint
}

func parseWorkspaces(raw string) []string {
	seen, result := map[string]bool{}, []string{}
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if workspacePattern.MatchString(value) && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func validWorkspaceConfig(values []string) bool {
	if len(values) == 0 || len(values) > maxWorkspaces {
		return false
	}
	seen := map[string]bool{}
	for _, value := range values {
		if !workspacePattern.MatchString(value) || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func parseRunnerURL(raw string) (*url.URL, string) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, runnerURLEnv + " must be a plain local HTTP URL without credentials, query data, or fragments"
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "localhost" && host != "host.docker.internal" && host != "grype-runner" && net.ParseIP(host) == nil {
		return nil, runnerURLEnv + " must resolve to localhost, host.docker.internal, grype-runner, or a literal local IP"
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() && !ip.IsPrivate() {
		return nil, runnerURLEnv + " must use a loopback or private-network IP"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed, ""
}

func validEngine(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), "grype ") && len(value) <= 160
}

func validResult(result ScanResult, workspaceID string) bool {
	if result.Status != "completed" || !validEngine(result.Engine) || result.WorkspaceID != workspaceID || result.VulnerabilityCount < 0 || result.VulnerabilityCount > 100000 || result.FixAvailableCount < 0 || result.FixAvailableCount > result.VulnerabilityCount || len(result.Severities) > 6 || result.DurationMS < 0 || result.DurationMS > int64((5*time.Minute)/time.Millisecond) {
		return false
	}
	count, seen := 0, map[string]bool{}
	for _, severity := range result.Severities {
		if !severityPattern.MatchString(severity.Severity) || severity.Count <= 0 || severity.Count > result.VulnerabilityCount || seen[severity.Severity] {
			return false
		}
		seen[severity.Severity] = true
		count += severity.Count
	}
	if count != result.VulnerabilityCount {
		return false
	}
	_, err := hex.DecodeString(result.ResultDigest)
	return len(result.ResultDigest) == sha256.Size*2 && err == nil
}
