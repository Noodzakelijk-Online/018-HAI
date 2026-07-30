// Package syft exposes a deliberately narrow local SBOM-inventory boundary.
// It inventories only named, operator-allowlisted disposable snapshots and
// returns aggregate metadata. It never returns an SBOM, package name, version,
// license, PURL, file path, source file, or repository content.
package syft

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
	enabledEnv       = "HAI_SYFT_ENABLED"
	runnerURLEnv     = "HAI_SYFT_RUNNER_URL"
	tokenEnv         = "HAI_SYFT_RUNNER_TOKEN"
	workspacesEnv    = "HAI_SYFT_WORKSPACES"
	timeoutEnv       = "HAI_SYFT_TIMEOUT_SECONDS"
	maxResponseBytes = 32 << 10
	maxWorkspaces    = 8
	defaultTimeout   = 120 * time.Second
)

var (
	ErrNotConfigured = errors.New("local Syft runner is not configured")
	ErrUnavailable   = errors.New("local Syft runner is unavailable")
	ErrWorkspace     = errors.New("workspace is not approved for SBOM inventory")
	workspacePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
	ecosystemPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
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

type EcosystemCount struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

type InventoryResult struct {
	Status             string           `json:"status"`
	Engine             string           `json:"engine"`
	WorkspaceID        string           `json:"workspaceId"`
	PackageCount       int              `json:"packageCount"`
	Ecosystems         []EcosystemCount `json:"ecosystems"`
	DurationMS         int64            `json:"durationMs"`
	ResultDigest       string           `json:"resultDigest"`
	Scope              string           `json:"scope"`
	WorkflowID         string           `json:"workflowId,omitempty"`
	WorkflowLinkStatus string           `json:"workflowLinkStatus,omitempty"`
	WorkflowLinkError  string           `json:"workflowLinkError,omitempty"`
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
	Inventory(context.Context, string) (*InventoryResult, error)
}

// WorkflowLinker adds only redacted aggregate inventory metadata to an
// existing workflow. It cannot move workflow state or accept dependencies.
type WorkflowLinker interface {
	AttachSBOMInventory(ownerIdentity, workflowID, workspaceID, resultDigest string, packageCount, ecosystemCount int) error
}

// WorkflowInventoryService is optional so read-only inventory callers and test
// doubles retain the narrow Service contract.
type WorkflowInventoryService interface {
	InventoryWithWorkflow(context.Context, string, string, string) (*InventoryResult, error)
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
	return Config{Enabled: strings.EqualFold(strings.TrimSpace(os.Getenv(enabledEnv)), "true"), RunnerURL: os.Getenv(runnerURLEnv), Token: os.Getenv(tokenEnv), Workspaces: parseWorkspaces(os.Getenv(workspacesEnv)), Timeout: timeout}
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
		Enabled: s.enabled, Configured: s.configured(), Provider: "Syft local aggregate SBOM inventory", Workspaces: workspaces, ConfigError: s.configErr,
		Capabilities: []string{"operator-triggered aggregate software inventory of an explicitly allowlisted local snapshot", "redacted package total, ecosystem counts, duration, and result digest"},
		Restrictions: []string{"no caller-selected path, source content, package name, version, license, PURL, file path, SBOM export, cloud upload, external network, write operation, or scheduled inventory", "the runner reads one read-only snapshot and retains no generated SBOM after returning aggregate metadata", "an inventory cannot expose source content, change a dependency, approve an action, verify task completion, or automatically block a workflow"},
		Scope:        "Owner-triggered local software-inventory evidence only. HAI returns aggregate redacted metadata from a configured snapshot; results require human review in the original workspace and do not become source facts or execution decisions.",
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
	request.Header.Set("User-Agent", "HAI-Syft/1.0")
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
	return &ProbeResult{Reachable: true, Engine: body.Engine, CheckedAt: s.now().UTC(), Scope: "Runner reachability only. The probe does not inventory a workspace or read any source content."}, nil
}

func (s *service) Inventory(ctx context.Context, workspaceID string) (*InventoryResult, error) {
	if !s.configured() {
		return nil, ErrNotConfigured
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if !s.workspaces[workspaceID] {
		return nil, ErrWorkspace
	}
	body, _ := json.Marshal(map[string]string{"workspaceId": workspaceID})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint("/v1/inventory").String(), bytes.NewReader(body))
	if err != nil {
		return nil, ErrUnavailable
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "HAI-Syft/1.0")
	request.Header.Set("X-HAI-Syft-Token", s.token)
	response, err := s.client.Do(request)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, ErrUnavailable
	}
	var result InventoryResult
	if json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes)).Decode(&result) != nil || !validResult(result, workspaceID) {
		return nil, ErrUnavailable
	}
	result.Scope = s.Status().Scope
	return &result, nil
}

// InventoryWithWorkflow preserves the original read-only inventory result and
// optionally links a bounded review signal. A workflow-link failure never hides
// the inventory result or exposes workflow internals.
func (s *service) InventoryWithWorkflow(ctx context.Context, ownerIdentity, workspaceID, workflowID string) (*InventoryResult, error) {
	result, err := s.Inventory(ctx, workspaceID)
	if err != nil || strings.TrimSpace(workflowID) == "" {
		return result, err
	}
	result.WorkflowID = strings.TrimSpace(workflowID)
	if s.workflows == nil {
		result.WorkflowLinkStatus = "not_linked"
		result.WorkflowLinkError = "workflow linkage is unavailable"
		return result, nil
	}
	if err := s.workflows.AttachSBOMInventory(ownerIdentity, result.WorkflowID, result.WorkspaceID, result.ResultDigest, result.PackageCount, len(result.Ecosystems)); err != nil {
		result.WorkflowLinkStatus = "link_failed"
		result.WorkflowLinkError = "SBOM inventory completed but could not be linked to the requested workflow"
		return result, nil
	}
	result.WorkflowLinkStatus = "linked_review_signal"
	return result, nil
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
	if host != "localhost" && host != "host.docker.internal" && host != "syft-runner" && net.ParseIP(host) == nil {
		return nil, runnerURLEnv + " must resolve to localhost, host.docker.internal, syft-runner, or a literal local IP"
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() && !ip.IsPrivate() {
		return nil, runnerURLEnv + " must use a loopback or private-network IP"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed, ""
}

func validEngine(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), "syft ") && len(value) <= 160
}

func validResult(result InventoryResult, workspaceID string) bool {
	if result.Status != "completed" || !validEngine(result.Engine) || result.WorkspaceID != workspaceID || result.PackageCount < 0 || result.PackageCount > 100000 || len(result.Ecosystems) > 64 || result.DurationMS < 0 || result.DurationMS > int64((5*time.Minute)/time.Millisecond) {
		return false
	}
	count, seen := 0, map[string]bool{}
	for _, ecosystem := range result.Ecosystems {
		if !ecosystemPattern.MatchString(ecosystem.ID) || ecosystem.Count <= 0 || ecosystem.Count > result.PackageCount || seen[ecosystem.ID] {
			return false
		}
		seen[ecosystem.ID] = true
		count += ecosystem.Count
	}
	if count != result.PackageCount {
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
