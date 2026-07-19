package planningoptimizer

import (
	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/models"

	"gorm.io/gorm"
)

// Repository keeps optimization proposal audits owner-scoped.
type Repository interface {
	Create(*models.OptimizationProposalRun) (*models.OptimizationProposalRun, error)
	List(ownerIdentity string, limit int) ([]models.OptimizationProposalRun, error)
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

func (r *gormRepository) Create(run *models.OptimizationProposalRun) (*models.OptimizationProposalRun, error) {
	if err := r.db.Create(run).Error; err != nil {
		return nil, err
	}
	return run, nil
}

func (r *gormRepository) List(ownerIdentity string, limit int) ([]models.OptimizationProposalRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	var runs []models.OptimizationProposalRun
	err := r.db.Where("owner_identity = ?", ownerIdentity).Order("created_at DESC").Limit(limit).Find(&runs).Error
	return runs, err
}
