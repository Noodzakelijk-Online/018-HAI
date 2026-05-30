package llm

import (
	"encoding/json"
	"fmt"
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
	DailyPaidBudgetEUR              float64    `json:"dailyPaidBudgetEur"`
	PaidCallsAllowed                bool       `json:"paidCallsAllowed"`
	LocalModelsAllowed              bool       `json:"localModelsAllowed"`
	FreeCloudQuotaAllowed           bool       `json:"freeCloudQuotaAllowed"`
	LocalFirst                      bool       `json:"localFirst"`
	CacheRepeatedPrompts            bool       `json:"cacheRepeatedPrompts"`
	RouteSimpleTasksToSmallModels    bool       `json:"routeSimpleTasksToSmallModels"`
	RouteComplexTasksToBestFreeModel bool       `json:"routeComplexTasksToBestAvailableFreeModel"`
	RequireApprovalBeforePaidUsage  bool       `json:"requireApprovalBeforePaidUsage"`
	TierOrder                       []string   `json:"tierOrder"`
	Providers                       []Provider `json:"providers"`
}

type Provider struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Enabled        bool    `json:"enabled"`
	Local          bool    `json:"local"`
	Paid           bool    `json:"paid"`
	QuotaRemaining int     `json:"quotaRemaining"`
	DailyBudgetEUR float64 `json:"dailyBudgetEur"`
	Models         []Model `json:"models"`
}

type Model struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Tier                string   `json:"tier"`
	Capabilities        []string `json:"capabilities"`
	MaxDifficulty       int      `json:"maxDifficulty"`
	MaxReasoning        string   `json:"maxReasoning"`
	EstimatedCostEUR    float64  `json:"estimatedCostEur"`
	RequiresApproval    bool     `json:"requiresApproval"`
	Enabled             bool     `json:"enabled"`
}

type RouteRequest struct {
	Task              string `json:"task"`
	TaskType          string `json:"taskType,omitempty"`
	Difficulty        int    `json:"difficulty,omitempty"`
	RequiredReasoning string `json:"requiredReasoning,omitempty"`
	ValidationPassed  *bool  `json:"validationPassed,omitempty"`
	PreviousModelID    string `json:"previousModelId,omitempty"`
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
	FallbackPath        []FallbackOption   `json:"fallbackPath"`
	Skipped            []SkippedModel      `json:"skipped"`
	LoggedAt           time.Time          `json:"loggedAt"`
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

	return &Service{policy: policy, logs: []RouteDecision{}}, nil
}

func (s *Service) Policy() Policy {
	return s.policy
}

func (s *Service) Logs() []RouteDecision {
	s.mu.Lock()
	defer s.mu.Unlock()

	copied := make([]RouteDecision, len(s.logs))
	copy(copied, s.logs)
	return copied
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
		FallbackPath:        fallbackPath(candidates[1:]),
		Skipped:            skipped,
		LoggedAt:           time.Now().UTC(),
	}

	s.addLog(decision)

	return decision, nil
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

func defaultPolicy() Policy {
	return Policy{
		DailyPaidBudgetEUR:              0,
		PaidCallsAllowed:                false,
		LocalModelsAllowed:              true,
		FreeCloudQuotaAllowed:           true,
		LocalFirst:                      true,
		CacheRepeatedPrompts:            true,
		RouteSimpleTasksToSmallModels:    true,
		RouteComplexTasksToBestFreeModel: true,
		RequireApprovalBeforePaidUsage:  true,
		TierOrder:                       []string{TierFree, TierCheap, TierAcceptable, TierHigh, TierExpensive},
		Providers: []Provider{
			{
				ID:             "ollama",
				Name:           "Ollama local",
				Enabled:        true,
				Local:          true,
				Paid:           false,
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
