package durablejob

import (
	"context"
	"fmt"
	"sync"
	"time"

	"automation-hub-backend/internal/backoff"
	"automation-hub-backend/internal/models"
)

// Job is the unit of work handed to a Handler. Aliased so callers registering
// handlers do not need to import the models package.
type Job = models.DurableJob

// Handler executes one job. It must be idempotent: delivery is at-least-once,
// so a handler can run twice if a worker dies after the side effect but before
// the status write.
type Handler func(ctx context.Context, job Job) error

// DefaultLease is how long a claimed job may be held before another worker is
// allowed to assume the holder died and reclaim it.
const DefaultLease = 5 * time.Minute

// Runner claims due jobs, executes their handler, and applies the retry policy.
// It is safe to run several Runners (in one process or many) against the same
// queue: claiming uses FOR UPDATE SKIP LOCKED.
type Runner struct {
	repo     Repository
	policy   backoff.Policy
	workerID string
	lease    time.Duration
	batch    int
	// now is injectable so retry scheduling is deterministic in tests.
	now func() time.Time

	mu       sync.RWMutex
	handlers map[string]Handler
}

// Options configures a Runner. Zero values fall back to sane defaults.
type Options struct {
	WorkerID string
	Policy   backoff.Policy
	Lease    time.Duration
	Batch    int
	Now      func() time.Time
}

// NewRunner builds a Runner over the given repository.
func NewRunner(repo Repository, opts Options) *Runner {
	if opts.WorkerID == "" {
		opts.WorkerID = fmt.Sprintf("worker-%d", time.Now().UnixNano())
	}
	if opts.Policy == (backoff.Policy{}) {
		opts.Policy = backoff.DefaultPolicy()
	}
	if opts.Lease <= 0 {
		opts.Lease = DefaultLease
	}
	if opts.Batch <= 0 {
		opts.Batch = 10
	}
	if opts.Now == nil {
		opts.Now = func() time.Time { return time.Now().UTC() }
	}
	return &Runner{
		repo:     repo,
		policy:   opts.Policy,
		workerID: opts.WorkerID,
		lease:    opts.Lease,
		batch:    opts.Batch,
		now:      opts.Now,
		handlers: map[string]Handler{},
	}
}

// Register binds a handler to a job kind. Registering an existing kind replaces it.
func (r *Runner) Register(kind string, handler Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[kind] = handler
}

func (r *Runner) handlerFor(kind string) (Handler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	handler, ok := r.handlers[kind]
	return handler, ok
}

// Enqueue schedules a job. runAt zero means "run as soon as possible".
func (r *Runner) Enqueue(kind, payload string, runAt time.Time, maxAttempts int) (*models.DurableJob, error) {
	if runAt.IsZero() {
		runAt = r.now()
	}
	return r.repo.Enqueue(&models.DurableJob{
		Kind:        kind,
		Payload:     payload,
		RunAt:       runAt.UTC(),
		MaxAttempts: maxAttempts,
		Status:      models.DurableJobPending,
	})
}

// EnsureScheduled enqueues a job of the given kind only when none is already
// pending or running. Recurring work (a periodic scan that re-enqueues itself)
// uses this at startup so restarts do not pile up duplicate schedules.
// It reports whether a new job was created.
func (r *Runner) EnsureScheduled(kind, payload string, runAt time.Time, maxAttempts int) (bool, error) {
	active, err := r.repo.CountActiveByKind(kind)
	if err != nil {
		return false, fmt.Errorf("count active %s jobs: %w", kind, err)
	}
	if active > 0 {
		return false, nil
	}
	if _, err := r.Enqueue(kind, payload, runAt, maxAttempts); err != nil {
		return false, err
	}
	return true, nil
}

// RunOnce performs a single durable cycle: recover leases abandoned by dead
// workers, claim the due batch, and execute it. It returns how many jobs were
// processed. Handler failures are recorded as retries/dead-letters, not
// returned, so one bad job cannot stall the queue.
func (r *Runner) RunOnce(ctx context.Context) (int, error) {
	now := r.now()
	if _, err := r.repo.ReapExpiredLeases(now, r.lease); err != nil {
		return 0, fmt.Errorf("reap expired leases: %w", err)
	}
	jobs, err := r.repo.ClaimDue(r.workerID, now, r.batch)
	if err != nil {
		return 0, fmt.Errorf("claim due jobs: %w", err)
	}
	processed := 0
	for _, job := range jobs {
		r.execute(ctx, job)
		processed++
	}
	return processed, nil
}

// execute runs one claimed job and records the outcome.
func (r *Runner) execute(ctx context.Context, job models.DurableJob) {
	attempt := job.Attempts + 1
	handler, ok := r.handlerFor(job.Kind)
	if !ok {
		// An unregistered kind is a deployment error, not a transient fault:
		// fail it straight to the dead letter rather than retrying forever.
		_ = r.repo.MarkDead(job.ID, r.now(), attempt, fmt.Sprintf("no handler registered for kind %q", job.Kind))
		return
	}

	err := r.safeInvoke(ctx, handler, job)
	if err == nil {
		_ = r.repo.MarkSucceeded(job.ID, r.now())
		return
	}
	if attempt >= job.MaxAttempts {
		_ = r.repo.MarkDead(job.ID, r.now(), attempt, err.Error())
		return
	}
	retryAt := r.now().Add(r.policy.Delay(attempt))
	_ = r.repo.MarkForRetry(job.ID, retryAt, attempt, err.Error())
}

// safeInvoke turns a panicking handler into a normal error so one bad job can
// never take down the worker process.
func (r *Runner) safeInvoke(ctx context.Context, handler Handler, job models.DurableJob) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("handler panicked: %v", recovered)
		}
	}()
	return handler(ctx, job)
}

// Start polls the queue until the context is cancelled. This is the long-running
// worker loop; call it from a goroutine at startup.
func (r *Runner) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := r.RunOnce(ctx); err != nil {
				// A repository error is transient (e.g. DB blip); the next tick retries.
				continue
			}
		}
	}
}
