// Package research provides bounded discovery through an operator-configured
// local SearXNG instance. It returns candidates, never verified evidence.
package research

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	enabledEnv             = "HAI_SEARXNG_ENABLED"
	baseURLEnv             = "HAI_SEARXNG_BASE_URL"
	maxQueryLength         = 512
	maxResults             = 10
	maxResponseBytes int64 = 1 << 20
	maxHealthBytes   int64 = 64 << 10
)

var ErrNotConfigured = errors.New("local research is not configured")

type Status struct {
	Enabled      bool     `json:"enabled"`
	Configured   bool     `json:"configured"`
	Provider     string   `json:"provider"`
	Endpoint     string   `json:"endpoint,omitempty"`
	ConfigError  string   `json:"configError,omitempty"`
	Capabilities []string `json:"capabilities"`
	Restrictions []string `json:"restrictions"`
	Scope        string   `json:"scope"`
}

type Request struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}

type Result struct {
	Title       string   `json:"title"`
	SourceURI   string   `json:"sourceUri"`
	Snippet     string   `json:"snippet"`
	Engines     []string `json:"engines,omitempty"`
	PublishedAt string   `json:"publishedAt,omitempty"`
}

type Response struct {
	Query   string   `json:"query"`
	Results []Result `json:"results"`
	Scope   string   `json:"scope"`
}

// ProbeResult is deliberately only an endpoint reachability signal. It does
// not establish that the configured engines, JSON output, or returned sources
// are safe or suitable for a particular task.
type ProbeResult struct {
	Reachable bool      `json:"reachable"`
	CheckedAt time.Time `json:"checkedAt"`
	Scope     string    `json:"scope"`
}

type Service interface {
	Status() Status
	Probe(context.Context) (*ProbeResult, error)
	Search(context.Context, Request) (*Response, error)
}

type service struct {
	enabled   bool
	baseURL   *url.URL
	configErr string
	client    *http.Client
}

func DefaultService() Service {
	return NewService(strings.EqualFold(strings.TrimSpace(os.Getenv(enabledEnv)), "true"), os.Getenv(baseURLEnv), nil)
}

func NewService(enabled bool, rawBaseURL string, client *http.Client) Service {
	s := &service{enabled: enabled}
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }, Transport: &http.Transport{Proxy: nil}}
	}
	s.client = client
	if enabled {
		s.baseURL, s.configErr = parseLocalBaseURL(rawBaseURL)
	}
	return s
}

func (s *service) Status() Status {
	status := Status{
		Enabled:      s.enabled,
		Configured:   s.enabled && s.configErr == "",
		Provider:     "SearXNG",
		ConfigError:  s.configErr,
		Capabilities: []string{"bounded local source discovery", "local endpoint reachability probe"},
		Restrictions: []string{"no public endpoint", "no page fetching", "no cookie or credential forwarding", "no automatic evidence, memory, workflow, or action updates"},
		Scope:        "Operator-configured local SearXNG discovery only. HAI sends a bounded query to the local instance, returns source candidates, does not fetch result pages, and never treats search snippets as verified evidence.",
	}
	if s.baseURL != nil {
		status.Endpoint = s.baseURL.String()
	}
	return status
}

func (s *service) Probe(ctx context.Context) (*ProbeResult, error) {
	if !s.enabled || s.configErr != "" || s.baseURL == nil {
		return nil, ErrNotConfigured
	}
	endpoint := *s.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/healthz"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("could not create local research health request")
	}
	request.Header.Set("Accept", "text/plain, application/json")
	request.Header.Set("User-Agent", "HAI-LocalResearch/1.0")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("local research service is unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("local research service did not pass health probe")
	}
	if _, err := io.Copy(io.Discard, io.LimitReader(response.Body, maxHealthBytes)); err != nil {
		return nil, fmt.Errorf("could not read local research health response")
	}
	return &ProbeResult{
		Reachable: true,
		CheckedAt: time.Now().UTC(),
		Scope:     "Endpoint reachability only. This does not verify JSON output, search-engine policy, external upstream behavior, result provenance, or evidence quality.",
	}, nil
}

func (s *service) Search(ctx context.Context, request Request) (*Response, error) {
	if !s.enabled || s.configErr != "" || s.baseURL == nil {
		return nil, ErrNotConfigured
	}
	query, err := validQuery(request.Query)
	if err != nil {
		return nil, err
	}
	limit := request.Limit
	if limit <= 0 || limit > maxResults {
		limit = 5
	}
	endpoint := *s.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/search"
	values := endpoint.Query()
	values.Set("q", query)
	values.Set("format", "json")
	values.Set("categories", "general")
	values.Set("safesearch", "2")
	endpoint.RawQuery = values.Encode()
	requestHTTP, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("could not create local research request")
	}
	requestHTTP.Header.Set("Accept", "application/json")
	requestHTTP.Header.Set("User-Agent", "HAI-LocalResearch/1.0")
	responseHTTP, err := s.client.Do(requestHTTP)
	if err != nil {
		return nil, fmt.Errorf("local research service is unavailable")
	}
	defer responseHTTP.Body.Close()
	if responseHTTP.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("local research service returned HTTP %d", responseHTTP.StatusCode)
	}
	var payload struct {
		Results []struct {
			Title         string   `json:"title"`
			URL           string   `json:"url"`
			Content       string   `json:"content"`
			Engines       []string `json:"engines"`
			PublishedDate string   `json:"publishedDate"`
		} `json:"results"`
	}
	if err := json.NewDecoder(io.LimitReader(responseHTTP.Body, maxResponseBytes)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("local research service returned invalid JSON")
	}
	results := make([]Result, 0, limit)
	for _, item := range payload.Results {
		if len(results) >= limit {
			break
		}
		uri, ok := normalizedSourceURL(item.URL)
		if !ok {
			continue
		}
		results = append(results, Result{Title: bounded(item.Title, 240), SourceURI: uri, Snippet: bounded(item.Content, 1000), Engines: boundedValues(item.Engines, 5, 80), PublishedAt: bounded(item.PublishedDate, 80)})
	}
	return &Response{Query: query, Results: results, Scope: s.Status().Scope}, nil
}

func parseLocalBaseURL(raw string) (*url.URL, string) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, "HAI_SEARXNG_BASE_URL must be a plain local HTTP(S) URL without credentials, query data, or fragments"
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "localhost" && host != "host.docker.internal" && host != "searxng" && host != "frontend" && net.ParseIP(host) == nil {
		return nil, "HAI_SEARXNG_BASE_URL must resolve to localhost, host.docker.internal, searxng, frontend, or a literal local IP"
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() && !ip.IsPrivate() {
		return nil, "HAI_SEARXNG_BASE_URL must use a loopback or private-network IP"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed, ""
}

func validQuery(raw string) (string, error) {
	query := strings.TrimSpace(raw)
	if query == "" || len(query) > maxQueryLength || strings.ContainsAny(query, "\r\n") || containsExternalBang(query) {
		return "", fmt.Errorf("research query is required, single-line, and limited to %d characters", maxQueryLength)
	}
	return query, nil
}

// SearXNG documents !! as an external bang / automatic redirect. HAI never
// follows redirects, but rejecting it also prevents the configured local
// instance from being asked to construct a direct external-search redirect.
func containsExternalBang(query string) bool {
	for _, token := range strings.Fields(query) {
		if strings.HasPrefix(token, "!!") {
			return true
		}
	}
	return false
}

func normalizedSourceURL(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return "", false
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), true
}

func bounded(raw string, limit int) string {
	value := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if len(value) > limit {
		return value[:limit]
	}
	return value
}

func boundedValues(values []string, maxItems, maxLength int) []string {
	result := make([]string, 0, maxItems)
	for _, value := range values {
		if len(result) >= maxItems {
			break
		}
		if cleaned := bounded(value, maxLength); cleaned != "" {
			result = append(result, cleaned)
		}
	}
	return result
}
