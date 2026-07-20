package braincatalog

import "testing"

func TestRecommendForNeedRanksReviewedCatalogWithoutHeldProjects(t *testing.T) {
	response, err := RecommendForNeed("local model evaluation")
	if err != nil {
		t.Fatalf("RecommendForNeed() error = %v", err)
	}
	if len(response.Recommendations) == 0 {
		t.Fatal("expected reviewed capability matches")
	}
	if response.Recommendations[0].RoadmapPriority == 0 || response.Recommendations[0].RoadmapReason == "" || len(response.Recommendations[0].CapabilityPlanes) == 0 {
		t.Fatalf("recommendations must carry the reviewed implementation context: %#v", response.Recommendations[0])
	}
	foundEvaluation := false
	for _, recommendation := range response.Recommendations {
		if recommendation.Status == StatusExcluded || recommendation.Status == StatusReferenceOnly || recommendation.Status == StatusLicenseReview {
			t.Fatalf("held project was recommended: %#v", recommendation)
		}
		if recommendation.ID == "lm-eval-harness" || recommendation.ID == "deepeval" {
			foundEvaluation = true
		}
		if recommendation.NextStep == "" || recommendation.Score <= 0 {
			t.Fatalf("recommendation lacks an actionable review boundary: %#v", recommendation)
		}
		if recommendation.UpstreamURL == "" || recommendation.SourceCatalogURL == "" || recommendation.VerifiedAt == "" || recommendation.VerificationNote == "" {
			t.Fatalf("recommendation lacks reviewed provenance: %#v", recommendation)
		}
	}
	if !foundEvaluation {
		t.Fatalf("expected local evaluation candidate: %#v", response.Recommendations)
	}
}

func TestRecommendForNeedRequiresSpecificTerms(t *testing.T) {
	if _, err := RecommendForNeed("to use it"); err == nil {
		t.Fatal("expected vague need to be rejected")
	}
}

func TestRecommendForNeedExpandsCommonOperationalTermsTransparently(t *testing.T) {
	response, err := RecommendForNeed("benchmark local LLM models and protect PII")
	if err != nil {
		t.Fatalf("RecommendForNeed() error = %v", err)
	}
	for _, expected := range []string{"model", "inference", "sensitive", "redaction"} {
		found := false
		for _, term := range response.ExpandedTerms {
			if term == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing expanded term %q: %#v", expected, response.ExpandedTerms)
		}
	}
	if !hasRecommendationID(response.Recommendations, "lm-eval-harness") || !hasRecommendationID(response.Recommendations, "presidio") {
		t.Fatalf("expected local evaluation and redaction candidates: %#v", response.Recommendations)
	}
	for _, recommendation := range response.Recommendations {
		if len(recommendation.MatchedTerms) == 0 {
			t.Fatalf("recommendation lacks traceable matched terms: %#v", recommendation)
		}
	}
}

func TestRecommendRecordsMicrosoftAgentFrameworkAsCandidateNotRuntime(t *testing.T) {
	recommendations := Recommend("agent migration", "Migrate an AutoGen successor workflow to Microsoft Agent Framework with human approval")
	found := false
	for _, recommendation := range recommendations {
		if recommendation.ID != "microsoft-agent-framework" {
			continue
		}
		found = true
		if recommendation.Status != StatusCandidate {
			t.Fatalf("Microsoft Agent Framework must remain a reviewed candidate: %#v", recommendation)
		}
		if !recommendation.RequiresApproval {
			t.Fatalf("Microsoft Agent Framework must require approval: %#v", recommendation)
		}
	}
	if !found {
		t.Fatalf("expected Microsoft Agent Framework recommendation: %#v", recommendations)
	}
}

func TestRecommendReviewedHighPriorityCandidatesWithoutClaimingActivation(t *testing.T) {
	recommendations := Recommend("operations", "Evaluate source-grounded answers with Evidently, then trial mistral.rs, RAGFlow document retrieval, and a LiveKit realtime voice session")
	ids := map[string]Recommendation{}
	for _, recommendation := range recommendations {
		ids[recommendation.ID] = recommendation
	}
	for _, id := range []string{"evidently", "ragflow"} {
		recommendation, ok := ids[id]
		if !ok || recommendation.Status != StatusIntegrated || !recommendation.RequiresApproval || recommendation.Role != "integrated profile; operator configuration and live probe required" {
			t.Fatalf("%s must remain integrated but configuration-gated: %#v", id, recommendations)
		}
	}
	if recommendation, ok := ids["livekit-agents"]; !ok || recommendation.Status != StatusCandidate || !recommendation.RequiresApproval {
		t.Fatalf("LiveKit Agents must remain a reviewed, approval-gated candidate: %#v", recommendations)
	}
	if recommendation, ok := ids["mistral-rs"]; !ok || recommendation.Status != StatusIntegrated || !recommendation.RequiresApproval || recommendation.Role != "integrated profile; operator configuration and live probe required" {
		t.Fatalf("mistral.rs must remain integrated but configuration- and approval-gated: %#v", recommendations)
	}
}

func TestRecommendSemanticCodeContextWithoutEnablingHostAutomation(t *testing.T) {
	recommendations := Recommend("coding", "Use Serena semantic code retrieval and language server diagnostics for a cross-file impact review")
	ids := map[string]Recommendation{}
	for _, recommendation := range recommendations {
		ids[recommendation.ID] = recommendation
	}
	if recommendation, ok := ids["serena"]; !ok || recommendation.Status != StatusCandidate || !recommendation.RequiresApproval {
		t.Fatalf("Serena must remain a review-first code-context candidate: %#v", recommendations)
	}

	for _, expected := range []string{"ufo", "goose"} {
		found := false
		request := "Compare Microsoft UFO Windows UI automation"
		if expected == "goose" {
			request = "Compare the Goose agent MCP workflow"
		}
		for _, recommendation := range Recommend("operations", request) {
			if recommendation.ID != expected {
				continue
			}
			found = true
			if recommendation.Status != StatusReferenceOnly || recommendation.Role != "reference or review only" || !recommendation.RequiresApproval {
				t.Fatalf("high-risk general agent reference must not become active: %#v", recommendation)
			}
		}
		if !found {
			t.Fatalf("expected %s reference recommendation", expected)
		}
	}
}

func hasRecommendationID(recommendations []CapabilityRecommendation, id string) bool {
	for _, recommendation := range recommendations {
		if recommendation.ID == id {
			return true
		}
	}
	return false
}
