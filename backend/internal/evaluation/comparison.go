package evaluation

import (
	"fmt"
	"math"
	"reflect"
	"strings"
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

func NewBaselineComparisonReceipt(spec BaselineComparisonReceiptSpec) (BaselineComparisonReceipt, error) {
	if !validID(strings.TrimSpace(spec.ID)) || spec.CreatedAt.IsZero() {
		return BaselineComparisonReceipt{}, fmt.Errorf("evaluation: comparison receipt id and created time are required")
	}
	comparison, err := CompareToBaseline(spec.Candidate, spec.Baseline, spec.Thresholds)
	if err != nil {
		return BaselineComparisonReceipt{}, err
	}
	receipt := BaselineComparisonReceipt{
		SchemaVersion:         ComparisonReceiptSchemaVersion,
		ID:                    strings.TrimSpace(spec.ID),
		CandidateRunID:        spec.Candidate.ID,
		CandidateRecordDigest: spec.Candidate.RecordDigest,
		BaselineRunID:         spec.Baseline.ID,
		BaselineRecordDigest:  spec.Baseline.RecordDigest,
		Thresholds:            spec.Thresholds,
		Comparison:            comparison,
		CreatedAt:             spec.CreatedAt.UTC(),
	}
	if err := validateBaselineComparisonReceiptShape(receipt); err != nil {
		return BaselineComparisonReceipt{}, err
	}
	receipt.ReceiptDigest = comparisonReceiptDigest(receipt)
	return receipt, nil
}

func ValidateBaselineComparisonReceipt(receipt BaselineComparisonReceipt) error {
	expected := strings.ToLower(strings.TrimSpace(receipt.ReceiptDigest))
	receipt.ReceiptDigest = ""
	if err := validateBaselineComparisonReceiptShape(receipt); err != nil {
		return err
	}
	if !validDigest(expected) || comparisonReceiptDigest(receipt) != expected {
		return fmt.Errorf("evaluation: comparison receipt digest mismatch")
	}
	return nil
}

func NewPromotionDecisionReceipt(spec PromotionDecisionReceiptSpec) (PromotionDecisionReceipt, error) {
	if !validID(strings.TrimSpace(spec.ID)) || spec.CreatedAt.IsZero() {
		return PromotionDecisionReceipt{}, fmt.Errorf("evaluation: promotion receipt id and created time are required")
	}
	decision := DecidePromotion(spec.Candidate, spec.Baseline, spec.Thresholds)
	receipt := PromotionDecisionReceipt{
		SchemaVersion:         PromotionReceiptSchemaVersion,
		ID:                    strings.TrimSpace(spec.ID),
		CandidateRunID:        spec.Candidate.ID,
		CandidateRecordDigest: spec.Candidate.RecordDigest,
		ComparisonReceiptID:   strings.TrimSpace(spec.ComparisonReceiptID),
		Thresholds:            spec.Thresholds,
		Decision:              clonePromotionDecision(decision),
		CreatedAt:             spec.CreatedAt.UTC(),
	}
	if spec.Baseline != nil {
		receipt.BaselineRunID = spec.Baseline.ID
		receipt.BaselineRecordDigest = spec.Baseline.RecordDigest
	}
	if err := validatePromotionDecisionReceiptShape(receipt); err != nil {
		return PromotionDecisionReceipt{}, err
	}
	receipt.ReceiptDigest = promotionReceiptDigest(receipt)
	return receipt, nil
}

func ValidatePromotionDecisionReceipt(receipt PromotionDecisionReceipt) error {
	expected := strings.ToLower(strings.TrimSpace(receipt.ReceiptDigest))
	receipt.ReceiptDigest = ""
	if err := validatePromotionDecisionReceiptShape(receipt); err != nil {
		return err
	}
	if !validDigest(expected) || promotionReceiptDigest(receipt) != expected {
		return fmt.Errorf("evaluation: promotion receipt digest mismatch")
	}
	return nil
}

func validateBaselineComparisonReceiptShape(receipt BaselineComparisonReceipt) error {
	if receipt.SchemaVersion != ComparisonReceiptSchemaVersion ||
		!validID(receipt.ID) ||
		!validID(receipt.CandidateRunID) ||
		!validID(receipt.BaselineRunID) ||
		receipt.CandidateRunID == receipt.BaselineRunID ||
		!validDigest(receipt.CandidateRecordDigest) ||
		!validDigest(receipt.BaselineRecordDigest) ||
		receipt.CreatedAt.IsZero() {
		return fmt.Errorf("evaluation: invalid comparison receipt identity, run binding, or time")
	}
	if err := receipt.Thresholds.Validate(); err != nil {
		return err
	}
	if receipt.Comparison.CandidateRunID != receipt.CandidateRunID ||
		receipt.Comparison.BaselineRunID != receipt.BaselineRunID {
		return fmt.Errorf("evaluation: comparison receipt result is bound to different runs")
	}
	return nil
}

func validatePromotionDecisionReceiptShape(receipt PromotionDecisionReceipt) error {
	if receipt.SchemaVersion != PromotionReceiptSchemaVersion ||
		!validID(receipt.ID) ||
		!validID(receipt.CandidateRunID) ||
		!validDigest(receipt.CandidateRecordDigest) ||
		receipt.CreatedAt.IsZero() {
		return fmt.Errorf("evaluation: invalid promotion receipt identity, candidate, or time")
	}
	if err := receipt.Thresholds.Validate(); err != nil {
		return err
	}
	if (receipt.BaselineRunID == "") != (receipt.BaselineRecordDigest == "") {
		return fmt.Errorf("evaluation: promotion receipt baseline id and digest must be provided together")
	}
	if receipt.BaselineRunID != "" {
		if !validID(receipt.BaselineRunID) ||
			receipt.BaselineRunID == receipt.CandidateRunID ||
			!validDigest(receipt.BaselineRecordDigest) {
			return fmt.Errorf("evaluation: invalid promotion receipt baseline binding")
		}
	}
	if receipt.ComparisonReceiptID != "" && !validID(receipt.ComparisonReceiptID) {
		return fmt.Errorf("evaluation: invalid comparison receipt reference")
	}
	switch receipt.Decision.Code {
	case PromotionPromote:
		if !receipt.Decision.Allowed || receipt.Decision.Comparison == nil {
			return fmt.Errorf("evaluation: promote receipt must contain an allowed comparison decision")
		}
	case PromotionHold, PromotionReject:
		if receipt.Decision.Allowed {
			return fmt.Errorf("evaluation: hold and reject receipts cannot allow promotion")
		}
	default:
		return fmt.Errorf("evaluation: unknown promotion decision code")
	}
	if len(receipt.Decision.Reasons) == 0 {
		return fmt.Errorf("evaluation: promotion receipt requires at least one reason")
	}
	if receipt.Decision.Comparison != nil {
		if receipt.ComparisonReceiptID == "" ||
			receipt.Decision.Comparison.CandidateRunID != receipt.CandidateRunID ||
			receipt.Decision.Comparison.BaselineRunID != receipt.BaselineRunID {
			return fmt.Errorf("evaluation: compared promotion decisions require a matching comparison receipt")
		}
	} else if receipt.ComparisonReceiptID != "" {
		return fmt.Errorf("evaluation: comparison receipt reference requires a comparison result")
	}
	return nil
}

func validateComparisonReceiptAgainstRuns(
	receipt BaselineComparisonReceipt,
	candidate RunRecord,
	baseline RunRecord,
) error {
	if err := ValidateBaselineComparisonReceipt(receipt); err != nil {
		return err
	}
	if receipt.CandidateRunID != candidate.ID ||
		receipt.CandidateRecordDigest != candidate.RecordDigest ||
		receipt.BaselineRunID != baseline.ID ||
		receipt.BaselineRecordDigest != baseline.RecordDigest {
		return fmt.Errorf("evaluation: comparison receipt run digest binding mismatch")
	}
	expected, err := CompareToBaseline(candidate, baseline, receipt.Thresholds)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(expected, receipt.Comparison) {
		return fmt.Errorf("evaluation: comparison receipt result mismatch")
	}
	return nil
}

func validatePromotionReceiptAgainstRuns(
	receipt PromotionDecisionReceipt,
	candidate RunRecord,
	baseline *RunRecord,
) error {
	if err := ValidatePromotionDecisionReceipt(receipt); err != nil {
		return err
	}
	if receipt.CandidateRunID != candidate.ID ||
		receipt.CandidateRecordDigest != candidate.RecordDigest {
		return fmt.Errorf("evaluation: promotion receipt candidate digest binding mismatch")
	}
	if baseline == nil {
		if receipt.BaselineRunID != "" || receipt.BaselineRecordDigest != "" {
			return fmt.Errorf("evaluation: promotion receipt references an unavailable baseline")
		}
	} else if receipt.BaselineRunID != baseline.ID ||
		receipt.BaselineRecordDigest != baseline.RecordDigest {
		return fmt.Errorf("evaluation: promotion receipt baseline digest binding mismatch")
	}
	expected := DecidePromotion(candidate, baseline, receipt.Thresholds)
	if !reflect.DeepEqual(expected, receipt.Decision) {
		return fmt.Errorf("evaluation: promotion receipt decision mismatch")
	}
	return nil
}

func comparisonReceiptDigest(receipt BaselineComparisonReceipt) string {
	receipt.ReceiptDigest = ""
	return digestJSON(receipt)
}

func promotionReceiptDigest(receipt PromotionDecisionReceipt) string {
	receipt.ReceiptDigest = ""
	return digestJSON(receipt)
}

func cloneBaselineComparisonReceipt(receipt BaselineComparisonReceipt) BaselineComparisonReceipt {
	copy := receipt
	copy.Comparison.Violations = append([]string(nil), receipt.Comparison.Violations...)
	return copy
}

func clonePromotionDecisionReceipt(receipt PromotionDecisionReceipt) PromotionDecisionReceipt {
	copy := receipt
	copy.Decision = clonePromotionDecision(receipt.Decision)
	return copy
}

func clonePromotionDecision(decision PromotionDecision) PromotionDecision {
	copy := decision
	copy.Reasons = append([]string(nil), decision.Reasons...)
	if decision.Comparison != nil {
		comparison := *decision.Comparison
		comparison.Violations = append([]string(nil), decision.Comparison.Violations...)
		copy.Comparison = &comparison
	}
	return copy
}
