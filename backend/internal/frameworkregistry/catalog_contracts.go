package frameworkregistry

import "fmt"

type catalogOperationalContract struct {
	inputs            []string
	outputs           []string
	contraindications []string
}

// builtinCatalogOperationalContracts supplies the framework-specific operating
// contract that cannot be inferred safely from a framework's broad purpose.
// Keep this table one-to-one with the 55 specification records in catalog.go.
var builtinCatalogOperationalContracts = map[string]catalogOperationalContract{
	"human-sovereignty": {
		inputs: []string{
			"verified operator identity and the active Constitution version",
			"the proposed objective, affected commitments, and requested authority",
		},
		outputs: []string{
			"an authority-boundary decision naming what HAI may and may not decide",
			"an operator choice record for any unresolved value or identity question",
		},
		contraindications: []string{
			"infer Robert's values, identity, or consent from observed behavior without confirmation",
		},
	},
	"whole-life-ontology": {
		inputs: []string{
			"source-linked candidate entities with names, types, dates, and identifiers",
			"the current project, case, pursuit, or life-domain vocabulary",
		},
		outputs: []string{
			"a provenance-preserving entity classification and relationship set",
			"a review queue of ambiguous identities, duplicates, and cross-domain links",
		},
		contraindications: []string{
			"merge people, cases, assets, or obligations solely because their labels are similar",
		},
	},
	"needs-wellbeing": {
		inputs: []string{
			"operator-stated needs, constraints, and desired quality-of-life outcomes",
			"recent source-backed wellbeing signals with confidence and observation dates",
		},
		outputs: []string{
			"a need-to-action map that distinguishes stated needs from inferred needs",
			"a trade-off proposal showing which wellbeing dimensions gain or lose",
		},
		contraindications: []string{
			"treat a single needs hierarchy or an inferred deficit as a diagnosis or universal truth",
		},
	},
	"capacity-state": {
		inputs: []string{
			"fresh availability, workload, energy, attention, health, and budget constraints",
			"task effort estimates, deadlines, dependencies, and minimum viable scope",
		},
		outputs: []string{
			"a capacity-feasible work allocation with stale-data warnings",
			"a reversible reschedule or scope-reduction proposal for overloaded periods",
		},
		contraindications: []string{
			"commit external time or money from inferred capacity that the operator has not confirmed",
		},
	},
	"goal-hierarchy": {
		inputs: []string{
			"an explicit desired outcome and its measurable success criteria",
			"candidate parent pursuits, projects, commitments, needs, and constitutional values",
		},
		outputs: []string{
			"a trace from atomic actions to the governing pursuit and value",
			"a decomposition with measurable completion boundaries and orphan-goal warnings",
		},
		contraindications: []string{
			"manufacture a parent goal or claim alignment where the operator has not supplied one",
		},
	},
	"intake-triage": {
		inputs: []string{
			"the immutable original request or source signal and its provenance",
			"sender identity, source trust, timing, urgency indicators, and known project links",
		},
		outputs: []string{
			"a normalized intake record with domain, urgency, risk, confidence, and routing reason",
			"a next-step decision to ignore, clarify, plan, route, quarantine, or escalate",
		},
		contraindications: []string{
			"convert ambiguous, injected, or low-trust content directly into executable instructions",
		},
	},
	"multi-criteria-prioritization": {
		inputs: []string{
			"candidate work items with deadlines, impact, effort, risk, reversibility, and dependencies",
			"operator priorities, non-negotiable commitments, and the applicable scoring policy",
		},
		outputs: []string{
			"a scored and tie-broken priority order with factor-level explanations",
			"a next-best-action recommendation and a list of items intentionally deferred",
		},
		contraindications: []string{
			"reduce legal duties, safety constraints, or human dignity to an unconstrained numeric score",
		},
	},
	"multi-agent-organization": {
		inputs: []string{
			"task decomposition, coupling between subtasks, and required specialist capabilities",
			"available agent identities, authority ceilings, context costs, and coordination budget",
		},
		outputs: []string{
			"an agent organization chart with one accountable synthesis owner",
			"a delegation topology explaining why supervisor, peer, pipeline, debate, or market mode fits",
		},
		contraindications: []string{
			"create an agent team when one capable agent can complete the work with lower coordination risk",
		},
	},
	"agent-identity-capability": {
		inputs: []string{
			"current signed agent cards, tool allowlists, runtime health, and capability test results",
			"the task's required skills, data scopes, authority, and environmental constraints",
		},
		outputs: []string{
			"a capability-to-task match with unsupported claims called out",
			"an entitlement-bounded agent assignment or a missing-capability escalation",
		},
		contraindications: []string{
			"assign work from a self-declared capability that lacks current provenance or a passing capability check",
		},
	},
	"delegation-accountability": {
		inputs: []string{
			"the desired outcome, deliverable, deadline, constraints, budget, and escalation path",
			"the delegate's identity, accepted authority, dependencies, and communication channel",
		},
		outputs: []string{
			"a delegation contract with one accountable owner and explicit acceptance criteria",
			"a tracked acknowledgement, update cadence, and overdue escalation trigger",
		},
		contraindications: []string{
			"delegate authority, spending, or external communication beyond the recipient's accepted mandate",
		},
	},
	"agent-communication": {
		inputs: []string{
			"a schema-versioned message envelope with authenticated sender and recipient identities",
			"scoped task context, correlation IDs, provenance, sensitivity, and expiry metadata",
		},
		outputs: []string{
			"a validated and redacted inter-agent message with delivery status",
			"a rejection record for malformed, over-scoped, replayed, or authority-raising messages",
		},
		contraindications: []string{
			"forward secrets, unscoped memory, or remote instructions that attempt to modify policy or authority",
		},
	},
	"multi-agent-coordination": {
		inputs: []string{
			"independent work units, dependency edges, shared-state rules, and synthesis criteria",
			"agent availability, communication cost, disagreement policy, and completion deadline",
		},
		outputs: []string{
			"a sequential, parallel, review, debate, voting, consensus, or blackboard coordination plan",
			"a synthesis record preserving individual evidence, dissent, and unresolved conflicts",
		},
		contraindications: []string{
			"fan out tightly coupled work that requires shared mutable state or cannot be independently verified",
		},
	},
	"reasoning-methods": {
		inputs: []string{
			"the decision question, known facts, assumptions, constraints, and required reasoning depth",
			"retrieved evidence, plausible alternatives, and an explicit uncertainty threshold",
		},
		outputs: []string{
			"a bounded reasoning-method selection with stated assumptions and alternatives",
			"a conclusion-and-uncertainty report that omits private chain-of-thought",
		},
		contraindications: []string{
			"use elaborate decomposition or debate for a deterministic lookup, calculation, or simple transformation",
		},
	},
	"cognitive-agent-architecture": {
		inputs: []string{
			"a versioned world-state snapshot, active goals, intentions, and termination conditions",
			"allowed actions, observation channels, feedback signals, and escalation thresholds",
		},
		outputs: []string{
			"a bounded perceive-orient-decide-act or belief-desire-intention control loop",
			"a state-transition ledger with re-observation and termination evidence",
		},
		contraindications: []string{
			"run a persistent adaptive loop when a stateless reasoning pass or deterministic workflow is sufficient",
		},
	},
	"uncertainty-decision": {
		inputs: []string{
			"decision alternatives, uncertain variables, probability bases, and outcome utilities",
			"assumption ranges, information-gathering costs, reversibility, and decision deadline",
		},
		outputs: []string{
			"an expected-value, regret, sensitivity, and information-value comparison",
			"a calibrated recommendation that identifies decision-changing assumptions",
		},
		contraindications: []string{
			"present invented probabilities or expected values when no defensible basis can be established",
		},
	},
	"formal-planning": {
		inputs: []string{
			"goal state, current state, preconditions, effects, resources, time windows, and constraints",
			"dependency graph, contingency options, approval gates, and failure conditions",
		},
		outputs: []string{
			"an ordered plan with preconditions, dependencies, resource allocations, and critical path",
			"a contingency and escalation plan for unsatisfied or failed preconditions",
		},
		contraindications: []string{
			"claim an executable plan while resource, dependency, or temporal feasibility remains unknown",
		},
	},
	"workflow-modeling": {
		inputs: []string{
			"business states, allowed transitions, triggers, guards, human tasks, timers, and side effects",
			"approval nodes, idempotency boundaries, compensations, and terminal-state definitions",
		},
		outputs: []string{
			"a deterministic state-machine or BPMN-like workflow contract",
			"a transition table covering waiting, failure, compensation, cancellation, and completion",
		},
		contraindications: []string{
			"encode an exploratory adaptive agent loop as a deterministic workflow without stable states and guards",
		},
	},
	"reliable-execution": {
		inputs: []string{
			"an approved action envelope, idempotency key, target, timeout, and postconditions",
			"retry policy, side-effect classification, compensation path, and worker lease",
		},
		outputs: []string{
			"an attributable execution receipt with attempts, side effects, and postcondition evidence",
			"a recoverable retry, compensation, or dead-letter record when execution cannot complete",
		},
		contraindications: []string{
			"execute a side effect without idempotency protection, granted authority, or a verifiable postcondition",
		},
	},
	"autonomy-levels": {
		inputs: []string{
			"the requested action, risk classification, reversibility, and affected external parties",
			"active mode, framework ceilings, standing mandates, case approvals, and Constitution rules",
		},
		outputs: []string{
			"the effective autonomy level and every limiting ceiling",
			"an authority decision identifying whether HAI may observe, draft, prepare, execute, or must escalate",
		},
		contraindications: []string{
			"infer permission from technical capability, model confidence, prior unrelated approval, or user silence",
		},
	},
	"approval-control": {
		inputs: []string{
			"the exact proposed action, target, payload digest, consequences, and rollback limits",
			"authenticated approver identity, approval scope, expiry, and policy-required role",
		},
		outputs: []string{
			"a short-lived action-bound approval proof or an explicit rejection",
			"an immutable decision record linking approval to the action actually executed",
		},
		contraindications: []string{
			"reuse, widen, self-issue, or transfer an approval proof to a different action or target",
		},
	},
	"memory-architecture": {
		inputs: []string{
			"a source-linked candidate memory with type, confidence, sensitivity, and project scope",
			"existing related memories, corrections, retention policy, and consent state",
		},
		outputs: []string{
			"a deduplicated working, episodic, semantic, project, preference, procedural, social, or prospective memory proposal",
			"a lifecycle decision to store, merge, supersede, archive, review, or reject",
		},
		contraindications: []string{
			"store unsupported model output, transient chatter, secrets, or inferred preferences as durable fact",
		},
	},
	"personal-knowledge-management": {
		inputs: []string{
			"source-linked notes, documents, projects, areas, resources, archives, and decisions",
			"current organization rules, retrieval problems, duplicate candidates, and preservation requirements",
		},
		outputs: []string{
			"a non-destructive classification and link plan for personal knowledge",
			"a provenance-preserving merge or archive proposal with before-and-after locations",
		},
		contraindications: []string{
			"delete or collapse distinct source records merely to make the taxonomy appear cleaner",
		},
	},
	"retrieval-context": {
		inputs: []string{
			"a task-specific research question, project scope, privacy boundary, and token budget",
			"searchable source metadata, lexical index, embeddings, graph links, freshness, and confidence",
		},
		outputs: []string{
			"a ranked minimal context bundle with source URIs and rank explanations",
			"a retrieval-gap record identifying missing, stale, excluded, or contradictory context",
		},
		contraindications: []string{
			"load broad private history when narrower project-scoped evidence can answer the question",
		},
	},
	"truth-evidence": {
		inputs: []string{
			"atomic claims, their proposed source links, timestamps, and required authority level",
			"primary source content, deterministic validators, contradiction candidates, and freshness rules",
		},
		outputs: []string{
			"a claim-to-evidence graph with verified, supported, conflicting, uncertain, or unsupported status",
			"a blocking review item for consequential claims that lack adequate support",
		},
		contraindications: []string{
			"treat citation presence, model confidence, or repeated secondary claims as proof of source support",
		},
	},
	"ingestion-synchronization": {
		inputs: []string{
			"connector identity, granted scopes, exclusions, local-only setting, and source account",
			"last successful cursor, remote item identifiers, modification timestamps, and deletion markers",
		},
		outputs: []string{
			"an incremental deduplicated sync batch with raw-item and extraction provenance",
			"a checkpointed cursor plus review records for sensitive, failed, or uncertain extraction",
		},
		contraindications: []string{
			"perform an unrestricted historical reread when incremental cursors or source exclusions are available",
		},
	},
	"ambient-perception": {
		inputs: []string{
			"authorized event streams, scan cadence, interruption policy, and source-specific consent",
			"known deadlines, commitments, waiting states, stale thresholds, and active pursuits",
		},
		outputs: []string{
			"a provenance-linked proposal for a deadline, blocker, commitment, opportunity, or stale loop",
			"an interruption decision explaining why the signal is surfaced now or deferred",
		},
		contraindications: []string{
			"continuously surveil unapproved sources or convert weak ambient signals directly into external action",
		},
	},
	"human-ai-interaction": {
		inputs: []string{
			"the user's current decision, attention state, accessibility needs, and preferred interaction channel",
			"action status, ownership, risk, explanation depth, and available recovery paths",
		},
		outputs: []string{
			"a concise proposal, question, notification, or explanation matched to user attention",
			"an explicit user decision or correction linked to the affected operation",
		},
		contraindications: []string{
			"hide essential risk, error, ownership, or recovery information behind hover-only or advanced controls",
		},
	},
	"privacy-protection": {
		inputs: []string{
			"data categories, sensitivity, processing purpose, consent, location, recipients, and retention policy",
			"the minimum fields required by the task and available local-processing options",
		},
		outputs: []string{
			"a minimized and redacted data-use envelope with allowed recipients and retention",
			"a deny, local-only, review, export, correction, or deletion decision with audit evidence",
		},
		contraindications: []string{
			"share personal data because it is technically accessible rather than necessary and consented for the purpose",
		},
	},
	"security-zero-trust": {
		inputs: []string{
			"authenticated principal, requested resource, action, device or runtime, and trust signals",
			"least-privilege policy, tool allowlist, secret boundary, artifact provenance, and current health",
		},
		outputs: []string{
			"a deny-by-default authorization decision with enforced scope",
			"a sandbox, secret-isolation, or supply-chain control plan for the permitted operation",
		},
		contraindications: []string{
			"trust a caller, tool, dependency, or network location solely because it is internal or previously used",
		},
	},
	"agent-threat-modeling": {
		inputs: []string{
			"agent, model, tool, memory, source, runtime, and external-system trust boundaries",
			"candidate prompt-injection, confused-deputy, exfiltration, spoofing, poisoning, and runaway-loop scenarios",
		},
		outputs: []string{
			"an agent-specific threat model with attack paths, mitigations, and residual risk",
			"a quarantine, deny, test, or review requirement mapped to each material threat",
		},
		contraindications: []string{
			"accept untrusted content as policy, tool authority, memory truth, or an authenticated agent message",
		},
	},
	"safety-engineering": {
		inputs: []string{
			"hazards, affected people and assets, severity, likelihood, detectability, and reversibility",
			"existing preventive, detective, recovery, rollback, emergency-stop, and incident controls",
		},
		outputs: []string{
			"a hazard register with layered control assignments and residual risk",
			"a fail-safe execution, rollback, emergency-stop, and incident-response plan",
		},
		contraindications: []string{
			"rely on a single model judgment or unexercised control for a high-impact hazard",
		},
	},
	"ai-governance": {
		inputs: []string{
			"system purpose, affected users, lifecycle owner, risk tier, jurisdictions, and decision impact",
			"applicable policies, oversight roles, transparency duties, records, and review cadence",
		},
		outputs: []string{
			"an accountable AI-use record with risk classification and control mapping",
			"a lifecycle approval, remediation, monitoring, or retirement decision",
		},
		contraindications: []string{
			"represent policy documentation as effective governance without an owner, evidence, and exercised controls",
		},
	},
	"model-intelligence": {
		inputs: []string{
			"task capability requirements, difficulty, reasoning level, latency target, and validation plan",
			"current provider health, model versions, local availability, quotas, prices, and paid-use policy",
		},
		outputs: []string{
			"a cheapest-capable model selection with skipped-model reasons and estimated cost",
			"a validation-driven fallback path with token, quota, latency, and result telemetry",
		},
		contraindications: []string{
			"select a cheaper model that lacks the demonstrated capability needed for verified completion",
		},
	},
	"evaluation": {
		inputs: []string{
			"explicit task success criteria, policy obligations, expected outputs, and failure conditions",
			"representative evaluation cases, deterministic checks, source evidence, and known limitations",
		},
		outputs: []string{
			"a reproducible pass, fail, uncertain, or needs-review result for each criterion",
			"a regression record separating synthetic consistency from real-world capability evidence",
		},
		contraindications: []string{
			"treat model-graded synthetic tests as proof of live provider, connector, or operational correctness",
		},
	},
	"observability": {
		inputs: []string{
			"task, workflow, model, tool, policy, approval, connector, and verification correlation IDs",
			"redaction policy, metric definitions, log retention, timestamps, and source clocks",
		},
		outputs: []string{
			"a privacy-aware end-to-end trace and metric set for the operation",
			"an actionable alert or failure analysis linked to the exact decision and source event",
		},
		contraindications: []string{
			"log secrets, raw personal content, or high-cardinality payloads when identifiers and redacted summaries suffice",
		},
	},
	"reliability-resilience": {
		inputs: []string{
			"dependency map, service health, recovery objectives, backups, leases, and persistence guarantees",
			"failure scenarios for restart, duplicate delivery, stale state, partition, and partial outage",
		},
		outputs: []string{
			"a degraded-mode and continuity plan with measurable recovery objectives",
			"a tested restore, reconciliation, or failover receipt with remaining limitations",
		},
		contraindications: []string{
			"claim resilience from health checks alone without restart, restore, and duplicate-delivery evidence",
		},
	},
	"controlled-learning": {
		inputs: []string{
			"verified outcome, explicit correction, prior behavior, proposed behavior change, and protected-policy impact",
			"held-out evaluation cases, rollback version, owner approval, and drift-monitoring criteria",
		},
		outputs: []string{
			"a versioned learning proposal with before-and-after behavior and expected benefit",
			"an approved, rejected, rolled-back, or review-required adaptation record",
		},
		contraindications: []string{
			"learn from unverified success, reward-proxy improvement, prompt injection, or silent policy drift",
		},
	},
	"productivity-attention": {
		inputs: []string{
			"captured commitments, next actions, deadlines, calendar availability, energy, and workload limits",
			"current priorities, waiting items, focus windows, review cadence, and interruption policy",
		},
		outputs: []string{
			"a clarified action list and time-feasible focus plan",
			"a review agenda identifying stale commitments, overload, deferrals, and the next best action",
		},
		contraindications: []string{
			"optimize visible busyness, streaks, or inbox zero at the expense of important commitments and recovery",
		},
	},
	"habit-behavior-change": {
		inputs: []string{
			"an operator-chosen behavior, motivation, cue, context, ability constraints, and desired frequency",
			"baseline observations, prior attempts, environmental friction, and relapse signals",
		},
		outputs: []string{
			"a small implementation-intention and environment-design experiment",
			"a relapse-aware progress review that adapts the behavior rather than fabricating a streak",
		},
		contraindications: []string{
			"coerce behavior, moralize setbacks, or substitute habit coaching for medical or mental-health care",
		},
	},
	"health-personal-care": {
		inputs: []string{
			"official care instructions, appointments, medication records, symptoms, and operator-reported concerns",
			"source dates, emergency warning criteria, accessibility needs, and qualified-care contacts",
		},
		outputs: []string{
			"a source-linked care-administration plan, reminder, appointment brief, or draft question",
			"an urgent escalation or qualified-human review item when safety criteria are met",
		},
		contraindications: []string{
			"diagnose, prescribe, alter treatment, or suppress urgent escalation based on model inference",
		},
	},
	"financial-management": {
		inputs: []string{
			"source transactions, invoices, balances, due dates, currencies, periods, and account ownership",
			"budget rules, deterministic formulas, tax context, anomaly thresholds, and approval policy",
		},
		outputs: []string{
			"a reconciled budget, cash-flow, invoice, debt, or anomaly analysis with calculation trace",
			"an approval-gated payment, commitment, correction, or follow-up proposal",
		},
		contraindications: []string{
			"initiate payment, accept terms, or report financial totals without source reconciliation and explicit approval",
		},
	},
	"home-garden-assets": {
		inputs: []string{
			"asset, property, garden, warranty, maintenance, safety, and service-provider records",
			"condition evidence, seasonal timing, quotes, dependencies, access constraints, and budget",
		},
		outputs: []string{
			"a prioritized maintenance, repair, inventory, or seasonal-work plan",
			"a source-linked completion record with before-and-after evidence and warranty impact",
		},
		contraindications: []string{
			"schedule unsafe work, choose a contractor, or commit spend without required expertise and approval",
		},
	},
	"work-service-delivery": {
		inputs: []string{
			"client request, agreed scope, price constraints, schedule, dependencies, and acceptance criteria",
			"delivery evidence, quality checks, change requests, communication history, and invoice state",
		},
		outputs: []string{
			"an end-to-end service plan from scope and quote through handoff and follow-up",
			"a quality-gated completion or exception record tied to client acceptance",
		},
		contraindications: []string{
			"promise scope, price, delivery date, or completion without authorized agreement and deliverable evidence",
		},
	},
	"entrepreneurship-venture": {
		inputs: []string{
			"problem hypothesis, target users, alternatives, market evidence, constraints, and strategic objective",
			"experiment design, budget ceiling, stage gate, success threshold, and downside risk",
		},
		outputs: []string{
			"a testable business or product hypothesis with the smallest useful experiment",
			"a stage-gate recommendation to continue, pivot, pause, or stop based on evidence",
		},
		contraindications: []string{
			"present market size, customer demand, or revenue projections as validated without primary evidence",
		},
	},
	"legal-government-case": {
		inputs: []string{
			"primary correspondence, filings, contracts, decisions, case identifiers, parties, and dated events",
			"deadlines, jurisdiction, requested remedy, claim-to-evidence links, and unresolved contradictions",
		},
		outputs: []string{
			"a source-linked case timeline, issue list, evidence bundle, and deadline plan",
			"an approval-gated factual draft or qualified-review request with unsupported claims flagged",
		},
		contraindications: []string{
			"give legal conclusions, file, send, concede, or make factual allegations without source support and approval",
		},
	},
	"communication": {
		inputs: []string{
			"recipient, relationship, channel, purpose, desired response, tone constraints, and send authority",
			"source support for factual claims, prior thread context, sensitivity, and follow-up deadline",
		},
		outputs: []string{
			"a recipient-appropriate draft with facts, requests, boundaries, and source references",
			"an approval, send, publish, schedule-follow-up, or needs-review decision",
		},
		contraindications: []string{
			"impersonate the operator, publish, or send consequential claims without factual support and required approval",
		},
	},
	"relationships-care": {
		inputs: []string{
			"operator-stated relationship context, commitments, boundaries, care needs, and desired outcome",
			"relevant communication history, uncertainty about others' intent, and privacy expectations",
		},
		outputs: []string{
			"a respectful conversation, care, boundary, or follow-up proposal that preserves agency",
			"a commitment reminder or conflict-preparation brief with inferred intent clearly labelled",
		},
		contraindications: []string{
			"manipulate, surveil, impersonate, diagnose, or claim certainty about another person's private intent",
		},
	},
	"learning-competence": {
		inputs: []string{
			"a learning objective, current demonstrated skill, target performance, deadline, and available practice time",
			"assessment rubric, practice results, retrieval history, errors, and source materials",
		},
		outputs: []string{
			"a gap-targeted deliberate-practice and retrieval schedule",
			"a demonstrated-competence assessment with evidence and the next learning adjustment",
		},
		contraindications: []string{
			"equate content consumption, course completion, or model-generated answers with demonstrated competence",
		},
	},
	"travel-mobility": {
		inputs: []string{
			"origin, destination, dates, calendar constraints, accessibility, travelers, documents, and preferences",
			"fresh timetable or route data, costs, cancellation terms, transfer risk, and contingency options",
		},
		outputs: []string{
			"a time-feasible itinerary with travel buffers, documents, costs, and alternatives",
			"an approval-gated booking proposal or disruption recovery plan based on refreshed data",
		},
		contraindications: []string{
			"book, spend, or rely on stale timetable, entry, accessibility, or cancellation information",
		},
	},
	"emergency-continuity": {
		inputs: []string{
			"critical contacts, assets, accounts, records, dependencies, recovery objectives, and incapacity authority",
			"backup status, recovery channels, communication fallbacks, emergency triggers, and last exercise date",
		},
		outputs: []string{
			"a protected emergency and continuity runbook with role-specific access",
			"an exercise, backup, contact-verification, or recovery-gap remediation record",
		},
		contraindications: []string{
			"expose recovery secrets or assume emergency authority that has not been explicitly pre-authorized",
		},
	},
	"agent-development-adapters": {
		inputs: []string{
			"a concrete missing capability, adapter protocol, upstream repository, license, and maintenance evidence",
			"sandbox boundary, tool contract, data scopes, resource limits, and governance compatibility tests",
		},
		outputs: []string{
			"a benchmarked adapter compatibility report against HAI's Go control plane",
			"a sandboxed prototype, rejection, or approval proposal with rollback and ownership",
		},
		contraindications: []string{
			"install an agent framework because it is popular when existing HAI capabilities meet the measured need",
		},
	},
	"durable-workflow-platforms": {
		inputs: []string{
			"measured single-node reliability gap, workload shape, recovery objectives, and scaling requirement",
			"candidate platform architecture, license, operating cost, migration plan, and rollback path",
		},
		outputs: []string{
			"a comparative durable-workflow platform benchmark against the current worker",
			"a prototype, adoption, defer, or reject decision with failure-recovery evidence",
		},
		contraindications: []string{
			"add distributed orchestration before a measured durability, throughput, or availability requirement exists",
		},
	},
	"memory-knowledge-implementations": {
		inputs: []string{
			"a measured PostgreSQL search, graph, vector, retention, or scale limitation",
			"representative data, privacy classification, benchmark queries, export, deletion, and recovery requirements",
		},
		outputs: []string{
			"a reproducible specialized-store benchmark against the PostgreSQL baseline",
			"a migration, hybrid, defer, or reject proposal preserving canonical provenance",
		},
		contraindications: []string{
			"introduce a second source of truth without a measured benefit and a tested export, deletion, and recovery path",
		},
	},
	"policy-security-implementations": {
		inputs: []string{
			"a concrete policy, identity, secret, provenance, or isolation gap and its threat model",
			"candidate component maintenance, license, failure mode, recovery plan, and integration boundary",
		},
		outputs: []string{
			"a fail-closed security-component evaluation with attack-surface comparison",
			"a sandboxed prototype, migration proposal, defer decision, or rejection with recovery evidence",
		},
		contraindications: []string{
			"replace an existing security boundary without proving stricter failure behavior and operational recovery",
		},
	},
	"evaluation-observability-implementations": {
		inputs: []string{
			"a specific evaluation, tracing, security-testing, metrics, logging, or error-tracking coverage gap",
			"representative redacted telemetry, privacy constraints, operating cost, retention, and integration requirements",
		},
		outputs: []string{
			"a reproducible tool benchmark against current evaluation and observability coverage",
			"an instrument, prototype, defer, or reject decision with data-exposure and cost analysis",
		},
		contraindications: []string{
			"add telemetry tooling that duplicates coverage or exports sensitive data without decision-useful benefit",
		},
	},
}

func operationalContractFor(spec catalogSpec) catalogOperationalContract {
	contract, ok := builtinCatalogOperationalContracts[spec.id]
	if !ok {
		panic(fmt.Sprintf("framework %q is missing its built-in operational contract", spec.id))
	}
	return contract
}

func materializeWorkflow(spec catalogSpec, contract catalogOperationalContract) []string {
	return uniqueStrings([]string{
		"validate " + contract.inputs[0],
		"apply " + spec.name + " to " + spec.problems[0],
		"produce " + contract.outputs[0],
		"verify " + spec.evaluation[0],
	})
}

func materializeDecisionRules(spec catalogSpec, contract catalogOperationalContract) []string {
	rules := []string{
		fmt.Sprintf(
			"apply only when %s is evidenced and the task materially involves %s",
			spec.triggers[0],
			spec.problems[0],
		),
		"stay within this authority boundary: " + spec.authority,
	}
	for _, contraindication := range contract.contraindications {
		rules = append(rules, "do not use this framework to "+contraindication)
	}
	return uniqueStrings(rules)
}
