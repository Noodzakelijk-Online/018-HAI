# Roadmap & Blocked Items

Honest forward view. Nothing here is claimed done; the completion matrix is the
source of truth for current state.

## Near-term (closes the 1 remaining Partial + hardening)

- **Full Docker Compose runtime verification (phase 032, TD-8):** run `docker compose up` where Docker is available and assert `/readyz` green across Postgres/Redis/Kafka/nginx/backend/frontend. *(The only remaining Partial.)*
- **RBAC — done on the backend (phase 008/TD-9):** IDP-JWT identity→role is wired + runtime-proven. Remaining: IDP emits a `role` claim; broaden `requirePermission` onto more routes.
- **Make dependency scans blocking (TD-6/BH-6):** x/net + pgx already bumped (20→17); finish with a Go 1.25.11+ toolchain bump + later dep jumps, then flip the scans to hard gates.
- Adopt the `apierror` envelope across handlers in step with the frontend (TD-1).

## Frontend-dependent (need Angular work)

- Wire the memory search UI and feature-flag/i18n surfaces into the dashboard (TD-7).
- Deeper accessibility + cross-browser visual passes on the existing pages.

## Larger initiatives

- Move list/search from in-memory to SQL with composite/trigram indexes at scale
  (see `docs/performance-baseline.md`).
- Real provider connectors behind reviewed, minimal OAuth scopes.

## Blocked items

| Item | Blocker | Next action |
| --- | --- | --- |
| Full Docker Compose runtime boot | Docker daemon unavailable in this environment | Run the compose stack where Docker is available (TD-8) |
| Real Gmail/Drive/Calendar connectors | Live OAuth credentials + scope review (intentionally disabled) | Provide reviewed, minimal scopes |
| Paid LLM routing / grounded LLM verification | Paid-budget approval (currently €0); no LLM provider configured | Approve budget / configure a local LLM provider |

Blocked items are blocked by external credentials/approvals or an unavailable
Docker daemon — not by engineering difficulty — and are documented rather than faked.

Blocked items are blocked by external credentials/approvals, not by engineering
difficulty, and are documented rather than faked.
