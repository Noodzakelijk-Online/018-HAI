package lifeledger

import (
	"context"
	"sort"
	"sync"
)

type MemoryRepository struct {
	mu                    sync.RWMutex
	commitments           map[string][]CommitmentRevision
	commitmentIdempotency map[string]CommitmentRevision
	costs                 map[string][]CostEntry
	costIdempotency       map[string]CostEntry
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		commitments:           make(map[string][]CommitmentRevision),
		commitmentIdempotency: make(map[string]CommitmentRevision),
		costs:                 make(map[string][]CostEntry),
		costIdempotency:       make(map[string]CostEntry),
	}
}

func (r *MemoryRepository) SaveCommitment(_ context.Context, record CommitmentRevision, expectedRevision uint64) (CommitmentRevision, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	idempotencyKey := ledgerKey(record.OwnerIdentity, record.IdempotencyKey)
	if existing, ok := r.commitmentIdempotency[idempotencyKey]; ok {
		if existing.RequestDigest != record.RequestDigest {
			return CommitmentRevision{}, false, ErrIdempotencyConflict
		}
		return cloneCommitment(existing), false, nil
	}
	key := ledgerKey(record.OwnerIdentity, record.CommitmentKey)
	history := r.commitments[key]
	current := uint64(len(history))
	if current != expectedRevision || record.Revision != expectedRevision+1 {
		return CommitmentRevision{}, false, ErrRevisionConflict
	}
	r.commitments[key] = append(history, cloneCommitment(record))
	r.commitmentIdempotency[idempotencyKey] = cloneCommitment(record)
	return cloneCommitment(record), true, nil
}

func (r *MemoryRepository) GetCommitment(_ context.Context, owner, key string) (CommitmentRevision, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	history := r.commitments[ledgerKey(owner, key)]
	if len(history) == 0 {
		return CommitmentRevision{}, ErrNotFound
	}
	return cloneCommitment(history[len(history)-1]), nil
}

func (r *MemoryRepository) ListCommitments(_ context.Context, owner string, limit int) ([]CommitmentRevision, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]CommitmentRevision, 0)
	for key, history := range r.commitments {
		if len(history) == 0 || !ownerKeyPrefix(key, owner) {
			continue
		}
		result = append(result, cloneCommitment(history[len(history)-1]))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ObservedAt.Equal(result[j].ObservedAt) {
			return result[i].CommitmentKey < result[j].CommitmentKey
		}
		return result[i].ObservedAt.After(result[j].ObservedAt)
	})
	return boundCommitments(result, limit), nil
}

func (r *MemoryRepository) ListCommitmentHistory(_ context.Context, owner, key string, limit int) ([]CommitmentRevision, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	history := r.commitments[ledgerKey(owner, key)]
	if len(history) == 0 {
		return nil, ErrNotFound
	}
	result := make([]CommitmentRevision, 0, len(history))
	for _, record := range history {
		result = append(result, cloneCommitment(record))
	}
	if len(result) > boundedLimit(limit) {
		result = result[len(result)-boundedLimit(limit):]
	}
	return result, nil
}

func (r *MemoryRepository) AppendCost(_ context.Context, record CostEntry) (CostEntry, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	idempotencyKey := ledgerKey(record.OwnerIdentity, record.IdempotencyKey)
	if existing, ok := r.costIdempotency[idempotencyKey]; ok {
		if existing.RequestDigest != record.RequestDigest {
			return CostEntry{}, false, ErrIdempotencyConflict
		}
		return cloneCost(existing), false, nil
	}
	r.costs[record.OwnerIdentity] = append(r.costs[record.OwnerIdentity], cloneCost(record))
	r.costIdempotency[idempotencyKey] = cloneCost(record)
	return cloneCost(record), true, nil
}

func (r *MemoryRepository) ListCosts(_ context.Context, owner string, limit int) ([]CostEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]CostEntry, 0, len(r.costs[owner]))
	for _, record := range r.costs[owner] {
		result = append(result, cloneCost(record))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ObservedAt.Equal(result[j].ObservedAt) {
			return result[i].ID.String() > result[j].ID.String()
		}
		return result[i].ObservedAt.After(result[j].ObservedAt)
	})
	if len(result) > boundedLimit(limit) {
		result = result[:boundedLimit(limit)]
	}
	return result, nil
}

func ledgerKey(owner, value string) string { return owner + "\x00" + value }
func ownerKeyPrefix(value, owner string) bool {
	return len(value) > len(owner) && value[:len(owner)+1] == owner+"\x00"
}

func boundedLimit(value int) int {
	if value <= 0 {
		return 50
	}
	if value > 200 {
		return 200
	}
	return value
}

func boundCommitments(values []CommitmentRevision, limit int) []CommitmentRevision {
	if len(values) > boundedLimit(limit) {
		return values[:boundedLimit(limit)]
	}
	return values
}

func cloneCommitment(value CommitmentRevision) CommitmentRevision {
	value.Evidence = append([]EvidenceReference(nil), value.Evidence...)
	value.DueAt = normalizedTimePointer(value.DueAt)
	value.LifeGraph = nil
	return value
}

func cloneCost(value CostEntry) CostEntry {
	value.Evidence = append([]EvidenceReference(nil), value.Evidence...)
	value.LifeGraph = nil
	return value
}
