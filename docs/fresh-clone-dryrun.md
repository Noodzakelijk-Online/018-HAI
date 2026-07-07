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

Status: **pending** — compose config is validated in CI, but a scripted,
asserted end-to-end boot (health+readiness green) is not yet automated. This is
the main gap between "builds from clean" and "boots from clean," tracked for a
future phase (003/031/092).

## Definition of pass

Backend: build + vet + test + doctor all succeed from a clean clone (**met**).
Full stack: `docker compose up` reaches `/readyz` ready with no manual fix-ups
(**pending automation**).
