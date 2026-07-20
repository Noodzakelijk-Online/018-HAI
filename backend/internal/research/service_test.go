package research

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestSearchUsesBoundedLocalSearXNGJSONAPI(t *testing.T) {
	client := &http.Client{Transport: roundTripper(func(request *http.Request) (*http.Response, error) {
		if got, want := request.URL.String(), "http://127.0.0.1:8080/search?categories=general&format=json&q=public+source&safesearch=2"; got != want {
			t.Fatalf("request URL = %q, want %q", got, want)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"results":[{"title":"Example","url":"https://example.com/path?tracking=1#anchor","content":"Evidence candidate","engines":["duckduckgo"],"publishedDate":"2026-07-19"},{"title":"Bad","url":"file:///tmp/private","content":"skip"}]}`))}, nil
	})}
	service := NewService(true, "http://127.0.0.1:8080", client)
	result, err := service.Search(context.Background(), Request{Query: "public source", Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(result.Results) != 1 || result.Results[0].SourceURI != "https://example.com/path" || result.Results[0].Title != "Example" {
		t.Fatalf("results = %#v", result.Results)
	}
	if !strings.Contains(strings.ToLower(result.Scope), "never treats search snippets as verified evidence") {
		t.Fatalf("scope must make the verification boundary explicit: %q", result.Scope)
	}
}

func TestLocalConfigRejectsRemoteOrCredentialedEndpoints(t *testing.T) {
	for _, endpoint := range []string{"https://example.com", "https://user:secret@localhost:8080", "https://localhost:8080/?key=secret", "http://8.8.8.8"} {
		service := NewService(true, endpoint, nil)
		if service.Status().Configured {
			t.Fatalf("endpoint %q must not be configured", endpoint)
		}
	}
}

func TestProbeUsesOnlyLocalHealthEndpointWithoutQueryOrCredentials(t *testing.T) {
	client := &http.Client{Transport: roundTripper(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.String() != "http://127.0.0.1:8080/healthz" {
			t.Fatalf("unexpected probe request: %s %s", request.Method, request.URL)
		}
		if request.Header.Get("Authorization") != "" || request.URL.RawQuery != "" {
			t.Fatalf("probe leaked credentials or query data: %#v", request)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("OK"))}, nil
	})}
	service := NewService(true, "http://127.0.0.1:8080", client)
	result, err := service.Probe(context.Background())
	if err != nil || !result.Reachable || !strings.Contains(result.Scope, "does not verify JSON output") {
		t.Fatalf("Probe result=%#v err=%v", result, err)
	}
}

func TestProbeRejectsUnhealthyHTTPStatus(t *testing.T) {
	client := &http.Client{Transport: roundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("unavailable"))}, nil
	})}
	service := NewService(true, "http://127.0.0.1:8080", client)
	if result, err := service.Probe(context.Background()); err == nil || result != nil {
		t.Fatalf("Probe result=%#v err=%v, want health failure", result, err)
	}
}

func TestSearchRejectsSearXNGExternalBangRedirectSyntax(t *testing.T) {
	service := NewService(true, "http://127.0.0.1:8080", &http.Client{Transport: roundTripper(func(*http.Request) (*http.Response, error) {
		t.Fatal("external bang query must not reach SearXNG")
		return nil, nil
	})})
	if _, err := service.Search(context.Background(), Request{Query: "!! example external search"}); err == nil {
		t.Fatal("external bang query was accepted")
	}
}

func TestSearchIsDisabledByDefault(t *testing.T) {
	service := NewService(false, "", nil)
	if _, err := service.Search(context.Background(), Request{Query: "anything"}); err != ErrNotConfigured {
		t.Fatalf("Search error = %v, want ErrNotConfigured", err)
	}
}
