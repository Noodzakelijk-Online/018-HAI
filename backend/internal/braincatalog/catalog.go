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
	ID            string `json:"id"`
	Name          string `json:"name"`
	UpstreamURL   string `json:"upstreamUrl"`
	CheckedAt     string `json:"checkedAt"`
	Available     bool   `json:"available"`
	Archived      bool   `json:"archived"`
	License       string `json:"license,omitempty"`
	DefaultBranch string `json:"defaultBranch,omitempty"`
	PushedAt      string `json:"pushedAt,omitempty"`
	Message       string `json:"message"`
	Disposition   Status `json:"disposition"`
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
		Status: StatusCandidate, Category: "local speech transcription", IntegrationMode: "operator-configured local intake adapter",
		Capabilities: []string{"offline transcription", "audio-to-text extraction", "local model execution"}, RecommendedFor: []string{"voice-note intake", "meeting evidence", "accessibility transcription"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Review local audio folders, consent, retention, model provenance, language handling, transcript confidence, and source links. HAI stores transcripts only through its existing source and memory verification path.",
		Rationale:  "Local speech-to-text can broaden safe intake without transmitting audio to a cloud service, but it requires explicit consent and evidence-quality controls.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight Multimodal AI repository list checked on 2026-07-19; no whisper.cpp binary or audio connector is configured by HAI.",
	},
	{
		ID: "a2a", Name: "A2A Protocol", UpstreamURL: "https://github.com/a2aproject/A2A", SourceCatalogURL: "https://api.ossinsight.io/v1/collections/10139/repos/", SourceCollection: "A2A Protocol",
		Status: StatusCompatibility, Category: "agent interoperability", IntegrationMode: "reviewed protocol bridge",
		Capabilities: []string{"agent task envelopes", "capability discovery patterns", "interoperability"}, RecommendedFor: []string{"external agent review", "protocol translation", "multi-agent interoperability"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Implement only a narrow local bridge with a fixed task schema, authenticated named peers, no implicit capability negotiation, and HAI-owned tool, budget, approval, and audit decisions.",
		Rationale:  "A2A can inform interoperability, but it must not become an unbounded remote-agent trust channel or a replacement for HAI's controlled runtime registry.",
		VerifiedAt: verifiedAt, VerificationNote: "OSS Insight A2A Protocol repository list checked on 2026-07-19; no A2A peer or transport is configured by HAI.",
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
		Status: StatusCandidate, Category: "self-hosted LLM observability", IntegrationMode: "operator-hosted trace and evaluation service adapter",
		Capabilities: []string{"LLM traces", "prompt versions", "evaluations", "datasets"}, RecommendedFor: []string{"model routing audit", "prompt evaluation", "agent trace review"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Host Langfuse locally and complete an adapter review covering trace redaction, retention, service credentials, data-egress controls, and health checks. HAI will not export prompts or task data by catalog entry.",
		Rationale:  "A self-hostable observability and evaluation candidate that can make HAI's model and agent decisions more inspectable without replacing the existing audit ledger.",
		VerifiedAt: verifiedAt, VerificationNote: "Upstream repository, self-hosting path, and LLM trace/evaluation capabilities checked on 2026-07-19.",
	},
	{
		ID: "promptfoo", Name: "Promptfoo", UpstreamURL: "https://github.com/promptfoo/promptfoo", SourceCatalogURL: "https://ossinsight.io/collections/llm-devtools", SourceCollection: "LLM DevTools",
		Status: StatusCandidate, Category: "LLM evaluation", IntegrationMode: "operator-configured local CLI or contained evaluation service",
		Capabilities: []string{"prompt regression testing", "provider comparison", "red-team evaluation"}, RecommendedFor: []string{"LLM routing validation", "prompt regression checks", "evaluation design"},
		RequiresApproval: true, LocalFirstCompatible: true,
		Activation: "Complete a review for provider credentials, test-data redaction, workspace confinement, network access, artifact retention, and no-write validation before HAI can invoke any evaluation run.",
		Rationale:  "A useful evaluation candidate for verifying routing and prompt changes, but test inputs can contain sensitive context and must not bypass HAI's source and secret controls.",
		VerifiedAt: verifiedAt, VerificationNote: "Catalog candidate recorded for review on 2026-07-19; no Promptfoo runtime is configured by HAI.",
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
		ids = append(ids, "continue", "cline", "opencode", "aider", "openhands")
	}
	if containsAny(text, "plan", "research", "workflow", "delegate", "multi-agent", "orchestr") {
		ids = append(ids, "crewai")
	}
	if containsAny(text, "sandbox", "isolate", "untrusted code") {
		ids = append(ids, "e2b")
	}
	if containsAny(text, "autogen", "agentchat", "magentic", "mcp workbench", "autogen migration") {
		ids = append(ids, "autogen")
	}
	if containsAny(text, "provider", "model gateway", "quota", "token cost", "model routing", "litellm") {
		ids = append(ids, "litellm")
	}
	if containsAny(text, "local model", "local inference", "gguf", "llama.cpp", "llama cpp", "offline model", "ollama") {
		ids = append(ids, "ollama", "llama-cpp")
	}
	if containsAny(text, "semantic memory", "embedding", "vector search", "pgvector") {
		ids = append(ids, "pgvector")
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
	if containsAny(text, "browser verification", "browser test", "browser flow", "web flow", "playwright", "ui regression") {
		ids = append(ids, "playwright")
	}
	if containsAny(text, "browser agent", "browser-use", "browser use", "web research", "browse website") {
		ids = append(ids, "browser-use", "playwright")
	}
	if containsAny(text, "guardrail", "prompt injection", "llm safety", "red team", "red-team", "jailbreak", "safety evaluation") {
		ids = append(ids, "nemo-guardrails", "garak", "promptfoo")
	}
	if containsAny(text, "voice note", "audio", "transcribe", "transcription", "speech to text", "speech-to-text") {
		ids = append(ids, "whisper-cpp")
	}
	if containsAny(text, "agent to agent", "agent-to-agent", "a2a protocol", "a2a") {
		ids = append(ids, "a2a")
	}
	if containsAny(text, "tabby", "self-hosted coding assistant", "code completion") {
		ids = append(ids, "tabby")
	}
	if containsAny(text, "letta", "agent memory") {
		ids = append(ids, "letta")
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
	if containsAny(text, "graphrag", "knowledge graph", "entity linking", "langchain", "llamaindex", "llama index", "cognee") {
		ids = append(ids, "langchain", "llamaindex", "cognee")
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
