package durablejob

import (
	"context"
	"errors"
	"testing"
	"time"

	"automation-hub-backend/internal/backoff"
	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
)

// fakeRepo is an in-memory Repository so retry, lease, and scheduling logic is
// verifiable without a database.
type fakeRepo struct {
	jobs map[uuid.UUID]*models.DurableJob
}

func newFakeRepo() *fakeRepo { return &fakeRepo{jobs: map[uuid.UUID]*models.DurableJob{}} }

func (f *fakeRepo) Enqueue(job *models.DurableJob) (*models.DurableJob, error) {
	if job.ID == uuid.Nil {
		job.ID = uuid.New()
	}
	if job.Status == "" {
		job.Status = models.DurableJobPending
	}
	if job.MaxAttempts <= 0 {
		job.MaxAttempts = 5
	}
	copyJob := *job
	f.jobs[job.ID] = &copyJob
	return &copyJob, nil
}

func (f *fakeRepo) ClaimDue(workerID string, now time.Time, limit int) ([]models.DurableJob, error) {
	claimed := []models.DurableJob{}
	for _, job := range f.jobs {
		if len(claimed) >= limit {
			break
		}
		if job.Status != models.DurableJobPending || job.RunAt.After(now) {
			continue
		}
		job.Status = models.DurableJobRunning
		job.LockedBy = workerID
		lockedAt := now
		job.LockedAt = &lockedAt
		claimed = append(claimed, *job)
	}
	return claimed, nil
}

func (f *fakeRepo) MarkSucceeded(id uuid.UUID, now time.Time) error {
	job := f.jobs[id]
	job.Status = models.DurableJobSucceeded
	job.CompletedAt = &now
	job.LockedBy = ""
	job.LockedAt = nil
	return nil
}

func (f *fakeRepo) MarkForRetry(id uuid.UUID, runAt time.Time, attempts int, lastErr string) error {
	job := f.jobs[id]
	job.Status = models.DurableJobPending
	job.RunAt = runAt
	job.Attempts = attempts
	job.LastError = lastErr
	job.LockedBy = ""
	job.LockedAt = nil
	return nil
}

func (f *fakeRepo) MarkDead(id uuid.UUID, now time.Time, attempts int, lastErr string) error {
	job := f.jobs[id]
	job.Status = models.DurableJobDead
	job.Attempts = attempts
	job.LastError = lastErr
	job.CompletedAt = &now
	job.LockedBy = ""
	job.LockedAt = nil
	return nil
}

func (f *fakeRepo) ReapExpiredLeases(now time.Time, lease time.Duration) (int, error) {
	cutoff := now.Add(-lease)
	reaped := 0
	for _, job := range f.jobs {
		if job.Status == models.DurableJobRunning && job.LockedAt != nil && job.LockedAt.Before(cutoff) {
			job.Status = models.DurableJobPending
			job.LockedBy = ""
			job.LockedAt = nil
			reaped++
		}
	}
	return reaped, nil
}

func (f *fakeRepo) Find(id uuid.UUID) (*models.DurableJob, error) {
	job, ok := f.jobs[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return job, nil
}

// fixedClock returns a controllable clock for deterministic retry scheduling.
func fixedClock(t *time.Time) func() time.Time { return func() time.Time { return *t } }

func TestRunnerExecutesJobAndMarksSucceeded(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	runner := NewRunner(repo, Options{WorkerID: "w1", Now: fixedClock(&now)})

	ran := false
	runner.Register("demo", func(ctx context.Context, job models.DurableJob) error {
		ran = true
		return nil
	})
	job, _ := runner.Enqueue("demo", `{"x":1}`, now, 3)

	processed, err := runner.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if processed != 1 || !ran {
		t.Fatalf("processed=%d ran=%v, want 1/true", processed, ran)
	}
	stored, _ := repo.Find(job.ID)
	if stored.Status != models.DurableJobSucceeded {
		t.Fatalf("status = %q, want succeeded", stored.Status)
	}
}

func TestRunnerRetriesWithBackoffAndSurvivesRestart(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	policy := backoff.Policy{Base: time.Minute, Factor: 2, Max: time.Hour}
	runner := NewRunner(repo, Options{WorkerID: "w1", Policy: policy, Now: fixedClock(&now)})
	runner.Register("flaky", func(ctx context.Context, job models.DurableJob) error {
		return errors.New("boom")
	})
	job, _ := runner.Enqueue("flaky", "{}", now, 3)

	if _, err := runner.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	stored, _ := repo.Find(job.ID)
	if stored.Status != models.DurableJobPending || stored.Attempts != 1 {
		t.Fatalf("after failure: status=%q attempts=%d, want pending/1", stored.Status, stored.Attempts)
	}
	// Backoff must push RunAt into the future by the first delay (1m).
	if !stored.RunAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("RunAt = %v, want %v (backoff delay)", stored.RunAt, now.Add(time.Minute))
	}
	// Not yet due: a poll before RunAt must not claim it.
	if processed, _ := runner.RunOnce(context.Background()); processed != 0 {
		t.Fatalf("claimed a job before its RunAt; processed=%d", processed)
	}

	// Durability: a brand-new Runner (as if the process restarted) picks the job
	// up once it is due, because state lives in the repository, not in memory.
	now = now.Add(2 * time.Minute)
	restarted := NewRunner(repo, Options{WorkerID: "w2", Policy: policy, Now: fixedClock(&now)})
	restarted.Register("flaky", func(ctx context.Context, job models.DurableJob) error { return nil })
	if processed, _ := restarted.RunOnce(context.Background()); processed != 1 {
		t.Fatalf("restarted worker processed=%d, want 1", processed)
	}
	stored, _ = repo.Find(job.ID)
	if stored.Status != models.DurableJobSucceeded {
		t.Fatalf("status after restart = %q, want succeeded", stored.Status)
	}
}

func TestRunnerDeadLettersAfterMaxAttempts(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	runner := NewRunner(repo, Options{WorkerID: "w1", Policy: backoff.Policy{Base: 0, Factor: 1}, Now: fixedClock(&now)})
	runner.Register("always-fails", func(ctx context.Context, job models.DurableJob) error {
		return errors.New("nope")
	})
	job, _ := runner.Enqueue("always-fails", "{}", now, 2)

	for i := 0; i < 2; i++ {
		if _, err := runner.RunOnce(context.Background()); err != nil {
			t.Fatalf("RunOnce %d: %v", i, err)
		}
	}
	stored, _ := repo.Find(job.ID)
	if stored.Status != models.DurableJobDead {
		t.Fatalf("status = %q after exhausting attempts, want dead", stored.Status)
	}
	if stored.Attempts != 2 || stored.LastError == "" {
		t.Fatalf("attempts=%d lastErr=%q, want 2 and a recorded error", stored.Attempts, stored.LastError)
	}
}

func TestRunnerReclaimsJobAfterWorkerCrash(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	lease := 30 * time.Second

	// Simulate a crash: the job is claimed (running, leased) but never finished.
	job, _ := repo.Enqueue(&models.DurableJob{
		Kind: "demo", Payload: "{}", RunAt: now, MaxAttempts: 3, Status: models.DurableJobPending,
	})
	if _, err := repo.ClaimDue("dead-worker", now, 10); err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	if stored, _ := repo.Find(job.ID); stored.Status != models.DurableJobRunning {
		t.Fatalf("precondition: job should be running/leased, got %q", stored.Status)
	}

	// After the lease expires, a healthy worker must reclaim and complete it.
	now = now.Add(lease + time.Second)
	survivor := NewRunner(repo, Options{WorkerID: "w2", Lease: lease, Now: fixedClock(&now)})
	survivor.Register("demo", func(ctx context.Context, job models.DurableJob) error { return nil })

	processed, err := survivor.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if processed != 1 {
		t.Fatalf("processed=%d, want 1 (crashed worker's job reclaimed)", processed)
	}
	stored, _ := repo.Find(job.ID)
	if stored.Status != models.DurableJobSucceeded {
		t.Fatalf("status = %q, want succeeded after recovery", stored.Status)
	}
}

func TestRunnerDeadLettersUnknownKindAndSurvivesPanic(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	runner := NewRunner(repo, Options{WorkerID: "w1", Now: fixedClock(&now)})

	unknown, _ := runner.Enqueue("not-registered", "{}", now, 3)
	if _, err := runner.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	stored, _ := repo.Find(unknown.ID)
	if stored.Status != models.DurableJobDead {
		t.Fatalf("unknown kind status = %q, want dead", stored.Status)
	}

	// A panicking handler must be contained and retried, not crash the worker.
	runner.Register("panics", func(ctx context.Context, job models.DurableJob) error {
		panic("kaboom")
	})
	panicking, _ := runner.Enqueue("panics", "{}", now, 3)
	if _, err := runner.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce after panic: %v", err)
	}
	stored, _ = repo.Find(panicking.ID)
	if stored.Status != models.DurableJobPending || stored.Attempts != 1 {
		t.Fatalf("panicking job status=%q attempts=%d, want pending/1", stored.Status, stored.Attempts)
	}
	if stored.LastError == "" {
		t.Fatalf("expected the panic to be recorded as the job error")
	}
}
