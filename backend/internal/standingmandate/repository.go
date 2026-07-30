package standingmandate

import (
	"context"
	"errors"
	"sort"
	"sync"

	"github.com/google/uuid"
)

var (
	ErrNotFound         = errors.New("standing mandate not found")
	ErrRevisionConflict = errors.New("standing mandate revision conflict")
	ErrDecisionConflict = errors.New("standing mandate decision already exists")
)

// Repository is the persistence boundary. Implementations must isolate records
// by owner and atomically enforce expectedRevision.
type Repository interface {
	Create(ctx context.Context, mandate StandingMandate) error
	Get(ctx context.Context, ownerIdentity string, id uuid.UUID) (*StandingMandate, error)
	Update(ctx context.Context, mandate StandingMandate, expectedRevision uint64) error
	List(ctx context.Context, ownerIdentity string) ([]StandingMandate, error)
	CreateDecision(ctx context.Context, decision AuthorizationDecision) error
	GetDecision(ctx context.Context, ownerIdentity string, id uuid.UUID) (*AuthorizationDecision, error)
	ListDecisions(
		ctx context.Context,
		ownerIdentity string,
		mandateID *uuid.UUID,
		limit int,
	) ([]AuthorizationDecision, error)
}

// MemoryRepository provides deterministic tests and local composition. It is
// not a substitute for the persistent repository required by production.
type MemoryRepository struct {
	mu        sync.RWMutex
	records   map[uuid.UUID]StandingMandate
	decisions map[uuid.UUID]AuthorizationDecision
}

var _ Repository = (*MemoryRepository)(nil)

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		records:   make(map[uuid.UUID]StandingMandate),
		decisions: make(map[uuid.UUID]AuthorizationDecision),
	}
}

func (r *MemoryRepository) Create(_ context.Context, mandate StandingMandate) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.records[mandate.ID]; exists {
		return ErrRevisionConflict
	}
	r.records[mandate.ID] = cloneMandate(mandate)
	return nil
}

func (r *MemoryRepository) Get(_ context.Context, ownerIdentity string, id uuid.UUID) (*StandingMandate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	mandate, exists := r.records[id]
	if !exists || mandate.OwnerIdentity != ownerIdentity {
		return nil, ErrNotFound
	}
	cloned := cloneMandate(mandate)
	return &cloned, nil
}

func (r *MemoryRepository) Update(_ context.Context, mandate StandingMandate, expectedRevision uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, exists := r.records[mandate.ID]
	if !exists || current.OwnerIdentity != mandate.OwnerIdentity {
		return ErrNotFound
	}
	if current.Revision != expectedRevision {
		return ErrRevisionConflict
	}
	r.records[mandate.ID] = cloneMandate(mandate)
	return nil
}

func (r *MemoryRepository) List(_ context.Context, ownerIdentity string) ([]StandingMandate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]StandingMandate, 0)
	for _, mandate := range r.records {
		if mandate.OwnerIdentity == ownerIdentity {
			result = append(result, cloneMandate(mandate))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID.String() < result[j].ID.String()
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}

func (r *MemoryRepository) CreateDecision(_ context.Context, decision AuthorizationDecision) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.decisions == nil {
		r.decisions = make(map[uuid.UUID]AuthorizationDecision)
	}
	if _, exists := r.decisions[decision.ID]; exists {
		return ErrDecisionConflict
	}
	for _, existing := range r.decisions {
		if existing.Evidence.DecisionDigest == decision.Evidence.DecisionDigest {
			return ErrDecisionConflict
		}
	}
	r.decisions[decision.ID] = cloneDecision(decision)
	return nil
}

func (r *MemoryRepository) GetDecision(
	_ context.Context,
	ownerIdentity string,
	id uuid.UUID,
) (*AuthorizationDecision, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	decision, exists := r.decisions[id]
	if !exists || decision.OwnerIdentity != ownerIdentity {
		return nil, ErrNotFound
	}
	cloned := cloneDecision(decision)
	return &cloned, nil
}

func (r *MemoryRepository) ListDecisions(
	_ context.Context,
	ownerIdentity string,
	mandateID *uuid.UUID,
	limit int,
) ([]AuthorizationDecision, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]AuthorizationDecision, 0)
	for _, decision := range r.decisions {
		if decision.OwnerIdentity != ownerIdentity {
			continue
		}
		if mandateID != nil && decision.MandateID != *mandateID {
			continue
		}
		result = append(result, cloneDecision(decision))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].EvaluatedAt.Equal(result[j].EvaluatedAt) {
			return result[i].ID.String() > result[j].ID.String()
		}
		return result[i].EvaluatedAt.After(result[j].EvaluatedAt)
	})
	if limit = boundedDecisionLimit(limit); len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}
