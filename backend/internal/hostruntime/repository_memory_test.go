package hostruntime

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

type memoryRepository struct {
	mu   sync.Mutex
	jobs map[uuid.UUID]Job
}

func newMemoryRepository() *memoryRepository { return &memoryRepository{jobs: map[uuid.UUID]Job{}} }

func (r *memoryRepository) Create(job Job) (*Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.jobs[job.ID] = job
	return copyJob(job), nil
}

func (r *memoryRepository) Lease(workerID, runtimeID string, now, expires time.Time, digest string) (*Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, job := range r.jobs {
		if job.RuntimeID != runtimeID || (job.Status != StatusPending && (job.Status != StatusLeased || job.LeaseExpires == nil || job.LeaseExpires.After(now))) {
			continue
		}
		job.Status, job.WorkerID, job.LeaseDigest = StatusLeased, workerID, digest
		job.LeaseExpires = &expires
		r.jobs[id] = job
		return copyJob(job), nil
	}
	return nil, nil
}

func (r *memoryRepository) ConfirmLease(workerID string, id uuid.UUID, digest string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok || job.Status != StatusLeased || job.WorkerID != workerID || job.LeaseDigest != digest || job.LeaseExpires == nil || !job.LeaseExpires.After(now) {
		return ErrStaleLease
	}
	return nil
}

func (r *memoryRepository) Complete(workerID string, id uuid.UUID, digest string, completion Completion, now time.Time) (*Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok || job.Status != StatusLeased || job.WorkerID != workerID || job.LeaseDigest != digest || job.LeaseExpires == nil || !job.LeaseExpires.After(now) {
		return nil, ErrStaleLease
	}
	job.Status, job.Output, job.Error = StatusCompleted, completion.Output, completion.Error
	job.ExitCode, job.CompletedAt = &completion.ExitCode, &now
	job.LeaseDigest, job.LeaseExpires = "", nil
	r.jobs[id] = job
	return copyJob(job), nil
}

func (r *memoryRepository) ListCompletedUnreconciled(limit int) ([]Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	jobs := make([]Job, 0)
	for _, job := range r.jobs {
		if job.Status == StatusCompleted && job.ReconciledAt == nil {
			jobs = append(jobs, job)
			if limit > 0 && len(jobs) >= limit {
				break
			}
		}
	}
	return jobs, nil
}

func (r *memoryRepository) MarkReconciled(id uuid.UUID, at time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok || job.Status != StatusCompleted || job.ReconciledAt != nil {
		return false, nil
	}
	job.ReconciledAt = &at
	r.jobs[id] = job
	return true, nil
}

func copyJob(job Job) *Job { return &job }
