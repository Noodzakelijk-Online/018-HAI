# 018-HAI

018-HAI is a local-first automation hub and Personal AI Operating System in progress. It combines an Angular dashboard, Go backend APIs, an IDP service, nginx gateway routing, Postgres, Redis, and Kafka into one Docker Compose based workspace.

The repository now contains operational control surfaces for automations, LLM routing policy, context memory, connected-source ingestion, task success planning/execution, workflow state handling, source-grounded verification, and an HAI OS overview dashboard. It is not yet a fully autonomous Claw/OpenClaw-style runtime. The current engine can plan, route, gather context, import local files, identify actionable source extractions, create workflow items, generate checklists, gate approvals, verify grounded output, run controlled automation launches, and block unsafe or unsupported results; richer browser/MCP/agent runtime adapters still need to be implemented.

## Current Status

Implemented:

- Angular dashboard with pages for automations, Control Center, HAI OS, LLM Routing, Memory, Task Blueprint, Connected Sources, Grounded Answers, and Workflow Engine.
- Go backend API with automation CRUD, launch metadata, health checks, diagnostics, LLM routing, memory, task, source, workflow, verification, and OS overview routes.
- Local Docker Compose setup for Windows 11 and general Docker Desktop use.
- Local-first LLM routing policy with configurable providers, local/free priority, paid usage disabled by default, selected-model explanation, and routing logs.
- Real local/free LLM generation calls for configured Ollama and OpenAI-compatible endpoints, with paid execution blocked unless explicitly approved by policy.
- Context memory CRUD/retrieve/export with deduplication, similarity merge, relevance scoring, archive/restore/delete, and source references.
- Universal task success engine that classifies requests, defines success criteria, retrieves memory/source context, routes model/tool choices, applies risk gates, produces an execution result, verifies claims, retries/falls back, queues review, logs events, and stores lessons only after verified execution.
- Connected-source registry with manual import, allowlisted local-folder sync, scheduled due-sync worker, sync-job records, extraction, search, provenance, pause/resume/revoke, correction, archive/delete, and audit logs.
- Workflow engine that turns actionable connected-source extractions or manual input into persistent workflow items with state, priority, risk, approval gates, generated checklists, blocked/waiting/completed/archive states, and audit events.
- Source-grounded answer and anti-hallucination layer that decomposes answers into claims, attaches evidence, validates source support, flags unsupported/conflicting claims, gates high-risk output, and records verification runs.
- CI workflow for backend, IDP, nginx config manager, frontend build, and Docker Compose config validation.

Not implemented yet:

- Real Claw-compatible agent runtime adapters for OpenClaw, QwenPaw, AnythingLLM, Khoj, LibreChat, Agent Zero, MCP tools, browser automation, and richer API/tool workflows.
- Full autonomous device control. Automation `launch` now has controlled adapters for API calls, allowlisted container-local scripts, and optionally Docker API container start requests, but broader autonomous desktop/browser/MCP/agent execution is still not implemented.
- Real OAuth connectors, webhook sync, or local folder watchers. The operational connected-source paths today are manual import, allowlisted local-folder scanning, scheduled due-sync for enabled local-folder sources, and workflow intake from extracted action items.
- Real vector embedding infrastructure. Current search and relevance are local deterministic scoring, not a production vector database.
- Production-grade provider quota accounting across restarts. The current router records decisions and blocks paid calls by policy, but durable quota ledgers still need implementation.
- Production-grade secret management, migrations, RBAC hardening, and deployment configuration.

## Repository Layout

```text
.
|-- backend/                 Go backend API and HAI engine modules
|-- frontend/                Angular dashboard
|-- idp/                     Go identity provider service
|-- nginx-config/            Gateway nginx config
|-- nginx-config-manager/    Go service for nginx route config updates
|-- automation-scripts/      Allowlisted scripts mounted read-only into backend
|-- connected-sources/       Allowlisted local files mounted read-only for ingestion
|-- generic-auto/            Placeholder generic automation service
|-- gate/                    Gateway-related legacy/config files
|-- kafka/                   Kafka-related config area
|-- docs/                    Architecture and feature blueprints
|-- .github/workflows/       CI pipeline
|-- docker-compose.local.yml Local-first Windows/Docker Compose setup
|-- docker-compose.yml       Legacy/default compose setup
|-- .env.example             Local environment template
|-- init.sql                 Postgres extension/bootstrap SQL
```

Important note: historical local state and `.env` files exist in this repository. New development should use `.env.example` copied to `.env.local`, and should avoid committing more runtime database files, image uploads, local secrets, or generated output.

## Quick Start

Prerequisites:

- Windows 11 with Docker Desktop, or another Docker Compose capable environment.
- Git.
- Node.js 20 for frontend development outside Docker.
- Go 1.21 for backend development outside Docker.

Local Docker start:

```powershell
copy .env.example .env.local
docker compose --env-file .env.local -f docker-compose.local.yml up --build
```

Open the dashboard:

```text
http://localhost
```

Default first-run local admin from `.env.example`:

```text
Email: noodzakelijkonline@gmail.com
Password: ChangeMe123!
```

Change `FIRST_RUN_ADMIN_PASSWORD` in `.env.local` before first start for a real local install. If the Postgres data folders already exist, changing first-run values will not rewrite the existing account.

Local service ports:

- Gateway/dashboard: `http://localhost`
- IDP Postgres: `localhost:5433`
- Automation Postgres: `localhost:5434`
- Redis: `localhost:6379`

Useful local checks:

```powershell
docker compose --env-file .env.local -f docker-compose.local.yml ps
docker compose --env-file .env.local -f docker-compose.local.yml logs backend
docker compose --env-file .env.local -f docker-compose.local.yml logs idp
docker compose --env-file .env.local -f docker-compose.local.yml logs kafka
docker compose --env-file .env.local -f docker-compose.local.yml config
```

If port 80 is already in use, change the nginx port mapping in `docker-compose.local.yml` from `"80:80"` to another host port, for example `"8088:80"`, then open `http://localhost:8088`.

Local folder ingestion:

1. Place text-like files under `connected-sources/`.
2. In the dashboard, open Connected Sources.
3. Connect a `Selected local folders` source if one does not exist.
4. Use Local Folder Sync with a folder path relative to `connected-sources/`, for example `.`.

The backend mounts this folder read-only at `CONNECTED_SOURCE_LOCAL_ROOT`. Folder paths that escape that root are rejected. Supported file types are `.txt`, `.md`, `.markdown`, `.csv`, `.tsv`, `.json`, `.yaml`, `.yml`, and `.log`.

For recurring local-folder ingestion, set the source `Sync` field to a duration such as `15m`, `1h`, `hourly`, `daily`, or `weekly`. The scheduler checks due sources every `SOURCE_SCHEDULER_INTERVAL_SECONDS` seconds when `SOURCE_SCHEDULER_ENABLED=true`. Only the local-folder adapter is scheduled today; other connector categories stay registered until real adapters are implemented.

## Developer Checks

Backend:

```bash
cd backend
go test ./...
go build ./...
```

Frontend:

```bash
cd frontend
npm ci
npm run build
```

Compose:

```bash
docker compose --env-file .env.example -f docker-compose.local.yml config
```

When Go is not installed locally, the backend can be checked through Docker:

```powershell
docker run --rm -v "${PWD}\backend:/app" -w /app golang:1.21.0 go test ./...
docker compose --env-file .env.example -f docker-compose.local.yml build backend
```

Known warning: the Angular production build currently exceeds the initial bundle budget by roughly 230 KB. The build succeeds, but the budget should be tightened or the bundle split before treating this as production-ready.

## Main API Areas

All backend routes are served under `/api/v1` through the gateway.

Automation:

- `GET /automation/`
- `POST /automation/`
- `PATCH /automation/`
- `DELETE /automation/:id`
- `POST /automation/:id/launch`
- `POST /automation/:id/health-check`
- `GET /automation/health/summary`
- `GET /automation/health-summary`
- `GET /automation/:id/diagnostics`

LLM routing:

- `GET /llm/policy`
- `POST /llm/route`
- `POST /llm/generate`
- `GET /llm/logs`

Memory:

- `GET /memory/`
- `POST /memory/`
- `POST /memory/retrieve`
- `GET /memory/export`
- `GET /memory/:id`
- `PATCH /memory/:id`
- `POST /memory/:id/archive`
- `POST /memory/:id/restore`
- `DELETE /memory/:id`

Task engine:

- `POST /task/plan`
- `POST /task/run`
- `POST /task/success`
- `GET /task/logs`
- `GET /task/review-queue`
- `POST /task/review-queue/:id/resolve`

Workflow engine:

- `GET /workflow/overview`
- `GET /workflow/`
- `POST /workflow/intake`
- `GET /workflow/:id`
- `POST /workflow/:id/transition`
- `PATCH /workflow/:id/checklist/:itemId`

Connected sources:

- `GET /sources/connectors`
- `GET /sources/`
- `POST /sources/`
- `PATCH /sources/:id`
- `POST /sources/:id/sync`
- `POST /sources/sync-due`
- `POST /sources/:id/reindex`
- `POST /sources/:id/pause`
- `POST /sources/:id/resume`
- `POST /sources/:id/revoke`
- `POST /sources/search`
- `GET /sources/extractions`
- `PATCH /sources/extractions/:id`
- `POST /sources/extractions/:id/archive`
- `DELETE /sources/extractions/:id`
- `GET /sources/audit-logs`

Verification:

- `POST /verification/answer`
- `GET /verification/runs`
- `GET /verification/runs/:id`

HAI OS:

- `GET /os/overview`

## Engine Behavior

The task success engine follows a completion-first loop:

1. Classify the request.
2. Infer the real goal.
3. Define success criteria.
4. Refresh due connected sources when the request likely depends on project, file, document, or local context.
5. Retrieve relevant memory and connected-source context.
6. Route a suitable model by capability and cost policy.
7. Route tools and mark unsafe tools as blocked.
8. Apply risk and approval gates.
9. Execute only currently allowed internal steps.
10. Produce a grounded execution result.
11. Verify claims against evidence.
12. Retry or escalate when validation fails.
13. Queue unresolved or risky work for review.
14. Store useful lessons only after verified completion.

The task engine execution step is internal and evidence-grounded. Separate automation launch adapters can call bounded API targets, run a single allowlisted container-local script, or start a Docker container when Docker control is deliberately enabled. The system still does not send emails, change accounts, post publicly, delete files, or broadly control the local machine.

High-risk task requests are added to the review queue. A review item can be approved or rejected from the dashboard or API. Approval re-runs the stored request with an explicit human-approval flag; rejection leaves the task blocked. Approval does not grant unrestricted device power, it only lets the controlled task engine proceed through its internal context, model, verification, and memory workflow.

The task engine now treats connected sources as an active preflight dependency. For requests that mention project/source/file/folder/document/repo context, or that require local/document context, it runs due scheduled source syncs before source search. This uses the same bounded local-folder scheduler path and does not force a full re-read when sources are not due.

## Workflow Engine

The workflow engine implements the 14 operational suggestions as a concrete backend module and dashboard page:

1. Persistent workflow state machine.
2. Event-trigger metadata and audit records.
3. Adapter-facing source and trigger fields.
4. Project/source context links.
5. Approval and autonomy decision rules.
6. Deterministic task/risk/deadline reasoning.
7. Generated execution checklists.
8. Deadline/risk/type priority scoring.
9. Blocked and waiting states for exceptions.
10. Full workflow audit trail.
11. Human approval gates before sensitive transitions.
12. Worker-ready lifecycle states such as `ready` and `in_progress`.
13. Checklist correction/progress feedback events.
14. Safety boundaries for legal, financial, public, destructive, and account-changing actions.

Manual input can be sent to `POST /workflow/intake`. Connected-source sync also sends actionable extractions with tasks or follow-ups into the workflow engine. Intake deduplicates by source URI so repeated syncs do not create duplicate workflow items for the same source record.

Workflow states:

```text
new_input -> classified -> linked -> checklist_generated -> ready -> in_progress -> completed -> archived
```

Sensitive or unclear work branches into:

```text
needs_approval
waiting_external_input
blocked
```

The dashboard page at `/workflow-engine` shows the workflow inbox, capability coverage, generated checklist, validated state transitions, approval toggle, safety rules, and audit trail.

## LLM Routing Policy

The router optimizes for verified completion before cost minimization. The default tier order is:

```text
free -> cheap -> acceptable -> high -> expensive
```

Default policy:

- Paid usage disabled.
- Daily paid budget: `0`.
- Local models allowed.
- Free cloud quota allowed.
- Paid or expensive usage requires manual approval.
- Local/free providers are preferred only when suitable for the task.
- Weak models are skipped when the estimated task difficulty is too high.
- Fallback routing is logged when validation fails.

Configuration:

- `LLM_POLICY_JSON` can replace the policy.
- `LLM_PROVIDERS_JSON` can replace the provider/model list.

The default provider list includes Ollama, LM Studio/OpenAI-compatible local servers, free-cloud placeholders, and paid placeholders. Provider invocation is implemented for:

- Ollama: set `OLLAMA_BASE_URL`, for example `http://host.docker.internal:11434` when Docker needs to reach Ollama on the Windows host.
- LM Studio or llama.cpp OpenAI-compatible servers: set `LM_STUDIO_BASE_URL`, for example `http://host.docker.internal:1234`.
- Free OpenAI-compatible quota providers: set `FREE_CLOUD_OPENAI_BASE_URL` and `FREE_CLOUD_API_KEY`, then enable/configure that provider through `LLM_PROVIDERS_JSON`.

Task execution can use a configured model endpoint to produce a draft, but the draft is still passed through source-grounded verification before the task can be marked complete. If no endpoint is configured or reachable, the engine falls back to evidence-based synthesis and review behavior.

## Memory System

The memory layer is local by default and is intended to stay compact:

- Store stable preferences, project context, decisions, source notes, and lessons.
- Deduplicate repeated content by hash.
- Merge highly similar memories.
- Retrieve only relevant context for a task.
- Rank by relevance, confidence, recency, and project match.
- Keep source labels/URIs where available.
- Allow view, edit, archive, restore, export, and delete from the dashboard.

Avoid storing raw chat logs, entire documents, secrets, or noisy transient state as memory. Store compact facts with provenance.

## Connected Sources

The source layer treats connected accounts and imports as structured context sources. It currently supports:

- Connector registry.
- Source registry.
- Manual import/sync.
- Local folder scanning from the read-only `./connected-sources` mount.
- Scheduled due-sync for enabled local-folder sources with non-manual sync frequencies.
- Raw item metadata.
- Text extraction and summaries.
- Entity/date/task/decision/follow-up fields.
- Searchable extracted records.
- Source provenance links.
- Audit logs.
- Pause, resume, revoke, reindex, correct, archive, and delete actions.

Local folder sync is bounded by:

- `CONNECTED_SOURCE_LOCAL_ROOT`, default `/root/connected-sources`.
- `CONNECTED_SOURCE_FILE_LIMIT`, default `100`, hard-capped at `500`.
- `CONNECTED_SOURCE_MAX_BYTES`, default `1048576`, hard-capped at `10485760`.
- `SOURCE_SCHEDULER_ENABLED`, default `true`.
- `SOURCE_SCHEDULER_INTERVAL_SECONDS`, default `60`, minimum `15`.
- Incremental sync based on `LastSyncedAt`, unless mode is `historical_backfill`.

Supported categories are represented for email, calendars, contacts/cloud documents, project boards, GitHub, local folders, and future connectors. Real account adapters must be added behind the existing service/repository interfaces.

## Verification and Grounded Answers

The verification layer treats LLM-style output as a draft until checked. It:

- Builds answers from retrieved/provided evidence.
- Splits important output into atomic claims.
- Links claims to source references.
- Scores source quality and relevance.
- Marks claims as `verified`, `source_supported`, `test_passed`, `human_approved`, `unsupported`, `uncertain`, `conflicting`, or `needs_review`.
- Blocks high-risk action output without approval.
- Prevents unsupported claims from becoming accepted memory unless explicitly verified or confirmed.

For code tasks, tests or build output should be added as evidence before marking a task complete.

## Automation Control Center

The automation control layer provides:

- Automation CRUD.
- Runtime/launch metadata fields.
- Launch button and `POST /automation/:id/launch`.
- Controlled launch execution for API targets, allowlisted scripts, and optionally Docker API container starts.
- Persisted launch events shown in diagnostics.
- HTTP/TCP/manual/disabled health check modes.
- Persisted health events.
- Health summary.
- Diagnostics panel.
- Last launch/check/success/failure data.
- Average latency and failure reason fields.

Runtime behavior:

- `browser_url`: prepares a target for client-side opening; no server-side device action is performed.
- `api`: sends a bounded GET or POST request to an absolute `http` or `https` target. Prefix the target with `GET ` or `POST ` to select the method; default is POST.
- `script`: executes a single script file without shell expansion. The target must resolve inside `AUTOMATION_SCRIPT_DIR`, mounted from `./automation-scripts` in local Compose.
- `docker_service`: blocked by default. It can send a Docker Engine API start request only when `AUTOMATION_DOCKER_CONTROL_ENABLED=true` and the Docker socket is deliberately mounted/configured.

Current limitation: OpenClaw/QwenPaw/MCP/browser/desktop agent runtimes are still blocked until explicit adapters and approval policies are added.

## Safety Rules for Developers

This project is intended to gain local execution power, so safety should be designed before autonomy:

- Keep read-only behavior as the default.
- Require approval for email sending, legal/government actions, financial actions, account changes, public posting, deletion, destructive file changes, and broad local execution.
- Use allowlists for tools, folders, runtime adapters, and network targets.
- Redact secrets in logs, prompts, memory, and verification evidence.
- Preserve source provenance for extracted facts.
- Never store unsupported claims as facts.
- Treat model confidence as insufficient for action.
- Log model/tool choices, action attempts, validation status, fallback paths, and review decisions.
- Prefer local storage and local models for private Robert/project context.
- Do not silently introduce paid model calls. Paid usage must remain disabled unless deliberately configured and approved.
- Keep runtime adapters behind explicit capability interfaces instead of coupling the platform to one agent project.

## Development Notes

- Backend uses Go 1.21, Gin, Gorm, Postgres, and Sarama/Kafka.
- Frontend uses Angular 16 and ng-zorro-antd.
- IDP and nginx-config-manager are separate Go services.
- Database schema changes currently rely on Gorm `AutoMigrate` plus `init.sql`; a production migration system is still needed.
- Docker Compose local mode uses one Kafka broker plus Zookeeper.
- The local gateway expects backend and IDP routes under `/api`.
- Do not rely on committed `.env` files for new work. Use `.env.example` -> `.env.local`.
- Do not commit generated database directories, uploaded images, frontend `dist`, `node_modules`, local caches, or Docker state.
- Keep UI changes consistent with the existing Angular/ng-zorro dashboard style.
- Keep APIs backward-compatible where dashboard pages already consume them.
- Add focused tests when changing shared behavior such as routing, memory retrieval, verification, task validation, or automation health.

## Documentation

Architecture and feature blueprints live in `docs/`:

- `docs/automation-control-center-blueprint.md`
- `docs/completion-first-context-routing-blueprint.md`
- `docs/universal-task-success-engine.md`
- `docs/connected-source-ingestion-extraction.md`
- `docs/anti-hallucination-verification.md`
- `docs/source-grounded-answer-engine.md`
- `docs/hai-personal-ai-operating-system.md`

These documents describe the target direction. The README describes the current repository state and the constraints developers must preserve while implementing the remaining engine behavior.

## License

See `LICENSE`.
