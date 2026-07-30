// Package presidio provides a deliberately narrow bridge to an operator-run
// local Presidio Analyzer. It returns bounded detection metadata only and does
// not anonymize, persist, or replay submitted text.
package presidio

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
	"unicode/utf8"
)

const (
	enabledEnv             = "HAI_PRESIDIO_ENABLED"
	baseURLEnv             = "HAI_PRESIDIO_BASE_URL"
	languageEnv            = "HAI_PRESIDIO_LANGUAGE"
	entitiesEnv            = "HAI_PRESIDIO_ENTITIES"
	maxTextLength          = 8192
	maxEntities            = 32
	maxResponseBytes int64 = 1 << 20
)

var (
	ErrNotConfigured  = errors.New("local Presidio analyzer is not configured")
	ErrInvalidRequest = errors.New("invalid Presidio analysis request")
	entityPattern     = regexp.MustCompile(`^[A-Z][A-Z_]{1,63}$`)
	languagePattern   = regexp.MustCompile(`^[a-z]{2}$`)
)

type Status struct {
	Enabled      bool     `json:"enabled"`
	Configured   bool     `json:"configured"`
	Provider     string   `json:"provider"`
	Endpoint     string   `json:"endpoint,omitempty"`
	Language     string   `json:"language"`
	EntityTypes  []string `json:"entityTypes"`
	ConfigError  string   `json:"configError,omitempty"`
	Capabilities []string `json:"capabilities"`
	Restrictions []string `json:"restrictions"`
	Scope        string   `json:"scope"`
}

type Request struct {
	Text string `json:"text"`
}

// Entity intentionally contains no text value. The caller receives only the
// type, confidence, and location needed to apply a review decision locally.
type Entity struct {
	Type  string  `json:"type"`
	Start int     `json:"start"`
	End   int     `json:"end"`
	Score float64 `json:"score"`
}

type Response struct {
	EntityCount int      `json:"entityCount"`
	Entities    []Entity `json:"entities"`
	Scope       string   `json:"scope"`
}

type Service interface {
	Status() Status
	Analyze(context.Context, Request) (*Response, error)
}

type service struct {
	enabled   bool
	baseURL   *url.URL
	language  string
	entities  []string
	configErr string
	client    *http.Client
}

func DefaultService() Service {
	return NewService(
		strings.EqualFold(strings.TrimSpace(os.Getenv(enabledEnv)), "true"),
		os.Getenv(baseURLEnv),
		os.Getenv(languageEnv),
		os.Getenv(entitiesEnv),
		nil,
	)
}

func NewService(enabled bool, rawBaseURL, rawLanguage, rawEntities string, client *http.Client) Service {
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
		if s.configErr == "" {
			s.language, s.configErr = parseLanguage(rawLanguage)
		}
		if s.configErr == "" {
			s.entities, s.configErr = parseEntityTypes(rawEntities)
		}
	}
	return s
}

func (s *service) Status() Status {
	status := Status{
		Enabled:      s.enabled,
		Configured:   s.enabled && s.configErr == "",
		Provider:     "Presidio Analyzer",
		Language:     s.language,
		EntityTypes:  append([]string(nil), s.entities...),
		ConfigError:  s.configErr,
		Capabilities: []string{"bounded local PII entity detection", "review metadata without detected text"},
		Restrictions: []string{"no anonymization, masking, or deletion", "no text persistence, replay, or audit payload", "no automatic provider, memory, fact, workflow, or external-action updates"},
		Scope:        "Operator-configured local Presidio analysis only. A detected entity creates a review signal; no result, including no detections, proves that content is safe for cloud providers or storage.",
	}
	if s.baseURL != nil {
		status.Endpoint = s.baseURL.String()
	}
	return status
}

func (s *service) Analyze(ctx context.Context, input Request) (*Response, error) {
	if !s.configured() {
		return nil, ErrNotConfigured
	}
	text := strings.TrimSpace(input.Text)
	textLength := utf8.RuneCountInString(text)
	if text == "" || textLength > maxTextLength {
		return nil, ErrInvalidRequest
	}
	payload, err := json.Marshal(map[string]any{
		"text":     text,
		"language": s.language,
		"entities": s.entities,
	})
	if err != nil {
		return nil, fmt.Errorf("could not encode local Presidio analysis request")
	}
	endpoint := s.endpoint("/analyze")
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("could not create local Presidio analysis request")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "HAI-Presidio-Privacy-Review/1.0")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("local Presidio analyzer is unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("local Presidio analyzer returned an unsuccessful response")
	}
	var raw []struct {
		EntityType string  `json:"entity_type"`
		Start      int     `json:"start"`
		End        int     `json:"end"`
		Score      float64 `json:"score"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes)).Decode(&raw); err != nil {
		return nil, fmt.Errorf("local Presidio analyzer returned invalid JSON")
	}
	allowed := make(map[string]bool, len(s.entities))
	for _, entityType := range s.entities {
		allowed[entityType] = true
	}
	entities := make([]Entity, 0, len(raw))
	for _, result := range raw {
		if len(entities) >= maxEntities || !allowed[result.EntityType] || result.Start < 0 || result.End < result.Start || result.End > textLength || result.Score < 0 || result.Score > 1 {
			continue
		}
		entities = append(entities, Entity{Type: result.EntityType, Start: result.Start, End: result.End, Score: result.Score})
	}
	return &Response{EntityCount: len(entities), Entities: entities, Scope: s.Status().Scope}, nil
}

func (s *service) configured() bool { return s.enabled && s.configErr == "" && s.baseURL != nil }

func (s *service) endpoint(path string) url.URL {
	endpoint := *s.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + path
	return endpoint
}

func parseLocalBaseURL(raw string) (*url.URL, string) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, baseURLEnv + " must be a plain local HTTP(S) URL without credentials, query data, or fragments"
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "localhost" && host != "host.docker.internal" && host != "presidio" && net.ParseIP(host) == nil {
		return nil, baseURLEnv + " must resolve to localhost, host.docker.internal, presidio, or a literal local IP"
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() && !ip.IsPrivate() {
		return nil, baseURLEnv + " must use a loopback or private-network IP"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed, ""
}

func parseLanguage(raw string) (string, string) {
	language := strings.ToLower(strings.TrimSpace(raw))
	if language == "" {
		language = "en"
	}
	if !languagePattern.MatchString(language) {
		return "", languageEnv + " must be a two-letter language code"
	}
	return language, ""
}

func parseEntityTypes(raw string) ([]string, string) {
	seen := map[string]bool{}
	entities := make([]string, 0, 8)
	for _, value := range strings.Split(raw, ",") {
		entityType := strings.ToUpper(strings.TrimSpace(value))
		if entityType == "" {
			continue
		}
		if !entityPattern.MatchString(entityType) {
			return nil, entitiesEnv + " contains an invalid entity type"
		}
		if !seen[entityType] {
			seen[entityType] = true
			entities = append(entities, entityType)
		}
		if len(entities) > maxEntities {
			return nil, entitiesEnv + " supports at most 32 entity types"
		}
	}
	if len(entities) == 0 {
		return nil, entitiesEnv + " requires at least one explicitly approved entity type"
	}
	return entities, ""
}
