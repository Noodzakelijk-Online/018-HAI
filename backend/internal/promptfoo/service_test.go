package promptfoo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPromptfooBridgeUsesOnlyConfiguredFixedSuite(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			if r.Method != http.MethodGet || r.Header.Get("Authorization") != "" || r.Header.Get("User-Agent") != "HAI-Promptfoo/1.0" {
				t.Fatalf("unexpected health request")
			}
			_, _ = w.Write([]byte(`{"status":"ok","engine":"promptfoo 0.121.19"}`))
		case "/v1/run":
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "" || r.Header.Get("User-Agent") != "HAI-Promptfoo/1.0" {
				t.Fatalf("unexpected evaluation request")
			}
			_, _ = w.Write([]byte(`{"status":"completed","engine":"promptfoo 0.121.19","suite":"hai_safety_regression_v1","modelId":"qwen2.5:7b","caseCount":6,"passedCount":5,"failedCount":1,"score":0.833333,"durationMs":1200,"resultDigest":"1234567890123456789012345678901234567890123456789012345678901234"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	service := NewService(true, server.URL, 0, nil)
	if probe, err := service.Probe(context.Background()); err != nil || !probe.Reachable {
		t.Fatalf("unexpected probe: %#v %v", probe, err)
	}
	result, err := service.Run(context.Background())
	if err != nil || result.CaseCount != 6 || result.PassedCount != 5 {
		t.Fatalf("unexpected evaluation result: %#v %v", result, err)
	}
}

func TestPromptfooBridgeRejectsExternalAndDisabledConfiguration(t *testing.T) {
	external := NewService(true, "https://example.com", 0, nil)
	if external.Status().Configured || external.Status().ConfigError == "" {
		t.Fatalf("external runner must be rejected: %#v", external.Status())
	}
	disabled := NewService(false, "http://127.0.0.1:8080", 0, nil)
	if _, err := disabled.Run(context.Background()); err != ErrNotConfigured {
		t.Fatalf("disabled runner must not be contacted: %v", err)
	}
}
