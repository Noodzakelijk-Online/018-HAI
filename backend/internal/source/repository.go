package source

import (
	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/models"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Repository interface {
	SaveConnector(connector *models.SourceConnector) (*models.SourceConnector, error)
	FindConnectors() ([]models.SourceConnector, error)
	CreateSource(source *models.ConnectedSource) (*models.ConnectedSource, error)
	UpdateSource(source *models.ConnectedSource) (*models.ConnectedSource, error)
	FindSources(includeDisabled bool) ([]models.ConnectedSource, error)
	FindSource(id uuid.UUID) (*models.ConnectedSource, error)
	CreateSyncJob(job *models.SourceSyncJob) (*models.SourceSyncJob, error)
	UpdateSyncJob(job *models.SourceSyncJob) (*models.SourceSyncJob, error)
	FindSyncJobs(sourceID *uuid.UUID) ([]models.SourceSyncJob, error)
	FindRawItem(sourceID uuid.UUID, externalID string) (*models.SourceRawItem, error)
	SaveRawItem(item *models.SourceRawItem) (*models.SourceRawItem, error)
	FindRawItems(sourceID uuid.UUID) ([]models.SourceRawItem, error)
	FindExtractionByRawItem(rawItemID uuid.UUID) (*models.SourceExtraction, error)
	SaveExtraction(extraction *models.SourceExtraction) (*models.SourceExtraction, error)
	FindExtractions(projectKey string, includeArchived bool) ([]models.SourceExtraction, error)
	FindExtraction(id uuid.UUID) (*models.SourceExtraction, error)
	DeleteExtraction(id uuid.UUID) error
	SaveIndexEntry(entry *models.SourceIndexEntry) (*models.SourceIndexEntry, error)
	SaveAuditLog(log *models.SourceAuditLog) (*models.SourceAuditLog, error)
	FindAuditLogs(sourceID *uuid.UUID) ([]models.SourceAuditLog, error)
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

func (r *GormRepository) SaveConnector(connector *models.SourceConnector) (*models.SourceConnector, error) {
	var existing models.SourceConnector
	if connector.ID == uuid.Nil {
		err := r.DB.Where("connector_key = ?", connector.ConnectorKey).First(&existing).Error
		if err == nil {
			connector.ID = existing.ID
			if connector.CreatedAt.IsZero() {
				connector.CreatedAt = existing.CreatedAt
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		} else {
			connector.ID = uuid.New()
		}
	} else if connector.CreatedAt.IsZero() {
		if err := r.DB.First(&existing, "id = ?", connector.ID).Error; err == nil {
			connector.CreatedAt = existing.CreatedAt
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	if err := r.DB.Select("*").Save(connector).Error; err != nil {
		return nil, err
	}
	return connector, nil
}

func (r *GormRepository) FindConnectors() ([]models.SourceConnector, error) {
	var connectors []models.SourceConnector
	err := r.DB.Order("category asc, name asc").Find(&connectors).Error
	return connectors, err
}

func (r *GormRepository) CreateSource(source *models.ConnectedSource) (*models.ConnectedSource, error) {
	if err := r.DB.Create(source).Error; err != nil {
		return nil, err
	}
	return source, nil
}

func (r *GormRepository) UpdateSource(source *models.ConnectedSource) (*models.ConnectedSource, error) {
	if err := r.DB.Save(source).Error; err != nil {
		return nil, err
	}
	return source, nil
}

func (r *GormRepository) FindSources(includeDisabled bool) ([]models.ConnectedSource, error) {
	var sources []models.ConnectedSource
	query := r.DB.Order("updated_at desc")
	if !includeDisabled {
		query = query.Where("enabled = ? AND status <> ?", true, "revoked")
	}
	err := query.Find(&sources).Error
	return sources, err
}

func (r *GormRepository) FindSource(id uuid.UUID) (*models.ConnectedSource, error) {
	var source models.ConnectedSource
	if err := r.DB.First(&source, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &source, nil
}

func (r *GormRepository) CreateSyncJob(job *models.SourceSyncJob) (*models.SourceSyncJob, error) {
	if err := r.DB.Create(job).Error; err != nil {
		return nil, err
	}
	return job, nil
}

func (r *GormRepository) UpdateSyncJob(job *models.SourceSyncJob) (*models.SourceSyncJob, error) {
	if err := r.DB.Save(job).Error; err != nil {
		return nil, err
	}
	return job, nil
}

func (r *GormRepository) FindSyncJobs(sourceID *uuid.UUID) ([]models.SourceSyncJob, error) {
	var jobs []models.SourceSyncJob
	query := r.DB.Order("created_at desc")
	if sourceID != nil {
		query = query.Where("source_id = ?", *sourceID)
	}
	err := query.Find(&jobs).Error
	return jobs, err
}

func (r *GormRepository) FindRawItem(sourceID uuid.UUID, externalID string) (*models.SourceRawItem, error) {
	var item models.SourceRawItem
	if err := r.DB.Where("source_id = ? AND external_id = ?", sourceID, externalID).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *GormRepository) SaveRawItem(item *models.SourceRawItem) (*models.SourceRawItem, error) {
	if err := r.DB.Save(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func (r *GormRepository) FindRawItems(sourceID uuid.UUID) ([]models.SourceRawItem, error) {
	var items []models.SourceRawItem
	err := r.DB.Where("source_id = ?", sourceID).Order("updated_at desc").Find(&items).Error
	return items, err
}

func (r *GormRepository) FindExtractionByRawItem(rawItemID uuid.UUID) (*models.SourceExtraction, error) {
	var extraction models.SourceExtraction
	if err := r.DB.Where("raw_item_id = ?", rawItemID).First(&extraction).Error; err != nil {
		return nil, err
	}
	return &extraction, nil
}

func (r *GormRepository) SaveExtraction(extraction *models.SourceExtraction) (*models.SourceExtraction, error) {
	if err := r.DB.Save(extraction).Error; err != nil {
		return nil, err
	}
	return extraction, nil
}

func (r *GormRepository) FindExtractions(projectKey string, includeArchived bool) ([]models.SourceExtraction, error) {
	var extractions []models.SourceExtraction
	query := r.DB.Order("updated_at desc")
	if projectKey != "" {
		query = query.Where("project_key = ?", projectKey)
	}
	if !includeArchived {
		query = query.Where("archived = ?", false)
	}
	err := query.Find(&extractions).Error
	return extractions, err
}

func (r *GormRepository) FindExtraction(id uuid.UUID) (*models.SourceExtraction, error) {
	var extraction models.SourceExtraction
	if err := r.DB.First(&extraction, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &extraction, nil
}

func (r *GormRepository) DeleteExtraction(id uuid.UUID) error {
	return r.DB.Delete(&models.SourceExtraction{}, id).Error
}

func (r *GormRepository) SaveIndexEntry(entry *models.SourceIndexEntry) (*models.SourceIndexEntry, error) {
	if err := r.DB.Save(entry).Error; err != nil {
		return nil, err
	}
	return entry, nil
}

func (r *GormRepository) SaveAuditLog(log *models.SourceAuditLog) (*models.SourceAuditLog, error) {
	if err := r.DB.Create(log).Error; err != nil {
		return nil, err
	}
	return log, nil
}

func (r *GormRepository) FindAuditLogs(sourceID *uuid.UUID) ([]models.SourceAuditLog, error) {
	var logs []models.SourceAuditLog
	query := r.DB.Order("created_at desc")
	if sourceID != nil {
		query = query.Where("source_id = ?", *sourceID)
	}
	err := query.Find(&logs).Error
	return logs, err
}
