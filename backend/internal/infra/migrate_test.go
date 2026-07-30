package infra

import (
	"strings"
	"testing"

	"automation-hub-backend/migrations"
)

func TestSplitSQLStatements(t *testing.T) {
	script := `-- a comment
DROP INDEX IF EXISTS idx_old;
CREATE UNIQUE INDEX IF NOT EXISTS idx_new ON t (a, b);

-- trailing comment only
`
	got := splitSQLStatements(script)
	if len(got) != 2 {
		t.Fatalf("expected 2 statements, got %d: %#v", len(got), got)
	}
	if got[0] != "DROP INDEX IF EXISTS idx_old" {
		t.Fatalf("statement[0] = %q", got[0])
	}
	if got[1] != "CREATE UNIQUE INDEX IF NOT EXISTS idx_new ON t (a, b)" {
		t.Fatalf("statement[1] = %q", got[1])
	}
}

func TestSplitSQLStatementsKeepsDollarQuotedBlocksIntact(t *testing.T) {
	// A guarded constraint uses DO $$ ... $$ and contains semicolons that must
	// NOT split the statement.
	script := `CREATE TABLE IF NOT EXISTS t (id int);
DO $$ BEGIN
  ALTER TABLE ONLY t ADD CONSTRAINT t_pkey PRIMARY KEY (id);
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;
CREATE INDEX IF NOT EXISTS i ON t (id);`
	got := splitSQLStatements(script)
	if len(got) != 3 {
		t.Fatalf("expected 3 statements, got %d: %#v", len(got), got)
	}
	if !strings.HasPrefix(got[1], "DO $$") || !strings.HasSuffix(got[1], "END $$") {
		t.Fatalf("dollar-quoted block was split: %q", got[1])
	}
	if !strings.Contains(got[1], "EXCEPTION WHEN duplicate_object") {
		t.Fatalf("block body lost its exception handler: %q", got[1])
	}
}

func TestPendingMigrationsSkipsAppliedAndKeepsOrder(t *testing.T) {
	all := []Migration{
		{Version: "post/0001_a"},
		{Version: "post/0002_b"},
		{Version: "post/0003_c"},
	}
	pending := pendingMigrations(all, map[string]bool{"post/0001_a": true})
	if len(pending) != 2 || pending[0].Version != "post/0002_b" || pending[1].Version != "post/0003_c" {
		t.Fatalf("pending = %#v, want 0002 then 0003", pending)
	}
}

func TestLoadMigrationsFromEmbeddedFiles(t *testing.T) {
	for _, tc := range []struct {
		dir         string
		wantVersion string
		wantDown    bool
	}{
		{"pre", "pre/0001_extensions", true},
		{"post", "post/0001_conversation_owner_identity", true},
	} {
		loaded, err := loadMigrations(migrations.Files, tc.dir)
		if err != nil {
			t.Fatalf("loadMigrations(%q): %v", tc.dir, err)
		}
		if len(loaded) == 0 {
			t.Fatalf("loadMigrations(%q) returned no migrations", tc.dir)
		}
		if loaded[0].Version != tc.wantVersion {
			t.Fatalf("first %q version = %q, want %q", tc.dir, loaded[0].Version, tc.wantVersion)
		}
		if loaded[0].UpSQL == "" {
			t.Fatalf("%q up SQL is empty", tc.wantVersion)
		}
		if tc.wantDown && loaded[0].DownSQL == "" {
			t.Fatalf("%q down SQL is empty; every up must have a down", tc.wantVersion)
		}
	}
}

func TestLegacyBaselineGuardsExistingPrimaryKeys(t *testing.T) {
	loaded, err := loadMigrations(migrations.Files, "pre")
	if err != nil {
		t.Fatalf("load pre migrations: %v", err)
	}

	var baseline string
	for _, migration := range loaded {
		if migration.Version == "pre/0002_baseline" {
			baseline = migration.UpSQL
			break
		}
	}
	if baseline == "" {
		t.Fatal("pre/0002_baseline migration not found")
	}
	if strings.Contains(baseline, ";;") {
		t.Fatal("baseline contains a duplicated statement terminator")
	}

	primaryKeyBlocks := 0
	for _, statement := range splitSQLStatements(baseline) {
		if !strings.Contains(statement, " PRIMARY KEY ") {
			continue
		}
		primaryKeyBlocks++
		if strings.Contains(statement, "WHEN invalid_table_definition THEN NULL") ||
			!strings.Contains(statement, "pg_get_constraintdef") ||
			!strings.Contains(statement, "RAISE;") {
			t.Fatalf("primary-key replay block lacks exact PostgreSQL catalog validation:\n%s", statement)
		}
	}
	if primaryKeyBlocks == 0 {
		t.Fatal("baseline contains no primary-key blocks to validate")
	}
}
