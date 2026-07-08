# Fresh-Clone Dry Run

Proves the project builds and boots from a clean checkout — no hidden local
state. Run this before claiming the stack is deployable.

## Backend (verified in the goal run)

```bash
git clone <repo> && cd 018-HAI/backend
go build ./...      # expect: success
go vet ./...        # expect: clean
go test ./...       # expect: all packages ok
go run ./cmd doctor # expect: readiness report, exit 0 on sane config
```

Status: **passing** — backend builds, vets, and tests green from a clean clone;
`doctor` runs. (See the final verification report for captured output.)

## Full stack (pending automated evidence)

```bash
cd 018-HAI
cp .env.example .env   # then set real keys for a real deployment
docker compose -f docker-compose.local.yml config   # validate compose (CI does this)
docker compose -f docker-compose.local.yml up -d     # boot Postgres/Redis/Kafka/backend/frontend/gateway
curl localhost/healthz     # expect {"status":"ok"}
curl localhost/readyz      # expect 200 ready
```

Status: a scripted, asserted end-to-end boot now exists —
`scripts/smoke-critical-path.sh` boots a **real local Postgres** + the backend
and asserts health/readiness + the critical path **and the workflow lifecycle**
(intake → approval gate → resolve → audit trail) (**ran 15/15 passing**), no Docker
required. What remains **pending** is the full **Docker Compose** multi-service
boot (Postgres + Redis + Kafka + nginx together), which was not run here because
the Docker daemon was unavailable; in that smoke, Kafka is degraded to a no-op
and Redis is not exercised (feeds phases 031/035 for compose-topology coverage).

## Definition of pass

Backend: build + vet + test + doctor succeed from a clean clone (**met**).
Critical path + workflow lifecycle: `scripts/smoke-critical-path.sh` reaches
`/readyz` ready and asserts the path + lifecycle against real Postgres (**met — 15/15**).
Full Docker Compose stack: `docker compose up` reaches `/readyz` ready with all
services (**pending — needs Docker**).
