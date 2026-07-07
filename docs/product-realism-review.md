# Product Realism Review

Honest assessment of whether 018-HAI is a real, working product versus a
scaffold — and where the seams are.

## Real and working

- Substantial backend (Go): ~50 packages, ~50 test packages passing, real
  services for memory, workflow, sources, verification, LLM routing, safety.
- Real Angular dashboard with 13 feature pages wired to real endpoints.
- Genuine safety machinery: approval gates, emergency stop, redaction, retry/
  dead-letter.
- Verified from a clean clone: build + vet + test + `doctor` all pass.

## Honest seams

| Area | Reality |
| --- | --- |
| New utility packages | Real and tested, but several not yet wired into the live app (rbac, upload, apierror, autonomygate, actionresolver). |
| Full-stack boot | A scripted critical-path smoke (`scripts/smoke-critical-path.sh`) boots a real local Postgres + backend and asserts health/readiness + the critical path (**ran 7/7**). The full **Docker Compose** multi-service boot (Redis/Kafka/nginx together) is still **not run here** (Docker unavailable). |
| Real external providers | Intentionally disabled pending OAuth/scope review — assisted, not automated. |
| Frontend polish | Accessibility, responsive matrix, onboarding wizard not done. |

## Is it real?

Yes — the critical path is genuinely implemented and exercised, not mocked. It is
a real product with a mature backend and a functional UI, plus a clearly-scoped
set of not-yet-wired capabilities and frontend polish remaining. Crucially, the
gaps are **documented as gaps**, not disguised as done.

## Biggest lever

Wire the tested decision/safety utilities into the execution path and automate a
full-stack smoke — that converts "tested capability" into "demonstrably working
product" for the phases currently held at Partial.
