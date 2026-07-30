// Package agentframework exposes a narrow Microsoft Agent Framework planning
// bridge. It never gives the external framework HAI tools, sources, memory,
// approvals, workflow state, or execution authority.
package agentframework

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	enabledEnv              = "HAI_AGENT_FRAMEWORK_ENABLED"
	baseURLEnv              = "HAI_AGENT_FRAMEWORK_BASE_URL"
	timeoutEnv              = "HAI_AGENT_FRAMEWORK_TIMEOUT_SECONDS"
	maxRequestChars         = 4000
	maxCriteria             = 8
	maxCriterionChars       = 240
	maxResponseBytes  int64 = 64 << 10
)

var (
	ErrNotConfigured  = errors.New("local Agent Framework planning runner is not configured")
	ErrInvalidRequest = errors.New("invalid local Agent Framework planning request")
)

type Status struct {
	Enabled      bool     `json:"enabled"`
	Configured   bool     `json:"configured"`
	Provider     string   `json:"provider"`
	Endpoint     string   `json:"endpoint,omitempty"`
	ConfigError  string   `json:"configError,omitempty"`
	Capabilities []string `json:"capabilities"`
	Restrictions []string `json:"restrictions"`
	Scope        string   `json:"scope"`
}

// Request contains only one short owner-provided problem and measurable
// criteria. It must not be used to pass source records, credentials, or tools.
type Request struct {
	Request         string   `json:"request"`
	SuccessCriteria []string `json:"successCriteria,omitempty"`
}

type Proposal struct {
	Goal             string   `json:"goal"`
	SuccessCriteria  []string `json:"successCriteria"`
	NextSteps        []string `json:"nextSteps"`
	Risk             string   `json:"risk"`
	RequiresApproval bool     `json:"requiresApproval"`
	Reasons          []string `json:"reasons"`
	Uncertainties    []string `json:"uncertainties"`
}

type Response struct {
	Engine        string   `json:"engine"`
	ModelID       string   `json:"modelId"`
	RequestDigest string   `json:"requestDigest"`
	Proposal      Proposal `json:"proposal"`
	Scope         string   `json:"scope"`
}

type ProbeResult struct {
	Reachable     bool      `json:"reachable"`
	Engine        string    `json:"engine,omitempty"`
	ModelID       string    `json:"modelId,omitempty"`
	ModelEndpoint string    `json:"modelEndpoint,omitempty"`
	CheckedAt     time.Time `json:"checkedAt"`
	Scope         string    `json:"scope"`
}

type Service interface {
	Status() Status
	Probe(context.Context) (*ProbeResult, error)
	Propose(context.Context, Request) (*Response, error)
}

// ModelMaintenanceGate is the narrow policy boundary required by this runner.
// The canonical LLM service may satisfy it without this package depending on a
// concrete maintenance implementation.
type ModelMaintenanceGate interface {
	EnsureConfiguredLocalModel(endpointURL, modelID string) error
}

type service struct {
	enabled         bool
	baseURL         *url.URL
	configErr       string
	client          *http.Client
	now             func() time.Time
	maintenanceGate ModelMaintenanceGate
}

func DefaultService() Service {
	timeout := 30 * time.Second
	if raw := strings.TrimSpace(os.Getenv(timeoutEnv)); raw != "" {
		if seconds, err := time.ParseDuration(raw + "s"); err == nil && seconds > 0 && seconds <= 90*time.Second {
			timeout = seconds
		}
	}
	return NewService(strings.EqualFold(strings.TrimSpace(os.Getenv(enabledEnv)), "true"), os.Getenv(baseURLEnv), timeout, nil)
}

func NewService(enabled bool, rawBaseURL string, timeout time.Duration, client *http.Client) Service {
	if timeout <= 0 || timeout > 90*time.Second {
		timeout = 30 * time.Second
	}
	if client == nil {
		client = &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }, Transport: &http.Transport{Proxy: nil}}
	}
	s := &service{enabled: enabled, client: client, now: time.Now}
	if enabled {
		s.baseURL, s.configErr = parseLocalBaseURL(rawBaseURL)
	}
	return s
}

// WithModelMaintenance binds this optional runner to HAI's canonical local
// model policy. A configured runner without this gate cannot receive a task,
// which prevents a side-channel model lifecycle outside daily maintenance.
func WithModelMaintenance(delegate Service, gate ModelMaintenanceGate) Service {
	if configured, ok := delegate.(*service); ok {
		configured.maintenanceGate = gate
	}
	return delegate
}

func (s *service) Status() Status {
	status := Status{
		Enabled: s.enabled, Configured: s.configured(), Provider: "Microsoft Agent Framework local planning runner", ConfigError: s.configErr,
		Capabilities: []string{"one bounded sequential planner/reviewer draft", "fixed local-model reachability probe", "schema-checked proposal artifact"},
		Restrictions: []string{
			"no Agent Framework tools, browser, web search, file access, MCP, skills, memory, sessions, checkpoints, hosted agents, A2A, retries, or delegation",
			"no HAI sources, account data, credentials, policy changes, approval decisions, workflow creation, execution, or completion claim",
			"OpenTelemetry is disabled; HAI keeps routing, validation, audit, approval, emergency-stop, and all side-effect authority",
		},
		Scope: "Operator-configured local Microsoft Agent Framework runner. It produces one review-only sequential planning artifact from a short task request; it cannot run or authorize HAI work.",
	}
	if s.baseURL != nil {
		status.Endpoint = s.baseURL.String()
	}
	return status
}

func (s *service) Probe(ctx context.Context) (*ProbeResult, error) {
	if !s.configured() {
		return nil, ErrNotConfigured
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint("/v1/probe").String(), nil)
	if err != nil {
		return nil, fmt.Errorf("could not create local Agent Framework probe request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "HAI-Agent-Framework-Planning/1.0")
	response, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("local Agent Framework planning runner is unavailable")
	}
	defer response.Body.Close()
	var body struct {
		Status        string `json:"status"`
		Engine        string `json:"engine"`
		Model         string `json:"modelId"`
		ModelEndpoint string `json:"modelEndpoint"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 4097)).Decode(&body) != nil || body.Status != "ok" || !validEngine(body.Engine) || !validBoundedText(body.Model, 160) {
		return nil, fmt.Errorf("local Agent Framework planning runner did not pass its constrained model probe")
	}
	return &ProbeResult{Reachable: true, Engine: body.Engine, ModelID: body.Model, ModelEndpoint: strings.TrimSpace(body.ModelEndpoint), CheckedAt: s.now().UTC(), Scope: "Local runner and fixed local-model reachability only. It does not create an Agent Framework proposal or authorize action."}, nil
}

func (s *service) Propose(ctx context.Context, input Request) (*Response, error) {
	if !s.configured() {
		return nil, ErrNotConfigured
	}
	if err := validateRequest(input); err != nil {
		return nil, err
	}
	if err := s.ensureMaintainedModel(ctx); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("could not encode local Agent Framework planning request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint("/v1/propose").String(), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("could not create local Agent Framework planning request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "HAI-Agent-Framework-Planning/1.0")
	response, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("local Agent Framework planning runner is unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("local Agent Framework planning runner returned an unsuccessful response")
	}
	var result Response
	if json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes)).Decode(&result) != nil || !validResponse(result, input) {
		return nil, fmt.Errorf("local Agent Framework planning runner returned an invalid proposal")
	}
	result.Scope = s.Status().Scope
	return &result, nil
}

func (s *service) configured() bool { return s.enabled && s.configErr == "" && s.baseURL != nil }
func (s *service) endpoint(path string) *url.URL {
	endpoint := *s.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + path
	return &endpoint
}

func (s *service) ensureMaintainedModel(ctx context.Context) error {
	if s.maintenanceGate == nil {
		return fmt.Errorf("central daily model maintenance gate is unavailable")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.endpoint("/healthz").String(), nil)
	if err != nil {
		return fmt.Errorf("could not create local Agent Framework runner status request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "HAI-Agent-Framework-Planning/1.0")
	response, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("local Agent Framework planning runner is unavailable")
	}
	defer response.Body.Close()
	var body struct {
		Status        string `json:"status"`
		Configured    bool   `json:"configured"`
		Model         string `json:"modelId"`
		ModelEndpoint string `json:"modelEndpoint"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 4097)).Decode(&body) != nil || body.Status != "ok" || !body.Configured || !validBoundedText(body.Model, 160) || !validBoundedText(body.ModelEndpoint, 512) {
		return fmt.Errorf("local Agent Framework planning runner did not disclose one fixed local model configuration")
	}
	if err := s.maintenanceGate.EnsureConfiguredLocalModel(body.ModelEndpoint, body.Model); err != nil {
		return fmt.Errorf("local Agent Framework planning model is not admitted: %w", err)
	}
	return nil
}

func parseLocalBaseURL(raw string) (*url.URL, string) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, baseURLEnv + " must be a plain local HTTP(S) URL without credentials, query data, or fragments"
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "localhost" && host != "host.docker.internal" && host != "agent-framework-runner" && net.ParseIP(host) == nil {
		return nil, baseURLEnv + " must resolve to localhost, host.docker.internal, agent-framework-runner, or a literal local IP"
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() && !ip.IsPrivate() {
		return nil, baseURLEnv + " must use a loopback or private-network IP"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed, ""
}

func validateRequest(input Request) error {
	if !validBoundedText(input.Request, maxRequestChars) || strings.ContainsAny(input.Request, "\r\n") || len(input.SuccessCriteria) > maxCriteria {
		return ErrInvalidRequest
	}
	for _, criterion := range input.SuccessCriteria {
		if !validBoundedText(criterion, maxCriterionChars) || strings.ContainsAny(criterion, "\r\n") {
			return ErrInvalidRequest
		}
	}
	return nil
}

func validResponse(result Response, input Request) bool {
	proposal := result.Proposal
	return validEngine(result.Engine) && validBoundedText(result.ModelID, 160) && result.RequestDigest == requestDigest(input) && validBoundedText(proposal.Goal, 400) && validRisk(proposal.Risk) && validStringList(proposal.SuccessCriteria, 1, 8, 240) && validStringList(proposal.NextSteps, 1, 8, 320) && validStringList(proposal.Reasons, 1, 5, 320) && validStringList(proposal.Uncertainties, 0, 5, 320)
}

func validRisk(value string) bool { return value == "low" || value == "medium" || value == "high" }
func validStringList(values []string, min, max, maxChars int) bool {
	if len(values) < min || len(values) > max {
		return false
	}
	for _, value := range values {
		if !validBoundedText(value, maxChars) || strings.ContainsAny(value, "\r\n") {
			return false
		}
	}
	return true
}
func validBoundedText(value string, limit int) bool {
	value = strings.TrimSpace(value)
	return value != "" && utf8.RuneCountInString(value) <= limit
}
func validEngine(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "microsoft-agent-framework ") && len(value) <= 160
}

func requestDigest(input Request) string {
	criteria := input.SuccessCriteria
	if criteria == nil {
		criteria = []string{}
	}
	encoded, _ := json.Marshal(struct {
		Request         string   `json:"request"`
		SuccessCriteria []string `json:"successCriteria"`
	}{Request: input.Request, SuccessCriteria: criteria})
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}
