package memoryengine

import (
	"automation-hub-backend/internal/memory"
	"automation-hub-backend/internal/models"
	pursuitpkg "automation-hub-backend/internal/pursuit"
	"automation-hub-backend/internal/workflow"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestNormalizeImportRejectsMismatchedPlatformHost(t *testing.T) {
	_, _, _, err := normalizeImport(ImportRequest{
		Platform:  "chatgpt",
		SourceURI: "https://gemini.google.com/app/example",
		Messages:  []ChatMessage{{Role: "user", Content: "Build the dashboard."}},
	})
	if err == nil {
		t.Fatalf("mismatched platform host was accepted")
	}
}

func TestNormalizeImportProducesStableHash(t *testing.T) {
	request := ImportRequest{
		Platform:   "chatgpt",
		ExternalID: "thread-1",
		Title:      "Dashboard",
		SourceURI:  "https://chatgpt.com/c/thread-1",
		Messages: []ChatMessage{
			{Role: "user", Content: "We need to build the dashboard."},
			{Role: "assistant", Content: "Decision: use a local-first architecture."},
		},
	}
	_, _, first, err := normalizeImport(request)
	if err != nil {
		t.Fatalf("normalize first: %v", err)
	}
	_, _, second, err := normalizeImport(request)
	if err != nil {
		t.Fatalf("normalize second: %v", err)
	}
	if first != second || len(first) != 64 {
		t.Fatalf("hash is not stable: %q/%q", first, second)
	}
}

func TestExtractInsightsRedactsSecretsAndClassifiesActions(t *testing.T) {
	conversation := models.AIConversationArchive{
		Platform:  "chatgpt",
		Title:     "HAI",
		SourceURI: "https://chatgpt.com/c/thread-1",
	}
	insights := extractInsights(conversation, ImportRequest{
		ProjectKey: "018-HAI",
		Messages: []ChatMessage{{
			Role:    "assistant",
			Content: "Action: VA should create the Trello card. Never store password=secret-value in memory.",
		}},
	})
	if len(insights) != 2 {
		t.Fatalf("insights = %#v, want action and rule", insights)
	}
	for _, insight := range insights {
		if strings.Contains(insight.Text, "secret-value") {
			t.Fatalf("secret was retained in operational insight: %q", insight.Text)
		}
	}
	if insights[0].Kind != "action" || insights[0].Owner != "VA" || insights[0].ProjectKey != "018-HAI" {
		t.Fatalf("action insight = %#v", insights[0])
	}
}

func TestClassifySentenceTreatsExplicitRiskAsRiskBeforeAction(t *testing.T) {
	got := classifySentence("Risk: legal reply may create a government consequence if unsupported claims are sent")
	if got != "risk" {
		t.Fatalf("classification = %q, want risk", got)
	}
}

func TestEncryptedPayloadRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	plaintext := []byte(`{"messages":[{"role":"user","content":"private"}]}`)
	ciphertext, nonce, err := encryptPayload(key, plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if strings.Contains(string(ciphertext), "private") {
		t.Fatalf("plaintext leaked into ciphertext")
	}
	decrypted, err := decryptPayload(key, nonce, ciphertext)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Fatalf("round trip = %q, want %q", decrypted, plaintext)
	}
}

func TestImportAutoLinksActionWorkflowToPursuit(t *testing.T) {
	repo := &memoryEngineRepoStub{}
	workflowID := uuid.New()
	workflowSpy := &memoryEngineWorkflowStub{recordID: workflowID}
	pursuitSpy := &memoryEnginePursuitLinker{}
	service := NewServiceWithPursuitLinker(
		repo,
		&memoryEngineMemoryStub{},
		workflowSpy,
		"test-memory-encryption-secret",
		pursuitSpy,
	)

	result, err := service.Import(ImportRequest{
		Platform:   "chatgpt",
		ExternalID: "thread-vivare-action",
		Title:      "Vivare legal dispute",
		SourceURI:  "https://chatgpt.com/c/thread-vivare-action",
		ProjectKey: "vivare",
		Messages: []ChatMessage{{
			Role:    "assistant",
			Content: "Action: draft the legal reply for Vivare and attach evidence.",
		}},
	})
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if len(result.WorkflowIDs) != 1 || result.WorkflowIDs[0] != workflowID {
		t.Fatalf("workflow ids = %#v, want %s", result.WorkflowIDs, workflowID)
	}
	if len(result.PursuitLinks) != 1 || !result.PursuitLinks[0].Linked || result.PursuitLinks[0].PursuitID == uuid.Nil {
		t.Fatalf("pursuit link result = %#v, want visible linked pursuit", result.PursuitLinks)
	}
	if result.PursuitLinks[0].Message == "" {
		t.Fatalf("pursuit link message missing from import result: %#v", result.PursuitLinks[0])
	}
	if len(workflowSpy.intakeRequests) != 1 {
		t.Fatalf("workflow intake requests = %d, want 1", len(workflowSpy.intakeRequests))
	}
	intake := workflowSpy.intakeRequests[0]
	if intake.ProjectKey != "vivare" || intake.SourceType != "ai_chat" || intake.Trigger != "memory_engine.import" {
		t.Fatalf("workflow intake = %#v", intake)
	}
	if !intake.RequiresReview || !strings.Contains(intake.ReviewReason, "Robert") {
		t.Fatalf("workflow review gate = %#v", intake)
	}
	if len(pursuitSpy.requests) != 1 {
		t.Fatalf("pursuit linker requests = %d, want 1", len(pursuitSpy.requests))
	}
	linkRequest := pursuitSpy.requests[0]
	if linkRequest.WorkflowID != workflowID || linkRequest.ProjectKey != "vivare" || linkRequest.SourceType != "ai_chat" {
		t.Fatalf("pursuit link request = %#v", linkRequest)
	}
	if !linkRequest.AllowCreateCandidate {
		t.Fatalf("AI-chat action workflows must be allowed to create reviewable pursuit candidates when no match exists")
	}
	if !strings.Contains(linkRequest.SourceID, result.Conversation.ID.String()+":") {
		t.Fatalf("source id = %q, want conversation:insight identity", linkRequest.SourceID)
	}
	if linkRequest.SourceURI != "https://chatgpt.com/c/thread-vivare-action" || linkRequest.SourceLabel != "chatgpt: Vivare legal dispute" {
		t.Fatalf("source ref = %q/%q", linkRequest.SourceURI, linkRequest.SourceLabel)
	}
}

func TestImportRoutesContradictionsAndHighRisksToGovernedWorkflows(t *testing.T) {
	repo := &memoryEngineRepoStub{}
	workflowSpy := &memoryEngineWorkflowStub{}
	pursuitSpy := &memoryEnginePursuitLinker{}
	service := NewServiceWithPursuitLinker(
		repo,
		&memoryEngineMemoryStub{},
		workflowSpy,
		"test-memory-encryption-secret",
		pursuitSpy,
	)

	result, err := service.Import(ImportRequest{
		Platform:   "chatgpt",
		ExternalID: "thread-review-critical",
		Title:      "Vivare evidence review",
		SourceURI:  "https://chatgpt.com/c/thread-review-critical",
		ProjectKey: "vivare",
		Messages: []ChatMessage{
			{
				Role:    "assistant",
				Content: "Contradiction: Vivare says the document was never sent but the email evidence shows it was sent in March.",
			},
			{
				Role:    "assistant",
				Content: "Risk: legal reply may create a government or financial consequence if unsupported claims are sent.",
			},
		},
	})
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if len(result.WorkflowIDs) != 2 {
		t.Fatalf("workflow ids = %#v, want two review-critical workflows", result.WorkflowIDs)
	}
	if len(workflowSpy.intakeRequests) != 2 {
		t.Fatalf("workflow intake requests = %d, want 2", len(workflowSpy.intakeRequests))
	}
	contentTypes := map[string]workflow.IntakeRequest{}
	for _, request := range workflowSpy.intakeRequests {
		contentTypes[request.ContentType] = request
	}
	for _, contentType := range []string{"ai_chat_contradiction", "ai_chat_risk"} {
		request, ok := contentTypes[contentType]
		if !ok {
			t.Fatalf("missing workflow intake content type %s in %#v", contentType, workflowSpy.intakeRequests)
		}
		if request.ProjectKey != "vivare" || request.SourceType != "ai_chat" || request.Trigger != "memory_engine.import" {
			t.Fatalf("workflow intake %s = %#v", contentType, request)
		}
		if !request.RequiresReview || request.ReviewReason == "" {
			t.Fatalf("workflow intake %s missing review gate: %#v", contentType, request)
		}
	}
	if len(pursuitSpy.requests) != 2 {
		t.Fatalf("pursuit workflow link requests = %d, want 2", len(pursuitSpy.requests))
	}
	for _, request := range pursuitSpy.requests {
		if !request.AllowCreateCandidate || request.ProjectKey != "vivare" || request.SourceType != "ai_chat" {
			t.Fatalf("pursuit link request = %#v, want candidate-capable ai_chat request", request)
		}
	}
}

func TestImportDoesNotCreateWorkflowForLowRiskPassiveRiskNote(t *testing.T) {
	repo := &memoryEngineRepoStub{}
	workflowSpy := &memoryEngineWorkflowStub{}
	service := NewServiceWithPursuitLinker(
		repo,
		&memoryEngineMemoryStub{},
		workflowSpy,
		"test-memory-encryption-secret",
		nil,
	)

	result, err := service.Import(ImportRequest{
		Platform:   "chatgpt",
		ExternalID: "thread-passive-risk",
		Title:      "Dashboard polish",
		SourceURI:  "https://chatgpt.com/c/thread-passive-risk",
		ProjectKey: "018-HAI",
		Messages: []ChatMessage{{
			Role:    "assistant",
			Content: "Risk: a small visual inconsistency could fail if the browser cache is stale during local review.",
		}},
	})
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if len(result.WorkflowIDs) != 0 || len(workflowSpy.intakeRequests) != 0 {
		t.Fatalf("low-risk passive risk created workflow ids=%#v intakes=%#v", result.WorkflowIDs, workflowSpy.intakeRequests)
	}
}

func TestImportAutoLinksStableMemoryToPursuit(t *testing.T) {
	repo := &memoryEngineRepoStub{}
	pursuitSpy := &memoryEnginePursuitLinker{}
	service := NewServiceWithPursuitLinker(
		repo,
		&memoryEngineMemoryStub{},
		nil,
		"test-memory-encryption-secret",
		pursuitSpy,
	)

	result, err := service.Import(ImportRequest{
		Platform:   "chatgpt",
		ExternalID: "thread-vivare-rule",
		Title:      "Vivare legal dispute",
		SourceURI:  "https://chatgpt.com/c/thread-vivare-rule",
		ProjectKey: "vivare",
		Messages: []ChatMessage{{
			Role:    "assistant",
			Content: "Rule: Always use formal Dutch tone for Vivare correspondence.",
		}},
	})
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if len(result.WorkflowIDs) != 0 {
		t.Fatalf("workflow ids = %#v, want none for stable memory insight", result.WorkflowIDs)
	}
	if len(result.PursuitLinks) != 1 || !result.PursuitLinks[0].Linked {
		t.Fatalf("pursuit memory link result = %#v, want visible linked pursuit", result.PursuitLinks)
	}
	if len(pursuitSpy.memoryRequests) != 1 {
		t.Fatalf("memory link requests = %d, want 1", len(pursuitSpy.memoryRequests))
	}
	linkRequest := pursuitSpy.memoryRequests[0]
	if linkRequest.MemoryID == uuid.Nil || linkRequest.ProjectKey != "vivare" {
		t.Fatalf("memory link request = %#v", linkRequest)
	}
	if linkRequest.AllowCreateCandidate {
		t.Fatalf("stable memory records should not create new pursuits without an explicit workflow/action signal")
	}
	if !strings.Contains(linkRequest.Input, "formal Dutch tone") {
		t.Fatalf("memory link input = %q, want insight text", linkRequest.Input)
	}
	if linkRequest.SourceURI != "https://chatgpt.com/c/thread-vivare-rule" || linkRequest.SourceLabel != "chatgpt: Vivare legal dispute" {
		t.Fatalf("source ref = %q/%q", linkRequest.SourceURI, linkRequest.SourceLabel)
	}
}

func TestDashboardSurfacesSourceCorrectionLessons(t *testing.T) {
	now := time.Now().UTC()
	service := NewService(
		&memoryEngineRepoStub{},
		&memoryEngineMemoryStub{memories: []models.ContextMemory{
			{
				ID:          uuid.New(),
				ProjectKey:  "018-HAI",
				Kind:        "lesson",
				Content:     "Robert corrected source extraction behavior.",
				Summary:     "Use corrected evidence checklist extraction.",
				Tags:        "connected-source,source-correction,correction,email",
				Confidence:  0.78,
				SourceURI:   "local://correction",
				SourceLabel: "Corrected email",
				UpdatedAt:   now,
			},
			{
				ID:         uuid.New(),
				ProjectKey: "018-HAI",
				Kind:       "source",
				Content:    "Generic source summary",
				Tags:       "connected-source,email",
				Confidence: 0.68,
				UpdatedAt:  now.Add(time.Minute),
			},
			{
				ID:          uuid.New(),
				ProjectKey:  "archived",
				Kind:        "lesson",
				Content:     "Archived correction",
				Tags:        "source-correction",
				Confidence:  0.9,
				Archived:    true,
				UpdatedAt:   now.Add(time.Hour),
				SourceLabel: "Archived",
			},
		}},
		nil,
		"",
	)

	dashboard, err := service.Dashboard()
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if len(dashboard.SourceCorrections) != 1 {
		t.Fatalf("source corrections = %#v, want one active correction lesson", dashboard.SourceCorrections)
	}
	correction := dashboard.SourceCorrections[0]
	if correction.ProjectKey != "018-HAI" || correction.SourceLabel != "Corrected email" {
		t.Fatalf("correction = %#v, want project-scoped source correction", correction)
	}
	if len(dashboard.Projects) != 1 || dashboard.Projects[0].ProjectKey != "018-HAI" || dashboard.Projects[0].Corrections != 1 {
		t.Fatalf("project summaries = %#v, want source correction count", dashboard.Projects)
	}
}

type memoryEngineRepoStub struct {
	conversations []models.AIConversationArchive
	insights      []models.AIMemoryInsight
}

func (r *memoryEngineRepoStub) FindConversation(platform, externalID string) (*models.AIConversationArchive, error) {
	return nil, gorm.ErrRecordNotFound
}

func (r *memoryEngineRepoStub) FindConversationByID(id uuid.UUID) (*models.AIConversationArchive, error) {
	return nil, gorm.ErrRecordNotFound
}

func (r *memoryEngineRepoStub) SaveConversation(conversation *models.AIConversationArchive) (*models.AIConversationArchive, error) {
	if conversation.ID == uuid.Nil {
		conversation.ID = uuid.New()
	}
	return conversation, nil
}

func (r *memoryEngineRepoStub) FindConversations(limit int) ([]models.AIConversationArchive, error) {
	return r.conversations, nil
}

func (r *memoryEngineRepoStub) DeleteConversation(id uuid.UUID) error {
	return nil
}

func (r *memoryEngineRepoStub) SaveInsight(insight *models.AIMemoryInsight) (*models.AIMemoryInsight, error) {
	if insight.ID == uuid.Nil {
		insight.ID = uuid.New()
	}
	return insight, nil
}

func (r *memoryEngineRepoStub) FindInsights(kind, projectKey string, needsReview *bool, limit int) ([]models.AIMemoryInsight, error) {
	result := []models.AIMemoryInsight{}
	for _, insight := range r.insights {
		if kind != "" && insight.Kind != kind {
			continue
		}
		if projectKey != "" && insight.ProjectKey != projectKey {
			continue
		}
		if needsReview != nil && insight.NeedsReview != *needsReview {
			continue
		}
		result = append(result, insight)
	}
	return result, nil
}

func (r *memoryEngineRepoStub) ArchiveInsights(conversationID uuid.UUID, revision int) error {
	return nil
}

func (r *memoryEngineRepoStub) DeleteMemoriesBySourceURI(sourceURI string) error {
	return nil
}

type memoryEngineMemoryStub struct {
	memories []models.ContextMemory
}

func (s *memoryEngineMemoryStub) Create(request memory.CreateRequest) (*models.ContextMemory, error) {
	return &models.ContextMemory{
		ID:          uuid.New(),
		ProjectKey:  request.ProjectKey,
		Kind:        request.Kind,
		Content:     request.Content,
		Summary:     request.Summary,
		Confidence:  request.Confidence,
		SourceURI:   request.SourceURI,
		SourceLabel: request.SourceLabel,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}, nil
}

func (s *memoryEngineMemoryStub) Update(id uuid.UUID, request memory.UpdateRequest) (*models.ContextMemory, error) {
	return &models.ContextMemory{ID: id, Content: request.Content, Kind: request.Kind}, nil
}

func (s *memoryEngineMemoryStub) FindAll(projectKey string, includeArchived bool) ([]models.ContextMemory, error) {
	result := []models.ContextMemory{}
	for _, item := range s.memories {
		if projectKey != "" && item.ProjectKey != projectKey {
			continue
		}
		if !includeArchived && item.Archived {
			continue
		}
		result = append(result, item)
	}
	return result, nil
}

func (s *memoryEngineMemoryStub) FindByID(id uuid.UUID) (*models.ContextMemory, error) {
	return nil, gorm.ErrRecordNotFound
}

func (s *memoryEngineMemoryStub) Archive(id uuid.UUID, archived bool) (*models.ContextMemory, error) {
	return &models.ContextMemory{ID: id, Archived: archived}, nil
}

func (s *memoryEngineMemoryStub) Delete(id uuid.UUID) error {
	return nil
}

func (s *memoryEngineMemoryStub) Retrieve(request memory.RetrieveRequest) (*memory.RetrieveResult, error) {
	return &memory.RetrieveResult{Query: request.Query, ProjectKey: request.ProjectKey}, nil
}

type memoryEngineWorkflowStub struct {
	intakeRequests []workflow.IntakeRequest
	recordID       uuid.UUID
}

func (s *memoryEngineWorkflowStub) Intake(request workflow.IntakeRequest) (*workflow.WorkflowRecord, error) {
	s.intakeRequests = append(s.intakeRequests, request)
	if s.recordID == uuid.Nil {
		s.recordID = uuid.New()
	}
	return &workflow.WorkflowRecord{Item: models.WorkflowItem{ID: s.recordID, Title: request.Input, ProjectKey: request.ProjectKey}}, nil
}

func (s *memoryEngineWorkflowStub) Items(bool) ([]models.WorkflowItem, error) {
	return nil, nil
}

func (s *memoryEngineWorkflowStub) ApprovalItems() ([]models.WorkflowItem, error) {
	return nil, nil
}

func (s *memoryEngineWorkflowStub) Dashboard() (*workflow.WorkflowDashboard, error) {
	return &workflow.WorkflowDashboard{}, nil
}

func (s *memoryEngineWorkflowStub) Get(uuid.UUID) (*workflow.WorkflowRecord, error) {
	return nil, gorm.ErrRecordNotFound
}

func (s *memoryEngineWorkflowStub) Transition(uuid.UUID, workflow.TransitionRequest) (*workflow.WorkflowRecord, error) {
	return nil, nil
}

func (s *memoryEngineWorkflowStub) ResolveApproval(uuid.UUID, workflow.ApprovalResolutionRequest) (*workflow.WorkflowRecord, error) {
	return nil, nil
}

func (s *memoryEngineWorkflowStub) ResolveInterruptedExecution(uuid.UUID, workflow.InterruptedExecutionResolutionRequest) (*workflow.WorkflowRecord, error) {
	return nil, nil
}

func (s *memoryEngineWorkflowStub) ResolveProposal(uuid.UUID, uuid.UUID, workflow.ProposalResolutionRequest) (*workflow.WorkflowRecord, error) {
	return nil, nil
}

func (s *memoryEngineWorkflowStub) UpdateChecklistItem(uuid.UUID, uuid.UUID, workflow.ChecklistUpdateRequest) (*workflow.WorkflowRecord, error) {
	return nil, nil
}

func (s *memoryEngineWorkflowStub) RetractSource(string, string, string) error {
	return nil
}

func (s *memoryEngineWorkflowStub) RecoverStaleClaims(workflow.RunDueRequest) (*workflow.ClaimRecoverySummary, error) {
	return &workflow.ClaimRecoverySummary{}, nil
}

func (s *memoryEngineWorkflowStub) RunDue(workflow.RunDueRequest) (*workflow.WorkflowRunSummary, error) {
	return &workflow.WorkflowRunSummary{}, nil
}

func (s *memoryEngineWorkflowStub) RunDueOpenLoops(workflow.RunDueRequest) (*workflow.OpenLoopRunSummary, error) {
	return &workflow.OpenLoopRunSummary{}, nil
}

func (s *memoryEngineWorkflowStub) Overview() workflow.Overview {
	return workflow.Overview{}
}

type memoryEnginePursuitLinker struct {
	requests       []pursuitpkg.AutoLinkWorkflowRequest
	memoryRequests []pursuitpkg.AutoLinkMemoryRequest
}

func (s *memoryEnginePursuitLinker) AutoLinkWorkflow(request pursuitpkg.AutoLinkWorkflowRequest) (*pursuitpkg.AutoLinkResult, error) {
	s.requests = append(s.requests, request)
	return &pursuitpkg.AutoLinkResult{Linked: true, PursuitID: uuid.New(), Score: 0.8, Message: "source-derived workflow linked to pursuit"}, nil
}

func (s *memoryEnginePursuitLinker) AutoLinkMemory(request pursuitpkg.AutoLinkMemoryRequest) (*pursuitpkg.AutoLinkResult, error) {
	s.memoryRequests = append(s.memoryRequests, request)
	return &pursuitpkg.AutoLinkResult{Linked: true, PursuitID: uuid.New(), Score: 0.8, Message: "memory insight linked to pursuit"}, nil
}
