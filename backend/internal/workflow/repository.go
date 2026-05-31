package workflow

import (
	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/models"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Repository interface {
	CreateItem(item *models.WorkflowItem) (*models.WorkflowItem, error)
	UpdateItem(item *models.WorkflowItem) (*models.WorkflowItem, error)
	FindItem(id uuid.UUID) (*models.WorkflowItem, error)
	FindActiveItemBySourceURI(sourceURI string) (*models.WorkflowItem, error)
	FindItems(includeArchived bool) ([]models.WorkflowItem, error)
	CreateChecklistItem(item *models.WorkflowChecklistItem) (*models.WorkflowChecklistItem, error)
	UpdateChecklistItem(item *models.WorkflowChecklistItem) (*models.WorkflowChecklistItem, error)
	FindChecklist(workflowID uuid.UUID) ([]models.WorkflowChecklistItem, error)
	CreateEvent(event *models.WorkflowEvent) (*models.WorkflowEvent, error)
	FindEvents(workflowID uuid.UUID) ([]models.WorkflowEvent, error)
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

func (r *GormRepository) CreateItem(item *models.WorkflowItem) (*models.WorkflowItem, error) {
	if err := r.DB.Create(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func (r *GormRepository) UpdateItem(item *models.WorkflowItem) (*models.WorkflowItem, error) {
	if err := r.DB.Save(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func (r *GormRepository) FindItem(id uuid.UUID) (*models.WorkflowItem, error) {
	var item models.WorkflowItem
	if err := r.DB.First(&item, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *GormRepository) FindActiveItemBySourceURI(sourceURI string) (*models.WorkflowItem, error) {
	var item models.WorkflowItem
	err := r.DB.Where("source_uri = ? AND archived = ?", sourceURI, false).Order("updated_at desc").First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *GormRepository) FindItems(includeArchived bool) ([]models.WorkflowItem, error) {
	var items []models.WorkflowItem
	query := r.DB.Order("priority_score desc, updated_at desc")
	if !includeArchived {
		query = query.Where("archived = ?", false)
	}
	err := query.Find(&items).Error
	return items, err
}

func (r *GormRepository) CreateChecklistItem(item *models.WorkflowChecklistItem) (*models.WorkflowChecklistItem, error) {
	if err := r.DB.Create(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func (r *GormRepository) UpdateChecklistItem(item *models.WorkflowChecklistItem) (*models.WorkflowChecklistItem, error) {
	if err := r.DB.Save(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func (r *GormRepository) FindChecklist(workflowID uuid.UUID) ([]models.WorkflowChecklistItem, error) {
	var items []models.WorkflowChecklistItem
	err := r.DB.Where("workflow_id = ?", workflowID).Order("position asc, created_at asc").Find(&items).Error
	return items, err
}

func (r *GormRepository) CreateEvent(event *models.WorkflowEvent) (*models.WorkflowEvent, error) {
	if err := r.DB.Create(event).Error; err != nil {
		return nil, err
	}
	return event, nil
}

func (r *GormRepository) FindEvents(workflowID uuid.UUID) ([]models.WorkflowEvent, error) {
	var events []models.WorkflowEvent
	err := r.DB.Where("workflow_id = ?", workflowID).Order("created_at desc").Find(&events).Error
	return events, err
}
