package knowledgegraph

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

var (
	ErrNotFound = errors.New("knowledge graph entity not found")
	ErrExists   = errors.New("knowledge graph entity already exists")
)

// MemoryRepository is deterministic and copy-safe. It is suitable for tests,
// embedded local operation, and as the reference behavior for durable adapters.
type MemoryRepository struct {
	mu              sync.RWMutex
	nodes           map[string]Node
	edges           map[string]Edge
	deletionSignals []DeletionSignal
	nextNode        uint64
	nextEdge        uint64
	nextSignal      uint64
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		nodes: make(map[string]Node),
		edges: make(map[string]Edge),
	}
}

func (r *MemoryRepository) CreateNode(_ context.Context, node Node) (Node, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if node.ID == "" {
		r.nextNode++
		node.ID = fmt.Sprintf("node-%06d", r.nextNode)
	}
	if _, exists := r.nodes[node.ID]; exists {
		return Node{}, ErrExists
	}
	node = cloneNode(node)
	r.nodes[node.ID] = node
	return cloneNode(node), nil
}

func (r *MemoryRepository) UpdateNode(_ context.Context, node Node) (Node, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, ok := r.nodes[node.ID]
	if !ok || current.OwnerIdentity != node.OwnerIdentity {
		return Node{}, ErrNotFound
	}
	node = cloneNode(node)
	r.nodes[node.ID] = node
	return cloneNode(node), nil
}

func (r *MemoryRepository) GetNode(_ context.Context, ownerIdentity, id string) (Node, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	node, ok := r.nodes[id]
	if !ok || node.OwnerIdentity != ownerIdentity {
		return Node{}, ErrNotFound
	}
	return cloneNode(node), nil
}

func (r *MemoryRepository) ListNodes(_ context.Context, ownerIdentity string, options ListOptions) ([]Node, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Node, 0)
	for _, node := range r.nodes {
		if node.OwnerIdentity != ownerIdentity {
			continue
		}
		if node.Archived && !options.IncludeArchived {
			continue
		}
		if node.DeletedAt != nil && !options.IncludeDeleted {
			continue
		}
		result = append(result, cloneNode(node))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (r *MemoryRepository) CreateEdge(_ context.Context, edge Edge) (Edge, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if edge.ID == "" {
		r.nextEdge++
		edge.ID = fmt.Sprintf("edge-%06d", r.nextEdge)
	}
	if _, exists := r.edges[edge.ID]; exists {
		return Edge{}, ErrExists
	}
	edge = cloneEdge(edge)
	r.edges[edge.ID] = edge
	return cloneEdge(edge), nil
}

func (r *MemoryRepository) UpdateEdge(_ context.Context, edge Edge) (Edge, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, ok := r.edges[edge.ID]
	if !ok || current.OwnerIdentity != edge.OwnerIdentity {
		return Edge{}, ErrNotFound
	}
	edge = cloneEdge(edge)
	r.edges[edge.ID] = edge
	return cloneEdge(edge), nil
}

func (r *MemoryRepository) GetEdge(_ context.Context, ownerIdentity, id string) (Edge, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	edge, ok := r.edges[id]
	if !ok || edge.OwnerIdentity != ownerIdentity {
		return Edge{}, ErrNotFound
	}
	return cloneEdge(edge), nil
}

func (r *MemoryRepository) ListEdges(_ context.Context, ownerIdentity string, options ListOptions) ([]Edge, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Edge, 0)
	for _, edge := range r.edges {
		if edge.OwnerIdentity != ownerIdentity {
			continue
		}
		if edge.Archived && !options.IncludeArchived {
			continue
		}
		if edge.DeletedAt != nil && !options.IncludeDeleted {
			continue
		}
		result = append(result, cloneEdge(edge))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (r *MemoryRepository) DeleteNode(_ context.Context, ownerIdentity, id, reason string, at time.Time) (DeletionSignal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	node, ok := r.nodes[id]
	if !ok || node.OwnerIdentity != ownerIdentity {
		return DeletionSignal{}, ErrNotFound
	}
	if node.DeletedAt != nil {
		for _, signal := range r.deletionSignals {
			if signal.OwnerIdentity == ownerIdentity && signal.EntityType == EntityNode && signal.EntityID == id {
				return cloneDeletionSignal(signal), nil
			}
		}
	}

	node.Archived = true
	node.DeletedAt = timePointer(at)
	node.UpdatedAt = at
	r.nodes[id] = node

	propagated := make([]string, 0)
	for edgeID, edge := range r.edges {
		if edge.OwnerIdentity != ownerIdentity || edge.DeletedAt != nil {
			continue
		}
		if edge.FromNodeID == id || edge.ToNodeID == id {
			edge.Archived = true
			edge.DeletedAt = timePointer(at)
			edge.UpdatedAt = at
			r.edges[edgeID] = edge
			propagated = append(propagated, edgeID)
		}
	}
	sort.Strings(propagated)

	signal := r.newDeletionSignal(ownerIdentity, EntityNode, id, reason, at)
	signal.PropagatedEdgeIDs = propagated
	r.deletionSignals = append(r.deletionSignals, signal)
	return cloneDeletionSignal(signal), nil
}

func (r *MemoryRepository) DeleteEdge(_ context.Context, ownerIdentity, id, reason string, at time.Time) (DeletionSignal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	edge, ok := r.edges[id]
	if !ok || edge.OwnerIdentity != ownerIdentity {
		return DeletionSignal{}, ErrNotFound
	}
	if edge.DeletedAt != nil {
		for _, signal := range r.deletionSignals {
			if signal.OwnerIdentity == ownerIdentity && signal.EntityType == EntityEdge && signal.EntityID == id {
				return cloneDeletionSignal(signal), nil
			}
		}
	}

	edge.Archived = true
	edge.DeletedAt = timePointer(at)
	edge.UpdatedAt = at
	r.edges[id] = edge

	signal := r.newDeletionSignal(ownerIdentity, EntityEdge, id, reason, at)
	r.deletionSignals = append(r.deletionSignals, signal)
	return cloneDeletionSignal(signal), nil
}

func (r *MemoryRepository) ListDeletionSignals(_ context.Context, ownerIdentity string) ([]DeletionSignal, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]DeletionSignal, 0)
	for _, signal := range r.deletionSignals {
		if signal.OwnerIdentity == ownerIdentity {
			result = append(result, cloneDeletionSignal(signal))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (r *MemoryRepository) newDeletionSignal(ownerIdentity string, entityType EntityType, entityID, reason string, at time.Time) DeletionSignal {
	r.nextSignal++
	return DeletionSignal{
		ID:            fmt.Sprintf("deletion-%06d", r.nextSignal),
		OwnerIdentity: ownerIdentity,
		EntityType:    entityType,
		EntityID:      entityID,
		Reason:        reason,
		CreatedAt:     at,
	}
}

func cloneNode(node Node) Node {
	node.Properties = cloneMap(node.Properties)
	node.ProjectKeys = cloneStrings(node.ProjectKeys)
	node.Tags = cloneStrings(node.Tags)
	node.Sources = cloneSources(node.Sources)
	node.ValidFrom = cloneTime(node.ValidFrom)
	node.ValidUntil = cloneTime(node.ValidUntil)
	node.DeletedAt = cloneTime(node.DeletedAt)
	return node
}

func cloneEdge(edge Edge) Edge {
	edge.Properties = cloneMap(edge.Properties)
	edge.ProjectKeys = cloneStrings(edge.ProjectKeys)
	edge.Sources = cloneSources(edge.Sources)
	edge.ValidFrom = cloneTime(edge.ValidFrom)
	edge.ValidUntil = cloneTime(edge.ValidUntil)
	edge.DeletedAt = cloneTime(edge.DeletedAt)
	return edge
}

func cloneDeletionSignal(signal DeletionSignal) DeletionSignal {
	signal.PropagatedEdgeIDs = cloneStrings(signal.PropagatedEdgeIDs)
	return signal
}

func cloneMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func cloneStrings(values []string) []string {
	return append([]string(nil), values...)
}

func cloneSources(values []SourceReference) []SourceReference {
	return append([]SourceReference(nil), values...)
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func timePointer(value time.Time) *time.Time {
	copyValue := value
	return &copyValue
}
