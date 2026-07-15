package modelintelligence

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// remoteProvider is a generic OpenAI-compatible provider (ollama, lm-studio,
// custom) configured from env. It is not_configured unless a valid base URL is
// set, is never active without a successful probe, and never executes actions.
type remoteProvider struct {
	id         string
	name       string
	baseURL    string
	probePath  string
	genPath    string
	modelID    string
	lanes      []RoutingLane
	arch       ArchitectureFamily
	configErr  string
	httpClient *http.Client
}

func newRemoteProvider(id, name, baseURLEnv, modelID string, arch ArchitectureFamily, lanes []RoutingLane) *remoteProvider {
	p := &remoteProvider{
		id:         id,
		name:       name,
		baseURL:    strings.TrimSpace(os.Getenv(baseURLEnv)),
		probePath:  "/v1/models",
		genPath:    "/v1/chat/completions",
		modelID:    modelID,
		lanes:      lanes,
		arch:       arch,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
	if p.baseURL == "" {
		p.configErr = baseURLEnv + " not set"
	} else if err := validateEndpointURL(p.baseURL); err != nil {
		p.configErr = err.Error()
	}
	return p
}

func (p *remoteProvider) ID() string          { return p.id }
func (p *remoteProvider) DisplayName() string { return p.name }
func (p *remoteProvider) configured() bool    { return p.baseURL != "" && p.configErr == "" }

func (p *remoteProvider) status() ProviderStatus {
	if !p.configured() {
		return ProviderNotConfigured
	}
	return ProviderConfigured
}

func (p *remoteProvider) claim() ClaimLevel {
	if !p.configured() {
		return ClaimContractDefined
	}
	return ClaimConfigured
}

func (p *remoteProvider) Profiles() []ModelProfile {
	return []ModelProfile{{
		ProviderID:         p.id,
		ModelID:            p.modelID,
		DisplayName:        p.name,
		ArchitectureFamily: p.arch,
		Lanes:              p.lanes,
		Local:              true,
		Paid:               false,
		Status:             p.status(),
		ClaimLevel:         p.claim(),
	}}
}

func (p *remoteProvider) Probe(ctx context.Context, now time.Time) ProbeResult {
	if !p.configured() {
		return ProbeResult{ProviderID: p.id, Status: ProviderNotConfigured, Detail: p.configErr, CheckedAt: now}
	}
	return probeModelsEndpoint(ctx, p.httpClient, p.id, p.baseURL, p.probePath, now)
}

func (p *remoteProvider) Generate(ctx context.Context, req InferenceRequest, now time.Time) (InferenceResult, error) {
	if !p.configured() {
		return InferenceResult{ProviderID: p.id, OK: false, Error: p.configErr}, fmt.Errorf("%s: %s", p.id, p.configErr)
	}
	probe := p.Probe(ctx, now)
	if probe.Status != ProviderActive {
		return InferenceResult{ProviderID: p.id, OK: false, Error: probe.Detail}, fmt.Errorf("%s: not active: %s", p.id, probe.Detail)
	}
	return chatCompletion(ctx, p.httpClient, p.id, p.modelID, p.baseURL, p.genPath, req)
}

// Registry holds all configured providers and their profiles (§10.17).
type Registry struct {
	providers []Provider
}

// NewRegistryFromEnv assembles the initial provider set:
//   - test-fast-triage, test-verifier (always active, deterministic, local)
//   - dspark (env, not_configured by default)
//   - ollama, lm-studio, custom-openai-compatible (env, not_configured by default)
func NewRegistryFromEnv() *Registry {
	return &Registry{providers: []Provider{
		&testFastTriageProvider{},
		&testVerifierProvider{},
		NewDSparkProvider(DSparkConfigFromEnv()),
		newRemoteProvider("ollama", "Ollama (local OpenAI-compatible)", "OLLAMA_BASE_URL", "ollama-default", ArchOllamaUnknown, []RoutingLane{LaneFastTriage, LaneDrafting}),
		newRemoteProvider("lm-studio", "LM Studio (local OpenAI-compatible)", "LM_STUDIO_BASE_URL", "lm-studio-default", ArchLocalRuntimeUnknown, []RoutingLane{LaneFastTriage, LaneDrafting}),
		newRemoteProvider("custom-openai-compatible", "Custom OpenAI-compatible", "CUSTOM_OPENAI_BASE_URL", "custom-default", ArchOpenAICompatibleUnknown, []RoutingLane{LaneDrafting, LaneParallelBatch}),
	}}
}

// Providers returns all registered providers.
func (r *Registry) Providers() []Provider { return r.providers }

// Provider returns a provider by id.
func (r *Registry) Provider(id string) (Provider, bool) {
	for _, p := range r.providers {
		if p.ID() == id {
			return p, true
		}
	}
	return nil, false
}

// Profiles returns every model profile across all providers.
func (r *Registry) Profiles() []ModelProfile {
	var out []ModelProfile
	for _, p := range r.providers {
		out = append(out, p.Profiles()...)
	}
	return out
}

// Profile returns a specific profile by provider + model id.
func (r *Registry) Profile(providerID, modelID string) (ModelProfile, bool) {
	for _, prof := range r.Profiles() {
		if prof.ProviderID == providerID && prof.ModelID == modelID {
			return prof, true
		}
	}
	return ModelProfile{}, false
}

// ProfilesForLane returns the usable (active) profiles that serve a lane.
func (r *Registry) ProfilesForLane(lane RoutingLane) []ModelProfile {
	var out []ModelProfile
	for _, prof := range r.Profiles() {
		if prof.ServesLane(lane) && prof.Usable() {
			out = append(out, prof)
		}
	}
	return out
}
