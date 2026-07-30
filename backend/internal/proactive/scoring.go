package proactive

import (
	"fmt"
	"math"
	"time"
)

func scoreSignal(signal Signal, weights ScoreWeights, now time.Time) ScoreExplanation {
	urgency, urgencyReason := urgencyScore(signal, now)
	risk := riskScore(signal.Risk)
	components := []ScoreComponent{
		scoreComponent(ComponentRelevance, signal.Relevance, weights.Relevance, "declared relevance to the owner's active work"),
		scoreComponent(ComponentUrgency, urgency, weights.Urgency, urgencyReason),
		scoreComponent(ComponentImportance, signal.Importance, weights.Importance, "declared consequence and goal importance"),
		scoreComponent(ComponentRisk, risk, weights.Risk, "potential harm if the open loop is ignored"),
	}
	total := 0.0
	for _, component := range components {
		total += component.Contribution
	}
	total = round6(total)
	return ScoreExplanation{
		Total:      total,
		Components: components,
		Summary:    fmt.Sprintf("ranked %.3f from relevance, urgency, importance, and risk; safety gates are evaluated separately", total),
	}
}

func scoreComponent(name ScoreComponentName, value, weight float64, reason string) ScoreComponent {
	return ScoreComponent{
		Name:         name,
		Value:        round6(value),
		Weight:       round6(weight),
		Contribution: round6(value * weight),
		Reason:       reason,
	}
}

func urgencyScore(signal Signal, now time.Time) (float64, string) {
	if signal.DueAt == nil {
		switch signal.Type {
		case SignalCapacityConstraint, SignalReviewQueue:
			return 0.65, "capacity and review signals should be assessed promptly"
		case SignalSourceChange:
			return 0.50, "source changes should be reviewed before dependent work continues"
		default:
			return 0.25, "no explicit deadline is attached"
		}
	}
	remaining := signal.DueAt.Sub(now)
	switch {
	case remaining <= 0:
		return 1.0, "deadline is due or overdue"
	case remaining <= 24*time.Hour:
		return 0.95, "deadline is within 24 hours"
	case remaining <= 3*24*time.Hour:
		return 0.80, "deadline is within three days"
	case remaining <= 7*24*time.Hour:
		return 0.65, "deadline is within seven days"
	case remaining <= 30*24*time.Hour:
		return 0.40, "deadline is within 30 days"
	default:
		return 0.20, "deadline is more than 30 days away"
	}
}

func riskScore(value RiskLevel) float64 {
	switch value {
	case RiskCritical:
		return 1
	case RiskHigh:
		return 0.8
	case RiskMedium:
		return 0.5
	default:
		return 0.2
	}
}

func round6(value float64) float64 {
	return math.Round(value*1_000_000) / 1_000_000
}

func normalizeLearnedWeights(weights ScoreWeights) ScoreWeights {
	values := []*float64{&weights.Relevance, &weights.Urgency, &weights.Importance, &weights.Risk}
	for _, value := range values {
		*value = math.Max(0.10, math.Min(0.50, *value))
	}
	for iteration := 0; iteration < 8; iteration++ {
		total := weights.Relevance + weights.Urgency + weights.Importance + weights.Risk
		diff := 1 - total
		if math.Abs(diff) < 0.000001 {
			break
		}
		adjustable := 0
		for _, value := range values {
			if (diff > 0 && *value < 0.50) || (diff < 0 && *value > 0.10) {
				adjustable++
			}
		}
		if adjustable == 0 {
			break
		}
		step := diff / float64(adjustable)
		for _, value := range values {
			if (diff > 0 && *value < 0.50) || (diff < 0 && *value > 0.10) {
				*value = math.Max(0.10, math.Min(0.50, *value+step))
			}
		}
	}
	weights.Relevance = round6(weights.Relevance)
	weights.Urgency = round6(weights.Urgency)
	weights.Importance = round6(weights.Importance)
	weights.Risk = round6(1 - weights.Relevance - weights.Urgency - weights.Importance)
	return weights
}
