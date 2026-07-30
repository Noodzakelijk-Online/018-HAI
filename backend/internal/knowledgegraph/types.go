package knowledgegraph

import (
	"context"
	"time"
)

// NodeKind is the closed ontology used by HAI's core graph. Domain-specific
// detail belongs in Properties rather than in ad-hoc node kinds.
type NodeKind string

const (
	NodePerson       NodeKind = "person"
	NodeOrganization NodeKind = "organization"
	NodeProject      NodeKind = "project"
	NodeGoal         NodeKind = "goal"
	NodeTask         NodeKind = "task"
	NodeEvent        NodeKind = "event"
	NodeDocument     NodeKind = "document"
	NodeSource       NodeKind = "source"
	NodeClaim        NodeKind = "claim"
	NodePreference   NodeKind = "preference"
	NodeDecision     NodeKind = "decision"
	NodeObligation   NodeKind = "obligation"
	NodeDeadline     NodeKind = "deadline"
	NodePlace        NodeKind = "place"
	NodeAccount      NodeKind = "account"
	NodeCapability   NodeKind = "capability"
)

// RelationshipKind is deliberately bounded. It makes traversal and policy
// decisions inspectable instead of allowing opaque caller-defined predicates.
type RelationshipKind string

const (
	RelationRelatedTo   RelationshipKind = "related_to"
	RelationMemberOf    RelationshipKind = "member_of"
	RelationOwns        RelationshipKind = "owns"
	RelationWorksOn     RelationshipKind = "works_on"
	RelationSupports    RelationshipKind = "supports"
	RelationDependsOn   RelationshipKind = "depends_on"
	RelationParentOf    RelationshipKind = "parent_of"
	RelationAssignedTo  RelationshipKind = "assigned_to"
	RelationCausedBy    RelationshipKind = "caused_by"
	RelationDerivedFrom RelationshipKind = "derived_from"
	RelationEvidencedBy RelationshipKind = "evidenced_by"
	RelationContradicts RelationshipKind = "contradicts"
	RelationConfirms    RelationshipKind = "confirms"
	RelationPrefers     RelationshipKind = "prefers"
	RelationDecided     RelationshipKind = "decided"
	RelationObligatedTo RelationshipKind = "obligated_to"
	RelationDueAt       RelationshipKind = "due_at"
	RelationLocatedAt   RelationshipKind = "located_at"
	RelationCapableOf   RelationshipKind = "capable_of"
	RelationMentions    RelationshipKind = "mentions"
	RelationSupersedes  RelationshipKind = "supersedes"
	RelationCorrectedBy RelationshipKind = "corrected_by"
)

type VerificationStatus string

const (
	VerificationUnverified      VerificationStatus = "unverified"
	VerificationSourceSupported VerificationStatus = "source_supported"
	VerificationSchemaValidated VerificationStatus = "schema_validated"
	VerificationTestPassed      VerificationStatus = "test_passed"
	VerificationHumanApproved   VerificationStatus = "human_approved"
	VerificationVerified        VerificationStatus = "verified"
	VerificationUncertain       VerificationStatus = "uncertain"
	VerificationConflicting     VerificationStatus = "conflicting"
	VerificationUnsupported     VerificationStatus = "unsupported"
	VerificationNeedsReview     VerificationStatus = "needs_review"
)

type Sensitivity string

const (
	SensitivityPublic     Sensitivity = "public"
	SensitivityInternal   Sensitivity = "internal"
	SensitivitySensitive  Sensitivity = "sensitive"
	SensitivityRestricted Sensitivity = "restricted"
)

// SourceReference ties graph knowledge back to evidence. A reference must have
// at least one of ID, URI, or SourceNodeID.
type SourceReference struct {
	ID           string    `json:"id,omitempty"`
	URI          string    `json:"uri,omitempty"`
	Label        string    `json:"label,omitempty"`
	SourceNodeID string    `json:"sourceNodeId,omitempty"`
	Excerpt      string    `json:"excerpt,omitempty"`
	ContentHash  string    `json:"contentHash,omitempty"`
	Authority    string    `json:"authority,omitempty"`
	CapturedAt   time.Time `json:"capturedAt,omitempty"`
	LocalOnly    bool      `json:"localOnly"`
}

type Node struct {
	ID                 string             `json:"id"`
	OwnerIdentity      string             `json:"ownerIdentity"`
	Kind               NodeKind           `json:"kind"`
	DeduplicationKey   string             `json:"deduplicationKey"`
	Label              string             `json:"label"`
	Content            string             `json:"content,omitempty"`
	Properties         map[string]string  `json:"properties,omitempty"`
	ProjectKeys        []string           `json:"projectKeys,omitempty"`
	Tags               []string           `json:"tags,omitempty"`
	Confidence         float64            `json:"confidence"`
	VerificationStatus VerificationStatus `json:"verificationStatus"`
	Sources            []SourceReference  `json:"sources,omitempty"`
	ValidFrom          *time.Time         `json:"validFrom,omitempty"`
	ValidUntil         *time.Time         `json:"validUntil,omitempty"`
	Sensitivity        Sensitivity        `json:"sensitivity"`
	LocalOnly          bool               `json:"localOnly"`
	ConflictGroupID    string             `json:"conflictGroupId,omitempty"`
	SupersedesID       string             `json:"supersedesId,omitempty"`
	CorrectedByID      string             `json:"correctedById,omitempty"`
	Archived           bool               `json:"archived"`
	CreatedAt          time.Time          `json:"createdAt"`
	UpdatedAt          time.Time          `json:"updatedAt"`
	DeletedAt          *time.Time         `json:"deletedAt,omitempty"`
}

type Edge struct {
	ID                 string             `json:"id"`
	OwnerIdentity      string             `json:"ownerIdentity"`
	FromNodeID         string             `json:"fromNodeId"`
	ToNodeID           string             `json:"toNodeId"`
	Relationship       RelationshipKind   `json:"relationship"`
	Label              string             `json:"label,omitempty"`
	Properties         map[string]string  `json:"properties,omitempty"`
	ProjectKeys        []string           `json:"projectKeys,omitempty"`
	Confidence         float64            `json:"confidence"`
	VerificationStatus VerificationStatus `json:"verificationStatus"`
	Sources            []SourceReference  `json:"sources,omitempty"`
	ValidFrom          *time.Time         `json:"validFrom,omitempty"`
	ValidUntil         *time.Time         `json:"validUntil,omitempty"`
	Sensitivity        Sensitivity        `json:"sensitivity"`
	LocalOnly          bool               `json:"localOnly"`
	Archived           bool               `json:"archived"`
	CreatedAt          time.Time          `json:"createdAt"`
	UpdatedAt          time.Time          `json:"updatedAt"`
	DeletedAt          *time.Time         `json:"deletedAt,omitempty"`
}

type CreateNodeRequest struct {
	OwnerIdentity      string
	Kind               NodeKind
	DeduplicationKey   string
	Label              string
	Content            string
	Properties         map[string]string
	ProjectKeys        []string
	Tags               []string
	Confidence         float64
	VerificationStatus VerificationStatus
	Sources            []SourceReference
	ValidFrom          *time.Time
	ValidUntil         *time.Time
	Sensitivity        Sensitivity
	LocalOnly          bool
}

type CreateEdgeRequest struct {
	OwnerIdentity      string
	FromNodeID         string
	ToNodeID           string
	Relationship       RelationshipKind
	Label              string
	Properties         map[string]string
	ProjectKeys        []string
	Confidence         float64
	VerificationStatus VerificationStatus
	Sources            []SourceReference
	ValidFrom          *time.Time
	ValidUntil         *time.Time
	Sensitivity        Sensitivity
	LocalOnly          bool
}

type WriteAction string

const (
	WriteCreated   WriteAction = "created"
	WriteMerged    WriteAction = "merged"
	WriteConflict  WriteAction = "conflict_preserved"
	WriteCorrected WriteAction = "corrected"
)

type NodeWriteResult struct {
	Node               Node        `json:"node"`
	Action             WriteAction `json:"action"`
	ConflictingNodeIDs []string    `json:"conflictingNodeIds,omitempty"`
}

type EdgeWriteResult struct {
	Edge   Edge        `json:"edge"`
	Action WriteAction `json:"action"`
}

type EntityType string

const (
	EntityNode EntityType = "node"
	EntityEdge EntityType = "edge"
)

// DeletionSignal lets indexes, caches, embeddings, and downstream memories
// remove derived material without losing the fact that deletion occurred.
type DeletionSignal struct {
	ID                string     `json:"id"`
	OwnerIdentity     string     `json:"ownerIdentity"`
	EntityType        EntityType `json:"entityType"`
	EntityID          string     `json:"entityId"`
	PropagatedEdgeIDs []string   `json:"propagatedEdgeIds,omitempty"`
	Reason            string     `json:"reason"`
	CreatedAt         time.Time  `json:"createdAt"`
}

type ListOptions struct {
	IncludeArchived bool
	IncludeDeleted  bool
}

type RetrieveRequest struct {
	OwnerIdentity   string
	Query           string
	ProjectKeys     []string
	At              time.Time
	MaxDepth        int
	Limit           int
	AllowLocalOnly  bool
	IncludeArchived bool
}

type RetrievalExplanation struct {
	NodeID      string   `json:"nodeId"`
	Score       float64  `json:"score"`
	Depth       int      `json:"depth"`
	DirectMatch bool     `json:"directMatch"`
	Path        []string `json:"path"`
	Factors     []string `json:"factors"`
	Summary     string   `json:"summary"`
}

type RetrievedNode struct {
	Node        Node                 `json:"node"`
	Explanation RetrievalExplanation `json:"explanation"`
}

type SubgraphResult struct {
	Nodes       []RetrievedNode `json:"nodes"`
	Edges       []Edge          `json:"edges"`
	Query       string          `json:"query"`
	ProjectKeys []string        `json:"projectKeys,omitempty"`
	MaxDepth    int             `json:"maxDepth"`
	Limit       int             `json:"limit"`
	Truncated   bool            `json:"truncated"`
	Explanation string          `json:"explanation"`
}

// Repository is owner-scoped at every read and mutation boundary.
type Repository interface {
	CreateNode(context.Context, Node) (Node, error)
	UpdateNode(context.Context, Node) (Node, error)
	GetNode(context.Context, string, string) (Node, error)
	ListNodes(context.Context, string, ListOptions) ([]Node, error)

	CreateEdge(context.Context, Edge) (Edge, error)
	UpdateEdge(context.Context, Edge) (Edge, error)
	GetEdge(context.Context, string, string) (Edge, error)
	ListEdges(context.Context, string, ListOptions) ([]Edge, error)

	DeleteNode(context.Context, string, string, string, time.Time) (DeletionSignal, error)
	DeleteEdge(context.Context, string, string, string, time.Time) (DeletionSignal, error)
	ListDeletionSignals(context.Context, string) ([]DeletionSignal, error)
}
