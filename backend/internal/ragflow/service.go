// Package ragflow provides a deliberately narrow bridge to an operator-run
// local RAGFlow instance. It retrieves candidate evidence from an explicit
// dataset allowlist; it never ingests content, invokes RAGFlow agents, or
// treats returned chunks as verified HAI memory.
package ragflow

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
	enabledEnv             = "HAI_RAGFLOW_ENABLED"
	baseURLEnv             = "HAI_RAGFLOW_BASE_URL"
	apiKeyEnv              = "HAI_RAGFLOW_API_KEY"
	datasetIDsEnv          = "HAI_RAGFLOW_DATASET_IDS"
	maxQueryLength         = 512
	maxResults             = 10
	maxResponseBytes int64 = 1 << 20
	maxHealthBytes   int64 = 64 << 10
)

var (
	ErrNotConfigured  = errors.New("local RAGFlow retrieval is not configured")
	ErrInvalidRequest = errors.New("invalid RAGFlow retrieval request")
	datasetIDPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)
)

type Status struct {
	Enabled      bool     `json:"enabled"`
	Configured   bool     `json:"configured"`
	Provider     string   `json:"provider"`
	Endpoint     string   `json:"endpoint,omitempty"`
	DatasetCount int      `json:"datasetCount"`
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
	ChunkID          string  `json:"chunkId"`
	DatasetID        string  `json:"datasetId"`
	DocumentID       string  `json:"documentId,omitempty"`
	DocumentName     string  `json:"documentName,omitempty"`
	Content          string  `json:"content"`
	Similarity       float64 `json:"similarity,omitempty"`
	TermSimilarity   float64 `json:"termSimilarity,omitempty"`
	VectorSimilarity float64 `json:"vectorSimilarity,omitempty"`
}

type Response struct {
	Query      string   `json:"query"`
	Results    []Result `json:"results"`
	Total      int      `json:"total"`
	DatasetIDs []string `json:"datasetIds"`
	Scope      string   `json:"scope"`
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
	enabled    bool
	baseURL    *url.URL
	apiKey     string
	datasetIDs []string
	configErr  string
	client     *http.Client
	now        func() time.Time
}

func DefaultService() Service {
	return NewService(
		strings.EqualFold(strings.TrimSpace(os.Getenv(enabledEnv)), "true"),
		os.Getenv(baseURLEnv),
		os.Getenv(apiKeyEnv),
		os.Getenv(datasetIDsEnv),
		nil,
	)
}

func NewService(enabled bool, rawBaseURL, apiKey, rawDatasetIDs string, client *http.Client) Service {
	s := &service{enabled: enabled, apiKey: strings.TrimSpace(apiKey), now: time.Now}
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
			s.datasetIDs, s.configErr = parseDatasetIDs(rawDatasetIDs)
		}
	}
	return s
}

func (s *service) Status() Status {
	status := Status{
		Enabled:      s.enabled,
		Configured:   s.enabled && s.configErr == "",
		Provider:     "RAGFlow",
		DatasetCount: len(s.datasetIDs),
		ConfigError:  s.configErr,
		Capabilities: []string{"fixed-dataset candidate evidence retrieval", "local endpoint reachability probe"},
		Restrictions: []string{"no document ingestion or deletion", "no agent, chat, MCP, or code-executor calls", "no automatic memory, fact, workflow, or external-action updates"},
		Scope:        "Operator-configured local RAGFlow retrieval only. Returned chunks are candidate evidence and must pass HAI source-grounding and verification before they influence memory, facts, tasks, or external actions.",
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
	endpoint := s.endpoint("/api/v1/system/healthz")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("could not create local RAGFlow health request")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "HAI-RAGFlow-Retrieval/1.0")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("local RAGFlow endpoint is unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("local RAGFlow endpoint did not pass health probe")
	}
	var health struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxHealthBytes)).Decode(&health); err != nil || !strings.EqualFold(strings.TrimSpace(health.Status), "ok") {
		return nil, fmt.Errorf("local RAGFlow dependencies are not healthy")
	}
	return &ProbeResult{Reachable: true, CheckedAt: s.now().UTC(), Scope: "Endpoint and reported RAGFlow dependency health only. This does not validate retrieval credentials, dataset access, chunk provenance, or downstream verification."}, nil
}

func (s *service) Retrieve(ctx context.Context, request Request) (*Response, error) {
	if !s.configured() {
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
	payload, err := json.Marshal(map[string]any{
		"question":    query,
		"dataset_ids": s.datasetIDs,
		"page":        1,
		"page_size":   limit,
		"top_k":       limit,
		"keyword":     true,
		"highlight":   false,
		"use_kg":      false,
		"toc_enhance": false,
	})
	if err != nil {
		return nil, fmt.Errorf("could not encode local RAGFlow retrieval request")
	}
	endpoint := s.endpoint("/api/v1/retrieval")
	requestHTTP, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("could not create local RAGFlow retrieval request")
	}
	requestHTTP.Header.Set("Accept", "application/json")
	requestHTTP.Header.Set("Content-Type", "application/json")
	requestHTTP.Header.Set("Authorization", "Bearer "+s.apiKey)
	requestHTTP.Header.Set("User-Agent", "HAI-RAGFlow-Retrieval/1.0")
	responseHTTP, err := s.client.Do(requestHTTP)
	if err != nil {
		return nil, fmt.Errorf("local RAGFlow retrieval is unavailable")
	}
	defer responseHTTP.Body.Close()
	if responseHTTP.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("local RAGFlow retrieval returned an unsuccessful response")
	}
	var payloadResponse struct {
		Code int `json:"code"`
		Data struct {
			Total  int `json:"total"`
			Chunks []struct {
				Content          string  `json:"content"`
				DocumentID       string  `json:"document_id"`
				DocumentKeyword  string  `json:"document_keyword"`
				ID               string  `json:"id"`
				KBID             string  `json:"kb_id"`
				Similarity       float64 `json:"similarity"`
				TermSimilarity   float64 `json:"term_similarity"`
				VectorSimilarity float64 `json:"vector_similarity"`
			} `json:"chunks"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(responseHTTP.Body, maxResponseBytes)).Decode(&payloadResponse); err != nil {
		return nil, fmt.Errorf("local RAGFlow retrieval returned invalid JSON")
	}
	if payloadResponse.Code != 0 {
		return nil, fmt.Errorf("local RAGFlow retrieval rejected the request")
	}
	allowed := make(map[string]bool, len(s.datasetIDs))
	for _, id := range s.datasetIDs {
		allowed[id] = true
	}
	results := make([]Result, 0, limit)
	for _, chunk := range payloadResponse.Data.Chunks {
		if len(results) >= limit || !allowed[chunk.KBID] || strings.TrimSpace(chunk.ID) == "" || strings.TrimSpace(chunk.Content) == "" {
			continue
		}
		results = append(results, Result{
			ChunkID:          bounded(chunk.ID, 128),
			DatasetID:        bounded(chunk.KBID, 128),
			DocumentID:       bounded(chunk.DocumentID, 128),
			DocumentName:     bounded(chunk.DocumentKeyword, 240),
			Content:          bounded(chunk.Content, 6000),
			Similarity:       chunk.Similarity,
			TermSimilarity:   chunk.TermSimilarity,
			VectorSimilarity: chunk.VectorSimilarity,
		})
	}
	return &Response{Query: query, Results: results, Total: len(results), DatasetIDs: append([]string(nil), s.datasetIDs...), Scope: s.Status().Scope}, nil
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
	if host != "localhost" && host != "host.docker.internal" && host != "ragflow" && net.ParseIP(host) == nil {
		return nil, baseURLEnv + " must resolve to localhost, host.docker.internal, ragflow, or a literal local IP"
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() && !ip.IsPrivate() {
		return nil, baseURLEnv + " must use a loopback or private-network IP"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed, ""
}

func parseDatasetIDs(raw string) ([]string, string) {
	seen := map[string]bool{}
	ids := make([]string, 0, 8)
	for _, value := range strings.Split(raw, ",") {
		id := strings.TrimSpace(value)
		if id == "" {
			continue
		}
		if !datasetIDPattern.MatchString(id) {
			return nil, datasetIDsEnv + " contains an invalid dataset ID"
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
		if len(ids) > 16 {
			return nil, datasetIDsEnv + " supports at most 16 dataset IDs"
		}
	}
	if len(ids) == 0 {
		return nil, datasetIDsEnv + " requires at least one explicitly approved dataset ID"
	}
	return ids, ""
}

func validQuery(raw string) (string, error) {
	query := strings.TrimSpace(raw)
	if query == "" || len(query) > maxQueryLength || strings.ContainsAny(query, "\r\n") {
		return "", ErrInvalidRequest
	}
	return query, nil
}

func bounded(raw string, limit int) string {
	value := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if len(value) > limit {
		return value[:limit]
	}
	return value
}
