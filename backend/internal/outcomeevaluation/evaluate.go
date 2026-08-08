package outcomeevaluation

import (
	"fmt"
	"math"
	"sort"
	"time"
)

const noExecutionAuthority = "none"

type measurementPoint struct {
	at    time.Time
	value float64
}

// Evaluate returns a deterministic, advisory snapshot. It performs no writes,
// approvals, policy changes, or execution, and the result cannot grant them.
func Evaluate(request EvaluationRequest) (Evaluation, error) {
	normalized, err := normalizeAndValidate(request)
	if err != nil {
		return Evaluation{}, err
	}

	requestHash, err := hashValue(normalized)
	if err != nil {
		return Evaluation{}, err
	}
	result := Evaluation{
		ID:            "outcome-evaluation-" + requestHash,
		SchemaVersion: SchemaVersion,
		Scope:         normalized.Outcome.Scope,
		OutcomeID:     normalized.Outcome.ID,
		Window:        normalized.Outcome.Window,
		AsOf:          normalized.AsOf,
	}

	observations := observationsByIndicator(normalized.Observations)
	corrections := correctionsByObservation(normalized.Corrections)
	allTargetsMet := true
	anyInsufficient := false
	anyConflict := false
	anyRegression := false
	for _, indicator := range normalized.Outcome.Indicators {
		evaluation := evaluateIndicator(indicator, observations[indicator.ID], corrections)
		result.Indicators = append(result.Indicators, evaluation)
		result.ReviewReasons = append(result.ReviewReasons, evaluation.ReviewReasons...)
		result.Recommendations = append(result.Recommendations, recommendationsFor(normalized, evaluation)...)
		result.ReviewRequired = result.ReviewRequired || evaluation.ReviewRequired
		allTargetsMet = allTargetsMet && evaluation.Target == TargetMet
		anyInsufficient = anyInsufficient || evaluation.Evidence == EvidenceInsufficient
		anyConflict = anyConflict || evaluation.Evidence == EvidenceConflicting
		anyRegression = anyRegression || evaluation.Regression == RegressionDetected
	}
	result.ReviewReasons = canonicalStrings(result.ReviewReasons)
	sort.Slice(result.Recommendations, func(i, j int) bool { return result.Recommendations[i].ID < result.Recommendations[j].ID })

	switch {
	case anyConflict:
		result.State = OutcomeReviewRequired
	case anyRegression:
		result.State = OutcomeRegression
		result.ReviewRequired = true
	case anyInsufficient:
		result.State = OutcomeInsufficientEvidence
		result.ReviewRequired = true
	case result.ReviewRequired:
		result.State = OutcomeReviewRequired
	case allTargetsMet:
		result.State = OutcomeAchieved
	default:
		result.State = OutcomeOnTrack
	}

	result.AuditDigest, err = evaluationDigest(result)
	if err != nil {
		return Evaluation{}, err
	}
	return result, nil
}

func evaluateIndicator(indicator Indicator, observations []Observation, corrections map[string][]UserCorrection) IndicatorEvaluation {
	result := IndicatorEvaluation{
		IndicatorID:   indicator.ID,
		Evidence:      EvidenceSufficient,
		BaselineValue: indicator.Baseline.Value,
		Trend:         TrendUnknown,
		Regression:    RegressionUnknown,
		Target:        TargetUnknown,
	}
	conflict := indicator.Baseline.Verification == VerificationDisputed
	corrected := false
	modelAttribution := false
	weakAttribution := false

	for _, observation := range observations {
		effective, correctionConflict, wasCorrected := applyCorrection(observation, corrections[observation.ID], indicator.TargetTolerance)
		result.Effective = append(result.Effective, effective)
		conflict = conflict || correctionConflict || effective.Verification == VerificationDisputed
		corrected = corrected || wasCorrected
		modelAttribution = modelAttribution || effective.Attribution.Method == AttributionModelEstimate
		weakAttribution = weakAttribution || effective.Attribution.Method == AttributionUnknown || effective.Attribution.Confidence < 0.5
	}

	points, observationConflict := measurementPoints(result.Effective, indicator.TargetTolerance)
	conflict = conflict || observationConflict
	baselineUsable := substantive(indicator.Baseline.Verification)
	if conflict {
		result.Evidence = EvidenceConflicting
		result.ReviewRequired = true
		result.ReviewReasons = append(result.ReviewReasons, "conflicting or disputed evidence")
	} else if !baselineUsable || len(points) < indicator.MinimumObservations {
		result.Evidence = EvidenceInsufficient
		result.ReviewRequired = true
		result.ReviewReasons = append(result.ReviewReasons, "insufficient substantive evidence")
	}
	if corrected {
		result.ReviewRequired = true
		result.ReviewReasons = append(result.ReviewReasons, "user correction applied")
	}
	if modelAttribution {
		result.ReviewRequired = true
		result.ReviewReasons = append(result.ReviewReasons, "model-estimated attribution requires validation")
	} else if weakAttribution {
		result.ReviewRequired = true
		result.ReviewReasons = append(result.ReviewReasons, "attribution is unknown or weak")
	}

	if len(points) > 0 {
		current := points[len(points)-1].value
		delta := current - indicator.Baseline.Value
		result.CurrentValue = floatPointer(current)
		result.DeltaFromBaseline = floatPointer(delta)
		result.Target = targetStatus(indicator, current)
	}
	if result.Evidence == EvidenceSufficient {
		slope := trendSlope(indicator, points)
		result.TrendPerDay = floatPointer(slope)
		switch {
		case slope > indicator.TrendThresholdPerDay:
			result.Trend = TrendImproving
		case slope < -indicator.TrendThresholdPerDay:
			result.Trend = TrendDeclining
			result.ReviewRequired = true
			result.ReviewReasons = append(result.ReviewReasons, "declining longitudinal trend")
		default:
			result.Trend = TrendStable
		}
		current := *result.CurrentValue
		if performance(indicator, current)-performance(indicator, indicator.Baseline.Value) <= -indicator.RegressionThreshold {
			result.Regression = RegressionDetected
			result.ReviewRequired = true
			result.ReviewReasons = append(result.ReviewReasons, "regression from baseline")
		} else {
			result.Regression = RegressionNone
		}
	}
	result.ReviewReasons = canonicalStrings(result.ReviewReasons)
	return result
}

func applyCorrection(observation Observation, corrections []UserCorrection, tolerance float64) (EffectiveObservation, bool, bool) {
	result := EffectiveObservation{
		ObservationID: observation.ID,
		Value:         observation.Value,
		ObservedAt:    observation.ObservedAt,
		Verification:  observation.Verification,
		SourceIDs:     sourceIDs(observation.Sources),
		Attribution:   observation.Attribution,
	}
	if len(corrections) == 0 {
		return result, false, false
	}
	latest := corrections[len(corrections)-1]
	conflict := false
	for i := len(corrections) - 2; i >= 0 && corrections[i].CorrectedAt.Equal(latest.CorrectedAt); i-- {
		if materiallyDifferent(corrections[i].CorrectedValue, latest.CorrectedValue, tolerance) {
			conflict = true
		}
	}
	result.AppliedCorrectionID = latest.ID
	result.Value = latest.CorrectedValue
	result.Verification = latest.CorrectedVerification
	result.SourceIDs = sourceIDs(latest.Sources)
	return result, conflict, true
}

func measurementPoints(values []EffectiveObservation, tolerance float64) ([]measurementPoint, bool) {
	var result []measurementPoint
	conflict := false
	for start := 0; start < len(values); {
		end := start + 1
		for end < len(values) && values[end].ObservedAt.Equal(values[start].ObservedAt) {
			end++
		}
		var sum float64
		count := 0
		var first float64
		for i := start; i < end; i++ {
			if !substantive(values[i].Verification) {
				continue
			}
			if count == 0 {
				first = values[i].Value
			} else if materiallyDifferent(first, values[i].Value, tolerance) {
				conflict = true
			}
			sum += values[i].Value
			count++
		}
		if count > 0 {
			result = append(result, measurementPoint{at: values[start].ObservedAt, value: sum / float64(count)})
		}
		start = end
	}
	return result, conflict
}

func trendSlope(indicator Indicator, points []measurementPoint) float64 {
	if len(points) < 2 {
		return 0
	}
	origin := points[0].at
	var sumX, sumY float64
	for _, point := range points {
		x := point.at.Sub(origin).Hours() / 24
		sumX += x
		sumY += performance(indicator, point.value)
	}
	meanX := sumX / float64(len(points))
	meanY := sumY / float64(len(points))
	var numerator, denominator float64
	for _, point := range points {
		x := point.at.Sub(origin).Hours() / 24
		y := performance(indicator, point.value)
		numerator += (x - meanX) * (y - meanY)
		denominator += (x - meanX) * (x - meanX)
	}
	if denominator == 0 {
		return 0
	}
	return numerator / denominator
}

func performance(indicator Indicator, value float64) float64 {
	switch indicator.Direction {
	case DirectionHigher:
		return value
	case DirectionLower:
		return -value
	case DirectionMaintain:
		return -math.Abs(value - indicator.TargetValue)
	default:
		return math.Inf(-1)
	}
}

func targetStatus(indicator Indicator, value float64) TargetStatus {
	switch indicator.Direction {
	case DirectionHigher:
		if value+indicator.TargetTolerance >= indicator.TargetValue {
			return TargetMet
		}
	case DirectionLower:
		if value-indicator.TargetTolerance <= indicator.TargetValue {
			return TargetMet
		}
	case DirectionMaintain:
		if math.Abs(value-indicator.TargetValue) <= indicator.TargetTolerance {
			return TargetMet
		}
	}
	return TargetNotMet
}

func recommendationsFor(request EvaluationRequest, indicator IndicatorEvaluation) []LearningRecommendation {
	var kinds []RecommendationKind
	if indicator.Evidence == EvidenceInsufficient {
		kinds = append(kinds, RecommendationCollectEvidence)
	}
	if indicator.Evidence == EvidenceConflicting {
		kinds = append(kinds, RecommendationReconcileEvidence)
	}
	if indicator.Regression == RegressionDetected {
		kinds = append(kinds, RecommendationReviewRegression)
	}
	if indicator.Trend == TrendDeclining {
		kinds = append(kinds, RecommendationReviewTrend)
	}
	if contains(indicator.ReviewReasons, "user correction applied") {
		kinds = append(kinds, RecommendationReviewCorrection)
	}
	if contains(indicator.ReviewReasons, "model-estimated attribution requires validation") || contains(indicator.ReviewReasons, "attribution is unknown or weak") {
		kinds = append(kinds, RecommendationValidateAttribution)
	}

	evidenceIDs := make([]string, 0, len(indicator.Effective)*2)
	for _, observation := range indicator.Effective {
		evidenceIDs = append(evidenceIDs, observation.ObservationID)
		if observation.AppliedCorrectionID != "" {
			evidenceIDs = append(evidenceIDs, observation.AppliedCorrectionID)
		}
	}
	evidenceIDs = canonicalStrings(evidenceIDs)
	result := make([]LearningRecommendation, 0, len(kinds))
	for _, kind := range kinds {
		recommendation := LearningRecommendation{
			Kind: kind, IndicatorID: indicator.IndicatorID, Summary: recommendationSummary(kind), EvidenceIDs: evidenceIDs,
			Control: RecommendationControl{AdvisoryOnly: true, ReviewRequired: true, ExecutionAuthority: noExecutionAuthority, MayExecute: false, MayChangePolicy: false},
		}
		fingerprint, _ := hashValue(struct {
			SchemaVersion string
			Scope         Scope
			OutcomeID     string
			IndicatorID   string
			Kind          RecommendationKind
			EvidenceIDs   []string
			AsOf          time.Time
		}{SchemaVersion, request.Outcome.Scope, request.Outcome.ID, indicator.IndicatorID, kind, evidenceIDs, request.AsOf})
		recommendation.ID = "learning-recommendation-" + fingerprint
		result = append(result, recommendation)
	}
	return result
}

func recommendationSummary(kind RecommendationKind) string {
	switch kind {
	case RecommendationCollectEvidence:
		return "Collect additional provenance-backed observations before drawing a longitudinal conclusion."
	case RecommendationReconcileEvidence:
		return "Ask the owner to reconcile conflicting or disputed observations and retain the resolution as a correction."
	case RecommendationReviewRegression:
		return "Review the measured regression with the owner before considering any change to the intervention."
	case RecommendationReviewTrend:
		return "Review the declining trend and its measurement assumptions with the owner."
	case RecommendationReviewCorrection:
		return "Review the user correction and preserve both the original and corrected values in the audit trail."
	case RecommendationValidateAttribution:
		return "Validate the attribution basis independently; confidence alone is not evidence of causation."
	default:
		return "Review the outcome evidence with the owner."
	}
}

func observationsByIndicator(values []Observation) map[string][]Observation {
	result := make(map[string][]Observation)
	for _, value := range values {
		result[value.IndicatorID] = append(result[value.IndicatorID], value)
	}
	return result
}

func correctionsByObservation(values []UserCorrection) map[string][]UserCorrection {
	result := make(map[string][]UserCorrection)
	for _, value := range values {
		result[value.ObservationID] = append(result[value.ObservationID], value)
	}
	return result
}

func substantive(status VerificationStatus) bool {
	return status == VerificationUserConfirmed || status == VerificationSourceSupported || status == VerificationVerified
}

func sourceIDs(values []SourceReference) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.ID)
	}
	return canonicalStrings(result)
}

func canonicalStrings(values []string) []string {
	sort.Strings(values)
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func materiallyDifferent(a, b, tolerance float64) bool {
	return math.Abs(a-b) > math.Max(tolerance, 1e-9)
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func floatPointer(value float64) *float64 { return &value }

// ValidateNoAuthority verifies the package's immutable recommendation boundary.
func (evaluation Evaluation) ValidateNoAuthority() error {
	for _, recommendation := range evaluation.Recommendations {
		control := recommendation.Control
		if !control.AdvisoryOnly || !control.ReviewRequired || control.ExecutionAuthority != noExecutionAuthority || control.MayExecute || control.MayChangePolicy {
			return fmt.Errorf("%w: recommendation %q exceeds advisory authority", ErrIntegrityViolation, recommendation.ID)
		}
	}
	return nil
}
