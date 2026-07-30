// Package gitleaks exposes a deliberately narrow local secret-scan boundary.
// It scans only named, operator-allowlisted disposable snapshots and returns
// aggregate redacted metadata. It never returns matched text, a secret, path,
// line number, commit, author, raw report, or source file.
package gitleaks

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
	enabledEnv       = "HAI_GITLEAKS_ENABLED"
	runnerURLEnv     = "HAI_GITLEAKS_RUNNER_URL"
	tokenEnv         = "HAI_GITLEAKS_RUNNER_TOKEN"
	workspacesEnv    = "HAI_GITLEAKS_WORKSPACES"
	timeoutEnv       = "HAI_GITLEAKS_TIMEOUT_SECONDS"
	maxResponseBytes = 32 << 10
	maxWorkspaces    = 8
	defaultTimeout   = 120 * time.Second
)

var (
	ErrNotConfigured = errors.New("local Gitleaks runner is not configured")
	ErrUnavailable   = errors.New("local Gitleaks runner is unavailable")
	ErrWorkspace     = errors.New("workspace is not approved for secret scanning")
	workspacePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
	rulePattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
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

type RuleCount struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

type ScanResult struct {
	Status             string      `json:"status"`
	Engine             string      `json:"engine"`
	WorkspaceID        string      `json:"workspaceId"`
	FindingCount       int         `json:"findingCount"`
	AffectedFiles      int         `json:"affectedFiles"`
	Rules              []RuleCount `json:"rules"`
	DurationMS         int64       `json:"durationMs"`
	ResultDigest       string      `json:"resultDigest"`
	Scope              string      `json:"scope"`
	WorkflowID         string      `json:"workflowId,omitempty"`
	WorkflowLinkStatus string      `json:"workflowLinkStatus,omitempty"`
	WorkflowLinkError  string      `json:"workflowLinkError,omitempty"`
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

// WorkflowLinker adds redacted aggregate scan metadata to an existing HAI
// workflow. It cannot mark a workflow complete or alter its execution state.
type WorkflowLinker interface {
	AttachSecretScan(ownerIdentity, workflowID, workspaceID, resultDigest string, findingCount, affectedFiles int) error
}

// WorkflowScanService is optional so existing scan callers and test doubles
// keep the small Service contract. HAI's concrete service implements it when
// the router has supplied the workflow linker.
type WorkflowScanService interface {
	ScanWithWorkflow(context.Context, string, string, string) (*ScanResult, error)
}

type service struct {
	enabled    bool
	runnerURL  *url.URL
	token      string
	workspaces map[string]bool
	configErr  string
	client     *http.Client
	now        func() time.Time
	workflows  WorkflowLinker
}

func ConfigFromEnv() Config {
	timeout := defaultTimeout
	if raw := strings.TrimSpace(os.Getenv(timeoutEnv)); raw != "" {
		if seconds, err := time.ParseDuration(raw + "s"); err == nil && seconds >= 15*time.Second && seconds <= 5*time.Minute {
			timeout = seconds
		}
	}
	return Config{
		Enabled:    strings.EqualFold(strings.TrimSpace(os.Getenv(enabledEnv)), "true"),
		RunnerURL:  os.Getenv(runnerURLEnv),
		Token:      os.Getenv(tokenEnv),
		Workspaces: parseWorkspaces(os.Getenv(workspacesEnv)),
		Timeout:    timeout,
	}
}

func DefaultService(workflows ...WorkflowLinker) Service {
	return NewService(ConfigFromEnv(), nil, workflows...)
}

func NewService(config Config, client *http.Client, workflows ...WorkflowLinker) Service {
	if config.Timeout < 15*time.Second || config.Timeout > 5*time.Minute {
		config.Timeout = defaultTimeout
	}
	if client == nil {
		client = &http.Client{Timeout: config.Timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }, Transport: &http.Transport{Proxy: nil}}
	}
	s := &service{enabled: config.Enabled, token: strings.TrimSpace(config.Token), workspaces: map[string]bool{}, client: client, now: time.Now}
	if len(workflows) > 0 {
		s.workflows = workflows[0]
	}
	for _, workspace := range config.Workspaces {
		s.workspaces[workspace] = true
	}
	if s.enabled {
		s.runnerURL, s.configErr = parseRunnerURL(config.RunnerURL)
		if s.configErr == "" && len(s.token) < 16 {
			s.configErr = tokenEnv + " must contain a separate local-only token with at least 16 characters"
		}
		if s.configErr == "" && (len(s.workspaces) == 0 || len(s.workspaces) > maxWorkspaces) {
			s.configErr = workspacesEnv + " requires one to eight reviewed snapshot names"
		}
		if s.configErr == "" && !validWorkspaceConfig(config.Workspaces) {
			s.configErr = workspacesEnv + " contains an invalid or duplicate snapshot name"
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
		Enabled: s.enabled, Configured: s.configured(), Provider: "Gitleaks local aggregate secret scanner", Workspaces: workspaces, ConfigError: s.configErr,
		Capabilities: []string{"operator-triggered aggregate secret scan of an explicitly allowlisted local snapshot", "redacted finding count, affected-file count, rule count, and result digest"},
		Restrictions: []string{"no caller-selected path, source content, matched text, secret, line, commit, author, raw report, cloud upload, external network, write operation, or scheduled scan", "the runner reads one read-only snapshot and deletes its redacted temporary report before responding", "a scan result cannot expose a secret, modify a file, approve an action, verify task completion, or automatically block a workflow"},
		Scope:        "Owner-triggered local repository safety evidence only. HAI returns aggregate redacted metadata from a configured snapshot; findings require human review in the original workspace and do not become source facts or execution decisions.",
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
	request.Header.Set("User-Agent", "HAI-Gitleaks/1.0")
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
	return &ProbeResult{Reachable: true, Engine: body.Engine, CheckedAt: s.now().UTC(), Scope: "Runner reachability only. The probe does not scan a workspace or read any source content."}, nil
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
	request.Header.Set("User-Agent", "HAI-Gitleaks/1.0")
	request.Header.Set("X-HAI-Gitleaks-Token", s.token)
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

// ScanWithWorkflow preserves the normal read-only scan and optionally records
// a bounded, owner-scoped review signal. A linkage failure never hides the
// completed aggregate scan result and never exposes workflow internals.
func (s *service) ScanWithWorkflow(ctx context.Context, ownerIdentity, workspaceID, workflowID string) (*ScanResult, error) {
	result, err := s.Scan(ctx, workspaceID)
	if err != nil || strings.TrimSpace(workflowID) == "" {
		return result, err
	}
	result.WorkflowID = strings.TrimSpace(workflowID)
	if s.workflows == nil {
		result.WorkflowLinkStatus = "not_linked"
		result.WorkflowLinkError = "workflow linkage is unavailable"
		return result, nil
	}
	if err := s.workflows.AttachSecretScan(ownerIdentity, result.WorkflowID, result.WorkspaceID, result.ResultDigest, result.FindingCount, result.AffectedFiles); err != nil {
		result.WorkflowLinkStatus = "link_failed"
		result.WorkflowLinkError = "secret scan completed but could not be linked to the requested workflow"
		return result, nil
	}
	result.WorkflowLinkStatus = "linked_security_signal"
	return result, nil
}

func (s *service) configured() bool { return s.enabled && s.configErr == "" && s.runnerURL != nil }

func (s *service) endpoint(path string) *url.URL {
	endpoint := *s.runnerURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + path
	return &endpoint
}

func parseWorkspaces(raw string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if !workspacePattern.MatchString(value) || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
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
	if host != "localhost" && host != "host.docker.internal" && host != "gitleaks-runner" && net.ParseIP(host) == nil {
		return nil, runnerURLEnv + " must resolve to localhost, host.docker.internal, gitleaks-runner, or a literal local IP"
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() && !ip.IsPrivate() {
		return nil, runnerURLEnv + " must use a loopback or private-network IP"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed, ""
}

func validEngine(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), "gitleaks ") && len(value) <= 160
}

func validResult(result ScanResult, workspaceID string) bool {
	if result.Status != "completed" || !validEngine(result.Engine) || result.WorkspaceID != workspaceID || result.FindingCount < 0 || result.FindingCount > 100000 || result.AffectedFiles < 0 || result.AffectedFiles > result.FindingCount || len(result.Rules) > 100 || result.DurationMS < 0 || result.DurationMS > int64((5*time.Minute)/time.Millisecond) {
		return false
	}
	count := 0
	for _, rule := range result.Rules {
		if !rulePattern.MatchString(rule.ID) || rule.Count <= 0 || rule.Count > result.FindingCount {
			return false
		}
		count += rule.Count
	}
	if count != result.FindingCount {
		return false
	}
	_, err := hex.DecodeString(result.ResultDigest)
	return len(result.ResultDigest) == sha256.Size*2 && err == nil
}

func Digest(metadata any) string {
	payload, _ := json.Marshal(metadata)
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}
