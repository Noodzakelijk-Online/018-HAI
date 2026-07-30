package agentregistry

import (
	"context"
	"sort"
	"sync"
)

type Repository interface {
	Create(context.Context, Agent) (Agent, error)
	Get(context.Context, string, string) (Agent, error)
	List(context.Context, string) ([]Agent, error)
	CompareAndSwap(context.Context, Agent, uint64) (Agent, error)
	AppendTransition(context.Context, string, string, Transition) error
	ListTransitions(context.Context, string, string) ([]Transition, error)
	CreateAssignment(context.Context, Assignment) error
	GetAssignment(context.Context, string, string) (Assignment, error)
}

type MemoryRepository struct {
	mu          sync.RWMutex
	agents      map[string]Agent
	transitions map[string][]Transition
	assignments map[string]Assignment
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		agents:      make(map[string]Agent),
		transitions: make(map[string][]Transition),
		assignments: make(map[string]Assignment),
	}
}

func (r *MemoryRepository) Create(_ context.Context, agent Agent) (Agent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := ownerKey(agent.OwnerIdentity, agent.ID)
	if _, exists := r.agents[key]; exists {
		return Agent{}, ErrAlreadyExists
	}
	r.agents[key] = cloneAgent(agent)
	return cloneAgent(agent), nil
}

func (r *MemoryRepository) Get(_ context.Context, owner, id string) (Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	agent, exists := r.agents[ownerKey(owner, id)]
	if !exists {
		return Agent{}, ErrNotFound
	}
	return cloneAgent(agent), nil
}

func (r *MemoryRepository) List(_ context.Context, owner string) ([]Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Agent, 0)
	for _, agent := range r.agents {
		if agent.OwnerIdentity == owner {
			result = append(result, cloneAgent(agent))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (r *MemoryRepository) CompareAndSwap(_ context.Context, agent Agent, expected uint64) (Agent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := ownerKey(agent.OwnerIdentity, agent.ID)
	current, exists := r.agents[key]
	if !exists {
		return Agent{}, ErrNotFound
	}
	if current.Revision != expected {
		return Agent{}, ErrConflict
	}
	if agent.Revision != expected+1 {
		return Agent{}, ErrConflict
	}
	r.agents[key] = cloneAgent(agent)
	return cloneAgent(agent), nil
}

func (r *MemoryRepository) AppendTransition(_ context.Context, owner, id string, transition Transition) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.agents[ownerKey(owner, id)]; !exists {
		return ErrNotFound
	}
	key := ownerKey(owner, id)
	r.transitions[key] = append(r.transitions[key], transition)
	return nil
}

func (r *MemoryRepository) ListTransitions(_ context.Context, owner, id string) ([]Transition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, exists := r.agents[ownerKey(owner, id)]; !exists {
		return nil, ErrNotFound
	}
	values := r.transitions[ownerKey(owner, id)]
	return append([]Transition(nil), values...), nil
}

func (r *MemoryRepository) CreateAssignment(_ context.Context, assignment Assignment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := ownerKey(assignment.OwnerIdentity, assignment.ID)
	if _, exists := r.assignments[key]; exists {
		return ErrAssignmentExists
	}
	r.assignments[key] = cloneAssignment(assignment)
	return nil
}

func (r *MemoryRepository) GetAssignment(_ context.Context, owner, id string) (Assignment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	assignment, exists := r.assignments[ownerKey(owner, id)]
	if !exists {
		return Assignment{}, ErrNotFound
	}
	return cloneAssignment(assignment), nil
}

func ownerKey(owner, id string) string {
	return owner + "\x00" + id
}

func cloneAgent(agent Agent) Agent {
	agent.Capabilities = append([]CapabilityDeclaration(nil), agent.Capabilities...)
	for i := range agent.Capabilities {
		agent.Capabilities[i].Operations = append([]string(nil), agent.Capabilities[i].Operations...)
	}
	agent.ToolAllowlist = append([]string(nil), agent.ToolAllowlist...)
	agent.DataAllowlist = append([]string(nil), agent.DataAllowlist...)
	agent.FolderAllowlist = append([]string(nil), agent.FolderAllowlist...)
	return agent
}

func cloneAssignment(value Assignment) Assignment {
	value.Explanation.Components = append([]ScoreComponent(nil), value.Explanation.Components...)
	value.Explanation.Constraints = append([]string(nil), value.Explanation.Constraints...)
	return value
}
