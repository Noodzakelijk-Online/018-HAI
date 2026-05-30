package verification

import (
	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Repository interface {
	CreateRun(run *models.VerificationRun) (*models.VerificationRun, error)
	UpdateRun(run *models.VerificationRun) (*models.VerificationRun, error)
	CreateEvidence(evidence *models.VerificationEvidence) (*models.VerificationEvidence, error)
	CreateClaim(claim *models.VerificationClaim) (*models.VerificationClaim, error)
	CreateAuditLog(log *models.VerificationAuditLog) (*models.VerificationAuditLog, error)
	FindRuns() ([]models.VerificationRun, error)
	FindClaims(runID uuid.UUID) ([]models.VerificationClaim, error)
	FindEvidence(runID uuid.UUID) ([]models.VerificationEvidence, error)
}

type GormRepository struct {
	DB *gorm.DB
}

func NewGormRepository(db *gorm.DB) Repository {
	return &GormRepository{DB: db}
}

func DefaultRepository() Repository {
	db, err := infra.GetDefaultDB()
	if err != nil {
		panic(err)
	}
	return NewGormRepository(db)
}

func (r *GormRepository) CreateRun(run *models.VerificationRun) (*models.VerificationRun, error) {
	if err := r.DB.Create(run).Error; err != nil {
		return nil, err
	}
	return run, nil
}

func (r *GormRepository) UpdateRun(run *models.VerificationRun) (*models.VerificationRun, error) {
	if err := r.DB.Save(run).Error; err != nil {
		return nil, err
	}
	return run, nil
}

func (r *GormRepository) CreateEvidence(evidence *models.VerificationEvidence) (*models.VerificationEvidence, error) {
	if err := r.DB.Create(evidence).Error; err != nil {
		return nil, err
	}
	return evidence, nil
}

func (r *GormRepository) CreateClaim(claim *models.VerificationClaim) (*models.VerificationClaim, error) {
	if err := r.DB.Create(claim).Error; err != nil {
		return nil, err
	}
	return claim, nil
}

func (r *GormRepository) CreateAuditLog(log *models.VerificationAuditLog) (*models.VerificationAuditLog, error) {
	if err := r.DB.Create(log).Error; err != nil {
		return nil, err
	}
	return log, nil
}

func (r *GormRepository) FindRuns() ([]models.VerificationRun, error) {
	var runs []models.VerificationRun
	err := r.DB.Order("created_at desc").Find(&runs).Error
	return runs, err
}

func (r *GormRepository) FindClaims(runID uuid.UUID) ([]models.VerificationClaim, error) {
	var claims []models.VerificationClaim
	err := r.DB.Where("run_id = ?", runID).Order("created_at asc").Find(&claims).Error
	return claims, err
}

func (r *GormRepository) FindEvidence(runID uuid.UUID) ([]models.VerificationEvidence, error) {
	var evidence []models.VerificationEvidence
	err := r.DB.Where("run_id = ?", runID).Order("quality_score desc").Find(&evidence).Error
	return evidence, err
}
