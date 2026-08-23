package chatgptlogs

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchUsesOnlyBoundedReadOnlySearchAndAcceptsSSE(t *testing.T) {
	methods := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/json, text/event-stream" || r.Header.Get("Authorization") != "" {
			t.Fatalf("unsafe headers: accept=%q authorization=%q", r.Header.Get("Accept"), r.Header.Get("Authorization"))
		}
		var request struct {
			ID     int             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		methods = append(methods, request.Method)
		w.Header().Set("Content-Type", "text/event-stream")
		switch request.Method {
		case "initialize":
			w.Header().Set("MCP-Session-Id", "history-session")
			_, _ = w.Write([]byte("event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"protocolVersion\":\"2025-06-18\"}}\n\n"))
		case "notifications/initialized":
			if r.Header.Get("MCP-Session-Id") != "history-session" {
				t.Fatalf("notification session = %q", r.Header.Get("MCP-Session-Id"))
			}
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			_, _ = w.Write([]byte("event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{\"tools\":[{\"name\":\"search\"},{\"name\":\"get_raw\"}]}}\n\n"))
		case "tools/call":
			var params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			if err := json.Unmarshal(request.Params, &params); err != nil {
				t.Fatal(err)
			}
			if params.Name != "search" || params.Arguments["query"] != "why did the build fail" || params.Arguments["project"] != "018-HAI" || params.Arguments["limit"] != float64(5) || params.Arguments["offset"] != float64(0) || params.Arguments["rank_pool"] != float64(200) || params.Arguments["max_chars"] != float64(maxToolTextRunes) {
				t.Fatalf("unexpected tool call: %#v", params)
			}
			_, _ = w.Write([]byte("event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":3,\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"bounded historical context\"}]}}\n\n"))
		default:
			t.Fatalf("unexpected MCP method %q", request.Method)
		}
	}))
	defer server.Close()

	service := NewService(true, server.URL+"/mcp", server.Client())
	items, err := service.Search(context.Background(), SearchRequest{Query: "why did the build fail", ProjectKey: "018-HAI"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if strings.Join(methods, ",") != "initialize,notifications/initialized,tools/list,tools/call" {
		t.Fatalf("methods = %v", methods)
	}
	if len(items) != 1 || items[0].Content != "bounded historical context" || !items[0].Untrusted || items[0].Tool != "search" {
		t.Fatalf("unexpected context: %#v", items)
	}
}

func TestSearchFailsClosedForUnsafeConfigurationAndMissingSearch(t *testing.T) {
	unsafe := NewService(true, "https://example.com/mcp", nil)
	if unsafe.Status().Configured {
		t.Fatalf("remote endpoint must be rejected: %#v", unsafe.Status())
	}
	if _, err := unsafe.Search(context.Background(), SearchRequest{Query: "test"}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("unsafe Search() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "initialize":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18"}}`))
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"get_raw"}]}}`))
		}
	}))
	defer server.Close()
	configured := NewService(true, server.URL+"/mcp", server.Client())
	if _, err := configured.Search(context.Background(), SearchRequest{Query: "test"}); err == nil || !strings.Contains(err.Error(), "requested reviewed read-only tool") {
		t.Fatalf("missing search error = %v", err)
	}
}

func TestSearchRejectsInvalidInputBeforeNetwork(t *testing.T) {
	service := NewService(true, "http://127.0.0.1:8099/mcp", &http.Client{Transport: roundTripper(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid request reached network")
		return nil, nil
	})})
	for _, request := range []SearchRequest{{}, {Query: "nul\x00query"}, {Query: strings.Repeat("x", maxQueryRunes+1)}, {Query: "ok", ProjectKey: strings.Repeat("p", maxProjectRunes+1)}} {
		if _, err := service.Search(context.Background(), request); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("Search(%#v) error = %v", request, err)
		}
	}
}

func TestNormalizeArgumentsAllowsReviewedToolsAndClampsBudgets(t *testing.T) {
	tests := []struct {
		tool string
		raw  string
	}{
		{"list_sources", `{}`},
		{"list_conversations", `{"platform":"codex","project":"018-HAI","limit":200}`},
		{"search", `{"query":"unfinished commitment","roles":["user","assistant"],"order":"recent"}`},
		{"get_conversation", `{"conversation_id":42,"limit":100}`},
		{"get_context", `{"message_id":"message-1","before":50,"after":50}`},
		{"get_message", `{"message_id":99}`},
		{"get_raw", `{"conversation_id":"conversation-1","artifacts":50}`},
		{"sync_status", `{"runs":50,"pending":50,"failures":50}`},
		{"stats", `{"platform":"chatgpt_work","top_projects":50}`},
	}
	for _, test := range tests {
		t.Run(test.tool, func(t *testing.T) {
			arguments, err := normalizeArguments(test.tool, json.RawMessage(test.raw))
			if err != nil {
				t.Fatalf("normalizeArguments() error = %v", err)
			}
			if arguments["max_chars"] != maxToolTextRunes {
				t.Fatalf("max_chars = %#v", arguments["max_chars"])
			}
			for _, key := range []string{"limit", "before", "after", "artifacts", "runs", "pending", "failures", "top_projects"} {
				if value, exists := arguments[key]; exists && value.(int64) > 20 {
					t.Fatalf("%s escaped clamp: %#v", key, value)
				}
			}
		})
	}
}

func TestNormalizeArgumentsRejectsUnknownToolsFieldsAndMissingIdentifiers(t *testing.T) {
	for _, test := range []CallRequest{
		{Tool: "delete_conversation", Arguments: json.RawMessage(`{}`)},
		{Tool: "search", Arguments: json.RawMessage(`{"query":"ok","command":"rm"}`)},
		{Tool: "search", Arguments: json.RawMessage(`{"query":"ok","platform":"unknown"}`)},
		{Tool: "get_message", Arguments: json.RawMessage(`{}`)},
		{Tool: "get_raw", Arguments: json.RawMessage(`{}`)},
	} {
		if _, err := normalizeArguments(test.Tool, test.Arguments); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("normalizeArguments(%#v) error = %v", test, err)
		}
	}
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }
