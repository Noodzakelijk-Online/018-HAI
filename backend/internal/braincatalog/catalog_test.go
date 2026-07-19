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
}

func TestOSSInsightCandidatesHaveLocalActivationBoundaries(t *testing.T) {
	for _, id := range []string{"litellm", "pgvector", "temporal", "prometheus", "mcp-inspector", "playwright", "wasmtime", "ortools"} {
		entry, ok := EntryByID(id)
		if !ok || entry.Status != StatusCandidate || entry.SourceCollection == "" || entry.SourceCatalogURL == "" || !entry.LocalFirstCompatible {
			t.Fatalf("%s must be a source-backed local candidate: %#v", id, entry)
		}
	}
	if entry, ok := EntryByID("llama-cpp"); !ok || entry.Status != StatusIntegrated || !entry.LocalFirstCompatible || !entry.RequiresApproval {
		t.Fatalf("llama.cpp must report its integrated-but-not-active local provider profile: %#v", entry)
	}
	if entry, ok := EntryByID("qdrant"); !ok || entry.Status != StatusReferenceOnly {
		t.Fatalf("Qdrant must not create a second active vector store by default: %#v", entry)
	}
	for _, id := range []string{"activepieces", "mem0", "openmetadata"} {
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
	for _, id := range []string{"temporal", "prometheus", "litellm"} {
		if recommendation, ok := ids[id]; !ok || recommendation.Status != StatusCandidate {
			t.Fatalf("missing governed %s recommendation: %#v", id, recommendations)
		}
	}
	if !ids["temporal"].RequiresApproval || !ids["litellm"].RequiresApproval {
		t.Fatalf("durable execution and gateway candidates need explicit review: %#v", recommendations)
	}
}

func TestRecommendNewCandidatesNeverClaimsExecution(t *testing.T) {
	recommendations := Recommend("operations", "Use a local GGUF model, verify a browser flow, run a WASI helper, and create a route optimization proposal")
	ids := map[string]Recommendation{}
	for _, recommendation := range recommendations {
		ids[recommendation.ID] = recommendation
	}
	for _, id := range []string{"playwright", "wasmtime", "ortools"} {
		recommendation, ok := ids[id]
		if !ok || recommendation.Status != StatusCandidate || recommendation.Role != "optional capability" {
			t.Fatalf("missing non-executable %s recommendation: %#v", id, recommendations)
		}
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
	for _, id := range []string{"continue", "aider", "openhands"} {
		if _, ok := ids[id]; !ok {
			t.Fatalf("missing %s recommendation: %#v", id, recommendations)
		}
	}
	if !ids["aider"].RequiresApproval || !ids["openhands"].RequiresApproval {
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
