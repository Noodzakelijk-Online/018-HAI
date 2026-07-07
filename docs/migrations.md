# Database Migrations & Rollback Safety

## Principle

Schema changes ship as reviewable, reversible migration files — not only Gorm
`AutoMigrate` — so every change can be audited and rolled back.

## Layout (recommended)

```
backend/migrations/
  0001_init.up.sql        0001_init.down.sql
  0002_add_indexes.up.sql 0002_add_indexes.down.sql
```

Each change is a pair: `*.up.sql` applies it, `*.down.sql` reverses it. Number
sequentially; never edit a migration that has shipped — add a new one.

## Rules

1. **Additive first.** Add columns/tables/indexes before the code that needs
   them; make destructive changes only after the new code is proven in canary.
2. **Every up has a down.** A migration without a tested rollback is not
   production-ready.
3. **Backfill safely.** Large backfills run in batches, off the hot path.
4. **Guard startup.** The app should not run against a schema newer than it
   understands; the startup config guard + `/readyz` help catch misconfig.

## Rollback procedure

1. Deploy the previous binary/tag.
2. Apply the corresponding `*.down.sql` for any migration the new code added.
3. Confirm `/readyz` is green and `backend reconcile` is clean.
4. Never leave the schema ahead of the running binary.

## Example (indexing at scale)

```sql
-- 0002_add_indexes.up.sql
CREATE INDEX IF NOT EXISTS idx_memories_scope_recent
  ON context_memories (project_key, archived, updated_at);

-- 0002_add_indexes.down.sql
DROP INDEX IF EXISTS idx_memories_scope_recent;
```
