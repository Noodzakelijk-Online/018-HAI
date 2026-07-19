package llm

import (
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/safety"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	TierLocal      = "local"
	TierFree       = "free"
	TierCheap      = "cheap"
	TierAcceptable = "acceptable"
	TierHigh       = "high"
	TierPremium    = "premium"
	TierExpensive  = "expensive"
)

var tierRank = map[string]int{
	TierLocal:      0,
	TierFree:       1,
	TierCheap:      2,
	TierAcceptable: 3,
	TierHigh:       4,
	TierPremium:    5,
	TierExpensive:  6,
}

var reasoningRank = map[string]int{
	"low":       1,
	"medium":    2,
	"high":      3,
	"very_high": 4,
}

type Policy struct {
	DailyPaidBudgetEUR               float64                 `json:"dailyPaidBudgetEur"`
	PaidCallsAllowed                 bool                    `json:"paidCallsAllowed"`
	LocalModelsAllowed               bool                    `json:"localModelsAllowed"`
	FreeCloudQuotaAllowed            bool                    `json:"freeCloudQuotaAllowed"`
	LocalFirst                       bool                    `json:"localFirst"`
	CacheRepeatedPrompts             bool                    `json:"cacheRepeatedPrompts"`
	RouteSimpleTasksToSmallModels    bool                    `json:"routeSimpleTasksToSmallModels"`
	RouteComplexTasksToBestFreeModel bool                    `json:"routeComplexTasksToBestAvailableFreeModel"`
	RequireApprovalBeforePaidUsage   bool                    `json:"requireApprovalBeforePaidUsage"`
	RequireRecentLiveProviderProbe   bool                    `json:"requireRecentLiveProviderProbe"`
	ProviderProbeMaxAgeSeconds       int                     `json:"providerProbeMaxAgeSeconds"`
	TierOrder                        []string                `json:"tierOrder"`
	DailyBudgetUsedEUR               float64                 `json:"dailyBudgetUsedEur"`
	InputTokensUsed                  int                     `json:"inputTokensUsed"`
	OutputTokensUsed                 int                     `json:"outputTokensUsed"`
	Providers                        []Provider              `json:"providers"`
	InferenceInfrastructure          InferenceInfrastructure `json:"inferenceInfrastructure"`
}

type InferenceInfrastructure struct {
	KVCacheLoadStrategy             string `json:"kvCacheLoadStrategy"`
	DisaggregatedServingVerified    bool   `json:"disaggregatedServingVerified"`
	DualPathInfrastructureAvailable bool   `json:"dualPathInfrastructureAvailable"`
	Reason                          string `json:"reason"`
}

type Provider struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	Enabled          bool    `json:"enabled"`
	Local            bool    `json:"local"`
	Paid             bool    `json:"paid"`
	EndpointURL      string  `json:"endpointUrl,omitempty"`
	APIKeyEnv        string  `json:"apiKeyEnv,omitempty"`
	Configured       bool    `json:"configured"`
	ReadinessStatus  string  `json:"readinessStatus,omitempty"`
	ReadinessReason  string  `json:"readinessReason,omitempty"`
	QuotaRemaining   int     `json:"quotaRemaining"`
	DailyBudgetEUR   float64 `json:"dailyBudgetEur"`
	BudgetUsedEUR    float64 `json:"budgetUsedEur"`
	InputTokensUsed  int     `json:"inputTokensUsed"`
	OutputTokensUsed int     `json:"outputTokensUsed"`
	Models           []Model `json:"models"`
}

type Model struct {
	ID                            string   `json:"id"`
	Name                          string   `json:"name"`
	Tier                          string   `json:"tier"`
	Capabilities                  []string `json:"capabilities"`
	MaxDifficulty                 int      `json:"maxDifficulty"`
	MaxReasoning                  string   `json:"maxReasoning"`
	EstimatedCostEUR              float64  `json:"estimatedCostEur"`
	InputCostPerMillionTokensEUR  float64  `json:"inputCostPerMillionTokensEur"`
	OutputCostPerMillionTokensEUR float64  `json:"outputCostPerMillionTokensEur"`
	PricingUnit                   string   `json:"pricingUnit,omitempty"`
	PricingSource                 string   `json:"pricingSource,omitempty"`
	RequiresApproval              bool     `json:"requiresApproval"`
	Enabled                       bool     `json:"enabled"`
	BudgetUsedEUR                 float64  `json:"budgetUsedEur"`
	InputTokensUsed               int      `json:"inputTokensUsed"`
	OutputTokensUsed              int      `json:"outputTokensUsed"`
}

type RouteRequest struct {
	Task              string `json:"task"`
	TaskType          string `json:"taskType,omitempty"`
	Difficulty        int    `json:"difficulty,omitempty"`
	RequiredReasoning string `json:"requiredReasoning,omitempty"`
	ValidationPassed  *bool  `json:"validationPassed,omitempty"`
	PreviousModelID   string `json:"previousModelId,omitempty"`
}

type TaskClassification struct {
	TaskType             string   `json:"taskType"`
	Difficulty           int      `json:"difficulty"`
	RequiredReasoning    string   `json:"requiredReasoning"`
	RequiredCapabilities []string `json:"requiredCapabilities"`
	Reason               string   `json:"reason"`
}

type RouteDecision struct {
	SelectedProviderID    string             `json:"selectedProviderId"`
	SelectedModelID       string             `json:"selectedModelId"`
	SelectedModelName     string             `json:"selectedModelName"`
	Tier                  string             `json:"tier"`
	Reason                string             `json:"reason"`
	EstimatedCostEUR      float64            `json:"estimatedCostEur"`
	EstimatedInputTokens  int                `json:"estimatedInputTokens"`
	EstimatedOutputTokens int                `json:"estimatedOutputTokens"`
	PricingSource         string             `json:"pricingSource,omitempty"`
	RequiresApproval      bool               `json:"requiresApproval"`
	Classification        TaskClassification `json:"classification"`
	FallbackPath          []FallbackOption   `json:"fallbackPath"`
	Skipped               []SkippedModel     `json:"skipped"`
	LoggedAt              time.Time          `json:"loggedAt"`
}

type GenerateRequest struct {
	Task              string         `json:"task"`
	SystemPrompt      string         `json:"systemPrompt,omitempty"`
	Context           []string       `json:"context,omitempty"`
	RouteRequest      *RouteRequest  `json:"routeRequest,omitempty"`
	RouteDecision     *RouteDecision `json:"routeDecision,omitempty"`
	AllowPaidApproved bool           `json:"allowPaidApproved,omitempty"`
	Temperature       float64        `json:"temperature,omitempty"`
	MaxTokens         int            `json:"maxTokens,omitempty"`
}

type GenerationResult struct {
	ProviderID       string    `json:"providerId"`
	ModelID          string    `json:"modelId"`
	ModelName        string    `json:"modelName"`
	Tier             string    `json:"tier"`
	Output           string    `json:"output"`
	Status           string    `json:"status"`
	Reason           string    `json:"reason"`
	EstimatedCostEUR float64   `json:"estimatedCostEur"`
	DurationMs       int64     `json:"durationMs"`
	FallbackPath     []string  `json:"fallbackPath"`
	LoggedAt         time.Time `json:"loggedAt"`
}

type ProviderProbeResult struct {
	ProviderID       string     `json:"providerId"`
	ProviderName     string     `json:"providerName"`
	Status           string     `json:"status"`
	Reason           string     `json:"reason"`
	EndpointURL      string     `json:"endpointUrl,omitempty"`
	HTTPStatus       int        `json:"httpStatus,omitempty"`
	ModelsSeen       int        `json:"modelsSeen"`
	DurationMs       int64      `json:"durationMs"`
	Live             bool       `json:"live"`
	RequiresReview   bool       `json:"requiresReview"`
	CheckedAt        time.Time  `json:"checkedAt"`
	LastSuccessfulAt *time.Time `json:"lastSuccessfulAt,omitempty"`
}

type FallbackOption struct {
	ProviderID            string  `json:"providerId"`
	ModelID               string  `json:"modelId"`
	ModelName             string  `json:"modelName"`
	Tier                  string  `json:"tier"`
	EstimatedCostEUR      float64 `json:"estimatedCostEur"`
	EstimatedInputTokens  int     `json:"estimatedInputTokens"`
	EstimatedOutputTokens int     `json:"estimatedOutputTokens"`
	RequiresApproval      bool    `json:"requiresApproval"`
}

type SkippedModel struct {
	ProviderID string `json:"providerId"`
	ModelID    string `json:"modelId"`
	Reason     string `json:"reason"`
}

type Service struct {
	policy       Policy
	mu           sync.Mutex
	logs         []RouteDecision
	usage        map[string]UsageCounter
	probeHistory ProbeHistoryRepository
}

type UsageCounter struct {
	BudgetUsedEUR    float64
	InputTokensUsed  int
	OutputTokensUsed int
}

func NewServiceFromEnv() (*Service, error) {
	return newServiceFromEnv(nil)
}

// NewServiceFromEnvWithProbeHistory keeps live provider probing and durable
// readiness evidence together for the API process. Other in-process users can
// still construct a policy-only service without opening a database connection.
func NewServiceFromEnvWithProbeHistory(probeHistory ProbeHistoryRepository) (*Service, error) {
	return newServiceFromEnv(probeHistory)
}

func newServiceFromEnv(probeHistory ProbeHistoryRepository) (*Service, error) {
	policy := defaultPolicy()
	if raw := strings.TrimSpace(os.Getenv("LLM_PROVIDERS_JSON")); raw != "" {
		var providers []Provider
		if err := json.Unmarshal([]byte(raw), &providers); err != nil {
			return nil, fmt.Errorf("invalid LLM_PROVIDERS_JSON: %w", err)
		}
		policy.Providers = providers
	}

	if raw := strings.TrimSpace(os.Getenv("LLM_POLICY_JSON")); raw != "" {
		if err := json.Unmarshal([]byte(raw), &policy); err != nil {
			return nil, fmt.Errorf("invalid LLM_POLICY_JSON: %w", err)
		}
	}

	policy = annotateInfrastructure(annotatePolicyReadiness(normalizeProbePolicy(policy)))
	return &Service{policy: policy, logs: []RouteDecision{}, usage: map[string]UsageCounter{}, probeHistory: probeHistory}, nil
}

func (s *Service) Policy() Policy {
	return s.annotateUsage(annotateInfrastructure(annotatePolicyReadiness(normalizeProbePolicy(s.policy))))
}

func annotateInfrastructure(policy Policy) Policy {
	strategy := strings.ToLower(strings.TrimSpace(os.Getenv("LLM_KV_CACHE_LOAD_STRATEGY")))
	switch strategy {
	case "traditional", "storage_to_prefill", "storage_to_decode", "dual", "auto":
	default:
		strategy = "disabled"
	}
	verified := envEnabled("LLM_DUALPATH_INFRASTRUCTURE_VERIFIED")
	available := verified && (strategy == "dual" || strategy == "auto")
	reason := "DualPath is disabled; ordinary local model servers use their native KV-cache path."
	if strategy != "disabled" && !verified {
		reason = "KV-cache strategy is configured as a capability hint, but disaggregated prefill/decode, shared storage, transfer network, and scheduler infrastructure are not verified."
	}
	if available {
		reason = "Operator reports verified DualPath-compatible disaggregated serving infrastructure; provider/runtime telemetry must still confirm the selected data path."
	}
	policy.InferenceInfrastructure = InferenceInfrastructure{
		KVCacheLoadStrategy:             strategy,
		DisaggregatedServingVerified:    verified,
		DualPathInfrastructureAvailable: available,
		Reason:                          reason,
	}
	return policy
}

func envEnabled(name string) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return value == "1" || value == "true" || value == "yes"
}

func (s *Service) Logs() []RouteDecision {
	s.mu.Lock()
	defer s.mu.Unlock()

	copied := make([]RouteDecision, len(s.logs))
	copy(copied, s.logs)
	return copied
}

func (s *Service) ProbeProviders() []ProviderProbeResult {
	results := []ProviderProbeResult{}
	for _, provider := range s.Policy().Providers {
		results = append(results, probeProvider(provider, s.policy))
	}
	return results
}

// ProbeAndRecordProviders executes the same bounded live checks as
// ProbeProviders and persists only redacted readiness evidence when a history
// repository is configured.
func (s *Service) ProbeAndRecordProviders() ([]ProviderProbeResult, error) {
	results := s.ProbeProviders()
	if s.probeHistory == nil {
		return results, nil
	}
	recorded := make([]ProviderProbeResult, 0, len(results))
	for _, result := range results {
		probe, err := s.probeHistory.RecordProviderProbe(providerProbeRecord(result))
		if err != nil {
			return nil, fmt.Errorf("record provider probe for %s: %w", result.ProviderID, err)
		}
		recorded = append(recorded, providerProbeResult(*probe))
	}
	return recorded, nil
}

func (s *Service) ProviderProbeHistory(limit int) ([]ProviderProbeResult, error) {
	if s.probeHistory == nil {
		return []ProviderProbeResult{}, nil
	}
	probes, err := s.probeHistory.FindRecentProviderProbes(limit)
	if err != nil {
		return nil, fmt.Errorf("load provider probe history: %w", err)
	}
	results := make([]ProviderProbeResult, 0, len(probes))
	for _, probe := range probes {
		results = append(results, providerProbeResult(probe))
	}
	return results, nil
}

func providerProbeRecord(result ProviderProbeResult) *models.LLMProviderProbe {
	return &models.LLMProviderProbe{
		ProviderID:       result.ProviderID,
		ProviderName:     result.ProviderName,
		Status:           result.Status,
		Reason:           safety.RedactSecrets(result.Reason),
		EndpointURL:      safety.RedactURL(result.EndpointURL),
		HTTPStatus:       result.HTTPStatus,
		ModelsSeen:       result.ModelsSeen,
		DurationMs:       result.DurationMs,
		Live:             result.Live,
		RequiresReview:   result.RequiresReview,
		CheckedAt:        result.CheckedAt,
		LastSuccessfulAt: result.LastSuccessfulAt,
	}
}

func providerProbeResult(probe models.LLMProviderProbe) ProviderProbeResult {
	return ProviderProbeResult{
		ProviderID:       probe.ProviderID,
		ProviderName:     probe.ProviderName,
		Status:           probe.Status,
		Reason:           probe.Reason,
		EndpointURL:      probe.EndpointURL,
		HTTPStatus:       probe.HTTPStatus,
		ModelsSeen:       probe.ModelsSeen,
		DurationMs:       probe.DurationMs,
		Live:             probe.Live,
		RequiresReview:   probe.RequiresReview,
		CheckedAt:        probe.CheckedAt,
		LastSuccessfulAt: probe.LastSuccessfulAt,
	}
}

func (s *Service) Route(request RouteRequest) (RouteDecision, error) {
	classification := classifyTask(request)
	candidates, skipped := s.candidates(classification, request)
	estimatedInputTokens := estimateTokens(request.Task)
	estimatedOutputTokens := estimateRouteOutputTokens(classification)
	if len(candidates) == 0 {
		decision := RouteDecision{
			Reason:                "No enabled model satisfies the task, budget, quota, and approval policy.",
			EstimatedInputTokens:  estimatedInputTokens,
			EstimatedOutputTokens: estimatedOutputTokens,
			Classification:        classification,
			Skipped:               skipped,
			LoggedAt:              time.Now().UTC(),
		}
		s.addLog(decision)
		return decision, nil
	}

	selected := candidates[0]
	estimatedCostEUR := estimateRouteCostEUR(selected.model, estimatedInputTokens, estimatedOutputTokens)
	decision := RouteDecision{
		SelectedProviderID:    selected.provider.ID,
		SelectedModelID:       selected.model.ID,
		SelectedModelName:     selected.model.Name,
		Tier:                  selected.model.Tier,
		Reason:                selectionReason(selected, classification, estimatedInputTokens, estimatedOutputTokens, estimatedCostEUR),
		EstimatedCostEUR:      estimatedCostEUR,
		EstimatedInputTokens:  estimatedInputTokens,
		EstimatedOutputTokens: estimatedOutputTokens,
		PricingSource:         selected.model.PricingSource,
		RequiresApproval:      selected.model.RequiresApproval || selected.provider.Paid,
		Classification:        classification,
		FallbackPath:          fallbackPath(candidates[1:], estimatedInputTokens, estimatedOutputTokens),
		Skipped:               skipped,
		LoggedAt:              time.Now().UTC(),
	}

	s.addLog(decision)

	return decision, nil
}

func (s *Service) Generate(request GenerateRequest) (*GenerationResult, error) {
	started := time.Now().UTC()
	if safety.EmergencyStopActive() {
		return &GenerationResult{
			Status:     "blocked",
			Reason:     safety.EmergencyStopReason(),
			DurationMs: time.Since(started).Milliseconds(),
			LoggedAt:   time.Now().UTC(),
		}, nil
	}
	decision := request.RouteDecision
	if decision == nil || decision.SelectedModelID == "" {
		routeRequest := RouteRequest{Task: request.Task}
		if request.RouteRequest != nil {
			routeRequest = *request.RouteRequest
		}
		routed, err := s.Route(routeRequest)
		if err != nil {
			return nil, err
		}
		decision = &routed
	}
	if decision.SelectedModelID == "" {
		return &GenerationResult{
			Status:       "skipped",
			Reason:       "no model was selected by the routing policy",
			DurationMs:   time.Since(started).Milliseconds(),
			FallbackPath: fallbackLabels(decision.FallbackPath),
			LoggedAt:     time.Now().UTC(),
		}, nil
	}

	provider, model, ok := s.findProviderModel(decision.SelectedProviderID, decision.SelectedModelID)
	if !ok {
		return nil, fmt.Errorf("selected provider/model not found: %s/%s", decision.SelectedProviderID, decision.SelectedModelID)
	}
	if (provider.Paid || model.EstimatedCostEUR > 0 || model.Tier == TierExpensive || model.RequiresApproval) && !request.AllowPaidApproved {
		return &GenerationResult{
			ProviderID:       provider.ID,
			ModelID:          model.ID,
			ModelName:        model.Name,
			Tier:             model.Tier,
			Status:           "blocked",
			Reason:           "paid or approval-required model execution is disabled until manually approved",
			EstimatedCostEUR: model.EstimatedCostEUR,
			DurationMs:       time.Since(started).Milliseconds(),
			FallbackPath:     fallbackLabels(decision.FallbackPath),
			LoggedAt:         time.Now().UTC(),
		}, nil
	}
	endpoint := strings.TrimRight(strings.TrimSpace(provider.EndpointURL), "/")
	readiness := providerRuntimeReadiness(provider)
	if !readiness.configured {
		return &GenerationResult{
			ProviderID:       provider.ID,
			ModelID:          model.ID,
			ModelName:        model.Name,
			Tier:             model.Tier,
			Status:           generationStatusForReadiness(readiness.status),
			Reason:           readiness.reason,
			EstimatedCostEUR: model.EstimatedCostEUR,
			DurationMs:       time.Since(started).Milliseconds(),
			FallbackPath:     fallbackLabels(decision.FallbackPath),
			LoggedAt:         time.Now().UTC(),
		}, nil
	}
	if strictReason := s.strictProbeReason(provider, normalizeProbePolicy(s.policy), time.Now().UTC()); strictReason != "" {
		return &GenerationResult{
			ProviderID:       provider.ID,
			ModelID:          model.ID,
			ModelName:        model.Name,
			Tier:             model.Tier,
			Status:           "skipped",
			Reason:           strictReason,
			EstimatedCostEUR: model.EstimatedCostEUR,
			DurationMs:       time.Since(started).Milliseconds(),
			FallbackPath:     fallbackLabels(decision.FallbackPath),
			LoggedAt:         time.Now().UTC(),
		}, nil
	}
	if provider.ID == "odysseus" {
		return &GenerationResult{
			ProviderID:       provider.ID,
			ModelID:          model.ID,
			ModelName:        model.Name,
			Tier:             model.Tier,
			Status:           "blocked",
			Reason:           odysseusExecutionBlockedReason(),
			EstimatedCostEUR: model.EstimatedCostEUR,
			DurationMs:       time.Since(started).Milliseconds(),
			FallbackPath:     fallbackLabels(decision.FallbackPath),
			LoggedAt:         time.Now().UTC(),
		}, nil
	}
	if provider.ID == "litellm" {
		probe := probeProvider(provider, s.policy)
		if !probe.Live {
			return &GenerationResult{
				ProviderID:       provider.ID,
				ModelID:          model.ID,
				ModelName:        model.Name,
				Tier:             model.Tier,
				Status:           "skipped",
				Reason:           "LiteLLM gateway must pass a live authenticated /v1/models probe before generation: " + probe.Reason,
				EstimatedCostEUR: model.EstimatedCostEUR,
				DurationMs:       time.Since(started).Milliseconds(),
				FallbackPath:     fallbackLabels(decision.FallbackPath),
				LoggedAt:         time.Now().UTC(),
			}, nil
		}
	}

	output, err := s.callProvider(context.Background(), provider, model, endpoint, request)
	if err != nil {
		return &GenerationResult{
			ProviderID:       provider.ID,
			ModelID:          model.ID,
			ModelName:        model.Name,
			Tier:             model.Tier,
			Status:           "failed",
			Reason:           safety.RedactSecrets(err.Error()),
			EstimatedCostEUR: model.EstimatedCostEUR,
			DurationMs:       time.Since(started).Milliseconds(),
			FallbackPath:     fallbackLabels(decision.FallbackPath),
			LoggedAt:         time.Now().UTC(),
		}, nil
	}
	inputTokens := estimateTokens(buildPrompt(request))
	outputTokens := estimateTokens(output)
	actualCostEUR := estimateModelUsageCostEUR(model, inputTokens, outputTokens)
	if actualCostEUR == 0 {
		actualCostEUR = model.EstimatedCostEUR
	}
	s.addUsage(provider.ID, model.ID, actualCostEUR, inputTokens, outputTokens)
	return &GenerationResult{
		ProviderID:       provider.ID,
		ModelID:          model.ID,
		ModelName:        model.Name,
		Tier:             model.Tier,
		Output:           safety.RedactSecrets(strings.TrimSpace(output)),
		Status:           "completed",
		Reason:           "model endpoint returned a draft; verification must still ground important claims",
		EstimatedCostEUR: actualCostEUR,
		DurationMs:       time.Since(started).Milliseconds(),
		FallbackPath:     fallbackLabels(decision.FallbackPath),
		LoggedAt:         time.Now().UTC(),
	}, nil
}

func (s *Service) findProviderModel(providerID, modelID string) (Provider, Model, bool) {
	for _, provider := range s.policy.Providers {
		if provider.ID != providerID {
			continue
		}
		for _, model := range provider.Models {
			if model.ID == modelID {
				return provider, model, true
			}
		}
	}
	return Provider{}, Model{}, false
}

func (s *Service) callProvider(ctx context.Context, provider Provider, model Model, endpoint string, request GenerateRequest) (string, error) {
	timeout := intEnv("LLM_GENERATION_TIMEOUT_SECONDS", 60)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	prompt := buildPrompt(request)
	switch provider.ID {
	case "ollama":
		return callOllama(ctx, endpoint, model.ID, prompt, request)
	case "odysseus":
		return "", fmt.Errorf(odysseusExecutionBlockedReason())
	default:
		return callOpenAICompatible(ctx, endpoint, provider, model.ID, prompt, request)
	}
}

func probeProvider(provider Provider, policy Policy) ProviderProbeResult {
	started := time.Now().UTC()
	result := ProviderProbeResult{
		ProviderID:   provider.ID,
		ProviderName: provider.Name,
		EndpointURL:  safety.RedactURL(provider.EndpointURL),
		CheckedAt:    started,
	}
	readiness := providerRuntimeReadiness(provider)
	if !readiness.configured {
		result.Status = readiness.status
		result.Reason = readiness.reason
		result.RequiresReview = readiness.status == "blocked_endpoint" || readiness.status == "invalid_endpoint"
		return result
	}
	if provider.Paid && (!policy.PaidCallsAllowed || policy.RequireApprovalBeforePaidUsage) {
		result.Status = "blocked"
		result.Reason = "paid provider probe is blocked until server-side paid approval exists"
		result.RequiresReview = true
		return result
	}
	if provider.ID == "odysseus" {
		return probeOdysseusProvider(provider, result, started)
	}

	endpoint := strings.TrimRight(strings.TrimSpace(provider.EndpointURL), "/")
	probePath := "/v1/models"
	if provider.ID == "ollama" {
		probePath = "/api/tags"
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(intEnv("LLM_PROVIDER_PROBE_TIMEOUT_SECONDS", 5))*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+probePath, nil)
	if err != nil {
		result.Status = "failed"
		result.Reason = safety.RedactSecrets(err.Error())
		return result
	}
	req.Header.Set("User-Agent", "018-HAI-Provider-Probe/1.0")
	if provider.APIKeyEnv != "" {
		if key := strings.TrimSpace(os.Getenv(provider.APIKeyEnv)); key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
	}
	resp, err := noRedirectHTTPClient().Do(req)
	result.DurationMs = time.Since(started).Milliseconds()
	if err != nil {
		result.Status = "failed"
		result.Reason = safety.RedactSecrets(err.Error())
		return result
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	result.HTTPStatus = resp.StatusCode
	if resp.StatusCode >= 300 {
		result.Status = "failed"
		result.Reason = fmt.Sprintf("probe returned HTTP %d: %s", resp.StatusCode, safety.RedactSecrets(compactOutput(raw, 300)))
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			result.RequiresReview = true
		}
		return result
	}
	result.ModelsSeen = countProbeModels(provider, raw)
	result.Live = true
	result.Status = "live"
	if result.ModelsSeen > 0 {
		result.Reason = fmt.Sprintf("provider endpoint responded and reported %d model(s)", result.ModelsSeen)
	} else {
		result.Reason = "provider endpoint responded; model count was not available"
	}
	return result
}

func probeOdysseusProvider(provider Provider, result ProviderProbeResult, started time.Time) ProviderProbeResult {
	endpoint := strings.TrimRight(strings.TrimSpace(provider.EndpointURL), "/")
	paths := []string{"/api/health", "/health", "/api/v1/health", "/"}
	lastReason := ""

	for _, path := range paths {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(intEnv("LLM_PROVIDER_PROBE_TIMEOUT_SECONDS", 5))*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+path, nil)
		if err != nil {
			cancel()
			result.Status = "failed"
			result.Reason = safety.RedactSecrets(err.Error())
			return result
		}
		req.Header.Set("User-Agent", "018-HAI-Odysseus-Probe/1.0")
		if provider.APIKeyEnv != "" {
			if key := strings.TrimSpace(os.Getenv(provider.APIKeyEnv)); key != "" {
				req.Header.Set("Authorization", "Bearer "+key)
			}
		}
		resp, err := noRedirectHTTPClient().Do(req)
		result.DurationMs = time.Since(started).Milliseconds()
		cancel()
		if err != nil {
			lastReason = safety.RedactSecrets(err.Error())
			continue
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
		_ = resp.Body.Close()
		result.HTTPStatus = resp.StatusCode
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			result.Status = "auth_required"
			result.Reason = fmt.Sprintf("Odysseus endpoint responded with HTTP %d at %s; configure ODYSSEUS_API_TOKEN if auth is enabled", resp.StatusCode, path)
			result.RequiresReview = true
			return result
		}
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			result.Status = "failed"
			result.Reason = fmt.Sprintf("Odysseus probe at %s returned redirect HTTP %d; redirects are not followed", path, resp.StatusCode)
			result.RequiresReview = true
			return result
		}
		if resp.StatusCode < 300 {
			result.Live = true
			result.Status = "live"
			result.ModelsSeen = countProbeModels(provider, raw)
			result.Reason = fmt.Sprintf("Odysseus workspace is reachable via %s; workspace-agent execution remains approval-gated and disabled until a reviewed task adapter exists", path)
			return result
		}
		lastReason = fmt.Sprintf("Odysseus probe at %s returned HTTP %d: %s", path, resp.StatusCode, safety.RedactSecrets(compactOutput(raw, 300)))
	}

	result.Status = "failed"
	result.Reason = firstNonEmpty(lastReason, "Odysseus endpoint did not respond on known health paths")
	return result
}

func countProbeModels(provider Provider, raw []byte) int {
	if provider.ID == "ollama" {
		var decoded struct {
			Models []interface{} `json:"models"`
		}
		if err := json.Unmarshal(raw, &decoded); err == nil {
			return len(decoded.Models)
		}
		return 0
	}
	var decoded struct {
		Data []interface{} `json:"data"`
	}
	if err := json.Unmarshal(raw, &decoded); err == nil {
		return len(decoded.Data)
	}
	return 0
}

func callOllama(ctx context.Context, endpoint, modelID, prompt string, request GenerateRequest) (string, error) {
	payload := map[string]interface{}{
		"model":  modelID,
		"prompt": prompt,
		"stream": false,
	}
	if request.Temperature > 0 {
		payload["options"] = map[string]interface{}{"temperature": request.Temperature}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := noRedirectHTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("ollama returned HTTP %d: %s", resp.StatusCode, compactOutput(raw, 500))
	}
	var decoded struct {
		Response string `json:"response"`
		Error    string `json:"error"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return "", err
	}
	if decoded.Error != "" {
		return "", fmt.Errorf("%s", decoded.Error)
	}
	return decoded.Response, nil
}

func callOpenAICompatible(ctx context.Context, endpoint string, provider Provider, modelID, prompt string, request GenerateRequest) (string, error) {
	maxTokens := request.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 800
	}
	payload := map[string]interface{}{
		"model": modelID,
		"messages": []map[string]string{
			{"role": "system", "content": firstNonEmpty(request.SystemPrompt, "You are a careful local-first assistant. Use only provided context when factual grounding is required.")},
			{"role": "user", "content": prompt},
		},
		"temperature": request.Temperature,
		"max_tokens":  maxTokens,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if provider.APIKeyEnv != "" {
		if key := strings.TrimSpace(os.Getenv(provider.APIKeyEnv)); key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
	}
	resp, err := noRedirectHTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("openai-compatible endpoint returned HTTP %d: %s", resp.StatusCode, compactOutput(raw, 500))
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error interface{} `json:"error"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return "", err
	}
	if decoded.Error != nil {
		return "", fmt.Errorf("endpoint returned error: %v", decoded.Error)
	}
	if len(decoded.Choices) == 0 {
		return "", fmt.Errorf("endpoint returned no choices")
	}
	return decoded.Choices[0].Message.Content, nil
}

func (s *Service) addLog(decision RouteDecision) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logs = append([]RouteDecision{decision}, s.logs...)
	if len(s.logs) > 50 {
		s.logs = s.logs[:50]
	}
}

func (s *Service) addUsage(providerID, modelID string, budgetEUR float64, inputTokens, outputTokens int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.usage == nil {
		s.usage = map[string]UsageCounter{}
	}
	current := s.usage[providerID]
	current.BudgetUsedEUR += budgetEUR
	current.InputTokensUsed += inputTokens
	current.OutputTokensUsed += outputTokens
	s.usage[providerID] = current
	if strings.TrimSpace(modelID) != "" {
		modelUsage := s.usage[usageKey(providerID, modelID)]
		modelUsage.BudgetUsedEUR += budgetEUR
		modelUsage.InputTokensUsed += inputTokens
		modelUsage.OutputTokensUsed += outputTokens
		s.usage[usageKey(providerID, modelID)] = modelUsage
	}
}

func (s *Service) annotateUsage(policy Policy) Policy {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range policy.Providers {
		usage := s.usage[policy.Providers[index].ID]
		policy.Providers[index].BudgetUsedEUR = usage.BudgetUsedEUR
		policy.Providers[index].InputTokensUsed = usage.InputTokensUsed
		policy.Providers[index].OutputTokensUsed = usage.OutputTokensUsed
		policy.DailyBudgetUsedEUR += usage.BudgetUsedEUR
		policy.InputTokensUsed += usage.InputTokensUsed
		policy.OutputTokensUsed += usage.OutputTokensUsed
		for modelIndex := range policy.Providers[index].Models {
			model := &policy.Providers[index].Models[modelIndex]
			modelUsage := s.usage[usageKey(policy.Providers[index].ID, model.ID)]
			model.BudgetUsedEUR = modelUsage.BudgetUsedEUR
			model.InputTokensUsed = modelUsage.InputTokensUsed
			model.OutputTokensUsed = modelUsage.OutputTokensUsed
		}
	}
	return policy
}

func usageKey(providerID, modelID string) string {
	return providerID + "\x00" + modelID
}

func estimateModelUsageCostEUR(model Model, inputTokens, outputTokens int) float64 {
	return costFromMillionTokenPrice(model.InputCostPerMillionTokensEUR, inputTokens) +
		costFromMillionTokenPrice(model.OutputCostPerMillionTokensEUR, outputTokens)
}

func estimateRouteCostEUR(model Model, inputTokens, outputTokens int) float64 {
	cost := estimateModelUsageCostEUR(model, inputTokens, outputTokens)
	if cost > 0 {
		return cost
	}
	return model.EstimatedCostEUR
}

func estimateRouteOutputTokens(classification TaskClassification) int {
	reasoningMultiplier := map[string]int{
		"low":       1,
		"medium":    2,
		"high":      3,
		"very_high": 4,
	}
	multiplier := reasoningMultiplier[classification.RequiredReasoning]
	if multiplier == 0 {
		multiplier = 2
	}
	estimate := 180 + (classification.Difficulty * 120 * multiplier)
	if hasCapability(classification.RequiredCapabilities, "coding") {
		estimate += 500
	}
	if hasCapability(classification.RequiredCapabilities, "verification") {
		estimate += 350
	}
	if estimate > 4000 {
		return 4000
	}
	return estimate
}

func costFromMillionTokenPrice(priceEUR float64, tokens int) float64 {
	if priceEUR <= 0 || tokens <= 0 {
		return 0
	}
	return priceEUR * float64(tokens) / 1_000_000
}

func estimateTokens(value string) int {
	words := len(strings.Fields(value))
	chars := len([]rune(value))
	byChars := chars / 4
	if chars%4 != 0 {
		byChars++
	}
	return maxInt(words, byChars)
}

type candidate struct {
	provider Provider
	model    Model
}

func (s *Service) candidates(classification TaskClassification, request RouteRequest) ([]candidate, []SkippedModel) {
	candidates := []candidate{}
	skipped := []SkippedModel{}
	policy := normalizeProbePolicy(s.policy)

	for _, provider := range policy.Providers {
		if !provider.Enabled {
			for _, model := range provider.Models {
				skipped = append(skipped, SkippedModel{ProviderID: provider.ID, ModelID: model.ID, Reason: "provider disabled"})
			}
			continue
		}
		if provider.Local && !policy.LocalModelsAllowed {
			for _, model := range provider.Models {
				skipped = append(skipped, SkippedModel{ProviderID: provider.ID, ModelID: model.ID, Reason: "local models disabled by policy"})
			}
			continue
		}
		if provider.Paid && (!policy.PaidCallsAllowed || policy.DailyPaidBudgetEUR <= 0) {
			for _, model := range provider.Models {
				skipped = append(skipped, SkippedModel{ProviderID: provider.ID, ModelID: model.ID, Reason: "paid usage disabled by policy"})
			}
			continue
		}
		if !provider.Local && !provider.Paid && provider.QuotaRemaining <= 0 && !policy.FreeCloudQuotaAllowed {
			for _, model := range provider.Models {
				skipped = append(skipped, SkippedModel{ProviderID: provider.ID, ModelID: model.ID, Reason: "free cloud quota unavailable"})
			}
			continue
		}
		if !provider.Local && !provider.Paid && provider.QuotaRemaining == 0 {
			for _, model := range provider.Models {
				skipped = append(skipped, SkippedModel{ProviderID: provider.ID, ModelID: model.ID, Reason: "free cloud quota exhausted or unknown"})
			}
			continue
		}
		readiness := providerRuntimeReadiness(provider)
		if !readiness.configured {
			for _, model := range provider.Models {
				skipped = append(skipped, SkippedModel{ProviderID: provider.ID, ModelID: model.ID, Reason: readiness.reason})
			}
			continue
		}
		if strictReason := s.strictProbeReason(provider, policy, time.Now().UTC()); strictReason != "" {
			for _, model := range provider.Models {
				skipped = append(skipped, SkippedModel{ProviderID: provider.ID, ModelID: model.ID, Reason: strictReason})
			}
			continue
		}
		for _, model := range provider.Models {
			reason := unsuitableReason(provider, model, classification, policy, request)
			if reason != "" {
				skipped = append(skipped, SkippedModel{ProviderID: provider.ID, ModelID: model.ID, Reason: reason})
				continue
			}
			candidates = append(candidates, candidate{provider: provider, model: model})
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		left := candidates[i]
		right := candidates[j]
		if policy.LocalFirst && left.provider.Local != right.provider.Local {
			return left.provider.Local
		}
		if tierRank[left.model.Tier] != tierRank[right.model.Tier] {
			return tierRank[left.model.Tier] < tierRank[right.model.Tier]
		}
		if left.model.EstimatedCostEUR != right.model.EstimatedCostEUR {
			return left.model.EstimatedCostEUR < right.model.EstimatedCostEUR
		}
		return left.model.ID < right.model.ID
	})

	return candidates, skipped
}

// strictProbeReason keeps optional strict routing fail-closed. It deliberately
// checks the latest result, not merely a historical success, because a later
// failed probe is fresh evidence that the endpoint is unavailable.
func (s *Service) strictProbeReason(provider Provider, policy Policy, now time.Time) string {
	if !policy.RequireRecentLiveProviderProbe {
		return ""
	}
	if s.probeHistory == nil {
		return "strict live-probe policy cannot verify provider readiness"
	}
	probe, err := s.probeHistory.FindLatestProviderProbe(provider.ID)
	if err != nil {
		return "strict live-probe policy could not read provider readiness history"
	}
	if probe == nil {
		return "provider has not passed a persisted live readiness check"
	}
	if !probe.Live {
		return "provider latest readiness check is not live"
	}
	if now.Sub(probe.CheckedAt.UTC()) > time.Duration(policy.ProviderProbeMaxAgeSeconds)*time.Second {
		return "provider live readiness check is stale"
	}
	return ""
}

func unsuitableReason(provider Provider, model Model, classification TaskClassification, policy Policy, request RouteRequest) string {
	if !model.Enabled {
		return "model disabled"
	}
	if request.ValidationPassed != nil && !*request.ValidationPassed && request.PreviousModelID == model.ID {
		return "previous validation failed"
	}
	if model.MaxDifficulty > 0 && model.MaxDifficulty < classification.Difficulty {
		return "model difficulty limit too low"
	}
	if reasoningRank[model.MaxReasoning] < reasoningRank[classification.RequiredReasoning] {
		return "model reasoning level too weak"
	}
	for _, capability := range classification.RequiredCapabilities {
		if !hasCapability(model.Capabilities, capability) {
			return "missing capability: " + capability
		}
	}
	if provider.Paid && !policy.PaidCallsAllowed {
		return "paid usage disabled by policy"
	}
	if (provider.Paid || model.Tier == TierExpensive) && policy.RequireApprovalBeforePaidUsage {
		return "manual approval required before paid or expensive usage"
	}
	return ""
}

func classifyTask(request RouteRequest) TaskClassification {
	task := strings.ToLower(request.Task)
	taskType := firstNonEmpty(request.TaskType, "general")
	difficulty := request.Difficulty
	reasoning := request.RequiredReasoning
	capabilities := []string{"general"}
	reasons := []string{}

	if strings.Contains(task, "code") || strings.Contains(task, "bug") || strings.Contains(task, "compile") || strings.Contains(task, "api") {
		taskType = firstNonEmpty(request.TaskType, "coding")
		capabilities = append(capabilities, "coding")
		difficulty = maxInt(difficulty, 3)
		reasoning = maxReasoning(reasoning, "medium")
		reasons = append(reasons, "coding terms detected")
	}
	if strings.Contains(task, "architecture") || strings.Contains(task, "multi-agent") || strings.Contains(task, "autonomous") {
		taskType = firstNonEmpty(request.TaskType, "architecture")
		capabilities = append(capabilities, "planning")
		difficulty = maxInt(difficulty, 4)
		reasoning = maxReasoning(reasoning, "high")
		reasons = append(reasons, "architecture terms detected")
	}
	if strings.Contains(task, "legal") || strings.Contains(task, "financial") || strings.Contains(task, "medical") {
		taskType = firstNonEmpty(request.TaskType, "high_stakes")
		capabilities = append(capabilities, "verification")
		difficulty = maxInt(difficulty, 5)
		reasoning = maxReasoning(reasoning, "very_high")
		reasons = append(reasons, "high-stakes terms detected")
	}
	if strings.Contains(task, "summarize") || strings.Contains(task, "extract") || strings.Contains(task, "classify") {
		taskType = firstNonEmpty(request.TaskType, "extraction")
		capabilities = append(capabilities, "extraction")
		difficulty = maxInt(difficulty, 1)
		reasoning = maxReasoning(reasoning, "low")
		reasons = append(reasons, "simple extraction/classification terms detected")
	}
	if len(task) > 800 {
		difficulty = maxInt(difficulty, 4)
		reasoning = maxReasoning(reasoning, "high")
		reasons = append(reasons, "long task context")
	}
	if difficulty == 0 {
		difficulty = 2
	}
	if reasoning == "" {
		reasoning = "medium"
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "default general task classification")
	}

	return TaskClassification{
		TaskType:             taskType,
		Difficulty:           difficulty,
		RequiredReasoning:    reasoning,
		RequiredCapabilities: uniqueStrings(capabilities),
		Reason:               strings.Join(reasons, "; "),
	}
}

func selectionReason(selected candidate, classification TaskClassification, inputTokens, outputTokens int, estimatedCostEUR float64) string {
	return fmt.Sprintf(
		"Selected the cheapest suitable %s-tier model after classifying the task as %s difficulty %d with %s reasoning. The model satisfies required capabilities: %s. Estimated route size: %d input / %d output tokens, EUR %.6f.",
		selected.model.Tier,
		classification.TaskType,
		classification.Difficulty,
		classification.RequiredReasoning,
		strings.Join(classification.RequiredCapabilities, ", "),
		inputTokens,
		outputTokens,
		estimatedCostEUR,
	)
}

func fallbackPath(candidates []candidate, inputTokens, outputTokens int) []FallbackOption {
	path := []FallbackOption{}
	for _, candidate := range candidates {
		path = append(path, FallbackOption{
			ProviderID:            candidate.provider.ID,
			ModelID:               candidate.model.ID,
			ModelName:             candidate.model.Name,
			Tier:                  candidate.model.Tier,
			EstimatedCostEUR:      estimateRouteCostEUR(candidate.model, inputTokens, outputTokens),
			EstimatedInputTokens:  inputTokens,
			EstimatedOutputTokens: outputTokens,
			RequiresApproval:      candidate.provider.Paid || candidate.model.RequiresApproval,
		})
	}
	return path
}

func fallbackLabels(options []FallbackOption) []string {
	labels := []string{}
	for _, option := range options {
		labels = append(labels, option.ProviderID+"/"+option.ModelID)
	}
	return labels
}

type providerReadiness struct {
	configured bool
	status     string
	reason     string
}

func annotatePolicyReadiness(policy Policy) Policy {
	for index := range policy.Providers {
		readiness := providerRuntimeReadiness(policy.Providers[index])
		policy.Providers[index].Configured = readiness.configured
		policy.Providers[index].ReadinessStatus = readiness.status
		policy.Providers[index].ReadinessReason = readiness.reason
	}
	return policy
}

func providerRuntimeReadiness(provider Provider) providerReadiness {
	if !provider.Enabled {
		return providerReadiness{configured: false, status: "disabled", reason: "provider disabled"}
	}
	endpoint := strings.TrimSpace(provider.EndpointURL)
	if endpoint == "" {
		return providerReadiness{configured: false, status: "not_configured", reason: "provider endpoint is not configured"}
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return providerReadiness{configured: false, status: "invalid_endpoint", reason: "provider endpoint must be an absolute http or https URL"}
	}
	host := parsed.Hostname()
	if unsafeEndpointHost(host) && strings.ToLower(strings.TrimSpace(os.Getenv("LLM_ALLOW_LINK_LOCAL_ENDPOINTS"))) != "true" {
		return providerReadiness{configured: false, status: "blocked_endpoint", reason: "provider endpoint uses link-local, metadata, or unspecified address space"}
	}
	if isLoopbackOnlyProvider(provider.ID) && !isLocalModelHost(host) {
		return providerReadiness{configured: false, status: "blocked_endpoint", reason: localProviderDisplayName(provider.ID) + " endpoint must use localhost, loopback, or host.docker.internal"}
	}
	if provider.APIKeyEnv != "" && strings.TrimSpace(os.Getenv(provider.APIKeyEnv)) == "" {
		return providerReadiness{configured: false, status: "missing_api_key", reason: "required API key environment variable " + provider.APIKeyEnv + " is not set"}
	}
	return providerReadiness{configured: true, status: "configured", reason: "provider endpoint and required credentials are configured"}
}

func isLoopbackOnlyProvider(providerID string) bool {
	switch providerID {
	case "ollama", "lm-studio", "llama-cpp", "localai", "vllm", "litellm":
		return true
	default:
		return false
	}
}

func localProviderDisplayName(providerID string) string {
	switch providerID {
	case "ollama":
		return "Ollama"
	case "lm-studio":
		return "LM Studio"
	case "llama-cpp":
		return "llama.cpp"
	case "localai":
		return "LocalAI"
	case "vllm":
		return "vLLM"
	case "litellm":
		return "LiteLLM gateway"
	default:
		return "local provider"
	}
}

func generationStatusForReadiness(status string) string {
	switch status {
	case "blocked_endpoint":
		return "blocked"
	default:
		return "skipped"
	}
}

func unsafeEndpointHost(host string) bool {
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil {
		return false
	}
	return ip.IsUnspecified() || ip.IsLinkLocalUnicast()
}

// isLocalModelHost allows a local server on the host OS when HAI itself runs
// in Docker. It intentionally rejects LAN and public endpoints for llama.cpp:
// this provider is the explicit local/offline route, not a cloud gateway.
func isLocalModelHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "localhost" || host == "host.docker.internal" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func noRedirectHTTPClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func buildPrompt(request GenerateRequest) string {
	parts := []string{}
	if request.SystemPrompt != "" {
		parts = append(parts, request.SystemPrompt)
	}
	if len(request.Context) > 0 {
		parts = append(parts, "Relevant context:")
		for _, item := range request.Context {
			item = strings.TrimSpace(item)
			if item != "" {
				parts = append(parts, "- "+item)
			}
		}
	}
	parts = append(parts, "Task:")
	parts = append(parts, strings.TrimSpace(request.Task))
	return strings.Join(parts, "\n")
}

func compactOutput(value []byte, limit int) string {
	text := strings.Join(strings.Fields(safety.RedactSecrets(string(value))), " ")
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}

func intEnv(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func normalizeProbePolicy(policy Policy) Policy {
	if policy.ProviderProbeMaxAgeSeconds <= 0 {
		policy.ProviderProbeMaxAgeSeconds = 900
	}
	return policy
}

func defaultPolicy() Policy {
	ollamaEndpoint := strings.TrimSpace(os.Getenv("OLLAMA_BASE_URL"))
	lmStudioEndpoint := strings.TrimSpace(os.Getenv("LM_STUDIO_BASE_URL"))
	llamaCPPEndpoint := strings.TrimSpace(os.Getenv("LLAMA_CPP_BASE_URL"))
	llamaCPPModelID := strings.TrimSpace(os.Getenv("LLAMA_CPP_MODEL_ID"))
	if llamaCPPModelID == "" {
		llamaCPPModelID = "local-model"
	}
	localAIEndpoint := strings.TrimSpace(os.Getenv("LOCALAI_BASE_URL"))
	localAIModelID := strings.TrimSpace(os.Getenv("LOCALAI_MODEL_ID"))
	if localAIModelID == "" {
		localAIModelID = "localai-default"
	}
	vLLMEndpoint := strings.TrimSpace(os.Getenv("VLLM_BASE_URL"))
	vLLMModelID := strings.TrimSpace(os.Getenv("VLLM_MODEL_ID"))
	if vLLMModelID == "" {
		vLLMModelID = "vllm-default"
	}
	liteLLMEnabled := envEnabled("LITELLM_ENABLED")
	liteLLMEndpoint := strings.TrimSpace(os.Getenv("LITELLM_BASE_URL"))
	liteLLMModelID := strings.TrimSpace(os.Getenv("LITELLM_MODEL_ID"))
	if liteLLMModelID == "" {
		liteLLMModelID = "local-model"
	}
	odysseusEndpoint := strings.TrimSpace(os.Getenv("ODYSSEUS_BASE_URL"))
	odysseusAPIKeyEnv := ""
	if strings.TrimSpace(os.Getenv("ODYSSEUS_API_TOKEN")) != "" {
		odysseusAPIKeyEnv = "ODYSSEUS_API_TOKEN"
	}
	freeCloudEndpoint := strings.TrimSpace(os.Getenv("FREE_CLOUD_OPENAI_BASE_URL"))
	nousEndpoint := strings.TrimSpace(os.Getenv("NOUS_PORTAL_BASE_URL"))
	nousAPIKeyEnv := ""
	if strings.TrimSpace(os.Getenv("NOUS_PORTAL_API_KEY")) != "" {
		nousAPIKeyEnv = "NOUS_PORTAL_API_KEY"
	}
	mixtureEndpoint := strings.TrimSpace(os.Getenv("MIXTURE_OF_AGENTS_BASE_URL"))
	mixtureAPIKeyEnv := ""
	if strings.TrimSpace(os.Getenv("MIXTURE_OF_AGENTS_API_KEY")) != "" {
		mixtureAPIKeyEnv = "MIXTURE_OF_AGENTS_API_KEY"
	}
	openAICodexEndpoint := strings.TrimSpace(os.Getenv("OPENAI_CODEX_BASE_URL"))
	openAICodexAPIKeyEnv := ""
	if strings.TrimSpace(os.Getenv("OPENAI_CODEX_API_KEY")) != "" {
		openAICodexAPIKeyEnv = "OPENAI_CODEX_API_KEY"
	}
	return Policy{
		DailyPaidBudgetEUR:               0,
		PaidCallsAllowed:                 false,
		LocalModelsAllowed:               true,
		FreeCloudQuotaAllowed:            true,
		LocalFirst:                       true,
		CacheRepeatedPrompts:             true,
		RouteSimpleTasksToSmallModels:    true,
		RouteComplexTasksToBestFreeModel: true,
		RequireApprovalBeforePaidUsage:   true,
		RequireRecentLiveProviderProbe:   envEnabled("LLM_REQUIRE_RECENT_LIVE_PROBE"),
		ProviderProbeMaxAgeSeconds:       intEnv("LLM_PROVIDER_PROBE_MAX_AGE_SECONDS", 900),
		TierOrder:                        []string{TierLocal, TierFree, TierCheap, TierAcceptable, TierHigh, TierPremium, TierExpensive},
		Providers: []Provider{
			{
				ID:             "ollama",
				Name:           "Ollama local",
				Enabled:        true,
				Local:          true,
				Paid:           false,
				EndpointURL:    ollamaEndpoint,
				QuotaRemaining: -1,
				Models: []Model{
					{ID: "phi3:mini", Name: "Phi small local", Tier: TierLocal, Capabilities: []string{"general", "classification", "extraction"}, MaxDifficulty: 2, MaxReasoning: "low", Enabled: true},
					{ID: "phi4:latest", Name: "Phi general local", Tier: TierLocal, Capabilities: []string{"general", "classification", "extraction"}, MaxDifficulty: 3, MaxReasoning: "medium", Enabled: true},
					{ID: "gemma3:4b", Name: "Gemma compact local", Tier: TierLocal, Capabilities: []string{"general", "classification", "summarization"}, MaxDifficulty: 3, MaxReasoning: "medium", Enabled: true},
					{ID: "gemma3:12b", Name: "Gemma capable local", Tier: TierLocal, Capabilities: []string{"general", "planning", "extraction"}, MaxDifficulty: 4, MaxReasoning: "high", Enabled: true},
					{ID: "mistral:7b", Name: "Mistral local", Tier: TierLocal, Capabilities: []string{"general", "planning", "summarization"}, MaxDifficulty: 4, MaxReasoning: "high", Enabled: true},
					{ID: "mixtral:8x7b", Name: "Mixtral local", Tier: TierLocal, Capabilities: []string{"general", "planning", "verification"}, MaxDifficulty: 5, MaxReasoning: "very_high", Enabled: true},
					{ID: "llama3.1:8b", Name: "Llama local", Tier: TierLocal, Capabilities: []string{"general", "planning", "extraction"}, MaxDifficulty: 4, MaxReasoning: "high", Enabled: true},
					{ID: "llama3.1:70b", Name: "Llama large local", Tier: TierLocal, Capabilities: []string{"general", "planning", "verification"}, MaxDifficulty: 5, MaxReasoning: "very_high", Enabled: true},
					{ID: "qwen2.5:7b", Name: "Qwen general local", Tier: TierLocal, Capabilities: []string{"general", "planning", "extraction"}, MaxDifficulty: 4, MaxReasoning: "high", Enabled: true},
					{ID: "qwen2.5-coder:7b", Name: "Qwen coder local", Tier: TierLocal, Capabilities: []string{"general", "coding", "extraction"}, MaxDifficulty: 4, MaxReasoning: "high", Enabled: true},
					{ID: "qwen2.5-coder:32b", Name: "Qwen coder large local", Tier: TierLocal, Capabilities: []string{"general", "coding", "planning", "verification"}, MaxDifficulty: 5, MaxReasoning: "very_high", Enabled: true},
					{ID: "deepseek-coder:6.7b", Name: "DeepSeek coder local", Tier: TierLocal, Capabilities: []string{"general", "coding", "planning"}, MaxDifficulty: 4, MaxReasoning: "high", Enabled: true},
					{ID: "deepseek-r1:8b", Name: "DeepSeek reasoning local", Tier: TierLocal, Capabilities: []string{"general", "planning", "verification"}, MaxDifficulty: 4, MaxReasoning: "high", Enabled: true},
				},
			},
			{
				ID:             "lm-studio",
				Name:           "LM Studio local server",
				Enabled:        true,
				Local:          true,
				Paid:           false,
				EndpointURL:    lmStudioEndpoint,
				QuotaRemaining: -1,
				Models: []Model{
					{ID: "openai-compatible-local", Name: "OpenAI-compatible local endpoint", Tier: TierLocal, Capabilities: []string{"general", "coding", "planning", "extraction"}, MaxDifficulty: 4, MaxReasoning: "high", Enabled: true},
					{ID: "local-small", Name: "Configured small local model", Tier: TierLocal, Capabilities: []string{"general", "classification", "extraction"}, MaxDifficulty: 2, MaxReasoning: "low", Enabled: true},
					{ID: "local-best", Name: "Configured best local model", Tier: TierLocal, Capabilities: []string{"general", "coding", "planning", "verification", "extraction"}, MaxDifficulty: 5, MaxReasoning: "very_high", Enabled: true},
				},
			},
			{
				ID:             "llama-cpp",
				Name:           "llama.cpp local server",
				Enabled:        true,
				Local:          true,
				Paid:           false,
				EndpointURL:    llamaCPPEndpoint,
				QuotaRemaining: -1,
				Models: []Model{
					{ID: llamaCPPModelID, Name: "Configured llama.cpp GGUF model", Tier: TierLocal, Capabilities: []string{"general", "coding", "planning", "verification", "extraction"}, MaxDifficulty: 5, MaxReasoning: "very_high", Enabled: true},
				},
			},
			{
				ID:             "localai",
				Name:           "LocalAI local server",
				Enabled:        true,
				Local:          true,
				Paid:           false,
				EndpointURL:    localAIEndpoint,
				QuotaRemaining: -1,
				Models: []Model{
					{ID: localAIModelID, Name: "Configured LocalAI local model", Tier: TierLocal, Capabilities: []string{"general", "coding", "planning", "verification", "extraction"}, MaxDifficulty: 5, MaxReasoning: "very_high", Enabled: true},
				},
			},
			{
				ID:             "vllm",
				Name:           "vLLM local server",
				Enabled:        true,
				Local:          true,
				Paid:           false,
				EndpointURL:    vLLMEndpoint,
				QuotaRemaining: -1,
				Models: []Model{
					{ID: vLLMModelID, Name: "Configured vLLM local model", Tier: TierLocal, Capabilities: []string{"general", "coding", "planning", "verification", "extraction"}, MaxDifficulty: 5, MaxReasoning: "very_high", Enabled: true},
				},
			},
			{
				ID:             "litellm",
				Name:           "LiteLLM local gateway",
				Enabled:        liteLLMEnabled,
				Local:          true,
				Paid:           false,
				EndpointURL:    liteLLMEndpoint,
				APIKeyEnv:      "LITELLM_API_KEY",
				QuotaRemaining: -1,
				Models: []Model{
					{ID: liteLLMModelID, Name: "Configured LiteLLM local model alias", Tier: TierLocal, Capabilities: []string{"general", "coding", "planning", "verification", "extraction"}, MaxDifficulty: 5, MaxReasoning: "very_high", RequiresApproval: true, Enabled: true},
				},
			},
			{
				ID:             "odysseus",
				Name:           "Odysseus AI Workspace",
				Enabled:        true,
				Local:          true,
				Paid:           false,
				EndpointURL:    odysseusEndpoint,
				APIKeyEnv:      odysseusAPIKeyEnv,
				QuotaRemaining: -1,
				Models: []Model{
					{ID: "odysseus-workspace-agent", Name: "Odysseus workspace agent", Tier: TierLocal, Capabilities: []string{"general", "planning", "verification", "extraction"}, MaxDifficulty: 4, MaxReasoning: "high", RequiresApproval: true, Enabled: true},
				},
			},
			{
				ID:             "free-cloud",
				Name:           "Configured free cloud quota",
				Enabled:        false,
				Local:          false,
				Paid:           false,
				EndpointURL:    freeCloudEndpoint,
				APIKeyEnv:      "FREE_CLOUD_API_KEY",
				QuotaRemaining: 0,
				Models: []Model{
					{ID: "free-best-available", Name: "Best configured free model", Tier: TierFree, Capabilities: []string{"general", "coding", "planning", "verification", "extraction"}, MaxDifficulty: 5, MaxReasoning: "very_high", Enabled: true},
					{ID: "free-fast-classifier", Name: "Free fast classifier", Tier: TierFree, Capabilities: []string{"general", "classification", "extraction"}, MaxDifficulty: 2, MaxReasoning: "low", Enabled: true},
					{ID: "free-coder", Name: "Free coder quota model", Tier: TierFree, Capabilities: []string{"general", "coding", "extraction"}, MaxDifficulty: 4, MaxReasoning: "high", Enabled: true},
				},
			},
			{
				ID:             "nous-portal",
				Name:           "Nous Portal",
				Enabled:        true,
				Local:          false,
				Paid:           true,
				EndpointURL:    nousEndpoint,
				APIKeyEnv:      nousAPIKeyEnv,
				QuotaRemaining: 0,
				DailyBudgetEUR: 0,
				Models:         nousPortalModels(),
			},
			{
				ID:             "mixture-of-agents",
				Name:           "Mixture of Agents",
				Enabled:        true,
				Local:          false,
				Paid:           true,
				EndpointURL:    mixtureEndpoint,
				APIKeyEnv:      mixtureAPIKeyEnv,
				QuotaRemaining: 0,
				DailyBudgetEUR: 0,
				Models: []Model{
					catalogModel("moa-default", "Default", TierPremium, []string{"general", "coding", "planning", "verification", "extraction"}, 5, "very_high", 2, 8, true, "default estimate for multi-agent orchestration; verify with provider invoice or override through LLM_PROVIDERS_JSON"),
				},
			},
			{
				ID:             "openai-codex",
				Name:           "OpenAI Codex",
				Enabled:        true,
				Local:          false,
				Paid:           true,
				EndpointURL:    openAICodexEndpoint,
				APIKeyEnv:      openAICodexAPIKeyEnv,
				QuotaRemaining: 0,
				DailyBudgetEUR: 0,
				Models: []Model{
					catalogModel("gpt-5.5", "GPT-5.5", TierPremium, []string{"general", "coding", "planning", "verification", "extraction"}, 5, "very_high", 5, 15, true, "default estimate; verify with OpenAI/Codex invoice or override through LLM_PROVIDERS_JSON"),
					catalogModel("gpt-5.4", "GPT-5.4", TierHigh, []string{"general", "coding", "planning", "verification", "extraction"}, 5, "very_high", 3, 10, true, "default estimate; verify with OpenAI/Codex invoice or override through LLM_PROVIDERS_JSON"),
					catalogModel("gpt-5.4-mini", "GPT-5.4-mini", TierCheap, []string{"general", "coding", "planning", "extraction"}, 4, "high", 0.25, 1, true, "default estimate; verify with OpenAI/Codex invoice or override through LLM_PROVIDERS_JSON"),
					catalogModel("gpt-5.3-codex-spark", "GPT-5.3-codex-spark", TierCheap, []string{"general", "coding", "extraction"}, 4, "high", 0.15, 0.6, true, "default estimate; verify with OpenAI/Codex invoice or override through LLM_PROVIDERS_JSON"),
				},
			},
			{
				ID:             "cheap-provider",
				Name:           "Cheap API provider",
				Enabled:        false,
				Local:          false,
				Paid:           true,
				QuotaRemaining: 0,
				DailyBudgetEUR: 0,
				Models: []Model{
					{ID: "cheap-small", Name: "Cheap small model", Tier: TierCheap, Capabilities: []string{"general", "classification", "extraction"}, MaxDifficulty: 3, MaxReasoning: "medium", EstimatedCostEUR: 0.001, RequiresApproval: true, Enabled: true},
					{ID: "cheap-coder", Name: "Cheap coder model", Tier: TierCheap, Capabilities: []string{"general", "coding", "extraction"}, MaxDifficulty: 4, MaxReasoning: "high", EstimatedCostEUR: 0.003, RequiresApproval: true, Enabled: true},
				},
			},
			{
				ID:             "acceptable-provider",
				Name:           "Acceptable API provider",
				Enabled:        false,
				Local:          false,
				Paid:           true,
				QuotaRemaining: 0,
				DailyBudgetEUR: 0,
				Models: []Model{
					{ID: "acceptable-general", Name: "Acceptable general model", Tier: TierAcceptable, Capabilities: []string{"general", "planning", "extraction"}, MaxDifficulty: 4, MaxReasoning: "high", EstimatedCostEUR: 0.01, RequiresApproval: true, Enabled: true},
					{ID: "acceptable-coder", Name: "Acceptable coder model", Tier: TierAcceptable, Capabilities: []string{"general", "coding", "planning"}, MaxDifficulty: 4, MaxReasoning: "high", EstimatedCostEUR: 0.015, RequiresApproval: true, Enabled: true},
				},
			},
			{
				ID:             "high-provider",
				Name:           "High capability API provider",
				Enabled:        false,
				Local:          false,
				Paid:           true,
				QuotaRemaining: 0,
				DailyBudgetEUR: 0,
				Models: []Model{
					{ID: "high-reasoning", Name: "High reasoning model", Tier: TierHigh, Capabilities: []string{"general", "coding", "planning", "verification", "extraction"}, MaxDifficulty: 5, MaxReasoning: "very_high", EstimatedCostEUR: 0.03, RequiresApproval: true, Enabled: true},
				},
			},
			{
				ID:             "premium-provider",
				Name:           "Premium API provider",
				Enabled:        false,
				Local:          false,
				Paid:           true,
				QuotaRemaining: 0,
				DailyBudgetEUR: 0,
				Models: []Model{
					{ID: "premium-best", Name: "Premium best-available model", Tier: TierPremium, Capabilities: []string{"general", "coding", "planning", "verification", "extraction"}, MaxDifficulty: 5, MaxReasoning: "very_high", EstimatedCostEUR: 0.04, RequiresApproval: true, Enabled: true},
				},
			},
			{
				ID:             "paid-provider",
				Name:           "Paid provider placeholder",
				Enabled:        false,
				Local:          false,
				Paid:           true,
				QuotaRemaining: 0,
				DailyBudgetEUR: 0,
				Models: []Model{
					{ID: "paid-high-capability", Name: "Paid high capability model", Tier: TierExpensive, Capabilities: []string{"general", "coding", "planning", "verification", "extraction"}, MaxDifficulty: 5, MaxReasoning: "very_high", EstimatedCostEUR: 0.05, RequiresApproval: true, Enabled: true},
					{ID: "expensive-frontier", Name: "Expensive frontier model", Tier: TierExpensive, Capabilities: []string{"general", "coding", "planning", "verification", "extraction"}, MaxDifficulty: 5, MaxReasoning: "very_high", EstimatedCostEUR: 0.08, RequiresApproval: true, Enabled: true},
				},
			},
		},
	}
}

func nousPortalModels() []Model {
	frontier := []string{"general", "coding", "planning", "verification", "extraction"}
	reasoning := []string{"general", "planning", "verification", "extraction"}
	coding := []string{"general", "coding", "planning", "extraction"}
	fast := []string{"general", "classification", "extraction", "summarization"}

	return []Model{
		nousModel("opus-4.8", "Opus 4.8", TierExpensive, frontier, 5, "very_high", 15, 75),
		nousModel("sonnet-4.6", "Sonnet 4.6", TierHigh, frontier, 5, "very_high", 3, 15),
		nousModel("haiku-4.5", "Haiku 4.5", TierAcceptable, fast, 3, "medium", 0.8, 4),
		nousModel("gpt-5.5", "GPT-5.5", TierPremium, frontier, 5, "very_high", 5, 15),
		nousModel("gpt-5.5-pro", "GPT-5.5-pro", TierExpensive, frontier, 5, "very_high", 15, 60),
		nousModel("gpt-5.4-mini", "GPT-5.4-mini", TierCheap, coding, 3, "medium", 0.25, 1),
		nousModel("gemini-3-pro-preview", "Gemini 3 pro Preview", TierPremium, reasoning, 5, "very_high", 2.5, 15),
		nousModel("gemini-3.1-pro-preview", "Gemini 3.1 pro Preview", TierHigh, reasoning, 5, "very_high", 1.25, 10),
		nousModel("gemini-3.5-flash", "Gemini 3.5 flash", TierCheap, fast, 3, "medium", 0.3, 2.5),
		nousModel("grok-4.3", "Grok 4.3", TierHigh, frontier, 5, "very_high", 3, 15),
		nousModel("deepseek-v4-pro", "Deepseek V4 Pro", TierHigh, frontier, 5, "very_high", 0.55, 2.2),
		nousModel("deepseek-v4-flash", "Deepseek V4 Flash", TierCheap, coding, 4, "high", 0.14, 0.28),
		nousModel("qwen3.7-max", "Qwen3.7 Max", TierPremium, frontier, 5, "very_high", 1.6, 6.4),
		nousModel("qwen3.7-plus", "Qwen3.7 Plus", TierAcceptable, coding, 4, "high", 0.4, 1.2),
		nousModel("qwen3.6-35b-a3b", "Qwen3.6 35b A3b", TierCheap, coding, 4, "high", 0.2, 0.8),
		nousModel("kimi-k2.7-code", "Kimi K2.7 Code", TierHigh, coding, 5, "very_high", 0.6, 2.4),
		nousModel("minimax-m3", "Minimax M3", TierHigh, frontier, 5, "very_high", 0.5, 2),
		nousModel("glm-5.2", "Glm 5.2", TierHigh, reasoning, 5, "very_high", 0.5, 2),
		nousModel("glm-5.1", "Glm 5.1", TierAcceptable, reasoning, 4, "high", 0.25, 1),
		nousModel("mimo-v2.5-pro", "Mimo V2.5 Pro", TierHigh, frontier, 5, "very_high", 0.8, 3.2),
		nousModel("hy3-preview", "Hy3 Preview", TierPremium, reasoning, 5, "very_high", 1, 4),
		nousModel("step-3.7-flash", "Step 3.7 Flash", TierCheap, fast, 3, "medium", 0.2, 0.8),
		nousModel("nemotron-3-super-120b-a12b", "Nemotron 3 Super 120b A12b", TierPremium, frontier, 5, "very_high", 0.8, 3.2),
		catalogModel("step-3.7-flash-free", "Step 3.7 Flash:Free", TierFree, fast, 3, "medium", 0, 0, true, "free quota estimate; verify availability and quota with Nous Portal or override through LLM_PROVIDERS_JSON"),
	}
}

func nousModel(id, name, tier string, capabilities []string, maxDifficulty int, maxReasoning string, inputCostPerMillion, outputCostPerMillion float64) Model {
	return catalogModel(id, name, tier, capabilities, maxDifficulty, maxReasoning, inputCostPerMillion, outputCostPerMillion, true, "default estimate; verify with provider invoice or override through LLM_PROVIDERS_JSON")
}

func catalogModel(id, name, tier string, capabilities []string, maxDifficulty int, maxReasoning string, inputCostPerMillion, outputCostPerMillion float64, requiresApproval bool, pricingSource string) Model {
	return Model{
		ID:                            id,
		Name:                          name,
		Tier:                          tier,
		Capabilities:                  capabilities,
		MaxDifficulty:                 maxDifficulty,
		MaxReasoning:                  maxReasoning,
		EstimatedCostEUR:              estimateDefaultRouteCostEUR(inputCostPerMillion, outputCostPerMillion),
		InputCostPerMillionTokensEUR:  inputCostPerMillion,
		OutputCostPerMillionTokensEUR: outputCostPerMillion,
		PricingUnit:                   "EUR per 1M tokens",
		PricingSource:                 pricingSource,
		RequiresApproval:              requiresApproval,
		Enabled:                       true,
	}
}

func estimateDefaultRouteCostEUR(inputCostPerMillion, outputCostPerMillion float64) float64 {
	return costFromMillionTokenPrice(inputCostPerMillion, 1000) + costFromMillionTokenPrice(outputCostPerMillion, 1000)
}

func odysseusExecutionBlockedReason() string {
	return "Odysseus workspace is connected for discovery and health probing only; agent task execution must go through a reviewed controlled runtime adapter and approval queue before HAI may invoke Odysseus tools"
}

func hasCapability(capabilities []string, capability string) bool {
	for _, existing := range capabilities {
		if existing == capability {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func maxReasoning(left, right string) string {
	if reasoningRank[left] >= reasoningRank[right] {
		return left
	}
	return right
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
