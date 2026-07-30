package temporalbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/workflow"

	"github.com/google/uuid"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	temporalworkflow "go.temporal.io/sdk/workflow"
)

const (
	enabledEnv   = "HAI_TEMPORAL_ENABLED"
	addressEnv   = "HAI_TEMPORAL_ADDRESS"
	namespaceEnv = "HAI_TEMPORAL_NAMESPACE"
	queueEnv     = "HAI_TEMPORAL_TASK_QUEUE"

	followUpWorkflowType = "hai.governed-follow-up-check.v1"
	maxRunAhead          = 365 * 24 * time.Hour
	maxFollowUpLimit     = 50
)

var (
	ErrNotConfigured = errors.New("Temporal durable workflow bridge is not configured")
	ErrUnavailable   = errors.New("Temporal durable workflow service is unavailable")
)

type Status struct {
	Enabled       bool   `json:"enabled"`
	Configured    bool   `json:"configured"`
	WorkerStarted bool   `json:"workerStarted"`
	Address       string `json:"address,omitempty"`
	Namespace     string `json:"namespace,omitempty"`
	TaskQueue     string `json:"taskQueue,omitempty"`
	ConfigError   string `json:"configError,omitempty"`
	WorkerError   string `json:"workerError,omitempty"`
	Scope         string `json:"scope"`
}

type FollowUpRequest struct {
	RunAt time.Time `json:"runAt"`
	Limit int       `json:"limit,omitempty"`
}

type FollowUpRun struct {
	ID                 uuid.UUID      `json:"id"`
	TemporalWorkflowID string         `json:"temporalWorkflowId"`
	WorkflowType       string         `json:"workflowType"`
	Status             string         `json:"status"`
	ScheduledFor       time.Time      `json:"scheduledFor"`
	StartedAt          *time.Time     `json:"startedAt,omitempty"`
	CompletedAt        *time.Time     `json:"completedAt,omitempty"`
	Summary            string         `json:"summary"`
	Result             FollowUpResult `json:"result"`
	CreatedAt          time.Time      `json:"createdAt"`
	UpdatedAt          time.Time      `json:"updatedAt"`
}

// FollowUpInput contains only opaque HAI run and scheduling metadata. HAI's
// own database remains the source of workflow context, ownership, and approval
// decisions.
type FollowUpInput struct {
	RunID string    `json:"runId"`
	RunAt time.Time `json:"runAt"`
	Limit int       `json:"limit"`
}

type FollowUpResult struct {
	Checked   int    `json:"checked"`
	Triggered int    `json:"triggered"`
	Resolved  int    `json:"resolved"`
	Skipped   int    `json:"skipped"`
	Summary   string `json:"summary"`
}

type config struct {
	enabled   bool
	address   string
	namespace string
	queue     string
}

type Service struct {
	config    config
	configErr string
	repo      Repository
	workflows workflow.Service
	now       func() time.Time
	dial      func(client.Options) (client.Client, error)
	mu        sync.RWMutex
	client    client.Client
	worker    worker.Worker
	workerErr string
}

func NewService(repo Repository, workflows workflow.Service, enabled bool, address, namespace, queue string) *Service {
	s := &Service{
		config: config{
			enabled:   enabled,
			address:   strings.TrimSpace(address),
			namespace: strings.TrimSpace(namespace),
			queue:     strings.TrimSpace(queue),
		},
		repo:      repo,
		workflows: workflows,
		now:       time.Now,
		dial:      client.Dial,
	}
	if s.config.enabled {
		s.configErr = validateConfig(s.config)
	}
	return s
}

func NewServiceFromEnv(workflows workflow.Service) *Service {
	return NewService(
		DefaultRepository(),
		workflows,
		strings.EqualFold(strings.TrimSpace(os.Getenv(enabledEnv)), "true"),
		strings.TrimSpace(os.Getenv(addressEnv)),
		strings.TrimSpace(os.Getenv(namespaceEnv)),
		strings.TrimSpace(os.Getenv(queueEnv)),
	)
}

func (s *Service) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Status{
		Enabled:       s.config.enabled,
		Configured:    s.config.enabled && s.configErr == "",
		WorkerStarted: s.worker != nil,
		Address:       s.config.address,
		Namespace:     s.config.namespace,
		TaskQueue:     s.config.queue,
		ConfigError:   s.configErr,
		WorkerError:   s.workerErr,
		Scope:         "Opt-in local durable follow-up checks only. A run creates HAI follow-up proposals; it cannot send messages, execute tools, or bypass approvals.",
	}
}

// StartWorker launches exactly one worker for this process. A failed launch is
// surfaced through Status; it never prevents HAI's regular API from starting.
func (s *Service) StartWorker() {
	if !s.config.enabled || s.configErr != "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.worker != nil {
		return
	}
	s.workerErr = ""
	clientValue, err := s.dial(client.Options{HostPort: s.config.address, Namespace: s.config.namespace})
	if err != nil {
		s.workerErr = "could not connect to the configured local Temporal service"
		return
	}
	healthCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_, err = clientValue.CheckHealth(healthCtx, &client.CheckHealthRequest{})
	cancel()
	if err != nil {
		clientValue.Close()
		s.workerErr = "configured local Temporal service is not healthy yet"
		return
	}
	activity := &followUpActivity{repo: s.repo, workflows: s.workflows, now: s.now}
	workerValue := worker.New(clientValue, s.config.queue, worker.Options{})
	workerValue.RegisterWorkflow(GovernedFollowUpWorkflow)
	workerValue.RegisterActivity(activity.Run)
	if err := workerValue.Start(); err != nil {
		clientValue.Close()
		s.workerErr = "could not start the governed follow-up worker"
		return
	}
	s.client = clientValue
	s.worker = workerValue
}

// StartWorkerEventually handles Compose ordering without turning an unavailable
// local scheduler into an application-startup failure. It makes at most thirty
// local connection attempts, then leaves the explicit admin retry route
// available. It never schedules or executes a follow-up by itself.
func (s *Service) StartWorkerEventually(ctx context.Context) {
	if !s.config.enabled || s.configErr != "" {
		return
	}
	go func() {
		for attempt := 0; attempt < 30; attempt++ {
			s.StartWorker()
			if s.Status().WorkerStarted {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}()
}

func (s *Service) ScheduleFollowUp(ctx context.Context, ownerIdentity string, request FollowUpRequest) (*FollowUpRun, error) {
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	if ownerIdentity == "" {
		return nil, fmt.Errorf("owner identity is required")
	}
	if err := validateRequest(s.now().UTC(), request); err != nil {
		return nil, err
	}
	if !s.config.enabled || s.configErr != "" {
		return nil, ErrNotConfigured
	}
	s.mu.RLock()
	clientValue := s.client
	workerStarted := s.worker != nil
	s.mu.RUnlock()
	if clientValue == nil || !workerStarted {
		return nil, ErrUnavailable
	}

	now := s.now().UTC()
	workflowID := "hai-follow-up-" + uuid.NewString()
	record := &models.TemporalWorkflowRun{
		OwnerIdentity:      ownerIdentity,
		TemporalWorkflowID: workflowID,
		WorkflowType:       followUpWorkflowType,
		Status:             "scheduled",
		ScheduledFor:       request.RunAt.UTC(),
		Summary:            "governed follow-up check is scheduled; it will create proposals only",
		ResultJSON:         "{}",
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	stored, err := s.repo.Create(record)
	if err != nil {
		return nil, err
	}
	_, err = clientValue.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: s.config.queue,
	}, GovernedFollowUpWorkflow, FollowUpInput{
		RunID: stored.ID.String(),
		RunAt: request.RunAt.UTC(),
		Limit: normalizeLimit(request.Limit),
	})
	if err != nil {
		stored.Status = "failed"
		stored.Summary = "Temporal rejected the governed follow-up schedule"
		stored.UpdatedAt = s.now().UTC()
		_, _ = s.repo.Update(stored)
		return nil, ErrUnavailable
	}
	run := runFromModel(*stored)
	return &run, nil
}

func (s *Service) Runs(ownerIdentity string, limit int) ([]FollowUpRun, error) {
	records, err := s.repo.ListForOwner(strings.TrimSpace(ownerIdentity), limit)
	if err != nil {
		return nil, err
	}
	runs := make([]FollowUpRun, 0, len(records))
	for _, record := range records {
		runs = append(runs, runFromModel(record))
	}
	return runs, nil
}

func GovernedFollowUpWorkflow(ctx temporalworkflow.Context, input FollowUpInput) (FollowUpResult, error) {
	if wait := input.RunAt.Sub(temporalworkflow.Now(ctx)); wait > 0 {
		if err := temporalworkflow.Sleep(ctx, wait); err != nil {
			return FollowUpResult{}, err
		}
	}
	options := temporalworkflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy:         &temporal.RetryPolicy{InitialInterval: time.Second, MaximumAttempts: 3},
	}
	ctx = temporalworkflow.WithActivityOptions(ctx, options)
	var result FollowUpResult
	if err := temporalworkflow.ExecuteActivity(ctx, "Run").Get(ctx, &result); err != nil {
		return FollowUpResult{}, err
	}
	return result, nil
}

type followUpActivity struct {
	repo      Repository
	workflows workflow.Service
	now       func() time.Time
}

func (a *followUpActivity) Run(_ context.Context, input FollowUpInput) (FollowUpResult, error) {
	if a.repo == nil || a.workflows == nil {
		return FollowUpResult{}, errors.New("governed follow-up activity is not initialized")
	}
	// The durable run ID is the only record key supplied to the activity. The
	// owner identity stays in HAI's own database and never enters Temporal.
	runID, parseErr := uuid.Parse(input.RunID)
	if parseErr != nil {
		return FollowUpResult{}, errors.New("invalid governed follow-up run")
	}
	result, err := a.runByID(runID, input.Limit)
	if err != nil {
		return FollowUpResult{}, err
	}
	return result, nil
}

func (a *followUpActivity) runByID(runID uuid.UUID, limit int) (FollowUpResult, error) {
	record, err := a.repo.FindByID(runID)
	if err != nil {
		return FollowUpResult{}, err
	}
	if record.Status == "completed" {
		return runFromModel(*record).Result, nil
	}
	ownerIdentity := strings.TrimSpace(record.OwnerIdentity)
	if ownerIdentity == "" || record.WorkflowType != followUpWorkflowType {
		return FollowUpResult{}, errors.New("invalid governed follow-up run record")
	}
	now := a.now().UTC()
	record.Status = "running"
	record.StartedAt = &now
	record.Summary = "governed follow-up check is running through HAI controls"
	record.UpdatedAt = now
	if _, err := a.repo.Update(record); err != nil {
		return FollowUpResult{}, err
	}
	summary, err := a.workflows.RunDueOpenLoopsForOwner(ownerIdentity, workflow.RunDueRequest{Limit: normalizeLimit(limit)})
	if err != nil {
		record.Status = "failed"
		record.Summary = "HAI follow-up proposal generation failed"
		record.UpdatedAt = a.now().UTC()
		_, _ = a.repo.Update(record)
		return FollowUpResult{}, err
	}
	result := FollowUpResult{Checked: summary.Checked, Triggered: summary.Triggered, Resolved: summary.Resolved, Skipped: summary.Skipped,
		Summary: fmt.Sprintf("HAI checked %d due open loops and created %d follow-up proposals", summary.Checked, summary.Triggered)}
	encoded, _ := json.Marshal(result)
	completed := a.now().UTC()
	record.Status = "completed"
	record.CompletedAt = &completed
	record.Summary = result.Summary
	record.ResultJSON = string(encoded)
	record.UpdatedAt = completed
	if _, err := a.repo.Update(record); err != nil {
		return FollowUpResult{}, err
	}
	return result, nil
}

func validateConfig(value config) string {
	if value.address == "" || value.namespace == "" || value.queue == "" {
		return "HAI_TEMPORAL_ADDRESS, HAI_TEMPORAL_NAMESPACE, and HAI_TEMPORAL_TASK_QUEUE are required when HAI_TEMPORAL_ENABLED=true"
	}
	host, port, err := net.SplitHostPort(value.address)
	if err != nil || port == "" {
		return "HAI_TEMPORAL_ADDRESS must be a local host:port value"
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "localhost" || host == "host.docker.internal" || host == "temporal" {
		return ""
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return ""
	}
	return "HAI_TEMPORAL_ADDRESS may only target localhost, loopback IPs, host.docker.internal, or the local temporal service"
}

func validateRequest(now time.Time, request FollowUpRequest) error {
	if request.RunAt.IsZero() {
		return fmt.Errorf("runAt is required")
	}
	if request.RunAt.Before(now.Add(-time.Minute)) || request.RunAt.After(now.Add(maxRunAhead)) {
		return fmt.Errorf("runAt must be within the next 365 days")
	}
	if request.Limit < 0 || request.Limit > maxFollowUpLimit {
		return fmt.Errorf("limit must be between 1 and %d", maxFollowUpLimit)
	}
	return nil
}

func normalizeLimit(value int) int {
	if value <= 0 {
		return 10
	}
	if value > maxFollowUpLimit {
		return maxFollowUpLimit
	}
	return value
}

func runFromModel(record models.TemporalWorkflowRun) FollowUpRun {
	var result FollowUpResult
	_ = json.Unmarshal([]byte(record.ResultJSON), &result)
	return FollowUpRun{ID: record.ID, TemporalWorkflowID: record.TemporalWorkflowID, WorkflowType: record.WorkflowType, Status: record.Status, ScheduledFor: record.ScheduledFor, StartedAt: record.StartedAt, CompletedAt: record.CompletedAt, Summary: record.Summary, Result: result, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
}
