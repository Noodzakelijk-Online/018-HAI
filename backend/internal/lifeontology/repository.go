package lifeontology

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

var (
	ErrNotFound       = errors.New("life ontology record not found")
	ErrExists         = errors.New("life ontology record already exists")
	ErrCorruptStorage = errors.New("life ontology storage is corrupt")
)

// MemoryRepository is an append-only, owner-scoped repository suitable for a
// local-first read model. Every read revalidates signed envelopes so corrupted
// or mutated storage fails closed.
type MemoryRepository struct {
	mu        sync.RWMutex
	entities  map[string]Entity
	relations map[string]Relation
	proposals map[string]MergeProposal
	decisions map[string]ContactReviewDecision
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		entities: make(map[string]Entity), relations: make(map[string]Relation), proposals: make(map[string]MergeProposal),
		decisions: make(map[string]ContactReviewDecision),
	}
}

func (r *MemoryRepository) AppendEntity(_ context.Context, entity Entity) (Entity, error) {
	if r == nil {
		return Entity{}, fmt.Errorf("repository is unavailable")
	}
	if err := validateStoredEntity(entity); err != nil {
		return Entity{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.entities[entity.ID]; exists {
		return Entity{}, ErrExists
	}
	r.entities[entity.ID] = cloneEntity(entity)
	return cloneEntity(entity), nil
}

func (r *MemoryRepository) GetEntity(_ context.Context, owner, id string) (Entity, error) {
	if r == nil {
		return Entity{}, fmt.Errorf("repository is unavailable")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	entity, exists := r.entities[strings.TrimSpace(id)]
	if !exists || entity.OwnerIdentity != strings.TrimSpace(owner) {
		return Entity{}, ErrNotFound
	}
	if err := validateStoredEntity(entity); err != nil {
		return Entity{}, err
	}
	return cloneEntity(entity), nil
}

func (r *MemoryRepository) ListEntities(_ context.Context, owner string) ([]Entity, error) {
	if r == nil {
		return nil, fmt.Errorf("repository is unavailable")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	owner = strings.TrimSpace(owner)
	result := make([]Entity, 0)
	for _, entity := range r.entities {
		if entity.OwnerIdentity != owner {
			continue
		}
		if err := validateStoredEntity(entity); err != nil {
			return nil, err
		}
		result = append(result, cloneEntity(entity))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (r *MemoryRepository) AppendRelation(_ context.Context, relation Relation) (Relation, error) {
	if r == nil {
		return Relation{}, fmt.Errorf("repository is unavailable")
	}
	if err := validateStoredRelation(relation); err != nil {
		return Relation{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.relations[relation.ID]; exists {
		return Relation{}, ErrExists
	}
	r.relations[relation.ID] = cloneRelation(relation)
	return cloneRelation(relation), nil
}

func (r *MemoryRepository) GetRelation(_ context.Context, owner, id string) (Relation, error) {
	if r == nil {
		return Relation{}, fmt.Errorf("repository is unavailable")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	relation, exists := r.relations[strings.TrimSpace(id)]
	if !exists || relation.OwnerIdentity != strings.TrimSpace(owner) {
		return Relation{}, ErrNotFound
	}
	if err := validateStoredRelation(relation); err != nil {
		return Relation{}, err
	}
	return cloneRelation(relation), nil
}

func (r *MemoryRepository) ListRelations(_ context.Context, owner string) ([]Relation, error) {
	if r == nil {
		return nil, fmt.Errorf("repository is unavailable")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	owner = strings.TrimSpace(owner)
	result := make([]Relation, 0)
	for _, relation := range r.relations {
		if relation.OwnerIdentity != owner {
			continue
		}
		if err := validateStoredRelation(relation); err != nil {
			return nil, err
		}
		result = append(result, cloneRelation(relation))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (r *MemoryRepository) AppendMergeProposal(_ context.Context, proposal MergeProposal) (MergeProposal, error) {
	if r == nil {
		return MergeProposal{}, fmt.Errorf("repository is unavailable")
	}
	if err := validateMergeProposal(proposal); err != nil {
		return MergeProposal{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, exists := r.proposals[proposal.ID]; exists {
		return cloneProposal(existing), ErrExists
	}
	r.proposals[proposal.ID] = cloneProposal(proposal)
	return cloneProposal(proposal), nil
}

func (r *MemoryRepository) ListMergeProposals(_ context.Context, owner string) ([]MergeProposal, error) {
	if r == nil {
		return nil, fmt.Errorf("repository is unavailable")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	owner = strings.TrimSpace(owner)
	result := make([]MergeProposal, 0)
	for _, proposal := range r.proposals {
		if proposal.OwnerIdentity != owner {
			continue
		}
		if err := validateMergeProposal(proposal); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrCorruptStorage, err)
		}
		result = append(result, cloneProposal(proposal))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (r *MemoryRepository) GetMergeProposal(_ context.Context, owner, id string) (MergeProposal, error) {
	if r == nil {
		return MergeProposal{}, fmt.Errorf("repository is unavailable")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	proposal, exists := r.proposals[strings.TrimSpace(id)]
	if !exists || proposal.OwnerIdentity != strings.TrimSpace(owner) {
		return MergeProposal{}, ErrNotFound
	}
	if err := validateMergeProposal(proposal); err != nil {
		return MergeProposal{}, fmt.Errorf("%w: %v", ErrCorruptStorage, err)
	}
	return cloneProposal(proposal), nil
}

func (r *MemoryRepository) AppendContactReviewDecision(_ context.Context, decision ContactReviewDecision, canonical *Entity) (ContactReviewDecision, error) {
	if r == nil {
		return ContactReviewDecision{}, fmt.Errorf("repository is unavailable")
	}
	if err := validateContactReviewDecision(decision); err != nil {
		return ContactReviewDecision{}, err
	}
	if canonical != nil {
		if err := validateStoredEntity(*canonical); err != nil {
			return ContactReviewDecision{}, err
		}
		if canonical.OwnerIdentity != decision.OwnerIdentity || canonical.ID != decision.CanonicalEntityID {
			return ContactReviewDecision{}, fmt.Errorf("canonical contact does not match review decision")
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.decisions {
		if existing.OwnerIdentity == decision.OwnerIdentity && existing.IdempotencyKey == decision.IdempotencyKey {
			return cloneContactReviewDecision(existing), ErrExists
		}
		if existing.OwnerIdentity == decision.OwnerIdentity && existing.Subject == decision.Subject && existing.SubjectID == decision.SubjectID {
			return cloneContactReviewDecision(existing), ErrExists
		}
	}
	if _, exists := r.decisions[decision.ID]; exists {
		return ContactReviewDecision{}, ErrExists
	}
	if canonical != nil {
		if existing, exists := r.entities[canonical.ID]; exists {
			if existing.EntityDigest != canonical.EntityDigest {
				return ContactReviewDecision{}, fmt.Errorf("%w: canonical contact identity collision", ErrCorruptStorage)
			}
		} else {
			r.entities[canonical.ID] = cloneEntity(*canonical)
		}
	}
	r.decisions[decision.ID] = cloneContactReviewDecision(decision)
	return cloneContactReviewDecision(decision), nil
}

func (r *MemoryRepository) GetContactReviewDecisionByIdempotency(_ context.Context, owner, key string) (ContactReviewDecision, error) {
	if r == nil {
		return ContactReviewDecision{}, fmt.Errorf("repository is unavailable")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	owner, key = strings.TrimSpace(owner), strings.TrimSpace(key)
	for _, decision := range r.decisions {
		if decision.OwnerIdentity == owner && decision.IdempotencyKey == key {
			if err := validateContactReviewDecision(decision); err != nil {
				return ContactReviewDecision{}, fmt.Errorf("%w: %v", ErrCorruptStorage, err)
			}
			return cloneContactReviewDecision(decision), nil
		}
	}
	return ContactReviewDecision{}, ErrNotFound
}

func (r *MemoryRepository) ListContactReviewDecisions(_ context.Context, owner string, limit int) ([]ContactReviewDecision, error) {
	if r == nil {
		return nil, fmt.Errorf("repository is unavailable")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	owner = strings.TrimSpace(owner)
	result := make([]ContactReviewDecision, 0)
	for _, decision := range r.decisions {
		if decision.OwnerIdentity != owner {
			continue
		}
		if err := validateContactReviewDecision(decision); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrCorruptStorage, err)
		}
		result = append(result, cloneContactReviewDecision(decision))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].RecordedAt.Equal(result[j].RecordedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].RecordedAt.After(result[j].RecordedAt)
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func cloneEntity(value Entity) Entity {
	value.ExternalKeys = append([]ExternalKey(nil), value.ExternalKeys...)
	value.Attributes = cloneMap(value.Attributes)
	value.Provenance = append([]Provenance(nil), value.Provenance...)
	value.DueAt = cloneTime(value.DueAt)
	value.ValidUntil = cloneTime(value.ValidUntil)
	return value
}

func cloneRelation(value Relation) Relation {
	value.Attributes = cloneMap(value.Attributes)
	value.Provenance = append([]Provenance(nil), value.Provenance...)
	value.ValidUntil = cloneTime(value.ValidUntil)
	return value
}

func cloneProposal(value MergeProposal) MergeProposal {
	value.CandidateEntityIDs = append([]string(nil), value.CandidateEntityIDs...)
	value.Reasons = append([]string(nil), value.Reasons...)
	return value
}

func cloneContactReviewDecision(value ContactReviewDecision) ContactReviewDecision {
	value.CandidateEntityIDs = append([]string(nil), value.CandidateEntityIDs...)
	return value
}

func cloneMap(value map[string]string) map[string]string {
	if value == nil {
		return nil
	}
	result := make(map[string]string, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}
