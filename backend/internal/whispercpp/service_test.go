package whispercpp

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

func TestTranscribeUsesOnlyConfiguredLocalFolder(t *testing.T) {
	client := &http.Client{Transport: roundTrip(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/v1/transcribe" {
			t.Fatalf("path = %s", request.URL.Path)
		}
		body, _ := io.ReadAll(request.Body)
		if string(body) != `{"folder":"voice-notes"}` {
			t.Fatalf("body = %s", body)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"status":"completed","transcripts":[{"path":"voice-notes/meeting.m4a","text":"Need a follow-up.","modelId":"ggml-base.en.bin","language":"en"}]}`)), Header: make(http.Header)}, nil
	})}
	service := NewService(true, "http://whispercpp-runner:8080", time.Minute, client)
	transcripts, err := service.Transcribe(context.Background(), "voice-notes")
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if len(transcripts) != 1 || transcripts[0].Path != "voice-notes/meeting.m4a" {
		t.Fatalf("transcripts = %#v", transcripts)
	}
}

func TestTranscribeRejectsRootAndTraversalFolders(t *testing.T) {
	service := NewService(true, "http://whispercpp-runner:8080", time.Minute, &http.Client{})
	for _, value := range []string{"", ".", "../outside", "/absolute"} {
		if _, err := service.Transcribe(context.Background(), value); err == nil {
			t.Fatalf("Transcribe(%q) succeeded", value)
		}
	}
}

func TestStatusIsHonestWhenDisabled(t *testing.T) {
	status := NewService(false, "", time.Minute, nil).Status()
	if status.Configured || status.Enabled || !strings.Contains(status.Scope, "Operator-triggered") {
		t.Fatalf("status = %#v", status)
	}
}
