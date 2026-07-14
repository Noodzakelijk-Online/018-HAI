package modelintelligence

import (
	"context"
	"time"
)

// InferenceRequest is a bounded model call routed to a lane.
type InferenceRequest struct {
	Lane            RoutingLane
	Prompt          string
	MaxOutputTokens int
	Effort          ReasoningEffort
}

// InferenceResult is the bounded outcome of a model call plus telemetry.
type InferenceResult struct {
	ProviderID           string      `json:"providerId"`
	ModelID              string      `json:"modelId"`
	Lane                 RoutingLane `json:"lane"`
	Output               string      `json:"output"`
	InputTokensEstimate  int         `json:"inputTokensEstimate"`
	OutputTokensEstimate int         `json:"outputTokensEstimate"`
	DurationMs           int64       `json:"durationMs"`
	TokensPerSecond      float64     `json:"tokensPerSecond"`
	OK                   bool        `json:"ok"`
	Error                string      `json:"error,omitempty"`
}

// ProbeResult is the truthful outcome of probing a provider (§10.17).
type ProbeResult struct {
	ProviderID string         `json:"providerId"`
	Status     ProviderStatus `json:"status"`
	ModelsSeen int            `json:"modelsSeen"`
	DurationMs int64          `json:"durationMs"`
	Detail     string         `json:"detail,omitempty"`
	CheckedAt  time.Time      `json:"checkedAt"`
}

// Provider is a model provider adapter. Providers must report truthful status,
// never mark themselves active without a successful probe, and never execute
// external actions.
type Provider interface {
	ID() string
	DisplayName() string
	// Profiles returns the architecture-aware profiles this provider serves.
	Profiles() []ModelProfile
	// Probe checks reachability and returns a truthful status.
	Probe(ctx context.Context, now time.Time) ProbeResult
	// Generate performs a bounded inference call. It must return an error (not a
	// fabricated result) when the provider is not usable.
	Generate(ctx context.Context, req InferenceRequest, now time.Time) (InferenceResult, error)
}

// estimateTokens is a deterministic, provider-agnostic token estimate (~4 chars
// per token) used for telemetry when a provider does not report exact usage.
func estimateTokens(s string) int {
	n := len(s) / 4
	if n < 1 && len(s) > 0 {
		return 1
	}
	return n
}
