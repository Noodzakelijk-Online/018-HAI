package runtimelab

import "automation-hub-backend/internal/executionbroker"

// Registry holds all runtime adapters (§15 required targets).
type Registry struct {
	adapters []Adapter
}

// NewRegistry assembles the required runtime targets:
//   - hai-local-safe-worker (real executor)
//   - Hermes, OpenClaw, Odysseus (external agent runtimes; env-driven, honest)
//   - browser runtime contract, local script runtime contract (contracts only)
func NewRegistry(broker *executionbroker.Broker) *Registry {
	return &Registry{adapters: []Adapter{
		newSafeWorkerRuntime(broker),
		newRemoteRuntime("hermes", "Hermes", "HERMES_BASE_URL"),
		newRemoteRuntime("openclaw", "OpenClaw", "OPENCLAW_BASE_URL"),
		newRemoteRuntime("odysseus", "Odysseus", "ODYSSEUS_BASE_URL"),
		newBrowserContract(),
		newLocalScriptContract(),
	}}
}

// Adapters returns all adapters.
func (r *Registry) Adapters() []Adapter { return r.adapters }

// Adapter returns an adapter by runtime id.
func (r *Registry) Adapter(id string) (Adapter, bool) {
	for _, a := range r.adapters {
		if a.Info().ID == id {
			return a, true
		}
	}
	return nil, false
}
