# Database Migrations & Rollback Safety

## Principle

Schema changes ship as reviewable, reversible, **recorded** migration files —
not only Gorm `AutoMigrate` — so every change can be audited and rolled back.
This is implemented, not aspirational (see [Status](#status)).

## Layout (implemented)

```
backend/migrations/
  embed.go                         // //go:embed pre/*.sql post/*.sql
  pre/
    0001_extensions.up.sql         0001_extensions.down.sql
  post/
    0001_conversation_owner_identity.up.sql   ...down.sql
```

Two phases run around table creation:

- **`pre/`** — applied **before** AutoMigrate. Extensions and anything the model
  DDL depends on (e.g. `uuid-ossp` for UUID defaults).
- **`post/`** — applied **after** AutoMigrate. Indexes, constraints, and
  backfills that need the tables to exist.

Each change is a pair: `*.up.sql` applies it, `*.down.sql` reverses it. Number
sequentially within a phase; never edit a migration that has shipped — add a
new one. Applied versions are recorded in the `schema_migrations` table, so runs
are idempotent and auditable.

## Runner

`internal/infra/migrate.go` loads the embedded files, applies each pending
migration inside its own transaction, and records it. `RunMigrations` (called on
startup and by `GetDefaultDB`) executes: **pre → AutoMigrate (gated) → post**.

### CLI

```
backend migrate status          # show applied/pending per phase, no changes
backend migrate up              # apply all pending migrations
backend migrate down <version>  # roll back one post migration, e.g.
                                #   post/0001_conversation_owner_identity
```

### AutoMigrate is retired

`migrations/pre/0002_baseline.up.sql` contains the full schema (53 tables, 301
indexes, 56 constraints), generated from a migrated database. Because of it,
**`DB_AUTOMIGRATE` now defaults to `false`** — production never mutates its own
schema implicitly.

- Default (unset/`false`): the versioned migrations are the source of truth.
- `DB_AUTOMIGRATE=true`: development only, to materialise a newly-added Gorm
  model before you regenerate the baseline for it.

The baseline is idempotent — tables/indexes use `IF NOT EXISTS` and constraints
are wrapped in exception-guarded `DO` blocks — so it is safe to apply to a
database that was previously built by AutoMigrate.

**After adding or changing a model:**

```
DB_AUTOMIGRATE=true backend migrate up   # let Gorm materialise the model
scripts/generate-migration-baseline.sh   # capture the new schema
# review the diff, commit, go back to DB_AUTOMIGRATE=false
```

## Rules

1. **Additive first.** Add columns/tables/indexes before the code that needs
   them; make destructive changes only after the new code is proven.
2. **Every up has a down.** A migration without a tested rollback is not
   production-ready. `migrate down` refuses a migration with no down file.
3. **Backfill safely.** Large backfills run in batches, off the hot path.
4. **Guard startup.** The startup config guard + `/readyz` catch a schema the
   running binary does not understand.

## Rollback procedure

1. Deploy the previous binary/tag.
2. `backend migrate down <version>` for any post migration the new code added.
3. Confirm `/readyz` is green and `backend reconcile` is clean.
4. Never leave the schema ahead of the running binary.

## Status

- Versioned runner, `schema_migrations` tracking, two-phase ordering, and the
  `migrate status|up|down` CLI: **implemented**.
- Unit tests (`internal/infra/migrate_test.go`): parsing, ordering, statement
  splitting, and embedded-file loading — **passing**.
- Live integration (`internal/infra/migrate_integration_test.go`, `-tags
  integration`): full schema + pre/post apply, idempotency, and rollback against
  **Postgres 17** — **passing**.
- Baseline + `DB_AUTOMIGRATE=false` default — **done and verified** on Postgres
  17: a fresh database builds all 54 tables from migrations alone, a second run
  is a no-op, and the baseline applies cleanly over an AutoMigrate-built schema.
