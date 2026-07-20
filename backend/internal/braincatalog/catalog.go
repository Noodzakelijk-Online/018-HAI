// Package braincatalog owns HAI's curated view of external agent projects.
//
// It is deliberately a catalog, not a package manager: HAI must not download,
// install, or execute third-party agent code merely because it appears in an
// awesome-list. Entries become usable only through a reviewed adapter and the
// existing approval-gated runtime path.
package braincatalog

import "strings"

// Status describes HAI's adoption decision for an external project.
type Status string

const (
	StatusCandidate     Status = "candidate"
	StatusIntegrated    Status = "integrated_profile"
	StatusCompatibility Status = "compatibility_only"
	StatusReferenceOnly Status = "reference_only"
	StatusExcluded      Status = "excluded"
	StatusLicenseReview Status = "license_review"
)

// ControlMapping records how an upstream pattern is translated into an
// HAI-owned control. A recommendation therefore never delegates safety,
// policy, or execution authority to a third-party project.
type ControlMapping struct {
	SourcePattern string `json:"sourcePattern"`
	HAIControl    string `json:"haiControl"`
	Boundary      string `json:"boundary"`
}

// Entry is a transparent, source-backed integration decision. Verification is
// a curation snapshot rather than a claim that a runtime has been installed.
type Entry struct {
	ID                   string           `json:"id"`
	Name                 string           `json:"name"`
	UpstreamURL          string           `json:"upstreamUrl"`
	SourceCatalogURL     string           `json:"sourceCatalogUrl"`
	SourceCollection     string           `json:"sourceCollection,omitempty"`
	Status               Status           `json:"status"`
	Category             string           `json:"category"`
	IntegrationMode      string           `json:"integrationMode"`
	Capabilities         []string         `json:"capabilities"`
	RecommendedFor       []string         `json:"recommendedFor"`
	RequiresApproval     bool             `json:"requiresApproval"`
	LocalFirstCompatible bool             `json:"localFirstCompatible"`
	Activation           string           `json:"activation"`
	Rationale            string           `json:"rationale"`
	VerifiedAt           string           `json:"verifiedAt"`
	VerificationNote     string           `json:"verificationNote"`
	ControlMappings      []ControlMapping `json:"controlMappings,omitempty"`
}

// Recommendation makes a candidate visible to the planner without claiming it
// is installed or selected for execution.
type Recommendation struct {
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	Status           Status           `json:"status"`
	Role             string           `json:"role"`
	Rationale        string           `json:"rationale"`
	RequiresApproval bool             `json:"requiresApproval"`
	Activation       string           `json:"activation"`
	ControlMappings  []ControlMapping `json:"controlMappings,omitempty"`
}

// UpstreamReview is a point-in-time public metadata check for a catalog entry.
// It deliberately does not change HAI's adoption status: an upstream being
// available is neither an approval nor proof that its adapter is safe.
type UpstreamReview struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	UpstreamURL     string   `json:"upstreamUrl"`
	CheckedAt       string   `json:"checkedAt"`
	Available       bool     `json:"available"`
	Archived        bool     `json:"archived"`
	License         string   `json:"license,omitempty"`
	DefaultBranch   string   `json:"defaultBranch,omitempty"`
	PushedAt        string   `json:"pushedAt,omitempty"`
	Message         string   `json:"message"`
	Disposition     Status   `json:"disposition"`
	Readiness       string   `json:"readiness"`
	ReadinessReason string   `json:"readinessReason"`
	RequiredGates   []string `json:"requiredGates,omitempty"`
}

const sourceCatalogURL = "https://github.com/e2b-dev/awesome-ai-agents"
const verifiedAt = "2026-07-19"

var discoverySources = []CatalogSource{
	{Name: "Awesome AI Agents", URL: sourceCatalogURL, Scope: "external agent projects"},
	{Name: "OSS Insight", URL: "https://ossinsight.io/collections", Scope: "curated repository collections"},
}

// CatalogSource records a discovery index. Discovery records are evidence for
// curation, not an installation, endorsement, or runtime trust decision.
type CatalogSource struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Scope string `json:"scope"`
}

// DiscoverySources returns a copy so API callers cannot modify the registry.
func DiscoverySources() []CatalogSource {
	return append([]CatalogSource(nil), discoverySources...)
}

var entries = []Entry{
	{
		ID: "anythingllm", Name: "AnythingLLM", UpstreamURL: "https://github.com/Mintplex-Labs/anything-llm", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10108/repos/", SourceCollection: "RAG Frameworks",
		Status: StatusCandidate, Category: "local RAG workspace adapter", IntegrationMode: "reviewed local workspace bridge",
		Capabilities: []string{"document workspaces", "RAG retrieval", "agent workspace patterns", "local-model connections"}, RecommendedFor: []string{"approved document workspaces", "RAG adapter evaluation", "local research preparation"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Review one local workspace deployment with source-folder and connector allowlists, local-model endpoints, document retention and deletion controls, import provenance, role boundaries, audit export, and a no-external-action default. AnythingLLM may prepare retrieved context or draft output, but it cannot become HAI's memory authority, modify source records, send content, or execute tools without separate HAI approval.",
		Rationale:  "AnythingLLM is a maintained, local-first workspace/RAG candidate that can complement HAI's source-grounded research flow when a demonstrated workspace need exists, while HAI keeps project memory, verification, provider policy, and approvals authoritative.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight RAG Frameworks repository list and GitHub metadata checked on 2026-07-19: active master branch, MIT licence; no AnythingLLM workspace, connector, model, or agent is installed or configured by HAI.",
		ControlMappings: []ControlMapping{
			{SourcePattern: "document workspace", HAIControl: "source registry, provenance links, and memory review", Boundary: "imports do not become HAI memory facts without source support or confirmation"},
			{SourcePattern: "workspace agent", HAIControl: "task planner and approval-gated runtime adapters", Boundary: "no workspace agent can self-authorize tools or external effects"},
		},
	},
	{
		ID: "github-mcp-server", Name: "GitHub MCP Server", UpstreamURL: "https://github.com/github/github-mcp-server", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10105/repos/", SourceCollection: "MCP Servers",
		Status: StatusCandidate, Category: "scoped GitHub tool integration", IntegrationMode: "reviewed local MCP bridge",
		Capabilities: []string{"repository inspection", "issue and pull-request operations", "GitHub tool schemas"}, RecommendedFor: []string{"repository context", "issue triage", "pull-request review"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Review one local MCP process, a GitHub App or fine-grained token with the minimum repository scope, fixed tool allowlist, rate limits, audit events, and a read-only-first operating mode. Write, merge, label, comment, or workflow actions remain separate HAI approvals.",
		Rationale:  "The maintained official GitHub MCP server is a useful candidate for source-grounded repository work, but HAI must keep repository scope, credentials, write policy, and final execution authority in its own connector and approval layers.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight MCP Servers repository list and GitHub metadata checked on 2026-07-19: active main branch, MIT licence; no GitHub MCP server is installed, configured, or credentialed by HAI.",
		ControlMappings: []ControlMapping{
			{SourcePattern: "GitHub MCP tools", HAIControl: "GitHub connector scopes, audit events, and approval queue", Boundary: "catalog discovery never creates credentials or grants repository access"},
			{SourcePattern: "repository write operations", HAIControl: "risk policy and per-action confirmation", Boundary: "writes, comments, merges, and workflow dispatches stay approval-gated"},
		},
	},
	{
		ID: "playwright-mcp", Name: "Playwright MCP", UpstreamURL: "https://github.com/microsoft/playwright-mcp", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10105/repos/", SourceCollection: "MCP Servers",
		Status: StatusCandidate, Category: "controlled browser MCP automation", IntegrationMode: "reviewed local browser adapter",
		Capabilities: []string{"browser tool schemas", "page inspection", "scripted browser actions"}, RecommendedFor: []string{"approved browser verification", "reproducible UI checks", "read-first web workflows"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Review a local browser profile with explicit origin, download, upload, credential, storage, and action allowlists. Begin with read-only checks and deterministic test flows; external messages, posts, account changes, uploads, purchases, and deletion require a separate HAI approval.",
		Rationale:  "Playwright MCP can expose HAI's existing browser verification discipline through a standard tool boundary, without broadening browser autonomy or bypassing the current approval policy.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight MCP Servers repository list and GitHub metadata checked on 2026-07-19: active main branch, Apache-2.0 licence; no Playwright MCP server is installed or connected by HAI.",
		ControlMappings: []ControlMapping{
			{SourcePattern: "browser MCP tools", HAIControl: "browser origin and action allowlists", Boundary: "the tool cannot inherit logged-in accounts or external-action permission"},
			{SourcePattern: "browser state", HAIControl: "source evidence and verification records", Boundary: "browser observations do not become facts without HAI verification"},
		},
	},
	{
		ID: "google-genai-toolbox", Name: "Gen AI Toolbox", UpstreamURL: "https://github.com/googleapis/genai-toolbox", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10105/repos/", SourceCollection: "MCP Servers",
		Status: StatusCandidate, Category: "database tool boundary", IntegrationMode: "reviewed local database-tool bridge",
		Capabilities: []string{"database tool definitions", "MCP exposure", "connection pooling patterns"}, RecommendedFor: []string{"approved read-only data lookup", "source-backed operational queries", "connector design"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Review a named local database connection with read-only credentials, approved query templates, row and time limits, parameter validation, redacted audit logs, and a disconnect path. It cannot receive production credentials, execute arbitrary SQL, or become a second source-of-truth service.",
		Rationale:  "Gen AI Toolbox offers a relevant MCP design for narrowly exposing approved data queries, while HAI keeps connection ownership, provenance, query policy, and write denial in its own back office.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight MCP Servers repository list and GitHub metadata checked on 2026-07-19: active main branch, Apache-2.0 licence; no Gen AI Toolbox process or database connection is configured by HAI.",
	},
	{
		ID: "qodo-pr-agent", Name: "Qodo PR-Agent", UpstreamURL: "https://github.com/qodo-ai/pr-agent", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10136/repos/", SourceCollection: "AI Code Review",
		Status: StatusCandidate, Category: "repository-review automation", IntegrationMode: "reviewed read-only pull-request analysis adapter",
		Capabilities: []string{"pull-request analysis", "change summaries", "review suggestions", "test-gap detection"}, RecommendedFor: []string{"developer quality gates", "pull-request triage", "review preparation"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Review a read-only repository scope, an operator-selected local or approved model, prompt and diff redaction, result retention, and a no-comment/no-merge default. Any remote model egress, issue comment, review submission, label change, or merge remains a separate HAI approval.",
		Rationale:  "Qodo PR-Agent is a maintained candidate for making code-review work more inspectable, but it must remain a proposal generator under HAI's source, test, and approval gates.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight AI Code Review repository list and GitHub metadata checked on 2026-07-19: active main branch, MIT licence; no Qodo PR-Agent integration is installed or authorised by HAI.",
	},
	{
		ID: "swe-agent", Name: "SWE-agent", UpstreamURL: "https://github.com/SWE-agent/SWE-agent", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10136/repos/", SourceCollection: "AI Code Review",
		Status: StatusCandidate, Category: "sandboxed code-task execution", IntegrationMode: "reviewed local workspace worker candidate",
		Capabilities: []string{"issue-to-patch planning", "test-driven code changes", "workspace task loops"}, RecommendedFor: []string{"contained bug-fix experiments", "repository task planning", "test-backed patch proposals"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Review an isolated local workspace image, fixed repository mount, no-secret environment, deny-by-default network, command and time limits, test allowlist, diff capture, rollback, and explicit human acceptance. It cannot commit, push, merge, access unrelated folders, or invoke a paid model without separate approval.",
		Rationale:  "SWE-agent is a relevant maintained candidate for a narrowly contained coding-worker path, but HAI will preserve its own planning, verification, approval, and repository-boundary controls.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight AI Code Review and Agent Harness repository lists and GitHub metadata checked on 2026-07-19: active main branch, MIT licence; no SWE-agent worker is installed or executable through HAI.",
		ControlMappings: []ControlMapping{
			{SourcePattern: "agent shell loop", HAIControl: "controlled runtime worker and workspace allowlist", Boundary: "no generic shell, secret, network, or host access"},
			{SourcePattern: "generated patch", HAIControl: "diff audit and deterministic tests", Boundary: "a patch never becomes a commit, push, or completion claim automatically"},
		},
	},
	{
		ID: "openlit", Name: "OpenLIT", UpstreamURL: "https://github.com/openlit/openlit", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10135/repos/", SourceCollection: "AI Observability",
		Status: StatusCandidate, Category: "local AI observability", IntegrationMode: "reviewed local telemetry adapter",
		Capabilities: []string{"LLM traces", "latency and token metrics", "tool observability", "OpenTelemetry export"}, RecommendedFor: []string{"model routing diagnostics", "tool failure analysis", "local performance evidence"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Review a local collector, attribute allowlist, prompt and secret redaction, retention limit, sampling policy, export disablement, and health checks. Telemetry stays read-only and cannot select models, alter paid budgets, approve tasks, or transmit data to an unapproved endpoint.",
		Rationale:  "OpenLIT is a maintained alternative observability candidate. It is useful for a future measured telemetry gap, while HAI keeps OpenLLMetry, audit records, and its local cost policy authoritative.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight AI Observability repository list and GitHub metadata checked on 2026-07-19: active main branch, Apache-2.0 licence; no OpenLIT collector is installed or configured by HAI.",
	},
	{
		ID: "langmem", Name: "LangMem", UpstreamURL: "https://github.com/langchain-ai/langmem", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10114/repos/", SourceCollection: "AI Agent Memory",
		Status: StatusReferenceOnly, Category: "memory-consolidation patterns", IntegrationMode: "architecture reference",
		Capabilities: []string{"memory extraction", "long-term memory patterns", "context management"}, RecommendedFor: []string{"memory-consolidation review", "context retrieval design", "preference revision patterns"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Do not introduce a parallel memory store. Revisit only for a measured gap in HAI's source-linked memory consolidation, with a provenance, correction, export, deletion, and rollback design that preserves HAI as the sole active memory authority.",
		Rationale:  "LangMem supplies useful memory-engineering patterns but must not replace HAI's editable, local-first, source-grounded memory plane.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight AI Agent Memory repository list and GitHub metadata checked on 2026-07-19: active main branch, MIT licence; LangMem is not installed or connected.",
	},
	{
		ID: "pyrit", Name: "PyRIT", UpstreamURL: "https://github.com/Azure/PyRIT", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10138/repos/", SourceCollection: "AI Red Teaming",
		Status: StatusExcluded, Category: "AI red-team evaluation", IntegrationMode: "excluded upstream",
		Capabilities: []string{"adversarial prompt testing", "risk evaluation patterns"}, RecommendedFor: []string{"safety-test research only"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Do not integrate. Select a maintained safety-testing alternative only after a separate fixture, provider, egress, and no-write evaluation review.",
		Rationale:  "PyRIT is visible in the OSS Insight AI Red Teaming list but is archived upstream, so it does not meet HAI's active-candidate maintenance bar.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight AI Red Teaming repository list and GitHub metadata checked on 2026-07-19: archived=true, MIT licence; excluded from activation.",
	},
	{
		ID: "phoenix", Name: "Arize Phoenix", UpstreamURL: "https://github.com/Arize-ai/phoenix", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10135/repos/", SourceCollection: "AI Observability",
		Status: StatusLicenseReview, Category: "LLM observability", IntegrationMode: "license-review reference",
		Capabilities: []string{"traces", "evaluation dashboards", "retrieval observability"}, RecommendedFor: []string{"observability comparison"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Do not integrate until the upstream licence files, local deployment terms, telemetry retention, data egress, redaction, and collector ownership are reviewed. A missing SPDX value is not treated as an open-source licence grant.",
		Rationale:  "Phoenix is maintained and relevant, but the current GitHub API metadata reports NOASSERTION. HAI holds it for explicit licence and data-handling review rather than assuming it is acceptable.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight AI Observability repository list and GitHub metadata checked on 2026-07-19: active main branch, licence=NOASSERTION; no Phoenix deployment is configured by HAI.",
	},
	{
		ID: "taskweaver", Name: "TaskWeaver", UpstreamURL: "https://github.com/microsoft/TaskWeaver", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10137/repos/", SourceCollection: "Agent Sandboxing",
		Status: StatusExcluded, Category: "code-interpreter agent", IntegrationMode: "excluded upstream",
		Capabilities: []string{"code-interpreter patterns", "plugin orchestration"}, RecommendedFor: []string{"architecture research only"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Do not integrate. Reconsider only if a maintained successor and a complete sandbox, tool, data, and rollback design are independently reviewed.",
		Rationale:  "TaskWeaver is relevant to governed code execution but is archived upstream, which disqualifies it from HAI's active-candidate set.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight Agent Sandboxing and Agent Harness repository lists and GitHub metadata checked on 2026-07-19: archived=true, MIT licence; excluded from activation.",
	},
	{
		ID: "presidio", Name: "Presidio", UpstreamURL: "https://github.com/data-privacy-stack/presidio", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10116/repos/", SourceCollection: "AI Safety & Alignment",
		Status: StatusIntegrated, Category: "sensitive-data detection and redaction", IntegrationMode: "integrated opt-in local redaction adapter",
		Capabilities: []string{"PII detection", "redaction", "masking", "anonymisation"}, RecommendedFor: []string{"secret redaction", "source-import privacy checks", "safe audit previews"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "HAI ships a disabled-by-default local Analyzer bridge for bounded, manually submitted text and explicit language/entity allowlists. Before enabling it, review false positives, local model/language coverage, source retention, and capacity. The bridge returns metadata only; it cannot anonymize, delete source records, change approval status, or conceal original evidence from an authorised owner.",
		Rationale:  "HAI now exposes Presidio through a bounded local PII-detection bridge that strengthens its deterministic privacy boundary without introducing a second data authority.",
		VerifiedAt: "2026-07-20", VerificationNote: "OSS Insight AI Safety & Alignment listing and the current data-privacy-stack/presidio upstream were checked on 2026-07-20. The project has moved from the Microsoft GitHub namespace, is MIT licensed, and explicitly warns that automated detection is not complete. HAI has a disabled local Analyzer bridge but does not install or configure a Presidio service.",
	},
	{
		ID: "guardrails-ai", Name: "Guardrails AI", UpstreamURL: "https://github.com/guardrails-ai/guardrails", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10116/repos/", SourceCollection: "AI Safety & Alignment",
		Status: StatusIntegrated, Category: "structured-output validation", IntegrationMode: "integrated opt-in internal fixed-schema validation bridge",
		Capabilities: []string{"schema validation", "output validators", "retry signals", "structured extraction checks"}, RecommendedFor: []string{"structured extraction", "planning validation", "grounded-output review"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "HAI ships a disabled internal runner that validates one bounded redacted action_proposal JSON contract through Guardrails AI's Pydantic schema path. Enable it only with the local validation profile; no Hub validator download, LLM call, retry, persistence, execution, policy change, or approval is available.",
		Rationale:  "Guardrails AI complements HAI's deterministic schemas and verification statuses with a constrained review signal rather than replacing the safety policy or human approval gate.",
		VerifiedAt: "2026-07-20", VerificationNote: "OSS Insight AI Safety & Alignment listing and current Guardrails AI upstream were reviewed on 2026-07-20. HAI implements only an opt-in internal fixed-schema bridge; no local runner is enabled by default and proposal text is neither stored nor returned.",
	},
	{
		ID: "lm-eval-harness", Name: "LM Evaluation Harness", UpstreamURL: "https://github.com/EleutherAI/lm-evaluation-harness", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10119/repos/", SourceCollection: "AI Evaluation & Testing",
		Status: StatusIntegrated, Category: "offline model evaluation", IntegrationMode: "integrated opt-in internal fixed-suite local benchmark runner",
		Capabilities: []string{"benchmark suites", "few-shot evaluation", "repeatable model comparison", "result artifacts"}, RecommendedFor: []string{"local model comparison", "routing regression", "capability baselines"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "HAI ships a disabled local runner for one preconfigured local OpenAI-compatible model and a six-case synthetic suite. Enable the model-evaluation profile only after reviewing the named local endpoint, fixture provenance, resource limits, and no-production-data rule. Results can inform an operator review but cannot select a model, spend budget, or change HAI routing automatically.",
		Rationale:  "LM Evaluation Harness now adds reproducible local model evidence where HAI needs to compare capability rather than assume the cheapest provider is sufficient.",
		VerifiedAt: "2026-07-20", VerificationNote: "OSS Insight AI Evaluation & Testing listing and current LM Evaluation Harness upstream were reviewed on 2026-07-20. HAI implements only an opt-in six-case synthetic local bridge; no runner is enabled by default and no raw generations, task rows, or result artifacts are retained or returned.",
	},
	{
		ID: "openllmetry", Name: "OpenLLMetry", UpstreamURL: "https://github.com/traceloop/openllmetry", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10135/repos/", SourceCollection: "AI Observability",
		Status: StatusCandidate, Category: "LLM trace instrumentation", IntegrationMode: "reviewed local telemetry bridge",
		Capabilities: []string{"OpenTelemetry traces", "model-call instrumentation", "latency metrics", "cost and token signals"}, RecommendedFor: []string{"routing observability", "evaluation traces", "failure analysis"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Review local collector ownership, attribute allowlist, secret and prompt redaction, retention, sampling, export disablement, and health checks. Telemetry is observational only: it cannot grant provider access, alter budgets, or approve execution.",
		Rationale:  "OpenLLMetry can make model and tool decisions inspectable through HAI's existing audit and budget controls while avoiding an unreviewed external telemetry destination.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight AI Observability repository list and GitHub metadata checked on 2026-07-19; no OpenLLMetry instrumentation or collector is configured by HAI.",
	},
	{
		ID: "graphrag", Name: "Microsoft GraphRAG", UpstreamURL: "https://github.com/microsoft/graphrag", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10134/repos/", SourceCollection: "Knowledge Graphs for AI",
		Status: StatusReferenceOnly, Category: "graph-based retrieval patterns", IntegrationMode: "architecture reference",
		Capabilities: []string{"entity graph extraction", "community summaries", "graph retrieval"}, RecommendedFor: []string{"retrieval-gap analysis", "case timeline research", "entity-linking design"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Do not introduce a second index or memory authority. Revisit only after a measured source-linked retrieval gap and an approved design for extraction provenance, graph updates, deletion, export, and rollback.",
		Rationale:  "GraphRAG offers useful retrieval and evidence-linking patterns, but HAI keeps the existing source-linked memory plane authoritative until a demonstrated gap justifies a bounded adapter.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight Knowledge Graphs for AI repository list and GitHub metadata checked on 2026-07-19; Microsoft GraphRAG is not installed or connected.",
	},
	{
		ID: "haystack", Name: "Haystack", UpstreamURL: "https://github.com/deepset-ai/haystack", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10108/repos/", SourceCollection: "RAG Frameworks",
		Status: StatusReferenceOnly, Category: "retrieval pipeline patterns", IntegrationMode: "architecture reference",
		Capabilities: []string{"document pipelines", "retrieval components", "evaluation patterns", "agent pipeline design"}, RecommendedFor: []string{"source-ingestion design", "retrieval-gap analysis", "document-processing review"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Do not create a parallel retrieval or memory system. Revisit only for a measured document-processing gap with a source, retention, evaluation, and rollback design that remains inside HAI's local provenance controls.",
		Rationale:  "Haystack provides mature retrieval-pipeline patterns, but HAI must preserve one controlled source, memory, verification, and execution plane.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight RAG Frameworks repository list and GitHub metadata checked on 2026-07-19; Haystack is not installed or connected.",
	},
	{
		ID: "fastmcp", Name: "FastMCP", UpstreamURL: "https://github.com/jlowin/fastmcp", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10105/repos/", SourceCollection: "MCP Servers",
		Status: StatusIntegrated, Category: "MCP tool-server authoring", IntegrationMode: "integrated local read-only HAI MCP bridge",
		Capabilities: []string{"authenticated MCP server", "fixed read-only HAI workflow tools", "typed tool schemas", "separate client and bridge tokens"}, RecommendedFor: []string{"reviewed local HAI operational context", "MCP capability design", "read-only agent situational awareness"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Set one owner identity and two different local-only 32+ character tokens, then run the `mcp-bridge` Compose profile. The bridge binds only to 127.0.0.1 and exposes exactly two authenticated read-only tools. It must not be added to the generic no-auth MCP preflight list; any future write tool needs a separate HAI adapter and approval review.",
		Rationale:  "The local FastMCP bridge gives a reviewed agent client bounded situational awareness while HAI keeps all workflow mutation, source access, memory, approval, execution, and audit authority. It is intentionally not a generic tool registry or process launcher.",
		VerifiedAt: "2026-07-20", VerificationNote: "Current jlowin/fastmcp main revision and Apache-2.0 license checked on 2026-07-20. The isolated profile pins fastmcp 3.4.4 and implements no upstream tool execution, source access, or write capability by default.",
	},
	{
		ID: "vllm", Name: "vLLM", UpstreamURL: "https://github.com/vllm-project/vllm", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10109/repos/", SourceCollection: "LLM Inference Engines",
		Status: StatusIntegrated, Category: "local high-throughput model inference", IntegrationMode: "integrated loopback OpenAI-compatible provider profile",
		Capabilities: []string{"local model serving", "OpenAI-compatible API", "batched inference", "model capability discovery"}, RecommendedFor: []string{"local reasoning", "larger local models", "high-volume extraction"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Review a loopback-only deployment with explicit GPU, model, quantization, context-window, retention, and resource limits. Reuse HAI's existing OpenAI-compatible provider probe and EUR 0 routing policy; HAI cannot select, send data to, or start vLLM until an operator configures and verifies the endpoint.",
		Rationale:  "HAI now implements a distinct vLLM provider profile for a measured local throughput or serving need while preserving explicit configuration, loopback-only reachability, live probing, and the existing model, budget, and approval policy.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight LLM Inference Engines repository list and GitHub metadata checked on 2026-07-19: active main branch, Apache-2.0 licence. HAI implements only the provider profile; no vLLM endpoint or model is configured by HAI.",
	},
	{
		ID: "deepeval", Name: "DeepEval", UpstreamURL: "https://github.com/confident-ai/deepeval", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10119/repos/", SourceCollection: "AI Evaluation & Testing",
		Status: StatusCandidate, Category: "LLM quality evaluation", IntegrationMode: "contained, no-write local evaluation adapter",
		Capabilities: []string{"test cases", "quality metrics", "regression checks", "evaluation reports"}, RecommendedFor: []string{"retrieval evaluation", "grounded-answer checks", "model-routing regression"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Run only a reviewed local evaluation suite with redacted fixtures, an explicit provider allowlist, bounded time and cost, and retained result artifacts. A score may create a review item but cannot mark a task verified, alter a model route, or enable a provider on its own.",
		Rationale:  "DeepEval adds a distinct evaluation-oriented path beside Promptfoo and garak. HAI can use it for evidence about quality changes while keeping verification, provider policy, and completion decisions in HAI.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight AI Evaluation & Testing repository list and GitHub metadata checked on 2026-07-19; no DeepEval runner is configured by HAI.",
	},
	{
		ID: "ollama", Name: "Ollama", UpstreamURL: "https://github.com/ollama/ollama", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10109/repos/", SourceCollection: "LLM Inference Engines",
		Status: StatusIntegrated, Category: "local model inference", IntegrationMode: "operator-configured loopback Ollama provider",
		Capabilities: []string{"local model discovery", "local generation", "model tags probe", "local-first routing"}, RecommendedFor: []string{"local reasoning", "classification", "extraction", "drafting"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Set a loopback-only OLLAMA_BASE_URL and run HAI's persisted provider probe. HAI selects only a live local model under the EUR 0 policy and still requires the task's existing approval gate before consequential generation or execution.",
		Rationale:  "HAI already has a real local Ollama provider, tag probe, readiness persistence, and local-first route selection. The catalog makes that implemented boundary visible without installing Ollama or selecting a model automatically.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight LLM Inference Engines repository list and HAI's existing local-provider implementation checked on 2026-07-19.",
	},
	{
		ID: "browser-use", Name: "browser-use", UpstreamURL: "https://github.com/browser-use/browser-use", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10113/repos/", SourceCollection: "AI Browser Agents",
		Status: StatusCandidate, Category: "agentic browser execution", IntegrationMode: "reviewed local browser adapter",
		Capabilities: []string{"browser task planning", "tool-mediated browsing", "structured browser outcomes"}, RecommendedFor: []string{"browser workflow design", "approved research", "read-only verification"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Review a local, named browser profile with origin, download, upload, credential, and action allowlists. Start with read-only verification; sending, posting, account changes, purchases, uploads, and destructive actions require separate HAI approvals.",
		Rationale:  "A relevant browser-agent candidate, but browser autonomy can cause irreversible external effects. HAI retains its current controlled browser verification path until a narrow adapter proves safe.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight AI Browser Agents repository list checked on 2026-07-19; no browser-use runtime is installed or configured by HAI.",
	},
	{
		ID: "nemo-guardrails", Name: "NVIDIA NeMo Guardrails", UpstreamURL: "https://github.com/NVIDIA-NeMo/Guardrails", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10116/repos/", SourceCollection: "AI Safety & Alignment",
		Status: StatusCandidate, Category: "LLM interaction guardrails", IntegrationMode: "operator-hosted validation adapter",
		Capabilities: []string{"input controls", "output controls", "topic and policy rails"}, RecommendedFor: []string{"draft validation", "high-risk output review", "policy testing"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Complete a local adapter review covering policy ownership, false-positive handling, data redaction, model routing, audit events, and fail-closed behavior. Guardrails may flag work but cannot approve or execute it.",
		Rationale:  "A useful defense-in-depth candidate for LLM interaction validation. It must complement, never replace, HAI's deterministic risk policy and human approvals.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight AI Safety & Alignment repository list checked on 2026-07-19; no NeMo Guardrails service is configured by HAI.",
	},
	{
		ID: "garak", Name: "garak", UpstreamURL: "https://github.com/NVIDIA/garak", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10138/repos/", SourceCollection: "AI Red Teaming",
		Status: StatusCandidate, Category: "LLM vulnerability testing", IntegrationMode: "contained, no-write evaluation runner",
		Capabilities: []string{"probe suites", "model safety testing", "evaluation reports"}, RecommendedFor: []string{"provider validation", "prompt safety review", "pre-release checks"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Run only against explicitly configured local test providers with redacted fixtures, a time limit, no production credentials, and retained audit results. A failed probe creates review work; it cannot mutate policy or runtime configuration.",
		Rationale:  "A strong red-team candidate that can give HAI evidence before enabling a model or agent profile, provided evaluation inputs and execution remain contained.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight AI Red Teaming repository list checked on 2026-07-19; no garak runner is configured by HAI.",
	},
	{
		ID: "whisper-cpp", Name: "whisper.cpp", UpstreamURL: "https://github.com/ggml-org/whisper.cpp", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10118/repos/", SourceCollection: "Multimodal AI",
		Status: StatusIntegrated, Category: "local speech transcription", IntegrationMode: "operator-configured local intake adapter",
		Capabilities: []string{"offline transcription", "audio-to-text extraction", "local model execution"}, RecommendedFor: []string{"voice-note intake", "meeting evidence", "accessibility transcription"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Enable the local-transcription Compose profile, place one reviewed GGML model in the local model folder, then create an owner-scoped local-only whisper-audio source with an explicit subfolder. HAI stores returned transcripts only through its existing source and memory verification path.",
		Rationale:  "Local speech-to-text can broaden safe intake without transmitting audio to a cloud service, but it requires explicit consent and evidence-quality controls.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight Multimodal AI repository list checked on 2026-07-19; HAI includes a disabled-by-default local whisper.cpp runner that reads only an explicit selected folder and returns transcript metadata through the source review path.",
	},
	{
		ID: "a2a", Name: "A2A Protocol", UpstreamURL: "https://github.com/a2aproject/A2A", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10139/repos/", SourceCollection: "A2A Protocol",
		Status: StatusIntegrated, Category: "agent interoperability", IntegrationMode: "integrated local controlled-planning bridge",
		Capabilities: []string{"authenticated task envelopes", "local Agent Card capability advertisement", "non-executable planning drafts"}, RecommendedFor: []string{"reviewed local peer planning", "protocol translation", "multi-agent interoperability"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Enable only with `HAI_A2A_BRIDGE_ENABLED=true`, a named owner, a separate 32+ character local peer token, and a loopback/private `HAI_A2A_BRIDGE_URL`. HAI implements only A2A 1.0-shaped JSON-RPC `SendMessage` for bounded standalone planning drafts and requires `A2A-Version: 1.0`; no peer discovery, polling, streaming, push, file input, task persistence, source refresh, approval, or execution is available.",
		Rationale:  "The local A2A subset gives a reviewed peer a useful planning interface while HAI retains all workflow, source, memory, provider, approval, execution, verification, and audit authority. It is intentionally not a remote-agent trust channel or runtime registry.",
		VerifiedAt: "2026-07-20", VerificationNote: "OSS Insight A2A Protocol listing and current Linux Foundation a2aproject/A2A upstream were reviewed on 2026-07-20. HAI implements a restricted A2A 1.0-shaped JSON-RPC `SendMessage` planning profile and Agent Card, with an explicit bearer token. It does not claim full task-lifecycle conformance or depend on an unstable broad agent runtime.",
		ControlMappings: []ControlMapping{
			{SourcePattern: "Agent Card discovery", HAIControl: "local-only fixed Agent Card with bearer authentication", Boundary: "no automatic peer discovery, tool discovery, or credential negotiation"},
			{SourcePattern: "agent task envelope", HAIControl: "side-effect-free HAI planning preview", Boundary: "the bridge cannot create tasks, refresh sources, persist attempts, request approval, execute tools, or return HAI context"},
			{SourcePattern: "agent collaboration", HAIControl: "HAI workflow, approval, verification, and audit systems", Boundary: "peer output cannot become an action or completion signal"},
		},
	},
	{
		ID: "tabby", Name: "Tabby", UpstreamURL: "https://github.com/TabbyML/tabby", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10112/repos/", SourceCollection: "AI Coding Assistants",
		Status: StatusCandidate, Category: "self-hosted coding assistance", IntegrationMode: "operator-hosted editor-assistance adapter",
		Capabilities: []string{"self-hosted completion", "code context", "local model integration"}, RecommendedFor: []string{"developer assistance", "local coding experiments"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Review a local deployment, workspace scope, model provider, telemetry, repository privacy, and read-only-first integration. HAI will not grant editor, terminal, or Git write authority by catalog entry.",
		Rationale:  "A self-hosted coding-assistance candidate that can support local development workflows while preserving HAI's review and execution boundaries.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight AI Coding Assistants repository list checked on 2026-07-19; no Tabby service is configured by HAI.",
	},
	{
		ID: "letta", Name: "Letta", UpstreamURL: "https://github.com/letta-ai/letta", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10114/repos/", SourceCollection: "AI Agent Memory",
		Status: StatusReferenceOnly, Category: "agent memory patterns", IntegrationMode: "architecture reference",
		Capabilities: []string{"agent memory", "stateful context", "memory tooling"}, RecommendedFor: []string{"memory design", "retrieval experiments"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Do not introduce a second memory store. Port only a measured, source-linked memory capability through HAI's existing local records, review, export, and deletion controls.",
		Rationale:  "Letta provides useful memory-system patterns, but HAI must keep one editable, provenance-aware memory authority.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight AI Agent Memory repository list checked on 2026-07-19; Letta is not installed or connected.",
	},
	{
		ID: "comfyui", Name: "ComfyUI", UpstreamURL: "https://github.com/comfyanonymous/ComfyUI", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10111/repos/", SourceCollection: "AI Image Generation",
		Status: StatusReferenceOnly, Category: "local image generation workflows", IntegrationMode: "optional reviewed artifact service",
		Capabilities: []string{"node-based image workflows", "local asset generation"}, RecommendedFor: []string{"approved visual artifacts", "image workflow design"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Keep disabled unless an approved artifact workflow defines model provenance, content controls, local storage, GPU limits, and a human publication gate.",
		Rationale:  "A capable local visual-artifact reference, but it does not expand HAI's core decision or execution authority by itself.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight AI Image Generation repository list checked on 2026-07-19; no ComfyUI service is configured by HAI.",
	},
	{
		ID: "daytona", Name: "Daytona", UpstreamURL: "https://github.com/daytonaio/daytona", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10137/repos/", SourceCollection: "Agent Sandboxing",
		Status: StatusReferenceOnly, Category: "agent sandboxing patterns", IntegrationMode: "sandbox architecture reference",
		Capabilities: []string{"isolated workspaces", "execution sandboxing", "workspace lifecycle"}, RecommendedFor: []string{"runtime isolation design", "sandbox review"},
		RequiresApproval: true, LocalFirstCompatible: false,
		Activation: "Do not enable by default. Any future sandbox must prove local deployment, workspace isolation, network policy, credential handling, cost controls, and audit coverage before a HAI adapter is considered.",
		Rationale:  "Useful sandbox architecture reference, but it must not weaken HAI's local-first execution boundary or introduce an unreviewed control plane.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight Agent Sandboxing repository list checked on 2026-07-19; no Daytona environment is configured by HAI.",
	},
	{
		ID: "langfuse", Name: "Langfuse", UpstreamURL: "https://github.com/langfuse/langfuse", SourceCatalogURL: "https://ossinsight.io/collections/llm-devtools", SourceCollection: "LLM DevTools",
		Status: StatusIntegrated, Category: "self-hosted LLM observability", IntegrationMode: "opt-in local aggregate-trace observability bridge",
		Capabilities: []string{"local health and readiness", "aggregate control-plane traces", "OTLP/HTTP JSON export"}, RecommendedFor: []string{"local operations visibility", "model-routing audit context", "agent trace review"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Host Langfuse locally, configure a project key pair and HAI_LANGFUSE_ENABLED=true, then use the owner-only probe before an explicit aggregate operational-snapshot export. Review local retention, trace redaction, and deletion controls separately. HAI will not export prompts, task data, source records, model payloads, tokens, files, or workflow records.",
		Rationale:  "HAI now has a bounded local Langfuse bridge for explicit aggregate operational trace evidence without replacing its audit ledger or handing Langfuse routing, approval, verification, memory, workflow, or execution authority.",
		VerifiedAt: "2026-07-20", VerificationNote: "Official repository and self-hosting documentation reviewed on 2026-07-20: active MIT core, current self-host health/readiness endpoints, project key basic authentication, and OTLP/HTTP trace ingestion. HAI implements only a local health/readiness probe plus one owner-triggered aggregate-only OTLP/JSON span. No Langfuse service, credentials, trace export, prompt, dataset, score, evaluation, callout, or cloud endpoint is configured by default.",
		ControlMappings: []ControlMapping{
			{SourcePattern: "observability trace", HAIControl: "fixed aggregate-only operational snapshot", Boundary: "no prompt, source, file, model payload, token, workflow record, or caller-selected data is exported"},
			{SourcePattern: "trace acceptance", HAIControl: "HAI audit, approval, verification, and routing controls", Boundary: "a Langfuse trace cannot authorize, verify, route, retain memory, or execute work"},
		},
	},
	{
		ID: "promptfoo", Name: "Promptfoo", UpstreamURL: "https://github.com/promptfoo/promptfoo", SourceCatalogURL: "https://ossinsight.io/collections/llm-devtools", SourceCollection: "LLM DevTools",
		Status: StatusIntegrated, Category: "LLM safety regression", IntegrationMode: "integrated opt-in internal fixed-suite local evaluation bridge",
		Capabilities: []string{"prompt regression testing", "provider comparison", "synthetic high-risk action regression"}, RecommendedFor: []string{"local model safety regression", "prompt-injection regression checks", "evaluation design"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Enable only the contained `safety-evaluation` profile after reviewing one local OpenAI-compatible endpoint and its model provenance. HAI invokes a fixed six-case synthetic suite; it accepts no caller-provided provider, model, endpoint, prompt, command, source, or data. Review aggregate evidence before any separate routing or policy decision.",
		Rationale:  "HAI implements a bounded Promptfoo bridge for repeatable local prompt-injection and high-risk-action regression evidence without turning Promptfoo into an agent, data store, policy engine, or production red-team service.",
		VerifiedAt: "2026-07-20", VerificationNote: "Official repository and documentation reviewed on 2026-07-20: MIT, active main branch, v0.121.19, local CLI/library evaluation with explicit OpenAI-compatible chat endpoints and declarative assertions. HAI pins that version in an opt-in internal runner and returns aggregate metadata only; no Promptfoo runtime, provider, real prompt, source record, telemetry export, or safety claim is configured by default.",
		ControlMappings: []ControlMapping{
			{SourcePattern: "prompt or red-team test", HAIControl: "fixed synthetic regression suite", Boundary: "callers cannot choose prompts, datasets, providers, commands, or real account context"},
			{SourcePattern: "evaluation pass or failure", HAIControl: "model review and audit evidence", Boundary: "a score cannot change routing, policy, verification, approval, memory, workflow, or execution"},
		},
	},
	{
		ID: "airbyte", Name: "Airbyte", UpstreamURL: "https://github.com/airbytehq/airbyte", SourceCatalogURL: "https://ossinsight.io/collections/data-integration", SourceCollection: "Data Integration",
		Status: StatusCandidate, Category: "connector ingestion", IntegrationMode: "operator-hosted, read-first connector bridge",
		Capabilities: []string{"incremental source sync", "connector catalogue", "schema-aware ingestion"}, RecommendedFor: []string{"connected-source ingestion", "incremental sync design"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Review one connector at a time with least-privilege credentials, source scope, retention, cursors, audit events, local-only storage, pause/revoke controls, and no destructive destination writes.",
		Rationale:  "A potential adapter for expanding HAI's source ingestion, but it must not become a parallel source-of-truth or receive broad account access by default.",
		VerifiedAt: verifiedAt, VerificationNote: "Upstream data-movement project checked on 2026-07-19; no Airbyte service or connector is configured by HAI.",
	},
	{
		ID: "odoo", Name: "Odoo", UpstreamURL: "https://github.com/odoo/odoo", SourceCatalogURL: "https://ossinsight.io/collections/business-management", SourceCollection: "Business Management",
		Status: StatusCandidate, Category: "business system bridge", IntegrationMode: "scoped, read-first business-system API adapter",
		Capabilities: []string{"business records", "projects", "contacts", "accounting-adjacent workflows"}, RecommendedFor: []string{"business context", "project operations", "account bridge design"},
		RequiresApproval: true, LocalFirstCompatible: false,
		Activation: "Start with a read-only, named Odoo instance and resource allowlist. Any write, financial, customer, or accounting action remains a separate approval-gated workflow.",
		Rationale:  "A relevant external business-system integration candidate, not a replacement for HAI's decision, approval, audit, or memory planes.",
		VerifiedAt: verifiedAt, VerificationNote: "Upstream business-management project checked on 2026-07-19; HAI has no Odoo connection or credentials configured.",
	},
	{
		ID: "continue", Name: "Continue", UpstreamURL: "https://github.com/continuedev/continue", SourceCatalogURL: sourceCatalogURL,
		Status: StatusCandidate, Category: "coding review", IntegrationMode: "operator-configured CLI or CI check",
		Capabilities: []string{"source-controlled coding checks", "PR review", "local CLI"}, RecommendedFor: []string{"coding", "repository review", "verification"},
		RequiresApproval: false, LocalFirstCompatible: true,
		Activation: "Install and configure Continue outside HAI, then add a reviewed check-only adapter. HAI will not install it or grant repository write access.",
		Rationale:  "Active Apache-2.0 project with a Windows CLI path and a narrow check/review role that complements HAI verification.",
		VerifiedAt: verifiedAt, VerificationNote: "Upstream repository and release activity checked on 2026-07-19.",
	},
	{
		ID: "cline", Name: "Cline", UpstreamURL: "https://github.com/cline/cline", SourceCatalogURL: "https://ossinsight.io/collections/llm-devtools", SourceCollection: "LLM DevTools",
		Status: StatusCandidate, Category: "interactive coding agent", IntegrationMode: "operator-configured editor extension or local bridge",
		Capabilities: []string{"interactive coding assistance", "tool-mediated workspace work", "MCP-aware development workflows"}, RecommendedFor: []string{"coding", "repository review", "developer-controlled task execution"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Keep Cline outside HAI until a reviewed, workspace-confined adapter exists. Any proposed bridge must use an explicit model provider, tool and network allowlists, a review-first change flow, and HAI approval before write-capable work.",
		Rationale:  "Active Apache-2.0 LLM-devtool project with relevant developer workflows, but its tool-mediated workspace access is high-risk and must not inherit authority from a catalog recommendation.",
		VerifiedAt: verifiedAt, VerificationNote: "GitHub repository and Apache-2.0 licence metadata checked on 2026-07-19.",
	},
	{
		ID: "opencode", Name: "OpenCode", UpstreamURL: "https://github.com/anomalyco/opencode", SourceCatalogURL: "https://ossinsight.io/collections/model-context-protocol-mcp-client", SourceCollection: "Model Context Protocol (MCP) Client",
		Status: StatusCandidate, Category: "terminal coding agent", IntegrationMode: "operator-configured local CLI or confined bridge",
		Capabilities: []string{"interactive coding assistance", "terminal-mediated workspace work", "MCP-aware development workflows"}, RecommendedFor: []string{"coding", "repository review", "developer-controlled task execution"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Keep OpenCode outside HAI until a reviewed, workspace-confined adapter exists. Any proposed bridge must use an explicit model provider, tool and network allowlists, a review-first change flow, and HAI approval before write-capable work.",
		Rationale:  "Active MIT local CLI candidate from the MCP-client collection with useful developer workflows, but terminal and workspace access must remain independently reviewed and approval-gated.",
		VerifiedAt: verifiedAt, VerificationNote: "GitHub repository metadata checked on 2026-07-19; the upstream repository reports an active anomalyco/opencode project and MIT licence.",
	},
	{
		ID: "openhands", Name: "OpenHands", UpstreamURL: "https://github.com/OpenHands/OpenHands", SourceCatalogURL: sourceCatalogURL,
		Status: StatusCandidate, Category: "sandboxed development agent", IntegrationMode: "operator-configured container or service adapter",
		Capabilities: []string{"coding agent", "sandboxed workspace", "skills", "MCP integration"}, RecommendedFor: []string{"coding", "repository work", "sandboxed task execution"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Run OpenHands in an isolated local container, define a workspace and network allowlist, then complete a real approval-gated adapter review.",
		Rationale:  "Actively released development-agent project, but it can modify workspaces and invoke tools, so HAI keeps it disabled pending a specific adapter and operator verification.",
		VerifiedAt: verifiedAt, VerificationNote: "Upstream repository and release activity checked on 2026-07-19.",
	},
	{
		ID: "crewai", Name: "CrewAI", UpstreamURL: "https://github.com/crewAIInc/crewAI", SourceCatalogURL: sourceCatalogURL,
		Status: StatusCandidate, Category: "multi-agent orchestration", IntegrationMode: "operator-hosted service adapter",
		Capabilities: []string{"role-based agents", "task orchestration", "flows"}, RecommendedFor: []string{"planning", "research", "multi-step workflows"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Host a reviewed CrewAI service with explicit tools and a local model route; bind only an allowlisted task API to HAI.",
		Rationale:  "Actively maintained MIT project that can contribute orchestration patterns, but HAI must own policy, approval, audit, and final execution decisions.",
		VerifiedAt: verifiedAt, VerificationNote: "Upstream repository and release activity checked on 2026-07-19.",
	},
	{
		ID: "aider", Name: "Aider", UpstreamURL: "https://github.com/Aider-AI/aider", SourceCatalogURL: sourceCatalogURL,
		Status: StatusCandidate, Category: "interactive coding agent", IntegrationMode: "operator-configured workspace CLI adapter",
		Capabilities: []string{"repository editing", "git-aware code changes", "model-assisted coding"}, RecommendedFor: []string{"coding", "small repository changes"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Configure a confined workspace and an explicit model provider, then implement a read-only/review-first adapter before any write-enabled workflow is considered.",
		Rationale:  "Apache-2.0 project suitable for controlled code-assistance experiments; direct repository edits remain high-risk and are not enabled by this catalog.",
		VerifiedAt: verifiedAt, VerificationNote: "Upstream repository availability and maintenance activity checked on 2026-07-19.",
	},
	{
		ID: "e2b", Name: "E2B", UpstreamURL: "https://github.com/e2b-dev/E2B", SourceCatalogURL: sourceCatalogURL,
		Status: StatusReferenceOnly, Category: "remote execution sandbox", IntegrationMode: "external sandbox SDK/service",
		Capabilities: []string{"isolated code sandbox", "agent execution environment"}, RecommendedFor: []string{"sandbox design", "isolated execution"},
		RequiresApproval: true, LocalFirstCompatible: false,
		Activation: "Do not enable by default. A separate budget, data-egress, and API-key review is required before HAI may call an external E2B sandbox.",
		Rationale:  "Its SDK is active and useful as a sandbox reference, but a hosted sandbox conflicts with HAI's local-first and EUR 0 paid-default policy until explicitly approved.",
		VerifiedAt: verifiedAt, VerificationNote: "Upstream repository and release activity checked on 2026-07-19.",
	},
	{
		ID: "autogpt", Name: "AutoGPT", UpstreamURL: "https://github.com/Significant-Gravitas/AutoGPT", SourceCatalogURL: sourceCatalogURL,
		Status: StatusLicenseReview, Category: "agent platform", IntegrationMode: "separate platform deployment",
		Capabilities: []string{"agent workflows", "continuous agents", "platform UI"}, RecommendedFor: []string{"workflow patterns"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Do not vendor or integrate platform code until its per-directory licensing, hosting, and security model are reviewed.",
		Rationale:  "The repository is active, but it contains differently licensed areas; HAI will not import code under an unreviewed licensing assumption.",
		VerifiedAt: verifiedAt, VerificationNote: "Upstream repository and licensing notice checked on 2026-07-19.",
	},
	{
		ID: "autogen", Name: "AutoGen", UpstreamURL: "https://github.com/microsoft/autogen", SourceCatalogURL: sourceCatalogURL,
		Status: StatusCompatibility, Category: "legacy multi-agent compatibility", IntegrationMode: "operator-hosted bridge or protocol translation",
		Capabilities: []string{"event-driven agent messaging", "team and delegation patterns", "MCP workbench compatibility", "structured task events"}, RecommendedFor: []string{"existing AutoGen workloads", "MCP compatibility", "migration planning"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Use only behind a dedicated operator-hosted bridge with a narrow task schema. HAI does not install or execute AutoGen project code. New capability must use a HAI-native adapter or undergo a separate successor-framework review.",
		Rationale:  "AutoGen is maintenance mode but documents useful interoperability patterns. HAI maps those patterns into its native task, workflow, approval, and audit pipeline rather than using AutoGen as a new execution foundation.",
		VerifiedAt: verifiedAt, VerificationNote: "Official maintenance notice and MCP trusted-server warning checked on 2026-07-19.",
		ControlMappings: []ControlMapping{
			{SourcePattern: "event-driven agent messages", HAIControl: "task events, workflow state, and immutable audit records", Boundary: "HAI owns task lifecycle and completion decisions"},
			{SourcePattern: "agent teams and delegation", HAIControl: "planner recommendations and approval-gated workflow assignments", Boundary: "no AutoGen agent can self-authorize an action"},
			{SourcePattern: "MCP Workbench", HAIControl: "agent runtime registry with trusted-server, tool, folder, and network allowlists", Boundary: "MCP tools require a reviewed adapter and the existing risk gate"},
			{SourcePattern: "code execution", HAIControl: "controlled runtime executor with workspace and approval constraints", Boundary: "no generic executor is exposed through this catalog"},
		},
	},
	{
		ID: "microsoft-agent-framework", Name: "Microsoft Agent Framework", UpstreamURL: "https://github.com/microsoft/agent-framework", SourceCatalogURL: "https://github.com/microsoft/autogen",
		SourceCollection: "Official AutoGen successor",
		Status:           StatusCandidate, Category: "multi-agent workflow orchestration", IntegrationMode: "operator-hosted bridge with HAI-owned task boundary",
		Capabilities: []string{"durable agent workflows", "human-in-the-loop orchestration", "checkpointing patterns", "A2A and MCP interoperability"}, RecommendedFor: []string{"AutoGen migration", "reviewed multi-agent workflow", "agent interoperability planning"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Review a narrow, local operator-hosted bridge for one fixed task schema. HAI keeps provider routing, approval, budget, source controls, audit records, emergency stop, and completion verification; no framework-owned tool execution, cloud hosting, or implicit peer discovery is permitted.",
		Rationale:  "Microsoft positions Agent Framework as AutoGen's successor with workflow, human-in-the-loop, observability, and interoperability patterns. It is a stronger future migration target than new AutoGen code, but it remains an external framework that must not replace HAI's control plane.",
		VerifiedAt: verifiedAt, VerificationNote: "Official AutoGen maintenance notice and Microsoft Agent Framework repository, MIT license, and July 2026 release activity checked on 2026-07-19.",
		ControlMappings: []ControlMapping{
			{SourcePattern: "workflow checkpointing and restart", HAIControl: "workflow state machine, durable follow-up records, and verified completion", Boundary: "HAI owns state transitions and does not trust upstream completion signals"},
			{SourcePattern: "human-in-the-loop orchestration", HAIControl: "approval queue and autonomy policy", Boundary: "a framework callback cannot approve or execute a protected action"},
			{SourcePattern: "A2A and MCP interoperability", HAIControl: "reviewed adapters with named peers and local MCP preflight", Boundary: "no implicit peer discovery, process launch, or tool activation"},
			{SourcePattern: "provider middleware", HAIControl: "local-first LLM router with EUR 0 paid default", Boundary: "framework provider settings cannot bypass HAI routing or budget policy"},
		},
	},
	{
		ID: "metagpt", Name: "MetaGPT", UpstreamURL: "https://github.com/FoundationAgents/MetaGPT", SourceCatalogURL: sourceCatalogURL,
		Status: StatusExcluded, Category: "multi-agent software workflow", IntegrationMode: "reference only",
		Capabilities: []string{"role-based software workflow"}, RecommendedFor: []string{"architecture reference"},
		RequiresApproval: false, LocalFirstCompatible: true,
		Activation: "Keep as a reference only until a new upstream review establishes current maintenance and an integration need.",
		Rationale:  "The repository remains available but its latest release and substantive push activity are materially older than the active candidates.",
		VerifiedAt: verifiedAt, VerificationNote: "Upstream repository activity checked on 2026-07-19.",
	},
	{
		ID: "litellm", Name: "LiteLLM", UpstreamURL: "https://github.com/BerriAI/litellm", SourceCatalogURL: "https://ossinsight.io/collections/ai-gateways", SourceCollection: "ai-gateways",
		Status: StatusIntegrated, Category: "self-hosted LLM gateway", IntegrationMode: "operator-hosted loopback-only gateway profile",
		Capabilities: []string{"OpenAI-compatible provider gateway", "local and cloud provider routing", "quota and spend telemetry", "model fallback"}, RecommendedFor: []string{"provider normalization", "local-first model routing", "quota observability"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Set LITELLM_ENABLED=true, a separate LITELLM_API_KEY, LITELLM_MODEL_ID, and a loopback or host.docker.internal LITELLM_BASE_URL. HAI probes /v1/models with the key, rejects remote endpoints, and requires manual approval for generation.",
		Rationale:  "HAI now has a guarded local LiteLLM profile, but the proxy's upstream billing cannot be inferred from its endpoint. HAI therefore retains its EUR 0 policy, approval, audit, and model-selection controls rather than trusting the gateway.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight ai-gateways listing and upstream self-hosted proxy documentation checked on 2026-07-19.",
	},
	{
		ID: "pgvector", Name: "pgvector", UpstreamURL: "https://github.com/pgvector/pgvector", SourceCatalogURL: "https://ossinsight.io/collections/vector-database--vector-store", SourceCollection: "Vector Database & Vector Store",
		Status: StatusIntegrated, Category: "local semantic retrieval", IntegrationMode: "opt-in local pgvector retrieval adapter",
		Capabilities: []string{"vector similarity search", "embedding storage in PostgreSQL", "hybrid memory retrieval"}, RecommendedFor: []string{"semantic memory", "connected-source retrieval", "local evidence search"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Set HAI_SEMANTIC_RETRIEVAL_ENABLED=true, a loopback/host.docker.internal HAI_EMBEDDING_BASE_URL, and HAI_EMBEDDING_MODEL. HAI creates the extension/table, indexes only cached source extractions, and falls back to keyword search whenever local semantic retrieval is unavailable.",
		Rationale:  "HAI now uses pgvector in its existing local PostgreSQL ownership boundary, without introducing a second memory store or a cloud embedding dependency. Owner, project, archive, and sensitivity filters remain in the SQL retrieval query.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight Vector Database & Vector Store listing plus upstream pgvector 0.8.5 PostgreSQL 17 image and exact-search documentation checked on 2026-07-19.",
	},
	{
		ID: "temporal", Name: "Temporal", UpstreamURL: "https://github.com/temporalio/temporal", SourceCatalogURL: "https://ossinsight.io/collections/workflow-scheduler", SourceCollection: "Workflow Scheduler",
		Status: StatusIntegrated, Category: "durable workflow execution", IntegrationMode: "opt-in local service and narrow governed Go worker",
		Capabilities: []string{"durable workflow state", "retry handling", "scheduled work", "worker visibility"}, RecommendedFor: []string{"follow-ups", "long-running workflows", "bounded retries"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Enable the local durability Compose profile and HAI_TEMPORAL_ENABLED. The one registered worker can only run governed follow-up proposal checks; HAI remains authoritative for approval and completion decisions.",
		Rationale:  "Temporal is wired as a local restart-safe scheduling layer for one HAI-owned workflow. It is infrastructure, not an autonomous decision-maker or policy bypass.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight Workflow Scheduler listing and upstream MIT-licensed release activity checked on 2026-07-19.",
	},
	{
		ID: "prometheus", Name: "Prometheus", UpstreamURL: "https://github.com/prometheus/prometheus", SourceCatalogURL: "https://ossinsight.io/collections/monitoring-tool", SourceCollection: "Monitoring Tool",
		Status: StatusIntegrated, Category: "operational observability", IntegrationMode: "opt-in authenticated Prometheus exposition endpoint",
		Capabilities: []string{"service metrics", "health alert rules", "time-series queries", "local monitoring"}, RecommendedFor: []string{"runtime health", "queue metrics", "budget and throughput monitoring"},
		RequiresApproval: false, LocalFirstCompatible: true,
		Activation: "Set HAI_PROMETHEUS_ENABLED=true and a separate HAI_PROMETHEUS_TOKEN, then configure a local collector to scrape /metrics with a bearer token. The exporter has no source-content labels and is disabled unless explicitly enabled.",
		Rationale:  "HAI now exposes a small authenticated Prometheus surface for HTTP request counts and latency. A collector remains operator-configured; Prometheus does not replace HAI's action-oriented system-status view.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight Monitoring Tool listing and upstream Apache-2.0 release activity checked on 2026-07-19.",
	},
	{
		ID: "grafana", Name: "Grafana", UpstreamURL: "https://github.com/grafana/grafana", SourceCatalogURL: "https://ossinsight.io/collections/monitoring-tool", SourceCollection: "Monitoring Tool",
		Status: StatusReferenceOnly, Category: "observability visualization", IntegrationMode: "optional local dashboard",
		Capabilities: []string{"metrics visualization", "alerts", "operational dashboards"}, RecommendedFor: []string{"advanced observability", "operator diagnostics"},
		RequiresApproval: false, LocalFirstCompatible: true,
		Activation: "Do not add Grafana until Prometheus metrics exist and the HAI system-status views cannot meet an identified advanced-observability need.",
		Rationale:  "Grafana is capable but would duplicate HAI's control-room surface unless it is justified by real metrics and advanced operator needs.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight Monitoring Tool listing checked on 2026-07-19.",
	},
	{
		ID: "mcp-inspector", Name: "MCP Inspector", UpstreamURL: "https://github.com/modelcontextprotocol/inspector", SourceCatalogURL: "https://ossinsight.io/collections/model-context-protocol-mcp-client", SourceCollection: "Model Context Protocol (MCP) Client",
		Status: StatusIntegrated, Category: "MCP pre-activation validation", IntegrationMode: "HAI-owned local-only Streamable HTTP preflight",
		Capabilities: []string{"MCP handshake", "bounded tool inventory", "manual connection testing"}, RecommendedFor: []string{"MCP adapter review", "tool allowlist verification", "runtime health diagnostics"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Set HAI_MCP_PREFLIGHT_ENABLED=true and configure reviewed localhost, loopback, or host.docker.internal Streamable HTTP endpoints. An admin may run initialize plus tools/list; HAI never starts a process, accepts credentials, or calls a tool.",
		Rationale:  "The upstream Inspector is a capable developer tool, but its proxy can start processes and connect broadly. HAI adopts only the useful pre-activation protocol check behind a tighter local-only boundary.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight MCP client listing and upstream Inspector architecture/security guidance checked on 2026-07-19.",
	},
	{
		ID: "langchain", Name: "LangChain", UpstreamURL: "https://github.com/langchain-ai/langchain", SourceCatalogURL: "https://ossinsight.io/collections/ai-agent-frameworks", SourceCollection: "AI Agent Frameworks and GraphRAG",
		Status: StatusReferenceOnly, Category: "reasoning and retrieval patterns", IntegrationMode: "architecture reference",
		Capabilities: []string{"tool calling patterns", "retrieval chains", "agent orchestration"}, RecommendedFor: []string{"adapter design", "retrieval design"},
		RequiresApproval: false, LocalFirstCompatible: true,
		Activation: "Do not add as a parallel agent stack. Port only a justified capability through HAI-native Go interfaces after a concrete gap is documented.",
		Rationale:  "It is a broad ecosystem, but importing it would duplicate HAI planning, routing, memory, and tool controls without a clear operational gain.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight AI Agent Frameworks and GraphRAG listings checked on 2026-07-19.",
	},
	{
		ID: "llamaindex", Name: "LlamaIndex", UpstreamURL: "https://github.com/run-llama/llama_index", SourceCatalogURL: "https://ossinsight.io/collections/graphrag---knowledge-graph-based-rag", SourceCollection: "GraphRAG - Knowledge Graph based RAG",
		Status: StatusReferenceOnly, Category: "retrieval and indexing patterns", IntegrationMode: "architecture reference",
		Capabilities: []string{"document indexing", "retrieval pipelines", "source-grounded context"}, RecommendedFor: []string{"connected-source ingestion", "retrieval evaluation"},
		RequiresApproval: false, LocalFirstCompatible: true,
		Activation: "Keep as a reference until a source-retrieval gap cannot be met by HAI's native extraction, full-text, and planned pgvector path.",
		Rationale:  "Its retrieval patterns are useful, but another primary indexing framework would create duplicate memory ownership and harder provenance controls.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight GraphRAG listing checked on 2026-07-19.",
	},
	{
		ID: "cognee", Name: "Cognee", UpstreamURL: "https://github.com/topoteretes/cognee", SourceCatalogURL: "https://ossinsight.io/collections/graphrag---knowledge-graph-based-rag", SourceCollection: "GraphRAG - Knowledge Graph based RAG",
		Status: StatusReferenceOnly, Category: "knowledge graph memory", IntegrationMode: "architecture reference",
		Capabilities: []string{"knowledge graph enrichment", "semantic memory", "entity relationships"}, RecommendedFor: []string{"evidence graph design", "entity linking"},
		RequiresApproval: false, LocalFirstCompatible: true,
		Activation: "Use only as a design reference until HAI has a verified graph-query need, source provenance model, and retention plan.",
		Rationale:  "Graph memory may help later, but adding a second knowledge system before the current memory and source layers are fully operational would increase inconsistency risk.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight GraphRAG listing checked on 2026-07-19.",
	},
	{
		ID: "qdrant", Name: "Qdrant", UpstreamURL: "https://github.com/qdrant/qdrant", SourceCatalogURL: "https://ossinsight.io/collections/vector-database--vector-store", SourceCollection: "Vector Database & Vector Store",
		Status: StatusReferenceOnly, Category: "dedicated vector database", IntegrationMode: "alternative local service",
		Capabilities: []string{"vector search", "collection management", "payload filtering"}, RecommendedFor: []string{"future high-volume semantic retrieval"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Do not add unless pgvector has a measured scale or capability limit. Any migration requires a provenance-preserving export, retention plan, and rollback evidence.",
		Rationale:  "Qdrant is a credible dedicated option, but is intentionally deferred to avoid two active vector stores before HAI has a demonstrated need.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight Vector Database & Vector Store listing checked on 2026-07-19.",
	},
	{
		ID: "llama-cpp", Name: "llama.cpp", UpstreamURL: "https://github.com/ggml-org/llama.cpp", SourceCatalogURL: "https://ossinsight.io/collections/chatgpt-alternatives", SourceCollection: "ChatGPT Alternatives",
		Status: StatusIntegrated, Category: "local model inference", IntegrationMode: "operator-configured loopback OpenAI-compatible model server",
		Capabilities: []string{"local GGUF inference", "OpenAI-compatible server", "CPU and GPU deployment", "offline model serving"}, RecommendedFor: []string{"local-first LLM routing", "low-VRAM inference", "offline fallback"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Install and start llama.cpp outside HAI on a loopback-only endpoint, record model provenance and hardware limits, then set LLAMA_CPP_BASE_URL and LLAMA_CPP_MODEL_ID. HAI rejects non-local endpoints and requires a live /v1/models probe before routing or generation.",
		Rationale:  "HAI now owns a first-class, local-only llama.cpp provider profile in both model services. The upstream server remains operator-installed and is not active until its configured endpoint passes a live probe.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight ChatGPT Alternatives listing plus upstream MIT license and current release activity checked on 2026-07-19.",
	},
	{
		ID: "searxng", Name: "SearXNG", UpstreamURL: "https://github.com/searxng/searxng", SourceCatalogURL: "https://ossinsight.io/collections/search-engine", SourceCollection: "Search Engine",
		Status: StatusIntegrated, Category: "local public-source discovery", IntegrationMode: "operator-configured local JSON search adapter",
		Capabilities: []string{"self-hosted metasearch", "JSON source candidates", "privacy-oriented discovery", "search-engine aggregation"}, RecommendedFor: []string{"current public research", "source discovery", "grounded-answer evidence selection"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Run and review a local SearXNG instance separately, enable its JSON format, then set HAI_SEARXNG_ENABLED=true and HAI_SEARXNG_BASE_URL to a local/private endpoint. HAI sends bounded queries only, returns candidate sources, does not fetch pages, and does not treat snippets as verified evidence.",
		Rationale:  "HAI now has a constrained local discovery adapter for the gap between a research question and source selection. It remains disabled by default because its configured search engines receive the query, and AGPL-3.0 deployment terms must be reviewed independently.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight Search Engine collection, upstream repository activity, AGPL-3.0 license, and the official JSON search API documentation checked on 2026-07-19.",
	},
	{
		ID: "playwright", Name: "Playwright", UpstreamURL: "https://github.com/microsoft/playwright", SourceCatalogURL: "https://ossinsight.io/collections/testing-tools", SourceCollection: "Testing Tools",
		Status: StatusIntegrated, Category: "controlled browser verification", IntegrationMode: "opt-in named local read-only verification worker",
		Capabilities: []string{"browser automation", "deterministic web verification", "trace artifacts", "cross-browser testing"}, RecommendedFor: []string{"web workflow verification", "regression checks", "approved browser tasks"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Use only through a reviewed adapter with named approved flows, origin allowlists, no secret capture, bounded downloads, and trace retention controls. A browser test cannot send, publish, purchase, or change accounts without the normal HAI approval gate.",
		Rationale:  "Playwright is a maintained, Apache-2.0 local testing framework that can verify an approved browser workflow. It is not a general web-execution permission.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight Testing Tools listing and upstream Apache-2.0 license/current releases checked on 2026-07-19.",
	},
	{
		ID: "wasmtime", Name: "Wasmtime", UpstreamURL: "https://github.com/bytecodealliance/wasmtime", SourceCatalogURL: "https://ossinsight.io/collections/webassembly-runtime", SourceCollection: "WebAssembly Runtime",
		Status: StatusIntegrated, Category: "bounded local WASM execution", IntegrationMode: "opt-in content-addressed local WASI runner",
		Capabilities: []string{"WASM runtime", "WASI capability controls", "resource limits", "portable local execution"}, RecommendedFor: []string{"deterministic transforms", "untrusted plugin experiments", "bounded local helpers"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Enable the wasi Compose profile only after adding reviewed .wasm modules and their SHA-256 hashes to HAI_WASI_MODULES. The runner has no inherited network, preopened directories, environment, or arguments and is capped at 256 MiB, 0.5 CPU, and five seconds. Each run remains approval-gated.",
		Rationale:  "Wasmtime is a maintained Apache-2.0 runtime with Windows distributions and configurable resource controls, but sandboxing still depends on HAI's explicit capability policy and adapter implementation.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight WebAssembly Runtime listing and upstream Apache-2.0/current release documentation checked on 2026-07-19.",
	},
	{
		ID: "ortools", Name: "OR-Tools", UpstreamURL: "https://github.com/google/or-tools", SourceCatalogURL: "https://ossinsight.io/collections/optimization-solvers", SourceCollection: "Optimization Solvers",
		Status: StatusIntegrated, Category: "deterministic planning optimisation", IntegrationMode: "opt-in internal CP-SAT proposal service",
		Capabilities: []string{"constraint solving", "bounded schedule proposals", "no-overlap planning", "infeasibility evidence"}, RecommendedFor: []string{"task sequencing", "calendar suggestions", "field-job planning"},
		RequiresApproval: false, LocalFirstCompatible: true,
		Activation: "Set HAI_PLANNING_OPTIMIZER_ENABLED=true and run the Compose optimization profile. The internal service accepts only opaque job IDs, minute windows, durations, priorities, and optional fixed starts; it returns a schedule proposal and deferred work. It has no workflow, calendar, filesystem, tool, or external-network apply endpoint.",
		Rationale:  "HAI now uses OR-Tools in a narrow local CP-SAT service that complements LLM planning with deterministic constraints. Results remain proposals; external or workflow changes still require the existing HAI planning, verification, and approval paths.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight Optimization Solvers listing and upstream OR-Tools Apache-2.0 v9.15 release, CP-SAT documentation, and current Python package checked on 2026-07-19.",
	},
	{
		ID: "activepieces", Name: "Activepieces", UpstreamURL: "https://github.com/activepieces/activepieces", SourceCatalogURL: "https://ossinsight.io/collections/zapier-alternatives", SourceCollection: "Zapier Alternatives",
		Status: StatusReferenceOnly, Category: "workflow connector platform", IntegrationMode: "operator-hosted platform reference",
		Capabilities: []string{"workflow connectors", "event triggers", "MCP ecosystem", "approval-aware automation patterns"}, RecommendedFor: []string{"connector design", "workflow template research"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Keep as a reference until HAI has a specific connector gap that justifies a reviewed, narrowly scoped adapter. Do not deploy a second autonomous workflow control plane by default.",
		Rationale:  "The community edition is MIT and actively maintained, but a broad automation platform would duplicate HAI's workflow, secrets, approval, and audit responsibilities without a demonstrated gap.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight Zapier Alternatives listing and upstream community/enterprise licensing split checked on 2026-07-19.",
	},
	{
		ID: "n8n", Name: "n8n", UpstreamURL: "https://github.com/n8n-io/n8n", SourceCatalogURL: "https://ossinsight.io/collections/zapier-alternatives", SourceCollection: "Zapier Alternatives",
		Status: StatusLicenseReview, Category: "workflow automation platform", IntegrationMode: "separate platform deployment",
		Capabilities: []string{"workflow automation", "integrations", "visual workflows", "self-hosting"}, RecommendedFor: []string{"connector landscape", "workflow pattern research"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Do not integrate or vendor n8n until the Sustainable Use License, enterprise-file restrictions, secret handling, and overlap with HAI workflow ownership are reviewed for the intended deployment.",
		Rationale:  "n8n is capable and currently maintained, but its fair-code licensing and overlapping automation control plane require an explicit legal and architecture decision before adoption.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight Zapier Alternatives listing and upstream Sustainable Use License restrictions checked on 2026-07-19.",
	},
	{
		ID: "mem0", Name: "Mem0", UpstreamURL: "https://github.com/mem0ai/mem0", SourceCatalogURL: "https://ossinsight.io/collections/llm-tools", SourceCollection: "LLM Tools",
		Status: StatusReferenceOnly, Category: "agent memory patterns", IntegrationMode: "architecture reference",
		Capabilities: []string{"long-term memory", "memory consolidation", "retrieval filters", "memory lifecycle"}, RecommendedFor: []string{"memory evaluation", "consolidation design"},
		RequiresApproval: false, LocalFirstCompatible: true,
		Activation: "Do not add as a second memory authority. Port only a measured memory capability through HAI's existing source-link, correction, retention, and deletion model after a native gap is demonstrated.",
		Rationale:  "Mem0 is an active Apache-2.0 memory project, but adopting it wholesale would split ownership of provenance, corrections, and personal-data retention.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight LLM Tools listing and upstream Apache-2.0/current release activity checked on 2026-07-19.",
	},
	{
		ID: "openmetadata", Name: "OpenMetadata", UpstreamURL: "https://github.com/open-metadata/OpenMetadata", SourceCatalogURL: "https://ossinsight.io/collections/open-source-data-catalogs", SourceCollection: "Open Source Data Catalogs",
		Status: StatusReferenceOnly, Category: "source-governance and lineage patterns", IntegrationMode: "architecture reference",
		Capabilities: []string{"metadata catalog", "data lineage", "governance", "source discovery"}, RecommendedFor: []string{"connected-source provenance", "data-quality governance"},
		RequiresApproval: false, LocalFirstCompatible: true,
		Activation: "Use as a reference until HAI's connected-source estate has a demonstrated enterprise-scale metadata governance gap. Keep HAI's source registry and audit model authoritative for local personal data.",
		Rationale:  "OpenMetadata is actively maintained and Apache-2.0, but is a large independent control plane whose deployment would exceed HAI's current local-first scope.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight Open Source Data Catalogs listing and upstream Apache-2.0/current release activity checked on 2026-07-19.",
	},
	{
		ID: "pydantic-ai", Name: "PydanticAI", UpstreamURL: "https://github.com/pydantic/pydantic-ai", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10098/repos/", SourceCollection: "AI Agent Frameworks",
		Status: StatusIntegrated, Category: "typed local planning and structured-output boundary", IntegrationMode: "integrated opt-in local structured-proposal runner",
		Capabilities: []string{"typed model outputs", "schema-first agent plans", "tool result validation", "dependency injection patterns"}, RecommendedFor: []string{"structured planning", "schema-constrained extraction", "validated agent proposals"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Enable only the typed-planning Compose profile with one operator-reviewed loopback OpenAI-compatible model. HAI sends a short task request and optional success criteria to a fixed Pydantic schema. The runner has no tools, MCP, web, file, source, memory, persistence, retry, provider-selection, approval, or execution capability; its draft remains subject to HAI validation and policy.",
		Rationale:  "The integrated local PydanticAI runner adds a constrained model-assisted planning draft without replacing HAI's deterministic planner, verifier, provider router, memory, audit, or approval control plane.",
		VerifiedAt: "2026-07-20", VerificationNote: "Upstream main and v2.13.0 release checked on 2026-07-20: MIT licence and maintained Python package. HAI pins pydantic-ai-slim[openai] 2.13.0 in an optional internal runner and exposes only one local schema-validated proposal endpoint.",
		ControlMappings: []ControlMapping{
			{SourcePattern: "typed agent output", HAIControl: "HAI-owned schemas and verification status", Boundary: "model output remains a draft until HAI validates it"},
			{SourcePattern: "tool-capable agent", HAIControl: "runtime allowlists and approval queue", Boundary: "the adapter cannot select tools or produce side effects"},
		},
	},
	{
		ID: "localai", Name: "LocalAI", UpstreamURL: "https://github.com/mudler/LocalAI", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10109/repos/", SourceCollection: "LLM Inference Engines",
		Status: StatusIntegrated, Category: "local multimodal OpenAI-compatible inference", IntegrationMode: "integrated loopback OpenAI-compatible provider profile",
		Capabilities: []string{"local OpenAI-compatible API", "local model hosting", "multimodal serving", "CPU-capable inference"}, RecommendedFor: []string{"local model fallback", "OpenAI-compatible local endpoint", "offline multimodal preparation"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Review a loopback-only LocalAI endpoint with explicit model provenance, resource limits, model allowlists, no public bind, provider health checks, and HAI's existing EUR 0 budget policy. HAI must not auto-download models, expose the endpoint, or route sensitive data before configuration review.",
		Rationale:  "HAI now implements the LocalAI provider contract alongside Ollama and llama.cpp while preserving explicit configuration, loopback-only reachability, a live probe, and local-first routing policy.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight LLM Inference Engines listing and GitHub metadata checked on 2026-07-19: active master branch, MIT licence. HAI implements only the provider profile; no LocalAI service or model is installed or configured by HAI.",
		ControlMappings: []ControlMapping{
			{SourcePattern: "OpenAI-compatible server", HAIControl: "LLM router provider policy and loopback probe", Boundary: "the server cannot enable paid fallback or public egress"},
			{SourcePattern: "model catalogue", HAIControl: "operator-approved model provenance", Boundary: "HAI never downloads or selects a model implicitly"},
		},
	},
	{
		ID: "cloudquery", Name: "CloudQuery", UpstreamURL: "https://github.com/cloudquery/cloudquery", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10056/repos/", SourceCollection: "Data Integration",
		Status: StatusCandidate, Category: "read-first source inventory connector", IntegrationMode: "reviewed scoped data-ingestion bridge",
		Capabilities: []string{"connector schemas", "incremental extraction patterns", "source inventory", "local destination support"}, RecommendedFor: []string{"approved source ingestion", "account inventory", "incremental connector design"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Review a single read-first source connector with least-privilege credentials, field and folder allowlists, cursor handling, local retention, provenance links, audit events, revocation, and a no-write probe. Do not import broad cloud or SaaS account inventories by default.",
		Rationale:  "CloudQuery offers mature connector and incremental-ingestion patterns that can inform a scoped HAI source adapter, while HAI remains the owner of source permissions, extraction, memory updates, deletion, and approvals.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight Data Integration listing and GitHub metadata checked on 2026-07-19: active main branch, MPL-2.0 licence; no CloudQuery connector, credential, or destination is installed by HAI.",
		ControlMappings: []ControlMapping{
			{SourcePattern: "source connector", HAIControl: "connector registry and per-source permission policy", Boundary: "no credentials, scopes, or sync jobs are created from catalog discovery"},
			{SourcePattern: "extracted records", HAIControl: "provenance, memory review, and deletion controls", Boundary: "ingested data does not become a fact or task without HAI processing"},
		},
	},
	{
		ID: "opik", Name: "Opik", UpstreamURL: "https://github.com/comet-ml/opik", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10135/repos/", SourceCollection: "AI Observability",
		Status: StatusCandidate, Category: "local trace and evaluation observability", IntegrationMode: "reviewed local telemetry adapter",
		Capabilities: []string{"LLM traces", "agent evaluation", "experiment comparison", "quality monitoring"}, RecommendedFor: []string{"local evaluation evidence", "trace review", "agent quality diagnostics"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Review a local-only telemetry deployment with trace redaction, short retention, non-production fixtures first, explicit provider egress control, and an export/delete path. It cannot become HAI's audit authority or receive secrets, full personal documents, or unredacted credentials.",
		Rationale:  "Opik is a maintained Apache-2.0 local observability candidate that can complement HAI's audit records with evaluation evidence when Langfuse or OpenLLMetry do not meet a demonstrated review need.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight AI Observability and AI Evaluation & Testing listings plus GitHub metadata checked on 2026-07-19: active main branch, Apache-2.0 licence; no Opik service or telemetry export is configured by HAI.",
		ControlMappings: []ControlMapping{
			{SourcePattern: "LLM trace", HAIControl: "redaction and audit-event policy", Boundary: "observability data cannot override HAI verification or approval decisions"},
			{SourcePattern: "evaluation dashboard", HAIControl: "source-backed metric definitions", Boundary: "metrics remain advisory and must identify their scope and freshness"},
		},
	},
	{
		ID: "deepteam", Name: "DeepTeam", UpstreamURL: "https://github.com/confident-ai/deepteam", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10138/repos/", SourceCollection: "AI Red Teaming",
		Status: StatusCandidate, Category: "contained AI red-team evaluation", IntegrationMode: "reviewed no-write local evaluation adapter",
		Capabilities: []string{"agent red teaming", "attack scenario generation", "safety evaluation", "reporting patterns"}, RecommendedFor: []string{"redacted safety regression", "prompt-injection evaluation", "agent policy tests"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Review an isolated test-only runner with approved synthetic or redacted fixtures, local or explicitly approved providers, strict rate and cost limits, no external target actions, and report-only output. It cannot access connected accounts, real secrets, or execute discovered attack paths.",
		Rationale:  "DeepTeam is a maintained Apache-2.0 candidate for repeatable safety regression testing that can strengthen HAI's verification plane without granting a test harness any operational authority.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight AI Red Teaming listing and GitHub metadata checked on 2026-07-19: active main branch, Apache-2.0 licence; no DeepTeam dependency or evaluation job is installed by HAI.",
		ControlMappings: []ControlMapping{
			{SourcePattern: "red-team scenario", HAIControl: "synthetic fixture and provider policy", Boundary: "tests cannot use connected sources or real accounts"},
			{SourcePattern: "safety report", HAIControl: "verification review queue", Boundary: "a finding creates review work, not an autonomous remediation"},
		},
	},
	{
		ID: "openspec", Name: "OpenSpec", UpstreamURL: "https://github.com/Fission-AI/OpenSpec", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10112/repos/", SourceCollection: "AI Coding Assistants",
		Status: StatusCandidate, Category: "spec-driven coding workflow", IntegrationMode: "reviewed repository-local planning adapter",
		Capabilities: []string{"change specifications", "acceptance criteria", "implementation plans", "coding workflow structure"}, RecommendedFor: []string{"software task planning", "acceptance criteria", "reviewable coding proposals"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Review a repository-local, read-only specification generator that writes no files until an owner approves a proposed change scope, tests, rollback, and workspace boundary. Generated specifications are planning artifacts and cannot authorize code edits, commits, branches, or pulls.",
		Rationale:  "OpenSpec provides a maintained, lightweight spec-first pattern that can improve HAI coding-task clarity without introducing another coding agent, source authority, or execution path.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight AI Coding Assistants listing and GitHub metadata checked on 2026-07-19: active main branch, MIT licence; no OpenSpec package, repository hook, or filesystem writer is installed by HAI.",
		ControlMappings: []ControlMapping{
			{SourcePattern: "change specification", HAIControl: "task success criteria and review queue", Boundary: "a specification is not permission to edit code"},
			{SourcePattern: "repository workflow", HAIControl: "workspace allowlist and approval policy", Boundary: "no commit, pull request, or network action is implicit"},
		},
	},
	{
		ID: "pipecat", Name: "Pipecat", UpstreamURL: "https://github.com/pipecat-ai/pipecat", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10118/repos/", SourceCollection: "Multimodal AI",
		Status: StatusCandidate, Category: "local voice and multimodal intake", IntegrationMode: "reviewed local input pipeline adapter",
		Capabilities: []string{"voice pipelines", "multimodal events", "turn detection patterns", "local transport options"}, RecommendedFor: []string{"approved voice capture", "multimodal intake", "ambient input prototypes"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Review an opt-in local microphone or file-import adapter with explicit capture indicator, per-source consent, local retention, transcription provenance, redaction, pause controls, and no always-on recording default. It cannot invoke tools or contacts from a spoken instruction without HAI's standard approval path.",
		Rationale:  "Pipecat is a maintained BSD-2-Clause framework that can inform a consentful local voice-intake path, while HAI preserves the user-controlled ambient, memory, execution, and safety boundaries.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight Multimodal AI and Agent Harness listings plus GitHub metadata checked on 2026-07-19: active main branch, BSD-2-Clause licence; no Pipecat pipeline or audio capture is enabled by HAI.",
		ControlMappings: []ControlMapping{
			{SourcePattern: "voice event", HAIControl: "source intake permission and provenance", Boundary: "audio is never captured or retained by default"},
			{SourcePattern: "multimodal agent turn", HAIControl: "planner and approval-gated runtime", Boundary: "input interpretation cannot self-authorize action"},
		},
	},
	{
		ID: "llm-guard", Name: "LLM Guard", UpstreamURL: "https://github.com/protectai/llm-guard", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10116/repos/", SourceCollection: "AI Safety & Alignment",
		Status: StatusExcluded, Category: "LLM security toolkit", IntegrationMode: "not adopted",
		Capabilities: []string{"prompt filtering", "output filtering", "security scanning"}, RecommendedFor: []string{"safety pattern research"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Do not integrate. Reassess only if a maintained successor, clear data-handling model, and a demonstrated gap beyond HAI's existing redaction and validation controls are recorded.",
		Rationale:  "The current upstream is archived. HAI will not add an archived safety dependency to a control plane that must remain maintainable.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight AI Safety & Alignment listing and GitHub metadata checked on 2026-07-19: repository reports archived=true despite an MIT licence; no LLM Guard package is installed by HAI.",
	},
	{
		ID: "openai-evals", Name: "OpenAI Evals", UpstreamURL: "https://github.com/openai/evals", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10119/repos/", SourceCollection: "AI Evaluation & Testing",
		Status: StatusLicenseReview, Category: "LLM evaluation framework", IntegrationMode: "licence-review reference",
		Capabilities: []string{"evaluation framework", "benchmark registry", "model quality patterns"}, RecommendedFor: []string{"evaluation design", "benchmark research"},
		RequiresApproval: true, LocalFirstCompatible: false,
		Activation: "Do not integrate until the missing SPDX licence signal, current dependency/maintenance model, provider egress, test-data handling, and HAI evaluation overlap are explicitly reviewed.",
		Rationale:  "The repository remains active, but its GitHub metadata does not currently provide an SPDX licence assertion. HAI holds it rather than treating its popularity as deployment approval.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight AI Evaluation & Testing listing and GitHub metadata checked on 2026-07-19: active main branch, licence reported as NOASSERTION; no OpenAI Evals package or provider access is configured by HAI.",
	},
	{
		ID: "agentbench", Name: "AgentBench", UpstreamURL: "https://github.com/THUDM/AgentBench", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10141/repos/", SourceCollection: "Agent Harness",
		Status: StatusReferenceOnly, Category: "agent benchmark reference", IntegrationMode: "evaluation architecture reference",
		Capabilities: []string{"agent benchmark tasks", "agent evaluation taxonomy", "completion assessment patterns"}, RecommendedFor: []string{"benchmark design", "agent quality research"},
		RequiresApproval: false, LocalFirstCompatible: true,
		Activation: "Use as a reference for HAI-native, redacted evaluation fixtures only. Do not import its task environments, external services, or benchmark claims as HAI production evidence without a dedicated reproduction plan.",
		Rationale:  "AgentBench is maintained and Apache-2.0, but HAI needs task-specific, source-controlled evaluation fixtures rather than a second benchmark runtime.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight Agent Harness listing and GitHub metadata checked on 2026-07-19: active main branch, Apache-2.0 licence; no AgentBench task environment is installed by HAI.",
	},
	{
		ID: "omniparser", Name: "OmniParser", UpstreamURL: "https://github.com/microsoft/OmniParser", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10113/repos/", SourceCollection: "AI Browser Agents",
		Status: StatusLicenseReview, Category: "screen parsing for GUI agents", IntegrationMode: "licence-review reference",
		Capabilities: []string{"screen parsing", "visual element detection", "GUI grounding patterns"}, RecommendedFor: []string{"screen-understanding research", "browser verification design"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Do not integrate until its CC-BY-4.0 distribution implications, screenshot privacy, model weights, local hardware requirements, output retention, and interaction with HAI's browser allowlists are reviewed.",
		Rationale:  "The project is active, but screen capture is sensitive and the reported licence needs an explicit product and data-handling review before it can influence HAI browser workflows.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight AI Browser Agents listing and GitHub metadata checked on 2026-07-19: active master branch, CC-BY-4.0 licence; no OmniParser model, screenshot capture, or GUI agent is installed by HAI.",
	},
	{
		ID: "mcp-servers", Name: "MCP Servers Reference", UpstreamURL: "https://github.com/modelcontextprotocol/servers", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10105/repos/", SourceCollection: "MCP Servers",
		Status: StatusLicenseReview, Category: "MCP server reference collection", IntegrationMode: "licence-review reference",
		Capabilities: []string{"MCP server examples", "tool schema patterns", "connector reference"}, RecommendedFor: []string{"MCP adapter design", "tool boundary research"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Do not adopt the collection as a tool bundle. Each server needs its own repository, licence, credential, network, tool allowlist, preflight, audit, rollback, and approval review before any local adapter is considered.",
		Rationale:  "The repository is active but reports no SPDX licence through GitHub metadata and contains heterogeneous server examples; a collection cannot inherit a single trust decision.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight MCP Servers listing and GitHub metadata checked on 2026-07-19: active main branch, licence reported as NOASSERTION; no MCP Servers example or tool has been installed or enabled by HAI.",
	},
	{
		ID: "evidently", Name: "Evidently", UpstreamURL: "https://github.com/evidentlyai/evidently", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10135/repos/", SourceCollection: "AI Observability",
		Status: StatusIntegrated, Category: "local AI quality evaluation and monitoring", IntegrationMode: "integrated opt-in internal report-only evaluation bridge",
		Capabilities: []string{"LLM evaluation", "RAG evaluation", "data-quality checks", "drift detection", "pass/fail test suites"}, RecommendedFor: []string{"source-grounded answer regression", "retrieval evaluation", "routing quality review", "input-quality monitoring"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "HAI includes a disabled internal report runner. Enable only the local evaluation Compose profile after reviewing synthetic/redacted fixture provenance, capacity, retention, and result review. The bridge rejects detected personal data and secrets, returns metadata only, and cannot mark an answer verified, change routing or policy, enable a provider, or execute an action.",
		Rationale:  "Evidently now contributes a contained local quality-evidence path without displacing HAI's source grounding, deterministic validators, approval gates, or audit authority.",
		VerifiedAt: "2026-07-20", VerificationNote: "Official repository reviewed on 2026-07-20: Apache-2.0, with offline reports, test suites, and optional self-hosted monitoring. HAI ships only an opt-in internal DataSummary bridge for bounded synthetic/redacted fixtures; no service is enabled, no fixture is persisted, and no telemetry export is configured.",
		ControlMappings: []ControlMapping{
			{SourcePattern: "evaluation report or test suite", HAIControl: "verification evidence and review queue", Boundary: "a score cannot claim completion or change policy automatically"},
			{SourcePattern: "monitoring dashboard", HAIControl: "local observability and retention policy", Boundary: "no prompt, source, or telemetry egress is implicit"},
		},
	},
	{
		ID: "livekit-agents", Name: "LiveKit Agents", UpstreamURL: "https://github.com/livekit/agents", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10118/repos/", SourceCollection: "Multimodal AI",
		Status: StatusCandidate, Category: "opt-in realtime voice and multimodal intake", IntegrationMode: "reviewed, operator-hosted realtime intake bridge",
		Capabilities: []string{"realtime voice sessions", "multimodal conversation", "MCP tool compatibility", "agent testing", "job scheduling"}, RecommendedFor: []string{"opt-in voice assistant", "accessibility intake", "real-time local interaction prototypes"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Review an opt-in local or self-hosted LiveKit deployment with a visible capture state, per-session consent, explicit STT/LLM/TTS providers, retained-transcript controls, a named room allowlist, and HAI's existing tool and approval gates. It must not activate a microphone, make calls, contact anyone, or invoke MCP tools without separate HAI authorization.",
		Rationale:  "LiveKit Agents is a maintained Apache-2.0 framework for real-time multimodal interaction and can eventually provide a consentful voice front door, while HAI keeps task creation, memory, provider routing, execution, and external effects under its own controls.",
		VerifiedAt: verifiedAt, VerificationNote: "Official repository reviewed on 2026-07-19: Apache-2.0, latest livekit-agents@1.6.6 released 2026-07-18, supports MCP and local terminal testing but production requires explicit LiveKit URL, API key, and secret. No LiveKit service, room, capture device, or credentials are configured by HAI.",
		ControlMappings: []ControlMapping{
			{SourcePattern: "realtime voice session", HAIControl: "source-intake consent, provenance, and pause controls", Boundary: "audio is not captured or retained by default"},
			{SourcePattern: "function or MCP tool", HAIControl: "runtime registry and approval-gated action policy", Boundary: "a spoken instruction cannot self-authorize a tool or external effect"},
		},
	},
	{
		ID: "mistral-rs", Name: "mistral.rs", UpstreamURL: "https://github.com/ericlbuehler/mistral.rs", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10109/repos/", SourceCollection: "LLM Inference Engines",
		Status: StatusIntegrated, Category: "local multimodal model serving", IntegrationMode: "integrated loopback OpenAI-compatible provider profile",
		Capabilities: []string{"local inference", "OpenAI-compatible serving", "Anthropic-compatible serving", "multimodal inputs", "hardware-aware tuning"}, RecommendedFor: []string{"local-first model experiments", "multimodal intake evaluation", "OpenAI-compatible provider compatibility"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Review one loopback-only local server with an approved model, model licence, hardware and resource limit, context window, request retention setting, file-input policy, and disabled built-in agentic tools. Set MISTRAL_RS_BASE_URL and MISTRAL_RS_MODEL_ID only after that review. HAI calls only /v1/models and /v1/chat/completions through its existing local provider probe and EUR 0 routing policy; it never starts the server or selects a model automatically.",
		Rationale:  "HAI now implements a distinct mistral.rs loopback provider profile for an explicit local inference need while retaining live probing, local-first routing, budget controls, audit, and approval gates. Its upstream agentic surfaces are not integrated.",
		VerifiedAt: verifiedAt, VerificationNote: "Official repository reviewed on 2026-07-20: MIT, active master branch, OpenAI-compatible /v1 and Anthropic-compatible Messages endpoints plus optional agentic tools. HAI implements only the loopback /v1/models and /v1/chat/completions provider profile; no mistral.rs server, model, UI, file endpoint, MCP, Skills, or built-in tool surface is configured.",
		ControlMappings: []ControlMapping{
			{SourcePattern: "OpenAI-compatible server", HAIControl: "local provider probe and EUR 0 router", Boundary: "provider availability does not bypass model selection, budget, or task approval"},
			{SourcePattern: "agentic shell, web, or code tool", HAIControl: "controlled runtime executor", Boundary: "upstream built-in tools remain disabled and are never inherited by HAI"},
		},
	},
	{
		ID: "ag2", Name: "AG2", UpstreamURL: "https://github.com/ag2ai/ag2", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10104/repos/", SourceCollection: "AI Agent Frameworks",
		Status: StatusCompatibility, Category: "multi-agent framework compatibility", IntegrationMode: "operator-hosted compatibility bridge or migration reference",
		Capabilities: []string{"agent collaboration", "human-in-the-loop workflows", "tool-use patterns", "structured outputs", "multi-agent orchestration"}, RecommendedFor: []string{"existing AG2 workload review", "AutoGen-era migration analysis", "multi-agent interoperability research"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Do not make AG2 a second HAI runtime. Review only a narrow, local bridge for an existing AG2 workload with a fixed task schema, model allowlist, disabled code execution by default, workspace and network constraints, and HAI-owned audit and approval enforcement. New HAI work continues to use native workflow controls or separately reviewed successor profiles.",
		Rationale:  "AG2 remains an actively maintained, Apache-2.0 AutoGen-derived framework with useful human-in-the-loop and multi-agent patterns. It overlaps HAI's orchestration layer, so its correct role is compatibility and migration review, not a parallel autonomous control plane.",
		VerifiedAt: verifiedAt, VerificationNote: "Official repository reviewed on 2026-07-19: Apache-2.0, active main branch, now uses the ag2 package and documents multi-agent, tool, and code-execution patterns. No AG2 package, agent, model key, or code executor is installed or configured by HAI.",
		ControlMappings: []ControlMapping{
			{SourcePattern: "agent cooperation and handoff", HAIControl: "workflow state, assignments, and approval queue", Boundary: "HAI owns lifecycle, approval, and completion state"},
			{SourcePattern: "code execution or registered tool", HAIControl: "controlled runtime executor and tool allowlist", Boundary: "no AG2 agent receives generic host, secret, or network authority"},
		},
	},
	{
		ID: "ragflow", Name: "RAGFlow", UpstreamURL: "https://github.com/infiniflow/ragflow", SourceCatalogURL: "https://github.com/infiniflow/ragflow", SourceCollection: "user-provided RAG candidate",
		Status: StatusIntegrated, Category: "source-linked document retrieval and parsing", IntegrationMode: "integrated opt-in local document retrieval bridge",
		Capabilities: []string{"document parsing", "retrieval and reranking", "grounded citations", "chunk inspection", "multimodal document intake"}, RecommendedFor: []string{"document-heavy research", "evidence-linked retrieval evaluation", "complex PDF and office-document parsing"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "First measure a real document parsing or retrieval gap against HAI's existing source-ingestion path. Then review a separately deployed local instance with a named source-folder allowlist, explicit connector scopes, local model endpoints, retention/deletion/export controls, citation and chunk provenance, CPU/RAM/disk limits, and every code-execution feature disabled. Imported text remains an external retrieval index: it cannot become HAI memory, create facts, send data, or call tools without HAI verification and approval.",
		Rationale:  "RAGFlow is a current Apache-2.0, self-hostable RAG engine with document parsing, reranking, citation, and multimodal ingestion capabilities. It can strengthen document-heavy retrieval after a measured gap review, but it is too broad to become a competing source, memory, workflow, or agent control plane.",
		VerifiedAt: "2026-07-20", VerificationNote: "Official repository reviewed on 2026-07-20: Apache-2.0, active main branch, current v0.26.4 deployment guidance, with cited retrieval and document-parsing capabilities. Its self-hosting guidance requires at least 4 CPU cores, 16 GB RAM, 50 GB disk, Docker Compose, and gVisor only for its optional code executor. No RAGFlow service, index, connector, model, or code executor is configured by HAI.",
		ControlMappings: []ControlMapping{
			{SourcePattern: "document parsing and chunks", HAIControl: "source registry, provenance, and correction workflow", Boundary: "parsed material is not trusted memory or a verified claim by default"},
			{SourcePattern: "retrieval citation", HAIControl: "grounded-answer claim verification", Boundary: "a cited chunk must still be checked for support, freshness, and conflicts"},
			{SourcePattern: "agent or code-executor component", HAIControl: "approval-gated runtime registry", Boundary: "RAGFlow agent and executor features remain disabled outside a separately reviewed adapter"},
		},
	},
	{
		ID: "serena", Name: "Serena", UpstreamURL: "https://github.com/oraios/serena", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10106/repos/", SourceCollection: "Coding Agents",
		Status: StatusCandidate, Category: "read-only semantic code context", IntegrationMode: "reviewed local MCP code-context bridge",
		Capabilities: []string{"symbol-level code retrieval", "reference lookup", "language-server diagnostics", "semantic repository context"}, RecommendedFor: []string{"large repository inspection", "source-grounded code planning", "cross-file impact review", "semantic code retrieval"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "A self-started local Serena HTTP endpoint may first pass HAI's read-only MCP preflight; HAI never starts the process or supplies credentials. Then review one named repository root, a read-only symbol and diagnostic tool allowlist, a fixed language-server set, bounded response sizes, workspace isolation, audit events, and an immediate disconnect path. Editing, shell commands, Serena memory writes, JetBrains integration, external-project lookup, and automatic language-server installation remain disabled. Any later write path must use HAI's separate controlled runtime and approval flow.",
		Rationale:  "Serena can reduce code-context cost and improve repository inspection through symbol-aware retrieval without becoming HAI's coding agent or execution authority. Its editing and shell features are deliberately outside this candidate's scope.",
		VerifiedAt: "2026-07-20", VerificationNote: "Official repository reviewed on 2026-07-20: active main branch, MIT, latest v1.6.0 released 2026-07-16. It provides MCP-based retrieval, editing, refactoring, diagnostics, and optional memory; no Serena process, language server, repository mount, or MCP endpoint is configured by HAI.",
		ControlMappings: []ControlMapping{
			{SourcePattern: "symbol retrieval and diagnostics", HAIControl: "repository source links and code-review evidence", Boundary: "retrieved source context is read-only evidence, not an approved code change"},
			{SourcePattern: "symbolic editing, shell, and memory tools", HAIControl: "controlled runtime executor and HAI memory plane", Boundary: "those upstream tools remain disabled and cannot inherit workspace, secret, or host authority"},
		},
	},
	{
		ID: "ufo", Name: "Microsoft UFO", UpstreamURL: "https://github.com/microsoft/UFO", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10113/repos/", SourceCollection: "AI Browser Agents",
		Status: StatusReferenceOnly, Category: "Windows and multi-device agent architecture", IntegrationMode: "high-risk host-automation architecture reference",
		Capabilities: []string{"Windows UI automation", "device capability matching", "DAG orchestration", "execution recovery patterns"}, RecommendedFor: []string{"Windows automation safety research", "device capability registry design", "controlled execution architecture"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Do not install or connect UFO as an HAI runtime. Revisit only after a separate Windows execution safety review defines an isolated user session, visible operator control, per-application allowlist, no-secret boundary, screen-capture retention policy, deterministic rollback, emergency stop, and per-action approval. Multi-device registration, GUI clicking, Win32/UIA/COM access, API keys, and model routing are all out of scope for this reference profile.",
		Rationale:  "UFO documents useful capability-matching and recovery patterns, but its Windows desktop and multi-device execution scope would bypass HAI's current controlled-runtime safety boundary if adopted directly.",
		VerifiedAt: "2026-07-20", VerificationNote: "Official repository reviewed on 2026-07-20: active main branch; UFO2 exposes Windows UIA, Win32, and WinCOM control, while UFO3 adds multi-device orchestration and requires explicit LLM configuration. No UFO process, device agent, screen capture, UI automation, or provider credential is configured by HAI.",
		ControlMappings: []ControlMapping{
			{SourcePattern: "device capability matching and DAG orchestration", HAIControl: "HAI runtime registry and workflow state", Boundary: "HAI does not auto-register devices or dispatch work to a host agent"},
			{SourcePattern: "Windows GUI and native-control automation", HAIControl: "approval-gated controlled runtime", Boundary: "no UIA, Win32, WinCOM, screenshot, or click authority is inherited"},
		},
	},
	{
		ID: "goose", Name: "Goose", UpstreamURL: "https://github.com/aaif-goose/goose", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10106/repos/", SourceCollection: "Coding Agents",
		Status: StatusReferenceOnly, Category: "general-purpose local agent architecture", IntegrationMode: "second-control-plane reference",
		Capabilities: []string{"desktop and CLI agent patterns", "MCP extension patterns", "provider compatibility", "workflow recipes"}, RecommendedFor: []string{"local agent boundary research", "MCP extension review", "provider interoperability comparison"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Do not embed, install, or run Goose from HAI. Its provider, extension, desktop, CLI, API, recipe, filesystem, and execution surfaces are a separate general-purpose agent control plane. Revisit only for a narrow, fixed-schema interoperability case that preserves HAI-owned provider policy, tool allowlists, approvals, audit events, workspace limits, and emergency stop.",
		Rationale:  "Goose is an active extensible local agent with broad provider and MCP support, but its general-purpose execution model overlaps HAI's planner, runtime registry, router, and governance layers. It is valuable as a comparison source, not a runtime dependency.",
		VerifiedAt: "2026-07-20", VerificationNote: "Official repository reviewed on 2026-07-20: active main branch, Apache-2.0, latest v1.43.0 released 2026-07-14; it provides a Windows desktop app, CLI, API, provider connections, and MCP extensions. No Goose binary, extension, provider account, workspace, or API is configured by HAI.",
		ControlMappings: []ControlMapping{
			{SourcePattern: "provider and MCP extension ecosystem", HAIControl: "HAI provider router and reviewed runtime registry", Boundary: "no provider key, extension, or tool trust is inherited"},
			{SourcePattern: "desktop, CLI, API, and workflow execution", HAIControl: "HAI workflow engine and approval queue", Boundary: "a general-purpose upstream agent cannot create a parallel execution path"},
		},
	},
	{
		ID: "minio", Name: "MinIO", UpstreamURL: "https://github.com/minio/minio", SourceCatalogURL: "https://ossinsight.io/collections/distributed-file-storage", SourceCollection: "Distributed File Storage",
		Status: StatusExcluded, Category: "object storage", IntegrationMode: "not adopted",
		Capabilities: []string{"S3-compatible object storage", "local artifact storage"}, RecommendedFor: []string{"storage architecture reference"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Do not add to HAI. Reassess object storage only after an attachment-volume need is measured and a maintained, licence-compatible option is selected.",
		Rationale:  "The upstream repository is archived and AGPLv3, so it does not meet HAI's current maintenance and licensing adoption bar.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight Distributed File Storage listing and upstream archive/licensing status checked on 2026-07-19.",
	},
}

// Entries returns a deep copy so callers cannot mutate the registry.
func Entries() []Entry {
	out := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		entry.Capabilities = append([]string(nil), entry.Capabilities...)
		entry.RecommendedFor = append([]string(nil), entry.RecommendedFor...)
		entry.ControlMappings = append([]ControlMapping(nil), entry.ControlMappings...)
		out = append(out, entry)
	}
	return out
}

func EntryByID(id string) (Entry, bool) {
	for _, entry := range entries {
		if entry.ID == strings.ToLower(strings.TrimSpace(id)) {
			entry.Capabilities = append([]string(nil), entry.Capabilities...)
			entry.RecommendedFor = append([]string(nil), entry.RecommendedFor...)
			entry.ControlMappings = append([]ControlMapping(nil), entry.ControlMappings...)
			return entry, true
		}
	}
	return Entry{}, false
}

// Recommend maps a task to relevant external capabilities. It never marks a
// candidate as configured, selected for execution, or safe to invoke.
func Recommend(taskType, request string) []Recommendation {
	text := strings.ToLower(taskType + " " + request)
	ids := []string{}
	if containsAny(text, "code", "coding", "repository", "repo", "pull request", "test", "build", "bug", "commit") {
		ids = append(ids, "continue", "cline", "opencode", "aider", "openhands", "qodo-pr-agent", "swe-agent")
	}
	if containsAny(text, "serena", "semantic code", "symbol retrieval", "symbolic code", "cross-file impact", "language server diagnostics") {
		ids = append(ids, "serena")
	}
	if containsAny(text, "plan", "research", "workflow", "delegate", "multi-agent", "orchestr") {
		ids = append(ids, "crewai")
	}
	if containsAny(text, "ag2", "ag2 migration", "ag2 workflow") {
		ids = append(ids, "ag2")
	}
	if containsAny(text, "typed plan", "typed output", "structured plan", "structured extraction", "schema first", "plan schema", "pydantic ai", "pydanticai") {
		ids = append(ids, "pydantic-ai")
	}
	if containsAny(text, "sandbox", "isolate", "untrusted code") {
		ids = append(ids, "e2b")
	}
	if containsAny(text, "autogen", "agentchat", "magentic", "mcp workbench", "autogen migration") {
		ids = append(ids, "autogen")
	}
	if containsAny(text, "microsoft agent framework", "agent framework", "autogen successor", "agent framework migration") {
		ids = append(ids, "microsoft-agent-framework")
	}
	if containsAny(text, "provider", "model gateway", "quota", "token cost", "model routing", "litellm") {
		ids = append(ids, "litellm")
	}
	if containsAny(text, "local model", "local inference", "gguf", "llama.cpp", "llama cpp", "offline model", "ollama") {
		ids = append(ids, "ollama", "llama-cpp")
	}
	if containsAny(text, "localai", "local ai", "openai compatible local", "multimodal local model") {
		ids = append(ids, "localai")
	}
	if containsAny(text, "vllm", "high throughput", "batched inference", "serve a model") {
		ids = append(ids, "vllm")
	}
	if containsAny(text, "mistral.rs", "mistral rs", "anthropic compatible local", "local multimodal model", "local multimodal inference") {
		ids = append(ids, "mistral-rs")
	}
	if containsAny(text, "semantic memory", "embedding", "vector search", "pgvector") {
		ids = append(ids, "pgvector")
	}
	if containsAny(text, "ragflow", "document retrieval", "document parsing", "complex pdf", "evidence retrieval", "reranking", "re-ranking") {
		ids = append(ids, "ragflow")
	}
	if containsAny(text, "source inventory", "inventory source", "inventory a source", "inventory sources", "source ingestion", "incremental connector", "cloudquery", "read first connector", "account inventory") {
		ids = append(ids, "cloudquery")
	}
	if containsAny(text, "durable workflow", "scheduled", "follow-up", "follow up", "retry", "temporal") {
		ids = append(ids, "temporal")
	}
	if containsAny(text, "observability", "monitoring", "metrics", "prometheus") {
		ids = append(ids, "prometheus")
	}
	if containsAny(text, "mcp inspect", "mcp health", "mcp server", "mcp inspector") {
		ids = append(ids, "mcp-inspector")
	}
	if containsAny(text, "mcp server", "create a tool server", "publish a tool", "fastmcp", "github mcp", "playwright mcp", "database mcp") {
		ids = append(ids, "fastmcp")
		if containsAny(text, "github mcp", "repository", "repo", "pull request", "issue") {
			ids = append(ids, "github-mcp-server")
		}
		if containsAny(text, "playwright mcp", "browser") {
			ids = append(ids, "playwright-mcp")
		}
		if containsAny(text, "database mcp", "database", "sql", "query") {
			ids = append(ids, "google-genai-toolbox")
		}
	}
	if containsAny(text, "browser verification", "browser test", "browser flow", "web flow", "playwright", "ui regression") {
		ids = append(ids, "playwright", "playwright-mcp")
	}
	if containsAny(text, "browser agent", "browser-use", "browser use", "web research", "browse website") {
		ids = append(ids, "browser-use", "playwright")
	}
	if containsAny(text, "microsoft ufo", "ufo windows", "windows ui automation", "desktop agentos", "multi-device agent") {
		ids = append(ids, "ufo")
	}
	if containsAny(text, "aaif goose", "goose agent", "goose mcp", "goose workflow") {
		ids = append(ids, "goose")
	}
	if containsAny(text, "guardrail", "prompt injection", "llm safety", "red team", "red-team", "jailbreak", "safety evaluation") {
		ids = append(ids, "nemo-guardrails", "garak", "promptfoo")
	}
	if containsAny(text, "deepteam", "red team regression", "agent red team") {
		ids = append(ids, "deepteam")
	}
	if containsAny(text, "pii", "personal data", "sensitive data", "secret redaction", "redact", "redaction", "anonymize", "anonymise", "presidio") {
		ids = append(ids, "presidio")
	}
	if containsAny(text, "schema validation", "structured output validation", "validate structured output", "output validator", "guardrails ai", "guardrails-ai") {
		ids = append(ids, "guardrails-ai")
	}
	if containsAny(text, "evaluate", "evaluation", "quality regression", "retrieval evaluation", "deepeval") {
		ids = append(ids, "deepeval", "evidently")
	}
	if containsAny(text, "opik", "evaluation traces", "experiment comparison") {
		ids = append(ids, "opik")
	}
	if containsAny(text, "model benchmark", "benchmark model", "benchmark a local model", "offline model evaluation", "lm evaluation", "lm-eval", "lm evaluation harness") {
		ids = append(ids, "lm-eval-harness")
	}
	if containsAny(text, "trace instrumentation", "trace telemetry", "open telemetry", "opentelemetry", "openllmetry", "model traces") {
		ids = append(ids, "openllmetry", "openlit", "phoenix")
	}
	if containsAny(text, "voice note", "audio", "transcribe", "transcription", "speech to text", "speech-to-text") {
		ids = append(ids, "whisper-cpp")
	}
	if containsAny(text, "voice pipeline", "multimodal intake", "pipecat", "ambient voice") {
		ids = append(ids, "pipecat", "livekit-agents")
	}
	if containsAny(text, "livekit", "realtime voice", "real-time voice", "voice session", "voice assistant") {
		ids = append(ids, "livekit-agents")
	}
	if containsAny(text, "agent to agent", "agent-to-agent", "a2a protocol", "a2a") {
		ids = append(ids, "a2a")
	}
	if containsAny(text, "tabby", "self-hosted coding assistant", "code completion") {
		ids = append(ids, "tabby")
	}
	if containsAny(text, "openspec", "spec driven", "specification", "acceptance criteria", "change plan") {
		ids = append(ids, "openspec")
	}
	if containsAny(text, "letta", "agent memory", "memory consolidation", "long term memory", "long-term memory", "langmem") {
		ids = append(ids, "letta", "langmem")
	}
	if containsAny(text, "comfyui", "image generation", "generate image") {
		ids = append(ids, "comfyui")
	}
	if containsAny(text, "wasm", "webassembly", "wasi", "bounded helper") {
		ids = append(ids, "wasmtime")
	}
	if containsAny(text, "schedule optimization", "route optimization", "resource assignment", "constraint solver", "or-tools", "ortools") {
		ids = append(ids, "ortools")
	}
	if containsAny(text, "activepieces", "connector platform", "automation platform", "n8n", "mem0", "data lineage", "openmetadata", "open metadata") {
		if containsAny(text, "activepieces", "connector platform", "automation platform") {
			ids = append(ids, "activepieces")
		}
		if containsAny(text, "n8n") {
			ids = append(ids, "n8n")
		}
		if containsAny(text, "mem0") {
			ids = append(ids, "mem0")
		}
		if containsAny(text, "data lineage", "openmetadata", "open metadata") {
			ids = append(ids, "openmetadata")
		}
	}
	if containsAny(text, "daytona", "managed sandbox", "workspace sandbox") {
		ids = append(ids, "daytona")
	}
	if containsAny(text, "graphrag", "knowledge graph", "entity linking", "langchain", "llamaindex", "llama index", "cognee", "haystack", "document pipeline", "anythingllm", "anything llm", "rag workspace", "document workspace") {
		ids = append(ids, "langchain", "llamaindex", "cognee", "graphrag", "haystack")
		if containsAny(text, "anythingllm", "anything llm", "rag workspace", "document workspace") {
			ids = append(ids, "anythingllm")
		}
	}
	if containsAny(text, "qdrant", "dedicated vector database") {
		ids = append(ids, "qdrant")
	}
	seen := map[string]bool{}
	out := []Recommendation{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		entry, ok := EntryByID(id)
		if !ok {
			continue
		}
		role := "optional capability"
		if entry.Status == StatusCompatibility {
			role = "legacy compatibility only"
		} else if entry.Status == StatusIntegrated {
			role = "integrated profile; operator configuration and live probe required"
		} else if entry.Status != StatusCandidate {
			role = "reference or review only"
		}
		out = append(out, Recommendation{
			ID: id, Name: entry.Name, Status: entry.Status, Role: role,
			Rationale: entry.Rationale, RequiresApproval: entry.RequiresApproval, Activation: entry.Activation,
			ControlMappings: append([]ControlMapping(nil), entry.ControlMappings...),
		})
	}
	return out
}

func containsAny(text string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(text, value) {
			return true
		}
	}
	return false
}
