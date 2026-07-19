// Package mcppreflight provides a deliberately narrow MCP server review gate.
//
// It can only handshake with explicitly configured local Streamable HTTP MCP
// servers and list their tools. It does not start processes, accept arbitrary
// URLs, retain server responses, or call a tool. That keeps inspection useful
// before a runtime adapter is approved without turning HAI into an unrestricted
// MCP client.
package mcppreflight

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"automation-hub-backend/internal/braincatalog"
)

const (
	serversEnv       = "HAI_MCP_PREFLIGHT_SERVERS"
	enabledEnv       = "HAI_MCP_PREFLIGHT_ENABLED"
	timeoutEnv       = "HAI_MCP_PREFLIGHT_TIMEOUT_SECONDS"
	protocolVersion  = "2025-06-18"
	maxResponseBytes = 1 << 20
	maxTools         = 100
)

var serverIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

// Server is a reviewed MCP endpoint. It intentionally has no auth fields:
// secrets belong in a dedicated adapter after review, never in a listing tool.
type Server struct {
	ID        string `json:"id"`
	CatalogID string `json:"catalogId"`
	URL       string `json:"url"`
}

// Config controls the optional local-only preflight service.
type Config struct {
	Enabled bool
	Servers []Server
	Timeout time.Duration
}

// Tool is the bounded, non-secret portion of a tools/list result. HAI does not
// retain tool schemas or descriptions because those are third-party payloads.
type Tool struct {
	Name           string `json:"name"`
	Title          string `json:"title,omitempty"`
	HasInputSchema bool   `json:"hasInputSchema"`
}

// Result is an auditable result of a read-only preflight. It contains no raw
// MCP response, credentials, headers, or tool arguments.
type Result struct {
	ID              string    `json:"id"`
	ServerID        string    `json:"serverId"`
	CatalogID       string    `json:"catalogId,omitempty"`
	CatalogName     string    `json:"catalogName,omitempty"`
	URL             string    `json:"url,omitempty"`
	Status          string    `json:"status"`
	Detail          string    `json:"detail"`
	ProtocolVersion string    `json:"protocolVersion,omitempty"`
	ToolCount       int       `json:"toolCount"`
	Tools           []Tool    `json:"tools,omitempty"`
	Truncated       bool      `json:"truncated"`
	DurationMs      int64     `json:"durationMs"`
	CheckedAt       time.Time `json:"checkedAt"`
}

// ServerStatus is safe to show in an authenticated operator view.
type ServerStatus struct {
	ID          string  `json:"id"`
	CatalogID   string  `json:"catalogId,omitempty"`
	CatalogName string  `json:"catalogName,omitempty"`
	URL         string  `json:"url,omitempty"`
	Configured  bool    `json:"configured"`
	LastAttempt *Result `json:"lastAttempt,omitempty"`
}

// Overview explains whether the preflight is configured without pretending a
// server was contacted.
type Overview struct {
	Enabled     bool           `json:"enabled"`
	ConfigError string         `json:"configError,omitempty"`
	Scope       string         `json:"scope"`
	Servers     []ServerStatus `json:"servers"`
}

// Service owns the configured review boundary and a bounded in-memory recent
// attempt view. The durable operation/audit ledger remains the place for a
// later, approved execution adapter; preflight itself never executes a tool.
type Service struct {
	config    Config
	configErr string
	client    *http.Client
	now       func() time.Time

	mu       sync.Mutex
	sequence int
	last     map[string]Result
}

// NewService builds a preflight service from explicit config.
func NewService(config Config) *Service {
	if config.Timeout <= 0 {
		config.Timeout = 5 * time.Second
	}
	if config.Timeout > 30*time.Second {
		config.Timeout = 30 * time.Second
	}
	s := &Service{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
			// A configured endpoint must be contacted directly. Following a
			// redirect would defeat the local-endpoint review boundary.
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
			Transport:     &http.Transport{Proxy: nil},
		},
		now:  time.Now,
		last: map[string]Result{},
	}
	if config.Enabled {
		s.configErr = validateConfig(config)
	}
	return s
}

// NewServiceFromEnv builds an optional service. The server format is a
// semicolon-separated list, for example:
// HAI_MCP_PREFLIGHT_SERVERS=local-docs@mcp-inspector=http://host.docker.internal:3001/mcp
func NewServiceFromEnv() *Service {
	timeout := 5 * time.Second
	if raw := strings.TrimSpace(os.Getenv(timeoutEnv)); raw != "" {
		if seconds, err := time.ParseDuration(raw + "s"); err == nil {
			timeout = seconds
		}
	}
	return NewService(Config{
		Enabled: strings.EqualFold(strings.TrimSpace(os.Getenv(enabledEnv)), "true"),
		Servers: parseServers(os.Getenv(serversEnv)),
		Timeout: timeout,
	})
}

// Overview reports the static configuration and last preflight per server.
func (s *Service) Overview() Overview {
	s.mu.Lock()
	defer s.mu.Unlock()
	servers := make([]ServerStatus, 0, len(s.config.Servers))
	for _, server := range s.config.Servers {
		item := ServerStatus{ID: server.ID, CatalogID: server.CatalogID, CatalogName: catalogName(server.CatalogID), URL: server.URL, Configured: s.config.Enabled && s.configErr == ""}
		if attempt, ok := s.last[server.ID]; ok {
			copy := attempt
			item.LastAttempt = &copy
		}
		servers = append(servers, item)
	}
	return Overview{
		Enabled:     s.config.Enabled,
		ConfigError: s.configErr,
		Scope:       "Read-only Streamable HTTP preflight: initialize and tools/list only. HAI never starts an MCP process or calls a listed tool.",
		Servers:     servers,
	}
}

// Preflight performs initialize, initialized notification, and tools/list for
// an explicitly configured local endpoint. A successful result is evidence of
// reachability and declared tools only; it is never execution approval.
func (s *Service) Preflight(ctx context.Context, serverID string) (Result, bool) {
	server, ok := s.server(serverID)
	if !ok {
		return Result{}, false
	}
	start := time.Now()
	result := Result{
		ServerID:    server.ID,
		CatalogID:   server.CatalogID,
		CatalogName: catalogName(server.CatalogID),
		URL:         server.URL,
		CheckedAt:   s.now().UTC(),
	}
	if !s.config.Enabled {
		result.Status = "disabled"
		result.Detail = enabledEnv + " is false"
		return s.record(result, start), true
	}
	if s.configErr != "" {
		result.Status = "blocked"
		result.Detail = "preflight configuration is invalid: " + s.configErr
		return s.record(result, start), true
	}

	init, sessionID, err := s.rpc(ctx, server, "initialize", 1, map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]string{
			"name":    "HAI MCP preflight",
			"version": "1.0.0",
		},
	}, "")
	if err != nil {
		result.Status = "failed"
		result.Detail = safeError("initialize", err)
		return s.record(result, start), true
	}
	result.ProtocolVersion = protocolFromResult(init.Result)
	if result.ProtocolVersion == "" {
		result.ProtocolVersion = protocolVersion
	}
	if err := s.notifyInitialized(ctx, server, sessionID, result.ProtocolVersion); err != nil {
		result.Status = "failed"
		result.Detail = safeError("initialized notification", err)
		return s.record(result, start), true
	}

	toolsResponse, _, err := s.rpc(ctx, server, "tools/list", 2, map[string]any{}, sessionID)
	if err != nil {
		result.Status = "failed"
		result.Detail = safeError("tools/list", err)
		return s.record(result, start), true
	}
	tools, count, truncated, err := boundedTools(toolsResponse.Result)
	if err != nil {
		result.Status = "failed"
		result.Detail = "tools/list returned an invalid result"
		return s.record(result, start), true
	}
	result.Status = "ready"
	result.Detail = "MCP handshake and tool listing completed; no tool was called and no runtime was enabled"
	result.Tools = tools
	result.ToolCount = count
	result.Truncated = truncated
	return s.record(result, start), true
}

func (s *Service) server(id string) (Server, bool) {
	for _, server := range s.config.Servers {
		if server.ID == id {
			return server, true
		}
	}
	return Server{}, false
}

func (s *Service) record(result Result, start time.Time) Result {
	result.DurationMs = time.Since(start).Milliseconds()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sequence++
	result.ID = fmt.Sprintf("mcp-preflight-%d", s.sequence)
	s.last[result.ServerID] = result
	return result
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

type rpcError struct {
	Code int `json:"code"`
}

func (s *Service) rpc(ctx context.Context, server Server, method string, id int, params any, sessionID string) (rpcResponse, string, error) {
	payload := map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}
	body, err := json.Marshal(payload)
	if err != nil {
		return rpcResponse{}, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, bytes.NewReader(body))
	if err != nil {
		return rpcResponse{}, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("MCP-Protocol-Version", protocolVersion)
	req.Header.Set("User-Agent", "HAI-MCP-Preflight/1.0")
	if sessionID != "" {
		req.Header.Set("MCP-Session-Id", sessionID)
	}
	response, err := s.client.Do(req)
	if err != nil {
		return rpcResponse{}, "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return rpcResponse{}, "", fmt.Errorf("HTTP %d", response.StatusCode)
	}
	decoded, err := decodeResponse(response.Body)
	if err != nil {
		return rpcResponse{}, "", err
	}
	if decoded.Error != nil {
		return rpcResponse{}, "", fmt.Errorf("MCP error %d", decoded.Error.Code)
	}
	if len(decoded.Result) == 0 || string(decoded.Result) == "null" {
		return rpcResponse{}, "", fmt.Errorf("MCP result is empty")
	}
	return decoded, strings.TrimSpace(response.Header.Get("MCP-Session-Id")), nil
}

func (s *Service) notifyInitialized(ctx context.Context, server Server, sessionID, version string) error {
	payload := map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized", "params": map[string]any{}}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("MCP-Protocol-Version", version)
	req.Header.Set("User-Agent", "HAI-MCP-Preflight/1.0")
	if sessionID != "" {
		req.Header.Set("MCP-Session-Id", sessionID)
	}
	response, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", response.StatusCode)
	}
	return nil
}

func decodeResponse(body io.Reader) (rpcResponse, error) {
	limited := io.LimitReader(body, maxResponseBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return rpcResponse{}, err
	}
	if len(data) > maxResponseBytes {
		return rpcResponse{}, fmt.Errorf("response exceeds %d byte limit", maxResponseBytes)
	}
	var decoded rpcResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		return rpcResponse{}, fmt.Errorf("response is not JSON")
	}
	if decoded.JSONRPC != "2.0" {
		return rpcResponse{}, fmt.Errorf("response does not use JSON-RPC 2.0")
	}
	return decoded, nil
}

func protocolFromResult(raw json.RawMessage) string {
	var result struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return ""
	}
	return strings.TrimSpace(result.ProtocolVersion)
}

func boundedTools(raw json.RawMessage) ([]Tool, int, bool, error) {
	var result struct {
		Tools []struct {
			Name        string          `json:"name"`
			Title       string          `json:"title"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, 0, false, err
	}
	count := len(result.Tools)
	truncated := count > maxTools
	if truncated {
		result.Tools = result.Tools[:maxTools]
	}
	tools := make([]Tool, 0, len(result.Tools))
	for _, item := range result.Tools {
		name := redactDisplay(strings.TrimSpace(item.Name))
		if name == "" {
			name = "redacted-tool"
		}
		tools = append(tools, Tool{
			Name:           truncate(name, 128),
			Title:          truncate(redactDisplay(strings.TrimSpace(item.Title)), 160),
			HasInputSchema: len(item.InputSchema) > 0 && string(item.InputSchema) != "null",
		})
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools, count, truncated, nil
}

func safeError(stage string, err error) string {
	message := strings.TrimSpace(err.Error())
	if strings.HasPrefix(message, "HTTP ") || strings.HasPrefix(message, "MCP error") || strings.HasPrefix(message, "response ") {
		return stage + " failed: " + message
	}
	return stage + " failed; endpoint did not provide a usable response"
}

func redactDisplay(value string) string {
	lower := strings.ToLower(value)
	for _, marker := range []string{"password", "secret", "token", "authorization", "api key", "apikey"} {
		if strings.Contains(lower, marker) {
			return "[redacted]"
		}
	}
	return value
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}

func parseServers(raw string) []Server {
	parts := strings.Split(raw, ";")
	servers := make([]Server, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		profile, endpoint, ok := strings.Cut(part, "=")
		if !ok {
			servers = append(servers, Server{ID: part})
			continue
		}
		id, catalogID, hasCatalogID := strings.Cut(strings.TrimSpace(profile), "@")
		server := Server{ID: strings.TrimSpace(id), URL: strings.TrimSpace(endpoint)}
		if hasCatalogID {
			server.CatalogID = strings.TrimSpace(catalogID)
		}
		servers = append(servers, server)
	}
	return servers
}

func validateConfig(config Config) string {
	if len(config.Servers) == 0 {
		return serversEnv + " must contain at least one reviewed local server when preflight is enabled"
	}
	seen := map[string]bool{}
	for _, server := range config.Servers {
		if !serverIDPattern.MatchString(server.ID) {
			return "server id must use letters, digits, hyphen, or underscore"
		}
		if seen[server.ID] {
			return "server ids must be unique"
		}
		seen[server.ID] = true
		if err := validateCatalogProfile(server.CatalogID); err != nil {
			return "server " + server.ID + ": " + err.Error()
		}
		if err := validateLocalURL(server.URL); err != nil {
			return "server " + server.ID + ": " + err.Error()
		}
	}
	return ""
}

func validateCatalogProfile(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("catalog profile is required")
	}
	entry, ok := braincatalog.EntryByID(id)
	if !ok {
		return fmt.Errorf("catalog profile is not reviewed")
	}
	if entry.Status != braincatalog.StatusIntegrated && entry.Status != braincatalog.StatusCandidate && entry.Status != braincatalog.StatusCompatibility {
		return fmt.Errorf("catalog profile is not eligible for MCP preflight")
	}
	if entry.SourceCollection != "MCP Servers" && entry.ID != "mcp-inspector" {
		return fmt.Errorf("catalog profile is not an MCP capability")
	}
	return nil
}

func catalogName(id string) string {
	entry, ok := braincatalog.EntryByID(strings.TrimSpace(id))
	if !ok {
		return ""
	}
	return entry.Name
}

func validateLocalURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL scheme must be http/https")
	}
	if u.User != nil {
		return fmt.Errorf("URL credentials are not allowed")
	}
	if u.Hostname() == "" {
		return fmt.Errorf("URL host is empty")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("URL query and fragment are not allowed")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "localhost" || host == "host.docker.internal" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("only localhost, loopback IPs, or host.docker.internal are allowed")
}
