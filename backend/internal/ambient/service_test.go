package ambient

import (
	"automation-hub-backend/internal/memoryengine"
	"automation-hub-backend/internal/models"
	pursuitpkg "automation-hub-backend/internal/pursuit"
	"automation-hub-backend/internal/workflow"
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

	accepted, err := engine.Accept(opportunity.ID, ResolutionRequest{Note: "Approve draft creation."})
	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if accepted.WorkflowID == nil || *accepted.WorkflowID != workflowID {
		t.Fatalf("accepted workflow id = %v, want %s", accepted.WorkflowID, workflowID)
	}
	if len(workflowSpy.intakeRequests) != 1 || !workflowSpy.intakeRequests[0].RequiresReview {
		t.Fatalf("workflow intake requests = %#v", workflowSpy.intakeRequests)
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
	links            []pursuitpkg.LinkRequest
	linkedPursuitIDs []uuid.UUID
}

func (s *ambientPursuitSpy) Dashboard() (*pursuitpkg.Dashboard, error) {
	if s.dashboard != nil {
		return s.dashboard, nil
	}
	return &pursuitpkg.Dashboard{}, nil
}

func (s *ambientPursuitSpy) List(bool) ([]models.Pursuit, error) {
	return nil, nil
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
