package llm

import (
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
	TierFree       = "free"
	TierCheap      = "cheap"
	TierAcceptable = "acceptable"
	TierHigh       = "high"
	TierExpensive  = "expensive"
)

var tierRank = map[string]int{
	TierFree:       0,
	TierCheap:      1,
	TierAcceptable: 2,
	TierHigh:       3,
	TierExpensive:  4,
}

var reasoningRank = map[string]int{
	"low":       1,
	"medium":    2,
	"high":      3,
	"very_high": 4,
}

type Policy struct {
	DailyPaidBudgetEUR               float64    `json:"dailyPaidBudgetEur"`
	PaidCallsAllowed                 bool       `json:"paidCallsAllowed"`
	LocalModelsAllowed               bool       `json:"localModelsAllowed"`
	FreeCloudQuotaAllowed            bool       `json:"freeCloudQuotaAllowed"`
	LocalFirst                       bool       `json:"localFirst"`
	CacheRepeatedPrompts             bool       `json:"cacheRepeatedPrompts"`
	RouteSimpleTasksToSmallModels    bool       `json:"routeSimpleTasksToSmallModels"`
	RouteComplexTasksToBestFreeModel bool       `json:"routeComplexTasksToBestAvailableFreeModel"`
	RequireApprovalBeforePaidUsage   bool       `json:"requireApprovalBeforePaidUsage"`
	TierOrder                        []string   `json:"tierOrder"`
	Providers                        []Provider `json:"providers"`
}

type Provider struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Enabled         bool    `json:"enabled"`
	Local           bool    `json:"local"`
	Paid            bool    `json:"paid"`
	EndpointURL     string  `json:"endpointUrl,omitempty"`
	APIKeyEnv       string  `json:"apiKeyEnv,omitempty"`
	Configured      bool    `json:"configured"`
	ReadinessStatus string  `json:"readinessStatus,omitempty"`
	ReadinessReason string  `json:"readinessReason,omitempty"`
	QuotaRemaining  int     `json:"quotaRemaining"`
	DailyBudgetEUR  float64 `json:"dailyBudgetEur"`
	Models          []Model `json:"models"`
}

type Model struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Tier             string   `json:"tier"`
	Capabilities     []string `json:"capabilities"`
	MaxDifficulty    int      `json:"maxDifficulty"`
	MaxReasoning     string   `json:"maxReasoning"`
	EstimatedCostEUR float64  `json:"estimatedCostEur"`
	RequiresApproval bool     `json:"requiresApproval"`
	Enabled          bool     `json:"enabled"`
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
	SelectedProviderID string             `json:"selectedProviderId"`
	SelectedModelID    string             `json:"selectedModelId"`
	SelectedModelName  string             `json:"selectedModelName"`
	Tier               string             `json:"tier"`
	Reason             string             `json:"reason"`
	EstimatedCostEUR   float64            `json:"estimatedCostEur"`
	RequiresApproval   bool               `json:"requiresApproval"`
	Classification     TaskClassification `json:"classification"`
	FallbackPath       []FallbackOption   `json:"fallbackPath"`
	Skipped            []SkippedModel     `json:"skipped"`
	LoggedAt           time.Time          `json:"loggedAt"`
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
	ProviderID     string    `json:"providerId"`
	ProviderName   string    `json:"providerName"`
	Status         string    `json:"status"`
	Reason         string    `json:"reason"`
	EndpointURL    string    `json:"endpointUrl,omitempty"`
	HTTPStatus     int       `json:"httpStatus,omitempty"`
	ModelsSeen     int       `json:"modelsSeen"`
	DurationMs     int64     `json:"durationMs"`
	Live           bool      `json:"live"`
	RequiresReview bool      `json:"requiresReview"`
	CheckedAt      time.Time `json:"checkedAt"`
}

type FallbackOption struct {
	ProviderID       string  `json:"providerId"`
	ModelID          string  `json:"modelId"`
	ModelName        string  `json:"modelName"`
	Tier             string  `json:"tier"`
	EstimatedCostEUR float64 `json:"estimatedCostEur"`
	RequiresApproval bool    `json:"requiresApproval"`
}

type SkippedModel struct {
	ProviderID string `json:"providerId"`
	ModelID    string `json:"modelId"`
	Reason     string `json:"reason"`
}

type Service struct {
	policy Policy
	mu     sync.Mutex
	logs   []RouteDecision
}

func NewServiceFromEnv() (*Service, error) {
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

	policy = annotatePolicyReadiness(policy)
	return &Service{policy: policy, logs: []RouteDecision{}}, nil
}

func (s *Service) Policy() Policy {
	return annotatePolicyReadiness(s.policy)
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

func (s *Service) Route(request RouteRequest) (RouteDecision, error) {
	classification := classifyTask(request)
	candidates, skipped := s.candidates(classification, request)
	if len(candidates) == 0 {
		decision := RouteDecision{
			Reason:         "No enabled model satisfies the task, budget, quota, and approval policy.",
			Classification: classification,
			Skipped:        skipped,
			LoggedAt:       time.Now().UTC(),
		}
		s.addLog(decision)
		return decision, nil
	}

	selected := candidates[0]
	decision := RouteDecision{
		SelectedProviderID: selected.provider.ID,
		SelectedModelID:    selected.model.ID,
		SelectedModelName:  selected.model.Name,
		Tier:               selected.model.Tier,
		Reason:             selectionReason(selected, classification),
		EstimatedCostEUR:   selected.model.EstimatedCostEUR,
		RequiresApproval:   selected.model.RequiresApproval || selected.provider.Paid,
		Classification:     classification,
		FallbackPath:       fallbackPath(candidates[1:]),
		Skipped:            skipped,
		LoggedAt:           time.Now().UTC(),
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
	return &GenerationResult{
		ProviderID:       provider.ID,
		ModelID:          model.ID,
		ModelName:        model.Name,
		Tier:             model.Tier,
		Output:           safety.RedactSecrets(strings.TrimSpace(output)),
		Status:           "completed",
		Reason:           "model endpoint returned a draft; verification must still ground important claims",
		EstimatedCostEUR: model.EstimatedCostEUR,
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
		result.Reason = fmt.Sprintf("probe returned HTTP %d: %s", resp.StatusCode, compactOutput(raw, 300))
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

type candidate struct {
	provider Provider
	model    Model
}

func (s *Service) candidates(classification TaskClassification, request RouteRequest) ([]candidate, []SkippedModel) {
	candidates := []candidate{}
	skipped := []SkippedModel{}

	for _, provider := range s.policy.Providers {
		if !provider.Enabled {
			for _, model := range provider.Models {
				skipped = append(skipped, SkippedModel{ProviderID: provider.ID, ModelID: model.ID, Reason: "provider disabled"})
			}
			continue
		}
		if provider.Local && !s.policy.LocalModelsAllowed {
			for _, model := range provider.Models {
				skipped = append(skipped, SkippedModel{ProviderID: provider.ID, ModelID: model.ID, Reason: "local models disabled by policy"})
			}
			continue
		}
		if provider.Paid && (!s.policy.PaidCallsAllowed || s.policy.DailyPaidBudgetEUR <= 0) {
			for _, model := range provider.Models {
				skipped = append(skipped, SkippedModel{ProviderID: provider.ID, ModelID: model.ID, Reason: "paid usage disabled by policy"})
			}
			continue
		}
		if !provider.Local && !provider.Paid && provider.QuotaRemaining <= 0 && !s.policy.FreeCloudQuotaAllowed {
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
		for _, model := range provider.Models {
			reason := unsuitableReason(provider, model, classification, s.policy, request)
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
		if s.policy.LocalFirst && left.provider.Local != right.provider.Local {
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

func selectionReason(selected candidate, classification TaskClassification) string {
	return fmt.Sprintf(
		"Selected the cheapest suitable %s-tier model after classifying the task as %s difficulty %d with %s reasoning. The model satisfies required capabilities: %s.",
		selected.model.Tier,
		classification.TaskType,
		classification.Difficulty,
		classification.RequiredReasoning,
		strings.Join(classification.RequiredCapabilities, ", "),
	)
}

func fallbackPath(candidates []candidate) []FallbackOption {
	path := []FallbackOption{}
	for _, candidate := range candidates {
		path = append(path, FallbackOption{
			ProviderID:       candidate.provider.ID,
			ModelID:          candidate.model.ID,
			ModelName:        candidate.model.Name,
			Tier:             candidate.model.Tier,
			EstimatedCostEUR: candidate.model.EstimatedCostEUR,
			RequiresApproval: candidate.provider.Paid || candidate.model.RequiresApproval,
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
	if provider.APIKeyEnv != "" && strings.TrimSpace(os.Getenv(provider.APIKeyEnv)) == "" {
		return providerReadiness{configured: false, status: "missing_api_key", reason: "required API key environment variable " + provider.APIKeyEnv + " is not set"}
	}
	return providerReadiness{configured: true, status: "configured", reason: "provider endpoint and required credentials are configured"}
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

func defaultPolicy() Policy {
	ollamaEndpoint := strings.TrimSpace(os.Getenv("OLLAMA_BASE_URL"))
	lmStudioEndpoint := strings.TrimSpace(os.Getenv("LM_STUDIO_BASE_URL"))
	freeCloudEndpoint := strings.TrimSpace(os.Getenv("FREE_CLOUD_OPENAI_BASE_URL"))
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
		TierOrder:                        []string{TierFree, TierCheap, TierAcceptable, TierHigh, TierExpensive},
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
					{ID: "phi3:mini", Name: "Phi small local", Tier: TierFree, Capabilities: []string{"general", "extraction"}, MaxDifficulty: 2, MaxReasoning: "low", Enabled: true},
					{ID: "qwen2.5-coder:7b", Name: "Qwen coder local", Tier: TierFree, Capabilities: []string{"general", "coding", "extraction"}, MaxDifficulty: 4, MaxReasoning: "high", Enabled: true},
					{ID: "deepseek-coder:6.7b", Name: "DeepSeek coder local", Tier: TierFree, Capabilities: []string{"general", "coding", "planning"}, MaxDifficulty: 4, MaxReasoning: "high", Enabled: true},
					{ID: "llama3.1:8b", Name: "Llama local", Tier: TierFree, Capabilities: []string{"general", "planning", "extraction"}, MaxDifficulty: 4, MaxReasoning: "high", Enabled: true},
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
					{ID: "openai-compatible-local", Name: "OpenAI-compatible local endpoint", Tier: TierFree, Capabilities: []string{"general", "coding", "planning", "extraction"}, MaxDifficulty: 4, MaxReasoning: "high", Enabled: true},
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
				},
			},
		},
	}
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
