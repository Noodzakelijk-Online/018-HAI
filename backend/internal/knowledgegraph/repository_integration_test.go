//go:build integration

package knowledgegraph

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/models"
	"automation-hub-backend/migrations"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func knowledgeGraphIntegrationRepository(t *testing.T) (*GormRepository, *gorm.DB) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("HAI_TEST_DATABASE_DSN"))
	if dsn == "" {
		t.Skip("HAI_TEST_DATABASE_DSN not set; skipping Postgres integration test")
	}
	if !strings.EqualFold(
		strings.TrimSpace(os.Getenv("HAI_ALLOW_DESTRUCTIVE_DATABASE_TESTS")),
		"true",
	) {
		t.Skip("HAI_ALLOW_DESTRUCTIVE_DATABASE_TESTS=true is required")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	var databaseName string
	if err := db.Raw("SELECT current_database()").Scan(&databaseName).Error; err != nil {
		t.Fatalf("read current database: %v", err)
	}
	if !strings.Contains(strings.ToLower(databaseName), "test") {
		t.Fatalf("refusing destructive integration setup for database %q", databaseName)
	}
	if err := db.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public;").Error; err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	for _, path := range []string{
		"pre/0001_extensions.up.sql",
		"pre/0011_knowledge_graph.up.sql",
	} {
		sql, err := migrations.Files.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", path, err)
		}
		if err := db.Exec(string(sql)).Error; err != nil {
			t.Fatalf("apply migration %s: %v", path, err)
		}
	}
	return NewGormRepository(db), db
}

func TestKnowledgeGraphPostgresOwnerRoundTripCorrectionAndTombstones(t *testing.T) {
	repository, db := knowledgeGraphIntegrationRepository(t)
	ctx := context.Background()
	now := time.Date(2026, time.July, 30, 18, 0, 0, 0, time.UTC)
	service := NewService(repository, func() time.Time { return now })

	source := createTestNode(t, service, "robert", NodeSource, "Primary record", func(request *CreateNodeRequest) {
		request.Content = "Original evidence"
		request.Sources = []SourceReference{{
			ID: "source-record", URI: "file:///primary-record.txt",
			Authority: "primary", CapturedAt: now, LocalOnly: true,
		}}
	})
	claim := createTestNode(t, service, "robert", NodeClaim, "Supported claim", func(request *CreateNodeRequest) {
		request.Content = "Initial claim"
		request.DeduplicationKey = "claim|supported"
		request.Properties = map[string]string{"case": "VIVARE-1"}
		request.ProjectKeys = []string{"vivare"}
		request.Tags = []string{"legal"}
		request.Sources = []SourceReference{{
			SourceNodeID: source.ID, Excerpt: "Original evidence",
			Authority: "primary", CapturedAt: now, LocalOnly: true,
		}}
		request.LocalOnly = true
	})
	if _, err := service.GetNode(ctx, "other-owner", claim.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner node read error = %v, want ErrNotFound", err)
	}
	otherNodes, err := service.ListNodes(ctx, "other-owner", ListOptions{
		IncludeArchived: true,
		IncludeDeleted:  true,
	})
	if err != nil || len(otherNodes) != 0 {
		t.Fatalf("cross-owner node list leaked data: %#v err=%v", otherNodes, err)
	}

	edgeResult, err := service.CreateEdge(ctx, CreateEdgeRequest{
		OwnerIdentity: "robert", FromNodeID: claim.ID, ToNodeID: source.ID,
		Relationship: RelationEvidencedBy, Label: "Supported by",
		Properties:  map[string]string{"strength": "direct"},
		ProjectKeys: []string{"vivare"}, Confidence: 0.95,
		VerificationStatus: VerificationSourceSupported,
		Sources: []SourceReference{{
			SourceNodeID: source.ID, Excerpt: "Original evidence",
			Authority: "primary", CapturedAt: now, LocalOnly: true,
		}},
		Sensitivity: SensitivityRestricted, LocalOnly: true,
	})
	if err != nil {
		t.Fatalf("CreateEdge: %v", err)
	}
	storedEdge, err := service.GetEdge(ctx, "robert", edgeResult.Edge.ID)
	if err != nil {
		t.Fatalf("GetEdge: %v", err)
	}
	if storedEdge.Properties["strength"] != "direct" ||
		len(storedEdge.Sources) != 1 ||
		!storedEdge.LocalOnly {
		t.Fatalf("edge round trip lost data: %#v", storedEdge)
	}
	if _, err := service.GetEdge(ctx, "other-owner", storedEdge.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner edge read error = %v, want ErrNotFound", err)
	}

	now = now.Add(time.Minute)
	corrected, err := service.CorrectNode(ctx, "robert", claim.ID, CreateNodeRequest{
		Content: "Corrected claim", Confidence: 1,
		VerificationStatus: VerificationHumanApproved,
		Sources: []SourceReference{{
			ID: "owner-correction", URI: "local://owner-confirmation",
			Authority: "owner", CapturedAt: now, LocalOnly: true,
		}},
	})
	if err != nil {
		t.Fatalf("CorrectNode: %v", err)
	}
	if corrected.Node.SupersedesID != claim.ID || corrected.Action != WriteCorrected {
		t.Fatalf("correction link missing: %#v", corrected)
	}
	original, err := service.GetNode(ctx, "robert", claim.ID)
	if err != nil {
		t.Fatalf("GetNode original: %v", err)
	}
	if !original.Archived || original.CorrectedByID != corrected.Node.ID {
		t.Fatalf("original correction state = %#v", original)
	}
	var originalRevisions int64
	if err := db.Model(&models.KnowledgeGraphNodeRevision{}).
		Where("owner_identity = ? AND node_id = ?", "robert", claim.ID).
		Count(&originalRevisions).Error; err != nil {
		t.Fatalf("count original revisions: %v", err)
	}
	if originalRevisions != 2 {
		t.Fatalf("original revisions = %d, want 2", originalRevisions)
	}
	var replacementRevisions int64
	if err := db.Model(&models.KnowledgeGraphNodeRevision{}).
		Where("owner_identity = ? AND node_id = ?", "robert", corrected.Node.ID).
		Count(&replacementRevisions).Error; err != nil {
		t.Fatalf("count replacement revisions: %v", err)
	}
	if replacementRevisions != 1 {
		t.Fatalf("replacement revisions = %d, want 1", replacementRevisions)
	}
	var provenanceEvents int64
	if err := db.Model(&models.KnowledgeGraphProvenanceEvent{}).
		Where(
			"owner_identity = ? AND entity_type = ? AND entity_id IN ?",
			"robert",
			EntityNode,
			[]string{claim.ID, corrected.Node.ID},
		).
		Count(&provenanceEvents).Error; err != nil {
		t.Fatalf("count provenance: %v", err)
	}
	if provenanceEvents != 3 {
		t.Fatalf("provenance events = %d, want 3", provenanceEvents)
	}

	now = now.Add(time.Minute)
	archived, err := service.ArchiveNode(ctx, "robert", source.ID, true)
	if err != nil {
		t.Fatalf("ArchiveNode: %v", err)
	}
	if !archived.Archived {
		t.Fatal("archive state was not persisted")
	}
	activeNodes, err := service.ListNodes(ctx, "robert", ListOptions{})
	if err != nil {
		t.Fatalf("ListNodes active: %v", err)
	}
	for _, node := range activeNodes {
		if node.ID == source.ID || node.ID == claim.ID {
			t.Fatalf("archived node %s remained active", node.ID)
		}
	}
	if _, err := service.ArchiveNode(ctx, "robert", source.ID, false); err != nil {
		t.Fatalf("restore archived node: %v", err)
	}

	now = now.Add(time.Minute)
	signal, err := service.DeleteNode(ctx, "robert", source.ID, "owner requested deletion")
	if err != nil {
		t.Fatalf("DeleteNode: %v", err)
	}
	if signal.EntityType != EntityNode ||
		signal.EntityID != source.ID ||
		len(signal.PropagatedEdgeIDs) != 1 ||
		signal.PropagatedEdgeIDs[0] != storedEdge.ID {
		t.Fatalf("deletion propagation = %#v", signal)
	}
	repeated, err := service.DeleteNode(ctx, "robert", source.ID, "retry")
	if err != nil {
		t.Fatalf("idempotent DeleteNode: %v", err)
	}
	if repeated.ID != signal.ID || repeated.Reason != signal.Reason {
		t.Fatalf("idempotent deletion returned a new signal: %#v", repeated)
	}
	if _, err := service.DeleteNode(ctx, "other-owner", source.ID, "cross-owner"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner deletion error = %v, want ErrNotFound", err)
	}
	tombstonedEdges, err := service.ListEdges(ctx, "robert", ListOptions{
		IncludeArchived: true,
		IncludeDeleted:  true,
	})
	if err != nil || len(tombstonedEdges) != 1 || tombstonedEdges[0].DeletedAt == nil {
		t.Fatalf("tombstoned edges = %#v err=%v", tombstonedEdges, err)
	}
	otherSignals, err := service.ListDeletionSignals(ctx, "other-owner")
	if err != nil || len(otherSignals) != 0 {
		t.Fatalf("deletion signal leaked across owners: %#v err=%v", otherSignals, err)
	}

	if err := db.Exec(
		"DELETE FROM knowledge_graph_nodes WHERE id = ?",
		source.ID,
	).Error; err == nil {
		t.Fatal("Postgres allowed physical node deletion")
	}
	if err := db.Exec(
		"UPDATE knowledge_graph_node_revisions SET operation = 'updated' WHERE node_id = ?",
		claim.ID,
	).Error; err == nil {
		t.Fatal("Postgres allowed immutable revision mutation")
	}
	if err := db.Exec(
		"UPDATE knowledge_graph_provenance_events SET operation = 'updated' WHERE entity_id = ?",
		claim.ID,
	).Error; err == nil {
		t.Fatal("Postgres allowed immutable provenance mutation")
	}
}

func TestKnowledgeGraphPostgresConflictsAndCorruptJSONFailClosed(t *testing.T) {
	repository, db := knowledgeGraphIntegrationRepository(t)
	ctx := context.Background()
	now := time.Date(2026, time.July, 30, 19, 0, 0, 0, time.UTC)
	service := NewService(repository, func() time.Time { return now })

	first := createTestNode(t, service, "robert", NodePreference, "Contact tone", func(request *CreateNodeRequest) {
		request.DeduplicationKey = "contact-tone"
		request.Content = "Formal"
	})
	secondResult, err := service.CreateNode(ctx, CreateNodeRequest{
		OwnerIdentity: "robert", Kind: NodePreference,
		DeduplicationKey: "contact-tone", Label: "Contact tone",
		Content: "Informal", Confidence: 0.8,
		VerificationStatus: VerificationSourceSupported,
		Sensitivity:        SensitivityInternal,
	})
	if err != nil {
		t.Fatalf("create conflict: %v", err)
	}
	if secondResult.Action != WriteConflict ||
		secondResult.Node.ConflictGroupID == "" {
		t.Fatalf("conflict result = %#v", secondResult)
	}
	var conflictMembers int64
	if err := db.Model(&models.KnowledgeGraphConflictRecord{}).
		Where(
			"owner_identity = ? AND conflict_group_id = ?",
			"robert",
			secondResult.Node.ConflictGroupID,
		).
		Count(&conflictMembers).Error; err != nil {
		t.Fatalf("count conflict members: %v", err)
	}
	if conflictMembers != 2 {
		t.Fatalf("conflict members = %d, want 2", conflictMembers)
	}

	if err := db.Exec(`
		UPDATE knowledge_graph_nodes
		SET properties_json = '{"priority": 7}'::jsonb,
		    revision = revision + 1,
		    transaction_from = ?,
		    updated_at = ?
		WHERE owner_identity = ? AND id = ?
	`, now.Add(time.Minute), now.Add(time.Minute), "robert", first.ID).Error; err != nil {
		t.Fatalf("inject structurally valid corrupt JSON: %v", err)
	}
	if _, err := service.GetNode(ctx, "robert", first.ID); !errors.Is(err, ErrCorruptStorage) {
		t.Fatalf("corrupt stored JSON error = %v, want ErrCorruptStorage", err)
	}
}

func TestKnowledgeGraphPostgresMigrationRerunAndRollback(t *testing.T) {
	_, db := knowledgeGraphIntegrationRepository(t)
	upSQL, err := migrations.Files.ReadFile("pre/0011_knowledge_graph.up.sql")
	if err != nil {
		t.Fatalf("read up migration: %v", err)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		if err := db.Exec(string(upSQL)).Error; err != nil {
			t.Fatalf("rerun up migration %d: %v", attempt, err)
		}
	}
	for _, table := range []string{
		"knowledge_graph_nodes",
		"knowledge_graph_edges",
		"knowledge_graph_node_revisions",
		"knowledge_graph_edge_revisions",
		"knowledge_graph_provenance_events",
		"knowledge_graph_conflict_records",
		"knowledge_graph_deletion_signals",
	} {
		var exists bool
		if err := db.Raw(
			"SELECT to_regclass(?) IS NOT NULL",
			"public."+table,
		).Scan(&exists).Error; err != nil {
			t.Fatalf("query table %s: %v", table, err)
		}
		if !exists {
			t.Fatalf("migration did not create %s", table)
		}
	}

	downSQL, err := migrations.Files.ReadFile("pre/0011_knowledge_graph.down.sql")
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}
	if err := db.Exec(string(downSQL)).Error; err != nil {
		t.Fatalf("run down migration: %v", err)
	}
	for _, table := range []string{
		"knowledge_graph_nodes",
		"knowledge_graph_edges",
		"knowledge_graph_node_revisions",
		"knowledge_graph_edge_revisions",
		"knowledge_graph_provenance_events",
		"knowledge_graph_conflict_records",
		"knowledge_graph_deletion_signals",
	} {
		var exists bool
		if err := db.Raw(
			"SELECT to_regclass(?) IS NOT NULL",
			"public."+table,
		).Scan(&exists).Error; err != nil {
			t.Fatalf("query rolled-back table %s: %v", table, err)
		}
		if exists {
			t.Fatalf("rollback left %s behind", table)
		}
	}
}
