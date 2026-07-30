package openlit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestExportUsesFixedAggregateOnlyOTLPTrace(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/traces" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	service := NewService(true, server.URL, 3*time.Second, nil).(*service)
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
	encoded, _ := json.Marshal(payload)
	text := strings.ToLower(string(encoded))
	for _, forbidden := range []string{"prompt", "completion", "source.text", "workflow.", "token.", "model.", "credential", "secret"} {
		if strings.Contains(text, forbidden) {
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
	if _, err := NewService(true, server.URL, 3*time.Second, nil).ExportOperationalSnapshot(context.Background()); err == nil {
		t.Fatal("rejected spans must fail export")
	}
}

func TestConfigurationRejectsExternalAndUnexpectedEndpoints(t *testing.T) {
	for _, endpoint := range []string{"https://openlit.io", "http://127.0.0.1:4318/other", "http://bad key:4318"} {
		if service := NewService(true, endpoint, 3*time.Second, nil); service.Status().Configured {
			t.Fatalf("invalid configuration unexpectedly accepted: %s", endpoint)
		}
	}
}
