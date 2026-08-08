package controlledlearning

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

type Repository interface {
	CreateOutcome(context.Context, OutcomeRecord) (OutcomeRecord, error)
	GetOutcome(context.Context, string, string) (OutcomeRecord, error)
	ListOutcomes(context.Context, OutcomeQuery) ([]OutcomeRecord, error)
	CreateProposal(context.Context, LearningProposal) (LearningProposal, error)
	GetProposal(context.Context, string, string) (LearningProposal, error)
	ListProposals(context.Context, ProposalQuery) ([]LearningProposal, error)
	DecideProposal(context.Context, string, string, int64, ReviewDecision, ProposalStatus) (LearningProposal, error)
	ListDecisions(context.Context, string, string) ([]ReviewDecision, error)
}

type MemoryRepository struct {
	mu                  sync.RWMutex
	outcomes            map[string]OutcomeRecord
	outcomeIdempotency  map[string]string
	proposals           map[string]LearningProposal
	proposalIdempotency map[string]string
	decisions           map[string][]ReviewDecision
	applications        map[string]ApplicationRecord
	applicationByKey    map[string]string
	applicationEvents   map[string][]ApplicationEvent
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		outcomes:            map[string]OutcomeRecord{},
		outcomeIdempotency:  map[string]string{},
		proposals:           map[string]LearningProposal{},
		proposalIdempotency: map[string]string{},
		decisions:           map[string][]ReviewDecision{},
		applications:        map[string]ApplicationRecord{},
		applicationByKey:    map[string]string{},
		applicationEvents:   map[string][]ApplicationEvent{},
	}
}

func (repository *MemoryRepository) CreateOutcome(ctx context.Context, record OutcomeRecord) (OutcomeRecord, error) {
	if err := ctx.Err(); err != nil {
		return OutcomeRecord{}, err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	idempotency := scopedKey(record.OwnerIdentity, record.IdempotencyKey)
	if existingID, exists := repository.outcomeIdempotency[idempotency]; exists {
		existing := repository.outcomes[scopedKey(record.OwnerIdentity, existingID)]
		if existing.EvidenceDigest != record.EvidenceDigest {
			return OutcomeRecord{}, ErrIdempotencyConflict
		}
		return cloneOutcome(existing), nil
	}
	key := scopedKey(record.OwnerIdentity, record.ID)
	if _, exists := repository.outcomes[key]; exists {
		return OutcomeRecord{}, ErrIdempotencyConflict
	}
	repository.outcomes[key] = cloneOutcome(record)
	repository.outcomeIdempotency[idempotency] = record.ID
	return cloneOutcome(record), nil
}

func (repository *MemoryRepository) GetOutcome(ctx context.Context, ownerIdentity, id string) (OutcomeRecord, error) {
	if err := ctx.Err(); err != nil {
		return OutcomeRecord{}, err
	}
	owner := strings.TrimSpace(ownerIdentity)
	if owner == "" {
		return OutcomeRecord{}, ErrOwnerScopeViolation
	}
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	record, exists := repository.outcomes[scopedKey(owner, id)]
	if !exists {
		return OutcomeRecord{}, ErrNotFound
	}
	return cloneOutcome(record), nil
}

func (repository *MemoryRepository) ListOutcomes(ctx context.Context, query OutcomeQuery) ([]OutcomeRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	owner := strings.TrimSpace(query.OwnerIdentity)
	if owner == "" {
		return nil, ErrOwnerScopeViolation
	}
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	result := make([]OutcomeRecord, 0)
	for _, record := range repository.outcomes {
		if record.OwnerIdentity != owner {
			continue
		}
		if query.OperationID != "" && record.OperationID != query.OperationID {
			continue
		}
		result = append(result, cloneOutcome(record))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].RecordedAt.Equal(result[j].RecordedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].RecordedAt.After(result[j].RecordedAt)
	})
	return limitOutcomes(result, query.Limit), nil
}

func (repository *MemoryRepository) CreateProposal(ctx context.Context, proposal LearningProposal) (LearningProposal, error) {
	if err := ctx.Err(); err != nil {
		return LearningProposal{}, err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	idempotency := scopedKey(proposal.OwnerIdentity, proposal.IdempotencyKey)
	if existingID, exists := repository.proposalIdempotency[idempotency]; exists {
		existing := repository.proposals[scopedKey(proposal.OwnerIdentity, existingID)]
		if existing.ProposalDigest != proposal.ProposalDigest {
			return LearningProposal{}, ErrIdempotencyConflict
		}
		return cloneProposal(existing), nil
	}
	key := scopedKey(proposal.OwnerIdentity, proposal.ID)
	if _, exists := repository.proposals[key]; exists {
		return LearningProposal{}, ErrIdempotencyConflict
	}
	repository.proposals[key] = cloneProposal(proposal)
	repository.proposalIdempotency[idempotency] = proposal.ID
	return cloneProposal(proposal), nil
}

func (repository *MemoryRepository) GetProposal(ctx context.Context, ownerIdentity, id string) (LearningProposal, error) {
	if err := ctx.Err(); err != nil {
		return LearningProposal{}, err
	}
	owner := strings.TrimSpace(ownerIdentity)
	if owner == "" {
		return LearningProposal{}, ErrOwnerScopeViolation
	}
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	proposal, exists := repository.proposals[scopedKey(owner, id)]
	if !exists {
		return LearningProposal{}, ErrNotFound
	}
	return cloneProposal(proposal), nil
}

func (repository *MemoryRepository) ListProposals(ctx context.Context, query ProposalQuery) ([]LearningProposal, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	owner := strings.TrimSpace(query.OwnerIdentity)
	if owner == "" {
		return nil, ErrOwnerScopeViolation
	}
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	result := make([]LearningProposal, 0)
	for _, proposal := range repository.proposals {
		if proposal.OwnerIdentity != owner {
			continue
		}
		if query.Status != "" && proposal.Status != query.Status {
			continue
		}
		result = append(result, cloneProposal(proposal))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].UpdatedAt.Equal(result[j].UpdatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})
	return limitProposals(result, query.Limit), nil
}

func (repository *MemoryRepository) DecideProposal(
	ctx context.Context,
	ownerIdentity string,
	proposalID string,
	expectedRevision int64,
	decision ReviewDecision,
	nextStatus ProposalStatus,
) (LearningProposal, error) {
	if err := ctx.Err(); err != nil {
		return LearningProposal{}, err
	}
	owner := strings.TrimSpace(ownerIdentity)
	if owner == "" {
		return LearningProposal{}, ErrOwnerScopeViolation
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	key := scopedKey(owner, proposalID)
	proposal, exists := repository.proposals[key]
	if !exists {
		return LearningProposal{}, ErrNotFound
	}
	if proposal.Revision != expectedRevision {
		return LearningProposal{}, ErrRevisionConflict
	}
	if decision.OwnerIdentity != owner || decision.ProposalID != proposalID ||
		decision.ProposalDigest != proposal.ProposalDigest {
		return LearningProposal{}, ErrOwnerScopeViolation
	}
	if err := verifyReviewDecisionIntegrity(decision); err != nil {
		return LearningProposal{}, err
	}
	if err := validateDecisionTransition(proposal, decision, nextStatus); err != nil {
		return LearningProposal{}, err
	}
	if decision.Kind == DecisionApprove ||
		decision.Kind == DecisionEscalateGovernance ||
		nextStatus == ProposalApproved ||
		nextStatus == ProposalGovernanceReview {
		return LearningProposal{}, ErrInvalidStateChange
	}
	proposal.Status = nextStatus
	proposal.Revision++
	proposal.UpdatedAt = decision.DecidedAt
	repository.proposals[key] = cloneProposal(proposal)
	repository.decisions[key] = append(repository.decisions[key], cloneDecision(decision))
	return cloneProposal(proposal), nil
}

func (repository *MemoryRepository) ListDecisions(ctx context.Context, ownerIdentity, proposalID string) ([]ReviewDecision, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	owner := strings.TrimSpace(ownerIdentity)
	if owner == "" {
		return nil, ErrOwnerScopeViolation
	}
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	key := scopedKey(owner, proposalID)
	if _, exists := repository.proposals[key]; !exists {
		return nil, ErrNotFound
	}
	items := repository.decisions[key]
	result := make([]ReviewDecision, len(items))
	for index := range items {
		result[index] = cloneDecision(items[index])
	}
	return result, nil
}

func scopedKey(ownerIdentity, id string) string {
	return strings.TrimSpace(ownerIdentity) + "\x00" + strings.TrimSpace(id)
}

func limitOutcomes(values []OutcomeRecord, requested int) []OutcomeRecord {
	limit := normalizedLimit(requested)
	if len(values) > limit {
		values = values[:limit]
	}
	return values
}

func limitProposals(values []LearningProposal, requested int) []LearningProposal {
	limit := normalizedLimit(requested)
	if len(values) > limit {
		values = values[:limit]
	}
	return values
}

func normalizedLimit(requested int) int {
	if requested <= 0 {
		return 100
	}
	if requested > 500 {
		return 500
	}
	return requested
}

func cloneOutcome(value OutcomeRecord) OutcomeRecord {
	copy := value
	copy.DomainPackIDs = append([]string(nil), value.DomainPackIDs...)
	copy.Sources = append([]SourceReference(nil), value.Sources...)
	copy.Criteria = make([]CriterionResult, len(value.Criteria))
	for index := range value.Criteria {
		copy.Criteria[index] = value.Criteria[index]
		copy.Criteria[index].SourceIDs = append([]string(nil), value.Criteria[index].SourceIDs...)
	}
	copy.Metrics = append([]MetricResult(nil), value.Metrics...)
	copy.Tags = append([]string(nil), value.Tags...)
	copy.Reconciliation.FailedCriteria = append([]string(nil), value.Reconciliation.FailedCriteria...)
	copy.Reconciliation.DriftSignals = append([]string(nil), value.Reconciliation.DriftSignals...)
	copy.Reconciliation.SuggestedMethods = append([]LearningMethod(nil), value.Reconciliation.SuggestedMethods...)
	return copy
}

func cloneProposal(value LearningProposal) LearningProposal {
	copy := value
	copy.EvidenceIDs = append([]string(nil), value.EvidenceIDs...)
	return copy
}

func cloneDecision(value ReviewDecision) ReviewDecision {
	return value
}

func cloneApplication(value ApplicationRecord) ApplicationRecord {
	copy := value
	copy.Evidence = append([]ApplicationEvidence(nil), value.Evidence...)
	copy.RollbackEvidence = append([]ApplicationEvidence(nil), value.RollbackEvidence...)
	return copy
}

func cloneApplicationEvent(value ApplicationEvent) ApplicationEvent {
	copy := value
	copy.Evidence = append([]ApplicationEvidence(nil), value.Evidence...)
	return copy
}

func validateRepository(repository Repository) error {
	if repository == nil {
		return fmt.Errorf("controlled learning repository is required")
	}
	return nil
}
