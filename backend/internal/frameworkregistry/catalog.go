package frameworkregistry

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var catalogSemanticVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var catalogPlaceholderPattern = regexp.MustCompile(`(?i)\b(tbd|placeholder|framework[- ]specific|input goes here|output goes here)\b`)

var forbiddenGenericCatalogEntries = map[string]struct{}{
	"operator-scoped request or source signal":                                             {},
	"current risk and authority context":                                                   {},
	"framework-scoped recommendation":                                                      {},
	"authority and approval constraints":                                                   {},
	"evidence and completion requirements":                                                 {},
	"validate inputs and source trust":                                                     {},
	"apply the framework's decision rules":                                                 {},
	"check constitution, authority, risk, and evidence":                                    {},
	"produce a reviewable recommendation or bounded plan":                                  {},
	"verify outcome and record learning":                                                   {},
	"select this framework only when its triggers or problem types materially match":       {},
	"prefer the least complex combination that covers the task and its safety obligations": {},
	"do not convert capability into authority":                                             {},
}

type catalogSpec struct {
	id              string
	version         string
	name            string
	family          string
	purpose         string
	problems        []string
	triggers        []string
	agents          []string
	authority       string
	maxAutonomy     int
	riskCeiling     string
	evidence        []string
	evaluation      []string
	implementations []string
	status          string
	conflicts       []string
}

// BuiltinCatalog returns a fresh copy of the complete v2 framework taxonomy.
// The records are configuration and decision metadata; implementation products
// named by the catalog are candidates, not installed or trusted dependencies.
func BuiltinCatalog() []Framework {
	specs := []catalogSpec{
		{
			id: "human-sovereignty", name: "Human sovereignty", family: "constitutional",
			purpose:  "Keep the operator as the final authority and prevent HAI from substituting its own goals, values, or identity.",
			problems: []string{"all tasks", "value conflict", "authority boundary", "personal decision"},
			triggers: []string{"always", "preference", "identity", "values", "who decides", "my life"},
			agents:   []string{"chief_of_staff", "policy_guardian"}, authority: "observe and recommend unless the Constitution grants a narrower standing mandate",
			maxAutonomy: 5, riskCeiling: "high",
			evidence:   []string{"active Constitution version", "verified operator identity", "applicable approval record"},
			evaluation: []string{"no action exceeds delegated authority", "operator can inspect and reverse the recommendation"},
		},
		{
			id: "whole-life-ontology", name: "Whole-life ontology", family: "life_model",
			purpose:  "Classify people, needs, assets, obligations, projects, cases, opportunities, and risks in one stable life model.",
			problems: []string{"life-domain classification", "cross-domain context", "entity linking", "portfolio overview"},
			triggers: []string{"project", "case", "person", "relationship", "home", "health", "finance", "work", "venture"},
			agents:   []string{"chief_of_staff", "ontology_steward"}, authority: "read and classify",
			maxAutonomy: 4, riskCeiling: "medium",
			evidence:   []string{"source-linked entity references", "owner-confirmed corrections for ambiguous identity"},
			evaluation: []string{"domain and entity assignments are explainable", "duplicate concepts are merged without losing provenance"},
		},
		{
			id: "needs-wellbeing", name: "Human needs and wellbeing", family: "life_model",
			purpose:  "Relate work to safety, health, belonging, esteem, autonomy, competence, meaning, and material stability without treating one hierarchy as universal truth.",
			problems: []string{"need assessment", "wellbeing planning", "life priority", "trade-off"},
			triggers: []string{"need", "wellbeing", "maslow", "safety", "belonging", "esteem", "meaning", "quality of life"},
			agents:   []string{"chief_of_staff", "wellbeing_planner"}, authority: "recommend only",
			maxAutonomy: 2, riskCeiling: "medium",
			evidence:   []string{"operator-stated needs", "recent outcome signals", "uncertainty label"},
			evaluation: []string{"recommendations map to an explicit need", "operator can reject the inferred need"},
		},
		{
			id: "capacity-state", name: "Personal state and capacity", family: "life_model",
			purpose:  "Plan within current time, energy, attention, stress, health, money, and environmental constraints.",
			problems: []string{"capacity planning", "overload", "scheduling", "task sizing"},
			triggers: []string{"tired", "stress", "overloaded", "capacity", "energy", "attention", "available time", "budget"},
			agents:   []string{"chief_of_staff", "capacity_planner"}, authority: "recommend and reschedule reversible internal work",
			maxAutonomy: 5, riskCeiling: "medium",
			evidence:   []string{"fresh calendar or workload signal", "operator-provided capacity where available"},
			evaluation: []string{"plan fits declared constraints", "stale capacity data is visibly marked"},
		},
		{
			id: "goal-hierarchy", name: "Goal hierarchy", family: "life_model",
			purpose:  "Trace atomic actions through tasks, workflows, projects, pursuits, strategic outcomes, needs, and constitutional values.",
			problems: []string{"goal decomposition", "alignment", "scope control", "completion definition"},
			triggers: []string{"goal", "objective", "outcome", "pursuit", "project", "why", "success criteria"},
			agents:   []string{"chief_of_staff", "planner"}, authority: "plan and simulate",
			maxAutonomy: 4, riskCeiling: "medium",
			evidence:   []string{"explicit desired outcome", "traceable parent pursuit or reviewable candidate"},
			evaluation: []string{"every action has a parent objective", "completion is measurable rather than asserted"},
		},
		{
			id: "intake-triage", name: "Intake and triage", family: "orchestration",
			purpose:  "Normalize incoming requests and signals, classify urgency and risk, and decide whether to ignore, ask, plan, route, or escalate.",
			problems: []string{"task intake", "signal classification", "request clarification", "routing"},
			triggers: []string{"new request", "incoming", "email", "message", "document", "event", "unknown"},
			agents:   []string{"intake_agent", "risk_reviewer"}, authority: "classify and propose",
			maxAutonomy: 3, riskCeiling: "high",
			evidence:   []string{"original input retained as untrusted source", "classification confidence and reason"},
			evaluation: []string{"no input is silently converted into high-risk action", "ambiguous inputs enter review"},
		},
		{
			id: "multi-criteria-prioritization", name: "Multi-criteria prioritization", family: "orchestration",
			purpose:  "Rank work using urgency, impact, risk, value, effort, reversibility, dependency, and operator involvement.",
			problems: []string{"priority ranking", "portfolio choice", "next best action", "resource allocation"},
			triggers: []string{"priority", "what next", "urgent", "important", "rank", "backlog", "attention"},
			agents:   []string{"chief_of_staff", "portfolio_planner"}, authority: "recommend and reorder internal queues",
			maxAutonomy: 5, riskCeiling: "medium",
			evidence:   []string{"scored criteria", "source-backed deadlines", "explicit tie-break rule"},
			evaluation: []string{"ranking factors are visible", "urgent low-value work does not automatically displace critical work"},
		},
		{
			id: "multi-agent-organization", name: "Multi-agent organization", family: "agents",
			purpose:  "Choose an organizational mode such as chief-of-staff, supervisor, peer team, pipeline, debate, or market allocation.",
			problems: []string{"multi-agent task", "specialist delegation", "parallel research", "complex project"},
			triggers: []string{"multi-agent", "specialist", "team", "delegate", "parallel", "supervisor", "debate"},
			agents:   []string{"chief_of_staff", "agent_team_manager"}, authority: "compose agents inside the task authority ceiling",
			maxAutonomy: 5, riskCeiling: "medium",
			evidence:   []string{"agent roster and capability claims", "explicit reporting structure"},
			evaluation: []string{"coordination cost is justified", "one accountable agent owns synthesis"},
		},
		{
			id: "agent-identity-capability", name: "Agent identity and capability", family: "agents",
			purpose:  "Represent each agent with an identity, scope, tools, data access, competence, limitations, and escalation contract.",
			problems: []string{"agent selection", "capability matching", "tool entitlement", "specialist role"},
			triggers: []string{"agent", "capability", "skill", "role", "tool access", "specialist"},
			agents:   []string{"agent_team_manager", "policy_guardian"}, authority: "inspect and assign within existing entitlements",
			maxAutonomy: 5, riskCeiling: "high",
			evidence:   []string{"current agent card", "tool allowlist", "runtime health and capability provenance"},
			evaluation: []string{"assigned agent has proven required capability", "missing capability is escalated rather than guessed"},
		},
		{
			id: "delegation-accountability", name: "Delegation and accountability", family: "agents",
			purpose:  "Create a clear delegation contract with objective, deliverable, limits, dependencies, deadline, evidence, and escalation.",
			problems: []string{"delegation", "work assignment", "handoff", "accountability"},
			triggers: []string{"delegate", "assign", "va", "freelancer", "owner", "responsible", "handoff"},
			agents:   []string{"chief_of_staff", "delegation_manager"}, authority: "prepare or issue internal assignments within standing permissions",
			maxAutonomy: 5, riskCeiling: "medium",
			evidence:   []string{"delegation contract", "acceptance or acknowledgement", "deliverable evidence"},
			evaluation: []string{"one accountable owner is named", "authority limits are explicit"},
		},
		{
			id: "agent-communication", name: "Agent communication", family: "agents",
			purpose:  "Exchange typed task, context, evidence, progress, exception, approval, and result messages across agents and runtimes.",
			problems: []string{"agent interoperability", "message contract", "context transfer", "runtime bridge"},
			triggers: []string{"mcp", "a2a", "agent protocol", "message", "handoff", "runtime adapter"},
			agents:   []string{"interoperability_broker", "policy_guardian"}, authority: "transmit only scoped, redacted envelopes",
			maxAutonomy: 5, riskCeiling: "high",
			evidence:        []string{"schema-valid envelope", "sender identity", "correlation and provenance identifiers"},
			evaluation:      []string{"messages validate against the contract", "untrusted remote instructions cannot raise authority"},
			implementations: []string{"MCP", "A2A", "typed internal events"},
		},
		{
			id: "multi-agent-coordination", name: "Multi-agent coordination patterns", family: "agents",
			purpose:  "Select sequential, parallel, map-reduce, review, debate, voting, consensus, or blackboard coordination according to task structure.",
			problems: []string{"parallel work", "review pipeline", "consensus", "distributed analysis"},
			triggers: []string{"parallel", "compare", "reviewer", "debate", "vote", "consensus", "map reduce"},
			agents:   []string{"agent_team_manager", "synthesis_agent"}, authority: "coordinate reasoning only",
			maxAutonomy: 4, riskCeiling: "medium",
			evidence:   []string{"coordination plan", "individual outputs", "synthesis provenance"},
			evaluation: []string{"parallel work is independent enough to justify fan-out", "disagreement is retained in synthesis"},
		},
		{
			id: "reasoning-methods", name: "Cognitive and reasoning methods", family: "reasoning",
			purpose:  "Choose bounded decomposition, critique, reflection, hypothesis testing, causal analysis, or analogical reasoning suited to the task.",
			problems: []string{"complex reasoning", "analysis", "diagnosis", "planning", "creative options"},
			triggers: []string{"reason", "analyze", "complex", "compare", "diagnose", "hypothesis", "root cause"},
			agents:   []string{"reasoning_agent", "critic"}, authority: "reason and recommend",
			maxAutonomy: 3, riskCeiling: "high",
			evidence:   []string{"stated assumptions", "retrieved evidence", "alternative explanations"},
			evaluation: []string{"method fits the question", "final output exposes conclusions and uncertainty without hidden chain-of-thought"},
			conflicts:  []string{"cognitive-agent-architecture"},
		},
		{
			id: "cognitive-agent-architecture", name: "Cognitive-agent architectures", family: "reasoning",
			purpose:  "Structure perceive-orient-decide-act, belief-desire-intention, goal-plan-action, or shared-workspace loops.",
			problems: []string{"agent loop design", "long-running goal", "stateful autonomy", "architecture"},
			triggers: []string{"cognitive architecture", "ooda", "bdi", "agent loop", "world state", "long running"},
			agents:   []string{"chief_of_staff", "world_state_observer", "planner", "critic"}, authority: "plan and simulate; execution remains separately gated",
			maxAutonomy: 4, riskCeiling: "high",
			evidence:   []string{"fresh world-state snapshot", "goal and intention ledger", "action feedback"},
			evaluation: []string{"loop terminates or escalates", "state is re-observed before consequential actions"},
			conflicts:  []string{"reasoning-methods", "workflow-modeling"},
		},
		{
			id: "uncertainty-decision", name: "Decision-making under uncertainty", family: "reasoning",
			purpose:  "Represent confidence, ambiguity, expected value, regret, sensitivity, and information value instead of hiding uncertainty.",
			problems: []string{"uncertain decision", "forecast", "trade-off", "incomplete evidence"},
			triggers: []string{"uncertain", "confidence", "probability", "forecast", "risk", "what if", "unknown"},
			agents:   []string{"decision_analyst", "risk_reviewer"}, authority: "recommend only for consequential decisions",
			maxAutonomy: 2, riskCeiling: "high",
			evidence:   []string{"assumption register", "confidence basis", "sensitivity analysis where material"},
			evaluation: []string{"uncertainty is calibrated", "decision changes are tested against plausible ranges"},
		},
		{
			id: "formal-planning", name: "Formal planning", family: "planning",
			purpose:  "Model goals, preconditions, effects, dependencies, resources, time, constraints, and contingency branches.",
			problems: []string{"multi-step plan", "dependency planning", "resource planning", "scheduling"},
			triggers: []string{"plan", "dependency", "critical path", "resource", "schedule", "contingency", "constraint"},
			agents:   []string{"planner", "constraint_checker"}, authority: "plan and simulate",
			maxAutonomy: 4, riskCeiling: "medium",
			evidence:   []string{"goal state", "known constraints", "dependency and resource assumptions"},
			evaluation: []string{"plan is executable under constraints", "critical dependencies have fallback or escalation"},
		},
		{
			id: "workflow-modeling", name: "Workflow modelling", family: "planning",
			purpose:  "Represent state machines, BPMN-like flows, event-condition-action rules, human tasks, timers, and compensations.",
			problems: []string{"workflow design", "state machine", "business process", "human-in-loop process"},
			triggers: []string{"workflow", "state", "transition", "bpmn", "event", "timer", "compensation"},
			agents:   []string{"workflow_designer", "policy_guardian"}, authority: "design and configure; activation requires existing permissions",
			maxAutonomy: 5, riskCeiling: "high",
			evidence:   []string{"state transition contract", "failure and compensation path", "approval nodes"},
			evaluation: []string{"invalid transitions are rejected", "waiting and failure states are explicit"},
			conflicts:  []string{"cognitive-agent-architecture"},
		},
		{
			id: "reliable-execution", name: "Reliable execution", family: "execution",
			purpose:  "Use idempotency, durable queues, retries, timeouts, circuit breakers, leases, compensation, and dead-letter review.",
			problems: []string{"task execution", "worker reliability", "retry", "external side effect"},
			triggers: []string{"execute", "worker", "retry", "timeout", "idempotent", "failure", "recover"},
			agents:   []string{"execution_coordinator", "reliability_monitor"}, authority: "execute only after the policy layer grants authority",
			maxAutonomy: 8, riskCeiling: "high",
			evidence:   []string{"idempotency key", "action receipt", "postcondition verification"},
			evaluation: []string{"retries do not duplicate side effects", "failed work remains visible and recoverable"},
		},
		{
			id: "autonomy-levels", name: "Autonomy levels 0-10", family: "governance",
			purpose:  "Express authority from observe-only through bounded autonomous execution without conflating capability with permission.",
			problems: []string{"authority decision", "standing mandate", "autonomy setting", "execution policy"},
			triggers: []string{"autonomy", "permission", "authority", "automatic", "standing approval", "execute"},
			agents:   []string{"policy_guardian", "chief_of_staff"}, authority: "calculate a ceiling; never grant itself authority",
			maxAutonomy: 10, riskCeiling: "high",
			evidence:   []string{"active Constitution", "current mode", "standing mandate or case approval", "risk classification"},
			evaluation: []string{"actual action level is at or below every applicable ceiling", "authority escalation requires explicit approval"},
		},
		{
			id: "approval-control", name: "Approval control", family: "governance",
			purpose:  "Require informed, scoped, attributable approval for risky, irreversible, financial, legal, public, account, or destructive actions.",
			problems: []string{"human approval", "high-risk action", "exception decision", "consent"},
			triggers: []string{"approve", "send", "publish", "pay", "delete", "legal", "government", "account change", "financial"},
			agents:   []string{"approval_broker", "policy_guardian"}, authority: "prepare approval request; cannot self-approve",
			maxAutonomy: 6, riskCeiling: "high",
			evidence:   []string{"exact proposed action", "risk and consequences", "approver identity", "scope and expiry"},
			evaluation: []string{"approval matches the executed action", "replay or scope expansion is blocked"},
		},
		{
			id: "memory-architecture", name: "Memory architecture", family: "knowledge",
			purpose:  "Separate working, episodic, semantic, project, preference, procedural, social, and prospective memory with lifecycle controls.",
			problems: []string{"memory storage", "context continuity", "preference learning", "lesson retention"},
			triggers: []string{"remember", "memory", "preference", "history", "lesson", "context", "recall"},
			agents:   []string{"memory_steward", "privacy_guardian"}, authority: "propose memory; store only verified or owner-confirmed records",
			maxAutonomy: 5, riskCeiling: "medium",
			evidence:   []string{"source reference", "confidence", "memory type", "retention state"},
			evaluation: []string{"irrelevant memory is not loaded", "corrections supersede rather than silently overwrite provenance"},
		},
		{
			id: "personal-knowledge-management", name: "Personal knowledge management", family: "knowledge",
			purpose:  "Organize projects, areas, resources, archives, notes, links, decisions, and maps of content without forcing one method.",
			problems: []string{"knowledge organization", "notes", "project context", "archive"},
			triggers: []string{"notes", "knowledge", "para", "zettelkasten", "map of content", "archive", "organize"},
			agents:   []string{"knowledge_curator"}, authority: "classify, link, and propose organization",
			maxAutonomy: 5, riskCeiling: "low",
			evidence:   []string{"original source link", "classification reason", "non-destructive change log"},
			evaluation: []string{"information remains findable", "organization reduces duplication without deleting evidence"},
		},
		{
			id: "retrieval-context", name: "Retrieval and context", family: "knowledge",
			purpose:  "Combine semantic, lexical, metadata, graph, project, recency, and confidence ranking while respecting token and privacy budgets.",
			problems: []string{"context retrieval", "search", "rag", "source discovery"},
			triggers: []string{"search", "retrieve", "context", "rag", "relevant", "find", "source"},
			agents:   []string{"context_planner", "retrieval_agent"}, authority: "read scoped sources",
			maxAutonomy: 4, riskCeiling: "medium",
			evidence:   []string{"retrieval query", "rank factors", "source URI and freshness"},
			evaluation: []string{"relevant evidence is recalled", "unrelated private context is excluded"},
		},
		{
			id: "truth-evidence", name: "Knowledge and truth", family: "knowledge",
			purpose:  "Separate claims from evidence, support temporal truth, detect conflict, and prevent unsupported output becoming fact or action.",
			problems: []string{"factual answer", "claim verification", "evidence graph", "conflict detection"},
			triggers: []string{"fact", "evidence", "claim", "verify", "source", "truth", "citation", "contradiction"},
			agents:   []string{"evidence_agent", "verification_critic"}, authority: "verify and block unsupported consequential claims",
			maxAutonomy: 4, riskCeiling: "high",
			evidence:   []string{"claim-to-source links", "source authority and freshness", "deterministic checks where possible"},
			evaluation: []string{"citation recall and precision", "unsupported claims remain reviewable and do not drive action"},
		},
		{
			id: "ingestion-synchronization", name: "Data ingestion and synchronization", family: "knowledge",
			purpose:  "Connect sources with least privilege, metadata-first intake, incremental cursors, deduplication, extraction, and provenance.",
			problems: []string{"connector sync", "data import", "incremental ingestion", "extraction"},
			triggers: []string{"sync", "connector", "import", "webhook", "backfill", "cursor", "source account"},
			agents:   []string{"connector_agent", "extraction_agent", "privacy_guardian"}, authority: "read only within connector permissions unless separately approved",
			maxAutonomy: 5, riskCeiling: "high",
			evidence:   []string{"permission grant", "sync cursor", "raw item identity", "extraction provenance"},
			evaluation: []string{"incremental sync does not duplicate records", "revocation stops future access"},
		},
		{
			id: "ambient-perception", name: "Perception and ambient intelligence", family: "proactivity",
			purpose:  "Observe authorized event streams for deadlines, commitments, blockers, stale work, and opportunities without continuous indiscriminate surveillance.",
			problems: []string{"ambient scan", "open-loop detection", "deadline detection", "proactive suggestion"},
			triggers: []string{"ambient", "monitor", "open loop", "deadline", "stale", "proactive", "background"},
			agents:   []string{"ambient_observer", "opportunity_classifier", "privacy_guardian"}, authority: "observe and propose by default",
			maxAutonomy: 3, riskCeiling: "medium",
			evidence:   []string{"authorized source event", "freshness", "proposal provenance"},
			evaluation: []string{"useful proposal rate", "interruption burden", "no proposal becomes work without lifecycle policy"},
		},
		{
			id: "human-ai-interaction", name: "Human-AI interaction", family: "interaction",
			purpose:  "Use dialogue, proposals, progressive disclosure, explainability, notifications, and correction loops matched to user attention.",
			problems: []string{"user interaction", "decision support", "explanation", "notification"},
			triggers: []string{"ask", "chat", "explain", "notify", "proposal", "dashboard", "review"},
			agents:   []string{"interaction_coordinator"}, authority: "communicate and collect explicit decisions",
			maxAutonomy: 3, riskCeiling: "medium",
			evidence:   []string{"decision context", "status and ownership", "accessible recovery path"},
			evaluation: []string{"user can understand the next action", "important state is not hidden in hover-only content"},
		},
		{
			id: "privacy-protection", name: "Privacy protection", family: "governance",
			purpose:  "Apply data minimization, purpose limitation, local-first processing, retention limits, consent, redaction, and deletion controls.",
			problems: []string{"personal data", "sensitive source", "cloud sharing", "retention"},
			triggers: []string{"private", "personal data", "sensitive", "secret", "local only", "share", "retention"},
			agents:   []string{"privacy_guardian", "data_steward"}, authority: "block or redact data flows that exceed consent",
			maxAutonomy: 5, riskCeiling: "high",
			evidence:   []string{"purpose and permission", "data classification", "processing location"},
			evaluation: []string{"minimum necessary data is processed", "exports and deletion requests are auditable"},
		},
		{
			id: "security-zero-trust", name: "Security and zero trust", family: "governance",
			purpose:  "Enforce authenticated identities, least privilege, deny-by-default tools, secret isolation, secure supply chains, and tamper-evident audit.",
			problems: []string{"authentication", "authorization", "secret handling", "runtime isolation", "supply chain"},
			triggers: []string{"security", "credential", "token", "permission", "runtime", "container", "dependency"},
			agents:   []string{"security_guardian", "runtime_broker"}, authority: "deny unsafe access; cannot reveal secrets",
			maxAutonomy: 5, riskCeiling: "high",
			evidence:   []string{"verified identity", "policy decision", "artifact provenance", "runtime boundary"},
			evaluation: []string{"unauthorized paths fail closed", "secrets never enter prompts, logs, or client payloads"},
		},
		{
			id: "agent-threat-modeling", name: "Agentic threat modelling", family: "governance",
			purpose:  "Model prompt injection, confused deputy, tool abuse, data exfiltration, memory poisoning, spoofed agents, and runaway loops.",
			problems: []string{"agent security review", "prompt injection", "tool threat", "untrusted content"},
			triggers: []string{"prompt injection", "untrusted", "agent threat", "exfiltration", "memory poisoning", "spoof"},
			agents:   []string{"security_guardian", "red_team_reviewer"}, authority: "block and quarantine suspicious instructions",
			maxAutonomy: 3, riskCeiling: "high",
			evidence:   []string{"trust-boundary map", "threat scenario", "mitigation and residual risk"},
			evaluation: []string{"known attack paths are tested", "untrusted content cannot alter policy or authority"},
		},
		{
			id: "safety-engineering", name: "Safety engineering", family: "governance",
			purpose:  "Apply hazard analysis, defense in depth, fail-safe defaults, reversibility, emergency stop, and post-incident learning.",
			problems: []string{"hazard analysis", "high-impact workflow", "safety control", "incident"},
			triggers: []string{"safety", "hazard", "danger", "emergency stop", "irreversible", "incident"},
			agents:   []string{"safety_reviewer", "policy_guardian"}, authority: "stop unsafe execution",
			maxAutonomy: 5, riskCeiling: "high",
			evidence:   []string{"hazard register", "control mapping", "recovery and rollback plan"},
			evaluation: []string{"unsafe states fail closed", "control effectiveness is exercised rather than merely documented"},
		},
		{
			id: "ai-governance", name: "AI governance", family: "governance",
			purpose:  "Manage accountability, risk tiering, transparency, human oversight, records, impact review, and lifecycle ownership.",
			problems: []string{"AI governance", "policy compliance", "impact assessment", "accountability"},
			triggers: []string{"governance", "policy", "compliance", "audit", "accountable", "impact assessment"},
			agents:   []string{"governance_reviewer", "policy_guardian"}, authority: "review and block non-compliant use",
			maxAutonomy: 4, riskCeiling: "high",
			evidence:   []string{"system purpose", "risk classification", "responsible owner", "decision record"},
			evaluation: []string{"controls map to identified risk", "records support independent review"},
		},
		{
			id: "model-intelligence", name: "Model intelligence", family: "reasoning",
			purpose:  "Route to the cheapest model proven capable for the task while respecting local-first policy, quotas, budgets, and validation.",
			problems: []string{"model routing", "capability matching", "cost control", "fallback"},
			triggers: []string{"model", "llm", "route", "token", "cost", "ollama", "provider"},
			agents:   []string{"model_router", "validation_critic"}, authority: "select within enabled providers and budget policy",
			maxAutonomy: 5, riskCeiling: "medium",
			evidence:   []string{"provider health", "capability profile", "price and quota data", "validation result"},
			evaluation: []string{"verified completion rate by route", "paid calls remain disabled unless approved"},
		},
		{
			id: "evaluation", version: "1.1.0", name: "Evaluation", family: "evaluation",
			purpose:  "Measure task success, groundedness, policy compliance, tool correctness, cost, latency, and user outcome with suitable evaluators.",
			problems: []string{"quality evaluation", "benchmark", "acceptance test", "regression"},
			triggers: []string{"evaluate", "test", "benchmark", "quality", "acceptance", "score"},
			agents:   []string{"evaluation_agent", "critic"}, authority: "test and report",
			maxAutonomy: 4, riskCeiling: "high",
			evidence:   []string{"evaluation dataset or criteria", "reproducible result", "known limitations"},
			evaluation: []string{"evaluators correlate with real success", "synthetic tests are not represented as real-world proof"},
		},
		{
			id: "observability", name: "Observability", family: "evaluation",
			purpose:  "Trace task, model, tool, policy, approval, workflow, connector, and verification activity with privacy-aware logs and metrics.",
			problems: []string{"monitoring", "trace", "metrics", "failure analysis"},
			triggers: []string{"log", "trace", "metric", "monitor", "observability", "diagnostic"},
			agents:   []string{"observability_agent", "privacy_guardian"}, authority: "observe and alert",
			maxAutonomy: 4, riskCeiling: "medium",
			evidence:   []string{"correlation IDs", "source timestamps", "redaction state"},
			evaluation: []string{"important decisions are traceable end to end", "telemetry does not expose secrets"},
		},
		{
			id: "reliability-resilience", name: "Reliability and resilience", family: "execution",
			purpose:  "Design for degraded dependencies, restarts, duplicate delivery, stale state, partial failure, recovery, backup, and continuity.",
			problems: []string{"resilience", "availability", "recovery", "dependency failure"},
			triggers: []string{"reliability", "resilience", "restart", "outage", "backup", "degraded", "recovery"},
			agents:   []string{"reliability_monitor", "recovery_coordinator"}, authority: "recover reversible local state within runbooks",
			maxAutonomy: 8, riskCeiling: "medium",
			evidence:   []string{"health probes", "recovery receipt", "post-recovery verification"},
			evaluation: []string{"recovery objectives are measured", "degraded operation remains truthful"},
		},
		{
			id: "controlled-learning", name: "Learning and controlled self-improvement", family: "learning",
			purpose:  "Learn from verified outcomes and explicit corrections while preventing unsafe policy drift, reward hacking, or silent self-modification.",
			problems: []string{"preference learning", "workflow improvement", "lesson extraction", "policy update"},
			triggers: []string{"learn", "correction", "improve", "lesson", "feedback", "self improvement"},
			agents:   []string{"learning_steward", "policy_guardian"}, authority: "propose changes; protected rules require explicit approval",
			maxAutonomy: 3, riskCeiling: "high",
			evidence:   []string{"verified outcome", "before-and-after behavior", "owner correction or approval"},
			evaluation: []string{"change improves held-out outcomes", "rollback is available", "protected policies do not drift"},
		},
		{
			id: "productivity-attention", name: "Productivity and attention pack", family: "domain_pack",
			purpose:  "Manage capture, clarification, next actions, time blocking, focus, reviews, energy matching, and workload limits.",
			problems: []string{"personal productivity", "focus", "task management", "weekly review"},
			triggers: []string{"todo", "focus", "procrastinate", "weekly review", "calendar block", "inbox"},
			agents:   []string{"productivity_coach", "scheduler"}, authority: "reorganize internal plans; external commitments require approval",
			maxAutonomy: 5, riskCeiling: "medium",
			evidence:   []string{"current commitments", "calendar availability", "declared priority"},
			evaluation: []string{"important commitments progress", "planning overhead and interruptions stay bounded"},
		},
		{
			id: "habit-behavior-change", name: "Behaviour change and habits pack", family: "domain_pack",
			purpose:  "Use cue-routine-reward, implementation intentions, tiny habits, environment design, and relapse-aware tracking.",
			problems: []string{"habit formation", "behavior change", "routine", "motivation"},
			triggers: []string{"habit", "routine", "behavior", "motivation", "implementation intention", "streak"},
			agents:   []string{"habit_coach"}, authority: "suggest and remind; never coerce",
			maxAutonomy: 2, riskCeiling: "medium",
			evidence:   []string{"operator-chosen behavior", "observed or self-reported outcome"},
			evaluation: []string{"behavior improves without harmful pressure", "setbacks adjust the plan rather than fabricate success"},
		},
		{
			id: "health-personal-care", name: "Health and personal care pack", family: "domain_pack",
			purpose:  "Support appointments, medication reminders, symptoms, routines, records, and escalation while avoiding diagnosis or unsafe treatment decisions.",
			problems: []string{"health administration", "appointment", "medication reminder", "personal care"},
			triggers: []string{"health", "doctor", "medical", "medicine", "symptom", "appointment", "care"},
			agents:   []string{"health_admin_assistant", "safety_reviewer"}, authority: "organize and draft; medical decisions require qualified human review",
			maxAutonomy: 3, riskCeiling: "high",
			evidence:   []string{"official care instruction or operator record", "fresh appointment information"},
			evaluation: []string{"no diagnosis is presented as fact", "urgent warning signs are escalated"},
		},
		{
			id: "financial-management", name: "Financial management pack", family: "domain_pack",
			purpose:  "Track budgets, bills, invoices, cash flow, debt, goals, tax records, and anomalies under strict approval and calculation rules.",
			problems: []string{"budget", "invoice", "cash flow", "financial planning"},
			triggers: []string{"money", "budget", "invoice", "payment", "bank", "tax", "debt", "price"},
			agents:   []string{"finance_admin_agent", "calculation_verifier"}, authority: "analyze and draft; no payment or commitment without approval",
			maxAutonomy: 3, riskCeiling: "high",
			evidence:   []string{"source transaction or invoice", "deterministic calculation", "currency and period"},
			evaluation: []string{"figures reconcile", "payments and commitments remain approval-gated"},
		},
		{
			id: "home-garden-assets", name: "Home, garden and asset management pack", family: "domain_pack",
			purpose:  "Manage maintenance, repairs, inventory, warranties, service providers, seasonal work, documents, and household risks.",
			problems: []string{"home maintenance", "garden work", "asset record", "repair"},
			triggers: []string{"home", "garden", "repair", "maintenance", "warranty", "asset", "contractor"},
			agents:   []string{"home_operations_agent"}, authority: "plan and schedule drafts; purchases and external commitments require approval",
			maxAutonomy: 5, riskCeiling: "medium",
			evidence:   []string{"asset or property record", "quote or maintenance source", "before-and-after evidence"},
			evaluation: []string{"maintenance closes with evidence", "safety-critical repairs escalate"},
		},
		{
			id: "work-service-delivery", name: "Work and service delivery pack", family: "domain_pack",
			purpose:  "Move client requests through scope, quote, schedule, execution, quality check, handoff, invoice, and follow-up.",
			problems: []string{"client work", "service job", "deliverable", "quality gate"},
			triggers: []string{"client", "job", "quote", "deliverable", "service", "invoice", "handoff"},
			agents:   []string{"service_delivery_manager", "quality_reviewer"}, authority: "manage internal work; promises, prices, and sends require approval",
			maxAutonomy: 5, riskCeiling: "high",
			evidence:   []string{"agreed scope", "deliverable evidence", "acceptance criteria"},
			evaluation: []string{"scope and acceptance criteria are met", "unresolved work is not marked complete"},
		},
		{
			id: "entrepreneurship-venture", name: "Entrepreneurship and venture pack", family: "domain_pack",
			purpose:  "Support problem discovery, hypothesis testing, lean experiments, business models, market evidence, risks, and stage gates.",
			problems: []string{"venture", "business model", "market experiment", "product strategy"},
			triggers: []string{"business", "venture", "startup", "market", "customer discovery", "mvp", "revenue"},
			agents:   []string{"venture_analyst", "experiment_designer"}, authority: "research and propose; spend and public commitments require approval",
			maxAutonomy: 4, riskCeiling: "high",
			evidence:   []string{"customer or market evidence", "experiment design", "decision threshold"},
			evaluation: []string{"learning goal is met", "claims distinguish evidence from hypothesis"},
		},
		{
			id: "legal-government-case", name: "Legal, government and case management pack", family: "domain_pack",
			purpose:  "Build source-linked case files, issue lists, timelines, deadlines, evidence bundles, drafts, contradiction checks, and approval gates.",
			problems: []string{"legal case", "government correspondence", "insurance dispute", "evidence bundle"},
			triggers: []string{"legal", "lawyer", "court", "government", "municipality", "insurance", "hearing", "case", "dispute"},
			agents:   []string{"case_manager", "evidence_agent", "legal_risk_reviewer"}, authority: "organize and draft only; external legal action requires explicit approval",
			maxAutonomy: 3, riskCeiling: "high",
			evidence:   []string{"primary case sources", "dated timeline", "claim-to-evidence map", "deadline provenance"},
			evaluation: []string{"every material factual claim is source-linked", "no external filing or send occurs without approval"},
		},
		{
			id: "communication", name: "Communication pack", family: "domain_pack",
			purpose:  "Choose recipient-aware tone, structure, factual grounding, channel, follow-up, and approval according to communication risk.",
			problems: []string{"email draft", "message", "public post", "stakeholder update"},
			triggers: []string{"email", "reply", "message", "write to", "post", "publish", "communication"},
			agents:   []string{"communication_drafter", "tone_reviewer", "evidence_agent"}, authority: "draft by default; sending or publishing follows policy",
			maxAutonomy: 3, riskCeiling: "high",
			evidence:   []string{"recipient and purpose", "support for factual claims", "approval for consequential send"},
			evaluation: []string{"message meets purpose and tone", "unsupported or defamatory claims are blocked"},
		},
		{
			id: "relationships-care", name: "Relationship and care pack", family: "domain_pack",
			purpose:  "Support respectful communication, commitments, boundaries, care tasks, conflict preparation, and important follow-ups without manipulation.",
			problems: []string{"relationship", "care commitment", "conflict preparation", "social follow-up"},
			triggers: []string{"relationship", "family", "friend", "care", "conflict", "boundary", "follow up"},
			agents:   []string{"relationship_support_agent"}, authority: "suggest and draft; never impersonate or manipulate",
			maxAutonomy: 2, riskCeiling: "high",
			evidence:   []string{"operator-stated relationship context", "explicit uncertainty for inferred intent"},
			evaluation: []string{"advice preserves agency and dignity", "private communications are minimized and protected"},
		},
		{
			id: "learning-competence", name: "Learning and competence pack", family: "domain_pack",
			purpose:  "Set learning objectives, assess gaps, schedule deliberate practice, retrieve knowledge, test recall, and track demonstrated competence.",
			problems: []string{"learning plan", "skill development", "study", "competence assessment"},
			triggers: []string{"learn", "study", "skill", "practice", "course", "competence", "exam"},
			agents:   []string{"learning_coach", "assessment_agent"}, authority: "plan, tutor, and test",
			maxAutonomy: 4, riskCeiling: "low",
			evidence:   []string{"learning objective", "practice results", "assessment criteria"},
			evaluation: []string{"competence is demonstrated, not inferred from content consumption", "practice adapts to measured gaps"},
		},
		{
			id: "travel-mobility", name: "Travel and mobility pack", family: "domain_pack",
			purpose:  "Coordinate itinerary, availability, travel time, accessibility, costs, documents, disruptions, and contingency options.",
			problems: []string{"travel planning", "route", "appointment logistics", "mobility"},
			triggers: []string{"travel", "trip", "route", "train", "flight", "drive", "appointment location"},
			agents:   []string{"travel_planner", "calendar_agent"}, authority: "research and prepare; booking and spend require approval",
			maxAutonomy: 5, riskCeiling: "high",
			evidence:   []string{"current schedule", "current route or timetable source", "cost and cancellation terms"},
			evaluation: []string{"itinerary is feasible", "time-sensitive information is refreshed before commitment"},
		},
		{
			id: "emergency-continuity", name: "Emergency and continuity pack", family: "domain_pack",
			purpose:  "Prepare emergency contacts, critical records, backups, account recovery, communication fallback, and incapacity authority.",
			problems: []string{"emergency planning", "continuity", "backup", "account recovery"},
			triggers: []string{"emergency", "continuity", "backup", "incapacity", "recovery kit", "critical contact"},
			agents:   []string{"continuity_coordinator", "security_guardian"}, authority: "prepare and alert; emergency authority must be explicitly pre-authorized",
			maxAutonomy: 5, riskCeiling: "high",
			evidence:   []string{"current recovery plan", "verified contact route", "backup status"},
			evaluation: []string{"runbook is exercised", "sensitive recovery material remains protected"},
		},
		{
			id: "agent-development-adapters", name: "Agent-development framework adapters", family: "implementation",
			purpose:  "Allow agents built with different frameworks to participate through governed adapters without replacing HAI's Go control plane.",
			problems: []string{"agent framework integration", "runtime adapter", "external agent team"},
			triggers: []string{"langgraph", "autogen", "semantic kernel", "crewai", "agents sdk", "agent framework"},
			agents:   []string{"runtime_architect", "security_reviewer"}, authority: "evaluate and sandbox adapters; no automatic installation",
			maxAutonomy: 3, riskCeiling: "high",
			evidence:        []string{"upstream license and maintenance review", "capability test", "sandbox and policy contract"},
			evaluation:      []string{"adapter cannot bypass HAI governance", "real integration test proves claimed capability"},
			implementations: []string{"LangGraph", "Microsoft AutoGen", "Microsoft Agent Framework", "Semantic Kernel", "OpenAI Agents SDK", "Google Agent Development Kit", "CrewAI", "PydanticAI", "LlamaIndex Agents", "Haystack Agents", "Hugging Face smolagents", "Agno", "BeeAI", "Mastra", "LangChain", "DSPy", "Letta", "CAMEL", "MetaGPT", "AutoGPT", "SuperAGI", "Flowise", "Langflow"},
			status:          StatusExperimental,
		},
		{
			id: "durable-workflow-platforms", name: "Durable workflow platform adapters", family: "implementation",
			purpose:  "Evaluate external durable execution platforms when single-node workers no longer meet measured reliability needs.",
			problems: []string{"distributed workflow", "durable execution platform", "multi-worker scale"},
			triggers: []string{"temporal", "restate", "camunda", "conductor", "airflow", "distributed worker", "high availability"},
			agents:   []string{"platform_architect", "reliability_reviewer"}, authority: "evaluate and prototype; production adoption requires architectural approval",
			maxAutonomy: 2, riskCeiling: "high",
			evidence:        []string{"measured current limitation", "migration and rollback plan", "operational cost"},
			evaluation:      []string{"platform solves a proven gap", "failure and recovery are integration-tested"},
			implementations: []string{"Temporal", "Restate", "Azure Durable Functions", "Camunda", "Zeebe", "Netflix Conductor", "Argo Workflows", "Prefect", "Dagster", "Airflow", "n8n", "Windmill", "Kestra", "Node-RED", "NiFi", "Dapr Workflows"},
			status:          StatusExperimental,
		},
		{
			id: "memory-knowledge-implementations", name: "Memory and knowledge implementations", family: "implementation",
			purpose:  "Keep PostgreSQL canonical while evaluating replaceable vector, search, graph, or memory components only for proven needs.",
			problems: []string{"vector index", "knowledge graph", "search backend", "memory implementation"},
			triggers: []string{"pgvector", "qdrant", "weaviate", "neo4j", "mem0", "zep", "graph database"},
			agents:   []string{"data_architect", "privacy_reviewer"}, authority: "benchmark and propose; migration requires approval",
			maxAutonomy: 2, riskCeiling: "high",
			evidence:        []string{"data classification", "benchmark against PostgreSQL baseline", "export and deletion behavior"},
			evaluation:      []string{"specialized store improves a measured bottleneck", "canonical provenance remains recoverable"},
			implementations: []string{"PostgreSQL", "pgvector", "OpenSearch", "Elasticsearch", "Qdrant", "Weaviate", "Milvus", "Chroma", "Neo4j", "ArangoDB", "Memgraph", "Graphiti", "Mem0", "Zep", "Letta memory", "LangMem", "Apache AGE", "RDF/SPARQL"},
			status:          StatusExperimental,
		},
		{
			id: "policy-security-implementations", name: "Policy, identity and security implementations", family: "implementation",
			purpose:  "Evaluate policy engines, identity systems, secret stores, provenance, and sandboxing behind HAI's existing security contracts.",
			problems: []string{"policy engine", "identity provider", "secret store", "sandbox"},
			triggers: []string{"opa", "cedar", "casbin", "spicedb", "openfga", "keycloak", "vault", "sigstore", "sandbox"},
			agents:   []string{"security_architect", "policy_guardian"}, authority: "assess and test in isolation",
			maxAutonomy: 2, riskCeiling: "high",
			evidence:        []string{"threat model", "license and maintenance status", "migration and recovery plan"},
			evaluation:      []string{"integration fails closed", "new component reduces rather than expands attack surface"},
			implementations: []string{"Open Policy Agent", "Cedar", "Casbin", "SpiceDB", "OpenFGA", "Keycloak", "Authentik", "HashiCorp Vault", "SOPS", "Sigstore", "in-toto", "TUF", "SLSA", "CycloneDX", "SPDX", "AIBOM", "gVisor", "Firecracker", "WebAssembly sandboxes", "container isolation", "seccomp", "AppArmor"},
			status:          StatusExperimental,
		},
		{
			id: "evaluation-observability-implementations", name: "Evaluation and observability implementations", family: "implementation",
			purpose:  "Evaluate tracing, evaluation, security testing, metrics, logs, and error tracking without confusing tool installation with proven coverage.",
			problems: []string{"evaluation platform", "telemetry platform", "AI observability", "security testing"},
			triggers: []string{"opentelemetry", "langfuse", "phoenix", "promptfoo", "deepeval", "ragas", "grafana", "prometheus"},
			agents:   []string{"evaluation_architect", "observability_agent"}, authority: "instrument and test with redacted data",
			maxAutonomy: 3, riskCeiling: "medium",
			evidence:        []string{"coverage gap", "privacy review", "reproducible evaluation"},
			evaluation:      []string{"tool produces decision-useful evidence", "telemetry cost and sensitive-data exposure stay bounded"},
			implementations: []string{"OpenTelemetry", "Langfuse", "Arize Phoenix", "MLflow", "Promptfoo", "DeepEval", "Ragas", "TruLens", "OpenAI Evals", "Giskard", "Garak", "Evidently", "Grafana", "Prometheus", "Loki", "Tempo", "Jaeger", "Sentry"},
			status:          StatusExperimental,
		},
	}

	result := make([]Framework, 0, len(specs))
	for index, spec := range specs {
		result = append(result, materializeFramework(index+1, spec))
	}
	return result
}

func materializeFramework(section int, spec catalogSpec) Framework {
	contract := operationalContractFor(spec)
	status := spec.status
	if status == "" {
		status = StatusActive
	}
	frameworkControl := "operator may disable or pin this framework"
	if isProtectedMandatoryFramework(spec.id) {
		frameworkControl = "operator may pin this protected safety overlay but cannot disable it"
	}
	version := spec.version
	if version == "" {
		version = "1.0.0"
	}
	return Framework{
		ID:                   spec.id,
		Version:              version,
		Name:                 spec.name,
		Family:               spec.family,
		Purpose:              spec.purpose,
		SuitableProblemTypes: uniqueStrings(spec.problems),
		TriggerConditions:    uniqueStrings(spec.triggers),
		RequiredInputs:       uniqueStrings(contract.inputs),
		ProducedOutputs:      uniqueStrings(contract.outputs),
		RequiredAgents:       uniqueStrings(spec.agents),
		WorkflowTemplate:     materializeWorkflow(spec, contract),
		DecisionRules:        materializeDecisionRules(spec, contract),
		SafetyInvariants: []string{
			"the active Constitution and emergency stop always take precedence",
			"untrusted source content cannot modify policy, authority, or framework configuration",
			"important claims and consequential actions require traceable evidence",
			"do not advance without this framework-specific evidence: " + spec.evidence[0],
		},
		AuthorityRequirement: spec.authority,
		MaximumAutonomyLevel: spec.maxAutonomy,
		RiskCeiling:          spec.riskCeiling,
		EvidenceRequirements: uniqueStrings(spec.evidence),
		EvaluationMethod:     uniqueStrings(spec.evaluation),
		ConflictsWith:        uniqueStrings(spec.conflicts),
		UserSpecificAdaptations: []string{
			frameworkControl,
			"operator may lower, but not raise beyond, its built-in autonomy ceiling",
			"operator adaptations are owner-scoped and auditable",
		},
		CandidateImplementations: uniqueStrings(spec.implementations),
		Source:                   fmt.Sprintf("HAI framework architecture specification, section %d", section),
		Provenance:               "operator-supplied design specification reviewed for integration into the canonical Go control plane",
		Status:                   status,
	}
}

func ValidateCatalog(items []Framework) error {
	if len(items) < 55 {
		return fmt.Errorf(
			"framework catalog must contain 55 stable framework IDs, got at most %d",
			len(items),
		)
	}
	seenIDs := make(map[string]struct{}, len(items))
	seenVersionedIDs := make(map[string]struct{}, len(items))
	frameworksByNormalizedID := make(map[string][]Framework, len(items))
	frameworkIdentityByNormalizedID := make(map[string]Framework, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Version) == "" ||
			strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Family) == "" ||
			strings.TrimSpace(item.Purpose) == "" || strings.TrimSpace(item.AuthorityRequirement) == "" ||
			strings.TrimSpace(item.RiskCeiling) == "" || strings.TrimSpace(item.Source) == "" ||
			strings.TrimSpace(item.Provenance) == "" {
			return fmt.Errorf("framework %q is missing required scalar metadata", item.ID)
		}
		if !catalogSemanticVersionPattern.MatchString(item.Version) {
			return fmt.Errorf("framework %q has invalid semantic version %q", item.ID, item.Version)
		}
		if item.RiskCeiling != "low" && item.RiskCeiling != "medium" && item.RiskCeiling != "high" {
			return fmt.Errorf("framework %q has unsupported risk ceiling %q", item.ID, item.RiskCeiling)
		}
		normalizedID := strings.ToLower(strings.TrimSpace(item.ID))
		versionedID := normalizedID + "@" + item.Version
		if _, ok := seenVersionedIDs[versionedID]; ok {
			return fmt.Errorf("duplicate versioned framework record %q", versionedID)
		}
		seenVersionedIDs[versionedID] = struct{}{}
		if identity, ok := frameworkIdentityByNormalizedID[normalizedID]; ok {
			if !sameFrameworkIdentity(identity, item) {
				return fmt.Errorf(
					"framework %q changes stable name or family across versions",
					item.ID,
				)
			}
		} else {
			frameworkIdentityByNormalizedID[normalizedID] = item
		}
		seenIDs[normalizedID] = struct{}{}
		frameworksByNormalizedID[normalizedID] = append(
			frameworksByNormalizedID[normalizedID],
			item,
		)
		if item.MaximumAutonomyLevel < 0 || item.MaximumAutonomyLevel > 10 {
			return fmt.Errorf("framework %q has invalid autonomy level %d", item.ID, item.MaximumAutonomyLevel)
		}
		if item.Status != StatusActive && item.Status != StatusExperimental && item.Status != StatusDeprecated {
			return fmt.Errorf("framework %q has invalid status %q", item.ID, item.Status)
		}
		requiredSlices := map[string][]string{
			"suitable problem types": item.SuitableProblemTypes,
			"trigger conditions":     item.TriggerConditions,
			"required inputs":        item.RequiredInputs,
			"produced outputs":       item.ProducedOutputs,
			"required agents":        item.RequiredAgents,
			"workflow template":      item.WorkflowTemplate,
			"decision rules":         item.DecisionRules,
			"safety invariants":      item.SafetyInvariants,
			"evidence requirements":  item.EvidenceRequirements,
			"evaluation method":      item.EvaluationMethod,
			"user adaptations":       item.UserSpecificAdaptations,
		}
		for field, values := range requiredSlices {
			if err := validateCatalogStrings(values, true); err != nil {
				return fmt.Errorf("framework %q has invalid %s: %w", item.ID, field, err)
			}
		}
		if err := validateCatalogStrings(item.ConflictsWith, false); err != nil {
			return fmt.Errorf("framework %q has invalid conflicts: %w", item.ID, err)
		}
		if err := validateCatalogSemanticQuality(item); err != nil {
			return err
		}
	}
	if len(seenIDs) != 55 {
		return fmt.Errorf(
			"framework catalog must contain 55 stable framework IDs, got %d",
			len(seenIDs),
		)
	}
	activeByNormalizedID := make(map[string]Framework, len(frameworksByNormalizedID))
	for id, versions := range frameworksByNormalizedID {
		sort.SliceStable(versions, func(i, j int) bool {
			return compareSemanticVersions(versions[i].Version, versions[j].Version) > 0
		})
		activeByNormalizedID[id] = versions[0]
	}
	for _, item := range activeByNormalizedID {
		for _, conflict := range item.ConflictsWith {
			normalizedConflict := strings.ToLower(strings.TrimSpace(conflict))
			if normalizedConflict == strings.ToLower(strings.TrimSpace(item.ID)) {
				return fmt.Errorf("framework %q cannot conflict with itself", item.ID)
			}
			if _, ok := seenIDs[normalizedConflict]; !ok {
				return fmt.Errorf("framework %q references unknown conflict %q", item.ID, conflict)
			}
			other := activeByNormalizedID[normalizedConflict]
			if !containsCatalogString(other.ConflictsWith, item.ID) {
				return fmt.Errorf(
					"framework conflict %q -> %q is not symmetric",
					item.ID,
					other.ID,
				)
			}
		}
	}
	return nil
}

func validateCatalogSemanticQuality(item Framework) error {
	fields := map[string][]string{
		"required inputs":       item.RequiredInputs,
		"produced outputs":      item.ProducedOutputs,
		"workflow template":     item.WorkflowTemplate,
		"decision rules":        item.DecisionRules,
		"evidence requirements": item.EvidenceRequirements,
		"evaluation method":     item.EvaluationMethod,
	}
	for field, values := range fields {
		for _, value := range values {
			normalized := strings.ToLower(strings.TrimSpace(value))
			if _, forbidden := forbiddenGenericCatalogEntries[normalized]; forbidden {
				return fmt.Errorf(
					"framework %q has generic boilerplate in %s: %q",
					item.ID,
					field,
					value,
				)
			}
			if catalogPlaceholderPattern.MatchString(value) {
				return fmt.Errorf(
					"framework %q has placeholder text in %s: %q",
					item.ID,
					field,
					value,
				)
			}
		}
	}

	const contraindicationPrefix = "do not use this framework to "
	for _, rule := range item.DecisionRules {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(rule)), contraindicationPrefix) {
			return nil
		}
	}
	return fmt.Errorf("framework %q has no explicit contraindication decision rule", item.ID)
}

func containsCatalogString(values []string, candidate string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(candidate)) {
			return true
		}
	}
	return false
}

func validateCatalogStrings(values []string, required bool) error {
	if required && len(values) == 0 {
		return fmt.Errorf("must not be empty")
	}
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return fmt.Errorf("element %d is blank", index)
		}
		normalized := strings.ToLower(trimmed)
		if _, ok := seen[normalized]; ok {
			return fmt.Errorf("contains duplicate element %q", value)
		}
		seen[normalized] = struct{}{}
	}
	return nil
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func sortedUnique(values []string) []string {
	result := uniqueStrings(values)
	sort.SliceStable(result, func(i, j int) bool {
		return strings.ToLower(result[i]) < strings.ToLower(result[j])
	})
	return result
}
