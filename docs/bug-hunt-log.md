# Bug Hunt Log

A running log of defects and risks found while working the repository, with
status. Kept honest: unresolved items stay open rather than being silently
closed.

| # | Severity | Area | Finding | Status |
| --- | --- | --- | --- | --- |
| BH-1 | Low (flaky) | `internal/agentruntime` | `TestHermesAdapterInvokesControlledCli` times out under parallel `go test ./...` load — it shells to a controlled CLI with a 5s wall-clock timeout and starves. Passes 4/4 in isolation on this branch and on untouched base `0f7f12c`. | Open — recommend raising the timeout or marking the test to not run in parallel (`t.Parallel` budget / `-p 1` for that package). |
| BH-2 | Low (repo hygiene) | repo root | `hai-engine-control.zip` (2.2 MB) committed as a binary artifact; opaque to review, bloats clones. | Open — extract to source or move to release assets / `.gitignore`. |
| BH-3 | Low (CI) | `backend/go.mod` | Declares Go 1.21 while local toolchain is 1.25.6; builds pass but the version is unpinned in CI. | Open — pin the Go version in `.github/workflows/ci.yml`. |
| BH-4 | Info | `internal/router` | Before this work the API had no global security headers and no rate limiting. | Resolved — `securityHeadersMiddleware` + config-gated `rateLimitMiddleware` added (phases 029/018). |
| BH-5 | Medium (tests) | `frontend` | 7 pre-existing specs failed — stale scaffold assertions (`AppComponent` expected "app is running") and missing test providers (`No provider for NzNotificationService`, no `HttpClientTestingModule`, unresolved DI tokens). | **Resolved** — added `HttpClientTestingModule` to service specs, mocked providers + `NO_ERRORS_SCHEMA` for component specs, corrected the AppComponent assertion. Full suite now 20/20 green in headless Chrome (phase 041). |

## How to add an entry

Record: severity, area, a concrete reproduction or observation, and a status
(Open / Resolved / Won't-fix with reason). Never mark Resolved without evidence.
