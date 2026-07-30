// Package whispercpp provides a narrow bridge to a local whisper.cpp runner.
// It only transcribes files already selected through a connected source; it
// never accepts audio bytes, model choices, language settings, or paths from a
// browser client.
package whispercpp

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
	"strings"
	"time"
)

const (
	enabledEnv               = "HAI_WHISPER_CPP_ENABLED"
	baseURLEnv               = "HAI_WHISPER_CPP_BASE_URL"
	timeoutEnv               = "HAI_WHISPER_CPP_TIMEOUT_SECONDS"
	maxResponseBytes   int64 = 2 << 20
	maxTranscripts           = 25
	maxTranscriptChars       = 100000
)

var ErrNotConfigured = errors.New("local whisper.cpp transcription runner is not configured")

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
	Engine     string    `json:"engine,omitempty"`
	Configured bool      `json:"configured"`
	CheckedAt  time.Time `json:"checkedAt"`
	Scope      string    `json:"scope"`
}

type Transcript struct {
	Path     string `json:"path"`
	Text     string `json:"text"`
	ModelID  string `json:"modelId"`
	Language string `json:"language"`
}

type Service interface {
	Status() Status
	Probe(context.Context) (*ProbeResult, error)
	Transcribe(context.Context, string) ([]Transcript, error)
}

type service struct {
	enabled   bool
	baseURL   *url.URL
	configErr string
	client    *http.Client
	now       func() time.Time
}

func DefaultService() Service {
	timeout := 300 * time.Second
	if raw := strings.TrimSpace(os.Getenv(timeoutEnv)); raw != "" {
		if seconds, err := time.ParseDuration(raw + "s"); err == nil && seconds >= 10*time.Second && seconds <= 600*time.Second {
			timeout = seconds
		}
	}
	return NewService(strings.EqualFold(strings.TrimSpace(os.Getenv(enabledEnv)), "true"), os.Getenv(baseURLEnv), timeout, nil)
}

func NewService(enabled bool, rawBaseURL string, timeout time.Duration, client *http.Client) Service {
	if timeout < 10*time.Second || timeout > 600*time.Second {
		timeout = 300 * time.Second
	}
	if client == nil {
		client = &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }, Transport: &http.Transport{Proxy: nil}}
	}
	s := &service{enabled: enabled, client: client, now: time.Now}
	if enabled {
		s.baseURL, s.configErr = parseLocalBaseURL(rawBaseURL)
	}
	return s
}

func (s *service) Status() Status {
	status := Status{
		Enabled: s.enabled, Configured: s.configured(), Provider: "whisper.cpp local transcription runner", ConfigError: s.configErr,
		Capabilities: []string{"offline audio-to-text from an explicit selected source folder", "source-linked transcript extraction through HAI's existing review, retention, audit, and workflow intake path"},
		Restrictions: []string{"no microphone capture, browser audio upload, cloud transcription, automatic scans, arbitrary path, model, or language selection", "runner sees only read-only selected audio folders and returns transcript metadata, never raw media", "transcripts are uncertain source evidence until independently reviewed; transcription cannot send, publish, execute, or approve work"},
		Scope:        "Operator-triggered local transcription for an explicitly configured whisper-audio source only. Audio remains local and is not retained by this bridge; HAI stores the returned transcript as source evidence.",
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
	endpoint := s.endpoint("/healthz")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("could not create local whisper.cpp health request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "HAI-WhisperCPP/1.0")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("local whisper.cpp runner is unavailable")
	}
	defer resp.Body.Close()
	var body struct {
		Status     string `json:"status"`
		Engine     string `json:"engine"`
		Configured bool   `json:"configured"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 4097)).Decode(&body) != nil || body.Status != "ok" || !validEngine(body.Engine) {
		return nil, fmt.Errorf("local whisper.cpp runner did not pass health probe")
	}
	return &ProbeResult{Reachable: true, Engine: body.Engine, Configured: body.Configured, CheckedAt: s.now().UTC(), Scope: "Endpoint reachability only. A configured model and selected source folder are still required before any transcript can be created."}, nil
}

func (s *service) Transcribe(ctx context.Context, folder string) ([]Transcript, error) {
	if !s.configured() {
		return nil, ErrNotConfigured
	}
	folder, err := normalizeFolder(folder)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(struct {
		Folder string `json:"folder"`
	}{Folder: folder})
	if err != nil {
		return nil, fmt.Errorf("could not create local transcription request")
	}
	endpoint := s.endpoint("/v1/transcribe")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("could not create local transcription request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "HAI-WhisperCPP/1.0")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("local whisper.cpp runner is unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("local whisper.cpp runner returned an unsuccessful response")
	}
	var bodyResult struct {
		Status      string       `json:"status"`
		Transcripts []Transcript `json:"transcripts"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&bodyResult) != nil || bodyResult.Status != "completed" {
		return nil, fmt.Errorf("local whisper.cpp runner returned invalid transcript metadata")
	}
	if len(bodyResult.Transcripts) > maxTranscripts {
		return nil, fmt.Errorf("local whisper.cpp runner exceeded transcript limit")
	}
	for i := range bodyResult.Transcripts {
		if err := validateTranscript(&bodyResult.Transcripts[i]); err != nil {
			return nil, err
		}
	}
	return bodyResult.Transcripts, nil
}

func (s *service) configured() bool { return s.enabled && s.configErr == "" && s.baseURL != nil }
func (s *service) endpoint(value string) url.URL {
	endpoint := *s.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + value
	return endpoint
}

func normalizeFolder(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || value == "." || strings.HasPrefix(value, "/") || strings.Contains(value, "//") {
		return "", fmt.Errorf("an explicit relative selected audio folder is required")
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || len(cleaned) > 400 {
		return "", fmt.Errorf("audio folder must stay inside the selected intake root")
	}
	return cleaned, nil
}

func validateTranscript(value *Transcript) error {
	if value == nil {
		return fmt.Errorf("local whisper.cpp runner returned invalid transcript metadata")
	}
	path, err := normalizeFolder(value.Path)
	if err != nil {
		return fmt.Errorf("local whisper.cpp runner returned an invalid transcript path")
	}
	value.Path = path
	value.Text = strings.TrimSpace(value.Text)
	if value.Text == "" || len(value.Text) > maxTranscriptChars {
		return fmt.Errorf("local whisper.cpp runner returned an invalid transcript")
	}
	if !validToken(value.ModelID, 160) || !validToken(value.Language, 32) {
		return fmt.Errorf("local whisper.cpp runner returned invalid model metadata")
	}
	return nil
}

func validEngine(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), "whisper.cpp ") && len(value) <= 160
}
func validToken(value string, max int) bool {
	if len(value) == 0 || len(value) > max {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._:/-", r)) {
			return false
		}
	}
	return true
}

func parseLocalBaseURL(raw string) (*url.URL, string) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, baseURLEnv + " must be a plain local HTTP(S) URL without credentials, query data, or fragments"
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "localhost" && host != "host.docker.internal" && host != "whispercpp-runner" && net.ParseIP(host) == nil {
		return nil, baseURLEnv + " must resolve to localhost, host.docker.internal, whispercpp-runner, or a literal local IP"
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() && !ip.IsPrivate() {
		return nil, baseURLEnv + " must use a loopback or private-network IP"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed, ""
}
