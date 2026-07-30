// Package langfuse exports one explicit, aggregate-only HAI operational trace
// to an operator-hosted local Langfuse instance. It is not a model gateway,
// prompt store, source store, evaluator, or workflow authority.
package langfuse

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
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
	enabledEnv             = "HAI_LANGFUSE_ENABLED"
	baseURLEnv             = "HAI_LANGFUSE_BASE_URL"
	publicKeyEnv           = "HAI_LANGFUSE_PUBLIC_KEY"
	secretKeyEnv           = "HAI_LANGFUSE_SECRET_KEY"
	timeoutEnv             = "HAI_LANGFUSE_TIMEOUT_SECONDS"
	maxResponseBytes int64 = 64 << 10
)

var ErrNotConfigured = errors.New("local Langfuse observability bridge is not configured")

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

type ProbeResult struct {
	Healthy   bool      `json:"healthy"`
	Ready     bool      `json:"ready"`
	CheckedAt time.Time `json:"checkedAt"`
	Scope     string    `json:"scope"`
}

type ExportResult struct {
	TraceID    string    `json:"traceId"`
	SpanID     string    `json:"spanId"`
	ExportedAt time.Time `json:"exportedAt"`
	Scope      string    `json:"scope"`
}

type Service interface {
	Status() Status
	Probe(context.Context) (*ProbeResult, error)
	ExportOperationalSnapshot(context.Context) (*ExportResult, error)
}

type service struct {
	enabled   bool
	baseURL   *url.URL
	publicKey string
	secretKey string
	configErr string
	client    *http.Client
	now       func() time.Time
	newID     func(int) (string, error)
}

func DefaultService() Service {
	timeout := 8 * time.Second
	if raw := strings.TrimSpace(os.Getenv(timeoutEnv)); raw != "" {
		if seconds, err := time.ParseDuration(raw + "s"); err == nil && seconds >= 2*time.Second && seconds <= 30*time.Second {
			timeout = seconds
		}
	}
	return NewService(
		strings.EqualFold(strings.TrimSpace(os.Getenv(enabledEnv)), "true"),
		os.Getenv(baseURLEnv),
		os.Getenv(publicKeyEnv),
		os.Getenv(secretKeyEnv),
		timeout,
		nil,
	)
}

func NewService(enabled bool, rawBaseURL, publicKey, secretKey string, timeout time.Duration, client *http.Client) Service {
	if timeout < 2*time.Second || timeout > 30*time.Second {
		timeout = 8 * time.Second
	}
	if client == nil {
		client = &http.Client{
			Timeout:       timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
			Transport:     &http.Transport{Proxy: nil},
		}
	}
	s := &service{enabled: enabled, publicKey: strings.TrimSpace(publicKey), secretKey: strings.TrimSpace(secretKey), client: client, now: time.Now, newID: randomHex}
	if enabled {
		s.baseURL, s.configErr = parseLocalBaseURL(rawBaseURL)
		if s.configErr == "" && !validKey(s.publicKey) {
			s.configErr = publicKeyEnv + " is required when " + enabledEnv + " is true"
		}
		if s.configErr == "" && !validKey(s.secretKey) {
			s.configErr = secretKeyEnv + " is required when " + enabledEnv + " is true"
		}
	}
	return s
}

func (s *service) Status() Status {
	status := Status{
		Enabled:     s.enabled,
		Configured:  s.configured(),
		Provider:    "Langfuse self-hosted observability",
		ConfigError: s.configErr,
		Capabilities: []string{
			"local health and readiness probe",
			"explicit aggregate-only operational trace export over OTLP/HTTP JSON",
		},
		Restrictions: []string{
			"no automatic export, prompt, completion, source text, file, token, model payload, workflow record, or credential export",
			"no Langfuse prompt, dataset, score, evaluation, web-callout, organization, project, or user API access",
			"a trace cannot change HAI routing, approval, verification, memory, workflow, provider, policy, or execution state",
		},
		Scope: "Owner-triggered aggregate HAI control-plane observability only. Langfuse remains an optional local trace viewer; HAI remains the authority for audit, safety, routing, verification, and execution decisions.",
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
	if err := s.getOK(ctx, "/api/public/health?failIfDatabaseUnavailable=true"); err != nil {
		return nil, fmt.Errorf("local Langfuse health check failed")
	}
	if err := s.getOK(ctx, "/api/public/ready"); err != nil {
		return nil, fmt.Errorf("local Langfuse readiness check failed")
	}
	return &ProbeResult{Healthy: true, Ready: true, CheckedAt: s.now().UTC(), Scope: "Endpoint, database-aware health, and readiness only. This does not validate project permissions, trace ingestion, data retention, redaction, or Langfuse configuration."}, nil
}

func (s *service) ExportOperationalSnapshot(ctx context.Context) (*ExportResult, error) {
	if !s.configured() {
		return nil, ErrNotConfigured
	}
	traceID, err := s.newID(16)
	if err != nil {
		return nil, fmt.Errorf("could not create observability trace identifier")
	}
	spanID, err := s.newID(8)
	if err != nil {
		return nil, fmt.Errorf("could not create observability span identifier")
	}
	now := s.now().UTC()
	payload, err := json.Marshal(otlpTracePayload(traceID, spanID, now))
	if err != nil {
		return nil, fmt.Errorf("could not encode aggregate observability trace")
	}
	endpoint := s.endpoint("/api/public/otel/v1/traces")
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("could not create local Langfuse trace request")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(s.publicKey+":"+s.secretKey)))
	request.Header.Set("User-Agent", "HAI-Langfuse-Observability/1.0")
	request.Header.Set("x-langfuse-ingestion-version", "4")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("local Langfuse trace export is unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("local Langfuse trace export was not accepted")
	}
	var body struct {
		PartialSuccess *struct {
			RejectedSpans int64 `json:"rejectedSpans"`
		} `json:"partialSuccess"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes)).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("local Langfuse trace export returned invalid metadata")
	}
	if body.PartialSuccess != nil && body.PartialSuccess.RejectedSpans > 0 {
		return nil, fmt.Errorf("local Langfuse rejected the aggregate observability trace")
	}
	return &ExportResult{TraceID: traceID, SpanID: spanID, ExportedAt: now, Scope: s.Status().Scope}, nil
}

func (s *service) getOK(ctx context.Context, path string) error {
	endpoint := s.endpoint(path)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "HAI-Langfuse-Observability/1.0")
	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes))
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status")
	}
	return nil
}

func (s *service) configured() bool { return s.enabled && s.baseURL != nil && s.configErr == "" }

func (s *service) endpoint(path string) url.URL {
	endpoint := *s.baseURL
	query := ""
	if index := strings.IndexByte(path, '?'); index >= 0 {
		query, path = path[index+1:], path[:index]
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + path
	endpoint.RawQuery = query
	return endpoint
}

func parseLocalBaseURL(raw string) (*url.URL, string) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, baseURLEnv + " must be a plain local HTTP(S) URL without credentials, query data, or fragments"
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "localhost" && host != "host.docker.internal" && host != "langfuse" && host != "langfuse-web" && net.ParseIP(host) == nil {
		return nil, baseURLEnv + " must resolve to localhost, host.docker.internal, langfuse, langfuse-web, or a literal local IP"
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() && !ip.IsPrivate() {
		return nil, baseURLEnv + " must use a loopback or private-network IP"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed, ""
}

func validKey(value string) bool {
	if len(value) < 8 || len(value) > 512 {
		return false
	}
	for _, character := range value {
		if character <= ' ' || character > '~' || character == ':' {
			return false
		}
	}
	return true
}

func randomHex(bytesCount int) (string, error) {
	value := make([]byte, bytesCount)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

type otlpAttribute struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

func otlpString(key, value string) otlpAttribute {
	return otlpAttribute{Key: key, Value: map[string]string{"stringValue": value}}
}

func otlpTracePayload(traceID, spanID string, at time.Time) map[string]any {
	nanos := fmt.Sprintf("%d", at.UnixNano())
	return map[string]any{
		"resourceSpans": []any{map[string]any{
			"resource": map[string]any{"attributes": []otlpAttribute{
				otlpString("service.name", "hai-control-plane"),
				otlpString("service.namespace", "hai"),
				otlpString("telemetry.sdk.language", "go"),
				otlpString("hai.data.classification", "aggregate_only"),
			}},
			"scopeSpans": []any{map[string]any{
				"scope": map[string]any{"name": "hai.langfuse.bridge", "version": "1"},
				"spans": []any{map[string]any{
					"traceId": traceID, "spanId": spanID, "name": "hai.observability.manual_snapshot", "kind": 1,
					"startTimeUnixNano": nanos, "endTimeUnixNano": nanos,
					"attributes": []otlpAttribute{
						otlpString("hai.export.schema", "1"),
						otlpString("hai.export.scope", "aggregate_only"),
						otlpString("hai.export.trigger", "owner_manual"),
						otlpString("hai.approval.authority", "hai"),
						otlpString("hai.execution.authority", "hai"),
						otlpString("hai.external.action", "false"),
					},
					"status": map[string]any{"code": 1},
				}},
			}},
		}},
	}
}
