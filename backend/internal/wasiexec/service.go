// Package wasiexec is HAI's narrow Wasmtime bridge. It admits only declared,
// content-addressed modules and never gives a module host filesystem, network,
// arguments, or environment variables.
package wasiexec

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrNotConfigured = errors.New("WASI execution is not configured")
	ErrUnavailable   = errors.New("local WASI runner is unavailable")
)

type Module struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
}
type Status struct {
	Enabled     bool     `json:"enabled"`
	Configured  bool     `json:"configured"`
	Modules     []Module `json:"modules"`
	ConfigError string   `json:"configError,omitempty"`
	Scope       string   `json:"scope"`
}
type Run struct {
	ID          string     `json:"id"`
	ModuleID    string     `json:"moduleId"`
	Status      string     `json:"status"`
	Summary     string     `json:"summary"`
	ExitCode    int        `json:"exitCode"`
	CreatedAt   time.Time  `json:"createdAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}
type repo struct{ db *gorm.DB }
type service struct {
	enabled   bool
	runner    string
	token     string
	modules   []Module
	configErr string
	repo      *repo
	client    *http.Client
	now       func() time.Time
}

func DefaultService() *service {
	var modules []Module
	_ = json.Unmarshal([]byte(os.Getenv("HAI_WASI_MODULES")), &modules)
	db, err := infra.GetDefaultDB()
	if err != nil {
		panic(err)
	}
	return NewService(&repo{db}, strings.EqualFold(strings.TrimSpace(os.Getenv("HAI_WASI_ENABLED")), "true"), os.Getenv("HAI_WASI_RUNNER_URL"), os.Getenv("HAI_WASI_RUNNER_TOKEN"), modules)
}
func NewService(r *repo, enabled bool, runner, token string, modules []Module) *service {
	s := &service{enabled: enabled, runner: strings.TrimRight(strings.TrimSpace(runner), "/"), token: strings.TrimSpace(token), modules: modules, repo: r, now: time.Now, client: &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{Proxy: nil}}}
	if enabled {
		s.configErr = validate(s)
	}
	return s
}
func (s *service) Status() Status {
	return Status{Enabled: s.enabled, Configured: s.enabled && s.configErr == "", Modules: append([]Module(nil), s.modules...), ConfigError: s.configErr, Scope: "Manifest-approved, content-addressed local WASI modules only. No inherited filesystem, network, environment, arguments, or module input/output retention."}
}
func (s *service) Modules() []Module { return append([]Module(nil), s.modules...) }
func (s *service) Run(ctx context.Context, owner, id string) (*Run, error) {
	if owner == "" {
		return nil, errors.New("owner identity is required")
	}
	if !s.enabled || s.configErr != "" {
		return nil, ErrNotConfigured
	}
	var module *Module
	for i := range s.modules {
		if s.modules[i].ID == id {
			module = &s.modules[i]
			break
		}
	}
	if module == nil {
		return nil, errors.New("WASI module is not admitted")
	}
	now := s.now().UTC()
	record := &models.WASIRun{ID: uuid.New(), OwnerIdentity: owner, ModuleID: module.ID, ModuleSHA256: module.SHA256, Status: "running", Summary: "admitted local WASI module is running", CreatedAt: now}
	if err := s.repo.db.Create(record).Error; err != nil {
		return nil, err
	}
	payload, _ := json.Marshal(module)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.runner+"/run", bytes.NewReader(payload))
	req.Header.Set("X-HAI-WASI-Token", s.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := s.client.Do(req)
	if err != nil {
		return s.finish(record, "failed", "local WASI runner is unavailable", -1, ErrUnavailable)
	}
	defer res.Body.Close()
	var body struct {
		Status   string `json:"status"`
		Summary  string `json:"summary"`
		ExitCode int    `json:"exitCode"`
	}
	if res.StatusCode != 200 || json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&body) != nil || (body.Status != "completed" && body.Status != "failed") {
		return s.finish(record, "failed", "local WASI runner returned an invalid result", -1, ErrUnavailable)
	}
	return s.finish(record, body.Status, bounded(body.Summary, 240), body.ExitCode, nil)
}
func (s *service) finish(r *models.WASIRun, status, summary string, code int, err error) (*Run, error) {
	now := s.now().UTC()
	r.Status = status
	r.Summary = summary
	r.ExitCode = code
	r.CompletedAt = &now
	_ = s.repo.db.Save(r).Error
	out := toRun(*r)
	return &out, err
}
func (s *service) Runs(owner string, limit int) ([]Run, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	var records []models.WASIRun
	err := s.repo.db.Where("owner_identity = ?", owner).Order("created_at DESC").Limit(limit).Find(&records).Error
	out := make([]Run, 0, len(records))
	for _, r := range records {
		out = append(out, toRun(r))
	}
	return out, err
}
func toRun(r models.WASIRun) Run {
	return Run{ID: r.ID.String(), ModuleID: r.ModuleID, Status: r.Status, Summary: r.Summary, ExitCode: r.ExitCode, CreatedAt: r.CreatedAt, CompletedAt: r.CompletedAt}
}
func validate(s *service) string {
	if s.runner == "" || len(s.token) < 16 || len(s.modules) == 0 {
		return "HAI_WASI_RUNNER_URL, a 16+ character HAI_WASI_RUNNER_TOKEN, and HAI_WASI_MODULES are required when HAI_WASI_ENABLED=true"
	}
	u, err := url.Parse(s.runner)
	if err != nil || u.Scheme != "http" || u.Host == "" {
		return "HAI_WASI_RUNNER_URL must be a local http URL"
	}
	host := strings.ToLower(u.Hostname())
	if host != "wasi-runner" && host != "localhost" && host != "host.docker.internal" && net.ParseIP(host) == nil {
		return "HAI_WASI_RUNNER_URL may only target the local wasi-runner, localhost, host.docker.internal, or a loopback IP"
	}
	seen := map[string]bool{}
	for _, m := range s.modules {
		if m.ID == "" || seen[m.ID] || m.Name == "" || m.File != strings.TrimSpace(m.File) || strings.ContainsAny(m.File, "/\\") || !strings.HasSuffix(m.File, ".wasm") || !validHash(m.SHA256) {
			return "every WASI module needs a unique id, name, basename .wasm file, and SHA-256"
		}
		seen[m.ID] = true
	}
	return ""
}
func validHash(v string) bool {
	if len(v) != 64 {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil
}
func bounded(v string, n int) string {
	v = strings.TrimSpace(v)
	if len(v) > n {
		return v[:n]
	}
	return v
}
func Digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
