package browserverify

import (
	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/models"

	"gorm.io/gorm"
)

type Repository interface {
	Create(*models.BrowserVerificationRun) (*models.BrowserVerificationRun, error)
	Update(*models.BrowserVerificationRun) (*models.BrowserVerificationRun, error)
	List(string, int) ([]models.BrowserVerificationRun, error)
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

func (r *gormRepository) Create(run *models.BrowserVerificationRun) (*models.BrowserVerificationRun, error) {
	if err := r.db.Create(run).Error; err != nil {
		return nil, err
	}
	return run, nil
}

func (r *gormRepository) Update(run *models.BrowserVerificationRun) (*models.BrowserVerificationRun, error) {
	if err := r.db.Save(run).Error; err != nil {
		return nil, err
	}
	return run, nil
}

func (r *gormRepository) List(owner string, limit int) ([]models.BrowserVerificationRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	var runs []models.BrowserVerificationRun
	err := r.db.Where("owner_identity = ?", owner).Order("created_at DESC").Limit(limit).Find(&runs).Error
	return runs, err
}
