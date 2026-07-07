# Data Model & Persistence

## Store of record

PostgreSQL via Gorm. Uploaded media lives on disk under `IMAGE_SAVE_DIR`.

## Key table: `context_memories`

| Column | Type | Notes |
| --- | --- | --- |
| id | uuid (pk) | `uuid_generate_v4()` |
| project_key | varchar(255), **indexed** | tenant/project scope |
| kind | varchar(50), **indexed** | preference/project/decision/contact/note |
| content | text (not null) | the memory |
| summary | text | compacted summary |
| tags | varchar(512) | comma-joined, lowercased |
| confidence | float (default 0.7) | invariant: [0,1] |
| content_hash | varchar(64), **indexed** | dedup key |
| archived | bool (default false), **indexed** | soft archival |
| last_used_at / created_at / updated_at | timestamps | recency + audit |

## Persistence principles

- **Ownership/scope:** every user-owned record carries a project scope; queries
  filter by it (isolation-tested).
- **Invariants:** enforced in code (`internal/invariants`) at write time and
  auditable at rest via `backend reconcile`.
- **Deduplication:** exact (content hash) and near-duplicate merge on write.
- **Soft delete first:** archive before delete; retention policy governs
  candidacy (`internal/retention`).

## Indexing posture & scale

Current indexes cover the hot paths (project_key, kind, content_hash, archived).
List/search runs in memory today (fine to tens of thousands of rows/project); at
larger scale, add a composite `(project_key, archived, updated_at)` index and a
trigram/full-text index on `content` (see `docs/performance-baseline.md`).

## Migrations

Ship migration files (not only Gorm `AutoMigrate`) so schema changes are
reviewable and reversible — see `docs/migrations.md`.
