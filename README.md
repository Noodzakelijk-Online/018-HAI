# 018-HAI

018-HAI is a local-first automation hub and Personal AI Operating System in progress. It combines an Angular dashboard, Go backend APIs, an IDP service, nginx gateway routing, Postgres, Redis, and Kafka into one Docker Compose based workspace.

The repository now contains operational control surfaces for automations, LLM routing policy, context memory, connected-source ingestion, task success planning/execution, workflow state handling, source-grounded verification, and an HAI OS overview dashboard. It is not yet a fully autonomous Claw/OpenClaw-style runtime. The current engine can plan, route, gather context, import local files, identify actionable source extractions, create workflow items, generate checklists, gate approvals, verify grounded output, run controlled automation launches, and block unsafe or unsupported results; richer browser/MCP/agent runtime adapters still need to be implemented.

## Canonical Stack Decision

The canonical product stack is this Codex-built Go backend, Angular dashboard, Postgres persistence, and Docker Compose local runtime. The Manus React/tRPC/MySQL implementation should be treated as reference material only. Useful Manus behavior should be ported into this stack deliberately, not developed as a parallel product.

This decision is captured in [ADR 0001](docs/architecture-decision-records/0001-canonical-stack-and-readiness.md). The dashboard HAI OS page also exposes real-world readiness gates so internal AI logic is not mistaken for proven external integration behavior.

## Current Status

Implemented:

- Angular dashboard with pages for automations, Control Center, HAI OS, LLM Routing, Memory, Task Blueprint, Connected Sources, Grounded Answers, and Workflow Engine.
- Go backend API with automation CRUD, launch metadata, health checks, diagnostics, LLM routing, memory, task, source, workflow, verification, and OS overview routes.
- Local Docker Compose setup for Windows 11 and general Docker Desktop use.
- Local-first LLM routing policy with configurable providers, local/free priority, paid usage disabled by default, provider readiness checks, selected-model explanation, and routing logs.
- Real local/free LLM generation calls for configured Ollama and OpenAI-compatible endpoints. Unconfigured providers are skipped, unsafe link-local endpoints are blocked by default, and paid execution is blocked unless explicitly approved by policy.
- Guarded Hermes and Odysseus agent runtime adapters. HAI can probe both runtimes, show configuration/readiness in the Command Dashboard, and execute approved tasks through the existing task/workflow engine. Runtime execution is disabled by default and remains subject to approval, emergency stop, allowlists, bounded output, timeouts, audit logs, and downstream verification.
- Context memory CRUD/retrieve/export with deduplication, similarity merge, relevance scoring, archive/restore/delete, and source references.
- Universal task success engine that classifies requests, defines success criteria, retrieves memory/source context, routes model/tool choices, applies risk gates, produces an execution result, verifies claims, retries/falls back, queues review, logs events, and stores lessons only after verified execution.
- Connected-source registry with manual import, allowlisted local-folder sync, scheduled due-sync worker, sync-job records, extraction, search, provenance, pause/resume/revoke, correction, archive/delete, connector readiness status, and audit logs.
- Workflow engine that turns actionable connected-source extractions or manual input into persistent workflow items with state, priority, risk, approval gates, generated checklists, source links, decision records, transition records, durable retry limits, task-engine worker execution, verification-gated completion, and audit events.
- Guarded workflow scheduler that periodically runs due workflow items and due open-loop follow-ups through the existing approval-gated workflow/task engines.
- Shared backend engine instances for LLM routing, task execution, source retrieval, memory, and verification within the running API process, so workflow-worker and dashboard-initiated task decisions appear in the same in-memory task/LLM logs.
- Source-grounded answer and anti-hallucination layer that decomposes answers into claims, attaches evidence, validates source support, flags unsupported/conflicting claims, gates high-risk output, and records verification runs.
- Backend API shared-key gate for local gateway traffic. When `BACKEND_API_SHARED_KEY` is set, `/api/v1` backend routes require `X-HAI-Backend-Key`; the checked-in local nginx config injects that header after IDP auth.
- CI workflow for backend, IDP, nginx config manager, frontend build, and Docker Compose config validation.
- Nginx config-manager hardening: Kafka automation events are revalidated before config writes, config file paths are constrained to the configured directory, public routes use the generated URL path instead of the upstream host, and Docker-socket reload is disabled by default.

Not implemented yet:

- Additional Claw-compatible task execution adapters for OpenClaw, QwenPaw, AnythingLLM, Khoj, LibreChat, Agent Zero, generic MCP tools, browser automation, and richer API/tool workflows. Hermes and Odysseus now have controlled first-party adapters, but must still be installed/configured separately.
- Full autonomous device control. Automation `launch` now has guarded adapters for host-allowlisted API calls, explicitly enabled allowlisted container-local scripts, and optionally Docker API container start requests for allowlisted containers, but broader autonomous desktop/browser/MCP/agent execution is still not implemented.
- Real OAuth connectors, webhook sync, or local folder watchers. The operational connected-source paths today are manual import, allowlisted local-folder scanning, scheduled due-sync for enabled local-folder sources, and workflow intake from extracted action items. Email, calendar, cloud/document, project-board, and GitHub connectors are registered as disabled `not_implemented` adapter contracts until real adapters are added.
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

Change `BACKEND_API_SHARED_KEY` before a real local install and update the matching `X-HAI-Backend-Key` value in `nginx-config/nginx.conf`. The backend enforces this key only when it is non-empty; the gateway also still requires IDP authentication before proxying backend routes.

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

All backend routes are served under `/api/v1` through the gateway. The local nginx config only proxies the exact backend namespaces, or paths below them, to the backend; unknown `/api/v1/...` paths fall through to the IDP route instead of being broadly forwarded.

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
- `GET /llm/probes`
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
- `GET /workflow/approvals`
- `GET /workflow/dashboard`
- `GET /workflow/`
- `POST /workflow/intake`
- `POST /workflow/run-due`
- `POST /workflow/open-loops/run-due`
- `GET /workflow/:id`
- `POST /workflow/:id/transition`
- `POST /workflow/:id/approval`
- `POST /workflow/:id/proposals/:proposalId/resolve`
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
9. Execute only currently allowed internal steps or an explicitly selected controlled automation.
10. Capture deterministic runtime output as execution evidence.
11. Produce and verify a grounded execution result.
12. Retry or escalate when validation fails.
13. Queue unresolved or risky work for review.
14. Store useful lessons only after verified completion.

Tasks that require tools or local execution can no longer complete from generated text alone. Action-oriented execution requests must provide an `automationId` that identifies a registered automation; analysis-only requests that merely discuss an API, test strategy, or architecture do not trigger runtime execution. The task engine calls the shared controlled automation service, captures its bounded API/script/Docker launch result, adds that deterministic result to verification evidence, and requires a `completed` runtime status before completion. Missing, blocked, failed, or unconfigured runtime execution goes to review without blind retries. If answer verification needs a stronger model after a successful launch, the retry reuses the captured runtime evidence instead of executing the external action again.

The automation launch adapters can call bounded API targets, run a single allowlisted container-local script, or start a Docker container when Docker control is deliberately enabled. The system still does not send emails, change accounts, post publicly, delete files, or broadly control the local machine unless a separately reviewed adapter and approval policy is implemented.

High-risk task requests are added to the review queue. A review item can be approved or rejected from the dashboard or API. Approval re-runs the stored request with an explicit human-approval flag; rejection leaves the task blocked. An approved review item is marked completed only when the rerun is actually validated; unresolved runtime or verification blockers reuse the original queue item as actionable `needs_review` work instead of creating duplicate reviews. Approval does not grant unrestricted device power, it only lets the controlled task engine proceed through its configured automation, context, model, verification, and memory workflow.

The task engine now treats connected sources as an active preflight dependency. For requests that mention project/source/file/folder/document/repo context, or that require local/document context, it runs due scheduled source syncs before source search. This uses the same bounded local-folder scheduler path and does not force a full re-read when sources are not due.

## Workflow Engine

The workflow engine now implements the first real operational slice of the personal operations engine. It is not a fully autonomous agent, but it does more than display workflow screens:

1. Normalizes intake from manual input and connected-source events.
2. Classifies task type, risk, priority, confidence, and autonomy level.
3. Matches likely project/case context and stores match evidence.
4. Creates persistent workflow items with a state machine.
5. Generates task-type checklists and deadline reminder steps.
6. Stores separate transition, decision, source-link, intake, evidence-claim, open-loop, proposal, quality-gate, and rulebook records.
7. Creates approval gates for legal, government, insurance, lawyer, financial, public-posting, destructive, and account-changing work.
8. Creates open loops for waiting/blocked/approval items with follow-up dates.
9. Creates proposal options so Robert can approve, reject, change tone, request evidence, or block a workflow.
10. Creates software quality gates for developer/GitHub work such as commits, tests, README/setup, and Windows 11 run path.
11. Runs low-risk or approved ready workflows through the task engine worker.
12. Uses durable retry counters, backoff, and blocked-after-limit behavior.
13. Runs due open loops into follow-up proposals, checklist steps, and approval gates.
14. Resolves proposal decisions into workflow state changes.
15. Evaluates quality gates before verified completion.
16. Surfaces operational monitoring for approvals, blocked work, ready work, high-risk items, due open loops, missing next actions, and safety rules.

Manual input can be sent to `POST /workflow/intake`. Connected-source sync also sends actionable extractions with tasks or follow-ups into the workflow engine. Source-derived intake deduplicates first by stable source type plus extraction identity, then falls back to source URI for older/manual callers. Each source workflow stores a deterministic revision hash over its executable content, provenance, project, and review requirements. An unchanged revision reuses the active workflow; a changed revision archives the prior workflow and builds a fresh checklist, evidence set, quality gates, and approval state. This prevents corrected source data from inheriting stale instructions or a human approval granted to an earlier version. In-progress workflows cannot be superseded until their execution outcome is reviewed. Separate records may therefore share a mailbox, sender, document, or board URI without collapsing into one workflow. Uncertain or sensitive extractions are forced into `needs_approval` rather than entering the autonomous ready queue.

Raw connected-source content and connector metadata are stored separately. Reindexing uses the cached content while preserving metadata, and keyword/vector index entries are updated idempotently instead of duplicated. Correcting an extraction reindexes it and reconciles its workflow candidate. Archiving, deleting, or removing all actionable tasks from an extraction retracts the pending source-derived workflow into a blocked review state; an in-progress workflow must first use interrupted-execution review so source deletion cannot hide a possibly executed action.

Source sync completion is all-or-retry at the cursor boundary. Item persistence, extraction, index, or required workflow-intake failures are recorded in `itemsFailed` with bounded error details. A partially successful job is stored as `partial_failure`, scheduled sync reports it as failed, and `LastSyncedAt` plus the cursor remain unchanged so the next incremental run retries the missing work. Concurrent sync requests for the same source are rejected within the local process instead of racing and duplicating autonomous workflow candidates.

## HAI Memory Engine and Command Dashboard

The canonical Go/Angular application now includes a private AI-conversation memory path:

- `POST /api/v1/memory-engine/import` accepts a user-authorized capture from ChatGPT, Gemini, Copilot, or DeepSeek.
- Raw conversation payloads are encrypted with AES-GCM before PostgreSQL storage. Set `HAI_MEMORY_ENCRYPTION_KEY`; local Compose falls back to the backend shared key only when the dedicated key is empty.
- Imports are deduplicated by platform, thread identity, and content hash. Changed threads create a new archive revision.
- Indexed operational facts are secret-redacted and separated into decisions, actions, risks, rules, and contradictions.
- Stable, verified facts feed the existing context-memory retrieval layer.
- Extracted actions feed the existing workflow engine and retain source links. Risky, uncertain, or Robert-owned actions require review.
- `/command-dashboard` shows Needs Robert, VA-ready work, open loops, contradictions, project status, recent decisions, search results, and encrypted archive metadata.
- The raw archive can be inspected or deleted through authenticated API routes. Deleting an archive also deletes its extracted operational facts.

The private Chrome/Edge extension lives in `browser-extension/`. Load it as an unpacked extension, keep the default local endpoint `http://127.0.0.1:7070/api/v1/memory-engine/import`, enter `BACKEND_API_SHARED_KEY`, open one of Robert's own AI conversation pages, and click **Capture current conversation**. The extension reads only the currently open thread after that explicit click. It does not read cookies, passwords, local storage, hidden account data, or unrelated pages, and it sends requests with `credentials: omit`.

Browser DOM selectors can change when providers redesign their chat pages. A failed capture is reported rather than silently treated as complete. Account-wide historical backfill should use official exports where available; automatic sidebar traversal is intentionally not enabled because it is brittle and can trigger platform limits.

The workflow layer now stores operational history in separate tables:

- `WorkflowTransition` records every state change.
- `WorkflowSourceLink` records source provenance separately from the workflow item.
- `WorkflowDecision` records classification, priority, approval, deadline, retry, and completion decisions.
- `WorkflowChecklistItem` supports detected due dates and check reminders.
- `WorkflowIntakeRecord` stores normalized source/input metadata.
- `WorkflowProjectMatch` stores likely project/card/folder links.
- `WorkflowEvidenceClaim` stores source-linked factual claims and review flags.
- `WorkflowOpenLoop` stores waiting states and follow-up actions.
- `WorkflowProposal` stores recommended actions and approval options.
- `WorkflowQualityGate` stores acceptance checks for developer, legal, publishing, and verification workflows.
- `WorkflowRule` stores the default editable safety/workflow rulebook.

Approved or low-risk ready items can be consumed by `POST /workflow/run-due`. Each worker must first atomically claim an item by moving it from `ready` to `in_progress` with a unique claim ID and renewable lease; concurrent scheduler or API workers therefore cannot execute the same item from the same queue state or persist results owned by another worker. Workflows can store an `automationId`, which is passed through the workflow worker into the task engine for controlled execution. The worker completes the item only when the runtime action, task verification, and mandatory quality gates pass. Transient engine failures use durable retry counters and backoff, while explicit `review_required` results block immediately rather than repeating a potentially non-idempotent action. High-risk items remain in `needs_approval` until approved from `/workflow/approvals`, a resolved proposal, or the dashboard approval queue.

Due open loops can be processed with `POST /workflow/open-loops/run-due`. Due follow-ups are also atomically leased before creating checklist or proposal records, preventing duplicate follow-up artifacts when multiple workers poll together. Follow-up checklist and proposal creation is idempotent, so a partial failure can release or recover the lease and retry without duplicating records. High-risk or Robert-owned follow-ups are moved into approval review; low-risk unblocked follow-ups can be made worker-ready. Already blocked workflows stay blocked and keep their blocker while receiving a follow-up proposal. Proposal decisions can be resolved through `POST /workflow/:id/proposals/:proposalId/resolve`, which records the decision and can approve, reject/block, or send the workflow back for changes. Completed or archived workflows reject further proposal changes.

The scheduler runs stale-claim recovery before each execution pass. Expired workflow execution leases are moved to `blocked` review with a durable `recoveryStatus=needs_review` because external side effects may already have occurred and blind retrying could duplicate them. Expired open-loop leases are safely reopened because that path is idempotent. Pre-lease `in_progress`/`processing` rows are migrated through the same policy after one lease window. Operators can also run this explicitly with `POST /workflow/recover-stale` or the **Recover Stale** dashboard action.

Interrupted executions must be resolved through `POST /workflow/:id/interruption/resolve`. The operator can confirm that no side effects occurred and grant one additional controlled retry, keep the item blocked with a review note, or confirm completion with linked evidence. High-risk retries return to fresh approval review. Evidence-backed completion records a source link, evidence claim, decision, transition, audit event, and passed verification gate. Generic transitions, proposal decisions, and approval actions cannot bypass an unresolved interruption, and generic state changes cannot mark work complete outside the verification engine.

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

The dashboard page at `/workflow-engine` shows the workflow inbox, operational monitor, expired claim and interrupted-review counts, due open loops, approval queue with approve/reject buttons, worker, follow-up, and stale-claim recovery controls, structured interrupted-execution resolution, retry status, verification status, generated checklist, intake records, project matches, evidence claims, proposal decision buttons, quality gate status, source links, decisions, validated transitions, safety rules, default rulebook, and audit trail.

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

The default provider list includes Ollama, LM Studio/OpenAI-compatible local servers, Odysseus workspace probing, free-cloud placeholders, and paid placeholders. Provider invocation is implemented for:

- Ollama: set `OLLAMA_BASE_URL`, for example `http://host.docker.internal:11434` when Docker needs to reach Ollama on the Windows host.
- LM Studio or llama.cpp OpenAI-compatible servers: set `LM_STUDIO_BASE_URL`, for example `http://host.docker.internal:1234`.
- Odysseus as an LLM provider remains probe-only. Approved tool-capable execution uses the separate controlled agent-runtime adapter described below, rather than bypassing the task engine through `/llm/generate`.
- Free OpenAI-compatible quota providers: set `FREE_CLOUD_OPENAI_BASE_URL` and `FREE_CLOUD_API_KEY`, then enable/configure that provider through `LLM_PROVIDERS_JSON`.

Task execution can use a configured model endpoint to produce a draft, but the draft is still passed through source-grounded verification before the task can be marked complete. If no endpoint is configured or reachable, the engine falls back to evidence-based synthesis and review behavior.

Provider readiness is explicit. A provider must be enabled, have an absolute `http` or `https` endpoint, pass the link-local/metadata endpoint guard, and provide any required API key environment variable before it can be selected. Provider calls and provider probes do not follow redirects. `GET /api/v1/llm/probes` performs a live, bounded readiness check against configured local/free providers (`/api/tags` for Ollama, `/v1/models` for OpenAI-compatible endpoints, and health/UI paths for Odysseus) so configuration can be separated from real endpoint availability. Client requests to `/llm/generate` cannot approve paid or approval-required model use by setting request JSON; paid approval must be implemented as a server-side approval workflow before paid generation is allowed. The dashboard shows configured, disabled, blocked, missing-key, auth-required, and live-probe states so placeholder providers are not mistaken for live integrations. The HAI OS readiness gate counts only configured providers with at least one enabled no-approval executable model that is allowed by local/free quota policy; Odysseus and other approval-gated/runtime-only connectors are reported separately.

Odysseus upstream reference: [pewdiepie-archdaemon/odysseus](https://github.com/pewdiepie-archdaemon/odysseus). The upstream project describes a self-hosted AI workspace with local models, agents, MCP, local tools, browser use, files, memory, email, and calendar features. HAI invokes its `/api/chat_stream` agent mode only through an approved automation with `allow_bash=false`, web search disabled, a preselected session, a host allowlist, and a bounded timeout/output capture.

Hermes upstream reference: [NousResearch/hermes-agent](https://github.com/NousResearch/hermes-agent). HAI uses Hermes' documented noninteractive `hermes chat -q ... -Q` path. The adapter never adds `--yolo`, enables filesystem checkpoints, passes arguments without shell interpolation, runs in a dedicated configured workspace, inherits only allowlisted environment variables, and stops the process when the configured timeout expires.

### Controlled Hermes and Odysseus runtimes

1. Install and configure Hermes or Odysseus separately. HAI does not download or auto-update third-party agent software.
2. Create an HAI automation with `launchType=agent_runtime` and `runtimeType=hermes` or `runtimeType=odysseus`.
3. Supply that automation ID to a task/workflow request. Direct automation launch calls cannot forge approval and therefore cannot execute agent tasks.
4. Approve the resulting review item. The server then replays the stored task with a server-side approval record.
5. HAI captures the runtime output as evidence and still requires verification before the task can be marked complete.

Hermes configuration:

- `HERMES_AGENT_ENABLED=true`
- `HERMES_EXECUTABLE=hermes` or an explicit installed executable path
- `AGENT_RUNTIME_WORKSPACE_ROOT=<parent folder dedicated to agent workspaces>`
- `HERMES_WORKSPACE=<dedicated allowlisted workspace>`
- optional `HERMES_TOOLSETS`, `HERMES_ENV_ALLOWLIST`, `HERMES_MAX_TURNS`, and `HERMES_TIMEOUT_SECONDS`

Odysseus configuration:

- `ODYSSEUS_AGENT_ENABLED=true`
- `ODYSSEUS_BASE_URL=http://host.docker.internal:7000` for a default Windows-host installation
- `ODYSSEUS_API_TOKEN=<scoped token>`
- `ODYSSEUS_AGENT_SESSION_ID=<existing configured session>`
- optional `ODYSSEUS_AGENT_WORKSPACE` and `ODYSSEUS_AGENT_TIMEOUT_SECONDS`

Use a narrowly scoped Odysseus token. Read-only scopes are the correct default. Email sending, calendar writes, document deletion, host control, public posting, and other consequential operations must remain unavailable unless the matching HAI workflow was explicitly approved.

Controlled runtime outputs and LLM provider error bodies are redacted for common secrets before they are stored or returned in operational logs. Script execution still remains disabled by default and runs with a minimal environment when enabled.

Emergency stop:

- Set `HAI_EMERGENCY_STOP=true` to block LLM generation, automation launches, task execution, workflow workers, and follow-up workers.
- Optional: set `HAI_EMERGENCY_STOP_REASON` to show a redacted operator reason in the HAI OS overview.
- Planning, policy inspection, dashboards, and review queues remain visible so blocked work can still be inspected.

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

Supported categories are represented for email, calendars, contacts/cloud documents, project boards, GitHub, local folders, and future connectors. Only `local-folder` is currently operational. Real account adapters must be added behind the existing service/repository interfaces before those connectors can be enabled or scheduled.

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
- Guarded launch execution for host-allowlisted API targets, explicitly enabled allowlisted scripts, and optionally Docker API starts for allowlisted containers.
- Persisted launch events shown in diagnostics.
- HTTP/TCP/manual/disabled health check modes.
- Persisted health events.
- Health summary.
- Diagnostics panel.
- Last launch/check/success/failure data.
- Average latency and failure reason fields.

Runtime behavior:

- `browser_url`: prepares a target for client-side opening; no server-side device action is performed.
- `api`: sends a bounded GET or POST request to an absolute `http` or `https` target. Prefix the target with `GET ` or `POST ` to select the method; default is POST. The host must be present in `AUTOMATION_API_ALLOWED_HOSTS`. Link-local, metadata, and unspecified IP targets are blocked by default. Redirects are not followed, so an allowlisted target cannot bounce the backend into an unreviewed destination.
- `script`: blocked by default. It executes a single script file without shell expansion only when `AUTOMATION_SCRIPT_EXECUTION_ENABLED=true`. The target must resolve inside `AUTOMATION_SCRIPT_DIR`, including after symlink resolution. Scripts run from their own folder with a minimal environment; add specific variables to `AUTOMATION_SCRIPT_ENV_ALLOWLIST` only when a reviewed script needs them.
- `docker_service`: blocked by default. It can send a Docker Engine API start request only when `AUTOMATION_DOCKER_CONTROL_ENABLED=true`, the Docker socket is deliberately mounted/configured, and the target container is listed in `AUTOMATION_DOCKER_ALLOWED_CONTAINERS`.

Automation health checks are also treated as server-side network actions. HTTP and TCP health-check targets must use hosts allowed by `AUTOMATION_HEALTH_ALLOWED_HOSTS`, link-local/metadata targets are blocked unless `AUTOMATION_HEALTH_ALLOW_LINK_LOCAL=true`, and HTTP redirects are not followed.

Workflow autonomy:

- `WORKFLOW_SCHEDULER_ENABLED`, default `true`, runs the workflow scheduler in the backend.
- `WORKFLOW_OPEN_LOOP_SCHEDULER_ENABLED`, default `true`, lets the scheduler trigger due follow-up proposals before running ready workflow items.
- `WORKFLOW_SCHEDULER_INTERVAL_SECONDS`, default `60`, minimum `15`.
- `WORKFLOW_SCHEDULER_RUN_LIMIT`, default `5`, capped at `50`.
- `WORKFLOW_CLAIM_LEASE_SECONDS`, default `900`, minimum `60`, capped at `86400`. Active task workers renew the lease every third of this duration.
- The scheduler uses the same workflow, task, verification, and controlled automation services as the API, so approval-required work remains in the approval queue, emergency stop still blocks execution, owned leases prevent duplicate concurrent execution and stale result writes, task-runner panics enter normal retry handling, review-required runtime outcomes do not auto-retry, and completion requires runtime plus verification success.

The nginx config-manager no longer receives `/var/run/docker.sock` in `docker-compose.local.yml` by default. It can write generated route config files, but nginx reload via Docker API is skipped unless `NGINX_RELOAD_ENABLED=true` and the operator deliberately restores a reviewed Docker socket mount.

Current limitation: OpenClaw, QwenPaw, generic MCP, browser, and desktop-agent execution still need dedicated adapters. Hermes and Odysseus are implemented, but a live end-to-end run still requires the operator to install/configure those upstream runtimes, create scoped credentials/session state, and enable the matching HAI environment flags. They are not bundled into the HAI backend image.

## Safety Rules for Developers

This project is intended to gain local execution power, so safety should be designed before autonomy:

- Keep read-only behavior as the default.
- Require approval for email sending, legal/government actions, financial actions, account changes, public posting, deletion, destructive file changes, and broad local execution.
- Never treat client-supplied `humanApproved` or generic-transition `approved` fields as approval provenance. Public task/verification handlers clear those fields, and approval-required workflows must use the dedicated approval queue endpoint.
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
- The local gateway expects backend and IDP routes under `/api`. Backend engine APIs are routed only under `/api/v1/automation`, `/api/v1/llm`, `/api/v1/memory`, `/api/v1/task`, `/api/v1/sources`, `/api/v1/verification`, `/api/v1/os`, and `/api/v1/workflow`.
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
