package evaluation

import (
	"fmt"
	"math"
)

type RegressionThresholds struct {
	MinOverallScore     float64 `json:"minOverallScore"`
	MinCasePassRate     float64 `json:"minCasePassRate"`
	MaxOverallScoreDrop float64 `json:"maxOverallScoreDrop"`
	MaxCasePassRateDrop float64 `json:"maxCasePassRateDrop"`
	MaxRequiredFailures int     `json:"maxRequiredFailures"`
	MaxCriterionErrors  int     `json:"maxCriterionErrors"`
}

func (thresholds RegressionThresholds) Validate() error {
	for name, value := range map[string]float64{
		"min overall score":       thresholds.MinOverallScore,
		"min case pass rate":      thresholds.MinCasePassRate,
		"max overall score drop":  thresholds.MaxOverallScoreDrop,
		"max case pass rate drop": thresholds.MaxCasePassRateDrop,
	} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
			return fmt.Errorf("evaluation: %s must be between 0 and 1", name)
		}
	}
	if thresholds.MaxRequiredFailures < 0 || thresholds.MaxCriterionErrors < 0 {
		return fmt.Errorf("evaluation: failure and error limits cannot be negative")
	}
	return nil
}

type BaselineComparison struct {
	CandidateRunID    string   `json:"candidateRunId"`
	BaselineRunID     string   `json:"baselineRunId"`
	OverallScoreDelta float64  `json:"overallScoreDelta"`
	CasePassRateDelta float64  `json:"casePassRateDelta"`
	Regressed         bool     `json:"regressed"`
	Violations        []string `json:"violations,omitempty"`
}

func CompareToBaseline(candidate, baseline RunRecord, thresholds RegressionThresholds) (BaselineComparison, error) {
	if err := thresholds.Validate(); err != nil {
		return BaselineComparison{}, err
	}
	if err := ValidateRunRecord(candidate); err != nil {
		return BaselineComparison{}, fmt.Errorf("evaluation: invalid candidate: %w", err)
	}
	if err := ValidateRunRecord(baseline); err != nil {
		return BaselineComparison{}, fmt.Errorf("evaluation: invalid baseline: %w", err)
	}
	if candidate.Status != RunStatusCompleted || baseline.Status != RunStatusCompleted {
		return BaselineComparison{}, fmt.Errorf("evaluation: candidate and baseline must be completed")
	}
	if candidate.Dataset != baseline.Dataset {
		return BaselineComparison{}, fmt.Errorf("evaluation: candidate and baseline dataset versions must match")
	}
	comparison := BaselineComparison{
		CandidateRunID:    candidate.ID,
		BaselineRunID:     baseline.ID,
		OverallScoreDelta: candidate.OverallScore - baseline.OverallScore,
		CasePassRateDelta: candidate.CasePassRate - baseline.CasePassRate,
	}
	if candidate.OverallScore < thresholds.MinOverallScore {
		comparison.Violations = append(comparison.Violations, "overall score is below the absolute minimum")
	}
	if candidate.CasePassRate < thresholds.MinCasePassRate {
		comparison.Violations = append(comparison.Violations, "case pass rate is below the absolute minimum")
	}
	if baseline.OverallScore-candidate.OverallScore > thresholds.MaxOverallScoreDrop {
		comparison.Violations = append(comparison.Violations, "overall score regression exceeds the allowed drop")
	}
	if baseline.CasePassRate-candidate.CasePassRate > thresholds.MaxCasePassRateDrop {
		comparison.Violations = append(comparison.Violations, "case pass rate regression exceeds the allowed drop")
	}
	if candidate.RequiredFailureCount > thresholds.MaxRequiredFailures {
		comparison.Violations = append(comparison.Violations, "required criterion failures exceed the allowed limit")
	}
	if candidate.CriterionErrorCount > thresholds.MaxCriterionErrors {
		comparison.Violations = append(comparison.Violations, "criterion errors exceed the allowed limit")
	}
	comparison.Regressed = len(comparison.Violations) > 0
	return comparison, nil
}

type PromotionDecisionCode string

const (
	PromotionPromote PromotionDecisionCode = "promote"
	PromotionHold    PromotionDecisionCode = "hold"
	PromotionReject  PromotionDecisionCode = "reject"
)

type PromotionDecision struct {
	Code       PromotionDecisionCode `json:"code"`
	Allowed    bool                  `json:"allowed"`
	Reasons    []string              `json:"reasons"`
	Comparison *BaselineComparison   `json:"comparison,omitempty"`
}

// DecidePromotion is fail-closed: only a valid completed canary with a valid,
// comparable baseline and no threshold violation can be promoted.
func DecidePromotion(candidate RunRecord, baseline *RunRecord, thresholds RegressionThresholds) PromotionDecision {
	reject := func(reason string) PromotionDecision {
		return PromotionDecision{Code: PromotionReject, Allowed: false, Reasons: []string{reason}}
	}
	if err := thresholds.Validate(); err != nil {
		return reject("regression policy is invalid")
	}
	if err := ValidateRunRecord(candidate); err != nil {
		return reject("candidate run is invalid or has been tampered with")
	}
	if candidate.Status != RunStatusCompleted {
		return reject("candidate evaluation did not complete")
	}
	if candidate.Mode == RunModeShadow {
		return PromotionDecision{
			Code: PromotionHold, Allowed: false,
			Reasons: []string{"shadow runs are observation-only and cannot authorize promotion"},
		}
	}
	if candidate.Mode != RunModeCanary {
		return reject("candidate run mode is not promotion eligible")
	}
	if baseline == nil {
		return PromotionDecision{
			Code: PromotionHold, Allowed: false,
			Reasons: []string{"a valid baseline is required before promotion"},
		}
	}
	comparison, err := CompareToBaseline(candidate, *baseline, thresholds)
	if err != nil {
		return reject("candidate could not be compared with the baseline")
	}
	if candidate.BaselineRunID == "" || candidate.BaselineRunID != baseline.ID {
		return reject("candidate does not pin the supplied baseline run")
	}
	if comparison.Regressed {
		return PromotionDecision{
			Code: PromotionReject, Allowed: false,
			Reasons: append([]string(nil), comparison.Violations...), Comparison: &comparison,
		}
	}
	return PromotionDecision{
		Code: PromotionPromote, Allowed: true,
		Reasons:    []string{"canary completed and satisfied all absolute and baseline thresholds"},
		Comparison: &comparison,
	}
}
