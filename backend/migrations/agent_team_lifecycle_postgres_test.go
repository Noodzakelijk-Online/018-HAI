package migrations_test

import (
	"errors"
	"strings"
	"testing"

	"automation-hub-backend/internal/infra"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestAgentTeamLifecycleMigrationRollbackSafety(t *testing.T) {
	db := openIsolatedMigrationDatabase(t)
	const version = "pre/0018_agent_team_lifecycle"
	files := migrationFilesThrough(t, version)
	if _, err := infra.ApplyMigrations(db, files, "pre"); err != nil {
		t.Fatalf("apply pre migrations: %v", err)
	}

	if err := infra.RollbackMigration(db, files, "pre", version); err != nil {
		t.Fatalf("roll back empty agent-team migration: %v", err)
	}
	if agentTeamRelationExists(t, db, "agent_teams") {
		t.Fatal("agent_teams still exists after empty rollback")
	}
	if count, err := infra.ApplyMigrations(db, files, "pre"); err != nil || count != 1 {
		t.Fatalf("reapply agent-team migration = (%d, %v), want (1, nil)", count, err)
	}

	rollbackFixture := errors.New("rollback agent-team fixture")
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`
			INSERT INTO public.agent_teams (
				owner_identity, team_id, team_key, created_at
			) VALUES (?, ?, ?, now())`,
			"migration-test-"+uuid.NewString(), uuid.New(), "rollback_guard",
		).Error; err != nil {
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
	if !agentTeamRelationExists(t, db, "agent_teams") {
		t.Fatal("refused rollback removed agent-team tables")
	}
}

func agentTeamRelationExists(t *testing.T, db *gorm.DB, relation string) bool {
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
