//go:build integration

package migrations_test

import (
	"strings"
	"testing"

	"automation-hub-backend/internal/infra"

	"github.com/google/uuid"
)

func TestPursuitGoalContractBackfillAndConstraintsInPostgres(t *testing.T) {
	db := openIsolatedMigrationDatabase(t)
	filesBefore := migrationFilesThrough(t, "pre/0032_pursuit_workflow_standing_mandate_binding")
	if _, err := infra.ApplyMigrations(db, filesBefore, "pre"); err != nil {
		t.Fatalf("apply migrations before pursuit goal contract: %v", err)
	}
	pursuitID := uuid.New()
	if err := db.Exec(`
		INSERT INTO public.pursuits (
			id, owner_identity, title, completion_definition, status, risk_level,
			autonomy_level, completion_state, archived, created_at, updated_at
		) VALUES (?, 'alice', 'Backfilled pursuit', 'Verified result exists', 'active', 'low',
			'manual', 'open', false, now(), now())`, pursuitID).Error; err != nil {
		t.Fatalf("insert pre-contract pursuit: %v", err)
	}

	files := migrationFilesThrough(t, "pre/0033_pursuit_goal_contract")
	if _, err := infra.ApplyMigrations(db, files, "pre"); err != nil {
		t.Fatalf("apply pursuit goal contract migration: %v", err)
	}

	var criteriaCount, stopCount int
	if err := db.Raw(`
		SELECT jsonb_array_length(success_criteria), jsonb_array_length(stop_conditions)
		FROM public.pursuits WHERE id = ?`, pursuitID).Row().Scan(&criteriaCount, &stopCount); err != nil {
		t.Fatalf("read backfilled goal contract: %v", err)
	}
	if criteriaCount != 1 || stopCount != 1 {
		t.Fatalf("backfilled counts = criteria %d, stops %d", criteriaCount, stopCount)
	}

	if err := db.Exec(`UPDATE public.pursuits SET success_criteria = '{}'::jsonb WHERE id = ?`, pursuitID).Error; err == nil {
		t.Fatal("database accepted non-array success criteria")
	}
	if err := db.Exec(`UPDATE public.pursuits SET review_cadence_days = -1 WHERE id = ?`, pursuitID).Error; err == nil {
		t.Fatal("database accepted a negative review cadence")
	}
	if err := infra.RollbackMigration(db, files, "pre", "pre/0033_pursuit_goal_contract"); err == nil || !strings.Contains(err.Error(), "refusing to remove non-empty pursuit goal contracts") {
		t.Fatalf("non-empty rollback error = %v, want durable-contract refusal", err)
	}
}
