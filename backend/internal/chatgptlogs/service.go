// Package chatgptlogs provides a bounded, read-only task-context adapter for
// an operator-run chatgpt-codex-mcp-daemon endpoint. HAI never starts the MCP
// process and calls only the daemon's search tool.
package chatgptlogs

import (
	"bufio"
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
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	enabledEnv             = "HAI_CHATGPT_LOGS_MCP_ENABLED"
	baseURLEnv             = "HAI_CHATGPT_LOGS_MCP_URL"
	timeoutEnv             = "HAI_CHATGPT_LOGS_MCP_TIMEOUT_SECONDS"
	protocolVersion        = "2025-06-18"
	searchTool             = "search"
	maxQueryRunes          = 1000
	maxProjectRunes        = 240
	maxToolTextRunes       = 12000
	maxResponseBytes int64 = 128 << 10
)

var (
	ErrNotConfigured  = errors.New("ChatGPT logs MCP context is not configured")
	ErrInvalidRequest = errors.New("invalid ChatGPT logs MCP search request")
)

type Status struct {
	Enabled      bool     `json:"enabled"`
	Configured   bool     `json:"configured"`
	Endpoint     string   `json:"endpoint,omitempty"`
	ConfigError  string   `json:"configError,omitempty"`
	Capabilities []string `json:"capabilities"`
	Restrictions []string `json:"restrictions"`
	Scope        string   `json:"scope"`
}

type SearchRequest struct {
	Query      string `json:"query"`
	ProjectKey string `json:"projectKey,omitempty"`
}

type ContextItem struct {
	Provider   string `json:"provider"`
	Tool       string `json:"tool"`
	Query      string `json:"query"`
	ProjectKey string `json:"projectKey,omitempty"`
	Content    string `json:"content"`
	SourceURI  string `json:"sourceUri"`
	Untrusted  bool   `json:"untrusted"`
}

type Service interface {
	Status() Status
	Search(context.Context, SearchRequest) ([]ContextItem, error)
}

type service struct {
	enabled   bool
	baseURL   *url.URL
	configErr string
	client    *http.Client
}

func DefaultService() Service {
	timeout := 8 * time.Second
	if raw := strings.TrimSpace(os.Getenv(timeoutEnv)); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds >= 1 && seconds <= 30 {
			timeout = time.Duration(seconds) * time.Second
		}
	}
	client := &http.Client{
		Timeout:       timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		Transport:     &http.Transport{Proxy: nil},
	}
	return NewService(strings.EqualFold(strings.TrimSpace(os.Getenv(enabledEnv)), "true"), os.Getenv(baseURLEnv), client)
}

func NewService(enabled bool, rawBaseURL string, client *http.Client) Service {
	s := &service{enabled: enabled}
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
	}
	return s
}

func (s *service) Status() Status {
	status := Status{
		Enabled:     s.enabled,
		Configured:  s.configured(),
		ConfigError: s.configErr,
		Capabilities: []string{
			"bounded full-text conversation-history search",
			"project-filtered read-only task context",
		},
		Restrictions: []string{
			"no MCP process launch, arbitrary tool selection, or write-capable call",
			"no conversation detail, raw record, message, sync, import, insight, or database mutation call",
			"retrieved text is untrusted context and never grants execution authority",
		},
		Scope: "Opt-in local chatgpt-codex-mcp-daemon retrieval. HAI verifies the search tool, calls only search with fixed row and character limits, and supplies the bounded result as untrusted task context.",
	}
	if s.baseURL != nil {
		status.Endpoint = s.baseURL.String()
	}
	return status
}

func (s *service) Search(ctx context.Context, input SearchRequest) ([]ContextItem, error) {
	if !s.configured() {
		return nil, ErrNotConfigured
	}
	query := strings.TrimSpace(input.Query)
	project := strings.TrimSpace(input.ProjectKey)
	if query == "" || utf8.RuneCountInString(query) > maxQueryRunes || strings.ContainsRune(query, '\x00') || utf8.RuneCountInString(project) > maxProjectRunes || strings.ContainsRune(project, '\x00') {
		return nil, ErrInvalidRequest
	}
	session, tools, err := s.sessionAndTools(ctx)
	if err != nil {
		return nil, err
	}
	if !hasTool(tools, searchTool) {
		return nil, fmt.Errorf("ChatGPT logs MCP endpoint does not expose the required read-only search tool")
	}
	arguments := map[string]any{
		"query":     query,
		"limit":     5,
		"offset":    0,
		"order":     "rank",
		"rank_pool": 200,
		"max_chars": maxToolTextRunes,
	}
	if project != "" {
		arguments["project"] = project
	}
	response, _, err := s.rpc(ctx, session, 3, "tools/call", map[string]any{"name": searchTool, "arguments": arguments})
	if err != nil {
		return nil, fmt.Errorf("ChatGPT logs MCP search failed")
	}
	text, err := toolText(response.Result)
	if err != nil {
		return nil, fmt.Errorf("ChatGPT logs MCP search returned no usable context")
	}
	return []ContextItem{{
		Provider:   "chatgpt-codex-mcp-daemon",
		Tool:       searchTool,
		Query:      query,
		ProjectKey: project,
		Content:    truncateRunes(text, maxToolTextRunes),
		SourceURI:  s.baseURL.String(),
		Untrusted:  true,
	}}, nil
}

func (s *service) configured() bool { return s.enabled && s.baseURL != nil && s.configErr == "" }

type mcpTool struct {
	Name string `json:"name"`
}

func (s *service) sessionAndTools(ctx context.Context) (string, []mcpTool, error) {
	init, session, err := s.rpc(ctx, "", 1, "initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]string{"name": "HAI ChatGPT logs context", "version": "1.0"},
	})
	if err != nil || !matchesID(init.ID, 1) || protocolFromResult(init.Result) != protocolVersion {
		return "", nil, fmt.Errorf("ChatGPT logs MCP initialize failed")
	}
	if _, _, err := s.rpc(ctx, session, 0, "notifications/initialized", map[string]any{}); err != nil {
		return "", nil, fmt.Errorf("ChatGPT logs MCP initialization notification failed")
	}
	listed, _, err := s.rpc(ctx, session, 2, "tools/list", map[string]any{})
	if err != nil || !matchesID(listed.ID, 2) {
		return "", nil, fmt.Errorf("ChatGPT logs MCP tool inventory failed")
	}
	var payload struct {
		Tools []mcpTool `json:"tools"`
	}
	if json.Unmarshal(listed.Result, &payload) != nil {
		return "", nil, fmt.Errorf("ChatGPT logs MCP tool inventory was invalid")
	}
	return session, payload.Tools, nil
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code int `json:"code"`
	} `json:"error"`
}

func (s *service) rpc(ctx context.Context, session string, id int, method string, params any) (rpcResponse, string, error) {
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
	request.Header.Set("MCP-Protocol-Version", protocolVersion)
	request.Header.Set("User-Agent", "HAI-ChatGPT-Logs-ReadOnly/1.0")
	if session != "" {
		request.Header.Set("MCP-Session-Id", session)
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
		return rpcResponse{}, safeSessionID(response.Header.Get("MCP-Session-Id")), err
	}
	decoded, err := decodeRPCResponse(response)
	if err != nil || decoded.JSONRPC != "2.0" || decoded.Error != nil {
		return rpcResponse{}, "", fmt.Errorf("invalid JSON-RPC response")
	}
	return decoded, safeSessionID(response.Header.Get("MCP-Session-Id")), nil
}

func decodeRPCResponse(response *http.Response) (rpcResponse, error) {
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || int64(len(data)) > maxResponseBytes {
		return rpcResponse{}, fmt.Errorf("response exceeded limit or could not be read")
	}
	if strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		scanner := bufio.NewScanner(bytes.NewReader(data))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			candidate := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			var decoded rpcResponse
			if json.Unmarshal([]byte(candidate), &decoded) == nil && decoded.JSONRPC == "2.0" {
				return decoded, nil
			}
		}
		return rpcResponse{}, fmt.Errorf("event stream contained no JSON-RPC response")
	}
	var decoded rpcResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		return rpcResponse{}, err
	}
	return decoded, nil
}

func toolText(raw json.RawMessage) (string, error) {
	var result struct {
		IsError bool `json:"isError"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(raw, &result) != nil || result.IsError {
		return "", ErrInvalidRequest
	}
	for _, item := range result.Content {
		if item.Type == "text" && strings.TrimSpace(item.Text) != "" {
			return strings.TrimSpace(item.Text), nil
		}
	}
	return "", ErrInvalidRequest
}

func parseLocalBaseURL(raw string) (*url.URL, string) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, baseURLEnv + " must be a plain local HTTP(S) MCP URL without credentials, query data, or fragments"
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host != "localhost" && host != "host.docker.internal" {
		ip := net.ParseIP(host)
		if ip == nil || (!ip.IsLoopback() && !ip.IsPrivate()) {
			return nil, baseURLEnv + " must use localhost, host.docker.internal, or a literal local/private IP"
		}
	}
	if strings.TrimRight(parsed.Path, "/") == "" {
		return nil, baseURLEnv + " must include the configured MCP path"
	}
	return parsed, ""
}

func protocolFromResult(raw json.RawMessage) string {
	var result struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(raw, &result)
	return strings.TrimSpace(result.ProtocolVersion)
}

func matchesID(raw json.RawMessage, expected int) bool {
	return strings.TrimSpace(string(raw)) == strconv.Itoa(expected)
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
	if len(value) > 512 || strings.ContainsAny(value, "\r\n") {
		return ""
	}
	return value
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}
