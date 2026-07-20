package braincatalog

import "testing"

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
	if entry, ok := EntryByID("autogen"); !ok || entry.Status != StatusCompatibility || !entry.RequiresApproval || len(entry.ControlMappings) == 0 {
		t.Fatalf("AutoGen must remain a gated compatibility profile: %#v", entry)
	}
	if entry, ok := EntryByID("autogpt"); !ok || entry.Status != StatusLicenseReview {
		t.Fatalf("AutoGPT must require license review: %#v", entry)
	}
	for _, id := range []string{"openllmetry", "deepeval", "airbyte", "odoo", "browser-use", "nemo-guardrails", "garak", "tabby", "github-mcp-server", "playwright-mcp", "google-genai-toolbox", "qodo-pr-agent", "swe-agent", "openlit", "anythingllm", "cloudquery", "opik", "deepteam", "openspec", "pipecat", "livekit-agents", "serena"} {
		if entry, ok := EntryByID(id); !ok || entry.Status != StatusCandidate || !entry.RequiresApproval {
			t.Fatalf("%s must remain a review-first candidate: %#v", id, entry)
		}
	}
	for _, id := range []string{"presidio", "guardrails-ai", "lm-eval-harness", "promptfoo", "evidently", "ragflow"} {
		if entry, ok := EntryByID(id); !ok || entry.Status != StatusIntegrated || !entry.RequiresApproval || !entry.LocalFirstCompatible {
			t.Fatalf("%s must expose its implemented local profile without claiming that it is configured: %#v", id, entry)
		}
	}
	if entry, ok := EntryByID("langfuse"); !ok || entry.Status != StatusIntegrated || !entry.RequiresApproval || !entry.LocalFirstCompatible {
		t.Fatalf("Langfuse must expose only its local, approval-aware bridge: %#v", entry)
	}
	if entry, ok := EntryByID("ag2"); !ok || entry.Status != StatusCompatibility || !entry.RequiresApproval {
		t.Fatalf("AG2 must remain a gated compatibility profile: %#v", entry)
	}
	for _, id := range []string{"ufo", "goose"} {
		if entry, ok := EntryByID(id); !ok || entry.Status != StatusReferenceOnly || !entry.RequiresApproval || len(entry.ControlMappings) == 0 {
			t.Fatalf("%s must remain a non-active high-risk reference: %#v", id, entry)
		}
	}
	if entry, ok := EntryByID("localai"); !ok || entry.Status != StatusIntegrated || !entry.RequiresApproval {
		t.Fatalf("LocalAI must report its integrated, approval-gated local provider profile: %#v", entry)
	}
	if entry, ok := EntryByID("vllm"); !ok || entry.Status != StatusIntegrated || !entry.RequiresApproval {
		t.Fatalf("vLLM must report its integrated, approval-gated local provider profile: %#v", entry)
	}
	if entry, ok := EntryByID("mistral-rs"); !ok || entry.Status != StatusIntegrated || !entry.LocalFirstCompatible || !entry.RequiresApproval {
		t.Fatalf("mistral.rs must report its integrated, approval-gated local provider profile: %#v", entry)
	}
	if entry, ok := EntryByID("whisper-cpp"); !ok || entry.Status != StatusIntegrated || !entry.LocalFirstCompatible || !entry.RequiresApproval {
		t.Fatalf("whisper.cpp must report its integrated, approval-gated local source profile: %#v", entry)
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
	for _, id := range []string{"activepieces", "mem0", "letta", "comfyui", "daytona", "openmetadata"} {
		if entry, ok := EntryByID(id); !ok || entry.Status != StatusReferenceOnly {
			t.Fatalf("%s must remain a reference rather than a parallel control plane: %#v", id, entry)
		}
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
	if entry, ok := EntryByID("agentbench"); !ok || entry.Status != StatusReferenceOnly {
		t.Fatalf("AgentBench must remain a reference-only evaluation pattern: %#v", entry)
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
	for _, id := range []string{"continue", "cline", "opencode", "aider", "openhands"} {
		if _, ok := ids[id]; !ok {
			t.Fatalf("missing %s recommendation: %#v", id, recommendations)
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
	recommendations := Recommend("operations", "Use Ollama for a local model, transcribe a voice note, run a browser agent safety evaluation, and exchange an A2A task envelope")
	ids := map[string]Recommendation{}
	for _, recommendation := range recommendations {
		ids[recommendation.ID] = recommendation
	}
	if recommendation, ok := ids["ollama"]; !ok || recommendation.Status != StatusIntegrated || !recommendation.RequiresApproval {
		t.Fatalf("Ollama must surface as HAI's existing approval-gated local provider profile: %#v", recommendations)
	}
	for _, id := range []string{"browser-use", "nemo-guardrails", "garak"} {
		if recommendation, ok := ids[id]; !ok || recommendation.Status != StatusCandidate || !recommendation.RequiresApproval {
			t.Fatalf("%s must remain a review-first candidate: %#v", id, recommendations)
		}
	}
	if recommendation, ok := ids["whisper-cpp"]; !ok || recommendation.Status != StatusIntegrated || !recommendation.RequiresApproval {
		t.Fatalf("whisper.cpp must surface as HAI's approval-gated local transcription profile: %#v", recommendations)
	}
	if recommendation, ok := ids["a2a"]; !ok || recommendation.Status != StatusIntegrated || !recommendation.RequiresApproval {
		t.Fatalf("A2A must surface as a gated local planning bridge: %#v", recommendations)
	}
}

func TestRecommendAnythingLLMWorkspaceStaysReviewFirst(t *testing.T) {
	recommendations := Recommend("research", "Create a local RAG workspace in AnythingLLM for approved document research")
	for _, recommendation := range recommendations {
		if recommendation.ID != "anythingllm" {
			continue
		}
		if recommendation.Status != StatusCandidate || !recommendation.RequiresApproval || recommendation.Role != "optional capability" {
			t.Fatalf("AnythingLLM must remain a review-first candidate: %#v", recommendation)
		}
		return
	}
	t.Fatalf("AnythingLLM workspace request should surface the local workspace candidate: %#v", recommendations)
}

func TestRecommendAdditionalOSSInsightProfilesKeepTheirRecordedBoundaries(t *testing.T) {
	recommendations := Recommend("operations", "Design a FastMCP tool server, serve a local model with vLLM, and evaluate grounded retrieval quality")
	ids := map[string]Recommendation{}
	for _, recommendation := range recommendations {
		ids[recommendation.ID] = recommendation
	}
	for _, id := range []string{"deepeval"} {
		recommendation, ok := ids[id]
		if !ok || recommendation.Status != StatusCandidate || !recommendation.RequiresApproval || recommendation.Role != "optional capability" {
			t.Fatalf("%s must remain a review-first candidate: %#v", id, recommendations)
		}
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
	for _, id := range []string{"github-mcp-server", "playwright-mcp", "qodo-pr-agent", "swe-agent"} {
		recommendation, ok := ids[id]
		if !ok || recommendation.Status != StatusCandidate || !recommendation.RequiresApproval || recommendation.Role != "optional capability" {
			t.Fatalf("%s must remain a review-first candidate: %#v", id, recommendations)
		}
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
		if !ok || recommendation.Role != "optional capability" && recommendation.Role != "reference or review only" {
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
	recommendations := Recommend("operations", "Create typed structured plans, use LocalAI for local inference, inventory a source, prepare a voice pipeline, and run DeepTeam red-team regression")
	ids := map[string]Recommendation{}
	for _, recommendation := range recommendations {
		ids[recommendation.ID] = recommendation
	}
	for _, id := range []string{"cloudquery", "pipecat", "deepteam"} {
		recommendation, ok := ids[id]
		if !ok || recommendation.Status != StatusCandidate || !recommendation.RequiresApproval || recommendation.Role != "optional capability" {
			t.Fatalf("%s must remain a review-first live-gap candidate: %#v", id, recommendations)
		}
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
