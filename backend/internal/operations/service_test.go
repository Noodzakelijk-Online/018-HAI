package operations

import (
	"testing"

	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
)

// fakeRepo is an in-memory Repository for service tests (no DB).
type fakeRepo struct {
	ops    map[uuid.UUID]models.Operation
	events []models.OperationEvent
}

func newFakeRepo() *fakeRepo { return &fakeRepo{ops: map[uuid.UUID]models.Operation{}} }

func (f *fakeRepo) Create(op *models.Operation) (*models.Operation, error) {
	if op.ID == uuid.Nil {
		op.ID = uuid.New()
	}
	f.ops[op.ID] = *op
	return op, nil
}
func (f *fakeRepo) Update(op *models.Operation) (*models.Operation, error) {
	f.ops[op.ID] = *op
	return op, nil
}
func (f *fakeRepo) GetByID(owner, ws string, id uuid.UUID) (*models.Operation, error) {
	op, ok := f.ops[id]
	if !ok || op.OwnerUserID != owner || op.WorkspaceID != ws {
		return nil, ErrNotFound
	}
	return &op, nil
}
func (f *fakeRepo) FindByDedupeKey(ws, key string) (*models.Operation, bool, error) {
	for _, op := range f.ops {
		if op.WorkspaceID == ws && op.DedupeKey == key &&
			op.Status != string(StatusArchived) && op.Status != string(StatusDismissed) {
			copyOp := op
			return &copyOp, true, nil
		}
	}
	return nil, false, nil
}
func (f *fakeRepo) List(fl Filter) ([]models.Operation, error) {
	var out []models.Operation
	for _, op := range f.ops {
		if op.OwnerUserID == fl.OwnerUserID && op.WorkspaceID == fl.WorkspaceID {
			out = append(out, op)
		}
	}
	return out, nil
}
func (f *fakeRepo) ListDue(owner, ws string, limit int) ([]models.Operation, error) {
	var out []models.Operation
	for _, op := range f.ops {
		if op.OwnerUserID == owner && op.WorkspaceID == ws {
			out = append(out, op)
		}
	}
	return out, nil
}
func (f *fakeRepo) Dashboard(owner, ws string) (Dashboard, error) {
	d := Dashboard{CountsByStatus: map[string]int{}, CountsByRisk: map[string]int{}}
	for _, op := range f.ops {
		if op.OwnerUserID == owner && op.WorkspaceID == ws {
			d.CountsByStatus[op.Status]++
		}
	}
	return d, nil
}
func (f *fakeRepo) AppendEvent(evt *models.OperationEvent) error {
	f.events = append(f.events, *evt)
	return nil
}
func (f *fakeRepo) ListEvents(id uuid.UUID, limit int) ([]models.OperationEvent, error) {
	var out []models.OperationEvent
	for _, e := range f.events {
		if e.OperationID == id {
			out = append(out, e)
		}
	}
	return out, nil
}

func sampleInput() NewOperationInput {
	return NewOperationInput{
		OwnerUserID:   "user-1",
		WorkspaceID:   "local",
		Title:         "Lawyer email about hearing",
		OperationType: "email",
		SourceType:    "gmail",
		DedupeKey:     "dedupe-abc",
	}
}

func TestIngestCreatesOperationAndEvent(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	res, err := svc.Ingest(sampleInput())
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if !res.Created || res.Operation.Status != string(StatusNew) {
		t.Fatalf("expected a created new operation, got %+v", res)
	}
	if len(repo.ops) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(repo.ops))
	}
	if len(repo.events) != 1 || repo.events[0].EventType != "created" {
		t.Fatalf("expected a 'created' event, got %+v", repo.events)
	}
}

func TestIngestDuplicateIsIdempotent(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	if _, err := svc.Ingest(sampleInput()); err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	res, err := svc.Ingest(sampleInput()) // same dedupe key
	if err != nil {
		t.Fatalf("second ingest: %v", err)
	}
	if res.Created {
		t.Fatalf("duplicate ingest must not create a second operation")
	}
	if len(repo.ops) != 1 {
		t.Fatalf("expected exactly 1 operation after duplicate sync, got %d", len(repo.ops))
	}
}

func TestTransitionEnforcesStateMachine(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	res, _ := svc.Ingest(sampleInput())
	op := res.Operation

	// Illegal: new -> running.
	if _, err := svc.Transition(op, StatusRunning, "hai", "", "bad"); err == nil {
		t.Fatalf("new -> running must be rejected")
	}
	// Legal path: new -> classified -> ready -> running.
	classified, err := svc.Transition(op, StatusClassified, "hai", "", "classified")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	ready, err := svc.Transition(*classified, StatusReady, "hai", "", "ready")
	if err != nil {
		t.Fatalf("ready: %v", err)
	}
	if _, err := svc.Transition(*ready, StatusRunning, "hai", "", "run"); err != nil {
		t.Fatalf("running: %v", err)
	}
}

func TestCompleteRequiresVerification(t *testing.T) {
	now := StatusVerifying
	op := models.Operation{
		OwnerUserID: "u", WorkspaceID: "local", Title: "t", DedupeKey: "k",
		Status:             string(now),
		RiskLevel:          string(RiskLow),
		AutonomyLevel:      string(AutonomyAuto),
		OwnerType:          string(OwnerHAI),
		CurrentDecision:    string(DecisionRunSafeLocalWorker),
		VerificationStatus: string(VerificationFailed), // not passed / not_required
	}
	repo := newFakeRepo()
	repo.ops[uuid.New()] = op
	svc := NewService(repo)
	if _, err := svc.Transition(op, StatusCompleted, "hai", "", "done"); err == nil {
		t.Fatalf("verifying -> completed must fail when verification failed")
	}
	op.VerificationStatus = string(VerificationPassed)
	if _, err := svc.Transition(op, StatusCompleted, "hai", "", "done"); err != nil {
		t.Fatalf("verifying -> completed should succeed when verification passed: %v", err)
	}
}
