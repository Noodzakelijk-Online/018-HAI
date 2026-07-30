// Package miniswe exposes a deliberately narrow, disposable patch-proposal
// bridge for mini-SWE-agent. It is not a host coding agent: source snapshots
// are copied by an isolated runner, results are diff-only, and HAI never
// applies, commits, pushes, or persists generated source content.
package miniswe

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/workflow"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	enabledEnv    = "HAI_MINISWE_ENABLED"
	runnerURLEnv  = "HAI_MINISWE_RUNNER_URL"
	tokenEnv      = "HAI_MINISWE_RUNNER_TOKEN"
	workspacesEnv = "HAI_MINISWE_WORKSPACES"
	timeoutEnv    = "HAI_MINISWE_TIMEOUT_SECONDS"
	maxTaskChars  = 4000
	maxDiffBytes  = 200 << 10
	maxResponse   = 256 << 10
	// The disposable worker receives a bind-mounted input root. Limiting that
	// root to one configured snapshot prevents a proposal agent from reading a
	// sibling workspace during an approved run.
	maxWorkspaces  = 1
	defaultTimeout = 300 * time.Second
)

var (
	ErrNotConfigured    = errors.New("mini-SWE patch proposal runner is not configured")
	ErrUnavailable      = errors.New("mini-SWE patch proposal runner is unavailable")
	ErrApprovalRequired = errors.New("workflow needs an explicit approved high-risk review before a patch can be proposed")
	ErrWorkflowNotReady = errors.New("workflow must be ready before a patch can be proposed")
	ErrWorkspaceDenied  = errors.New("workspace is not an approved disposable source snapshot")
	ErrInvalidRequest   = errors.New("invalid mini-SWE patch proposal request")
	workspacePattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
)

type Status struct {
	Enabled      bool     `json:"enabled"`
	Configured   bool     `json:"configured"`
	Provider     string   `json:"provider"`
	Endpoint     string   `json:"endpoint,omitempty"`
	Workspaces   []string `json:"workspaces"`
	ConfigError  string   `json:"configError,omitempty"`
	Capabilities []string `json:"capabilities"`
	Restrictions []string `json:"restrictions"`
	Scope        string   `json:"scope"`
}

type ProbeResult struct {
	Reachable bool      `json:"reachable"`
	Engine    string    `json:"engine,omitempty"`
	ModelID   string    `json:"modelId,omitempty"`
	CheckedAt time.Time `json:"checkedAt"`
	Scope     string    `json:"scope"`
}

type Job struct {
	ID            string     `json:"id"`
	WorkflowID    string     `json:"workflowId"`
	WorkspaceID   string     `json:"workspaceId"`
	Status        string     `json:"status"`
	Summary       string     `json:"summary"`
	DiffDigest    string     `json:"diffDigest,omitempty"`
	ChangedFiles  int        `json:"changedFiles"`
	DiffTruncated bool       `json:"diffTruncated"`
	CreatedAt     time.Time  `json:"createdAt"`
	CompletedAt   *time.Time `json:"completedAt,omitempty"`
}

// Proposal is intentionally returned only for the initiating request. The
// persisted Job excludes Diff so source content cannot be replayed from HAI's
// database or routine audit screens.
type Proposal struct {
	Job
	Diff               string `json:"diff,omitempty"`
	WorkflowLinkStatus string `json:"workflowLinkStatus,omitempty"`
	WorkflowLinkError  string `json:"workflowLinkError,omitempty"`
}

type Repository interface {
	Create(*models.MiniSWEPatchProposal) error
	Save(*models.MiniSWEPatchProposal) error
	ListForOwner(ownerIdentity string, limit int) ([]models.MiniSWEPatchProposal, error)
}

type WorkflowLookup interface {
	GetForOwner(ownerIdentity string, id uuid.UUID) (*workflow.WorkflowRecord, error)
}

// WorkflowPatchLinker accepts only an opaque patch-proposal reference. It must
// not receive the generated diff because that remains response-only.
type WorkflowPatchLinker interface {
	AttachMiniSWEPatchProposal(ownerIdentity, workflowID, proposalID, workspaceID, diffDigest string, changedFiles int) error
}

type Config struct {
	Enabled    bool
	RunnerURL  string
	Token      string
	Workspaces []string
	Timeout    time.Duration
}

type Service interface {
	Status() Status
	Probe(context.Context) (*ProbeResult, error)
	ProposePatch(context.Context, string, uuid.UUID, string) (*Proposal, error)
	Jobs(string, int) ([]Job, error)
}

// ModelMaintenanceGate is the narrow policy boundary required by the isolated
// runner. The canonical LLM service may satisfy it without this package
// depending on a concrete maintenance implementation.
type ModelMaintenanceGate interface {
	EnsureMiniSWEOllamaModel(endpointURL, modelID string) error
}

type service struct {
	repo            Repository
	workflows       WorkflowLookup
	enabled         bool
	runnerURL       *url.URL
	token           string
	workspaces      map[string]bool
	configErr       string
	client          *http.Client
	now             func() time.Time
	maintenanceGate ModelMaintenanceGate
}

type gormRepository struct{ db *gorm.DB }

func (r *gormRepository) Create(record *models.MiniSWEPatchProposal) error {
	return r.db.Create(record).Error
}
func (r *gormRepository) Save(record *models.MiniSWEPatchProposal) error {
	return r.db.Save(record).Error
}
func (r *gormRepository) ListForOwner(ownerIdentity string, limit int) ([]models.MiniSWEPatchProposal, error) {
	var records []models.MiniSWEPatchProposal
	err := r.db.Where("owner_identity = ?", ownerIdentity).Order("created_at DESC").Limit(limit).Find(&records).Error
	return records, err
}

func DefaultService(workflows WorkflowLookup) Service {
	db, err := infra.GetDefaultDB()
	if err != nil {
		panic(err)
	}
	return NewService(&gormRepository{db: db}, workflows, ConfigFromEnv(), nil)
}

func ConfigFromEnv() Config {
	timeout := defaultTimeout
	if raw := strings.TrimSpace(os.Getenv(timeoutEnv)); raw != "" {
		if seconds, err := time.ParseDuration(raw + "s"); err == nil && seconds >= 30*time.Second && seconds <= 5*time.Minute {
			timeout = seconds
		}
	}
	return Config{
		Enabled:    strings.EqualFold(strings.TrimSpace(os.Getenv(enabledEnv)), "true"),
		RunnerURL:  os.Getenv(runnerURLEnv),
		Token:      os.Getenv(tokenEnv),
		Workspaces: parseWorkspaces(os.Getenv(workspacesEnv)),
		Timeout:    timeout,
	}
}

func NewService(repo Repository, workflows WorkflowLookup, config Config, client *http.Client) Service {
	if config.Timeout < 30*time.Second || config.Timeout > 5*time.Minute {
		config.Timeout = defaultTimeout
	}
	if client == nil {
		client = &http.Client{Timeout: config.Timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }, Transport: &http.Transport{Proxy: nil}}
	}
	s := &service{repo: repo, workflows: workflows, enabled: config.Enabled, token: strings.TrimSpace(config.Token), workspaces: make(map[string]bool), client: client, now: time.Now}
	for _, workspace := range config.Workspaces {
		s.workspaces[workspace] = true
	}
	if s.enabled {
		s.runnerURL, s.configErr = parseRunnerURL(config.RunnerURL)
		if s.configErr == "" && len(s.token) < 16 {
			s.configErr = tokenEnv + " must contain a separate local-only token with at least 16 characters"
		}
		if s.configErr == "" && len(s.workspaces) == 0 {
			s.configErr = workspacesEnv + " requires at least one reviewed source snapshot name"
		}
		if s.configErr == "" && !validWorkspaceConfig(config.Workspaces) {
			s.configErr = workspacesEnv + " contains an invalid or duplicate source snapshot name"
		}
		if s.configErr == "" && workflows == nil {
			s.configErr = "workflow approval lookup is required"
		}
	}
	return s
}

// WithModelMaintenance binds mini-SWE's separate disposable Ollama volume to
// the same durable maintenance history used by normal HAI model routing.
func WithModelMaintenance(delegate Service, gate ModelMaintenanceGate) Service {
	if configured, ok := delegate.(*service); ok {
		configured.maintenanceGate = gate
	}
	return delegate
}

func (s *service) Status() Status {
	workspaces := make([]string, 0, len(s.workspaces))
	for workspace := range s.workspaces {
		workspaces = append(workspaces, workspace)
	}
	sort.Strings(workspaces)
	return Status{
		Enabled: s.enabled, Configured: s.configured(), Provider: "mini-SWE-agent disposable patch proposal runner", Endpoint: endpointString(s.runnerURL), Workspaces: workspaces, ConfigError: s.configErr,
		Capabilities: []string{"one approval-gated patch proposal against a selected disposable local source snapshot", "bounded unified diff and digest", "runner and local model readiness probe"},
		Restrictions: []string{"no host filesystem, original checkout, Git metadata, credentials, Docker socket, external network, apply, commit, push, or automatic retry", "no browser-provided task text or source files", "generated diff is not persisted and requires independent human review before any apply"},
		Scope:        "An owner can request one mini-SWE patch proposal only from an already approved, ready workflow and an allowlisted sanitized source snapshot. The isolated runner copies the snapshot to temporary storage and HAI returns a review-only diff; HAI retains all source, workflow, approval, audit, verification, and execution authority.",
	}
}

func (s *service) Probe(ctx context.Context) (*ProbeResult, error) {
	if !s.configured() {
		return nil, ErrNotConfigured
	}
	endpoint := s.endpoint("/v1/probe")
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), nil)
	if err != nil {
		return nil, ErrUnavailable
	}
	request.Header.Set("X-HAI-MiniSWE-Token", s.token)
	request.Header.Set("Accept", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer response.Body.Close()
	var body struct {
		Status string `json:"status"`
		Engine string `json:"engine"`
		Model  string `json:"modelId"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&body) != nil || body.Status != "ok" || !validBounded(body.Engine, 160) || !validBounded(body.Model, 160) {
		return nil, ErrUnavailable
	}
	return &ProbeResult{Reachable: true, Engine: body.Engine, ModelID: body.Model, CheckedAt: s.now().UTC(), Scope: "Runner and preloaded local model reachability only. The probe does not copy a source snapshot, start mini-SWE, or create a patch proposal."}, nil
}

func (s *service) ProposePatch(ctx context.Context, ownerIdentity string, workflowID uuid.UUID, workspaceID string) (*Proposal, error) {
	if !s.configured() {
		return nil, ErrNotConfigured
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	workspaceID = strings.TrimSpace(workspaceID)
	if ownerIdentity == "" || workflowID == uuid.Nil || !s.workspaces[workspaceID] {
		if workspaceID != "" && !s.workspaces[workspaceID] {
			return nil, ErrWorkspaceDenied
		}
		return nil, ErrInvalidRequest
	}
	record, err := s.workflows.GetForOwner(ownerIdentity, workflowID)
	if err != nil || record == nil {
		return nil, ErrInvalidRequest
	}
	item := record.Item
	if !item.RequiresApproval || item.ApprovalStatus != "approved" {
		return nil, ErrApprovalRequired
	}
	if item.CurrentState != workflow.StateReady {
		return nil, ErrWorkflowNotReady
	}
	task := workflowTask(item)
	if task == "" {
		return nil, ErrInvalidRequest
	}
	if err := s.ensureMaintainedModel(ctx); err != nil {
		return nil, fmt.Errorf("mini-SWE model maintenance: %w", err)
	}
	now := s.now().UTC()
	job := &models.MiniSWEPatchProposal{ID: uuid.New(), OwnerIdentity: ownerIdentity, WorkflowID: workflowID, WorkspaceID: workspaceID, Status: "running", Summary: "approved workflow was copied to an isolated disposable patch proposal workspace", CreatedAt: now}
	if err := s.repo.Create(job); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]string{"workspaceId": workspaceID, "task": task})
	if err != nil {
		return s.finish(job, "failed", "could not encode the bounded patch proposal request", "", 0, false, err)
	}
	endpoint := s.endpoint("/v1/propose-patch")
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return s.finish(job, "failed", "could not create the local patch proposal request", "", 0, false, ErrUnavailable)
	}
	request.Header.Set("X-HAI-MiniSWE-Token", s.token)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return s.finish(job, "failed", "local mini-SWE runner is unavailable", "", 0, false, ErrUnavailable)
	}
	defer response.Body.Close()
	var body struct {
		Status       string `json:"status"`
		Summary      string `json:"summary"`
		Diff         string `json:"diff"`
		DiffDigest   string `json:"diffDigest"`
		ChangedFiles int    `json:"changedFiles"`
		Truncated    bool   `json:"truncated"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, maxResponse)).Decode(&body) != nil || !validRunnerResponse(body.Status, body.Summary, body.Diff, body.DiffDigest, body.ChangedFiles) {
		return s.finish(job, "failed", "local mini-SWE runner returned an invalid patch proposal", "", 0, false, ErrUnavailable)
	}
	if body.Status == "failed" {
		return s.finish(job, "failed", body.Summary, "", 0, false, ErrUnavailable)
	}
	if body.Truncated {
		return s.finish(job, "failed", "local mini-SWE output exceeded the complete-review limit; no patch proposal was retained", "", 0, false, ErrUnavailable)
	}
	proposal, err := s.finish(job, "completed", body.Summary, body.Diff, body.ChangedFiles, body.Truncated, nil)
	if err != nil || proposal == nil {
		return proposal, err
	}
	return s.linkProposal(ownerIdentity, proposal)
}

func (s *service) linkProposal(ownerIdentity string, proposal *Proposal) (*Proposal, error) {
	linker, ok := s.workflows.(WorkflowPatchLinker)
	if !ok {
		proposal.WorkflowLinkStatus = "not_linked"
		proposal.WorkflowLinkError = "workflow proposal linkage is unavailable"
		return proposal, nil
	}
	if err := linker.AttachMiniSWEPatchProposal(ownerIdentity, proposal.WorkflowID, proposal.ID, proposal.WorkspaceID, proposal.DiffDigest, proposal.ChangedFiles); err != nil {
		proposal.WorkflowLinkStatus = "link_failed"
		proposal.WorkflowLinkError = "patch proposal completed but could not be linked to the requested workflow"
		return proposal, nil
	}
	proposal.WorkflowLinkStatus = "linked_review_signal"
	return proposal, nil
}

func (s *service) ensureMaintainedModel(ctx context.Context) error {
	if s.maintenanceGate == nil {
		return fmt.Errorf("%w: central daily model maintenance gate is unavailable", ErrUnavailable)
	}
	endpoint := s.endpoint("/healthz")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return ErrUnavailable
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "HAI-MiniSWE/1.0")
	response, err := s.client.Do(request)
	if err != nil {
		return ErrUnavailable
	}
	defer response.Body.Close()
	var body struct {
		Status        string `json:"status"`
		Configured    bool   `json:"configured"`
		ModelID       string `json:"modelId"`
		ModelEndpoint string `json:"modelEndpoint"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&body) != nil || body.Status != "ok" || !body.Configured || !validBounded(body.ModelID, 160) || !validBounded(body.ModelEndpoint, 256) {
		return ErrUnavailable
	}
	if err := s.maintenanceGate.EnsureMiniSWEOllamaModel(body.ModelEndpoint, body.ModelID); err != nil {
		return fmt.Errorf("%w: mini-SWE model maintenance could not admit the configured model", ErrUnavailable)
	}
	return nil
}

func (s *service) finish(job *models.MiniSWEPatchProposal, status, summary, diff string, changedFiles int, truncated bool, err error) (*Proposal, error) {
	now := s.now().UTC()
	job.Status = status
	job.Summary = bounded(summary, 512)
	job.ChangedFiles = changedFiles
	job.DiffTruncated = truncated
	if diff != "" {
		digest := sha256.Sum256([]byte(diff))
		job.DiffDigest = hex.EncodeToString(digest[:])
	}
	job.CompletedAt = &now
	if saveErr := s.repo.Save(job); saveErr != nil && err == nil {
		err = saveErr
	}
	return &Proposal{Job: toJob(*job), Diff: diff}, err
}

func (s *service) Jobs(ownerIdentity string, limit int) ([]Job, error) {
	if strings.TrimSpace(ownerIdentity) == "" {
		return nil, ErrInvalidRequest
	}
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	records, err := s.repo.ListForOwner(strings.TrimSpace(ownerIdentity), limit)
	if err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(records))
	for _, record := range records {
		jobs = append(jobs, toJob(record))
	}
	return jobs, nil
}

func toJob(record models.MiniSWEPatchProposal) Job {
	return Job{ID: record.ID.String(), WorkflowID: record.WorkflowID.String(), WorkspaceID: record.WorkspaceID, Status: record.Status, Summary: record.Summary, DiffDigest: record.DiffDigest, ChangedFiles: record.ChangedFiles, DiffTruncated: record.DiffTruncated, CreatedAt: record.CreatedAt, CompletedAt: record.CompletedAt}
}

func (s *service) configured() bool { return s.enabled && s.configErr == "" && s.runnerURL != nil }
func (s *service) endpoint(path string) url.URL {
	endpoint := *s.runnerURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + path
	return endpoint
}

func endpointString(endpoint *url.URL) string {
	if endpoint == nil {
		return ""
	}
	return endpoint.String()
}

func parseRunnerURL(raw string) (*url.URL, string) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, runnerURLEnv + " must be a plain local http URL without credentials, query data, or fragments"
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "miniswe-runner" && host != "localhost" && host != "host.docker.internal" {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return nil, runnerURLEnv + " may only target miniswe-runner, localhost, host.docker.internal, or a loopback IP"
		}
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed, ""
}

func parseWorkspaces(raw string) []string {
	result := make([]string, 0, maxWorkspaces)
	for _, value := range strings.Split(raw, ",") {
		workspace := strings.TrimSpace(value)
		if workspace == "" {
			continue
		}
		result = append(result, workspace)
	}
	return result
}

func validWorkspaceConfig(workspaces []string) bool {
	if len(workspaces) == 0 || len(workspaces) > maxWorkspaces {
		return false
	}
	seen := map[string]bool{}
	for _, workspace := range workspaces {
		if !workspacePattern.MatchString(workspace) || seen[workspace] {
			return false
		}
		seen[workspace] = true
	}
	return true
}

func workflowTask(item models.WorkflowItem) string {
	parts := []string{strings.TrimSpace(item.Title), strings.TrimSpace(item.Description)}
	task := strings.Join(parts, "\n\n")
	task = strings.Join(strings.Fields(task), " ")
	if !validBounded(task, maxTaskChars) {
		return ""
	}
	return task
}

func validRunnerResponse(status, summary, diff, digest string, changedFiles int) bool {
	if status != "completed" && status != "failed" || !validBounded(summary, 512) || changedFiles < 0 || changedFiles > 2000 {
		return false
	}
	if status == "failed" {
		return diff == "" && digest == ""
	}
	if len(diff) > maxDiffBytes || !validDigest(digest) {
		return false
	}
	if changedFiles > 0 && diff == "" {
		return false
	}
	calculated := sha256.Sum256([]byte(diff))
	return digest == hex.EncodeToString(calculated[:])
}

func validDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validBounded(value string, limit int) bool {
	value = strings.TrimSpace(value)
	return value != "" && utf8.RuneCountInString(value) <= limit && !strings.ContainsAny(value, "\r\n")
}

func bounded(value string, limit int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if utf8.RuneCountInString(value) > limit {
		return string([]rune(value)[:limit])
	}
	return value
}
