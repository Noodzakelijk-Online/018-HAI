package modelintelligence

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// DSpark is an inference backend, not an action runtime (§10.17). This adapter
// is env-driven, validates its endpoint strictly, probes truthfully, and never
// executes actions. It is not_configured unless explicitly enabled with a valid
// base URL.
type DSparkProvider struct {
	enabled    bool
	baseURL    string
	probePath  string
	genPath    string
	modelID    string
	local      bool
	configErr  string
	httpClient *http.Client
}

// DSparkConfig is the resolved DSpark configuration.
type DSparkConfig struct {
	Enabled   bool
	BaseURL   string
	ProbePath string
	GenPath   string
	ModelID   string
}

// DSparkConfigFromEnv reads DSpark config from the environment.
func DSparkConfigFromEnv() DSparkConfig {
	return DSparkConfig{
		Enabled:   strings.EqualFold(strings.TrimSpace(os.Getenv("DSPARK_ENABLED")), "true"),
		BaseURL:   strings.TrimSpace(os.Getenv("DSPARK_BASE_URL")),
		ProbePath: envDefault("DSPARK_PROBE_PATH", "/v1/models"),
		GenPath:   envDefault("DSPARK_GENERATION_PATH", "/v1/chat/completions"),
		ModelID:   envDefault("DSPARK_MODEL_ID", "dspark-default"),
	}
}

// NewDSparkProvider builds a DSpark provider from config, validating the URL.
// Any validation failure yields a not_configured provider carrying the reason.
func NewDSparkProvider(cfg DSparkConfig) *DSparkProvider {
	p := &DSparkProvider{
		enabled:    cfg.Enabled,
		baseURL:    cfg.BaseURL,
		probePath:  firstNonBlank(cfg.ProbePath, "/v1/models"),
		genPath:    firstNonBlank(cfg.GenPath, "/v1/chat/completions"),
		modelID:    firstNonBlank(cfg.ModelID, "dspark-default"),
		local:      isLocalEndpointURL(cfg.BaseURL),
		httpClient: newDirectHTTPClient(5 * time.Second),
	}
	if !cfg.Enabled {
		p.configErr = "DSPARK_ENABLED is false or missing"
		return p
	}
	if cfg.BaseURL == "" {
		p.configErr = "DSPARK_BASE_URL missing"
		return p
	}
	if err := validateDSparkURL(cfg.BaseURL); err != nil {
		p.configErr = err.Error()
	}
	return p
}

func (p *DSparkProvider) ID() string          { return "dspark" }
func (p *DSparkProvider) DisplayName() string { return "DSpark (OpenAI-compatible inference backend)" }

// ModelMaintenanceIdentity returns the only local DSpark model identity HAI
// can call. A remote DSpark endpoint is outside this auxiliary execution lane.
func (p *DSparkProvider) ModelMaintenanceIdentity() (string, string, bool) {
	if !p.local || !p.configured() {
		return "", "", false
	}
	return p.baseURL, p.modelID, true
}

func (p *DSparkProvider) configured() bool { return p.enabled && p.baseURL != "" && p.configErr == "" }

func (p *DSparkProvider) status() ProviderStatus {
	if !p.configured() {
		return ProviderNotConfigured
	}
	return ProviderConfigured // never active without a successful probe
}

func (p *DSparkProvider) Profiles() []ModelProfile {
	return []ModelProfile{{
		ProviderID:         "dspark",
		ModelID:            p.modelID,
		DisplayName:        "DSpark configured model",
		ArchitectureFamily: ArchOpenAICompatibleUnknown,
		Lanes:              []RoutingLane{LaneFastTriage, LaneDrafting, LaneParallelBatch},
		Local:              p.local,
		Paid:               false,
		Status:             p.status(),
		ClaimLevel:         dsparkClaim(p),
	}}
}

func dsparkClaim(p *DSparkProvider) ClaimLevel {
	if !p.configured() {
		return ClaimContractDefined
	}
	return ClaimConfigured
}

func (p *DSparkProvider) Probe(ctx context.Context, now time.Time) ProbeResult {
	if !p.configured() {
		return ProbeResult{ProviderID: p.ID(), Status: ProviderNotConfigured, Detail: p.configErr, CheckedAt: now}
	}
	return probeModelsEndpoint(ctx, p.httpClient, p.ID(), p.baseURL, p.probePath, now)
}

func (p *DSparkProvider) Generate(ctx context.Context, req InferenceRequest, now time.Time) (InferenceResult, error) {
	// Never fabricate output: require a successful probe first. In Phase 2B the
	// provider is only used when configured + probed active by the caller.
	if !p.configured() {
		return InferenceResult{ProviderID: p.ID(), OK: false, Error: p.configErr}, fmt.Errorf("dspark: %s", p.configErr)
	}
	probe := p.Probe(ctx, now)
	if probe.Status != ProviderActive {
		return InferenceResult{ProviderID: p.ID(), OK: false, Error: probe.Detail}, fmt.Errorf("dspark: not active: %s", probe.Detail)
	}
	return chatCompletion(ctx, p.httpClient, p.ID(), p.modelID, p.baseURL, p.genPath, req)
}

// validateDSparkURL rejects invalid, link-local, metadata, and unspecified
// hosts (§10.17). Localhost is allowed (existing local endpoint policy).
func validateDSparkURL(raw string) error {
	if err := validateLocalEndpointURL(raw); err != nil {
		return fmt.Errorf("dspark: %w", err)
	}
	return nil
}

func envDefault(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func firstNonBlank(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
