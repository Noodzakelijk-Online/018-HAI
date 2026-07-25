// Package durablejob provides the durable worker model: background jobs that
// are persisted, scheduled, retried with backoff, and recovered when the worker
// process dies. It replaces "in-process only" scheduling for work that must not
// be lost across a restart.
package durablejob

import (
	"time"

	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repository is the persistence seam. The Gorm implementation below is used in
// production; tests use an in-memory fake so retry/lease logic is verifiable
// without a database.
type Repository interface {
	Enqueue(job *models.DurableJob) (*models.DurableJob, error)
	// ClaimDue atomically leases up to limit jobs that are due at now.
	ClaimDue(workerID string, now time.Time, limit int) ([]models.DurableJob, error)
	MarkSucceeded(id uuid.UUID, now time.Time) error
	// MarkForRetry returns a job to pending with a future RunAt.
	MarkForRetry(id uuid.UUID, runAt time.Time, attempts int, lastErr string) error
	MarkDead(id uuid.UUID, now time.Time, attempts int, lastErr string) error
	// ReapExpiredLeases returns jobs whose worker died back to pending.
	ReapExpiredLeases(now time.Time, lease time.Duration) (int, error)
	Find(id uuid.UUID) (*models.DurableJob, error)
	// CountActiveByKind counts jobs of a kind that are still pending or running.
	// Used to keep recurring work singleton across restarts.
	CountActiveByKind(kind string) (int64, error)
}

type gormRepository struct{ db *gorm.DB }

// NewGormRepository returns the Postgres-backed repository.
func NewGormRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

// DefaultRepository builds the repository over the default database.
func DefaultRepository() (Repository, error) {
	db, err := infra.GetDefaultDB()
	if err != nil {
		return nil, err
	}
	return NewGormRepository(db), nil
}

func (r *gormRepository) Enqueue(job *models.DurableJob) (*models.DurableJob, error) {
	if job.Queue == "" {
		job.Queue = "default"
	}
	if job.Status == "" {
		job.Status = models.DurableJobPending
	}
	if job.MaxAttempts <= 0 {
		job.MaxAttempts = 5
	}
	if job.RunAt.IsZero() {
		job.RunAt = time.Now().UTC()
	}
	if job.Payload == "" {
		job.Payload = "{}"
	}
	if err := r.db.Create(job).Error; err != nil {
		return nil, err
	}
	return job, nil
}

// ClaimDue uses SELECT ... FOR UPDATE SKIP LOCKED so multiple workers can poll
// the same queue concurrently without ever claiming the same job twice.
func (r *gormRepository) ClaimDue(workerID string, now time.Time, limit int) ([]models.DurableJob, error) {
	if limit <= 0 {
		limit = 10
	}
	claimed := []models.DurableJob{}
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var candidates []models.DurableJob
		if err := tx.Raw(`
			SELECT * FROM durable_jobs
			WHERE status = ? AND run_at <= ?
			ORDER BY run_at
			LIMIT ?
			FOR UPDATE SKIP LOCKED`,
			models.DurableJobPending, now, limit).Scan(&candidates).Error; err != nil {
			return err
		}
		for i := range candidates {
			if err := tx.Model(&models.DurableJob{}).
				Where("id = ?", candidates[i].ID).
				Updates(map[string]any{
					"status":    models.DurableJobRunning,
					"locked_by": workerID,
					"locked_at": now,
				}).Error; err != nil {
				return err
			}
			candidates[i].Status = models.DurableJobRunning
			candidates[i].LockedBy = workerID
			lockedAt := now
			candidates[i].LockedAt = &lockedAt
		}
		claimed = candidates
		return nil
	})
	return claimed, err
}

func (r *gormRepository) MarkSucceeded(id uuid.UUID, now time.Time) error {
	return r.db.Model(&models.DurableJob{}).Where("id = ?", id).Updates(map[string]any{
		"status":       models.DurableJobSucceeded,
		"completed_at": now,
		"locked_by":    "",
		"locked_at":    nil,
		"last_error":   "",
	}).Error
}

func (r *gormRepository) MarkForRetry(id uuid.UUID, runAt time.Time, attempts int, lastErr string) error {
	return r.db.Model(&models.DurableJob{}).Where("id = ?", id).Updates(map[string]any{
		"status":     models.DurableJobPending,
		"run_at":     runAt,
		"attempts":   attempts,
		"last_error": lastErr,
		"locked_by":  "",
		"locked_at":  nil,
	}).Error
}

func (r *gormRepository) MarkDead(id uuid.UUID, now time.Time, attempts int, lastErr string) error {
	return r.db.Model(&models.DurableJob{}).Where("id = ?", id).Updates(map[string]any{
		"status":       models.DurableJobDead,
		"attempts":     attempts,
		"last_error":   lastErr,
		"completed_at": now,
		"locked_by":    "",
		"locked_at":    nil,
	}).Error
}

// ReapExpiredLeases recovers jobs whose worker died while holding the lease.
func (r *gormRepository) ReapExpiredLeases(now time.Time, lease time.Duration) (int, error) {
	cutoff := now.Add(-lease)
	result := r.db.Model(&models.DurableJob{}).
		Where("status = ? AND locked_at IS NOT NULL AND locked_at < ?", models.DurableJobRunning, cutoff).
		Updates(map[string]any{
			"status":    models.DurableJobPending,
			"locked_by": "",
			"locked_at": nil,
		})
	return int(result.RowsAffected), result.Error
}

func (r *gormRepository) CountActiveByKind(kind string) (int64, error) {
	var count int64
	err := r.db.Model(&models.DurableJob{}).
		Where("kind = ? AND status IN ?", kind, []string{models.DurableJobPending, models.DurableJobRunning}).
		Count(&count).Error
	return count, err
}

func (r *gormRepository) Find(id uuid.UUID) (*models.DurableJob, error) {
	var job models.DurableJob
	if err := r.db.First(&job, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &job, nil
}
