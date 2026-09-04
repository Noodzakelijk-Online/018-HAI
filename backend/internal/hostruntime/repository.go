package hostruntime

import (
	"automation-hub-backend/internal/infra"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type gormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

func DefaultRepository() (Repository, error) {
	db, err := infra.GetDefaultDB()
	if err != nil {
		return nil, err
	}
	return NewGormRepository(db), nil
}

func (r *gormRepository) Create(job Job) (*Job, error) {
	if err := r.db.Create(&job).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

func (r *gormRepository) Lease(workerID, runtimeID string, now, expires time.Time, digest string) (*Job, error) {
	var leased *Job
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var job Job
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("(status = ? OR (status = ? AND lease_expires <= ?)) AND runtime_id = ?", StatusPending, StatusLeased, now, runtimeID).
			Order("created_at").
			First(&job).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil
			}
			return err
		}
		result := tx.Model(&Job{}).Where("id = ? AND (status = ? OR (status = ? AND lease_expires <= ?))", job.ID, StatusPending, StatusLeased, now).Updates(map[string]any{
			"status":        StatusLeased,
			"worker_id":     workerID,
			"lease_digest":  digest,
			"lease_expires": expires,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("host runtime job changed before lease")
		}
		job.Status, job.WorkerID, job.LeaseDigest = StatusLeased, workerID, digest
		job.LeaseExpires = &expires
		leased = &job
		return nil
	})
	return leased, err
}

func (r *gormRepository) ConfirmLease(workerID string, id uuid.UUID, digest string, now time.Time) error {
	var job Job
	err := r.db.
		Where("id = ? AND status = ? AND worker_id = ? AND lease_digest = ? AND lease_expires > ?", id, StatusLeased, workerID, digest, now).
		First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrStaleLease
	}
	return err
}

func (r *gormRepository) Complete(workerID string, id uuid.UUID, digest string, completion Completion, now time.Time) (*Job, error) {
	result := r.db.Model(&Job{}).
		Where("id = ? AND status = ? AND worker_id = ? AND lease_digest = ? AND lease_expires > ?", id, StatusLeased, workerID, digest, now).
		Updates(map[string]any{
			"status":        StatusCompleted,
			"lease_digest":  "",
			"lease_expires": nil,
			"output":        completion.Output,
			"error":         completion.Error,
			"exit_code":     completion.ExitCode,
			"completed_at":  now,
		})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, ErrStaleLease
	}
	var job Job
	if err := r.db.First(&job, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

func (r *gormRepository) ListCompletedUnreconciled(limit int) ([]Job, error) {
	if limit <= 0 {
		limit = 20
	}
	var jobs []Job
	err := r.db.
		Where("status = ? AND reconciled_at IS NULL", StatusCompleted).
		Order("completed_at ASC").
		Limit(limit).
		Find(&jobs).Error
	return jobs, err
}

func (r *gormRepository) MarkReconciled(id uuid.UUID, at time.Time) (bool, error) {
	result := r.db.Model(&Job{}).
		Where("id = ? AND status = ? AND reconciled_at IS NULL", id, StatusCompleted).
		Update("reconciled_at", at.UTC())
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}
