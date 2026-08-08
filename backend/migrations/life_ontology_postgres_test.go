package migrations_test

import (
	"errors"
	"strings"
	"testing"

	"automation-hub-backend/internal/infra"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestLifeOntologyMigrationRollbackSafety(t *testing.T) {
	db := openIsolatedMigrationDatabase(t)
	const version = "pre/0019_life_ontology"
	files := migrationFilesThrough(t, version)
	if _, err := infra.ApplyMigrations(db, files, "pre"); err != nil {
		t.Fatalf("apply pre migrations: %v", err)
	}

	for _, table := range []string{"life_ontology_merge_proposals", "life_ontology_relations", "life_ontology_entities"} {
		var count int64
		if err := db.Raw("SELECT count(*) FROM public." + table).Scan(&count).Error; err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Skipf("dedicated destructive test database required; %s contains %d records", table, count)
		}
	}

	if err := infra.RollbackMigration(db, files, "pre", version); err != nil {
		t.Fatalf("roll back empty life ontology migration: %v", err)
	}
	if lifeOntologyRelationExists(t, db, "life_ontology_entities") {
		t.Fatal("life_ontology_entities still exists after empty rollback")
	}
	if count, err := infra.ApplyMigrations(db, files, "pre"); err != nil || count != 1 {
		t.Fatalf("reapply life ontology migration = (%d, %v), want (1, nil)", count, err)
	}

	rollbackFixture := errors.New("rollback life ontology fixture")
	err := db.Transaction(func(tx *gorm.DB) error {
		owner := "migration-life-ontology-" + uuid.NewString()
		digest := strings.Repeat("a", 64)
		entityID := "life-entity-" + digest
		payload := `{"id":"` + entityID + `","ownerIdentity":"` + owner +
			`","type":"asset","domain":"personal_administration","name":"fixture",` +
			`"status":"active","priority":1,"validFrom":"2026-07-31T10:00:00Z",` +
			`"observedAt":"2026-07-31T10:00:00Z","confidence":1,` +
			`"verificationStatus":"source_supported","provenance":[{"referenceId":"fixture",` +
			`"contentDigest":"` + digest + `","capturedAt":"2026-07-31T10:00:00Z",` +
			`"localOnly":false}],"provenanceDigest":"` + digest + `",` +
			`"sensitivity":"internal","localOnly":false,"entityDigest":"` + digest +
			`","createdAt":"2026-07-31T10:00:00Z"}`
		if err := tx.Exec(`
			INSERT INTO public.life_ontology_entities (
				owner_identity, entity_id, entity_type, life_domain,
				lifecycle_status, verification_status, sensitivity, local_only,
				priority, entity_digest, provenance_digest, valid_from,
				observed_at, created_at, payload
			) VALUES (?, ?, 'asset', 'personal_administration', 'active',
				'source_supported', 'internal', false, 1, ?, ?,
				'2026-07-31T10:00:00Z', '2026-07-31T10:00:00Z',
				'2026-07-31T10:00:00Z', CAST(? AS jsonb))`,
			owner, entityID, digest, digest, payload).Error; err != nil {
			return err
		}
		err := infra.RollbackMigration(tx, files, "pre", version)
		if err == nil || !strings.Contains(err.Error(), "refusing to roll back non-empty") {
			t.Fatalf("non-empty rollback error = %v", err)
		}
		return rollbackFixture
	})
	if !errors.Is(err, rollbackFixture) {
		t.Fatalf("fixture transaction error = %v", err)
	}
	if !lifeOntologyRelationExists(t, db, "life_ontology_entities") {
		t.Fatal("refused rollback removed life ontology tables")
	}
}

func lifeOntologyRelationExists(t *testing.T, db *gorm.DB, relation string) bool {
	t.Helper()
	var exists bool
	if err := db.Raw(`
		SELECT EXISTS (
			SELECT 1 FROM pg_class
			WHERE oid = to_regclass('public.' || ?)
		)`, relation).Row().Scan(&exists); err != nil {
		t.Fatalf("check relation %s: %v", relation, err)
	}
	return exists
}
