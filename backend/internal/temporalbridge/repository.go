// Package temporalbridge provides an opt-in, local Temporal bridge for one
// HAI-owned durable workflow. It never receives source content or external
// action payloads and only calls existing governed HAI services.
package temporalbridge

import (
	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Repository interface {
	Create(*models.TemporalWorkflowRun) (*models.TemporalWorkflowRun, error)
	Update(*models.TemporalWorkflowRun) (*models.TemporalWorkflowRun, error)
	FindByID(uuid.UUID) (*models.TemporalWorkflowRun, error)
	FindForOwner(string, string) (*models.TemporalWorkflowRun, error)
	ListForOwner(string, int) ([]models.TemporalWorkflowRun, error)
}

func (r *gormRepository) FindByID(id uuid.UUID) (*models.TemporalWorkflowRun, error) {
	var run models.TemporalWorkflowRun
	if err := r.db.First(&run, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &run, nil
}

type gormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

func DefaultRepository() Repository {
	db, err := infra.GetDefaultDB()
	if err != nil {
		panic(err)
	}
	return NewGormRepository(db)
}

func (r *gormRepository) Create(run *models.TemporalWorkflowRun) (*models.TemporalWorkflowRun, error) {
	if err := r.db.Create(run).Error; err != nil {
		return nil, err
	}
	return run, nil
}

func (r *gormRepository) Update(run *models.TemporalWorkflowRun) (*models.TemporalWorkflowRun, error) {
	if err := r.db.Save(run).Error; err != nil {
		return nil, err
	}
	return run, nil
}

func (r *gormRepository) FindForOwner(ownerIdentity, workflowID string) (*models.TemporalWorkflowRun, error) {
	var run models.TemporalWorkflowRun
	if err := r.db.Where("owner_identity = ? AND temporal_workflow_id = ?", ownerIdentity, workflowID).First(&run).Error; err != nil {
		return nil, err
	}
	return &run, nil
}

func (r *gormRepository) ListForOwner(ownerIdentity string, limit int) ([]models.TemporalWorkflowRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	var runs []models.TemporalWorkflowRun
	err := r.db.Where("owner_identity = ?", ownerIdentity).Order("created_at DESC").Limit(limit).Find(&runs).Error
	return runs, err
}
