// Package hostruntimereconcile projects completed Windows host-runtime work
// into HAI's immutable automation audit ledger. It never executes host work.
package hostruntimereconcile

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"automation-hub-backend/internal/hostruntime"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/safety"

	"github.com/google/uuid"
)

const completionEventPrefix = "host-runtime-completion:"

type HostJobs interface {
	CompletedUnreconciled(limit int) ([]hostruntime.Job, error)
	MarkReconciled(id uuid.UUID) (bool, error)
}

type AutomationLedger interface {
	FindLaunchEventByExecutionReference(reference string) (*models.AutomationLaunchEvent, error)
	FindByID(id uuid.UUID) (*models.Automation, error)
	Update(automation *models.Automation) (*models.Automation, error)
	SaveLaunchEvent(event *models.AutomationLaunchEvent) error
}

type Service struct {
	hostJobs HostJobs
	ledger   AutomationLedger
	now      func() time.Time
}

func NewService(hostJobs HostJobs, ledger AutomationLedger) *Service {
	return &Service{hostJobs: hostJobs, ledger: ledger, now: func() time.Time { return time.Now().UTC() }}
}

// ReconcileCompleted writes an idempotent terminal event for every completed
// host job. Unknown jobs are retained and retried; they are never silently
// treated as automation completions.
func (s *Service) ReconcileCompleted(limit int) (int, error) {
	if s == nil || s.hostJobs == nil || s.ledger == nil {
		return 0, fmt.Errorf("host runtime reconciliation dependencies are unavailable")
	}
	jobs, err := s.hostJobs.CompletedUnreconciled(limit)
	if err != nil {
		return 0, fmt.Errorf("list unreconciled host runtime jobs: %w", err)
	}
	completed := 0
	var failures []error
	for _, job := range jobs {
		if err := s.reconcile(job); err != nil {
			failures = append(failures, err)
			continue
		}
		completed++
	}
	return completed, errors.Join(failures...)
}

func (s *Service) reconcile(job hostruntime.Job) error {
	launch, err := s.ledger.FindLaunchEventByExecutionReference(job.ID.String())
	if err != nil {
		return fmt.Errorf("find queued automation launch for host job %s: %w", job.ID, err)
	}
	if launch.AutomationID == uuid.Nil || launch.RuntimeTaskID != job.TaskID || launch.RuntimeType != job.RuntimeID {
		return fmt.Errorf("host job %s does not match its queued automation launch", job.ID)
	}
	automation, err := s.ledger.FindByID(launch.AutomationID)
	if err != nil {
		return fmt.Errorf("find automation for host job %s: %w", job.ID, err)
	}
	status := "completed"
	message := "Windows host runtime completed the approved task"
	exitCode := 0
	if job.ExitCode != nil {
		exitCode = *job.ExitCode
	}
	if exitCode != 0 || strings.TrimSpace(job.Error) != "" {
		status = "failed"
		message = firstNonEmpty(job.Error, "Windows host runtime failed the approved task")
	}
	finished := s.now().UTC()
	if job.CompletedAt != nil {
		finished = job.CompletedAt.UTC()
	}
	event := &models.AutomationLaunchEvent{
		ID:                 uuid.New(),
		AutomationID:       automation.ID,
		OwnerIdentity:      job.OwnerIdentity,
		RuntimeType:        job.RuntimeID,
		LaunchType:         "agent_runtime_host_completion",
		RuntimeTaskID:      job.TaskID,
		ExecutionReference: job.ID.String(),
		EventKey:           completionEventPrefix + job.ID.String(),
		Target:             "host-runtime://" + job.RuntimeID,
		Status:             status,
		Message:            safety.RedactSecrets(message),
		Output:             safety.RedactSecrets(job.Output),
		AuditEvents: []string{
			"Windows host-runtime job completion reconciled into the immutable automation ledger",
			"host runtime job " + job.ID.String(),
			"no completion was inferred before the bridge submitted its terminal result",
		},
		ExitCode:    exitCode,
		DurationMs:  durationSince(job.CreatedAt, finished),
		StartedAt:   job.CreatedAt.UTC(),
		CompletedAt: finished,
	}
	if err := s.ledger.SaveLaunchEvent(event); err != nil {
		return fmt.Errorf("persist host completion event for job %s: %w", job.ID, err)
	}
	if status == "completed" {
		automation.LastSuccessAt = &finished
		automation.LastFailureReason = ""
	} else {
		automation.LastFailureAt = &finished
		automation.LastFailureReason = safety.RedactSecrets(message)
	}
	if _, err := s.ledger.Update(automation); err != nil {
		return fmt.Errorf("update automation terminal state for host job %s: %w", job.ID, err)
	}
	if _, err := s.hostJobs.MarkReconciled(job.ID); err != nil {
		return fmt.Errorf("mark host job %s reconciled: %w", job.ID, err)
	}
	return nil
}

func durationSince(start, end time.Time) int64 {
	if start.IsZero() || end.Before(start) {
		return 0
	}
	return end.Sub(start).Milliseconds()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
