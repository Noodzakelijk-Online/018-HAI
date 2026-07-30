package modelintelligence

import "fmt"

// ModelMaintenanceGate is implemented by HAI's canonical LLM policy service.
// Model Intelligence is an auxiliary local lane, so it never owns a parallel
// model update policy or lets a benchmark bypass the routing service's daily
// freshness decision.
type ModelMaintenanceGate interface {
	EnsureConfiguredLocalModel(endpointURL, modelID string) error
}

// MaintainedProvider identifies a real local model runtime that must pass the
// canonical daily maintenance check before Model Intelligence can call it.
// Deterministic in-process rules intentionally do not implement this contract:
// there is no downloaded model artifact to update or verify.
type MaintainedProvider interface {
	ModelMaintenanceIdentity() (endpointURL, modelID string, ok bool)
}

// MaintainedLocalProvider preserves the original public interface name for
// integrations compiled against it. The broader name reflects that a
// provider-managed local bridge may expose the same maintenance identity.
type MaintainedLocalProvider = MaintainedProvider

// WithModelMaintenance binds Model Intelligence to the canonical policy gate.
// It is deliberately injected by the router instead of constructing another LLM
// service, keeping maintenance history, update controls, and execution blocks
// in one authoritative place.
func (s *Service) WithModelMaintenance(gate ModelMaintenanceGate) *Service {
	s.mu.Lock()
	s.maintenanceGate = gate
	s.mu.Unlock()
	return s
}

func (s *Service) ensureModelMaintenance(provider Provider) error {
	maintained, ok := provider.(MaintainedProvider)
	if !ok {
		return nil
	}
	endpointURL, modelID, applicable := maintained.ModelMaintenanceIdentity()
	if !applicable {
		return nil
	}
	s.mu.Lock()
	gate := s.maintenanceGate
	s.mu.Unlock()
	if gate == nil {
		return fmt.Errorf("daily model maintenance gate is unavailable for this local model runtime")
	}
	if err := gate.EnsureConfiguredLocalModel(endpointURL, modelID); err != nil {
		return fmt.Errorf("daily model maintenance blocked local model execution: %w", err)
	}
	return nil
}
