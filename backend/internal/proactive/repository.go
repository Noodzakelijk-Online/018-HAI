package proactive

import (
	"context"
	"sort"
	"sync"
	"time"
)

type Repository interface {
	PutRule(context.Context, TriggerRule) error
	GetRule(context.Context, string, string) (TriggerRule, error)
	CreateProposal(context.Context, Proposal) error
	GetProposal(context.Context, string, string) (Proposal, error)
	FindByIdempotency(context.Context, string, string) (Proposal, error)
	LatestByOpenLoop(context.Context, string, string) (Proposal, error)
	ListProposals(context.Context, string, ProposalFilter) ([]Proposal, error)
	CompareAndSwapProposal(context.Context, Proposal, uint64) (Proposal, error)
	MarkOpenLoopResolved(context.Context, string, string) error
	IsOpenLoopResolved(context.Context, string, string) (bool, error)
	GetWeights(context.Context, string) (ScoreWeights, error)
	PutWeights(context.Context, string, ScoreWeights) error
	AppendFeedback(context.Context, Feedback) error
}

type MemoryRepository struct {
	mu          sync.RWMutex
	rules       map[string]TriggerRule
	proposals   map[string]Proposal
	idempotency map[string]string
	resolved    map[string]bool
	weights     map[string]ScoreWeights
	feedback    map[string][]Feedback
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		rules:       make(map[string]TriggerRule),
		proposals:   make(map[string]Proposal),
		idempotency: make(map[string]string),
		resolved:    make(map[string]bool),
		weights:     make(map[string]ScoreWeights),
		feedback:    make(map[string][]Feedback),
	}
}

func (r *MemoryRepository) PutRule(_ context.Context, rule TriggerRule) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := ownerKey(rule.OwnerIdentity, rule.ID)
	if current, exists := r.rules[key]; exists && current.Version >= rule.Version {
		return ErrConflict
	}
	r.rules[key] = cloneRule(rule)
	return nil
}

func (r *MemoryRepository) GetRule(_ context.Context, owner, id string) (TriggerRule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rule, exists := r.rules[ownerKey(owner, id)]
	if !exists {
		return TriggerRule{}, ErrNotFound
	}
	return cloneRule(rule), nil
}

func (r *MemoryRepository) CreateProposal(_ context.Context, proposal Proposal) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := ownerKey(proposal.OwnerIdentity, proposal.ID)
	if _, exists := r.proposals[key]; exists {
		return ErrAlreadyExists
	}
	idempotencyKey := ownerKey(proposal.OwnerIdentity, proposal.IdempotencyKey)
	if existingID, exists := r.idempotency[idempotencyKey]; exists {
		existing := r.proposals[ownerKey(proposal.OwnerIdentity, existingID)]
		if existing.SignalDigest != proposal.SignalDigest {
			return ErrIdempotencyConflict
		}
		return ErrAlreadyExists
	}
	r.proposals[key] = cloneProposal(proposal)
	r.idempotency[idempotencyKey] = proposal.ID
	return nil
}

func (r *MemoryRepository) GetProposal(_ context.Context, owner, id string) (Proposal, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	proposal, exists := r.proposals[ownerKey(owner, id)]
	if !exists {
		return Proposal{}, ErrNotFound
	}
	return cloneProposal(proposal), nil
}

func (r *MemoryRepository) FindByIdempotency(_ context.Context, owner, idempotencyKey string) (Proposal, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, exists := r.idempotency[ownerKey(owner, idempotencyKey)]
	if !exists {
		return Proposal{}, ErrNotFound
	}
	proposal, exists := r.proposals[ownerKey(owner, id)]
	if !exists {
		return Proposal{}, ErrNotFound
	}
	return cloneProposal(proposal), nil
}

func (r *MemoryRepository) LatestByOpenLoop(_ context.Context, owner, openLoopKey string) (Proposal, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var latest Proposal
	found := false
	for _, proposal := range r.proposals {
		if proposal.OwnerIdentity != owner || proposal.OpenLoopKey != openLoopKey {
			continue
		}
		if !found || proposal.CreatedAt.After(latest.CreatedAt) ||
			(proposal.CreatedAt.Equal(latest.CreatedAt) && proposal.ID < latest.ID) {
			latest = proposal
			found = true
		}
	}
	if !found {
		return Proposal{}, ErrNotFound
	}
	return cloneProposal(latest), nil
}

func (r *MemoryRepository) ListProposals(_ context.Context, owner string, filter ProposalFilter) ([]Proposal, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	statuses := make(map[ProposalStatus]struct{}, len(filter.Statuses))
	for _, status := range filter.Statuses {
		statuses[status] = struct{}{}
	}
	result := make([]Proposal, 0)
	for _, proposal := range r.proposals {
		if proposal.OwnerIdentity != owner {
			continue
		}
		if len(statuses) > 0 {
			if _, allowed := statuses[proposal.Status]; !allowed {
				continue
			}
		}
		result = append(result, cloneProposal(proposal))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Score.Total != result[j].Score.Total {
			return result[i].Score.Total > result[j].Score.Total
		}
		if !result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].CreatedAt.Before(result[j].CreatedAt)
		}
		return result[i].ID < result[j].ID
	})
	if filter.Limit > 0 && len(result) > filter.Limit {
		result = result[:filter.Limit]
	}
	return result, nil
}

func (r *MemoryRepository) CompareAndSwapProposal(_ context.Context, proposal Proposal, expected uint64) (Proposal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := ownerKey(proposal.OwnerIdentity, proposal.ID)
	current, exists := r.proposals[key]
	if !exists {
		return Proposal{}, ErrNotFound
	}
	if current.Revision != expected || proposal.Revision != expected+1 {
		return Proposal{}, ErrConflict
	}
	r.proposals[key] = cloneProposal(proposal)
	return cloneProposal(proposal), nil
}

func (r *MemoryRepository) MarkOpenLoopResolved(_ context.Context, owner, openLoopKey string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resolved[ownerKey(owner, openLoopKey)] = true
	return nil
}

func (r *MemoryRepository) IsOpenLoopResolved(_ context.Context, owner, openLoopKey string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.resolved[ownerKey(owner, openLoopKey)], nil
}

func (r *MemoryRepository) GetWeights(_ context.Context, owner string) (ScoreWeights, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	weights, exists := r.weights[owner]
	if !exists {
		return ScoreWeights{}, ErrNotFound
	}
	return weights, nil
}

func (r *MemoryRepository) PutWeights(_ context.Context, owner string, weights ScoreWeights) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.weights[owner] = weights
	return nil
}

func (r *MemoryRepository) AppendFeedback(_ context.Context, feedback Feedback) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.feedback[feedback.OwnerIdentity] = append(r.feedback[feedback.OwnerIdentity], feedback)
	return nil
}

func ownerKey(owner, id string) string {
	return owner + "\x00" + id
}

func cloneRule(rule TriggerRule) TriggerRule {
	rule.SignalTypes = append([]SignalType(nil), rule.SignalTypes...)
	rule.Retry.Intervals = append([]time.Duration(nil), rule.Retry.Intervals...)
	return rule
}

func cloneProposal(proposal Proposal) Proposal {
	proposal.Evidence = append([]EvidenceSnapshot(nil), proposal.Evidence...)
	proposal.Score.Components = append([]ScoreComponent(nil), proposal.Score.Components...)
	if proposal.SnoozedUntil != nil {
		value := *proposal.SnoozedUntil
		proposal.SnoozedUntil = &value
	}
	if proposal.NextReviewAt != nil {
		value := *proposal.NextReviewAt
		proposal.NextReviewAt = &value
	}
	return proposal
}
