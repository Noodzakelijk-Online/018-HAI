package modelintelligence

import "strings"

// OperationBudget bounds an operation's model usage (§19). Defaults are
// conservative: minimal context, never the model's maximum window.
type OperationBudget struct {
	MaximumInputTokens      int             `json:"maximumInputTokens"`
	MaximumOutputTokens     int             `json:"maximumOutputTokens"`
	MaximumReasoning        ReasoningEffort `json:"maximumReasoningEffort"`
	MaximumContextItems     int             `json:"maximumContextItems"`
	MaximumSourceBytes      int             `json:"maximumSourceBytes"`
	ContextStrategy         ContextStrategy `json:"contextStrategy"`
	CacheStrategy           CacheType       `json:"cacheStrategy"`
	BatchEligible           bool            `json:"batchEligible"`
	PredictedOutputEligible bool            `json:"predictedOutputEligible"`
}

// DefaultBudget returns the conservative default budget (§19).
func DefaultBudget() OperationBudget {
	return OperationBudget{
		MaximumInputTokens:  4000,
		MaximumOutputTokens: 1000,
		MaximumReasoning:    EffortLow,
		MaximumContextItems: 5,
		MaximumSourceBytes:  20_000,
		ContextStrategy:     ContextEvidenceOnly,
		CacheStrategy:       CacheDeterministicResult,
		BatchEligible:       false,
	}
}

// RecommendReasoning maps a task to a reasoning-effort ceiling (§19). Deep
// reasoning is never permission to act; it still requires evidence + approval.
func RecommendReasoning(operationType string, highRisk bool) ReasoningEffort {
	t := strings.ToLower(operationType)
	switch {
	case containsAny(t, "classify", "classification", "extract", "dedupe", "metadata", "cleanup", "organize"):
		return EffortLow
	case containsAny(t, "code review", "architecture", "legal", "admin", "scope", "dispute"):
		return EffortHigh
	case highRisk:
		return EffortHigh
	default:
		return EffortMedium // normal drafting/planning
	}
}

// RecommendContextStrategy chooses a context strategy (§19). Never long context
// by default; escalate only for genuinely large sources.
func RecommendContextStrategy(sourceBytes int, highRisk bool) ContextStrategy {
	switch {
	case sourceBytes > 200_000:
		return ContextDossierMode
	case sourceBytes > 40_000:
		return ContextSummaryPlusEvid
	case highRisk:
		return ContextSummaryPlusEvid
	default:
		return ContextEvidenceOnly
	}
}

// BudgetForOperation builds a bounded budget for an operation.
func BudgetForOperation(operationType string, sourceBytes int, highRisk bool) OperationBudget {
	b := DefaultBudget()
	b.MaximumReasoning = RecommendReasoning(operationType, highRisk)
	b.ContextStrategy = RecommendContextStrategy(sourceBytes, highRisk)
	if b.ContextStrategy == ContextDossierMode || b.ContextStrategy == ContextSummaryPlusEvid {
		b.MaximumContextItems = 12
		b.MaximumInputTokens = 12000
	}
	if highRisk {
		// High-risk work may draft more but never auto-executes.
		b.MaximumOutputTokens = 1500
	}
	b.BatchEligible = !highRisk
	return b
}
