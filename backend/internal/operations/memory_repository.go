package operations

import (
	"sort"
	"sync"

	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
)

// MemoryRepository is an in-process Repository implementation. It is a real
// port implementation (not a mock) used by the background loop's tests and by
// the DB-free smoke path; production uses GormRepository.
type MemoryRepository struct {
	mu     sync.Mutex
	ops    map[uuid.UUID]models.Operation
	events []models.OperationEvent
}

// NewMemoryRepository builds an empty in-memory repository.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{ops: map[uuid.UUID]models.Operation{}}
}

func (r *MemoryRepository) Create(op *models.Operation) (*models.Operation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if op.ID == uuid.Nil {
		op.ID = uuid.New()
	}
	r.ops[op.ID] = *op
	cp := r.ops[op.ID]
	return &cp, nil
}

func (r *MemoryRepository) Update(op *models.Operation) (*models.Operation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ops[op.ID] = *op
	cp := r.ops[op.ID]
	return &cp, nil
}

func (r *MemoryRepository) GetByID(ownerUserID, workspaceID string, id uuid.UUID) (*models.Operation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	op, ok := r.ops[id]
	if !ok || op.OwnerUserID != ownerUserID || op.WorkspaceID != workspaceID {
		return nil, ErrNotFound
	}
	cp := op
	return &cp, nil
}

func (r *MemoryRepository) FindByDedupeKey(workspaceID, dedupeKey string) (*models.Operation, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var found *models.Operation
	for _, op := range r.ops {
		if op.WorkspaceID != workspaceID || op.DedupeKey != dedupeKey {
			continue
		}
		if op.Status == string(StatusArchived) || op.Status == string(StatusDismissed) {
			continue
		}
		if found == nil || op.CreatedAt.After(found.CreatedAt) {
			cp := op
			found = &cp
		}
	}
	if found == nil {
		return nil, false, nil
	}
	return found, true, nil
}

func (r *MemoryRepository) List(f Filter) ([]models.Operation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []models.Operation
	for _, op := range r.ops {
		if op.OwnerUserID != f.OwnerUserID || op.WorkspaceID != f.WorkspaceID {
			continue
		}
		if f.Status != "" && op.Status != string(f.Status) {
			continue
		}
		if f.RiskLevel != "" && op.RiskLevel != string(f.RiskLevel) {
			continue
		}
		out = append(out, op)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	out = applyWindow(out, f.Offset, limit)
	return out, nil
}

func (r *MemoryRepository) ListDue(ownerUserID, workspaceID string, limit int) ([]models.Operation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	actionable := map[string]bool{}
	for _, s := range actionableStatuses {
		actionable[s] = true
	}
	var out []models.Operation
	for _, op := range r.ops {
		if op.OwnerUserID != ownerUserID || op.WorkspaceID != workspaceID {
			continue
		}
		if !actionable[op.Status] {
			continue
		}
		out = append(out, op)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	out = applyWindow(out, 0, limit)
	return out, nil
}

func (r *MemoryRepository) Dashboard(ownerUserID, workspaceID string) (Dashboard, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d := Dashboard{CountsByStatus: map[string]int{}, CountsByRisk: map[string]int{}}
	var all []models.Operation
	for _, op := range r.ops {
		if op.OwnerUserID != ownerUserID || op.WorkspaceID != workspaceID {
			continue
		}
		d.CountsByStatus[op.Status]++
		d.CountsByRisk[op.RiskLevel]++
		all = append(all, op)
	}
	d.NeedsRobert = d.CountsByStatus[string(StatusAwaitingApproval)]
	d.DoneWhileAway = d.CountsByStatus[string(StatusCompleted)]
	d.Blocked = d.CountsByStatus[string(StatusBlocked)]
	d.Running = d.CountsByStatus[string(StatusRunning)]
	d.Failed = d.CountsByStatus[string(StatusFailed)]
	sort.Slice(all, func(i, j int) bool { return all[i].UpdatedAt.After(all[j].UpdatedAt) })
	d.Recent = applyWindow(all, 0, 20)
	return d, nil
}

func (r *MemoryRepository) AppendEvent(evt *models.OperationEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if evt.ID == uuid.Nil {
		evt.ID = uuid.New()
	}
	r.events = append(r.events, *evt)
	return nil
}

func (r *MemoryRepository) ListEvents(operationID uuid.UUID, limit int) ([]models.OperationEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []models.OperationEvent
	for _, e := range r.events {
		if e.OperationID == operationID {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	return applyWindow(out, 0, limit), nil
}

func applyWindow[T any](items []T, offset, limit int) []T {
	if offset >= len(items) {
		return nil
	}
	items = items[offset:]
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}
