// Package guardrails bridges HAI to one fixed-schema, internal Guardrails AI
// validator. It produces review metadata only and cannot invoke an LLM, alter
// policy, persist proposals, or authorize an external action.
package guardrails

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

	"automation-hub-backend/internal/privacyfilter"
)

const (
	enabledEnv             = "HAI_GUARDRAILS_ENABLED"
	baseURLEnv             = "HAI_GUARDRAILS_BASE_URL"
	timeoutEnv             = "HAI_GUARDRAILS_TIMEOUT_SECONDS"
	maxProposalChars       = 4096
	maxResponseBytes int64 = 16 << 10
	schemaName             = "action_proposal"
)

var (
	ErrNotConfigured  = errors.New("local Guardrails AI runner is not configured")
	ErrInvalidRequest = errors.New("invalid Guardrails AI validation request")
	ErrUnsafeProposal = errors.New("proposal contains detected personal data or secrets")
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

type Request struct {
	Schema   string `json:"schema"`
	Proposal string `json:"proposal"`
}

type Response struct {
	Status         string `json:"status"`
	Engine         string `json:"engine"`
	Schema         string `json:"schema"`
	Valid          bool   `json:"valid"`
	ViolationCount int    `json:"violationCount"`
	ProposalDigest string `json:"proposalDigest"`
	Scope          string `json:"scope"`
}

type ProbeResult struct {
	Reachable bool      `json:"reachable"`
	Engine    string    `json:"engine,omitempty"`
	CheckedAt time.Time `json:"checkedAt"`
	Scope     string    `json:"scope"`
}

type Service interface {
	Status() Status
	Probe(context.Context) (*ProbeResult, error)
	Validate(context.Context, Request) (*Response, error)
}

type service struct {
	enabled   bool
	baseURL   *url.URL
	configErr string
	client    *http.Client
	now       func() time.Time
}

func DefaultService() Service {
	timeout := 8 * time.Second
	if raw := strings.TrimSpace(os.Getenv(timeoutEnv)); raw != "" {
		if seconds, err := time.ParseDuration(raw + "s"); err == nil && seconds > 0 && seconds <= 30*time.Second {
			timeout = seconds
		}
	}
	return NewService(strings.EqualFold(strings.TrimSpace(os.Getenv(enabledEnv)), "true"), os.Getenv(baseURLEnv), timeout, nil)
}

func NewService(enabled bool, rawBaseURL string, timeout time.Duration, client *http.Client) Service {
	if timeout <= 0 || timeout > 30*time.Second {
		timeout = 8 * time.Second
	}
	if client == nil {
		client = &http.Client{
			Timeout:       timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
			Transport:     &http.Transport{Proxy: nil},
		}
	}
	s := &service{enabled: enabled, client: client, now: time.Now}
	if enabled {
		s.baseURL, s.configErr = parseLocalBaseURL(rawBaseURL)
	}
	return s
}

func (s *service) Status() Status {
	status := Status{
		Enabled:     s.enabled,
		Configured:  s.configured(),
		Provider:    "Guardrails AI local runner",
		ConfigError: s.configErr,
		Capabilities: []string{
			"offline validation of one fixed action-proposal schema",
			"local health probe and non-reversible validation metadata for review",
		},
		Restrictions: []string{
			"no model invocation, Hub validator download, retry, policy change, routing change, or automatic completion",
			"no production source records, credentials, prompts, raw proposal export, or proposal persistence",
			"only bounded action_proposal JSON with no detected personal data or secrets",
		},
		Scope: "Operator-configured internal Guardrails AI validator. It can mark a fixed structured proposal valid or needs review; it never approves, executes, or stores the proposal.",
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
	endpoint := s.endpoint("/healthz")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("could not create local Guardrails AI health request")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "HAI-Guardrails-Validation/1.0")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("local Guardrails AI runner is unavailable")
	}
	defer response.Body.Close()
	var body struct {
		Status string `json:"status"`
		Engine string `json:"engine"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 4097)).Decode(&body) != nil || body.Status != "ok" || !validEngine(body.Engine) {
		return nil, fmt.Errorf("local Guardrails AI runner did not pass health probe")
	}
	return &ProbeResult{Reachable: true, Engine: body.Engine, CheckedAt: s.now().UTC(), Scope: "Endpoint reachability only. It does not validate a proposal or authorize a downstream HAI action."}, nil
}

func (s *service) Validate(ctx context.Context, input Request) (*Response, error) {
	if !s.configured() {
		return nil, ErrNotConfigured
	}
	if err := validateRequest(input); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("could not encode local Guardrails AI request")
	}
	endpoint := s.endpoint("/v1/validate")
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("could not create local Guardrails AI request")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "HAI-Guardrails-Validation/1.0")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("local Guardrails AI runner is unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("local Guardrails AI runner returned an unsuccessful response")
	}
	var result Response
	if json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes)).Decode(&result) != nil || !validResponse(result, input) {
		return nil, fmt.Errorf("local Guardrails AI runner returned invalid validation metadata")
	}
	result.Scope = s.Status().Scope
	return &result, nil
}

func (s *service) configured() bool { return s.enabled && s.configErr == "" && s.baseURL != nil }

func (s *service) endpoint(path string) url.URL {
	endpoint := *s.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + path
	return endpoint
}

func validateRequest(input Request) error {
	if input.Schema != schemaName || strings.TrimSpace(input.Proposal) == "" || utf8.RuneCountInString(input.Proposal) > maxProposalChars {
		return ErrInvalidRequest
	}
	if len(privacyfilter.Scan(input.Proposal, 0).SensitiveFields) > 0 {
		return ErrUnsafeProposal
	}
	return nil
}

func validResponse(result Response, input Request) bool {
	if (result.Status != "valid" && result.Status != "needs_review") || result.Valid != (result.Status == "valid") || !validEngine(result.Engine) || result.Schema != input.Schema || result.ViolationCount < 0 || result.ViolationCount > 20 || len(result.ProposalDigest) != 64 {
		return false
	}
	digest := sha256.Sum256([]byte(input.Proposal))
	return result.ProposalDigest == hex.EncodeToString(digest[:])
}

func validEngine(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "guardrails-ai ") && len(value) <= 160
}

func parseLocalBaseURL(raw string) (*url.URL, string) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, baseURLEnv + " must be a plain local HTTP(S) URL without credentials, query data, or fragments"
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "localhost" && host != "host.docker.internal" && host != "guardrails-runner" && net.ParseIP(host) == nil {
		return nil, baseURLEnv + " must resolve to localhost, host.docker.internal, guardrails-runner, or a literal local IP"
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() && !ip.IsPrivate() {
		return nil, baseURLEnv + " must use a loopback or private-network IP"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed, ""
}
