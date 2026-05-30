# Automation Hub - Unified Repository

This repository contains the complete automation hub system with all components consolidated into a single repository structure.

## Project Structure

```
automation-hub-unified/
├── backend/                    # Go backend service
├── frontend/                   # Angular frontend application
├── gate/                      # API gateway configuration
├── idp/                       # Identity provider service
├── kafka/                     # Kafka configuration
├── nginx-config-manager/      # Nginx configuration manager
├── docs/                      # Documentation
├── generic-auto/              # Generic automation scripts
├── nginx-config/              # Nginx configurations
├── docker-compose.yml         # Main docker compose file
├── makefile                   # Build automation
└── init.sql                   # Database initialization

```

## Components

### Backend (`/backend`)
Go-based backend service providing the core API functionality.

### Frontend (`/frontend`) 
Angular-based web application providing the user interface.

### Gate (`/gate`)
API gateway configuration for routing and load balancing.

### Identity Provider (`/idp`)
Authentication and authorization service.

### Kafka (`/kafka`)
Message broker configuration for event-driven communication.

### Nginx Config Manager (`/nginx-config-manager`)
Service for managing nginx configurations dynamically.

## Getting Started

1. Clone this repository
2. Run `make` to see available build commands
3. Use `docker-compose up` to start all services
4. Access the application through the configured gateway

## Local Windows 11 Setup

Use the local compose file for a fresh local-first install. It builds services
from this repository where source exists, uses one Kafka broker, and keeps local
operator secrets out of Git.

```powershell
copy .env.example .env.local
docker compose --env-file .env.local -f docker-compose.local.yml up --build
```

Open the dashboard at `http://localhost`.

Default first-run local admin:

- Email: `noodzakelijkonline@gmail.com`
- Password: `ChangeMe123!`

Change `FIRST_RUN_ADMIN_PASSWORD` in `.env.local` before first start for a real
local install. If Postgres data directories already exist, changing the first-run
admin values will not rewrite the existing account; remove the local database
folders only when you intentionally want a clean local database.

The local setup exposes database ports for troubleshooting:

- IDP Postgres: `localhost:5433`
- Automation Postgres: `localhost:5434`
- Redis: `localhost:6379`

Useful checks:

```powershell
docker compose --env-file .env.local -f docker-compose.local.yml ps
docker compose --env-file .env.local -f docker-compose.local.yml logs backend
docker compose --env-file .env.local -f docker-compose.local.yml logs idp
docker compose --env-file .env.local -f docker-compose.local.yml logs kafka
```

Troubleshooting:

- If port 80 is already in use, change the nginx port mapping in
  `docker-compose.local.yml` from `"80:80"` to another host port such as
  `"8088:80"`, then open `http://localhost:8088`.
- If login fails on a reused database, confirm the account exists in the IDP
  database or start from clean local database folders.
- If Kafka takes longer on first boot, wait until `docker compose ps` reports it
  as healthy before retrying the backend or IDP.

## LLM Routing Policy

The backend exposes a local-first LLM routing policy at `/api/v1/llm/policy` and
a route explanation endpoint at `/api/v1/llm/route`. The dashboard page
**LLM Routing** shows enabled and disabled providers, quota, budget, selected
model, selection reason, and fallback history.

Default policy:

- tier order: `free -> cheap -> acceptable -> high -> expensive`
- paid usage disabled
- daily paid budget: `0`
- local models allowed
- free cloud quota allowed
- paid or expensive usage requires manual approval

Providers can be replaced by configuration through `LLM_PROVIDERS_JSON`, and the
whole policy can be replaced through `LLM_POLICY_JSON`.

## Context Memory

The backend exposes a local context memory API at `/api/v1/memory`. The
dashboard page **Memory** lets an operator store, correct, retrieve, archive,
restore, export, and delete compact project memories.

Memory behavior:

- stores useful preferences, project context, decisions, and source notes
- deduplicates exact repeated memories by content hash
- merges highly similar memories into compact records
- retrieves only relevant records for a task or query
- ranks context by relevance, confidence, recency, and project match
- explains which context was used and why
- uses the local automation database by default

## Completion-First Task Blueprint

The backend exposes a task planning API at `/api/v1/task/plan` and recent plan
logs at `/api/v1/task/logs`. The dashboard page **Task Blueprint** shows the
intake classification, success criteria, relevant context, selected model,
validation plan, controlled execution mode, and proposed memory updates for a
task before it is treated as complete.

The implementation blueprint is documented in
`docs/completion-first-context-routing-blueprint.md`. Its operating rule is that
verified completion comes before token, compute, or model-cost minimization.
Resource efficiency is applied only after the task's capability, context,
safety, and validation needs are satisfied.

## Universal Task Success Engine

The task API also exposes `/api/v1/task/run`, `/api/v1/task/success`, and
`/api/v1/task/review-queue`.
The success engine turns each task into a structured loop: understand the
request, infer the real goal, define success criteria, retrieve context, route a
model and tools, check risk, execute only allowed steps, validate the result,
retry or escalate when needed, log the decisions, and store useful lessons.

The dashboard **Task Blueprint** page shows tool choices, blocked tools, risk
gate status, universal task steps, validation result, retry policy, engine
events, review queue items, and lessons proposed or stored in memory. The design
is documented in `docs/universal-task-success-engine.md`.

## Connected Sources

The backend exposes a connected-source registry and extraction API at
`/api/v1/sources`. The dashboard page **Connected Sources** lets an operator
view connector categories, connect local-first sources, manually import items,
sync and extract structured records, search extracted context, inspect source
links, correct uncertain records, archive/delete extracted data, pause/re-index
sources, revoke access, and review audit logs.

Supported source categories include email, calendar, cloud/document, project
board, GitHub, and selected local folders. The first operational path is manual
import plus incremental cursor tracking; real OAuth, webhook, scheduled worker,
and folder watcher adapters can plug into the same source registry and sync-job
tables. The task success engine searches connected-source extractions before
planning so relevant source context can be used without re-reading everything.

The design is documented in
`docs/connected-source-ingestion-extraction.md`.

## Grounded Answers and Verification

The backend exposes `/api/v1/verification/answer` for source-grounded answers
and anti-hallucination checks. The engine turns answers into atomic claims,
links claims to connected-source or provided evidence, scores source quality,
checks whether evidence supports each claim, runs simple deterministic arithmetic
checks, treats test-result evidence as code verification, blocks high-risk
claims without approval, and marks unsupported or conflicting claims for review.

The dashboard page **Grounded Answers** shows the answer mode, sources used and
rejected, source quality score, claim-level verification status, unsupported
claims, missing sources, and previous verification runs. Verified memory updates
are allowed only when explicitly requested, and unsupported claims are never
stored as facts.

The design is documented in `docs/anti-hallucination-verification.md` and
`docs/source-grounded-answer-engine.md`.

## HAI Personal AI OS

The capstone dashboard is available at `/hai-os`, backed by
`GET /api/v1/os/overview`. It summarizes the control, knowledge, memory,
reasoning, execution, verification, governance, and observability planes in one
local-first operating view. It shows review load, automation health, connected
source coverage, memory coverage, model/provider policy, verification activity,
and direct links into each operational surface.

The design is documented in `docs/hai-personal-ai-operating-system.md`.

## Development

Each component maintains its own build configuration and can be developed independently while being part of the unified repository structure.

## Automation Control Center

The Control Center adds operational monitoring on top of the existing automation
list: health status per automation, manual health checks, a health summary, and
a diagnostics view with recent check history.

### What was added

Backend (`/backend`):

- `internal/infra/database.go` — the operational models (health events,
  dependencies, route checks, alerts, incidents, SLOs) are now part of
  `AutoMigrate`, and the `uuid-ossp` extension is ensured on startup.
- `internal/router/routes.go` — three routes registered:
  - `GET  /api/v1/automation/health/summary`
  - `POST /api/v1/automation/:id/health-check`
  - `GET  /api/v1/automation/:id/diagnostics`
- `internal/automation/automation_service.go` — each health check now persists
  an `AutomationHealthEvent` (history) and the diagnostics response returns the
  last 10 events plus the last checked/success/failure timestamps. The health
  summary now reports `total`, `healthy`, `warning`, `degraded`, `broken` and
  `unknown` counts.
- `internal/automation/automation_repository.go` — `SaveHealthEvent` and
  `FindHealthEvents` for persisting and reading health history.

Frontend (`/frontend`):

- `services/automations/automations.service.ts` — `getHealthSummary`,
  `runHealthCheck` and `getDiagnostics` methods.
- `pages/control-center/` — the Control Center page (summary bar, automation
  table with status badges, last checked/success/failure, failure reason, a
  manual **Run Check** button, an **Open** link for browser-safe URLs, and a
  **Diagnostics** modal with configuration checks and recent health events).
- A **Control Center** entry was added to the home menu, routed at
  `/control-center`.

### Health check behaviour

- `http` (default): GETs the health check URL (or `http://host:port`) and
  compares against the expected status code.
- `tcp`: dials `host:port`.
- `manual` / `disabled`: reported as `unknown`, no automatic probe.

Repeated failures escalate the status: `warning` → `degraded` → `broken`.

### Safety

No arbitrary command execution is performed. Health checks are HTTP/TCP probes
only, and opening a target uses a browser `window.open` with `noopener`. Local
runner / service control remains out of scope (see the blueprint, step 10).

### How to test

Backend compiles:

```bash
cd backend
go build ./...
```

Frontend compiles:

```bash
cd frontend
npm install
npm run build
```

Full stack (optional, end-to-end):

```bash
docker-compose up --build
```

Then open the gateway, log in, and choose **Control Center** from the home
menu. Use **Run Check** on a row to probe an automation; the status badge,
timestamps and summary bar update, and **Diagnostics** shows the recorded
history.

API smoke test (with the backend running):

```bash
# health summary
curl http://localhost/api/v1/automation/health/summary

# run a health check for one automation
curl -X POST http://localhost/api/v1/automation/<automation-id>/health-check

# diagnostics for one automation
curl http://localhost/api/v1/automation/<automation-id>/diagnostics
```

## License

See LICENSE file for details.

