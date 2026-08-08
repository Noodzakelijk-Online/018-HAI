package workflow

import (
	"sort"
	"testing"
	"time"

	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
)

func TestReminderActivationLedgerSeparatesPreparationDecisionAndExecution(t *testing.T) {
	repo := newReminderActivationFakeRepo()
	workflowID := uuid.New()
	checklistID := uuid.New()
	reminderAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	repo.items[workflowID] = &models.WorkflowItem{
		ID: workflowID, OwnerIdentity: "alice", Title: "Review appointment",
		CurrentState: StateReady, TaskType: "administrative", RiskLevel: "low",
		AutonomyLevel: "manual", SourceURI: "calendar://internal-source",
	}
	repo.checklist[workflowID] = []models.WorkflowChecklistItem{{
		ID: checklistID, WorkflowID: workflowID, Label: "Review before appointment",
		Status: "open", ReminderAt: &reminderAt,
	}}
	candidate := WorkflowReminderCandidate{Workflow: *repo.items[workflowID], Reminder: repo.checklist[workflowID][0]}
	digest, err := reminderEvidenceDigest(candidate)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(repo).(ReminderActivationService)
	prepare := ReminderActivationPrepareRequest{
		ExpectedReminderDigest: digest,
		IdempotencyKey:         "reminder:appointment:one",
		ActivationKind:         ReminderActivationKindInternal,
		Confirmation:           ReminderActivationPrepareConfirmation,
	}
	result, err := service.PrepareReminderActivationForOwner("alice", "alice", checklistID, prepare)
	if err != nil {
		t.Fatalf("prepare reminder activation: %v", err)
	}
	if result.Replayed || result.CanExecute || result.Authority != ReminderActivationRequestAuthority ||
		result.Request.ActivationKind != ReminderActivationKindInternal {
		t.Fatalf("preparation result = %#v", result)
	}
	replayed, err := service.PrepareReminderActivationForOwner("alice", "alice", checklistID, prepare)
	if err != nil || !replayed.Replayed || replayed.Request.ID != result.Request.ID {
		t.Fatalf("preparation replay = %#v, %v", replayed, err)
	}
	if _, err := service.PrepareReminderActivationForOwner("bob", "bob", checklistID, prepare); err == nil {
		t.Fatal("foreign owner prepared Alice's reminder")
	}

	decision, err := service.DecideReminderActivationForOwner(
		"alice", "alice", result.Request.ID,
		ReminderActivationDecisionRequest{
			Decision: ReminderActivationDecisionApproved, Reason: "Keep this internal reminder visible.",
			Confirmation:                    ReminderActivationApproveConfirmation,
			ExpectedActivationRequestDigest: result.Request.RecordDigest,
		},
	)
	if err != nil {
		t.Fatalf("approve reminder preparation: %v", err)
	}
	if decision.Replayed || decision.CanExecute || decision.Authority != ReminderActivationDecisionAuthority ||
		decision.Decision.ExpiresAt == nil {
		t.Fatalf("decision result = %#v", decision)
	}
	replayedDecision, err := service.DecideReminderActivationForOwner(
		"alice", "alice", result.Request.ID,
		ReminderActivationDecisionRequest{
			Decision: ReminderActivationDecisionApproved, Reason: "Keep this internal reminder visible.",
			Confirmation:                    ReminderActivationApproveConfirmation,
			ExpectedActivationRequestDigest: result.Request.RecordDigest,
		},
	)
	if err != nil || !replayedDecision.Replayed || replayedDecision.Decision.ID != decision.Decision.ID {
		t.Fatalf("decision replay = %#v, %v", replayedDecision, err)
	}
	history, err := service.ReminderActivationHistoryForOwner("alice", 10)
	if err != nil {
		t.Fatalf("activation history: %v", err)
	}
	if history.CanExecute || history.Authority != ReminderActivationHistoryAuthority || len(history.Items) != 1 ||
		history.Items[0].LatestDecision == nil || history.Items[0].Status != ReminderActivationDecisionApproved {
		t.Fatalf("activation history = %#v", history)
	}
	decisions, err := service.ReminderActivationDecisionHistoryForOwner("alice", result.Request.ID, 10)
	if err != nil || decisions.CanExecute || len(decisions.Decisions) != 1 {
		t.Fatalf("decision history = %#v, %v", decisions, err)
	}
}

func TestReminderActivationFailsClosedForStaleEvidenceAndGenericKinds(t *testing.T) {
	repo := newReminderActivationFakeRepo()
	workflowID := uuid.New()
	checklistID := uuid.New()
	reminderAt := time.Now().UTC().Add(time.Hour)
	repo.items[workflowID] = &models.WorkflowItem{
		ID: workflowID, OwnerIdentity: "alice", Title: "Current reminder",
		CurrentState: StateReady, RiskLevel: "low",
	}
	repo.checklist[workflowID] = []models.WorkflowChecklistItem{{
		ID: checklistID, WorkflowID: workflowID, Label: "Current", Status: "open", ReminderAt: &reminderAt,
	}}
	candidate := WorkflowReminderCandidate{Workflow: *repo.items[workflowID], Reminder: repo.checklist[workflowID][0]}
	digest, _ := reminderEvidenceDigest(candidate)
	service := NewService(repo).(ReminderActivationService)
	for _, request := range []ReminderActivationPrepareRequest{
		{ExpectedReminderDigest: digest, IdempotencyKey: "external-effect", ActivationKind: "calendar_write", Confirmation: ReminderActivationPrepareConfirmation},
		{ExpectedReminderDigest: digest, IdempotencyKey: "wrong-confirmation", ActivationKind: ReminderActivationKindInternal, Confirmation: "SEND IT"},
		{ExpectedReminderDigest: "stale" + digest[5:], IdempotencyKey: "stale", ActivationKind: ReminderActivationKindInternal, Confirmation: ReminderActivationPrepareConfirmation},
	} {
		if _, err := service.PrepareReminderActivationForOwner("alice", "alice", checklistID, request); err == nil {
			t.Fatalf("unsafe preparation accepted: %#v", request)
		}
	}
}

type reminderActivationFakeRepo struct {
	*fakeWorkflowRepo
	requests  map[uuid.UUID]models.WorkflowReminderActivationRequest
	decisions map[uuid.UUID][]models.WorkflowReminderActivationDecision
}

func newReminderActivationFakeRepo() *reminderActivationFakeRepo {
	return &reminderActivationFakeRepo{
		fakeWorkflowRepo: newFakeWorkflowRepo(),
		requests:         map[uuid.UUID]models.WorkflowReminderActivationRequest{},
		decisions:        map[uuid.UUID][]models.WorkflowReminderActivationDecision{},
	}
}

func (r *reminderActivationFakeRepo) LoadReminderActivationSourceForOwner(owner string, itemID uuid.UUID) (*WorkflowReminderCandidate, error) {
	for workflowID, checklist := range r.checklist {
		workflow := r.items[workflowID]
		if workflow == nil || workflow.OwnerIdentity != owner || workflow.Archived ||
			workflow.CurrentState == StateCompleted || workflow.CurrentState == StateArchived {
			continue
		}
		for _, item := range checklist {
			if item.ID == itemID && item.Status == "open" && item.ReminderAt != nil {
				return &WorkflowReminderCandidate{Workflow: *workflow, Reminder: item}, nil
			}
		}
	}
	return nil, nil
}

func (r *reminderActivationFakeRepo) FindOrCreateReminderActivationRequest(wanted *models.WorkflowReminderActivationRequest) (*models.WorkflowReminderActivationRequest, bool, error) {
	for _, existing := range r.requests {
		if existing.OwnerIdentity == wanted.OwnerIdentity && existing.IdempotencyKey == wanted.IdempotencyKey {
			copy := existing
			return &copy, false, nil
		}
	}
	r.requests[wanted.ID] = *wanted
	copy := *wanted
	return &copy, true, nil
}

func (r *reminderActivationFakeRepo) ListReminderActivationRequestsForOwner(owner string, limit int) ([]models.WorkflowReminderActivationRequest, error) {
	result := []models.WorkflowReminderActivationRequest{}
	for _, request := range r.requests {
		if request.OwnerIdentity == owner {
			result = append(result, request)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].RequestedAt.After(result[j].RequestedAt) })
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (r *reminderActivationFakeRepo) LoadReminderActivationRequestForOwner(owner string, requestID uuid.UUID) (*models.WorkflowReminderActivationRequest, *models.WorkflowReminderActivationDecision, error) {
	request, ok := r.requests[requestID]
	if !ok || request.OwnerIdentity != owner {
		return nil, nil, nil
	}
	var latest *models.WorkflowReminderActivationDecision
	values := r.decisions[requestID]
	if len(values) > 0 {
		copy := values[len(values)-1]
		latest = &copy
	}
	copy := request
	return &copy, latest, nil
}

func (r *reminderActivationFakeRepo) SaveReminderActivationDecision(wanted *models.WorkflowReminderActivationDecision) (*models.WorkflowReminderActivationDecision, bool, error) {
	for _, existing := range r.decisions[wanted.ActivationRequestID] {
		if existing.RequestDigest == wanted.RequestDigest {
			copy := existing
			return &copy, false, nil
		}
	}
	r.decisions[wanted.ActivationRequestID] = append(r.decisions[wanted.ActivationRequestID], *wanted)
	copy := *wanted
	return &copy, true, nil
}

func (r *reminderActivationFakeRepo) ListReminderActivationDecisionsForOwner(owner string, requestID uuid.UUID, limit int) ([]models.WorkflowReminderActivationDecision, error) {
	values := r.decisions[requestID]
	result := make([]models.WorkflowReminderActivationDecision, 0, len(values))
	for index := len(values) - 1; index >= 0; index-- {
		if values[index].OwnerIdentity == owner {
			result = append(result, values[index])
		}
	}
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}
