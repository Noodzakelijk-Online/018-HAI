package deepeval

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type maintenanceGateStub struct { endpoint, modelID string; err error }
func (s *maintenanceGateStub) EnsureConfiguredLocalModel(endpointURL, modelID string) error { s.endpoint, s.modelID = endpointURL, modelID; return s.err }

func TestDeepEvalBridgeUsesOnlyConfiguredFixedSuite(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			if r.Method != http.MethodGet || r.Header.Get("Authorization") != "" || r.Header.Get("User-Agent") != "HAI-DeepEval/1.0" {
				t.Fatalf("unexpected health request")
			}
			_, _ = w.Write([]byte(`{"status":"ok","engine":"deepeval 4.1.1","configured":true,"suite":"hai_source_grounding_regression_v1","metric":"FaithfulnessMetric","modelId":"qwen2.5:7b","modelEndpoint":"http://127.0.0.1:11434/v1"}`))
		case "/v1/run":
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "" || r.Header.Get("User-Agent") != "HAI-DeepEval/1.0" {
				t.Fatalf("unexpected evaluation request")
			}
			_, _ = w.Write([]byte(`{"status":"completed","engine":"deepeval 4.1.1","suite":"hai_source_grounding_regression_v1","metric":"FaithfulnessMetric","modelId":"qwen2.5:7b","caseCount":3,"passedCount":2,"failedCount":1,"score":0.75,"durationMs":1200,"resultDigest":"1234567890123456789012345678901234567890123456789012345678901234"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	gate := &maintenanceGateStub{}
	service := WithModelMaintenance(NewService(true, server.URL, 0, nil), gate)
	if probe, err := service.Probe(context.Background()); err != nil || !probe.Reachable {
		t.Fatalf("unexpected probe: %#v %v", probe, err)
	}
	result, err := service.Run(context.Background())
	if err != nil || result.CaseCount != 3 || result.PassedCount != 2 || result.Metric != metricName {
		t.Fatalf("unexpected evaluation result: %#v %v", result, err)
	}
	if gate.endpoint != "http://127.0.0.1:11434/v1" || gate.modelID != "qwen2.5:7b" { t.Fatalf("maintenance gate was not called: %#v", gate) }
}

func TestDeepEvalBridgeRejectsExternalAndDisabledConfiguration(t *testing.T) {
	external := NewService(true, "https://example.com", 0, nil)
	if external.Status().Configured || external.Status().ConfigError == "" {
		t.Fatalf("external runner must be rejected: %#v", external.Status())
	}
	disabled := NewService(false, "http://127.0.0.1:8080", 0, nil)
	if _, err := disabled.Run(context.Background()); err != ErrNotConfigured {
		t.Fatalf("disabled runner must not be contacted: %v", err)
	}
}

func TestDeepEvalResultDigestMustBeHex(t *testing.T) {
	result := Result{Status: "completed", Engine: "deepeval 4.1.1", Suite: suiteName, Metric: metricName, ModelID: "qwen2.5:7b", CaseCount: 3, PassedCount: 3, FailedCount: 0, Score: 1, DurationMS: 1, ResultDigest: strings.Repeat("z", 64)}
	if validResult(result) {
		t.Fatal("non-hex result digest was accepted")
	}
}
