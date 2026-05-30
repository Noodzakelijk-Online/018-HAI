# Connected-Source Ingestion and Extraction

Connected accounts and folders are treated as structured context sources. The
system fetches metadata first, imports only allowed items, deduplicates by source
and external id, extracts searchable records, links every extracted record back
to its source, and only promotes useful low-risk facts into memory.

## Supported Connector Categories

- email accounts
- calendars
- cloud drives and documents
- Trello or project boards
- GitHub repositories, issues, pull requests, commits, and actions
- selected local folders

The connector registry is configuration-friendly, so future connector keys can
be added without changing the task engine. The initial implementation exposes
manual import, historical backfill, incremental sync, scheduled sync, webhook
sync, and folder watcher as modes. Real OAuth/webhook workers can attach to the
same source registry and sync-job table.

## Data Flow

1. Register connector capabilities and minimum permissions.
2. Connect a source with local-only, sync-frequency, and exclude controls.
3. Sync imported items incrementally with a cursor.
4. Store raw item metadata and content hash.
5. Extract text, summary, entities, dates, tasks, decisions, and follow-ups.
6. Store keyword index entries and local vector-reference placeholders.
7. Mark sensitive or uncertain records for review.
8. Store provenance through source URI, label, source id, and raw item id.
9. Search extracted records by keyword relevance, project match, recency, and
   source provenance.
10. Promote useful non-sensitive records into context memory.
11. Log sync, extraction, correction, archive, delete, pause, re-index, and
    revoke actions.

## API Surface

- `GET /api/v1/sources/connectors`
- `GET /api/v1/sources`
- `POST /api/v1/sources`
- `PATCH /api/v1/sources/:id`
- `POST /api/v1/sources/:id/sync`
- `POST /api/v1/sources/:id/reindex`
- `POST /api/v1/sources/:id/pause`
- `POST /api/v1/sources/:id/resume`
- `POST /api/v1/sources/:id/revoke`
- `POST /api/v1/sources/search`
- `GET /api/v1/sources/extractions`
- `PATCH /api/v1/sources/extractions/:id`
- `POST /api/v1/sources/extractions/:id/archive`
- `DELETE /api/v1/sources/extractions/:id`
- `GET /api/v1/sources/audit-logs`

## Task Integration

The universal task success engine searches connected-source extractions before
planning. Relevant source context is included in the task context plan with
scores and provenance. Sensitive records are not loaded unless explicitly
allowed by the search request.
