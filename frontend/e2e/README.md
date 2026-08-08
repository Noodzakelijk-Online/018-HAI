# Browser-level end-to-end tests

This self-contained Playwright suite drives the real HAI UI against a running
stack. Its dependencies do not affect the Angular application build.

## Covered operator chain

`tests/acceptance.spec.ts` proves:

```text
password login
-> owner-scoped read-only local source registration
-> bounded source sync
-> explicit pursuit creation
-> project-matched high-risk workflow intake
-> exact runtime selection
-> durable human approval
-> one exact selected read-only execution
-> terminal completion with deterministic verification evidence
```

The source path is the read-only `connected-sources/` mount. The suite does not
authorize an external provider or request an irreversible operation. Its runtime
target is the backend's real `GET /readyz` endpoint. Missing controls, failed
proposal resolution, an unexpected execution route, non-terminal workflows, or
absent verification evidence fail the test; no acceptance step is optional.

## Run it

```powershell
# Start HAI from the repository root.
docker compose --env-file .env.local -f docker-compose.local.yml up -d --build

# Install and run from frontend/e2e.
npm install
npx playwright install chromium
$env:E2E_BASE_URL = 'http://localhost'
$env:E2E_OPERATOR_EMAIL = 'your-seeded-operator@example.com'
$env:E2E_OPERATOR_PASSWORD = 'your-local-password'
npm test
```

Credentials come from the environment and are never committed. The test is
skipped when `E2E_OPERATOR_PASSWORD` is absent.

## Current evidence

The suite passed against the rebuilt local Windows Compose stack on 2026-08-04
in 10.7 seconds. See `docs/completion-matrix.md` for the exact acceptance scope
and remaining external gates.
