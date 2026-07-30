# Database Migrations And Rollback Safety

## Principle

Schema changes ship as reviewable, reversible, recorded migration files, not
only Gorm `AutoMigrate`. Every applied version is auditable in
`schema_migrations`, and every shipped migration must have a reviewed down
file.

## Implemented Layout

```text
backend/migrations/
  embed.go
  pre/
    0001_extensions.up.sql
    0001_extensions.down.sql
    0002_baseline.up.sql
    0002_baseline.down.sql
    0003_framework_registry.up.sql
    0003_framework_registry.down.sql
    0004_task_state_storage.up.sql
    0004_task_state_storage.down.sql
  post/
    0001_conversation_owner_identity.up.sql
    0001_conversation_owner_identity.down.sql
    0002_durable_jobs_indexes.up.sql
    0002_durable_jobs_indexes.down.sql
```

The phases run in this order:

1. **`pre/`** migrations apply before model initialization. They provide
   extensions, the versioned baseline, and schema that must exist independently
   of runtime model creation.
2. **Gated AutoMigrate** runs only when `DB_AUTOMIGRATE=true`. This is a
   development aid, not the production schema source of truth.
3. **`post/`** migrations apply constraints, indexes, or backfills that depend
   on existing tables.

Migration versions are ordered within their phase. Do not edit a migration
after it has shipped; add a new numbered pair.

## Runner

`internal/infra/migrate.go` loads the embedded files, applies each pending
migration in its own transaction, and records it. `RunMigrations`, called by
startup and `GetDefaultDB`, executes:

```text
pre -> optional development AutoMigrate -> post
```

### CLI

```text
backend migrate status
backend migrate up
backend migrate down <target>
```

Rollback targets may include their phase:

```text
backend migrate down pre/0003_framework_registry
backend migrate down post/0001_conversation_owner_identity
```

For backward compatibility, an unqualified target defaults to `post`:

```text
backend migrate down 0001_conversation_owner_identity
```

Use an explicit phase in operator procedures. It prevents a correct version
name from being interpreted against the wrong migration directory.

## AutoMigrate Is Retired By Default

`migrations/pre/0002_baseline.up.sql` is the versioned baseline generated from
the migrated schema. `DB_AUTOMIGRATE` defaults to `false`.

- Default or `DB_AUTOMIGRATE=false`: embedded SQL migrations are the schema
  source of truth.
- `DB_AUTOMIGRATE=true`: development only, used to materialize a newly added
  Gorm model before regenerating and reviewing the baseline or a new migration.

The baseline uses idempotent table/index creation and guarded constraints so it
can be applied to a database previously built by AutoMigrate.

After adding or changing a model:

```text
DB_AUTOMIGRATE=true backend migrate up
scripts/generate-migration-baseline.sh
```

Review the generated diff, add a reversible migration when appropriate, and
return to `DB_AUTOMIGRATE=false`.

## Framework Registry Migration

`pre/0003_framework_registry` creates the owner-scoped Framework Registry:

- `framework_preferences`;
- `framework_selection_records`;
- `robert_constitution_versions`;
- digest, owner, status, JSON-shape, and autonomy constraints;
- immutable selection-history and Constitution-lifecycle triggers.

The exact rollback command is:

```text
backend migrate down pre/0003_framework_registry
```

This removes all three tables and their history. It is a destructive rollback,
not a feature toggle. Back up the database, stop code that depends on the
registry, and deploy a compatible previous application version first.

## Task-State Storage Migration

`pre/0004_task_state_storage` creates owner-scoped, durable task-success state:

- immutable completion-plan logs;
- review items with immutable request provenance;
- append-only approval/rejection decisions bound to an exact owner, review
  request digest, and task plan;
- constraints and triggers that reject cross-owner or mutable audit history.

The exact rollback command is:

```text
backend migrate down pre/0004_task_state_storage
```

This removes task completion history, review items, and decisions. Treat it as
a destructive recovery operation: stop task workers, back up the database, and
deploy a compatible application before rolling it back.

## Rules

1. **Additive first.** Add columns, tables, and indexes before code depends on
   them. Delay destructive changes until the new path is proven.
2. **Every up has a down.** `migrate down` refuses a migration without its down
   file.
3. **Backfill safely.** Large backfills run in bounded batches outside the
   request hot path.
4. **Use explicit phases.** Production rollback commands should say `pre/...`
   or `post/...`.
5. **Guard startup.** Startup configuration and `/readyz` expose a schema or
   dependency state the running binary cannot use.
6. **Protect evidence.** Back up owner preferences, selection audit history,
   and Constitution records before a registry rollback.

## Rollback Procedure

1. Stop new writes and capture a database backup.
2. Deploy or prepare the previous compatible binary/tag.
3. Run the exact phase-qualified rollback command.
4. Run `backend migrate status` and confirm the intended version is pending.
5. Start the compatible application.
6. Confirm public `/healthz` and `/readyz` probes and run
   `backend reconcile`.
7. Exercise one authenticated bounded operator flow before reopening normal
   work.

## Verification Status

- Versioned runner, `schema_migrations`, pre/post ordering, and
  `migrate status|up|down`: implemented.
- Explicit pre- and post-phase target parsing: implemented and covered by CLI
  tests.
- Migration parsing, ordering, statement splitting, and embedded-file loading:
  covered by focused unit tests.
- Framework Registry migration and rollback behavior: covered in its own
  isolated Postgres database.
- Task-state persistence, owner scope, redaction, provenance, and immutability:
  covered in a separate isolated Postgres database.
- The migration runner applies the complete pre/post set in a third isolated
  Postgres database, so destructive framework tests cannot make task or runner
  evidence pass accidentally.
- A clean-clone migration and destructive rollback rehearsal on Robert's target
  Windows installation remains environment-dependent release evidence and must
  not be inferred from unit tests alone.
