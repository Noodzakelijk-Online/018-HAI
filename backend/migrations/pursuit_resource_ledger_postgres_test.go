//go:build integration

package migrations_test

import (
	"strings"
	"testing"

	"automation-hub-backend/internal/infra"

	"github.com/google/uuid"
)

func TestPursuitResourceLedgerConstraintsAndImmutabilityInPostgres(t *testing.T) {
	db := openIsolatedMigrationDatabase(t)
	files := migrationFilesThrough(t, "pre/0034_pursuit_resource_ledger")
	if _, err := infra.ApplyMigrations(db, files, "pre"); err != nil {
		t.Fatalf("apply pursuit resource ledger migration: %v", err)
	}
	pursuitID := uuid.New()
	if err := db.Exec(`
		INSERT INTO public.pursuits (
			id, owner_identity, title, status, risk_level, autonomy_level,
			completion_state, archived, created_at, updated_at
		) VALUES (?, 'alice', 'Metered pursuit', 'active', 'low', 'manual', 'open', false, now(), now())`, pursuitID).Error; err != nil {
		t.Fatalf("insert pursuit: %v", err)
	}

	effortID := uuid.New()
	if err := db.Exec(`
		INSERT INTO pursuit_resource_events (
			id, pursuit_id, owner_identity, kind, effort_minutes, actor,
			idempotency_key, record_digest, occurred_at
		) VALUES (?, ?, 'alice', 'effort_recorded', 90, 'alice', 'effort-1', ?, now())`,
		effortID, pursuitID, strings.Repeat("a", 64)).Error; err != nil {
		t.Fatalf("insert effort event: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO pursuit_resource_events (
			pursuit_id, owner_identity, kind, amount_minor, currency, evidence_uri, actor,
			idempotency_key, record_digest, occurred_at
		) VALUES (?, 'alice', 'spend_incurred', 2500, 'EUR', 'receipt://one', 'alice', 'spend-1', ?, now())`,
		pursuitID, strings.Repeat("b", 64)).Error; err != nil {
		t.Fatalf("insert spend event: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO pursuit_resource_events (
			pursuit_id, owner_identity, kind, amount_minor, currency, evidence_uri, actor,
			idempotency_key, record_digest, occurred_at
		) VALUES (?, 'alice', 'spend_refund', 1000, 'EUR', 'refund://one', 'alice', 'refund-1', ?, now())`,
		pursuitID, strings.Repeat("c", 64)).Error; err != nil {
		t.Fatalf("insert refund event: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO pursuit_resource_events (
			pursuit_id, owner_identity, kind, amount_minor, currency, evidence_uri, actor,
			idempotency_key, record_digest, occurred_at
		) VALUES (?, 'alice', 'spend_refund', 1600, 'EUR', 'refund://too-much', 'alice', 'refund-2', ?, now())`,
		pursuitID, strings.Repeat("d", 64)).Error; err == nil {
		t.Fatal("database accepted a refund larger than recorded net spend")
	}
	if err := db.Exec(`UPDATE pursuit_resource_events SET note = 'changed' WHERE id = ?`, effortID).Error; err == nil {
		t.Fatal("database accepted an update to an append-only resource event")
	}
	if err := db.Exec(`DELETE FROM pursuit_resource_events WHERE id = ?`, effortID).Error; err == nil {
		t.Fatal("database accepted deletion of an append-only resource event")
	}
	if err := db.Exec(`TRUNCATE pursuit_resource_events`).Error; err == nil {
		t.Fatal("database accepted truncation of an append-only resource ledger")
	}
	if err := infra.RollbackMigration(db, files, "pre", "pre/0034_pursuit_resource_ledger"); err == nil || !strings.Contains(err.Error(), "refusing to remove non-empty pursuit resource ledger") {
		t.Fatalf("non-empty rollback error = %v, want ledger refusal", err)
	}
}
