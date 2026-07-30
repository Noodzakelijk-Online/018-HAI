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
	if svc := NewService(Config{Enabled: true, Servers: []Server{{ID: "serena", CatalogID: "serena", URL: "http://127.0.0.1:3000/mcp"}}}); svc.Overview().ConfigError != "" {
		t.Fatalf("reviewed read-only Serena MCP profile must be preflight eligible: %q", svc.Overview().ConfigError)
	}
	if svc := NewService(Config{Enabled: true, Servers: []Server{{ID: "toolbox", CatalogID: "google-genai-toolbox", URL: "http://127.0.0.1:5000/mcp/approved_readonly_toolset"}}}); svc.Overview().ConfigError != "" {
		t.Fatalf("integrated MCP Toolbox inspection profile must be preflight eligible: %q", svc.Overview().ConfigError)
	}
}

func TestGitHubReadOnlyToolAllowlist(t *testing.T) {
	allowed := []Tool{
		{Name: "get_file_contents"},
		{Name: "issue_read"},
		{Name: "list_pull_requests"},
		{Name: "search_code"},
	}
	if violations := githubReadOnlyToolViolations(allowed); len(violations) != 0 {
		t.Fatalf("read-only tools must be allowed: %v", violations)
	}

	violations := githubReadOnlyToolViolations([]Tool{
		{Name: "create_pull_request"},
		{Name: "update_issue"},
		{Name: "get_repository"},
	})
	if got, want := strings.Join(violations, ","), "create_pull_request,update_issue"; got != want {
		t.Fatalf("violations = %q, want %q", got, want)
	}
}

func TestGitHubAllowlistChecksToolsBeyondTheDisplayLimit(t *testing.T) {
	declared := make([]map[string]string, 0, maxTools+1)
	for index := 0; index < maxTools; index++ {
		declared = append(declared, map[string]string{"name": "get_repository"})
	}
	declared = append(declared, map[string]string{"name": "create_issue"})
	raw, err := json.Marshal(map[string]any{"tools": declared})
	if err != nil {
		t.Fatalf("marshal tools: %v", err)
	}
	tools, names, count, truncated, err := boundedTools(raw)
	if err != nil || count != maxTools+1 || !truncated || len(tools) != maxTools || len(names) != maxTools+1 {
		t.Fatalf("bounded inventory = tools=%d names=%d count=%d truncated=%t err=%v", len(tools), len(names), count, truncated, err)
	}
	if got, want := strings.Join(githubReadOnlyToolNameViolations(names), ","), "create_issue"; got != want {
		t.Fatalf("tail violation = %q, want %q", got, want)
	}
}

func TestPlaywrightReadOnlyToolContractExcludesInteractiveInventory(t *testing.T) {
	allowed, name, found := readOnlyToolContract("playwright-mcp")
	if !found || name != "Playwright MCP" {
		t.Fatalf("Playwright contract = found=%t name=%q", found, name)
	}
	if violations := readOnlyToolNameViolations([]string{"browser_snapshot", "browser_find", "browser_console_messages"}, allowed); len(violations) != 0 {
		t.Fatalf("inspection tools must be allowed: %v", violations)
	}
	if got, want := strings.Join(readOnlyToolNameViolations([]string{"browser_click", "browser_file_upload", "browser_run_code_unsafe"}, allowed), ","), "browser_click,browser_file_upload,browser_run_code_unsafe"; got != want {
		t.Fatalf("interactive violations = %q, want %q", got, want)
	}
}

func TestGitHubPreflightBlocksNonReadOnlyInventoryWithoutCallingTools(t *testing.T) {
	methods := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			w.Header().Set("MCP-Session-Id", "github-session")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18"}}`))
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"get_file_contents"},{"name":"create_pull_request"}]}}`))
		default:
			t.Fatalf("preflight must not call %q", request.Method)
		}
	}))
	defer server.Close()

	svc := NewService(Config{Enabled: true, Servers: []Server{{ID: "github", CatalogID: "github-mcp-server", URL: server.URL}}})
	result, found := svc.Preflight(context.Background(), "github")
	if !found || result.Status != "blocked" || result.ReadOnlyVerified {
		t.Fatalf("GitHub inventory with write tool must be blocked: %#v", result)
	}
	if got, want := strings.Join(methods, ","), "initialize,notifications/initialized,tools/list"; got != want {
		t.Fatalf("preflight methods = %q, want %q", got, want)
	}
}

func TestGitHubPreflightAcceptsOnlyReviewedReadOnlyInventory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "initialize":
			w.Header().Set("MCP-Session-Id", "github-session")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18"}}`))
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"get_file_contents"},{"name":"issue_read"},{"name":"list_commits"},{"name":"search_code"}]}}`))
		default:
			t.Fatalf("unexpected MCP method %q", request.Method)
		}
	}))
	defer server.Close()

	svc := NewService(Config{Enabled: true, Servers: []Server{{ID: "github", CatalogID: "github-mcp-server", URL: server.URL}}})
	result, found := svc.Preflight(context.Background(), "github")
	if !found || result.Status != "ready" || !result.ReadOnlyVerified || result.ToolCount != 4 {
		t.Fatalf("reviewed GitHub inventory must be ready: %#v", result)
	}
}

func TestPlaywrightPreflightBlocksInteractiveInventoryWithoutCallingTools(t *testing.T) {
	methods := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			w.Header().Set("MCP-Session-Id", "playwright-session")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18"}}`))
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"browser_snapshot"},{"name":"browser_click"}]}}`))
		default:
			t.Fatalf("preflight must not call %q", request.Method)
		}
	}))
	defer server.Close()

	svc := NewService(Config{Enabled: true, Servers: []Server{{ID: "playwright", CatalogID: "playwright-mcp", URL: server.URL}}})
	result, found := svc.Preflight(context.Background(), "playwright")
	if !found || result.Status != "blocked" || result.ReadOnlyVerified {
		t.Fatalf("interactive Playwright inventory must be blocked: %#v", result)
	}
	if got, want := strings.Join(methods, ","), "initialize,notifications/initialized,tools/list"; got != want {
		t.Fatalf("preflight methods = %q, want %q", got, want)
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

func TestPreflightFailsClosedForProtocolDowngradeOrMismatchedResponseID(t *testing.T) {
	for _, reply := range []string{
		`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2024-11-05"}}`,
		`{"jsonrpc":"2.0","id":99,"result":{"protocolVersion":"2025-06-18"}}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(reply))
		}))
		svc := NewService(Config{Enabled: true, Servers: []Server{{ID: "test", CatalogID: "mcp-inspector", URL: server.URL}}})
		result, found := svc.Preflight(context.Background(), "test")
		server.Close()
		if !found || result.Status != "failed" || !strings.Contains(result.Detail, "initialize") {
			t.Fatalf("preflight must reject reply %s, got %#v", reply, result)
		}
	}
}
