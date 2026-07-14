package modelintelligence

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// observed accumulates real metrics for a (provider, model) pair.
type observed struct {
	runs      int
	failures  int
	sumTPS    float64
	lastProbe *time.Time
	lastBench *time.Time
	claim     ClaimLevel
}

// Service is the model-intelligence orchestrator. It exposes the registry,
// router, budgets, cache, and telemetry, and runs bounded lane calls that
// record real telemetry. It never fabricates provider state or telemetry.
type Service struct {
	reg       *Registry
	router    *Router
	telemetry *TelemetryStore
	cache     *Cache
	now       func() time.Time

	mu             sync.Mutex
	budgetDefaults OperationBudget
	observedByKey  map[string]*observed
}

// NewService builds a service over a registry.
func NewService(reg *Registry) *Service {
	return &Service{
		reg:            reg,
		router:         NewRouter(reg),
		telemetry:      NewTelemetryStore(),
		cache:          NewCache(),
		now:            time.Now,
		budgetDefaults: DefaultBudget(),
		observedByKey:  map[string]*observed{},
	}
}

// DefaultService builds a service from the environment with durable telemetry
// when a database is available (telemetry survives restart, §18).
func DefaultService() *Service {
	s := NewService(NewRegistryFromEnv())
	if repo := DefaultTelemetryRepository(); repo != nil {
		s.WithTelemetryRepository(repo)
	}
	return s
}

// WithTelemetryRepository seeds the store from durable telemetry and persists
// every future row. Returns the service for chaining.
func (s *Service) WithTelemetryRepository(repo TelemetryRepository) *Service {
	if rows, err := repo.LoadAll(); err == nil {
		s.telemetry.Seed(rows)
	}
	s.telemetry.SetPersist(func(t ModelRunTelemetry) { _ = repo.Save(t) })
	return s
}

// ProviderSummary is a provider's truthful status for the overview.
type ProviderSummary struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Status     ProviderStatus `json:"status"`
	ClaimLevel ClaimLevel     `json:"claimLevel"`
	Local      bool           `json:"local"`
	Models     int            `json:"models"`
}

// Overview is the model-intelligence dashboard roll-up.
type Overview struct {
	Providers     []ProviderSummary `json:"providers"`
	Lanes         []RoutingLane     `json:"lanes"`
	TotalProfiles int               `json:"totalProfiles"`
	ActiveModels  int               `json:"activeModels"`
	TelemetryRuns int               `json:"telemetryRuns"`
	CacheHits     int               `json:"cacheHits"`
	CacheMisses   int               `json:"cacheMisses"`
	LaneWinners   []LaneWinner      `json:"laneWinners"`
}

// Overview returns the dashboard roll-up with truthful provider states.
func (s *Service) Overview() Overview {
	profs := s.Profiles()
	byProvider := map[string]*ProviderSummary{}
	order := []string{}
	active := 0
	for _, prof := range profs {
		ps := byProvider[prof.ProviderID]
		if ps == nil {
			ps = &ProviderSummary{ID: prof.ProviderID, Name: prof.DisplayName, Status: prof.Status, ClaimLevel: prof.ClaimLevel, Local: prof.Local}
			byProvider[prof.ProviderID] = ps
			order = append(order, prof.ProviderID)
		}
		ps.Models++
		if prof.Usable() {
			active++
		}
	}
	providers := make([]ProviderSummary, 0, len(order))
	for _, id := range order {
		providers = append(providers, *byProvider[id])
	}
	hits, misses := s.cache.Stats()
	return Overview{
		Providers:     providers,
		Lanes:         allLanes(),
		TotalProfiles: len(profs),
		ActiveModels:  active,
		TelemetryRuns: len(s.telemetry.All()),
		CacheHits:     hits,
		CacheMisses:   misses,
		LaneWinners:   s.LaneWinners(),
	}
}

// Cache exposes the cache (for wiring/tests).
func (s *Service) Cache() *Cache { return s.cache }

// Telemetry returns all recorded telemetry.
func (s *Service) Telemetry() []ModelRunTelemetry { return s.telemetry.All() }

// LaneWinners returns the fastest observed model per lane.
func (s *Service) LaneWinners() []LaneWinner { return s.telemetry.LaneWinners() }

// Profiles returns every profile merged with observed metrics.
func (s *Service) Profiles() []ModelProfile {
	s.mu.Lock()
	defer s.mu.Unlock()
	profs := s.reg.Profiles()
	for i := range profs {
		if o := s.observedByKey[profs[i].Key()]; o != nil {
			profs[i].ObservedRuns = o.runs
			profs[i].ObservedFailures = o.failures
			if o.runs > 0 {
				profs[i].ObservedTokensPerSecond = o.sumTPS / float64(o.runs)
			}
			profs[i].LastProbedAt = o.lastProbe
			profs[i].LastBenchmarkedAt = o.lastBench
			if o.claim != "" {
				profs[i].ClaimLevel = o.claim
			}
		}
	}
	return profs
}

// Profile returns a single merged profile.
func (s *Service) Profile(providerID, modelID string) (ModelProfile, bool) {
	for _, p := range s.Profiles() {
		if p.ProviderID == providerID && p.ModelID == modelID {
			return p, true
		}
	}
	return ModelProfile{}, false
}

// Probe probes a provider and records the result truthfully.
func (s *Service) Probe(ctx context.Context, providerID string) (ProbeResult, error) {
	p, ok := s.reg.Provider(providerID)
	if !ok {
		return ProbeResult{}, fmt.Errorf("modelintelligence: unknown provider %q", providerID)
	}
	now := s.now().UTC()
	res := p.Probe(ctx, now)
	s.mu.Lock()
	for _, prof := range p.Profiles() {
		o := s.ensureObserved(prof.Key())
		t := now
		o.lastProbe = &t
		if res.Status == ProviderActive && claimRank(o.claim) < claimRank(ClaimProbed) {
			o.claim = ClaimProbed
		}
	}
	s.mu.Unlock()
	return res, nil
}

// BenchmarkResult is the outcome of a bounded benchmark call.
type BenchmarkResult struct {
	ProviderID      string     `json:"providerId"`
	ModelID         string     `json:"modelId"`
	OK              bool       `json:"ok"`
	OutputTokens    int        `json:"outputTokens"`
	DurationMs      int64      `json:"durationMs"`
	TokensPerSecond float64    `json:"tokensPerSecond"`
	ClaimLevel      ClaimLevel `json:"claimLevel"`
	Detail          string     `json:"detail,omitempty"`
}

// Benchmark runs one bounded real call against a model and records telemetry.
// It never fabricates a result: if the provider cannot serve, it returns the
// truthful error and does not promote the claim level.
func (s *Service) Benchmark(ctx context.Context, providerID, modelID string) (BenchmarkResult, error) {
	prof, ok := s.reg.Profile(providerID, modelID)
	if !ok {
		return BenchmarkResult{}, fmt.Errorf("modelintelligence: unknown model %s/%s", providerID, modelID)
	}
	p, _ := s.reg.Provider(providerID)
	now := s.now().UTC()
	lane := LaneFastTriage
	if len(prof.Lanes) > 0 {
		lane = prof.Lanes[0]
	}
	res, err := p.Generate(ctx, InferenceRequest{Lane: lane, Prompt: "CLAIM: benchmark ping\nEVIDENCE: benchmark ping", MaxOutputTokens: 32, Effort: EffortLow}, now)
	s.recordTelemetry(res, lane, "", err == nil, false)
	out := BenchmarkResult{ProviderID: providerID, ModelID: modelID, OK: err == nil}
	if err != nil {
		out.Detail = err.Error()
		out.ClaimLevel = prof.ClaimLevel
		return out, nil // truthful non-fatal: benchmark attempted, provider not usable
	}
	out.OutputTokens = res.OutputTokensEstimate
	out.DurationMs = res.DurationMs
	out.TokensPerSecond = res.TokensPerSecond
	s.mu.Lock()
	o := s.ensureObserved(prof.Key())
	t := now
	o.lastBench = &t
	if claimRank(o.claim) < claimRank(ClaimBenchmarked) {
		o.claim = ClaimBenchmarked
	}
	out.ClaimLevel = o.claim
	s.mu.Unlock()
	return out, nil
}

// RouteLanes classifies the work into lanes and routes each one.
func (s *Service) RouteLanes(in LaneInput) []RouteDecision {
	now := s.now().UTC()
	lanes := ClassifyLanes(in)
	out := make([]RouteDecision, 0, len(lanes))
	for _, lane := range lanes {
		if lane == LanePrivacyFilter {
			continue // handled by the privacyfilter package, not a model route
		}
		out = append(out, s.router.Route(lane, in, now))
	}
	return out
}

// RunLane routes a lane and, if routable, runs a bounded model call, recording
// telemetry and (when safe) caching the result. Returns the decision and, if a
// call was made, the inference result.
func (s *Service) RunLane(ctx context.Context, lane RoutingLane, in LaneInput, prompt, operationID string) (RouteDecision, *InferenceResult, error) {
	now := s.now().UTC()
	dec := s.router.Route(lane, in, now)
	if !dec.Routable {
		return dec, nil, nil
	}
	p, _ := s.reg.Provider(dec.ProviderID)
	res, err := p.Generate(ctx, InferenceRequest{Lane: lane, Prompt: prompt, MaxOutputTokens: 256, Effort: EffortLow}, now)
	s.recordTelemetry(res, lane, operationID, err == nil, false)
	if err != nil {
		return dec, nil, err
	}
	// Cache the deterministic result (safe: local, not high-risk action here).
	s.cache.Store(CacheDeterministicResult, prompt, res.Output, "", res.OK, in.SafeForCloud, in.HighRisk, now)
	return dec, &res, nil
}

// TriageResult is the fast-triage lane's effect on an operation.
type TriageResult struct {
	Category   string `json:"category"`
	Summary    string `json:"summary"`
	ProviderID string `json:"providerId"`
	ModelID    string `json:"modelId"`
	Routed     bool   `json:"routed"`
}

// Triage runs the fast-triage lane over an operation's text so the lane has a
// real behavioral effect (category + summary written back by the caller). It
// honors the privacy filter via SafeForCloud.
func (s *Service) Triage(ctx context.Context, operationType, title, content string, safeForCloud, highRisk bool, operationID string) (TriageResult, error) {
	in := LaneInput{OperationType: operationType, Title: title, Content: content, SafeForCloud: safeForCloud, HighRisk: highRisk}
	prompt := title + "\n" + content
	dec, res, err := s.RunLane(ctx, LaneFastTriage, in, prompt, operationID)
	if err != nil {
		return TriageResult{}, err
	}
	if res == nil {
		return TriageResult{Routed: false}, nil
	}
	category, summary := parseTriageOutput(res.Output)
	return TriageResult{Category: category, Summary: summary, ProviderID: dec.ProviderID, ModelID: dec.ModelID, Routed: true}, nil
}

func parseTriageOutput(out string) (string, string) {
	category, summary := "general", ""
	for _, part := range strings.Split(out, ";") {
		part = strings.TrimSpace(part)
		if v := strings.TrimPrefix(part, "category="); v != part {
			category = v
		} else if v := strings.TrimPrefix(part, "summary="); v != part {
			summary = v
		}
	}
	return category, summary
}

// TokenBudgetDefaults returns the current default budget.
func (s *Service) TokenBudgetDefaults() OperationBudget {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.budgetDefaults
}

// SetTokenBudgetDefaults updates the default budget (validated).
func (s *Service) SetTokenBudgetDefaults(b OperationBudget) (OperationBudget, error) {
	if !b.MaximumReasoning.IsValid() {
		return OperationBudget{}, fmt.Errorf("modelintelligence: invalid reasoning effort %q", b.MaximumReasoning)
	}
	if !b.ContextStrategy.IsValid() {
		return OperationBudget{}, fmt.Errorf("modelintelligence: invalid context strategy %q", b.ContextStrategy)
	}
	if b.MaximumInputTokens <= 0 || b.MaximumOutputTokens <= 0 {
		return OperationBudget{}, fmt.Errorf("modelintelligence: token maxima must be positive")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.budgetDefaults = b
	return s.budgetDefaults, nil
}

func (s *Service) recordTelemetry(res InferenceResult, lane RoutingLane, operationID string, ok, cacheHit bool) {
	s.telemetry.Record(ModelRunTelemetry{
		ProviderID: res.ProviderID, ModelID: res.ModelID, Lane: lane, OperationID: operationID,
		InputTokens: res.InputTokensEstimate, OutputTokens: res.OutputTokensEstimate,
		DurationMs: res.DurationMs, TokensPerSecond: res.TokensPerSecond, OK: ok, CacheHit: cacheHit,
		CreatedAt: s.now().UTC(),
	})
	if res.ProviderID == "" || res.ModelID == "" {
		return
	}
	key := res.ProviderID + "/" + res.ModelID
	s.mu.Lock()
	o := s.ensureObserved(key)
	o.runs++
	if !ok {
		o.failures++
	}
	o.sumTPS += res.TokensPerSecond
	s.mu.Unlock()
}

// ensureObserved must be called with s.mu held.
func (s *Service) ensureObserved(key string) *observed {
	o := s.observedByKey[key]
	if o == nil {
		o = &observed{}
		s.observedByKey[key] = o
	}
	return o
}

func claimRank(c ClaimLevel) int {
	for i, x := range allClaimLevels() {
		if x == c {
			return i
		}
	}
	return -1
}
