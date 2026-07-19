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
