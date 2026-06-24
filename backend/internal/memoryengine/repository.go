package memoryengine

import (
	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Repository interface {
	FindConversation(platform, externalID string) (*models.AIConversationArchive, error)
	FindConversationByID(id uuid.UUID) (*models.AIConversationArchive, error)
	SaveConversation(conversation *models.AIConversationArchive) (*models.AIConversationArchive, error)
	FindConversations(limit int) ([]models.AIConversationArchive, error)
	DeleteConversation(id uuid.UUID) error
	SaveInsight(insight *models.AIMemoryInsight) (*models.AIMemoryInsight, error)
	FindInsights(kind, projectKey string, needsReview *bool, limit int) ([]models.AIMemoryInsight, error)
	ArchiveInsights(conversationID uuid.UUID, revision int) error
	DeleteMemoriesBySourceURI(sourceURI string) error
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

func (r *GormRepository) FindConversation(platform, externalID string) (*models.AIConversationArchive, error) {
	var conversation models.AIConversationArchive
	err := r.DB.
		Where("platform = ? AND external_id = ? AND archived = ?", platform, externalID, false).
		First(&conversation).Error
	if err != nil {
		return nil, err
	}
	return &conversation, nil
}

func (r *GormRepository) FindConversationByID(id uuid.UUID) (*models.AIConversationArchive, error) {
	var conversation models.AIConversationArchive
	if err := r.DB.First(&conversation, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &conversation, nil
}

func (r *GormRepository) SaveConversation(conversation *models.AIConversationArchive) (*models.AIConversationArchive, error) {
	if err := r.DB.Save(conversation).Error; err != nil {
		return nil, err
	}
	return conversation, nil
}

func (r *GormRepository) FindConversations(limit int) ([]models.AIConversationArchive, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var conversations []models.AIConversationArchive
	err := r.DB.
		Where("archived = ?", false).
		Order("captured_at desc").
		Limit(limit).
		Find(&conversations).Error
	return conversations, err
}

func (r *GormRepository) DeleteConversation(id uuid.UUID) error {
	return r.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("conversation_id = ?", id).Delete(&models.AIMemoryInsight{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.AIConversationArchive{}, id).Error
	})
}

func (r *GormRepository) SaveInsight(insight *models.AIMemoryInsight) (*models.AIMemoryInsight, error) {
	if err := r.DB.Create(insight).Error; err != nil {
		return nil, err
	}
	return insight, nil
}

func (r *GormRepository) FindInsights(kind, projectKey string, needsReview *bool, limit int) ([]models.AIMemoryInsight, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var insights []models.AIMemoryInsight
	query := r.DB.Where("status <> ?", "superseded")
	if kind != "" {
		query = query.Where("kind = ?", kind)
	}
	if projectKey != "" {
		query = query.Where("project_key = ?", projectKey)
	}
	if needsReview != nil {
		query = query.Where("needs_review = ?", *needsReview)
	}
	err := query.Order("created_at desc").Limit(limit).Find(&insights).Error
	return insights, err
}

func (r *GormRepository) ArchiveInsights(conversationID uuid.UUID, revision int) error {
	return r.DB.
		Model(&models.AIMemoryInsight{}).
		Where("conversation_id = ? AND revision < ?", conversationID, revision).
		Update("status", "superseded").Error
}

func (r *GormRepository) DeleteMemoriesBySourceURI(sourceURI string) error {
	if sourceURI == "" {
		return nil
	}
	return r.DB.
		Where("source_uri = ? AND tags LIKE ?", sourceURI, "%ai-history%").
		Delete(&models.ContextMemory{}).Error
}
