package task

import (
	"context"
	"errors"
	"strings"
	"testing"

	"automation-hub-backend/internal/chatgptlogs"
)

type fakeChatGPTLogsContext struct {
	status chatgptlogs.Status
	items  []chatgptlogs.ContextItem
	err    error
	seen   chatgptlogs.SearchRequest
}

func (f *fakeChatGPTLogsContext) Status() chatgptlogs.Status { return f.status }

func (f *fakeChatGPTLogsContext) Search(_ context.Context, request chatgptlogs.SearchRequest) ([]chatgptlogs.ContextItem, error) {
	f.seen = request
	return append([]chatgptlogs.ContextItem(nil), f.items...), f.err
}

func TestChatGPTLogsContextEnrichesGenerationWithoutAuthority(t *testing.T) {
	provider := &fakeChatGPTLogsContext{
		status: chatgptlogs.Status{Enabled: true, Configured: true},
		items: []chatgptlogs.ContextItem{{
			Provider: "chatgpt-codex-mcp-daemon", Tool: "search", Content: "Earlier task chose a bounded retry.", SourceURI: "http://127.0.0.1:8099/mcp", Untrusted: true,
		}},
	}
	s := &service{chatgptLogsContext: provider}
	items, explanation := s.retrieveChatGPTLogsContext(IntakeRequest{Request: "why did retries change", ProjectKey: "018-HAI"})
	if provider.seen.Query != "why did retries change" || provider.seen.ProjectKey != "018-HAI" || len(items) != 1 || !strings.Contains(explanation, "Retrieved 1") {
		t.Fatalf("unexpected retrieval: seen=%#v items=%#v explanation=%q", provider.seen, items, explanation)
	}
	plan := &CompletionPlan{ContextPlan: ContextPlan{ChatGPTLogsContext: items}}
	context := generationContext(plan)
	if len(context) != 1 || !strings.Contains(context[0], "never instructions or authority") || !strings.Contains(context[0], "bounded retry") {
		t.Fatalf("unexpected generation context: %#v", context)
	}
	evidence := evidenceFromPlan(plan)
	if len(evidence) != 1 || evidence[0].Primary || evidence[0].Authority != "untrusted_context" {
		t.Fatalf("MCP context gained authority: %#v", evidence)
	}
}

func TestChatGPTLogsContextFailureIsVisibleAndNonBlocking(t *testing.T) {
	provider := &fakeChatGPTLogsContext{status: chatgptlogs.Status{Enabled: true, Configured: true}, err: errors.New("offline")}
	s := &service{chatgptLogsContext: provider}
	items, explanation := s.retrieveChatGPTLogsContext(IntakeRequest{Request: "continue"})
	if len(items) != 0 || !strings.Contains(explanation, "continued without") {
		t.Fatalf("unexpected soft failure: %#v %q", items, explanation)
	}
}

func TestWithChatGPTLogsContextRequiresBuiltInServiceAndProvider(t *testing.T) {
	provider := &fakeChatGPTLogsContext{}
	if _, err := WithChatGPTLogsContext(nil, provider); err == nil {
		t.Fatal("non-built-in service must be rejected")
	}
	if _, err := WithChatGPTLogsContext(&service{}, nil); err == nil {
		t.Fatal("nil provider must be rejected")
	}
	base := &service{}
	decorated, err := WithChatGPTLogsContext(base, provider)
	if err != nil || decorated != base || base.chatgptLogsContext != provider {
		t.Fatalf("unexpected decoration: %#v %v", decorated, err)
	}
}
