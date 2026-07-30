package models

import (
	"time"

	"github.com/google/uuid"
)

// KnowledgeGraphNode is the mutable current projection of an owner-scoped
// knowledge node. Every mutation is also captured in an immutable revision.
type KnowledgeGraphNode struct {
	ID                 string     `gorm:"type:varchar(160);primaryKey" json:"id"`
	OwnerIdentity      string     `gorm:"type:varchar(255);not null;uniqueIndex:uq_knowledge_graph_nodes_owner_id,priority:1;index:idx_knowledge_graph_nodes_owner_kind,priority:1" json:"-"`
	Kind               string     `gorm:"type:varchar(40);not null;index:idx_knowledge_graph_nodes_owner_kind,priority:2" json:"kind"`
	DeduplicationKey   string     `gorm:"type:varchar(512);not null;index:idx_knowledge_graph_nodes_dedup,priority:3" json:"deduplicationKey"`
	Label              string     `gorm:"type:varchar(1000);not null;default:''" json:"label"`
	Content            string     `gorm:"type:text;not null;default:''" json:"content,omitempty"`
	PropertiesJSON     string     `gorm:"type:jsonb;not null;default:'{}'" json:"-"`
	ProjectKeysJSON    string     `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	TagsJSON           string     `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	Confidence         float64    `gorm:"type:numeric(5,4);not null" json:"confidence"`
	VerificationStatus string     `gorm:"type:varchar(40);not null;index" json:"verificationStatus"`
	SourcesJSON        string     `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	ValidFrom          *time.Time `gorm:"index" json:"validFrom,omitempty"`
	ValidUntil         *time.Time `gorm:"index" json:"validUntil,omitempty"`
	Sensitivity        string     `gorm:"type:varchar(32);not null;index" json:"sensitivity"`
	LocalOnly          bool       `gorm:"not null;default:false;index" json:"localOnly"`
	ConflictGroupID    string     `gorm:"type:varchar(160);not null;default:'';index" json:"conflictGroupId,omitempty"`
	SupersedesID       string     `gorm:"type:varchar(160);not null;default:'';index" json:"supersedesId,omitempty"`
	CorrectedByID      string     `gorm:"type:varchar(160);not null;default:'';index" json:"correctedById,omitempty"`
	Archived           bool       `gorm:"not null;default:false;index" json:"archived"`
	Revision           uint64     `gorm:"type:bigint;not null" json:"revision"`
	TransactionFrom    time.Time  `gorm:"not null" json:"transactionFrom"`
	CreatedAt          time.Time  `gorm:"not null" json:"createdAt"`
	UpdatedAt          time.Time  `gorm:"not null;index:idx_knowledge_graph_nodes_owner_updated,priority:2,sort:desc" json:"updatedAt"`
	DeletedAt          *time.Time `gorm:"index" json:"deletedAt,omitempty"`
}

func (KnowledgeGraphNode) TableName() string { return "knowledge_graph_nodes" }

// KnowledgeGraphEdge is the current projection of an owner-scoped
// relationship. Composite foreign keys prevent cross-owner graph edges.
type KnowledgeGraphEdge struct {
	ID                 string     `gorm:"type:varchar(160);primaryKey" json:"id"`
	OwnerIdentity      string     `gorm:"type:varchar(255);not null;uniqueIndex:uq_knowledge_graph_edges_owner_id,priority:1;index:idx_knowledge_graph_edges_owner_relationship,priority:1" json:"-"`
	FromNodeID         string     `gorm:"type:varchar(160);not null;index" json:"fromNodeId"`
	ToNodeID           string     `gorm:"type:varchar(160);not null;index" json:"toNodeId"`
	Relationship       string     `gorm:"type:varchar(48);not null;index:idx_knowledge_graph_edges_owner_relationship,priority:2" json:"relationship"`
	Label              string     `gorm:"type:varchar(1000);not null;default:''" json:"label,omitempty"`
	PropertiesJSON     string     `gorm:"type:jsonb;not null;default:'{}'" json:"-"`
	ProjectKeysJSON    string     `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	Confidence         float64    `gorm:"type:numeric(5,4);not null" json:"confidence"`
	VerificationStatus string     `gorm:"type:varchar(40);not null;index" json:"verificationStatus"`
	SourcesJSON        string     `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	ValidFrom          *time.Time `gorm:"index" json:"validFrom,omitempty"`
	ValidUntil         *time.Time `gorm:"index" json:"validUntil,omitempty"`
	Sensitivity        string     `gorm:"type:varchar(32);not null;index" json:"sensitivity"`
	LocalOnly          bool       `gorm:"not null;default:false;index" json:"localOnly"`
	Archived           bool       `gorm:"not null;default:false;index" json:"archived"`
	Revision           uint64     `gorm:"type:bigint;not null" json:"revision"`
	TransactionFrom    time.Time  `gorm:"not null" json:"transactionFrom"`
	CreatedAt          time.Time  `gorm:"not null" json:"createdAt"`
	UpdatedAt          time.Time  `gorm:"not null;index:idx_knowledge_graph_edges_owner_updated,priority:2,sort:desc" json:"updatedAt"`
	DeletedAt          *time.Time `gorm:"index" json:"deletedAt,omitempty"`
}

func (KnowledgeGraphEdge) TableName() string { return "knowledge_graph_edges" }

// KnowledgeGraphNodeRevision is an immutable transaction-time snapshot.
type KnowledgeGraphNodeRevision struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	OwnerIdentity string    `gorm:"type:varchar(255);not null;uniqueIndex:uq_knowledge_graph_node_revisions,priority:1;index:idx_knowledge_graph_node_revisions_owner_node,priority:1" json:"-"`
	NodeID        string    `gorm:"type:varchar(160);not null;uniqueIndex:uq_knowledge_graph_node_revisions,priority:2;index:idx_knowledge_graph_node_revisions_owner_node,priority:2" json:"nodeId"`
	Revision      uint64    `gorm:"type:bigint;not null;uniqueIndex:uq_knowledge_graph_node_revisions,priority:3" json:"revision"`
	Operation     string    `gorm:"type:varchar(40);not null" json:"operation"`
	SnapshotJSON  string    `gorm:"type:jsonb;not null" json:"-"`
	TransactionAt time.Time `gorm:"not null;index:idx_knowledge_graph_node_revisions_owner_node,priority:3,sort:desc" json:"transactionAt"`
}

func (KnowledgeGraphNodeRevision) TableName() string {
	return "knowledge_graph_node_revisions"
}

// KnowledgeGraphEdgeRevision is an immutable transaction-time snapshot.
type KnowledgeGraphEdgeRevision struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	OwnerIdentity string    `gorm:"type:varchar(255);not null;uniqueIndex:uq_knowledge_graph_edge_revisions,priority:1;index:idx_knowledge_graph_edge_revisions_owner_edge,priority:1" json:"-"`
	EdgeID        string    `gorm:"type:varchar(160);not null;uniqueIndex:uq_knowledge_graph_edge_revisions,priority:2;index:idx_knowledge_graph_edge_revisions_owner_edge,priority:2" json:"edgeId"`
	Revision      uint64    `gorm:"type:bigint;not null;uniqueIndex:uq_knowledge_graph_edge_revisions,priority:3" json:"revision"`
	Operation     string    `gorm:"type:varchar(40);not null" json:"operation"`
	SnapshotJSON  string    `gorm:"type:jsonb;not null" json:"-"`
	TransactionAt time.Time `gorm:"not null;index:idx_knowledge_graph_edge_revisions_owner_edge,priority:3,sort:desc" json:"transactionAt"`
}

func (KnowledgeGraphEdgeRevision) TableName() string {
	return "knowledge_graph_edge_revisions"
}

// KnowledgeGraphProvenanceEvent preserves the evidence set attached to every
// graph revision, including an explicit empty set.
type KnowledgeGraphProvenanceEvent struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	OwnerIdentity string    `gorm:"type:varchar(255);not null;uniqueIndex:uq_knowledge_graph_provenance_revision,priority:1;index:idx_knowledge_graph_provenance_entity,priority:1" json:"-"`
	EntityType    string    `gorm:"type:varchar(16);not null;uniqueIndex:uq_knowledge_graph_provenance_revision,priority:2;index:idx_knowledge_graph_provenance_entity,priority:2" json:"entityType"`
	EntityID      string    `gorm:"type:varchar(160);not null;uniqueIndex:uq_knowledge_graph_provenance_revision,priority:3;index:idx_knowledge_graph_provenance_entity,priority:3" json:"entityId"`
	Revision      uint64    `gorm:"type:bigint;not null;uniqueIndex:uq_knowledge_graph_provenance_revision,priority:4" json:"revision"`
	Operation     string    `gorm:"type:varchar(40);not null" json:"operation"`
	SourcesJSON   string    `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	RecordedAt    time.Time `gorm:"not null;index:idx_knowledge_graph_provenance_entity,priority:4,sort:desc" json:"recordedAt"`
}

func (KnowledgeGraphProvenanceEvent) TableName() string {
	return "knowledge_graph_provenance_events"
}

// KnowledgeGraphConflictRecord is the immutable membership history for one
// preserved conflict group.
type KnowledgeGraphConflictRecord struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	OwnerIdentity   string    `gorm:"type:varchar(255);not null;uniqueIndex:uq_knowledge_graph_conflict_member,priority:1;index:idx_knowledge_graph_conflict_group,priority:1" json:"-"`
	ConflictGroupID string    `gorm:"type:varchar(160);not null;uniqueIndex:uq_knowledge_graph_conflict_member,priority:2;index:idx_knowledge_graph_conflict_group,priority:2" json:"conflictGroupId"`
	NodeID          string    `gorm:"type:varchar(160);not null;uniqueIndex:uq_knowledge_graph_conflict_member,priority:3" json:"nodeId"`
	DetectedAt      time.Time `gorm:"not null" json:"detectedAt"`
}

func (KnowledgeGraphConflictRecord) TableName() string {
	return "knowledge_graph_conflict_records"
}

// KnowledgeGraphDeletionSignal is an immutable tombstone propagation receipt.
type KnowledgeGraphDeletionSignal struct {
	ID                    string    `gorm:"type:varchar(160);primaryKey" json:"id"`
	OwnerIdentity         string    `gorm:"type:varchar(255);not null;uniqueIndex:uq_knowledge_graph_deletion_entity,priority:1;index:idx_knowledge_graph_deletions_owner_created,priority:1" json:"-"`
	EntityType            string    `gorm:"type:varchar(16);not null;uniqueIndex:uq_knowledge_graph_deletion_entity,priority:2" json:"entityType"`
	EntityID              string    `gorm:"type:varchar(160);not null;uniqueIndex:uq_knowledge_graph_deletion_entity,priority:3" json:"entityId"`
	PropagatedEdgeIDsJSON string    `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	Reason                string    `gorm:"type:text;not null" json:"reason"`
	CreatedAt             time.Time `gorm:"not null;index:idx_knowledge_graph_deletions_owner_created,priority:2" json:"createdAt"`
}

func (KnowledgeGraphDeletionSignal) TableName() string {
	return "knowledge_graph_deletion_signals"
}
