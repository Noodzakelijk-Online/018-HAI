package runnermaintenance

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

type gateStub struct {
	endpoint string
	modelID  string
	err      error
}

func (s *gateStub) EnsureConfiguredLocalModel(endpointURL, modelID string) error {
	s.endpoint, s.modelID = endpointURL, modelID
	return s.err
}

func TestEnsureConfiguredLocalModelAdmitsOnlyADeclaredFixedPair(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" || r.Header.Get("User-Agent") != "HAI-Test/1.0" {
			t.Fatalf("unexpected request %s", r.URL.String())
		}
		_, _ = w.Write([]byte(`{"status":"ok","configured":true,"modelId":"qwen2.5:7b","modelEndpoint":"http://127.0.0.1:11434/v1"}`))
	}))
	defer server.Close()
	baseURL, _ := url.Parse(server.URL)
	gate := &gateStub{}
	if err := EnsureConfiguredLocalModel(context.Background(), server.Client(), baseURL, "HAI-Test/1.0", "test", gate); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gate.endpoint != "http://127.0.0.1:11434/v1" || gate.modelID != "qwen2.5:7b" {
		t.Fatalf("unexpected gate request: %#v", gate)
	}
}

func TestEnsureConfiguredLocalModelRejectsUnconfiguredOrBlockedRunner(t *testing.T) {
	unconfiguredServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","configured":false}`))
	}))
	defer unconfiguredServer.Close()
	unconfiguredURL, _ := url.Parse(unconfiguredServer.URL)
	if err := EnsureConfiguredLocalModel(context.Background(), unconfiguredServer.Client(), unconfiguredURL, "HAI-Test/1.0", "test", &gateStub{}); err == nil {
		t.Fatal("unconfigured runner was accepted")
	}

	blocked := &gateStub{err: errors.New("daily check failed")}
	configuredServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","configured":true,"modelId":"qwen2.5:7b","modelEndpoint":"http://127.0.0.1:11434/v1"}`))
	}))
	defer configuredServer.Close()
	configuredURL, _ := url.Parse(configuredServer.URL)
	if err := EnsureConfiguredLocalModel(context.Background(), configuredServer.Client(), configuredURL, "HAI-Test/1.0", "test", blocked); err == nil {
		t.Fatal("blocked model was accepted")
	}
}
