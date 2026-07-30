# Operator Runbook

Day-to-day operation of 018-HAI on one local host. This runbook covers the
55-record Framework Registry and the durable task-review path in addition to
ordinary stack health.

HAI is fail-closed operational software. A visible dashboard, a healthy
container, or a persisted approval does not prove that an external action
completed. Use the audit, source, runtime, and verification evidence required
by the action.

## Start And Stop

Create an untracked `.env.local` from `.env.example`, replace placeholder
secrets, and use the local Compose file:

```bash
docker compose -f docker-compose.local.yml --env-file .env.local up -d --build
docker compose -f docker-compose.local.yml --env-file .env.local ps
```

Stop the stack without deleting volumes:

```bash
docker compose -f docker-compose.local.yml --env-file .env.local down
```

Do not add `-v` unless the reviewed operation is intentionally deleting local
database and queue volumes.

## Health Checks

| Check | Command | Healthy |
| --- | --- | --- |
| Liveness | `curl localhost/healthz` | HTTP 200 and `{"status":"ok"}` |
| Readiness | `curl localhost/readyz` | HTTP 200 with `ready` or explicitly understood `degraded`; HTTP 503 with `not_ready` is blocking |
| Config | `backend doctor` | Exit 0 and no `fail` lines |
| Migration state | `backend migrate status` | Required `pre` and `post` versions are applied |
| Data integrity | `backend reconcile` | No unresolved invariant failure |

Gateway `/healthz` and `/readyz` are intentionally public. They expose only
service status. Operational `/api/v1/*` routes remain authenticated unless a
route is explicitly documented as a public callback.

`degraded` is not equivalent to full capability. For example, a missing LLM
provider may leave the control plane available while generation is unavailable.

## Identity And Permission Contract

The gateway and backend derive identity and role from a verified IDP token.
Neither `X-HAI-Role`, request JSON, nor the backend shared key grants a user
role.

| Role | Registry | Tasks |
| --- | --- | --- |
| `viewer` | Read the owner-scoped catalog, Constitution, and selection history. | Read owner-scoped completion logs and review queue. |
| `operator` | Viewer access plus create a selection recommendation. | Plan tasks and operate non-approval controls; cannot approve/run an approval-gated task or resolve a review. |
| `owner` | All registry operations, including preferences and Constitution lifecycle. | Plan, run, approve/reject, and resolve owner-scoped task reviews. |

The seeded `FIRST_RUN_ADMIN_EMAIL` account is the owner. New local
registrations default to operator. Unknown roles grant no permissions.

## Framework Registry Operations

Open `/framework-registry` after signing in.

### Interpret Status Correctly

| Displayed state | Meaning |
| --- | --- |
| `active` | Catalog lifecycle; enabled by default unless this owner disables the record. |
| `experimental` | Catalog lifecycle; disabled by default and eligible only after owner enablement plus a direct framework/name/candidate match. |
| `deprecated` | Catalog lifecycle; retained for compatibility but excluded from selection even if a preference says enabled. There are no deprecated records in catalog v1. |
| `disabled` | Owner-effective preference, not a catalog lifecycle status. |

The current built-in catalog has 55 version `1.0.0` records: 50 active and five
experimental. Enabling an experimental record only makes its decision contract
eligible for a directly matching request. It does not install or trust a named
third-party product. A deprecated record remains excluded from selection.

Protected overlays may be pinned but cannot be disabled. Preferences may lower
an autonomy ceiling, never raise it. Adaptations may add bounded owner context,
but may not bypass approval, the Constitution, emergency stop, secret
redaction, or tool policy.

### Inspect A Selection

For any consequential recommendation, record or inspect:

- catalog version and SHA-256 digest;
- selector algorithm version;
- effective-preference digest;
- Constitution version, exact source, and digest;
- selected framework IDs and versions;
- reasons, conflicts, required agents, authority ceiling, approval reasons;
- evidence requirements and completion criteria.
- operating-contract digest, matched life domains, needs and capacity state;
- verified/unassigned agent cards, delegation budget/deadline/constraints;
- communication/coordination contract and per-action autonomy decision;
- stop conditions, outcome monitoring, and Chief-of-Staff summary.

The current selector version is `selector-v4`. Do not compare selection UUIDs
to prove replay: the selection time contributes to the decision identity. Use
the version and digest envelope.

Autonomy uses the exact per-action ladder: 0 observe only, 1 inform,
2 recommend, 3 draft, 4 plan and simulate, 5 prepare action, 6 execute after
case-specific approval, 7 execute under standing approval, 8 execute a
reversible low-risk action automatically, 9 execute and notify, and 10 operate
fully autonomously inside a tightly bounded mandate. An approval may authorize
scope but never raises a framework, Constitution, tool, or runtime ceiling.

The public selection preview accepts planning hints, not trusted approval,
owner, or risk assertions. A preview cannot authorize execution.

### Constitution Draft And Activation

When no stored owner version is active, the exact fallback source is:

```text
builtin-robert-constitution-v1:v1
```

Creating a draft does not change active authority. To activate a reviewed
draft, the owner must:

1. sign in with an owner session that has admin permission;
2. inspect the draft and its change summary;
3. enter `ACTIVATE CONSTITUTION` exactly;
4. provide a redacted approval note of at least 10 characters;
5. submit activation once and confirm the previous version became
   `superseded`.

The confirmation is case-sensitive and whitespace-sensitive. Do not paste a
leading/trailing space. A draft is immutable after insertion; make a new draft
to change content.

Ordinary Constitution prose is versioned governance context, not executable
policy. Only code-owned protected controls and valid restrictive typed rules
are machine-enforced:

```text
HAI-RULE v1 deny-capability capability=<known-capability>
HAI-RULE v1 require-approval capability=<known-capability>
HAI-RULE v1 authority-ceiling level=<0..10>
```

A malformed typed rule rejects the draft. There is no rule that grants a
capability or raises authority.

## Durable Task Review Operations

Migration `pre/0004_task_state_storage` stores:

- append-only, redacted completion-plan snapshots;
- review items with immutable owner and request provenance;
- immutable approval/rejection decisions bound to the review revision, exact
  request digest, task plan, resolver, and `task-review:<id>` source.

Authenticated `/task/logs` and `/task/review-queue` reads use this store. If the
repository is unavailable, the API returns an error instead of an empty list.

### State Machine

| State | Meaning | Valid next state |
| --- | --- | --- |
| `open` | First owner decision is pending. | `approved` or `rejected` |
| `needs_review` | An earlier approved attempt failed, was blocked, or did not validate; a new revision awaits a decision. | `approved` or `rejected` |
| `approved` | The exact stored request was approved and its attempt is in progress or has no recorded outcome yet. | `completed` or `needs_review` |
| `rejected` | Owner rejected that review revision. | Terminal |
| `completed` | Approved attempt passed task validation. | Terminal |

Approval is one-shot for the current review revision. The service reruns the
stored request and verifies the owner, review ID, plan/project/automation
bindings, approval source, and request digest. New client fields do not replace
the reviewed action.

### Normal Review Procedure

1. Open the task review queue and inspect the request, project, target,
   success criteria, risk, and source evidence.
2. Reject if the action is wrong, stale, under-specified, or lacks evidence.
3. Approve only if the exact stored action is acceptable.
4. Keep the page open until a terminal `completed` state or a new
   `needs_review` state is visible.
5. Inspect completion validation and runtime evidence. Approval by itself is
   not completion evidence.

Do not submit the same resolution twice. A second decision against a resolved
revision returns a conflict.

### Recovery After Failure Or Restart

An item in `needs_review` can be inspected, corrected through a new governed
task, and approved or rejected as a new revision.

An item left `approved` after a backend crash is indeterminate:

1. enable emergency stop if a side effect could still be running;
2. inspect task logs, automation/runtime audit, the target system, and any
   idempotency or correlation identifier;
3. determine whether the side effect did not start, completed, or has an
   unknown outcome;
4. do not resolve the same review again and do not repeat the side effect
   blindly;
5. collect a support bundle and escalate for reviewed reconciliation.

There is currently no automatic task-review recovery worker and no public
endpoint to reset `approved` to `needs_review`. Do not mutate the database by
hand to bypass this boundary. The action approval proof and consumed-nonce set
are process-local, so external execution is not claimed as exactly once across
a restart.

## Migrations And Rollback

Inspect and apply embedded migrations:

```text
backend migrate status
backend migrate up
```

Relevant pre-phase migrations:

| Migration | Data |
| --- | --- |
| `pre/0003_framework_registry` | Preferences, immutable selection records, Constitution versions/lifecycle |
| `pre/0004_task_state_storage` | Completion logs, review items, immutable decisions |
| `pre/0005_framework_operating_contract` | Immutable selector-v4 operating context, digest, and Chief-of-Staff trace |

Explicit rollback targets:

```text
backend migrate down pre/0005_framework_operating_contract
backend migrate down pre/0004_task_state_storage
backend migrate down pre/0003_framework_registry
```

These rollbacks delete operator and audit history. The runner requires reverse
order and rejects older targets while a later migration in the phase remains
applied. Before any rollback:

1. stop task execution and dependent application versions;
2. take and verify a PostgreSQL backup;
3. deploy a compatible earlier application version;
4. use the explicit `pre/...` target;
5. verify migration state and owner-scoped reads after restart.

See [Database Migrations And Rollback Safety](migrations.md).

## Routine Security Tasks

- **Rotate backend and JWT keys:** update `.env.local`, recreate affected
  services, invalidate old sessions where required, and rerun authenticated
  smoke checks.
- **Backups:** follow [backup and restore](backup-restore.md); test restore
  evidence, not only dump creation.
- **Rate limiting:** if exposed beyond loopback, configure
  `RATE_LIMIT_PER_MINUTE` and verify Redis-backed enforcement.
- **Emergency stop:** set `HAI_EMERGENCY_STOP=true` in `.env.local` and recreate
  the backend service. Confirm task, workflow, LLM, automation, and runtime
  execution are blocked while read/review surfaces remain available.
- **Secret handling:** never put credentials in Constitution prose, framework
  adaptations, task requests, approval notes, or support bundles.

## Incident Response

| Symptom | First step |
| --- | --- |
| `/readyz` returns 503 | Read the failing checks, run `backend doctor`, repair the dependency, and restart. |
| Mass 401 responses | Check the IDP session and verified JWT, then the gateway/backend shared-key wiring. |
| Registry history is empty unexpectedly | Treat a 500 as a storage incident; verify migration `0003` and database connectivity. Do not create replacement history. |
| Task history/review queue returns 500 | Verify migration `0004` and database connectivity. Do not treat the queue as empty. |
| Selection fails at 12 frameworks | Inspect direct intent and required-overlay coverage; do not raise the cap or remove safety overlays without a reviewed selector change. |
| Constitution activation returns 400 | Re-enter the exact phrase, use an owner session, and inspect typed-rule syntax and approval-note length. |
| Constitution activation returns 409 | Refresh versions; the draft is stale, invalid, or lost its lifecycle precondition. |
| Review resolution returns 404 | Confirm owner scope and review ID. Do not search another owner's records. |
| Review resolution returns 409 | The revision was already resolved or changed state. Refresh before taking another action. |
| Review is stuck `approved` | Follow the indeterminate-outcome recovery procedure above. |
| Runaway or uncertain execution | Enable emergency stop, preserve logs/evidence, and inspect the external target before retrying. |
| Suspected data corruption | Run `backend reconcile` for a dry-run report; do not delete audit rows. |

## Readiness And Escalation

Collect a redacted support bundle, migration status, readiness payload, relevant
selection/review IDs, and the last correlated logs before escalating. Do not
include raw source content, tokens, passwords, approval proofs, or private
request payloads.

Repository tests prove implementation contracts, not real-world capability.
Before operational trust still require:

- a clean Windows 11 clone, migration, sign-in, and browser journey;
- a two-real-account owner-isolation exercise;
- one bounded configured local-model task;
- separately approved and evidenced live connector/runtime exercises;
- crash/restart and restore evidence for the intended deployment;
- a reviewed recovery procedure for indeterminate approved task reviews.

See [Framework Registry](framework-registry.md),
[verification evidence](verification-honest-health-readiness.md), and
[troubleshooting](troubleshooting.md).
