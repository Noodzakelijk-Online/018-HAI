# 018-HAI

018-HAI is a local-first Human Autonomous Intelligence Shell: a governed
Personal AI Operating System for turning authorized source material, durable
memory, workflows, approvals, and controlled execution into inspectable work.

The canonical product is this repository's Go, Angular, Postgres, and Docker
Compose stack. It is not an unrestricted desktop agent: planning, execution,
verification, and approval are separate; external effects remain blocked until a
reviewed runtime, policy, and evidence path are configured.

> **Current repository state, evidence reviewed through 2026-08-04:** this repository
> implements a governed local operating layer, including the Angular dashboard,
> Go engines, IDP, Compose topology, pursuit/workflow routing, persistence, and
> safety gates. On the development workspace used for this review, the Compose
> services were healthy, nginx served `/` and Angular deep links such as
> `/control-center`, gateway health routes responded, and protected APIs rejected
> unsigned sessions. Those observations are local-environment evidence, not a
> claim that every Windows machine or account integration is ready. Backend/IDP
> tests, frontend production build and 226 unit tests, Compose validation, and a
> Postgres-backed critical-path smoke have been exercised. A signed-in browser
> acceptance run on the local Windows Compose stack also completed source intake,
> pursuit creation, exact runtime selection, durable approval, a real read-only
> backend probe, and terminal verification. A clean-clone Windows run and any
> newly configured third-party account, paid model, browser-control, mutable
> runtime, or broad-host-control journey remain release gates.

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

This is an implementation and acceptance-tested local milestone. It does not
replace the release gates in the verification snapshot below: a fresh-machine
browser flow, newly configured provider acceptance, local-model task, and each
mutable runtime dry run are still required before relying on those paths for
personal work.

## Product Boundary

HAI is the product. A pursuit is the high-level objective or case that connects
the systems below; it is not a second product or a replacement workflow engine.
The earlier Manus React/tRPC/MySQL implementation is reference material only.
Useful behavior from it should be ported deliberately into this stack rather
than maintained in parallel.

See [ADR 0001](docs/architecture-decision-records/0001-canonical-stack-and-readiness.md)
for the canonical-stack decision.

## Framework Registry

The [Framework Registry](docs/framework-registry.md) defines HAI's versioned,
owner-scoped contract for selecting the smallest suitable set of planning,
reasoning, governance, domain, and evaluation frameworks for a task. Its
implemented `framework-catalog-v2` contains 55 records (54 at `1.0.0` and the
evaluation framework at `1.1.0`; 50 active and five experimental), mandatory
safety overlays, deterministic `selector-v5` selection with enforced task-risk
ceilings, owner-scoped preferences, Constitution lifecycle, authority ceilings,
reproducibility digests, API/UI/task/workflow integration, and versioned
pre-phase migrations.

The [Framework Operating Contract Matrix](docs/framework-operating-contract-matrix.md)
maps all 55 research families to enforced, structured, or catalog-only
behavior and states the remaining live-system boundary for each.

Durable task plans and runs are projected into an append-only, owner-scoped
operational life graph. The graph links tasks to projects, pursuits,
workflows, verified memories, and outcomes with typed relations and source
digests. Preview requests never write graph state, local-only records remain
hidden until explicitly requested on the local governance screen, and graph
records cannot grant approval or execution authority.

Connected-source sync uses the same graph boundary. After a raw item and its
extraction are durably stored and indexed, HAI appends an immutable document
record linked to the registered source and project. Owner identity, content
digest, sensitivity, verification state, and local-only policy are retained.
Graph outages appear as audited sync warnings without discarding a successful
email, file, Trello, Drive, GitHub, or other source ingestion. Operator
corrections and archive changes create new observations rather than rewriting
history.

Selector v4 also produces a durable Chief-of-Staff operating contract: all
matched life domains, needs state, freshness-aware human capacity, verified
agent cards with explicit identity/capability/access/cost/health/revocation
fields, authority-bounded delegation contracts, replay-resistant typed
communication, coordination mode, exact per-action autonomy decisions, stop conditions,
outcome monitoring, and an operating-contract digest. Workflow due dates flow
into delegation deadlines; every delegation defaults to zero financial
authority. The Advanced registry view exposes these details without turning
the Basic view into a diagnostic wall.

Task planning also applies a deterministic, advisory-only resource schedule.
Conservative step durations, dependencies, deadlines, paid/token/tool budgets,
and owner-confirmed Life Ops capacity are evaluated before execution. When an
owner has an active read-only Google Calendar source, opaque busy intervals are
subtracted from that capacity; cancelled and transparent/free events are
ignored. Unknown or stale capacity requires review, confirmed zero remaining
capacity blocks the plan, and Calendar read failures fail closed. The Task
Blueprint shows the feasibility result, reserved source-linked intervals,
scheduled steps, blockers, and approval flags. This path cannot move events,
consume approval, or grant execution authority.

Catalog lifecycle and owner-effective state are separate. `active` records are
enabled by default; `experimental` records are disabled by default and need an
owner opt-in plus a direct match; `deprecated` records are excluded from
selection. `disabled` is an owner preference, not a fourth catalog lifecycle
status. An owner can enable an experimental record or disable an ordinary
active record, but cannot disable a protected safety overlay.

The built-in fallback Constitution has the exact source
`builtin-robert-constitution-v1:v1`. Registry selection records retain the
catalog version/digest, selector version, effective-preference digest,
Constitution digest/source, selected framework versions, reasons, evidence
requirements, and authority ceiling. Protected overlays cannot be disabled;
owner preferences may only enable an experimental record, pin a relevant
record, lower autonomy, or add bounded safe adaptations.

Constitution activation is owner-only and requires the exact, case-sensitive
confirmation `ACTIVATE CONSTITUTION` with no leading or trailing whitespace,
plus a redacted approval note of at least 10 characters. Ordinary Constitution
prose is immutable, versioned governance context; it is not executable policy.
Only code-owned protected controls and valid restrictive `HAI-RULE v1`
`deny-capability`, `require-approval`, or `authority-ceiling` entries are
machine-enforced. No Constitution entry can grant authority.

Framework records are decision metadata, not installed tools or granted
authority. A named agent framework, workflow platform, memory store, policy
engine, or evaluation product is only a candidate implementation until its
adapter is configured and passes security, capability, integration, audit, and
real-world verification gates.

The Go routes, Angular `/framework-registry` page, and nginx authenticated API
allowlist are wired together. Repository tests cover the component, service,
route, permission, and static gateway contracts. A clean-machine signed-in
browser exercise remains environment-dependent acceptance evidence.

Viewers can inspect the owner-scoped registry and selection history. Operators
can also request and persist a selection recommendation. Only an owner can
change framework preferences, create a Constitution draft, activate a
Constitution, run an approval-gated task, or resolve a task review item.

## What Is Implemented

| Area | Implemented capability | Important operating boundary |
| --- | --- | --- |
| Operator UI | Angular onboarding, Quick Capture, Control Center, Command Dashboard, HAI OS, pursuits, workflow exceptions, sources, memory, LLM policy, grounded answers, task planning, and the Framework Registry. | A dashboard card is operational visibility, not proof that an external action occurred. |
| Pursuits and workflows | Durable pursuits, workflow states, checklists, decisions, open loops, blockers, follow-ups, approvals, review queues, retries, task-attempt evidence, read-only VA delegation briefs, ambient opportunity routing, navigable related-pursuit links, owner-scoped internal reminder proposals, an append-only reminder preparation/decision ledger, owner-authorized internal reminder delivery receipts, and calendar-aware resource/dependency planning. | In the canonical routed stack, new source, assistant, or ambient context is matched to an active pursuit first; otherwise it becomes an approval-gated candidate, not executable work. Resource plans and reminder projections are advisory and owner-scoped. A preparation request or approval alone is evidence only. After a separate exact owner authorization, the durable workflow worker may create one source-bound internal proactivity signal with an idempotent receipt. It cannot create Calendar events, send email or messages, invoke providers, execute follow-ups, or mutate the source checklist. |
| Memory and knowledge | Compact memory, retrieval, deduplication, correction, export/deletion planning, provenance, encrypted user-authorized conversation capture, and source/extraction links. | Raw imported conversations are not automatically promoted to trusted facts. |
| Source ingestion | Allowlisted local files; MBOX/EML, ICS, Trello JSON, WhatsApp exports, Odoo/HERP snapshots, normalized JSON feeds, synced document folders, read-only GitHub, Gmail, Google Drive, Google Contacts, Google Calendar, Trello, ShareT, LARO, and Worker Control sync. | Gmail and Trello have bounded live acceptance evidence but are unconfigured by default. ShareT has a bounded, paginated, read-only adapter with contract coverage; its live token activation remains operator-gated. Drive, Contacts, and Calendar have unit/contract coverage but still need real sandbox acceptance runs. Imported contacts remain review candidates. Meaningful events within 14 days may create source-backed preparation work; past events stay context-only. Overlaps within 30 days create stable review-gated conflict records, while moved or cancelled events retract stale work. No Calendar or ShareT write-back exists. WhatsApp and browser accounts remain export/local-folder paths. |
| LLM routing | Local-first routing, seven-tier model policy, local/OpenAI-compatible endpoint probes, fallback logging, cached/repeated-prompt controls, and a EUR 0 paid default. | A configured endpoint is not live-proven until it passes a bounded probe and validated task. Paid generation remains disabled by default. |
| Verification | Source-grounded answers, claim/evidence status, schema/deterministic validation, review routing, and verification-gated task completion. | Model confidence alone never authorizes a factual claim or consequential action. |
| Controlled execution | Reviewed API, script, Docker, Hermes, Odysseus, and OpenClaw adapter surfaces with bounded output, workspace/host allowlists, audit records, verification, emergency stop, and an internal action-bound approval proof before mutating side effects. | Direct mutating HTTP launches cannot create the proof and fail closed. The approved task-review path issues a short-lived proof signed by a stable deployment key; PostgreSQL atomically records its one allowed consumption across restarts and backend instances. External runtimes remain disabled until explicitly configured and validated, and external side effects still require postcondition/idempotency evidence. |
| Optional local runners | Disabled-by-default Compose profiles for aggregate security scans, no-tool planning drafts, selected-folder document extraction, and disposable patch proposals. | They publish no host ports and have private networks, read-only mounts, and resource limits. Configuration or container health is not live proof; each real snapshot, model, or document path still needs retained approval, audit, and verification evidence. |
| Proactive planning | Ambient scans identify stale work, blockers, approvals, open loops, contradiction candidates, and delegation opportunities. Governance Control records owner `accept`, `dismiss`, bounded `snooze`, indefinite `suppress`, and `resume` feedback in an immutable owner-scoped ledger that changes later attention evaluation. | Ambient mode is suggestion-first and cannot bypass approval, verification, leases, audit, or emergency stop. Attention feedback has `canExecute:false`, grants no delivery or execution authority, and invokes no notification or external effect. |
| Advisory ambient outcome monitor | Governance Control can bind an existing outcome indicator to one of three fixed read-only local collectors: `workflow_open_loop_count`, `workflow_verified_completion_count`, or `overdue_commitment_count`. A durable singleton sweep leases due targets, appends immutable source-digested observations and run receipts, composes them into the existing outcome-evaluation service, and may surface an owner-scoped proactivity inbox decision. | The monitor is `advisory_monitor_only`. It cannot execute or deliver work, notify anyone, write Calendar data, mutate a workflow, authorize a mandate, or mutate learning. It reads only canonical local ledgers and accepts no caller-supplied SQL, URL, script, expression, or arbitrary tool instruction. Live external-account correctness and target-machine acceptance remain separate gates. |
| Operations | nginx gateway, IDP, Postgres, Redis, optional Kafka-compatible event bus, health/readiness, support bundle, doctor/reconcile/migrate commands, versioned SQL migrations, a durable job runner (persisted retry + crash recovery), CI, Compose validation, and local smoke coverage. | **Source, workflow, and ambient** scheduling all run on the durable worker: each is a self-rescheduling singleton job with backoff retry and lease-based crash recovery, and each falls back to its in-process ticker (logging that it did) if the queue is unreachable. The workflow schedule also consumes only separately owner-authorized internal reminder deliveries; each delivery is revalidated, source-bound, idempotently receipted, and recorded as a local proactivity signal. No external notification, Calendar write, provider invocation, or follow-up execution is performed. This is a single-node worker, not a distributed or HA platform. |

### Readiness Terms

- **Implemented**: code, persistence, API contract, and focused automated coverage exist in this repository.
- **Locally validated**: a bounded build, Compose, Postgres, gateway, or smoke check exercised the path. This is not third-party proof.
- **Live-proven**: a configured account, provider, or runtime completed a bounded approved end-to-end task on the target machine with audit and verification evidence.

No configured provider, runtime, dashboard state, or generated answer upgrades itself to live-proven.

### Advisory Outcome Monitor

The outcome monitor is a local evidence bridge, not an autonomous executor.
An administrator configures a target against an existing owner/workspace outcome
and indicator in Governance Control. Read-capable roles may inspect targets and
their immutable observations/runs; write-capable roles may request a bounded
due pass; administrator permission is required to create, enable/disable, or
recover target state.

The guarded API surface is under
`/api/v1/outcome-evaluations/workspaces/:workspaceId`:

- `GET|PUT /outcomes/:outcomeId/monitor` lists or creates a target;
- `PATCH /outcomes/:outcomeId/monitor/:targetId/enabled` pauses or resumes it;
- `GET /outcomes/:outcomeId/monitor/:targetId/observations` and `/runs` expose
  bounded immutable history;
- `POST /monitors/run-due` performs a bounded advisory pass; and
- `POST /monitors/recover` releases only expired leases.

The durable scheduler is configured with
`OUTCOME_MONITOR_SCHEDULER_ENABLED`, `OUTCOME_MONITOR_SWEEP_SECONDS`,
`OUTCOME_MONITOR_POLL_SECONDS`, `OUTCOME_MONITOR_LEASE_SECONDS`,
`OUTCOME_MONITOR_SCOPE_LIMIT`, and `OUTCOME_MONITOR_BATCH_LIMIT`. Invalid or
out-of-range values fall back to bounded defaults documented in
`.env.example`. Disabling the scheduler does not remove the records or grant a
different execution path.

Durable workers process work already due when HAI starts, then use five-minute
idle polls by default. This keeps a local installation quiet when no source,
workflow, ambient, or outcome work is pending; explicit manual passes and
normal recurring due times remain governed by their existing schedules.

Required acceptance before relying on this path includes exact replay without
duplicate observations or inbox items, two-owner isolation, active-lease
fencing and expired-lease recovery, disable behavior, and proof that a monitor
pass causes no task/runtime execution, notification, message delivery,
Calendar write, workflow mutation, mandate authorization, or learning update.
Repository implementation and focused tests do not by themselves prove
real-world source correctness or production readiness.

### Optional Runtime Profiles

Recovered security, agent-planning, document, and patch-proposal helpers are
now wired as isolated, disabled-by-default Compose profiles. The ordinary
`docker compose up` does not start them. See
[Optional Runtime Profiles](docs/optional-runtime-profiles.md) for the exact
profile names, environment contract, resource ceilings, read-only mount rules,
activation commands, and evidence required before any capability is described
as live-proven.

MLflow and OpenLIT remain bridge-only integrations to separately operated
local/private services; this repository does not silently install or expose
either observability server.

### Status At A Glance

| Status | Current position |
| --- | --- |
| Canonical product | This Go/Angular/Postgres/Docker Compose repository. The separate Manus React/tRPC/MySQL implementation is reference-only. |
| Local platform | The current Windows Compose workspace has a retained browser acceptance run covering password login, read-only local source registration and sync, explicit pursuit creation, governed high-risk workflow intake, durable approval, and one bounded worker pass (2026-08-04). A separate fresh-clone Windows 11 acceptance run is still required. |
| Core operating flow | Pursuits, workflows, task attempts, approvals, verification, audit, compact memory, source extraction, and ambient proposals are implemented and persisted. |
| Intake safety | New source, assistant, and ambient input is matched to an active pursuit or becomes a non-executable candidate. An approval-capable user must accept a candidate before its first governed workflow is created. |
| External accounts | Local/export ingestion and read-only GitHub sync are available. Gmail and Trello have bounded live acceptance evidence. Google Drive, Google Contacts, and primary Google Calendar have separate read-only OAuth adapters with bounded backfills and native change/sync cursors, but no retained live sandbox acceptance evidence yet. Contact candidates require review. Calendar event times feed deterministic due dates, bounded preparation proposals, and overlap review; moving or cancelling source events retracts stale Calendar-derived work without deleting obligations. These paths cannot write back. WhatsApp and browser connectors are not live. |
| Models and runtimes | Local/free-first routing and guarded adapter surfaces exist. No provider or runtime is live-proven until its scoped probe, approved task, audit, and verification evidence exist. |
| Production readiness | Not claimed. Clean-machine deployment and bounded acceptance for each newly enabled provider or mutable runtime remain release gates. |

### Verification Snapshot

This is the current evidence boundary, not a feature checklist. Re-run the
target-machine checks before relying on a path for real work.

| Surface | Current evidence | Still required before operational trust |
| --- | --- | --- |
| Local Compose and gateway | The local services are running; `/`, `/control-center`, `/healthz`, and `/readyz` are served through nginx. Both health probes are intentionally public; protected `/api/v1/*` engine routes still require a signed session. Angular deep links return the application shell. | Fresh-clone Windows 11 run with a newly created `.env.local`. |
| Browser session | The unauthenticated session check returns `401`; Angular routes a browser without a refreshable session to `/login`. A signed-in Playwright acceptance run completed source intake, pursuit creation, exact runtime selection, durable approval, read-only execution, terminal verification, and creation of an immutable completion attestation. | Repeat the acceptance run on each release target and add retained coverage for any new mutable or external action. |
| Go and Angular code | The full Go suite, frontend production build, 380 headless Angular tests, migration-chain contract through `0059`, isolated PostgreSQL reminder-ledger tests, live workflow-repository PostgreSQL test, and signed-in browser reminder prepare/approve/persist/cleanup acceptance pass. Migrations `0046` and `0047` define the append-only owner-scoped preparation/decision ledger; `0055` through `0057` add the separately authorized internal-delivery receipt ledger. | Keep these gates green and close the existing CSS/initial-bundle budget warnings before a production release. The browser exercise proves preparation and approval persistence only. It does not prove a scheduled internal delivery or any Calendar write, message delivery, provider invocation, or follow-up execution. |
| Sources and LLMs | Local/export ingestion, provider probes, GitHub sync, and bounded Gmail/Trello acceptance evidence exist. | A scoped local-model task and any newly configured account need their own retained audit and verification evidence. |
| Runtimes and external effects | Script, Docker, Hermes, Odysseus, and OpenClaw adapters have bounded, approval-aware interfaces. The local registry-to-read-only-API path is acceptance-tested with deterministic receipt verification. | Explicit upstream installation, narrow allowlists, a reviewed dry run, and a verified approved task for every mutable or external adapter. |

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

For an open checklist item with `ReminderAt`, an authenticated owner may first
read the current reminder proposal and then append a narrowly scoped
`internal_notification` preparation request. An approval-capable owner may
append `approved`, `rejected`, `needs_clarification`, or `revoked` decision
evidence. Requests and decisions are immutable, digest-bound, owner-scoped,
idempotent, time-limited, and always return `canExecute:false`. Preparation and
approval do not create a Calendar event, send a notification, email, or other
message, call a provider, run a follow-up, or change the workflow/checklist.
After a separate owner authorization, the durable worker may emit one internal
proactivity signal and an immutable delivery receipt. It cannot deliver to
Calendar, email, chat, or any provider. Any external delivery path would still
require its own authorization, effect ledger, provider acceptance, and
postcondition proof.
The two reminder mutation routes bypass the legacy process-local
`Idempotency-Key` rejection cache and defer replay/conflict handling to the
durable owner-scoped ledger. Preparation uses the body `idempotencyKey`; a
decision uses its canonical request digest and current decision-chain tip.

If a pursuit linker is supplied without the native lifecycle router, derived
workflow creation is deferred and the source or conversation import remains
visible for repair. This fail-closed compatibility state creates no workflow;
it is not supported production wiring.

Direct `/task/*` planning and run sessions are useful for bounded operator
work. Owner-scoped completion-plan snapshots, review items, and review
decisions are persisted by `pre/0004_task_state_storage`; completion snapshots
and decisions are append-only, while review-item provenance is immutable and
only its governed state may advance. An approved review replays the exact
stored action; a validated result becomes `completed`, while an execution error
or failed validation returns the item to `needs_review`.

When a direct task is explicitly scoped to a valid pursuit, HAI also persists
a compact task-attempt projection. The pursuit/workflow ledger remains the
canonical restart-safe record for workflow-owned runs; those runs retain the
same pursuit context through planning and verification without writing a
duplicate direct task-attempt projection. Durable review storage also provides
a manual, dry-run-first reconciliation action for an item left `approved` by a
process failure. It never repeats the side effect: linked durable evidence can
close a verified completion, while an unproven outcome returns to
`needs_review`. There is deliberately no automatic recovery worker, so
operators must inspect evidence and follow the
[operator runbook](docs/operator-runbook.md).

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
- Connected-source searches and extraction lists apply source ownership and a
  fail-closed source-revocation barrier. Revoked source rows and audit history
  remain administratively inspectable, but their cached extractions and stale
  semantic embeddings are excluded immediately from task context. Lexical
  retrieval is always available; vector retrieval is claimed only when the
  configured local semantic adapter is healthy.
- The runtime registry enforces emergency stop at its own boundary, including
  direct Hermes, Odysseus, and OpenClaw registry execution calls.
- Runtime execution is constrained by enablement flags, allowlisted tools,
  hosts, paths, workspaces, timeouts, output limits, redacted audit records,
  and verification before completion.
- Built-in system processes cannot assign themselves a lower risk, authority,
  autonomy, reversibility, cost, tool, runtime, or operation classification.
  The local safe worker, task-runtime launcher, and local model-maintenance
  worker each have an exact server-owned workload policy; an unknown system
  identity or any policy mismatch is denied before Constitution, mandate, or
  approval evaluation. The matched policy ID is retained in authorization
  evidence and rechecked immediately before one-time receipt consumption.
- Mutating API, script, Docker-start, and agent-runtime actions additionally
  require an internal HMAC-signed approval proof bound to the owner, automation,
  exact action digest, scope, and recorded approval source. Proofs default to a
  five-minute lifetime, are single-use, and are issued only by the trusted
  approved task-review path. Read-only API `GET`/`HEAD` probes are exempt from
  the proof but not from ordinary authentication, enablement, allowlists, audit,
  or safety policy.
- Production approval-proof signing uses the explicit
  `HAI_APPROVAL_PROOF_SIGNING_KEY`; startup fails closed when the key is missing
  or shorter than 32 bytes. Consumption is an owner-scoped, append-only
  PostgreSQL claim, so replay protection survives restart and coordinates
  multiple backend instances. Rotating the key invalidates unexpired proofs.
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

- Live WhatsApp, browser, and other unlisted account OAuth/API integrations.
  Gmail, Google Drive, Google Contacts, Google Calendar, and Trello read-only connectors exist
  but remain unconfigured by default; every configured account still needs its
  own bounded acceptance evidence before operational trust.
- Provider webhooks, local file watchers, a dedicated vector database, generic
  MCP, QwenPaw, browser automation, and desktop-agent execution.
- Hermes, Odysseus, and OpenClaw upstream installations. HAI provides guarded
  adapters, not the upstream software or unrestricted credentials.
- Paid LLM use, public posting, financial commitments, account changes,
  deletion, and unrestricted device control.
- Distributed workers, leader election, worker heartbeats, or high
  availability. Versioned pre/post SQL migrations are implemented, but
  clean-clone and rollback acceptance still belong in each target release.
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
Postgres + Redis + optional event bus
```

The local deployment targets Windows 11 with Docker Desktop. The control-plane
backend, IDP, and nginx configuration manager use Go 1.25.13 and share an
executable CI alignment contract. They use Gin, Gorm, Postgres, and
Sarama/Kafka when the optional event-bus profile is enabled. The frontend uses Angular 20 and ng-zorro-antd 20.
Versioned SQL migrations are the schema source of truth and `DB_AUTOMIGRATE`
defaults to `false`. Startup applies pre-phase migrations, optionally runs
development-only AutoMigrate when explicitly enabled, then applies
post-phase migrations. See
[migration safety](docs/migrations.md).

## Quick Start

### Prerequisites

- Windows 11 with Docker Desktop, or another Docker Compose-capable environment.
- Git.
- Node.js 20 for frontend development outside Docker.
- Go 1.25.13 for control-plane backend, IDP, and nginx-config-manager
  development outside Docker. Their modules, Docker builders, and CI toolchains
  are checked for version alignment.

### Start the local stack

```powershell
./scripts/initialize-windows.ps1
docker compose --env-file .env.local -f docker-compose.local.yml config --quiet
docker compose --env-file .env.local -f docker-compose.local.yml up --build -d
docker compose --env-file .env.local -f docker-compose.local.yml ps
```

The initializer prompts for the first-run owner email and password, generates
the four production-required backend signing/encryption secrets, and writes an
ignored loopback-only `.env.local`. For a non-Windows shell, copy the template,
set a real `FIRST_RUN_ADMIN_PASSWORD`, then run
`./scripts/generate-secrets.sh >> .env.local` before starting Compose.

`docker compose --env-file .env.local up --build -d` is equivalent. The
default `docker-compose.yml` intentionally delegates to the same local,
source-built stack. It does not pull the old `jacksonbarreto/*` images or start
the retired multi-broker Kafka topology.

Open [http://localhost:8088](http://localhost:8088) with the default
configuration. If you deliberately override `GATEWAY_HOST_PORT`, use that port
instead.

For explicitly configured, guarded public HTTPS access, see
[Governed ngrok cloud access](docs/ngrok-cloud-access.md). The tunnel is
disabled by default and never publishes the local gateway directly.

### Windows 11 installer

For a product-style local installation, build the Inno Setup executable and
use its Start menu shortcuts rather than manually operating Compose. The
installer keeps the source-built stack loopback-only, stores first-run secrets
outside the application directory, and refuses to start a competing HAI stack.
It also starts the separate loopback-only A2A planning connector at
`http://127.0.0.1:8091` by default. That connector is never served through the
dashboard gateway or the optional ngrok tunnel. See
[Windows installer](docs/windows-installer.md).

### Desktop resource defaults

The ordinary local stack now applies explicit memory, CPU, and process ceilings
to every always-on service: backend, frontend, IDP, gateway, both Postgres
databases, and Redis. The Redpanda broker and nginx configuration consumer are
an opt-in `event-bus` profile, so an idle local HAI installation does not pay
for them. Optional model, evaluation, document, and agent runners remain
profile-gated and are not started by the standard command.

The limit variables are grouped in `.env.example` (`BACKEND_MEMORY_LIMIT`,
`POSTGRES_AUTOMATION_MEMORY_LIMIT`, and similar). Change them only for an
observed workload, then validate the rendered configuration before restarting:

```powershell
docker compose --env-file .env.local -f docker-compose.local.yml config --quiet
```

To enable Kafka-compatible account/event delivery and dynamic gateway
configuration deliberately, set `HAI_EVENT_BUS_ENABLED=true` in `.env.local`
and start the additional profile:

```powershell
docker compose --env-file .env.local -f docker-compose.local.yml --profile event-bus up -d
```

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

Change `FIRST_RUN_ADMIN_PASSWORD` before first use and generate every required
secret with the command above. If the Postgres data volume already exists,
changing first-run values does not rewrite the existing account. Do not commit
`.env.local`, Docker state, database directories, uploaded material, frontend
build output, or secrets.

### Optional Google sign-in and password recovery

The local password login works without external accounts. The login page only
offers Google sign-in or email recovery after their private credentials are set
in `.env.local`; it will not route an operator to a broken OAuth flow or claim a
reset code was delivered when no mail sender exists.

For a dedicated Google OAuth **web** client, register this redirect URI for the
local gateway:

```text
http://localhost:8088/api/v1/auth/google/callback
```

Then set `GOOGLE_OAUTH_CLIENT_ID`, `GOOGLE_OAUTH_CLIENT_SECRET`, and
`GOOGLE_LOGIN_REDIRECT_URL` in `.env.local`, and recreate the IDP container.
The Gmail, Drive, Contacts, and Calendar connected-source callback is separate. Register
`http://localhost:8088/api/v1/sources/oauth/google/callback`, set it as
`GOOGLE_OAUTH_REDIRECT_URL`, and enable both APIs you intend to use. Each source
requests only its own read-only scope. Also set independent
`HAI_OAUTH_TOKEN_ENCRYPTION_KEY` and `HAI_OAUTH_STATE_SIGNING_KEY` values; HAI
does not fall back to JWT or backend secrets. Google redirects the browser, so a
public tunnel is not required for this local callback.

For optional public access through the governed ngrok profile, register the
equivalent callback URIs for the reserved `HAI_NGROK_URL` instead. Set each
enabled callback to that exact public origin and path. The tunnel launcher
rejects localhost, a different host, or a mismatched path while a Google flow
is configured, so remote sign-in and source consent cannot fail after the
tunnel has started.

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
- `/healthz` and `/readyz` reach the backend through nginx without a session.
  They are intentionally public liveness/readiness probes and return backend
  health JSON (`/readyz` uses HTTP `200` or `503` according to readiness).
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

### Import Phase 2 local feed records

Phase 2's controlled background worker uses a separate intake boundary. Put
operator-reviewed JSON feed files under `phase2-feeds/`, set
`HAI_PHASE2_FEED_FILES` to their comma-separated filenames in `.env.local`,
then restart the backend. The feed folder is mounted read-only. Any verified
safe-worker artifact is confined to `agent-workspaces/phase2`; emergency-stop
and autonomy-mode controls persist in the named Docker volume
`018-hai-phase2-control-state` and are included in the recovery procedure.

### Connect LARO case intelligence

HAI includes a dedicated `laro` read-only connected-source adapter. Create the
credential in **LARO Settings > HAI**, then set these values in HAI's protected
local environment and restart only the backend service:

```text
HAI_LARO_ENABLED=true
HAI_LARO_BASE_URL=https://your-laro-origin.example/laro
HAI_LARO_CONNECTOR_TOKEN=<one-time LARO credential>
HAI_LARO_SYNC_LIMIT=50
```

Create the source from **Connected Sources** with connector `laro`, **Local
only** disabled, and an empty sync target. The endpoint and credential remain
environment-owned rather than being stored in the source row. Sync is bounded
and cursor-based. Imported LARO records are always sensitive and review-gated;
HAI does not create automatic memory from them and has no LARO write path.

## Dashboard Entry Points

| Route | Purpose |
| --- | --- |
| `/control-center` | Primary operational overview and bounded maintenance actions. |
| `/command-dashboard` | Robert-only decisions, open loops, source-backed context, memory-derived work, and unified approval actions for pursuits and linked workflows. |
| `/pursuits` | Long-running objectives with workflow, source, memory, verification, blocker, approval, activity, and related-pursuit links. |
| `/workflow-engine` | Work queue, approvals, quality gates, interruptions, retries, follow-ups, and read-only internal reminder proposals. |
| `/connected-sources` | Source configuration, sync history, extraction inspection, reindexing, pause/resume, and revocation. |
| `/memory` | Compact memory search, correction, archive, retrieval, and export controls. |
| `/llm-policy` | Provider/model configuration, budget/policy visibility, probes, routing, and fallback history. |
| `/ambient-brain` | Proactive opportunities, scan history, need-profile preferences, and decision handoffs. |
| `/task-blueprint` | Explicit bounded task planning, execution, validation, and review. |
| `/framework-registry` | Versioned decision frameworks, owner preferences, selection evidence, and Constitution controls. |

These screens are authenticated operator surfaces. Technical logs and deep
diagnostics remain behind their relevant detail or audit views.

## API Overview

Backend engine APIs are served under `/api/v1` through the gateway. Principal
areas are:

- `/automation`: registered automations, launch/stop, health checks, and diagnostics.
- `/agent-runtimes`: runtime inventory, health, skill discovery, controlled stop, and OpenClaw ecosystem inspection.
- `/llm`: policy, probes, routing, generation, and redacted decision history.
- `/memory` and `/memory-engine`: compact memory, encrypted conversation import, search, and insights.
- `/sources`: source registry, connectors, sync, extraction management, search, and audit records.
- `/pursuits`: high-level objectives, matching, intake, navigable related-pursuit links, summary, review, decisions, evidence, blockers, next actions, approvals, activity, planning, and approval-gated candidate acceptance.
- `/workflow`: intake, state transitions, approvals, due work, follow-ups, owner-scoped reminder proposals, non-executing reminder activation request/decision evidence, quality/review state, and dashboard data.
- `/task`: bounded plans/runs, durable owner-scoped completion logs, review
  queue, and exact-action review resolution.
- `/verification`: grounded answers and verification run history.
- `/framework-registry`: catalog, owner-effective preferences, selection
  history, and Constitution lifecycle.
- `/ambient`, `/agent-cycle`, `/assistant`, and `/os`: proactive planning, controlled refreshes, command bridge, and operating-system summary.

Use the route tests in `backend/internal/router/` and each subsystem's
documentation for the current Go API contracts. In particular, the
[Framework Registry API table](docs/framework-registry.md#api) lists every
registry endpoint and permission. [docs/swagger.yaml](docs/swagger.yaml) is a
legacy IDP authentication specification; it does not describe the Go control
plane and must not be used as evidence that those routes exist.

## Controlled Models and Runtimes

### Models

The router chooses the cheapest suitable model, not mechanically the cheapest
model. Its policy prioritizes local/free availability, task difficulty,
validation, fallback history, quotas, and the daily budget. Paid calls are
disabled by default with a EUR 0 budget; request JSON cannot self-approve paid
or approval-required use.

Supported configuration families include Ollama, llama.cpp/LM Studio or other
OpenAI-compatible local servers, and configured free/freemium providers. Model
catalog entries cover Qwen, DeepSeek, Llama, Mistral/Mixtral, Gemma, Phi, and
other configured provider models. Provider status must be read as configuration
and probe history, not as a live-service guarantee.

### Agent runtimes

Hermes, DeepSeek Harness, Odysseus, and OpenClaw are optional controlled adapters. HAI can inspect
their configured capabilities and run a bounded approved task only after the
operator installs the upstream runtime, configures scoped credentials/workspace
state, enables the adapter, and validates it. HAI does not bundle these tools,
send messages through them, control browsers, create cron jobs, or bypass their
or HAI's security boundaries.

DeepSeek Harness uses only its documented `dsh --profile headless` path in a
dedicated workspace. The Web UI, ACP server, plugin installation, model keys,
and its own permission configuration remain operator-managed. The upstream is
an MIT-licensed developer preview with compatibility-breaking changes, so an
operator must pin and validate the installed version before relying on it.

API, script, and Docker adapters have the same default posture: disabled until
explicitly allowlisted and configured. The emergency stop blocks runtime
registry execution even when an adapter is invoked directly. Mutating API,
script, Docker-start, and agent-runtime actions also require the internal
action-bound approval proof described above. The proof is issued only from an
approved task review and is validated before network, process/filesystem,
Docker-socket, or agent-runtime access. Direct mutating launch requests
therefore block; read-only API `GET`/`HEAD` probes remain available within the
normal access and allowlist policy.

### Local provider fixture

For a controlled HTTP compatibility check without downloading a model or
contacting a provider, the optional `provider-fixture` Compose profile serves
both Ollama discovery (`/api/tags`) and OpenAI-compatible discovery
(`GET /v1/models`) plus deterministic generation-shaped responses. It is not
an LLM and is never started by the normal local stack. It has no host port,
read-only storage, no Linux capabilities, and a 32 MB / 0.10 CPU / 32 PID
limit.

Use it only in an isolated test configuration where a test-only provider is
explicitly pointed at `http://provider-fixture:11434`:

```powershell
docker compose --env-file .env.example --profile provider-fixture -f docker-compose.local.yml up --build provider-fixture
```

This validates only HAI's network compatibility with a deterministic local
service. It is not evidence that Ollama, LM Studio, a cloud provider, or a
model has been installed or accepted for real work.

## Developer Checks

```powershell
# Backend (use Docker when Go is not installed locally)
docker run --rm -v hai-go-module-cache:/go/pkg/mod -v "${PWD}/backend:/workspace" -w /workspace golang:1.25.13 go test ./...
docker run --rm -v hai-go-module-cache:/go/pkg/mod -v "${PWD}/backend:/workspace" -w /workspace golang:1.25.13 go vet ./...
docker run --rm -v hai-go-module-cache:/go/pkg/mod -v "${PWD}/backend:/workspace" -w /workspace golang:1.25.13 go build ./...

# Identity service (Go 1.25.13)
Set-Location idp
go vet ./...
go test ./...
go build ./...

# Nginx configuration service (Go 1.25.13)
Set-Location ..\nginx-config-manager
go vet ./...
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

With the matching local Go toolchains installed, run the backend commands from
`backend/`, and the IDP and nginx-config-manager commands from their respective
directories. These are the same build-and-test surfaces required by CI. The
critical-path smoke is `scripts/smoke-critical-path.sh` from a Bash-capable
shell with its prerequisites.

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
phase2-feeds/            Read-only Phase 2 JSON feed intake root
agent-workspaces/        Bounded local safe-worker output root
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
- [Framework Registry and task approval contract](docs/framework-registry.md)
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
