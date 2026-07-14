# 018-HAI

018-HAI is a local-first Human Autonomous Intelligence Shell: a Personal AI Operating System for turning source material, memory, workflows, approvals, and controlled execution into inspectable operational work. It combines an Angular dashboard, Go APIs, an IDP, nginx gateway, Postgres, Redis, Kafka, and Docker Compose for a Windows 11-oriented local deployment.

The canonical product is this Go/Angular/Postgres stack. It is a governed operations system, not an unrestricted desktop agent: planning, execution, verification, and approval are deliberately separate, and external effects remain blocked unless a reviewed runtime and approval path are configured.

## Canonical Stack Decision

The canonical product stack is this Codex-built Go backend, Angular dashboard, Postgres persistence, and Docker Compose local runtime. The Manus React/tRPC/MySQL implementation should be treated as reference material only. Useful Manus behavior should be ported into this stack deliberately, not developed as a parallel product.

This decision is captured in [ADR 0001](docs/architecture-decision-records/0001-canonical-stack-and-readiness.md). The dashboard HAI OS page also exposes real-world readiness gates so internal AI logic is not mistaken for proven external integration behavior.

## Current State

**Status snapshot: 2026-07-14, `main`.** 018-HAI has an implemented, safety-gated operating layer. It is a local deployment candidate with code-level and local-service verification; it is **not** yet a proven real-world autonomous system for live accounts, providers, or unrestricted device control.

### How To Read This Status

The repository uses three deliberately different readiness terms:

- **Implemented** means the Go/Angular/Postgres path, persistence, API contract, and focused automated coverage exist in this repository.
- **Locally validated** means a bounded local check has exercised the relevant code path, build, Compose configuration, or real local Postgres smoke path. It does not establish third-party correctness.
- **Live-proven** requires an explicitly configured account, provider, or runtime to pass a bounded, approved end-to-end check on the target machine with inspectable audit and verification evidence.

No dashboard status, model configuration, source connection, or generated answer upgrades itself to live-proven. Until that evidence exists, HAI keeps consequential work behind its existing review, approval, verification, runtime, and emergency-stop controls.

What is implemented in this repository:

- **User experience:** onboarding, quick capture, Command Dashboard, Control Center, HAI OS, pursuits, workflow exceptions, automations, LLM routing, memory, connected sources, grounded answers, and task planning.
- **Core engine:** task intake and success criteria, context retrieval, policy-aware model/tool routing, controlled execution, retry/backoff, review queues, verification-gated completion, source-linked audit history, and a chat-command bridge that turns explicit run requests into pursuit-linked workflow work. Pursuits have an explicit completion/reopen lifecycle: new evidence never silently reopens completed work. An authenticated owner now flows through pursuit intake, planning, runtime-recovery workflow handoffs, personal ambient pursuit proposals, and private ambient need profiles into the task runner, task context retrieval, and verified lesson storage.
- **Knowledge and memory:** encrypted user-authorized conversation capture, compact context memory, retrieval/search/filter/pagination, deduplication, corrections, export/deletion planning, and source provenance. Authenticated memory APIs, AI-conversation imports, source-derived consolidation, workflow intake/feedback, direct task planning/runs, and explicit ambient accept/dismiss feedback are owner-scoped. Background/system work is deliberately ownerless unless it originates from an owner-bound source or workflow.
- **Connected-source import paths:** local folders, MBOX/EML email exports, ICS calendar exports, synced document folders, Trello JSON exports, WhatsApp exports, Odoo/HERP snapshots, normalized JSON feeds, and read-only GitHub repository/issue/pull-request/commit/workflow-run sync. New authenticated source records are owner-scoped through source search, extraction management, sync history, audits, workflow intake, and pursuit linking; ownerless legacy records remain visible in local-development compatibility mode.
- **Governance:** the backend independently verifies browser-session or bearer JWTs before using a signed principal for audit attribution; client-supplied actor labels are ignored. Approval gates, emergency stop, request rate limits, idempotency, redacted audit records, path safety, runtime allowlists, and a paid-model policy disabled by default are implemented.
- **Runtime and providers:** local/free model routing supports Ollama and OpenAI-compatible endpoints. Hermes, Odysseus, and OpenClaw are controlled adapters with bounded inventory/probe/execute paths. Every runtime remains disabled until explicitly configured and is subject to approval, workspace, timeout, audit, and verification gates.
- **Operations:** `/healthz`, `/readyz`, `backend doctor`, `backend reconcile`, support-bundle and build-information endpoints, feature flags, CI checks, Docker Compose configuration validation, and a real-Postgres critical-path smoke test.

### Readiness At A Glance

| Area | Repository status | What remains before it is trusted for real work |
| --- | --- | --- |
| Go API and Angular dashboard | Implemented; backend/IDP tests, Angular production build, and Compose configuration validation are part of CI. | Run the full Compose stack on the target Windows machine and exercise the intended user flows. |
| Workflow, verification, memory, and pursuits | Implemented with persistence, audit history, approval gates, retry/review states, and safety normalization. | Use representative, non-sensitive source fixtures and review the resulting audit trail before enabling automation. |
| Local/export source ingestion | Implemented for allowlisted local files and authorized exports. | OAuth/API connectors, webhooks, and a real account-specific bridge need separate scoped setup and live validation. |
| GitHub source sync | Implemented as a read-only REST connector, token optional for public repositories. | Configure a least-privilege token where required and validate against the chosen repository. |
| LLM routing | Local/free routing, endpoint guards, provider probes, fallback logging, and a EUR 0 paid default are implemented. | Install/configure a local model or free provider and pass a bounded live probe plus a representative validated task. |
| Controlled execution | API/script/Docker adapters and task evidence gates are implemented; script and Docker control are disabled by default. | Enable one narrowly scoped adapter, then prove an approved end-to-end workflow without expanding device permissions. |
| Hermes, Odysseus, and OpenClaw | Controlled adapter code and configuration surfaces are present; upstream software is not bundled. | Install/configure each upstream runtime separately, use dedicated workspaces/credentials, and validate one low-risk approved task at a time. |
| Authentication and RBAC | Signed identity is revalidated by the backend; explicit RBAC routes default to viewer when a JWT has no role. | Add IDP role issuance and broaden permission checks before relying on multi-role operation. |
| Owner-scoped operating work | Authenticated sources, memory, pursuits, verification, workflows, task history, reviews, and direct source preflight preserve the caller identity. | Exercise two real local accounts before relying on isolation for shared or multi-user operation. |
| Ambient planning | Dashboard-triggered scans, proposals, scan history, and need-profile overrides are owner-scoped and suggestion-only. Ownerless system work retains a separate shared baseline. | Exercise two real local accounts and one ownerless system-worker path before relying on the boundary for shared or multi-user operation. |
| Scheduled source refresh | A system worker processes globally due sources; an authenticated task preflights only that caller's explicitly owned due sources before source search. | Add per-owner scheduler dispatch only after account and source-credential boundaries are live-validated. |

### Operator Entry Points

After signing in, the Angular dashboard exposes these authenticated operator surfaces:

- `/control-center` for the primary operational overview and controlled maintenance cycle.
- `/command-dashboard` for Robert-only decisions, open loops, memory-derived work, and source-backed context.
- `/pursuits` for durable objectives, their workflow/evidence links, blockers, approvals, and explicit closure or reopen decisions.
- `/workflow-engine` for the execution queue, interrupted work, quality gates, approvals, and follow-ups.
- `/connected-sources`, `/memory`, `/llm-policy`, `/ambient-brain`, and `/task-blueprint` for source, context, provider, proactive-planning, and explicit-command operations.

These screens surface operational state; they do not prove that an external action occurred. The linked audit, approval, runtime, and verification records remain the source of truth.

Verified evidence is maintained in [the completion matrix](docs/codex-goal/completion-matrix.md), [final verification report](docs/codex-goal/final-verification-report.md), and [fresh-clone dry run](docs/fresh-clone-dryrun.md). The critical-path smoke has passed against a real local Postgres instance. That evidence proves the exercised local path, not live LLM, email, calendar, Drive, browser, or third-party-runtime correctness.

### Deliberate Boundaries

- HAI does not send messages, spend money, post publicly, delete data, change accounts, or take broad device control by default.
- OAuth account authorization/refresh, provider webhooks, file-system watchers, dedicated vector infrastructure, and additional Claw-compatible adapters remain follow-up work.
- A configured provider or runtime is not considered proven until its live probe and approved workflow are exercised on the target machine.
- The full Docker Compose topology has configuration and health checks, but its end-to-end multi-service boot remains the main outstanding deployment verification. See [fresh-clone dry run](docs/fresh-clone-dryrun.md) and [technical debt](docs/technical-debt.md).
- The bundled IDP currently issues a stable `user_id` for authenticated-session attribution. It does not yet issue role claims, so endpoints with explicit RBAC checks default to viewer until role issuance is implemented.
- Owner boundaries protect owner-scoped sources, memories, conversation imports, pursuits, verification runs/evidence links, authenticated workflow/task execution, task-history and review-queue views, review resolution, and ambient proposal/scan/profile records. Authenticated workflow lists, dashboards, details, and mutation routes enforce that boundary, and source identity/URI deduplication is scoped to the same verified owner so one account cannot reuse or supersede another account's work. A dashboard-triggered ambient scan or need-profile update requires an authenticated owner, reads only that owner's pursuit dashboard and planning profile, creates only owner-tagged proposals and scan history, and never starts workflow execution. Private need profiles overlay the shared baseline without modifying it; ownerless system workers continue to use that baseline. Verification uses only the caller's visible source context, persists the caller on the verification run, owner-scopes history/detail views, owner-scopes verified memory writes, and validates a requested pursuit before attaching evidence. Authenticated pursuit links validate workflow, memory, and source targets against the same owner before they are persisted. Owner-scoped pursuit detail, evidence, and approval responses also filter old/imported workflow, memory, source, and verification links when their target is no longer visible to that owner. Legacy ownerless records are read-compatible for local development but are never adopted or modified by an authenticated owner. Background/system workers retain a separate ownerless view and must not be exposed through authenticated operator endpoints.
- An authenticated task searches only sources visible to that owner and can preflight only sources explicitly owned by that caller. It never invokes the global due-source scheduler or modifies ownerless legacy sources; the global worker remains a separate controlled system operation.
- An authenticated agent-cycle request is an owner-scoped operating refresh: it reads only that owner's memory and pursuit state and does not start global source sync, workflow execution, ambient scans, or shared operational learning. The HTTP routes reject unauthenticated cycle runs; system-worker phases are invoked in-process by controlled schedulers until per-owner scheduling is implemented.
- The dashboard is an operator surface, not evidence that an external action ran. Completion and audit records remain authoritative only when they contain the required runtime, source, verification, and approval evidence.

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
|-- browser-extension/       Explicit, user-authorized browser conversation capture
|-- scripts/                 Smoke and operational verification scripts
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

Change `BACKEND_API_SHARED_KEY` before a real local install. Docker Compose injects the same `.env.local` value into nginx at startup; the backend requires `X-HAI-Backend-Key` only when the value is non-empty, and the gateway still requires IDP authentication before proxying backend routes.

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

The backend mounts this folder read-only at `CONNECTED_SOURCE_LOCAL_ROOT`. Folder paths that escape that root are rejected. General local ingestion supports `.txt`, `.md`, `.markdown`, `.csv`, `.tsv`, `.json`, `.yaml`, `.yml`, and `.log`; the export connectors also accept `.mbox`, `.eml`, and `.ics` within the same allowlisted root.

For recurring source ingestion, set the source `Sync` field to a duration such as `15m`, `1h`, `hourly`, `daily`, or `weekly`. The low-power system scheduler checks due sources every `SOURCE_SCHEDULER_INTERVAL_SECONDS` seconds when `SOURCE_SCHEDULER_ENABLED=true`; the documented local default is 600 seconds. Manual sync remains available from the dashboard when immediate work is needed. Authenticated task requests use owner-scoped source search and, when fresh source context is needed, preflight only their explicitly owned due sources. They never start the global scheduler or modify ownerless legacy sources. The scheduler supports local folders, email/calendar/project-board exports, synced document folders, GitHub REST sources, normalized JSON feeds, WhatsApp exports, and Odoo/HERP snapshots.

## Developer Checks

Backend:

```bash
cd backend
go vet ./...
go test ./...
go build ./...
```

Frontend:

```bash
cd frontend
npm ci
npm run build
npx ng test --watch=false --browsers=ChromeHeadlessNoSandbox
```

Compose:

```bash
docker compose --env-file .env.example -f docker-compose.local.yml config
```

On a Bash-capable developer shell with the smoke prerequisites available:

```bash
scripts/smoke-critical-path.sh
```

When Go is not installed locally, the backend can be checked through Docker:

```powershell
docker run --rm -v "${PWD}\backend:/app" -w /app golang:1.21.13 go test ./...
docker compose --env-file .env.example -f docker-compose.local.yml build backend
```

## Main API Areas

All backend routes are served under `/api/v1` through the gateway. The local nginx config only proxies the exact backend namespaces, or paths below them, to the backend; unknown `/api/v1/...` paths fall through to the IDP route instead of being broadly forwarded.

Platform and operator routes include `GET /healthz`, `GET /readyz`, `GET /api/v1/flags`, `GET /api/v1/system/info`, and the admin-protected `GET /api/v1/system/support-bundle`.

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

Assistant command bridge:

- `POST /assistant/command`
- `GET /assistant/logs`

Workflow engine:

- `GET /workflow/overview`
- `GET /workflow/approvals`
- `GET /workflow/dashboard`
- `GET /workflow/`
- `POST /workflow/intake`
- `POST /workflow/recover-stale`
- `POST /workflow/run-due`
- `POST /workflow/open-loops/run-due`
- `GET /workflow/:id`
- `POST /workflow/:id/transition`
- `POST /workflow/:id/approval`
- `POST /workflow/:id/interruption/resolve`
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
- `GET /sources/sync-jobs`
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

Pursuits:

- `GET /pursuits/`
- `POST /pursuits/`
- `GET /pursuits/dashboard`
- `GET /pursuits/brief`
- `GET /pursuits/decisions`
- `POST /pursuits/match`
- `POST /pursuits/intake`
- `GET /pursuits/:id/evidence`
- `GET /pursuits/:id`
- `PATCH /pursuits/:id`
- `POST /pursuits/:id/archive`
- `POST /pursuits/:id/reopen`
- `POST /pursuits/:id/summary`
- `POST /pursuits/:id/review`
- `POST /pursuits/:id/decisions/resolve`
- `GET /pursuits/:id/activity`
- `GET /pursuits/:id/next-actions`
- `GET /pursuits/:id/blockers`
- `GET /pursuits/:id/approvals`
- `POST /pursuits/:id/intake`
- `POST /pursuits/:id/plan`
- `POST /pursuits/:id/links`
- `DELETE /pursuits/:id/links/:linkId`

Ambient, agent-cycle, and controlled runtime support:

- `GET /ambient/overview`
- `POST /ambient/scan`
- `PATCH /ambient/needs/:key`
- `POST /ambient/opportunities/:id/accept`
- `POST /ambient/opportunities/:id/dismiss`
- `POST /agent-cycle/run`
- `GET /agent-runtimes/`
- `GET /agent-runtimes/health`
- `GET /agent-runtimes/:id/skills`
- `POST /agent-runtimes/:id/tasks/:taskId/stop`

Conversation memory archive APIs are available under `/memory-engine` for explicit import, dashboard/search, conversation inspection/deletion, and extracted insights. They are intentionally separate from compact context-memory APIs under `/memory`.

## Engine Behavior

The task success engine follows a completion-first loop:

1. Classify the request.
2. Infer the real goal.
3. Define success criteria.
4. For system work, refresh due connected sources when the request likely depends on project, file, document, or local context. For authenticated work, search only the caller's visible sources and do not start the global scheduler.
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

The task engine treats connected sources as an active preflight dependency. System/background requests that mention project/source/file/folder/document/repo context, or require local/document context, run due scheduled source syncs before source search. Authenticated requests run the same preflight only for their explicitly owned due sources, then use owner-scoped search. They do not fan out into another user's scheduled sources or modify ownerless legacy sources.

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

Manual input can be sent to `POST /workflow/intake`. In the canonical application this compatibility endpoint now normalizes direct calls with stable `workflow_api` provenance and routes them through pursuit matching before returning the same workflow-record response contract. This prevents API-created work from becoming an orphaned parallel path: the resulting workflow is linked to an existing or reviewable candidate pursuit and remains subject to the same approval, worker, verification, and audit controls. Connected-source sync also sends actionable extractions with tasks or follow-ups into the workflow engine. Source-derived intake deduplicates first by stable source type plus extraction identity, then falls back to source URI for older/manual callers. Each source workflow stores a deterministic revision hash over its executable content, provenance, project, and review requirements. An unchanged revision reuses the active workflow; a changed revision archives the prior workflow and builds a fresh checklist, evidence set, quality gates, and approval state. This prevents corrected source data from inheriting stale instructions or a human approval granted to an earlier version. In-progress workflows cannot be superseded until their execution outcome is reviewed. Separate records may therefore share a mailbox, sender, document, or board URI without collapsing into one workflow. Uncertain or sensitive extractions are forced into `needs_approval` rather than entering the autonomous ready queue.

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

The private Chrome/Edge extension lives in `browser-extension/`. Load it as an unpacked extension, keep the default local endpoint `http://127.0.0.1:17070/api/v1/memory-engine/import`, enter `BACKEND_API_SHARED_KEY`, open one of Robert's own AI conversation pages, and click **Capture current conversation**. The extension reads only the currently open thread after that explicit click. It does not read cookies, passwords, local storage, hidden account data, or unrelated pages, and it sends requests with `credentials: omit`.

### Normalized JSON source feeds

The operational `json-feed` connector is the bridge between HAI and account-specific adapters. A local service can use the official Gmail, Calendar, GitHub, Trello, Drive, or other permitted API, retain its own credentials, and expose normalized read-only records to HAI:

```json
{
  "nextCursor": "provider-cursor-2",
  "items": [
    {
      "externalId": "stable-provider-item-id",
      "title": "Item title",
      "content": "Extractable source text",
      "sourceUri": "provider://account/item-id",
      "itemType": "email",
      "projectKey": "018-HAI"
    }
  ]
}
```

HAI sends the previous cursor as the `cursor` query parameter, persists the returned `nextCursor`, deduplicates records, extracts tasks and decisions, creates workflow candidates, updates useful memory, and records sync/audit history. The endpoint must use HTTP(S), its hostname must be explicitly listed in `CONNECTED_SOURCE_HTTP_ALLOWED_HOSTS`, redirects are rejected, and response size and timeout are bounded. Keep provider credentials in the bridge or provider secret store, never in `syncTarget`.

DNS results are checked again when the socket is opened, so an allowlisted hostname cannot redirect HAI into link-local or metadata address space through DNS rebinding. Proxy environment variables are not used for feed retrieval. Scheduled failures are persisted as sync jobs and routed into the workflow review queue, making unavailable or misconfigured sources visible operational work instead of log-only failures.

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

## Pursuits Layer

Pursuits are the durable objective containers above individual workflows. A pursuit groups related workflow items, source material, memories, verification evidence, runtime attempts, decisions, blockers, next actions, and approvals into one operator-facing objective. The `/pursuits` dashboard and detail view distinguish work that needs Robert's decision, is ready for delegation, can be processed by a bounded system workflow, or is waiting on an external party. Linked workflow quality gates are shown in pursuit detail; a failed or `needs_review` gate becomes a named pursuit blocker, is placed in Robert's action queue, and prevents a completion recommendation until reviewed.

Pursuit intake and matching reuse the existing source, workflow, memory, verification, and ambient-planning services rather than introducing a parallel agent implementation. Source sync, AI-memory extraction, ambient opportunities, HAI chat runs, and the legacy workflow-intake API all use the shared pursuit route when the pursuit service is configured. That route centralizes matching, candidate creation, approval requirements, and closed-pursuit protection before a workflow is created; producer-specific fallback mode remains available for isolated deployments and tests. Matching first resolves an existing source type/ID, then a stable source URI, before using project and text evidence; this avoids duplicate pursuit candidates when older or manual callers have a reliable URI but no provider item ID. Creation, planning, intake, review, decision resolution, archive operations, and link changes record the authenticated session principal when present; a client payload cannot claim another person as the actor. Local development without an IDP session records the fallback operator/system identity honestly. This supports traceable operation but does not itself authorize a sensitive action: the workflow approval queue, execution policy, and verification gates remain authoritative.

Operational pursuit activity, including evidence/workflow links and link removals, advances the durable `LastActivityAt` value. Passive summary refreshes remain in the audit feed but do not reset freshness, so the stale-pursuit view measures real movement rather than background observation or direct field edits.

Closed pursuits are removed from active operational queues. During an ambient scan, any open or accepted pursuit-derived opportunity whose linked pursuit is completed or archived is completed with a closure note; dismissed opportunities remain untouched as operator feedback. This prevents the proactive layer from resurfacing work that Robert has already closed.

Closed pursuits also reject direct intake, planning, and decision-resolution requests before the workflow engine is invoked. When global intake finds that its best match is closed, it preserves that historical pursuit and creates a separate reviewable candidate rather than treating a valid new source signal as a sync failure or silently reactivating old work. A summary refresh is read-only for a closed pursuit, so an old client request or late refresh cannot silently reactivate completed work.

Reopening is a separate audited transition at `POST /pursuits/:id/reopen`. It clears the closed state but does not execute work; new work must still enter through the governed workflow, approval, and verification paths. Archiving uses `POST /pursuits/:id/archive` with `{"archived": true}`; the public archive endpoint rejects restore-shaped, missing, or malformed bodies. An approval to create a runtime-recovery workflow is written to the decision audit only after the required governed workflow is persisted, so an intake failure leaves the original decision pending and visible. Generic updates cannot reactivate a closed pursuit. This prevents a late source sync, old browser request, or stale client from treating historical work as active again.

The HAI chat at `/task-blueprint` can be opened from a pursuit detail page. A planning-only command shows matching pursuit context without creating operational work. An explicit **Run** command receives a deterministic `assistant_command` source identity, is routed into the selected or matched pursuit, and creates or reuses a governed workflow. The command identity is retained as a pursuit `command_origin` link, so a repeated command resolves to its existing pursuit before heuristic matching. The command bridge then creates a task plan and queues that workflow for the existing worker scheduler; it does not directly execute the task as a second parallel path or run unrelated ready workflows. An authenticated operating-refresh command returns only the caller's context and pursuit state; global source/workflow/ambient maintenance stays with the controlled system worker. This prevents duplicate execution and cross-owner worker activity. High-risk work still enters approval review, and completion remains dependent on the worker's runtime, verification, and quality-gate evidence.

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

Odysseus upstream reference: [pewdiepie-archdaemon/odysseus](https://github.com/pewdiepie-archdaemon/odysseus). The supplied Odysseus package exposes a self-hosted AI workspace with a scoped Codex integration API, local model workspace, agent loop, prompt/tool security layers, memory, todos/reminders, email, calendar, contacts, documents, research/search, MCP, Cookbook model serving, companion/webhook surfaces, and Codex/Claude bridge assets. HAI integrates it as a controlled runtime architecture, not as an unrestricted local-admin bridge. Runtime execution still goes through HAI approval, audit, verification, host allowlisting, token scoping, bounded timeouts, and secret-redacted output capture.

Hermes upstream reference: [NousResearch/hermes-agent](https://github.com/NousResearch/hermes-agent). HAI treats Hermes as an optional agentic substrate, not as uncontrolled core logic. The runtime registry exposes Hermes' broader ecosystem as controlled capabilities: model/provider routing, skills and skill learning, memory and session search, MCP servers, gateway channels, cron jobs, subagent delegation, mixture-of-agents orchestration, terminal backends, context compression, OpenClaw migration, and ACP integration. Actual execution still uses Hermes' documented noninteractive `hermes chat -q ... -Q` path.

The Hermes adapter never adds `--yolo`, enables filesystem checkpoints, passes arguments without shell interpolation, runs in a dedicated configured workspace, inherits only allowlisted environment variables plus explicit HAI task metadata, and stops the process when the configured timeout expires. HAI remains the policy, approval, audit, and verification control plane.

OpenClaw upstream package reference: local `openclaw-main.zip` / OpenClaw distribution. HAI treats OpenClaw as a local-first Gateway and agent runtime ecosystem, not as a bundled dependency. When `OPENCLAW_ECOSYSTEM_PATH` points at the supplied zip or an extracted OpenClaw package, the runtime registry inventories the actual skills, skill scripts, root scripts, extensions, provider paths, channel adapters, companion apps, core packages, source modules, documentation corpus, QA assets, test suites, config profiles, security assets, deployment descriptors, Codex prompt maps, GitHub workflows, reusable GitHub Actions, issue templates, security/CodeQL maps, repository instructions, repository docs/config, and control UI/controller surfaces for dashboard visibility and future planning. Actual execution through HAI uses only the documented noninteractive `openclaw agent --message ... --thinking ...` CLI path.

Before execution, HAI wraps the approved user task in an OpenClaw task envelope. That envelope maps the task to relevant indexed OpenClaw skills, lists visible provider/tool surfaces, carries relevant security/CI/issue/instruction maps, carries the original request, and repeats the blocked surfaces and validation checklist. The selected route trace is persisted into automation launch history, task evidence, workflow evidence, and dashboard summaries. The OpenClaw adapter does not call `openclaw message send`, approve pairings, invoke nodes, control browsers, create cron jobs, execute public posting, or bypass OpenClaw's Gateway auth/scopes/sandboxing. Those surfaces are visible as configured capabilities only. HAI remains the approval, audit, workspace, timeout, secret-redaction, and verification control plane.

OpenClaw task profiling now also routes HAI-native personal operating work: pursuits, open loops, deadlines, source-grounded evidence, memory/context, Odoo/HERP operations, and local-model/provider setup are mapped to indexed OpenClaw skills and reference maps while outbound communication, public posting, destructive file changes, and legal/financial/government commitments stay blocked until separate HAI approval and verification exist.

### Controlled Hermes, Odysseus, and OpenClaw runtimes

1. Install and configure Hermes, Odysseus, or OpenClaw separately. HAI does not download or auto-update third-party agent software.
2. Create an HAI automation with `launchType=agent_runtime` and `runtimeType=hermes`, `runtimeType=odysseus`, or `runtimeType=openclaw`.
3. Supply that automation ID to a task/workflow request. Direct automation launch calls cannot forge approval and therefore cannot execute agent tasks.
4. Approve the resulting review item. The server then replays the stored task with a server-side approval record.
5. HAI captures the runtime output as evidence and still requires verification before the task can be marked complete.

Hermes configuration:

- `HERMES_AGENT_ENABLED=true`
- `HERMES_EXECUTABLE=hermes` or an explicit installed executable path
- optional `HERMES_HOME` and `HERMES_PROFILE` to isolate Hermes state/profile for HAI
- optional `HERMES_IGNORE_USER_CONFIG=true` for tighter noninteractive isolation
- `AGENT_RUNTIME_WORKSPACE_ROOT=<parent folder dedicated to agent workspaces>`
- `HERMES_WORKSPACE=<dedicated allowlisted workspace>`
- optional `HERMES_TOOLSETS`, `HERMES_SKILLS`, `HERMES_ENV_ALLOWLIST`, `HERMES_MAX_TURNS`, and `HERMES_TIMEOUT_SECONDS`
- optional ecosystem visibility flags: `HERMES_GATEWAY_ENABLED`, `HERMES_CRON_ENABLED`, `HERMES_MCP_ENABLED`, `HERMES_MOA_ENABLED`, `HERMES_SUBAGENTS_ENABLED`, `HERMES_MEMORY_SYNC_ENABLED`, `HERMES_ACP_ENABLED`, and `HERMES_TERMINAL_BACKENDS`

Recommended first Hermes posture for HAI is restrictive:

- use a dedicated `HERMES_HOME` and `HERMES_WORKSPACE`
- start with `HERMES_TOOLSETS=safe,skills,todo` or another reviewed small set
- preload only reviewed skills through `HERMES_SKILLS`
- enable gateway, cron, MCP, MoA, subagents, and ACP one surface at a time after HAI approval/audit flows are verified

Odysseus configuration:

- `ODYSSEUS_AGENT_ENABLED=true`
- `ODYSSEUS_BASE_URL=http://host.docker.internal:7000` for a default Windows-host installation
- `ODYSSEUS_API_TOKEN=<scoped token>`
- `ODYSSEUS_AGENT_SESSION_ID=<existing configured session>`
- optional `ODYSSEUS_AGENT_WORKSPACE` and `ODYSSEUS_AGENT_TIMEOUT_SECONDS`
- optional ecosystem visibility flags: `ODYSSEUS_TODOS_ENABLED`, `ODYSSEUS_NOTES_ENABLED`, `ODYSSEUS_TASKS_ENABLED`, `ODYSSEUS_EMAIL_ENABLED`, `ODYSSEUS_CALENDAR_ENABLED`, `ODYSSEUS_CONTACTS_ENABLED`, `ODYSSEUS_DOCUMENTS_ENABLED`, `ODYSSEUS_MEMORY_SYNC_ENABLED`, `ODYSSEUS_RESEARCH_ENABLED`, `ODYSSEUS_SEARCH_ENABLED`, `ODYSSEUS_MCP_ENABLED`, `ODYSSEUS_COOKBOOK_ENABLED`, `ODYSSEUS_LOCAL_MODEL_DISCOVERY_ENABLED`, `ODYSSEUS_BROWSER_ENABLED`, `ODYSSEUS_VAULT_ENABLED`, `ODYSSEUS_GALLERY_ENABLED`, `ODYSSEUS_TTS_ENABLED`, `ODYSSEUS_STT_ENABLED`, `ODYSSEUS_COMPANION_ENABLED`, `ODYSSEUS_WEBHOOKS_ENABLED`, `ODYSSEUS_CODEX_BRIDGE_ENABLED`, `ODYSSEUS_CLAUDE_BRIDGE_ENABLED`, `ODYSSEUS_AGENT_MIGRATION_ENABLED`, and `ODYSSEUS_CONTEXT_BUDGET_ENABLED`
- high-risk runtime toggles: `ODYSSEUS_SHELL_ENABLED`, `ODYSSEUS_AGENT_ALLOW_BASH`, `ODYSSEUS_AGENT_ALLOW_WEB_SEARCH`, and `ODYSSEUS_AGENT_ALLOW_RESEARCH`

Use a narrowly scoped Odysseus token created for the Codex Agent integration surface. HAI checks `/api/codex/capabilities` and treats `403` as an intentional restriction, not as something to work around. The adapter does not SSH into the Odysseus host, query its database, import Python internals, use Docker directly, or bypass `/api/codex/*` and `/api/chat_stream`.

Recommended first Odysseus posture for HAI is restrictive:

- use read-only token scopes first
- keep `ODYSSEUS_SHELL_ENABLED=false`, `ODYSSEUS_AGENT_ALLOW_BASH=false`, `ODYSSEUS_AGENT_ALLOW_WEB_SEARCH=false`, and `ODYSSEUS_AGENT_ALLOW_RESEARCH=false`
- enable todos/memory/documents before email/calendar/cookbook/host-control surfaces
- only enable email send, calendar writes, document deletion, Cookbook serve/stop, browser control, shell, or public posting after the matching HAI approval workflow has been tested end to end
- keep the Odysseus instance on localhost, `host.docker.internal`, or another host explicitly listed in `AGENT_RUNTIME_ALLOWED_HOSTS`

OpenClaw configuration:

- `OPENCLAW_AGENT_ENABLED=true`
- `OPENCLAW_EXECUTABLE=openclaw` or an explicit installed executable path
- optional `OPENCLAW_GATEWAY_URL=ws://host.docker.internal:18789` for a Windows-host Gateway
- `OPENCLAW_GATEWAY_TOKEN=<gateway token>` when `OPENCLAW_GATEWAY_ENABLED=true`
- optional `OPENCLAW_STATE_DIR` and `OPENCLAW_CONFIG_PATH` to isolate OpenClaw state/config for HAI
- optional `OPENCLAW_ECOSYSTEM_PATH=<extracted OpenClaw repo/package or openclaw-main.zip>` for read-only ecosystem inventory. HAI scans names and metadata paths for skills, skill scripts, root scripts, extensions, providers, channels, apps, packages, source modules, docs, QA/test assets, config profiles, security assets, deployment descriptors, Codex prompt maps, GitHub workflows, reusable GitHub Actions, issue templates, security/CodeQL maps, repository instructions, repository docs/config, and control UI/controller modules; it does not execute scripts, import OpenClaw code, or dispatch upstream automations from this path.
- OpenClaw ecosystem uploads use the command dashboard upload route `/api/v1/agent-runtimes/openclaw/ecosystem/upload`. The local nginx gateway gives only that route a bounded `800m` body limit and disables request buffering so `openclaw-main.zip` style archives can reach the backend. The backend still validates `.zip`, rejects unsafe archive paths, and caps accepted archives at 750 MB.
- `AGENT_RUNTIME_WORKSPACE_ROOT=<parent folder dedicated to agent workspaces>`
- `OPENCLAW_WORKSPACE=<dedicated allowlisted workspace>`
- optional `OPENCLAW_THINKING`, `OPENCLAW_TIMEOUT_SECONDS`, and `OPENCLAW_ENV_ALLOWLIST`
- optional ecosystem visibility flags: `OPENCLAW_GATEWAY_ENABLED`, `OPENCLAW_MESSAGES_ENABLED`, `OPENCLAW_SKILLS_ENABLED`, `OPENCLAW_PLUGINS_ENABLED`, `OPENCLAW_MCP_ENABLED`, `OPENCLAW_MEMORY_ENABLED`, `OPENCLAW_CRON_ENABLED`, `OPENCLAW_BROWSER_ENABLED`, `OPENCLAW_CANVAS_ENABLED`, `OPENCLAW_NODES_ENABLED`, `OPENCLAW_VOICE_ENABLED`, `OPENCLAW_TALK_ENABLED`, `OPENCLAW_WEBCHAT_ENABLED`, `OPENCLAW_PAIRING_ENABLED`, `OPENCLAW_MULTI_AGENT_ENABLED`, `OPENCLAW_APP_SDK_ENABLED`, `OPENCLAW_PLUGIN_SDK_ENABLED`, `OPENCLAW_LOCAL_MODELS_ENABLED`, `OPENCLAW_CHANNELS_ENABLED`, `OPENCLAW_PROVIDERS_ENABLED`, and `OPENCLAW_COMPANION_APPS`
- high-risk runtime toggles: `OPENCLAW_EXEC_APPROVALS_ENABLED`, `OPENCLAW_HOST_TOOLS_ENABLED`, `OPENCLAW_PUBLIC_POSTING_ENABLED`, `OPENCLAW_WEB_SEARCH_ENABLED`, `OPENCLAW_SANDBOX_REQUIRED`, `OPENCLAW_SANDBOX_MODE`, `OPENCLAW_SANDBOX_DOCKER_ENABLED`, `OPENCLAW_SANDBOX_SSH_ENABLED`, and `OPENCLAW_SANDBOX_OPENSHELL_ENABLED`
- `OPENCLAW_ALLOW_HIGH_RISK_EXECUTION=false` by default. If any high-risk OpenClaw surfaces are enabled, HAI blocks generic OpenClaw runtime execution until those surfaces are disabled again or this flag is deliberately set to `true`; per-task HAI approval and OpenClaw's own downstream policies still apply.

Recommended first OpenClaw posture for HAI is restrictive:

- use a dedicated OpenClaw state/config and a dedicated `OPENCLAW_WORKSPACE`
- keep `OPENCLAW_AGENT_CLI_ENABLED=true` and all outbound/device/control surfaces disabled
- keep `OPENCLAW_ALLOW_HIGH_RISK_EXECUTION=false` unless a reviewed high-risk runtime policy exists
- keep `OPENCLAW_SANDBOX_REQUIRED=true` with `OPENCLAW_SANDBOX_MODE=all`
- enable skills, memory, local models, and Gateway readiness before messaging, browser, cron, nodes, host tools, or public posting
- only enable channel sends, pairing approval, browser control, node commands, cron writes, host tools, public posting, or web search after the matching HAI approval and audit workflow is tested end to end
- keep the Gateway on localhost, `host.docker.internal`, or another host explicitly listed in `AGENT_RUNTIME_ALLOWED_HOSTS`

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
- Read-only local-folder, email-export, calendar-export, synced-document, Trello-export, GitHub REST, JSON-feed, WhatsApp-export, and Odoo/HERP source adapters.
- Scheduled due-sync for enabled sources with non-manual sync frequencies.
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
- `CONNECTED_SOURCE_HTTP_ALLOWED_HOSTS`, exact hostname allowlist for normalized JSON feeds.
- `CONNECTED_SOURCE_HTTP_ALLOW_LINK_LOCAL`, default `false`; keep disabled unless a reviewed local adapter specifically needs link-local access.
- `CONNECTED_SOURCE_HTTP_TIMEOUT_SECONDS`, default `20`, bounded to `1-120`.
- `CONNECTED_SOURCE_HTTP_MAX_BYTES`, default `2097152`, bounded to `1 KiB-20 MiB`.
- `SOURCE_SCHEDULER_ENABLED`, default `true`.
- `SOURCE_SCHEDULER_INTERVAL_SECONDS`, default `600`, minimum `15`.
- `SOURCE_SCHEDULER_RUN_ON_STARTUP`, default `false`; set to `true` only when startup should immediately scan due sources.
- Incremental sync based on `LastSyncedAt`, unless mode is `historical_backfill`.

HAI now ships the following read-only connector paths: local MBOX/EML email exports, ICS calendar exports, selected synced cloud-document folders, Trello JSON exports, GitHub REST repositories/issues/pull requests/commits/workflow runs, local folders, normalized JSON feeds, WhatsApp exports, and Odoo/HERP snapshots. The first four export/sync-folder adapters remain local-first: place the authorized export or folder under `connected-sources/`, select its connector in Connected Sources, and keep the source `Local only` switch enabled. For GitHub, use `owner/repository` as the source target and set `GITHUB_SOURCE_TOKEN` only when private-repository access or higher API rate limits are required. OAuth adapters and provider webhook subscriptions remain separate, future work rather than implied capabilities.

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
- `WORKFLOW_SCHEDULER_INTERVAL_SECONDS`, default `600`, minimum `15`.
- `WORKFLOW_SCHEDULER_RUN_LIMIT`, default `2`, capped at `50`.
- `WORKFLOW_SCHEDULER_RUN_ON_STARTUP`, default `false`; set to `true` only when backend startup should immediately process due work.
- `WORKFLOW_CLAIM_LEASE_SECONDS`, default `900`, minimum `60`, capped at `86400`. Active task workers renew the lease every third of this duration.
- The scheduler uses the same workflow, task, verification, and controlled automation services as the API, so approval-required work remains in the approval queue, emergency stop still blocks execution, owned leases prevent duplicate concurrent execution and stale result writes, task-runner panics enter normal retry handling, review-required runtime outcomes do not auto-retry, and completion requires runtime plus verification success.

Ambient proactive planning:

- `/ambient-brain` reviews active workflows, approval gates, blockers, due open loops, source-linked contradictions, and delegation candidates without waiting for a prompt.
- Each opportunity has a deterministic fingerprint, compact evidence manifest, explained score, need category, next action, source link, risk score, and approval requirement.
- The five default need dimensions are health/capacity, safety/stability, relationships/belonging, reputation/capability, and growth/self-direction. They are a shared baseline for ownerless system work. Authenticated profile updates create private per-owner overrides, so one user's priorities and notes cannot alter another user's scan. They are planning preferences, not diagnoses or judgments about a person's worth or status.
- `AMBIENT_SCHEDULER_ENABLED=true` enables periodic scans.
- `AMBIENT_EXECUTION_ENABLED=false` keeps the default suggestion-only. Enabling it only permits bounded calls into the existing workflow and open-loop workers; it cannot bypass approvals, verification, emergency stop, leases, or audit controls.
- `AMBIENT_RUN_ON_STARTUP=false` prevents startup scans from competing with local boot and Docker initialization.
- `AMBIENT_SCAN_INTERVAL_SECONDS`, default `3600`, plus `AMBIENT_MINIMUM_SCORE`, `AMBIENT_MINIMUM_CONFIDENCE`, `AMBIENT_OPPORTUNITY_LIMIT`, `AMBIENT_EXECUTION_LIMIT`, `AMBIENT_DISMISS_COOLDOWN_HOURS`, and `AMBIENT_SCAN_RETENTION`, default `100`, bound background activity and storage growth.
- `AUTONOMY_WORLD_STATE_TTL_SECONDS` defines when an execution observation becomes stale and must be refreshed. `AUTONOMY_TELEMETRY_LIMIT` bounds recent action/state records returned to the dashboard.

The Proactive Brain also records execution-based autonomy telemetry for workflow worker attempts: compact world-state snapshots, typed action traces, verification status, latency, retries, human intervention, recovery, raw completion, and completion under policy. Its deterministic stress suite checks approval, stale-state, action-interface, and prompt-injection guards. These checks validate HAI's local policy boundaries; they do not substitute for provider-specific or real-world benchmark evidence.

### YAGNI decision discipline

Coding and architecture tasks pass through a deterministic minimality ladder before execution:

1. Confirm the requested implementation needs to exist.
2. Prefer the language standard library.
3. Prefer native browser, operating-system, database, or platform capabilities.
4. Reuse existing project dependencies and abstractions.
5. Prefer a one-line or small patch.
6. Permit narrowly scoped custom code only when earlier rungs are insufficient.

The selected rung is included in task plans, model instructions, validation steps, and engine events. New dependencies and speculative abstractions are blocked by default. Public Ponytail cost/code-reduction figures are treated as unverified claims until reproduced against HAI's own tasks and telemetry.
- Accepting a proposal links it to the controlled workflow engine. Dismissal applies a cooldown so the same source revision does not repeatedly interrupt the operator.
- Incremental scans reuse stable source fingerprints and store source identity, redacted URI, and revision time rather than duplicating raw source content into each scan. Scan history is pruned to the configured retention limit.

DualPath KV-cache boundary:

- [DualPath](https://arxiv.org/abs/2602.21548) addresses KV-cache storage bandwidth in disaggregated LLM serving. It requires separate prefill/decode engines, shared KV-cache storage, a compatible compute transfer network such as RDMA, and a global scheduler.
- It is not a general memory compression technique and does not automatically apply to a normal Ollama or LM Studio process.
- `LLM_KV_CACHE_LOAD_STRATEGY=disabled` and `LLM_DUALPATH_INFRASTRUCTURE_VERIFIED=false` are the safe defaults. A non-disabled strategy is only a provider capability hint until the required infrastructure is implemented and verified.
- HAI's compact evidence manifests and incremental scans reduce application-level storage traffic independently of DualPath.

The nginx config-manager no longer receives `/var/run/docker.sock` in `docker-compose.local.yml` by default. It can write generated route config files, but nginx reload via Docker API is skipped unless `NGINX_RELOAD_ENABLED=true` and the operator deliberately restores a reviewed Docker socket mount.

Current limitation: QwenPaw, generic MCP, browser, and desktop-agent execution still need dedicated adapters. Hermes, Odysseus, and OpenClaw are implemented as controlled runtime adapters, but a live end-to-end run still requires the operator to install/configure those upstream runtimes, create scoped credentials/session state where needed, and enable the matching HAI environment flags. They are not bundled into the HAI backend image.

## Safety Rules for Developers

This project is intended to gain local execution power, so safety should be designed before autonomy:

- Keep read-only behavior as the default.
- Require approval for email sending, legal/government actions, financial actions, account changes, public posting, deletion, destructive file changes, and broad local execution.
- Never treat client-supplied `humanApproved` or generic-transition `approved` fields as approval provenance. Public task/verification handlers clear those fields, and approval-required workflows must use the dedicated approval queue endpoint.
- Treat pursuit risk and autonomy labels as conservative policy inputs: HAI derives a risk floor from the pursuit goal, description, and desired outcome, records any normalization in the pursuit audit trail, and never lets a supplied label downgrade detected high-risk work into system-autonomous work.
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
- Docker Compose local mode uses PostgreSQL 17 with Docker-managed named volumes, one Kafka broker, and Zookeeper. PostgreSQL data directories must never be committed or copied as ordinary Git files because required empty runtime directories are not preserved. Export and restore with `pg_dump`/`pg_restore` when migrating existing data or changing major versions.
- The local gateway expects backend and IDP routes under `/api`. Backend engine APIs are routed only under `/api/v1/automation`, `/api/v1/llm`, `/api/v1/memory`, `/api/v1/memory-engine`, `/api/v1/pursuits`, `/api/v1/task`, `/api/v1/sources`, `/api/v1/verification`, `/api/v1/os`, `/api/v1/workflow`, `/api/v1/agent-runtimes`, and `/api/v1/ambient`.
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
- `docs/operator-runbook.md`
- `docs/acceptance-test-matrix.md`
- `docs/technical-debt.md`
- `docs/codex-goal/final-verification-report.md`

These documents cover the target direction, evidence, operator procedures, and known debt. The README is the concise current-state entrypoint; do not update it by claiming an external provider or runtime is live without an executed readiness/approval/verification record.

## License

See `LICENSE`.
