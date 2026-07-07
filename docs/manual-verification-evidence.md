# Manual Verification Evidence

Executed commands and their results — the evidentiary basis for status claims.
Captured on the goal-run branch.

## Commands run

| Command | Result |
| --- | --- |
| `go version` | `go1.25.6 darwin/arm64` |
| `go build ./...` | **PASS** |
| `go vet ./...` | **CLEAN** |
| `go test ./...` | **53 packages ok**, 0 failures |
| `find . -name '*_test.go' \| wc -l` | **76** test files |
| `go run ./cmd doctor` | `12 ok, 2 warn, 0 fail` → READY WITH WARNINGS |
| `go run ./cmd reconcile` | runs (dry-run; graceful without DB) |

The two warnings are the expected empty `BACKEND_API_SHARED_KEY` and
`HAI_MEMORY_ENCRYPTION_KEY` under default local config — correctly surfaced, not
failures.

## What this proves

- The backend compiles, vets clean, and passes its full unit suite from the
  current tree.
- CLI subcommands (`doctor`, `reconcile`) run and behave as documented.
- Readiness tooling correctly distinguishes warnings from failures.

## What it does NOT prove (honest boundary)

- A live, multi-service compose boot with health/readiness green — **not run
  here** (requires the container stack); tracked at 003/031/092.
- Angular runtime behavior — the frontend builds in CI but its component tests
  are not exercised in this evidence set.

New behaviors added this run (e.g. `/memory/query`, `/system/info`, `/readyz`,
rate limiting, idempotency) are each covered by package/httptest cases counted in
the 53 passing packages.
