package garak

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGarakBridgeUsesOnlyConfiguredFixedSuite(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			if r.Method != http.MethodGet || r.Header.Get("Authorization") != "" || r.Header.Get("User-Agent") != "HAI-Garak/1.0" {
				t.Fatalf("unexpected health request")
			}
			_, _ = w.Write([]byte(`{"status":"ok","engine":"garak 0.15.1","configured":true,"suite":"hai_prompt_injection_regression_v1","modelId":"qwen2.5:7b"}`))
		case "/v1/run":
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "" || r.Header.Get("User-Agent") != "HAI-Garak/1.0" {
				t.Fatalf("unexpected scan request")
			}
			_, _ = w.Write([]byte(`{"status":"completed","engine":"garak 0.15.1","suite":"hai_prompt_injection_regression_v1","modelId":"qwen2.5:7b","probe":"promptinject.HijackLongPrompt","detector":"promptinject.AttackRogueString","caseCount":4,"passedCount":3,"failedCount":1,"score":0.75,"durationMs":1200,"resultDigest":"1234567890123456789012345678901234567890123456789012345678901234"}`))
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
	if err != nil || result.CaseCount != 4 || result.PassedCount != 3 || result.Probe != probeName {
		t.Fatalf("unexpected scan result: %#v %v", result, err)
	}
}

func TestGarakBridgeRejectsExternalAndDisabledConfiguration(t *testing.T) {
	external := NewService(true, "https://example.com", 0, nil)
	if external.Status().Configured || external.Status().ConfigError == "" {
		t.Fatalf("external runner must be rejected: %#v", external.Status())
	}
	disabled := NewService(false, "http://127.0.0.1:8080", 0, nil)
	if _, err := disabled.Run(context.Background()); err != ErrNotConfigured {
		t.Fatalf("disabled runner must not be contacted: %v", err)
	}
}

func TestGarakResultDigestMustBeHex(t *testing.T) {
	result := Result{Status: "completed", Engine: "garak 0.15.1", Suite: suiteName, ModelID: "qwen2.5:7b", Probe: probeName, Detector: detectorName, CaseCount: 4, PassedCount: 4, Score: 1, DurationMS: 1, ResultDigest: strings.Repeat("z", 64)}
	if validResult(result) {
		t.Fatal("non-hex result digest was accepted")
	}
}
