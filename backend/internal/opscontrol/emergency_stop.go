// Package opscontrol is HAI's always-on runtime control surface (§30/§31). It
// provides a verifiable emergency stop that actually halts background
// processing and survives restart, truthful Docker Desktop dependency handling,
// crash/reboot recovery of stuck operations, and a Windows-runtime readiness
// checklist. It never claims always-on/Windows capability that is not real:
// Windows-specific gates report pending on non-Windows hosts.
package opscontrol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// EmergencyStopState is the persisted emergency-stop state.
type EmergencyStopState struct {
	Engaged   bool       `json:"engaged"`
	Reason    string     `json:"reason,omitempty"`
	Actor     string     `json:"actor,omitempty"`
	EngagedAt *time.Time `json:"engagedAt,omitempty"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// EmergencyStopStore persists the emergency-stop state to a JSON file so the
// halt survives a crash/reboot (recovery-after-restart, §31). Reads/writes are
// serialized.
type EmergencyStopStore struct {
	mu    sync.Mutex
	path  string
	state EmergencyStopState
}

// NewEmergencyStopStore loads (or initializes) the store rooted at dir.
func NewEmergencyStopStore(dir string) *EmergencyStopStore {
	s := &EmergencyStopStore{path: filepath.Join(dir, "emergency_stop.json")}
	s.load()
	return s
}

func (s *EmergencyStopStore) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return // no prior state; default disengaged
	}
	var st EmergencyStopState
	if json.Unmarshal(data, &st) == nil {
		s.state = st
	}
}

func (s *EmergencyStopStore) persist() {
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(s.path), 0o755)
	_ = os.WriteFile(s.path, data, 0o600)
}

// State returns the current emergency-stop state.
func (s *EmergencyStopStore) State() EmergencyStopState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Engaged reports whether the emergency stop is active.
func (s *EmergencyStopStore) Engaged() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.Engaged
}

// Engage activates the emergency stop and persists it.
func (s *EmergencyStopStore) Engage(reason, actor string, now time.Time) EmergencyStopState {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := now.UTC()
	s.state = EmergencyStopState{Engaged: true, Reason: reason, Actor: actor, EngagedAt: &t, UpdatedAt: t}
	s.persist()
	return s.state
}

// Disengage clears the emergency stop and persists it.
func (s *EmergencyStopStore) Disengage(actor string, now time.Time) EmergencyStopState {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = EmergencyStopState{Engaged: false, Actor: actor, UpdatedAt: now.UTC()}
	s.persist()
	return s.state
}
