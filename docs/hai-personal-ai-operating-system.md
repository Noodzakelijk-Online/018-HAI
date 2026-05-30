# HAI Personal AI Operating System

HAI is now organized as a local-first personal AI operating system. The main
rule is completion first: resource efficiency is applied after safety,
traceability, correctness, and verified completion.

## Planes

- Control: dashboard, automations, policy settings, health, and review queues.
- Knowledge: connected-source ingestion, extraction, search, index records, and
  source provenance.
- Memory: preferences, project context, decisions, procedures, lessons learned,
  edit/correct/archive/export/delete.
- Reasoning: task classification, success criteria, context planning, model and
  tool routing, retry, escalation, and review state.
- Execution: guarded automation launch and controlled task runs.
- Verification: source-grounded answers, claim checks, deterministic
  calculations, conflict detection, and unsupported-claim review.
- Governance: local-only controls, sensitive-source flags, approval gates,
  provider budget policy, and audit logs.
- Observability: OS overview, health summaries, sync logs, verification runs,
  model decisions, task events, and diagnostics.

## Dashboard

The **HAI OS** page reads `/api/v1/os/overview` and shows:

- completion-first and local-first state
- paid budget and paid-call lockout
- automations and unhealthy automation count
- connected source and extraction count
- context memory count
- LLM provider count
- verification run count
- total review load
- plane-level status and links into the relevant dashboard pages

Unresolved work is surfaced as `needs_review`; it is not treated as complete.
