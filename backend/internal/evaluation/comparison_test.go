package evaluation

import (
	"strings"
	"testing"
)

func TestCompareToBaselineAppliesAbsoluteAndRegressionThresholds(t *testing.T) {
	dataset := testDataset(t, 1, []byte(`{"prompt":"hello"}`))
	baseline := testRun(t, dataset, RunModeShadow, "", 1, CriterionPassed, 0.95)
	baseline.ID = "baseline"
	baseline.RecordDigest = runDigest(baseline)
	candidate := testRun(t, dataset, RunModeCanary, baseline.ID, 2, CriterionPassed, 0.84)
	thresholds := RegressionThresholds{
		MinOverallScore: 0.8, MinCasePassRate: 1,
		MaxOverallScoreDrop: 0.05, MaxCasePassRateDrop: 0,
		MaxRequiredFailures: 0, MaxCriterionErrors: 0,
	}
	comparison, err := CompareToBaseline(candidate, baseline, thresholds)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if !comparison.Regressed || len(comparison.Violations) != 1 ||
		comparison.Violations[0] != "overall score regression exceeds the allowed drop" {
		t.Fatalf("unexpected comparison: %#v", comparison)
	}
}

func TestPromotionIsFailClosed(t *testing.T) {
	dataset := testDataset(t, 1, []byte(`{"prompt":"hello"}`))
	baseline := testRun(t, dataset, RunModeShadow, "", 1, CriterionPassed, 0.9)
	baseline.ID = "baseline"
	baseline.RecordDigest = runDigest(baseline)
	thresholds := RegressionThresholds{
		MinOverallScore: 0.8, MinCasePassRate: 1,
		MaxOverallScoreDrop: 0.05, MaxCasePassRateDrop: 0,
		MaxRequiredFailures: 0, MaxCriterionErrors: 0,
	}

	shadow := testRun(t, dataset, RunModeShadow, baseline.ID, 2, CriterionPassed, 0.95)
	if decision := DecidePromotion(shadow, &baseline, thresholds); decision.Code != PromotionHold || decision.Allowed {
		t.Fatalf("shadow run was promotion eligible: %#v", decision)
	}
	canary := testRun(t, dataset, RunModeCanary, baseline.ID, 2, CriterionPassed, 0.95)
	if decision := DecidePromotion(canary, nil, thresholds); decision.Code != PromotionHold || decision.Allowed {
		t.Fatalf("missing baseline did not hold promotion: %#v", decision)
	}
	if decision := DecidePromotion(canary, &baseline, thresholds); decision.Code != PromotionPromote || !decision.Allowed {
		t.Fatalf("passing canary was not promoted: %#v", decision)
	}
	tampered := cloneRun(canary)
	tampered.OverallScore = 1
	if decision := DecidePromotion(tampered, &baseline, thresholds); decision.Code != PromotionReject || decision.Allowed {
		t.Fatalf("tampered canary was not rejected: %#v", decision)
	}
}

func TestPromotionRejectsCriterionErrorsAndWrongBaselinePin(t *testing.T) {
	dataset := testDataset(t, 1, []byte(`{"prompt":"hello"}`))
	baseline := testRun(t, dataset, RunModeShadow, "", 1, CriterionPassed, 0.9)
	baseline.ID = "baseline"
	baseline.RecordDigest = runDigest(baseline)
	thresholds := RegressionThresholds{
		MinOverallScore: 0, MinCasePassRate: 0,
		MaxOverallScoreDrop: 1, MaxCasePassRateDrop: 1,
		MaxRequiredFailures: 1, MaxCriterionErrors: 0,
	}
	withError := testRun(t, dataset, RunModeCanary, baseline.ID, 2, CriterionError, 0)
	if decision := DecidePromotion(withError, &baseline, thresholds); decision.Code != PromotionReject ||
		decision.Comparison == nil || decision.Allowed {
		t.Fatalf("criterion error did not reject promotion: %#v", decision)
	}
	wrongPin := testRun(t, dataset, RunModeCanary, "another-baseline", 2, CriterionPassed, 0.95)
	if decision := DecidePromotion(wrongPin, &baseline, thresholds); decision.Code != PromotionReject || decision.Allowed {
		t.Fatalf("wrong baseline pin did not reject promotion: %#v", decision)
	}
}

func TestFailedRunsAreImmutableButNeverPromotionEligible(t *testing.T) {
	dataset := testDataset(t, 1, []byte(`{"prompt":"hello"}`))
	spec := testRunSpec(dataset, RunModeCanary, "", 1, CriterionPassed, 1)
	spec.Status = RunStatusFailed
	spec.FailureCode = "evaluator_timeout"
	spec.Observations = nil
	record, err := NewRunRecord(spec)
	if err != nil {
		t.Fatalf("new failed run: %v", err)
	}
	if err := ValidateRunRecord(record); err != nil {
		t.Fatalf("failed run record is not valid evidence: %v", err)
	}
	if decision := DecidePromotion(record, nil, RegressionThresholds{}); decision.Code != PromotionReject || decision.Allowed {
		t.Fatalf("failed run was promotion eligible: %#v", decision)
	}
}

func TestInvalidThresholdsRejectPromotion(t *testing.T) {
	dataset := testDataset(t, 1, []byte(`{"prompt":"hello"}`))
	candidate := testRun(t, dataset, RunModeCanary, "baseline", 1, CriterionPassed, 1)
	decision := DecidePromotion(candidate, nil, RegressionThresholds{MaxOverallScoreDrop: 1.1})
	if decision.Code != PromotionReject || decision.Allowed ||
		len(decision.Reasons) != 1 || !strings.Contains(decision.Reasons[0], "policy") {
		t.Fatalf("invalid thresholds did not fail closed: %#v", decision)
	}
}
