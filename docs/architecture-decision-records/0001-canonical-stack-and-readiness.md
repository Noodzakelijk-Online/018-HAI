# ADR 0001: Canonical Stack and Real-World Readiness

## Status

Accepted.

## Decision

The canonical 018-HAI product stack is:

- Go backend API
- Angular dashboard
- Postgres persistence
- Docker Compose local-first Windows 11 deployment path

The Manus React/tRPC/MySQL implementation is not a second production product stack. Treat it as reference material for UX, product behavior, and possible migration candidates. Behavior from Manus should be ported into the canonical Go/Angular stack through explicit issues, tests, and review.

## Why

The two stacks overlap across task execution, verification, connected sources, LLM routing, memory, and workflow orchestration. Keeping both as active products would split persistence models, safety policies, audit logs, connectors, and runtime execution paths. That would make autonomous behavior harder to trust.

## Readiness Policy

Internal unit tests prove code consistency only. They do not prove real-world correctness against live LLM providers, real accounts, real documents, or external APIs.

A feature may be shown as operational only when it has:

- A real adapter, not only a registry entry or placeholder.
- Explicit readiness state in the API/UI when the adapter or provider is disabled, unconfigured, missing credentials, blocked, or not implemented.
- A persisted audit trail.
- Failure behavior that blocks or reviews instead of silently succeeding.
- Source provenance where factual claims are involved.
- Integration or smoke-test evidence against a real local service, sandbox account, or seeded source fixture.

Until then, the dashboard and README must label the feature as partial, guarded, reference-only, or unproven.

Concrete application of this policy:

- LLM providers are selectable only when enabled, configured with an absolute endpoint, credential-ready when required, and not blocked by endpoint safety rules. Provider calls do not follow redirects.
- Account/source connectors without real OAuth/API adapters remain disabled `not_implemented` contracts. The current operational source adapter is `local-folder`.
- Scheduled sync must not pretend unsupported connectors are live. It may process operational local-folder sources and explicit manual import payloads only.

## Controlled Runtime Safety Baseline

Server-side execution must stay conservative:

- API launches are host-allowlisted by `AUTOMATION_API_ALLOWED_HOSTS`.
- Link-local, metadata, or unspecified API targets are blocked by default.
- API launches do not follow redirects.
- Script execution is disabled unless `AUTOMATION_SCRIPT_EXECUTION_ENABLED=true`.
- Script targets must stay inside `AUTOMATION_SCRIPT_DIR`, including after symlink resolution.
- Scripts run with a minimal environment. Extra environment variables require explicit `AUTOMATION_SCRIPT_ENV_ALLOWLIST` entries.
- Docker control is disabled unless `AUTOMATION_DOCKER_CONTROL_ENABLED=true`.
- Docker targets must be listed in `AUTOMATION_DOCKER_ALLOWED_CONTAINERS`.
- Unsupported runtime types must block and audit the reason.

This runtime layer is not a broad desktop, browser, MCP, or Claw/OpenClaw agent runtime yet.

## Consequences

- New work should consolidate into the Go/Angular stack.
- Manus code can inform behavior, but should not receive parallel feature development unless the product direction changes.
- HAI OS overview should show real-world readiness gates so users can distinguish implemented local logic from proven real integrations.
- Integration work should proceed one connector/runtime at a time, with tests and audit evidence before autonomy is increased.
