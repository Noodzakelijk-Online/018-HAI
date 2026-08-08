# Roadmap & Blocked Items

Honest forward view. Nothing here is claimed done; the completion matrix is the
source of truth for current state.

## Near-term hardening

- **Fresh-clone Windows acceptance (phase 032, TD-8):** the maintained Windows
  Compose installation now builds and runs with healthy Postgres, Redis, Kafka,
  nginx, IDP, backend, and frontend services. Repeat the same acceptance from a
  clean clone and empty volumes before calling installation reproducibility
  complete.
- **RBAC — done on the backend (phase 008/TD-9):** IDP-JWT identity→role is wired + runtime-proven. Remaining: IDP emits a `role` claim; broaden `requirePermission` onto more routes.
- **Make the frontend dependency scan blocking (TD-6/BH-6):** the backend is
  clean and blocking. Perform a coordinated migration from the vulnerable
  Angular 16/CDK/ng-zorro family to a supported release, preserve authenticated
  routes and the full UI regression suite, then make the frontend scan a hard
  gate.
- Adopt the `apierror` envelope across handlers in step with the frontend (TD-1).
- **Advisory outcome monitor release acceptance:** retain a disposable-PostgreSQL
  and signed-browser run for all three fixed collectors, exact replay after a
  transient composition failure, two-owner isolation, disabled-target behavior,
  active/expired lease fencing, and the Governance Control lifecycle. Assert
  that the path creates only observations, runs, outcome evaluations,
  proactivity decisions, and inbox records, with zero execution, delivery,
  Calendar, workflow, mandate, provider, or learning effects.

## Frontend-dependent (need Angular work)

- Wire the memory search UI and feature-flag/i18n surfaces into the dashboard (TD-7).
- Deeper accessibility + cross-browser visual passes on the existing pages.

## Larger initiatives

- Move list/search from in-memory to SQL with composite/trigram indexes at scale
  (see `docs/performance-baseline.md`).
- Retained live acceptance runs for each implemented, read-only provider connector.
- Add deployed metrics and alerts for durable outcome-monitor sweep latency,
  due backlog, lease recovery, redacted failures, and composition retries only
  after the local acceptance contract is retained. Do not add effect authority
  to solve an observability gap.

## Blocked items

| Item | Blocker | Next action |
| --- | --- | --- |
| Fresh-clone Windows 11 acceptance | The maintained local stack is proven, but it contains retained volumes and configured local state | Clone into a clean directory, create a new `.env.local`, build empty volumes, and run the documented operator chain |
| Google Drive/Contacts/Calendar live acceptance | Live sandbox credentials and retained evidence; adapters remain unconfigured by default | Run bounded consent, backfill, incremental-change, revoke, and source-link acceptance for each account |
| Paid LLM routing / grounded LLM verification | Paid-budget approval (currently €0); no LLM provider configured | Approve budget / configure a local LLM provider |

Blocked items are blocked by external credentials/approvals or an unavailable
Docker daemon — not by engineering difficulty — and are documented rather than faked.

Blocked items are blocked by external credentials/approvals, not by engineering
difficulty, and are documented rather than faked.
