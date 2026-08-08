package frameworkevidence

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"automation-hub-backend/internal/infra"
	"automation-hub-backend/migrations"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestGormRepositoryPostgresContract(t *testing.T) {
	db := openFrameworkEvidencePostgres(t)
	record := validRecord()
	record.OwnerIdentity = "framework-evidence-owner"
	record.TaskPlanID = "framework-evidence-plan"
	record.FrameworkSelectionID = "framework-evidence-selection"
	record.PreflightDigest, _ = PreflightDigest(
		record.OwnerIdentity,
		record.TaskPlanID,
		record.FrameworkSelectionID,
		record.EvaluatedAt,
		record.AssertionsJSON,
	)

	rollback := errors.New("rollback framework evidence fixture")
	err := db.Transaction(func(tx *gorm.DB) error {
		repository := NewGormRepository(tx)
		if err := repository.Store(t.Context(), record); err != nil {
			t.Fatalf("store record: %v", err)
		}
		if err := repository.Store(t.Context(), record); err != nil {
			t.Fatalf("exact replay: %v", err)
		}
		resolved, err := repository.Resolve(
			t.Context(),
			record.OwnerIdentity,
			record.TaskPlanID,
			record.FrameworkSelectionID,
			record.PreflightDigest,
		)
		if err != nil {
			t.Fatalf("resolve record: %v", err)
		}
		if !bytes.Equal(resolved.AssertionsJSON, record.AssertionsJSON) {
			t.Fatalf("Postgres changed assertion bytes: got %q want %q", resolved.AssertionsJSON, record.AssertionsJSON)
		}
		resolved.AssertionsJSON[0] = '['
		reloaded, err := repository.Resolve(
			t.Context(),
			record.OwnerIdentity,
			record.TaskPlanID,
			record.FrameworkSelectionID,
			record.PreflightDigest,
		)
		if err != nil || !bytes.Equal(reloaded.AssertionsJSON, record.AssertionsJSON) {
			t.Fatalf("resolved payload alias changed storage: record=%q err=%v", reloaded.AssertionsJSON, err)
		}
		conflict := record
		conflict.AssertionsJSON = []byte(`{"assertions":[]}`)
		if err := repository.Store(t.Context(), conflict); !errors.Is(err, ErrInvalidRecord) {
			t.Fatalf("altered assertion replay error = %v, want ErrInvalidRecord", err)
		}
		if _, err := repository.Resolve(
			t.Context(),
			"another-owner",
			record.TaskPlanID,
			record.FrameworkSelectionID,
			record.PreflightDigest,
		); !errors.Is(err, ErrNotFound) {
			t.Fatalf("cross-owner resolve error = %v, want ErrNotFound", err)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("fixture transaction error = %v", err)
	}
}

func openFrameworkEvidencePostgres(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("HAI_TEST_DATABASE_DSN"))
	if dsn == "" {
		t.Skip("HAI_TEST_DATABASE_DSN not set; skipping framework evidence Postgres test")
	}
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("HAI_ALLOW_DESTRUCTIVE_DATABASE_TESTS")), "true") {
		t.Skip("HAI_ALLOW_DESTRUCTIVE_DATABASE_TESTS=true is required")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open Postgres: %v", err)
	}
	if _, err := infra.ApplyMigrations(db, migrations.Files, "pre"); err != nil {
		t.Fatalf("apply pre migrations: %v", err)
	}
	return db
}
