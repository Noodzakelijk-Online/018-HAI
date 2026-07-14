package modelintelligence

import "time"

// ModelProfile is the architecture-aware metadata + observed telemetry for one
// (provider, model) pair (§16). Metadata is declared; observed metrics are only
// filled from real probes/benchmarks/telemetry — never fabricated.
type ModelProfile struct {
	ProviderID         string             `json:"providerId"`
	ModelID            string             `json:"modelId"`
	DisplayName        string             `json:"displayName"`
	ArchitectureFamily ArchitectureFamily `json:"architectureFamily"`
	Lanes              []RoutingLane      `json:"lanes"`
	ContextWindow      int                `json:"contextWindow"`
	Local              bool               `json:"local"`
	Paid               bool               `json:"paid"`
	Status             ProviderStatus     `json:"status"`
	ClaimLevel         ClaimLevel         `json:"claimLevel"`

	// Observed metrics (filled only from real runs; zero means not measured).
	ObservedTokensPerSecond float64    `json:"observedTokensPerSecond"`
	ObservedRuns            int        `json:"observedRuns"`
	ObservedFailures        int        `json:"observedFailures"`
	LastProbedAt            *time.Time `json:"lastProbedAt,omitempty"`
	LastBenchmarkedAt       *time.Time `json:"lastBenchmarkedAt,omitempty"`
}

// Key is the stable "providerId/modelId" identifier.
func (p ModelProfile) Key() string { return p.ProviderID + "/" + p.ModelID }

// ServesLane reports whether the model is declared to serve the given lane.
func (p ModelProfile) ServesLane(l RoutingLane) bool {
	for _, x := range p.Lanes {
		if x == l {
			return true
		}
	}
	return false
}

// Usable reports whether the model may currently be routed to.
func (p ModelProfile) Usable() bool { return p.Status.Usable() }
