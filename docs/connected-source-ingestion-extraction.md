# Connected-Source Ingestion and Extraction

Connected accounts and folders are treated as structured context sources. The
system fetches metadata first, imports only allowed items, deduplicates by source
and external id, extracts searchable records, links every extracted record back
to its source, and only promotes useful low-risk facts into memory.

## Supported Connector Categories

- email accounts
- contact/address-book accounts
- calendars
- cloud drives and documents
- Trello or project boards
- ShareT link inventories
- GitHub repositories, issues, pull requests, commits, and actions
- selected local folders

The live Google paths are separate least-privilege grants: Gmail read-only,
Drive read-only, Contacts read-only, and Calendar read-only. Google Contacts
uses a bounded People API backfill followed by the provider's native sync token.
Google Calendar uses a bounded primary-calendar backfill followed by its native
sync token. Event start and end times remain structured source metadata. A
meaningful upcoming event inside the 14-day planning horizon may create a
source-backed preparation proposal and deterministic due/check reminder; past
backfill events stay searchable context. Event intervals inside a bounded 30-day
horizon are compared locally. An overlap involving a changed event creates one
stable, review-gated conflict record; moving or cancelling the event retracts
the stale conflict workflow. Imported people are review-required candidates.
Contact removals and calendar cancellations become non-destructive source
tombstones; a cancellation first stops any prior source-derived workflow and
then opens owner review. It never deletes canonical people, tasks, obligations,
or evidence. These adapters have no remote create, update, merge,
invitation-response, or delete method.

The ShareT path uses an operator-created `connector:read` credential stored only
in the HAI environment. It verifies read capability, follows all link-history
pages up to an explicit completeness limit, and fails instead of silently
truncating a larger account. Imported records contain card/board labels,
permissions, lifecycle state, counts, and public-link provenance. Participant
email addresses and all credentials are excluded. HAI has no ShareT create,
update, comment, relay, or revoke method.

The connector registry is configuration-friendly, so future connector keys can
be added without changing the task engine. The implementation exposes
manual import, historical backfill, incremental sync, scheduled sync, webhook
sync, and folder watcher as modes. Connector status is reported separately:
some paths are live read-only adapters, some are local/export readers, and some
remain modeled only. They share the same source registry and sync-job table.

## Data Flow

1. Register connector capabilities and minimum permissions.
2. Connect a source with local-only, sync-frequency, and exclude controls.
3. Sync imported items incrementally with a cursor.
4. Store raw item metadata and content hash.
5. Extract text, summary, entities, dates, tasks, decisions, and follow-ups.
6. Store compact lexical keyword index entries. Embeddings are not written or
   claimed until a real local embedding adapter is configured.
7. Mark sensitive or uncertain records for review.
8. Store provenance through source URI, label, source id, and raw item id.
9. Search extracted records by keyword relevance, project match, recency, and
   source provenance.
10. Promote useful non-sensitive records into context memory.
11. Log sync, extraction, correction, archive, delete, pause, re-index, and
    revoke actions.
12. Delete derived index records in the same database transaction when an
    extraction is deleted; record the delete audit event only after it commits.

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
