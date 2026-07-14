# Verification evidence — honest health, readiness, and boot

This document records exactly what was run and what happened, on a real machine,
for the `feat/honest-health-readiness` change set. It follows the project rule of
separating **verified by the developer locally** from **verified on the client
laptop**. Everything below is the former; the laptop column is filled in during
the remote (TeamViewer/AnyDesk) session.

- Date: 2026-07-14
- Host: Linux 6.8, Docker 29.1.3, Docker Compose 2.40.3
- Stack file: `docker-compose.local.yml` with `--env-file .env.local`
- Go build/test run in `golang:1.23` container (no Go toolchain on host)

> Note on which compose file is authoritative: `docker-compose.local.yml` builds
> the backend, frontend and IDP **from source**. `docker-compose.yml` pulls
> prebuilt upstream `jacksonbarreto/*` images and is not the HAI stack. Use the
> local file. This corrects the earlier `ANALYSIS_REPORT.md`, which recommended
> the opposite.

---

## 1. The boot blocker (before the fix)

Command:

```
docker compose -f docker-compose.local.yml --env-file .env.local up -d --build
```

Result: **FAILED to start.**

```
Error response from daemon: failed to set up container networking:
Bind for 0.0.0.0:6379 failed: port is already allocated
```

Root cause: the Redis service published host port `6379`, which was already in
use on the machine by an unrelated container. Redis failed to bind, its
dependents never started, and **zero** application containers came up.

## 2. After the fix — full boot

Change: infrastructure services (Postgres, Redis) now bind to `127.0.0.1` on
configurable host ports, defaulting Redis to `6380` to avoid the common
already-running-Redis clash. Only the gateway is published on all interfaces.

```
docker compose -f docker-compose.local.yml --env-file .env.local up -d --build
docker compose -f docker-compose.local.yml --env-file .env.local ps
```

Result: **11/11 services up, 10 healthy.**

| Service | Status |
| --- | --- |
| backend | Up (healthy) |
| frontend | Up (healthy) |
| nginx (gateway) | Up (healthy) |
| idp | Up (healthy) |
| postgres-automation | Up (healthy) |
| postgres-idp | Up (healthy) |
| redis | Up (healthy) |
| kafka | Up (healthy) |
| zookeeper | Up (healthy) |
| generic-auto | Up (healthy) |
| nginxconfigmanager | Up |

## 3. The readiness lie (before the fix)

`/readyz` was backed by `doctor.Diagnose`, which performs no I/O — it only checks
that configuration strings are non-empty. To demonstrate, all three data
dependencies were stopped and the endpoint was queried:

```
docker stop 018-hai-postgres-automation 018-hai-redis 018-hai-kafka
curl -s http://127.0.0.1:17070/readyz -w '%{http_code}'
```

Result: **HTTP 200, `"status":"ready"`, 14 ok / 0 fail** — with the database,
cache and event bus all stopped. Docker also reported the backend container
`healthy` throughout, because its healthcheck was a plain TCP socket probe.

## 4. Honest readiness (after the fix)

`/readyz` now runs live probes (authenticated Postgres ping, Redis PING over
RESP, Kafka client connect, LLM provider HTTP) and reports three states.

**All dependencies up:**

```
curl -s http://127.0.0.1:17070/readyz -w '%{http_code}'
```

```
HTTP 200  status=degraded  {ok:18, warn:1, fail:0}
  [OK  ] database.connection   reachable in 14ms
  [OK  ] redis.connection      reachable in 2ms
  [OK  ] kafka.connection      reachable in 3ms
  [WARN] llm.provider          no provider configured (OLLAMA_BASE_URL / FREE_CLOUD_OPENAI_BASE_URL unset)
```

`degraded`, not `ready`, is correct: no LLM provider is configured by default, so
generation genuinely is unavailable. Nothing is being hidden.

**Database stopped:**

```
docker stop 018-hai-postgres-automation
curl -s http://127.0.0.1:17070/readyz -w '%{http_code}'
```

```
HTTP 503  status=not_ready  {ok:17, warn:1, fail:1}
  [FAIL] database.connection   unreachable after 2ms: connect postgres-automation:5432/automation ...
```

The endpoint now fails, with HTTP 503, for the exact reason readiness exists.

## 5. Honest container healthcheck

The backend healthcheck now polls `/readyz` with `curl -f`, so it fails on the
503. Observed transitions:

- All dependencies up: backend reaches `healthy` ~32s after start.
- Database stopped: backend flips to `unhealthy` within ~40s (`retries: 3`).
- Database restarted: backend returns to `healthy` on the next probe (~8s).

Health probe log line while the DB was down:

```
exit=22 out=curl: (22) The requested URL returned error: 503
```

## 6. Placeholder-secret detection

`.env.example` ships `RUN_MODE=production` with `change-this-*` placeholder
secrets. The doctor report now flags these instead of passing them:

- In production mode: `security.backendApiKey` / `jwtSecret` / `memoryEncryptionKey`
  report **FAIL** when the value is a known placeholder, so `/readyz` is
  `not_ready` until real secrets are set.
- Outside production: the same values report **WARN** (visible, non-blocking).

`scripts/generate-secrets.sh` writes real `openssl rand -hex 32` values that can
be appended to `.env.local`.

## 7. Gateway routing drift

The nginx `/api/v1/*` allowlist had drifted from the backend's actual route
groups. Before the fix, three groups fell through to the IDP and returned 404
via the gateway: `agent-cycle`, `flags`, `system`.

```
# before: GET http://localhost:8088/api/v1/flags -> HTTP 404
# after:  GET http://localhost:8088/api/v1/flags -> HTTP 401 (reaches backend, gated by gateway login)
```

The 401 is correct: the route now maps to the backend, which the gateway guards
with its `auth_request` login check. `/healthz` and `/readyz` are now routable
through the gateway too (`/healthz` open, `/readyz` behind auth).

## 8. Two bugs the new probe surfaced immediately

Running the live probes for the first time exposed two latent
misconfigurations, both now fixed:

1. **Redis was unreachable from the backend.** Redis was attached only to
   `idp-network` while the backend runs on `automation-hub-network`; the probe
   reported `dial redis:6379: lookup redis ... no such host`. Redis is now also
   on `automation-hub-network`. (The backend does not yet *use* Redis — quota
   and rate-limit state is still in-process — but it can now reach it.)
2. **`JWT_SECRET` was never passed to the backend container.** It was defined for
   the IDP only, so the backend's identity middleware silently ran as a no-op.
   The secret (and `RUN_MODE`) are now passed to the backend.

## 9. Go build and tests

```
go build ./...      # clean
go vet ./...        # clean
go test ./...       # ok: 55 packages, 0 failures
```

New tests: `internal/doctor/probe_test.go` (probe severity/criticality, timeout
bounding, order preservation, placeholder detection, the exact "valid config +
dead database" regression) and updated `internal/router/readiness_test.go` for
the tri-state semantics.

## 10. Frontend — System Status page

A new authenticated page at `/system-status` (nav: Control Center → System →
System Status) consumes `/readyz` and renders it: an overall banner
(ready/degraded/not ready), a recommended-next-actions list built from the
non-healthy checks, and per-subsystem cards that sort failures to the top. It
polls every 15s, and treats a 503 body as data rather than an error, so a
not-ready backend is shown rather than swallowed.

Build: the Angular image built cleanly; the page compiled into its own lazy
chunk (`pages-system-status-system-status-module`, 10.53 kB).

Verified in a real browser (Playwright, logged in as a locally-registered test
user, through the gateway on :8088):

- **All dependencies up** — banner "Degraded — serving with warnings", 18 ok / 1
  warn / 0 fail; the one warning is the absent LLM provider, with live probe
  timings shown (database 14 ms, redis 2 ms, kafka 11 ms).
  Screenshot: `docs/evidence/system-status-healthy.png`.
- **Database stopped** — the page (after its poll / a Refresh click) flips to a
  red "Not ready — a critical dependency is down", 17 ok / 1 warn / 1 fail; the
  Database card sorts to the top and shows the real driver error. Everything
  else stays green. Screenshot: `docs/evidence/system-status-db-down.png`.
- **Unauthenticated** — `/readyz` through the gateway returns 401 with no body,
  so infrastructure detail (hostnames, brokers, which secrets are placeholders)
  is not exposed to anonymous callers.

Note: the browser check registered a throwaway local account
(`verify@local.test`) via the open `/api/v1/auth/register` endpoint. It exists
only in the local Postgres volume.

---

## 11. Redis-backed rate limiting (quota across restarts)

The per-IP rate limiter kept its counters in a per-process map, so the limit
reset on every restart and could not hold across multiple backend instances.
Counters now live in Redis when `REDIS_ADDR` is set, with an in-process
fallback when it is not.

Fixing this surfaced another plumbing gap first: `RATE_LIMIT_PER_MINUTE` was
defined in `.env` but never passed to the backend container, so the limiter
could not be enabled at all regardless of configuration. Now passed through.

Verified with `RATE_LIMIT_PER_MINUTE=5`:

```
# backend log on startup:
ratelimit: using shared Redis store at redis:6379

# six requests in one window:
req 1..5 -> 200
req 6    -> 429   (Retry-After set)

# the counter is visibly in Redis:
redis-cli --scan --pattern 'ratelimit:*'
  ratelimit:172.23.0.1:29733334
```

Durability across a backend restart (the actual ask):

```
# before restart
ratelimit:172.23.0.1:29733334 = 100
docker restart 018-hai-backend
# after restart — same key, same value, untouched by the restart
ratelimit:172.23.0.1:29733334 = 100
```

And that enforcement reads the shared state rather than process memory: seeding
the counter in Redis to 99 caused the next request to be rejected immediately,
without the backend having counted it locally.

Default is unchanged (`RATE_LIMIT_PER_MINUTE=0`, disabled): 12 rapid requests
all return 200, so normal use is unaffected. Unit tests cover the limit
boundary, per-key isolation, and fail-open/fail-closed behaviour when Redis is
unavailable, using a deterministic fake so they need no running Redis.

## 12. Honest connector status

The connector catalog reported all 9 connectors as `operational`, and the
dashboard showed "9 connectors operational". But only two make a live network
connection (GitHub REST, and the JSON feed); four are named after cloud services
(email, calendar, cloud-documents, project-board) while actually reading export
files from a local folder; one (`odoo-herp`) generates built-in domain models
with no live connection; the rest read local folders/exports. There is no OAuth
anywhere in the backend.

`AdapterStatus` was overloaded: it was both the UI label and the gate deciding
whether a source can be created (`service.go`). So the fix separates the two:

- Honest status values: `operational` (live remote), `local_only` (reads local
  files/folders), `modeled` (built-in models), `not_implemented` (contract only).
- A new `adapterIsUsable` helper gates source creation: everything except
  `not_implemented` can still be created, so the local-folder connectors that
  genuinely work keep working.

Verified:

```
# catalog now reports honestly:
github, json-feed          -> operational   (2)
email, calendar, cloud-documents,
project-board, local-folder,
whatsapp-export            -> local_only    (6)
odoo-herp                  -> modeled       (1)

# creating a local_only source still succeeds (function preserved):
POST /api/v1/sources {connectorKey: "email"} -> HTTP 201
```

The dashboard now reads **"2 live · 6 local · 1 modeled of 9 connectors"** instead
of "9 operational", and each connector in the picker is labelled honestly
("… — local files only", "… — live", "… — built-in model"). Screenshot:
`docs/evidence/connectors-honest-status.png`. All 55 backend test packages pass;
the catalog test now asserts the honest statuses.

This is deliberately a downward change in the headline number — it removes an
overstatement, which is the point. No connector lost any real capability.

## 13. RBAC is now enforced (was built but wired to nothing)

The `rbac` package (roles owner/operator/viewer; permissions read/write/approve/
admin) was fully implemented and unit-tested but attached to **zero** routes —
`requirePermission` appeared 0 times in `routes.go`. And no role reached the
backend: the gateway forwarded only the cookie and shared key, so every caller
defaulted to `viewer`, which meant wiring write-gates naively would have 403'd
the entire UI.

This wires it end to end:

- `enforcePermissions()` gates the whole `/api/v1` group by HTTP method — reads
  need `viewer`, mutations need `operator`/`owner`.
- The gateway injects `X-HAI-Role: owner` for a request that passed
  `auth_verify` (an authenticated session is the local operator). It is set, not
  added, so a client cannot forge its own role.
- An unauthenticated caller still resolves to `viewer`, so a leaked shared key
  can read but cannot mutate.

Verified on the running stack:

| Caller | GET | POST/DELETE |
| --- | --- | --- |
| UI through the gateway (authenticated) | 200 | 201 — UI unaffected |
| Direct backend, shared key only (viewer) | 200 | **403** — mutation blocked |
| Direct backend, explicit `X-HAI-Role: owner` | 200 | 201 |

So a bare shared key on the local network can no longer create, update, or
delete — mutations now require an authenticated session. Unit tests cover the
method→permission mapping and the viewer/operator/owner outcomes.

Honest limitation: because the IDP does not issue per-user roles, an
authenticated session maps to a single `owner` role rather than distinct
per-user roles. True multi-user RBAC needs the IDP to carry a role claim; the
enforcement and role-resolution path are now in place for when it does.

## What is NOT claimed here

- No OAuth connector was implemented. Connector statuses are now honest about
  that (section 12), but building an actual Gmail/Drive/Calendar/Trello adapter
  is still outstanding and needs the client's OAuth credentials.
- Redis now backs rate-limit counters, but the LLM usage/budget accounting in
  `internal/llm` is still an in-process map. Persisting that is the natural next
  step; it was left out here because it cannot be exercised end to end until an
  LLM provider is configured, and this change set only ships what was verified.
- RBAC is now enforced across the API (section 13), but as a single owner role
  per authenticated session, not per-user roles — that needs IDP role support.

These are called out honestly rather than presented as done.
