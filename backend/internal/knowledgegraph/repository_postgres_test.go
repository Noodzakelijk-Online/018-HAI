package knowledgegraph

import (
	"errors"
	"testing"
	"time"

	"automation-hub-backend/internal/models"
)

func TestKnowledgeGraphModelRoundTripPreservesTypedContent(t *testing.T) {
	now := time.Date(2026, time.July, 30, 18, 0, 0, 0, time.UTC)
	validUntil := now.Add(24 * time.Hour)
	deletedAt := now.Add(2 * time.Hour)
	node := Node{
		ID: "node-round-trip", OwnerIdentity: "robert", Kind: NodeClaim,
		DeduplicationKey: "claim|round-trip", Label: "Round trip",
		Content: "Evidence-backed content", Properties: map[string]string{"case": "A-1"},
		ProjectKeys: []string{"hai"}, Tags: []string{"evidence"},
		Confidence: 0.875, VerificationStatus: VerificationSourceSupported,
		Sources: []SourceReference{{
			ID: "source-1", URI: "file:///evidence.pdf", Label: "Evidence",
			Excerpt: "supporting text", ContentHash: "sha256:value",
			Authority: "primary", CapturedAt: now, LocalOnly: true,
		}},
		ValidFrom: &now, ValidUntil: &validUntil,
		Sensitivity: SensitivityRestricted, LocalOnly: true,
		ConflictGroupID: "conflict-1", SupersedesID: "node-old",
		Archived: true, CreatedAt: now, UpdatedAt: deletedAt, DeletedAt: &deletedAt,
	}
	row, err := nodeToModel(node, 3, deletedAt)
	if err != nil {
		t.Fatalf("nodeToModel: %v", err)
	}
	decoded, err := nodeFromModel(row)
	if err != nil {
		t.Fatalf("nodeFromModel: %v", err)
	}
	if decoded.ID != node.ID ||
		decoded.Properties["case"] != "A-1" ||
		len(decoded.Sources) != 1 ||
		decoded.Sources[0].Excerpt != "supporting text" ||
		decoded.DeletedAt == nil ||
		!decoded.DeletedAt.Equal(deletedAt) {
		t.Fatalf("node round trip lost content: %#v", decoded)
	}

	edge := Edge{
		ID: "edge-round-trip", OwnerIdentity: "robert",
		FromNodeID: "node-a", ToNodeID: "node-b",
		Relationship: RelationEvidencedBy, Label: "Evidence edge",
		Properties:  map[string]string{"weight": "high"},
		ProjectKeys: []string{"hai"}, Confidence: 0.9,
		VerificationStatus: VerificationVerified,
		Sources:            node.Sources,
		ValidFrom:          &now,
		ValidUntil:         &validUntil,
		Sensitivity:        SensitivitySensitive,
		LocalOnly:          true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	edgeRow, err := edgeToModel(edge, 1, now)
	if err != nil {
		t.Fatalf("edgeToModel: %v", err)
	}
	decodedEdge, err := edgeFromModel(edgeRow)
	if err != nil {
		t.Fatalf("edgeFromModel: %v", err)
	}
	if decodedEdge.Relationship != edge.Relationship ||
		decodedEdge.Properties["weight"] != "high" ||
		len(decodedEdge.Sources) != 1 {
		t.Fatalf("edge round trip lost content: %#v", decodedEdge)
	}
}

func TestKnowledgeGraphModelDecodingFailsClosedOnCorruptJSON(t *testing.T) {
	now := time.Date(2026, time.July, 30, 18, 0, 0, 0, time.UTC)
	base := models.KnowledgeGraphNode{
		ID: "node-corrupt", OwnerIdentity: "robert", Kind: string(NodeClaim),
		DeduplicationKey: "claim|corrupt", Label: "Corrupt",
		PropertiesJSON: "{}", ProjectKeysJSON: "[]", TagsJSON: "[]",
		Confidence: 0.8, VerificationStatus: string(VerificationSourceSupported),
		SourcesJSON: "[]", Sensitivity: string(SensitivityInternal),
		Revision: 1, TransactionFrom: now, CreatedAt: now, UpdatedAt: now,
	}
	tests := []struct {
		name   string
		mutate func(*models.KnowledgeGraphNode)
	}{
		{
			name: "wrong top-level shape",
			mutate: func(row *models.KnowledgeGraphNode) {
				row.ProjectKeysJSON = `{}`
			},
		},
		{
			name: "incompatible object value",
			mutate: func(row *models.KnowledgeGraphNode) {
				row.PropertiesJSON = `{"priority": 7}`
			},
		},
		{
			name: "null provenance",
			mutate: func(row *models.KnowledgeGraphNode) {
				row.SourcesJSON = `null`
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			row := base
			test.mutate(&row)
			_, err := nodeFromModel(row)
			if !errors.Is(err, ErrCorruptStorage) {
				t.Fatalf("nodeFromModel error = %v, want ErrCorruptStorage", err)
			}
		})
	}
}

func TestNilKnowledgeGraphRepositoryFailsClosed(t *testing.T) {
	var repository *GormRepository
	if _, err := repository.ListNodes(
		t.Context(),
		"robert",
		ListOptions{},
	); err == nil {
		t.Fatal("nil repository did not fail closed")
	}
}
