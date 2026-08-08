package runtimelab

import (
	"context"
	"time"

	"automation-hub-backend/internal/executionbroker"
)

// SetupRequirement is one exact step required to make an unavailable runtime
// usable (§2C: "exact setup requirements for unavailable runtimes").
type SetupRequirement struct {
	Step   string `json:"step"`
	Detail string `json:"detail"`
}

// RuntimeInfo is a runtime adapter's static description (§15 Info).
type RuntimeInfo struct {
	ID          string      `json:"id"`
	DisplayName string      `json:"displayName"`
	Kind        RuntimeKind `json:"kind"`
	Description string      `json:"description"`
}

// Health is a runtime's truthful health plus setup requirements when not ready.
type Health struct {
	Status            executionbroker.RuntimeStatus `json:"status"`
	Detail            string                        `json:"detail,omitempty"`
	Claim             executionbroker.ClaimLevel    `json:"claimLevel"`
	SetupRequirements []SetupRequirement            `json:"setupRequirements,omitempty"`
}

// ProbeResult is the truthful outcome of probing a runtime.
type ProbeResult struct {
	RuntimeID        string                        `json:"runtimeId"`
	Status           executionbroker.RuntimeStatus `json:"status"`
	DiscoveryState   string                        `json:"discoveryState"`
	ReadinessLevel   RuntimeReadinessLevel         `json:"readinessLevel"`
	Protocol         string                        `json:"protocol,omitempty"`
	RuntimeVersion   string                        `json:"runtimeVersion,omitempty"`
	ProtocolValid    bool                          `json:"protocolValid"`
	IdentityVerified bool                          `json:"identityVerified"`
	Authenticated    bool                          `json:"authenticated"`
	Capabilities     []string                      `json:"capabilities,omitempty"`
	EvidenceSHA256   string                        `json:"evidenceSha256,omitempty"`
	DurationMs       int64                         `json:"durationMs"`
	Detail           string                        `json:"detail,omitempty"`
	CheckedAt        time.Time                     `json:"checkedAt"`
}

// Adapter is the runtime adapter contract (§15). It mirrors Info / HealthCheck /
// Capabilities / DryRun / Execute / Stop / ClaimLevel / SetupRequirements.
// Execute must refuse (return an error) unless the runtime actually ran — no
// fake execution.
type Adapter interface {
	Info() RuntimeInfo
	HealthCheck(ctx context.Context) Health
	Capabilities() []string
	SetupRequirements() []SetupRequirement
	Probe(ctx context.Context, now time.Time) ProbeResult
	// Execute runs a bounded task. Non-safe runtimes return an error describing
	// the setup required; only really-executed work returns a result.
	Execute(ctx context.Context, payload map[string]any) (executionbroker.RuntimeResult, error)
	// Stop is a no-op for stateless adapters but part of the contract.
	Stop(ctx context.Context) error
}
