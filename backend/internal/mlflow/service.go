// Package mlflow provides a deliberately narrow bridge to an operator-run
// local MLflow tracking server. It reads an allowlisted projection of recent
// evaluation runs; it does not train, register, serve, modify, or delete
// models, experiments, runs, artifacts, prompts, datasets, or traces.
package mlflow

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
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	enabledEnv       = "HAI_MLFLOW_ENABLED"
	baseURLEnv       = "HAI_MLFLOW_BASE_URL"
	bearerTokenEnv   = "HAI_MLFLOW_BEARER_TOKEN"
	experimentIDsEnv = "HAI_MLFLOW_EXPERIMENT_IDS"
	metricKeysEnv    = "HAI_MLFLOW_METRIC_KEYS"
	timeoutEnv       = "HAI_MLFLOW_TIMEOUT_SECONDS"
	maxExperiments   = 16
	maxMetricKeys    = 24
	maxRuns          = 25
	maxResponseBytes = 256 << 10
)

var (
	ErrNotConfigured = errors.New("local MLflow evaluation bridge is not configured")
	ErrUnavailable   = errors.New("local MLflow evaluation evidence is unavailable")
	experimentIDRE   = regexp.MustCompile(`^[0-9]{1,32}$`)
	metricKeyRE      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:/-]{0,119}$`)
)

type Status struct {
	Enabled       bool     `json:"enabled"`
	Configured    bool     `json:"configured"`
	Provider      string   `json:"provider"`
	Endpoint      string   `json:"endpoint,omitempty"`
	ExperimentIDs []string `json:"experimentIds"`
	MetricKeys    []string `json:"metricKeys"`
	ConfigError   string   `json:"configError,omitempty"`
	Capabilities  []string `json:"capabilities"`
	Restrictions  []string `json:"restrictions"`
	Scope         string   `json:"scope"`
}

type ProbeResult struct {
	Reachable bool      `json:"reachable"`
	CheckedAt time.Time `json:"checkedAt"`
	Scope     string    `json:"scope"`
}

type Metric struct {
	Key       string  `json:"key"`
	Value     float64 `json:"value"`
	Timestamp int64   `json:"timestamp,omitempty"`
	Step      int64   `json:"step,omitempty"`
}

type EvaluationRun struct {
	ExperimentID string   `json:"experimentId"`
	RunID        string   `json:"runId"`
	RunName      string   `json:"runName,omitempty"`
	Status       string   `json:"status"`
	StartedAt    int64    `json:"startedAt,omitempty"`
	EndedAt      int64    `json:"endedAt,omitempty"`
	Metrics      []Metric `json:"metrics"`
}

type RunsResponse struct {
	Runs          []EvaluationRun `json:"runs"`
	ExperimentIDs []string        `json:"experimentIds"`
	MetricKeys    []string        `json:"metricKeys"`
	Scope         string          `json:"scope"`
}

type Service interface {
	Status() Status
	Probe(context.Context) (*ProbeResult, error)
	RecentRuns(context.Context, int) (*RunsResponse, error)
}

type service struct {
	enabled       bool
	baseURL       *url.URL
	bearerToken   string
	experimentIDs []string
	metricKeys    []string
	configErr     string
	client        *http.Client
	now           func() time.Time
}

func DefaultService() Service {
	timeout := 8 * time.Second
	if raw := strings.TrimSpace(os.Getenv(timeoutEnv)); raw != "" {
		if seconds, err := time.ParseDuration(raw + "s"); err == nil && seconds >= time.Second && seconds <= 30*time.Second {
			timeout = seconds
		}
	}
	return NewService(
		strings.EqualFold(strings.TrimSpace(os.Getenv(enabledEnv)), "true"),
		os.Getenv(baseURLEnv),
		os.Getenv(bearerTokenEnv),
		os.Getenv(experimentIDsEnv),
		os.Getenv(metricKeysEnv),
		timeout,
		nil,
	)
}

func NewService(enabled bool, rawBaseURL, bearerToken, rawExperimentIDs, rawMetricKeys string, timeout time.Duration, client *http.Client) Service {
	if timeout < time.Second || timeout > 30*time.Second {
		timeout = 8 * time.Second
	}
	if client == nil {
		client = &http.Client{
			Timeout:       timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
			Transport:     &http.Transport{Proxy: nil},
		}
	}
	s := &service{enabled: enabled, bearerToken: strings.TrimSpace(bearerToken), client: client, now: time.Now}
	if enabled {
		s.baseURL, s.configErr = parseLocalBaseURL(rawBaseURL)
		if s.configErr == "" {
			s.experimentIDs, s.configErr = parseExperimentIDs(rawExperimentIDs)
		}
		if s.configErr == "" {
			s.metricKeys, s.configErr = parseMetricKeys(rawMetricKeys)
		}
		if s.configErr == "" && !validToken(s.bearerToken) {
			s.configErr = bearerTokenEnv + " contains invalid characters"
		}
	}
	return s
}

func (s *service) Status() Status {
	status := Status{
		Enabled: s.enabled, Configured: s.configured(), Provider: "MLflow local evaluation evidence",
		ExperimentIDs: append([]string(nil), s.experimentIDs...), MetricKeys: append([]string(nil), s.metricKeys...), ConfigError: s.configErr,
		Capabilities: []string{"fixed-experiment recent evaluation-run projection", "allowlisted metric evidence for manual model-review context", "local endpoint reachability probe"},
		Restrictions: []string{"no prompts, parameters, tags, datasets, artifacts, model versions, trace content, or credential responses", "no experiment, run, metric, model, registry, or deployment mutation", "no automatic model routing, budget, provider, verification, workflow, memory, or execution updates"},
		Scope:        "Operator-configured local MLflow read-only evaluation evidence. Returned metrics remain contextual evidence for a human model review; HAI's router remains governed by its own live probes, policy, budget, and validation rules.",
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
	if _, err := s.searchRuns(ctx, 1); err != nil {
		return nil, ErrUnavailable
	}
	return &ProbeResult{Reachable: true, CheckedAt: s.now().UTC(), Scope: "Authenticated read-only search over the configured experiment allowlist only. It does not retain a run or change MLflow or HAI state."}, nil
}

func (s *service) RecentRuns(ctx context.Context, limit int) (*RunsResponse, error) {
	if !s.configured() {
		return nil, ErrNotConfigured
	}
	if limit <= 0 || limit > maxRuns {
		limit = 10
	}
	runs, err := s.searchRuns(ctx, limit)
	if err != nil {
		return nil, ErrUnavailable
	}
	return &RunsResponse{Runs: runs, ExperimentIDs: append([]string(nil), s.experimentIDs...), MetricKeys: append([]string(nil), s.metricKeys...), Scope: s.Status().Scope}, nil
}

func (s *service) searchRuns(ctx context.Context, limit int) ([]EvaluationRun, error) {
	payload, err := json.Marshal(map[string]any{
		"experiment_ids": s.experimentIDs,
		"run_view_type":  "ACTIVE_ONLY",
		"max_results":    limit,
		"order_by":       []string{"attributes.start_time DESC"},
	})
	if err != nil {
		return nil, err
	}
	endpoint := s.endpoint("/api/2.0/mlflow/runs/search")
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "HAI-MLflow-Evaluation/1.0")
	if s.bearerToken != "" {
		request.Header.Set("Authorization", "Bearer "+s.bearerToken)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MLflow returned HTTP %d", response.StatusCode)
	}
	var body struct {
		Runs []struct {
			Info struct {
				RunID        string `json:"run_id"`
				ExperimentID string `json:"experiment_id"`
				RunName      string `json:"run_name"`
				Status       string `json:"status"`
				StartTime    int64  `json:"start_time"`
				EndTime      int64  `json:"end_time"`
			} `json:"info"`
			Data struct {
				Metrics []Metric `json:"metrics"`
			} `json:"data"`
		} `json:"runs"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes)).Decode(&body); err != nil {
		return nil, err
	}
	allowedExperiments := asSet(s.experimentIDs)
	allowedMetrics := asSet(s.metricKeys)
	result := make([]EvaluationRun, 0, len(body.Runs))
	for _, run := range body.Runs {
		if len(result) >= limit || !allowedExperiments[run.Info.ExperimentID] || !validID(run.Info.RunID, 128) || !validRunStatus(run.Info.Status) || run.Info.StartTime < 0 || run.Info.EndTime < 0 {
			continue
		}
		metrics := make([]Metric, 0, len(run.Data.Metrics))
		for _, metric := range run.Data.Metrics {
			if allowedMetrics[metric.Key] && metric.Timestamp >= 0 && metric.Step >= 0 && !isInvalidMetric(metric.Value) {
				metrics = append(metrics, Metric{Key: metric.Key, Value: metric.Value, Timestamp: metric.Timestamp, Step: metric.Step})
			}
		}
		sort.Slice(metrics, func(i, j int) bool { return metrics[i].Key < metrics[j].Key })
		result = append(result, EvaluationRun{ExperimentID: run.Info.ExperimentID, RunID: run.Info.RunID, RunName: bounded(run.Info.RunName, 160), Status: run.Info.Status, StartedAt: run.Info.StartTime, EndedAt: run.Info.EndTime, Metrics: metrics})
	}
	return result, nil
}

func (s *service) configured() bool { return s.enabled && s.baseURL != nil && s.configErr == "" }

func (s *service) endpoint(path string) url.URL {
	endpoint := *s.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + path
	return endpoint
}

func parseLocalBaseURL(raw string) (*url.URL, string) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, baseURLEnv + " must be a plain local HTTP(S) URL without credentials, query data, or fragments"
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "localhost" && host != "host.docker.internal" && host != "mlflow" && net.ParseIP(host) == nil {
		return nil, baseURLEnv + " must resolve to localhost, host.docker.internal, mlflow, or a literal local IP"
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() && !ip.IsPrivate() {
		return nil, baseURLEnv + " must use a loopback or private-network IP"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed, ""
}

func parseExperimentIDs(raw string) ([]string, string) {
	return parseAllowlist(raw, experimentIDsEnv, maxExperiments, experimentIDRE)
}
func parseMetricKeys(raw string) ([]string, string) {
	return parseAllowlist(raw, metricKeysEnv, maxMetricKeys, metricKeyRE)
}

func parseAllowlist(raw, env string, max int, expression *regexp.Regexp) ([]string, string) {
	seen := map[string]bool{}
	values := make([]string, 0, max)
	for _, rawValue := range strings.Split(raw, ",") {
		value := strings.TrimSpace(rawValue)
		if value == "" {
			continue
		}
		if !expression.MatchString(value) || seen[value] {
			return nil, env + " contains an invalid or duplicate value"
		}
		seen[value] = true
		values = append(values, value)
		if len(values) > max {
			return nil, env + " exceeds its configured allowlist limit"
		}
	}
	if len(values) == 0 {
		return nil, env + " requires at least one explicitly approved value"
	}
	sort.Strings(values)
	return values, ""
}

func asSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func validToken(value string) bool {
	for _, character := range value {
		if character <= ' ' || character > '~' {
			return false
		}
	}
	return true
}

func validID(value string, max int) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > max {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
}

func validRunStatus(value string) bool {
	switch value {
	case "RUNNING", "SCHEDULED", "FINISHED", "FAILED", "KILLED":
		return true
	default:
		return false
	}
}

func isInvalidMetric(value float64) bool {
	return value != value || value > 1.7976931348623157e+308 || value < -1.7976931348623157e+308
}

func bounded(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
