package llm

import (
	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/models"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ProbeHistoryRepository persists redacted readiness evidence independently of
// runtime provider configuration. It deliberately has no credential fields.
type ProbeHistoryRepository interface {
	RecordProviderProbe(probe *models.LLMProviderProbe) (*models.LLMProviderProbe, error)
	FindRecentProviderProbes(limit int) ([]models.LLMProviderProbe, error)
}

type GormProbeHistoryRepository struct {
	DB *gorm.DB
}

func NewGormProbeHistoryRepository(db *gorm.DB) ProbeHistoryRepository {
	return &GormProbeHistoryRepository{DB: db}
}

func DefaultProbeHistoryRepository() (ProbeHistoryRepository, error) {
	db, err := infra.GetDefaultDB()
	if err != nil {
		return nil, fmt.Errorf("initialize provider probe history: %w", err)
	}
	return NewGormProbeHistoryRepository(db), nil
}

func (r *GormProbeHistoryRepository) RecordProviderProbe(probe *models.LLMProviderProbe) (*models.LLMProviderProbe, error) {
	if probe == nil {
		return nil, fmt.Errorf("provider probe is required")
	}
	if strings.TrimSpace(probe.ProviderID) == "" {
		return nil, fmt.Errorf("provider id is required")
	}
	if probe.CheckedAt.IsZero() {
		probe.CheckedAt = time.Now().UTC()
	}
	probe.CheckedAt = probe.CheckedAt.UTC()
	if probe.Live {
		lastSuccess := probe.CheckedAt
		probe.LastSuccessfulAt = &lastSuccess
	} else {
		var previous models.LLMProviderProbe
		err := r.DB.Where("provider_id = ?", probe.ProviderID).Order("checked_at DESC").First(&previous).Error
		if err == nil && previous.LastSuccessfulAt != nil {
			lastSuccess := previous.LastSuccessfulAt.UTC()
			probe.LastSuccessfulAt = &lastSuccess
		} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	if err := r.DB.Create(probe).Error; err != nil {
		return nil, err
	}
	return probe, nil
}

func (r *GormProbeHistoryRepository) FindRecentProviderProbes(limit int) ([]models.LLMProviderProbe, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	var probes []models.LLMProviderProbe
	if err := r.DB.Order("checked_at DESC").Limit(limit).Find(&probes).Error; err != nil {
		return nil, err
	}
	return probes, nil
}
