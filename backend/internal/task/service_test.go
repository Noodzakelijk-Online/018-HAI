package task

import (
	"automation-hub-backend/internal/llm"
	"automation-hub-backend/internal/memory"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/source"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestPlanIncludesSuccessCriteriaAndValidationGate(t *testing.T) {
	mem := &fakeMemoryService{}
	llmService := newTaskTestLLMService(t)
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

func TestPlanRefreshesDueSourcesBeforeSourceSearch(t *testing.T) {
	mem := &fakeMemoryService{}
	src := &fakeTaskSourceService{}
	llmService := newTaskTestLLMService(t)
	service := NewService(mem, llmService, src)

	plan, err := service.Plan(IntakeRequest{
		Request:    "Summarize local project files and source context",
		ProjectKey: "018-HAI",
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if src.refreshCalls != 1 {
		t.Fatalf("refreshCalls = %d, want 1", src.refreshCalls)
	}
	if src.searchCalls != 1 {
		t.Fatalf("searchCalls = %d, want 1", src.searchCalls)
	}
	if len(src.order) < 2 || src.order[0] != "refresh" || src.order[1] != "search" {
		t.Fatalf("order = %#v, want refresh before search", src.order)
	}
	if plan.ContextPlan.SourceRefresh == nil {
		t.Fatalf("expected source refresh result in context plan")
	}
	if len(plan.ContextPlan.SourceContext) != 1 {
		t.Fatalf("source context = %d, want 1", len(plan.ContextPlan.SourceContext))
	}
}

func TestRunQueuesReviewForHighRiskTask(t *testing.T) {
	mem := &fakeMemoryService{}
	llmService := newTaskTestLLMService(t)
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

func TestResolveReviewItemApprovesAndRunsTask(t *testing.T) {
	mem := &fakeMemoryService{}
	llmService := newTaskTestLLMService(t)
	service := NewService(mem, llmService)

	plan, err := service.Run(IntakeRequest{
		Request:    "Send a public posting after approval",
		ProjectKey: "018-HAI",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if plan.ReviewQueueItem == nil {
		t.Fatalf("expected review queue item")
	}

	result, err := service.ResolveReviewItem(plan.ReviewQueueItem.ID, ApprovalDecision{
		Approved: true,
		Note:     "Operator approved controlled internal execution only.",
	})
	if err != nil {
		t.Fatalf("ResolveReviewItem: %v", err)
	}
	if result.Item.Decision != "approved" {
		t.Fatalf("decision = %q, want approved", result.Item.Decision)
	}
	if result.Plan == nil {
		t.Fatalf("expected approved item to rerun task")
	}
	if !result.Plan.RiskAssessment.ApprovalGranted {
		t.Fatalf("expected approval to be reflected in risk assessment")
	}
}

func TestRunValidatedTaskStoresLesson(t *testing.T) {
	mem := &fakeMemoryService{}
	llmService := newTaskTestLLMService(t)
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

func newTaskTestLLMService(t *testing.T) *llm.Service {
	t.Helper()
	t.Setenv("LLM_PROVIDERS_JSON", "")
	t.Setenv("LLM_POLICY_JSON", "")
	t.Setenv("OLLAMA_BASE_URL", "http://localhost:11434")
	t.Setenv("LM_STUDIO_BASE_URL", "")
	t.Setenv("FREE_CLOUD_OPENAI_BASE_URL", "")
	llmService, err := llm.NewServiceFromEnv()
	if err != nil {
		t.Fatalf("NewServiceFromEnv: %v", err)
	}
	return llmService
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

type fakeTaskSourceService struct {
	refreshCalls int
	searchCalls  int
	order        []string
}

func (s *fakeTaskSourceService) Connectors() ([]models.SourceConnector, error) {
	return nil, nil
}

func (s *fakeTaskSourceService) CreateSource(request source.CreateSourceRequest) (*models.ConnectedSource, error) {
	return nil, nil
}

func (s *fakeTaskSourceService) UpdateSource(id uuid.UUID, request source.UpdateSourceRequest) (*models.ConnectedSource, error) {
	return nil, nil
}

func (s *fakeTaskSourceService) Sources(includeDisabled bool) ([]models.ConnectedSource, error) {
	return nil, nil
}

func (s *fakeTaskSourceService) Sync(sourceID uuid.UUID, request source.ImportRequest) (*source.SyncResult, error) {
	return nil, nil
}

func (s *fakeTaskSourceService) RunDueScheduledSyncs(now time.Time) (*source.ScheduledSyncRun, error) {
	s.refreshCalls++
	s.order = append(s.order, "refresh")
	return &source.ScheduledSyncRun{Checked: 1, Due: 1, Completed: 1}, nil
}

func (s *fakeTaskSourceService) Reindex(sourceID uuid.UUID) (*source.SyncResult, error) {
	return nil, nil
}

func (s *fakeTaskSourceService) Pause(sourceID uuid.UUID, paused bool) (*models.ConnectedSource, error) {
	return nil, nil
}

func (s *fakeTaskSourceService) Revoke(sourceID uuid.UUID) (*models.ConnectedSource, error) {
	return nil, nil
}

func (s *fakeTaskSourceService) Search(request source.SearchRequest) (*source.SearchResult, error) {
	s.searchCalls++
	s.order = append(s.order, "search")
	return &source.SearchResult{
		Query: request.Query,
		UsedContext: []source.RankedExtraction{
			{
				Extraction: models.SourceExtraction{
					ID:          uuid.New(),
					ProjectKey:  request.ProjectKey,
					ContentType: "local_file_md",
					Summary:     "Local project files describe scheduled ingestion and approval-gated execution.",
					SourceLabel: "project-note.md",
				},
				Score:       0.92,
				Explanation: "same project, source linked",
			},
		},
		Explanation: "fake source context retrieved",
	}, nil
}

func (s *fakeTaskSourceService) Extractions(projectKey string, includeArchived bool) ([]models.SourceExtraction, error) {
	return nil, nil
}

func (s *fakeTaskSourceService) UpdateExtraction(id uuid.UUID, request models.SourceExtraction) (*models.SourceExtraction, error) {
	return nil, nil
}

func (s *fakeTaskSourceService) ArchiveExtraction(id uuid.UUID, archived bool) (*models.SourceExtraction, error) {
	return nil, nil
}

func (s *fakeTaskSourceService) DeleteExtraction(id uuid.UUID) error {
	return nil
}

func (s *fakeTaskSourceService) AuditLogs(sourceID *uuid.UUID) ([]models.SourceAuditLog, error) {
	return nil, nil
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
