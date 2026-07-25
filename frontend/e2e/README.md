# Browser-level end-to-end tests

Self-contained [Playwright](https://playwright.dev) suite that drives the **real
UI against a running stack**. It lives in its own folder with its own
`package.json` so it never affects the Angular app build.

## What it covers

`tests/acceptance.spec.ts` walks the operator acceptance flow the external
review asked to see proven at the browser level:

```
login → source connection → sync → workflow creation → approval → one bounded execution
```

It uses the app's existing `data-testid` hooks (`login-submit`,
`workflow-create`, `workflow-apply-transition`, `workflow-run-worker`,
`panel-activity-audit`).

## Run it

```bash
# 1. Bring up the stack from the repo root
docker compose -f docker-compose.local.yml up --build

# 2. Install and run (from this folder)
cd frontend/e2e
npm install
npm run install-browsers
E2E_BASE_URL=http://localhost:8080 \
E2E_OPERATOR_EMAIL=you@example.com \
E2E_OPERATOR_PASSWORD='...' \
npm test
```

Credentials come from the environment; nothing is committed. If
`E2E_OPERATOR_PASSWORD` is unset the acceptance test is skipped (it needs a
seeded, login-capable account).

## Status (be honest about this)

- **Authored** against real selectors: yes.
- **Executed green in CI/locally:** not yet — it needs a running stack plus a
  seeded operator account, and the source connect/sync steps carry
  `TODO(selector)` markers to confirm two testids on the sources page in the
  target build.
- Tracked as a deferred external gate in `docs/completion-matrix.md`.

Do not report this suite as passing until it has actually run against the target
stack.
