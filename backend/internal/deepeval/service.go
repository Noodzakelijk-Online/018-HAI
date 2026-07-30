// Package deepeval bridges HAI to a fixed, local DeepEval source-grounding
// regression. It returns aggregate synthetic evidence only and never forwards
// caller answers, sources, prompts, models, or runtime controls.
package deepeval

import (
	"automation-hub-backend/internal/runnermaintenance"
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
	"strings"
	"time"
)

const (
	enabledEnv             = "HAI_DEEPEVAL_ENABLED"
	baseURLEnv             = "HAI_DEEPEVAL_BASE_URL"
	timeoutEnv             = "HAI_DEEPEVAL_TIMEOUT_SECONDS"
	suiteName              = "hai_source_grounding_regression_v1"
	metricName             = "FaithfulnessMetric"
	maxResponseBytes int64 = 16 << 10
)

var ErrNotConfigured = errors.New("local DeepEval runner is not configured")

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
	Metric       string  `json:"metric"`
	ModelID      string  `json:"modelId"`
	CaseCount    int     `json:"caseCount"`
	PassedCount  int     `json:"passedCount"`
	FailedCount  int     `json:"failedCount"`
	Score        float64 `json:"score"`
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
	maintenanceGate runnermaintenance.Gate
}

func DefaultService() Service {
	timeout := 180 * time.Second
	if raw := strings.TrimSpace(os.Getenv(timeoutEnv)); raw != "" {
		if seconds, err := time.ParseDuration(raw + "s"); err == nil && seconds >= 10*time.Second && seconds <= 300*time.Second {
			timeout = seconds
		}
	}
	return NewService(strings.EqualFold(strings.TrimSpace(os.Getenv(enabledEnv)), "true"), os.Getenv(baseURLEnv), timeout, nil)
}

func NewService(enabled bool, rawBaseURL string, timeout time.Duration, client *http.Client) Service {
	if timeout < 10*time.Second || timeout > 300*time.Second {
		timeout = 180 * time.Second
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

// WithModelMaintenance binds the fixed evaluator to HAI's canonical local
// model policy before it may issue any synthetic evaluation request.
func WithModelMaintenance(delegate Service, gate runnermaintenance.Gate) Service {
	if configured, ok := delegate.(*service); ok {
		configured.maintenanceGate = gate
	}
	return delegate
}

func (s *service) Status() Status {
	status := Status{
		Enabled: s.enabled, Configured: s.configured(), Provider: "DeepEval local grounding evaluator", ConfigError: s.configErr,
		Capabilities: []string{"repeatable local source-grounding regression with a fixed DeepEval faithfulness metric", "aggregate evaluator accuracy evidence across fixed synthetic faithful and unsupported answers"},
		Restrictions: []string{"one runner-configured local OpenAI-compatible judge and fixed three-case hai_source_grounding_regression_v1 suite only", "no HAI answer, source, account, secret, workflow, runtime, raw fixture, model generation, metric reason, case row, persistence, or telemetry export", "a result cannot select a model, change routing or policy, verify completion, approve, or execute an action"},
		Scope:        "Operator-configured local synthetic source-grounding regression evidence only. HAI returns aggregate metadata from a fixed DeepEval suite; it is not an evaluation of real HAI outputs or a production truth guarantee.",
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
		return nil, fmt.Errorf("could not create local DeepEval health request")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "HAI-DeepEval/1.0")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("local DeepEval runner is unavailable")
	}
	defer response.Body.Close()
	var body struct {
		Status     string `json:"status"`
		Engine     string `json:"engine"`
		Configured bool   `json:"configured"`
		Suite      string `json:"suite"`
		Metric     string `json:"metric"`
		ModelID    string `json:"modelId"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 4097)).Decode(&body) != nil || body.Status != "ok" || !body.Configured || body.Suite != suiteName || body.Metric != metricName || !validEngine(body.Engine) || !validModelID(body.ModelID) {
		return nil, fmt.Errorf("local DeepEval runner did not pass health probe")
	}
	return &ProbeResult{Reachable: true, Engine: body.Engine, CheckedAt: s.now().UTC(), Scope: "Endpoint reachability only. It does not run an evaluation or alter HAI behavior."}, nil
}

func (s *service) Run(ctx context.Context) (*Result, error) {
	if !s.configured() {
		return nil, ErrNotConfigured
	}
	if err := runnermaintenance.EnsureConfiguredLocalModel(ctx, s.client, s.baseURL, "HAI-DeepEval/1.0", "DeepEval", s.maintenanceGate); err != nil {
		return nil, err
	}
	endpoint := s.endpoint("/v1/run")
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader([]byte(`{}`)))
	if err != nil {
		return nil, fmt.Errorf("could not create local DeepEval request")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "HAI-DeepEval/1.0")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("local DeepEval runner is unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("local DeepEval runner returned an unsuccessful response")
	}
	var result Result
	if json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes)).Decode(&result) != nil || !validResult(result) {
		return nil, fmt.Errorf("local DeepEval runner returned invalid evaluation metadata")
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
	return result.Status == "completed" && validEngine(result.Engine) && result.Suite == suiteName && result.Metric == metricName && validModelID(result.ModelID) && result.CaseCount == 3 && result.PassedCount >= 0 && result.FailedCount >= 0 && result.PassedCount+result.FailedCount == result.CaseCount && result.Score >= 0 && result.Score <= 1 && result.DurationMS >= 0 && result.DurationMS <= 300000 && validDigest(result.ResultDigest)
}

func validDigest(value string) bool {
	_, err := hex.DecodeString(value)
	return len(value) == 64 && err == nil
}

func validEngine(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "deepeval ") && len(value) <= 160
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
	if host != "localhost" && host != "host.docker.internal" && host != "deepeval-runner" && net.ParseIP(host) == nil {
		return nil, baseURLEnv + " must resolve to localhost, host.docker.internal, deepeval-runner, or a literal local IP"
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() && !ip.IsPrivate() {
		return nil, baseURLEnv + " must use a loopback or private-network IP"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed, ""
}
