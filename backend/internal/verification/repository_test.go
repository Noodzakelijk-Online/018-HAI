package verification

import (
	"database/sql"
	"strings"
	"testing"

	"automation-hub-backend/internal/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestVerificationRunsForOwnerQueryExcludesOwnerlessLegacyRows(t *testing.T) {
	t.Setenv("HAI_LEGACY_DATA_OWNER_IDENTITY", "legacy-owner")
	sqlDB, err := sql.Open("pgx", "postgres://unused")
	if err != nil {
		t.Fatalf("open dry-run connection: %v", err)
	}
	defer sqlDB.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		DryRun:               true,
		DisableAutomaticPing: true,
	})
	if err != nil {
		t.Fatalf("open dry-run gorm: %v", err)
	}

	var runs []models.VerificationRun
	statement := verificationRunsForOwnerQuery(db, "alice").Find(&runs).Statement
	query := strings.ToLower(statement.SQL.String())
	if !strings.Contains(query, "owner_identity = $1") {
		t.Fatalf("authenticated query lacks exact owner scope: %s", query)
	}
	if strings.Contains(query, "owner_identity is null") || strings.Contains(query, "owner_identity = ''") || strings.Contains(query, " or ") {
		t.Fatalf("authenticated query exposes ownerless legacy rows: %s", query)
	}
	if len(statement.Vars) != 1 || statement.Vars[0] != "alice" {
		t.Fatalf("authenticated query vars = %#v, want exact owner", statement.Vars)
	}

	statement = verificationRunsForOwnerQuery(db, "legacy-owner").Find(&runs).Statement
	query = strings.ToLower(statement.SQL.String())
	if !strings.Contains(query, "owner_identity is null") || !strings.Contains(query, " or ") {
		t.Fatalf("configured migration owner cannot inspect ownerless legacy rows: %s", query)
	}

	statement = verificationRunsForOwnerQuery(db, "").Find(&runs).Statement
	if strings.Contains(strings.ToLower(statement.SQL.String()), "owner_identity") {
		t.Fatalf("explicit internal/global query was unexpectedly owner-filtered: %s", statement.SQL.String())
	}
}
