package migrations_test

import (
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/infra"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestFrameworkEvidencePreflightPostgresImmutability(t *testing.T) {
	db := openFrameworkEvidenceMigrationDB(t)
	owner := "migration-framework-evidence-" + uuid.NewString()
	now := time.Now().UTC().Truncate(time.Microsecond)

	for _, test := range []struct {
		name      string
		operation func(*gorm.DB) error
	}{
		{
			name: "update",
			operation: func(tx *gorm.DB) error {
				return tx.Exec(`UPDATE public.framework_evidence_preflights SET status = 'passed' WHERE owner_identity = ?`, owner).Error
			},
		},
		{
			name: "delete",
			operation: func(tx *gorm.DB) error {
				return tx.Exec(`DELETE FROM public.framework_evidence_preflights WHERE owner_identity = ?`, owner).Error
			},
		},
		{
			name: "truncate",
			operation: func(tx *gorm.DB) error {
				return tx.Exec(`TRUNCATE TABLE public.framework_evidence_preflights`).Error
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := db.Transaction(func(tx *gorm.DB) error {
				insertFrameworkEvidenceMigrationFixture(t, tx, owner, now)
				return test.operation(tx)
			})
			if err == nil || !strings.Contains(err.Error(), "append-only") {
				t.Fatalf("%s error = %v, want append-only rejection", test.name, err)
			}
		})
	}
}

func TestFrameworkEvidencePreflightPostgresConstraints(t *testing.T) {
	db := openFrameworkEvidenceMigrationDB(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	tests := []struct {
		name    string
		digest  string
		status  string
		payload []byte
	}{
		{name: "uppercase digest", digest: strings.Repeat("A", 64), status: "passed", payload: []byte(`{"assertions":[]}`)},
		{name: "non-passing status", digest: strings.Repeat("a", 64), status: "blocked", payload: []byte(`{"assertions":[]}`)},
		{name: "invalid assertions JSON", digest: strings.Repeat("a", 64), status: "passed", payload: []byte(`{"assertions"`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := db.Transaction(func(tx *gorm.DB) error {
				return tx.Exec(`
					INSERT INTO public.framework_evidence_preflights (
						contract_version, owner_identity, task_plan_id,
						framework_selection_id, preflight_digest, status,
						assertions_json, evaluated_at, created_at
					) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					1,
					"constraint-owner-"+uuid.NewString(),
					"plan-1",
					"selection-1",
					test.digest,
					test.status,
					test.payload,
					now,
					now,
				).Error
			})
			if err == nil {
				t.Fatalf("Postgres accepted %s", test.name)
			}
		})
	}
}

func openFrameworkEvidenceMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := openIsolatedMigrationDatabase(t)
	files := migrationFilesThrough(t, "pre/0030_framework_evidence_preflights")
	if _, err := infra.ApplyMigrations(db, files, "pre"); err != nil {
		t.Fatalf("apply pre migrations: %v", err)
	}
	return db
}

func insertFrameworkEvidenceMigrationFixture(t *testing.T, tx *gorm.DB, owner string, now time.Time) {
	t.Helper()
	if err := tx.Exec(`
		INSERT INTO public.framework_evidence_preflights (
			contract_version, owner_identity, task_plan_id,
			framework_selection_id, preflight_digest, status,
			assertions_json, evaluated_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		1,
		owner,
		"plan-"+uuid.NewString(),
		"selection-"+uuid.NewString(),
		strings.Repeat("a", 64),
		"passed",
		[]byte(`{"assertions":[]}`),
		now,
		now,
	).Error; err != nil {
		t.Fatalf("insert framework evidence fixture: %v", err)
	}
}
