package pursuit

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"automation-hub-backend/internal/controlledlearning"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/resourceplanner"

	"github.com/google/uuid"
)

const portfolioCalibrationReadTimeout = 5 * time.Second

const portfolioCalibrationMaxInt64 = int64(1<<63 - 1)

type PortfolioEstimateCalibrationBinding struct {
	ScopeKey             string                           `json:"scopeKey"`
	ProposalID           string                           `json:"proposalId"`
	ProposalVersion      string                           `json:"proposalVersion"`
	ApplicationID        string                           `json:"applicationId"`
	EvidenceDigest       string                           `json:"evidenceDigest"`
	SourceDuration       resourceplanner.DurationEstimate `json:"sourceDuration"`
	SourceEstimatedUsage resourceplanner.Usage            `json:"sourceEstimatedUsage"`
}

type PortfolioEstimateCalibrationRecommendation struct {
	PursuitID                   uuid.UUID `json:"pursuitId"`
	ScopeKey                    string    `json:"scopeKey"`
	Status                      string    `json:"status"`
	Reason                      string    `json:"reason"`
	ProposalID                  string    `json:"proposalId,omitempty"`
	ProposalVersion             string    `json:"proposalVersion,omitempty"`
	ApplicationID               string    `json:"applicationId,omitempty"`
	EvidenceDigest              string    `json:"evidenceDigest,omitempty"`
	SampleCount                 int       `json:"sampleCount,omitempty"`
	EffortMultiplier            float64   `json:"effortMultiplier,omitempty"`
	CostMultiplier              float64   `json:"costMultiplier,omitempty"`
	EffortDispersion            float64   `json:"effortDispersion,omitempty"`
	CostDispersion              float64   `json:"costDispersion,omitempty"`
	Confidence                  float64   `json:"confidence,omitempty"`
	ObservedFrom                time.Time `json:"observedFrom,omitempty"`
	ObservedThrough             time.Time `json:"observedThrough,omitempty"`
	SourceOptimisticMinutes     int64     `json:"sourceOptimisticMinutes,omitempty"`
	SourceExpectedMinutes       int64     `json:"sourceExpectedMinutes,omitempty"`
	SourcePessimisticMinutes    int64     `json:"sourcePessimisticMinutes,omitempty"`
	SourceCostMicros            int64     `json:"sourceCostMicros,omitempty"`
	SuggestedOptimisticMinutes  int64     `json:"suggestedOptimisticMinutes,omitempty"`
	SuggestedExpectedMinutes    int64     `json:"suggestedExpectedMinutes,omitempty"`
	SuggestedPessimisticMinutes int64     `json:"suggestedPessimisticMinutes,omitempty"`
	SuggestedCostMicros         int64     `json:"suggestedCostMicros,omitempty"`
	AppliedAt                   time.Time `json:"appliedAt,omitempty"`
	Applied                     bool      `json:"applied"`
}

func (s *service) portfolioCalibrationRecommendations(
	ownerIdentity string,
	inputs map[uuid.UUID]PortfolioPursuitPlanningInput,
	pursuits map[uuid.UUID]models.Pursuit,
) ([]PortfolioEstimateCalibrationRecommendation, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	if s == nil || s.portfolioCalibration == nil {
		for _, input := range inputs {
			if input.Calibration != nil {
				return nil, fmt.Errorf("portfolio calibration binding cannot be verified because the learning ledger is unavailable")
			}
		}
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), portfolioCalibrationReadTimeout)
	defer cancel()
	type lookupResult struct {
		value *controlledlearning.AppliedEstimateCalibration
		err   error
	}
	latestCache := map[string]lookupResult{}
	exactCache := map[string]lookupResult{}
	result := make([]PortfolioEstimateCalibrationRecommendation, 0, len(inputs))
	for pursuitID, input := range inputs {
		item := pursuits[pursuitID]
		scope := portfolioCalibrationScope(item.ProjectKey, pursuitID)
		if input.Calibration != nil {
			cacheKey := scope + "\x00" + input.Calibration.ProposalVersion
			lookup, exists := exactCache[cacheKey]
			if !exists {
				lookup.value, lookup.err = s.portfolioCalibration.AppliedEstimateCalibration(
					ctx, ownerIdentity, scope, input.Calibration.ProposalVersion,
				)
				exactCache[cacheKey] = lookup
			}
			if lookup.err != nil {
				return nil, fmt.Errorf("verify portfolio calibration binding for pursuit %s: %w", pursuitID, lookup.err)
			}
			recommendation, err := boundPortfolioCalibrationRecommendation(pursuitID, scope, input, lookup.value)
			if err != nil {
				return nil, err
			}
			result = append(result, recommendation)
			continue
		}

		lookup, exists := latestCache[scope]
		if !exists {
			lookup.value, lookup.err = s.portfolioCalibration.LatestAppliedEstimateCalibration(
				ctx, ownerIdentity, scope,
			)
			latestCache[scope] = lookup
		}
		if lookup.err != nil {
			result = append(result, PortfolioEstimateCalibrationRecommendation{
				PursuitID: pursuitID, ScopeKey: scope, Status: "unavailable",
				Reason:  "reviewed estimate calibration could not be verified; explicit estimates remain unchanged",
				Applied: false,
			})
			continue
		}
		if lookup.value == nil {
			continue
		}
		recommendation := portfolioCalibrationRecommendation(pursuitID, scope, input.Duration, input.EstimatedUsage, lookup.value)
		recommendation.Status = "available"
		recommendation.Reason = fmt.Sprintf(
			"Human-approved calibration from %d verified settlement(s) is available as an optional estimate; it has not been applied.",
			lookup.value.SampleCount,
		)
		result = append(result, recommendation)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].PursuitID.String() < result[j].PursuitID.String()
	})
	return result, nil
}

func boundPortfolioCalibrationRecommendation(
	pursuitID uuid.UUID,
	scope string,
	input PortfolioPursuitPlanningInput,
	calibration *controlledlearning.AppliedEstimateCalibration,
) (PortfolioEstimateCalibrationRecommendation, error) {
	binding := input.Calibration
	if binding == nil || calibration == nil || binding.ScopeKey != scope ||
		binding.ProposalID != calibration.ProposalID ||
		binding.ProposalVersion != calibration.ProposalVersion ||
		binding.ApplicationID != calibration.ApplicationID ||
		binding.EvidenceDigest != calibration.EvidenceDigest {
		return PortfolioEstimateCalibrationRecommendation{}, fmt.Errorf("portfolio calibration binding for pursuit %s does not match an effective reviewed revision", pursuitID)
	}
	recommendation := portfolioCalibrationRecommendation(
		pursuitID, scope, binding.SourceDuration, binding.SourceEstimatedUsage, calibration,
	)
	if input.Duration.OptimisticMinutes != recommendation.SuggestedOptimisticMinutes ||
		input.Duration.ExpectedMinutes != recommendation.SuggestedExpectedMinutes ||
		input.Duration.PessimisticMinutes != recommendation.SuggestedPessimisticMinutes ||
		input.EstimatedUsage.CostMicros != recommendation.SuggestedCostMicros {
		return PortfolioEstimateCalibrationRecommendation{}, fmt.Errorf("portfolio calibration binding for pursuit %s is stale or its reviewed estimate was changed", pursuitID)
	}
	recommendation.Status = "bound"
	recommendation.Reason = "The explicit estimate is bound to this exact human-approved calibration revision."
	recommendation.Applied = true
	return recommendation, nil
}

func portfolioCalibrationRecommendation(
	pursuitID uuid.UUID,
	scope string,
	sourceDuration resourceplanner.DurationEstimate,
	sourceUsage resourceplanner.Usage,
	calibration *controlledlearning.AppliedEstimateCalibration,
) PortfolioEstimateCalibrationRecommendation {
	optimistic := calibratedMinutes(sourceDuration.OptimisticMinutes, calibration.EffortMultiplier)
	expected := maxInt64(optimistic, calibratedMinutes(sourceDuration.ExpectedMinutes, calibration.EffortMultiplier))
	pessimistic := maxInt64(expected, calibratedMinutes(sourceDuration.PessimisticMinutes, calibration.EffortMultiplier))
	return PortfolioEstimateCalibrationRecommendation{
		PursuitID: pursuitID, ScopeKey: scope,
		ProposalID: calibration.ProposalID, ProposalVersion: calibration.ProposalVersion,
		ApplicationID: calibration.ApplicationID, EvidenceDigest: calibration.EvidenceDigest,
		SampleCount:      calibration.SampleCount,
		EffortMultiplier: calibration.EffortMultiplier, CostMultiplier: calibration.CostMultiplier,
		EffortDispersion: calibration.EffortDispersion, CostDispersion: calibration.CostDispersion,
		Confidence: calibration.Confidence, ObservedFrom: calibration.ObservedFrom.UTC(),
		ObservedThrough:             calibration.ObservedThrough.UTC(),
		SourceOptimisticMinutes:     sourceDuration.OptimisticMinutes,
		SourceExpectedMinutes:       sourceDuration.ExpectedMinutes,
		SourcePessimisticMinutes:    sourceDuration.PessimisticMinutes,
		SourceCostMicros:            sourceUsage.CostMicros,
		SuggestedOptimisticMinutes:  optimistic,
		SuggestedExpectedMinutes:    expected,
		SuggestedPessimisticMinutes: pessimistic,
		SuggestedCostMicros:         calibratedCost(sourceUsage.CostMicros, calibration.CostMultiplier),
		AppliedAt:                   calibration.AppliedAt.UTC(), Applied: false,
	}
}

func calibratedMinutes(value int64, multiplier float64) int64 {
	if value <= 0 {
		return value
	}
	scaled := float64(value) * multiplier
	if scaled >= float64(portfolioCalibrationMaxInt64) {
		return portfolioCalibrationMaxInt64
	}
	return maxInt64(1, int64(math.Round(scaled)))
}

func calibratedCost(value int64, multiplier float64) int64 {
	if value <= 0 {
		return value
	}
	scaled := float64(value) * multiplier
	if scaled >= float64(portfolioCalibrationMaxInt64) {
		return portfolioCalibrationMaxInt64
	}
	return maxInt64(0, int64(math.Round(scaled)))
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
