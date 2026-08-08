package migrations_test

import (
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"automation-hub-backend/migrations"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func migrationFilesThrough(t *testing.T, version string) fs.FS {
	t.Helper()
	target := strings.TrimPrefix(strings.TrimSpace(version), "pre/")
	if target == "" || target == version {
		t.Fatalf("invalid pre-migration version %q", version)
	}
	files := fstest.MapFS{
		"pre": &fstest.MapFile{Mode: fs.ModeDir},
	}
	entries, err := fs.ReadDir(migrations.Files, "pre")
	if err != nil {
		t.Fatalf("read embedded pre migrations: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || (!strings.HasSuffix(name, ".up.sql") && !strings.HasSuffix(name, ".down.sql")) {
			continue
		}
		stem := strings.TrimSuffix(strings.TrimSuffix(name, ".up.sql"), ".down.sql")
		if stem > target {
			continue
		}
		data, err := fs.ReadFile(migrations.Files, "pre/"+name)
		if err != nil {
			t.Fatalf("read embedded migration %s: %v", name, err)
		}
		files["pre/"+name] = &fstest.MapFile{Data: data, Mode: 0o600}
	}
	return files
}

// openIsolatedMigrationDatabase creates a database for one destructive test.
// The configured database is used only as a connection template; its schema
// and data are never modified by migration lifecycle tests.
func openIsolatedMigrationDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("HAI_TEST_DATABASE_DSN"))
	if dsn == "" {
		t.Skip("HAI_TEST_DATABASE_DSN not set; skipping migration integration test")
	}
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("HAI_ALLOW_DESTRUCTIVE_DATABASE_TESTS")), "true") {
		t.Skip("HAI_ALLOW_DESTRUCTIVE_DATABASE_TESTS=true is required")
	}

	adminConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse Postgres test DSN: %v", err)
	}
	adminConfig.Database = "postgres"
	adminSQL := stdlib.OpenDB(*adminConfig)
	admin, err := gorm.Open(postgres.New(postgres.Config{Conn: adminSQL}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		_ = adminSQL.Close()
		t.Fatalf("open Postgres administration connection: %v", err)
	}

	databaseName := "hai_migration_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedDatabase := `"` + databaseName + `"`
	if err := admin.Exec("CREATE DATABASE " + quotedDatabase).Error; err != nil {
		_ = adminSQL.Close()
		t.Fatalf("create isolated migration database: %v", err)
	}

	testConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		_ = admin.Exec("DROP DATABASE " + quotedDatabase + " WITH (FORCE)").Error
		_ = adminSQL.Close()
		t.Fatalf("parse isolated Postgres test DSN: %v", err)
	}
	testConfig.Database = databaseName
	testSQL := stdlib.OpenDB(*testConfig)
	testDB, err := gorm.Open(postgres.New(postgres.Config{Conn: testSQL}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		_ = testSQL.Close()
		_ = admin.Exec("DROP DATABASE " + quotedDatabase + " WITH (FORCE)").Error
		_ = adminSQL.Close()
		t.Fatalf("open isolated migration database: %v", err)
	}

	t.Cleanup(func() {
		_ = testSQL.Close()
		if err := admin.Exec("DROP DATABASE " + quotedDatabase + " WITH (FORCE)").Error; err != nil {
			t.Errorf("drop isolated migration database %s: %v", databaseName, err)
		}
		if err := adminSQL.Close(); err != nil {
			t.Errorf("close Postgres administration connection: %v", err)
		}
	})
	return testDB
}
