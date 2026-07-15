package operations

import (
	"strings"
	"time"

	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
)

// Service orchestrates the Operation Ledger over the repository + domain rules.
type Service struct {
	repo Repository
	now  func() time.Time
}

// NewService builds a service over repo.
func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// DefaultService builds a service over the default repository.
func DefaultService() *Service { return NewService(DefaultRepository()) }

// IngestResult is the outcome of Ingest.
type IngestResult struct {
	Operation models.Operation
	Created   bool
}

// Ingest creates an Operation for a source item, or — if an active Operation
// already exists for the same dedupe key — refreshes it instead of creating a
// duplicate (§10.9 step 7-9).
func (s *Service) Ingest(in NewOperationInput) (IngestResult, error) {
	now := s.now().UTC()
	if existing, found, err := s.repo.FindByDedupeKey(firstNonEmpty(in.WorkspaceID, "local"), in.DedupeKey); err != nil {
		return IngestResult{}, err
	} else if found {
		if j := strings.TrimSpace(in.EvidenceJSON); j != "" && j != "{}" {
			existing.EvidenceJSON = in.EvidenceJSON
		}
		existing.UpdatedAt = now
		updated, err := s.repo.Update(existing)
		if err != nil {
			return IngestResult{}, err
		}
		return IngestResult{Operation: *updated, Created: false}, nil
	}
	op, err := NewOperation(in, now)
	if err != nil {
		return IngestResult{}, err
	}
	created, err := s.repo.Create(&op)
	if err != nil {
		return IngestResult{}, err
	}
	_ = s.repo.AppendEvent(&models.OperationEvent{
		OperationID: created.ID,
		EventType:   "created",
		ActorType:   string(OwnerHAI),
		AfterStatus: created.Status,
		Message:     "operation created from " + created.SourceType,
		PayloadJSON: "{}",
		CreatedAt:   now,
	})
	return IngestResult{Operation: *created, Created: true}, nil
}

// Get returns an operation scoped to owner/workspace.
func (s *Service) Get(ownerUserID, workspaceID string, id uuid.UUID) (*models.Operation, error) {
	return s.repo.GetByID(ownerUserID, workspaceID, id)
}

// List returns operations for a filter.
func (s *Service) List(f Filter) ([]models.Operation, error) { return s.repo.List(f) }

// ListDue returns operations the background loop may still progress.
func (s *Service) ListDue(ownerUserID, workspaceID string, limit int) ([]models.Operation, error) {
	return s.repo.ListDue(ownerUserID, workspaceID, limit)
}

// Dashboard returns the Background Operations roll-up.
func (s *Service) Dashboard(ownerUserID, workspaceID string) (Dashboard, error) {
	return s.repo.Dashboard(ownerUserID, workspaceID)
}

// Events returns an operation's audit trail.
func (s *Service) Events(operationID uuid.UUID) ([]models.OperationEvent, error) {
	return s.repo.ListEvents(operationID, 0)
}

// Transition moves an operation to a new status (validated) and audits it.
func (s *Service) Transition(op models.Operation, to OperationStatus, actorType, actorID, message string) (*models.Operation, error) {
	updated, evt, err := ApplyTransition(op, to, actorType, actorID, message, s.now().UTC())
	if err != nil {
		return nil, err
	}
	saved, err := s.repo.Update(&updated)
	if err != nil {
		return nil, err
	}
	_ = s.repo.AppendEvent(&evt)
	return saved, nil
}

// Save persists a mutated operation (e.g. after a policy decision or execution
// result) and appends a domain event.
func (s *Service) Save(op models.Operation, eventType, actorType, message string) (*models.Operation, error) {
	now := s.now().UTC()
	op.UpdatedAt = now
	op.Version++
	saved, err := s.repo.Update(&op)
	if err != nil {
		return nil, err
	}
	_ = s.repo.AppendEvent(&models.OperationEvent{
		OperationID: op.ID,
		EventType:   eventType,
		ActorType:   actorType,
		AfterStatus: op.Status,
		Message:     message,
		PayloadJSON: "{}",
		CreatedAt:   now,
	})
	return saved, nil
}
