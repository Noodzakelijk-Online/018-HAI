package task

import (
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/llm"
	"automation-hub-backend/internal/memory"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/source"
	"automation-hub-backend/internal/verification"

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

func TestAnalyzeIntakeDoesNotRequireRuntimeForAPIExplanation(t *testing.T) {
	analysis := analyzeIntake(IntakeRequest{Request: "Explain the API architecture and compare routing options"})
	if analysis.NeedsTools || analysis.NeedsLocalExecution {
		t.Fatalf("analysis-only request incorrectly requires runtime execution: %#v", analysis)
	}
}

func TestAnalyzeIntakeRequiresRuntimeForTechnicalImplementation(t *testing.T) {
	analysis := analyzeIntake(IntakeRequest{Request: "Implement API code and run repository tests"})
	if !analysis.NeedsTools || !analysis.NeedsLocalExecution {
		t.Fatalf("implementation request did not require controlled local execution: %#v", analysis)
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

func TestPlanScopesMemoryAndSourceSearchToOwnerAndSkipsGlobalRefresh(t *testing.T) {
	mem := &fakeMemoryService{}
	src := &fakeTaskSourceService{}
	service := NewService(mem, newTaskTestLLMService(t), src)

	plan, err := service.Plan(IntakeRequest{
		OwnerIdentity: "alice",
		Request:       "Summarize local project files and source context",
		ProjectKey:    "018-HAI",
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.OwnerIdentity != "alice" {
		t.Fatalf("plan owner = %q, want alice", plan.OwnerIdentity)
	}
	if len(mem.ownerRetrieveOwners) != 1 || mem.ownerRetrieveOwners[0] != "alice" {
		t.Fatalf("owner-scoped memory retrieval = %#v, want alice", mem.ownerRetrieveOwners)
	}
	if src.refreshCalls != 0 {
		t.Fatalf("owner-scoped task triggered global source refresh %d times", src.refreshCalls)
	}
	if len(src.ownerRefreshOwners) != 1 || src.ownerRefreshOwners[0] != "alice" {
		t.Fatalf("owner-scoped refresh owners = %#v, want alice", src.ownerRefreshOwners)
	}
	if plan.ContextPlan.SourceRefresh == nil {
		t.Fatal("owner-scoped task did not retain its source refresh result")
	}
	if len(src.searchRequests) != 1 || src.searchRequests[0].OwnerIdentity != "alice" {
		t.Fatalf("source search requests = %#v, want owner alice", src.searchRequests)
	}
}

func TestOwnerScopedTaskHistoryAndReviewQueueDoNotLeakAcrossOwners(t *testing.T) {
	service := NewService(&fakeMemoryService{}, newTaskTestLLMService(t))
	scoped, ok := service.(OwnerScopedService)
	if !ok {
		t.Fatal("native task service does not implement OwnerScopedService")
	}
	if _, err := service.Plan(IntakeRequest{OwnerIdentity: "alice", Request: "Plan Alice project context"}); err != nil {
		t.Fatalf("Plan alice: %v", err)
	}
	if _, err := service.Plan(IntakeRequest{OwnerIdentity: "bob", Request: "Plan Bob project context"}); err != nil {
		t.Fatalf("Plan bob: %v", err)
	}
	if logs := scoped.LogsForOwner("alice"); len(logs) != 1 || logs[0].OwnerIdentity != "alice" {
		t.Fatalf("alice logs = %#v, want only Alice record", logs)
	}

	aliceReview, err := service.Run(IntakeRequest{OwnerIdentity: "alice", Request: "Delete Alice account data"})
	if err != nil || aliceReview.ReviewQueueItem == nil {
		t.Fatalf("Run alice high-risk task = %#v, %v", aliceReview, err)
	}
	bobReview, err := service.Run(IntakeRequest{OwnerIdentity: "bob", Request: "Delete Bob account data"})
	if err != nil || bobReview.ReviewQueueItem == nil {
		t.Fatalf("Run bob high-risk task = %#v, %v", bobReview, err)
	}
	if queue := scoped.ReviewQueueForOwner("alice"); len(queue) != 1 || queue[0].Request.OwnerIdentity != "alice" {
		t.Fatalf("alice review queue = %#v, want only Alice item", queue)
	}
	if _, err := scoped.ResolveReviewItemForOwner("alice", bobReview.ReviewQueueItem.ID, ApprovalDecision{Approved: false}); err == nil {
		t.Fatal("expected cross-owner review resolution to be rejected")
	}
	if _, err := scoped.ResolveReviewItemForOwner("alice", aliceReview.ReviewQueueItem.ID, ApprovalDecision{Approved: false, Note: "not approved"}); err != nil {
		t.Fatalf("owner could not resolve own review item: %v", err)
	}
}

func TestPursuitScopedTaskRunPersistsStartAndFinalOutcome(t *testing.T) {
	recorder := &fakePursuitAttemptRecorder{}
	service := NewServiceWithEnginesAndPursuitAttempts(
		&fakeMemoryService{},
		newTaskTestLLMService(t),
		nil,
		nil,
		nil,
		recorder,
	)
	pursuitID := uuid.New()
	plan, err := service.Run(IntakeRequest{
		OwnerIdentity:  "alice",
		PursuitID:      pursuitID.String(),
		Request:        "Delete account data after legal review with api_key=plain-text-secret",
		ProjectKey:     "018-HAI",
		ExecuteAllowed: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(recorder.attempts) != 2 {
		t.Fatalf("persist calls = %d, want start and final outcome", len(recorder.attempts))
	}
	final := recorder.attempts[1]
	if final.PursuitID != pursuitID || final.TaskPlanID != plan.ID || final.Status != "review_required" {
		t.Fatalf("final pursuit attempt = %#v", final)
	}
	if final.CompletedAt == nil || final.BlockedReason == "" {
		t.Fatalf("final attempt lacks completion audit: %#v", final)
	}
	if strings.Contains(final.RequestSummary, "plain-text-secret") {
		t.Fatalf("task attempt leaked request secret: %#v", final)
	}
}

func TestPursuitScopedTaskPlanRejectsMalformedPursuitID(t *testing.T) {
	service := NewServiceWithEnginesAndPursuitAttempts(
		&fakeMemoryService{},
		newTaskTestLLMService(t),
		nil,
		nil,
		nil,
		&fakePursuitAttemptRecorder{},
	)
	if _, err := service.Plan(IntakeRequest{Request: "Plan a bounded review", PursuitID: "not-a-uuid"}); err == nil {
		t.Fatal("expected malformed pursuit id to be rejected")
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

func TestRunBlocksExecutionWhenEmergencyStopActive(t *testing.T) {
	t.Setenv("HAI_EMERGENCY_STOP", "true")
	mem := &fakeMemoryService{}
	llmService := newTaskTestLLMService(t)
	service := NewService(mem, llmService)

	plan, err := service.Run(IntakeRequest{
		Request:        "Create a low-risk admin checklist",
		ProjectKey:     "018-HAI",
		ExecuteAllowed: true,
		HumanApproved:  true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if plan.CompletionStatus != "review_required" {
		t.Fatalf("status = %q, want review_required", plan.CompletionStatus)
	}
	if plan.ExecutionResult == nil || plan.ExecutionResult.BlockedReason == "" {
		t.Fatalf("expected blocked execution result, got %#v", plan.ExecutionResult)
	}
	if plan.ReviewQueueItem == nil {
		t.Fatalf("expected review queue item")
	}
}

func TestRunWithoutExecutionPermissionQueuesReviewForToolWork(t *testing.T) {
	mem := &fakeMemoryService{}
	llmService := newTaskTestLLMService(t)
	service := NewService(mem, llmService)

	plan, err := service.Run(IntakeRequest{
		Request:    "Run local Docker build and tests for the project",
		ProjectKey: "018-HAI",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if plan.CompletionStatus != "review_required" {
		t.Fatalf("status = %q, want review_required", plan.CompletionStatus)
	}
	if plan.ExecutionResult != nil {
		t.Fatalf("execution result = %#v, want nil when execution was not explicitly allowed", plan.ExecutionResult)
	}
	if len(service.ReviewQueue()) == 0 {
		t.Fatalf("expected review queue entry")
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

func TestRunToolTaskRequiresConfiguredControlledRuntime(t *testing.T) {
	mem := &fakeMemoryService{}
	llmService := newTaskTestLLMService(t)
	service := NewService(mem, llmService)

	plan, err := service.Run(IntakeRequest{
		Request:        "Run local script tests for the project",
		ProjectKey:     "018-HAI",
		AutomationID:   uuid.NewString(),
		ExecuteAllowed: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if plan.CompletionStatus != "review_required" {
		t.Fatalf("status = %q, want review_required", plan.CompletionStatus)
	}
	if plan.ExecutionResult == nil || plan.ExecutionResult.BlockedReason != "controlled runtime executor is not configured" {
		t.Fatalf("unexpected execution result: %#v", plan.ExecutionResult)
	}
	if plan.RetryPolicy.RetryAvailable {
		t.Fatalf("configuration blockers must not be retried automatically")
	}
}

func TestRunToolTaskExecutesConfiguredAutomation(t *testing.T) {
	mem := &fakeMemoryService{}
	llmService := newTaskTestLLMService(t)
	executor := &fakeToolExecutor{result: completedToolResult()}
	service := NewServiceWithEngines(mem, llmService, nil, nil, executor)

	plan, err := service.Run(IntakeRequest{
		Request:        "Run local script tests for the project",
		ProjectKey:     "018-HAI",
		AutomationID:   executor.result.AutomationID,
		ExecuteAllowed: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if plan.CompletionStatus != "validated" {
		t.Fatalf("status = %q, want validated: %#v", plan.CompletionStatus, plan.ValidationResult)
	}
	if executor.calls != 1 {
		t.Fatalf("runtime calls = %d, want 1", executor.calls)
	}
	if plan.ExecutionResult == nil || plan.ExecutionResult.ToolExecution == nil {
		t.Fatalf("expected persisted runtime evidence")
	}
}

func TestRunToolTaskUsesLaunchEventURIAsRuntimeEvidence(t *testing.T) {
	mem := &fakeMemoryService{}
	llmService := newTaskTestLLMService(t)
	executor := &fakeToolExecutor{result: completedToolResult()}
	verifier := &sequencedVerificationService{}
	service := NewServiceWithEngines(mem, llmService, nil, verifier, executor)

	plan, err := service.Run(IntakeRequest{
		Request:        "Run local script tests and verify exact runtime evidence",
		ProjectKey:     "018-HAI",
		AutomationID:   executor.result.AutomationID,
		ExecuteAllowed: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if plan.CompletionStatus != "validated" {
		t.Fatalf("status = %q, want validated", plan.CompletionStatus)
	}
	if len(verifier.requests) == 0 {
		t.Fatalf("verification service was not called")
	}
	foundRuntimeEvidence := false
	for _, evidence := range verifier.requests[len(verifier.requests)-1].ExternalEvidence {
		if evidence.SourceType == "controlled_runtime" {
			foundRuntimeEvidence = true
			if evidence.SourceID != executor.result.LaunchEventID || evidence.SourceURI != "automation-launch://"+executor.result.LaunchEventID {
				t.Fatalf("runtime evidence did not use launch event id: %#v", evidence)
			}
			if !strings.Contains(evidence.Snippet, "runtime=openclaw") || !strings.Contains(evidence.Snippet, "skills=autoreview, gitcrawl") {
				t.Fatalf("runtime route trace missing from evidence snippet: %#v", evidence)
			}
		}
	}
	if !foundRuntimeEvidence {
		t.Fatalf("controlled runtime evidence missing: %#v", verifier.requests[len(verifier.requests)-1].ExternalEvidence)
	}
}

func TestRunToolTaskBlocksNilRuntimeResult(t *testing.T) {
	mem := &fakeMemoryService{}
	llmService := newTaskTestLLMService(t)
	executor := &fakeToolExecutor{}
	service := NewServiceWithEngines(mem, llmService, nil, nil, executor)

	plan, err := service.Run(IntakeRequest{
		Request:        "Run local script tests for the project",
		ProjectKey:     "018-HAI",
		AutomationID:   uuid.NewString(),
		ExecuteAllowed: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if plan.CompletionStatus != "review_required" || plan.ExecutionResult == nil {
		t.Fatalf("nil runtime result was not blocked: %#v", plan)
	}
	if plan.ExecutionResult.BlockedReason != "controlled runtime execution returned no result" {
		t.Fatalf("blocked reason = %q", plan.ExecutionResult.BlockedReason)
	}
}

func TestValidationRetryReusesSuccessfulRuntimeExecution(t *testing.T) {
	mem := &fakeMemoryService{}
	llmService := newTaskTestLLMService(t)
	executor := &fakeToolExecutor{result: completedToolResult()}
	verifier := &sequencedVerificationService{
		statuses: []string{verification.StatusNeedsReview, verification.StatusSourceSupported},
	}
	service := NewServiceWithEngines(mem, llmService, nil, verifier, executor)

	plan, err := service.Run(IntakeRequest{
		Request:        "Run local script tests and verify the result",
		ProjectKey:     "018-HAI",
		AutomationID:   executor.result.AutomationID,
		ExecuteAllowed: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if plan.CompletionStatus != "validated" || plan.RetryPolicy.CurrentAttempt != 2 {
		t.Fatalf("retry did not validate: status=%q retry=%#v", plan.CompletionStatus, plan.RetryPolicy)
	}
	if executor.calls != 1 {
		t.Fatalf("runtime executed %d times, want exactly once", executor.calls)
	}
	if !hasTaskAction(plan.ExecutionResult.Actions, "automation.launch", "reused") {
		t.Fatalf("expected retry to record reused runtime evidence")
	}
}

func TestApprovedReviewIsNotFalselyCompletedWhenRuntimeStillBlocked(t *testing.T) {
	mem := &fakeMemoryService{}
	llmService := newTaskTestLLMService(t)
	service := NewService(mem, llmService)
	plan, err := service.Run(IntakeRequest{
		Request:      "Delete account data by running a local script",
		ProjectKey:   "018-HAI",
		AutomationID: uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	result, err := service.ResolveReviewItem(plan.ReviewQueueItem.ID, ApprovalDecision{
		Approved: true,
		Note:     "Approved only for the configured controlled runtime.",
	})
	if err != nil {
		t.Fatalf("ResolveReviewItem: %v", err)
	}
	if result.Item.Status != "needs_review" {
		t.Fatalf("review status = %q, want needs_review", result.Item.Status)
	}
	if result.Plan == nil || result.Plan.CompletionStatus != "review_required" {
		t.Fatalf("blocked approved task was falsely completed: %#v", result.Plan)
	}
	if queue := service.ReviewQueue(); len(queue) != 1 {
		t.Fatalf("blocked approval created duplicate review items: %#v", queue)
	}
	if _, err := service.ResolveReviewItem(result.Item.ID, ApprovalDecision{Approved: false, Note: "Reject until runtime is configured."}); err != nil {
		t.Fatalf("needs_review item could not be resolved again: %v", err)
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

type fakeMemoryService struct {
	ownerCreateOwners   []string
	ownerRetrieveOwners []string
}

type fakeToolExecutor struct {
	result   *ToolExecutionResult
	err      error
	calls    int
	requests []ToolExecutionRequest
}

func (f *fakeToolExecutor) Execute(request ToolExecutionRequest) (*ToolExecutionResult, error) {
	f.calls++
	f.requests = append(f.requests, request)
	return f.result, f.err
}

func completedToolResult() *ToolExecutionResult {
	launchEventID := uuid.NewString()
	return &ToolExecutionResult{
		AutomationID:  uuid.NewString(),
		LaunchEventID: launchEventID,
		RuntimeType:   "script",
		LaunchType:    "script",
		Target:        "verify-project.sh",
		Status:        "completed",
		Message:       "script completed",
		Output:        "build and tests passed",
		RuntimeRouteTrace: &models.AutomationRuntimeRouteTrace{
			RuntimeID:         "openclaw",
			Intent:            "code_review",
			ExecutionMode:     "read_only",
			RiskLevel:         "medium",
			RecommendedSkills: []string{"autoreview", "gitcrawl"},
			BlockedSurfaces:   []string{"external_message_sending"},
		},
		ExitCode:    0,
		DurationMs:  25,
		AuditEvents: []string{"script executed without shell"},
		ExecutedAt:  time.Now().UTC(),
	}
}

type sequencedVerificationService struct {
	statuses []string
	calls    int
	requests []verification.AnswerRequest
}

func (s *sequencedVerificationService) Answer(request verification.AnswerRequest) (*verification.VerificationResult, error) {
	s.requests = append(s.requests, request)
	status := verification.StatusSourceSupported
	if s.calls < len(s.statuses) {
		status = s.statuses[s.calls]
	}
	s.calls++
	run := models.VerificationRun{
		ID:       uuid.New(),
		Answer:   "controlled runtime evidence checked",
		Status:   status,
		Question: request.Question,
	}
	claims := []models.VerificationClaim{{
		ID:          uuid.New(),
		ClaimText:   "controlled runtime evidence checked",
		Status:      status,
		Confidence:  0.9,
		NeedsReview: status == verification.StatusNeedsReview,
	}}
	result := &verification.VerificationResult{Run: run, Claims: claims}
	if status == verification.StatusNeedsReview {
		result.UnsupportedClaims = claims
	}
	return result, nil
}

func (s *sequencedVerificationService) Runs() ([]models.VerificationRun, error) {
	return nil, nil
}

func (s *sequencedVerificationService) RunsForOwner(string) ([]models.VerificationRun, error) {
	return nil, nil
}

func (s *sequencedVerificationService) RunDetails(id uuid.UUID) (*verification.VerificationResult, error) {
	return nil, nil
}

func (s *sequencedVerificationService) RunDetailsForOwner(string, uuid.UUID) (*verification.VerificationResult, error) {
	return nil, nil
}

func hasTaskAction(actions []ExecutedAction, name, status string) bool {
	for _, action := range actions {
		if action.Name == name && action.Status == status {
			return true
		}
	}
	return false
}

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

func (f *fakeMemoryService) CreateForOwner(ownerIdentity string, request memory.CreateRequest) (*models.ContextMemory, error) {
	f.ownerCreateOwners = append(f.ownerCreateOwners, ownerIdentity)
	created, err := f.Create(request)
	if created != nil {
		created.OwnerIdentity = ownerIdentity
	}
	return created, err
}

type fakePursuitAttemptRecorder struct {
	attempts []models.PursuitTaskAttempt
}

func (f *fakePursuitAttemptRecorder) UpsertTaskAttempt(attempt models.PursuitTaskAttempt) error {
	f.attempts = append(f.attempts, attempt)
	return nil
}

type fakeTaskSourceService struct {
	refreshCalls       int
	ownerRefreshOwners []string
	searchCalls        int
	order              []string
	searchRequests     []source.SearchRequest
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

func (s *fakeTaskSourceService) SyncJobs(sourceID *uuid.UUID) ([]models.SourceSyncJob, error) {
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

func (s *fakeTaskSourceService) RunDueScheduledSyncsForOwner(now time.Time, ownerIdentity string) (*source.ScheduledSyncRun, error) {
	s.ownerRefreshOwners = append(s.ownerRefreshOwners, ownerIdentity)
	s.order = append(s.order, "owner-refresh")
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
	s.searchRequests = append(s.searchRequests, request)
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

func (f *fakeMemoryService) UpdateForOwner(_ string, id uuid.UUID, request memory.UpdateRequest) (*models.ContextMemory, error) {
	return f.Update(id, request)
}

func (f *fakeMemoryService) FindAllForOwner(_ string, projectKey string, includeArchived bool) ([]models.ContextMemory, error) {
	return f.FindAll(projectKey, includeArchived)
}

func (f *fakeMemoryService) FindByIDForOwner(_ string, id uuid.UUID) (*models.ContextMemory, error) {
	return f.FindByID(id)
}

func (f *fakeMemoryService) ArchiveForOwner(_ string, id uuid.UUID, archived bool) (*models.ContextMemory, error) {
	return f.Archive(id, archived)
}

func (f *fakeMemoryService) DeleteForOwner(_ string, id uuid.UUID) error {
	return f.Delete(id)
}

func (f *fakeMemoryService) RetrieveForOwner(ownerIdentity string, request memory.RetrieveRequest) (*memory.RetrieveResult, error) {
	f.ownerRetrieveOwners = append(f.ownerRetrieveOwners, ownerIdentity)
	return f.Retrieve(request)
}
