package migrations_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/infra"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestApprovalProofConsumptionMigrationRollbackSafety(t *testing.T) {
	db := openIsolatedMigrationDatabase(t)
	const version = "pre/0028_automation_approval_proof_consumptions"
	files := migrationFilesThrough(t, version)
	if _, err := infra.ApplyMigrations(db, files, "pre"); err != nil {
		t.Fatalf("apply pre migrations: %v", err)
	}

	if err := infra.RollbackMigration(db, files, "pre", version); err != nil {
		t.Fatalf("roll back empty approval proof migration: %v", err)
	}
	if approvalProofConsumptionRelationExists(t, db) {
		t.Fatal("approval proof consumption ledger still exists after empty rollback")
	}
	if count, err := infra.ApplyMigrations(db, files, "pre"); err != nil || count != 1 {
		t.Fatalf("reapply approval proof migration = (%d, %v), want (1, nil)", count, err)
	}

	rollbackFixture := errors.New("rollback approval proof fixture")
	err := db.Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC().Truncate(time.Microsecond)
		if err := tx.Exec(`
			INSERT INTO public.automation_approval_proof_consumptions (
				contract_version, owner_identity, proof_id, automation_id,
				action_digest, scope, approval_source_id, nonce_digest,
				signature_digest, record_digest, issued_at, expires_at,
				consumed_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			"automation-approval-proof-consumption.v1",
			"migration-owner-"+uuid.NewString(),
			uuid.New(),
			uuid.New(),
			strings.Repeat("a", 64),
			"automation.script.execute",
			"task-review:"+uuid.NewString(),
			strings.Repeat("b", 64),
			strings.Repeat("c", 64),
			strings.Repeat("d", 64),
			now.Add(-time.Second),
			now.Add(time.Minute),
			now,
		).Error; err != nil {
			return err
		}
		err := infra.RollbackMigration(tx, files, "pre", version)
		if err == nil || !strings.Contains(err.Error(), "refusing to remove non-empty immutable approval proof consumption ledger") {
			t.Fatalf("non-empty rollback error = %v", err)
		}
		if !approvalProofConsumptionRelationExists(t, tx) {
			t.Fatal("refused rollback removed approval proof consumption ledger")
		}
		return rollbackFixture
	})
	if !errors.Is(err, rollbackFixture) {
		t.Fatalf("fixture transaction error = %v", err)
	}
}

func approvalProofConsumptionRelationExists(t *testing.T, db *gorm.DB) bool {
	t.Helper()
	var exists bool
	if err := db.Raw(`SELECT to_regclass('public.automation_approval_proof_consumptions') IS NOT NULL`).Row().Scan(&exists); err != nil {
		t.Fatalf("check approval proof consumption relation: %v", err)
	}
	return exists
}
