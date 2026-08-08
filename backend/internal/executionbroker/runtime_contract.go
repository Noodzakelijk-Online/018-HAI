// Package executionbroker executes approved/safe operations through bounded
// runtime adapters (§10.14). In Phase 2A only the local safe worker actually
// executes; other runtimes are contract-defined but must never be presented as
// fake-live. No runtime may execute when its status is unavailable,
// not_configured, blocked, or failed.
package executionbroker

import "context"

// RuntimeStatus is the health/availability of a runtime adapter.
type RuntimeStatus string

const (
	RuntimeReady         RuntimeStatus = "ready"
	RuntimeNotConfigured RuntimeStatus = "not_configured"
	RuntimeUnavailable   RuntimeStatus = "unavailable"
	RuntimeBlocked       RuntimeStatus = "blocked"
	RuntimeFailed        RuntimeStatus = "failed"
)

// CanExecute reports whether a runtime in this status may execute (§10.14).
func (s RuntimeStatus) CanExecute() bool { return s == RuntimeReady }

// ClaimLevel is how strongly a runtime/provider capability is proven (§10.17).
// Never auto-promote to production_ready.
type ClaimLevel string

const (
	ClaimDocumentedOnly    ClaimLevel = "documented_only"
	ClaimContractDefined   ClaimLevel = "contract_defined"
	ClaimConfigured        ClaimLevel = "configured"
	ClaimProbed            ClaimLevel = "probed"
	ClaimSmokeTested       ClaimLevel = "smoke_tested"
	ClaimExercisedSafeTask ClaimLevel = "exercised_with_local_safe_task"
	ClaimOperatorVerified  ClaimLevel = "operator_verified"
	ClaimProductionReady   ClaimLevel = "production_ready"
)

// RuntimeHealth is a runtime's reported health.
type RuntimeHealth struct {
	Status RuntimeStatus `json:"status"`
	Detail string        `json:"detail,omitempty"`
	Claim  ClaimLevel    `json:"claimLevel"`
}

// DryRunResult is the plan validation from a runtime before execution.
type DryRunResult struct {
	OK      bool   `json:"ok"`
	Summary string `json:"summary"`
}

// RuntimeResult is the bounded outcome of a runtime execution.
type RuntimeResult struct {
	OK            bool   `json:"ok"`
	BoundedOutput string `json:"boundedOutput"`
	Error         string `json:"error,omitempty"`
}

// RuntimeAdapter is the contract every runtime must satisfy (§10.14). Adapters
// must be typed, bounded, testable, and claim-level aware.
type RuntimeAdapter interface {
	ID() string
	DisplayName() string
	ClaimLevel(ctx context.Context) ClaimLevel
	HealthCheck(ctx context.Context) RuntimeHealth
	// DryRun/Execute take an opaque action plan payload; Phase 2A only the local
	// safe worker implements real execution.
	DryRun(ctx context.Context, payload map[string]any) (DryRunResult, error)
	Execute(ctx context.Context, payload map[string]any) (RuntimeResult, error)
}
