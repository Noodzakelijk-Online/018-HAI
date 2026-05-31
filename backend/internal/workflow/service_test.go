package workflow

import (
	"automation-hub-backend/internal/models"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestIntakeCreatesApprovalGatedLegalWorkflow(t *testing.T) {
	repo := newFakeWorkflowRepo()
	service := NewService(repo)

	record, err := service.Intake(IntakeRequest{
		Input:      "Email from lawyer about Vivare legal hearing tomorrow. Draft formal reply.",
		ProjectKey: "Vivare dispute",
		SourceType: "email",
		SourceURI:  "mailto:lawyer@example.test",
		Trigger:    "email.sync",
	})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	if record.Item.CurrentState != StateNeedsApproval {
		t.Fatalf("state = %q, want needs approval", record.Item.CurrentState)
	}
	if !record.Item.RequiresApproval {
		t.Fatalf("expected approval requirement")
	}
	if record.Item.PriorityScore < 80 {
		t.Fatalf("priority = %d, want high legal priority", record.Item.PriorityScore)
	}
	if len(record.Checklist) == 0 {
		t.Fatalf("expected generated checklist")
	}
	if !hasApprovalChecklist(record.Checklist) {
		t.Fatalf("expected approval-marked checklist item")
	}
	if len(record.Events) != 1 {
		t.Fatalf("events = %d, want 1 audit event", len(record.Events))
	}
}

func TestTransitionRequiresApprovalFromNeedsApprovalToReady(t *testing.T) {
	repo := newFakeWorkflowRepo()
	service := NewService(repo)
	record, err := service.Intake(IntakeRequest{Input: "Publish public Medium article from Trello card"})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}

	if _, err := service.Transition(record.Item.ID, TransitionRequest{TargetState: StateReady}); err == nil {
		t.Fatalf("expected transition without approval to fail")
	}
	approved, err := service.Transition(record.Item.ID, TransitionRequest{TargetState: StateReady, Approved: true, Message: "Robert approved draft-only workflow"})
	if err != nil {
		t.Fatalf("Transition approved: %v", err)
	}
	if approved.Item.CurrentState != StateReady {
		t.Fatalf("state = %q, want ready", approved.Item.CurrentState)
	}
	if len(approved.Events) < 2 {
		t.Fatalf("expected transition audit event")
	}
}

func TestChecklistUpdateAuditsProgress(t *testing.T) {
	service := NewService(newFakeWorkflowRepo())
	record, err := service.Intake(IntakeRequest{Input: "Create Trello checklist for Docker build issue"})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	updated, err := service.UpdateChecklistItem(record.Item.ID, record.Checklist[0].ID, ChecklistUpdateRequest{Status: "done"})
	if err != nil {
		t.Fatalf("UpdateChecklistItem: %v", err)
	}
	if updated.Checklist[0].Status != "done" {
		t.Fatalf("checklist status = %q, want done", updated.Checklist[0].Status)
	}
	if len(updated.Events) < 2 {
		t.Fatalf("expected checklist audit event")
	}
}

func TestIntakeDeduplicatesBySourceURI(t *testing.T) {
	service := NewService(newFakeWorkflowRepo())
	first, err := service.Intake(IntakeRequest{
		Input:     "Follow up: request missing evidence bundle from legal contact.",
		SourceURI: "local://source/item/123",
		Trigger:   "source.extraction",
	})
	if err != nil {
		t.Fatalf("Intake first: %v", err)
	}
	second, err := service.Intake(IntakeRequest{
		Input:     "Follow up: request missing evidence bundle from legal contact.",
		SourceURI: "local://source/item/123",
		Trigger:   "source.extraction",
	})
	if err != nil {
		t.Fatalf("Intake second: %v", err)
	}
	if first.Item.ID != second.Item.ID {
		t.Fatalf("deduped ID = %s, want %s", second.Item.ID, first.Item.ID)
	}
	if len(second.Events) < 2 {
		t.Fatalf("expected dedupe audit event")
	}
}

func hasApprovalChecklist(items []models.WorkflowChecklistItem) bool {
	for _, item := range items {
		if item.RequiresApproval {
			return true
		}
	}
	return false
}

type fakeWorkflowRepo struct {
	items     map[uuid.UUID]*models.WorkflowItem
	checklist map[uuid.UUID][]models.WorkflowChecklistItem
	events    map[uuid.UUID][]models.WorkflowEvent
}

func newFakeWorkflowRepo() *fakeWorkflowRepo {
	return &fakeWorkflowRepo{
		items:     map[uuid.UUID]*models.WorkflowItem{},
		checklist: map[uuid.UUID][]models.WorkflowChecklistItem{},
		events:    map[uuid.UUID][]models.WorkflowEvent{},
	}
}

func (r *fakeWorkflowRepo) CreateItem(item *models.WorkflowItem) (*models.WorkflowItem, error) {
	if item.ID == uuid.Nil {
		item.ID = uuid.New()
	}
	now := time.Now().UTC()
	item.CreatedAt = now
	item.UpdatedAt = now
	r.items[item.ID] = item
	return item, nil
}

func (r *fakeWorkflowRepo) UpdateItem(item *models.WorkflowItem) (*models.WorkflowItem, error) {
	item.UpdatedAt = time.Now().UTC()
	r.items[item.ID] = item
	return item, nil
}

func (r *fakeWorkflowRepo) FindItem(id uuid.UUID) (*models.WorkflowItem, error) {
	item, ok := r.items[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	copied := *item
	return &copied, nil
}

func (r *fakeWorkflowRepo) FindActiveItemBySourceURI(sourceURI string) (*models.WorkflowItem, error) {
	for _, item := range r.items {
		if item.SourceURI == sourceURI && !item.Archived {
			copied := *item
			return &copied, nil
		}
	}
	return nil, nil
}

func (r *fakeWorkflowRepo) FindItems(includeArchived bool) ([]models.WorkflowItem, error) {
	result := []models.WorkflowItem{}
	for _, item := range r.items {
		if includeArchived || !item.Archived {
			result = append(result, *item)
		}
	}
	return result, nil
}

func (r *fakeWorkflowRepo) CreateChecklistItem(item *models.WorkflowChecklistItem) (*models.WorkflowChecklistItem, error) {
	if item.ID == uuid.Nil {
		item.ID = uuid.New()
	}
	now := time.Now().UTC()
	item.CreatedAt = now
	item.UpdatedAt = now
	r.checklist[item.WorkflowID] = append(r.checklist[item.WorkflowID], *item)
	return item, nil
}

func (r *fakeWorkflowRepo) UpdateChecklistItem(item *models.WorkflowChecklistItem) (*models.WorkflowChecklistItem, error) {
	item.UpdatedAt = time.Now().UTC()
	items := r.checklist[item.WorkflowID]
	for index := range items {
		if items[index].ID == item.ID {
			items[index] = *item
			r.checklist[item.WorkflowID] = items
			return item, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *fakeWorkflowRepo) FindChecklist(workflowID uuid.UUID) ([]models.WorkflowChecklistItem, error) {
	return append([]models.WorkflowChecklistItem{}, r.checklist[workflowID]...), nil
}

func (r *fakeWorkflowRepo) CreateEvent(event *models.WorkflowEvent) (*models.WorkflowEvent, error) {
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	event.CreatedAt = time.Now().UTC()
	r.events[event.WorkflowID] = append([]models.WorkflowEvent{*event}, r.events[event.WorkflowID]...)
	return event, nil
}

func (r *fakeWorkflowRepo) FindEvents(workflowID uuid.UUID) ([]models.WorkflowEvent, error) {
	return append([]models.WorkflowEvent{}, r.events[workflowID]...), nil
}
