//go:build integration

package phase2

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/operations"
	"automation-hub-backend/migrations"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestGormEvidencePackRepositoryPersistsAndEnforcesOwnerScope(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("HAI_TEST_DATABASE_DSN"))
	if dsn == "" {
		t.Skip("HAI_TEST_DATABASE_DSN not set; skipping Postgres integration test")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open Postgres: %v", err)
	}
	if _, err := infra.ApplyMigrations(db, migrations.Files, "pre"); err != nil {
		t.Fatalf("apply pre migrations: %v", err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })

	suffix := uuid.NewString()
	owner := "evidence-owner-" + suffix
	workspace := "evidence-workspace-" + suffix
	operationService := operations.NewService(operations.NewGormRepository(tx))
	ingested, err := operationService.Ingest(operations.NewOperationInput{
		OwnerUserID:        owner,
		WorkspaceID:        workspace,
		Title:              "Durable evidence",
		Description:        "Source-supported operation",
		OperationType:      "evidence_test",
		SourceType:         "integration_test",
		SourceURI:          "test://source/" + suffix,
		SourceRevisionHash: "sha256:" + strings.Repeat("a", 64),
		DedupeKey:          "evidence-test-" + suffix,
	})
	if err != nil {
		t.Fatalf("ingest operation: %v", err)
	}

	now := time.Date(2026, time.July, 31, 9, 0, 0, 0, time.UTC)
	repository := NewGormEvidencePackRepository(tx)
	created, err := repository.Create(t.Context(), EvidencePack{
		OwnerIdentity: owner,
		WorkspaceID:   workspace,
		OperationID:   ingested.Operation.ID,
		Title:         "Durable evidence",
		Markdown:      "# Durable evidence\n\nVerified.",
		Provenance: EvidenceProvenance{
			SourceType:         ingested.Operation.SourceType,
			SourceURI:          ingested.Operation.SourceURI,
			SourceRevisionHash: ingested.Operation.SourceRevisionHash,
			DedupeKey:          ingested.Operation.DedupeKey,
		},
		GeneratedAt: now,
	})
	if err != nil {
		t.Fatalf("create evidence pack: %v", err)
	}
	if created.ID == uuid.Nil || created.OwnerIdentity != owner ||
		created.WorkspaceID != workspace {
		t.Fatalf("created pack = %#v", created)
	}

	restarted := NewGormEvidencePackRepository(tx)
	got, err := restarted.Get(t.Context(), owner, workspace, created.ID)
	if err != nil {
		t.Fatalf("get after repository restart: %v", err)
	}
	if got.ContentDigest != created.ContentDigest ||
		got.Provenance.SourceRevisionHash != created.Provenance.SourceRevisionHash ||
		!got.GeneratedAt.Equal(now) {
		t.Fatalf("persisted pack lost fields: got %#v want %#v", got, created)
	}
	if _, err := restarted.Get(t.Context(), "other-owner", workspace, created.ID); !errors.Is(err, ErrEvidencePackNotFound) {
		t.Fatalf("cross-owner get error = %v, want not found", err)
	}
	if _, err := restarted.Get(t.Context(), owner, "other-workspace", created.ID); !errors.Is(err, ErrEvidencePackNotFound) {
		t.Fatalf("cross-workspace get error = %v, want not found", err)
	}

	if err := tx.Model(&evidencePackRow{}).
		Where("id = ?", created.ID).
		Update("title", "mutated").Error; err == nil {
		t.Fatal("immutable evidence pack accepted an update")
	}
}
