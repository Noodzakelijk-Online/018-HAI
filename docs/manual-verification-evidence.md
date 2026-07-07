# Manual Verification Evidence

Executed commands and their results — the evidentiary basis for status claims.
Reflects the final post-implementation state.

## Commands run

| Command | Result |
| --- | --- |
| `go version` | `go1.25.6 darwin/arm64` |
| `go build ./...` (backend) | **PASS** |
| `go vet ./...` (backend) | **CLEAN** |
| `go test ./...` (backend) | **54 packages ok**, 0 failures |
| `find backend -name '*_test.go' \| wc -l` | **78** test files |
| `go run ./cmd doctor` | `12 ok, 2 warn, 0 fail` → READY WITH WARNINGS |
| `go run ./cmd reconcile` | runs (dry-run; graceful without DB) |
| `npm run build` (frontend) | **PASS** (Angular production build) |
| `ng test --watch=false --browsers=ChromeHeadless` | **20/20 SUCCESS** (headless Chrome) |
| `find frontend/src -name '*.spec.ts' \| wc -l` | **11** test files |
| `scripts/smoke-critical-path.sh` | **7 passed, 0 failed** (see boundary below) |

The two `doctor` warnings are the expected empty `BACKEND_API_SHARED_KEY` and
`HAI_MEMORY_ENCRYPTION_KEY` under default local config — correctly surfaced, not
failures.

## What this proves

- The backend compiles, vets clean, and passes its full unit suite (54 packages).
- The Angular app builds and its unit suite is green (20/20 in headless Chrome),
  including 10 new component specs and 10 repaired pre-existing specs.
- CLI subcommands (`doctor`, `reconcile`) run and behave as documented.
- The **critical path runs end-to-end**: `smoke-critical-path.sh` booted a real
  local PostgreSQL and the backend and asserted `healthz`, `readyz`, memory
  create → persist → search, and the workflow/os/system surfaces (7/7).

## What it does NOT prove (honest boundary)

- The **full Docker Compose multi-service stack** (Postgres + Redis + Kafka +
  nginx together) was **not** booted — the Docker daemon was unavailable here.
  The smoke used a standalone local Postgres, with **Kafka degraded to a no-op**
  and Redis not exercised. Compose-topology behavior (event publishing, gateway
  routing) therefore has no executed test in this evidence set.
- **Live external providers** (Gmail/Drive/Calendar/paid LLMs) were not
  exercised — intentionally disabled in code.
- **Deeper workflow-lifecycle e2e** (intake → approve → execute → verify) beyond
  the critical-path surfaces is not yet asserted.

New behaviors added during the run (e.g. `/memory/query`, `/system/info`,
`/readyz`, rate limiting, idempotency, RBAC middleware) are each covered by
package/httptest cases within the 54 passing backend packages.
