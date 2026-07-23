#!/usr/bin/env bash
# Regenerate backend/migrations/pre/0002_baseline.up.sql from a migrated database.
#
# Run this after adding or changing a Gorm model:
#   1. DB_AUTOMIGRATE=true backend migrate up      # let Gorm materialise the model
#   2. scripts/generate-migration-baseline.sh      # capture the new schema
#   3. review the diff, commit, and go back to DB_AUTOMIGRATE=false
#
# The output is idempotent: tables/indexes use IF NOT EXISTS and constraints are
# wrapped in exception-guarded DO blocks, so it is safe to apply to a database
# that already has the schema.
#
# Usage:
#   scripts/generate-migration-baseline.sh                 # uses docker container "hai-pg"
#   PG_CONTAINER=mypg PG_DB=automation_hub scripts/generate-migration-baseline.sh
set -euo pipefail

PG_CONTAINER="${PG_CONTAINER:-hai-pg}"
PG_USER="${PG_USER:-postgres}"
PG_DB="${PG_DB:-automation_hub}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$REPO_ROOT/backend/migrations/pre/0002_baseline.up.sql"

cat > "$OUT" <<'HEADER'
-- Baseline schema: every table the application expects, generated from a
-- migrated database (pg_dump --schema-only) so production no longer depends on
-- Gorm AutoMigrate. Set DB_AUTOMIGRATE=false and this file is the source of truth.
--
-- Idempotent on purpose: tables/indexes use IF NOT EXISTS and constraints are
-- wrapped in exception-guarded DO blocks, so applying it to a database that was
-- already built by AutoMigrate is a safe no-op.
--
-- Regenerate with: scripts/generate-migration-baseline.sh
HEADER

docker exec "$PG_CONTAINER" pg_dump -U "$PG_USER" -d "$PG_DB" \
    --schema-only --no-owner --no-privileges -T schema_migrations \
| awk '
  /^--/       {next}
  /^SET /     {next}
  /^SELECT pg_catalog\./ {next}
  /^\\/       {next}
  /^[[:space:]]*$/ {next}
  {
    line=$0
    if (inalter==0 && line ~ /^ALTER TABLE ONLY/) { inalter=1; buf=line; next }
    if (inalter==1) {
      buf = buf "\n" line
      if (line ~ /;[[:space:]]*$/) {
        sub(/;[[:space:]]*$/, "", buf)
        print "DO $$ BEGIN"
        print buf ";"
        print "EXCEPTION WHEN duplicate_object THEN NULL; WHEN duplicate_table THEN NULL; END $$;"
        inalter=0; buf=""
      }
      next
    }
    sub(/^CREATE TABLE /, "CREATE TABLE IF NOT EXISTS ", line)
    sub(/^CREATE UNIQUE INDEX /, "CREATE UNIQUE INDEX IF NOT EXISTS ", line)
    sub(/^CREATE INDEX /, "CREATE INDEX IF NOT EXISTS ", line)
    sub(/^CREATE SCHEMA /, "CREATE SCHEMA IF NOT EXISTS ", line)
    print line
  }' >> "$OUT"

echo "wrote $OUT ($(wc -l < "$OUT") lines)"
echo "  tables:      $(grep -c '^CREATE TABLE IF NOT EXISTS' "$OUT")"
echo "  indexes:     $(grep -cE '^CREATE (UNIQUE )?INDEX IF NOT EXISTS' "$OUT")"
echo "  constraints: $(grep -c '^DO \$\$ BEGIN' "$OUT")"
