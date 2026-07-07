# Acceptance Test Matrix

Maps user-visible capabilities to the tests that prove them. "Automated" means a
Go test asserts it; "manual" means a documented step.

| Capability | Acceptance criterion | Coverage |
| --- | --- | --- |
| Remember context | Duplicate memories merge; retrieval ranks relevant ones | Automated — `memory/service_test.go` |
| Search memories | Filter by kind/tag, sort, paginate; bounded | Automated — `memory/query_test.go`, `handler_test.go` |
| Large datasets | Pagination correct at 50k rows | Automated — `memory/largedataset_test.go` |
| Hostile input | No panic; bounded output | Automated — `memory/adversarial_test.go` |
| Project isolation | No cross-project leakage | Automated — `memory/isolation_test.go` |
| Readiness | `/readyz` = 200 ready / 503 not-ready | Automated — `router/readiness_test.go` |
| Config self-check | `doctor` exits non-zero on failure | Automated — `doctor/doctor_test.go` |
| Security headers | Present on responses; Swagger CSP-exempt | Automated — `router/security_headers_test.go` |
| Rate limiting | 429 over limit; off by default | Automated — `router/rate_limit_test.go` |
| Idempotency | Duplicate key → 409; opt-in | Automated — `router/idempotency_test.go` |
| Provider fallback | Free before paid; never paid unless allowed | Automated — `providerfallback` |
| Autonomy gate | Risky/irreversible never auto-runs | Automated — `autonomygate` |
| Upload safety | Traversal/extension/size rejected | Automated — `upload`, `pathsafety` |
| Full-stack boot | Compose stack healthy end-to-end | **Manual/pending** — `docs/fresh-clone-dryrun.md` |
| Accessibility / responsive | WCAG + breakpoints | **Pending** — frontend work |

## Gate

A change is acceptable when: `go vet` clean, `go test ./...` green, `/readyz`
ready against the target config, and any new user-visible capability has a row
here with automated coverage (or a documented manual step if automation isn't
yet feasible).
