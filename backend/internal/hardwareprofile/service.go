package hardwareprofile

import (
	"strings"
	"sync"
	"time"
)

// PowerPolicy governs whether heavy model work runs now or is deferred (§18/§19).
type PowerPolicy struct {
	Mode                    string `json:"mode"` // performance | balanced | power_saver
	AllowHeavyWorkNow       bool   `json:"allowHeavyWorkNow"`
	DeferHeavyWorkOnBattery bool   `json:"deferHeavyWorkOnBattery"`
	NightBatchOnly          bool   `json:"nightBatchOnly"`
}

// DefaultPowerPolicy is balanced: heavy work allowed on AC, deferred on battery.
func DefaultPowerPolicy() PowerPolicy {
	return PowerPolicy{Mode: "balanced", AllowHeavyWorkNow: true, DeferHeavyWorkOnBattery: true, NightBatchOnly: false}
}

// AllowsHeavyWorkNow decides if heavy model work may run given the current
// battery status. On battery with defer enabled, heavy work is deferred.
func (p PowerPolicy) AllowsHeavyWorkNow(batteryStatus string) bool {
	if !p.AllowHeavyWorkNow || p.NightBatchOnly {
		return false
	}
	if p.DeferHeavyWorkOnBattery && strings.EqualFold(batteryStatus, "on_battery") {
		return false
	}
	return true
}

// Service stores the current hardware profile + power policy in-process.
type Service struct {
	mu      sync.Mutex
	profile *HardwareProfile
	power   PowerPolicy
	owner   string
	space   string
	now     func() time.Time
}

// NewService builds a service scoped to a single operator/workspace.
func NewService(ownerUserID, workspaceID string) *Service {
	return &Service{owner: ownerUserID, space: workspaceID, power: DefaultPowerPolicy(), now: time.Now}
}

// DefaultService builds a service for the local single operator.
func DefaultService() *Service { return NewService("local-operator", "local") }

// Get returns the current profile, detecting once if none exists.
func (s *Service) Get() HardwareProfile {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.profile == nil {
		p := Detect(s.owner, s.space, s.now().UTC())
		s.profile = &p
	}
	return *s.profile
}

// Detect re-runs truthful detection and stores the result.
func (s *Service) Detect() HardwareProfile {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := Detect(s.owner, s.space, s.now().UTC())
	if s.profile != nil {
		p.CreatedAt = s.profile.CreatedAt
	}
	s.profile = &p
	return p
}

// Patch applies operator-provided overrides (manual config is allowed for
// GPU/NPU/execution providers that cannot be auto-detected). Only provided
// fields are changed.
func (s *Service) Patch(patch HardwareProfilePatch) HardwareProfile {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.profile == nil {
		p := Detect(s.owner, s.space, s.now().UTC())
		s.profile = &p
	}
	if patch.PowerMode != nil {
		s.profile.PowerMode = *patch.PowerMode
	}
	if patch.BatteryStatus != nil {
		s.profile.BatteryStatus = *patch.BatteryStatus
	}
	if patch.GPUVendor != nil {
		s.profile.GPUVendor = *patch.GPUVendor
	}
	if patch.GPUModel != nil {
		s.profile.GPUModel = *patch.GPUModel
	}
	if patch.NPUVendor != nil {
		s.profile.NPUVendor = *patch.NPUVendor
	}
	if patch.NPUTopsDeclared != nil {
		s.profile.NPUTopsDeclared = *patch.NPUTopsDeclared
	}
	if patch.ExecutionProviders != nil {
		s.profile.ExecutionProviders = *patch.ExecutionProviders
	}
	if patch.LocalModelRuntimes != nil {
		s.profile.LocalModelRuntimes = *patch.LocalModelRuntimes
	}
	s.profile.UpdatedAt = s.now().UTC()
	return *s.profile
}

// HardwareProfilePatch carries operator overrides; nil fields are unchanged.
type HardwareProfilePatch struct {
	PowerMode          *string              `json:"powerMode,omitempty"`
	BatteryStatus      *string              `json:"batteryStatus,omitempty"`
	GPUVendor          *string              `json:"gpuVendor,omitempty"`
	GPUModel           *string              `json:"gpuModel,omitempty"`
	NPUVendor          *string              `json:"npuVendor,omitempty"`
	NPUTopsDeclared    *float64             `json:"npuTopsDeclared,omitempty"`
	ExecutionProviders *[]ExecutionProvider `json:"executionProviders,omitempty"`
	LocalModelRuntimes *[]string            `json:"localModelRuntimes,omitempty"`
}

// Power returns the current power policy.
func (s *Service) Power() PowerPolicy {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.power
}

// SetPower updates the power policy.
func (s *Service) SetPower(p PowerPolicy) PowerPolicy {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.Mode == "" {
		p.Mode = "balanced"
	}
	s.power = p
	return s.power
}

// SelectedServingStack returns the serving stack chosen for the current profile.
func (s *Service) SelectedServingStack() ServingStack { return s.Get().SelectServingStack() }
