package llm

import (
	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/models"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

// ProbeHistoryRepository persists redacted readiness evidence independently of
// runtime provider configuration. It deliberately has no credential fields.
type ProbeHistoryRepository interface {
	RecordProviderProbe(probe *models.LLMProviderProbe) (*models.LLMProviderProbe, error)
	FindRecentProviderProbes(limit int) ([]models.LLMProviderProbe, error)
	FindLatestProviderProbe(providerID string) (*models.LLMProviderProbe, error)
}

// ModelMaintenanceRepository keeps daily model-maintenance decisions separate
// from provider readiness probes. A model can be reachable while still needing
// a refresh, so the records must not be conflated.
type ModelMaintenanceRepository interface {
	RecordModelMaintenance(record *models.LLMModelMaintenance) (*models.LLMModelMaintenance, error)
	FindLatestModelMaintenance(providerID, modelID string) (*models.LLMModelMaintenance, error)
	FindRecentModelMaintenance(limit int) ([]models.LLMModelMaintenance, error)
}

// GenerationHistoryRepository stores aggregate, redacted model-call evidence.
// It must never receive prompts, outputs, source content, or credentials.
type GenerationHistoryRepository interface {
	RecordGeneration(record *models.LLMGenerationRecord) (*models.LLMGenerationRecord, error)
	FindRecentGenerations(limit int) ([]models.LLMGenerationRecord, error)
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

func DefaultModelMaintenanceRepository() (ModelMaintenanceRepository, error) {
	db, err := infra.GetDefaultDB()
	if err != nil {
		return nil, fmt.Errorf("initialize model maintenance history: %w", err)
	}
	return NewGormModelMaintenanceRepository(db), nil
}

func DefaultGenerationHistoryRepository() (GenerationHistoryRepository, error) {
	db, err := infra.GetDefaultDB()
	if err != nil {
		return nil, fmt.Errorf("initialize generation history: %w", err)
	}
	return NewGormGenerationHistoryRepository(db), nil
}

type GormModelMaintenanceRepository struct {
	DB *gorm.DB
}

// AcquireModelMaintenanceLease serializes one provider/model maintenance pass
// across backend processes. The session lock is released automatically if its
// process or database connection exits, avoiding a stale daily-maintenance
// block after a restart.
func (r *GormModelMaintenanceRepository) AcquireModelMaintenanceLease(ctx context.Context, providerID, modelID string) (func(), bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	providerID = strings.TrimSpace(providerID)
	modelID = strings.TrimSpace(modelID)
	if providerID == "" || modelID == "" {
		return nil, false, fmt.Errorf("provider id and model id are required")
	}
	db, err := r.DB.DB()
	if err != nil {
		return nil, false, err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, false, err
	}
	key := modelMaintenanceLeaseKey(providerID, modelID)
	var acquired bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&acquired); err != nil {
		_ = conn.Close()
		return nil, false, err
	}
	if !acquired {
		_ = conn.Close()
		return func() {}, false, nil
	}

	var once sync.Once
	release := func() {
		once.Do(func() {
			releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var released bool
			_ = conn.QueryRowContext(releaseCtx, "SELECT pg_advisory_unlock($1)", key).Scan(&released)
			_ = conn.Close()
		})
	}
	return release, true, nil
}

func modelMaintenanceLeaseKey(providerID, modelID string) int64 {
	digest := sha256.Sum256([]byte("hai:model-maintenance:" + strings.TrimSpace(providerID) + "\x00" + strings.TrimSpace(modelID)))
	return int64(binary.BigEndian.Uint64(digest[:8]))
}

type GormGenerationHistoryRepository struct {
	DB *gorm.DB
}

func NewGormModelMaintenanceRepository(db *gorm.DB) ModelMaintenanceRepository {
	return &GormModelMaintenanceRepository{DB: db}
}

func NewGormGenerationHistoryRepository(db *gorm.DB) GenerationHistoryRepository {
	return &GormGenerationHistoryRepository{DB: db}
}

func (r *GormGenerationHistoryRepository) RecordGeneration(record *models.LLMGenerationRecord) (*models.LLMGenerationRecord, error) {
	if record == nil || strings.TrimSpace(record.Status) == "" {
		return nil, fmt.Errorf("generation status is required")
	}
	if record.LoggedAt.IsZero() {
		record.LoggedAt = time.Now().UTC()
	}
	record.LoggedAt = record.LoggedAt.UTC()
	if err := r.DB.Create(record).Error; err != nil {
		return nil, err
	}
	return record, nil
}

func (r *GormGenerationHistoryRepository) FindRecentGenerations(limit int) ([]models.LLMGenerationRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	var records []models.LLMGenerationRecord
	if err := r.DB.Order("logged_at DESC").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (r *GormModelMaintenanceRepository) RecordModelMaintenance(record *models.LLMModelMaintenance) (*models.LLMModelMaintenance, error) {
	if record == nil {
		return nil, fmt.Errorf("model maintenance record is required")
	}
	if strings.TrimSpace(record.ProviderID) == "" || strings.TrimSpace(record.ModelID) == "" {
		return nil, fmt.Errorf("provider id and model id are required")
	}
	if record.CheckedAt.IsZero() {
		record.CheckedAt = time.Now().UTC()
	}
	record.CheckedAt = record.CheckedAt.UTC()
	if err := r.DB.Create(record).Error; err != nil {
		return nil, err
	}
	return record, nil
}

func (r *GormModelMaintenanceRepository) FindLatestModelMaintenance(providerID, modelID string) (*models.LLMModelMaintenance, error) {
	providerID = strings.TrimSpace(providerID)
	modelID = strings.TrimSpace(modelID)
	if providerID == "" || modelID == "" {
		return nil, fmt.Errorf("provider id and model id are required")
	}
	var record models.LLMModelMaintenance
	result := r.DB.Where("provider_id = ? AND model_id = ?", providerID, modelID).Order("checked_at DESC").Limit(1).Find(&record)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &record, nil
}

func (r *GormModelMaintenanceRepository) FindRecentModelMaintenance(limit int) ([]models.LLMModelMaintenance, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	var records []models.LLMModelMaintenance
	if err := r.DB.Order("checked_at DESC").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
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

// FindLatestProviderProbe supplies the one readiness record that strict
// routing needs. A missing record is a normal, fail-closed state rather than
// an error: the caller must not route the provider until it has been probed.
func (r *GormProbeHistoryRepository) FindLatestProviderProbe(providerID string) (*models.LLMProviderProbe, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return nil, fmt.Errorf("provider id is required")
	}
	var probe models.LLMProviderProbe
	result := r.DB.Where("provider_id = ?", providerID).Order("checked_at DESC").Limit(1).Find(&probe)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &probe, nil
}
