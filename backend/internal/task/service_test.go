package task

import (
	"automation-hub-backend/internal/llm"
	"automation-hub-backend/internal/memory"
	"automation-hub-backend/internal/models"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestPlanIncludesSuccessCriteriaAndValidationGate(t *testing.T) {
	mem := &fakeMemoryService{}
	llmService, err := llm.NewServiceFromEnv()
	if err != nil {
		t.Fatalf("NewServiceFromEnv: %v", err)
	}
	service := NewService(mem, llmService)

	plan, err := service.Plan(IntakeRequest{
		Request:    "Add API code and tests for completion-first routing",
		ProjectKey: "018-HAI",
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if len(plan.Intake.SuccessCriteria) == 0 {
		t.Fatalf("expected explicit success criteria")
	}
	if plan.ValidationPlan.CompletionGate == "" {
		t.Fatalf("expected validation completion gate")
	}
	if plan.ModelDecision.SelectedModelID == "" {
		t.Fatalf("expected model decision")
	}
	if len(plan.MemoryUpdateProposals) == 0 {
		t.Fatalf("expected memory update proposal")
	}
	if len(plan.Steps) == 0 {
		t.Fatalf("expected universal task steps")
	}
	if len(plan.ToolDecision.SelectedTools) == 0 {
		t.Fatalf("expected tool routing decision")
	}
}

func TestRunQueuesReviewForHighRiskTask(t *testing.T) {
	mem := &fakeMemoryService{}
	llmService, err := llm.NewServiceFromEnv()
	if err != nil {
		t.Fatalf("NewServiceFromEnv: %v", err)
	}
	service := NewService(mem, llmService)

	plan, err := service.Run(IntakeRequest{
		Request:    "Delete account data and send a public posting",
		ProjectKey: "018-HAI",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if plan.CompletionStatus != "review_required" {
		t.Fatalf("status = %q, want review_required", plan.CompletionStatus)
	}
	if plan.ReviewQueueItem == nil {
		t.Fatalf("expected review queue item")
	}
	if len(service.ReviewQueue()) == 0 {
		t.Fatalf("expected service review queue entry")
	}
}

func TestRunValidatedTaskStoresLesson(t *testing.T) {
	mem := &fakeMemoryService{}
	llmService, err := llm.NewServiceFromEnv()
	if err != nil {
		t.Fatalf("NewServiceFromEnv: %v", err)
	}
	service := NewService(mem, llmService)

	plan, err := service.Run(IntakeRequest{
		Request:    "Summarize project context for the dashboard",
		ProjectKey: "018-HAI",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if plan.CompletionStatus != "validated" {
		t.Fatalf("status = %q, want validated", plan.CompletionStatus)
	}
	if !plan.ValidationResult.Passed {
		t.Fatalf("expected validation to pass")
	}
	if len(plan.StoredMemoryIDs) == 0 {
		t.Fatalf("expected stored lesson memory")
	}
}

type fakeMemoryService struct{}

func (fakeMemoryService) Create(request memory.CreateRequest) (*models.ContextMemory, error) {
	return &models.ContextMemory{
		ID:         uuid.New(),
		ProjectKey: request.ProjectKey,
		Kind:       request.Kind,
		Content:    request.Content,
		Summary:    request.Summary,
		Confidence: request.Confidence,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}, nil
}

func (fakeMemoryService) Update(id uuid.UUID, request memory.UpdateRequest) (*models.ContextMemory, error) {
	return nil, nil
}

func (fakeMemoryService) FindAll(projectKey string, includeArchived bool) ([]models.ContextMemory, error) {
	return nil, nil
}

func (fakeMemoryService) FindByID(id uuid.UUID) (*models.ContextMemory, error) {
	return nil, gorm.ErrRecordNotFound
}

func (fakeMemoryService) Archive(id uuid.UUID, archived bool) (*models.ContextMemory, error) {
	return nil, nil
}

func (fakeMemoryService) Delete(id uuid.UUID) error {
	return nil
}

func (fakeMemoryService) Retrieve(request memory.RetrieveRequest) (*memory.RetrieveResult, error) {
	now := time.Now().UTC()
	return &memory.RetrieveResult{
		Query: request.Query,
		UsedContext: []memory.RankedMemory{
			{
				Memory: models.ContextMemory{
					ID:         uuid.New(),
					ProjectKey: request.ProjectKey,
					Kind:       "project",
					Summary:    "Completion-first routing prefers validated completion before cost minimization.",
					Confidence: 0.9,
					UpdatedAt:  now,
				},
				Score:       0.9,
				Explanation: "same project, high relevance",
			},
		},
		Explanation: "retrieved fake context",
	}, nil
}
