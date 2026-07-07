package quality

import "testing"

func TestHighQualityScoresHigh(t *testing.T) {
	s := Score(Inputs{Confidence: 0.95, EvidenceCount: 3, FreshnessDays: 0})
	if BandOf(s) != High {
		t.Fatalf("strong inputs should be high, got %.2f (%s)", s, BandOf(s))
	}
}

func TestWeakInputsScoreLow(t *testing.T) {
	s := Score(Inputs{Confidence: 0.2, EvidenceCount: 0, FreshnessDays: 120})
	if BandOf(s) != Low {
		t.Fatalf("weak inputs should be low, got %.2f (%s)", s, BandOf(s))
	}
}

func TestConfidenceClampedAndBounded(t *testing.T) {
	s := Score(Inputs{Confidence: 5, EvidenceCount: 3, FreshnessDays: 0})
	if s > 1.0 {
		t.Fatalf("score must not exceed 1.0, got %.2f", s)
	}
	neg := Score(Inputs{Confidence: -1, EvidenceCount: 0, FreshnessDays: 200})
	if neg < 0 {
		t.Fatalf("score must not be negative, got %.2f", neg)
	}
}

func TestBandBoundaries(t *testing.T) {
	if BandOf(0.75) != High || BandOf(0.45) != Medium || BandOf(0.44) != Low {
		t.Fatalf("band boundaries wrong")
	}
}
