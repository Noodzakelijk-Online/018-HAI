package ragflow

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

func TestRetrieveUsesOnlyConfiguredDatasetsAndReadOnlyEndpoint(t *testing.T) {
	client := &http.Client{Transport: roundTripper(func(request *http.Request) (*http.Response, error) {
		if got, want := request.URL.String(), "http://127.0.0.1:9380/api/v1/retrieval"; got != want {
			t.Fatalf("URL = %q, want %q", got, want)
		}
		if request.Method != http.MethodPost || request.Header.Get("Authorization") != "Bearer local-token" {
			t.Fatalf("unexpected request: %s authorization=%q", request.Method, request.Header.Get("Authorization"))
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		for _, forbidden := range []string{"document_ids", "rerank_id", "metadata_condition", "cross_languages"} {
			if _, found := payload[forbidden]; found {
				t.Fatalf("payload must not include %q: %#v", forbidden, payload)
			}
		}
		if got := payload["dataset_ids"]; got == nil || len(got.([]any)) != 1 || got.([]any)[0] != "dataset-a" {
			t.Fatalf("dataset allowlist = %#v", got)
		}
		body := `{"code":0,"data":{"total":2,"chunks":[{"id":"chunk-a","kb_id":"dataset-a","document_id":"doc-a","document_keyword":"Evidence.pdf","content":"Candidate evidence","similarity":0.9},{"id":"chunk-b","kb_id":"other","content":"must not pass"}]}}`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	service := NewService(true, "http://127.0.0.1:9380", "local-token", "dataset-a", client)
	result, err := service.Retrieve(context.Background(), Request{Query: "What evidence is available?", Limit: 10})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(result.Results) != 1 || result.Results[0].DatasetID != "dataset-a" || result.Results[0].Content != "Candidate evidence" {
		t.Fatalf("results = %#v", result.Results)
	}
	if !strings.Contains(strings.ToLower(result.Scope), "candidate evidence") || !strings.Contains(strings.ToLower(result.Scope), "verification") {
		t.Fatalf("scope must preserve verification boundary: %q", result.Scope)
	}
}

func TestLocalConfigRejectsUnsafeEndpointsOrIncompleteScope(t *testing.T) {
	for _, config := range []struct{ endpoint, key, datasets string }{
		{"https://example.com", "token", "dataset-a"},
		{"https://user:secret@localhost:9380", "token", "dataset-a"},
		{"https://localhost:9380/?key=secret", "token", "dataset-a"},
		{"http://8.8.8.8:9380", "token", "dataset-a"},
		{"http://127.0.0.1:9380", "", "dataset-a"},
		{"http://127.0.0.1:9380", "token", ""},
		{"http://127.0.0.1:9380", "token", "dataset-a,not valid"},
	} {
		service := NewService(true, config.endpoint, config.key, config.datasets, nil)
		if service.Status().Configured {
			t.Fatalf("unsafe config unexpectedly enabled: %#v", config)
		}
	}
}

func TestRetrievalIsDisabledByDefault(t *testing.T) {
	service := NewService(false, "", "", "", nil)
	if _, err := service.Retrieve(context.Background(), Request{Query: "anything"}); err != ErrNotConfigured {
		t.Fatalf("Retrieve error = %v, want ErrNotConfigured", err)
	}
}

func TestProbeUsesOnlyDocumentedHealthEndpointWithoutCredential(t *testing.T) {
	client := &http.Client{Transport: roundTripper(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.Path != "/api/v1/system/healthz" {
			t.Fatalf("unexpected probe request: %s %s", request.Method, request.URL)
		}
		if request.Header.Get("Authorization") != "" {
			t.Fatal("health probe must not expose the retrieval credential")
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})}
	service := NewService(true, "http://127.0.0.1:9380", "local-token", "dataset-a", client)
	if result, err := service.Probe(context.Background()); err != nil || !result.Reachable {
		t.Fatalf("Probe result=%#v err=%v", result, err)
	}
}
