package plangraph

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
)

var (
	ErrNotFound            = errors.New("plan graph not found")
	ErrRevisionConflict    = errors.New("plan graph revision conflict")
	ErrIdempotencyConflict = errors.New("plan graph idempotency conflict")
	ErrReferenceInvalid    = errors.New("plan graph reference is invalid")
	ErrReferenceStale      = errors.New("plan graph reference is stale")
	ErrPlanNotAccepted     = errors.New("plan graph revision is not accepted")
)

// Repository stores immutable plan revisions. CreateRevision must atomically
// verify expectedPreviousRevision and must never update an existing revision.
type Repository interface {
	CreateRevision(ctx context.Context, plan Plan, expectedPreviousRevision uint64) error
	GetLatest(ctx context.Context, ownerIdentity string, id uuid.UUID) (*Plan, error)
	GetRevision(ctx context.Context, ownerIdentity string, id uuid.UUID, revision uint64) (*Plan, error)
	FindByIdempotencyKey(ctx context.Context, ownerIdentity, key string) (*Plan, error)
	ListLatest(ctx context.Context, ownerIdentity string) ([]Plan, error)
}

type MemoryRepository struct {
	mu      sync.RWMutex
	records map[string]map[uuid.UUID][]Plan
	keys    map[string]map[string]revisionRef
}

type revisionRef struct {
	ID       uuid.UUID
	Revision uint64
}

var _ Repository = (*MemoryRepository)(nil)

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		records: make(map[string]map[uuid.UUID][]Plan),
		keys:    make(map[string]map[string]revisionRef),
	}
}

func (repository *MemoryRepository) CreateRevision(_ context.Context, plan Plan, expectedPreviousRevision uint64) error {
	plan = normalizePlan(plan)
	if err := validateStoredPlan(plan); err != nil {
		return err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	owner := plan.OwnerIdentity
	if repository.records[owner] == nil {
		repository.records[owner] = make(map[uuid.UUID][]Plan)
	}
	if repository.keys[owner] == nil {
		repository.keys[owner] = make(map[string]revisionRef)
	}
	if plan.IdempotencyKey != "" {
		if ref, exists := repository.keys[owner][plan.IdempotencyKey]; exists {
			existing := repository.records[owner][ref.ID][ref.Revision-1]
			if existing.Digest == plan.Digest {
				return nil
			}
			return ErrIdempotencyConflict
		}
	}
	revisions := repository.records[owner][plan.ID]
	current := uint64(len(revisions))
	if current != expectedPreviousRevision || plan.Revision != current+1 {
		return ErrRevisionConflict
	}
	repository.records[owner][plan.ID] = append(revisions, clonePlan(plan))
	if plan.IdempotencyKey != "" {
		repository.keys[owner][plan.IdempotencyKey] = revisionRef{ID: plan.ID, Revision: plan.Revision}
	}
	return nil
}

func (repository *MemoryRepository) GetLatest(_ context.Context, ownerIdentity string, id uuid.UUID) (*Plan, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	revisions := repository.records[strings.TrimSpace(ownerIdentity)][id]
	if len(revisions) == 0 {
		return nil, ErrNotFound
	}
	result := clonePlan(revisions[len(revisions)-1])
	return &result, nil
}

func (repository *MemoryRepository) GetRevision(_ context.Context, ownerIdentity string, id uuid.UUID, revision uint64) (*Plan, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	revisions := repository.records[strings.TrimSpace(ownerIdentity)][id]
	if revision == 0 || revision > uint64(len(revisions)) {
		return nil, ErrNotFound
	}
	result := clonePlan(revisions[revision-1])
	return &result, nil
}

func (repository *MemoryRepository) FindByIdempotencyKey(_ context.Context, ownerIdentity, key string) (*Plan, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	key = strings.TrimSpace(key)
	ref, exists := repository.keys[ownerIdentity][key]
	if !exists {
		return nil, ErrNotFound
	}
	result := clonePlan(repository.records[ownerIdentity][ref.ID][ref.Revision-1])
	return &result, nil
}

func (repository *MemoryRepository) ListLatest(_ context.Context, ownerIdentity string) ([]Plan, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	result := make([]Plan, 0, len(repository.records[ownerIdentity]))
	for _, revisions := range repository.records[strings.TrimSpace(ownerIdentity)] {
		if len(revisions) > 0 {
			result = append(result, clonePlan(revisions[len(revisions)-1]))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID.String() < result[j].ID.String()
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result, nil
}
