package source

import (
	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/models"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Repository interface {
	SaveConnector(connector *models.SourceConnector) (*models.SourceConnector, error)
	FindConnectors() ([]models.SourceConnector, error)
	CreateSource(source *models.ConnectedSource) (*models.ConnectedSource, error)
	UpdateSource(source *models.ConnectedSource) (*models.ConnectedSource, error)
	RevokeSource(source *models.ConnectedSource, ownerIdentity string, revokedAt time.Time) (*models.ConnectedSource, error)
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
	FindExtractionsForSources(sourceIDs []uuid.UUID, projectKey string, includeArchived bool) ([]models.SourceExtraction, error)
	FindExtraction(id uuid.UUID) (*models.SourceExtraction, error)
	DeleteExtractionForOwner(
		extraction *models.SourceExtraction,
		source *models.ConnectedSource,
		ownerIdentity string,
	) error
	SaveIndexEntry(entry *models.SourceIndexEntry) (*models.SourceIndexEntry, error)
	DeletePendingVectorIndex(extractionID uuid.UUID) error
	SaveAuditLog(log *models.SourceAuditLog) (*models.SourceAuditLog, error)
	FindAuditLogs(sourceID *uuid.UUID) ([]models.SourceAuditLog, error)
	SaveOAuthToken(token *models.SourceOAuthToken) error
	FindOAuthToken(sourceID uuid.UUID) (*models.SourceOAuthToken, error)
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
	create := false
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
			create = true
		}
	} else if connector.CreatedAt.IsZero() {
		if err := r.DB.First(&existing, "id = ?", connector.ID).Error; err == nil {
			connector.CreatedAt = existing.CreatedAt
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			create = true
		} else {
			return nil, err
		}
	}
	if create {
		if err := r.DB.Create(connector).Error; err != nil {
			return nil, err
		}
		return connector, nil
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

func (r *GormRepository) RevokeSource(
	expected *models.ConnectedSource,
	ownerIdentity string,
	revokedAt time.Time,
) (*models.ConnectedSource, error) {
	if expected == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var updated models.ConnectedSource
	err := r.DB.Transaction(func(tx *gorm.DB) error {
		var source models.ConnectedSource
		if err := tx.Where(
			"id = ? AND owner_identity = ? AND connector_key = ? AND default_project_key = ? AND updated_at = ?",
			expected.ID,
			ownerIdentity,
			expected.ConnectorKey,
			expected.DefaultProjectKey,
			expected.UpdatedAt,
		).First(&source).Error; err != nil {
			return err
		}
		if err := tx.Where("source_id = ?", expected.ID).
			Delete(&models.SourceOAuthToken{}).Error; err != nil {
			return err
		}
		result := tx.Model(&models.ConnectedSource{}).
			Where(
				"id = ? AND owner_identity = ? AND connector_key = ? AND default_project_key = ? AND updated_at = ?",
				expected.ID,
				ownerIdentity,
				expected.ConnectorKey,
				expected.DefaultProjectKey,
				expected.UpdatedAt,
			).
			Updates(map[string]any{
				"enabled":    false,
				"status":     "revoked",
				"revoked_at": revokedAt.UTC(),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return tx.First(&updated, "id = ?", expected.ID).Error
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
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
	return r.findExtractions(nil, projectKey, includeArchived)
}

func (r *GormRepository) FindExtractionsForSources(sourceIDs []uuid.UUID, projectKey string, includeArchived bool) ([]models.SourceExtraction, error) {
	if len(sourceIDs) == 0 {
		return []models.SourceExtraction{}, nil
	}
	return r.findExtractions(sourceIDs, projectKey, includeArchived)
}

func (r *GormRepository) findExtractions(sourceIDs []uuid.UUID, projectKey string, includeArchived bool) ([]models.SourceExtraction, error) {
	var extractions []models.SourceExtraction
	query := r.DB.Order("updated_at desc")
	if sourceIDs != nil {
		query = query.Where("source_id IN ?", sourceIDs)
	}
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

func (r *GormRepository) DeleteExtractionForOwner(
	expected *models.SourceExtraction,
	expectedSource *models.ConnectedSource,
	ownerIdentity string,
) error {
	if expected == nil || expectedSource == nil ||
		expected.SourceID != expectedSource.ID {
		return gorm.ErrRecordNotFound
	}
	return r.DB.Transaction(func(tx *gorm.DB) error {
		var source models.ConnectedSource
		if err := tx.Where(
			"id = ? AND owner_identity = ? AND connector_key = ? AND updated_at = ?",
			expectedSource.ID,
			ownerIdentity,
			expectedSource.ConnectorKey,
			expectedSource.UpdatedAt,
		).First(&source).Error; err != nil {
			return err
		}
		if err := tx.Where("extraction_id = ?", expected.ID).
			Delete(&models.SourceIndexEntry{}).Error; err != nil {
			return err
		}
		result := tx.Where(
			"id = ? AND source_id = ? AND project_key = ? AND raw_item_id = ? AND content_hash = ? AND source_uri = ? AND updated_at = ?",
			expected.ID,
			expected.SourceID,
			expected.ProjectKey,
			expected.RawItemID,
			expected.ContentHash,
			expected.SourceURI,
			expected.UpdatedAt,
		).
			Delete(&models.SourceExtraction{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func (r *GormRepository) SaveIndexEntry(entry *models.SourceIndexEntry) (*models.SourceIndexEntry, error) {
	if entry.ID == uuid.Nil {
		var existing models.SourceIndexEntry
		err := r.DB.Where(
			"extraction_id = ? AND index_type = ?",
			entry.ExtractionID,
			entry.IndexType,
		).First(&existing).Error
		if err == nil {
			entry.ID = existing.ID
			entry.CreatedAt = existing.CreatedAt
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	if err := r.DB.Save(entry).Error; err != nil {
		return nil, err
	}
	return entry, nil
}

// DeletePendingVectorIndex removes the legacy placeholder produced before a
// real local embedding adapter is configured. It deliberately leaves any
// future non-placeholder vector records intact.
func (r *GormRepository) DeletePendingVectorIndex(extractionID uuid.UUID) error {
	return r.DB.
		Where("extraction_id = ? AND index_type = ? AND vector_ref LIKE ?", extractionID, "vector_ref", "local-vector-pending:%").
		Delete(&models.SourceIndexEntry{}).Error
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

// SaveOAuthToken upserts the token for a source (one token set per source).
func (r *GormRepository) SaveOAuthToken(token *models.SourceOAuthToken) error {
	var existing models.SourceOAuthToken
	err := r.DB.Where("source_id = ?", token.SourceID).First(&existing).Error
	if err == nil {
		token.ID = existing.ID
		token.CreatedAt = existing.CreatedAt
		return r.DB.Select("*").Save(token).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if token.ID == uuid.Nil {
		token.ID = uuid.New()
	}
	return r.DB.Create(token).Error
}

func (r *GormRepository) FindOAuthToken(sourceID uuid.UUID) (*models.SourceOAuthToken, error) {
	var token models.SourceOAuthToken
	if err := r.DB.Where("source_id = ?", sourceID).First(&token).Error; err != nil {
		return nil, err
	}
	return &token, nil
}
