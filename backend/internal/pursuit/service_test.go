package pursuit

import (
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/workflow"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCreateClassifiesAndAuditsPursuit(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)

	created, err := service.Create(CreateRequest{
		Title:          "Vivare legal dispute",
		Description:    "Prepare evidence for lawyer and government-style hearing.",
		ProjectKey:     "vivare",
		DesiredOutcome: "Verified evidence bundle and approved legal reply.",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if created.RiskLevel != "high" || created.AutonomyLevel != "approve_before_execute" {
		t.Fatalf("risk/autonomy = %q/%q", created.RiskLevel, created.AutonomyLevel)
	}
	if created.NeedCategory != "safety_and_stability" {
		t.Fatalf("need category = %q", created.NeedCategory)
	}
	activity, _ := repo.FindActivities(created.ID, 10)
	if len(activity) != 1 || activity[0].EventType != "pursuit.created" {
		t.Fatalf("activity = %#v", activity)
	}
}

func TestCreateCannotDowngradeDetectedHighRiskOrUpgradeAutonomy(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)

	created, err := service.Create(CreateRequest{
		Title:         "Send legal evidence to the government",
		Description:   "Prepare the lawyer reply and attach the insurance evidence.",
		RiskLevel:     "low",
		AutonomyLevel: "autonomous_full_local_only",
		Actor:         "test-operator",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if created.RiskLevel != "high" || created.AutonomyLevel != "approve_before_execute" {
		t.Fatalf("unsafe policy override was accepted: %q/%q", created.RiskLevel, created.AutonomyLevel)
	}
	activity, _ := repo.FindActivities(created.ID, 10)
	foundNormalization := false
	for _, item := range activity {
		if item.EventType == "pursuit.safety_normalized" && item.Actor == "test-operator" {
			foundNormalization = true
		}
	}
	if !foundNormalization {
		t.Fatalf("safety normalization was not audited: %#v", activity)
	}
}

func TestUpdateCannotDowngradeRiskWhenGoalBecomesHighRisk(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Organize documents", RiskLevel: "low", AutonomyLevel: "autonomous_safe"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	description := "Prepare the legal evidence bundle and lawyer reply."
	updated, err := service.Update(created.ID, UpdateRequest{
		Description:   &description,
		RiskLevel:     "low",
		AutonomyLevel: "autonomous_safe",
		Actor:         "test-operator",
	})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if updated.RiskLevel != "high" || updated.AutonomyLevel != "approve_before_execute" {
		t.Fatalf("unsafe update override was accepted: %q/%q", updated.RiskLevel, updated.AutonomyLevel)
	}
	activity, _ := repo.FindActivities(created.ID, 10)
	foundNormalization := false
	for _, item := range activity {
		if item.EventType == "pursuit.safety_normalized" && item.Actor == "test-operator" {
			foundNormalization = true
		}
	}
	if !foundNormalization {
		t.Fatalf("updated safety normalization was not audited: %#v", activity)
	}
}

func TestPursuitRationaleFlowsIntoOperationalContext(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	rationale := "Legal evidence must be ready before the municipality deadline."
	created, err := service.Create(CreateRequest{
		Title:          "Prepare case materials",
		WhyItMatters:   rationale,
		DesiredOutcome: "A source-linked evidence package is ready for review.",
		RiskLevel:      "low",
		AutonomyLevel:  "autonomous_safe",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if created.WhyItMatters != rationale {
		t.Fatalf("why it matters = %q, want %q", created.WhyItMatters, rationale)
	}
	if created.RiskLevel != "high" || created.AutonomyLevel != "approve_before_execute" {
		t.Fatalf("rationale did not raise the safety floor: %q/%q", created.RiskLevel, created.AutonomyLevel)
	}
	if created.Domain != "stability" {
		t.Fatalf("rationale did not classify the pursuit domain: %q", created.Domain)
	}

	matches, err := service.Match(MatchRequest{Input: "municipality legal evidence"})
	if err != nil {
		t.Fatalf("Match returned error: %v", err)
	}
	if len(matches) != 1 || matches[0].Pursuit.ID != created.ID {
		t.Fatalf("rationale was not included in matching: %#v", matches)
	}
	if !strings.Contains(pursuitPlanInput(*created), "Why it matters: "+rationale) {
		t.Fatalf("planner input did not include rationale: %q", pursuitPlanInput(*created))
	}

	updatedRationale := "A complete evidence trail protects the case and prevents an unsupported response."
	updated, err := service.Update(created.ID, UpdateRequest{WhyItMatters: &updatedRationale, Actor: "test-operator"})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if updated.WhyItMatters != updatedRationale {
		t.Fatalf("updated rationale = %q, want %q", updated.WhyItMatters, updatedRationale)
	}

	brief, err := service.DelegationPackage(created.ID)
	if err != nil {
		t.Fatalf("DelegationPackage returned error: %v", err)
	}
	if brief.WhyItMatters != updatedRationale {
		t.Fatalf("delegation rationale = %q, want %q", brief.WhyItMatters, updatedRationale)
	}
}

func TestMatchUsesProjectAndExistingSourceLink(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "ASR insurance claim", ProjectKey: "asr", Description: "Insurance documents and claim follow-up"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := service.Link(created.ID, LinkRequest{LinkType: LinkSourceItem, LinkID: "email-1", Relationship: "evidence"}); err != nil {
		t.Fatalf("Link returned error: %v", err)
	}

	sourceMatches, err := service.Match(MatchRequest{SourceType: LinkSourceItem, SourceID: "email-1"})
	if err != nil {
		t.Fatalf("Match returned error: %v", err)
	}
	if len(sourceMatches) != 1 || sourceMatches[0].Score < 0.9 {
		t.Fatalf("source match = %#v", sourceMatches)
	}
	if _, err := service.Link(created.ID, LinkRequest{
		LinkType:     LinkWorkflow,
		LinkID:       uuid.New().String(),
		Relationship: "operational_work",
		SourceURI:    "local://email/asr-claim-12",
	}); err != nil {
		t.Fatalf("Link source URI returned error: %v", err)
	}
	uriMatches, err := service.Match(MatchRequest{SourceURI: "local://email/asr-claim-12"})
	if err != nil {
		t.Fatalf("Match source URI returned error: %v", err)
	}
	if len(uriMatches) != 1 || uriMatches[0].Pursuit.ID != created.ID || uriMatches[0].Score < 0.9 {
		t.Fatalf("source URI match = %#v", uriMatches)
	}

	projectMatches, err := service.Match(MatchRequest{Input: "follow up insurance claim documents", ProjectKey: "asr"})
	if err != nil {
		t.Fatalf("Match returned error: %v", err)
	}
	if len(projectMatches) == 0 || projectMatches[0].Pursuit.ID != created.ID {
		t.Fatalf("project match = %#v", projectMatches)
	}
}

func TestMatchUsesVisibleExactLinkWhenAnotherOwnerHasTheSameSource(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	alice, err := service.Create(CreateRequest{Title: "Alice legal evidence", OwnerIdentity: "alice", ProjectKey: "alice-case"})
	if err != nil {
		t.Fatalf("create alice pursuit: %v", err)
	}
	bob, err := service.Create(CreateRequest{Title: "Bob legal evidence", OwnerIdentity: "bob", ProjectKey: "bob-case"})
	if err != nil {
		t.Fatalf("create bob pursuit: %v", err)
	}
	const sourceID = "shared-message-41"
	const sourceURI = "local://mail/shared-message-41"
	if _, err := service.Link(bob.ID, LinkRequest{
		LinkType: LinkSourceItem, LinkID: sourceID, Relationship: "evidence", SourceURI: sourceURI, Confidence: 0.99,
	}); err != nil {
		t.Fatalf("link bob source: %v", err)
	}
	if _, err := service.Link(alice.ID, LinkRequest{
		LinkType: LinkSourceItem, LinkID: sourceID, Relationship: "evidence", SourceURI: sourceURI, Confidence: 0.10,
	}); err != nil {
		t.Fatalf("link alice source: %v", err)
	}

	sourceMatches, err := service.Match(MatchRequest{OwnerIdentity: " alice ", SourceType: LinkSourceItem, SourceID: sourceID})
	if err != nil {
		t.Fatalf("match by source id: %v", err)
	}
	if len(sourceMatches) != 1 || sourceMatches[0].Pursuit.ID != alice.ID {
		t.Fatalf("alice source matches = %#v, want alice pursuit", sourceMatches)
	}

	uriMatches, err := service.Match(MatchRequest{OwnerIdentity: "alice", SourceURI: sourceURI})
	if err != nil {
		t.Fatalf("match by source URI: %v", err)
	}
	if len(uriMatches) != 1 || uriMatches[0].Pursuit.ID != alice.ID {
		t.Fatalf("alice URI matches = %#v, want alice pursuit", uriMatches)
	}

	outsiderMatches, err := service.Match(MatchRequest{OwnerIdentity: "charlie", SourceType: LinkSourceItem, SourceID: sourceID})
	if err != nil {
		t.Fatalf("match for unrelated owner: %v", err)
	}
	if len(outsiderMatches) != 0 {
		t.Fatalf("unrelated owner received exact source match: %#v", outsiderMatches)
	}
}

func TestAutoLinkWorkflowLinksOperationalWorkAndSourceProvenance(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "ASR insurance claim", ProjectKey: "asr", Description: "Insurance documents and claim follow-up"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	workflowID := uuid.New()
	rawItemID := uuid.New()
	extractionID := uuid.New()
	repo.sourceItems[rawItemID] = models.SourceRawItem{
		ID:         rawItemID,
		Title:      "ASR claim email",
		ItemType:   "email",
		ProjectKey: "asr",
		SourceURI:  "local://email/asr-claim",
	}
	repo.extractions[extractionID] = models.SourceExtraction{
		ID:          extractionID,
		RawItemID:   rawItemID,
		ProjectKey:  "asr",
		Summary:     "Follow up on ASR claim evidence.",
		SourceURI:   "local://email/asr-claim",
		SourceLabel: "ASR claim email",
	}

	result, err := service.AutoLinkWorkflow(AutoLinkWorkflowRequest{
		WorkflowID:   workflowID,
		Input:        "follow up insurance claim documents",
		ProjectKey:   "asr",
		SourceURI:    "local://email/asr-claim",
		SourceLabel:  "ASR claim email",
		ExtractionID: extractionID.String(),
		RawItemID:    rawItemID.String(),
		Actor:        "source-worker",
	})
	if err != nil {
		t.Fatalf("AutoLinkWorkflow returned error: %v", err)
	}
	if !result.Linked || result.PursuitID != created.ID || result.Score < defaultAutoLinkMinimumScore {
		t.Fatalf("auto-link result = %#v", result)
	}
	links, _ := repo.FindLinks(created.ID)
	found := map[string]bool{}
	for _, link := range links {
		found[link.LinkType] = true
	}
	for _, linkType := range []string{LinkWorkflow, LinkSourceItem, LinkSourceExtraction} {
		if !found[linkType] {
			t.Fatalf("missing %s link in %#v", linkType, links)
		}
	}
	detail, err := service.Detail(created.ID)
	if err != nil {
		t.Fatalf("Detail returned error: %v", err)
	}
	if len(detail.Workflows) != 1 || len(detail.SourceItems) != 1 || len(detail.SourceExtractions) != 1 {
		t.Fatalf("detail workflow/source counts = %d/%d/%d", len(detail.Workflows), len(detail.SourceItems), len(detail.SourceExtractions))
	}
}

func TestAutoLinkWorkflowSkipsWeakPursuitMatch(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "ASR insurance claim", ProjectKey: "asr"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	result, err := service.AutoLinkWorkflow(AutoLinkWorkflowRequest{
		WorkflowID: uuid.New(),
		Input:      "garden appointment and unrelated shopping reminder",
		ProjectKey: "garden-admin",
	})
	if err != nil {
		t.Fatalf("AutoLinkWorkflow returned error: %v", err)
	}
	if result.Linked {
		t.Fatalf("weak match should not link: %#v", result)
	}
	links, _ := repo.FindLinks(created.ID)
	if len(links) != 0 {
		t.Fatalf("links = %#v, want none", links)
	}
}

func TestAutoLinkWorkflowDoesNotAttachOperationalWorkToClosedPursuit(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Completed ASR insurance claim", ProjectKey: "asr"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	closed := repo.pursuits[created.ID]
	closed.Status = StatusCompleted
	closed.CompletionState = CompletionVerified
	repo.pursuits[created.ID] = closed

	result, err := service.AutoLinkWorkflow(AutoLinkWorkflowRequest{
		WorkflowID: uuid.New(),
		Input:      "New ASR claim follow-up arrived after completion.",
		ProjectKey: "asr",
	})
	if err != nil {
		t.Fatalf("AutoLinkWorkflow returned error: %v", err)
	}
	if result.Linked || !strings.Contains(result.Message, "closed") {
		t.Fatalf("closed pursuit must reject operational auto-linking: %#v", result)
	}
	links, _ := repo.FindLinks(created.ID)
	if len(links) != 0 {
		t.Fatalf("closed pursuit links = %#v, want none", links)
	}
}

func TestRouteIntakeCreatesCandidateInsteadOfReopeningClosedPursuit(t *testing.T) {
	repo := newFakeRepo()
	workflowService := &fakeWorkflowIntake{repo: repo}
	service := NewService(repo, workflowService)
	closed, err := service.Create(CreateRequest{Title: "Completed ASR insurance claim", ProjectKey: "asr"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	closedRecord := repo.pursuits[closed.ID]
	closedRecord.Status = StatusCompleted
	closedRecord.CompletionState = CompletionVerified
	repo.pursuits[closed.ID] = closedRecord

	result, err := service.RouteIntake(IntakeRequest{
		Input:       "A new ASR claim follow-up arrived after the earlier claim was completed.",
		ProjectKey:  "asr",
		SourceType:  "email",
		SourceID:    "new-asr-follow-up",
		SourceURI:   "email://asr/new-follow-up",
		SourceLabel: "New ASR follow-up",
	})
	if err != nil {
		t.Fatalf("RouteIntake returned error: %v", err)
	}
	if !result.CreatedCandidate || result.PursuitID == uuid.Nil || result.PursuitID == closed.ID {
		t.Fatalf("closed pursuit should produce a new candidate: %#v", result)
	}
	if workflowService.calls != 1 {
		t.Fatalf("workflow intake calls = %d, want one governed candidate workflow", workflowService.calls)
	}
	persistedClosed, err := repo.FindByID(closed.ID)
	if err != nil {
		t.Fatalf("FindByID(closed): %v", err)
	}
	if persistedClosed.Status != StatusCompleted || persistedClosed.CompletionState != CompletionVerified {
		t.Fatalf("closed pursuit was reactivated: %#v", persistedClosed)
	}
	links, _ := repo.FindLinks(closed.ID)
	if len(links) != 0 {
		t.Fatalf("closed pursuit received new operational links: %#v", links)
	}
}

func TestAutoLinkWorkflowCreatesReviewableCandidateWhenNoMatchExists(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	workflowID := uuid.New()
	rawItemID := uuid.New()
	extractionID := uuid.New()

	result, err := service.AutoLinkWorkflow(AutoLinkWorkflowRequest{
		WorkflowID:           workflowID,
		Input:                "Prepare YouTube appeal evidence and schedule follow-up with platform support.",
		ProjectKey:           "youtube-removal",
		SourceType:           "ai_chat",
		SourceID:             "chat-1:insight-1",
		SourceURI:            "chat://youtube-removal/1",
		SourceLabel:          "YouTube appeal planning chat",
		RawItemID:            rawItemID.String(),
		ExtractionID:         extractionID.String(),
		Actor:                "memory-engine",
		AllowCreateCandidate: true,
	})
	if err != nil {
		t.Fatalf("AutoLinkWorkflow returned error: %v", err)
	}
	if !result.Linked || !result.Created || result.PursuitID == uuid.Nil {
		t.Fatalf("candidate result = %#v", result)
	}

	detail, err := service.Detail(result.PursuitID)
	if err != nil {
		t.Fatalf("Detail returned error: %v", err)
	}
	if detail.Pursuit.Status != StatusActive || detail.Pursuit.AutonomyLevel != "suggest" {
		t.Fatalf("candidate pursuit state = %s/%s", detail.Pursuit.Status, detail.Pursuit.AutonomyLevel)
	}
	if detail.Pursuit.SourceOfCreation != "ai_chat_pursuit_candidate" {
		t.Fatalf("source of creation = %q", detail.Pursuit.SourceOfCreation)
	}
	links, _ := repo.FindLinks(result.PursuitID)
	found := map[string]bool{}
	for _, link := range links {
		found[link.LinkType+":"+link.Relationship] = true
	}
	for _, expected := range []string{
		LinkWorkflow + ":candidate_operational_work",
		LinkSourceItem + ":candidate_source_record",
		LinkSourceExtraction + ":candidate_source_extraction",
	} {
		if !found[expected] {
			t.Fatalf("candidate missing link %s in %#v", expected, links)
		}
	}
}

func TestAutoCreatedCandidateRequiresRobertDecisionAndCanBeAccepted(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	workflowID := uuid.New()

	result, err := service.AutoLinkWorkflow(AutoLinkWorkflowRequest{
		WorkflowID:           workflowID,
		Input:                "Imported AI agent thread found OpenClaw setup work that may belong in HAI.",
		ProjectKey:           "hai",
		SourceType:           "openclaw",
		SourceURI:            "openclaw://import/thread-1",
		SourceLabel:          "OpenClaw integration import",
		Actor:                "agent-runtime",
		AllowCreateCandidate: true,
	})
	if err != nil {
		t.Fatalf("AutoLinkWorkflow returned error: %v", err)
	}
	if !result.Created {
		t.Fatalf("expected auto-created pursuit candidate, got %#v", result)
	}

	dashboard, err := service.Dashboard()
	if err != nil {
		t.Fatalf("Dashboard returned error: %v", err)
	}
	if len(dashboard.NeedsRobert) == 0 || dashboard.NeedsRobert[0].Pursuit.ID != result.PursuitID {
		t.Fatalf("candidate was not surfaced in Robert queue: %#v", dashboard.NeedsRobert)
	}

	detail, err := service.Detail(result.PursuitID)
	if err != nil {
		t.Fatalf("Detail returned error: %v", err)
	}
	if detail.Summary.NeedsRobert == 0 {
		t.Fatalf("candidate summary did not require Robert: %#v", detail.Summary)
	}
	if len(detail.ActionQueues.NeedsRobert) == 0 {
		t.Fatalf("candidate action was not Robert-owned: %#v", detail.ActionQueues)
	}
	candidateDecision := false
	for _, decision := range detail.DecisionQueue {
		if decision.DecisionType == "pursuit_candidate_review" {
			candidateDecision = true
			if !decision.RequiresApproval || decision.YesLabel != "Accept and plan" || decision.NoLabel != "Archive" {
				t.Fatalf("candidate decision labels/gate invalid: %#v", decision)
			}
		}
	}
	if !candidateDecision {
		t.Fatalf("candidate review decision missing: %#v", detail.DecisionQueue)
	}

	accepted, err := service.Plan(result.PursuitID, PlanRequest{Actor: "Robert"})
	if err != nil {
		t.Fatalf("Plan accepted candidate returned error: %v", err)
	}
	if accepted.Pursuit.SourceOfCreation != "openclaw_pursuit_intake" {
		t.Fatalf("source marker was not cleared after acceptance: %q", accepted.Pursuit.SourceOfCreation)
	}
	for _, decision := range accepted.DecisionQueue {
		if decision.DecisionType == "pursuit_candidate_review" {
			t.Fatalf("accepted candidate still has pending candidate review decision: %#v", accepted.DecisionQueue)
		}
	}
}

func TestAutoLinkMemoryLinksStableContextToPursuit(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Vivare legal dispute", ProjectKey: "vivare", Description: "Housing and legal evidence pursuit"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	memoryID := uuid.New()
	repo.memories[memoryID] = models.ContextMemory{
		ID:          memoryID,
		ProjectKey:  "vivare",
		Kind:        "rule",
		Content:     "Always use formal Dutch legal tone for Vivare correspondence.",
		Summary:     "Formal Dutch legal tone for Vivare.",
		SourceURI:   "https://chatgpt.com/c/vivare",
		SourceLabel: "chatgpt: Vivare legal dispute",
	}

	result, err := service.AutoLinkMemory(AutoLinkMemoryRequest{
		MemoryID:    memoryID,
		Input:       "Vivare legal dispute rule: Always use formal Dutch legal tone for Vivare correspondence.",
		ProjectKey:  "vivare",
		SourceURI:   "https://chatgpt.com/c/vivare",
		SourceLabel: "chatgpt: Vivare legal dispute",
		Actor:       "memory-engine",
	})
	if err != nil {
		t.Fatalf("AutoLinkMemory returned error: %v", err)
	}
	if !result.Linked || result.PursuitID != created.ID || result.Score < defaultAutoLinkMinimumScore {
		t.Fatalf("auto-link memory result = %#v", result)
	}
	detail, err := service.Detail(created.ID)
	if err != nil {
		t.Fatalf("Detail returned error: %v", err)
	}
	if len(detail.Memories) != 1 || detail.Memories[0].ID != memoryID {
		t.Fatalf("detail memories = %#v, want linked memory %s", detail.Memories, memoryID)
	}
}

func TestIntakeCreatesWorkflowAndLinksOperationalWork(t *testing.T) {
	repo := newFakeRepo()
	workflowService := &fakeWorkflowIntake{repo: repo}
	service := NewService(repo, workflowService)
	created, err := service.Create(CreateRequest{Title: "Government letter response", OwnerIdentity: "alice", ProjectKey: "letter", Description: "Legal/government reply"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	detail, err := service.Intake(created.ID, IntakeRequest{
		OwnerIdentity: "bob",
		Input:         "New government letter asks for a reply before Friday.",
		SourceType:    "email",
		SourceID:      "email-2",
		SourceURI:     "local://email-2",
		SourceLabel:   "Government letter",
	})
	if err != nil {
		t.Fatalf("Intake returned error: %v", err)
	}
	if workflowService.received.ProjectKey != "letter" {
		t.Fatalf("workflow project key = %q", workflowService.received.ProjectKey)
	}
	if workflowService.received.OwnerIdentity != "alice" {
		t.Fatalf("workflow owner = %q, want persisted pursuit owner alice", workflowService.received.OwnerIdentity)
	}
	if len(detail.Workflows) != 1 || len(detail.ApprovalItems) != 1 {
		t.Fatalf("detail workflows/approvals = %#v / %#v", detail.Workflows, detail.ApprovalItems)
	}
	if len(detail.NextActions) == 0 || detail.NextActions[0].Owner != "Robert" {
		t.Fatalf("next actions = %#v", detail.NextActions)
	}
	links, _ := repo.FindLinks(created.ID)
	foundWorkflowLink := false
	for _, link := range links {
		if link.LinkType == LinkWorkflow && link.Relationship == "operational_work" {
			foundWorkflowLink = true
		}
	}
	if !foundWorkflowLink {
		t.Fatalf("workflow link missing: %#v", links)
	}
}

func TestIntakeLinksSourceReferenceIntoPursuitEvidence(t *testing.T) {
	repo := newFakeRepo()
	workflowService := &fakeWorkflowIntake{repo: repo}
	service := NewService(repo, workflowService)
	created, err := service.Create(CreateRequest{Title: "ASR claim evidence", ProjectKey: "asr"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	rawID := uuid.New()
	extractionID := uuid.New()
	repo.sourceItems[rawID] = models.SourceRawItem{
		ID:         rawID,
		SourceID:   uuid.New(),
		ExternalID: "email-claim-1",
		ProjectKey: "asr",
		ItemType:   "email",
		Title:      "ASR requested receipt evidence",
		SourceURI:  "local://source/email-claim-1",
	}
	repo.extractions[extractionID] = models.SourceExtraction{
		ID:          extractionID,
		RawItemID:   rawID,
		ProjectKey:  "asr",
		Summary:     "ASR requested receipt evidence before Friday.",
		SourceURI:   "local://source/email-claim-1#extraction",
		SourceLabel: "ASR evidence request",
	}

	detail, err := service.Intake(created.ID, IntakeRequest{
		Input:       "Create a workflow to collect the receipts requested by ASR.",
		ProjectKey:  "asr",
		SourceType:  LinkSourceExtraction,
		SourceID:    extractionID.String(),
		SourceURI:   "local://source/email-claim-1#extraction",
		SourceLabel: "ASR evidence request",
	})
	if err != nil {
		t.Fatalf("Intake returned error: %v", err)
	}
	if len(detail.Workflows) != 1 {
		t.Fatalf("workflows = %d, want 1", len(detail.Workflows))
	}
	if len(detail.SourceExtractions) != 1 || detail.SourceExtractions[0].ID != extractionID {
		t.Fatalf("source extractions = %#v, want linked extraction %s", detail.SourceExtractions, extractionID)
	}
	links, _ := repo.FindLinks(created.ID)
	foundWorkflow := false
	foundExtraction := false
	for _, link := range links {
		if link.LinkType == LinkWorkflow && link.SourceURI == "local://source/email-claim-1#extraction" {
			foundWorkflow = true
		}
		if link.LinkType == LinkSourceExtraction && link.LinkID == extractionID.String() && link.Relationship == "source_extraction" {
			foundExtraction = true
		}
	}
	if !foundWorkflow || !foundExtraction {
		t.Fatalf("links did not preserve workflow and extraction provenance: %#v", links)
	}
}

func TestIntakePreservesDecisionEvidenceOnWorkflowLink(t *testing.T) {
	repo := newFakeRepo()
	workflowService := &fakeWorkflowIntake{repo: repo}
	service := NewService(repo, workflowService)
	created, err := service.Create(CreateRequest{Title: "Recover failed runtime", ProjectKey: "hai"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	launchID := uuid.New()
	sourceURI := "automation-launch://" + launchID.String()
	startedAt := time.Now().UTC()
	repo.launchEvents = append(repo.launchEvents, models.AutomationLaunchEvent{
		ID:          launchID,
		RuntimeType: "openclaw",
		LaunchType:  "agent_runtime",
		Status:      "blocked",
		Message:     "agent runtime registry is not configured",
		AuditEvents: []string{
			"launch requested",
			"agent runtime registry unavailable",
		},
		StartedAt:   startedAt,
		CompletedAt: startedAt.Add(time.Second),
	})

	detail, err := service.Intake(created.ID, IntakeRequest{
		Input:          "Create a governed recovery workflow before retrying this runtime attempt.",
		ProjectKey:     "hai",
		SourceType:     "pursuit_decision",
		SourceID:       "runtime:" + launchID.String() + ":review",
		SourceURI:      sourceURI,
		SourceLabel:    "openclaw",
		ContentType:    "runtime_attempt_review",
		Trigger:        "pursuit_decision_approved",
		Actor:          "Robert",
		RequiresReview: true,
		ReviewReason:   "blocked: agent runtime registry is not configured",
	})
	if err != nil {
		t.Fatalf("Intake returned error: %v", err)
	}
	if workflowService.received.SourceURI != sourceURI || workflowService.received.SourceLabel != "openclaw" {
		t.Fatalf("workflow provenance = %q/%q", workflowService.received.SourceURI, workflowService.received.SourceLabel)
	}
	if workflowService.received.ContentType != "runtime_attempt_review" || workflowService.received.Trigger != "pursuit_decision_approved" {
		t.Fatalf("workflow decision metadata = %q/%q", workflowService.received.ContentType, workflowService.received.Trigger)
	}
	links, _ := repo.FindLinks(created.ID)
	found := false
	foundRuntimeAttempt := false
	for _, link := range links {
		if link.LinkType == LinkWorkflow && link.SourceURI == sourceURI && link.SourceLabel == "openclaw" {
			found = true
		}
		if link.LinkType == LinkAgentRuntime && link.LinkID == launchID.String() && link.Relationship == "execution_attempt" {
			foundRuntimeAttempt = true
		}
	}
	if !found {
		t.Fatalf("workflow link did not retain decision evidence: %#v", links)
	}
	if !foundRuntimeAttempt {
		t.Fatalf("runtime launch evidence link missing after approved intake: %#v", links)
	}
	if len(detail.Workflows) != 1 {
		t.Fatalf("detail workflows = %#v", detail.Workflows)
	}
	if len(detail.RuntimeAttempts) != 1 || detail.RuntimeAttempts[0].ID != launchID {
		t.Fatalf("detail runtime attempts = %#v, want approved intake to retain runtime evidence", detail.RuntimeAttempts)
	}
	if !timelineContains(detail.Timeline, "runtime_audit", "agent runtime registry unavailable") {
		t.Fatalf("timeline = %#v, want linked runtime audit retained after recovery intake", detail.Timeline)
	}
	foundResolution := false
	for _, activity := range detail.Activity {
		if activity.EventType == "pursuit.decision_resolved" && activity.SourceID == "runtime:"+launchID.String()+":review" {
			foundResolution = true
		}
	}
	if !foundResolution {
		t.Fatalf("approved decision resolution activity missing: %#v", detail.Activity)
	}
}

func TestRouteIntakeMatchesExistingPursuitAndCreatesGovernedWorkflow(t *testing.T) {
	repo := newFakeRepo()
	workflowService := &fakeWorkflowIntake{repo: repo}
	service := NewService(repo, workflowService)
	created, err := service.Create(CreateRequest{
		Title:         "Vivare legal dispute",
		OwnerIdentity: "alice",
		ProjectKey:    "vivare",
		Description:   "Housing/legal evidence and formal replies.",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	result, err := service.RouteIntake(IntakeRequest{
		Input:         "New Vivare lawyer email asks for evidence before the hearing.",
		ProjectKey:    "vivare",
		SourceType:    "email",
		SourceID:      "email-77",
		SourceURI:     "local://email/vivare-77",
		SourceLabel:   "Vivare lawyer email",
		Actor:         "source-worker",
		OwnerIdentity: "alice",
	})
	if err != nil {
		t.Fatalf("RouteIntake returned error: %v", err)
	}
	if !result.Matched || result.CreatedCandidate || result.PursuitID != created.ID || result.Detail == nil {
		t.Fatalf("route result = %#v, want existing pursuit detail", result)
	}
	if result.Mode != "matched_existing" || result.Score < defaultAutoLinkMinimumScore {
		t.Fatalf("route mode/score = %s/%.2f", result.Mode, result.Score)
	}
	if workflowService.received.ProjectKey != "vivare" || workflowService.received.SourceURI != "local://email/vivare-77" {
		t.Fatalf("workflow intake provenance = %#v", workflowService.received)
	}
	if workflowService.received.OwnerIdentity != "alice" {
		t.Fatalf("matched workflow owner = %q, want persisted pursuit owner alice", workflowService.received.OwnerIdentity)
	}
	links, _ := repo.FindLinks(created.ID)
	found := false
	for _, link := range links {
		if link.LinkType == LinkWorkflow && link.Relationship == "operational_work" && link.SourceURI == "local://email/vivare-77" {
			found = true
		}
	}
	if !found {
		t.Fatalf("routed workflow link missing provenance: %#v", links)
	}
}

func TestDetailForOwnerHidesLegacyCrossOwnerWorkflowLink(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	alicePursuit, err := service.Create(CreateRequest{
		Title:         "Alice private pursuit",
		OwnerIdentity: "alice",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	bobWorkflowID := uuid.New()
	repo.workflows[bobWorkflowID] = models.WorkflowItem{
		ID:               bobWorkflowID,
		OwnerIdentity:    "bob",
		Title:            "Bob private approval workflow",
		CurrentState:     workflow.StateNeedsApproval,
		RequiresApproval: true,
		ApprovalStatus:   "pending",
	}
	foreignURI := "workflow://private/bob-approval"
	foreignLinkID := uuid.New()
	repo.links[foreignLinkID] = models.PursuitLink{
		ID:           foreignLinkID,
		PursuitID:    alicePursuit.ID,
		LinkType:     LinkWorkflow,
		LinkID:       bobWorkflowID.String(),
		Relationship: "legacy_import",
		SourceURI:    foreignURI,
		SourceLabel:  "Bob private workflow",
		CreatedAt:    time.Now().UTC(),
	}

	detail, err := service.DetailForOwner("alice", alicePursuit.ID)
	if err != nil {
		t.Fatalf("DetailForOwner returned error: %v", err)
	}
	if len(detail.Links) != 0 || len(detail.Workflows) != 0 || len(detail.ApprovalItems) != 0 {
		t.Fatalf("owner detail exposed legacy foreign workflow: %#v", detail)
	}
	if _, err := service.ResolveEvidenceForOwner("alice", alicePursuit.ID, foreignURI); err == nil {
		t.Fatalf("ResolveEvidenceForOwner resolved evidence from a hidden legacy link")
	}
	overview, err := service.ApprovalsForOwner("alice", alicePursuit.ID)
	if err != nil {
		t.Fatalf("ApprovalsForOwner returned error: %v", err)
	}
	if len(overview.ApprovalItems) != 0 || overview.Counts["approvalItems"] != 0 {
		t.Fatalf("owner approval overview exposed legacy foreign workflow: %#v", overview)
	}
}

func TestAutoLinkWorkflowRejectsForeignOwnerWorkflow(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	if _, err := service.Create(CreateRequest{
		Title:         "Prepare Vivare evidence bundle",
		Description:   "Collect evidence for the Vivare hearing.",
		OwnerIdentity: "alice",
		ProjectKey:    "vivare",
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	bobWorkflowID := uuid.New()
	repo.workflows[bobWorkflowID] = models.WorkflowItem{
		ID:            bobWorkflowID,
		OwnerIdentity: "bob",
		Title:         "Bob private evidence workflow",
		ProjectKey:    "vivare",
		CurrentState:  workflow.StateReady,
	}

	_, err := service.AutoLinkWorkflow(AutoLinkWorkflowRequest{
		OwnerIdentity: "alice",
		WorkflowID:    bobWorkflowID,
		Input:         "Prepare Vivare evidence bundle for the hearing.",
		ProjectKey:    "vivare",
		SourceURI:     "local://alice/vivare-intake",
		SourceLabel:   "Alice evidence intake",
	})
	if err == nil {
		t.Fatalf("AutoLinkWorkflow accepted a workflow owned by another user")
	}
}

func TestRouteIntakeCreatesReviewableCandidateWhenNoPursuitMatches(t *testing.T) {
	repo := newFakeRepo()
	workflowService := &fakeWorkflowIntake{repo: repo}
	service := NewService(repo, workflowService)

	result, err := service.RouteIntake(IntakeRequest{
		OwnerIdentity: "alice",
		Input:         "Prepare YouTube removal appeal evidence and draft platform support follow-up.",
		ProjectKey:    "youtube-removal",
		SourceType:    "ai_chat",
		SourceID:      "chat-99:action-1",
		SourceURI:     "chat://youtube-removal/99",
		SourceLabel:   "YouTube appeal planning chat",
		Actor:         "memory-engine",
	})
	if err != nil {
		t.Fatalf("RouteIntake returned error: %v", err)
	}
	if !result.CreatedCandidate || result.PursuitID == uuid.Nil || result.Detail == nil {
		t.Fatalf("route result = %#v, want created candidate detail", result)
	}
	if result.Mode != "candidate_created" {
		t.Fatalf("route mode = %q", result.Mode)
	}
	if result.Detail.Pursuit.SourceOfCreation != "ai_chat_pursuit_candidate" {
		t.Fatalf("source marker = %q", result.Detail.Pursuit.SourceOfCreation)
	}
	if len(result.Detail.DecisionQueue) == 0 || result.Detail.Summary.NeedsRobert == 0 {
		t.Fatalf("candidate did not surface Robert review: decisions=%#v summary=%#v", result.Detail.DecisionQueue, result.Detail.Summary)
	}
	if workflowService.received.Trigger != "pursuit_global_intake" {
		t.Fatalf("workflow trigger = %q", workflowService.received.Trigger)
	}
	if workflowService.received.OwnerIdentity != "alice" {
		t.Fatalf("candidate workflow owner = %q, want alice", workflowService.received.OwnerIdentity)
	}
}

func TestRouteWorkflowIntakeReturnsPursuitLinkedWorkflowRecord(t *testing.T) {
	repo := newFakeRepo()
	workflowService := &fakeWorkflowIntake{repo: repo}
	service := NewService(repo, workflowService)
	request := workflow.IntakeRequest{
		Input:       "Prepare the evidence bundle for the government hearing.",
		ProjectKey:  "vivare",
		SourceType:  "workflow_api",
		SourceID:    "workflow-api-vivare-01",
		SourceURI:   "workflow-api://intake/workflow-api-vivare-01",
		SourceLabel: "Direct workflow API intake",
		Trigger:     "workflow_api_intake",
		Actor:       "verified-operator",
	}

	record, err := service.RouteWorkflowIntake(request)
	if err != nil {
		t.Fatalf("RouteWorkflowIntake returned error: %v", err)
	}
	if record.Item.ID == uuid.Nil || record.Item.SourceID != request.SourceID || record.Item.SourceURI != request.SourceURI {
		t.Fatalf("returned workflow record did not preserve routed provenance: %#v", record.Item)
	}
	if workflowService.calls != 1 || workflowService.received.Trigger != request.Trigger || !workflowService.received.RequiresReview {
		t.Fatalf("workflow intake was not governed by the pursuit route: %#v", workflowService)
	}
	matches, err := service.Match(MatchRequest{SourceType: request.SourceType, SourceID: request.SourceID})
	if err != nil {
		t.Fatalf("Match returned error: %v", err)
	}
	if len(matches) != 1 || matches[0].Pursuit.ID == uuid.Nil {
		t.Fatalf("routed workflow did not create a traceable pursuit match: %#v", matches)
	}
}

func TestRouteIntakeReusesPursuitLinkedByAssistantCommandIdentity(t *testing.T) {
	repo := newFakeRepo()
	workflowService := &fakeWorkflowIntake{repo: repo}
	service := NewService(repo, workflowService)
	request := IntakeRequest{
		Input:       "Prepare a local runtime recovery workflow safely.",
		ProjectKey:  "018-HAI",
		SourceType:  LinkAssistantCommand,
		SourceID:    "assistant-7bc9d40f",
		SourceURI:   "assistant://command/assistant-7bc9d40f",
		SourceLabel: "HAI chat command",
		Actor:       "operator",
	}

	first, err := service.RouteIntake(request)
	if err != nil {
		t.Fatalf("first RouteIntake returned error: %v", err)
	}
	if !first.CreatedCandidate || first.PursuitID == uuid.Nil {
		t.Fatalf("first route result = %#v, want candidate", first)
	}

	second, err := service.RouteIntake(request)
	if err != nil {
		t.Fatalf("second RouteIntake returned error: %v", err)
	}
	if !second.Matched || second.CreatedCandidate || second.PursuitID != first.PursuitID {
		t.Fatalf("second route result = %#v, want exact existing pursuit match", second)
	}
	links, err := repo.FindLinks(first.PursuitID)
	if err != nil {
		t.Fatalf("FindLinks returned error: %v", err)
	}
	if !pursuitLinkExists(links, LinkAssistantCommand, request.SourceID, "command_origin") {
		t.Fatalf("assistant command source identity was not persisted: %#v", links)
	}
}

func pursuitLinkExists(links []models.PursuitLink, linkType, linkID, relationship string) bool {
	for _, link := range links {
		if link.LinkType == linkType && link.LinkID == linkID && link.Relationship == relationship {
			return true
		}
	}
	return false
}

func TestPlanCreatesFirstWorkflowFromPursuitContext(t *testing.T) {
	repo := newFakeRepo()
	workflowService := &fakeWorkflowIntake{repo: repo}
	service := NewService(repo, workflowService)
	created, err := service.Create(CreateRequest{
		Title:                "Insurance claim evidence bundle",
		OwnerIdentity:        "alice",
		ProjectKey:           "asr",
		Description:          "Collect evidence for insurer before any external response.",
		DesiredOutcome:       "Verified evidence bundle and approved reply draft.",
		CompletionDefinition: "Workflow exists, checklist is clear, and risky external sending remains approval-gated.",
		RiskLevel:            "high",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	detail, err := service.Plan(created.ID, PlanRequest{Actor: "robert"})
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	if workflowService.received.SourceType != LinkPursuit || workflowService.received.SourceID != created.ID.String() {
		t.Fatalf("workflow source = %s/%s, want pursuit/%s", workflowService.received.SourceType, workflowService.received.SourceID, created.ID)
	}
	if workflowService.received.Trigger != "pursuit_planning" || !workflowService.received.RequiresReview {
		t.Fatalf("workflow trigger/review = %s/%v", workflowService.received.Trigger, workflowService.received.RequiresReview)
	}
	if workflowService.received.OwnerIdentity != "alice" {
		t.Fatalf("planned workflow owner = %q, want persisted pursuit owner alice", workflowService.received.OwnerIdentity)
	}
	if !strings.Contains(workflowService.received.Input, "Verified evidence bundle") || !strings.Contains(workflowService.received.Input, "approval-gated") {
		t.Fatalf("workflow input missing pursuit context: %s", workflowService.received.Input)
	}
	if len(detail.Workflows) != 1 || detail.Summary.PlanningNeeded {
		t.Fatalf("detail workflows/planning = %d/%v", len(detail.Workflows), detail.Summary.PlanningNeeded)
	}
	links, _ := repo.FindLinks(created.ID)
	foundWorkflowLink := false
	for _, link := range links {
		if link.LinkType == LinkWorkflow && link.Relationship == "first_workflow_plan" {
			foundWorkflowLink = true
		}
	}
	if !foundWorkflowLink {
		t.Fatalf("first workflow plan link missing: %#v", links)
	}
	activity, _ := service.Activity(created.ID)
	foundPlanned := false
	for _, item := range activity {
		if item.EventType == "pursuit.planned" {
			foundPlanned = true
			break
		}
	}
	if !foundPlanned {
		t.Fatalf("pursuit planned activity missing: %#v", activity)
	}
}

func TestDetailSurfacesBlockersAndCompletionCandidate(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Finish HAI dashboard", ProjectKey: "018-HAI"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	blockedID := uuid.New()
	completedID := uuid.New()
	repo.workflows[blockedID] = models.WorkflowItem{
		ID:            blockedID,
		Title:         "Connect real source",
		ProjectKey:    "018-HAI",
		CurrentState:  workflow.StateBlocked,
		BlockedReason: "waiting for credentials",
		RiskLevel:     "medium",
	}
	repo.workflows[completedID] = models.WorkflowItem{
		ID:           completedID,
		Title:        "Create dashboard shell",
		ProjectKey:   "018-HAI",
		CurrentState: workflow.StateCompleted,
	}
	_, _ = service.Link(created.ID, LinkRequest{LinkType: LinkWorkflow, LinkID: blockedID.String()})
	_, _ = service.Link(created.ID, LinkRequest{LinkType: LinkWorkflow, LinkID: completedID.String()})

	detail, err := service.Detail(created.ID)
	if err != nil {
		t.Fatalf("Detail returned error: %v", err)
	}
	if len(detail.Blockers) != 1 || !strings.Contains(detail.Blockers[0].Reason, "credentials") {
		t.Fatalf("blockers = %#v", detail.Blockers)
	}
	if detail.Summary.CompletionCandidate {
		t.Fatalf("blocked pursuit cannot be a completion candidate")
	}
}

func TestDetailSurfacesFailedQualityGateAsPursuitBlocker(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Production-ready dashboard", ProjectKey: "018-HAI"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	workflowID := uuid.New()
	repo.workflows[workflowID] = models.WorkflowItem{
		ID:                 workflowID,
		Title:              "Validate dashboard build",
		ProjectKey:         "018-HAI",
		CurrentState:       workflow.StateCompleted,
		VerificationStatus: "verified",
	}
	repo.qualityGates = append(repo.qualityGates, models.WorkflowQualityGate{
		ID:         uuid.New(),
		WorkflowID: workflowID,
		Gate:       "tests or build evidence",
		Status:     "failed",
		Reason:     "the production build evidence is missing",
	})
	_, _ = service.Link(created.ID, LinkRequest{LinkType: LinkWorkflow, LinkID: workflowID.String()})

	detail, err := service.Detail(created.ID)
	if err != nil {
		t.Fatalf("Detail returned error: %v", err)
	}
	if len(detail.QualityGates) != 1 || detail.Summary.QualityGatesNeedingReview != 1 {
		t.Fatalf("quality gate summary = gates=%#v summary=%#v", detail.QualityGates, detail.Summary)
	}
	if len(detail.Blockers) != 1 || !strings.Contains(detail.Blockers[0].Label, "tests or build evidence") {
		t.Fatalf("quality gate blocker = %#v", detail.Blockers)
	}
	if len(detail.ActionQueues.NeedsRobert) != 1 || !strings.Contains(detail.ActionQueues.NeedsRobert[0].Label, "production build evidence") {
		t.Fatalf("quality gate action queue = %#v", detail.ActionQueues)
	}
	if detail.Summary.CompletionCandidate {
		t.Fatalf("failed quality gate must prevent a completion candidate: %#v", detail.Summary)
	}
}

func TestDetailSurfacesCompletionReviewDecisionForVerifiedWorkflows(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Close verified OpenClaw recovery", ProjectKey: "018-HAI"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	workflowID := uuid.New()
	repo.workflows[workflowID] = models.WorkflowItem{
		ID:                 workflowID,
		Title:              "Verify OpenClaw recovery",
		ProjectKey:         "018-HAI",
		CurrentState:       workflow.StateCompleted,
		VerificationStatus: "verified",
	}
	_, _ = service.Link(created.ID, LinkRequest{LinkType: LinkWorkflow, LinkID: workflowID.String()})

	detail, err := service.Detail(created.ID)
	if err != nil {
		t.Fatalf("Detail returned error: %v", err)
	}
	if !detail.Summary.CompletionCandidate {
		t.Fatalf("summary = %#v, want completion candidate", detail.Summary)
	}
	if detail.Summary.LinkedEvidence != 1 {
		t.Fatalf("linked evidence = %d, want verified workflow counted", detail.Summary.LinkedEvidence)
	}
	foundDecision := false
	for _, decision := range detail.DecisionQueue {
		if decision.DecisionType == "pursuit_completion_review" {
			foundDecision = true
			if decision.YesLabel != "Mark complete" || decision.NoLabel != "Keep active" || !decision.RequiresApproval {
				t.Fatalf("completion decision invalid: %#v", decision)
			}
		}
	}
	if !foundDecision {
		t.Fatalf("completion review decision missing: %#v", detail.DecisionQueue)
	}
	foundAction := false
	for _, action := range detail.NextActions {
		if action.YesLabel == "Mark complete" {
			foundAction = true
		}
	}
	if !foundAction || len(detail.ActionQueues.NeedsRobert) == 0 {
		t.Fatalf("completion review action missing: actions=%#v queues=%#v", detail.NextActions, detail.ActionQueues)
	}
}

func TestResolveCompletionReviewDecisionMarksPursuitComplete(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Close verified evidence pursuit", ProjectKey: "vivare"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	workflowID := uuid.New()
	repo.workflows[workflowID] = models.WorkflowItem{
		ID:                 workflowID,
		Title:              "Verify evidence bundle",
		ProjectKey:         "vivare",
		CurrentState:       workflow.StateCompleted,
		VerificationStatus: "verified",
	}
	repo.evidence = append(repo.evidence, models.WorkflowEvidenceClaim{
		ID:         uuid.New(),
		WorkflowID: workflowID,
		ClaimText:  "Evidence bundle checked against source records.",
		SourceURI:  "local://evidence/vivare-bundle",
		Status:     "verified",
	})
	_, _ = service.Link(created.ID, LinkRequest{LinkType: LinkWorkflow, LinkID: workflowID.String()})

	detail, err := service.ResolveDecision(created.ID, DecisionResolutionRequest{
		DecisionID:    completionReviewDecisionID(created.ID),
		DecisionType:  "pursuit_completion_review",
		Approved:      true,
		Note:          "Evidence is verified; close the pursuit.",
		EvidenceURI:   "local://evidence/vivare-bundle",
		EvidenceLabel: "Vivare verified evidence bundle",
		Actor:         "Robert",
	})
	if err != nil {
		t.Fatalf("ResolveDecision returned error: %v", err)
	}
	if detail.Pursuit.Status != StatusCompleted || detail.Pursuit.CompletionState != CompletionVerified {
		t.Fatalf("pursuit completion = %s/%s, want completed/verified", detail.Pursuit.Status, detail.Pursuit.CompletionState)
	}
	if detail.Summary.CompletionCandidate || detail.Summary.NeedsRobert != 0 {
		t.Fatalf("completed pursuit still looks actionable: %#v", detail.Summary)
	}
	for _, decision := range detail.DecisionQueue {
		if decision.DecisionType == "pursuit_completion_review" {
			t.Fatalf("completed pursuit still has completion review decision: %#v", detail.DecisionQueue)
		}
	}
	if !timelineContains(detail.Timeline, "pursuit_activity", "Evidence is verified") {
		t.Fatalf("timeline = %#v, want completion decision note", detail.Timeline)
	}
	if !timelineContains(detail.Timeline, "pursuit_activity", "Pursuit marked completed") {
		t.Fatalf("timeline = %#v, want completed activity", detail.Timeline)
	}
}

func TestResolveCompletionReviewDecisionRejectsUnverifiedRequest(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Do not close unsupported pursuit", ProjectKey: "vivare"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	_, err = service.ResolveDecision(created.ID, DecisionResolutionRequest{
		DecisionID:   completionReviewDecisionID(created.ID),
		DecisionType: "pursuit_completion_review",
		Approved:     true,
		Actor:        "Robert",
	})
	if err == nil || !strings.Contains(err.Error(), "requires verified evidence") {
		t.Fatalf("ResolveDecision error = %v, want verified evidence guard", err)
	}
	unchanged, _ := repo.FindByID(created.ID)
	if unchanged.Status == StatusCompleted || unchanged.CompletionState == CompletionVerified {
		t.Fatalf("pursuit completion changed despite missing evidence: %#v", unchanged)
	}
	activity, _ := repo.FindActivities(created.ID, 10)
	for _, item := range activity {
		if item.EventType == "pursuit.decision_resolved" || item.EventType == "pursuit.completed" {
			t.Fatalf("unsafe completion decision was audited as resolved: %#v", activity)
		}
	}
}

func TestDashboardUsesComputedCompletionCandidate(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Verify dashboard completion queue", ProjectKey: "018-HAI"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	workflowID := uuid.New()
	repo.workflows[workflowID] = models.WorkflowItem{
		ID:                 workflowID,
		Title:              "Verify completed workflow evidence",
		ProjectKey:         "018-HAI",
		CurrentState:       workflow.StateCompleted,
		VerificationStatus: "verified",
	}
	_, _ = service.Link(created.ID, LinkRequest{LinkType: LinkWorkflow, LinkID: workflowID.String()})

	stored, _ := repo.FindByID(created.ID)
	if stored.CompletionState == CompletionCandidate {
		t.Fatalf("test setup invalid: stored pursuit should not already be a completion candidate")
	}
	dashboard, err := service.Dashboard()
	if err != nil {
		t.Fatalf("Dashboard returned error: %v", err)
	}
	if dashboard.Counts["completionCandidates"] != 1 || len(dashboard.CompletionCandidates) != 1 {
		t.Fatalf("dashboard did not use computed completion candidate: counts=%#v candidates=%#v", dashboard.Counts, dashboard.CompletionCandidates)
	}
	if dashboard.CompletionCandidates[0].Pursuit.ID != created.ID || !dashboard.CompletionCandidates[0].CompletionCandidate {
		t.Fatalf("completion candidate metadata missing: %#v", dashboard.CompletionCandidates)
	}
}

func TestDashboardStatusCountsUseComputedBlockers(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	blockedPursuit, err := service.Create(CreateRequest{Title: "Blocked by linked workflow", ProjectKey: "ops"})
	if err != nil {
		t.Fatalf("Create blocked pursuit returned error: %v", err)
	}
	activePursuit, err := service.Create(CreateRequest{Title: "Active clean pursuit", ProjectKey: "ops"})
	if err != nil {
		t.Fatalf("Create active pursuit returned error: %v", err)
	}
	workflowID := uuid.New()
	repo.workflows[workflowID] = models.WorkflowItem{
		ID:            workflowID,
		Title:         "Wait for source credentials",
		ProjectKey:    "ops",
		CurrentState:  workflow.StateBlocked,
		BlockedReason: "missing source credentials",
	}
	_, _ = service.Link(blockedPursuit.ID, LinkRequest{LinkType: LinkWorkflow, LinkID: workflowID.String()})

	stored, _ := repo.FindByID(blockedPursuit.ID)
	if stored.Status != StatusActive {
		t.Fatalf("test setup invalid: stored pursuit status = %q, want active", stored.Status)
	}

	dashboard, err := service.Dashboard()
	if err != nil {
		t.Fatalf("Dashboard returned error: %v", err)
	}
	if dashboard.Counts["blocked"] != 1 || dashboard.Counts["active"] != 1 {
		t.Fatalf("dashboard counts = %#v, want computed blocked=1 and active=1", dashboard.Counts)
	}
	if len(dashboard.Blocked) != 1 || dashboard.Blocked[0].Pursuit.ID != blockedPursuit.ID {
		t.Fatalf("blocked queue = %#v, want blocked pursuit", dashboard.Blocked)
	}
	if activePursuit.ID == uuid.Nil {
		t.Fatalf("active pursuit was not created")
	}
}

func TestDashboardExcludesClosedPursuitsFromOperationalQueues(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	closed, err := service.Create(CreateRequest{Title: "Completed insurance evidence pursuit", ProjectKey: "asr"})
	if err != nil {
		t.Fatalf("Create closed pursuit returned error: %v", err)
	}
	active, err := service.Create(CreateRequest{Title: "Active automation pursuit", ProjectKey: "018-HAI"})
	if err != nil {
		t.Fatalf("Create active pursuit returned error: %v", err)
	}
	stored, _ := repo.FindByID(closed.ID)
	stored.Status = StatusCompleted
	stored.CompletionState = CompletionVerified
	if _, err := repo.Update(stored); err != nil {
		t.Fatalf("mark pursuit complete: %v", err)
	}

	dashboard, err := service.Dashboard()
	if err != nil {
		t.Fatalf("Dashboard returned error: %v", err)
	}
	if dashboard.Counts["completed"] != 1 || dashboard.Counts["active"] != 1 {
		t.Fatalf("dashboard counts = %#v, want completed=1 active=1", dashboard.Counts)
	}
	for _, queue := range [][]PursuitListItem{
		dashboard.NeedsRobert,
		dashboard.VAReady,
		dashboard.SystemReady,
		dashboard.Blocked,
		dashboard.Stale,
		dashboard.ReviewDue,
		dashboard.PlanningNeeded,
		dashboard.HighRisk,
		dashboard.CompletionCandidates,
		dashboard.RecentlyChanged,
	} {
		for _, item := range queue {
			if item.Pursuit.ID == closed.ID {
				t.Fatalf("closed pursuit leaked into operational queue: %#v", item)
			}
		}
	}
	if active.ID == uuid.Nil {
		t.Fatalf("active pursuit was not created")
	}
}

func TestClosedPursuitRejectsOperationalMutationAndSummaryRefresh(t *testing.T) {
	repo := newFakeRepo()
	workflowService := &fakeWorkflowIntake{repo: repo}
	service := NewService(repo, workflowService)
	created, err := service.Create(CreateRequest{Title: "Completed legal evidence pursuit", ProjectKey: "vivare"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	stored, _ := repo.FindByID(created.ID)
	stored.Status = StatusCompleted
	stored.CompletionState = CompletionVerified
	if _, err := repo.Update(stored); err != nil {
		t.Fatalf("mark pursuit complete: %v", err)
	}

	operations := []struct {
		name string
		run  func() error
	}{
		{
			name: "intake",
			run: func() error {
				_, err := service.Intake(created.ID, IntakeRequest{Input: "Draft another lawyer response"})
				return err
			},
		},
		{
			name: "plan",
			run: func() error {
				_, err := service.Plan(created.ID, PlanRequest{})
				return err
			},
		},
		{
			name: "decision",
			run: func() error {
				_, err := service.ResolveDecision(created.ID, DecisionResolutionRequest{DecisionID: "workflow:late:approval", Approved: true})
				return err
			},
		},
	}
	for _, operation := range operations {
		if err := operation.run(); err == nil || !strings.Contains(err.Error(), "closed pursuit") {
			t.Fatalf("%s error = %v, want closed pursuit rejection", operation.name, err)
		}
	}
	if workflowService.calls != 0 {
		t.Fatalf("closed pursuit created %d workflow(s)", workflowService.calls)
	}

	refreshed, err := service.RefreshSummary(created.ID, "system")
	if err != nil {
		t.Fatalf("RefreshSummary returned error: %v", err)
	}
	if refreshed.Pursuit.Status != StatusCompleted || refreshed.Pursuit.CompletionState != CompletionVerified {
		t.Fatalf("closed pursuit was reactivated by refresh: %#v", refreshed.Pursuit)
	}
}

func TestReopenRequiresExplicitTransitionAndRestoresGovernedIntake(t *testing.T) {
	repo := newFakeRepo()
	workflowService := &fakeWorkflowIntake{repo: repo}
	service := NewService(repo, workflowService)
	created, err := service.Create(CreateRequest{Title: "Archived automation recovery", ProjectKey: "018-HAI"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	stored, _ := repo.FindByID(created.ID)
	stored.Status = StatusCompleted
	stored.CompletionState = CompletionVerified
	stored.Archived = true
	if _, err := repo.Update(stored); err != nil {
		t.Fatalf("close pursuit: %v", err)
	}

	if _, err := service.Update(created.ID, UpdateRequest{Status: StatusActive, Actor: "operator"}); err == nil || !strings.Contains(err.Error(), "explicit reopen") {
		t.Fatalf("generic reactivation error = %v, want explicit reopen rejection", err)
	}
	if _, err := service.Archive(created.ID, false, "Robert"); err != nil {
		t.Fatalf("Archive restore should use reopen transition: %v", err)
	}
	reopened, err := repo.FindByID(created.ID)
	if err != nil {
		t.Fatalf("Find reopened pursuit: %v", err)
	}
	if reopened.Archived || reopened.Status != StatusActive || reopened.CompletionState != CompletionOpen {
		t.Fatalf("reopened pursuit = %#v", reopened)
	}
	activity, _ := repo.FindActivities(created.ID, 20)
	if !activityContains(activity, "pursuit.reopened") {
		t.Fatalf("reopen was not audited: %#v", activity)
	}
	if _, err := service.Intake(created.ID, IntakeRequest{Input: "Create a governed recovery workflow"}); err != nil {
		t.Fatalf("reopened pursuit intake returned error: %v", err)
	}
	if workflowService.calls != 1 {
		t.Fatalf("reopened pursuit created %d workflows, want 1", workflowService.calls)
	}
}

func activityContains(activity []models.PursuitActivity, eventType string) bool {
	for _, item := range activity {
		if item.EventType == eventType {
			return true
		}
	}
	return false
}

func TestDetailSeparatesRobertVAAndSystemActionQueues(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Client delivery follow-up", ProjectKey: "client", Description: "Client deadline needs a quote reply"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	approvalID := uuid.New()
	loopID := uuid.New()
	followUp := time.Now().Add(-2 * time.Hour).UTC()
	repo.workflows[approvalID] = models.WorkflowItem{
		ID:               approvalID,
		Title:            "Send client quote reply",
		ProjectKey:       "client",
		CurrentState:     workflow.StateNeedsApproval,
		RequiresApproval: true,
		ApprovalStatus:   "pending",
		ApprovalReason:   "external client communication requires Robert approval",
		RiskLevel:        "medium",
		NextAction:       "Approve the prepared client reply",
	}
	repo.workflows[loopID] = models.WorkflowItem{
		ID:           loopID,
		Title:        "Chase missing client input",
		ProjectKey:   "client",
		CurrentState: workflow.StateWaitingInput,
		RiskLevel:    "medium",
	}
	repo.openLoops = append(repo.openLoops, models.WorkflowOpenLoop{
		ID:               uuid.New(),
		WorkflowID:       loopID,
		ResponsibleParty: "VA",
		WaitingFor:       "client address confirmation",
		NextAction:       "Prepare a polite follow-up for the missing address",
		FollowUpAt:       &followUp,
		Status:           "follow_up_due",
	})
	_, _ = service.Link(created.ID, LinkRequest{LinkType: LinkWorkflow, LinkID: approvalID.String()})
	_, _ = service.Link(created.ID, LinkRequest{LinkType: LinkWorkflow, LinkID: loopID.String()})

	detail, err := service.Detail(created.ID)
	if err != nil {
		t.Fatalf("Detail returned error: %v", err)
	}
	if len(detail.ActionQueues.NeedsRobert) != 1 || !strings.Contains(detail.ActionQueues.NeedsRobert[0].Label, "Approve") {
		t.Fatalf("Robert queue = %#v", detail.ActionQueues.NeedsRobert)
	}
	if len(detail.ActionQueues.VAReady) != 1 || !strings.Contains(detail.ActionQueues.VAReady[0].Label, "follow-up") {
		t.Fatalf("VA queue = %#v", detail.ActionQueues.VAReady)
	}
	if len(detail.ActionQueues.Waiting) != 0 {
		t.Fatalf("waiting queue duplicated actionable open loop: %#v", detail.ActionQueues.Waiting)
	}
	if detail.Summary.RobertActions != 1 || detail.Summary.VAReadyActions != 1 {
		t.Fatalf("summary action counts = %#v", detail.Summary)
	}

	emptyLowRisk, err := service.Create(CreateRequest{Title: "Organize local notes", ProjectKey: "notes"})
	if err != nil {
		t.Fatalf("Create empty pursuit returned error: %v", err)
	}
	emptyDetail, err := service.Detail(emptyLowRisk.ID)
	if err != nil {
		t.Fatalf("Detail empty pursuit returned error: %v", err)
	}
	if len(emptyDetail.ActionQueues.SystemReady) != 1 || emptyDetail.Summary.SystemReadyActions != 1 {
		t.Fatalf("system queue = %#v summary=%#v", emptyDetail.ActionQueues.SystemReady, emptyDetail.Summary)
	}
}

func TestUpdateRejectsVerifiedCompletionWithoutEvidence(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Finish legal evidence bundle", ProjectKey: "vivare"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	_, err = service.Update(created.ID, UpdateRequest{
		Status:          StatusCompleted,
		CompletionState: CompletionVerified,
		Actor:           "operator",
	})
	if err == nil || !strings.Contains(err.Error(), "requires verified evidence") {
		t.Fatalf("Update error = %v, want verified evidence guard", err)
	}
	unchanged, _ := repo.FindByID(created.ID)
	if unchanged.Status == StatusCompleted || unchanged.CompletionState == CompletionVerified {
		t.Fatalf("pursuit completion changed despite missing evidence: %#v", unchanged)
	}
}

func TestUpdateAllowsVerifiedCompletionWithEvidence(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Finish legal evidence bundle", ProjectKey: "vivare"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	workflowID := uuid.New()
	repo.workflows[workflowID] = models.WorkflowItem{
		ID:                 workflowID,
		Title:              "Prepare evidence bundle",
		ProjectKey:         "vivare",
		CurrentState:       workflow.StateCompleted,
		VerificationStatus: "verified",
	}
	repo.evidence = append(repo.evidence, models.WorkflowEvidenceClaim{
		ID:         uuid.New(),
		WorkflowID: workflowID,
		ClaimText:  "Evidence bundle was reviewed and completed.",
		SourceURI:  "local://evidence/bundle",
		Status:     "verified",
	})
	_, _ = service.Link(created.ID, LinkRequest{LinkType: LinkWorkflow, LinkID: workflowID.String()})

	updated, err := service.Update(created.ID, UpdateRequest{
		Status: StatusCompleted,
		Actor:  "operator",
	})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if updated.Status != StatusCompleted || updated.CompletionState != CompletionVerified {
		t.Fatalf("updated completion = %s/%s, want completed/verified", updated.Status, updated.CompletionState)
	}
	detail, err := service.Detail(created.ID)
	if err != nil {
		t.Fatalf("Detail returned error: %v", err)
	}
	if detail.Summary.CompletionCandidate {
		t.Fatalf("completed pursuit remained a completion candidate: %#v", detail.Summary)
	}
	for _, decision := range detail.DecisionQueue {
		if decision.DecisionType == "pursuit_completion_review" {
			t.Fatalf("completed pursuit still has completion review decision: %#v", detail.DecisionQueue)
		}
	}
	for _, action := range detail.NextActions {
		if action.YesLabel == "Mark complete" {
			t.Fatalf("completed pursuit still has close-out action: %#v", detail.NextActions)
		}
	}
	if detail.Summary.NeedsRobert != 0 || len(detail.ActionQueues.NeedsRobert) != 0 {
		t.Fatalf("completed pursuit still needs Robert: summary=%#v queues=%#v", detail.Summary, detail.ActionQueues)
	}
}

func TestUpdateBlocksVerifiedCompletionWithUnresolvedWorkflowBlocker(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Finish legal evidence bundle with blocker", ProjectKey: "vivare"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	completedID := uuid.New()
	blockedID := uuid.New()
	repo.workflows[completedID] = models.WorkflowItem{
		ID:                 completedID,
		Title:              "Prepare evidence bundle",
		ProjectKey:         "vivare",
		CurrentState:       workflow.StateCompleted,
		VerificationStatus: "verified",
	}
	repo.workflows[blockedID] = models.WorkflowItem{
		ID:            blockedID,
		Title:         "Wait for missing lawyer document",
		ProjectKey:    "vivare",
		CurrentState:  workflow.StateBlocked,
		BlockedReason: "waiting for signed lawyer statement",
	}
	repo.evidence = append(repo.evidence, models.WorkflowEvidenceClaim{
		ID:         uuid.New(),
		WorkflowID: completedID,
		ClaimText:  "Evidence bundle was reviewed and completed.",
		SourceURI:  "local://evidence/bundle",
		Status:     "verified",
	})
	_, _ = service.Link(created.ID, LinkRequest{LinkType: LinkWorkflow, LinkID: completedID.String()})
	_, _ = service.Link(created.ID, LinkRequest{LinkType: LinkWorkflow, LinkID: blockedID.String()})

	_, err = service.Update(created.ID, UpdateRequest{
		Status: StatusCompleted,
		Actor:  "operator",
	})
	if err == nil || !strings.Contains(err.Error(), "unresolved operational work") || !strings.Contains(err.Error(), "lawyer statement") {
		t.Fatalf("Update error = %v, want unresolved blocker guard", err)
	}
	unchanged, _ := repo.FindByID(created.ID)
	if unchanged.Status == StatusCompleted || unchanged.CompletionState == CompletionVerified {
		t.Fatalf("pursuit completion changed despite unresolved blocker: %#v", unchanged)
	}
}

func TestUpdateBlocksVerifiedCompletionWithOpenProposal(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Finish proposal-gated automation work", ProjectKey: "018-HAI"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	workflowID := uuid.New()
	repo.workflows[workflowID] = models.WorkflowItem{
		ID:                 workflowID,
		Title:              "Choose runtime recovery option",
		ProjectKey:         "018-HAI",
		CurrentState:       workflow.StateCompleted,
		VerificationStatus: "verified",
	}
	repo.evidence = append(repo.evidence, models.WorkflowEvidenceClaim{
		ID:         uuid.New(),
		WorkflowID: workflowID,
		ClaimText:  "Runtime recovery option was verified.",
		SourceURI:  "local://evidence/runtime-recovery",
		Status:     "verified",
	})
	repo.proposals = append(repo.proposals, models.WorkflowProposal{
		ID:                uuid.New(),
		WorkflowID:        workflowID,
		Status:            "open",
		RecommendedAction: "Choose whether to retry OpenClaw now or keep it disabled.",
		Options:           "Retry now / keep disabled",
	})
	_, _ = service.Link(created.ID, LinkRequest{LinkType: LinkWorkflow, LinkID: workflowID.String()})

	_, err = service.Update(created.ID, UpdateRequest{
		Status: StatusCompleted,
		Actor:  "operator",
	})
	if err == nil || !strings.Contains(err.Error(), "unresolved operational work") || !strings.Contains(err.Error(), "retry OpenClaw") {
		t.Fatalf("Update error = %v, want unresolved proposal guard", err)
	}
	unchanged, _ := repo.FindByID(created.ID)
	if unchanged.Status == StatusCompleted || unchanged.CompletionState == CompletionVerified {
		t.Fatalf("pursuit completion changed despite open proposal: %#v", unchanged)
	}
}

func TestUpdateBlocksVerifiedCompletionWithNeedsReviewDecision(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Finish decision-gated automation work", ProjectKey: "018-HAI"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	workflowID := uuid.New()
	repo.workflows[workflowID] = models.WorkflowItem{
		ID:                 workflowID,
		Title:              "Verify runtime execution decision",
		ProjectKey:         "018-HAI",
		CurrentState:       workflow.StateCompleted,
		VerificationStatus: "verified",
	}
	repo.evidence = append(repo.evidence, models.WorkflowEvidenceClaim{
		ID:         uuid.New(),
		WorkflowID: workflowID,
		ClaimText:  "Runtime execution was verified.",
		SourceURI:  "local://evidence/runtime-execution",
		Status:     "verified",
	})
	repo.decisions = append(repo.decisions, models.WorkflowDecision{
		ID:           uuid.New(),
		WorkflowID:   workflowID,
		DecisionType: "worker_execution",
		Decision:     "needs_review",
		Reason:       "runtime output still needs Robert review before closing",
		RuleApplied:  "completion engine",
		Actor:        "workflow-worker",
		CreatedAt:    time.Now().UTC(),
	})
	_, _ = service.Link(created.ID, LinkRequest{LinkType: LinkWorkflow, LinkID: workflowID.String()})

	_, err = service.Update(created.ID, UpdateRequest{
		Status: StatusCompleted,
		Actor:  "operator",
	})
	if err == nil || !strings.Contains(err.Error(), "unresolved operational work") || !strings.Contains(err.Error(), "Robert review") {
		t.Fatalf("Update error = %v, want unresolved decision guard", err)
	}
	unchanged, _ := repo.FindByID(created.ID)
	if unchanged.Status == StatusCompleted || unchanged.CompletionState == CompletionVerified {
		t.Fatalf("pursuit completion changed despite needs-review decision: %#v", unchanged)
	}
}

func TestUpdateAllowsVerifiedCompletionWithLinkedVerification(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Complete automation runtime audit", ProjectKey: "018-HAI"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	_, _ = service.Link(created.ID, LinkRequest{
		LinkType:     LinkVerification,
		LinkID:       uuid.New().String(),
		Relationship: "completion_evidence",
		SourceURI:    "local://verification/runtime-audit",
		SourceLabel:  "Runtime audit verification",
	})

	updated, err := service.Update(created.ID, UpdateRequest{
		CompletionState: CompletionVerified,
		Actor:           "operator",
	})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if updated.CompletionState != CompletionVerified {
		t.Fatalf("completion state = %s, want verified", updated.CompletionState)
	}
}

func TestUpdateRejectsBareVerificationLinkWithoutEvidence(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Complete unsupported verification link", ProjectKey: "018-HAI"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	_, _ = service.Link(created.ID, LinkRequest{
		LinkType: LinkVerification,
		LinkID:   uuid.New().String(),
	})

	_, err = service.Update(created.ID, UpdateRequest{
		CompletionState: CompletionVerified,
		Actor:           "operator",
	})
	if err == nil || !strings.Contains(err.Error(), "requires verified evidence") {
		t.Fatalf("Update error = %v, want verified evidence guard", err)
	}
}

func TestDetailSurfacesLinkedVerificationRunEvidence(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Verify OpenClaw completion", ProjectKey: "018-HAI"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	runID := uuid.New()
	claimID := uuid.New()
	evidenceID := uuid.New()
	repo.verificationRuns[runID] = models.VerificationRun{
		ID:         runID,
		Mode:       "action",
		Question:   "Did OpenClaw complete the approved task?",
		ProjectKey: "018-HAI",
		Status:     "verified",
		Answer:     "The runtime attempt completed under HAI controls.",
	}
	repo.verificationClaims[claimID] = models.VerificationClaim{
		ID:        claimID,
		RunID:     runID,
		ClaimText: "The runtime attempt completed under HAI controls.",
		Status:    "verified",
	}
	repo.verificationEvidence[evidenceID] = models.VerificationEvidence{
		ID:          evidenceID,
		RunID:       runID,
		SourceType:  "automation_launch",
		SourceURI:   "automation-launch://openclaw",
		SourceLabel: "OpenClaw launch event",
		Snippet:     "OpenClaw completed the approved agent task.",
		Used:        true,
	}
	_, _ = service.Link(created.ID, LinkRequest{LinkType: LinkVerification, LinkID: runID.String(), Relationship: "completion_evidence"})

	detail, err := service.Detail(created.ID)
	if err != nil {
		t.Fatalf("Detail returned error: %v", err)
	}
	if len(detail.VerificationRuns) != 1 || detail.VerificationRuns[0].Status != "verified" {
		t.Fatalf("verification runs = %#v", detail.VerificationRuns)
	}
	if len(detail.VerificationClaims) != 1 || len(detail.VerificationEvidence) != 1 {
		t.Fatalf("verification claims/evidence = %#v / %#v", detail.VerificationClaims, detail.VerificationEvidence)
	}
	if detail.Summary.VerificationRuns != 1 || detail.Summary.LinkedEvidence == 0 {
		t.Fatalf("summary = %#v, want verification evidence counted", detail.Summary)
	}
}

func TestLinkVerificationCreatesAuditableEvidenceLink(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Verify legal evidence", ProjectKey: "vivare"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	verificationID := uuid.New()

	if err := service.LinkVerification(created.ID, verificationID); err != nil {
		t.Fatalf("LinkVerification returned error: %v", err)
	}

	links, err := repo.FindLinks(created.ID)
	if err != nil {
		t.Fatalf("FindLinks returned error: %v", err)
	}
	found := false
	for _, link := range links {
		if link.LinkType == LinkVerification && link.LinkID == verificationID.String() && link.Relationship == "verification_evidence" && link.SourceURI == "verification://"+verificationID.String() {
			found = true
		}
	}
	if !found {
		t.Fatalf("verification evidence link missing: %#v", links)
	}
}

func TestOwnerScopedVerificationLinkRejectsAnotherOwner(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	bob, err := service.Create(CreateRequest{Title: "Bob private verification", OwnerIdentity: "bob"})
	if err != nil {
		t.Fatalf("Create Bob pursuit: %v", err)
	}

	if err := service.LinkVerificationForOwner("alice", bob.ID, uuid.New()); err == nil {
		t.Fatal("Alice could link verification evidence to Bob's pursuit")
	}
	links, err := repo.FindLinks(bob.ID)
	if err != nil {
		t.Fatalf("FindLinks: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("cross-owner verification link persisted: %#v", links)
	}
}

func TestDetailSurfacesTaskRunEvidenceFromLinkedWorkflows(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Advance task runner pursuit", ProjectKey: "018-HAI"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	workflowID := uuid.New()
	lastRun := time.Now().UTC()
	nextRun := lastRun.Add(30 * time.Minute)
	repo.workflows[workflowID] = models.WorkflowItem{
		ID:                 workflowID,
		Title:              "Run task engine workflow",
		ProjectKey:         "018-HAI",
		CurrentState:       workflow.StateReady,
		LastTaskPlanID:     "task-plan-123",
		VerificationStatus: "needs_review",
		RetryCount:         1,
		MaxRetries:         2,
		LastRunAt:          &lastRun,
		NextRunAt:          &nextRun,
		LastWorkerError:    "validation failed after controlled runtime output",
		AutomationID:       uuid.NewString(),
	}
	_, _ = service.Link(created.ID, LinkRequest{LinkType: LinkWorkflow, LinkID: workflowID.String()})

	detail, err := service.Detail(created.ID)
	if err != nil {
		t.Fatalf("Detail returned error: %v", err)
	}
	if len(detail.TaskRuns) != 1 {
		t.Fatalf("task runs = %#v, want one execution summary", detail.TaskRuns)
	}
	run := detail.TaskRuns[0]
	if run.TaskPlanID != "task-plan-123" || !run.NeedsReview || run.Status != "blocked" {
		t.Fatalf("task run = %#v, want blocked needs-review task evidence", run)
	}
	if detail.Summary.TaskRuns != 1 {
		t.Fatalf("summary task runs = %d, want 1", detail.Summary.TaskRuns)
	}
}

func TestDetailSurfacesRobertDecisionQueueFromLinkedWorkflows(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Resolve legal approval pursuit", ProjectKey: "Vivare", RiskLevel: "high"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	workflowID := uuid.New()
	repo.workflows[workflowID] = models.WorkflowItem{
		ID:               workflowID,
		Title:            "Draft formal reply to lawyer",
		ProjectKey:       "Vivare",
		CurrentState:     workflow.StateNeedsApproval,
		RiskLevel:        "high",
		RequiresApproval: true,
		ApprovalStatus:   "pending",
		ApprovalReason:   "Legal external email must be approved by Robert before sending.",
		NextAction:       "Approve the draft reply or request revision.",
		SourceURI:        "email://lawyer/request",
		SourceLabel:      "Lawyer request email",
	}
	repo.proposals = append(repo.proposals, models.WorkflowProposal{
		ID:                uuid.New(),
		WorkflowID:        workflowID,
		RecommendedAction: "Send the formal reply only after evidence links are checked.",
		Options:           "Approve / request more evidence / reject",
		Status:            "open",
		CreatedAt:         time.Now().UTC(),
	})
	repo.decisions = append(repo.decisions, models.WorkflowDecision{
		ID:           uuid.New(),
		WorkflowID:   workflowID,
		DecisionType: "approval_gate",
		Decision:     "required",
		Reason:       "legal workflow must remain draft-only until approved",
		RuleApplied:  "approval rule engine",
		Actor:        "engine",
		CreatedAt:    time.Now().UTC(),
	})
	_, _ = service.Link(created.ID, LinkRequest{LinkType: LinkWorkflow, LinkID: workflowID.String()})

	detail, err := service.Detail(created.ID)
	if err != nil {
		t.Fatalf("Detail returned error: %v", err)
	}
	if len(detail.Decisions) != 1 {
		t.Fatalf("decisions = %#v, want linked workflow decision history", detail.Decisions)
	}
	if len(detail.DecisionQueue) != 3 {
		t.Fatalf("decision queue = %#v, want approval, proposal, and audit cards", detail.DecisionQueue)
	}
	first := detail.DecisionQueue[0]
	if first.Status != "pending" || !first.RequiresApproval || first.EvidenceURI != "email://lawyer/request" {
		t.Fatalf("first decision card = %#v, want pending approval with source evidence", first)
	}
	if first.YesConsequence == "" || first.NoConsequence == "" {
		t.Fatalf("first decision card missing consequences: %#v", first)
	}
	if detail.Summary.NeedsRobert != 2 || detail.Summary.DecisionCards != 3 {
		t.Fatalf("summary = %#v, want two pending Robert decisions and three decision cards", detail.Summary)
	}
}

func TestApprovalsReturnsGovernedDecisionOverview(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Approve legal source response", ProjectKey: "Vivare", RiskLevel: "high"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	workflowID := uuid.New()
	repo.workflows[workflowID] = models.WorkflowItem{
		ID:               workflowID,
		Title:            "Send lawyer evidence bundle",
		ProjectKey:       "Vivare",
		CurrentState:     workflow.StateNeedsApproval,
		RiskLevel:        "high",
		RequiresApproval: true,
		ApprovalStatus:   "pending",
		ApprovalReason:   "Legal correspondence must remain draft-only until Robert approves it.",
		NextAction:       "Review evidence bundle and approve or reject sending.",
		SourceURI:        "email://lawyer/evidence-bundle",
		SourceLabel:      "Lawyer evidence request",
	}
	_, _ = service.Link(created.ID, LinkRequest{LinkType: LinkWorkflow, LinkID: workflowID.String()})

	overview, err := service.Approvals(created.ID)
	if err != nil {
		t.Fatalf("Approvals returned error: %v", err)
	}
	if overview.Pursuit.ID != created.ID {
		t.Fatalf("overview pursuit id = %s, want %s", overview.Pursuit.ID, created.ID)
	}
	if len(overview.ApprovalItems) != 1 {
		t.Fatalf("approval items = %#v, want one raw approval workflow", overview.ApprovalItems)
	}
	if len(overview.DecisionQueue) != 1 || !overview.DecisionQueue[0].RequiresApproval {
		t.Fatalf("decision queue = %#v, want one Robert approval card", overview.DecisionQueue)
	}
	if len(overview.Actions) != 1 || !overview.Actions[0].RequiresApproval {
		t.Fatalf("approval actions = %#v, want one Robert approval action", overview.Actions)
	}
	if overview.Counts["approvalItems"] != 1 || overview.Counts["pendingDecisions"] != 1 || overview.Counts["needsRobert"] == 0 {
		t.Fatalf("counts = %#v, want approval, pending decision, and needs-Robert counts", overview.Counts)
	}
}

func TestDetailBuildsTimelineFromLinkedWorkflowAuditRecords(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Recover stuck workflow pursuit", ProjectKey: "018-HAI"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	workflowID := uuid.New()
	base := time.Now().UTC().Add(time.Hour)
	lastRun := base.Add(2 * time.Minute)
	repo.workflows[workflowID] = models.WorkflowItem{
		ID:             workflowID,
		Title:          "Recover controlled worker state",
		ProjectKey:     "018-HAI",
		CurrentState:   workflow.StateBlocked,
		RiskLevel:      "medium",
		LastRunAt:      &lastRun,
		LastTaskPlanID: "plan-recovery-1",
	}
	repo.transitions = append(repo.transitions, models.WorkflowTransition{
		ID:         uuid.New(),
		WorkflowID: workflowID,
		FromState:  workflow.StateInProgress,
		ToState:    workflow.StateBlocked,
		Trigger:    "worker recovery",
		Actor:      "workflow-worker",
		Reason:     "runtime result needs review",
		CreatedAt:  base,
	})
	repo.sourceLinks = append(repo.sourceLinks, models.WorkflowSourceLink{
		ID:           uuid.New(),
		WorkflowID:   workflowID,
		SourceType:   "email",
		SourceURI:    "email://thread/123",
		SourceLabel:  "Source email",
		Relationship: "evidence",
		CreatedAt:    base.Add(time.Minute),
	})
	repo.events = append(repo.events, models.WorkflowEvent{
		ID:         uuid.New(),
		WorkflowID: workflowID,
		EventType:  "worker_execution",
		Message:    "task engine blocked for review",
		ToState:    workflow.StateBlocked,
		Actor:      "workflow-worker",
		CreatedAt:  base.Add(3 * time.Minute),
	})
	_, _ = service.Link(created.ID, LinkRequest{LinkType: LinkWorkflow, LinkID: workflowID.String()})

	detail, err := service.Detail(created.ID)
	if err != nil {
		t.Fatalf("Detail returned error: %v", err)
	}
	if len(detail.Transitions) != 1 || len(detail.SourceLinks) != 1 || len(detail.Events) != 1 {
		t.Fatalf("audit records transitions=%#v sourceLinks=%#v events=%#v", detail.Transitions, detail.SourceLinks, detail.Events)
	}
	if len(detail.Timeline) < 5 {
		t.Fatalf("timeline = %#v, want pursuit, workflow, source, transition, and task records", detail.Timeline)
	}
	if detail.Timeline[0].Kind != "workflow_event" || detail.Timeline[0].WorkflowTitle != "Recover controlled worker state" {
		t.Fatalf("first timeline item = %#v, want newest workflow event with title", detail.Timeline[0])
	}
	if detail.Summary.TimelineItems != len(detail.Timeline) || !strings.Contains(detail.Summary.WhatChanged, "worker_execution") {
		t.Fatalf("summary = %#v, want timeline count and newest change", detail.Summary)
	}
}

func TestDetailSurfacesAutomationAndRuntimeAttempts(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Operate OpenClaw runtime safely", ProjectKey: "018-HAI"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	automationID := uuid.New()
	workflowID := uuid.New()
	repo.automations[automationID] = models.Automation{
		ID:          automationID,
		Name:        "OpenClaw runtime",
		RuntimeType: "openclaw",
		LaunchType:  "agent_runtime",
		Status:      "healthy",
	}
	repo.workflows[workflowID] = models.WorkflowItem{
		ID:           workflowID,
		Title:        "Run approved OpenClaw task",
		ProjectKey:   "018-HAI",
		CurrentState: workflow.StateCompleted,
		AutomationID: automationID.String(),
	}
	startedAt := time.Now().UTC()
	repo.launchEvents = append(repo.launchEvents, models.AutomationLaunchEvent{
		ID:           uuid.New(),
		AutomationID: automationID,
		RuntimeType:  "openclaw",
		LaunchType:   "agent_runtime",
		Status:       "completed",
		Message:      "OpenClaw completed the approved agent task",
		Output:       "bounded runtime output",
		AuditEvents: []string{
			"launch requested",
			"runtime safety policy evaluated",
			"OpenClaw ecosystem execution completed under explicit controls",
		},
		StartedAt:   startedAt,
		CompletedAt: startedAt.Add(time.Second),
	})
	_, _ = service.Link(created.ID, LinkRequest{LinkType: LinkWorkflow, LinkID: workflowID.String()})

	detail, err := service.Detail(created.ID)
	if err != nil {
		t.Fatalf("Detail returned error: %v", err)
	}
	if len(detail.Automations) != 1 || detail.Automations[0].RuntimeType != "openclaw" {
		t.Fatalf("automations = %#v", detail.Automations)
	}
	if len(detail.RuntimeAttempts) != 1 || detail.RuntimeAttempts[0].Status != "completed" {
		t.Fatalf("runtime attempts = %#v", detail.RuntimeAttempts)
	}
	if detail.Summary.RuntimeAttempts != 1 || detail.Summary.LinkedEvidence == 0 {
		t.Fatalf("summary = %#v, want runtime attempts counted as operational evidence", detail.Summary)
	}
	if detail.OperationalDigest.RuntimeAttempts != 1 || !strings.Contains(detail.OperationalDigest.RuntimeLine, "openclaw") {
		t.Fatalf("operational digest runtime line = %#v", detail.OperationalDigest)
	}
	if detail.OperationalDigest.Evidence == 0 || !strings.Contains(detail.OperationalDigest.EvidenceLine, "evidence") {
		t.Fatalf("operational digest evidence line = %#v", detail.OperationalDigest)
	}
	if !timelineContains(detail.Timeline, "runtime_audit", "runtime safety policy evaluated") {
		t.Fatalf("timeline = %#v, want runtime audit policy evaluation surfaced", detail.Timeline)
	}
	if !timelineContains(detail.Timeline, "runtime_audit", "OpenClaw ecosystem execution completed under explicit controls") {
		t.Fatalf("timeline = %#v, want runtime completion audit surfaced", detail.Timeline)
	}
	if !timelineSourceContains(detail.Timeline, "runtime_attempt", "automation-launch://") {
		t.Fatalf("timeline = %#v, want runtime attempt source URI surfaced", detail.Timeline)
	}
}

func TestAmbientOpportunityLinkCountsAsPursuitEvidence(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Restart stale admin pursuit", RiskLevel: "medium"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	opportunityID := uuid.New()
	sourceURI := "ambient://opportunities/" + opportunityID.String()

	if _, err := service.Link(created.ID, LinkRequest{
		LinkType:     LinkAmbientOpportunity,
		LinkID:       opportunityID.String(),
		Relationship: "ambient_proposal_accepted",
		SourceURI:    sourceURI,
		SourceLabel:  "Restart stale pursuit",
		Confidence:   0.82,
		Actor:        "ambient",
	}); err != nil {
		t.Fatalf("Link returned error: %v", err)
	}

	detail, err := service.Detail(created.ID)
	if err != nil {
		t.Fatalf("Detail returned error: %v", err)
	}
	if detail.Summary.LinkedEvidence != 1 {
		t.Fatalf("linked evidence = %d, want accepted ambient opportunity counted", detail.Summary.LinkedEvidence)
	}
	if detail.OperationalDigest.Evidence != 1 || !strings.Contains(detail.OperationalDigest.EvidenceLine, "1 evidence item") {
		t.Fatalf("operational digest = %#v, want ambient opportunity evidence surfaced", detail.OperationalDigest)
	}
	if !timelineContains(detail.Timeline, "pursuit_activity", "Linked ambient_opportunity") {
		t.Fatalf("timeline = %#v, want ambient opportunity link activity", detail.Timeline)
	}
}

func TestResolveEvidenceReturnsLinkedRuntimeAttempt(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Inspect linked OpenClaw evidence", ProjectKey: "018-HAI"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	launchID := uuid.New()
	repo.launchEvents = append(repo.launchEvents, models.AutomationLaunchEvent{
		ID:            launchID,
		RuntimeType:   "openclaw",
		LaunchType:    "agent_runtime_stop",
		RuntimeTaskID: created.ID.String(),
		Status:        "stopped",
		Message:       "runtime stop requested",
		AuditEvents:   []string{"runtime stop requested", "cancelled active runtime context"},
		ExitCode:      0,
		StartedAt:     time.Now().UTC(),
		CompletedAt:   time.Now().UTC(),
	})
	_, _ = service.Link(created.ID, LinkRequest{
		LinkType:     LinkAgentRuntime,
		LinkID:       launchID.String(),
		Relationship: "execution_attempt",
		SourceURI:    "automation-launch://" + launchID.String(),
		SourceLabel:  "OpenClaw runtime stop",
	})

	resolved, err := service.ResolveEvidence(created.ID, "automation-launch://"+launchID.String())
	if err != nil {
		t.Fatalf("ResolveEvidence returned error: %v", err)
	}
	if resolved.Kind != "runtime_attempt" || resolved.RuntimeAttempt == nil || resolved.RuntimeAttempt.ID != launchID {
		t.Fatalf("resolved evidence = %#v", resolved)
	}
	if resolved.Title != "Runtime stop: openclaw" || resolved.Status != "stopped" || resolved.NeedsReview {
		t.Fatalf("resolved runtime metadata = %#v", resolved)
	}
}

func TestResolveEvidenceRejectsUnlinkedRuntimeAttempt(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Do not leak runtime evidence", ProjectKey: "018-HAI"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	launchID := uuid.New()
	repo.launchEvents = append(repo.launchEvents, models.AutomationLaunchEvent{
		ID:          launchID,
		RuntimeType: "openclaw",
		LaunchType:  "agent_runtime",
		Status:      "blocked",
		StartedAt:   time.Now().UTC(),
		CompletedAt: time.Now().UTC(),
	})

	if _, err := service.ResolveEvidence(created.ID, "automation-launch://"+launchID.String()); err == nil {
		t.Fatalf("ResolveEvidence succeeded for unlinked runtime evidence")
	}
}

func TestFailedRuntimeAttemptCreatesPursuitBlockerAndRobertDecision(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Recover OpenClaw runtime", ProjectKey: "018-HAI"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	launchID := uuid.New()
	repo.launchEvents = append(repo.launchEvents, models.AutomationLaunchEvent{
		ID:          launchID,
		RuntimeType: "openclaw",
		LaunchType:  "agent_runtime",
		Status:      "blocked",
		Message:     "agent runtime registry is not configured",
		RuntimeRouteTrace: &models.AutomationRuntimeRouteTrace{
			RuntimeID:         "openclaw",
			Intent:            "software engineering and repository workflow",
			ExecutionMode:     "read-only planning plus approved low-risk local actions",
			RiskLevel:         "high",
			RecommendedSkills: []string{"autoreview", "gitcrawl"},
			VisibleTools:      []string{"browser"},
			BlockedSurfaces:   []string{"outbound message sending"},
		},
		ExitCode:    -1,
		StartedAt:   time.Now().UTC(),
		CompletedAt: time.Now().UTC(),
	})
	_, _ = service.Link(created.ID, LinkRequest{
		LinkType:     LinkAgentRuntime,
		LinkID:       launchID.String(),
		Relationship: "execution_attempt",
		SourceURI:    "automation-launch://" + launchID.String(),
		SourceLabel:  "OpenClaw blocked launch",
	})

	detail, err := service.Detail(created.ID)
	if err != nil {
		t.Fatalf("Detail returned error: %v", err)
	}
	if detail.Summary.Blocked == 0 || len(detail.Blockers) == 0 {
		t.Fatalf("failed runtime attempt was not surfaced as blocker: summary=%#v blockers=%#v", detail.Summary, detail.Blockers)
	}
	if detail.Summary.NeedsRobert == 0 || len(detail.ActionQueues.NeedsRobert) == 0 {
		t.Fatalf("failed runtime attempt did not require Robert: summary=%#v queues=%#v", detail.Summary, detail.ActionQueues)
	}
	foundDecision := false
	for _, decision := range detail.DecisionQueue {
		if decision.DecisionType == "runtime_attempt_review" {
			foundDecision = true
			if !decision.RequiresApproval || decision.YesLabel != "Create recovery workflow" || decision.NoLabel != "Keep blocked" {
				t.Fatalf("runtime review decision invalid: %#v", decision)
			}
			if decision.EvidenceURI != "automation-launch://"+launchID.String() {
				t.Fatalf("runtime decision evidence = %q", decision.EvidenceURI)
			}
			if decision.RiskLevel != "high" || !strings.Contains(decision.Reason, "skills=autoreview, gitcrawl") || !strings.Contains(decision.Reason, "blocked=outbound message sending") {
				t.Fatalf("runtime route trace missing from decision: %#v", decision)
			}
		}
	}
	if !foundDecision {
		t.Fatalf("runtime review decision missing: %#v", detail.DecisionQueue)
	}
}

func TestResolvedRuntimeAttemptDecisionStaysBlockedWithoutRepromptingRobert(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Keep failed OpenClaw runtime blocked", ProjectKey: "018-HAI"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	launchID := uuid.New()
	repo.launchEvents = append(repo.launchEvents, models.AutomationLaunchEvent{
		ID:          launchID,
		RuntimeType: "openclaw",
		LaunchType:  "agent_runtime",
		Status:      "blocked",
		Message:     "agent runtime registry is not configured",
		ExitCode:    -1,
		StartedAt:   time.Now().UTC(),
		CompletedAt: time.Now().UTC(),
	})
	_, _ = service.Link(created.ID, LinkRequest{
		LinkType:     LinkAgentRuntime,
		LinkID:       launchID.String(),
		Relationship: "execution_attempt",
		SourceURI:    "automation-launch://" + launchID.String(),
		SourceLabel:  "OpenClaw blocked launch",
	})
	decisionID := "runtime:" + launchID.String() + ":review"

	detail, err := service.ResolveDecision(created.ID, DecisionResolutionRequest{
		DecisionID:    decisionID,
		DecisionType:  "runtime_attempt_review",
		Approved:      false,
		Reason:        "blocked: agent runtime registry is not configured",
		Note:          "Keep blocked until OpenClaw is configured safely.",
		EvidenceURI:   "automation-launch://" + launchID.String(),
		EvidenceLabel: "OpenClaw blocked launch",
		Actor:         "Robert",
	})
	if err != nil {
		t.Fatalf("ResolveDecision returned error: %v", err)
	}
	if detail.Summary.Blocked == 0 || len(detail.Blockers) == 0 {
		t.Fatalf("resolved runtime attempt should remain visible as blocker: summary=%#v blockers=%#v", detail.Summary, detail.Blockers)
	}
	if detail.Summary.NeedsRobert != 0 {
		t.Fatalf("resolved runtime decision should not keep Robert in queue: summary=%#v", detail.Summary)
	}
	for _, decision := range detail.DecisionQueue {
		if decision.ID == decisionID {
			t.Fatalf("resolved runtime decision was regenerated: %#v", detail.DecisionQueue)
		}
	}
	if len(detail.ActionQueues.NeedsRobert) != 0 {
		t.Fatalf("resolved runtime blocker still routed to Robert: %#v", detail.ActionQueues)
	}
}

func TestApprovedRuntimeAttemptDecisionCreatesGovernedRecoveryWorkflow(t *testing.T) {
	repo := newFakeRepo()
	workflowService := &fakeWorkflowIntake{repo: repo}
	service := NewService(repo, workflowService)
	created, err := service.Create(CreateRequest{Title: "Recover OpenClaw runtime safely", OwnerIdentity: "alice", ProjectKey: "018-HAI"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	launchID := uuid.New()
	sourceURI := "automation-launch://" + launchID.String()
	repo.launchEvents = append(repo.launchEvents, models.AutomationLaunchEvent{
		ID:          launchID,
		RuntimeType: "openclaw",
		LaunchType:  "agent_runtime",
		Status:      "blocked",
		Message:     "high-risk surfaces detected",
		RuntimeRouteTrace: &models.AutomationRuntimeRouteTrace{
			RuntimeID:           "openclaw",
			Intent:              "software engineering and repository workflow",
			ExecutionMode:       "read_only",
			RiskLevel:           "high",
			RecommendedSkills:   []string{"autoreview", "gitcrawl"},
			VisibleTools:        []string{"browser", "host tools"},
			BlockedSurfaces:     []string{"outbound message sending", "host tools"},
			RequiredControls:    []string{"server-side HAI approval", "AGENT_RUNTIME_WORKSPACE_ROOT"},
			ValidationChecklist: []string{"verify controlled output", "store runtime evidence"},
		},
		ExitCode:    -1,
		StartedAt:   time.Now().UTC(),
		CompletedAt: time.Now().UTC(),
	})
	_, _ = service.Link(created.ID, LinkRequest{
		LinkType:     LinkAgentRuntime,
		LinkID:       launchID.String(),
		Relationship: "execution_attempt",
		SourceURI:    sourceURI,
		SourceLabel:  "OpenClaw blocked launch",
	})
	decisionID := "runtime:" + launchID.String() + ":review"

	detail, err := service.ResolveDecision(created.ID, DecisionResolutionRequest{
		DecisionID:    decisionID,
		DecisionType:  "runtime_attempt_review",
		Approved:      true,
		Reason:        "blocked: high-risk surfaces detected",
		Note:          "Create a safe recovery path first.",
		EvidenceURI:   sourceURI,
		EvidenceLabel: "OpenClaw blocked launch",
		Actor:         "Robert",
	})
	if err != nil {
		t.Fatalf("ResolveDecision returned error: %v", err)
	}
	if workflowService.calls != 1 {
		t.Fatalf("workflow intake calls = %d, want 1", workflowService.calls)
	}
	if workflowService.received.SourceURI != sourceURI || workflowService.received.SourceID != decisionID {
		t.Fatalf("workflow provenance = %#v", workflowService.received)
	}
	if workflowService.received.ContentType != "runtime_attempt_review" || workflowService.received.Trigger != "pursuit_decision_approved" || !workflowService.received.RequiresReview {
		t.Fatalf("workflow recovery metadata = %#v", workflowService.received)
	}
	if workflowService.received.OwnerIdentity != "alice" {
		t.Fatalf("recovery workflow owner = %q, want persisted pursuit owner alice", workflowService.received.OwnerIdentity)
	}
	for _, expected := range []string{"Route trace:", "skills=autoreview, gitcrawl", "Required controls:", "Validation checklist:", "Do not retry the runtime directly"} {
		if !strings.Contains(workflowService.received.Input, expected) {
			t.Fatalf("recovery workflow input missing %q:\n%s", expected, workflowService.received.Input)
		}
	}
	foundRecoveryWorkflow := false
	for _, link := range detail.Links {
		if link.LinkType == LinkWorkflow && link.Relationship == "runtime_recovery_workflow" && link.SourceURI == sourceURI {
			foundRecoveryWorkflow = true
		}
	}
	if !foundRecoveryWorkflow {
		t.Fatalf("runtime recovery workflow link missing: %#v", detail.Links)
	}
	if len(detail.Workflows) != 1 || detail.Workflows[0].RequiresApproval != true {
		t.Fatalf("recovery workflow not visible as approval-gated work: %#v", detail.Workflows)
	}
	if len(detail.ApprovalItems) != 1 || detail.Summary.NeedsRobert == 0 {
		t.Fatalf("recovery workflow should route back to Robert approval: summary=%#v approvals=%#v", detail.Summary, detail.ApprovalItems)
	}
	for _, decision := range detail.DecisionQueue {
		if decision.ID == decisionID {
			t.Fatalf("approved runtime decision was regenerated: %#v", detail.DecisionQueue)
		}
	}
}

func TestApprovedRuntimeDecisionRemainsPendingWhenRecoveryWorkflowFails(t *testing.T) {
	repo := newFakeRepo()
	workflowService := &fakeWorkflowIntake{err: errNotFound("workflow intake")}
	service := NewService(repo, workflowService)
	created, err := service.Create(CreateRequest{Title: "Keep failed runtime recovery actionable", ProjectKey: "018-HAI"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	launchID := uuid.New()
	sourceURI := "automation-launch://" + launchID.String()
	repo.launchEvents = append(repo.launchEvents, models.AutomationLaunchEvent{
		ID:          launchID,
		RuntimeType: "openclaw",
		LaunchType:  "agent_runtime",
		Status:      "blocked",
		Message:     "agent runtime registry is not configured",
		ExitCode:    -1,
		StartedAt:   time.Now().UTC(),
		CompletedAt: time.Now().UTC(),
	})
	if _, err := service.Link(created.ID, LinkRequest{
		LinkType:     LinkAgentRuntime,
		LinkID:       launchID.String(),
		Relationship: "execution_attempt",
		SourceURI:    sourceURI,
		SourceLabel:  "OpenClaw blocked launch",
	}); err != nil {
		t.Fatalf("Link returned error: %v", err)
	}
	decisionID := "runtime:" + launchID.String() + ":review"

	_, err = service.ResolveDecision(created.ID, DecisionResolutionRequest{
		DecisionID:   decisionID,
		DecisionType: "runtime_attempt_review",
		Approved:     true,
		EvidenceURI:  sourceURI,
		Actor:        "Robert",
	})
	if err == nil || !strings.Contains(err.Error(), "workflow intake not found") {
		t.Fatalf("ResolveDecision error = %v, want recovery workflow failure", err)
	}
	if workflowService.calls != 1 {
		t.Fatalf("workflow intake calls = %d, want 1", workflowService.calls)
	}

	detail, err := service.Detail(created.ID)
	if err != nil {
		t.Fatalf("Detail returned error: %v", err)
	}
	if detail.Summary.NeedsRobert == 0 || detail.Summary.Blocked == 0 {
		t.Fatalf("failed recovery decision was removed from operator queues: summary=%#v", detail.Summary)
	}
	for _, decision := range detail.DecisionQueue {
		if decision.ID == decisionID && decision.Status == "pending" {
			return
		}
	}
	t.Fatalf("failed recovery decision was incorrectly resolved: %#v", detail.DecisionQueue)
}

func TestRecoveredRuntimeAttemptStopsBlockingPursuit(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Recover OpenClaw through governed workflow", ProjectKey: "018-HAI"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	launchID := uuid.New()
	workflowID := uuid.New()
	sourceURI := "automation-launch://" + launchID.String()
	repo.launchEvents = append(repo.launchEvents, models.AutomationLaunchEvent{
		ID:          launchID,
		RuntimeType: "openclaw",
		LaunchType:  "agent_runtime",
		Status:      "blocked",
		Message:     "agent runtime registry is not configured",
		ExitCode:    -1,
		StartedAt:   time.Now().UTC(),
		CompletedAt: time.Now().UTC(),
	})
	repo.workflows[workflowID] = models.WorkflowItem{
		ID:                 workflowID,
		Title:              "Recover OpenClaw runtime configuration",
		ProjectKey:         "018-HAI",
		CurrentState:       workflow.StateCompleted,
		SourceType:         "pursuit_decision",
		SourceURI:          sourceURI,
		SourceLabel:        "OpenClaw blocked launch",
		VerificationStatus: "verified",
		RecoveryStatus:     workflow.RecoveryCompletedAfterRetry,
	}
	_, _ = service.Link(created.ID, LinkRequest{
		LinkType:     LinkAgentRuntime,
		LinkID:       launchID.String(),
		Relationship: "execution_attempt",
		SourceURI:    sourceURI,
		SourceLabel:  "OpenClaw blocked launch",
	})
	_, _ = service.Link(created.ID, LinkRequest{
		LinkType:     LinkWorkflow,
		LinkID:       workflowID.String(),
		Relationship: "operational_work",
		SourceURI:    sourceURI,
		SourceLabel:  "OpenClaw recovery workflow",
	})

	detail, err := service.Detail(created.ID)
	if err != nil {
		t.Fatalf("Detail returned error: %v", err)
	}
	if len(detail.RuntimeAttempts) != 1 {
		t.Fatalf("runtime attempt evidence disappeared: %#v", detail.RuntimeAttempts)
	}
	if detail.Summary.Blocked != 0 || len(detail.Blockers) != 0 {
		t.Fatalf("recovered runtime attempt still blocks pursuit: summary=%#v blockers=%#v", detail.Summary, detail.Blockers)
	}
	if detail.Summary.LinkedEvidence != 1 {
		t.Fatalf("linked evidence = %d, want verified recovery workflow counted as evidence", detail.Summary.LinkedEvidence)
	}
	for _, decision := range detail.DecisionQueue {
		if decision.DecisionType == "runtime_attempt_review" {
			t.Fatalf("recovered runtime attempt still requests Robert decision: %#v", detail.DecisionQueue)
		}
	}
	for _, action := range detail.NextActions {
		if strings.Contains(strings.ToLower(action.Label), "runtime attempt") {
			t.Fatalf("recovered runtime attempt still appears as next action: %#v", detail.NextActions)
		}
	}
	if detail.Summary.CompletionCandidate != true {
		t.Fatalf("summary = %#v, want recovered runtime workflow to make pursuit completion candidate", detail.Summary)
	}
}

func TestUpdateAllowsVerifiedCompletionWithLinkedRuntimeAttemptEvidence(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Complete OpenClaw runtime validation", ProjectKey: "018-HAI"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	launchID := uuid.New()
	repo.launchEvents = append(repo.launchEvents, models.AutomationLaunchEvent{
		ID:          launchID,
		RuntimeType: "openclaw",
		LaunchType:  "agent_runtime",
		Status:      "completed",
		Message:     "OpenClaw completed the approved agent task",
		StartedAt:   time.Now().UTC(),
		CompletedAt: time.Now().UTC(),
	})
	_, _ = service.Link(created.ID, LinkRequest{
		LinkType:     LinkAgentRuntime,
		LinkID:       launchID.String(),
		Relationship: "completion_evidence",
		SourceURI:    "automation-launch://" + launchID.String(),
		SourceLabel:  "OpenClaw runtime launch",
	})

	updated, err := service.Update(created.ID, UpdateRequest{
		CompletionState: CompletionVerified,
		Actor:           "operator",
	})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if updated.CompletionState != CompletionVerified {
		t.Fatalf("completion state = %s, want verified", updated.CompletionState)
	}
}

func TestDashboardKeepsHighRiskPursuitInRobertQueue(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{
		Title:       "Legal reply to municipality",
		Description: "Government legal response with possible financial consequence.",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	dashboard, err := service.Dashboard()
	if err != nil {
		t.Fatalf("Dashboard returned error: %v", err)
	}
	if len(dashboard.NeedsRobert) != 1 || dashboard.NeedsRobert[0].Pursuit.ID != created.ID {
		t.Fatalf("high-risk pursuit was not surfaced for Robert: %#v", dashboard.NeedsRobert)
	}
	if len(dashboard.VAReady) != 0 || len(dashboard.SystemReady) != 0 {
		t.Fatalf("high-risk pursuit leaked into VA/system queues: va=%#v system=%#v", dashboard.VAReady, dashboard.SystemReady)
	}
	item := dashboard.NeedsRobert[0]
	if item.CurrentState == "" || item.WhatChanged == "" || item.DecisionCards == 0 || item.TimelineItems == 0 {
		t.Fatalf("dashboard item missing operational summary metadata: %#v", item)
	}
}

func TestDashboardAggregatesRobertDecisionCards(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{
		Title:                 "Approve OpenClaw recovery plan",
		ProjectKey:            "018-HAI",
		RiskLevel:             "high",
		AutonomyLevel:         "approve_before_execute",
		NextRecommendedAction: "Create a governed recovery workflow before retrying OpenClaw.",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	dashboard, err := service.Dashboard()
	if err != nil {
		t.Fatalf("Dashboard returned error: %v", err)
	}
	if dashboard.Counts["decisionQueue"] != 1 {
		t.Fatalf("decision queue count = %d, want 1", dashboard.Counts["decisionQueue"])
	}
	if len(dashboard.DecisionQueue) != 1 {
		t.Fatalf("decision queue = %#v, want one card", dashboard.DecisionQueue)
	}
	card := dashboard.DecisionQueue[0]
	if card.Pursuit.ID != created.ID {
		t.Fatalf("decision card pursuit = %s, want %s", card.Pursuit.ID, created.ID)
	}
	if card.Decision.DecisionType != "pursuit_next_action" || card.Decision.Status != "pending" {
		t.Fatalf("decision card = %#v, want pending pursuit next action", card.Decision)
	}
	if !card.Decision.RequiresApproval || card.Decision.YesLabel == "" || card.Decision.NoLabel == "" {
		t.Fatalf("decision card missing Yes/No approval framing: %#v", card.Decision)
	}
	if card.CurrentState == "" || card.NextAction == "" || card.EvidenceLine == "" {
		t.Fatalf("decision card missing dashboard context: %#v", card)
	}
}

func TestDelegationPackageCompilesBoundedVAWorkWithChecklistAndSources(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{
		Title:          "Organize insurance evidence",
		DesiredOutcome: "A complete, source-linked evidence bundle is ready for Robert to review.",
		RiskLevel:      "low",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	workflowID := uuid.New()
	repo.workflows[workflowID] = models.WorkflowItem{
		ID:           workflowID,
		Title:        "Collect and organize evidence",
		Description:  "Review the source material and organize the evidence list.",
		CurrentState: "waiting_external_input",
		RiskLevel:    "low",
		NextAction:   "Prepare the evidence list from the linked sources.",
	}
	repo.openLoops = append(repo.openLoops, models.WorkflowOpenLoop{
		ID:               uuid.New(),
		WorkflowID:       workflowID,
		ResponsibleParty: "VA",
		WaitingFor:       "evidence review",
		NextAction:       "Prepare the evidence list from the linked sources.",
		Status:           "open",
	})
	repo.checklistItems = append(repo.checklistItems, models.WorkflowChecklistItem{
		ID:         uuid.New(),
		WorkflowID: workflowID,
		Label:      "List each source with its date and relevance.",
		Status:     "pending",
		Position:   1,
	})
	repo.sourceLinks = append(repo.sourceLinks, models.WorkflowSourceLink{
		ID:           uuid.New(),
		WorkflowID:   workflowID,
		SourceType:   "email_export",
		SourceURI:    "file:///connected-sources/claim.mbox",
		SourceLabel:  "Claim correspondence export",
		Relationship: "evidence",
	})
	if _, err := service.Link(created.ID, LinkRequest{LinkType: LinkWorkflow, LinkID: workflowID.String(), Relationship: "operational_work"}); err != nil {
		t.Fatalf("Link workflow returned error: %v", err)
	}

	brief, err := service.DelegationPackage(created.ID)
	if err != nil {
		t.Fatalf("DelegationPackage returned error: %v", err)
	}
	if !brief.Ready || brief.Status != "ready" {
		t.Fatalf("delegation package not ready: %#v", brief)
	}
	if len(brief.WorkItems) != 1 || brief.WorkItems[0].WorkflowID != workflowID.String() || len(brief.WorkItems[0].Checklist) != 1 {
		t.Fatalf("delegation work items missing workflow/checklist: %#v", brief.WorkItems)
	}
	if len(brief.SourceContext) != 1 || brief.SourceContext[0].SourceURI != "file:///connected-sources/claim.mbox" {
		t.Fatalf("delegation source context missing: %#v", brief.SourceContext)
	}
	if len(brief.BlockedActions) == 0 || len(brief.DeliveryRequirements) == 0 {
		t.Fatalf("delegation safety boundaries missing: %#v", brief)
	}
}

func TestDelegationPackageDoesNotReleaseHighRiskWorkToVA(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Send legal position to municipality", RiskLevel: "high"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	workflowID := uuid.New()
	repo.workflows[workflowID] = models.WorkflowItem{ID: workflowID, Title: "Prepare legal response", CurrentState: "needs_approval", RiskLevel: "high", RequiresApproval: true, ApprovalStatus: "pending"}
	repo.openLoops = append(repo.openLoops, models.WorkflowOpenLoop{ID: uuid.New(), WorkflowID: workflowID, ResponsibleParty: "VA", NextAction: "Send the legal response to the municipality", Status: "open"})
	if _, err := service.Link(created.ID, LinkRequest{LinkType: LinkWorkflow, LinkID: workflowID.String(), Relationship: "operational_work"}); err != nil {
		t.Fatalf("Link workflow returned error: %v", err)
	}

	brief, err := service.DelegationPackage(created.ID)
	if err != nil {
		t.Fatalf("DelegationPackage returned error: %v", err)
	}
	if brief.Ready || brief.Status != "not_ready" || len(brief.OutstandingRobertActions) == 0 {
		t.Fatalf("high-risk work leaked into VA handoff: %#v", brief)
	}
}

func TestFollowUpActionRiskUsesWholeActionTerms(t *testing.T) {
	if got := followUpActionRisk("high", "Prepare the applicant profile for Robert to review"); got != "low" {
		t.Fatalf("preparation action risk = %q, want low", got)
	}
	if got := followUpActionRisk("low", "File the completed response with the municipality"); got != "high" {
		t.Fatalf("filing action risk = %q, want high", got)
	}
}

func TestDashboardSurfacesDuePursuitReview(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{
		Title:                 "Organize local notes",
		ProjectKey:            "ops",
		NextRecommendedAction: "Review stale notes and choose next step",
		NextReviewAt:          time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	dashboard, err := service.Dashboard()
	if err != nil {
		t.Fatalf("Dashboard returned error: %v", err)
	}
	if dashboard.Counts["reviewDue"] != 1 || len(dashboard.ReviewDue) != 1 {
		t.Fatalf("review-due pursuit was not surfaced: counts=%#v reviewDue=%#v", dashboard.Counts, dashboard.ReviewDue)
	}
	if dashboard.ReviewDue[0].Pursuit.ID != created.ID || !dashboard.ReviewDue[0].ReviewDue {
		t.Fatalf("review-due item metadata missing: %#v", dashboard.ReviewDue[0])
	}

	detail, err := service.Detail(created.ID)
	if err != nil {
		t.Fatalf("Detail returned error: %v", err)
	}
	if !detail.Summary.ReviewDue {
		t.Fatalf("detail summary did not mark review due: %#v", detail.Summary)
	}
	if len(detail.ActionQueues.SystemReady) == 0 {
		t.Fatalf("low-risk review-due pursuit should be system-ready: %#v", detail.ActionQueues)
	}
	if !strings.Contains(detail.ActionQueues.SystemReady[0].Reason, "scheduled pursuit review is due") {
		t.Fatalf("review action reason missing: %#v", detail.ActionQueues.SystemReady[0])
	}
}

func TestBriefSummarizesOperatingPrioritiesAndDeduplicatesCards(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	highRisk, err := service.Create(CreateRequest{
		Title:                 "Prepare legal reply",
		ProjectKey:            "vivare",
		RiskLevel:             "high",
		NextRecommendedAction: "Approve formal reply direction",
	})
	if err != nil {
		t.Fatalf("Create high-risk returned error: %v", err)
	}
	_, err = service.Create(CreateRequest{
		Title:        "Review source backlog",
		ProjectKey:   "hai",
		NextReviewAt: time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("Create review-due returned error: %v", err)
	}

	brief, err := service.Brief()
	if err != nil {
		t.Fatalf("Brief returned error: %v", err)
	}
	if brief.OperatingMode != "needs_robert" {
		t.Fatalf("operating mode = %q, want needs_robert", brief.OperatingMode)
	}
	if brief.NeedsRobert != 1 || brief.PlanningNeeded != 2 || brief.ReviewDue != 1 {
		t.Fatalf("brief counts = Robert %d planning %d review %d", brief.NeedsRobert, brief.PlanningNeeded, brief.ReviewDue)
	}
	if !strings.Contains(brief.PrimaryAction, "Robert-only") {
		t.Fatalf("primary action = %q, want Robert-first guidance", brief.PrimaryAction)
	}
	if len(brief.Cards) < 2 {
		t.Fatalf("brief cards too small: %#v", brief.Cards)
	}
	if brief.Cards[0].PursuitID != highRisk.ID.String() || brief.Cards[0].Queue != "Robert" {
		t.Fatalf("first card = %#v, want high-risk Robert card first", brief.Cards[0])
	}
	seen := map[string]bool{}
	for _, card := range brief.Cards {
		if seen[card.PursuitID] {
			t.Fatalf("brief card duplicated pursuit %s in %#v", card.PursuitID, brief.Cards)
		}
		seen[card.PursuitID] = true
	}
}

func TestReviewClearsDueReviewAndAuditsDecision(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{
		Title:        "Review insurance claim pursuit",
		NextReviewAt: time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	detail, err := service.Review(created.ID, ReviewRequest{Action: "complete", Actor: "robert"})
	if err != nil {
		t.Fatalf("Review returned error: %v", err)
	}
	if detail.Summary.ReviewDue {
		t.Fatalf("review should not remain due after completion: %#v", detail.Summary)
	}
	if detail.Pursuit.NextReviewAt == nil || !detail.Pursuit.NextReviewAt.After(time.Now().UTC()) {
		t.Fatalf("next review was not scheduled forward: %#v", detail.Pursuit.NextReviewAt)
	}

	dashboard, err := service.Dashboard()
	if err != nil {
		t.Fatalf("Dashboard returned error: %v", err)
	}
	if dashboard.Counts["reviewDue"] != 0 || len(dashboard.ReviewDue) != 0 {
		t.Fatalf("review-due queue did not clear: counts=%#v reviewDue=%#v", dashboard.Counts, dashboard.ReviewDue)
	}
	activity, err := service.Activity(created.ID)
	if err != nil {
		t.Fatalf("Activity returned error: %v", err)
	}
	if len(activity) == 0 || activity[0].EventType != "pursuit.reviewed" {
		t.Fatalf("review activity was not recorded first: %#v", activity)
	}
}

func TestDashboardSurfacesPlanningNeededPursuits(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{
		Title:      "Prepare business idea",
		ProjectKey: "growth",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	dashboard, err := service.Dashboard()
	if err != nil {
		t.Fatalf("Dashboard returned error: %v", err)
	}
	if dashboard.Counts["planningNeeded"] != 1 || len(dashboard.PlanningNeeded) != 1 {
		t.Fatalf("planning-needed pursuit was not surfaced: counts=%#v planning=%#v", dashboard.Counts, dashboard.PlanningNeeded)
	}
	if dashboard.PlanningNeeded[0].Pursuit.ID != created.ID || !dashboard.PlanningNeeded[0].PlanningNeeded {
		t.Fatalf("planning-needed metadata missing: %#v", dashboard.PlanningNeeded[0])
	}

	detail, err := service.Detail(created.ID)
	if err != nil {
		t.Fatalf("Detail returned error: %v", err)
	}
	if !detail.Summary.PlanningNeeded {
		t.Fatalf("detail summary did not mark planning needed: %#v", detail.Summary)
	}
	if !strings.Contains(detail.Summary.CurrentState, "No linked workflow exists yet") {
		t.Fatalf("summary did not explain missing workflow planning need: %s", detail.Summary.CurrentState)
	}
}

func TestDetailIncludesCompactSourceItemsWithoutRawContent(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "ASR claim evidence", ProjectKey: "asr"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	rawID := uuid.New()
	repo.sourceItems[rawID] = models.SourceRawItem{
		ID:         rawID,
		SourceID:   uuid.New(),
		ExternalID: "email-123",
		ProjectKey: "asr",
		ItemType:   "email",
		Title:      "ASR requested receipts",
		SourceURI:  "local://source/email-123",
		Content:    "full raw private email body should not be exposed through pursuit detail sourceItems",
		Metadata:   `{"from":"insurer@example.test"}`,
		FetchedAt:  time.Now().UTC(),
	}
	if _, err := service.Link(created.ID, LinkRequest{LinkType: LinkSourceItem, LinkID: rawID.String(), Relationship: "evidence"}); err != nil {
		t.Fatalf("Link returned error: %v", err)
	}

	detail, err := service.Detail(created.ID)
	if err != nil {
		t.Fatalf("Detail returned error: %v", err)
	}
	if len(detail.SourceItems) != 1 || detail.SourceItems[0].Title != "ASR requested receipts" {
		t.Fatalf("source items = %#v", detail.SourceItems)
	}
	if detail.Summary.LinkedEvidence != 1 {
		t.Fatalf("linked evidence count = %d", detail.Summary.LinkedEvidence)
	}
}

func TestDetailBlocksArchivedAndMissingSourceExtractionEvidence(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Review legal evidence provenance", ProjectKey: "vivare"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	activeID := uuid.New()
	archivedID := uuid.New()
	missingID := uuid.New()
	repo.extractions[activeID] = models.SourceExtraction{
		ID:          activeID,
		ProjectKey:  "vivare",
		Summary:     "Active evidence summary.",
		SourceLabel: "Active evidence",
	}
	repo.extractions[archivedID] = models.SourceExtraction{
		ID:          archivedID,
		ProjectKey:  "vivare",
		Summary:     "Archived evidence summary.",
		SourceLabel: "Archived evidence",
		Archived:    true,
	}
	for _, link := range []LinkRequest{
		{LinkType: LinkSourceExtraction, LinkID: activeID.String(), Relationship: "evidence", SourceLabel: "Active evidence"},
		{LinkType: LinkSourceExtraction, LinkID: archivedID.String(), Relationship: "evidence", SourceLabel: "Archived evidence"},
		{LinkType: LinkSourceExtraction, LinkID: missingID.String(), Relationship: "evidence", SourceLabel: "Missing evidence"},
	} {
		if _, err := service.Link(created.ID, link); err != nil {
			t.Fatalf("Link returned error: %v", err)
		}
	}

	detail, err := service.Detail(created.ID)
	if err != nil {
		t.Fatalf("Detail returned error: %v", err)
	}
	if detail.Summary.LinkedEvidence != 1 {
		t.Fatalf("linked evidence count = %d, want only active extraction counted", detail.Summary.LinkedEvidence)
	}
	if detail.Summary.Blocked != 2 || len(detail.Blockers) != 2 {
		t.Fatalf("blockers summary=%#v blockers=%#v, want archived and missing extraction blockers", detail.Summary, detail.Blockers)
	}
	if detail.Summary.NeedsRobert == 0 || len(detail.ActionQueues.NeedsRobert) == 0 {
		t.Fatalf("source provenance blockers did not route to Robert: summary=%#v queues=%#v", detail.Summary, detail.ActionQueues)
	}
	reasons := strings.Join([]string{detail.Blockers[0].Reason, detail.Blockers[1].Reason}, " ")
	if !strings.Contains(reasons, "archived") || !strings.Contains(reasons, "missing") {
		t.Fatalf("source retraction blockers missing reason detail: %#v", detail.Blockers)
	}
	if !strings.Contains(detail.Summary.CurrentState, "source evidence was archived or is missing") {
		t.Fatalf("summary did not mention stale source evidence: %q", detail.Summary.CurrentState)
	}
}

func TestDeleteLinkRequiresOwningPursuit(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	first, _ := service.Create(CreateRequest{Title: "First pursuit"})
	second, _ := service.Create(CreateRequest{Title: "Second pursuit"})
	link, err := service.Link(first.ID, LinkRequest{LinkType: LinkMemory, LinkID: uuid.New().String()})
	if err != nil {
		t.Fatalf("Link returned error: %v", err)
	}

	if err := service.DeleteLink(second.ID, link.ID, "test-operator"); err == nil {
		t.Fatalf("expected deleting another pursuit's link to fail")
	}
	links, _ := repo.FindLinks(first.ID)
	if len(links) != 1 {
		t.Fatalf("link was removed from wrong pursuit: %#v", links)
	}
	if err := service.DeleteLink(first.ID, link.ID, "test-operator"); err != nil {
		t.Fatalf("DeleteLink returned error for owner: %v", err)
	}
	links, _ = repo.FindLinks(first.ID)
	if len(links) != 0 {
		t.Fatalf("owned link still present: %#v", links)
	}
	activities, _ := repo.FindActivities(first.ID, 20)
	for _, activity := range activities {
		if activity.EventType == "pursuit.link_removed" {
			if activity.Actor != "test-operator" {
				t.Fatalf("link removal actor = %q, want verified actor", activity.Actor)
			}
			return
		}
	}
	t.Fatal("expected pursuit.link_removed activity")
}

func TestLinkActivityRefreshesPursuitFreshness(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Refresh pursuit activity"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	staleAt := time.Now().UTC().Add(-15 * 24 * time.Hour)
	record := repo.pursuits[created.ID]
	record.LastActivityAt = &staleAt
	repo.pursuits[created.ID] = record

	if _, err := service.Link(created.ID, LinkRequest{
		LinkType:     LinkMemory,
		LinkID:       uuid.New().String(),
		Relationship: "context_memory",
		SourceURI:    "memory://project-context",
		Actor:        "operator",
	}); err != nil {
		t.Fatalf("Link returned error: %v", err)
	}

	updated, err := repo.FindByID(created.ID)
	if err != nil {
		t.Fatalf("FindByID returned error: %v", err)
	}
	if updated.LastActivityAt == nil || !updated.LastActivityAt.After(staleAt) {
		t.Fatalf("last activity = %v, want time after %v", updated.LastActivityAt, staleAt)
	}
	if isStale(*updated) {
		t.Fatalf("pursuit remained stale after a recorded link activity: %#v", updated)
	}
}

func TestSummaryRefreshDoesNotMaskStalePursuit(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	created, err := service.Create(CreateRequest{Title: "Keep stale pursuit visible"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	staleAt := time.Now().UTC().Add(-15 * 24 * time.Hour)
	record := repo.pursuits[created.ID]
	record.LastActivityAt = &staleAt
	repo.pursuits[created.ID] = record

	if _, err := service.RefreshSummary(created.ID, "system"); err != nil {
		t.Fatalf("RefreshSummary returned error: %v", err)
	}
	updated, err := repo.FindByID(created.ID)
	if err != nil {
		t.Fatalf("FindByID returned error: %v", err)
	}
	if updated.LastActivityAt == nil || !updated.LastActivityAt.Equal(staleAt) {
		t.Fatalf("summary refresh changed last activity to %v, want %v", updated.LastActivityAt, staleAt)
	}
	if !isStale(*updated) {
		t.Fatalf("summary refresh hid a stale pursuit: %#v", updated)
	}
	activity, err := repo.FindActivities(created.ID, 20)
	if err != nil {
		t.Fatalf("FindActivities returned error: %v", err)
	}
	if !activityContains(activity, "pursuit.summary_refreshed") {
		t.Fatalf("summary refresh was not retained in the audit feed: %#v", activity)
	}
}

func timelineContains(items []PursuitTimelineItem, kind, messagePart string) bool {
	for _, item := range items {
		if item.Kind == kind && strings.Contains(item.Message, messagePart) {
			return true
		}
	}
	return false
}

func timelineSourceContains(items []PursuitTimelineItem, kind, sourcePart string) bool {
	for _, item := range items {
		if item.Kind == kind && strings.Contains(item.SourceURI, sourcePart) {
			return true
		}
	}
	return false
}

type fakeRepo struct {
	pursuits             map[uuid.UUID]models.Pursuit
	links                map[uuid.UUID]models.PursuitLink
	activity             map[uuid.UUID][]models.PursuitActivity
	workflows            map[uuid.UUID]models.WorkflowItem
	checklistItems       []models.WorkflowChecklistItem
	openLoops            []models.WorkflowOpenLoop
	proposals            []models.WorkflowProposal
	qualityGates         []models.WorkflowQualityGate
	decisions            []models.WorkflowDecision
	transitions          []models.WorkflowTransition
	sourceLinks          []models.WorkflowSourceLink
	events               []models.WorkflowEvent
	evidence             []models.WorkflowEvidenceClaim
	memories             map[uuid.UUID]models.ContextMemory
	automations          map[uuid.UUID]models.Automation
	launchEvents         []models.AutomationLaunchEvent
	verificationRuns     map[uuid.UUID]models.VerificationRun
	verificationClaims   map[uuid.UUID]models.VerificationClaim
	verificationEvidence map[uuid.UUID]models.VerificationEvidence
	sourceItems          map[uuid.UUID]models.SourceRawItem
	extractions          map[uuid.UUID]models.SourceExtraction
	sourceOwners         map[uuid.UUID]string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		pursuits:             map[uuid.UUID]models.Pursuit{},
		links:                map[uuid.UUID]models.PursuitLink{},
		activity:             map[uuid.UUID][]models.PursuitActivity{},
		workflows:            map[uuid.UUID]models.WorkflowItem{},
		memories:             map[uuid.UUID]models.ContextMemory{},
		automations:          map[uuid.UUID]models.Automation{},
		verificationRuns:     map[uuid.UUID]models.VerificationRun{},
		verificationClaims:   map[uuid.UUID]models.VerificationClaim{},
		verificationEvidence: map[uuid.UUID]models.VerificationEvidence{},
		sourceItems:          map[uuid.UUID]models.SourceRawItem{},
		extractions:          map[uuid.UUID]models.SourceExtraction{},
		sourceOwners:         map[uuid.UUID]string{},
	}
}

func (r *fakeRepo) Create(pursuit *models.Pursuit) (*models.Pursuit, error) {
	if pursuit.ID == uuid.Nil {
		pursuit.ID = uuid.New()
	}
	now := time.Now().UTC()
	pursuit.CreatedAt = now
	pursuit.UpdatedAt = now
	r.pursuits[pursuit.ID] = *pursuit
	return pursuit, nil
}

func (r *fakeRepo) Update(pursuit *models.Pursuit) (*models.Pursuit, error) {
	pursuit.UpdatedAt = time.Now().UTC()
	r.pursuits[pursuit.ID] = *pursuit
	return pursuit, nil
}

func (r *fakeRepo) FindByID(id uuid.UUID) (*models.Pursuit, error) {
	item, ok := r.pursuits[id]
	if !ok {
		return nil, errNotFound("pursuit")
	}
	return &item, nil
}

func (r *fakeRepo) FindAll(includeArchived bool) ([]models.Pursuit, error) {
	result := []models.Pursuit{}
	for _, item := range r.pursuits {
		if !includeArchived && item.Archived {
			continue
		}
		result = append(result, item)
	}
	return result, nil
}

func (r *fakeRepo) CreateLink(link *models.PursuitLink) (*models.PursuitLink, error) {
	for _, existing := range r.links {
		if existing.PursuitID == link.PursuitID && existing.LinkType == link.LinkType && existing.LinkID == link.LinkID && existing.Relationship == link.Relationship {
			copyExisting := existing
			return &copyExisting, nil
		}
	}
	if link.ID == uuid.Nil {
		link.ID = uuid.New()
	}
	link.CreatedAt = time.Now().UTC()
	r.links[link.ID] = *link
	if link.LinkType == LinkWorkflow {
		if id, err := uuid.Parse(link.LinkID); err == nil {
			if _, ok := r.workflows[id]; !ok {
				r.workflows[id] = models.WorkflowItem{ID: id, Title: "Generated workflow", CurrentState: workflow.StateNeedsApproval, RequiresApproval: true, ApprovalStatus: "pending"}
			}
		}
	}
	return link, nil
}

func (r *fakeRepo) LinkVisibleToOwner(ownerIdentity, linkType, linkID string) (bool, bool, error) {
	switch linkType {
	case LinkWorkflow:
		id, err := uuid.Parse(linkID)
		if err != nil {
			return true, false, nil
		}
		item, ok := r.workflows[id]
		return true, ok && (item.OwnerIdentity == "" || item.OwnerIdentity == ownerIdentity), nil
	case LinkMemory:
		id, err := uuid.Parse(linkID)
		if err != nil {
			return true, false, nil
		}
		item, ok := r.memories[id]
		return true, ok && (item.OwnerIdentity == "" || item.OwnerIdentity == ownerIdentity), nil
	case LinkSourceItem:
		for id, item := range r.sourceItems {
			if linkID != id.String() && linkID != item.ExternalID {
				continue
			}
			owner := r.sourceOwners[item.SourceID]
			return true, owner == "" || owner == ownerIdentity, nil
		}
		return true, false, nil
	case LinkSourceExtraction:
		id, err := uuid.Parse(linkID)
		if err != nil {
			return true, false, nil
		}
		item, ok := r.extractions[id]
		if !ok {
			return true, false, nil
		}
		owner := r.sourceOwners[item.SourceID]
		return true, owner == "" || owner == ownerIdentity, nil
	case LinkVerification:
		id, err := uuid.Parse(linkID)
		if err != nil {
			return true, false, nil
		}
		item, ok := r.verificationRuns[id]
		return true, ok && (item.OwnerIdentity == "" || item.OwnerIdentity == ownerIdentity), nil
	default:
		return false, true, nil
	}
}

func (r *fakeRepo) DeleteLink(pursuitID uuid.UUID, id uuid.UUID) error {
	link, ok := r.links[id]
	if !ok || link.PursuitID != pursuitID {
		return errNotFound("link")
	}
	delete(r.links, id)
	return nil
}

func (r *fakeRepo) FindLinks(pursuitID uuid.UUID) ([]models.PursuitLink, error) {
	result := []models.PursuitLink{}
	for _, link := range r.links {
		if link.PursuitID == pursuitID {
			result = append(result, link)
		}
	}
	return result, nil
}

func (r *fakeRepo) FindLink(linkType, linkID string) (*models.PursuitLink, error) {
	for _, link := range r.links {
		if link.LinkType == linkType && link.LinkID == linkID {
			copyLink := link
			return &copyLink, nil
		}
	}
	return nil, errNotFound("link")
}

func (r *fakeRepo) FindLinkBySourceURI(sourceURI string) (*models.PursuitLink, error) {
	for _, link := range r.links {
		if link.SourceURI == sourceURI {
			copyLink := link
			return &copyLink, nil
		}
	}
	return nil, errNotFound("link")
}

func (r *fakeRepo) FindLinkForOwner(ownerIdentity, linkType, linkID string) (*models.PursuitLink, error) {
	return r.findVisibleLink(ownerIdentity, func(link models.PursuitLink) bool {
		return link.LinkType == linkType && link.LinkID == linkID
	})
}

func (r *fakeRepo) FindLinkBySourceURIForOwner(ownerIdentity, sourceURI string) (*models.PursuitLink, error) {
	return r.findVisibleLink(ownerIdentity, func(link models.PursuitLink) bool {
		return link.SourceURI == sourceURI
	})
}

func (r *fakeRepo) findVisibleLink(ownerIdentity string, matches func(models.PursuitLink) bool) (*models.PursuitLink, error) {
	var best *models.PursuitLink
	for _, link := range r.links {
		pursuit, ok := r.pursuits[link.PursuitID]
		if !ok || !pursuitVisibleTo(pursuit, ownerIdentity) || !matches(link) {
			continue
		}
		if best == nil || link.Confidence > best.Confidence || (link.Confidence == best.Confidence && link.CreatedAt.After(best.CreatedAt)) {
			copyLink := link
			best = &copyLink
		}
	}
	if best == nil {
		return nil, errNotFound("link")
	}
	return best, nil
}

func (r *fakeRepo) CreateActivity(activity *models.PursuitActivity) (*models.PursuitActivity, error) {
	if activity.ID == uuid.Nil {
		activity.ID = uuid.New()
	}
	if activity.CreatedAt.IsZero() {
		activity.CreatedAt = time.Now().UTC()
	}
	r.activity[activity.PursuitID] = append([]models.PursuitActivity{*activity}, r.activity[activity.PursuitID]...)
	return activity, nil
}

func (r *fakeRepo) FindActivities(pursuitID uuid.UUID, limit int) ([]models.PursuitActivity, error) {
	result := append([]models.PursuitActivity{}, r.activity[pursuitID]...)
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (r *fakeRepo) FindLinkedWorkflows(ids []uuid.UUID) ([]models.WorkflowItem, error) {
	result := []models.WorkflowItem{}
	for _, id := range ids {
		if item, ok := r.workflows[id]; ok {
			result = append(result, item)
		}
	}
	return result, nil
}

func (r *fakeRepo) FindLinkedChecklistItems(workflowIDs []uuid.UUID) ([]models.WorkflowChecklistItem, error) {
	workflowSet := map[uuid.UUID]bool{}
	for _, id := range workflowIDs {
		workflowSet[id] = true
	}
	result := []models.WorkflowChecklistItem{}
	for _, item := range r.checklistItems {
		if workflowSet[item.WorkflowID] {
			result = append(result, item)
		}
	}
	return result, nil
}

func (r *fakeRepo) FindLinkedOpenLoops(workflowIDs []uuid.UUID) ([]models.WorkflowOpenLoop, error) {
	workflowSet := map[uuid.UUID]bool{}
	for _, id := range workflowIDs {
		workflowSet[id] = true
	}
	result := []models.WorkflowOpenLoop{}
	for _, loop := range r.openLoops {
		if workflowSet[loop.WorkflowID] {
			result = append(result, loop)
		}
	}
	return result, nil
}

func (r *fakeRepo) FindLinkedProposals(workflowIDs []uuid.UUID) ([]models.WorkflowProposal, error) {
	workflowSet := map[uuid.UUID]bool{}
	for _, id := range workflowIDs {
		workflowSet[id] = true
	}
	result := []models.WorkflowProposal{}
	for _, proposal := range r.proposals {
		if workflowSet[proposal.WorkflowID] {
			result = append(result, proposal)
		}
	}
	return result, nil
}

func (r *fakeRepo) FindLinkedQualityGates(workflowIDs []uuid.UUID) ([]models.WorkflowQualityGate, error) {
	workflowSet := map[uuid.UUID]bool{}
	for _, id := range workflowIDs {
		workflowSet[id] = true
	}
	result := []models.WorkflowQualityGate{}
	for _, gate := range r.qualityGates {
		if workflowSet[gate.WorkflowID] {
			result = append(result, gate)
		}
	}
	return result, nil
}

func (r *fakeRepo) FindLinkedDecisions(workflowIDs []uuid.UUID) ([]models.WorkflowDecision, error) {
	workflowSet := map[uuid.UUID]bool{}
	for _, id := range workflowIDs {
		workflowSet[id] = true
	}
	result := []models.WorkflowDecision{}
	for _, decision := range r.decisions {
		if workflowSet[decision.WorkflowID] {
			result = append(result, decision)
		}
	}
	return result, nil
}

func (r *fakeRepo) FindLinkedTransitions(workflowIDs []uuid.UUID) ([]models.WorkflowTransition, error) {
	workflowSet := map[uuid.UUID]bool{}
	for _, id := range workflowIDs {
		workflowSet[id] = true
	}
	result := []models.WorkflowTransition{}
	for _, transition := range r.transitions {
		if workflowSet[transition.WorkflowID] {
			result = append(result, transition)
		}
	}
	return result, nil
}

func (r *fakeRepo) FindLinkedSourceLinks(workflowIDs []uuid.UUID) ([]models.WorkflowSourceLink, error) {
	workflowSet := map[uuid.UUID]bool{}
	for _, id := range workflowIDs {
		workflowSet[id] = true
	}
	result := []models.WorkflowSourceLink{}
	for _, link := range r.sourceLinks {
		if workflowSet[link.WorkflowID] {
			result = append(result, link)
		}
	}
	return result, nil
}

func (r *fakeRepo) FindLinkedEvents(workflowIDs []uuid.UUID) ([]models.WorkflowEvent, error) {
	workflowSet := map[uuid.UUID]bool{}
	for _, id := range workflowIDs {
		workflowSet[id] = true
	}
	result := []models.WorkflowEvent{}
	for _, event := range r.events {
		if workflowSet[event.WorkflowID] {
			result = append(result, event)
		}
	}
	return result, nil
}

func (r *fakeRepo) FindLinkedEvidence(workflowIDs []uuid.UUID) ([]models.WorkflowEvidenceClaim, error) {
	return append([]models.WorkflowEvidenceClaim{}, r.evidence...), nil
}

func (r *fakeRepo) FindLinkedMemories(ids []uuid.UUID) ([]models.ContextMemory, error) {
	result := []models.ContextMemory{}
	for _, id := range ids {
		if item, ok := r.memories[id]; ok {
			result = append(result, item)
		}
	}
	return result, nil
}

func (r *fakeRepo) FindLinkedSourceItems(ids []uuid.UUID) ([]models.SourceRawItem, error) {
	result := []models.SourceRawItem{}
	for _, id := range ids {
		if item, ok := r.sourceItems[id]; ok {
			result = append(result, item)
		}
	}
	return result, nil
}

func (r *fakeRepo) FindLinkedExtractions(ids []uuid.UUID) ([]models.SourceExtraction, error) {
	result := []models.SourceExtraction{}
	for _, id := range ids {
		if item, ok := r.extractions[id]; ok {
			result = append(result, item)
		}
	}
	return result, nil
}

func (r *fakeRepo) FindLinkedVerificationRuns(ids []uuid.UUID) ([]models.VerificationRun, error) {
	result := []models.VerificationRun{}
	for _, id := range ids {
		if item, ok := r.verificationRuns[id]; ok {
			result = append(result, item)
		}
	}
	return result, nil
}

func (r *fakeRepo) FindLinkedVerificationClaims(runIDs []uuid.UUID) ([]models.VerificationClaim, error) {
	runSet := map[uuid.UUID]bool{}
	for _, id := range runIDs {
		runSet[id] = true
	}
	result := []models.VerificationClaim{}
	for _, item := range r.verificationClaims {
		if runSet[item.RunID] {
			result = append(result, item)
		}
	}
	return result, nil
}

func (r *fakeRepo) FindLinkedVerificationEvidence(runIDs []uuid.UUID) ([]models.VerificationEvidence, error) {
	runSet := map[uuid.UUID]bool{}
	for _, id := range runIDs {
		runSet[id] = true
	}
	result := []models.VerificationEvidence{}
	for _, item := range r.verificationEvidence {
		if runSet[item.RunID] {
			result = append(result, item)
		}
	}
	return result, nil
}

func (r *fakeRepo) FindLinkedAutomations(ids []uuid.UUID) ([]models.Automation, error) {
	result := []models.Automation{}
	for _, id := range ids {
		if item, ok := r.automations[id]; ok {
			result = append(result, item)
		}
	}
	return result, nil
}

func (r *fakeRepo) FindLinkedAutomationLaunches(automationIDs []uuid.UUID, launchIDs []uuid.UUID, limit int) ([]models.AutomationLaunchEvent, error) {
	automationSet := map[uuid.UUID]bool{}
	for _, id := range automationIDs {
		automationSet[id] = true
	}
	launchSet := map[uuid.UUID]bool{}
	for _, id := range launchIDs {
		launchSet[id] = true
	}
	result := []models.AutomationLaunchEvent{}
	for _, event := range r.launchEvents {
		if automationSet[event.AutomationID] || launchSet[event.ID] {
			result = append(result, event)
		}
	}
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

type fakeWorkflowIntake struct {
	received workflow.IntakeRequest
	calls    int
	records  map[uuid.UUID]*workflow.WorkflowRecord
	repo     *fakeRepo
	err      error
}

func (f *fakeWorkflowIntake) Intake(request workflow.IntakeRequest) (*workflow.WorkflowRecord, error) {
	f.calls++
	f.received = request
	if f.err != nil {
		return nil, f.err
	}
	id := uuid.New()
	record := &workflow.WorkflowRecord{
		Item: models.WorkflowItem{
			ID:               id,
			OwnerIdentity:    request.OwnerIdentity,
			Title:            request.Input,
			ProjectKey:       request.ProjectKey,
			SourceType:       request.SourceType,
			SourceID:         request.SourceID,
			SourceURI:        request.SourceURI,
			CurrentState:     workflow.StateNeedsApproval,
			RiskLevel:        "high",
			RequiresApproval: true,
			ApprovalStatus:   "pending",
			ApprovalReason:   "high-risk pursuit intake",
			NextAction:       "Robert should approve the prepared response.",
		},
	}
	if f.records == nil {
		f.records = make(map[uuid.UUID]*workflow.WorkflowRecord)
	}
	f.records[id] = record
	if f.repo != nil {
		f.repo.workflows[id] = record.Item
	}
	return record, nil
}

func (f *fakeWorkflowIntake) Get(id uuid.UUID) (*workflow.WorkflowRecord, error) {
	record, ok := f.records[id]
	if !ok {
		return nil, errNotFound("workflow")
	}
	return record, nil
}

type errNotFound string

func (e errNotFound) Error() string {
	return string(e) + " not found"
}
