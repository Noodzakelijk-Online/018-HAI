// Package phase2 wires the HAI Phase 2 autonomous back-office control plane
// (Operation Ledger + account feeds + execution broker + background loop) and
// exposes it over HTTP. It is the composition root for the Phase 2A vertical
// slice: real feeds in, real (local, verified) execution out, honest status.
package phase2

import (
	"os"
	"path/filepath"
	"strings"

	"automation-hub-backend/internal/accountfeed"
	"automation-hub-backend/internal/autonomypolicy"
	"automation-hub-backend/internal/background"
	"automation-hub-backend/internal/executionbroker"
	"automation-hub-backend/internal/operations"

	"github.com/google/uuid"
)

// Config configures the Phase 2 module. Zero values fall back to safe defaults.
type Config struct {
	OwnerUserID   string
	WorkspaceID   string
	WorkspaceDir  string   // confined root for the local safe worker
	FeedsDir      string   // directory holding local JSON feed files
	FeedFiles     []string // JSON feed filenames within FeedsDir
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
		Mode:          autonomypolicy.Mode(env("HAI_PHASE2_MODE", string(autonomypolicy.ModeAutonomousSafe))),
		EmergencyStop: env("HAI_PHASE2_EMERGENCY_STOP", "") == "true",
	}
}

// Module is the composition of the Phase 2 services.
type Module struct {
	cfg     Config
	svc     *operations.Service
	broker  *executionbroker.Broker
	worker  *background.Worker
	readers []accountfeed.Reader
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
	return &Module{cfg: cfg, svc: svc, broker: broker, worker: worker, readers: readers}
}

// DefaultModule builds the module from env over the default (DB-backed) service.
func DefaultModule() *Module {
	return NewModule(operations.DefaultService(), ConfigFromEnv())
}

// Service exposes the Operation Ledger service.
func (m *Module) Service() *operations.Service { return m.svc }

// Worker exposes the background worker.
func (m *Module) Worker() *background.Worker { return m.worker }

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
