# Framework Operating Contract Matrix

This matrix maps the 55-family research specification to code in the canonical
Go/Angular HAI stack. It deliberately separates three states:

- **enforced**: the canonical task or workflow path makes a decision from the
  contract and fails closed when its condition is not met;
- **structured**: a versioned, persisted, inspectable contract exists, but the
  complete real-world domain or distributed runtime is not present;
- **catalogued**: the family is decision metadata or an experimental candidate,
  not an installed or trusted product.

No row should be read as evidence that an external provider, account, agent
framework, or domain service is configured on a particular machine.

## Cross-Cutting Families

| No. | Family | Repository state | Operational boundary |
| ---: | --- | --- | --- |
| 1 | Human sovereignty | **Enforced** through the versioned Constitution, protected rules, authority ceilings, approval gates, and immutable activation history. | HAI cannot infer or amend Robert's values automatically. |
| 2 | Whole-life ontology | **Structured** as deterministic multi-domain task assignments with confidence, signals, primary domain, source, and persistence. | A universal graph assigning every contact, asset, document, and cost is not yet implemented. |
| 3 | Human needs and wellbeing | **Structured** as sourced needs-state assessments or explicitly review-marked deterministic inferences. | It is not a clinical assessment and never silently becomes medical fact. |
| 4 | Personal state and capacity | **Enforced** when a trusted fresh capacity snapshot is supplied; stale, overloaded, and unavailable states constrain or block work. | No browser request may self-assert capacity; live wearable/health sensing is not claimed. |
| 5 | Goal hierarchy | **Structured** across pursuit, workflow, task plan, atomic task step, success criteria, verification, and outcome records. | Full strategy/programme dependency optimization remains incomplete. |
| 6 | Intake and triage | **Enforced** by task/workflow classification, risk, context/tool needs, and explicit success criteria. | Connector quality still depends on each configured source. |
| 7 | Prioritisation | **Enforced** in workflow priority and attention queues; selected frameworks add need, risk, and capacity context. | Portfolio-wide resource optimization remains bounded rather than globally optimal. |
| 8 | Multi-agent organisation | **Structured** with coordinator/specialist role contracts and explicit unassigned roles. | A catalog role name never creates a live agent. |
| 9 | Agent identity and capability | **Enforced** through complete cards for identity, owner, purpose, role, competence, tools, permissions, data boundaries, cost/model profile, reliability, schemas, evidence, escalation, availability, version, dependencies, health, evaluation, authority, revocation, provenance, and 24-hour verification freshness. | Runtime-specific capability probes must still supply trusted evidence. |
| 10 | Delegation and accountability | **Enforced** as deterministic delegation IDs, outcome, zero-spend budget, deadline, constraints, authority, evidence, completion, escalation, and state. | A delegate cannot grant itself authority or approve its own consequential work. |
| 11 | Agent communication | **Enforced** by `hai-agent-message-v1` validation for schema, correlation, idempotency key, type, confidentiality, authority, timestamp, expiry, payload size/digest, provenance, optional signature digest, evidence references, and secret rejection. | A distributed A2A transport and cryptographic signature authority are not bundled. |
| 12 | Multi-agent coordination | **Structured** for single-engine, sequential, hierarchical, parallel-specialist, and debate/critic modes. | Modes requiring unavailable or stale agents remain blocked; no live consensus cluster is claimed. |
| 13 | Cognitive and reasoning methods | **Catalogued and selected** by task type, difficulty, evidence, and uncertainty signals. | HAI records methods and outcomes, not hidden chain-of-thought. |
| 14 | Cognitive-agent architectures | **Structured** through the task loop, framework selector, context/model/tool routing, critic/validation, and review state. | Named external cognitive frameworks remain adapters or candidates. |
| 15 | Decision-making under uncertainty | **Enforced** through uncertainty labels, evidence requirements, conflict checks, and review escalation. | Probabilistic calibration needs longitudinal real outcomes. |
| 16 | Formal planning | **Enforced** through explicit task steps, dependencies where present, success criteria, risk, and bounded capacity step limits. | General-purpose PDDL/constraint solving is not claimed. |
| 17 | Workflow modelling | **Enforced** through durable workflow state, transitions, checklist, source links, approvals, retries, leases, and completion checks. | Cross-region distributed workflow execution is absent. |
| 18 | Reliable execution | **Enforced** through planning/execution separation, idempotency, bounded retries, action-bound approval proofs, postcondition verification, and visible failure states. | Exactly-once external side effects cannot be guaranteed. |
| 19 | Autonomy levels 0-10 | **Enforced per action** using the exact observe, inform, recommend, draft, plan/simulate, prepare, case-approved, standing-approved, reversible-auto, execute/notify, bounded-full ladder. | Levels 7-10 require real standing/bounded mandates; no global autonomy toggle creates them. |
| 20 | Approval | **Enforced** by owner-scoped review decisions and exact action-bound approval provenance. | Approval proof replay state is process-local and not distributed. |
| 21 | Memory architecture | **Structured** across task context and durable memory records with relevance and provenance. | All conceptual memory subtypes are not yet separate physical stores. |
| 22 | Personal knowledge management | **Structured** through sources, memory review/correction, project scoping, and pursuit links. | A complete personal knowledge graph remains a future layer. |
| 23 | Retrieval and context | **Enforced** by relevance-ranked memory/source retrieval and explicit context plans. | Retrieval quality depends on indexed, authorized source coverage. |
| 24 | Knowledge and truth | **Enforced** through claim/source status, schema checks, deterministic validation, tests, and review for unsupported output. | Tests prove covered contracts, not universal factual truth. |
| 25 | Ingestion and synchronization | **Structured** through source registry, cursors, metadata, extraction, indexing, sync state, and audit. | Each real connector still requires credentials, permissions, and acceptance evidence. |
| 26 | Perception and ambient intelligence | **Structured** through scans, open-loop detection, proposals, interruption policy, and outcome monitoring. | HAI does not perform unrestricted surveillance or stealth scraping. |
| 27 | Human-AI interaction | **Structured** through Basic/Advanced disclosure, approvals, review queues, provenance, and action-first UI. | Usability still requires target-user acceptance, not only component tests. |
| 28 | Privacy | **Enforced** through owner scope, local-first policy, minimization, exclusions, redaction, and deletion/revocation paths. | External providers remain independent privacy boundaries. |
| 29 | Security | **Enforced** through authenticated owner identity, RBAC, allowlists, secret handling, runtime policy, and fail-closed behavior. | A green unit suite is not a penetration test. |
| 30 | Agentic threat modelling | **Enforced** through untrusted-content boundaries, no authority in messages, protected prohibitions, and approval/runtime controls. | Threat coverage must evolve with new adapters and tools. |
| 31 | Safety engineering | **Enforced** with emergency stop, risk gates, stop conditions, reversibility preference, and human review. | Physical-world safety depends on the connected executor and environment. |
| 32 | AI governance | **Enforced** through versioned policy, immutable audit, owner activation, provider budgets, and review state. | Regulatory compliance remains jurisdiction and deployment specific. |
| 33 | Model intelligence | **Structured** through provider/model catalog, suitability routing, cost/quota state, validation, and fallback history. | Live capability and price claims require provider probes and current configuration. |
| 34 | Evaluation | **Enforced** through criteria, validation statuses, retries/review, and framework evidence/assurance checks. | Synthetic checks must not be represented as production effectiveness. |
| 35 | Observability | **Structured** through task/workflow state, selections, decisions, audit, runtime evidence, and health summaries. | Production traces and alerts require deployed observability services. |
| 36 | Reliability and resilience | **Enforced locally** through durable state, leases, retries, recovery states, health checks, and postcondition verification. | High availability and distributed lease coordination remain absent. |
| 37 | Controlled learning | **Enforced** as verified or operator-confirmed learning proposals; authority and policy cannot self-modify. | Autonomous weight training or unsupervised policy mutation is not present. |

## Personal Operating Packs

| No. | Pack | Repository state | Operational boundary |
| ---: | --- | --- | --- |
| 38 | Productivity and attention | **Structured** selection, capacity, prioritization, follow-up, and task planning. | Calendar/account coverage depends on configured connectors. |
| 39 | Behaviour change and habits | **Catalogued** with reviewable recommendation and outcome criteria. | No clinical or coercive behavior manipulation is authorized. |
| 40 | Health and personal care | **Catalogued with high-risk controls** and stricter evidence/approval expectations. | No diagnosis, treatment, or emergency service replacement is claimed. |
| 41 | Financial management | **Catalogued with zero-spend delegation default** and consequential-action approval. | Banking, payment, tax, and accounting integrations are not implied. |
| 42 | Home, garden, and assets | **Structured** through source/workflow/task patterns and reversible execution controls. | Device and asset integrations require explicit adapters and allowlists. |
| 43 | Work and service delivery | **Structured** through pursuits, workflows, delegation, deadlines, checklists, and verification. | Customer systems require configured connectors. |
| 44 | Entrepreneurship and venture | **Structured** through pursuits, priorities, evidence, planning, and outcome monitoring. | Market conclusions remain evidence-dependent, not guaranteed forecasts. |
| 45 | Legal, government, and case management | **Catalogued with mandatory evidence and approval controls**. | HAI is not legal counsel and cannot submit or send without the governed path. |
| 46 | Communication | **Structured** for recipient/tone context, drafting, source support, and approval. | Consequential external send remains approval-gated. |
| 47 | Relationships and care | **Catalogued with human-sovereignty and privacy overlays**. | HAI cannot infer consent or replace direct human judgment. |
| 48 | Learning and competence | **Structured** through goals, context, plans, memory, and verified lessons. | Credentials and competence must come from authoritative evidence. |
| 49 | Travel and mobility | **Catalogued** for planning, deadlines, calendar, and risk checks. | Booking or payment requires a real adapter and approval. |
| 50 | Emergency and continuity | **Catalogued with stop/escalation controls**. | HAI is not an emergency service and must direct urgent human action appropriately. |

## Implementation Candidate Families

| No. | Family | Repository state | Operational boundary |
| ---: | --- | --- | --- |
| 51 | Agent-development frameworks | **Experimental catalog only**; candidate products can be evaluated behind adapters. | No candidate is installed, trusted, or enabled by catalog mention. |
| 52 | Durable workflow platforms | **Experimental catalog only**; canonical Go workflow remains authoritative. | Temporal-like distributed execution is not present. |
| 53 | Memory and knowledge implementations | **Experimental catalog only**; current PostgreSQL/source/memory paths remain canonical. | RAG products require separate deployment, migration, privacy, and quality proof. |
| 54 | Policy, identity, and security implementations | **Experimental catalog only**; current IDP/RBAC/policy remains authoritative. | External policy engines or sandboxes need explicit integration and security review. |
| 55 | Evaluation and observability implementations | **Experimental catalog only**; current validation/audit/health paths remain canonical. | External telemetry products require configured storage, retention, and access controls. |

## Selector V4 Trust Boundary

The public preview accepts only bounded planning hints. Owner identity comes
from the authenticated session. Risk, approval, observed needs, capacity,
available agents, coordination preference, and workflow deadline are trusted
in-process inputs. Secrets are redacted before durable contract fields are
created. Agent cards are considered verified only when the runtime reports an
`available` status with provenance and a timestamp no older than 24 hours.

The operating-contract digest covers life domains, needs, capacity, agent
cards, delegations, communication, coordination, per-action autonomy, stop
conditions, outcome monitoring, and the eight Chief-of-Staff answers. It
supports trace comparison; it does not itself grant authority or prove an
external action occurred.

## Remaining Product Work

The largest remaining gaps are a universal cross-entity life-domain graph,
live sourced capacity feeds, a durable standing-mandate workflow, distributed
multi-agent/A2A transport, crash-safe distributed approval-proof consumption,
high-availability workflow coordination, full domain-specific services, and
real acceptance evidence for each configured account/provider/runtime.
