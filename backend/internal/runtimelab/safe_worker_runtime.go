package runtimelab

import (
	"context"
	"time"

	"automation-hub-backend/internal/executionbroker"
)

// safeWorkerRuntime is the one runtime that actually executes in this phase: the
// local safe worker. It proves the control plane end to end (§Scenario 6: "local
// safe worker can still prove control plane").
type safeWorkerRuntime struct {
	broker *executionbroker.Broker
}

func newSafeWorkerRuntime(broker *executionbroker.Broker) *safeWorkerRuntime {
	return &safeWorkerRuntime{broker: broker}
}

func (s *safeWorkerRuntime) Info() RuntimeInfo {
	return RuntimeInfo{
		ID:          executionbroker.LocalSafeWorkerID,
		DisplayName: "HAI Local Safe Worker",
		Kind:        KindLocalSafeWorker,
		Description: "Workspace-confined write/read-back/hash task with verification; the only runtime that actually executes in this phase.",
	}
}

func (s *safeWorkerRuntime) Capabilities() []string {
	return []string{"exercised:workspace_file_write", "exercised:read_back_and_hash", "exercised:postcondition_verification"}
}

func (s *safeWorkerRuntime) SetupRequirements() []SetupRequirement {
	h := s.broker.SafeWorker().HealthCheck(context.Background())
	if h.Status.CanExecute() {
		return nil
	}
	return []SetupRequirement{{Step: "Configure a workspace directory", Detail: "Set HAI_PHASE2_WORKSPACE_DIR to a writable directory to confine the safe worker."}}
}

func (s *safeWorkerRuntime) HealthCheck(ctx context.Context) Health {
	h := s.broker.SafeWorker().HealthCheck(ctx)
	return Health{Status: h.Status, Detail: h.Detail, Claim: h.Claim, SetupRequirements: s.SetupRequirements()}
}

func (s *safeWorkerRuntime) Probe(ctx context.Context, now time.Time) ProbeResult {
	h := s.broker.SafeWorker().HealthCheck(ctx)
	return ProbeResult{RuntimeID: executionbroker.LocalSafeWorkerID, Status: h.Status, Detail: h.Detail, CheckedAt: now}
}

func (s *safeWorkerRuntime) Execute(ctx context.Context, payload map[string]any) (executionbroker.RuntimeResult, error) {
	return s.broker.SafeWorker().Execute(ctx, payload)
}

func (s *safeWorkerRuntime) Stop(ctx context.Context) error { return nil }
