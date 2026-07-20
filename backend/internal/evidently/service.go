// Package evidently bridges HAI to a narrow, internal-only Evidently report
// runner. It accepts bounded synthetic or already-redacted fixtures only; it
// never receives production source records or changes HAI policy or state.
package evidently

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"automation-hub-backend/internal/privacyfilter"
)

const (
	enabledEnv             = "HAI_EVIDENTLY_ENABLED"
	baseURLEnv             = "HAI_EVIDENTLY_BASE_URL"
	timeoutEnv             = "HAI_EVIDENTLY_TIMEOUT_SECONDS"
	maxCases               = 25
	maxInputLength         = 512
	maxOutputLength        = 2048
	maxResponseBytes int64 = 64 << 10
)

var (
	ErrNotConfigured  = errors.New("local Evidently runner is not configured")
	ErrInvalidRequest = errors.New("invalid Evidently evaluation request")
	ErrUnsafeFixture  = errors.New("evaluation fixtures must be synthetic or already redacted")
	idPattern         = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,95}$`)
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

type Case struct {
	ID     string `json:"id"`
	Input  string `json:"input"`
	Output string `json:"output"`
}

type Request struct {
	FixtureKind string `json:"fixtureKind"`
	Cases       []Case `json:"cases"`
}

type Response struct {
	Status             string  `json:"status"`
	Engine             string  `json:"engine"`
	FixtureKind        string  `json:"fixtureKind"`
	CaseCount          int     `json:"caseCount"`
	EmptyOutputs       int     `json:"emptyOutputs"`
	DuplicateOutputs   int     `json:"duplicateOutputs"`
	AverageOutputChars float64 `json:"averageOutputChars"`
	ReportDigest       string  `json:"reportDigest"`
	Scope              string  `json:"scope"`
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
	Evaluate(context.Context, Request) (*Response, error)
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
	return NewService(
		strings.EqualFold(strings.TrimSpace(os.Getenv(enabledEnv)), "true"),
		os.Getenv(baseURLEnv),
		timeout,
		nil,
	)
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
		Configured:  s.enabled && s.configErr == "",
		Provider:    "Evidently local runner",
		ConfigError: s.configErr,
		Capabilities: []string{
			"offline Evidently report over bounded synthetic or redacted fixtures",
			"local health probe and report digest for review evidence",
		},
		Restrictions: []string{
			"no production source records, prompts, credentials, or raw report export",
			"no provider calls, cloud export, routing changes, policy changes, or automatic completion",
			"no fixture persistence in the runner or HAI bridge",
		},
		Scope: "Operator-configured internal Evidently report service. It produces review evidence only; a passing report does not verify an answer, enable a provider, or authorize an action.",
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
		return nil, fmt.Errorf("could not create local Evidently health request")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "HAI-Evidently-Evaluation/1.0")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("local Evidently runner is unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("local Evidently runner did not pass health probe")
	}
	var body struct {
		Status string `json:"status"`
		Engine string `json:"engine"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4097)).Decode(&body); err != nil || body.Status != "ok" || !validEngine(body.Engine) {
		return nil, fmt.Errorf("local Evidently runner returned invalid health metadata")
	}
	return &ProbeResult{Reachable: true, Engine: body.Engine, CheckedAt: s.now().UTC(), Scope: "Endpoint reachability only. It does not validate a fixture, report result, or downstream HAI verification."}, nil
}

func (s *service) Evaluate(ctx context.Context, input Request) (*Response, error) {
	if !s.configured() {
		return nil, ErrNotConfigured
	}
	if err := validateRequest(input); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("could not encode local evaluation request")
	}
	endpoint := s.endpoint("/v1/evaluate")
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("could not create local evaluation request")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "HAI-Evidently-Evaluation/1.0")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("local Evidently runner is unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("local Evidently runner returned an unsuccessful response")
	}
	var result Response
	if err := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes)).Decode(&result); err != nil || !validResponse(result, input) {
		return nil, fmt.Errorf("local Evidently runner returned invalid report metadata")
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
	if input.FixtureKind != "synthetic" && input.FixtureKind != "redacted" || len(input.Cases) == 0 || len(input.Cases) > maxCases {
		return ErrInvalidRequest
	}
	seen := map[string]bool{}
	for _, item := range input.Cases {
		if !idPattern.MatchString(item.ID) || seen[item.ID] || utf8.RuneCountInString(item.Input) > maxInputLength || utf8.RuneCountInString(item.Output) > maxOutputLength {
			return ErrInvalidRequest
		}
		seen[item.ID] = true
		if fixtureContainsSensitiveData(item.Input) || fixtureContainsSensitiveData(item.Output) {
			return ErrUnsafeFixture
		}
	}
	return nil
}

func fixtureContainsSensitiveData(value string) bool {
	return len(privacyfilter.Scan(value, 0).SensitiveFields) > 0
}

func validResponse(result Response, input Request) bool {
	if result.Status != "passed" && result.Status != "needs_review" || !validEngine(result.Engine) || result.FixtureKind != input.FixtureKind || result.CaseCount != len(input.Cases) || result.EmptyOutputs < 0 || result.EmptyOutputs > len(input.Cases) || result.DuplicateOutputs < 0 || result.DuplicateOutputs >= len(input.Cases) || result.AverageOutputChars < 0 || result.AverageOutputChars > maxOutputLength || len(result.ReportDigest) != 64 {
		return false
	}
	_, err := hex.DecodeString(result.ReportDigest)
	return err == nil
}

func validEngine(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "evidently ") && len(value) <= 160
}

func parseLocalBaseURL(raw string) (*url.URL, string) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, baseURLEnv + " must be a plain local HTTP(S) URL without credentials, query data, or fragments"
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "localhost" && host != "host.docker.internal" && host != "evidently-runner" && net.ParseIP(host) == nil {
		return nil, baseURLEnv + " must resolve to localhost, host.docker.internal, evidently-runner, or a literal local IP"
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() && !ip.IsPrivate() {
		return nil, baseURLEnv + " must use a loopback or private-network IP"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed, ""
}
