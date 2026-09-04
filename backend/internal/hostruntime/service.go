// Package hostruntime persists work that must be executed by a narrowly
// configured runtime on the local Windows host rather than by a container.
package hostruntime

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"automation-hub-backend/internal/safety"

	"github.com/google/uuid"
)

const (
	StatusPending   = "pending"
	StatusLeased    = "leased"
	StatusCompleted = "completed"
	StatusCancelled = "cancelled"

	maxPromptBytes = 50 * 1024
	maxResultBytes = 16 * 1024

	defaultLeaseSeconds    = 20 * 60
	minimumLeaseSeconds    = 60
	maximumLeaseSeconds    = 30 * 60
	defaultHarnessTimeout  = 120
	maximumHarnessTimeout  = 15 * 60
	completionGraceSeconds = 60
)

var (
	ErrInvalidTask      = errors.New("host runtime task is invalid")
	ErrStaleLease       = errors.New("host runtime lease is no longer valid")
	ErrEmergencyStopped = errors.New("host runtime lease blocked by emergency stop")
)

type ApprovedTask struct {
	OwnerIdentity string
	RuntimeID     string
	TaskID        string
	Prompt        string
	WorkspaceKey  string
	Approved      bool
}

type Completion struct {
	ExitCode int
	Output   string
	Error    string
}

// Dispatcher is the narrow capability agent-runtime adapters need in order to
// request host execution. It deliberately exposes no lease or completion
// operations, which remain private to the loopback Windows bridge.
type Dispatcher interface {
	Enqueue(ApprovedTask) (*Job, error)
}

type Job struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	OwnerIdentity string     `gorm:"type:text;not null;index" json:"ownerIdentity"`
	RuntimeID     string     `gorm:"type:text;not null;index" json:"runtimeId"`
	TaskID        string     `gorm:"type:text;not null;uniqueIndex" json:"taskId"`
	Prompt        string     `gorm:"type:text;not null" json:"prompt"`
	WorkspaceKey  string     `gorm:"type:text;not null" json:"workspaceKey"`
	Status        string     `gorm:"type:text;not null;index" json:"status"`
	WorkerID      string     `gorm:"type:text" json:"workerId,omitempty"`
	LeaseDigest   string     `gorm:"type:text" json:"-"`
	LeaseExpires  *time.Time `json:"leaseExpiresAt,omitempty"`
	Output        string     `gorm:"type:text" json:"output,omitempty"`
	Error         string     `gorm:"type:text" json:"error,omitempty"`
	ExitCode      *int       `json:"exitCode,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	CompletedAt   *time.Time `json:"completedAt,omitempty"`
	ReconciledAt  *time.Time `json:"reconciledAt,omitempty"`
}

func (Job) TableName() string { return "host_runtime_jobs" }

type Lease struct {
	Job   Job    `json:"job"`
	Token string `json:"leaseToken"`
}

type Repository interface {
	Create(Job) (*Job, error)
	Lease(workerID, runtimeID string, now, expires time.Time, digest string) (*Job, error)
	ConfirmLease(workerID string, id uuid.UUID, digest string, now time.Time) error
	Complete(workerID string, id uuid.UUID, digest string, completion Completion, now time.Time) (*Job, error)
	ListCompletedUnreconciled(limit int) ([]Job, error)
	MarkReconciled(id uuid.UUID, at time.Time) (bool, error)
}

type Service struct {
	repository Repository
	now        func() time.Time
	leaseFor   time.Duration
}

type Option func(*Service)

func WithClock(now func() time.Time) Option {
	return func(service *Service) {
		if now != nil {
			service.now = now
		}
	}
}

// WithLeaseDuration is primarily useful for tests and explicitly bounded
// deployments. Production construction uses configuredLeaseDuration so a
// Windows worker cannot outlive its own lease.
func WithLeaseDuration(duration time.Duration) Option {
	return func(service *Service) {
		if duration >= time.Duration(minimumLeaseSeconds)*time.Second &&
			duration <= time.Duration(maximumLeaseSeconds)*time.Second {
			service.leaseFor = duration
		}
	}
}

func NewService(repository Repository, options ...Option) *Service {
	service := &Service{repository: repository, now: func() time.Time { return time.Now().UTC() }, leaseFor: configuredLeaseDuration()}
	for _, option := range options {
		option(service)
	}
	return service
}

// configuredLeaseDuration reserves enough time for the configured Windows DSH
// process plus submission of its terminal result. A too-short operator value
// is raised rather than allowing an approved task to be leased a second time
// while its first execution is still running.
func configuredLeaseDuration() time.Duration {
	harnessTimeout := boundedEnvSeconds("DEEPSEEK_HARNESS_TIMEOUT_SECONDS", defaultHarnessTimeout, 1, maximumHarnessTimeout)
	configured := boundedEnvSeconds("HAI_HOST_RUNTIME_LEASE_SECONDS", defaultLeaseSeconds, minimumLeaseSeconds, maximumLeaseSeconds)
	minimum := harnessTimeout + completionGraceSeconds
	if configured < minimum {
		configured = minimum
	}
	return time.Duration(configured) * time.Second
}

func boundedEnvSeconds(name string, fallback, minimum, maximum int) int {
	seconds, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if err != nil || seconds < minimum || seconds > maximum {
		return fallback
	}
	return seconds
}

func (s *Service) Enqueue(task ApprovedTask) (*Job, error) {
	if s == nil || s.repository == nil || !task.Approved || !validTask(task) {
		return nil, ErrInvalidTask
	}
	job, err := s.repository.Create(Job{
		ID:            uuid.New(),
		OwnerIdentity: strings.TrimSpace(task.OwnerIdentity),
		RuntimeID:     strings.ToLower(strings.TrimSpace(task.RuntimeID)),
		TaskID:        strings.TrimSpace(task.TaskID),
		Prompt:        strings.TrimSpace(task.Prompt),
		WorkspaceKey:  strings.TrimSpace(task.WorkspaceKey),
		Status:        StatusPending,
	})
	if err != nil {
		return nil, fmt.Errorf("enqueue host runtime task: %w", err)
	}
	return job, nil
}

func (s *Service) Lease(workerID, runtimeID string) (*Lease, error) {
	if s == nil || s.repository == nil || !validIdentifier(workerID) || !validIdentifier(runtimeID) {
		return nil, ErrInvalidTask
	}
	// A durable job can wait in the queue while the operator activates the
	// emergency stop. Re-check at the lease boundary so already-queued work is
	// not converted into a new Windows process after the stop takes effect.
	if safety.EmergencyStopActive() {
		return nil, ErrEmergencyStopped
	}
	token, digest, err := randomLeaseToken()
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	job, err := s.repository.Lease(strings.TrimSpace(workerID), strings.ToLower(strings.TrimSpace(runtimeID)), now, now.Add(s.leaseFor), digest)
	if err != nil {
		return nil, fmt.Errorf("lease host runtime task: %w", err)
	}
	if job == nil {
		return nil, nil
	}
	return &Lease{Job: *job, Token: token}, nil
}

func (s *Service) Complete(workerID string, id uuid.UUID, token string, completion Completion) (*Job, error) {
	if s == nil || s.repository == nil || id == uuid.Nil || !validIdentifier(workerID) || strings.TrimSpace(token) == "" {
		return nil, ErrInvalidTask
	}
	job, err := s.repository.Complete(strings.TrimSpace(workerID), id, digestToken(token), sanitizeCompletion(completion), s.now().UTC())
	if errors.Is(err, ErrStaleLease) {
		return nil, ErrStaleLease
	}
	if err != nil {
		return nil, fmt.Errorf("complete host runtime task: %w", err)
	}
	return job, nil
}

// ConfirmLease is the final server-side gate immediately before the Windows
// bridge starts an external process. It closes the gap between leasing a task
// and launching it when an emergency stop or lease expiry occurs.
func (s *Service) ConfirmLease(workerID string, id uuid.UUID, token string) error {
	if s == nil || s.repository == nil || id == uuid.Nil || !validIdentifier(workerID) || strings.TrimSpace(token) == "" {
		return ErrInvalidTask
	}
	if safety.EmergencyStopActive() {
		return ErrEmergencyStopped
	}
	if err := s.repository.ConfirmLease(strings.TrimSpace(workerID), id, digestToken(token), s.now().UTC()); err != nil {
		if errors.Is(err, ErrStaleLease) {
			return ErrStaleLease
		}
		return fmt.Errorf("confirm host runtime lease: %w", err)
	}
	return nil
}

// CompletedUnreconciled exposes terminal host work to the audit reconciler.
// It cannot lease, complete, or execute work.
func (s *Service) CompletedUnreconciled(limit int) ([]Job, error) {
	if s == nil || s.repository == nil {
		return nil, ErrInvalidTask
	}
	return s.repository.ListCompletedUnreconciled(limit)
}

// MarkReconciled records that a host completion was projected into HAI's
// immutable automation audit ledger. It is safe for scheduler retries.
func (s *Service) MarkReconciled(id uuid.UUID) (bool, error) {
	if s == nil || s.repository == nil || id == uuid.Nil {
		return false, ErrInvalidTask
	}
	return s.repository.MarkReconciled(id, s.now().UTC())
}

func validTask(task ApprovedTask) bool {
	return validIdentifier(task.OwnerIdentity) && validIdentifier(task.RuntimeID) && validIdentifier(task.TaskID) &&
		validIdentifier(task.WorkspaceKey) && strings.TrimSpace(task.Prompt) != "" && len(task.Prompt) <= maxPromptBytes
}

func validIdentifier(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= 255 && !strings.ContainsAny(value, "\r\n\x00")
}

func randomLeaseToken() (string, string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", fmt.Errorf("generate host runtime lease token: %w", err)
	}
	token := hex.EncodeToString(bytes)
	return token, digestToken(token), nil
}

func digestToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func sanitizeCompletion(completion Completion) Completion {
	completion.Output = boundedRedacted(completion.Output)
	completion.Error = boundedRedacted(completion.Error)
	return completion
}

func boundedRedacted(value string) string {
	value = strings.TrimSpace(safety.RedactSecrets(value))
	if len(value) > maxResultBytes {
		return strings.TrimSpace(value[:maxResultBytes])
	}
	return value
}
