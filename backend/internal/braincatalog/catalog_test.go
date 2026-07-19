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
	for _, id := range []string{"fastmcp", "vllm", "deepeval", "langfuse", "promptfoo", "airbyte", "odoo", "browser-use", "nemo-guardrails", "garak", "whisper-cpp", "tabby"} {
		if entry, ok := EntryByID(id); !ok || entry.Status != StatusCandidate || !entry.RequiresApproval {
			t.Fatalf("%s must remain a review-first candidate: %#v", id, entry)
		}
	}
	if entry, ok := EntryByID("a2a"); !ok || entry.Status != StatusCompatibility || !entry.RequiresApproval {
		t.Fatalf("A2A must remain a review-first compatibility profile: %#v", entry)
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
	if sources := DiscoverySources(); len(sources) < 2 || sources[1].Name != "OSS Insight" {
		t.Fatalf("OSS Insight source is missing: %#v", sources)
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
	for _, id := range []string{"browser-use", "nemo-guardrails", "garak", "whisper-cpp"} {
		if recommendation, ok := ids[id]; !ok || recommendation.Status != StatusCandidate || !recommendation.RequiresApproval {
			t.Fatalf("%s must remain a review-first candidate: %#v", id, recommendations)
		}
	}
	if recommendation, ok := ids["a2a"]; !ok || recommendation.Status != StatusCompatibility || !recommendation.RequiresApproval {
		t.Fatalf("A2A must remain a gated compatibility recommendation: %#v", recommendations)
	}
}

func TestRecommendAdditionalOSSInsightCandidatesStayReviewFirst(t *testing.T) {
	recommendations := Recommend("operations", "Design a FastMCP tool server, serve a local model with vLLM, and evaluate grounded retrieval quality")
	ids := map[string]Recommendation{}
	for _, recommendation := range recommendations {
		ids[recommendation.ID] = recommendation
	}
	for _, id := range []string{"fastmcp", "vllm", "deepeval"} {
		recommendation, ok := ids[id]
		if !ok || recommendation.Status != StatusCandidate || !recommendation.RequiresApproval || recommendation.Role != "optional capability" {
			t.Fatalf("%s must remain a review-first candidate: %#v", id, recommendations)
		}
	}
}
