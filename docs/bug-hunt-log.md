# Bug Hunt Log

A running log of defects and risks found while working the repository, with
status. Kept honest: unresolved items stay open rather than being silently
closed.

| # | Severity | Area | Finding | Status |
| --- | --- | --- | --- | --- |
| BH-1 | Low (flaky) | `internal/agentruntime` | `TestHermesAdapterInvokesControlledCli` (and the OpenClaw equivalent) timed out under parallel `go test ./...` load — shells to a controlled CLI with a 5s wall-clock timeout and starves. | **Resolved** — CLI-invocation test timeouts raised 5s → 30s; passes repeatedly under full-suite load. |
| BH-2 | Low (repo hygiene) | repo root | `hai-engine-control.zip` (2.2 MB) committed as a binary artifact; opaque to review, bloats clones. | **Resolved** — removed from the repo (`git rm`); no code/config referenced it. |
| BH-3 | Low (CI) | `.github/workflows/ci.yml` | Go version unpinned (floating `1.21.x`) while local toolchain is 1.25.6. | **Resolved** — pinned to `1.21.13` across all Go jobs. |
| BH-6 | Medium (supply chain) | `backend` deps | `govulncheck` found 20 code-affecting vulnerabilities (`golang.org/x/net`, `pgx`, Go stdlib). | **Partially resolved** — x/net→0.17.0 + pgx→5.5.5 applied (**20 → 17**); remaining 17 are mostly Go-stdlib CVEs needing a toolchain bump. Advisory in CI; path-to-zero in `docs/dependency-vulnerabilities.md`. |
| BH-4 | Info | `internal/router` | Before this work the API had no global security headers and no rate limiting. | Resolved — `securityHeadersMiddleware` + config-gated `rateLimitMiddleware` added (phases 029/018). |
| BH-5 | Medium (tests) | `frontend` | 7 pre-existing specs failed — stale scaffold assertions (`AppComponent` expected "app is running") and missing test providers (`No provider for NzNotificationService`, no `HttpClientTestingModule`, unresolved DI tokens). | **Resolved** — added `HttpClientTestingModule` to service specs, mocked providers + `NO_ERRORS_SCHEMA` for component specs, corrected the AppComponent assertion. Full suite now 20/20 green in headless Chrome (phase 041). |

## How to add an entry

Record: severity, area, a concrete reproduction or observation, and a status
(Open / Resolved / Won't-fix with reason). Never mark Resolved without evidence.
