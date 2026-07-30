package lifeops

import (
	"errors"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("lifeops record not found")

type Repository interface {
	EntityDomainLinks(ownerIdentity, entityType, entityID string) ([]EntityDomainLink, error)
	ReplaceEntityDomainLinks(ownerIdentity, entityType, entityID string, links []EntityDomainLink) error
	SaveNeedObservation(observation NeedObservation) error
	NeedObservations(ownerIdentity string, domainID DomainID, limit int) ([]NeedObservation, error)
	SaveCapacitySnapshot(snapshot CapacitySnapshot) error
	CapacitySnapshots(ownerIdentity string, limit int) ([]CapacitySnapshot, error)
	FindGoal(ownerIdentity string, id uuid.UUID) (*GoalNode, error)
	ListGoals(ownerIdentity string) ([]GoalNode, error)
	SaveGoal(goal GoalNode) error
	SavePriorityAssessment(assessment PriorityAssessment) error
	PriorityAssessments(ownerIdentity, entityType, entityID string, limit int) ([]PriorityAssessment, error)
}

type MemoryRepository struct {
	mu         sync.RWMutex
	links      map[string][]EntityDomainLink
	needs      map[string][]NeedObservation
	capacities map[string][]CapacitySnapshot
	goals      map[string]map[uuid.UUID]GoalNode
	priorities map[string][]PriorityAssessment
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		links:      map[string][]EntityDomainLink{},
		needs:      map[string][]NeedObservation{},
		capacities: map[string][]CapacitySnapshot{},
		goals:      map[string]map[uuid.UUID]GoalNode{},
		priorities: map[string][]PriorityAssessment{},
	}
}

func (r *MemoryRepository) SavePriorityAssessment(assessment PriorityAssessment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.priorities[assessment.OwnerIdentity] = append(
		r.priorities[assessment.OwnerIdentity],
		clonePriorityAssessment(assessment),
	)
	return nil
}

func (r *MemoryRepository) PriorityAssessments(ownerIdentity, entityType, entityID string, limit int) ([]PriorityAssessment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]PriorityAssessment, 0)
	for _, assessment := range r.priorities[ownerIdentity] {
		if entityType != "" && assessment.EntityType != entityType {
			continue
		}
		if entityID != "" && assessment.EntityID != entityID {
			continue
		}
		result = append(result, clonePriorityAssessment(assessment))
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].AssessedAt.After(result[j].AssessedAt)
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (r *MemoryRepository) EntityDomainLinks(ownerIdentity, entityType, entityID string) ([]EntityDomainLink, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneLinks(r.links[entityKey(ownerIdentity, entityType, entityID)]), nil
}

func (r *MemoryRepository) ReplaceEntityDomainLinks(ownerIdentity, entityType, entityID string, links []EntityDomainLink) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.links[entityKey(ownerIdentity, entityType, entityID)] = cloneLinks(links)
	return nil
}

func (r *MemoryRepository) SaveNeedObservation(observation NeedObservation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	owner := observation.OwnerIdentity
	r.needs[owner] = append(r.needs[owner], cloneNeed(observation))
	return nil
}

func (r *MemoryRepository) NeedObservations(ownerIdentity string, domainID DomainID, limit int) ([]NeedObservation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]NeedObservation, 0)
	for _, observation := range r.needs[ownerIdentity] {
		if domainID == "" || observation.DomainID == domainID {
			result = append(result, cloneNeed(observation))
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].ObservedAt.After(result[j].ObservedAt)
	})
	return limitNeeds(result, limit), nil
}

func (r *MemoryRepository) SaveCapacitySnapshot(snapshot CapacitySnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	owner := snapshot.OwnerIdentity
	r.capacities[owner] = append(r.capacities[owner], cloneCapacity(snapshot))
	return nil
}

func (r *MemoryRepository) CapacitySnapshots(ownerIdentity string, limit int) ([]CapacitySnapshot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]CapacitySnapshot, 0, len(r.capacities[ownerIdentity]))
	for _, snapshot := range r.capacities[ownerIdentity] {
		result = append(result, cloneCapacity(snapshot))
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].CapturedAt.After(result[j].CapturedAt)
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (r *MemoryRepository) FindGoal(ownerIdentity string, id uuid.UUID) (*GoalNode, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	goal, ok := r.goals[ownerIdentity][id]
	if !ok {
		return nil, ErrNotFound
	}
	cloned := cloneGoal(goal)
	return &cloned, nil
}

func (r *MemoryRepository) ListGoals(ownerIdentity string) ([]GoalNode, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]GoalNode, 0, len(r.goals[ownerIdentity]))
	for _, goal := range r.goals[ownerIdentity] {
		result = append(result, cloneGoal(goal))
	}
	sort.SliceStable(result, func(i, j int) bool {
		leftRank, _ := GoalLevelRank(result[i].Level)
		rightRank, _ := GoalLevelRank(result[j].Level)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if !result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].CreatedAt.Before(result[j].CreatedAt)
		}
		return result[i].ID.String() < result[j].ID.String()
	})
	return result, nil
}

func (r *MemoryRepository) SaveGoal(goal GoalNode) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.goals[goal.OwnerIdentity] == nil {
		r.goals[goal.OwnerIdentity] = map[uuid.UUID]GoalNode{}
	}
	r.goals[goal.OwnerIdentity][goal.ID] = cloneGoal(goal)
	return nil
}

func entityKey(ownerIdentity, entityType, entityID string) string {
	return strings.Join([]string{ownerIdentity, entityType, entityID}, "\x00")
}

func cloneLinks(input []EntityDomainLink) []EntityDomainLink {
	result := append([]EntityDomainLink(nil), input...)
	for index := range result {
		result[index].Evidence = append([]string(nil), result[index].Evidence...)
	}
	return result
}

func cloneNeed(input NeedObservation) NeedObservation {
	input.Evidence = append([]string(nil), input.Evidence...)
	return input
}

func cloneCapacity(input CapacitySnapshot) CapacitySnapshot {
	input.Constraints = append([]string(nil), input.Constraints...)
	input.Signals.AvailableTools = append([]string(nil), input.Signals.AvailableTools...)
	input.Signals.AvailableHelpers = append([]string(nil), input.Signals.AvailableHelpers...)
	return input
}

func cloneGoal(input GoalNode) GoalNode {
	input.DomainIDs = append([]DomainID(nil), input.DomainIDs...)
	input.SuccessCriteria = append([]string(nil), input.SuccessCriteria...)
	input.StopConditions = append([]string(nil), input.StopConditions...)
	return input
}

func clonePriorityAssessment(input PriorityAssessment) PriorityAssessment {
	input.Contributions = append([]FactorContribution(nil), input.Contributions...)
	input.Reasons = append([]string(nil), input.Reasons...)
	return input
}

func limitNeeds(input []NeedObservation, limit int) []NeedObservation {
	if limit > 0 && len(input) > limit {
		return input[:limit]
	}
	return input
}
