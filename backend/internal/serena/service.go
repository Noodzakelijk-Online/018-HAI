// Package serena provides a deliberately narrow bridge to an operator-run
// Serena MCP server. It exposes semantic symbol metadata only; HAI never
// starts Serena, changes its active project, or calls its editing, shell, file,
// memory, project-management, or network tools.
package serena

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

const (
	enabledEnv                  = "HAI_SERENA_ENABLED"
	baseURLEnv                  = "HAI_SERENA_BASE_URL"
	projectIDEnv                = "HAI_SERENA_PROJECT_ID"
	protocolVersion             = "2025-06-18"
	findSymbolTool              = "find_symbol"
	maxPatternLength            = 160
	maxRelativePathLength       = 240
	maxSymbols                  = 10
	maxResponseBytes      int64 = 128 << 10
)

var (
	ErrNotConfigured  = errors.New("local Serena semantic retrieval is not configured")
	ErrInvalidRequest = errors.New("invalid Serena symbol request")
)

type Status struct {
	Enabled      bool     `json:"enabled"`
	Configured   bool     `json:"configured"`
	Provider     string   `json:"provider"`
	Endpoint     string   `json:"endpoint,omitempty"`
	ProjectID    string   `json:"projectId,omitempty"`
	ConfigError  string   `json:"configError,omitempty"`
	Capabilities []string `json:"capabilities"`
	Restrictions []string `json:"restrictions"`
	Scope        string   `json:"scope"`
}

type SymbolRequest struct {
	Pattern      string `json:"pattern"`
	RelativePath string `json:"relativePath,omitempty"`
}

type Symbol struct {
	NamePath     string `json:"namePath"`
	RelativePath string `json:"relativePath,omitempty"`
	Kind         string `json:"kind,omitempty"`
	StartLine    int    `json:"startLine,omitempty"`
	EndLine      int    `json:"endLine,omitempty"`
}

type SymbolResponse struct {
	Pattern      string   `json:"pattern"`
	RelativePath string   `json:"relativePath,omitempty"`
	Symbols      []Symbol `json:"symbols"`
	Scope        string   `json:"scope"`
}

type ProbeResult struct {
	Reachable     bool      `json:"reachable"`
	ToolAvailable bool      `json:"toolAvailable"`
	CheckedAt     time.Time `json:"checkedAt"`
	Scope         string    `json:"scope"`
}

type Service interface {
	Status() Status
	Probe(context.Context) (*ProbeResult, error)
	FindSymbols(context.Context, SymbolRequest) (*SymbolResponse, error)
}

type service struct {
	enabled   bool
	baseURL   *url.URL
	projectID string
	configErr string
	client    *http.Client
	now       func() time.Time
}

func DefaultService() Service {
	return NewService(
		strings.EqualFold(strings.TrimSpace(os.Getenv(enabledEnv)), "true"),
		os.Getenv(baseURLEnv),
		os.Getenv(projectIDEnv),
		nil,
	)
}

func NewService(enabled bool, rawBaseURL, rawProjectID string, client *http.Client) Service {
	s := &service{enabled: enabled, projectID: strings.TrimSpace(rawProjectID), now: time.Now}
	if client == nil {
		client = &http.Client{
			Timeout:       8 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
			Transport:     &http.Transport{Proxy: nil},
		}
	}
	s.client = client
	if enabled {
		s.baseURL, s.configErr = parseLocalBaseURL(rawBaseURL)
		if s.configErr == "" {
			s.projectID, s.configErr = parseProjectID(rawProjectID)
		}
	}
	return s
}

func (s *service) Status() Status {
	status := Status{
		Enabled:     s.enabled,
		Configured:  s.configured(),
		Provider:    "Serena semantic code context",
		ProjectID:   s.projectID,
		ConfigError: s.configErr,
		Capabilities: []string{
			"bounded semantic symbol lookup",
			"local MCP endpoint and allowlist probe",
		},
		Restrictions: []string{
			"no process launch or project activation",
			"no source-body, hover, shell, file, edit, memory, diagnostics, or cross-project calls",
			"no persistence, task, workflow, provider, policy, approval, or execution updates",
		},
		Scope: "Operator-configured, loopback Serena MCP only. HAI calls only find_symbol with source body and hover information disabled, returns bounded symbol metadata, and treats it as read-only code-review context rather than verified completion evidence.",
	}
	if s.baseURL != nil {
		status.Endpoint = s.baseURL.String()
	}
	return status
}

func (s *service) Probe(ctx context.Context) (*ProbeResult, error) {
	if !s.configured() {
		return nil, ErrNotConfigured
	}
	session, tools, err := s.sessionAndTools(ctx)
	_ = session
	if err != nil {
		return nil, err
	}
	if !hasTool(tools, findSymbolTool) {
		return nil, fmt.Errorf("local Serena endpoint does not expose the required read-only symbol tool")
	}
	return &ProbeResult{
		Reachable:     true,
		ToolAvailable: true,
		CheckedAt:     s.now().UTC(),
		Scope:         "MCP handshake and declared find_symbol availability only. The probe does not activate a project, call a Serena tool, validate language-server results, or establish code-change authority.",
	}, nil
}

func (s *service) FindSymbols(ctx context.Context, input SymbolRequest) (*SymbolResponse, error) {
	if !s.configured() {
		return nil, ErrNotConfigured
	}
	pattern, relativePath, err := validateRequest(input)
	if err != nil {
		return nil, err
	}
	session, tools, err := s.sessionAndTools(ctx)
	if err != nil {
		return nil, err
	}
	if !hasTool(tools, findSymbolTool) {
		return nil, fmt.Errorf("local Serena endpoint does not expose the required read-only symbol tool")
	}
	result, err := s.call(ctx, session, 3, "tools/call", map[string]any{
		"name": findSymbolTool,
		"arguments": map[string]any{
			"name_path_pattern":  pattern,
			"depth":              0,
			"relative_path":      relativePath,
			"include_body":       false,
			"include_info":       false,
			"substring_matching": true,
			"max_matches":        maxSymbols,
			"max_answer_chars":   8192,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("local Serena symbol lookup failed")
	}
	text, err := toolText(result.Result)
	if err != nil {
		return nil, fmt.Errorf("local Serena symbol lookup did not return usable read-only metadata")
	}
	return &SymbolResponse{
		Pattern:      pattern,
		RelativePath: relativePath,
		Symbols:      extractSymbols(text),
		Scope:        s.Status().Scope,
	}, nil
}

func (s *service) configured() bool {
	return s.enabled && s.baseURL != nil && s.configErr == "" && s.projectID != ""
}

type mcpSession struct{ id string }

type mcpTool struct {
	Name string `json:"name"`
}

func (s *service) sessionAndTools(ctx context.Context) (mcpSession, []mcpTool, error) {
	init, sessionID, err := s.callWithSession(ctx, "", 1, "initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]string{"name": "HAI Serena read-only bridge", "version": "1.0"},
	})
	if err != nil || !matchesID(init.ID, 1) || protocolFromResult(init.Result) != protocolVersion || sessionID == "" {
		return mcpSession{}, nil, fmt.Errorf("local Serena MCP initialize failed")
	}
	session := mcpSession{id: sessionID}
	if _, _, err := s.callWithSession(ctx, session.id, 0, "notifications/initialized", map[string]any{}); err != nil {
		return mcpSession{}, nil, fmt.Errorf("local Serena MCP initialization notification failed")
	}
	listed, err := s.call(ctx, session, 2, "tools/list", map[string]any{})
	if err != nil || !matchesID(listed.ID, 2) {
		return mcpSession{}, nil, fmt.Errorf("local Serena MCP tool inventory failed")
	}
	var payload struct {
		Tools []mcpTool `json:"tools"`
	}
	if err := json.Unmarshal(listed.Result, &payload); err != nil {
		return mcpSession{}, nil, fmt.Errorf("local Serena MCP tool inventory was invalid")
	}
	return session, payload.Tools, nil
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (s *service) call(ctx context.Context, session mcpSession, id int, method string, params any) (rpcResponse, error) {
	response, _, err := s.callWithSession(ctx, session.id, id, method, params)
	return response, err
}

func (s *service) callWithSession(ctx context.Context, sessionID string, id int, method string, params any) (rpcResponse, string, error) {
	payload := map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
	if id > 0 {
		payload["id"] = id
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return rpcResponse{}, "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL.String(), bytes.NewReader(body))
	if err != nil {
		return rpcResponse{}, "", err
	}
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "HAI-Serena-ReadOnly/1.0")
	if sessionID != "" {
		request.Header.Set("MCP-Session-Id", sessionID)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return rpcResponse{}, "", err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return rpcResponse{}, "", fmt.Errorf("HTTP %d", response.StatusCode)
	}
	if id == 0 {
		_, err = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes))
		return rpcResponse{}, "", err
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || int64(len(data)) > maxResponseBytes {
		return rpcResponse{}, "", fmt.Errorf("response exceeded limit or could not be read")
	}
	var decoded rpcResponse
	if err := json.Unmarshal(data, &decoded); err != nil || decoded.JSONRPC != "2.0" || decoded.Error != nil {
		return rpcResponse{}, "", fmt.Errorf("invalid JSON-RPC response")
	}
	return decoded, safeSessionID(response.Header.Get("MCP-Session-Id")), nil
}

func toolText(raw json.RawMessage) (string, error) {
	var result struct {
		IsError bool `json:"isError"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &result); err != nil || result.IsError {
		return "", ErrInvalidRequest
	}
	for _, item := range result.Content {
		if item.Type == "text" && strings.TrimSpace(item.Text) != "" {
			return item.Text, nil
		}
	}
	return "", ErrInvalidRequest
}

func extractSymbols(raw string) []Symbol {
	var value any
	if json.Unmarshal([]byte(raw), &value) != nil {
		return []Symbol{}
	}
	seen := map[string]bool{}
	items := make([]Symbol, 0, maxSymbols)
	var walk func(any)
	walk = func(node any) {
		if len(items) >= maxSymbols {
			return
		}
		switch current := node.(type) {
		case []any:
			for _, child := range current {
				walk(child)
			}
		case map[string]any:
			name, _ := current["name_path"].(string)
			if name != "" {
				item := Symbol{NamePath: truncate(cleanText(name), 240)}
				if relative, ok := current["relative_path"].(string); ok {
					item.RelativePath = truncate(cleanText(relative), maxRelativePathLength)
				}
				item.Kind = truncate(cleanText(fmt.Sprint(current["kind"])), 80)
				item.StartLine, item.EndLine = lineBounds(current["body_location"])
				key := item.NamePath + "|" + item.RelativePath
				if item.NamePath != "" && !seen[key] {
					seen[key] = true
					items = append(items, item)
				}
			}
			for _, child := range current {
				walk(child)
			}
		}
	}
	walk(value)
	sort.Slice(items, func(i, j int) bool {
		if items[i].RelativePath == items[j].RelativePath {
			return items[i].NamePath < items[j].NamePath
		}
		return items[i].RelativePath < items[j].RelativePath
	})
	return items
}

func lineBounds(raw any) (int, int) {
	value, ok := raw.(map[string]any)
	if !ok {
		return 0, 0
	}
	return positiveInt(value["start_line"]), positiveInt(value["end_line"])
}

func positiveInt(raw any) int {
	switch value := raw.(type) {
	case float64:
		if value >= 0 && value <= 1_000_000 {
			return int(value)
		}
	case int:
		if value >= 0 && value <= 1_000_000 {
			return value
		}
	}
	return 0
}

func validateRequest(input SymbolRequest) (string, string, error) {
	pattern := strings.TrimSpace(input.Pattern)
	if pattern == "" || len(pattern) > maxPatternLength || strings.ContainsAny(pattern, "\r\n\x00") {
		return "", "", ErrInvalidRequest
	}
	relativePath := strings.TrimSpace(strings.ReplaceAll(input.RelativePath, "\\", "/"))
	if relativePath == "" {
		return pattern, "", nil
	}
	if len(relativePath) > maxRelativePathLength || strings.ContainsAny(relativePath, "\r\n\x00") || strings.HasPrefix(relativePath, "/") || strings.Contains(relativePath, ":") {
		return "", "", ErrInvalidRequest
	}
	cleaned := path.Clean(relativePath)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", "", ErrInvalidRequest
	}
	return pattern, cleaned, nil
}

func parseLocalBaseURL(raw string) (*url.URL, string) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, baseURLEnv + " must be a plain local HTTP(S) MCP URL without credentials, query data, or fragments"
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "localhost" && host != "host.docker.internal" && host != "serena" && net.ParseIP(host) == nil {
		return nil, baseURLEnv + " must resolve to localhost, host.docker.internal, serena, or a literal local IP"
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() && !ip.IsPrivate() {
		return nil, baseURLEnv + " must use a loopback or private-network IP"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if parsed.Path == "" {
		return nil, baseURLEnv + " must include the configured MCP path"
	}
	return parsed, ""
}

func parseProjectID(raw string) (string, string) {
	projectID := strings.TrimSpace(raw)
	if projectID == "" || len(projectID) > 128 || strings.ContainsAny(projectID, "\r\n\x00") || strings.HasPrefix(projectID, "/") || strings.Contains(projectID, "..") {
		return "", projectIDEnv + " must be a stable non-path project label"
	}
	for _, r := range projectID {
		if !(r == '-' || r == '_' || r == '.' || r == '/' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return "", projectIDEnv + " may contain only letters, digits, dot, slash, hyphen, and underscore"
		}
	}
	return projectID, ""
}

func protocolFromResult(raw json.RawMessage) string {
	var result struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if json.Unmarshal(raw, &result) != nil {
		return ""
	}
	return strings.TrimSpace(result.ProtocolVersion)
}

func matchesID(raw json.RawMessage, expected int) bool {
	return strings.TrimSpace(string(raw)) == fmt.Sprintf("%d", expected)
}

func hasTool(tools []mcpTool, name string) bool {
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) == name {
			return true
		}
	}
	return false
}

func safeSessionID(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > 512 || strings.ContainsAny(value, "\r\n") {
		return ""
	}
	return value
}

func cleanText(raw string) string { return strings.Join(strings.Fields(strings.TrimSpace(raw)), " ") }

func truncate(raw string, limit int) string {
	if len(raw) > limit {
		return raw[:limit]
	}
	return raw
}
