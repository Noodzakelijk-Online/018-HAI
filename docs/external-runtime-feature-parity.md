# External Runtime Feature Parity

## Purpose

HAI treats OpenClaw, Hermes Agent, and Odysseus as subordinate capability
providers. They do not own pursuits, plans, policy, approval, canonical state,
verification, audit, or completion. PostgreSQL and the HAI Operation Ledger
remain authoritative.

The authenticated `GET /api/v1/runtime-lab/feature-parity` endpoint is the
machine-readable inventory. It accounts for every required analysis area for
each runtime and fails validation when an area is missing, an exclusion lacks a
reason, or a deferred/externally blocked feature lacks a priority,
requirements, and recommended implementation path.

Reading the inventory performs no network request, installation, configuration,
probe, self-test, or execution.

`GET /api/v1/runtime-lab/capabilities` projects the viable integration points
into HAI-native capability cards. Each card declares its input/output schema,
authentication state, availability, runtime location, required authority, risk,
EUR cost ceiling, context cost, timeout, retry behavior, reversibility,
approval requirements, verification method, and evidence. All external cards
start with `canInvoke:false`, `canExecuteExternalEffect:false`, and
`authority:contract_only`. After an owner explicitly configures an allowlisted
endpoint and runs a schema-valid probe, only the corresponding read-only
discovery card may return `canInvoke:true`. External effects remain false.

## Reviewed Upstreams

| Project | Repository | Branch | Reviewed revision | Release | License | HAI readiness ceiling |
| --- | --- | --- | --- | --- | --- | --- |
| OpenClaw | `openclaw/openclaw` | `main` | `fa9626c4e1002d83e6c06a09ad670c8f41f8b24e` | `v2026.7.1-2` | MIT | `declared` |
| Hermes Agent | `NousResearch/hermes-agent` | `main` | `b3aa561faffd64f05436e429a6415d175e534ec9` | `v2026.8.3` | MIT | `declared` |
| Odysseus | `odysseus-dev/odysseus` | `dev` | `e4fa4ae5dd1d709ce4168397bd1d200fec1b2494` | no formal release recorded | AGPL-3.0-or-later | `declared` |

Reviewed on 2026-08-08 from the upstream GitHub repositories and their own
README, protocol, security, threat-model, feature, and roadmap documentation.
The pinned revision is evidence of what was examined; it is not a dependency
lock or proof that the project is configured locally.

## Implemented Discovery Contracts

Runtime Lab now validates reviewed, non-mutating protocol responses instead of
treating an arbitrary HTTP 200 as health:

| Runtime | Requests | Validation | Highest discovery level | Identity limitation |
| --- | --- | --- | --- | --- |
| OpenClaw | `GET /health` | `ok=true`, `status=live` | `available` | The public health body does not identify or authenticate the runtime. |
| Hermes | `GET /health`; optionally authenticated `GET /v1/capabilities` | `platform=hermes-agent`, version, capability object/platform | `health_checked` | Capability discovery requires `HERMES_API_SERVER_KEY`; liveness does not. |
| Odysseus | `GET /api/health`, `GET /api/version` | healthy status, RFC3339 timestamp, non-empty version | `available` | The public responses do not carry a cryptographic product identity. |

Responses are limited to 64 KiB, redirects are refused, hosts are allowlisted,
JSON shape is checked, raw bodies are not returned, and only a SHA-256 evidence
digest plus bounded metadata reaches the dashboard. The OpenClaw adapter now
also performs the stricter Companion-specific `GET /health` handshake: its
response body is capped at 4 KiB, it accepts only `ok=true` with `status=live`,
and it refuses URL credentials and redirects. Query and fragment data are
never forwarded to the fixed `/health` request. A live Companion reports
`available` for read-only discovery; `ready` remains reserved for a genuinely
executable adapter. No gateway token is sent during this health probe. Discovery
evidence is currently in-process and intentionally expires on backend restart;
durable readiness restoration will require a PostgreSQL ledger record rather
than configuration inference.

## Disposition Rules

Every reviewed feature group has exactly one disposition:

- `integrated_directly`
- `adapted_for_hai`
- `hai_native_reimplementation`
- `already_present`
- `consolidated_existing`
- `constrained_unsafe`
- `excluded_irrelevant`
- `excluded_incompatible_license`
- `deferred`
- `blocked_external`

The current review intentionally reports zero `integrated_directly` items. The
existing generic health adapters are not feature integration. A feature may
move to a stronger disposition only after its HAI contract, authority boundary,
protocol implementation, test evidence, and operator documentation exist.

## Project Decisions

### OpenClaw

The relevant integration surface is the Gateway protocol: scoped WebSocket
clients, isolated agent workspaces/sessions, capability discovery, lifecycle
events, and bounded delegation. HAI will retain its own Plan Graph, memory,
scheduler, LLM budget, approval, and verification systems. OpenClaw channel
delivery and host tools stay blocked or constrained until channel-specific
credentials, approval receipts, sandboxing, and independent effect verification
exist. The OpenClaw Control UI, updater, and product shell are not imported.

Primary sources:

- <https://github.com/openclaw/openclaw/blob/main/README.md>
- <https://github.com/openclaw/openclaw/blob/main/docs/gateway/protocol.md>
- <https://github.com/openclaw/openclaw/blob/main/docs/concepts/multi-agent.md>
- <https://github.com/openclaw/openclaw/blob/main/docs/tools/skills.md>
- <https://github.com/openclaw/openclaw/blob/main/docs/gateway/sandboxing.md>

### Hermes Agent

The relevant integration surfaces are `hermes serve` and the documented
JSON-RPC/WebSocket and OpenAI-compatible APIs. HAI may adapt capability
discovery, bounded delegated tasks, tool-progress telemetry, and reviewed skill
metadata. Hermes planning, memory, cron, model routing, and product UI do not
replace HAI's corresponding canonical systems. Learned skills and
self-improvement must enter HAI as evidence-backed controlled-learning
proposals; they cannot mutate authority or policy.

Primary sources:

- <https://github.com/NousResearch/hermes-agent/blob/main/README.md>
- <https://github.com/NousResearch/hermes-agent/blob/main/website/docs/user-guide/features/api-server.md>
- <https://github.com/NousResearch/hermes-agent/blob/main/website/docs/user-guide/features/skills.md>
- <https://github.com/NousResearch/hermes-agent/blob/main/website/docs/user-guide/features/memory.md>
- <https://github.com/NousResearch/hermes-agent/blob/main/apps/desktop/README.md>

### Odysseus

Odysseus is an AGPL-3.0-or-later self-hosted workspace, so its server, UI,
tools, and product shell are not copied or linked into the canonical HAI code.
Useful behavior may be reproduced through HAI-native interfaces, or HAI may
later interoperate with a separately deployed service after legal and security
review.

The upstream threat model explicitly says that a logged-in admin can execute
shell commands, read/write files, send email, and control model serving. It also
documents that shell/filesystem tools lack confinement and network-egress
filtering. Those tools are therefore not exposed through the generic runtime
adapter. Odysseus email, Calendar, and model-serving behavior remains separate
and externally blocked unless a least-privilege, source-linked, approval-gated
protocol is proven.

Primary sources:

- <https://github.com/odysseus-dev/odysseus/blob/dev/README.md>
- <https://github.com/odysseus-dev/odysseus/blob/dev/THREAT_MODEL.md>
- <https://github.com/odysseus-dev/odysseus/blob/dev/SECURITY.md>
- <https://github.com/odysseus-dev/odysseus/blob/dev/ROADMAP.md>
- <https://github.com/odysseus-dev/odysseus/blob/dev/LICENSE>

## Readiness Levels

The delivery specification distinguishes `declared`, `configured`, `available`,
`health-checked`, `self-tested`, `integration-tested`, `demonstrated`, and
`production-ready`. This source review establishes only `declared` parity.
Runtime Lab may separately report a configured endpoint or a successful health
probe, but neither authorizes a task. A runtime can advance only with retained
evidence for the exact level; no level is inferred from a lower one.

## Next Integration Gates

1. Persist read-only discovery evidence in the Operation Ledger with owner,
   workspace, endpoint digest, reviewed schema revision, and expiry.
2. Map every discovered method/tool to a HAI capability card with input/output
   schema, authority, risk, cost, timeout, retry, reversibility, approval, and
   verification requirements.
3. Add request identity, cancellation, redacted events, result artifacts, and
   ambiguous-outcome reconciliation.
4. Run a loopback protocol self-test that cannot invoke a tool and retain its
   evidence independently from runtime configuration.
5. Add one bounded local task behind exact execution authorization and
   independent verification.
6. Keep Odysseus remote-only and read-only until its legal and security gates
   are accepted.

No external runtime is production-ready at this point.
