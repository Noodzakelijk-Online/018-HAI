package executionauth

import (
	"context"
	"sort"
	"sync"

	"github.com/google/uuid"
)

type MemoryRepository struct {
	mu           sync.RWMutex
	receipts     map[string]Receipt
	byID         map[string]string
	consumptions map[string]Consumption
	exercises    map[string]FinalEffectExercise
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		receipts:     map[string]Receipt{},
		byID:         map[string]string{},
		consumptions: map[string]Consumption{},
		exercises:    map[string]FinalEffectExercise{},
	}
}

func (r *MemoryRepository) ExerciseFinalEffect(
	_ context.Context,
	value FinalEffectExercise,
) error {
	if err := validateFinalEffectExercise(value); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := ownerKey(value.OwnerIdentity, value.ReceiptID.String())
	receiptKey, ok := r.byID[key]
	if !ok {
		return ErrNotFound
	}
	receipt := r.receipts[receiptKey]
	consumption, consumed := r.consumptions[key]
	if !consumed {
		return ErrNotAuthorized
	}
	if _, exists := r.exercises[key]; exists {
		return ErrAlreadyExercised
	}
	if !finalEffectMatches(receipt, consumption, value) {
		return ErrFinalEffectMismatch
	}
	r.exercises[key] = value
	return nil
}

func (r *MemoryRepository) GetFinalEffectExercise(
	_ context.Context,
	owner string,
	receiptID uuid.UUID,
) (FinalEffectExercise, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.exercises[ownerKey(owner, receiptID.String())]
	if !ok {
		return FinalEffectExercise{}, ErrNotFound
	}
	return value, nil
}

func (r *MemoryRepository) CreateOrGet(_ context.Context, receipt Receipt) (Receipt, bool, error) {
	if err := validateReceipt(receipt); err != nil {
		return Receipt{}, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := ownerKey(receipt.OwnerIdentity, receipt.IdempotencyKey)
	if existing, ok := r.receipts[key]; ok {
		if existing.RequestDigest != receipt.RequestDigest {
			return Receipt{}, false, ErrIdempotencyConflict
		}
		return cloneReceipt(existing), false, nil
	}
	r.receipts[key] = cloneReceipt(receipt)
	r.byID[ownerKey(receipt.OwnerIdentity, receipt.ID.String())] = key
	return cloneReceipt(receipt), true, nil
}

func (r *MemoryRepository) Get(_ context.Context, owner string, id uuid.UUID) (Receipt, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key, ok := r.byID[ownerKey(owner, id.String())]
	if !ok {
		return Receipt{}, ErrNotFound
	}
	return cloneReceipt(r.receipts[key]), nil
}

func (r *MemoryRepository) List(_ context.Context, owner string, limit int) ([]Receipt, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Receipt, 0)
	for _, receipt := range r.receipts {
		if receipt.OwnerIdentity == owner {
			result = append(result, cloneReceipt(receipt))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].EvaluatedAt.Equal(result[j].EvaluatedAt) {
			return result[i].ID.String() > result[j].ID.String()
		}
		return result[i].EvaluatedAt.After(result[j].EvaluatedAt)
	})
	limit = boundedLimit(limit)
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (r *MemoryRepository) Consume(_ context.Context, value Consumption) error {
	if err := validateConsumption(value); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := ownerKey(value.OwnerIdentity, value.ReceiptID.String())
	receiptKey, ok := r.byID[key]
	if !ok {
		return ErrNotFound
	}
	receipt := r.receipts[receiptKey]
	if receipt.Outcome != OutcomeAuthorized || receipt.DecisionDigest != value.ReceiptDigest {
		return ErrNotAuthorized
	}
	if _, exists := r.consumptions[key]; exists {
		return ErrAlreadyConsumed
	}
	r.consumptions[key] = value
	return nil
}

func (r *MemoryRepository) GetConsumption(
	_ context.Context,
	owner string,
	receiptID uuid.UUID,
) (Consumption, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.consumptions[ownerKey(owner, receiptID.String())]
	if !ok {
		return Consumption{}, ErrNotFound
	}
	return value, nil
}

func ownerKey(owner, id string) string { return owner + "\x00" + id }

func boundedLimit(value int) int {
	if value <= 0 {
		return 50
	}
	if value > 200 {
		return 200
	}
	return value
}

func cloneReceipt(value Receipt) Receipt {
	value.Evidence.Constitution.RequestedCapabilities = append(
		[]string(nil),
		value.Evidence.Constitution.RequestedCapabilities...,
	)
	value.Evidence.Constitution.DeniedCapabilities = append(
		[]string(nil),
		value.Evidence.Constitution.DeniedCapabilities...,
	)
	value.Evidence.Constitution.ApprovalRequiredCapabilities = append(
		[]string(nil),
		value.Evidence.Constitution.ApprovalRequiredCapabilities...,
	)
	if value.Evidence.Governance.FrameworkMaximumAutonomyLevel != nil {
		maximumAutonomy := *value.Evidence.Governance.FrameworkMaximumAutonomyLevel
		value.Evidence.Governance.FrameworkMaximumAutonomyLevel = &maximumAutonomy
	}
	if value.Evidence.Governance.FrameworkRequiresApproval != nil {
		requiresApproval := *value.Evidence.Governance.FrameworkRequiresApproval
		value.Evidence.Governance.FrameworkRequiresApproval = &requiresApproval
	}
	value.Evidence.Governance.EvidenceReferences = append(
		[]string(nil),
		value.Evidence.Governance.EvidenceReferences...,
	)
	value.Evidence.ReasonCodes = append([]string(nil), value.Evidence.ReasonCodes...)
	value.Evidence.Trace = append([]string(nil), value.Evidence.Trace...)
	return value
}
