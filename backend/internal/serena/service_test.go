package serena

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFindSymbolsUsesOnlyReadOnlyBoundedArguments(t *testing.T) {
	methods := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     int             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "" {
			t.Fatalf("unexpected request %s with authorization %q", r.Method, r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		methods = append(methods, request.Method)
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "initialize":
			w.Header().Set("MCP-Session-Id", "session-1")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18"}}`))
		case "notifications/initialized":
			if r.Header.Get("MCP-Session-Id") != "session-1" {
				t.Fatalf("notification session = %q", r.Header.Get("MCP-Session-Id"))
			}
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"find_symbol"},{"name":"execute_shell_command"},{"name":"replace_symbol_body"}]}}`))
		case "tools/call":
			var params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			if err := json.Unmarshal(request.Params, &params); err != nil {
				t.Fatalf("decode tool call: %v", err)
			}
			if params.Name != findSymbolTool {
				t.Fatalf("tool = %q, want %q", params.Name, findSymbolTool)
			}
			if params.Arguments["name_path_pattern"] != "Service/Find" || params.Arguments["relative_path"] != "backend/internal/serena/service.go" || params.Arguments["include_body"] != false || params.Arguments["include_info"] != false || params.Arguments["substring_matching"] != true || params.Arguments["max_matches"] != float64(maxSymbols) || params.Arguments["max_answer_chars"] != float64(8192) {
				t.Fatalf("unsafe or unexpected arguments: %#v", params.Arguments)
			}
			if _, found := params.Arguments["command"]; found {
				t.Fatalf("unexpected execution argument: %#v", params.Arguments)
			}
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"[{\"name_path\":\"Service/Find\",\"relative_path\":\"backend/internal/serena/service.go\",\"kind\":\"Method\",\"body_location\":{\"start_line\":123,\"end_line\":140},\"body\":\"must never escape\"}]"}]}}`))
		default:
			t.Fatalf("unexpected MCP method %q", request.Method)
		}
	}))
	defer server.Close()

	service := NewService(true, server.URL+"/mcp", "018-HAI", server.Client())
	result, err := service.FindSymbols(context.Background(), SymbolRequest{Pattern: "Service/Find", RelativePath: "backend\\internal/serena/service.go"})
	if err != nil {
		t.Fatalf("FindSymbols() error = %v", err)
	}
	if strings.Join(methods, ",") != "initialize,notifications/initialized,tools/list,tools/call" {
		t.Fatalf("methods = %v", methods)
	}
	if len(result.Symbols) != 1 || result.Symbols[0].NamePath != "Service/Find" || result.Symbols[0].RelativePath != "backend/internal/serena/service.go" || result.Symbols[0].Kind != "Method" || result.Symbols[0].StartLine != 123 || result.Symbols[0].EndLine != 140 {
		t.Fatalf("unexpected bounded symbols: %#v", result.Symbols)
	}
	if strings.Contains(fmtSymbols(result.Symbols), "must never escape") {
		t.Fatalf("source body leaked through symbol metadata: %#v", result.Symbols)
	}
}

func TestProbeNeverCallsAToolAndRequiresFindSymbol(t *testing.T) {
	methods := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		methods = append(methods, request.Method)
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "initialize":
			w.Header().Set("MCP-Session-Id", "session-2")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18"}}`))
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"find_symbol"}]}}`))
		default:
			t.Fatalf("probe must not call %q", request.Method)
		}
	}))
	defer server.Close()

	service := NewService(true, server.URL+"/mcp", "018-HAI", server.Client())
	result, err := service.Probe(context.Background())
	if err != nil || !result.Reachable || !result.ToolAvailable {
		t.Fatalf("Probe() = %#v, %v", result, err)
	}
	if strings.Join(methods, ",") != "initialize,notifications/initialized,tools/list" {
		t.Fatalf("probe methods = %v", methods)
	}
}

func TestServiceFailsClosedForUnsafeOrIncompleteConfiguration(t *testing.T) {
	for _, test := range []struct {
		name    string
		baseURL string
		project string
	}{
		{name: "remote", baseURL: "https://example.com/mcp", project: "018-HAI"},
		{name: "credential", baseURL: "http://token@127.0.0.1:9121/mcp", project: "018-HAI"},
		{name: "missing mcp path", baseURL: "http://127.0.0.1:9121", project: "018-HAI"},
		{name: "missing project", baseURL: "http://127.0.0.1:9121/mcp", project: ""},
		{name: "project path", baseURL: "http://127.0.0.1:9121/mcp", project: "../private"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := NewService(true, test.baseURL, test.project, nil)
			if service.Status().Configured {
				t.Fatalf("unsafe configuration must be blocked: %#v", service.Status())
			}
			if _, err := service.FindSymbols(context.Background(), SymbolRequest{Pattern: "Find"}); !errors.Is(err, ErrNotConfigured) {
				t.Fatalf("FindSymbols() error = %v, want ErrNotConfigured", err)
			}
		})
	}
}

func TestFindSymbolsRejectsTraversalAndOversizedInputsBeforeNetwork(t *testing.T) {
	service := NewService(true, "http://127.0.0.1:9121/mcp", "018-HAI", &http.Client{Transport: roundTripper(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid request reached network")
		return nil, nil
	})})
	for _, request := range []SymbolRequest{
		{Pattern: ""},
		{Pattern: "line\nbreak"},
		{Pattern: strings.Repeat("x", maxPatternLength+1)},
		{Pattern: "Find", RelativePath: "../secrets"},
		{Pattern: "Find", RelativePath: "/absolute.go"},
		{Pattern: "Find", RelativePath: "C:/absolute.go"},
	} {
		if _, err := service.FindSymbols(context.Background(), request); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("FindSymbols(%#v) error = %v, want ErrInvalidRequest", request, err)
		}
	}
}

func TestProbeRejectsMissingReadOnlyTool(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "initialize":
			w.Header().Set("MCP-Session-Id", "session-3")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18"}}`))
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"execute_shell_command"}]}}`))
		}
	}))
	defer server.Close()
	service := NewService(true, server.URL+"/mcp", "018-HAI", server.Client())
	if _, err := service.Probe(context.Background()); err == nil {
		t.Fatal("Probe() must reject a server without the required read-only tool")
	}
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func fmtSymbols(items []Symbol) string {
	data, _ := json.Marshal(items)
	return string(data)
}
