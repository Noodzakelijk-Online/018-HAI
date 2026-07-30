// Package gosec exposes a deliberately narrow Go source security boundary.
// It scans named, operator-allowlisted read-only snapshots and returns
// aggregate severity and confidence counts only. It never exposes source,
// file paths, rules, CWEs, findings, raw reports, or remediation steps.
package gosec

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
	enabledEnv       = "HAI_GOSEC_ENABLED"
	runnerURLEnv     = "HAI_GOSEC_RUNNER_URL"
	tokenEnv         = "HAI_GOSEC_RUNNER_TOKEN"
	workspacesEnv    = "HAI_GOSEC_WORKSPACES"
	timeoutEnv       = "HAI_GOSEC_TIMEOUT_SECONDS"
	maxResponseBytes = 32 << 10
	maxWorkspaces    = 8
	defaultTimeout   = 120 * time.Second
)

var (
	ErrNotConfigured  = errors.New("local Gosec runner is not configured")
	ErrUnavailable    = errors.New("local Gosec runner is unavailable")
	ErrWorkspace      = errors.New("workspace is not approved for Go security scanning")
	workspacePattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
	severityPattern   = regexp.MustCompile(`^(high|medium|low)$`)
	confidencePattern = regexp.MustCompile(`^(high|medium|low)$`)
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

type ConfidenceCount struct {
	Confidence string `json:"confidence"`
	Count      int    `json:"count"`
}

type ScanResult struct {
	Status       string            `json:"status"`
	Engine       string            `json:"engine"`
	WorkspaceID  string            `json:"workspaceId"`
	FindingCount int               `json:"findingCount"`
	Severities   []SeverityCount   `json:"severities"`
	Confidences  []ConfidenceCount `json:"confidences"`
	DurationMS   int64             `json:"durationMs"`
	ResultDigest string            `json:"resultDigest"`
	Scope        string            `json:"scope"`
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
		Enabled: s.enabled, Configured: s.configured(), Provider: "Gosec local aggregate Go security scanner", Workspaces: workspaces, ConfigError: s.configErr,
		Capabilities: []string{"operator-triggered aggregate static security analysis for an explicitly allowlisted local Go snapshot", "redacted finding total, severity/confidence counts, duration, and result digest"},
		Restrictions: []string{"no caller-selected path, source content, package, finding, rule, CWE, file path, raw report, cloud upload, external network, write operation, remediation, or scheduled scan", "the runner requires a self-contained vendored Go snapshot and forces module/network resolution off", "a scan result cannot approve an action, verify task completion, alter source, or automatically block a workflow"},
		Scope:        "Owner-triggered local Go source security evidence only. HAI returns aggregate redacted metadata from a configured snapshot; findings require human review in the original workspace and do not become source facts or execution decisions.",
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
	request.Header.Set("User-Agent", "HAI-Gosec/1.0")
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
	return &ProbeResult{Reachable: true, Engine: body.Engine, CheckedAt: s.now().UTC(), Scope: "Runner reachability only. The probe does not scan a workspace or read source content."}, nil
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
	request.Header.Set("User-Agent", "HAI-Gosec/1.0")
	request.Header.Set("X-HAI-Gosec-Token", s.token)
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
	if host != "localhost" && host != "host.docker.internal" && host != "gosec-runner" && net.ParseIP(host) == nil {
		return nil, runnerURLEnv + " must resolve to localhost, host.docker.internal, gosec-runner, or a literal local IP"
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() && !ip.IsPrivate() {
		return nil, runnerURLEnv + " must use a loopback or private-network IP"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed, ""
}

func validEngine(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), "gosec ") && len(value) <= 160
}

func validResult(result ScanResult, workspaceID string) bool {
	if result.Status != "completed" || !validEngine(result.Engine) || result.WorkspaceID != workspaceID || result.FindingCount < 0 || result.FindingCount > 100000 || len(result.Severities) > 3 || len(result.Confidences) > 3 || result.DurationMS < 0 || result.DurationMS > int64((5*time.Minute)/time.Millisecond) {
		return false
	}
	if !countsMatchFindings(result.FindingCount, result.Severities, func(item SeverityCount) (string, int) { return item.Severity, item.Count }, severityPattern) {
		return false
	}
	if !countsMatchFindings(result.FindingCount, result.Confidences, func(item ConfidenceCount) (string, int) { return item.Confidence, item.Count }, confidencePattern) {
		return false
	}
	_, err := hex.DecodeString(result.ResultDigest)
	return len(result.ResultDigest) == sha256.Size*2 && err == nil
}

func countsMatchFindings[T any](findingCount int, values []T, read func(T) (string, int), pattern *regexp.Regexp) bool {
	if findingCount == 0 {
		return len(values) == 0
	}
	count, seen := 0, map[string]bool{}
	for _, value := range values {
		label, amount := read(value)
		if !pattern.MatchString(label) || amount <= 0 || amount > findingCount || seen[label] {
			return false
		}
		seen[label] = true
		count += amount
	}
	return count == findingCount
}
