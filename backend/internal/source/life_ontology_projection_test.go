package source

import (
	"automation-hub-backend/internal/lifeontology"
	"automation-hub-backend/internal/models"
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestSyncProjectsDurableExtractionIntoOwnerLifeGraph(t *testing.T) {
	sourceID := uuid.New()
	repo := newFakeSourceRepo(&models.ConnectedSource{
		ID: sourceID, OwnerIdentity: "owner-1", ConnectorKey: "local-folder",
		Name: "Private project files", Category: "document", Enabled: true,
		LocalOnly: true, Status: "active", DefaultProjectKey: "018-HAI",
	})
	graph := lifeontology.NewService(lifeontology.NewMemoryRepository(), nil)
	configured, err := WithLifeOntologyProjection(NewService(repo, nil), graph)
	if err != nil {
		t.Fatalf("WithLifeOntologyProjection: %v", err)
	}

	result, err := configured.Sync(sourceID, ImportRequest{Mode: ModeManualImport, Items: []ImportItem{{
		ExternalID: "doc-1", Title: "Architecture decision", ItemType: "document",
		ProjectKey: "018-HAI", SourceURI: "file:///private/architecture.md",
		Content: "Decision: keep one canonical Go and Angular product stack with source-backed operational records.",
	}}})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Job.Status != "completed" || len(result.Errors) != 0 || len(result.Warnings) != 0 {
		t.Fatalf("unexpected sync result: %#v", result)
	}
	if len(result.LifeGraphProjections) != 1 {
		t.Fatalf("life graph projections = %#v, want one", result.LifeGraphProjections)
	}
	projection := result.LifeGraphProjections[0]
	if !projection.AdvisoryOnly || projection.CanExecute || projection.GrantsAuthority {
		t.Fatalf("projection authority boundary violated: %#v", projection)
	}
	if len(projection.LinkedEntityIDs) != 2 || len(projection.RelationIDs) != 2 {
		t.Fatalf("projection links = %#v, relations = %#v", projection.LinkedEntityIDs, projection.RelationIDs)
	}

	entities, err := graph.QueryEntities(context.Background(), "owner-1", lifeontology.EntityQuery{
		AllowLocalOnly: true, Limit: 20,
	})
	if err != nil {
		t.Fatalf("QueryEntities: %v", err)
	}
	if len(entities) != 3 {
		t.Fatalf("entities = %#v, want document, source, and project", entities)
	}
	for _, entity := range entities {
		if !entity.LocalOnly {
			t.Fatalf("entity %s lost local-only boundary", entity.ID)
		}
	}
	if !repo.hasAudit("life_graph.projected") {
		t.Fatalf("expected successful graph projection audit")
	}

	second, err := configured.Sync(sourceID, ImportRequest{Mode: ModeIncrementalSync, Items: []ImportItem{{
		ExternalID: "doc-1", Title: "Architecture decision", ItemType: "document",
		ProjectKey: "018-HAI", SourceURI: "file:///private/architecture.md",
		Content: "Decision: keep one canonical Go and Angular product stack with source-backed operational records.",
	}}})
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if len(second.LifeGraphProjections) != 1 || !second.LifeGraphProjections[0].AlreadyExisted {
		t.Fatalf("repeat projection was not idempotent: %#v", second.LifeGraphProjections)
	}
}

func TestSyncKeepsIngestionSuccessfulWhenLifeGraphProjectionFails(t *testing.T) {
	sourceID := uuid.New()
	repo := newFakeSourceRepo(&models.ConnectedSource{
		ID: sourceID, OwnerIdentity: "owner-1", ConnectorKey: "local-folder",
		Name: "Project files", Category: "document", Enabled: true, LocalOnly: true, Status: "active",
	})
	configured, err := WithLifeOntologyProjection(NewService(repo, nil), failingLifeOntologyProjector{})
	if err != nil {
		t.Fatalf("WithLifeOntologyProjection: %v", err)
	}
	result, err := configured.Sync(sourceID, ImportRequest{Items: []ImportItem{{
		ExternalID: "doc-2", Title: "Source record", ItemType: "document",
		Content: "A sufficiently detailed source record that remains searchable even when graph projection is unavailable.",
	}}})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Job.Status != "completed" || len(result.Extractions) != 1 || len(result.Errors) != 0 {
		t.Fatalf("graph outage falsely failed ingestion: %#v", result)
	}
	if len(result.Warnings) != 1 || len(result.LifeGraphProjections) != 0 {
		t.Fatalf("projection failure visibility = %#v", result)
	}
	if !repo.hasAudit("life_graph.projection_failed") {
		t.Fatalf("expected failed graph projection audit")
	}
}

func TestSyncProjectsContactCandidatesAsNeedsReviewOnly(t *testing.T) {
	sourceID := uuid.New()
	repo := newFakeSourceRepo(&models.ConnectedSource{
		ID: sourceID, OwnerIdentity: "owner-1", ConnectorKey: "gmail",
		Name: "Owner email", Category: "email", Enabled: true, LocalOnly: true, Status: "active",
	})
	graph := lifeontology.NewService(lifeontology.NewMemoryRepository(), nil)
	configured, err := WithLifeOntologyProjection(NewService(repo, nil), graph)
	if err != nil {
		t.Fatal(err)
	}
	result, err := configured.Sync(sourceID, ImportRequest{Items: []ImportItem{{
		ExternalID: "mail-1", Title: "Planning update", ItemType: "email",
		Content: "Joyce Jorayev confirmed that Robert should review the planning update tomorrow.",
	}}})
	if err != nil || len(result.LifeGraphProjections) != 1 {
		t.Fatalf("Sync() result=%#v err=%v", result, err)
	}
	people, err := graph.QueryEntities(context.Background(), "owner-1", lifeontology.EntityQuery{
		Types: []lifeontology.EntityType{lifeontology.EntityPerson}, AllowLocalOnly: true, Limit: 20,
	})
	if err != nil || len(people) < 3 {
		t.Fatalf("contact candidates = %#v err=%v", people, err)
	}
	for _, person := range people {
		if person.VerificationStatus != lifeontology.VerificationNeedsReview ||
			person.Confidence != 0.35 || person.Attributes["candidate"] != "true" ||
			person.Attributes["review_required"] != "true" || !person.LocalOnly {
			t.Fatalf("contact candidate was promoted beyond review material: %#v", person)
		}
	}
	otherOwner, err := graph.QueryEntities(context.Background(), "other-owner", lifeontology.EntityQuery{
		Types: []lifeontology.EntityType{lifeontology.EntityPerson}, AllowLocalOnly: true, Limit: 20,
	})
	if err != nil || len(otherOwner) != 0 {
		t.Fatalf("cross-owner contact leak = %#v err=%v", otherOwner, err)
	}
}

type failingLifeOntologyProjector struct{}

func (failingLifeOntologyProjector) ProjectOperationalRecord(context.Context, lifeontology.OperationalProjectionRequest) (lifeontology.OperationalProjectionResult, error) {
	return lifeontology.OperationalProjectionResult{}, errors.New("graph temporarily unavailable")
}
