package autonomygate

import "testing"

func TestApprovalAlwaysAuto(t *testing.T) {
	if Decide(Signals{Risk: "high", Reversible: false, Approved: true}) != Auto {
		t.Fatalf("explicit approval should permit auto")
	}
}

func TestHighRiskIrreversibleBlocks(t *testing.T) {
	if Decide(Signals{Confidence: 0.99, Risk: "high", Reversible: false}) != Block {
		t.Fatalf("high risk + irreversible must block")
	}
}

func TestHighRiskOrLowConfidenceReviews(t *testing.T) {
	if Decide(Signals{Confidence: 0.99, Risk: "high", Reversible: true}) != Review {
		t.Fatalf("high risk reversible should review")
	}
	if Decide(Signals{Confidence: 0.3, Risk: "low", Reversible: true}) != Review {
		t.Fatalf("low confidence should review")
	}
}

func TestOnlyLowRiskHighConfidenceCasesAuto(t *testing.T) {
	if Decide(Signals{Confidence: 0.9, Risk: "low", Reversible: true}) != Auto {
		t.Fatalf("safe case should auto")
	}
	if Decide(Signals{Confidence: 0.9, Risk: "medium", Reversible: true}) != Review {
		t.Fatalf("medium risk should require review before execution")
	}
	if Decide(Signals{Confidence: 0.9, Risk: "medium", Reversible: false}) != Review {
		t.Fatalf("medium risk irreversible should review")
	}
}
