package anythingllm

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

func TestRetrieveUsesOnlyConfiguredWorkspaceVectorSearch(t *testing.T) {
	client := &http.Client{Transport: roundTripper(func(request *http.Request) (*http.Response, error) {
		if got, want := request.URL.String(), "http://127.0.0.1:3001/api/v1/workspace/legal-workspace/vector-search"; got != want {
			t.Fatalf("URL = %q, want %q", got, want)
		}
		if request.Method != http.MethodPost || request.Header.Get("Authorization") != "Bearer local-token" {
			t.Fatalf("unexpected request: %s authorization=%q", request.Method, request.Header.Get("Authorization"))
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if got, want := payload["query"], "What evidence is available?"; got != want {
			t.Fatalf("query = %#v, want %q", got, want)
		}
		for _, forbidden := range []string{"message", "mode", "sessionId", "attachments", "reset", "scoreThreshold"} {
			if _, found := payload[forbidden]; found {
				t.Fatalf("payload must not include %q: %#v", forbidden, payload)
			}
		}
		body := `{"results":[{"id":"chunk-a","text":"Candidate evidence","score":0.9,"distance":0.1,"metadata":{"url":"file:///legal/letter.pdf","title":"Letter.pdf"}},{"id":"","text":"must not pass"}]}`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	service := NewService(true, "http://127.0.0.1:3001", "local-token", "legal-workspace", true, client)
	result, err := service.Retrieve(context.Background(), Request{Query: "What evidence is available?", WorkspaceSlug: "legal-workspace", Limit: 10})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(result.Results) != 1 || result.Results[0].ChunkID != "chunk-a" || result.Results[0].SourceURI != "file:///legal/letter.pdf" {
		t.Fatalf("results = %#v", result.Results)
	}
	if !strings.Contains(strings.ToLower(result.Scope), "candidate evidence") || !strings.Contains(strings.ToLower(result.Scope), "embedding") {
		t.Fatalf("scope must preserve embedding and verification boundary: %q", result.Scope)
	}
}

func TestProbeChecksOnlyConfiguredWorkspaces(t *testing.T) {
	client := &http.Client{Transport: roundTripper(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.Path != "/api/v1/workspaces" || request.Header.Get("Authorization") != "Bearer local-token" {
			t.Fatalf("unexpected probe request: %s %s auth=%q", request.Method, request.URL, request.Header.Get("Authorization"))
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"workspaces":[{"slug":"legal-workspace"},{"slug":"unapproved"}]}`))}, nil
	})}
	service := NewService(true, "http://127.0.0.1:3001", "local-token", "legal-workspace", true, client)
	if result, err := service.Probe(context.Background()); err != nil || !result.Reachable {
		t.Fatalf("Probe result=%#v err=%v", result, err)
	}
}

func TestLocalConfigRejectsUnsafeEndpointsOrIncompleteScope(t *testing.T) {
	for _, config := range []struct {
		endpoint, key, workspaces string
		localEmbeddings           bool
	}{
		{"https://example.com", "token", "workspace", true},
		{"https://user:secret@localhost:3001", "token", "workspace", true},
		{"https://localhost:3001/?key=secret", "token", "workspace", true},
		{"http://8.8.8.8:3001", "token", "workspace", true},
		{"http://127.0.0.1:3001", "", "workspace", true},
		{"http://127.0.0.1:3001", "token", "", true},
		{"http://127.0.0.1:3001", "token", "not valid", true},
		{"http://127.0.0.1:3001", "token", "workspace", false},
	} {
		service := NewService(true, config.endpoint, config.key, config.workspaces, config.localEmbeddings, nil)
		if service.Status().Configured {
			t.Fatalf("unsafe config unexpectedly enabled: %#v", config)
		}
	}
}

func TestRetrieveRejectsUnapprovedWorkspaceAndIsDisabledByDefault(t *testing.T) {
	service := NewService(true, "http://127.0.0.1:3001", "local-token", "approved", true, nil)
	if _, err := service.Retrieve(context.Background(), Request{Query: "anything", WorkspaceSlug: "other"}); err != ErrInvalidRequest {
		t.Fatalf("unapproved workspace error = %v, want ErrInvalidRequest", err)
	}
	disabled := NewService(false, "", "", "", false, nil)
	if _, err := disabled.Retrieve(context.Background(), Request{Query: "anything", WorkspaceSlug: "other"}); err != ErrNotConfigured {
		t.Fatalf("disabled retrieval error = %v, want ErrNotConfigured", err)
	}
}
