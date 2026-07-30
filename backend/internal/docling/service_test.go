package docling

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestExtractUsesOnlyConfiguredLocalFolderAndRunnerToken(t *testing.T) {
	client := &http.Client{Transport: roundTrip(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/v1/extract" {
			t.Fatalf("path = %s", request.URL.Path)
		}
		if request.Header.Get("X-HAI-Docling-Token") != "runner-token-123456" {
			t.Fatalf("runner token missing")
		}
		body, _ := io.ReadAll(request.Body)
		if string(body) != `{"folder":"legal/vivare"}` {
			t.Fatalf("body = %s", body)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"status":"completed","documents":[{"path":"legal/vivare/evidence.docx","text":"Source-backed evidence.","format":"docx","pageCount":2,"contentDigest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}`))}, nil
	})}
	service := NewService(Config{Enabled: true, RunnerURL: "http://docling-runner:8080", Token: "runner-token-123456", Timeout: time.Minute}, client)
	documents, err := service.Extract(context.Background(), "legal/vivare")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(documents) != 1 || documents[0].Path != "legal/vivare/evidence.docx" {
		t.Fatalf("documents = %#v", documents)
	}
}

func TestExtractRejectsTraversalAndEscapedRunnerMetadata(t *testing.T) {
	service := NewService(Config{Enabled: true, RunnerURL: "http://docling-runner:8080", Token: "runner-token-123456", Timeout: time.Minute}, &http.Client{})
	for _, value := range []string{"", ".", "../outside", "/absolute"} {
		if _, err := service.Extract(context.Background(), value); err == nil {
			t.Fatalf("Extract(%q) succeeded", value)
		}
	}

	client := &http.Client{Transport: roundTrip(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"status":"completed","documents":[{"path":"outside/evidence.docx","text":"Source-backed evidence.","format":"docx","pageCount":1,"contentDigest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}`))}, nil
	})}
	service = NewService(Config{Enabled: true, RunnerURL: "http://docling-runner:8080", Token: "runner-token-123456", Timeout: time.Minute}, client)
	if _, err := service.Extract(context.Background(), "legal/vivare"); !errorsIsUnavailable(err) {
		t.Fatalf("escaped runner metadata error = %v", err)
	}
}

func TestStatusIsHonestWhenDisabled(t *testing.T) {
	status := NewService(Config{}, nil).Status()
	if status.Configured || status.Enabled || !strings.Contains(status.Scope, "Operator-triggered") {
		t.Fatalf("status = %#v", status)
	}
}

func errorsIsUnavailable(err error) bool { return err == ErrUnavailable }
