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
		OwnerIdentity: "alice",
		Platform:      "chatgpt",
		ExternalID:    "thread-vivare-action",
		Title:         "Vivare legal dispute",
		SourceURI:     "https://chatgpt.com/c/thread-vivare-action",
		ProjectKey:    "vivare",
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
	if intake.OwnerIdentity != "alice" {
		t.Fatalf("workflow owner = %q, want alice", intake.OwnerIdentity)
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

func TestImportRoutesActionWorkflowThroughPursuitGateway(t *testing.T) {
	repo := &memoryEngineRepoStub{}
	workflowSpy := &memoryEngineWorkflowStub{}
	pursuitGateway := &memoryEnginePursuitGateway{}
	service := NewServiceWithPursuitLinker(
		repo,
		&memoryEngineMemoryStub{},
		workflowSpy,
		"test-memory-encryption-secret",
		pursuitGateway,
	)

	result, err := service.Import(ImportRequest{
		Platform:   "chatgpt",
		ExternalID: "thread-pursuit-gateway",
		Title:      "Vivare legal dispute",
		SourceURI:  "https://chatgpt.com/c/thread-pursuit-gateway",
		ProjectKey: "vivare",
		Messages: []ChatMessage{{
			Role:    "assistant",
			Content: "Action: draft the legal reply for Vivare and attach evidence.",
		}},
	})
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if len(workflowSpy.intakeRequests) != 0 {
		t.Fatalf("direct workflow intake bypassed pursuit gateway: %#v", workflowSpy.intakeRequests)
	}
	if len(pursuitGateway.routed) != 1 || pursuitGateway.routed[0].Trigger != "memory_engine.import" {
		t.Fatalf("pursuit gateway requests = %#v", pursuitGateway.routed)
	}
	if len(result.WorkflowIDs) != 1 || result.WorkflowIDs[0] == uuid.Nil {
		t.Fatalf("workflow ids = %#v, want routed workflow", result.WorkflowIDs)
	}
	if len(pursuitGateway.requests) != 0 {
		t.Fatalf("workflow was linked twice after pursuit routing: %#v", pursuitGateway.requests)
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
		OwnerIdentity: "alice",
		Platform:      "chatgpt",
		ExternalID:    "thread-vivare-rule",
		Title:         "Vivare legal dispute",
		SourceURI:     "https://chatgpt.com/c/thread-vivare-rule",
		ProjectKey:    "vivare",
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
	if linkRequest.MemoryID == uuid.Nil || linkRequest.ProjectKey != "vivare" || linkRequest.OwnerIdentity != "alice" {
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

func TestImportedConversationAndInsightsAreScopedToOwner(t *testing.T) {
	repo := &memoryEngineRepoStub{}
	service := NewService(repo, &memoryEngineMemoryStub{}, nil, "test-memory-encryption-secret")

	alice, err := service.Import(ImportRequest{
		OwnerIdentity: "alice",
		Platform:      "chatgpt",
		ExternalID:    "shared-thread",
		Title:         "Alice thread",
		SourceURI:     "https://chatgpt.com/c/shared-thread",
		Messages:      []ChatMessage{{Role: "user", Content: "Follow up with the insurer about the claim evidence."}},
	})
	if err != nil {
		t.Fatalf("import alice: %v", err)
	}
	bob, err := service.Import(ImportRequest{
		OwnerIdentity: "bob",
		Platform:      "chatgpt",
		ExternalID:    "shared-thread",
		Title:         "Bob thread",
		SourceURI:     "https://chatgpt.com/c/shared-thread",
		Messages:      []ChatMessage{{Role: "user", Content: "Follow up with the developer about the deployment."}},
	})
	if err != nil {
		t.Fatalf("import bob: %v", err)
	}
	if alice.Conversation.ID == bob.Conversation.ID || alice.Conversation.OwnerIdentity != "alice" || bob.Conversation.OwnerIdentity != "bob" {
		t.Fatalf("owner-specific conversations = %#v / %#v", alice.Conversation, bob.Conversation)
	}

	aliceConversations, err := service.ConversationsForOwner("alice", 10)
	if err != nil || len(aliceConversations) != 1 || aliceConversations[0].ID != alice.Conversation.ID {
		t.Fatalf("alice conversations = %#v, err=%v", aliceConversations, err)
	}
	bobConversations, err := service.ConversationsForOwner("bob", 10)
	if err != nil || len(bobConversations) != 1 || bobConversations[0].ID != bob.Conversation.ID {
		t.Fatalf("bob conversations = %#v, err=%v", bobConversations, err)
	}

	if _, err := service.ConversationForOwner("bob", alice.Conversation.ID); err == nil {
		t.Fatal("bob could read alice conversation")
	}
	if err := service.DeleteConversationForOwner("bob", alice.Conversation.ID); err == nil {
		t.Fatal("bob could delete alice conversation")
	}
	aliceInsights, err := service.InsightsForOwner("alice", "", "", nil, 10)
	if err != nil || len(aliceInsights) == 0 || aliceInsights[0].OwnerIdentity != "alice" {
		t.Fatalf("alice insights = %#v, err=%v", aliceInsights, err)
	}
	for _, insight := range aliceInsights {
		if insight.OwnerIdentity != "alice" {
			t.Fatalf("alice received another owner's insight: %#v", insight)
		}
	}
}

func TestAuthenticatedImportDoesNotAdoptOwnerlessLegacyConversation(t *testing.T) {
	repo := &memoryEngineRepoStub{conversations: []models.AIConversationArchive{{
		ID:          uuid.New(),
		Platform:    "chatgpt",
		ExternalID:  "legacy-thread",
		SourceURI:   "https://chatgpt.com/c/legacy-thread",
		ContentHash: "legacy",
		Revision:    1,
	}}}
	service := NewService(repo, &memoryEngineMemoryStub{}, nil, "test-memory-encryption-secret")
	result, err := service.Import(ImportRequest{
		OwnerIdentity: "alice",
		Platform:      "chatgpt",
		ExternalID:    "legacy-thread",
		Title:         "Alice private continuation",
		SourceURI:     "https://chatgpt.com/c/legacy-thread",
		Messages:      []ChatMessage{{Role: "assistant", Content: "Rule: keep this private."}},
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Conversation.OwnerIdentity != "alice" || result.Conversation.ID == repo.conversations[0].ID {
		t.Fatalf("authenticated import adopted legacy archive: %#v", result.Conversation)
	}
	if len(repo.conversations) != 2 || repo.conversations[0].OwnerIdentity != "" {
		t.Fatalf("legacy archive was overwritten instead of retained separately: %#v", repo.conversations)
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
	return r.FindConversationForOwner("", platform, externalID)
}

func (r *memoryEngineRepoStub) FindConversationForOwner(ownerIdentity, platform, externalID string) (*models.AIConversationArchive, error) {
	for _, conversation := range r.conversations {
		if conversation.Platform == platform && conversation.ExternalID == externalID && !conversation.Archived && memoryEngineRecordVisibleTo(conversation.OwnerIdentity, ownerIdentity) {
			copyConversation := conversation
			return &copyConversation, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *memoryEngineRepoStub) FindConversationByID(id uuid.UUID) (*models.AIConversationArchive, error) {
	return r.FindConversationByIDForOwner("", id)
}

func (r *memoryEngineRepoStub) FindConversationByIDForOwner(ownerIdentity string, id uuid.UUID) (*models.AIConversationArchive, error) {
	for _, conversation := range r.conversations {
		if conversation.ID == id && memoryEngineRecordVisibleTo(conversation.OwnerIdentity, ownerIdentity) {
			copyConversation := conversation
			return &copyConversation, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *memoryEngineRepoStub) SaveConversation(conversation *models.AIConversationArchive) (*models.AIConversationArchive, error) {
	if conversation.ID == uuid.Nil {
		conversation.ID = uuid.New()
	}
	for index, stored := range r.conversations {
		if stored.ID == conversation.ID {
			r.conversations[index] = *conversation
			return conversation, nil
		}
	}
	r.conversations = append(r.conversations, *conversation)
	return conversation, nil
}

func (r *memoryEngineRepoStub) FindConversations(limit int) ([]models.AIConversationArchive, error) {
	return r.FindConversationsForOwner("", limit)
}

func (r *memoryEngineRepoStub) FindConversationsForOwner(ownerIdentity string, limit int) ([]models.AIConversationArchive, error) {
	result := []models.AIConversationArchive{}
	for _, conversation := range r.conversations {
		if !conversation.Archived && memoryEngineRecordVisibleTo(conversation.OwnerIdentity, ownerIdentity) {
			result = append(result, conversation)
		}
	}
	return result, nil
}

func (r *memoryEngineRepoStub) DeleteConversation(id uuid.UUID) error {
	return nil
}

func (r *memoryEngineRepoStub) SaveInsight(insight *models.AIMemoryInsight) (*models.AIMemoryInsight, error) {
	if insight.ID == uuid.Nil {
		insight.ID = uuid.New()
	}
	r.insights = append(r.insights, *insight)
	return insight, nil
}

func (r *memoryEngineRepoStub) FindInsights(kind, projectKey string, needsReview *bool, limit int) ([]models.AIMemoryInsight, error) {
	return r.FindInsightsForOwner("", kind, projectKey, needsReview, limit)
}

func (r *memoryEngineRepoStub) FindInsightsForOwner(ownerIdentity, kind, projectKey string, needsReview *bool, limit int) ([]models.AIMemoryInsight, error) {
	result := []models.AIMemoryInsight{}
	for _, insight := range r.insights {
		if !memoryEngineRecordVisibleTo(insight.OwnerIdentity, ownerIdentity) {
			continue
		}
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

func memoryEngineRecordVisibleTo(recordOwner, ownerIdentity string) bool {
	return ownerIdentity == "" || recordOwner == "" || recordOwner == ownerIdentity
}

func (r *memoryEngineRepoStub) ArchiveInsights(conversationID uuid.UUID, revision int) error {
	return nil
}

func (r *memoryEngineRepoStub) DeleteMemoriesBySourceURI(ownerIdentity, sourceURI string) error {
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

func (s *memoryEngineMemoryStub) CreateForOwner(ownerIdentity string, request memory.CreateRequest) (*models.ContextMemory, error) {
	created, err := s.Create(request)
	if created != nil {
		created.OwnerIdentity = ownerIdentity
		s.memories = append(s.memories, *created)
	}
	return created, err
}

func (s *memoryEngineMemoryStub) Update(id uuid.UUID, request memory.UpdateRequest) (*models.ContextMemory, error) {
	return &models.ContextMemory{ID: id, Content: request.Content, Kind: request.Kind}, nil
}

func (s *memoryEngineMemoryStub) UpdateForOwner(string, id uuid.UUID, request memory.UpdateRequest) (*models.ContextMemory, error) {
	return s.Update(id, request)
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

func (s *memoryEngineMemoryStub) FindAllForOwner(ownerIdentity, projectKey string, includeArchived bool) ([]models.ContextMemory, error) {
	all, _ := s.FindAll(projectKey, includeArchived)
	visible := make([]models.ContextMemory, 0, len(all))
	for _, item := range all {
		if item.OwnerIdentity == "" || item.OwnerIdentity == ownerIdentity {
			visible = append(visible, item)
		}
	}
	return visible, nil
}

func (s *memoryEngineMemoryStub) FindByID(id uuid.UUID) (*models.ContextMemory, error) {
	return nil, gorm.ErrRecordNotFound
}

func (s *memoryEngineMemoryStub) FindByIDForOwner(string, uuid.UUID) (*models.ContextMemory, error) {
	return nil, gorm.ErrRecordNotFound
}

func (s *memoryEngineMemoryStub) Archive(id uuid.UUID, archived bool) (*models.ContextMemory, error) {
	return &models.ContextMemory{ID: id, Archived: archived}, nil
}

func (s *memoryEngineMemoryStub) ArchiveForOwner(string, id uuid.UUID, archived bool) (*models.ContextMemory, error) {
	return s.Archive(id, archived)
}

func (s *memoryEngineMemoryStub) Delete(id uuid.UUID) error {
	return nil
}

func (s *memoryEngineMemoryStub) DeleteForOwner(string, uuid.UUID) error { return nil }

func (s *memoryEngineMemoryStub) Retrieve(request memory.RetrieveRequest) (*memory.RetrieveResult, error) {
	return &memory.RetrieveResult{Query: request.Query, ProjectKey: request.ProjectKey}, nil
}

func (s *memoryEngineMemoryStub) RetrieveForOwner(string, request memory.RetrieveRequest) (*memory.RetrieveResult, error) {
	return s.Retrieve(request)
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

type memoryEnginePursuitGateway struct {
	memoryEnginePursuitLinker
	routed []workflow.IntakeRequest
}

func (s *memoryEnginePursuitGateway) RouteWorkflowIntake(request workflow.IntakeRequest) (*workflow.WorkflowRecord, error) {
	s.routed = append(s.routed, request)
	return &workflow.WorkflowRecord{Item: models.WorkflowItem{ID: uuid.New(), Title: request.Input, ProjectKey: request.ProjectKey}}, nil
}

func (s *memoryEnginePursuitLinker) AutoLinkWorkflow(request pursuitpkg.AutoLinkWorkflowRequest) (*pursuitpkg.AutoLinkResult, error) {
	s.requests = append(s.requests, request)
	return &pursuitpkg.AutoLinkResult{Linked: true, PursuitID: uuid.New(), Score: 0.8, Message: "source-derived workflow linked to pursuit"}, nil
}

func (s *memoryEnginePursuitLinker) AutoLinkMemory(request pursuitpkg.AutoLinkMemoryRequest) (*pursuitpkg.AutoLinkResult, error) {
	s.memoryRequests = append(s.memoryRequests, request)
	return &pursuitpkg.AutoLinkResult{Linked: true, PursuitID: uuid.New(), Score: 0.8, Message: "memory insight linked to pursuit"}, nil
}
