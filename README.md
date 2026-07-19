# 018-HAI

018-HAI is a local-first Human Autonomous Intelligence Shell: a governed
Personal AI Operating System for turning authorized source material, durable
memory, workflows, approvals, and controlled execution into inspectable work.

The canonical product is this repository's Go, Angular, Postgres, and Docker
Compose stack. It is not an unrestricted desktop agent: planning, execution,
verification, and approval are separate; external effects remain blocked until a
reviewed runtime, policy, and evidence path are configured.

> **Current repository state, evidence reviewed 2026-07-14:** this repository
> implements a governed local operating layer, including the Angular dashboard,
> Go engines, IDP, Compose topology, pursuit/workflow routing, persistence, and
> safety gates. On the development workspace used for this review, the Compose
> services were healthy, nginx served `/` and Angular deep links such as
> `/control-center`, gateway health routes responded, and protected APIs rejected
> unsigned sessions. Those observations are local-environment evidence, not a
> claim that every Windows machine or account integration is ready. Backend/IDP
> tests, frontend production build and unit tests, Compose validation, and a
> Postgres-backed critical-path smoke have been exercised. A clean-clone,
> signed-in Windows browser journey and any real third-party account, paid
> model, browser-control, or broad-host-control journey remain release gates.

### Current Change Boundary

The repository has recently completed a safety-focused pursuit hardening pass.
Pursuit dashboards, detail views, links, task-attempt summaries, runtime
evidence, source resolution, candidate routing, and decision handling are all
evaluated in the authenticated owner's scope. Related pursuits are navigable in
the dashboard, but a pursuit cannot link to itself and a relationship cannot
be used to expose another owner's operational record. Candidate pursuits remain
non-executable until an approval-capable user explicitly accepts them; decision
resolution is also permission-checked in the handler, not only in route wiring.
The Command Dashboard is the unified operator queue for governed workflow
approvals, proposal choices, candidate acceptance or archival, approved next
actions, runtime recovery, and verified pursuit completion; each control calls
the existing audited API rather than bypassing the relevant gate.

This is an implementation and focused-test milestone. It does not replace the
release gates in the verification snapshot below: a real two-account exercise,
fresh-machine browser flow, configured source import, local-model task, and
reviewed runtime dry run are still required before relying on those paths for
personal work.

## Product Boundary

HAI is the product. A pursuit is the high-level objective or case that connects
the systems below; it is not a second product or a replacement workflow engine.
The earlier Manus React/tRPC/MySQL implementation is reference material only.
Useful behavior from it should be ported deliberately into this stack rather
than maintained in parallel.

See [ADR 0001](docs/architecture-decision-records/0001-canonical-stack-and-readiness.md)
for the canonical-stack decision.

## What Is Implemented

| Area | Implemented capability | Important operating boundary |
| --- | --- | --- |
| Operator UI | Angular onboarding, Quick Capture, Control Center, Command Dashboard, HAI OS, pursuits, workflow exceptions, sources, memory, LLM policy, grounded answers, and task planning. | A dashboard card is operational visibility, not proof that an external action occurred. |
| Pursuits and workflows | Durable pursuits, workflow states, checklists, decisions, open loops, blockers, follow-ups, approvals, review queues, retries, task-attempt evidence, read-only VA delegation briefs, ambient opportunity routing, and navigable related-pursuit links. | In the canonical routed stack, new source, assistant, or ambient context is matched to an active pursuit first; otherwise it becomes an approval-gated candidate, not executable work. Links, evidence, and operational summaries stay owner-scoped; completion still needs workflow, verification, source, runtime, and approval evidence where applicable. |
| Memory and knowledge | Compact memory, retrieval, deduplication, correction, export/deletion planning, provenance, encrypted user-authorized conversation capture, and source/extraction links. | Raw imported conversations are not automatically promoted to trusted facts. |
| Source ingestion | Allowlisted local files; MBOX/EML, ICS, Trello JSON, WhatsApp exports, Odoo/HERP snapshots, normalized JSON feeds, synced document folders, and read-only GitHub sync. | Gmail, Calendar, Drive, Trello, WhatsApp, and browser accounts are export/local-folder paths, not live OAuth or browser connectors. |
| LLM routing | Local-first routing, seven-tier model policy, local/OpenAI-compatible endpoint probes, fallback logging, cached/repeated-prompt controls, and a EUR 0 paid default. | A configured endpoint is not live-proven until it passes a bounded probe and validated task. Paid generation remains disabled by default. |
| Verification | Source-grounded answers, claim/evidence status, schema/deterministic validation, review routing, and verification-gated task completion. | Model confidence alone never authorizes a factual claim or consequential action. |
| Controlled execution | Reviewed API, script, Docker, Hermes, Odysseus, and OpenClaw adapter surfaces with bounded output, workspace/host allowlists, audit records, verification, and emergency stop. A curated external agent catalog also informs task planning. | Script/Docker control and external runtimes are disabled until explicitly configured; high-risk actions need approval. The catalog cannot install, enable, or execute a third-party project. |
| Proactive planning | Ambient scans identify stale work, blockers, approvals, open loops, contradiction candidates, and delegation opportunities. | Ambient mode is suggestion-first and cannot bypass approval, verification, leases, audit, or emergency stop. |
| Operations | nginx gateway, IDP, Postgres, Redis, Kafka, health/readiness, support bundle, doctor/reconcile commands, CI, Compose validation, and local smoke coverage. | Schedulers are in-process, single-node workers, not a distributed or HA worker platform. |

### Readiness Terms

- **Implemented**: code, persistence, API contract, and focused automated coverage exist in this repository.
- **Locally validated**: a bounded build, Compose, Postgres, gateway, or smoke check exercised the path. This is not third-party proof.
- **Live-proven**: a configured account, provider, or runtime completed a bounded approved end-to-end task on the target machine with audit and verification evidence.

No configured provider, runtime, dashboard state, or generated answer upgrades itself to live-proven.

### Status At A Glance

| Status | Current position |
| --- | --- |
| Canonical product | This Go/Angular/Postgres/Docker Compose repository. The separate Manus React/tRPC/MySQL implementation is reference-only. |
| Local platform | Compose configuration and the development-workspace stack have been exercised. A fresh Windows 11 clone-and-sign-in acceptance run is still required. |
| Core operating flow | Pursuits, workflows, task attempts, approvals, verification, audit, compact memory, source extraction, and ambient proposals are implemented and persisted. |
| Intake safety | New source, assistant, and ambient input is matched to an active pursuit or becomes a non-executable candidate. An approval-capable user must accept a candidate before its first governed workflow is created. |
| External accounts | Local/export ingestion and read-only GitHub sync are available. Live Gmail, Drive, Calendar, Trello, WhatsApp, browser, and similar account connectors are not implemented. |
| Models and runtimes | Local/free-first routing and guarded adapter surfaces exist. No provider or runtime is live-proven until its scoped probe, approved task, audit, and verification evidence exist. |
| External capability catalog | Candidate capabilities from Awesome AI Agents and OSS Insight Collections are curated into task planning and a read-only API. Candidates include local model inference/gateway, Postgres semantic retrieval, durable workflow, metrics, bounded browser/WASM verification, deterministic planning, and reviewed agent profiles. | A catalog entry is not an installed dependency or executable runtime. See [the catalog decision record](docs/agent-tool-catalog.md), [OSS Insight curation](docs/ossinsight-brain-curation.md), and the [102-collection screening ledger](docs/ossinsight-screening-ledger.md). |
| Production readiness | Not claimed. Clean-machine deployment, signed-in browser coverage, two-real-account isolation, and bounded real-provider/runtime exercises remain release gates. |

### Verification Snapshot

This is the current evidence boundary, not a feature checklist. Re-run the
target-machine checks before relying on a path for real work.

| Surface | Current evidence | Still required before operational trust |
| --- | --- | --- |
| Local Compose and gateway | The local services are running; `/`, `/control-center`, `/healthz`, and `/readyz` are served through nginx. Angular deep links return the application shell. | Fresh-clone Windows 11 run with a newly created `.env.local`. |
| Browser session | The unauthenticated session check returns `401`; Angular routes a browser without a refreshable session to `/login`. | Sign in as the first-run owner, open each primary screen, create and review one bounded low-risk workflow, and confirm session refresh in a real browser. |
| Go and Angular code | Backend and IDP unit tests, backend vet/build, frontend production build and headless unit tests, plus Compose config validation have been run. | End-to-end browser coverage and a two-real-account authorization exercise. |
| Sources and LLMs | Local/export ingestion and provider probes are implemented; GitHub read-only sync is available when configured. | A scoped live source import and a bounded local-model task with retained audit and verification evidence. |
| Runtimes and external effects | Script, Docker, Hermes, Odysseus, and OpenClaw adapters have bounded, approval-aware interfaces. | Explicit upstream installation, narrow allowlists, a reviewed dry run, and a verified approved task. |

## Current Safe Operator Flows

After authentication, an operator can:

1. Create a pursuit with an objective, desired outcome, completion definition,
   priority, risk, and autonomy setting.
2. Import authorized local/exported material, inspect extractions, and route
   actionable context into a pursuit or workflow.
3. Create plans, checklists, follow-ups, review items, and source-linked
   verification work through the workflow and task engines.
4. Review Robert-only decisions, blockers, next actions, approvals, runtime
   evidence, and completion conditions from the Command Dashboard or a pursuit.
5. Configure and probe a local model endpoint, then run a bounded validated
   task subject to the local/free policy.
6. Configure one narrow approved automation or agent runtime after its
   allowlists, workspace, timeout, and safety settings are explicitly reviewed.

The normal durable path is:

```text
assistant command, source intake, or ambient opportunity
  -> pursuit match
  -> active pursuit + persisted workflow
     or candidate pursuit + explicit acceptance
  -> bounded task plan/run
  -> verification and audit evidence
  -> completion, review, retry, or follow-up
```

Ambient opportunities use the same path. An opportunity matched to an active
pursuit may create or reuse a governed workflow. An unmatched opportunity, or
one matched only to a candidate pursuit, is recorded with its provenance and
waits for an approval-capable operator to accept the candidate. It does not
create an orphaned executable workflow.

If a pursuit linker is supplied without the native lifecycle router, derived
workflow creation is deferred and the source or conversation import remains
visible for repair. This fail-closed compatibility state creates no workflow;
it is not supported production wiring.

Direct `/task/*` planning and run sessions are useful for bounded operator
work, but their full plan/review history is process-local. When explicitly
scoped to a valid pursuit, HAI also persists a compact task-attempt projection;
the workflow remains the restart-safe execution ledger. Workflow-owned runs
retain the same pursuit context through planning and verification, but write
only that canonical workflow ledger rather than a duplicate task-attempt
record.

Refreshing a pursuit summary is documentation activity, not operational
progress. It cannot reset the pursuit's last-activity signal or remove stale
work from the command dashboard.

## Safety and Ownership

- Verified owner identity is required for the personal pursuit, workflow,
  source, memory, verification, task, review, ambient, HAI OS, and runtime
  mutation APIs. Client-supplied actor or approval fields are not trusted.
- The bundled IDP persists `owner`, `operator`, and `viewer` roles and signs
  that role into access tokens. Request headers never grant a role; the seeded
  `FIRST_RUN_ADMIN_EMAIL` account is promoted to `owner`, while registrations
  default to `operator`.
- Interactive APIs use the same signed-role boundary: viewers can inspect
  owner-scoped state, operators can plan and edit it, and execution or approval
  resolution requires approval capability. HTTP sync and due-work controls are
  scoped to the authenticated owner; only in-process schedulers operate across
  owners.
- The IDP refreshes a valid refresh-token session before resolving the user on
  protected routes, and nginx relays that refreshed cookie to the browser, so
  access-token expiry does not strand an active local session on the login
  screen or send the backend a stale credential.
- Gateway API authentication failures remain JSON `401` responses. Angular's
  session guard, rather than nginx rewriting API errors to HTML, directs the
  browser to `/login` when a session is no longer refreshable.
- Owner-scoped pursuit detail, dashboards, activity, evidence, decisions, and
  links filter legacy records that are not visible to the current owner.
- Pursuit-to-pursuit relationships are owner-scoped too, so authenticated users
  cannot create or view a cross-owner case reference through pursuit metadata.
  A pursuit cannot create a self-referential relationship, and related-pursuit
  navigation is available only for records visible in the current owner's scope.
- Pursuit auto-linking and candidate creation refresh their operational summary
  inside the same authenticated owner scope, so malformed legacy links cannot
  persist another user's workflow state into a personal pursuit.
- Runtime launch and stop records retain the authenticated initiating owner.
  Owner-scoped pursuits reject unknown or other-owner runtime evidence, and
  shared automation history cannot make one operator's runtime output visible
  in another operator's pursuit.
- Direct task-attempt projections are similarly re-checked during pursuit
  aggregation, so malformed or legacy cross-owner task records cannot expose
  task summaries, review state, or blocked reasons.
- High-risk communication, legal/government, financial, account, public-post,
  deletion, destructive-file, and broad-host actions require explicit approval
  and do not run from a generic transition or chat request.
- Auto-created pursuit candidates are not active operational work. Generic
  pursuit intake, planning, task attempts, and ambient opportunity routing
  keep them out of the executable path; an approval-capable user must use the
  separate candidate-acceptance action before HAI can create or unlock the
  governed workflow path.
- An assistant command that creates or selects a pursuit candidate returns an
  auditable review handoff instead of attempting a direct task plan. It links
  the candidate back to the chat result, asks Robert to accept or archive it,
  and creates no workflow, task attempt, runtime action, or side effect before
  the explicit candidate-acceptance action.
- An assistant command that creates or reuses active pursuit work stops at the
  governed workflow ledger. The workflow worker supplies its WorkflowID to the
  task engine, so planning, retries, verification, and runtime evidence are
  recorded once on the workflow instead of also creating a duplicate direct
  task attempt from the chat command.
- Pursuit decision resolution requires approval capability both in route
  registration and in the handler. Alternate or future route wiring cannot
  turn a non-approver's request into a workflow or decision audit event.
- Source, AI-chat, and ambient producers configured with pursuit correlation
  but without the native pursuit lifecycle router fail closed. Imported signals
  and proposed ambient opportunities remain visible for repair, but no workflow
  or executable work is created. The full router is the supported production
  integration path.
- Connected-source searches and extraction lists apply source ownership in the
  repository query. They use the implemented lexical index and do not claim a
  vector or embedding capability before a real local adapter is configured.
- The runtime registry enforces emergency stop at its own boundary, including
  direct Hermes, Odysseus, and OpenClaw registry execution calls.
- Runtime execution is constrained by enablement flags, allowlisted tools,
  hosts, paths, workspaces, timeouts, output limits, redacted audit records,
  and verification before completion.
- Stopping a runtime task requires an approval-capable role. Uploading,
  selecting, or refreshing the shared OpenClaw ecosystem requires an owner
  role because it changes the host-wide runtime configuration.
- The shared automation registry follows the same boundary: reads are role
  scoped, launch/stop actions require approval capability, health checks
  require write capability, and create/update/delete/reorder operations require
  an owner. Reordering uses `PATCH`, never a side-effecting `GET` request.
- Ownerless legacy workflows, sources, extractions, and imported conversation
  archives are read-compatible only for local-development compatibility.
  Authenticated users cannot adopt, delete, or mutate them. Ownerless scheduler
  work stays in-process and is not exposed as an operator action.

For route-by-route ownership behavior, see
[backend endpoint audit](docs/backend-endpoint-audit.md). For the broader
threat model, see [threat model](docs/threat-model.md).

## Deliberate Gaps

These capabilities are not bundled or live-proven by this repository:

- Gmail, Google Calendar, Google Drive, Trello, WhatsApp, browser, and other
  account OAuth/API integrations. Use authorized exports or a scoped bridge
  until a connector has its own live validation.
- Provider webhooks, local file watchers, a dedicated vector database, generic
  MCP, QwenPaw, browser automation, and desktop-agent execution.
- Hermes, Odysseus, and OpenClaw upstream installations. HAI provides guarded
  adapters, not the upstream software or unrestricted credentials.
- Paid LLM use, public posting, financial commitments, account changes,
  deletion, and unrestricted device control.
- A production migration framework, distributed workers, leader election,
  worker heartbeats, or high availability.
- Verified multi-user isolation on two real accounts. Owner scoping is covered
  in code and focused tests, but a real two-account exercise remains required
  before shared operation is trusted.
- A clean-machine, signed-in Windows 11 deployment journey. The local Compose
  and gateway path has been exercised; target-machine acceptance remains a
  required release gate.

The [external provider reality review](docs/external-provider-reality-review.md)
records the current integration truthfulness boundary.

## Architecture

```text
Angular dashboard
        |
nginx gateway + IDP session boundary
        |
Go API and operating engines
  |-- pursuits and workflow engine
  |-- task, approval, verification, and audit engines
  |-- memory and connected-source ingestion
  |-- local-first LLM router and provider probes
  |-- ambient planning and controlled runtime registry
        |
Postgres + Redis + Kafka
```

The local deployment targets Windows 11 with Docker Desktop. The backend uses
Go 1.21, Gin, Gorm, Postgres, and Sarama/Kafka; the frontend uses Angular 16
and ng-zorro-antd. The current data model relies on Gorm `AutoMigrate` and
`init.sql`; a production migration system remains future work.

## Quick Start

### Prerequisites

- Windows 11 with Docker Desktop, or another Docker Compose-capable environment.
- Git.
- Node.js 20 for frontend development outside Docker.
- Go 1.21 for backend development outside Docker. The Docker/CI toolchain is pinned to Go 1.21.13.

### Start the local stack

```powershell
Copy-Item .env.example .env.local
# Edit .env.local: set a unique FIRST_RUN_ADMIN_PASSWORD and BACKEND_API_SHARED_KEY.
docker compose --env-file .env.local -f docker-compose.local.yml config --quiet
docker compose --env-file .env.local -f docker-compose.local.yml up --build -d
docker compose --env-file .env.local -f docker-compose.local.yml ps
```

Open [http://localhost](http://localhost).

For a single-user local preview, set `LOCAL_LOGIN_BYPASS_ENABLED=true` and keep
`GATEWAY_HOST_BIND=127.0.0.1`. The login screen then shows **Open local
dashboard**, which creates a normal signed session for the configured first-run
owner. It is deliberately hidden by default and must never be enabled on a
LAN- or internet-exposed gateway.

The `.env.example` development defaults are:

```text
Email: noodzakelijkonline@gmail.com
Password: ChangeMe123!
```

Change `FIRST_RUN_ADMIN_PASSWORD` and `BACKEND_API_SHARED_KEY` before first use.
If the Postgres data volume already exists, changing first-run values does not
rewrite the existing account. Do not commit `.env.local`, Docker state,
database directories, uploaded material, frontend build output, or secrets.

### Optional Google sign-in and password recovery

The local password login works without external accounts. The login page only
offers Google sign-in or email recovery after their private credentials are set
in `.env.local`; it will not route an operator to a broken OAuth flow or claim a
reset code was delivered when no mail sender exists.

For a dedicated Google OAuth **web** client, register this redirect URI for the
local gateway:

```text
http://localhost/api/v1/auth/google/callback
```

Then set `GOOGLE_OAUTH_CLIENT_ID`, `GOOGLE_OAUTH_CLIENT_SECRET`, and
`GOOGLE_LOGIN_REDIRECT_URL` in `.env.local`, and recreate the IDP container.
The Gmail connected-source callback is separate and, if configured, uses
`GOOGLE_OAUTH_REDIRECT_URL`; it must also be explicitly registered with Google.

For reset emails, set `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`,
`SMTP_PASSWORD`, `SMTP_FROM`, and `SMTP_REQUIRE_STARTTLS=true` in `.env.local`.
Use a dedicated mailbox or provider app password over STARTTLS (typically port
587), never a primary mailbox password. Recreate the IDP container after
changing either integration:

```powershell
docker compose --env-file .env.local -f docker-compose.local.yml up -d --build idp frontend gateway
```

### Verify the local gateway

```powershell
docker compose --env-file .env.local -f docker-compose.local.yml ps
docker compose --env-file .env.local -f docker-compose.local.yml logs backend
curl.exe -i http://localhost/
curl.exe -i http://localhost/healthz
curl.exe -i http://localhost/readyz
curl.exe -i http://localhost/api/v1/llm/policy
```

Expected behavior:

- `/` serves the Angular shell.
- `/healthz` and `/readyz` reach the backend through nginx.
- Protected engine routes such as `/api/v1/llm/policy` return `401` without a
  signed session, not anonymous application data.

If port 80 is already in use, change the nginx port mapping in
`docker-compose.local.yml` from `\"80:80\"` to, for example, `\"8088:80\"`, then
open `http://localhost:8088`.

For the target-machine acceptance sequence, use
[fresh-clone dry run](docs/fresh-clone-dryrun.md). For diagnosis, use
[troubleshooting](docs/troubleshooting.md) and the in-product support bundle.

### Import local or exported material

1. Place authorized files under `connected-sources/`.
2. Open **Connected Sources** in the dashboard.
3. Create or select an export/local-folder source and keep **Local only** enabled.
4. Use a path relative to `connected-sources/`, for example `.`.

The backend mounts this root read-only. Paths escaping it are rejected. The
general importer accepts `.txt`, `.md`, `.markdown`, `.csv`, `.tsv`, `.json`,
`.yaml`, `.yml`, and `.log`; export connectors also support `.mbox`, `.eml`,
and `.ics` within the same allowlisted root.

## Dashboard Entry Points

| Route | Purpose |
| --- | --- |
| `/control-center` | Primary operational overview and bounded maintenance actions. |
| `/command-dashboard` | Robert-only decisions, open loops, source-backed context, memory-derived work, and unified approval actions for pursuits and linked workflows. |
| `/pursuits` | Long-running objectives with workflow, source, memory, verification, blocker, approval, activity, and related-pursuit links. |
| `/workflow-engine` | Work queue, approvals, quality gates, interruptions, retries, and follow-ups. |
| `/connected-sources` | Source configuration, sync history, extraction inspection, reindexing, pause/resume, and revocation. |
| `/memory` | Compact memory search, correction, archive, retrieval, and export controls. |
| `/llm-policy` | Provider/model configuration, budget/policy visibility, probes, routing, and fallback history. |
| `/ambient-brain` | Proactive opportunities, scan history, need-profile preferences, and decision handoffs. |
| `/task-blueprint` | Explicit bounded task planning, execution, validation, and review. |

These screens are authenticated operator surfaces. Technical logs and deep
diagnostics remain behind their relevant detail or audit views.

## API Overview

Backend engine APIs are served under `/api/v1` through the gateway. Principal
areas are:

- `/automation`: registered automations, launch/stop, health checks, and diagnostics.
- `/agent-runtimes`: runtime inventory, health, skill discovery, controlled stop, and OpenClaw ecosystem inspection.
- `/brain-catalog`: authenticated, read-only agent-project curation and activation boundaries; it has no install or enable operation.
- `/llm`: policy, probes, routing, generation, and redacted decision history.
- `/memory` and `/memory-engine`: compact memory, encrypted conversation import, search, and insights.
- `/sources`: source registry, connectors, sync, extraction management, search, and audit records.
- `/pursuits`: high-level objectives, matching, intake, navigable related-pursuit links, summary, review, decisions, evidence, blockers, next actions, approvals, activity, planning, and approval-gated candidate acceptance.
- `/workflow`: intake, state transitions, approvals, due work, follow-ups, quality/review state, and dashboard data.
- `/task`: bounded plans/runs, logs, and review queue.
- `/verification`: grounded answers and verification run history.
- `/ambient`, `/agent-cycle`, `/assistant`, and `/os`: proactive planning, controlled refreshes, command bridge, and operating-system summary.

Use [Swagger](docs/swagger.yaml) and the route tests in
`backend/internal/router/` for exact request/response contracts.

## Controlled Models and Runtimes

### Models

The router chooses the cheapest suitable model, not mechanically the cheapest
model. Its policy prioritizes local/free availability, task difficulty,
validation, fallback history, quotas, and the daily budget. Paid calls are
disabled by default with a EUR 0 budget; request JSON cannot self-approve paid
or approval-required use.

Supported configuration families include Ollama, a first-class local
`llama.cpp` server, LM Studio, other configured OpenAI-compatible local
servers, and configured free/freemium providers. `LLAMA_CPP_BASE_URL` accepts
only `localhost`, loopback, or `host.docker.internal`; configure
`LLAMA_CPP_MODEL_ID` for the model loaded by `llama-server`. Model
catalog entries cover Qwen, DeepSeek, Llama, Mistral/Mixtral, Gemma, Phi, and
other configured provider models. Provider status must be read as configuration
and probe history, not as a live-service guarantee.

### Metrics

Prometheus telemetry is disabled by default. To enable it for a local collector,
set `HAI_PROMETHEUS_ENABLED=true` and a distinct `HAI_PROMETHEUS_TOKEN`; HAI
then exposes a bearer-token-protected `/metrics` endpoint. The exporter records
only matched-route request counts and latency, never source content, prompts,
identities, record IDs, or credentials as labels. A Prometheus server and its
retention policy remain operator-managed.

### Agent runtimes

Hermes, Odysseus, and OpenClaw are optional controlled adapters. HAI can inspect
their configured capabilities and run a bounded approved task only after the
operator installs the upstream runtime, configures scoped credentials/workspace
state, enables the adapter, and validates it. HAI does not bundle these tools,
send messages through them, control browsers, create cron jobs, or bypass their
or HAI's security boundaries.

API, script, and Docker adapters have the same default posture: disabled until
explicitly allowlisted and configured. The emergency stop blocks runtime
registry execution even when an adapter is invoked directly.

## Developer Checks

```powershell
# Backend (use Docker when Go is not installed locally)
docker run --rm -v hai-go-module-cache:/go/pkg/mod -v "${PWD}/backend:/workspace" -w /workspace golang:1.21.13 go test ./...
docker run --rm -v hai-go-module-cache:/go/pkg/mod -v "${PWD}/backend:/workspace" -w /workspace golang:1.21.13 go vet ./...
docker run --rm -v hai-go-module-cache:/go/pkg/mod -v "${PWD}/backend:/workspace" -w /workspace golang:1.21.13 go build ./...

# Identity and nginx configuration services
Set-Location idp
go test ./...
go build ./...
Set-Location ..\nginx-config-manager
go test ./...
go build ./...

# Frontend
Set-Location ..\frontend
npm.cmd ci
npm.cmd run build
npx ng test --watch=false --browsers=ChromeHeadlessNoSandbox

# Compose contract
Set-Location ..
docker compose --env-file .env.example -f docker-compose.local.yml config --quiet
```

With local Go installed, run the backend commands from `backend/`, and the IDP
and nginx-config-manager commands from their respective directories. These are
the same build-and-test surfaces required by CI. The critical-path smoke is
`scripts/smoke-critical-path.sh` from a Bash-capable shell with its prerequisites.

The repository's verification evidence is in:

- [completion matrix](docs/codex-goal/completion-matrix.md)
- [final verification report](docs/codex-goal/final-verification-report.md)
- [fresh-clone dry run](docs/fresh-clone-dryrun.md)
- [external provider reality review](docs/external-provider-reality-review.md)
- [technical debt](docs/technical-debt.md)

These reports distinguish exercised local behavior from unproven real-world
integrations. Treat target-machine and provider-specific checks as release
gates, not paperwork.

## Repository Layout

```text
backend/                 Go API and HAI engines
frontend/                Angular dashboard
idp/                     Identity provider service
nginx-config/            Gateway configuration used by local Compose
nginx-config-manager/    Generated route-config manager; Docker socket disabled by default
automation-scripts/      Read-only allowlisted script mount
connected-sources/       Read-only local/export ingestion root
browser-extension/       Explicit user-authorized conversation capture
scripts/                 Smoke and operational verification scripts
docs/                    Architecture, runbooks, evidence, audits, and roadmap
.github/workflows/       CI pipeline
docker-compose.local.yml Windows/local-first Compose topology
.env.example             Environment template; copy to untracked .env.local
generic-auto/            Legacy service, not the canonical HAI engine
gate/                    Legacy gateway/config area; local Compose uses nginx-config/
```

## Further Documentation

- [Operator runbook](docs/operator-runbook.md)
- [User guide](docs/user-guide.md)
- [HAI Personal AI Operating System blueprint](docs/hai-personal-ai-operating-system.md)
- [Universal task success engine](docs/universal-task-success-engine.md)
- [Connected-source ingestion](docs/connected-source-ingestion-extraction.md)
- [Verification and anti-hallucination policy](docs/anti-hallucination-verification.md)
- [Source-grounded answer engine](docs/source-grounded-answer-engine.md)
- [Automation Control Center](docs/automation-control-center-blueprint.md)
- [Release process](docs/release-process.md)
- [Privacy impact assessment](docs/privacy-impact-assessment.md)

## License

See [LICENSE](LICENSE).
