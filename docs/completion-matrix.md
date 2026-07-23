# Completion Matrix

Honest status of the review items. Columns are deliberately separated so
"implemented" is never confused with "proven against a real account":

- **Implemented** — code, persistence, and API/contract exist in this repo.
- **Unit-tested** — focused automated coverage runs in `go test ./...` (no
  external services).
- **Sandbox-tested** — exercised against a disposable real dependency (e.g. a
  throwaway Postgres, a sandbox mailbox) — not production, but not a mock.
- **Live-tested** — a real credential/account completed a bounded, approved,
  end-to-end run with audit/verification evidence.
- **Deferred** — intentionally not done here, with the reason.

Legend: ✅ done · 🟡 partial · ⬜ not yet · — n/a

## 1. Live connectors

| Item | Implemented | Unit-tested | Sandbox-tested | Live-tested | Notes / Deferred |
| --- | :--: | :--: | :--: | :--: | --- |
| Trello **read-only REST** connector (`internal/source/trello.go`) | ✅ | ✅ | ⬜ | ⬜ | 5 tests incl. GET-only (read-only) assertion, incremental cursor, provenance, host allowlist, credential-required. Live run needs a real `TRELLO_API_KEY` + least-privilege `TRELLO_READ_TOKEN`. |
| — least privilege / no stored secrets | ✅ | ✅ | — | — | Creds from env only; no key/token/board id on the row. |
| — incremental sync + cursor | ✅ | ✅ | ⬜ | ⬜ | Cursor = max card `dateLastActivity`; unchanged cards skipped (test). |
| — pause / revoke | ✅ | ✅ | — | — | Generic `Pause`/`Revoke` apply to the connector. |
| — provenance + audit logs | ✅ | ✅ | — | — | Card `shortUrl` on every extraction; audit via sync pipeline. |
| Trello **JSON export** (`project-board`, local) | ✅ | ✅ | — | — | Distinct `local_only` path; unchanged. |
| Gmail OAuth (consent, encrypted token, refresh, incremental, revoke, source links) | ✅ | ✅ | ⬜ | ⬜ | Pre-existing (`internal/source/oauth.go`, `internal/googleoauth`). **Sandbox mailbox acceptance run is the gate.** |
| Google Drive / Calendar | ✅ (export-only) | ✅ | — | — | Marked `local_only`. **Live API intentionally deferred** (documented in provider-reality review). |
| Documentation contradiction resolved | ✅ | — | — | — | `external-provider-reality-review.md` + README now report verified adapter status, not intent. |

## 2. Operational proof

| Item | Implemented | Unit-tested | Sandbox-tested | Live-tested | Notes / Deferred |
| --- | :--: | :--: | :--: | :--: | --- |
| Windows 11 fresh-clone acceptance run | — | — | ⬜ | ⬜ | **Deferred external gate** — requires a Windows 11 host + Docker Desktop. Build environment here is Linux. |
| Browser E2E tests (`frontend/e2e/`) | ✅ (authored) | — | ⬜ | ⬜ | Playwright suite for login→source→sync→workflow→approve→bounded-exec against real `data-testid`s. **Not yet executed** (needs running stack + seeded account; two source selectors flagged TODO). |
| Health/readiness vs the **actual** stack | ✅ | ✅ | ✅ | ✅ | **Proven against a live stack**, not mocks. Backend booted against real Postgres 17 + Redis: `/readyz` reported `database.connection reachable in 12ms`, `redis.connection reachable in 1ms`, and correctly flagged an unreachable Kafka broker as `warn` (`degraded`/HTTP 200). **Killing Postgres flipped it to `not_ready`/HTTP 503** with `fail:1`, while `/healthz` stayed 200 (liveness ≠ readiness); restarting Postgres recovered it to `degraded`/200. |

## 3. Production engineering

| Item | Implemented | Unit-tested | Sandbox-tested | Live-tested | Notes / Deferred |
| --- | :--: | :--: | :--: | :--: | --- |
| Versioned DB migrations (`backend/migrations/`, `internal/infra/migrate.go`) | ✅ | ✅ | ✅ | ✅ | Runner + `schema_migrations` + two-phase (pre/post) + `migrate status\|up\|down` CLI. **Verified against real Postgres 17**: full schema apply, idempotency, rollback+re-apply. |
| Replace production reliance on AutoMigrate | ✅ | ✅ | ✅ | ✅ | **Done.** Generated baseline (`pre/0002_baseline`, 53 tables / 301 indexes / 56 guarded constraints) and `DB_AUTOMIGRATE` now defaults to **false**. Proven 3 ways on real Postgres 17: fresh DB builds all 54 tables from migrations alone; re-run idempotent; baseline safe over an existing AutoMigrate-built DB. Regenerate via `scripts/generate-migration-baseline.sh`. |
| Durable worker (scheduling/retry) | ✅ | ✅ | ✅ | ⬜ | **Built** (`internal/durablejob`): persisted jobs, `RunAt` scheduling, bounded retry with backoff, dead-lettering, lease-based crash recovery, panic containment. 8 tests pass incl. **real Postgres**: survives process restart, 25 jobs × 2 concurrent workers each executed exactly once (`FOR UPDATE SKIP LOCKED`), orphaned leases reclaimed. **All three schedulers now run on it** — source (`source.scan` → one retryable `source.sync` per due source), workflow (`workflow.sweep`), and ambient (`ambient.scan`), via a shared `RegisterRecurring` helper that keeps each a self-rescheduling singleton. Each falls back to its legacy in-process ticker (with a log line) if the queue is unreachable. Recurring jobs reschedule on success **or** final attempt, so a burst of failures cannot silently kill a schedule. |
| Two-account isolation | 🟡 | ✅ | ⬜ | ⬜ | Owner-scoped isolation tested (`TestRunDueScheduledSyncsForOwnerDoesNotTouchAnotherOwnersSources`, alice/bob). **Two-real-account live isolation run is the gate.** |
| Risky runtimes disabled until gated | ✅ | ✅ | — | ⬜ | Agent runtimes, browser automation, paid providers, and external side effects are disabled by default behind approval boundaries. Per-runtime **live** integration tests remain deferred gates. |

## 4. Delivery hygiene

| Item | Status | Evidence |
| --- | :--: | --- |
| `.gitignore` excludes env backups (`.env.local.bak`, `*.bak`) | ✅ | `git check-ignore` confirms `.env.local.bak` ignored. |
| No local backups / personal fixtures / diagnostics in the archive | ✅ | `connected-sources/*.eml`, `connected-sources/summary`, `HAI-Work-Report.pdf`, `.claude/` now ignored; only `.gitkeep` tracked in data dirs. |
| No real credentials tracked | ✅ | Tracked `.env*` files are Docker-compose **dev defaults** (`postgres/postgres`, `JWT_SECRET=secret`), not production secrets. |
| Completion matrix | ✅ | This document. |

## Remaining external gates (need resources outside this environment)

1. **Trello live run** — real API key + least-privilege read token against a throwaway board.
2. **Gmail sandbox acceptance** — Google Cloud OAuth app + dedicated sandbox mailbox: consent → encrypted token → refresh → incremental → revoke → retained source links.
3. **Windows 11 fresh-clone run** — `docker compose up --build` + the full operator flow on a clean Windows host.
4. **Browser E2E execution** — run `frontend/e2e` against a live stack with a seeded account; confirm the two TODO source selectors.
5. **Two-real-account isolation** — two real provider accounts, end-to-end.
6. **Per-runtime live tests** — one controlled integration test + approval boundary each for agent runtimes, browser automation, and any paid provider before enabling.

> Closed since the first pass: the migration baseline (AutoMigrate is now off by
> default) and the durable worker model are both built and verified against real
> Postgres — they are no longer external gates.

## Reproduce the automated evidence

Backend has no local Go toolchain; use the pinned container (Go 1.21.13):

```bash
# Unit tests (no external services)
docker run --rm -v "$PWD/backend":/app -w /app golang:1.21.13 go test ./...

# Trello connector tests
docker run --rm -v "$PWD/backend":/app -w /app golang:1.21.13 \
  go test ./internal/source/ -run Trello -v

# Migration runner vs REAL Postgres 17 (data dir on tmpfs so it needs no disk)
docker network create hai-net
docker run -d --name hai-pg --network hai-net --tmpfs /var/lib/postgresql/data \
  -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=automation_hub postgres:17-alpine
docker run --rm --network hai-net \
  -e HAI_TEST_DATABASE_DSN="host=hai-pg user=postgres password=postgres dbname=automation_hub port=5432 sslmode=disable TimeZone=UTC" \
  -v "$PWD/backend":/app -w /app golang:1.21.13 \
  go test -tags integration -run 'Migrat|Rollback' ./internal/infra/ -v
```
