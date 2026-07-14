package ambient

import (
	"automation-hub-backend/internal/memoryengine"
	"automation-hub-backend/internal/models"
	pursuitpkg "automation-hub-backend/internal/pursuit"
	"automation-hub-backend/internal/workflow"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestOpportunityScoreRewardsNeedGapAndCapability(t *testing.T) {
	need := models.AmbientNeed{CurrentLevel: 20, TargetLevel: 90, PriorityWeight: 90}
	strong := models.AmbientOpportunity{Urgency: 80, Impact: 85, Effort: 20, Confidence: 90, Risk: 20}
	weak := models.AmbientOpportunity{Urgency: 30, Impact: 35, Effort: 80, Confidence: 40, Risk: 60}
	if opportunityScore(strong, need) <= opportunityScore(weak, need) {
		t.Fatalf("expected capable high-impact opportunity to rank higher")
	}
}

func TestFingerprintIsStableAndSourceScoped(t *testing.T) {
	first := fingerprint("workflow", "123", "growth")
	second := fingerprint("workflow", "123", "growth")
	other := fingerprint("workflow", "124", "growth")
	if first != second {
		t.Fatalf("expected stable fingerprint")
	}
	if first == other {
		t.Fatalf("expected source identity to change fingerprint")
	}
}

func TestNeedForWorkflowPrioritizesSafety(t *testing.T) {
	item := models.WorkflowItem{Title: "Reply to lawyer", RiskLevel: "high"}
	if got := needForWorkflow(item); got != "safety" {
		t.Fatalf("expected safety, got %s", got)
	}
}

func TestBuildCandidatesIncludesPursuitQueues(t *testing.T) {
	now := time.Now().UTC().Add(-20 * 24 * time.Hour)
	pursuitID := uuid.New()
	vaID := uuid.New()
	reviewID := uuid.New()
	planningID := uuid.New()
	highRiskID := uuid.New()
	pursuitDashboard := &pursuitpkg.Dashboard{
		NeedsRobert: []pursuitpkg.PursuitListItem{{
			Pursuit: models.Pursuit{
				ID:                    pursuitID,
				Title:                 "Vivare legal dispute",
				ProjectKey:            "vivare",
				RiskLevel:             "high",
				NeedCategory:          "safety_and_stability",
				PriorityScore:         94,
				Confidence:            0.82,
				NextRecommendedAction: "Approve or reject the prepared lawyer follow-up.",
				UpdatedAt:             now,
			},
			NeedsRobert: 1,
			NextAction:  "Review legal response proposal",
		}},
		VAReady: []pursuitpkg.PursuitListItem{{
			Pursuit: models.Pursuit{
				ID:            vaID,
				Title:         "Organize admin documents",
				RiskLevel:     "low",
				NeedCategory:  "esteem",
				PriorityScore: 60,
				Confidence:    0.76,
				UpdatedAt:     now,
			},
			NextAction: "Prepare document sorting checklist",
		}},
		ReviewDue: []pursuitpkg.PursuitListItem{{
			Pursuit: models.Pursuit{
				ID:                    reviewID,
				Title:                 "Review insurance claim evidence",
				RiskLevel:             "high",
				NeedCategory:          "safety_and_stability",
				PriorityScore:         76,
				Confidence:            0.74,
				NextRecommendedAction: "Check whether the evidence bundle still needs Robert.",
				UpdatedAt:             now,
			},
			ReviewDue:  true,
			NextAction: "Review evidence bundle status",
		}},
		PlanningNeeded: []pursuitpkg.PursuitListItem{{
			Pursuit: models.Pursuit{
				ID:            planningID,
				Title:         "Prepare business idea",
				RiskLevel:     "low",
				NeedCategory:  "growth",
				PriorityScore: 68,
				Confidence:    0.70,
				UpdatedAt:     now,
			},
			PlanningNeeded: true,
			NextAction:     "Create first workflow checklist",
		}},
		HighRisk: []pursuitpkg.PursuitListItem{{
			Pursuit: models.Pursuit{
				ID:                    highRiskID,
				Title:                 "Keep legal evidence boundaries current",
				RiskLevel:             "high",
				NeedCategory:          "safety_and_stability",
				PriorityScore:         84,
				Confidence:            0.77,
				NextRecommendedAction: "Check approval boundary before any external action.",
				UpdatedAt:             now,
			},
			NextAction: "Review approval boundary",
		}},
	}

	candidates := buildCandidates(&workflow.WorkflowDashboard{}, nil, &memoryengine.CommandDashboard{}, pursuitDashboard, testNeedMap())
	foundDecision := false
	foundVAReady := false
	foundReviewDue := false
	foundPlanning := false
	foundHighRisk := false
	for _, candidate := range candidates {
		switch candidate.SourceType {
		case "pursuit_decision":
			foundDecision = candidate.SourceID == pursuitID.String() && candidate.RequiresApproval && candidate.NeedKey == "safety"
		case "pursuit_va_ready":
			foundVAReady = candidate.SourceID == vaID.String() && !candidate.RequiresApproval && candidate.NeedKey == "esteem"
		case "pursuit_review_due":
			foundReviewDue = candidate.SourceID == reviewID.String() && candidate.RequiresApproval && candidate.NeedKey == "safety"
		case "pursuit_planning_needed":
			foundPlanning = candidate.SourceID == planningID.String() && !candidate.RequiresApproval && candidate.NeedKey == "growth"
		case "pursuit_high_risk":
			foundHighRisk = candidate.SourceID == highRiskID.String() && candidate.RequiresApproval && candidate.NeedKey == "safety"
		}
	}
	if !foundDecision || !foundVAReady || !foundReviewDue || !foundPlanning || !foundHighRisk {
		t.Fatalf("pursuit candidates missing: decision=%v va=%v reviewDue=%v planning=%v highRisk=%v candidates=%#v", foundDecision, foundVAReady, foundReviewDue, foundPlanning, foundHighRisk, candidates)
	}
}

func TestHighRiskPursuitCandidateSkipsAlreadyCoveredQueues(t *testing.T) {
	pursuitID := uuid.New()
	pursuitDashboard := &pursuitpkg.Dashboard{
		HighRisk: []pursuitpkg.PursuitListItem{{
			Pursuit: models.Pursuit{
				ID:            pursuitID,
				Title:         "Legal response already needs Robert",
				RiskLevel:     "high",
				NeedCategory:  "safety_and_stability",
				PriorityScore: 90,
				Confidence:    0.8,
				UpdatedAt:     time.Now().UTC(),
			},
			NeedsRobert: 1,
			NextAction:  "Approve legal response",
		}},
	}

	candidates := buildCandidates(&workflow.WorkflowDashboard{}, nil, &memoryengine.CommandDashboard{}, pursuitDashboard, testNeedMap())
	for _, candidate := range candidates {
		if candidate.SourceType == "pursuit_high_risk" && candidate.SourceID == pursuitID.String() {
			t.Fatalf("high-risk duplicate candidate was created for already-covered pursuit: %#v", candidate)
		}
	}
}

func TestAcceptPursuitOpportunityCreatesWorkflowAndLinksBack(t *testing.T) {
	pursuitID := uuid.New()
	workflowID := uuid.New()
	opportunity := &models.AmbientOpportunity{
		ID:               uuid.New(),
		Status:           StatusProposed,
		NeedKey:          "safety",
		Title:            "Robert decision needed: Vivare",
		Rationale:        "High-risk pursuit needs approval.",
		NextAction:       "Review the prepared legal response.",
		SourceType:       "pursuit_decision",
		SourceID:         pursuitID.String(),
		SourceURI:        "pursuit://" + pursuitID.String(),
		Confidence:       88,
		RequiresApproval: true,
	}
	workflowSpy := &ambientWorkflowSpy{recordID: workflowID}
	pursuitSpy := &ambientPursuitSpy{}
	engine := NewServiceWithPursuits(&ambientRepositoryStub{opportunity: opportunity}, workflowSpy, nil, pursuitSpy)

	accepted, err := engine.Accept(opportunity.ID, ResolutionRequest{Note: "Approve draft creation.", Actor: "verified-operator"})
	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if accepted.WorkflowID == nil || *accepted.WorkflowID != workflowID {
		t.Fatalf("accepted workflow id = %v, want %s", accepted.WorkflowID, workflowID)
	}
	if len(workflowSpy.intakeRequests) != 1 || !workflowSpy.intakeRequests[0].RequiresReview {
		t.Fatalf("workflow intake requests = %#v", workflowSpy.intakeRequests)
	}
	if workflowSpy.intakeRequests[0].Actor != "verified-operator" {
		t.Fatalf("workflow actor = %q, want verified operator", workflowSpy.intakeRequests[0].Actor)
	}
	if len(pursuitSpy.links) != 2 || pursuitSpy.linkedPursuitIDs[0] != pursuitID || pursuitSpy.linkedPursuitIDs[1] != pursuitID {
		t.Fatalf("pursuit links = %#v ids=%#v", pursuitSpy.links, pursuitSpy.linkedPursuitIDs)
	}
	link := pursuitSpy.links[0]
	if link.LinkType != pursuitpkg.LinkWorkflow || link.LinkID != workflowID.String() || link.Relationship != "ambient_follow_up" {
		t.Fatalf("link = %#v, want ambient workflow link", link)
	}
	proposalLink := pursuitSpy.links[1]
	if proposalLink.LinkType != pursuitpkg.LinkAmbientOpportunity || proposalLink.LinkID != opportunity.ID.String() || proposalLink.Relationship != "ambient_proposal_accepted" {
		t.Fatalf("proposal link = %#v, want accepted ambient proposal link", proposalLink)
	}
	if proposalLink.SourceURI != opportunity.SourceURI || proposalLink.SourceLabel != opportunity.Title {
		t.Fatalf("proposal provenance was not preserved: %#v", proposalLink)
	}
	if link.Actor != "verified-operator" || proposalLink.Actor != "verified-operator" {
		t.Fatalf("pursuit link actors were not attributed to verified operator: %#v %#v", link, proposalLink)
	}
}

func TestPursuitOpportunityOwnerCannotAcceptAnotherOwnersOpportunity(t *testing.T) {
	pursuitID := uuid.New()
	opportunity := &models.AmbientOpportunity{
		ID:         uuid.New(),
		Status:     StatusProposed,
		NeedKey:    "safety",
		Title:      "Private pursuit decision",
		Rationale:  "Requires review.",
		NextAction: "Review private evidence.",
		SourceType: "pursuit_decision",
		SourceID:   pursuitID.String(),
	}
	workflowSpy := &ambientWorkflowSpy{}
	pursuitSpy := &ambientPursuitSpy{owners: map[uuid.UUID]string{pursuitID: "bob"}}
	engine := NewServiceWithPursuits(&ambientRepositoryStub{opportunity: opportunity}, workflowSpy, nil, pursuitSpy)

	if _, err := engine.Accept(opportunity.ID, ResolutionRequest{OwnerIdentity: "alice"}); err == nil {
		t.Fatal("expected cross-owner pursuit opportunity acceptance to be rejected")
	}
	if len(workflowSpy.intakeRequests) != 0 {
		t.Fatalf("cross-owner acceptance created workflow work: %#v", workflowSpy.intakeRequests)
	}
}

func TestPursuitOpportunitiesAreFilteredForAuthenticatedOwner(t *testing.T) {
	aliceID := uuid.New()
	bobID := uuid.New()
	engine := NewServiceWithPursuits(nil, nil, nil, &ambientPursuitSpy{owners: map[uuid.UUID]string{aliceID: "alice", bobID: "bob"}}).(*service)
	visible := engine.visibleOpportunities("alice", []models.AmbientOpportunity{
		{ID: uuid.New(), OwnerIdentity: "alice", SourceType: "pursuit_stale", SourceID: aliceID.String()},
		{ID: uuid.New(), OwnerIdentity: "bob", SourceType: "pursuit_stale", SourceID: bobID.String()},
		{ID: uuid.New(), SourceType: "workflow", SourceID: "shared-workflow"},
	})
	if len(visible) != 1 || visible[0].SourceID != aliceID.String() {
		t.Fatalf("owner-visible opportunities = %#v", visible)
	}
}

func TestScanForOwnerBuildsPrivatePursuitProposalsOnly(t *testing.T) {
	pursuitID := uuid.New()
	repo := &ambientRepositoryStub{needs: defaultNeeds()}
	pursuits := &ambientPursuitSpy{
		dashboard: &pursuitpkg.Dashboard{
			ReviewDue: []pursuitpkg.PursuitListItem{{
				Pursuit:    models.Pursuit{ID: pursuitID, OwnerIdentity: "alice", Title: "Prepare evidence bundle", RiskLevel: "medium", Confidence: 0.82, PriorityScore: 80},
				NextAction: "Review the evidence bundle and choose the next safe action.",
			}},
		},
		pursuits: []models.Pursuit{{ID: pursuitID, OwnerIdentity: "alice", Title: "Prepare evidence bundle", Status: pursuitpkg.StatusActive}},
	}
	engine := NewServiceWithPursuits(repo, nil, nil, pursuits)

	scan, err := engine.ScanForOwner("alice", "manual")
	if err != nil {
		t.Fatalf("ScanForOwner: %v", err)
	}
	if scan.OwnerIdentity != "alice" || scan.Created != 1 || scan.Advanced != 0 {
		t.Fatalf("owner scan = %#v, want one private suggestion-only proposal", scan)
	}
	if repo.opportunity == nil || repo.opportunity.OwnerIdentity != "alice" || repo.opportunity.SourceID != pursuitID.String() {
		t.Fatalf("stored opportunity = %#v, want Alice pursuit proposal", repo.opportunity)
	}
	if repo.opportunity.WorkflowID != nil {
		t.Fatalf("personal scan unexpectedly created executable workflow state: %#v", repo.opportunity)
	}
}

func testNeedMap() map[string]models.AmbientNeed {
	result := map[string]models.AmbientNeed{}
	for _, need := range defaultNeeds() {
		result[need.Key] = need
	}
	return result
}

type ambientWorkflowSpy struct {
	intakeRequests []workflow.IntakeRequest
	recordID       uuid.UUID
}

func (s *ambientWorkflowSpy) Dashboard() (*workflow.WorkflowDashboard, error) {
	return &workflow.WorkflowDashboard{}, nil
}

func (s *ambientWorkflowSpy) Items(bool) ([]models.WorkflowItem, error) {
	return nil, nil
}

func (s *ambientWorkflowSpy) Intake(request workflow.IntakeRequest) (*workflow.WorkflowRecord, error) {
	s.intakeRequests = append(s.intakeRequests, request)
	if s.recordID == uuid.Nil {
		s.recordID = uuid.New()
	}
	return &workflow.WorkflowRecord{Item: models.WorkflowItem{ID: s.recordID, Title: request.Input}}, nil
}

func (s *ambientWorkflowSpy) RunDue(workflow.RunDueRequest) (*workflow.WorkflowRunSummary, error) {
	return &workflow.WorkflowRunSummary{}, nil
}

func (s *ambientWorkflowSpy) RunDueOpenLoops(workflow.RunDueRequest) (*workflow.OpenLoopRunSummary, error) {
	return &workflow.OpenLoopRunSummary{}, nil
}

type ambientPursuitSpy struct {
	dashboard        *pursuitpkg.Dashboard
	pursuits         []models.Pursuit
	links            []pursuitpkg.LinkRequest
	linkedPursuitIDs []uuid.UUID
	owners           map[uuid.UUID]string
}

func (s *ambientPursuitSpy) Dashboard() (*pursuitpkg.Dashboard, error) {
	if s.dashboard != nil {
		return s.dashboard, nil
	}
	return &pursuitpkg.Dashboard{}, nil
}

func (s *ambientPursuitSpy) DashboardForOwner(_ string) (*pursuitpkg.Dashboard, error) {
	return s.Dashboard()
}

func (s *ambientPursuitSpy) List(bool) ([]models.Pursuit, error) {
	return s.pursuits, nil
}

func (s *ambientPursuitSpy) ListForOwner(ownerIdentity string, _ bool) ([]models.Pursuit, error) {
	result := []models.Pursuit{}
	for _, item := range s.pursuits {
		if item.OwnerIdentity == ownerIdentity {
			result = append(result, item)
		}
	}
	return result, nil
}

func (s *ambientPursuitSpy) Link(id uuid.UUID, request pursuitpkg.LinkRequest) (*models.PursuitLink, error) {
	s.linkedPursuitIDs = append(s.linkedPursuitIDs, id)
	s.links = append(s.links, request)
	return &models.PursuitLink{
		ID:           uuid.New(),
		PursuitID:    id,
		LinkType:     request.LinkType,
		LinkID:       request.LinkID,
		Relationship: request.Relationship,
		SourceURI:    request.SourceURI,
		SourceLabel:  request.SourceLabel,
		Confidence:   request.Confidence,
	}, nil
}

func (s *ambientPursuitSpy) DetailForOwner(ownerIdentity string, id uuid.UUID) (*pursuitpkg.PursuitDetail, error) {
	if owner, found := s.owners[id]; found && owner != "" && owner != ownerIdentity {
		return nil, errors.New("pursuit not found")
	}
	return &pursuitpkg.PursuitDetail{Pursuit: models.Pursuit{ID: id, OwnerIdentity: ownerIdentity}}, nil
}

func TestClosedPursuitOpportunityUpdatesCompleteOnlyOpenPursuitWork(t *testing.T) {
	completedID := uuid.New()
	archivedID := uuid.New()
	activeID := uuid.New()
	updatedAt := time.Now().UTC()
	updates := closedPursuitOpportunityUpdates(
		[]models.AmbientOpportunity{
			{ID: uuid.New(), SourceType: "pursuit_decision", SourceID: completedID.String(), Status: StatusProposed},
			{ID: uuid.New(), SourceType: "pursuit_blocker", SourceID: archivedID.String(), Status: StatusAccepted},
			{ID: uuid.New(), SourceType: "pursuit_high_risk", SourceID: activeID.String(), Status: StatusProposed},
			{ID: uuid.New(), SourceType: "pursuit_review_due", SourceID: completedID.String(), Status: StatusDismissed},
		},
		[]models.Pursuit{
			{ID: completedID, Status: pursuitpkg.StatusCompleted},
			{ID: archivedID, Status: pursuitpkg.StatusArchived, Archived: true},
			{ID: activeID, Status: pursuitpkg.StatusActive},
		},
		updatedAt,
	)
	if len(updates) != 2 {
		t.Fatalf("updates = %#v, want completed and archived pursuit opportunities only", updates)
	}
	for _, item := range updates {
		if item.Status != StatusCompleted || item.LastSeenAt != updatedAt || !strings.Contains(item.ResolutionNote, "Linked pursuit closed") {
			t.Fatalf("closed pursuit update = %#v", item)
		}
	}
}
