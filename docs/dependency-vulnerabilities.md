# Dependency Vulnerabilities — Status & Next Action

Records the actual `govulncheck` result, the CI scanning posture, and the exact
remediation, so nothing is hand-waved.

## Applied this pass (real reduction: 20 → 17)

Upgraded the two most-relevant modules to fixed versions, cleanly (no
go-directive/vet cascade; build, vet, tests, and the smoke all stayed green):

- `golang.org/x/net` v0.15.0 → **v0.17.0** (HTTP/2 rapid-reset in the live server path, GO-2023-2102)
- `github.com/jackc/pgx/v5` v5.3.1 → **v5.5.5** (via `gorm.io/driver/postgres` → v1.5.9)

Code-affecting vulnerabilities dropped from **20 to 17**.

## Backend (`govulncheck ./...`)

**Remaining: 17 code-affecting vulnerabilities**, now almost entirely the Go
**standard library** (fixed across go1.25.7–1.25.11) plus later `x/net`/`pgx`
releases.

| Source | Present | Fixed in |
| --- | --- | --- |
| Go standard library | build toolchain | go1.25.11+ (crypto/tls, crypto/x509, html/template, net/http, net/url, os, …) |
| `golang.org/x/net` | v0.17.0 | later: v0.23.0 / v0.38.0 / v0.53.0 |
| `github.com/jackc/pgx/v5` | v5.5.5 | v5.9.2 |

## Why the CI gate is advisory (for now)

`govulncheck` (backend) and `npm audit` (frontend) run in CI as **advisory**
(`continue-on-error`) rather than hard gates. Reaching zero is **not a one-line
bump**: pushing `x/net`/`pgx` to their latest releases pulls newer
`golang.org/x/*` modules and, via `go mod tidy`, **raises the `go` directive in
`go.mod`**. That activates stricter `go vet` analyzers (e.g. "non-constant format
string"), which then fail the build on **pre-existing** code
(`internal/llm/policy.go:467`). And the standard-library CVEs are cleared only by
building on a newer Go patch (1.25.11+), which is a coordinated toolchain change.

Rather than ship a half-broken tree, the minimal safe fix was applied (20 → 17)
and the scans kept advisory, with the exact path to zero below.

## Exact next action (to reach zero and make the gate blocking)

1. ~~Minimal fixed versions of `x/net` + `pgx` (via the gorm driver).~~ **Done
   this pass (20 → 17).**
2. For the remaining: bump `x/net`→v0.53.0 and `pgx` (via
   `gorm.io/driver/postgres@latest`); run `go mod tidy`; if it raises the `go`
   directive, fix the newly-flagged vet issues (start with
   `internal/llm/policy.go:467` — `fmt.Errorf("%s", msg)`), then `go vet ./...`
   and `go test ./...`.
3. Build CI on **Go 1.25.11+** to clear the standard-library CVEs, and align the
   `go.mod` toolchain accordingly.
4. Re-run `govulncheck ./...`; confirm the "code is affected" count is 0, then
   flip both CI scans from `continue-on-error: true` to hard gates.

## Current CI posture

- Backend: `go vet` + build + test are **hard gates**; `govulncheck` is advisory.
- Frontend: build + **unit tests (headless Chrome)** are hard gates; `npm audit
  --audit-level=high` is advisory.
