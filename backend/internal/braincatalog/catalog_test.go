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
	if entry, ok := EntryByID("autogen"); !ok || entry.Status != StatusExcluded {
		t.Fatalf("AutoGen must remain excluded for new work: %#v", entry)
	}
	if entry, ok := EntryByID("autogpt"); !ok || entry.Status != StatusLicenseReview {
		t.Fatalf("AutoGPT must require license review: %#v", entry)
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
