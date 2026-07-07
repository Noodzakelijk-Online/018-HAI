# Technical Debt Register

Tracked debt with severity and a clear "done when". Complements the automated
engineering-action-register and the bug-hunt log.

| ID | Severity | Debt | Done when |
| --- | --- | --- | --- |
| TD-1 | Medium | New utilities (`apierror`, `rbac`, `pathsafety`, `upload`) exist and are tested but not yet adopted at every call site. | Handlers use `respondError`; routes enforce `rbac.Can`; all file I/O routes through `pathsafety`/`upload`. |
| TD-2 | Medium | List/search runs in memory; fine to tens of thousands of rows but not beyond. | Search/list backed by SQL with composite + trigram indexes (see performance-baseline). |
| TD-3 | Low | Go toolchain unpinned in CI (go.mod 1.21 vs local 1.25.6). | CI pins the Go version. |
| TD-4 | Low | `hai-engine-control.zip` (2.2 MB) committed as a binary. | Extracted to source / moved to release assets. |
| TD-5 | Low | `agentruntime` CLI test flaky under parallel load (5s timeout). | Timeout raised or package run isolated (`-p 1`). |
| TD-6 | Low | No dependency vulnerability scanning in CI. | `govulncheck` + `npm audit` gates added. |
| TD-7 | Info | i18n catalog and feature flags are backend-only; not yet surfaced in the Angular UI. | Dashboard consumes `/flags` and i18n messages. |

## Rules

- Every new debt entry names a concrete "done when", not a vague intention.
- Debt is paid down or explicitly re-prioritized each maintenance cycle; nothing
  is silently dropped.
