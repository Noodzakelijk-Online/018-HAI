//go:build integration

package migrations_test

import (
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/infra"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestWorkflowStandingMandateBindingIsOwnerScopedInPostgres(t *testing.T) {
	db := openIsolatedMigrationDatabase(t)
	files := migrationFilesThrough(t, "pre/0032_pursuit_workflow_standing_mandate_binding")
	if _, err := infra.ApplyMigrations(db, files, "pre"); err != nil {
		t.Fatalf("apply migrations through workflow mandate binding: %v", err)
	}

	aliceMandate := uuid.New()
	createdAt := time.Now().UTC().Add(-time.Minute)
	if err := db.Exec(`
		INSERT INTO public.standing_mandates (
			id, owner_identity, name, purpose, status, version, revision,
			scopes_json, autonomy_ceiling, approval_policy_json,
			stop_conditions_json, source_references_json, created_by,
			created_at, updated_at, activated_at
		) VALUES (?, 'alice', 'Safe status work', 'Bound test mandate', 'active', '1.0.0', 1,
			'["project.status"]'::jsonb, 2, '{}'::jsonb,
			'[]'::jsonb, '[]'::jsonb, 'alice', ?, ?, ?)`,
		aliceMandate, createdAt, createdAt, createdAt,
	).Error; err != nil {
		t.Fatalf("insert Alice mandate: %v", err)
	}

	if err := insertWorkflowWithMandate(db, "alice", aliceMandate); err != nil {
		t.Fatalf("same-owner workflow mandate binding rejected: %v", err)
	}
	if err := insertWorkflowWithMandate(db, "bob", aliceMandate); err == nil {
		t.Fatal("database accepted a workflow bound to another owner's mandate")
	}
	if err := insertWorkflowWithMandate(db, "alice", uuid.New()); err == nil {
		t.Fatal("database accepted a workflow bound to an unknown mandate")
	}
	if err := insertPursuitWithMandate(db, "alice", aliceMandate); err != nil {
		t.Fatalf("same-owner pursuit mandate binding rejected: %v", err)
	}
	if err := insertPursuitWithMandate(db, "bob", aliceMandate); err == nil {
		t.Fatal("database accepted a pursuit bound to another owner's mandate")
	}
	if err := db.Exec(`
		INSERT INTO public.workflow_items (
			id, owner_identity, title, current_state, task_type, risk_level,
			autonomy_level, requires_approval, approval_status, archived,
			created_at, updated_at
		) VALUES (?, 'alice', 'Unmandated planning', 'ready', 'administrative', 'low',
			'manual', false, 'not_required', false, now(), now())`,
		uuid.New(),
	).Error; err != nil {
		t.Fatalf("optional mandate column blocked unmandated workflow: %v", err)
	}

	if err := infra.RollbackMigration(
		db,
		files,
		"pre",
		"pre/0032_pursuit_workflow_standing_mandate_binding",
	); err == nil || !strings.Contains(err.Error(), "refusing to remove non-empty workflow standing mandate bindings") {
		t.Fatalf("non-empty rollback error = %v, want durable-binding refusal", err)
	}
}

func insertWorkflowWithMandate(db *gorm.DB, owner string, mandateID uuid.UUID) error {
	return db.Exec(`
		INSERT INTO public.workflow_items (
			id, owner_identity, title, current_state, task_type, risk_level,
			autonomy_level, requires_approval, approval_status, archived,
			created_at, updated_at, mandate_id
		) VALUES (?, ?, 'Mandated work', 'ready', 'administrative', 'low',
			'autonomous_safe', false, 'not_required', false, now(), now(), ?)`,
		uuid.New(), owner, mandateID,
	).Error
}

func insertPursuitWithMandate(db *gorm.DB, owner string, mandateID uuid.UUID) error {
	return db.Exec(`
		INSERT INTO public.pursuits (
			id, owner_identity, title, status, risk_level, autonomy_level,
			completion_state, archived, created_at, updated_at, mandate_id
		) VALUES (?, ?, 'Mandated pursuit', 'active', 'low', 'autonomous_safe',
			'open', false, now(), now(), ?)`,
		uuid.New(), owner, mandateID,
	).Error
}
