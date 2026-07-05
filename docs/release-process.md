# Release Process: Canary & Rollback

018-HAI runs local-first via Docker Compose. This defines how to ship a change
safely and roll back cleanly.

## Pre-release gates

1. `go build ./...` and `go test ./...` are green (backend).
2. `backend doctor` reports no failures for the target environment.
3. Frontend `ng build` succeeds.
4. Completion matrix and worklog updated for anything user-facing.

## Versioning

Semantic tags (`vMAJOR.MINOR.PATCH`). Tag only from a green build. The current
baseline tag is `v1.0.0`.

## Canary (single-host)

1. Bring up the new images alongside the current stack on a non-default port /
   compose project.
2. Probe the canary: `GET /healthz` (liveness) then `GET /readyz` (readiness).
   Do not proceed while `/readyz` returns 503.
3. Smoke the critical path: create a memory, run a workflow item through an
   approval gate, confirm an audit event.
4. Watch logs for redaction failures or unexpected 5xx for a soak window.

## Promote

Once the canary is healthy, switch the gateway/compose to the new images and
retire the old containers.

## Rollback

1. Re-deploy the previous image tag (compose `up -d` with the prior tag).
2. If a database migration shipped, apply its down-migration or restore from the
   most recent backup (see `docs/backup-restore.md`). Never leave the schema
   ahead of the running binary.
3. Confirm `/readyz` is green post-rollback.

## Migration safety

- Ship migration files (not only Gorm `AutoMigrate`) so changes are reviewable
  and reversible.
- Additive migrations first; destructive changes only after the new code is
  proven in canary.
