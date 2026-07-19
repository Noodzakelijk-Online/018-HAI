package mcppreflight

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPreflightUsesHandshakeAndNeverCallsTool(t *testing.T) {
	methods := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "" {
			t.Fatalf("unexpected request: method=%s authorization=%q", r.Method, r.Header.Get("Authorization"))
		}
		var request struct {
			Method string `json:"method"`
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
				t.Fatalf("initialized notification did not use session id")
			}
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			if r.Header.Get("MCP-Session-Id") != "session-1" {
				t.Fatalf("tools/list did not use session id")
			}
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"read_case","title":"Read case","inputSchema":{"type":"object"}},{"name":"token_dump","title":"Secret","inputSchema":{}}]}}`))
		default:
			t.Fatalf("unexpected MCP method %q", request.Method)
		}
	}))
	defer server.Close()

	svc := NewService(Config{Enabled: true, Servers: []Server{{ID: "test", CatalogID: "mcp-inspector", URL: server.URL}}})
	result, found := svc.Preflight(context.Background(), "test")
	if !found || result.Status != "ready" || result.ToolCount != 2 || result.ProtocolVersion != protocolVersion {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.CatalogID != "mcp-inspector" || result.CatalogName != "MCP Inspector" {
		t.Fatalf("preflight result must preserve reviewed catalog provenance: %#v", result)
	}
	if strings.Join(methods, ",") != "initialize,notifications/initialized,tools/list" {
		t.Fatalf("unexpected protocol methods: %v", methods)
	}
	if len(result.Tools) != 2 || result.Tools[0].Name != "[redacted]" || result.Tools[0].Title != "[redacted]" {
		t.Fatalf("tool output was not bounded/redacted: %#v", result.Tools)
	}
	if result.Tools[1].Name != "read_case" || !result.Tools[1].HasInputSchema {
		t.Fatalf("safe tool summary missing: %#v", result.Tools)
	}
	if strings.Contains(strings.ToLower(result.Detail), "token") {
		t.Fatalf("result detail must not echo server payload: %q", result.Detail)
	}
}

func TestPreflightIsFailClosedForDisabledOrUnsafeConfig(t *testing.T) {
	disabled := NewService(Config{Enabled: false, Servers: []Server{{ID: "local", CatalogID: "mcp-inspector", URL: "http://127.0.0.1:3000/mcp"}}})
	result, found := disabled.Preflight(context.Background(), "local")
	if !found || result.Status != "disabled" {
		t.Fatalf("disabled service must not run, got %#v", result)
	}

	unsafe := NewService(Config{Enabled: true, Servers: []Server{{ID: "remote", CatalogID: "mcp-inspector", URL: "https://example.com/mcp"}}})
	if unsafe.Overview().ConfigError == "" {
		t.Fatalf("non-local endpoint must be rejected")
	}
	result, found = unsafe.Preflight(context.Background(), "remote")
	if !found || result.Status != "blocked" {
		t.Fatalf("invalid config must be blocked, got %#v", result)
	}
}

func TestPreflightRequiresAnEligibleMCPCatalogProfile(t *testing.T) {
	for _, server := range []Server{
		{ID: "missing-profile", URL: "http://127.0.0.1:3000/mcp"},
		{ID: "unknown-profile", CatalogID: "not-a-profile", URL: "http://127.0.0.1:3000/mcp"},
		{ID: "non-mcp-profile", CatalogID: "cloudquery", URL: "http://127.0.0.1:3000/mcp"},
	} {
		svc := NewService(Config{Enabled: true, Servers: []Server{server}})
		if svc.Overview().ConfigError == "" {
			t.Fatalf("server %#v must be rejected without a reviewed MCP profile", server)
		}
		result, found := svc.Preflight(context.Background(), server.ID)
		if !found || result.Status != "blocked" {
			t.Fatalf("invalid server must fail closed: %#v", result)
		}
	}

	servers := parseServers("github@github-mcp-server=http://127.0.0.1:3000/mcp")
	if len(servers) != 1 || servers[0].ID != "github" || servers[0].CatalogID != "github-mcp-server" {
		t.Fatalf("profile-aware server parsing failed: %#v", servers)
	}
}

func TestValidateLocalURLRejectsCredentialsAndNonLocalHosts(t *testing.T) {
	for _, raw := range []string{
		"http://user:password@127.0.0.1:8080/mcp",
		"http://169.254.169.254/mcp",
		"http://0.0.0.0/mcp",
		"http://example.com/mcp",
		"http://localhost/mcp?access_token=secret",
	} {
		if err := validateLocalURL(raw); err == nil {
			t.Fatalf("%q must be rejected", raw)
		}
	}
	for _, raw := range []string{"http://localhost:8080/mcp", "http://127.0.0.1:8080/mcp", "http://host.docker.internal:8080/mcp"} {
		if err := validateLocalURL(raw); err != nil {
			t.Fatalf("%q should be allowed: %v", raw, err)
		}
	}
}
