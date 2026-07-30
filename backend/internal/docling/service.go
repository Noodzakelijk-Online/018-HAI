// Package docling provides a narrow bridge to a local Docling document
// extraction runner. It accepts only an already-registered relative source
// folder and returns bounded text plus provenance metadata. The browser never
// supplies files, models, parser flags, or filesystem paths.
package docling

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
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	enabledEnv       = "HAI_DOCLING_ENABLED"
	runnerURLEnv     = "HAI_DOCLING_RUNNER_URL"
	tokenEnv         = "HAI_DOCLING_RUNNER_TOKEN"
	timeoutEnv       = "HAI_DOCLING_TIMEOUT_SECONDS"
	maxResponseBytes = 3 << 20
	maxDocuments     = 10
	maxDocumentChars = 250000
	maxTotalChars    = 2_000_000
	defaultTimeout   = 180 * time.Second
)

var (
	ErrNotConfigured = errors.New("local Docling document extractor is not configured")
	ErrUnavailable   = errors.New("local Docling document extractor is unavailable")
	formatPattern    = regexp.MustCompile(`^[a-z0-9_-]{1,24}$`)
	digestPattern    = regexp.MustCompile(`^[a-f0-9]{64}$`)
	allowedFormats   = map[string]bool{"docx": true, "pptx": true, "xlsx": true, "html": true, "markdown": true, "text": true, "pdf": true}
)

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

type ProbeResult struct {
	Reachable  bool      `json:"reachable"`
	Engine     string    `json:"engine"`
	Configured bool      `json:"configured"`
	CheckedAt  time.Time `json:"checkedAt"`
	Scope      string    `json:"scope"`
}

type Document struct {
	Path          string `json:"path"`
	Text          string `json:"text"`
	Format        string `json:"format"`
	PageCount     int    `json:"pageCount"`
	ContentDigest string `json:"contentDigest"`
}

type Config struct {
	Enabled   bool
	RunnerURL string
	Token     string
	Timeout   time.Duration
}

type Service interface {
	Status() Status
	Probe(context.Context) (*ProbeResult, error)
	Extract(context.Context, string) ([]Document, error)
}

type service struct {
	enabled   bool
	runnerURL *url.URL
	token     string
	configErr string
	client    *http.Client
	now       func() time.Time
}

func ConfigFromEnv() Config {
	timeout := defaultTimeout
	if raw := strings.TrimSpace(os.Getenv(timeoutEnv)); raw != "" {
		if seconds, err := time.ParseDuration(raw + "s"); err == nil && seconds >= 30*time.Second && seconds <= 10*time.Minute {
			timeout = seconds
		}
	}
	return Config{
		Enabled:   strings.EqualFold(strings.TrimSpace(os.Getenv(enabledEnv)), "true"),
		RunnerURL: os.Getenv(runnerURLEnv),
		Token:     os.Getenv(tokenEnv),
		Timeout:   timeout,
	}
}

func DefaultService() Service { return NewService(ConfigFromEnv(), nil) }

func NewService(config Config, client *http.Client) Service {
	if config.Timeout < 30*time.Second || config.Timeout > 10*time.Minute {
		config.Timeout = defaultTimeout
	}
	if client == nil {
		client = &http.Client{
			Timeout: config.Timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Transport: &http.Transport{Proxy: nil},
		}
	}
	s := &service{enabled: config.Enabled, token: strings.TrimSpace(config.Token), client: client, now: time.Now}
	if s.enabled {
		s.runnerURL, s.configErr = parseRunnerURL(config.RunnerURL)
		if s.configErr == "" && len(s.token) < 16 {
			s.configErr = tokenEnv + " must contain a separate local-only token with at least 16 characters"
		}
	}
	return s
}

func (s *service) Status() Status {
	status := Status{
		Enabled: s.enabled, Configured: s.configured(), Provider: "Docling local structured document extractor", ConfigError: s.configErr,
		Capabilities: []string{
			"operator-triggered extraction from an explicit selected local document folder",
			"source-linked document text and bounded format/page metadata through HAI's existing review, retention, audit, and workflow intake path",
		},
		Restrictions: []string{
			"no browser file upload, arbitrary path, model, OCR, table, parser, remote-service, plugin, or scheduled-scan selection",
			"runner reads only a read-only selected source folder, keeps network/model downloads disabled, and returns text plus bounded metadata only",
			"extracted text remains uncertain source evidence until independently reviewed; extraction cannot send, publish, execute, approve work, or prove completion",
		},
		Scope: "Operator-triggered local document extraction for an explicitly configured docling-documents source only. HAI stores returned text as source evidence with source links; originals remain in the selected local folder.",
	}
	if s.runnerURL != nil {
		status.Endpoint = s.runnerURL.String()
	}
	return status
}

func (s *service) Probe(ctx context.Context) (*ProbeResult, error) {
	if !s.configured() {
		return nil, ErrNotConfigured
	}
	endpoint := s.endpoint("/healthz")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, ErrUnavailable
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "HAI-Docling/1.0")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer resp.Body.Close()
	var body struct {
		Status     string `json:"status"`
		Engine     string `json:"engine"`
		Configured bool   `json:"configured"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body) != nil || body.Status != "ok" || !body.Configured || !validEngine(body.Engine) {
		return nil, ErrUnavailable
	}
	return &ProbeResult{Reachable: true, Engine: body.Engine, Configured: true, CheckedAt: s.now().UTC(), Scope: "Runner reachability only. The probe does not read a source folder or extract document text."}, nil
}

func (s *service) Extract(ctx context.Context, folder string) ([]Document, error) {
	if !s.configured() {
		return nil, ErrNotConfigured
	}
	folder, err := normalizeFolder(folder)
	if err != nil {
		return nil, err
	}
	body, _ := json.Marshal(map[string]string{"folder": folder})
	endpoint := s.endpoint("/v1/extract")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return nil, ErrUnavailable
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "HAI-Docling/1.0")
	req.Header.Set("X-HAI-Docling-Token", s.token)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, ErrUnavailable
	}
	var bodyResult struct {
		Status    string     `json:"status"`
		Documents []Document `json:"documents"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&bodyResult) != nil || bodyResult.Status != "completed" || len(bodyResult.Documents) > maxDocuments {
		return nil, ErrUnavailable
	}
	totalChars := 0
	for index := range bodyResult.Documents {
		if err := validateDocument(&bodyResult.Documents[index], folder); err != nil {
			return nil, ErrUnavailable
		}
		totalChars += len(bodyResult.Documents[index].Text)
		if totalChars > maxTotalChars {
			return nil, ErrUnavailable
		}
	}
	return bodyResult.Documents, nil
}

func (s *service) configured() bool { return s.enabled && s.configErr == "" && s.runnerURL != nil }

func (s *service) endpoint(value string) url.URL {
	endpoint := *s.runnerURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + value
	return endpoint
}

func normalizeFolder(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || value == "." || strings.HasPrefix(value, "/") || strings.Contains(value, "//") {
		return "", fmt.Errorf("an explicit relative selected document folder is required")
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || len(cleaned) > 400 {
		return "", fmt.Errorf("document folder must stay inside the selected intake root")
	}
	return cleaned, nil
}

func validateDocument(value *Document, folder string) error {
	if value == nil {
		return errors.New("invalid document metadata")
	}
	documentPath, err := normalizeFilePath(value.Path)
	if err != nil || !strings.HasPrefix(documentPath, folder+"/") {
		return errors.New("invalid document path")
	}
	value.Path = documentPath
	value.Text = strings.TrimSpace(value.Text)
	value.Format = strings.ToLower(strings.TrimSpace(value.Format))
	value.ContentDigest = strings.ToLower(strings.TrimSpace(value.ContentDigest))
	if value.Text == "" || len(value.Text) > maxDocumentChars || !formatPattern.MatchString(value.Format) || !allowedFormats[value.Format] || value.PageCount < 0 || value.PageCount > 1000 || !digestPattern.MatchString(value.ContentDigest) {
		return errors.New("invalid document metadata")
	}
	return nil
}

func normalizeFilePath(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "//") {
		return "", errors.New("invalid path")
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || len(cleaned) > 800 {
		return "", errors.New("invalid path")
	}
	return cleaned, nil
}

func validEngine(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), "docling ") && len(value) <= 160
}

func parseRunnerURL(raw string) (*url.URL, string) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, runnerURLEnv + " must be a plain local HTTP(S) URL without credentials, query data, or fragments"
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "localhost" && host != "host.docker.internal" && host != "docling-runner" && net.ParseIP(host) == nil {
		return nil, runnerURLEnv + " must resolve to localhost, host.docker.internal, docling-runner, or a literal local IP"
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() && !ip.IsPrivate() {
		return nil, runnerURLEnv + " must use a loopback or private-network IP"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed, ""
}

// SortedFormats is intentionally small and stable. It is useful to callers
// that want to present the runner's fixed format boundary without exposing
// parser configuration or any dynamic model capability.
func SortedFormats() []string {
	formats := make([]string, 0, len(allowedFormats))
	for format := range allowedFormats {
		formats = append(formats, format)
	}
	sort.Strings(formats)
	return formats
}
