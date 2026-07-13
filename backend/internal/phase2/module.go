// Package phase2 wires the HAI Phase 2 autonomous back-office control plane
// (Operation Ledger + account feeds + execution broker + background loop) and
// exposes it over HTTP. It is the composition root for the Phase 2A vertical
// slice: real feeds in, real (local, verified) execution out, honest status.
package phase2

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"automation-hub-backend/internal/accountfeed"
	"automation-hub-backend/internal/autonomypolicy"
	"automation-hub-backend/internal/background"
	"automation-hub-backend/internal/executionbroker"
	"automation-hub-backend/internal/modelintelligence"
	"automation-hub-backend/internal/operations"
	"automation-hub-backend/internal/opscontrol"

	"github.com/google/uuid"
)

// Config configures the Phase 2 module. Zero values fall back to safe defaults.
type Config struct {
	OwnerUserID   string
	WorkspaceID   string
	WorkspaceDir  string   // confined root for the local safe worker
	FeedsDir      string   // directory holding local JSON feed files
	FeedFiles     []string // JSON feed filenames within FeedsDir
	StateDir      string   // persisted control state (emergency stop, mode)
	Mode          autonomypolicy.Mode
	EmergencyStop bool
}

// ConfigFromEnv reads the Phase 2 configuration from the environment.
func ConfigFromEnv() Config {
	return Config{
		OwnerUserID:   env("HAI_PHASE2_OWNER", "local-operator"),
		WorkspaceID:   env("HAI_PHASE2_WORKSPACE_ID", "local"),
		WorkspaceDir:  env("HAI_PHASE2_WORKSPACE_DIR", filepath.Join("data", "phase2", "workspace")),
		FeedsDir:      env("HAI_PHASE2_FEEDS_DIR", filepath.Join("data", "phase2", "feeds")),
		FeedFiles:     splitList(env("HAI_PHASE2_FEED_FILES", "inbox.json")),
		StateDir:      env("HAI_PHASE2_STATE_DIR", filepath.Join("data", "phase2", "state")),
		Mode:          autonomypolicy.Mode(env("HAI_PHASE2_MODE", string(autonomypolicy.ModeAutonomousSafe))),
		EmergencyStop: env("HAI_PHASE2_EMERGENCY_STOP", "") == "true",
	}
}

// Module is the composition of the Phase 2 services.
type Module struct {
	cfg        Config
	svc        *operations.Service
	broker     *executionbroker.Broker
	worker     *background.Worker
	readers    []accountfeed.Reader
	blockRules *BlockRuleStore
}

// NewModule wires a module over an operations service and config.
func NewModule(svc *operations.Service, cfg Config) *Module {
	if cfg.WorkspaceID == "" {
		cfg.WorkspaceID = "local"
	}
	if cfg.OwnerUserID == "" {
		cfg.OwnerUserID = "local-operator"
	}
	if cfg.Mode == "" {
		cfg.Mode = autonomypolicy.ModeAutonomousSafe
	}
	broker := executionbroker.NewBroker(cfg.WorkspaceDir)
	readers := buildReaders(cfg)
	worker := background.New(svc, broker, readers, background.Options{
		OwnerUserID:   cfg.OwnerUserID,
		WorkspaceID:   cfg.WorkspaceID,
		Mode:          cfg.Mode,
		EmergencyStop: cfg.EmergencyStop,
	})
	blockRules := NewBlockRuleStore()
	worker.WithBlockRules(blockRules)
	return &Module{cfg: cfg, svc: svc, broker: broker, worker: worker, readers: readers, blockRules: blockRules}
}

// DefaultModule builds the module from env over the default (DB-backed) service.
func DefaultModule() *Module {
	return NewModule(operations.DefaultService(), ConfigFromEnv())
}

// DefaultModuleWithModelIntel builds the default module and attaches a shared
// model-intelligence service so the background loop drives the fast-triage lane
// (its telemetry then surfaces on the model-intelligence dashboard).
func DefaultModuleWithModelIntel(mi *modelintelligence.Service) *Module {
	m := NewModule(operations.DefaultService(), ConfigFromEnv())
	m.worker.WithModelIntelligence(mi)
	return m
}

// Service exposes the Operation Ledger service.
func (m *Module) Service() *operations.Service { return m.svc }

// Broker exposes the execution broker (e.g. for the Runtime Lab).
func (m *Module) Broker() *executionbroker.Broker { return m.broker }

// OwnerUserID returns the configured single operator id.
func (m *Module) OwnerUserID() string { return m.cfg.OwnerUserID }

// WorkspaceID returns the configured workspace id.
func (m *Module) WorkspaceID() string { return m.cfg.WorkspaceID }

// FeedsDir returns the configured feeds directory (allowlisted feed root).
func (m *Module) FeedsDir() string { return m.cfg.FeedsDir }

// FeedFiles returns the configured local feed filenames.
func (m *Module) FeedFiles() []string { return m.cfg.FeedFiles }

// Worker exposes the background worker.
func (m *Module) Worker() *background.Worker { return m.worker }

// OpsControl builds the always-on runtime control service, wires its controller
// into the background worker (so pause/resume/mode take effect live), and
// registers the background runner used by emergency-stop verification.
func (m *Module) OpsControl() *opscontrol.Service {
	svc := opscontrol.NewService(m.cfg.StateDir, m.broker, m.svc, m.cfg.OwnerUserID, m.cfg.WorkspaceID)
	m.worker.WithControl(svc.Control())
	svc.SetBackgroundRunner(func(ctx context.Context) (int, error) {
		rep, err := m.worker.RunOnce(ctx)
		if err != nil {
			return 0, err
		}
		return rep.Classified + rep.AutoExecuted, nil
	})
	return svc
}

// Handler builds the HTTP handler for this module.
func (m *Module) Handler() *Handler { return NewHandler(m) }

// buildReaders constructs a local-file reader for each configured feed file.
func buildReaders(cfg Config) []accountfeed.Reader {
	var readers []accountfeed.Reader
	for _, name := range cfg.FeedFiles {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		feed := accountfeed.Feed{
			ID:           uuid.New(),
			Name:         strings.TrimSuffix(name, filepath.Ext(name)),
			Provider:     "local",
			AccountLabel: name,
			SourceType:   accountfeed.SourceLocalJSONFile,
			Path:         name,
			WorkspaceID:  cfg.WorkspaceID,
			OwnerUserID:  cfg.OwnerUserID,
			Enabled:      true,
		}
		r, err := accountfeed.NewLocalFileReader(feed, cfg.FeedsDir)
		if err != nil {
			continue // an invalid feed name is skipped, not fatal
		}
		readers = append(readers, r)
	}
	return readers
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return def
}

func splitList(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
