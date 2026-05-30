package memory

import (
	"automation-hub-backend/internal/models"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestCreateDeduplicatesExactMemory(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(repo)

	first, err := service.Create(CreateRequest{ProjectKey: "018-hai", Kind: "preference", Content: "Prefer local models before cloud models.", Tags: []string{"llm"}})
	if err != nil {
		t.Fatalf("Create first memory: %v", err)
	}
	second, err := service.Create(CreateRequest{ProjectKey: "018-hai", Kind: "preference", Content: "Prefer local models before cloud models.", Tags: []string{"routing"}})
	if err != nil {
		t.Fatalf("Create duplicate memory: %v", err)
	}

	if first.ID != second.ID {
		t.Fatalf("duplicate memory created new ID")
	}
	if len(repo.memories) != 1 {
		t.Fatalf("stored %d memories, want 1", len(repo.memories))
	}
}

func TestRetrieveRanksRelevantProjectMemory(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(repo)
	_, _ = service.Create(CreateRequest{ProjectKey: "018-hai", Kind: "project", Content: "The project uses Angular dashboard and Go backend.", Confidence: 0.9})
	_, _ = service.Create(CreateRequest{ProjectKey: "other", Kind: "project", Content: "Unrelated cooking notes.", Confidence: 1})

	result, err := service.Retrieve(RetrieveRequest{ProjectKey: "018-hai", Query: "Angular Go backend dashboard", Limit: 3})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(result.UsedContext) != 1 {
		t.Fatalf("retrieved %d memories, want 1", len(result.UsedContext))
	}
	if result.UsedContext[0].Memory.ProjectKey != "018-hai" {
		t.Fatalf("retrieved wrong project memory")
	}
	if result.UsedContext[0].Memory.LastUsedAt == nil {
		t.Fatalf("expected LastUsedAt to be updated")
	}
}

type fakeRepository struct {
	memories map[uuid.UUID]models.ContextMemory
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{memories: map[uuid.UUID]models.ContextMemory{}}
}

func (r *fakeRepository) Create(memory *models.ContextMemory) (*models.ContextMemory, error) {
	if memory.ID == uuid.Nil {
		memory.ID = uuid.New()
	}
	now := time.Now().UTC()
	memory.CreatedAt = now
	memory.UpdatedAt = now
	r.memories[memory.ID] = *memory
	return memory, nil
}

func (r *fakeRepository) Update(memory *models.ContextMemory) (*models.ContextMemory, error) {
	memory.UpdatedAt = time.Now().UTC()
	r.memories[memory.ID] = *memory
	return memory, nil
}

func (r *fakeRepository) FindByID(id uuid.UUID) (*models.ContextMemory, error) {
	memory := r.memories[id]
	return &memory, nil
}

func (r *fakeRepository) FindAll(projectKey string, includeArchived bool) ([]models.ContextMemory, error) {
	memories := []models.ContextMemory{}
	for _, memory := range r.memories {
		if projectKey != "" && memory.ProjectKey != projectKey {
			continue
		}
		if !includeArchived && memory.Archived {
			continue
		}
		memories = append(memories, memory)
	}
	return memories, nil
}

func (r *fakeRepository) FindByHash(projectKey, kind, contentHash string) (*models.ContextMemory, error) {
	for _, memory := range r.memories {
		if memory.ProjectKey == projectKey && memory.Kind == kind && memory.ContentHash == contentHash && !memory.Archived {
			copyMemory := memory
			return &copyMemory, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *fakeRepository) Delete(id uuid.UUID) error {
	delete(r.memories, id)
	return nil
}
