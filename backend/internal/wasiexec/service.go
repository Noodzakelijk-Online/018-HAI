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
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"automation-hub-backend/internal/executionauth"
	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/safety"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrNotConfigured            = errors.New("WASI execution is not configured")
	ErrUnavailable              = errors.New("local WASI runner is unavailable")
	ErrInvalidRunRequest        = errors.New("WASI execution request is incomplete")
	ErrAuthorizationUnavailable = errors.New("WASI final-effect authorization is unavailable")
	ErrNotAuthorized            = errors.New("WASI final effect is not authorized")
	ErrEmergencyStopActive      = errors.New("emergency stop blocks WASI execution")
)

const (
	wasiRunAction      = "wasi.module.run"
	wasiResourceType   = "wasi_module"
	wasiRuntimeID      = "wasi"
	wasiConsumer       = "wasi-runner"
	wasiAuthorityLevel = 6
	wasiAutonomyLevel  = 6
	finalEffectVersion = 1
)

type Module struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
}

// RunRequest contains references that bind an already approved task to one
// immutable WASI effect. Approval remains server-side: these are evidence
// identifiers, not caller assertions that approval occurred.
type RunRequest struct {
	ModuleID              string `json:"-"`
	TaskID                string `json:"taskId"`
	ProjectKey            string `json:"projectKey"`
	ApprovalSourceID      string `json:"approvalSourceId"`
	ApprovalBindingDigest string `json:"approvalBindingDigest"`
}

// FinalEffectAuthorizer is the only authority capable of admitting a WASI
// runner invocation. executionauth.Service satisfies this interface directly.
type FinalEffectAuthorizer interface {
	AuthorizeAndConsume(
		context.Context,
		executionauth.Request,
		string,
		string,
	) (executionauth.Receipt, error)
}

type finalEffectBinding struct {
	Version               int    `json:"version"`
	OwnerIdentity         string `json:"ownerIdentity"`
	Action                string `json:"action"`
	ResourceType          string `json:"resourceType"`
	ResourceID            string `json:"resourceId"`
	ModuleSHA256          string `json:"moduleSha256"`
	RuntimeID             string `json:"runtimeId"`
	TaskID                 string `json:"taskId"`
	ProjectKey             string `json:"projectKey"`
	ApprovalSourceID       string `json:"approvalSourceId"`
	ApprovalBindingDigest  string `json:"approvalBindingDigest"`
	RunnerEndpoint         string `json:"runnerEndpoint"`
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

type runRepository interface {
	Create(*models.WASIRun) error
	Save(*models.WASIRun) error
	ListForOwner(string, int) ([]models.WASIRun, error)
}

type repo struct{ db *gorm.DB }

func (r *repo) Create(run *models.WASIRun) error {
	return r.db.Create(run).Error
}

func (r *repo) Save(run *models.WASIRun) error {
	return r.db.Save(run).Error
}

func (r *repo) ListForOwner(owner string, limit int) ([]models.WASIRun, error) {
	var records []models.WASIRun
	err := r.db.Where("owner_identity = ?", owner).
		Order("created_at DESC").
		Limit(limit).
		Find(&records).Error
	return records, err
}

type service struct {
	enabled    bool
	runner     string
	token      string
	modules    []Module
	configErr  string
	repo       runRepository
	authorizer FinalEffectAuthorizer
	client     *http.Client
	now        func() time.Time
}

func DefaultService() *service {
	return DefaultServiceWithAuthorizer(nil)
}

// DefaultServiceWithAuthorizer creates the production service with an injected
// final-effect authority. Passing nil is deliberately fail closed.
func DefaultServiceWithAuthorizer(authorizer FinalEffectAuthorizer) *service {
	var modules []Module
	_ = json.Unmarshal([]byte(os.Getenv("HAI_WASI_MODULES")), &modules)
	db, err := infra.GetDefaultDB()
	if err != nil {
		panic(err)
	}
	return NewServiceWithAuthorizer(
		&repo{db},
		strings.EqualFold(strings.TrimSpace(os.Getenv("HAI_WASI_ENABLED")), "true"),
		os.Getenv("HAI_WASI_RUNNER_URL"),
		os.Getenv("HAI_WASI_RUNNER_TOKEN"),
		modules,
		authorizer,
	)
}

func NewService(r runRepository, enabled bool, runner, token string, modules []Module) *service {
	return NewServiceWithAuthorizer(r, enabled, runner, token, modules, nil)
}

func NewServiceWithAuthorizer(
	r runRepository,
	enabled bool,
	runner, token string,
	modules []Module,
	authorizer FinalEffectAuthorizer,
) *service {
	s := &service{
		enabled:    enabled,
		runner:     strings.TrimRight(strings.TrimSpace(runner), "/"),
		token:      strings.TrimSpace(token),
		modules:    modules,
		repo:       r,
		authorizer: authorizer,
		now:        time.Now,
		client: &http.Client{
			Timeout:   8 * time.Second,
			Transport: &http.Transport{Proxy: nil},
		},
	}
	if enabled {
		s.configErr = validate(s)
	}
	return s
}

func (s *service) Status() Status {
	return Status{
		Enabled:     s.enabled,
		Configured:  s.enabled && s.configErr == "",
		Modules:     append([]Module(nil), s.modules...),
		ConfigError: s.configErr,
		Scope:       "Manifest-approved, content-addressed local WASI modules only. No inherited filesystem, network, environment, arguments, or module input/output retention.",
	}
}

func (s *service) Modules() []Module {
	return append([]Module(nil), s.modules...)
}

func (s *service) Run(ctx context.Context, owner string, request RunRequest) (*Run, error) {
	owner = strings.TrimSpace(owner)
	request = normalizeRunRequest(request)
	if err := validateRunRequest(owner, request); err != nil {
		return nil, err
	}
	if !s.enabled || s.configErr != "" {
		return nil, ErrNotConfigured
	}
	if decision := safety.EvaluateEmergencyStop(); decision.Active {
		return nil, fmt.Errorf("%w: %s", ErrEmergencyStopActive, decision.Reason)
	}

	var module *Module
	for i := range s.modules {
		if s.modules[i].ID == request.ModuleID {
			module = &s.modules[i]
			break
		}
	}
	if module == nil {
		return nil, errors.New("WASI module is not admitted")
	}
	moduleValue := *module
	moduleValue.SHA256 = strings.ToLower(strings.TrimSpace(moduleValue.SHA256))

	now := s.now().UTC()
	record := &models.WASIRun{
		ID:            uuid.New(),
		OwnerIdentity: owner,
		ModuleID:      moduleValue.ID,
		ModuleSHA256:  moduleValue.SHA256,
		Status:        "authorizing",
		Summary:       "admitted local WASI module is awaiting final-effect authorization",
		CreatedAt:     now,
	}
	if s.repo == nil {
		return nil, ErrUnavailable
	}
	if err := s.repo.Create(record); err != nil {
		return nil, err
	}

	binding := buildFinalEffectBinding(owner, request, moduleValue, s.runner)
	effectDigest, err := finalEffectDigest(binding)
	if err != nil {
		return s.finish(record, "blocked", "WASI final-effect binding could not be encoded", -1, err)
	}
	authRequest := buildAuthorizationRequest(record.ID, binding, effectDigest)
	executionTarget := wasiConsumer + ":" + effectDigest

	payload, err := json.Marshal(moduleValue)
	if err != nil {
		return s.finish(record, "blocked", "WASI module payload could not be encoded", -1, err)
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		s.runner+"/run",
		bytes.NewReader(payload),
	)
	if err != nil {
		return s.finish(record, "blocked", "WASI runner request could not be created", -1, err)
	}
	req.Header.Set("X-HAI-WASI-Token", s.token)
	req.Header.Set("Content-Type", "application/json")

	// Consume authority for this exact effect immediately before invoking the
	// runner. No mutable work occurs after this point except the stop recheck.
	if s.authorizer == nil {
		return s.finish(
			record,
			"blocked",
			ErrAuthorizationUnavailable.Error(),
			-1,
			ErrAuthorizationUnavailable,
		)
	}
	receipt, err := s.authorizer.AuthorizeAndConsume(
		ctx,
		authRequest,
		wasiConsumer,
		executionTarget,
	)
	if err != nil {
		return s.finish(
			record,
			"blocked",
			"WASI final-effect authorization was denied",
			-1,
			fmt.Errorf("%w: %v", ErrNotAuthorized, err),
		)
	}
	if receipt.Outcome != executionauth.OutcomeAuthorized ||
		strings.TrimSpace(receipt.OwnerIdentity) != owner ||
		strings.TrimSpace(receipt.EffectDigest) != effectDigest {
		return s.finish(
			record,
			"blocked",
			"WASI final-effect authorization receipt did not match the requested effect",
			-1,
			ErrNotAuthorized,
		)
	}

	// A stop engaged while authorization was being consumed always wins.
	if decision := safety.EvaluateEmergencyStop(); decision.Active {
		return s.finish(
			record,
			"blocked",
			decision.Reason,
			-1,
			fmt.Errorf("%w: %s", ErrEmergencyStopActive, decision.Reason),
		)
	}

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
	if res.StatusCode != http.StatusOK ||
		json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&body) != nil ||
		(body.Status != "completed" && body.Status != "failed") {
		return s.finish(
			record,
			"failed",
			"local WASI runner returned an invalid result",
			-1,
			ErrUnavailable,
		)
	}
	return s.finish(record, body.Status, bounded(body.Summary, 240), body.ExitCode, nil)
}

func (s *service) finish(
	run *models.WASIRun,
	status, summary string,
	code int,
	err error,
) (*Run, error) {
	now := s.now().UTC()
	run.Status = status
	run.Summary = summary
	run.ExitCode = code
	run.CompletedAt = &now
	if saveErr := s.repo.Save(run); saveErr != nil && err == nil {
		err = saveErr
	}
	out := toRun(*run)
	return &out, err
}

func (s *service) Runs(owner string, limit int) ([]Run, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	if s.repo == nil {
		return nil, ErrUnavailable
	}
	records, err := s.repo.ListForOwner(strings.TrimSpace(owner), limit)
	out := make([]Run, 0, len(records))
	for _, record := range records {
		out = append(out, toRun(record))
	}
	return out, err
}

func toRun(run models.WASIRun) Run {
	return Run{
		ID:          run.ID.String(),
		ModuleID:    run.ModuleID,
		Status:      run.Status,
		Summary:     run.Summary,
		ExitCode:    run.ExitCode,
		CreatedAt:   run.CreatedAt,
		CompletedAt: run.CompletedAt,
	}
}

func normalizeRunRequest(request RunRequest) RunRequest {
	request.ModuleID = strings.TrimSpace(request.ModuleID)
	request.TaskID = strings.TrimSpace(request.TaskID)
	request.ProjectKey = strings.TrimSpace(request.ProjectKey)
	request.ApprovalSourceID = strings.TrimSpace(request.ApprovalSourceID)
	request.ApprovalBindingDigest = strings.ToLower(
		strings.TrimSpace(request.ApprovalBindingDigest),
	)
	return request
}

func validateRunRequest(owner string, request RunRequest) error {
	if owner == "" {
		return fmt.Errorf("%w: owner identity is required", ErrInvalidRunRequest)
	}
	for label, value := range map[string]string{
		"module id":       request.ModuleID,
		"task id":         request.TaskID,
		"project key":     request.ProjectKey,
		"approval source": request.ApprovalSourceID,
	} {
		if value == "" || len(value) > 256 || strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf(
				"%w: %s is required and must be bounded",
				ErrInvalidRunRequest,
				label,
			)
		}
	}
	if !validHash(request.ApprovalBindingDigest) ||
		request.ApprovalBindingDigest != strings.ToLower(request.ApprovalBindingDigest) {
		return fmt.Errorf(
			"%w: approval binding digest must be an exact lowercase SHA-256 digest",
			ErrInvalidRunRequest,
		)
	}
	return nil
}

func buildFinalEffectBinding(
	owner string,
	request RunRequest,
	module Module,
	runner string,
) finalEffectBinding {
	resourceID := module.ID + "@sha256:" + module.SHA256
	return finalEffectBinding{
		Version:               finalEffectVersion,
		OwnerIdentity:         owner,
		Action:                wasiRunAction,
		ResourceType:          wasiResourceType,
		ResourceID:            resourceID,
		ModuleSHA256:          module.SHA256,
		RuntimeID:             wasiRuntimeID,
		TaskID:                 request.TaskID,
		ProjectKey:             request.ProjectKey,
		ApprovalSourceID:       request.ApprovalSourceID,
		ApprovalBindingDigest:  request.ApprovalBindingDigest,
		RunnerEndpoint:         runner + "/run",
	}
}

func finalEffectDigest(binding finalEffectBinding) (string, error) {
	encoded, err := json.Marshal(binding)
	if err != nil {
		return "", err
	}
	return Digest(encoded), nil
}

func buildAuthorizationRequest(
	runID uuid.UUID,
	binding finalEffectBinding,
	effectDigest string,
) executionauth.Request {
	return executionauth.Request{
		OwnerIdentity:         binding.OwnerIdentity,
		IdempotencyKey:        "wasi-run:" + runID.String(),
		ActorIdentity:         binding.OwnerIdentity,
		ActorKind:             executionauth.ActorHuman,
		TaskID:                binding.TaskID,
		Action:                binding.Action,
		Stage:                 executionauth.StageExecution,
		ResourceType:          binding.ResourceType,
		ResourceID:            binding.ResourceID,
		ProjectKey:            binding.ProjectKey,
		ToolID:                wasiConsumer,
		RuntimeID:             binding.RuntimeID,
		RequiredAuthority:     wasiAuthorityLevel,
		RequestedAutonomy:     wasiAutonomyLevel,
		Risk:                  executionauth.RiskHigh,
		Reversible:            false,
		ApprovalSourceID:      binding.ApprovalSourceID,
		ApprovalBindingDigest: binding.ApprovalBindingDigest,
		EffectDigest:          effectDigest,
		Facts: map[string]string{
			"module_sha256":   binding.ModuleSHA256,
			"runner_endpoint": binding.RunnerEndpoint,
		},
		SourceReferences: []string{
			"wasi-module://" + binding.ResourceID,
			"task://" + binding.TaskID,
		},
	}
}

func validate(s *service) string {
	if s.runner == "" || len(s.token) < 16 || len(s.modules) == 0 {
		return "HAI_WASI_RUNNER_URL, a 16+ character HAI_WASI_RUNNER_TOKEN, and HAI_WASI_MODULES are required when HAI_WASI_ENABLED=true"
	}
	runnerURL, err := url.Parse(s.runner)
	if err != nil || runnerURL.Scheme != "http" || runnerURL.Host == "" {
		return "HAI_WASI_RUNNER_URL must be a local http URL"
	}
	host := strings.ToLower(runnerURL.Hostname())
	if !isLocalRunnerHost(host) {
		return "HAI_WASI_RUNNER_URL may only target the local wasi-runner, localhost, host.docker.internal, or a loopback IP"
	}
	seen := map[string]bool{}
	for _, module := range s.modules {
		if module.ID == "" ||
			seen[module.ID] ||
			module.Name == "" ||
			module.File != strings.TrimSpace(module.File) ||
			strings.ContainsAny(module.File, "/\\") ||
			!strings.HasSuffix(module.File, ".wasm") ||
			!validHash(module.SHA256) {
			return "every WASI module needs a unique id, name, basename .wasm file, and SHA-256"
		}
		seen[module.ID] = true
	}
	return ""
}

func isLocalRunnerHost(host string) bool {
	if host == "wasi-runner" || host == "localhost" || host == "host.docker.internal" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func validHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func bounded(value string, maximum int) string {
	value = strings.TrimSpace(value)
	if len(value) > maximum {
		return value[:maximum]
	}
	return value
}

func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
