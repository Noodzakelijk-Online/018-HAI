# Technical Debt Register

Tracked debt with severity and a clear "done when". Complements the automated
engineering-action-register and the bug-hunt log.

| ID | Severity | Debt | Done when |
| --- | --- | --- | --- |
| TD-1 | Medium | Partial utility adoption: `apierror` is now used in the RBAC 403 (live); file safety is live-enforced in the real upload path (`resolveImagePath`), but `pathsafety`/`upload` packages aren't the enforcer at every call site, and `apierror` isn't the envelope on every handler (existing handlers use `{"error":"..."}`, which the frontend depends on). | Handlers migrate to `respondError` in step with the frontend; file I/O routes through the shared `pathsafety`/`upload` helpers. |
| TD-2 | Medium | List/search runs in memory; fine to tens of thousands of rows but not beyond. | Search/list backed by SQL with composite + trigram indexes (see performance-baseline). |
| TD-3 | Low | ~~Go toolchain unpinned in CI.~~ | **Resolved** — CI pinned to `go 1.21.13`. |
| TD-4 | Low | ~~`hai-engine-control.zip` (2.2 MB) committed as a binary.~~ | **Resolved** — removed from the repo (no code/config referenced it). |
| TD-5 | Low | ~~`agentruntime` CLI test flaky under parallel load (5s timeout).~~ | **Resolved** — timeouts raised to 30s; passes repeatedly under load. |
| TD-6 | Medium | Dependency vulnerability scanning is **advisory**, not a blocking gate. `govulncheck` found 20 code-affecting vulns. | Blocking gate after the coordinated dependency+toolchain+vet upgrade in `docs/dependency-vulnerabilities.md`. |
| TD-7 | Info | i18n catalog and feature flags are backend-only; not yet surfaced in the Angular UI. | Dashboard consumes `/flags` and i18n messages. |
| TD-8 | Medium | Full Docker Compose multi-service boot (Postgres+Redis+Kafka+nginx together) not verified — Docker unavailable in this environment. Backend does not depend on Redis (idp does); Kafka degrades to a no-op. | `docker compose up` reaches `/readyz` ready across all services where Docker is available. |
| TD-9 | Medium | RBAC is enforced on admin routes but not per-user across user-facing routes — backend uses a shared API key (no per-request identity). | IDP identity (JWT/session) is mapped to an `rbac.Role` in request context and enforced on ownership-sensitive routes. |

## Rules

- Every new debt entry names a concrete "done when", not a vague intention.
- Debt is paid down or explicitly re-prioritized each maintenance cycle; nothing
  is silently dropped.
