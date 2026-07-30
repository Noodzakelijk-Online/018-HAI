package knowledgegraph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrCorruptStorage   = errors.New("knowledge graph storage is corrupt")
	ErrConcurrentUpdate = errors.New("knowledge graph entity changed concurrently")
)

const (
	nodeIDPrefix     = "node-"
	edgeIDPrefix     = "edge-"
	deletionIDPrefix = "deletion-"
)

// GormRepository stores current graph projections and immutable revision,
// provenance, conflict, and deletion history in PostgreSQL.
type GormRepository struct {
	DB *gorm.DB
}

func NewGormRepository(db *gorm.DB) *GormRepository {
	return &GormRepository{DB: db}
}

// DefaultRepository returns the canonical migrated PostgreSQL repository.
func DefaultRepository() (Repository, error) {
	db, err := infra.GetDefaultDB()
	if err != nil {
		return nil, err
	}
	return NewGormRepository(db), nil
}

func (r *GormRepository) CreateNode(ctx context.Context, node Node) (Node, error) {
	if err := r.requireDB(); err != nil {
		return Node{}, err
	}
	var created Node
	err := r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		created, err = r.createNode(tx, node, "created")
		return err
	})
	return created, err
}

func (r *GormRepository) UpdateNode(ctx context.Context, node Node) (Node, error) {
	if err := r.requireDB(); err != nil {
		return Node{}, err
	}
	var updated Node
	err := r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		currentRow, current, err := r.lockNode(tx, node.OwnerIdentity, node.ID)
		if err != nil {
			return err
		}
		if current.DeletedAt != nil {
			return fmt.Errorf("%w: node %q is tombstoned", ErrConcurrentUpdate, node.ID)
		}
		updated, err = r.updateNodeLocked(
			tx,
			currentRow,
			node,
			nodeMutationOperation(current, node),
		)
		return err
	})
	return updated, err
}

func (r *GormRepository) GetNode(
	ctx context.Context,
	ownerIdentity string,
	id string,
) (Node, error) {
	if err := r.requireDB(); err != nil {
		return Node{}, err
	}
	ownerIdentity, id = normalizedScope(ownerIdentity, id)
	if ownerIdentity == "" || id == "" {
		return Node{}, ErrNotFound
	}
	var row models.KnowledgeGraphNode
	err := r.DB.WithContext(ctx).
		Where("owner_identity = ? AND id = ?", ownerIdentity, id).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Node{}, ErrNotFound
	}
	if err != nil {
		return Node{}, err
	}
	return nodeFromModel(row)
}

func (r *GormRepository) ListNodes(
	ctx context.Context,
	ownerIdentity string,
	options ListOptions,
) ([]Node, error) {
	if err := r.requireDB(); err != nil {
		return nil, err
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	if ownerIdentity == "" {
		return []Node{}, nil
	}
	query := r.DB.WithContext(ctx).Where("owner_identity = ?", ownerIdentity)
	if !options.IncludeArchived {
		query = query.Where("archived = false")
	}
	if !options.IncludeDeleted {
		query = query.Where("deleted_at IS NULL")
	}
	var rows []models.KnowledgeGraphNode
	if err := query.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Node, 0, len(rows))
	for _, row := range rows {
		node, err := nodeFromModel(row)
		if err != nil {
			return nil, err
		}
		result = append(result, node)
	}
	return result, nil
}

func (r *GormRepository) CreateEdge(ctx context.Context, edge Edge) (Edge, error) {
	if err := r.requireDB(); err != nil {
		return Edge{}, err
	}
	var created Edge
	err := r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		created, err = r.createEdge(tx, edge, "created")
		return err
	})
	return created, err
}

func (r *GormRepository) UpdateEdge(ctx context.Context, edge Edge) (Edge, error) {
	if err := r.requireDB(); err != nil {
		return Edge{}, err
	}
	var updated Edge
	err := r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		currentRow, current, err := r.lockEdge(tx, edge.OwnerIdentity, edge.ID)
		if err != nil {
			return err
		}
		if current.DeletedAt != nil {
			return fmt.Errorf("%w: edge %q is tombstoned", ErrConcurrentUpdate, edge.ID)
		}
		updated, err = r.updateEdgeLocked(
			tx,
			currentRow,
			edge,
			edgeMutationOperation(current, edge),
		)
		return err
	})
	return updated, err
}

func (r *GormRepository) GetEdge(
	ctx context.Context,
	ownerIdentity string,
	id string,
) (Edge, error) {
	if err := r.requireDB(); err != nil {
		return Edge{}, err
	}
	ownerIdentity, id = normalizedScope(ownerIdentity, id)
	if ownerIdentity == "" || id == "" {
		return Edge{}, ErrNotFound
	}
	var row models.KnowledgeGraphEdge
	err := r.DB.WithContext(ctx).
		Where("owner_identity = ? AND id = ?", ownerIdentity, id).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Edge{}, ErrNotFound
	}
	if err != nil {
		return Edge{}, err
	}
	return edgeFromModel(row)
}

func (r *GormRepository) ListEdges(
	ctx context.Context,
	ownerIdentity string,
	options ListOptions,
) ([]Edge, error) {
	if err := r.requireDB(); err != nil {
		return nil, err
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	if ownerIdentity == "" {
		return []Edge{}, nil
	}
	query := r.DB.WithContext(ctx).Where("owner_identity = ?", ownerIdentity)
	if !options.IncludeArchived {
		query = query.Where("archived = false")
	}
	if !options.IncludeDeleted {
		query = query.Where("deleted_at IS NULL")
	}
	var rows []models.KnowledgeGraphEdge
	if err := query.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Edge, 0, len(rows))
	for _, row := range rows {
		edge, err := edgeFromModel(row)
		if err != nil {
			return nil, err
		}
		result = append(result, edge)
	}
	return result, nil
}

func (r *GormRepository) DeleteNode(
	ctx context.Context,
	ownerIdentity string,
	id string,
	reason string,
	at time.Time,
) (DeletionSignal, error) {
	if err := r.requireDB(); err != nil {
		return DeletionSignal{}, err
	}
	ownerIdentity, id = normalizedScope(ownerIdentity, id)
	reason = strings.TrimSpace(reason)
	var signal DeletionSignal
	err := r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		currentRow, current, err := r.lockNode(tx, ownerIdentity, id)
		if err != nil {
			return err
		}
		if current.DeletedAt != nil {
			signal, err = r.existingDeletionSignal(tx, ownerIdentity, EntityNode, id)
			return err
		}

		at = normalizedTransactionTime(at)
		current.Archived = true
		current.DeletedAt = timePointer(at)
		current.UpdatedAt = at
		if _, err := r.updateNodeLocked(tx, currentRow, current, "tombstoned"); err != nil {
			return err
		}

		var edgeRows []models.KnowledgeGraphEdge
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where(
				"owner_identity = ? AND deleted_at IS NULL AND (from_node_id = ? OR to_node_id = ?)",
				ownerIdentity,
				id,
				id,
			).
			Order("id ASC").
			Find(&edgeRows).Error; err != nil {
			return err
		}
		propagated := make([]string, 0, len(edgeRows))
		for _, edgeRow := range edgeRows {
			edge, err := edgeFromModel(edgeRow)
			if err != nil {
				return err
			}
			edge.Archived = true
			edge.DeletedAt = timePointer(at)
			edge.UpdatedAt = at
			if _, err := r.updateEdgeLocked(tx, edgeRow, edge, "tombstoned"); err != nil {
				return err
			}
			propagated = append(propagated, edge.ID)
		}
		sort.Strings(propagated)
		signal = DeletionSignal{
			ID:                deletionIDPrefix + uuid.NewString(),
			OwnerIdentity:     ownerIdentity,
			EntityType:        EntityNode,
			EntityID:          id,
			PropagatedEdgeIDs: propagated,
			Reason:            reason,
			CreatedAt:         at,
		}
		return tx.Create(deletionSignalToModel(signal)).Error
	})
	return signal, err
}

func (r *GormRepository) DeleteEdge(
	ctx context.Context,
	ownerIdentity string,
	id string,
	reason string,
	at time.Time,
) (DeletionSignal, error) {
	if err := r.requireDB(); err != nil {
		return DeletionSignal{}, err
	}
	ownerIdentity, id = normalizedScope(ownerIdentity, id)
	reason = strings.TrimSpace(reason)
	var signal DeletionSignal
	err := r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		currentRow, current, err := r.lockEdge(tx, ownerIdentity, id)
		if err != nil {
			return err
		}
		if current.DeletedAt != nil {
			signal, err = r.existingDeletionSignal(tx, ownerIdentity, EntityEdge, id)
			return err
		}
		at = normalizedTransactionTime(at)
		current.Archived = true
		current.DeletedAt = timePointer(at)
		current.UpdatedAt = at
		if _, err := r.updateEdgeLocked(tx, currentRow, current, "tombstoned"); err != nil {
			return err
		}
		signal = DeletionSignal{
			ID:            deletionIDPrefix + uuid.NewString(),
			OwnerIdentity: ownerIdentity,
			EntityType:    EntityEdge,
			EntityID:      id,
			Reason:        reason,
			CreatedAt:     at,
		}
		return tx.Create(deletionSignalToModel(signal)).Error
	})
	return signal, err
}

func (r *GormRepository) ListDeletionSignals(
	ctx context.Context,
	ownerIdentity string,
) ([]DeletionSignal, error) {
	if err := r.requireDB(); err != nil {
		return nil, err
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	if ownerIdentity == "" {
		return []DeletionSignal{}, nil
	}
	var rows []models.KnowledgeGraphDeletionSignal
	if err := r.DB.WithContext(ctx).
		Where("owner_identity = ?", ownerIdentity).
		Order("created_at ASC, id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]DeletionSignal, 0, len(rows))
	for _, row := range rows {
		signal, err := deletionSignalFromModel(row)
		if err != nil {
			return nil, err
		}
		result = append(result, signal)
	}
	return result, nil
}

// CorrectNode atomically inserts a replacement and archives/links the old
// node. Service.CorrectNode uses this extension when available.
func (r *GormRepository) CorrectNode(
	ctx context.Context,
	previous Node,
	replacement Node,
) (Node, error) {
	if err := r.requireDB(); err != nil {
		return Node{}, err
	}
	var created Node
	err := r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		currentRow, current, err := r.lockNode(
			tx,
			previous.OwnerIdentity,
			previous.ID,
		)
		if err != nil {
			return err
		}
		if current.DeletedAt != nil {
			return fmt.Errorf("%w: node %q is tombstoned", ErrConcurrentUpdate, current.ID)
		}
		if !reflect.DeepEqual(current, previous) {
			return ErrConcurrentUpdate
		}
		replacement.OwnerIdentity = current.OwnerIdentity
		replacement.SupersedesID = current.ID
		created, err = r.createNode(tx, replacement, "corrected")
		if err != nil {
			return err
		}
		current.Archived = true
		current.CorrectedByID = created.ID
		current.UpdatedAt = replacement.UpdatedAt
		_, err = r.updateNodeLocked(tx, currentRow, current, "corrected")
		return err
	})
	return created, err
}

func (r *GormRepository) createNode(
	tx *gorm.DB,
	node Node,
	operation string,
) (Node, error) {
	if strings.TrimSpace(node.ID) == "" {
		node.ID = nodeIDPrefix + uuid.NewString()
	}
	transactionAt := transactionTimeFor(node.CreatedAt, node.UpdatedAt)
	row, err := nodeToModel(node, 1, transactionAt)
	if err != nil {
		return Node{}, err
	}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if result.Error != nil {
		return Node{}, result.Error
	}
	if result.RowsAffected != 1 {
		return Node{}, ErrExists
	}
	created, err := nodeFromModel(row)
	if err != nil {
		return Node{}, err
	}
	if err := appendNodeHistory(tx, row, created, operation); err != nil {
		return Node{}, err
	}
	if err := recordConflict(tx, created, transactionAt); err != nil {
		return Node{}, err
	}
	return created, nil
}

func (r *GormRepository) createEdge(
	tx *gorm.DB,
	edge Edge,
	operation string,
) (Edge, error) {
	if strings.TrimSpace(edge.ID) == "" {
		edge.ID = edgeIDPrefix + uuid.NewString()
	}
	transactionAt := transactionTimeFor(edge.CreatedAt, edge.UpdatedAt)
	row, err := edgeToModel(edge, 1, transactionAt)
	if err != nil {
		return Edge{}, err
	}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if result.Error != nil {
		return Edge{}, result.Error
	}
	if result.RowsAffected != 1 {
		return Edge{}, ErrExists
	}
	created, err := edgeFromModel(row)
	if err != nil {
		return Edge{}, err
	}
	if err := appendEdgeHistory(tx, row, created, operation); err != nil {
		return Edge{}, err
	}
	return created, nil
}

func (r *GormRepository) lockNode(
	tx *gorm.DB,
	ownerIdentity string,
	id string,
) (models.KnowledgeGraphNode, Node, error) {
	ownerIdentity, id = normalizedScope(ownerIdentity, id)
	if ownerIdentity == "" || id == "" {
		return models.KnowledgeGraphNode{}, Node{}, ErrNotFound
	}
	var row models.KnowledgeGraphNode
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("owner_identity = ? AND id = ?", ownerIdentity, id).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.KnowledgeGraphNode{}, Node{}, ErrNotFound
	}
	if err != nil {
		return models.KnowledgeGraphNode{}, Node{}, err
	}
	node, err := nodeFromModel(row)
	return row, node, err
}

func (r *GormRepository) lockEdge(
	tx *gorm.DB,
	ownerIdentity string,
	id string,
) (models.KnowledgeGraphEdge, Edge, error) {
	ownerIdentity, id = normalizedScope(ownerIdentity, id)
	if ownerIdentity == "" || id == "" {
		return models.KnowledgeGraphEdge{}, Edge{}, ErrNotFound
	}
	var row models.KnowledgeGraphEdge
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("owner_identity = ? AND id = ?", ownerIdentity, id).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.KnowledgeGraphEdge{}, Edge{}, ErrNotFound
	}
	if err != nil {
		return models.KnowledgeGraphEdge{}, Edge{}, err
	}
	edge, err := edgeFromModel(row)
	return row, edge, err
}

func (r *GormRepository) updateNodeLocked(
	tx *gorm.DB,
	current models.KnowledgeGraphNode,
	node Node,
	operation string,
) (Node, error) {
	node.ID = current.ID
	node.OwnerIdentity = current.OwnerIdentity
	node.CreatedAt = current.CreatedAt
	transactionAt := transactionTimeFor(node.CreatedAt, node.UpdatedAt)
	row, err := nodeToModel(node, current.Revision+1, transactionAt)
	if err != nil {
		return Node{}, err
	}
	result := tx.Session(&gorm.Session{SkipHooks: true}).
		Model(&models.KnowledgeGraphNode{}).
		Where(
			"owner_identity = ? AND id = ? AND revision = ?",
			current.OwnerIdentity,
			current.ID,
			current.Revision,
		).
		Select("*").
		Updates(&row)
	if result.Error != nil {
		return Node{}, result.Error
	}
	if result.RowsAffected != 1 {
		return Node{}, ErrConcurrentUpdate
	}
	updated, err := nodeFromModel(row)
	if err != nil {
		return Node{}, err
	}
	if err := appendNodeHistory(tx, row, updated, operation); err != nil {
		return Node{}, err
	}
	if err := recordConflict(tx, updated, transactionAt); err != nil {
		return Node{}, err
	}
	return updated, nil
}

func (r *GormRepository) updateEdgeLocked(
	tx *gorm.DB,
	current models.KnowledgeGraphEdge,
	edge Edge,
	operation string,
) (Edge, error) {
	edge.ID = current.ID
	edge.OwnerIdentity = current.OwnerIdentity
	edge.CreatedAt = current.CreatedAt
	transactionAt := transactionTimeFor(edge.CreatedAt, edge.UpdatedAt)
	row, err := edgeToModel(edge, current.Revision+1, transactionAt)
	if err != nil {
		return Edge{}, err
	}
	result := tx.Session(&gorm.Session{SkipHooks: true}).
		Model(&models.KnowledgeGraphEdge{}).
		Where(
			"owner_identity = ? AND id = ? AND revision = ?",
			current.OwnerIdentity,
			current.ID,
			current.Revision,
		).
		Select("*").
		Updates(&row)
	if result.Error != nil {
		return Edge{}, result.Error
	}
	if result.RowsAffected != 1 {
		return Edge{}, ErrConcurrentUpdate
	}
	updated, err := edgeFromModel(row)
	if err != nil {
		return Edge{}, err
	}
	if err := appendEdgeHistory(tx, row, updated, operation); err != nil {
		return Edge{}, err
	}
	return updated, nil
}

func appendNodeHistory(
	tx *gorm.DB,
	row models.KnowledgeGraphNode,
	node Node,
	operation string,
) error {
	snapshot, err := kgEncodeObject(node)
	if err != nil {
		return fmt.Errorf("encode node revision: %w", err)
	}
	if err := tx.Create(&models.KnowledgeGraphNodeRevision{
		OwnerIdentity: row.OwnerIdentity,
		NodeID:        row.ID,
		Revision:      row.Revision,
		Operation:     operation,
		SnapshotJSON:  snapshot,
		TransactionAt: row.TransactionFrom,
	}).Error; err != nil {
		return err
	}
	return appendProvenance(
		tx,
		row.OwnerIdentity,
		string(EntityNode),
		row.ID,
		row.Revision,
		operation,
		row.SourcesJSON,
		row.TransactionFrom,
	)
}

func appendEdgeHistory(
	tx *gorm.DB,
	row models.KnowledgeGraphEdge,
	edge Edge,
	operation string,
) error {
	snapshot, err := kgEncodeObject(edge)
	if err != nil {
		return fmt.Errorf("encode edge revision: %w", err)
	}
	if err := tx.Create(&models.KnowledgeGraphEdgeRevision{
		OwnerIdentity: row.OwnerIdentity,
		EdgeID:        row.ID,
		Revision:      row.Revision,
		Operation:     operation,
		SnapshotJSON:  snapshot,
		TransactionAt: row.TransactionFrom,
	}).Error; err != nil {
		return err
	}
	return appendProvenance(
		tx,
		row.OwnerIdentity,
		string(EntityEdge),
		row.ID,
		row.Revision,
		operation,
		row.SourcesJSON,
		row.TransactionFrom,
	)
}

func appendProvenance(
	tx *gorm.DB,
	ownerIdentity string,
	entityType string,
	entityID string,
	revision uint64,
	operation string,
	sourcesJSON string,
	recordedAt time.Time,
) error {
	return tx.Create(&models.KnowledgeGraphProvenanceEvent{
		OwnerIdentity: ownerIdentity,
		EntityType:    entityType,
		EntityID:      entityID,
		Revision:      revision,
		Operation:     operation,
		SourcesJSON:   sourcesJSON,
		RecordedAt:    recordedAt,
	}).Error
}

func recordConflict(tx *gorm.DB, node Node, detectedAt time.Time) error {
	if strings.TrimSpace(node.ConflictGroupID) == "" {
		return nil
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&models.KnowledgeGraphConflictRecord{
			OwnerIdentity:   node.OwnerIdentity,
			ConflictGroupID: node.ConflictGroupID,
			NodeID:          node.ID,
			DetectedAt:      detectedAt,
		}).Error
}

func (r *GormRepository) existingDeletionSignal(
	tx *gorm.DB,
	ownerIdentity string,
	entityType EntityType,
	entityID string,
) (DeletionSignal, error) {
	var row models.KnowledgeGraphDeletionSignal
	err := tx.Where(
		"owner_identity = ? AND entity_type = ? AND entity_id = ?",
		ownerIdentity,
		entityType,
		entityID,
	).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DeletionSignal{}, fmt.Errorf(
			"%w: tombstoned %s %q has no deletion signal",
			ErrCorruptStorage,
			entityType,
			entityID,
		)
	}
	if err != nil {
		return DeletionSignal{}, err
	}
	return deletionSignalFromModel(row)
}

func nodeToModel(
	node Node,
	revision uint64,
	transactionAt time.Time,
) (models.KnowledgeGraphNode, error) {
	if err := validateNode(node); err != nil {
		return models.KnowledgeGraphNode{}, err
	}
	if strings.TrimSpace(node.ID) == "" {
		return models.KnowledgeGraphNode{}, fmt.Errorf("node id is required")
	}
	if revision == 0 || node.CreatedAt.IsZero() || node.UpdatedAt.IsZero() ||
		transactionAt.IsZero() {
		return models.KnowledgeGraphNode{}, fmt.Errorf(
			"node revision and transaction timestamps are required",
		)
	}
	if node.UpdatedAt.Before(node.CreatedAt) || transactionAt.Before(node.UpdatedAt) {
		return models.KnowledgeGraphNode{}, fmt.Errorf(
			"node transaction timestamps are not monotonic",
		)
	}
	properties, err := kgEncodeObject(node.Properties)
	if err != nil {
		return models.KnowledgeGraphNode{}, fmt.Errorf("encode node properties: %w", err)
	}
	projectKeys, err := kgEncodeArray(node.ProjectKeys)
	if err != nil {
		return models.KnowledgeGraphNode{}, fmt.Errorf("encode node project keys: %w", err)
	}
	tags, err := kgEncodeArray(node.Tags)
	if err != nil {
		return models.KnowledgeGraphNode{}, fmt.Errorf("encode node tags: %w", err)
	}
	sources, err := kgEncodeArray(node.Sources)
	if err != nil {
		return models.KnowledgeGraphNode{}, fmt.Errorf("encode node sources: %w", err)
	}
	return models.KnowledgeGraphNode{
		ID: node.ID, OwnerIdentity: node.OwnerIdentity, Kind: string(node.Kind),
		DeduplicationKey: node.DeduplicationKey, Label: node.Label,
		Content: node.Content, PropertiesJSON: properties,
		ProjectKeysJSON: projectKeys, TagsJSON: tags, Confidence: node.Confidence,
		VerificationStatus: string(node.VerificationStatus), SourcesJSON: sources,
		ValidFrom: cloneTime(node.ValidFrom), ValidUntil: cloneTime(node.ValidUntil),
		Sensitivity: string(node.Sensitivity), LocalOnly: node.LocalOnly,
		ConflictGroupID: node.ConflictGroupID, SupersedesID: node.SupersedesID,
		CorrectedByID: node.CorrectedByID, Archived: node.Archived,
		Revision: revision, TransactionFrom: transactionAt,
		CreatedAt: node.CreatedAt.UTC(), UpdatedAt: node.UpdatedAt.UTC(),
		DeletedAt: cloneTime(node.DeletedAt),
	}, nil
}

func nodeFromModel(row models.KnowledgeGraphNode) (Node, error) {
	var properties map[string]string
	if err := kgDecodeObject(row.PropertiesJSON, &properties); err != nil {
		return Node{}, corruptJSON("node properties", row.ID, err)
	}
	var projectKeys []string
	if err := kgDecodeArray(row.ProjectKeysJSON, &projectKeys); err != nil {
		return Node{}, corruptJSON("node project keys", row.ID, err)
	}
	var tags []string
	if err := kgDecodeArray(row.TagsJSON, &tags); err != nil {
		return Node{}, corruptJSON("node tags", row.ID, err)
	}
	var sources []SourceReference
	if err := kgDecodeArray(row.SourcesJSON, &sources); err != nil {
		return Node{}, corruptJSON("node sources", row.ID, err)
	}
	node := Node{
		ID: row.ID, OwnerIdentity: row.OwnerIdentity, Kind: NodeKind(row.Kind),
		DeduplicationKey: row.DeduplicationKey, Label: row.Label,
		Content: row.Content, Properties: properties, ProjectKeys: projectKeys,
		Tags: tags, Confidence: row.Confidence,
		VerificationStatus: VerificationStatus(row.VerificationStatus),
		Sources:            sources, ValidFrom: cloneTime(row.ValidFrom),
		ValidUntil: cloneTime(row.ValidUntil), Sensitivity: Sensitivity(row.Sensitivity),
		LocalOnly: row.LocalOnly, ConflictGroupID: row.ConflictGroupID,
		SupersedesID: row.SupersedesID, CorrectedByID: row.CorrectedByID,
		Archived: row.Archived, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		DeletedAt: cloneTime(row.DeletedAt),
	}
	if row.Revision == 0 || row.TransactionFrom.IsZero() {
		return Node{}, fmt.Errorf("%w: node %q lacks transaction metadata", ErrCorruptStorage, row.ID)
	}
	if err := validateNode(node); err != nil {
		return Node{}, fmt.Errorf("%w: node %q is invalid: %v", ErrCorruptStorage, row.ID, err)
	}
	return node, nil
}

func edgeToModel(
	edge Edge,
	revision uint64,
	transactionAt time.Time,
) (models.KnowledgeGraphEdge, error) {
	if err := validateEdge(edge); err != nil {
		return models.KnowledgeGraphEdge{}, err
	}
	if strings.TrimSpace(edge.ID) == "" {
		return models.KnowledgeGraphEdge{}, fmt.Errorf("edge id is required")
	}
	if revision == 0 || edge.CreatedAt.IsZero() || edge.UpdatedAt.IsZero() ||
		transactionAt.IsZero() {
		return models.KnowledgeGraphEdge{}, fmt.Errorf(
			"edge revision and transaction timestamps are required",
		)
	}
	if edge.UpdatedAt.Before(edge.CreatedAt) || transactionAt.Before(edge.UpdatedAt) {
		return models.KnowledgeGraphEdge{}, fmt.Errorf(
			"edge transaction timestamps are not monotonic",
		)
	}
	properties, err := kgEncodeObject(edge.Properties)
	if err != nil {
		return models.KnowledgeGraphEdge{}, fmt.Errorf("encode edge properties: %w", err)
	}
	projectKeys, err := kgEncodeArray(edge.ProjectKeys)
	if err != nil {
		return models.KnowledgeGraphEdge{}, fmt.Errorf("encode edge project keys: %w", err)
	}
	sources, err := kgEncodeArray(edge.Sources)
	if err != nil {
		return models.KnowledgeGraphEdge{}, fmt.Errorf("encode edge sources: %w", err)
	}
	return models.KnowledgeGraphEdge{
		ID: edge.ID, OwnerIdentity: edge.OwnerIdentity,
		FromNodeID: edge.FromNodeID, ToNodeID: edge.ToNodeID,
		Relationship: string(edge.Relationship), Label: edge.Label,
		PropertiesJSON: properties, ProjectKeysJSON: projectKeys,
		Confidence:         edge.Confidence,
		VerificationStatus: string(edge.VerificationStatus), SourcesJSON: sources,
		ValidFrom: cloneTime(edge.ValidFrom), ValidUntil: cloneTime(edge.ValidUntil),
		Sensitivity: string(edge.Sensitivity), LocalOnly: edge.LocalOnly,
		Archived: edge.Archived, Revision: revision, TransactionFrom: transactionAt,
		CreatedAt: edge.CreatedAt.UTC(), UpdatedAt: edge.UpdatedAt.UTC(),
		DeletedAt: cloneTime(edge.DeletedAt),
	}, nil
}

func edgeFromModel(row models.KnowledgeGraphEdge) (Edge, error) {
	var properties map[string]string
	if err := kgDecodeObject(row.PropertiesJSON, &properties); err != nil {
		return Edge{}, corruptJSON("edge properties", row.ID, err)
	}
	var projectKeys []string
	if err := kgDecodeArray(row.ProjectKeysJSON, &projectKeys); err != nil {
		return Edge{}, corruptJSON("edge project keys", row.ID, err)
	}
	var sources []SourceReference
	if err := kgDecodeArray(row.SourcesJSON, &sources); err != nil {
		return Edge{}, corruptJSON("edge sources", row.ID, err)
	}
	edge := Edge{
		ID: row.ID, OwnerIdentity: row.OwnerIdentity,
		FromNodeID: row.FromNodeID, ToNodeID: row.ToNodeID,
		Relationship: RelationshipKind(row.Relationship), Label: row.Label,
		Properties: properties, ProjectKeys: projectKeys, Confidence: row.Confidence,
		VerificationStatus: VerificationStatus(row.VerificationStatus),
		Sources:            sources, ValidFrom: cloneTime(row.ValidFrom),
		ValidUntil: cloneTime(row.ValidUntil), Sensitivity: Sensitivity(row.Sensitivity),
		LocalOnly: row.LocalOnly, Archived: row.Archived,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		DeletedAt: cloneTime(row.DeletedAt),
	}
	if row.Revision == 0 || row.TransactionFrom.IsZero() {
		return Edge{}, fmt.Errorf("%w: edge %q lacks transaction metadata", ErrCorruptStorage, row.ID)
	}
	if err := validateEdge(edge); err != nil {
		return Edge{}, fmt.Errorf("%w: edge %q is invalid: %v", ErrCorruptStorage, row.ID, err)
	}
	return edge, nil
}

func deletionSignalToModel(
	signal DeletionSignal,
) *models.KnowledgeGraphDeletionSignal {
	propagated, _ := kgEncodeArray(signal.PropagatedEdgeIDs)
	return &models.KnowledgeGraphDeletionSignal{
		ID: signal.ID, OwnerIdentity: signal.OwnerIdentity,
		EntityType: string(signal.EntityType), EntityID: signal.EntityID,
		PropagatedEdgeIDsJSON: propagated, Reason: signal.Reason,
		CreatedAt: signal.CreatedAt,
	}
}

func deletionSignalFromModel(
	row models.KnowledgeGraphDeletionSignal,
) (DeletionSignal, error) {
	var propagated []string
	if err := kgDecodeArray(row.PropagatedEdgeIDsJSON, &propagated); err != nil {
		return DeletionSignal{}, corruptJSON("deletion propagation", row.ID, err)
	}
	if row.EntityType != string(EntityNode) && row.EntityType != string(EntityEdge) {
		return DeletionSignal{}, fmt.Errorf(
			"%w: deletion signal %q has invalid entity type",
			ErrCorruptStorage,
			row.ID,
		)
	}
	if strings.TrimSpace(row.Reason) == "" {
		return DeletionSignal{}, fmt.Errorf(
			"%w: deletion signal %q has no reason",
			ErrCorruptStorage,
			row.ID,
		)
	}
	return DeletionSignal{
		ID: row.ID, OwnerIdentity: row.OwnerIdentity,
		EntityType: EntityType(row.EntityType), EntityID: row.EntityID,
		PropagatedEdgeIDs: propagated, Reason: row.Reason,
		CreatedAt: row.CreatedAt,
	}, nil
}

func kgEncodeObject(value any) (string, error) {
	if value == nil {
		value = map[string]string{}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	if string(encoded) == "null" {
		return "{}", nil
	}
	if len(encoded) == 0 || encoded[0] != '{' {
		return "", fmt.Errorf("JSON value is not an object")
	}
	return string(encoded), nil
}

func kgEncodeArray(value any) (string, error) {
	if value == nil {
		value = []string{}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	if string(encoded) == "null" {
		return "[]", nil
	}
	if len(encoded) == 0 || encoded[0] != '[' {
		return "", fmt.Errorf("JSON value is not an array")
	}
	return string(encoded), nil
}

func kgDecodeObject(raw string, target any) error {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw[0] != '{' {
		return fmt.Errorf("stored JSON is not an object")
	}
	return json.Unmarshal([]byte(raw), target)
}

func kgDecodeArray(raw string, target any) error {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw[0] != '[' {
		return fmt.Errorf("stored JSON is not an array")
	}
	return json.Unmarshal([]byte(raw), target)
}

func corruptJSON(field string, id string, err error) error {
	return fmt.Errorf("%w: decode %s for %q: %v", ErrCorruptStorage, field, id, err)
}

func nodeMutationOperation(current Node, next Node) string {
	switch {
	case !current.Archived && next.Archived && next.CorrectedByID != "":
		return "corrected"
	case !current.Archived && next.Archived:
		return "archived"
	case current.Archived && !next.Archived:
		return "restored"
	case current.ConflictGroupID == "" && next.ConflictGroupID != "":
		return "conflict_recorded"
	default:
		return "updated"
	}
}

func edgeMutationOperation(current Edge, next Edge) string {
	switch {
	case !current.Archived && next.Archived:
		return "archived"
	case current.Archived && !next.Archived:
		return "restored"
	default:
		return "updated"
	}
}

func normalizedScope(ownerIdentity string, id string) (string, string) {
	return strings.TrimSpace(ownerIdentity), strings.TrimSpace(id)
}

func normalizedTransactionTime(value time.Time) time.Time {
	if value.IsZero() {
		value = time.Now()
	}
	return value.UTC()
}

func transactionTimeFor(values ...time.Time) time.Time {
	transactionAt := normalizedTransactionTime(time.Now())
	for _, value := range values {
		value = value.UTC()
		if value.After(transactionAt) {
			transactionAt = value
		}
	}
	return transactionAt
}

func (r *GormRepository) requireDB() error {
	if r == nil || r.DB == nil {
		return fmt.Errorf("knowledge graph database is required")
	}
	return nil
}
