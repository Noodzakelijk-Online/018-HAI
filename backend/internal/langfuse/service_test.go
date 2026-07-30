package langfuse

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	testPublicKey = "pk-lf-test-public-key"
	testSecretKey = "sk-lf-test-secret-key"
)

func TestProbeRequiresDatabaseAwareHealthAndReadiness(t *testing.T) {
	requests := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.String())
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	service := NewService(true, server.URL, testPublicKey, testSecretKey, 3*time.Second, nil)
	result, err := service.Probe(context.Background())
	if err != nil || !result.Healthy || !result.Ready {
		t.Fatalf("probe result=%+v err=%v", result, err)
	}
	if len(requests) != 2 || requests[0] != "/api/public/health?failIfDatabaseUnavailable=true" || requests[1] != "/api/public/ready" {
		t.Fatalf("unexpected probe requests: %#v", requests)
	}
}

func TestProbeFailsWhenReadinessFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/public/ready" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	if _, err := NewService(true, server.URL, testPublicKey, testSecretKey, time.Second*3, nil).Probe(context.Background()); err == nil {
		t.Fatal("readiness failure must fail the probe")
	}
}

func TestExportUsesFixedAggregateOnlyOTLPTrace(t *testing.T) {
	var authorization, version string
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/public/otel/v1/traces" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
		authorization, version = r.Header.Get("Authorization"), r.Header.Get("x-langfuse-ingestion-version")
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	service := NewService(true, server.URL, testPublicKey, testSecretKey, 3*time.Second, nil).(*service)
	service.now = func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }
	service.newID = func(bytes int) (string, error) {
		if bytes == 16 {
			return strings.Repeat("a", 32), nil
		}
		return strings.Repeat("b", 16), nil
	}
	result, err := service.ExportOperationalSnapshot(context.Background())
	if err != nil || result.TraceID != strings.Repeat("a", 32) || result.SpanID != strings.Repeat("b", 16) {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if authorization == "" || version != "4" {
		t.Fatalf("missing Langfuse authentication/version headers: %q / %q", authorization, version)
	}
	encoded, _ := json.Marshal(payload)
	text := string(encoded)
	for _, forbidden := range []string{testPublicKey, testSecretKey, "prompt", "completion", "source.text", "workflow.", "token.", "model."} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("aggregate payload unexpectedly contains %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, "aggregate_only") || !strings.Contains(text, "hai.observability.manual_snapshot") {
		t.Fatalf("payload did not contain fixed aggregate schema: %s", text)
	}
}

func TestExportRejectsPartialAcceptance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"partialSuccess":{"rejectedSpans":"1"}}`))
	}))
	defer server.Close()
	if _, err := NewService(true, server.URL, testPublicKey, testSecretKey, 3*time.Second, nil).ExportOperationalSnapshot(context.Background()); err == nil {
		t.Fatal("rejected spans must fail export")
	}
}

func TestConfigurationRejectsExternalEndpointAndInvalidKeys(t *testing.T) {
	for _, configuration := range []struct{ baseURL, publicKey, secretKey string }{
		{"https://cloud.langfuse.com", testPublicKey, testSecretKey},
		{"http://127.0.0.1:3000", "bad key", testSecretKey},
		{"http://127.0.0.1:3000", testPublicKey, ""},
	} {
		if service := NewService(true, configuration.baseURL, configuration.publicKey, configuration.secretKey, 3*time.Second, nil); service.Status().Configured {
			t.Fatalf("invalid configuration unexpectedly accepted: %+v", configuration)
		}
	}
}
