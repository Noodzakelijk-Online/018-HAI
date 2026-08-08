package migrations_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/lifeontology"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestContactReviewDecisionMigrationRollbackSafety(t *testing.T) {
	db := openIsolatedMigrationDatabase(t)
	const version = "pre/0027_contact_review_decisions"
	files := migrationFilesThrough(t, version)
	if _, err := infra.ApplyMigrations(db, files, "pre"); err != nil {
		t.Fatalf("apply pre migrations: %v", err)
	}

	if err := infra.RollbackMigration(db, files, "pre", version); err != nil {
		t.Fatalf("roll back empty contact-review migration: %v", err)
	}
	if contactReviewRelationExists(t, db) {
		t.Fatal("contact-review ledger still exists after empty rollback")
	}
	if count, err := infra.ApplyMigrations(db, files, "pre"); err != nil || count != 1 {
		t.Fatalf("reapply contact-review migration = (%d, %v), want (1, nil)", count, err)
	}

	rollbackFixture := errors.New("rollback contact-review fixture")
	err := db.Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC().Truncate(time.Second)
		owner := "contact-review-migration-" + uuid.NewString()
		service := lifeontology.NewService(lifeontology.NewPostgresRepository(tx), func() time.Time { return now })
		candidate, err := service.RecordEntity(context.Background(), lifeontology.RecordEntityRequest{
			OwnerIdentity: owner,
			Type:          lifeontology.EntityPerson,
			Domain:        lifeontology.DomainRelationships,
			Name:          "Migration review candidate",
			Summary:       "Source-derived contact awaiting owner review",
			ExternalKeys: []lifeontology.ExternalKey{{
				Namespace: "source/contact-candidate",
				Value:     uuid.NewString(),
			}},
			Attributes:         map[string]string{"candidate": "true", "review_required": "true"},
			Status:             lifeontology.StatusOpen,
			Confidence:         0.35,
			VerificationStatus: lifeontology.VerificationNeedsReview,
			Provenance: []lifeontology.Provenance{{
				ReferenceID:   "migration-fixture",
				URI:           "https://example.test/contact-review",
				ContentDigest: strings.Repeat("a", 64),
				Authority:     "integration fixture",
				CapturedAt:    now.Add(-time.Minute),
				LocalOnly:     true,
			}},
			Sensitivity: lifeontology.SensitivitySensitive,
			LocalOnly:   true,
			ValidFrom:   now.Add(-time.Hour),
			ObservedAt:  now.Add(-time.Minute),
		})
		if err != nil {
			return err
		}
		if _, err := service.DecideContactCandidate(context.Background(), lifeontology.DecideContactCandidateRequest{
			OwnerIdentity:  owner,
			CandidateID:    candidate.Entity.ID,
			Action:         lifeontology.ContactReviewReject,
			Reason:         "Owner rejected the extracted contact candidate",
			IdempotencyKey: "migration-review-" + uuid.NewString(),
		}); err != nil {
			return err
		}
		err = infra.RollbackMigration(tx, files, "pre", version)
		if err == nil || !strings.Contains(err.Error(), "refusing to remove non-empty immutable contact review ledger") {
			t.Fatalf("non-empty rollback error = %v", err)
		}
		if !contactReviewRelationExists(t, tx) {
			t.Fatal("refused rollback removed contact-review ledger")
		}
		return rollbackFixture
	})
	if !errors.Is(err, rollbackFixture) {
		t.Fatalf("fixture transaction error = %v", err)
	}
}

func contactReviewRelationExists(t *testing.T, db *gorm.DB) bool {
	t.Helper()
	var exists bool
	if err := db.Raw(`SELECT to_regclass('public.life_ontology_contact_review_decisions') IS NOT NULL`).Row().Scan(&exists); err != nil {
		t.Fatalf("check contact-review relation: %v", err)
	}
	return exists
}
