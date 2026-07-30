package braincatalog

import (
	"strings"
	"testing"
)

func TestEntriesAreTransparentAndDisabledByPolicy(t *testing.T) {
	entries := Entries()
	if len(entries) < 8 {
		t.Fatalf("entries = %d, want curated catalog", len(entries))
	}
	for _, entry := range entries {
		if entry.UpstreamURL == "" || entry.SourceCatalogURL == "" || entry.VerifiedAt == "" || entry.Activation == "" {
			t.Fatalf("entry lacks provenance or activation boundary: %#v", entry)
		}
	}
	if entry, ok := EntryByID("autogen"); !ok || entry.Status != StatusCompatibility || !entry.RequiresApproval || len(entry.ControlMappings) == 0 || !strings.Contains(entry.IntegrationMode, "migration-preview") {
		t.Fatalf("AutoGen must remain a gated compatibility profile: %#v", entry)
	}
	if entry, ok := EntryByID("autogpt"); !ok || entry.Status != StatusLicenseReview {
		t.Fatalf("AutoGPT must require license review: %#v", entry)
	}
	if entry, ok := EntryByID("opencode-ai-legacy"); !ok || entry.Status != StatusExcluded || !entry.RequiresApproval {
		t.Fatalf("the archived opencode-ai project must remain excluded: %#v", entry)
	}
	if entry, ok := EntryByID("continue"); !ok || entry.Status != StatusExcluded || !entry.RequiresApproval || !strings.Contains(entry.VerificationNote, "no longer actively maintained") {
		t.Fatalf("the discontinued Continue project must remain excluded: %#v", entry)
	}
	if entry, ok := EntryByID("microsoft-jarvis"); !ok || entry.Status != StatusExcluded || !entry.RequiresApproval || !strings.Contains(entry.VerificationNote, "text-davinci-003") {
		t.Fatalf("the legacy JARVIS prototype must remain excluded: %#v", entry)
	}
	for _, id := range []string{"openllmetry", "browser-use", "tabby", "playwright-mcp", "opik", "pipecat", "livekit-agents"} {
		if entry, ok := EntryByID(id); !ok || entry.Status != StatusCandidate || !entry.RequiresApproval {
			t.Fatalf("%s must remain a review-first candidate: %#v", id, entry)
		}
	}
	for _, id := range []string{"openlit", "opik", "pipecat", "livekit-agents"} {
		entry, ok := EntryByID(id)
		if !ok || entry.VerifiedAt != "2026-07-21" || !strings.Contains(entry.VerificationNote, "same-day upstream push") {
			t.Fatalf("%s must retain its current upstream review evidence: %#v", id, entry)
		}
	}
	if entry, ok := EntryByID("google-genai-toolbox"); !ok || entry.Status != StatusIntegrated || !entry.RequiresApproval || !entry.LocalFirstCompatible || entry.Name != "MCP Toolbox" || entry.UpstreamURL != "https://github.com/googleapis/mcp-toolbox" || len(entry.RepositoryAliases) != 1 || entry.RepositoryAliases[0] != "googleapis/genai-toolbox" {
		t.Fatalf("MCP Toolbox rename must retain the stable profile and historic repository alias: %#v", entry)
	}
	if entry, ok := EntryByID("nemo-guardrails"); !ok || entry.Status != StatusLicenseReview || !entry.RequiresApproval {
		t.Fatalf("NeMo Guardrails must remain held for licence review: %#v", entry)
	}
	if entry, ok := EntryByID("airbyte"); !ok || entry.Status != StatusIntegrated || !entry.LocalFirstCompatible || !entry.RequiresApproval {
		t.Fatalf("Airbyte must remain a guarded local-first integration: %#v", entry)
	}
	for _, id := range []string{"presidio", "guardrails-ai", "lm-eval-harness", "promptfoo", "deepeval", "deepteam", "garak", "gitleaks", "gosec", "trivy", "grype", "syft", "evidently", "ragflow", "anythingllm", "serena", "mlflow", "odoo", "cloudquery", "openspec", "claude-code-project-instructions", "fabric-patterns", "mini-swe-agent", "openlit"} {
		if entry, ok := EntryByID(id); !ok || entry.Status != StatusIntegrated || !entry.RequiresApproval || !entry.LocalFirstCompatible {
			t.Fatalf("%s must expose its implemented local profile without claiming that it is configured: %#v", id, entry)
		}
	}
	if entry, ok := EntryByID("langfuse"); !ok || entry.Status != StatusIntegrated || !entry.RequiresApproval || !entry.LocalFirstCompatible {
		t.Fatalf("Langfuse must expose only its local, approval-aware bridge: %#v", entry)
	}
	if entry, ok := EntryByID("openlit"); !ok || !strings.Contains(entry.IntegrationMode, "aggregate OTLP") || len(entry.ControlMappings) != 2 {
		t.Fatalf("OpenLIT must expose only its local aggregate OTLP bridge: %#v", entry)
	}
	if entry, ok := EntryByID("ag2"); !ok || entry.Status != StatusCompatibility || !entry.RequiresApproval {
		t.Fatalf("AG2 must remain a gated compatibility profile: %#v", entry)
	}
	for _, id := range []string{"ufo", "goose"} {
		if entry, ok := EntryByID(id); !ok || entry.Status != StatusReferenceOnly || !entry.RequiresApproval || len(entry.ControlMappings) == 0 {
			t.Fatalf("%s must remain a non-active high-risk reference: %#v", id, entry)
		}
	}
	for _, id := range []string{"agno", "voltagent", "openai-agents-python", "langroid", "camel"} {
		if entry, ok := EntryByID(id); !ok || entry.Status != StatusReferenceOnly || !entry.RequiresApproval || len(entry.ControlMappings) == 0 {
			t.Fatalf("%s must remain an explicit non-active agent-framework reference: %#v", id, entry)
		}
	}
	if entry, ok := EntryByID("mastra"); !ok || entry.Status != StatusLicenseReview || !entry.RequiresApproval || len(entry.ControlMappings) == 0 {
		t.Fatalf("Mastra must remain held for licence review: %#v", entry)
	}
	if entry, ok := EntryByID("localai"); !ok || entry.Status != StatusIntegrated || !entry.RequiresApproval {
		t.Fatalf("LocalAI must report its integrated, approval-gated local provider profile: %#v", entry)
	}
	if entry, ok := EntryByID("vllm"); !ok || entry.Status != StatusIntegrated || !entry.RequiresApproval {
		t.Fatalf("vLLM must report its integrated, approval-gated local provider profile: %#v", entry)
	}
	if entry, ok := EntryByID("sglang"); !ok || entry.Status != StatusIntegrated || !entry.LocalFirstCompatible || !entry.RequiresApproval {
		t.Fatalf("SGLang must report its integrated, approval-gated local provider profile: %#v", entry)
	}
	if entry, ok := EntryByID("mistral-rs"); !ok || entry.Status != StatusIntegrated || !entry.LocalFirstCompatible || !entry.RequiresApproval {
		t.Fatalf("mistral.rs must report its integrated, approval-gated local provider profile: %#v", entry)
	}
	if entry, ok := EntryByID("whisper-cpp"); !ok || entry.Status != StatusIntegrated || !entry.LocalFirstCompatible || !entry.RequiresApproval {
		t.Fatalf("whisper.cpp must report its integrated, approval-gated local source profile: %#v", entry)
	}
	if entry, ok := EntryByID("docling"); !ok || entry.Status != StatusIntegrated || !entry.LocalFirstCompatible || !entry.RequiresApproval || entry.Implementation == nil {
		t.Fatalf("Docling must report its integrated, approval-gated local document profile: %#v", entry)
	}
	if entry, ok := EntryByID("pydantic-ai"); !ok || entry.Status != StatusIntegrated || !entry.LocalFirstCompatible || !entry.RequiresApproval {
		t.Fatalf("PydanticAI must report its integrated, approval-gated local typed-planning profile: %#v", entry)
	}
	if entry, ok := EntryByID("fastmcp"); !ok || entry.Status != StatusIntegrated || !entry.LocalFirstCompatible || !entry.RequiresApproval {
		t.Fatalf("FastMCP must report its integrated, approval-gated local read-only bridge: %#v", entry)
	}
	if entry, ok := EntryByID("a2a"); !ok || entry.Status != StatusIntegrated || !entry.LocalFirstCompatible || !entry.RequiresApproval {
		t.Fatalf("A2A must report its integrated, approval-gated local planning bridge: %#v", entry)
	}
}

func TestRecommendIncludesProjectInstructionsForRepositoryGuidance(t *testing.T) {
	recommendations := Recommend("planning", "Review this repository's AGENTS.md and CLAUDE.md project guidance before creating a bounded plan")
	for _, recommendation := range recommendations {
		if recommendation.ID == "claude-code-project-instructions" && recommendation.Status == StatusIntegrated && recommendation.RequiresApproval {
			return
		}
	}
	t.Fatalf("project-instructions profile was not recommended: %#v", recommendations)
}

func TestRecommendIncludesFabricPatternsForExplicitPromptPatternReview(t *testing.T) {
	recommendations := Recommend("planning", "Review a Fabric prompt pattern from the local pattern library before drafting")
	for _, recommendation := range recommendations {
		if recommendation.ID == "fabric-patterns" && recommendation.Status == StatusIntegrated && recommendation.RequiresApproval {
			return
		}
	}
	t.Fatalf("Fabric pattern profile was not recommended: %#v", recommendations)
}

func TestRecommendIncludesBoundedGitleaksProfileForSecretScanning(t *testing.T) {
	recommendations := Recommend("repository safety", "Run a Gitleaks secret scan against a reviewed repository snapshot")
	for _, recommendation := range recommendations {
		if recommendation.ID == "gitleaks" && recommendation.Status == StatusIntegrated && recommendation.RequiresApproval {
			return
		}
	}
	t.Fatalf("bounded Gitleaks profile was not recommended: %#v", recommendations)
}

func TestRecommendIncludesBoundedGosecProfileForGoSourceSafety(t *testing.T) {
	recommendations := Recommend("repository safety", "Run a Gosec Go static security scan against a reviewed vendored repository snapshot")
	for _, recommendation := range recommendations {
		if recommendation.ID == "gosec" && recommendation.Status == StatusIntegrated && recommendation.RequiresApproval {
			return
		}
	}
	t.Fatalf("bounded Gosec profile was not recommended: %#v", recommendations)
}

func TestRecommendIncludesBoundedTrivyProfileForConfigurationSafety(t *testing.T) {
	recommendations := Recommend("repository safety", "Run an offline Trivy configuration scan for Docker Compose and Terraform without changing infrastructure")
	for _, recommendation := range recommendations {
		if recommendation.ID == "trivy" && recommendation.Status == StatusIntegrated && recommendation.RequiresApproval {
			return
		}
	}
	t.Fatalf("bounded Trivy profile was not recommended: %#v", recommendations)
}

func TestRecommendIncludesBoundedSyftProfileForSoftwareInventory(t *testing.T) {
	recommendations := Recommend("repository safety", "Create an SBOM dependency inventory for a reviewed repository snapshot")
	for _, recommendation := range recommendations {
		if recommendation.ID == "syft" && recommendation.Status == StatusIntegrated && recommendation.RequiresApproval {
			return
		}
	}
	t.Fatalf("bounded Syft profile was not recommended: %#v", recommendations)
}

func TestRecommendIncludesBoundedGrypeProfileForVulnerabilityEvidence(t *testing.T) {
	recommendations := Recommend("repository safety", "Review aggregate vulnerability severity for a reviewed repository snapshot without changing dependencies")
	for _, recommendation := range recommendations {
		if recommendation.ID == "grype" && recommendation.Status == StatusIntegrated && recommendation.RequiresApproval {
			return
		}
	}
	t.Fatalf("bounded Grype profile was not recommended: %#v", recommendations)
}

func TestRecommendKeepsOmegaMemoryAsAReferenceOnlyMemoryOption(t *testing.T) {
	recommendations := Recommend("memory", "Review local cross model memory consolidation without adding a second store")
	for _, recommendation := range recommendations {
		if recommendation.ID == "omega-memory" && recommendation.Status == StatusReferenceOnly && recommendation.RequiresApproval {
			return
		}
	}
	t.Fatalf("Omega Memory reference was not recommended: %#v", recommendations)
}

func TestOSSInsightCollectionScreeningCoversEveryCollection(t *testing.T) {
	screening := OSSInsightScreening()
	if screening.Total != 138 || len(screening.Entries) != 138 {
		t.Fatalf("screening coverage = %d/%d, want 138 from the OSS Insight public API snapshot", screening.Total, len(screening.Entries))
	}
	seen := map[string]bool{}
	for _, entry := range screening.Entries {
		if entry.Collection == "" || entry.SourceURL == "" || entry.Rationale == "" || seen[entry.Collection] {
			t.Fatalf("invalid or duplicate collection screen: %#v", entry)
		}
		for _, id := range entry.RelatedEntryIDs {
			if _, ok := EntryByID(id); !ok {
				t.Fatalf("collection %q references catalog profile %q that is not registered", entry.Collection, id)
			}
		}
		seen[entry.Collection] = true
	}
	for _, collection := range []string{"ai-gateways", "Data Integration", "Business Management", "Search Engine", "WebAssembly Runtime", "MCP Servers", "LLM Inference Engines", "AI Agent Memory", "AI Red Teaming", "Agent Harness"} {
		if !seen[collection] {
			t.Fatalf("missing required collection screen: %s", collection)
		}
	}
	if screening.Represented == 0 || screening.Candidates == 0 || screening.Deferred == 0 {
		t.Fatalf("screening summary must make every outcome visible: %#v", screening)
	}
	businessManagementFound := false
	for _, entry := range screening.Entries {
		if entry.Collection != "Business Management" {
			continue
		}
		businessManagementFound = true
		if entry.Disposition != CollectionRepresented {
			t.Fatalf("Business Management must reflect the integrated read-only Odoo adapter: %#v", entry)
		}
	}
	if !businessManagementFound {
		t.Fatal("Business Management must remain in the OSS Insight screening")
	}
}

func TestOSSInsightCandidatesHaveLocalActivationBoundaries(t *testing.T) {
	if entry, ok := EntryByID("wasmtime"); !ok || entry.Status != StatusIntegrated || !entry.LocalFirstCompatible || !entry.RequiresApproval {
		t.Fatalf("Wasmtime must report its integrated-but-approval-gated local WASI profile: %#v", entry)
	}
	if entry, ok := EntryByID("playwright"); !ok || entry.Status != StatusIntegrated || !entry.LocalFirstCompatible || !entry.RequiresApproval {
		t.Fatalf("Playwright must report its integrated-but-approval-gated local verification profile: %#v", entry)
	}
	if entry, ok := EntryByID("temporal"); !ok || entry.Status != StatusIntegrated || !entry.LocalFirstCompatible || !entry.RequiresApproval {
		t.Fatalf("Temporal must report its integrated-but-approval-gated local durability profile: %#v", entry)
	}
	if entry, ok := EntryByID("pgvector"); !ok || entry.Status != StatusIntegrated || !entry.LocalFirstCompatible || !entry.RequiresApproval {
		t.Fatalf("pgvector must report its integrated-but-opt-in local retrieval profile: %#v", entry)
	}
	if entry, ok := EntryByID("mcp-inspector"); !ok || entry.Status != StatusIntegrated || !entry.LocalFirstCompatible || !entry.RequiresApproval {
		t.Fatalf("MCP Inspector must report the integrated-but-admin-gated local preflight profile: %#v", entry)
	}
	if entry, ok := EntryByID("llama-cpp"); !ok || entry.Status != StatusIntegrated || !entry.LocalFirstCompatible || !entry.RequiresApproval {
		t.Fatalf("llama.cpp must report its integrated-but-not-active local provider profile: %#v", entry)
	}
	if entry, ok := EntryByID("ollama"); !ok || entry.Status != StatusIntegrated || !entry.LocalFirstCompatible || !entry.RequiresApproval {
		t.Fatalf("Ollama must report HAI's implemented local provider profile: %#v", entry)
	}
	if entry, ok := EntryByID("prometheus"); !ok || entry.Status != StatusIntegrated || !entry.LocalFirstCompatible {
		t.Fatalf("Prometheus must report its integrated-but-opt-in metrics profile: %#v", entry)
	}
	if entry, ok := EntryByID("litellm"); !ok || entry.Status != StatusIntegrated || !entry.LocalFirstCompatible || !entry.RequiresApproval {
		t.Fatalf("LiteLLM must report its integrated-but-approval-gated local gateway profile: %#v", entry)
	}
	if entry, ok := EntryByID("ortools"); !ok || entry.Status != StatusIntegrated || !entry.LocalFirstCompatible || entry.RequiresApproval {
		t.Fatalf("OR-Tools must report its integrated proposal-only local planning profile: %#v", entry)
	}
	if entry, ok := EntryByID("qdrant"); !ok || entry.Status != StatusReferenceOnly {
		t.Fatalf("Qdrant must not create a second active vector store by default: %#v", entry)
	}
	for _, id := range []string{"activepieces", "agentops", "dagster", "mem0", "letta", "omega-memory", "comfyui", "openmetadata", "prefect", "promptflow"} {
		if entry, ok := EntryByID(id); !ok || entry.Status != StatusReferenceOnly {
			t.Fatalf("%s must remain a reference rather than a parallel control plane: %#v", id, entry)
		}
	}
	if entry, ok := EntryByID("daytona"); !ok || entry.Status != StatusExcluded {
		t.Fatalf("Daytona must remain excluded after its public upstream became unmaintained: %#v", entry)
	}
	if entry, ok := EntryByID("n8n"); !ok || entry.Status != StatusLicenseReview {
		t.Fatalf("n8n must remain under license review: %#v", entry)
	}
	if entry, ok := EntryByID("minio"); !ok || entry.Status != StatusExcluded {
		t.Fatalf("archived MinIO must remain excluded: %#v", entry)
	}
	if entry, ok := EntryByID("llm-guard"); !ok || entry.Status != StatusExcluded {
		t.Fatalf("archived LLM Guard must remain excluded: %#v", entry)
	}
	for _, id := range []string{"openai-evals", "omniparser", "mcp-servers"} {
		if entry, ok := EntryByID(id); !ok || entry.Status != StatusLicenseReview {
			t.Fatalf("%s must remain held for licence review: %#v", id, entry)
		}
	}
	if entry, ok := EntryByID("qodo-pr-agent"); !ok || entry.Status != StatusReferenceOnly || !strings.Contains(entry.VerificationNote, "LICENSE is MIT") {
		t.Fatalf("Qodo PR-Agent must remain a current but non-integrated review reference: %#v", entry)
	}
	if entry, ok := EntryByID("swe-rex"); !ok || entry.Status != StatusReferenceOnly || !strings.Contains(entry.Activation, "Do not install") {
		t.Fatalf("SWE-ReX must remain a non-integrated sandbox reference: %#v", entry)
	}
	if entry, ok := EntryByID("agentbench"); !ok || entry.Status != StatusReferenceOnly {
		t.Fatalf("AgentBench must remain a reference-only evaluation pattern: %#v", entry)
	}
	if entry, ok := EntryByID("swe-agent"); !ok || entry.Status != StatusReferenceOnly || !entry.RequiresApproval {
		t.Fatalf("SWE-agent must remain a superseded architecture reference: %#v", entry)
	}
	if entry, ok := EntryByID("swe-bench"); !ok || entry.Status != StatusExcluded || entry.LocalFirstCompatible || !entry.RequiresApproval || len(entry.ControlMappings) != 2 {
		t.Fatalf("SWE-bench must remain a capacity-gated benchmark exclusion: %#v", entry)
	}
	if entry, ok := EntryByID("whylogs"); !ok || entry.Status != StatusReferenceOnly || !entry.RequiresApproval || !entry.LocalFirstCompatible || len(entry.ControlMappings) != 2 {
		t.Fatalf("Whylogs must remain a local-only freshness-held profiling reference: %#v", entry)
	}
	if sources := DiscoverySources(); len(sources) < 2 || sources[1].Name != "OSS Insight" {
		t.Fatalf("OSS Insight source is missing: %#v", sources)
	}
}

func TestCapabilityPlaneCoverageClassifiesEveryCatalogEntry(t *testing.T) {
	coverage := CapabilityPlaneCoverageReport()
	if len(coverage) != len(capabilityPlaneOrder) {
		t.Fatalf("planes = %d, want %d", len(coverage), len(capabilityPlaneOrder))
	}
	classified := map[string]bool{}
	for _, plane := range coverage {
		if plane.Name == "" || plane.Description == "" || len(plane.Entries) == 0 {
			t.Fatalf("plane lacks transparent coverage: %#v", plane)
		}
		seen := map[string]bool{}
		for _, entry := range plane.Entries {
			if seen[entry.ID] {
				t.Fatalf("plane %s repeats %s", plane.Plane, entry.ID)
			}
			if _, ok := EntryByID(entry.ID); !ok {
				t.Fatalf("plane %s references missing catalog entry %s", plane.Plane, entry.ID)
			}
			seen[entry.ID] = true
			classified[entry.ID] = true
		}
	}
	for _, entry := range Entries() {
		if !classified[entry.ID] {
			t.Fatalf("catalog entry %s has no capability-plane classification", entry.ID)
		}
	}
}

func TestRecommendAutoGenCompatibilityIsExplicitAndGated(t *testing.T) {
	recommendations := Recommend("migration", "Migrate an AutoGen AgentChat workflow with MCP Workbench")
	for _, recommendation := range recommendations {
		if recommendation.ID != "autogen" {
			continue
		}
		if recommendation.Status != StatusCompatibility || recommendation.Role != "legacy compatibility only" || !recommendation.RequiresApproval {
			t.Fatalf("AutoGen recommendation is not safely gated: %#v", recommendation)
		}
		if len(recommendation.ControlMappings) < 4 {
			t.Fatalf("AutoGen must explain its HAI control mappings: %#v", recommendation)
		}
		return
	}
	t.Fatalf("AutoGen migration should surface the compatibility profile: %#v", recommendations)
}

func TestRecommendCodingDoesNotSelectAutoGenByDefault(t *testing.T) {
	for _, recommendation := range Recommend("coding", "Fix this repository and run tests") {
		if recommendation.ID == "autogen" {
			t.Fatalf("generic coding must not default to legacy AutoGen: %#v", recommendation)
		}
	}
}

func TestRecommendOperationalCapabilitiesPreservesReviewGates(t *testing.T) {
	recommendations := Recommend("operations", "Set up a durable follow-up retry workflow with metrics and a local model gateway")
	ids := map[string]Recommendation{}
	for _, recommendation := range recommendations {
		ids[recommendation.ID] = recommendation
	}
	if recommendation, ok := ids["temporal"]; !ok || recommendation.Status != StatusIntegrated || recommendation.Role != "integrated profile; operator configuration and live probe required" || !recommendation.RequiresApproval {
		t.Fatalf("Temporal must surface as an integrated profile with its durability gate: %#v", recommendations)
	}
	if recommendation, ok := ids["litellm"]; !ok || recommendation.Status != StatusIntegrated || recommendation.Role != "integrated profile; operator configuration and live probe required" || !recommendation.RequiresApproval {
		t.Fatalf("LiteLLM must surface as an integrated profile with its approval gate: %#v", recommendations)
	}
	if recommendation, ok := ids["prometheus"]; !ok || recommendation.Status != StatusIntegrated || recommendation.Role != "integrated profile; operator configuration and live probe required" {
		t.Fatalf("Prometheus must surface as an integrated profile with collector configuration still required: %#v", recommendations)
	}
}

func TestRecommendNewCandidatesNeverClaimsExecution(t *testing.T) {
	recommendations := Recommend("operations", "Use a local GGUF model, verify a browser flow, run a WASI helper, and create a route optimization proposal")
	ids := map[string]Recommendation{}
	for _, recommendation := range recommendations {
		ids[recommendation.ID] = recommendation
	}
	if recommendation, ok := ids["wasmtime"]; !ok || recommendation.Status != StatusIntegrated || recommendation.Role != "integrated profile; operator configuration and live probe required" || !recommendation.RequiresApproval {
		t.Fatalf("Wasmtime must surface as an integrated approval-gated profile: %#v", recommendations)
	}
	if recommendation, ok := ids["playwright"]; !ok || recommendation.Status != StatusIntegrated || recommendation.Role != "integrated profile; operator configuration and live probe required" || !recommendation.RequiresApproval {
		t.Fatalf("Playwright must surface as an integrated approval-gated verification profile: %#v", recommendations)
	}
	if recommendation, ok := ids["ortools"]; !ok || recommendation.Status != StatusIntegrated || recommendation.Role != "integrated profile; operator configuration and live probe required" {
		t.Fatalf("OR-Tools must surface as an integrated proposal-only profile: %#v", recommendations)
	}
	if recommendation, ok := ids["llama-cpp"]; !ok || recommendation.Status != StatusIntegrated || recommendation.Role != "integrated profile; operator configuration and live probe required" || !recommendation.RequiresApproval {
		t.Fatalf("llama.cpp must surface as an integrated profile with its activation gate: %#v", recommendations)
	}
	if !ids["playwright"].RequiresApproval || !ids["wasmtime"].RequiresApproval {
		t.Fatalf("local inference configuration and executable adapters must remain gated: %#v", recommendations)
	}
}

func TestRecommendPlatformReferencesStayBlocked(t *testing.T) {
	recommendations := Recommend("operations", "Compare Activepieces, n8n, Mem0, and OpenMetadata for an automation platform and data lineage")
	ids := map[string]Recommendation{}
	for _, recommendation := range recommendations {
		ids[recommendation.ID] = recommendation
	}
	for _, id := range []string{"activepieces", "mem0", "openmetadata"} {
		if recommendation, ok := ids[id]; !ok || recommendation.Status != StatusReferenceOnly || recommendation.Role != "reference or review only" {
			t.Fatalf("%s must not be recommended as an active tool: %#v", id, recommendations)
		}
	}
	if recommendation, ok := ids["n8n"]; !ok || recommendation.Status != StatusLicenseReview || recommendation.Role != "reference or review only" {
		t.Fatalf("n8n must remain a license-review recommendation: %#v", recommendations)
	}
}

func TestRecommendCodingNeverClaimsExecution(t *testing.T) {
	recommendations := Recommend("coding", "Review this repository and run tests")
	ids := map[string]Recommendation{}
	for _, recommendation := range recommendations {
		ids[recommendation.ID] = recommendation
	}
	for _, id := range []string{"cline", "opencode", "aider", "openhands"} {
		if _, ok := ids[id]; !ok {
			t.Fatalf("missing %s recommendation: %#v", id, recommendations)
		}
	}
	for _, id := range []string{"continue", "microsoft-jarvis"} {
		if _, ok := ids[id]; ok {
			t.Fatalf("excluded %s must not be recommended: %#v", id, recommendations)
		}
	}
	if !ids["cline"].RequiresApproval || !ids["opencode"].RequiresApproval || !ids["aider"].RequiresApproval || !ids["openhands"].RequiresApproval {
		t.Fatalf("write-capable agent candidates must require approval: %#v", recommendations)
	}
}

func TestRecommendExternalSandboxIsReferenceOnly(t *testing.T) {
	recommendations := Recommend("execution", "Run untrusted code in a sandbox")
	for _, recommendation := range recommendations {
		if recommendation.ID == "e2b" {
			if recommendation.Status != StatusReferenceOnly || recommendation.Role != "reference or review only" {
				t.Fatalf("E2B must only surface as a reference: %#v", recommendation)
			}
			return
		}
	}
	t.Fatalf("sandbox work must surface E2B as a reference: %#v", recommendations)
}

func TestRecommendNewOSSInsightCapabilitiesStayGoverned(t *testing.T) {
	recommendations := Recommend("operations", "Use Ollama for a local model, transcribe a voice note, extract document evidence, run a browser agent safety evaluation, and exchange an A2A task envelope")
	ids := map[string]Recommendation{}
	for _, recommendation := range recommendations {
		ids[recommendation.ID] = recommendation
	}
	if recommendation, ok := ids["ollama"]; !ok || recommendation.Status != StatusIntegrated || !recommendation.RequiresApproval {
		t.Fatalf("Ollama must surface as HAI's existing approval-gated local provider profile: %#v", recommendations)
	}
	if recommendation, ok := ids["browser-use"]; !ok || recommendation.Status != StatusCandidate || !recommendation.RequiresApproval {
		t.Fatalf("browser-use must remain a review-first candidate: %#v", recommendations)
	}
	if recommendation, ok := ids["nemo-guardrails"]; !ok || recommendation.Status != StatusLicenseReview || !recommendation.RequiresApproval {
		t.Fatalf("nemo guardrails must remain under licence review: %#v", recommendations)
	}
	if recommendation, ok := ids["garak"]; !ok || recommendation.Status != StatusIntegrated || !recommendation.RequiresApproval || recommendation.Role != "integrated profile; operator configuration and live probe required" {
		t.Fatalf("Garak must surface as a configuration-gated local safety profile: %#v", recommendations)
	}
	if recommendation, ok := ids["whisper-cpp"]; !ok || recommendation.Status != StatusIntegrated || !recommendation.RequiresApproval {
		t.Fatalf("whisper.cpp must surface as HAI's approval-gated local transcription profile: %#v", recommendations)
	}
	if recommendation, ok := ids["docling"]; !ok || recommendation.Status != StatusIntegrated || !recommendation.RequiresApproval {
		t.Fatalf("Docling must surface as HAI's approval-gated local document profile: %#v", recommendations)
	}
	if recommendation, ok := ids["a2a"]; !ok || recommendation.Status != StatusIntegrated || !recommendation.RequiresApproval {
		t.Fatalf("A2A must surface as a gated local planning bridge: %#v", recommendations)
	}
}

func TestRecommendAnythingLLMWorkspaceUsesTheRecordedLocalEvidenceBoundary(t *testing.T) {
	recommendations := Recommend("research", "Create a local RAG workspace in AnythingLLM for approved document research")
	for _, recommendation := range recommendations {
		if recommendation.ID != "anythingllm" {
			continue
		}
		if recommendation.Status != StatusIntegrated || !recommendation.RequiresApproval || recommendation.Role != "integrated profile; operator configuration and live probe required" {
			t.Fatalf("AnythingLLM must surface as a guarded local evidence profile: %#v", recommendation)
		}
		return
	}
	t.Fatalf("AnythingLLM workspace request should surface the local workspace profile: %#v", recommendations)
}

func TestRecommendAdditionalOSSInsightProfilesKeepTheirRecordedBoundaries(t *testing.T) {
	recommendations := Recommend("operations", "Design a FastMCP tool server, serve a local model with vLLM, and evaluate grounded retrieval quality")
	ids := map[string]Recommendation{}
	for _, recommendation := range recommendations {
		ids[recommendation.ID] = recommendation
	}
	if recommendation, ok := ids["deepeval"]; !ok || recommendation.Status != StatusIntegrated || !recommendation.RequiresApproval || recommendation.Role != "integrated profile; operator configuration and live probe required" {
		t.Fatalf("DeepEval must surface as an integrated, configuration-gated local regression profile: %#v", recommendations)
	}
	if recommendation, ok := ids["fastmcp"]; !ok || recommendation.Status != StatusIntegrated || !recommendation.RequiresApproval || recommendation.Role != "integrated profile; operator configuration and live probe required" {
		t.Fatalf("FastMCP must surface as an integrated, configuration-gated read-only bridge: %#v", recommendations)
	}
	if recommendation, ok := ids["vllm"]; !ok || recommendation.Status != StatusIntegrated || !recommendation.RequiresApproval || recommendation.Role != "integrated profile; operator configuration and live probe required" {
		t.Fatalf("vLLM must surface as an integrated, configuration-gated provider profile: %#v", recommendations)
	}
}

func TestRecommendNewControlledMCPAndCodeCandidatesStayReviewFirst(t *testing.T) {
	recommendations := Recommend("coding", "Use GitHub MCP to inspect a pull request, run a Playwright MCP browser check, and propose a sandboxed bug-fix patch")
	ids := map[string]Recommendation{}
	for _, recommendation := range recommendations {
		ids[recommendation.ID] = recommendation
	}
	for _, id := range []string{"playwright-mcp"} {
		recommendation, ok := ids[id]
		if !ok || recommendation.Status != StatusCandidate || !recommendation.RequiresApproval || recommendation.Role != "optional capability" {
			t.Fatalf("%s must remain a review-first candidate: %#v", id, recommendations)
		}
	}
	githubMCP, ok := ids["github-mcp-server"]
	if !ok || githubMCP.Status != StatusIntegrated || githubMCP.Role != "integrated profile; operator configuration and live probe required" {
		t.Fatalf("GitHub MCP context must surface as an integrated bounded profile: %#v", recommendations)
	}
	if recommendation, ok := ids["mini-swe-agent"]; !ok || recommendation.Status != StatusIntegrated || !recommendation.RequiresApproval || recommendation.Role != "integrated profile; operator configuration and live probe required" {
		t.Fatalf("mini-SWE must surface as a configuration-gated disposable patch profile: %#v", recommendations)
	}
	if recommendation, ok := ids["qodo-pr-agent"]; !ok || recommendation.Status != StatusReferenceOnly || recommendation.Role != "reference or review only" {
		t.Fatalf("qodo-pr-agent must remain a non-integrated review reference: %#v", recommendations)
	}
}

func TestNewOSSInsightReferencesAndExclusionsDoNotClaimActivation(t *testing.T) {
	recommendations := Recommend("operations", "Compare long-term memory consolidation, OpenTelemetry traces, Phoenix, and TaskWeaver")
	ids := map[string]Recommendation{}
	for _, recommendation := range recommendations {
		ids[recommendation.ID] = recommendation
	}
	if recommendation, ok := ids["langmem"]; !ok || recommendation.Status != StatusReferenceOnly || recommendation.Role != "reference or review only" {
		t.Fatalf("LangMem must remain a non-activating memory reference: %#v", recommendations)
	}
	for _, id := range []string{"openlit", "phoenix"} {
		recommendation, ok := ids[id]
		if !ok || recommendation.Role != "optional capability" && recommendation.Role != "reference or review only" && recommendation.Role != "integrated profile; operator configuration and live probe required" {
			t.Fatalf("%s must not claim activation: %#v", id, recommendations)
		}
	}
	if entry, ok := EntryByID("pyrit"); !ok || entry.Status != StatusExcluded {
		t.Fatalf("archived PyRIT must remain excluded: %#v", entry)
	}
	if entry, ok := EntryByID("taskweaver"); !ok || entry.Status != StatusExcluded {
		t.Fatalf("archived TaskWeaver must remain excluded: %#v", entry)
	}
}

func TestRecommendSafetyEvaluationAndTelemetryCandidatesStayReviewFirst(t *testing.T) {
	recommendations := Recommend("operations", "Redact PII before audit export, validate structured output, benchmark a local model, and add OpenTelemetry traces")
	ids := map[string]Recommendation{}
	for _, recommendation := range recommendations {
		ids[recommendation.ID] = recommendation
	}
	for _, id := range []string{"presidio", "guardrails-ai", "lm-eval-harness"} {
		recommendation, ok := ids[id]
		if !ok || recommendation.Status != StatusIntegrated || !recommendation.RequiresApproval || recommendation.Role != "integrated profile; operator configuration and live probe required" {
			t.Fatalf("%s must remain an integrated but configuration-gated profile: %#v", id, recommendations)
		}
	}
	if recommendation, ok := ids["openllmetry"]; !ok || recommendation.Status != StatusCandidate || !recommendation.RequiresApproval || recommendation.Role != "optional capability" {
		t.Fatalf("OpenLLMetry must remain a review-first candidate: %#v", recommendations)
	}
}

func TestMicrosoftAgentFrameworkRemainsAConfigurationGatedReviewOnlyProfile(t *testing.T) {
	entry, ok := EntryByID("microsoft-agent-framework")
	if !ok || entry.Status != StatusIntegrated || !entry.RequiresApproval || !entry.LocalFirstCompatible {
		t.Fatalf("Microsoft Agent Framework must remain an opt-in local review-only profile: %#v", entry)
	}
	if !strings.Contains(entry.IntegrationMode, "sequential planning") || !strings.Contains(entry.Activation, "HAI_AGENT_FRAMEWORK_ENABLED") || !strings.Contains(entry.Activation, "/api/v1/autogen-compat/migration-plan") {
		t.Fatalf("local profile and migration-plan boundaries must stay explicit: %#v", entry)
	}
	if !strings.Contains(entry.VerificationNote, "no tool/state authority") {
		t.Fatalf("framework boundary must not claim autonomous authority: %#v", entry)
	}
}

func TestRecommendRetrievalReferencesNeverClaimActivation(t *testing.T) {
	recommendations := Recommend("research", "Compare Haystack and Microsoft GraphRAG for a document pipeline and knowledge graph")
	ids := map[string]Recommendation{}
	for _, recommendation := range recommendations {
		ids[recommendation.ID] = recommendation
	}
	for _, id := range []string{"haystack", "graphrag"} {
		recommendation, ok := ids[id]
		if !ok || recommendation.Status != StatusReferenceOnly || recommendation.Role != "reference or review only" {
			t.Fatalf("%s must remain a non-activating reference: %#v", id, recommendations)
		}
	}
}

func TestRecommendLiveGapProfilesKeepTheirRecordedBoundaries(t *testing.T) {
	recommendations := Recommend("operations", "Create typed structured plans, use LocalAI for local inference, inventory a source, prepare a voice pipeline, and run DeepTeam and Garak red-team regression")
	ids := map[string]Recommendation{}
	for _, recommendation := range recommendations {
		ids[recommendation.ID] = recommendation
	}
	for _, id := range []string{"pipecat"} {
		recommendation, ok := ids[id]
		if !ok || recommendation.Status != StatusCandidate || !recommendation.RequiresApproval || recommendation.Role != "optional capability" {
			t.Fatalf("%s must remain a review-first live-gap candidate: %#v", id, recommendations)
		}
	}
	if recommendation, ok := ids["deepteam"]; !ok || recommendation.Status != StatusIntegrated || !recommendation.RequiresApproval || recommendation.Role != "integrated profile; operator configuration and live probe required" {
		t.Fatalf("DeepTeam must surface as an integrated, configuration-gated synthetic safety profile: %#v", recommendations)
	}
	if recommendation, ok := ids["garak"]; !ok || recommendation.Status != StatusIntegrated || !recommendation.RequiresApproval || recommendation.Role != "integrated profile; operator configuration and live probe required" {
		t.Fatalf("Garak must surface as an integrated, configuration-gated synthetic safety profile: %#v", recommendations)
	}
	if recommendation, ok := ids["cloudquery"]; !ok || recommendation.Status != StatusIntegrated || !recommendation.RequiresApproval || recommendation.Role != "integrated profile; operator configuration and live probe required" {
		t.Fatalf("CloudQuery must surface as an integrated, configuration-gated local summary profile: %#v", recommendations)
	}
	if recommendation, ok := ids["pydantic-ai"]; !ok || recommendation.Status != StatusIntegrated || !recommendation.RequiresApproval || recommendation.Role != "integrated profile; operator configuration and live probe required" {
		t.Fatalf("PydanticAI must surface as an integrated, configuration-gated typed-planning profile: %#v", recommendations)
	}
	if recommendation, ok := ids["localai"]; !ok || recommendation.Status != StatusIntegrated || !recommendation.RequiresApproval || recommendation.Role != "integrated profile; operator configuration and live probe required" {
		t.Fatalf("LocalAI must surface as an integrated, configuration-gated provider profile: %#v", recommendations)
	}
}

func TestHeldLiveGapProfilesDoNotClaimActivation(t *testing.T) {
	recommendations := Recommend("safety", "Compare LLM Guard, OpenAI Evals, OmniParser, AgentBench, and MCP Servers")
	ids := map[string]Recommendation{}
	for _, recommendation := range recommendations {
		ids[recommendation.ID] = recommendation
	}
	for _, id := range []string{"llm-guard", "openai-evals", "omniparser", "agentbench", "mcp-servers"} {
		if _, ok := ids[id]; ok {
			t.Fatalf("held profile %s must not be returned by generic capability recommendation: %#v", id, recommendations)
		}
	}
}
