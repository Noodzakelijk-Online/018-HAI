package outcomeevaluation

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"automation-hub-backend/internal/lifeontology"
)

var (
	testStart = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	testEnd   = testStart.Add(30 * 24 * time.Hour)
	testAsOf  = testEnd.Add(24 * time.Hour)
)

func TestEvaluateMeasuresOutcomeAndTarget(t *testing.T) {
	request := validRequest()
	request.Observations = []Observation{
		observation("obs-1", 12, testStart.Add(5*24*time.Hour)),
		observation("obs-2", 16, testStart.Add(15*24*time.Hour)),
	}

	result, err := Evaluate(request)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	indicator := result.Indicators[0]
	if indicator.Evidence != EvidenceSufficient || indicator.Trend != TrendImproving || indicator.Regression != RegressionNone || indicator.Target != TargetMet {
		t.Fatalf("unexpected measurement: %+v", indicator)
	}
	if indicator.CurrentValue == nil || *indicator.CurrentValue != 16 {
		t.Fatalf("current value = %v, want 16", indicator.CurrentValue)
	}
	if indicator.DeltaFromBaseline == nil || *indicator.DeltaFromBaseline != 6 {
		t.Fatalf("delta = %v, want 6", indicator.DeltaFromBaseline)
	}
	if result.State != OutcomeAchieved || result.ReviewRequired {
		t.Fatalf("state = %q, review = %v", result.State, result.ReviewRequired)
	}
	if len(result.Recommendations) != 0 {
		t.Fatalf("recommendations = %d, want 0", len(result.Recommendations))
	}
	if err := VerifyAuditDigest(result); err != nil {
		t.Fatalf("VerifyAuditDigest() error = %v", err)
	}
}

func TestEvaluateAppliesLatestUserCorrectionWithoutErasingOriginal(t *testing.T) {
	request := validRequest()
	request.Observations = []Observation{
		observation("obs-1", 11, testStart.Add(5*24*time.Hour)),
		observation("obs-2", 5, testStart.Add(15*24*time.Hour)),
	}
	request.Corrections = []UserCorrection{
		{
			ID: "correction-1", Scope: request.Outcome.Scope, ObservationID: "obs-2",
			ActorID: "owner-1", UserConfirmed: true, CorrectedValue: 13,
			CorrectedVerification: VerificationUserConfirmed, Reason: "I entered the wrong value.",
			CorrectedAt: testStart.Add(17 * 24 * time.Hour),
		},
	}

	result, err := Evaluate(request)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	indicator := result.Indicators[0]
	if indicator.CurrentValue == nil || *indicator.CurrentValue != 13 {
		t.Fatalf("corrected current value = %v, want 13", indicator.CurrentValue)
	}
	if got := indicator.Effective[1]; got.ObservationID != "obs-2" || got.AppliedCorrectionID != "correction-1" || got.Value != 13 {
		t.Fatalf("effective observation = %+v", got)
	}
	if request.Observations[1].Value != 5 {
		t.Fatal("Evaluate mutated the original observation")
	}
	if !indicator.ReviewRequired || !hasRecommendation(result, RecommendationReviewCorrection) {
		t.Fatalf("correction did not require review: %+v", result)
	}
}

func TestEvaluateClassifiesConflictingObservations(t *testing.T) {
	request := validRequest()
	when := testStart.Add(10 * 24 * time.Hour)
	request.Observations = []Observation{
		observation("obs-a", 12, when),
		observation("obs-b", 20, when),
		observation("obs-c", 14, testStart.Add(20*24*time.Hour)),
	}

	result, err := Evaluate(request)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	indicator := result.Indicators[0]
	if indicator.Evidence != EvidenceConflicting || indicator.Trend != TrendUnknown || indicator.Regression != RegressionUnknown {
		t.Fatalf("conflict classification = %+v", indicator)
	}
	if result.State != OutcomeReviewRequired || !hasRecommendation(result, RecommendationReconcileEvidence) {
		t.Fatalf("conflicting evidence did not fail to review: %+v", result)
	}
}

func TestEvaluateClassifiesInsufficientEvidence(t *testing.T) {
	request := validRequest()
	unverified := observation("obs-unverified", 40, testStart.Add(5*24*time.Hour))
	unverified.Verification = VerificationUnverified
	unverified.Sources = nil
	request.Observations = []Observation{unverified, observation("obs-only", 12, testStart.Add(15*24*time.Hour))}

	result, err := Evaluate(request)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	indicator := result.Indicators[0]
	if indicator.Evidence != EvidenceInsufficient || indicator.Trend != TrendUnknown || indicator.Regression != RegressionUnknown {
		t.Fatalf("insufficient classification = %+v", indicator)
	}
	if result.State != OutcomeInsufficientEvidence || !hasRecommendation(result, RecommendationCollectEvidence) {
		t.Fatalf("insufficient evidence result = %+v", result)
	}
}

func TestEvaluateTrendAndRegressionDirection(t *testing.T) {
	tests := []struct {
		name       string
		direction  DesiredDirection
		baseline   float64
		target     float64
		values     []float64
		trend      TrendClassification
		regression RegressionClassification
	}{
		{"higher improving", DirectionHigher, 10, 20, []float64{12, 14}, TrendImproving, RegressionNone},
		{"higher regression", DirectionHigher, 10, 20, []float64{8, 6}, TrendDeclining, RegressionDetected},
		{"lower improving", DirectionLower, 10, 4, []float64{8, 6}, TrendImproving, RegressionNone},
		{"lower regression", DirectionLower, 10, 4, []float64{12, 14}, TrendDeclining, RegressionDetected},
		{"maintain stable", DirectionMaintain, 10, 10, []float64{10.2, 9.9}, TrendStable, RegressionNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := validRequest()
			indicator := &request.Outcome.Indicators[0]
			indicator.Direction = tt.direction
			indicator.Baseline.Value = tt.baseline
			indicator.TargetValue = tt.target
			indicator.TargetTolerance = 0.5
			request.Observations = []Observation{
				observation("obs-1", tt.values[0], testStart.Add(5*24*time.Hour)),
				observation("obs-2", tt.values[1], testStart.Add(15*24*time.Hour)),
			}
			result, err := Evaluate(request)
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}
			got := result.Indicators[0]
			if got.Trend != tt.trend || got.Regression != tt.regression {
				t.Fatalf("trend/regression = %s/%s, want %s/%s", got.Trend, got.Regression, tt.trend, tt.regression)
			}
			if tt.regression == RegressionDetected && (result.State != OutcomeRegression || !hasRecommendation(result, RecommendationReviewRegression)) {
				t.Fatalf("regression controls missing: %+v", result)
			}
		})
	}
}

func TestEvaluateIsDeterministicAcrossInputOrder(t *testing.T) {
	request := validRequest()
	request.Observations = []Observation{
		observation("obs-b", 16, testStart.Add(15*24*time.Hour)),
		observation("obs-a", 12, testStart.Add(5*24*time.Hour)),
	}
	first, err := Evaluate(request)
	if err != nil {
		t.Fatalf("first Evaluate() error = %v", err)
	}
	request.Observations[0], request.Observations[1] = request.Observations[1], request.Observations[0]
	second, err := Evaluate(request)
	if err != nil {
		t.Fatalf("second Evaluate() error = %v", err)
	}
	if first.ID != second.ID || first.AuditDigest != second.AuditDigest || !reflect.DeepEqual(first, second) {
		t.Fatalf("evaluation is not deterministic:\nfirst:  %+v\nsecond: %+v", first, second)
	}

	tampered := first
	*tampered.Indicators[0].CurrentValue = 999
	if !errors.Is(VerifyAuditDigest(tampered), ErrIntegrityViolation) {
		t.Fatal("tampered evaluation passed audit verification")
	}
}

func TestModelConfidenceNeverBecomesVerificationOrAuthority(t *testing.T) {
	request := validRequest()
	first := observation("obs-1", 12, testStart.Add(5*24*time.Hour))
	second := observation("obs-2", 16, testStart.Add(15*24*time.Hour))
	for _, value := range []*Observation{&first, &second} {
		value.Attribution = Attribution{Method: AttributionModelEstimate, Confidence: 1, Rationale: "Model estimate only."}
	}
	request.Observations = []Observation{first, second}

	result, err := Evaluate(request)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !result.ReviewRequired || result.State != OutcomeReviewRequired || !hasRecommendation(result, RecommendationValidateAttribution) {
		t.Fatalf("model confidence was treated as conclusive: %+v", result)
	}
	for _, effective := range result.Indicators[0].Effective {
		if effective.Verification != VerificationVerified || effective.Attribution.Confidence != 1 {
			t.Fatalf("verification and attribution were conflated: %+v", effective)
		}
	}
	for _, recommendation := range result.Recommendations {
		control := recommendation.Control
		if !control.AdvisoryOnly || !control.ReviewRequired || control.ExecutionAuthority != "none" || control.MayExecute || control.MayChangePolicy {
			t.Fatalf("recommendation grants authority: %+v", recommendation)
		}
	}
	if err := result.ValidateNoAuthority(); err != nil {
		t.Fatalf("ValidateNoAuthority() error = %v", err)
	}

	tampered := result
	tampered.Recommendations[0].Control.MayExecute = true
	if !errors.Is(tampered.ValidateNoAuthority(), ErrIntegrityViolation) {
		t.Fatal("authority tampering was not rejected")
	}
}

func validRequest() EvaluationRequest {
	scope := Scope{OwnerID: "owner-1", WorkspaceID: "workspace-1"}
	return EvaluationRequest{
		Outcome: IntendedOutcome{
			ID: "outcome-1", Scope: scope, Statement: "Increase successful weekly planning.",
			LifeDomain: lifeontology.DomainPersonalAdmin,
			Window:     LongitudinalWindow{Start: testStart, End: testEnd},
			Indicators: []Indicator{{
				ID: "indicator-1", Name: "Successful plans", Unit: "count", Direction: DirectionHigher,
				TargetValue: 15, TargetTolerance: 0, TrendThresholdPerDay: 0.01,
				RegressionThreshold: 2, MinimumObservations: 2,
				Baseline: Baseline{
					ID: "baseline-1", Scope: scope, Value: 10, ObservedAt: testStart.Add(-24 * time.Hour),
					Verification: VerificationVerified, Sources: []SourceReference{source("baseline-source", testStart.Add(-24*time.Hour))},
				},
			}},
		},
		AsOf: testAsOf,
	}
}

func observation(id string, value float64, observedAt time.Time) Observation {
	return Observation{
		ID: id, Scope: Scope{OwnerID: "owner-1", WorkspaceID: "workspace-1"}, IndicatorID: "indicator-1",
		Value: value, ObservedAt: observedAt, RecordedAt: observedAt.Add(time.Hour),
		Verification: VerificationVerified, Sources: []SourceReference{source(id+"-source", observedAt.Add(time.Hour))},
		Attribution: Attribution{Method: AttributionControlledStudy, Confidence: 0.8, Rationale: "Measured against a controlled comparison."},
	}
}

func source(id string, retrievedAt time.Time) SourceReference {
	return SourceReference{
		ID: id, URI: "https://evidence.example/" + id,
		ContentDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		RetrievedAt:   retrievedAt, Status: SourceVerified,
	}
}

func hasRecommendation(result Evaluation, kind RecommendationKind) bool {
	for _, recommendation := range result.Recommendations {
		if recommendation.Kind == kind {
			return true
		}
	}
	return false
}
