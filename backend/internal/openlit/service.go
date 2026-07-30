// Package openlit exports one explicit, aggregate-only HAI operational trace
// to an operator-hosted local OpenLIT OTLP/HTTP collector. It is not an LLM
// gateway, prompt store, source store, evaluator, or workflow authority.
package openlit

import (
	"bytes"
	"context"
	"crypto/rand"
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
	enabledEnv             = "HAI_OPENLIT_ENABLED"
	otlpEndpointEnv        = "HAI_OPENLIT_OTLP_ENDPOINT"
	timeoutEnv             = "HAI_OPENLIT_TIMEOUT_SECONDS"
	maxResponseBytes int64 = 64 << 10
)

var ErrNotConfigured = errors.New("local OpenLIT observability bridge is not configured")

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

type ExportResult struct {
	TraceID    string    `json:"traceId"`
	SpanID     string    `json:"spanId"`
	ExportedAt time.Time `json:"exportedAt"`
	Scope      string    `json:"scope"`
}

type Service interface {
	Status() Status
	ExportOperationalSnapshot(context.Context) (*ExportResult, error)
}

type service struct {
	enabled   bool
	endpoint  *url.URL
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
	return NewService(strings.EqualFold(strings.TrimSpace(os.Getenv(enabledEnv)), "true"), os.Getenv(otlpEndpointEnv), timeout, nil)
}

func NewService(enabled bool, rawEndpoint string, timeout time.Duration, client *http.Client) Service {
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
	s := &service{enabled: enabled, client: client, now: time.Now, newID: randomHex}
	if enabled {
		s.endpoint, s.configErr = parseLocalOTLPEndpoint(rawEndpoint)
	}
	return s
}

func (s *service) Status() Status {
	status := Status{
		Enabled:     s.enabled,
		Configured:  s.configured(),
		Provider:    "OpenLIT local OTLP observability",
		ConfigError: s.configErr,
		Capabilities: []string{
			"owner-triggered aggregate-only operational trace export over OTLP/HTTP JSON",
			"collector acceptance check through the fixed trace export",
		},
		Restrictions: []string{
			"no OpenLIT SDK or automatic instrumentation in HAI",
			"no prompt, completion, source text, file, token, model payload, workflow record, credential, or caller-provided attribute export",
			"no collector installation, health-path guess, remote endpoint, routing, approval, verification, memory, policy, or execution authority",
		},
		Scope: "Owner-triggered aggregate HAI control-plane observability only. OpenLIT remains an optional local trace viewer; HAI remains the authority for audit, safety, routing, verification, and execution decisions.",
	}
	if s.endpoint != nil {
		status.Endpoint = s.endpoint.String()
	}
	return status
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
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("could not create local OpenLIT trace request")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "HAI-OpenLIT-Observability/1.0")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("local OpenLIT trace export is unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("local OpenLIT trace export was not accepted")
	}
	var body struct {
		PartialSuccess *struct {
			RejectedSpans json.RawMessage `json:"rejectedSpans"`
		} `json:"partialSuccess"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes)).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("local OpenLIT trace export returned invalid metadata")
	}
	if body.PartialSuccess != nil && rejectedSpans(body.PartialSuccess.RejectedSpans) > 0 {
		return nil, fmt.Errorf("local OpenLIT rejected the aggregate observability trace")
	}
	return &ExportResult{TraceID: traceID, SpanID: spanID, ExportedAt: now, Scope: s.Status().Scope}, nil
}

func (s *service) configured() bool { return s.enabled && s.endpoint != nil && s.configErr == "" }

func parseLocalOTLPEndpoint(raw string) (*url.URL, string) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, otlpEndpointEnv + " must be a plain local HTTP(S) URL without credentials, query data, or fragments"
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "localhost" && host != "host.docker.internal" && host != "openlit" && net.ParseIP(host) == nil {
		return nil, otlpEndpointEnv + " must resolve to localhost, host.docker.internal, openlit, or a literal local IP"
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() && !ip.IsPrivate() {
		return nil, otlpEndpointEnv + " must use a loopback or private-network IP"
	}
	path := strings.TrimRight(parsed.EscapedPath(), "/")
	if path != "" && path != "/v1/traces" {
		return nil, otlpEndpointEnv + " must be a collector base URL or its fixed /v1/traces endpoint"
	}
	parsed.Path = "/v1/traces"
	parsed.RawPath = ""
	return parsed, ""
}

func rejectedSpans(raw json.RawMessage) int64 {
	if len(raw) == 0 {
		return 0
	}
	var count int64
	if err := json.Unmarshal(raw, &count); err == nil {
		return count
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return 1
	}
	var parsed int64
	if _, err := fmt.Sscan(text, &parsed); err != nil {
		return 1
	}
	return parsed
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
				"scope": map[string]any{"name": "hai.openlit.bridge", "version": "1"},
				"spans": []any{map[string]any{
					"traceId": traceID, "spanId": spanID, "name": "hai.observability.manual_snapshot", "kind": 1,
					"startTimeUnixNano": nanos, "endTimeUnixNano": nanos,
					"attributes": []otlpAttribute{
						otlpString("hai.export.schema", "1"),
						otlpString("hai.export.scope", "aggregate_only"),
						otlpString("hai.export.trigger", "owner_manual"),
						otlpString("hai.message.capture", "disabled"),
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
