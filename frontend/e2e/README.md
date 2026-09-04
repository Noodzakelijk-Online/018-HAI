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
-> project-matched low-risk workflow intake
-> exact runtime selection
-> one exact selected read-only execution
-> terminal completion with deterministic verification evidence
```

The source path is the read-only `connected-sources/` mount. The suite does not
authorize an external provider, request an irreversible operation, or present
the health probe as legal work. Its runtime target is the backend's real
`GET /readyz` endpoint. High-risk approval and legal-evidence boundaries are
covered by backend policy tests; this browser suite proves the separate
read-only operator path. Missing controls, failed proposal resolution, an
unexpected execution route, non-terminal workflows, or absent verification
evidence fail the test; no acceptance step is optional.

## Run it

```powershell
# Start HAI from the repository root.
docker compose --env-file .env.local -f docker-compose.local.yml up -d --build

# Install and run from frontend/e2e.
npm install
npm run typecheck
npx playwright install chromium
$env:E2E_BASE_URL = 'http://localhost'
$env:E2E_OPERATOR_EMAIL = 'your-seeded-operator@example.com'
$env:E2E_OPERATOR_PASSWORD = 'your-local-password'
$env:E2E_ALLOW_MUTATION = 'true'
npm test
```

Credentials come from the environment and are never committed. The test creates
temporary local records, so it is skipped unless both `E2E_OPERATOR_PASSWORD`
and `E2E_ALLOW_MUTATION=true` are supplied. Set that flag only for a disposable
acceptance stack, not a personal or shared HAI database.

## Current evidence

The suite passed against the rebuilt local Windows Compose stack on 2026-08-04
in 10.7 seconds. See `docs/completion-matrix.md` for the exact acceptance scope
and remaining external gates.
