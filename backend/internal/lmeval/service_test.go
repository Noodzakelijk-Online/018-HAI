package lmeval

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type maintenanceGateStub struct { endpoint, modelID string; err error }
func (s *maintenanceGateStub) EnsureConfiguredLocalModel(endpointURL, modelID string) error { s.endpoint, s.modelID = endpointURL, modelID; return s.err }

func TestLMEvalBridgeUsesOnlyConfiguredFixedSuite(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			if r.Method != http.MethodGet || r.Header.Get("Authorization") != "" || r.Header.Get("User-Agent") != "HAI-LM-Eval/1.0" {
				t.Fatalf("unexpected health request")
			}
			_, _ = w.Write([]byte(`{"status":"ok","engine":"lm-eval 0.4.12","configured":true,"modelId":"qwen2.5:7b","modelEndpoint":"http://127.0.0.1:11434/v1"}`))
		case "/v1/run":
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "" || r.Header.Get("User-Agent") != "HAI-LM-Eval/1.0" {
				t.Fatalf("unexpected evaluation request")
			}
			_, _ = w.Write([]byte(`{"status":"completed","engine":"lm-eval 0.4.12","suite":"hai_synthetic_v1","modelId":"qwen2.5:7b","caseCount":6,"exactMatch":0.833333,"durationMs":1200,"resultDigest":"1234567890123456789012345678901234567890123456789012345678901234"}`))
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
	if err != nil || result.CaseCount != 6 || result.ModelID != "qwen2.5:7b" {
		t.Fatalf("unexpected evaluation result: %#v %v", result, err)
	}
	if gate.endpoint != "http://127.0.0.1:11434/v1" || gate.modelID != "qwen2.5:7b" { t.Fatalf("maintenance gate was not called: %#v", gate) }
}

func TestLMEvalBridgeRejectsExternalAndDisabledConfiguration(t *testing.T) {
	external := NewService(true, "https://example.com", 0, nil)
	if external.Status().Configured || external.Status().ConfigError == "" {
		t.Fatalf("external runner must be rejected: %#v", external.Status())
	}
	disabled := NewService(false, "http://127.0.0.1:8080", 0, nil)
	if _, err := disabled.Run(context.Background()); err != ErrNotConfigured {
		t.Fatalf("disabled runner must not be contacted: %v", err)
	}
}
