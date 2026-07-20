// Package anythingllm exposes a deliberately narrow bridge to an
// operator-managed local AnythingLLM workspace. It performs only vector
// searches against an explicit workspace allowlist; it never opens chat,
// uploads documents, changes a workspace, or treats returned chunks as facts.
package anythingllm

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
	"regexp"
	"strings"
	"time"
)

const (
	enabledEnv                        = "HAI_ANYTHINGLLM_ENABLED"
	baseURLEnv                        = "HAI_ANYTHINGLLM_BASE_URL"
	apiKeyEnv                         = "HAI_ANYTHINGLLM_API_KEY"
	workspaceSlugsEnv                 = "HAI_ANYTHINGLLM_WORKSPACE_SLUGS"
	localEmbeddingsConfirmedEnv       = "HAI_ANYTHINGLLM_LOCAL_EMBEDDINGS_CONFIRMED"
	maxQueryLength                    = 512
	maxResults                        = 10
	maxResponseBytes            int64 = 1 << 20
	maxProbeBytes               int64 = 64 << 10
)

var (
	ErrNotConfigured  = errors.New("local AnythingLLM evidence retrieval is not configured")
	ErrInvalidRequest = errors.New("invalid AnythingLLM evidence retrieval request")
	workspacePattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)
)

type Status struct {
	Enabled                  bool     `json:"enabled"`
	Configured               bool     `json:"configured"`
	Provider                 string   `json:"provider"`
	Endpoint                 string   `json:"endpoint,omitempty"`
	WorkspaceCount           int      `json:"workspaceCount"`
	WorkspaceSlugs           []string `json:"workspaceSlugs"`
	LocalEmbeddingsConfirmed bool     `json:"localEmbeddingsConfirmed"`
	ConfigError              string   `json:"configError,omitempty"`
	Capabilities             []string `json:"capabilities"`
	Restrictions             []string `json:"restrictions"`
	Scope                    string   `json:"scope"`
}

type Request struct {
	Query         string `json:"query"`
	WorkspaceSlug string `json:"workspaceSlug"`
	Limit         int    `json:"limit,omitempty"`
}

type Result struct {
	ChunkID       string  `json:"chunkId"`
	WorkspaceSlug string  `json:"workspaceSlug"`
	Title         string  `json:"title,omitempty"`
	Content       string  `json:"content"`
	SourceURI     string  `json:"sourceUri,omitempty"`
	Score         float64 `json:"score,omitempty"`
	Distance      float64 `json:"distance,omitempty"`
}

type Response struct {
	Query         string   `json:"query"`
	WorkspaceSlug string   `json:"workspaceSlug"`
	Results       []Result `json:"results"`
	Total         int      `json:"total"`
	Scope         string   `json:"scope"`
}

type ProbeResult struct {
	Reachable bool      `json:"reachable"`
	CheckedAt time.Time `json:"checkedAt"`
	Scope     string    `json:"scope"`
}

type Service interface {
	Status() Status
	Probe(context.Context) (*ProbeResult, error)
	Retrieve(context.Context, Request) (*Response, error)
}

type service struct {
	enabled                  bool
	baseURL                  *url.URL
	apiKey                   string
	workspaceSlugs           []string
	localEmbeddingsConfirmed bool
	configErr                string
	client                   *http.Client
	now                      func() time.Time
}

func DefaultService() Service {
	return NewService(
		strings.EqualFold(strings.TrimSpace(os.Getenv(enabledEnv)), "true"),
		os.Getenv(baseURLEnv),
		os.Getenv(apiKeyEnv),
		os.Getenv(workspaceSlugsEnv),
		strings.EqualFold(strings.TrimSpace(os.Getenv(localEmbeddingsConfirmedEnv)), "true"),
		nil,
	)
}

func NewService(enabled bool, rawBaseURL, apiKey, rawWorkspaceSlugs string, localEmbeddingsConfirmed bool, client *http.Client) Service {
	s := &service{
		enabled:                  enabled,
		apiKey:                   strings.TrimSpace(apiKey),
		localEmbeddingsConfirmed: localEmbeddingsConfirmed,
		now:                      time.Now,
	}
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
		if s.configErr == "" && s.apiKey == "" {
			s.configErr = apiKeyEnv + " is required when " + enabledEnv + " is true"
		}
		if s.configErr == "" {
			s.workspaceSlugs, s.configErr = parseWorkspaceSlugs(rawWorkspaceSlugs)
		}
		if s.configErr == "" && !s.localEmbeddingsConfirmed {
			s.configErr = localEmbeddingsConfirmedEnv + " must be true after confirming the allowed workspaces use local embeddings"
		}
	}
	return s
}

func (s *service) Status() Status {
	status := Status{
		Enabled:                  s.enabled,
		Configured:               s.configured(),
		Provider:                 "AnythingLLM",
		WorkspaceCount:           len(s.workspaceSlugs),
		WorkspaceSlugs:           append([]string(nil), s.workspaceSlugs...),
		LocalEmbeddingsConfirmed: s.localEmbeddingsConfirmed,
		ConfigError:              s.configErr,
		Capabilities:             []string{"fixed-workspace vector-search candidate evidence", "allowlisted workspace access probe"},
		Restrictions:             []string{"no chat, agents, tools, attachments, or history", "no document upload, ingestion, deletion, or workspace changes", "no automatic memory, fact, workflow, or external-action updates"},
		Scope:                    "Operator-configured local AnythingLLM workspace retrieval only. The upstream vector search may use the workspace embedding provider; HAI requires explicit local-embedding confirmation. Returned chunks are candidate evidence and must pass HAI source-grounding and verification before they influence memory, facts, tasks, or external actions.",
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
	endpoint := s.endpoint("/api/v1/workspaces")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("could not create local AnythingLLM workspace probe")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+s.apiKey)
	request.Header.Set("User-Agent", "HAI-AnythingLLM-Evidence/1.0")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("local AnythingLLM endpoint is unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("local AnythingLLM endpoint did not pass workspace probe")
	}
	var payload struct {
		Workspaces []struct {
			Slug string `json:"slug"`
		} `json:"workspaces"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxProbeBytes)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("local AnythingLLM workspace probe returned invalid JSON")
	}
	found := make(map[string]bool, len(payload.Workspaces))
	for _, workspace := range payload.Workspaces {
		found[workspace.Slug] = true
	}
	for _, slug := range s.workspaceSlugs {
		if !found[slug] {
			return nil, fmt.Errorf("configured local AnythingLLM workspace is unavailable")
		}
	}
	return &ProbeResult{Reachable: true, CheckedAt: s.now().UTC(), Scope: "Authenticated allowlisted-workspace reachability only. This does not validate upstream embedding locality, indexed-document quality, result provenance, or downstream verification."}, nil
}

func (s *service) Retrieve(ctx context.Context, request Request) (*Response, error) {
	if !s.configured() {
		return nil, ErrNotConfigured
	}
	query, err := validQuery(request.Query)
	if err != nil {
		return nil, err
	}
	workspaceSlug := strings.TrimSpace(request.WorkspaceSlug)
	if !contains(s.workspaceSlugs, workspaceSlug) {
		return nil, ErrInvalidRequest
	}
	limit := request.Limit
	if limit <= 0 || limit > maxResults {
		limit = 5
	}
	payload, err := json.Marshal(map[string]any{"query": query, "topN": limit})
	if err != nil {
		return nil, fmt.Errorf("could not encode local AnythingLLM vector-search request")
	}
	endpoint := s.endpoint("/api/v1/workspace/" + url.PathEscape(workspaceSlug) + "/vector-search")
	requestHTTP, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("could not create local AnythingLLM vector-search request")
	}
	requestHTTP.Header.Set("Accept", "application/json")
	requestHTTP.Header.Set("Content-Type", "application/json")
	requestHTTP.Header.Set("Authorization", "Bearer "+s.apiKey)
	requestHTTP.Header.Set("User-Agent", "HAI-AnythingLLM-Evidence/1.0")
	responseHTTP, err := s.client.Do(requestHTTP)
	if err != nil {
		return nil, fmt.Errorf("local AnythingLLM vector search is unavailable")
	}
	defer responseHTTP.Body.Close()
	if responseHTTP.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("local AnythingLLM vector search returned an unsuccessful response")
	}
	var payloadResponse struct {
		Results []struct {
			ID       string  `json:"id"`
			Text     string  `json:"text"`
			Distance float64 `json:"distance"`
			Score    float64 `json:"score"`
			Metadata struct {
				URL   string `json:"url"`
				Title string `json:"title"`
			} `json:"metadata"`
		} `json:"results"`
	}
	if err := json.NewDecoder(io.LimitReader(responseHTTP.Body, maxResponseBytes)).Decode(&payloadResponse); err != nil {
		return nil, fmt.Errorf("local AnythingLLM vector search returned invalid JSON")
	}
	results := make([]Result, 0, limit)
	for _, result := range payloadResponse.Results {
		if len(results) >= limit || strings.TrimSpace(result.ID) == "" || strings.TrimSpace(result.Text) == "" {
			continue
		}
		results = append(results, Result{
			ChunkID:       bounded(result.ID, 128),
			WorkspaceSlug: workspaceSlug,
			Title:         bounded(result.Metadata.Title, 240),
			Content:       bounded(result.Text, 6000),
			SourceURI:     bounded(result.Metadata.URL, 512),
			Score:         result.Score,
			Distance:      result.Distance,
		})
	}
	return &Response{Query: query, WorkspaceSlug: workspaceSlug, Results: results, Total: len(results), Scope: s.Status().Scope}, nil
}

func (s *service) configured() bool { return s.enabled && s.configErr == "" && s.baseURL != nil }

func (s *service) endpoint(path string) url.URL {
	endpoint := *s.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + path
	return endpoint
}

func parseLocalBaseURL(raw string) (*url.URL, string) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, baseURLEnv + " must be a plain local HTTP(S) URL without credentials, query data, or fragments"
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "localhost" && host != "host.docker.internal" && host != "anythingllm" && net.ParseIP(host) == nil {
		return nil, baseURLEnv + " must resolve to localhost, host.docker.internal, anythingllm, or a literal local IP"
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() && !ip.IsPrivate() {
		return nil, baseURLEnv + " must use a loopback or private-network IP"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed, ""
}

func parseWorkspaceSlugs(raw string) ([]string, string) {
	seen := map[string]bool{}
	slugs := make([]string, 0, 8)
	for _, value := range strings.Split(raw, ",") {
		slug := strings.TrimSpace(value)
		if slug == "" {
			continue
		}
		if !workspacePattern.MatchString(slug) {
			return nil, workspaceSlugsEnv + " contains an invalid workspace slug"
		}
		if !seen[slug] {
			seen[slug] = true
			slugs = append(slugs, slug)
		}
		if len(slugs) > 16 {
			return nil, workspaceSlugsEnv + " supports at most 16 workspace slugs"
		}
	}
	if len(slugs) == 0 {
		return nil, workspaceSlugsEnv + " requires at least one explicitly approved workspace slug"
	}
	return slugs, ""
}

func validQuery(raw string) (string, error) {
	query := strings.TrimSpace(raw)
	if query == "" || len(query) > maxQueryLength || strings.ContainsAny(query, "\r\n") {
		return "", ErrInvalidRequest
	}
	return query, nil
}

func contains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func bounded(raw string, limit int) string {
	value := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if len(value) > limit {
		return value[:limit]
	}
	return value
}
