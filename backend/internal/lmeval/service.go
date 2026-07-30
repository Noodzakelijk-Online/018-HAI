// Package lmeval bridges HAI to one fixed local LM Evaluation Harness suite.
// It produces bounded, metadata-only model-evaluation evidence. A result can
// inform an operator review but can never select a provider, change routing,
// mark work complete, or authorize execution.
package lmeval

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
	"strings"
	"time"
)

const (
	enabledEnv             = "HAI_LM_EVAL_ENABLED"
	baseURLEnv             = "HAI_LM_EVAL_BASE_URL"
	timeoutEnv             = "HAI_LM_EVAL_TIMEOUT_SECONDS"
	suiteName              = "hai_synthetic_v1"
	maxResponseBytes int64 = 16 << 10
)

var ErrNotConfigured = errors.New("local LM Evaluation Harness runner is not configured")

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

type Result struct {
	Status       string  `json:"status"`
	Engine       string  `json:"engine"`
	Suite        string  `json:"suite"`
	ModelID      string  `json:"modelId"`
	CaseCount    int     `json:"caseCount"`
	ExactMatch   float64 `json:"exactMatch"`
	DurationMS   int64   `json:"durationMs"`
	ResultDigest string  `json:"resultDigest"`
	Scope        string  `json:"scope"`
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
	Run(context.Context) (*Result, error)
}

type service struct {
	enabled   bool
	baseURL   *url.URL
	configErr string
	client    *http.Client
	now       func() time.Time
}

func DefaultService() Service {
	timeout := 130 * time.Second
	if raw := strings.TrimSpace(os.Getenv(timeoutEnv)); raw != "" {
		if seconds, err := time.ParseDuration(raw + "s"); err == nil && seconds >= 10*time.Second && seconds <= 300*time.Second {
			timeout = seconds
		}
	}
	return NewService(strings.EqualFold(strings.TrimSpace(os.Getenv(enabledEnv)), "true"), os.Getenv(baseURLEnv), timeout, nil)
}

func NewService(enabled bool, rawBaseURL string, timeout time.Duration, client *http.Client) Service {
	if timeout < 10*time.Second || timeout > 300*time.Second {
		timeout = 130 * time.Second
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
		Provider:    "LM Evaluation Harness local runner",
		ConfigError: s.configErr,
		Capabilities: []string{
			"repeatable six-case synthetic local model comparison",
			"local OpenAI-compatible endpoint evaluation with aggregate exact-match evidence",
		},
		Restrictions: []string{
			"one runner-configured local model and fixed hai_synthetic_v1 suite only",
			"no production prompts, source records, credentials, samples, raw generations, persistence, telemetry export, or public benchmark download",
			"a score cannot select a model, change routing or policy, verify completion, approve, or execute an action",
		},
		Scope: "Operator-configured local evaluation evidence only. HAI returns aggregate metadata from a fixed synthetic suite; it never treats the score as a routing, safety, or completion decision.",
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
		return nil, fmt.Errorf("could not create local LM Evaluation Harness health request")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "HAI-LM-Eval/1.0")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("local LM Evaluation Harness runner is unavailable")
	}
	defer response.Body.Close()
	var body struct {
		Status string `json:"status"`
		Engine string `json:"engine"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 4097)).Decode(&body) != nil || body.Status != "ok" || !validEngine(body.Engine) {
		return nil, fmt.Errorf("local LM Evaluation Harness runner did not pass health probe")
	}
	return &ProbeResult{Reachable: true, Engine: body.Engine, CheckedAt: s.now().UTC(), Scope: "Endpoint reachability only. It does not run a model evaluation or alter model routing."}, nil
}

func (s *service) Run(ctx context.Context) (*Result, error) {
	if !s.configured() {
		return nil, ErrNotConfigured
	}
	endpoint := s.endpoint("/v1/run")
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader([]byte(`{}`)))
	if err != nil {
		return nil, fmt.Errorf("could not create local LM Evaluation Harness request")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "HAI-LM-Eval/1.0")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("local LM Evaluation Harness runner is unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("local LM Evaluation Harness runner returned an unsuccessful response")
	}
	var result Result
	if json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes)).Decode(&result) != nil || !validResult(result) {
		return nil, fmt.Errorf("local LM Evaluation Harness runner returned invalid evaluation metadata")
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

func validResult(result Result) bool {
	return result.Status == "completed" && validEngine(result.Engine) && result.Suite == suiteName && validModelID(result.ModelID) && result.CaseCount == 6 && result.ExactMatch >= 0 && result.ExactMatch <= 1 && result.DurationMS >= 0 && result.DurationMS <= 300000 && len(result.ResultDigest) == 64
}

func validEngine(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "lm-eval ") && len(value) <= 160
}

func validModelID(value string) bool {
	if len(value) == 0 || len(value) > 96 {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || strings.ContainsRune("._:/-", character)) {
			return false
		}
	}
	return true
}

func parseLocalBaseURL(raw string) (*url.URL, string) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, baseURLEnv + " must be a plain local HTTP(S) URL without credentials, query data, or fragments"
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "localhost" && host != "host.docker.internal" && host != "lm-eval-runner" && net.ParseIP(host) == nil {
		return nil, baseURLEnv + " must resolve to localhost, host.docker.internal, lm-eval-runner, or a literal local IP"
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() && !ip.IsPrivate() {
		return nil, baseURLEnv + " must use a loopback or private-network IP"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed, ""
}
