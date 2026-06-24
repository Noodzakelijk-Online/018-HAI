package agentruntime

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRegistryRequiresApproval(t *testing.T) {
	adapter := &fakeAdapter{info: Info{
		ID:               "test",
		Enabled:          true,
		Configured:       true,
		ExecutionEnabled: true,
		RequiresApproval: true,
	}}
	registry := NewRegistry(adapter)
	result := registry.Execute(context.Background(), "test", Task{Prompt: "do work"})
	if result.Status != "blocked" || adapter.called {
		t.Fatalf("unapproved task was executed: %#v", result)
	}
}

func TestRegistryExecutesApprovedTask(t *testing.T) {
	adapter := &fakeAdapter{info: Info{
		ID:               "test",
		Enabled:          true,
		Configured:       true,
		ExecutionEnabled: true,
		RequiresApproval: true,
	}}
	registry := NewRegistry(adapter)
	result := registry.Execute(context.Background(), "test", Task{Prompt: "do work", HumanApproved: true})
	if result.Status != "completed" || !adapter.called {
		t.Fatalf("approved task was not executed: %#v", result)
	}
}

func TestOdysseusAdapterUsesAgentModeWithoutBash(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/codex/capabilities" {
			_, _ = w.Write([]byte(`{"integration":"codex"}`))
			return
		}
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"delta\":\"completed safely\"}\n\ndata: [DONE]\n\n"))
	}))
	defer server.Close()

	adapter := &odysseusAdapter{
		enabled:     true,
		baseURL:     server.URL,
		token:       "test-token",
		sessionID:   "session-1",
		timeout:     5 * time.Second,
		outputLimit: defaultOutputLimit,
		allowedHost: map[string]bool{"127.0.0.1": true},
	}
	result := adapter.ExecuteTask(context.Background(), Task{Prompt: "inspect the task", HumanApproved: true})
	if result.Status != "completed" || result.Output != "completed safely" {
		t.Fatalf("result = %#v", result)
	}
	for _, expected := range []string{"mode=agent", "allow_bash=false", "allow_web_search=false"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("request body %q missing %q", body, expected)
		}
	}
}

func TestOdysseusStreamRejectsTruncatedOutput(t *testing.T) {
	if _, err := readOdysseusStream(strings.NewReader("data: {\"delta\":\"partial\"}\n\n"), 4096); err == nil {
		t.Fatalf("expected incomplete stream to be rejected")
	}
}

func TestHermesWorkspaceMustStayInsideRuntimeRoot(t *testing.T) {
	root := t.TempDir()
	adapter := &hermesAdapter{
		workspaceRoot: root,
		workspace:     root + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "outside",
	}
	if reason := adapter.workspaceBlockedReason(); reason == "" {
		t.Fatalf("expected workspace escape to be rejected")
	}
}

type fakeAdapter struct {
	info   Info
	called bool
}

func (a *fakeAdapter) Info() Info {
	return a.info
}

func (a *fakeAdapter) HealthCheck(context.Context) Health {
	return Health{RuntimeID: a.info.ID, Status: "ready"}
}

func (a *fakeAdapter) ExecuteTask(context.Context, Task) Result {
	a.called = true
	return Result{RuntimeID: a.info.ID, Status: "completed"}
}
