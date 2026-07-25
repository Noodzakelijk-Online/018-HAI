package infra

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// Versioned SQL migration runner.
//
// It applies reviewable, ordered .sql files and records each applied version in
// schema_migrations, so the schema is reproducible and auditable rather than
// derived implicitly from Gorm AutoMigrate. Migrations are split into two
// phases (see the migrations package): "pre" runs before AutoMigrate, "post"
// runs after it, which lets us retire AutoMigrate incrementally without
// reordering table-dependent DDL.

const schemaMigrationsTable = "schema_migrations"

// Migration is one versioned change loaded from an embedded directory.
type Migration struct {
	Version string // e.g. "post/0001_conversation_owner_identity"
	UpSQL   string
	DownSQL string
}

// loadMigrations reads every NNNN_name.up.sql (and optional matching .down.sql)
// from dir in fsys and returns them sorted by version.
func loadMigrations(fsys fs.FS, dir string) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir %q: %w", dir, err)
	}
	migrations := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		base := strings.TrimSuffix(name, ".up.sql")
		upSQL, err := fs.ReadFile(fsys, dir+"/"+name)
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", name, err)
		}
		migration := Migration{Version: dir + "/" + base, UpSQL: string(upSQL)}
		if downSQL, err := fs.ReadFile(fsys, dir+"/"+base+".down.sql"); err == nil {
			migration.DownSQL = string(downSQL)
		}
		migrations = append(migrations, migration)
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	return migrations, nil
}

// pendingMigrations returns, in order, the migrations whose version is not in
// applied. Pure and DB-free so ordering/idempotency can be unit-tested.
func pendingMigrations(all []Migration, applied map[string]bool) []Migration {
	pending := make([]Migration, 0, len(all))
	for _, migration := range all {
		if !applied[migration.Version] {
			pending = append(pending, migration)
		}
	}
	return pending
}

// dollarTagAt reports the dollar-quote delimiter starting at index i (e.g. `$$`
// or `$tag$`) and its width, or width 0 if there is none. Dollar quoting matters
// because a DO $$ ... $$ block contains semicolons that must not split it.
func dollarTagAt(script string, i int) (string, int) {
	if i >= len(script) || script[i] != '$' {
		return "", 0
	}
	for j := i + 1; j < len(script); j++ {
		c := script[j]
		if c == '$' {
			return script[i : j+1], j + 1 - i
		}
		isTagChar := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
		if !isTagChar {
			return "", 0
		}
	}
	return "", 0
}

// trimStatement drops leading blank/comment-only lines and surrounding space.
// Comments inside a statement body are preserved so dollar-quoted blocks stay
// byte-for-byte intact.
func trimStatement(raw string) string {
	lines := strings.Split(raw, "\n")
	start := 0
	for start < len(lines) {
		trimmed := strings.TrimSpace(lines[start])
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			start++
			continue
		}
		break
	}
	return strings.TrimSpace(strings.Join(lines[start:], "\n"))
}

// splitSQLStatements breaks a migration file into individual statements so the
// runner does not depend on the driver's multi-statement behavior. It splits on
// `;` at the top level only, honouring dollar-quoted blocks, and skips
// comment-only chunks.
func splitSQLStatements(script string) []string {
	statements := []string{}
	var current strings.Builder
	flush := func() {
		if statement := trimStatement(current.String()); statement != "" {
			statements = append(statements, statement)
		}
		current.Reset()
	}
	openTag := ""
	for i := 0; i < len(script); {
		if openTag == "" {
			if tag, width := dollarTagAt(script, i); width > 0 {
				openTag = tag
				current.WriteString(script[i : i+width])
				i += width
				continue
			}
			if script[i] == ';' {
				flush()
				i++
				continue
			}
		} else if tag, width := dollarTagAt(script, i); width > 0 && tag == openTag {
			openTag = ""
			current.WriteString(script[i : i+width])
			i += width
			continue
		}
		current.WriteByte(script[i])
		i++
	}
	flush()
	return statements
}

func ensureSchemaMigrationsTable(db *gorm.DB) error {
	return db.Exec(fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s (version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`,
		schemaMigrationsTable)).Error
}

func appliedVersions(db *gorm.DB) (map[string]bool, error) {
	var versions []string
	if err := db.Raw(fmt.Sprintf("SELECT version FROM %s", schemaMigrationsTable)).Scan(&versions).Error; err != nil {
		return nil, err
	}
	applied := make(map[string]bool, len(versions))
	for _, version := range versions {
		applied[version] = true
	}
	return applied, nil
}

// ApplyMigrations applies all pending up migrations in dir and returns the count
// applied. Each migration runs inside its own transaction and is recorded on
// success, so a failure leaves earlier migrations committed and this one absent.
func ApplyMigrations(db *gorm.DB, fsys fs.FS, dir string) (int, error) {
	if err := ensureSchemaMigrationsTable(db); err != nil {
		return 0, fmt.Errorf("ensure schema_migrations: %w", err)
	}
	all, err := loadMigrations(fsys, dir)
	if err != nil {
		return 0, err
	}
	applied, err := appliedVersions(db)
	if err != nil {
		return 0, fmt.Errorf("load applied versions: %w", err)
	}
	count := 0
	for _, migration := range pendingMigrations(all, applied) {
		if err := db.Transaction(func(tx *gorm.DB) error {
			for _, statement := range splitSQLStatements(migration.UpSQL) {
				if err := tx.Exec(statement).Error; err != nil {
					return fmt.Errorf("apply %s: %w", migration.Version, err)
				}
			}
			return tx.Exec(fmt.Sprintf("INSERT INTO %s (version) VALUES (?)", schemaMigrationsTable), migration.Version).Error
		}); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// RollbackMigration reverses a single applied migration by running its down SQL
// and removing its schema_migrations row, in one transaction.
func RollbackMigration(db *gorm.DB, fsys fs.FS, dir, version string) error {
	all, err := loadMigrations(fsys, dir)
	if err != nil {
		return err
	}
	var target *Migration
	for i := range all {
		if all[i].Version == version {
			target = &all[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("migration %q not found in %q", version, dir)
	}
	if strings.TrimSpace(target.DownSQL) == "" {
		return fmt.Errorf("migration %q has no down file; refusing to rollback", version)
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, statement := range splitSQLStatements(target.DownSQL) {
			if err := tx.Exec(statement).Error; err != nil {
				return fmt.Errorf("rollback %s: %w", version, err)
			}
		}
		return tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE version = ?", schemaMigrationsTable), version).Error
	})
}

// MigrationStatus reports, per phase, which versions are applied and which are
// pending. Used by the `migrate status` subcommand.
type MigrationStatus struct {
	Dir     string
	Applied []string
	Pending []string
}

// Status returns the applied/pending split for a directory without changing it.
func Status(db *gorm.DB, fsys fs.FS, dir string) (MigrationStatus, error) {
	status := MigrationStatus{Dir: dir}
	if err := ensureSchemaMigrationsTable(db); err != nil {
		return status, err
	}
	all, err := loadMigrations(fsys, dir)
	if err != nil {
		return status, err
	}
	applied, err := appliedVersions(db)
	if err != nil {
		return status, err
	}
	for _, migration := range all {
		if applied[migration.Version] {
			status.Applied = append(status.Applied, migration.Version)
		} else {
			status.Pending = append(status.Pending, migration.Version)
		}
	}
	return status, nil
}
