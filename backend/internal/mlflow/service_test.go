package mlflow

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestRecentRunsUsesFixedExperimentAndMetricAllowlists(t *testing.T) {
	client := &http.Client{Transport: roundTripper(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.String() != "http://127.0.0.1:5000/api/2.0/mlflow/runs/search" {
			t.Fatalf("unexpected request %s %s", request.Method, request.URL)
		}
		if request.Header.Get("Authorization") != "Bearer local-token" {
			t.Fatalf("unexpected auth %q", request.Header.Get("Authorization"))
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if got := payload["experiment_ids"].([]any); len(got) != 1 || got[0] != "12" {
			t.Fatalf("experiment ids %#v", got)
		}
		if _, exists := payload["filter"]; exists {
			t.Fatalf("caller-controlled filter must not be sent: %#v", payload)
		}
		body := `{"runs":[{"info":{"run_id":"run-1","experiment_id":"12","run_name":"evaluation","status":"FINISHED","start_time":10,"end_time":11},"data":{"metrics":[{"key":"accuracy","value":0.92,"timestamp":11,"step":1},{"key":"secret_metric","value":1}] }},{"info":{"run_id":"run-2","experiment_id":"99","status":"FINISHED"},"data":{"metrics":[{"key":"accuracy","value":1}]}}]}`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	service := NewService(true, "http://127.0.0.1:5000", "local-token", "12", "accuracy", 0, client)
	result, err := service.RecentRuns(context.Background(), 25)
	if err != nil {
		t.Fatalf("RecentRuns: %v", err)
	}
	if len(result.Runs) != 1 || result.Runs[0].RunID != "run-1" || len(result.Runs[0].Metrics) != 1 || result.Runs[0].Metrics[0].Key != "accuracy" {
		t.Fatalf("result %#v", result)
	}
}

func TestConfigurationFailsClosedForUnsafeOrIncompleteScope(t *testing.T) {
	for _, config := range []struct{ endpoint, experiments, metrics string }{
		{"https://example.com", "12", "accuracy"},
		{"http://user:secret@localhost:5000", "12", "accuracy"},
		{"http://127.0.0.1:5000", "", "accuracy"},
		{"http://127.0.0.1:5000", "12", ""},
		{"http://127.0.0.1:5000", "12,12", "accuracy"},
		{"http://127.0.0.1:5000", "12", "bad metric"},
	} {
		service := NewService(true, config.endpoint, "", config.experiments, config.metrics, 0, nil)
		if service.Status().Configured {
			t.Fatalf("unsafe config configured: %#v", config)
		}
	}
}

func TestDisabledBridgeCannotReadRuns(t *testing.T) {
	service := NewService(false, "", "", "", "", 0, nil)
	if _, err := service.RecentRuns(context.Background(), 1); err != ErrNotConfigured {
		t.Fatalf("error=%v", err)
	}
}

func TestProbeReadsOnlyConfiguredScope(t *testing.T) {
	client := &http.Client{Transport: roundTripper(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/api/2.0/mlflow/runs/search" || request.Method != http.MethodPost {
			t.Fatalf("unexpected probe %s %s", request.Method, request.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"runs":[]}`))}, nil
	})}
	service := NewService(true, "http://127.0.0.1:5000", "", "12", "accuracy", 0, client)
	if probe, err := service.Probe(context.Background()); err != nil || !probe.Reachable {
		t.Fatalf("probe=%#v err=%v", probe, err)
	}
}
