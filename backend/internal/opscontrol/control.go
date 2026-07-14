package opscontrol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"automation-hub-backend/internal/autonomypolicy"
)

// Controller is the dynamic background control consulted by the worker each
// pass: current autonomy mode + emergency-stop state. Both survive restart.
type Controller struct {
	mu        sync.Mutex
	emergency *EmergencyStopStore
	modePath  string
	mode      autonomypolicy.Mode
}

// NewController builds a controller rooted at dir (persisting mode + stop).
func NewController(dir string) *Controller {
	c := &Controller{
		emergency: NewEmergencyStopStore(dir),
		modePath:  filepath.Join(dir, "background_mode.json"),
		mode:      autonomypolicy.ModeAutonomousSafe,
	}
	c.loadMode()
	return c
}

func (c *Controller) loadMode() {
	data, err := os.ReadFile(c.modePath)
	if err != nil {
		return
	}
	var wrap struct {
		Mode string `json:"mode"`
	}
	if json.Unmarshal(data, &wrap) == nil {
		if m := autonomypolicy.Mode(wrap.Mode); m.AllowsBackgroundProcessing() || wrap.Mode != "" {
			if _, err := autonomypolicy.ParseMode(wrap.Mode); err == nil {
				c.mode = m
			}
		}
	}
}

func (c *Controller) persistMode() {
	data, _ := json.MarshalIndent(map[string]string{"mode": string(c.mode)}, "", "  ")
	_ = os.MkdirAll(filepath.Dir(c.modePath), 0o755)
	_ = os.WriteFile(c.modePath, data, 0o600)
}

// Mode returns the current autonomy mode. When the emergency stop is engaged the
// effective mode is emergency_stopped regardless of the stored mode.
func (c *Controller) Mode() autonomypolicy.Mode {
	if c.emergency.Engaged() {
		return autonomypolicy.ModeEmergencyStopped
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.mode
}

// StoredMode returns the configured mode ignoring emergency stop.
func (c *Controller) StoredMode() autonomypolicy.Mode {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.mode
}

// EmergencyStop reports whether the emergency stop is engaged (the worker's
// Control contract).
func (c *Controller) EmergencyStop() bool { return c.emergency.Engaged() }

// SetMode updates + persists the autonomy mode.
func (c *Controller) SetMode(m autonomypolicy.Mode) (autonomypolicy.Mode, error) {
	if _, err := autonomypolicy.ParseMode(string(m)); err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mode = m
	c.persistMode()
	return c.mode, nil
}

// Engage activates the emergency stop.
func (c *Controller) Engage(reason, actor string, now time.Time) EmergencyStopState {
	return c.emergency.Engage(reason, actor, now)
}

// Disengage clears the emergency stop.
func (c *Controller) Disengage(actor string, now time.Time) EmergencyStopState {
	return c.emergency.Disengage(actor, now)
}

// EmergencyState returns the persisted emergency-stop state.
func (c *Controller) EmergencyState() EmergencyStopState { return c.emergency.State() }
