// Package quality computes a confidence/quality score and band for a piece of
// information from its stated confidence, how much evidence supports it, and how
// fresh it is. Pure and deterministic.
package quality

// Band is a coarse quality bucket for display.
type Band string

const (
	High   Band = "high"
	Medium Band = "medium"
	Low    Band = "low"
)

// Inputs are the signals that feed a quality score.
type Inputs struct {
	Confidence    float64 // stated confidence, clamped to [0,1]
	EvidenceCount int     // supporting sources/evidence items
	FreshnessDays int     // age in days (0 = today)
}

// Score returns a quality score in [0,1] weighting confidence most, then
// evidence, then freshness.
func Score(in Inputs) float64 {
	confidence := clamp01(in.Confidence)

	evidence := 0.0
	switch {
	case in.EvidenceCount >= 3:
		evidence = 1.0
	case in.EvidenceCount == 2:
		evidence = 0.7
	case in.EvidenceCount == 1:
		evidence = 0.4
	}

	freshness := 1.0
	switch {
	case in.FreshnessDays <= 0:
		freshness = 1.0
	case in.FreshnessDays >= 90:
		freshness = 0.0
	default:
		freshness = 1.0 - float64(in.FreshnessDays)/90.0
	}

	return confidence*0.6 + evidence*0.25 + freshness*0.15
}

// BandOf buckets a score into high/medium/low.
func BandOf(score float64) Band {
	switch {
	case score >= 0.75:
		return High
	case score >= 0.45:
		return Medium
	default:
		return Low
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
