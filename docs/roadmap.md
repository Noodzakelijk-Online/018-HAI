# Roadmap & Blocked Items

Honest forward view. Nothing here is claimed done; the completion matrix is the
source of truth for current state.

## Near-term (unblocks sign-off phases)

- Adopt `apierror` envelope across all handlers (finish phase 009 adoption).
- Enforce `rbac.Can` in route middleware (phase 106 enforcement).
- Route every file call site through `pathsafety`/`upload` (phase 015/047 adoption).
- Automate the full compose smoke so 003/031/092 gain runtime evidence.

## Frontend-dependent (need Angular work)

- Accessibility review (049), responsive/browser matrix (050).
- Onboarding / first-run wizard (105).
- Wire memory search UI and feature-flag/i18n surfaces into the dashboard.

## Larger initiatives

- Move list/search from in-memory to SQL with composite/trigram indexes at scale
  (see `docs/performance-baseline.md`).
- Real provider connectors behind reviewed, minimal OAuth scopes.

## Blocked items

| Item | Blocker |
| --- | --- |
| Real Gmail/Drive/Calendar connectors | Require live OAuth credentials + scope review (intentionally disabled). |
| Paid LLM routing | Requires explicit paid-budget approval (currently €0). |

Blocked items are blocked by external credentials/approvals, not by engineering
difficulty, and are documented rather than faked.
