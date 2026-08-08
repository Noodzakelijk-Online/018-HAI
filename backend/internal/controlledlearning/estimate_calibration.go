package controlledlearning

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const (
	EstimateCalibrationInsufficientEvidence = "insufficient_evidence"
	EstimateCalibrationMonitoring           = "monitoring"
	EstimateCalibrationStable               = "stable"
	EstimateCalibrationReviewRequired       = "review_required"
	EstimateCalibrationApplied              = "applied"

	portfolioEffortMetric     = "portfolio_effort_minutes"
	portfolioCostMetric       = "portfolio_cost_micros"
	minimumCalibrationSamples = 3
	calibrationAlgorithmV1    = "portfolio-estimate-median-mad-v1"
	calibrationAlgorithm      = "portfolio-estimate-median-mad-v2"

	calibrationEvaluationInitial    = "initial_cohort"
	calibrationEvaluationPostReview = "post_review_cohort"
)

// EstimateCalibrationDefinition is the structured, reviewable payload stored
// in a controlled-learning proposal. It is advisory: consumers may offer its
// values to the owner, but must never silently replace an explicit estimate.
type EstimateCalibrationDefinition struct {
	Kind                       string    `json:"kind"`
	Version                    int       `json:"version"`
	AlgorithmVersion           string    `json:"algorithmVersion"`
	EvaluationMode             string    `json:"evaluationMode,omitempty"`
	ScopeKey                   string    `json:"scopeKey"`
	ReviewAnchorVersion        string    `json:"reviewAnchorVersion,omitempty"`
	ReviewAnchorEvidenceDigest string    `json:"reviewAnchorEvidenceDigest,omitempty"`
	SampleCount                int       `json:"sampleCount"`
	CostSampleCount            int       `json:"costSampleCount"`
	EffortMultiplier           float64   `json:"effortMultiplier"`
	CostMultiplier             float64   `json:"costMultiplier"`
	EffortDispersion           float64   `json:"effortDispersion"`
	CostDispersion             float64   `json:"costDispersion"`
	Confidence                 float64   `json:"confidence"`
	EvidenceDigest             string    `json:"evidenceDigest"`
	ObservedFrom               time.Time `json:"observedFrom"`
	ObservedThrough            time.Time `json:"observedThrough"`
}

type EstimateCalibrationProposalResult struct {
	Status           string                      `json:"status"`
	ScopeKey         string                      `json:"scopeKey"`
	SampleCount      int                         `json:"sampleCount"`
	NewEvidenceCount int                         `json:"newEvidenceCount"`
	DriftDetected    bool                        `json:"driftDetected"`
	Proposal         *LearningProposal           `json:"proposal,omitempty"`
	Calibration      *AppliedEstimateCalibration `json:"calibration,omitempty"`
}

type AppliedEstimateCalibration struct {
	EstimateCalibrationDefinition
	ProposalID      string    `json:"proposalId"`
	ProposalVersion string    `json:"proposalVersion"`
	ApplicationID   string    `json:"applicationId"`
	AppliedAt       time.Time `json:"appliedAt"`
}

type estimateCalibrationSample struct {
	OutcomeID  string
	OccurredAt time.Time
	Effort     float64
	Cost       float64
	HasCost    bool
}

type estimateCalibrationReviewState struct {
	Pending           *LearningProposal
	PendingDefinition *EstimateCalibrationDefinition
	AnchorVersion     string
	AnchorDigest      string
	AnchorThrough     time.Time
}

// ProposeEstimateCalibration aggregates comparable, verified portfolio
// settlement outcomes. The result is always review-required; this function
// cannot approve or apply the proposal it creates.
func (service *Service) ProposeEstimateCalibration(
	ctx context.Context,
	ownerIdentity string,
	scopeKey string,
) (EstimateCalibrationProposalResult, error) {
	if err := ctx.Err(); err != nil {
		return EstimateCalibrationProposalResult{}, err
	}
	owner := strings.TrimSpace(ownerIdentity)
	scope := strings.TrimSpace(scopeKey)
	if owner == "" {
		return EstimateCalibrationProposalResult{}, ErrOwnerScopeViolation
	}
	if err := validateRequired("estimate calibration scope", scope, maxIdentifierLength); err != nil {
		return EstimateCalibrationProposalResult{}, err
	}
	outcomes, err := service.repository.ListOutcomes(ctx, OutcomeQuery{
		OwnerIdentity: owner,
		Limit:         500,
	})
	if err != nil {
		return EstimateCalibrationProposalResult{}, err
	}
	samples := make([]estimateCalibrationSample, 0, len(outcomes))
	for _, outcome := range outcomes {
		if outcome.ProjectKey != scope || outcome.Basis != EvidenceVerifiedOutcome ||
			outcome.Status != OutcomeSucceeded ||
			(outcome.Verification != VerificationVerified && outcome.Verification != VerificationTestPassed) ||
			!containsString(outcome.Tags, "portfolio-settlement") {
			continue
		}
		effort, effortOK := exactMetricRatio(outcome.Metrics, portfolioEffortMetric)
		if !effortOK {
			continue
		}
		cost, costOK := exactMetricRatio(outcome.Metrics, portfolioCostMetric)
		samples = append(samples, estimateCalibrationSample{
			OutcomeID: outcome.ID, OccurredAt: outcome.OccurredAt.UTC(), Effort: effort,
			Cost: cost, HasCost: costOK,
		})
	}
	current, err := service.LatestAppliedEstimateCalibration(ctx, owner, scope)
	if err != nil {
		return EstimateCalibrationProposalResult{}, err
	}
	reviewState, err := service.estimateCalibrationReviewState(ctx, owner, scope, current)
	if err != nil {
		return EstimateCalibrationProposalResult{}, err
	}
	result := EstimateCalibrationProposalResult{
		Status:      EstimateCalibrationInsufficientEvidence,
		ScopeKey:    scope,
		Calibration: current,
	}
	if reviewState.Pending != nil && reviewState.PendingDefinition != nil {
		result.Status = string(reviewState.Pending.Status)
		result.SampleCount = reviewState.PendingDefinition.SampleCount
		newCohort := calibrationSamplesAfter(samples, reviewState.PendingDefinition.ObservedThrough)
		result.NewEvidenceCount = len(newCohort)
		if len(newCohort) >= minimumCalibrationSamples {
			effort, cost, _, _, _ := calibrationCohortValues(newCohort)
			result.DriftDetected = materialCalibrationChange(
				reviewState.PendingDefinition.EffortMultiplier,
				boundedCalibrationMultiplier(median(effort)),
			)
			if len(cost) >= minimumCalibrationSamples {
				result.DriftDetected = result.DriftDetected || materialCalibrationChange(
					reviewState.PendingDefinition.CostMultiplier,
					boundedCalibrationMultiplier(median(cost)),
				)
			}
		}
		pending := cloneProposal(*reviewState.Pending)
		result.Proposal = &pending
		return result, nil
	}
	cohort := samples
	if !reviewState.AnchorThrough.IsZero() {
		cohort = calibrationSamplesAfter(samples, reviewState.AnchorThrough)
		result.Status = EstimateCalibrationMonitoring
	}
	result.SampleCount = len(cohort)
	result.NewEvidenceCount = len(cohort)
	if len(cohort) < minimumCalibrationSamples {
		return result, nil
	}
	effortRatios, costRatios, evidenceIDs, observedFrom, observedThrough := calibrationCohortValues(cohort)
	if len(effortRatios) < minimumCalibrationSamples {
		return result, nil
	}
	sort.Strings(evidenceIDs)
	evidenceDigest, err := digestValue(evidenceIDs)
	if err != nil {
		return EstimateCalibrationProposalResult{}, err
	}
	effortMedian := median(effortRatios)
	effortMultiplier := boundedCalibrationMultiplier(effortMedian)
	effortDispersion := roundedCalibrationValue(medianAbsoluteDeviation(effortRatios, effortMedian))
	costMultiplier := 1.0
	costDispersion := 0.0
	if len(costRatios) >= minimumCalibrationSamples {
		costMedian := median(costRatios)
		costMultiplier = boundedCalibrationMultiplier(costMedian)
		costDispersion = roundedCalibrationValue(medianAbsoluteDeviation(costRatios, costMedian))
	}
	confidence := calibrationConfidence(len(effortRatios), effortDispersion)
	currentEffort, currentCost := 1.0, 1.0
	currentVersion := "portfolio-estimate-calibration:v0"
	if current != nil {
		currentEffort = current.EffortMultiplier
		currentCost = current.CostMultiplier
		currentVersion = current.ProposalVersion
		result.Calibration = current
	}
	if !materialCalibrationChange(currentEffort, effortMultiplier) &&
		!materialCalibrationChange(currentCost, costMultiplier) {
		result.Status = EstimateCalibrationStable
		return result, nil
	}
	evaluationMode := calibrationEvaluationInitial
	if !reviewState.AnchorThrough.IsZero() {
		evaluationMode = calibrationEvaluationPostReview
		result.DriftDetected = current != nil
	}
	definition := EstimateCalibrationDefinition{
		Kind: "portfolio_estimate_calibration", Version: 2, AlgorithmVersion: calibrationAlgorithm,
		EvaluationMode: evaluationMode, ScopeKey: scope,
		ReviewAnchorVersion:        reviewState.AnchorVersion,
		ReviewAnchorEvidenceDigest: reviewState.AnchorDigest,
		SampleCount:                len(effortRatios), CostSampleCount: len(costRatios),
		EffortMultiplier: effortMultiplier, CostMultiplier: costMultiplier,
		EffortDispersion: effortDispersion, CostDispersion: costDispersion, Confidence: confidence,
		EvidenceDigest: evidenceDigest, ObservedFrom: observedFrom, ObservedThrough: observedThrough,
	}
	change, err := json.Marshal(definition)
	if err != nil {
		return EstimateCalibrationProposalResult{}, fmt.Errorf("encode estimate calibration proposal: %w", err)
	}
	versionBasisDigest, err := digestValue(struct {
		AlgorithmVersion string
		EvidenceIDs      []string
	}{calibrationAlgorithm, evidenceIDs})
	if err != nil {
		return EstimateCalibrationProposalResult{}, err
	}
	versionDigest := strings.TrimPrefix(versionBasisDigest, "sha256:")
	if len(versionDigest) > 16 {
		versionDigest = versionDigest[:16]
	}
	proposal, err := service.Propose(ctx, ProposeRequest{
		OwnerIdentity:  owner,
		IdempotencyKey: "portfolio-estimate-calibration:" + versionDigest,
		Method:         MethodOutcomeReconciliation,
		Target:         TargetPlanningEstimateCalibration,
		Title:          "Review portfolio estimate calibration for " + scope,
		Hypothesis: fmt.Sprintf(
			"Verified settlement history indicates effort estimates should be multiplied by %.3f and cost estimates by %.3f for this scope.",
			effortMultiplier, costMultiplier,
		),
		ProposedChange:  string(change),
		CurrentVersion:  currentVersion,
		ProposedVersion: "portfolio-estimate-calibration:" + versionDigest,
		RollbackPlan:    "Roll back this application to restore the prior reviewed calibration; never alter source settlement evidence.",
		EvaluationPlan:  "Compare the next three verified settlements with the reviewed estimates and create a new proposal only when material drift remains.",
		EvidenceIDs:     evidenceIDs,
	})
	if err != nil {
		return EstimateCalibrationProposalResult{}, err
	}
	result.Proposal = &proposal
	result.Status = string(proposal.Status)
	return result, nil
}

func (service *Service) estimateCalibrationReviewState(
	ctx context.Context,
	ownerIdentity string,
	scopeKey string,
	current *AppliedEstimateCalibration,
) (estimateCalibrationReviewState, error) {
	state := estimateCalibrationReviewState{}
	if current != nil {
		state.AnchorVersion = current.ProposalVersion
		state.AnchorDigest = current.EvidenceDigest
		state.AnchorThrough = current.ObservedThrough.UTC()
	}
	proposals, err := service.repository.ListProposals(ctx, ProposalQuery{
		OwnerIdentity: ownerIdentity,
		Limit:         500,
	})
	if err != nil {
		return state, err
	}
	for index := range proposals {
		proposal := proposals[index]
		if proposal.Target != TargetPlanningEstimateCalibration {
			continue
		}
		if err := verifyProposalIntegrity(proposal); err != nil {
			return state, err
		}
		definition, err := decodeEstimateCalibration(proposal.ProposedChange)
		if err != nil {
			return state, err
		}
		if definition.ScopeKey != scopeKey {
			continue
		}
		if activeEstimateCalibrationProposalStatus(proposal.Status) {
			if state.Pending == nil || proposal.UpdatedAt.After(state.Pending.UpdatedAt) {
				pending := cloneProposal(proposal)
				definitionCopy := definition
				state.Pending = &pending
				state.PendingDefinition = &definitionCopy
			}
			continue
		}
		if terminalProposalStatus(proposal.Status) && definition.ObservedThrough.After(state.AnchorThrough) {
			state.AnchorVersion = proposal.ProposedVersion
			state.AnchorDigest = definition.EvidenceDigest
			state.AnchorThrough = definition.ObservedThrough.UTC()
		}
	}
	return state, nil
}

func activeEstimateCalibrationProposalStatus(status ProposalStatus) bool {
	switch status {
	case ProposalReviewRequired, ProposalChangesRequested, ProposalGovernanceRequired:
		return true
	default:
		return false
	}
}

func calibrationSamplesAfter(samples []estimateCalibrationSample, after time.Time) []estimateCalibrationSample {
	result := make([]estimateCalibrationSample, 0, len(samples))
	for _, sample := range samples {
		if sample.OccurredAt.After(after) {
			result = append(result, sample)
		}
	}
	return result
}

func calibrationCohortValues(samples []estimateCalibrationSample) (
	[]float64,
	[]float64,
	[]string,
	time.Time,
	time.Time,
) {
	effortRatios := make([]float64, 0, len(samples))
	costRatios := make([]float64, 0, len(samples))
	evidenceIDs := make([]string, 0, len(samples))
	observedFrom := time.Time{}
	observedThrough := time.Time{}
	for _, sample := range samples {
		effortRatios = append(effortRatios, sample.Effort)
		if sample.HasCost {
			costRatios = append(costRatios, sample.Cost)
		}
		evidenceIDs = append(evidenceIDs, sample.OutcomeID)
		if observedFrom.IsZero() || sample.OccurredAt.Before(observedFrom) {
			observedFrom = sample.OccurredAt
		}
		if sample.OccurredAt.After(observedThrough) {
			observedThrough = sample.OccurredAt
		}
	}
	return effortRatios, costRatios, evidenceIDs, observedFrom.UTC(), observedThrough.UTC()
}

// LatestAppliedEstimateCalibration returns only a human-approved application.
// Rejected, pending, failed, or rolled-back proposals are deliberately ignored.
func (service *Service) LatestAppliedEstimateCalibration(
	ctx context.Context,
	ownerIdentity string,
	scopeKey string,
) (*AppliedEstimateCalibration, error) {
	calibrations, err := service.appliedEstimateCalibrations(ctx, ownerIdentity, scopeKey)
	if err != nil || len(calibrations) == 0 {
		return nil, err
	}
	return calibrations[0], nil
}

// AppliedEstimateCalibration resolves one exact, still-effective revision.
// Planning uses this method to reproduce a reviewed plan even when a newer
// calibration has subsequently been approved.
func (service *Service) AppliedEstimateCalibration(
	ctx context.Context,
	ownerIdentity string,
	scopeKey string,
	proposalVersion string,
) (*AppliedEstimateCalibration, error) {
	version := strings.TrimSpace(proposalVersion)
	if version == "" {
		return nil, ErrNotFound
	}
	calibrations, err := service.appliedEstimateCalibrations(ctx, ownerIdentity, scopeKey)
	if err != nil {
		return nil, err
	}
	for _, calibration := range calibrations {
		if calibration.ProposalVersion == version {
			return calibration, nil
		}
	}
	return nil, ErrNotFound
}

func (service *Service) appliedEstimateCalibrations(
	ctx context.Context,
	ownerIdentity string,
	scopeKey string,
) ([]*AppliedEstimateCalibration, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	owner := strings.TrimSpace(ownerIdentity)
	scope := strings.TrimSpace(scopeKey)
	if owner == "" {
		return nil, ErrOwnerScopeViolation
	}
	if service.applicationRepository == nil {
		return nil, nil
	}
	proposals, err := service.repository.ListProposals(ctx, ProposalQuery{
		OwnerIdentity: owner,
		Status:        ProposalApproved,
		Limit:         500,
	})
	if err != nil {
		return nil, err
	}
	calibrations := make([]*AppliedEstimateCalibration, 0)
	for _, proposal := range proposals {
		if proposal.Target != TargetPlanningEstimateCalibration || proposal.Revision < 2 {
			continue
		}
		if err := verifyProposalIntegrity(proposal); err != nil {
			return nil, err
		}
		definition, err := decodeEstimateCalibration(proposal.ProposedChange)
		if err != nil {
			return nil, err
		}
		if definition.ScopeKey != scope {
			continue
		}
		application, err := service.applicationRepository.GetProposalApplication(
			ctx, owner, proposal.ID, proposal.Revision-1, ApplicationModeApply,
		)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if err := verifyApplicationIntegrity(application); err != nil {
			return nil, err
		}
		if application.Status != ApplicationApplied || application.ProtectedTarget ||
			application.Target != TargetPlanningEstimateCalibration ||
			application.ProposalDigest != proposal.ProposalDigest ||
			application.AppliedVersion != proposal.ProposedVersion {
			continue
		}
		calibrations = append(calibrations, &AppliedEstimateCalibration{
			EstimateCalibrationDefinition: definition,
			ProposalID:                    proposal.ID,
			ProposalVersion:               proposal.ProposedVersion,
			ApplicationID:                 application.ID,
			AppliedAt:                     application.CompletedAt.UTC(),
		})
	}
	sort.Slice(calibrations, func(i, j int) bool {
		if calibrations[i].AppliedAt.Equal(calibrations[j].AppliedAt) {
			return calibrations[i].ProposalVersion > calibrations[j].ProposalVersion
		}
		return calibrations[i].AppliedAt.After(calibrations[j].AppliedAt)
	})
	return calibrations, nil
}

func decodeEstimateCalibration(value string) (EstimateCalibrationDefinition, error) {
	var definition EstimateCalibrationDefinition
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&definition); err != nil {
		return definition, fmt.Errorf("decode estimate calibration proposal: %w", err)
	}
	legacy := definition.Version == 1 && definition.AlgorithmVersion == calibrationAlgorithmV1
	current := definition.Version == 2 && definition.AlgorithmVersion == calibrationAlgorithm &&
		(definition.EvaluationMode == calibrationEvaluationInitial ||
			definition.EvaluationMode == calibrationEvaluationPostReview)
	if definition.Kind != "portfolio_estimate_calibration" || (!legacy && !current) ||
		strings.TrimSpace(definition.ScopeKey) == "" || definition.SampleCount < minimumCalibrationSamples ||
		definition.CostSampleCount < 0 || definition.CostSampleCount > definition.SampleCount ||
		definition.EffortMultiplier < 0.5 || definition.EffortMultiplier > 2 ||
		definition.CostMultiplier < 0.5 || definition.CostMultiplier > 2 ||
		definition.EffortDispersion < 0 || definition.CostDispersion < 0 ||
		definition.Confidence < 0 || definition.Confidence > 1 ||
		!sha256DigestPattern.MatchString(definition.EvidenceDigest) ||
		definition.ObservedFrom.IsZero() || definition.ObservedThrough.Before(definition.ObservedFrom) {
		return definition, ErrIntegrityViolation
	}
	if current && definition.EvaluationMode == calibrationEvaluationPostReview &&
		(strings.TrimSpace(definition.ReviewAnchorVersion) == "" ||
			!sha256DigestPattern.MatchString(definition.ReviewAnchorEvidenceDigest)) {
		return definition, ErrIntegrityViolation
	}
	if current && definition.EvaluationMode == calibrationEvaluationInitial &&
		(definition.ReviewAnchorVersion != "" || definition.ReviewAnchorEvidenceDigest != "") {
		return definition, ErrIntegrityViolation
	}
	return definition, nil
}

func exactMetricRatio(metrics []MetricResult, name string) (float64, bool) {
	for _, metric := range metrics {
		if metric.Name != name || metric.Direction != MetricExact || metric.Expected <= 0 || metric.Actual < 0 {
			continue
		}
		ratio := metric.Actual / metric.Expected
		if math.IsNaN(ratio) || math.IsInf(ratio, 0) {
			return 0, false
		}
		return ratio, true
	}
	return 0, false
}

func median(values []float64) float64 {
	ordered := append([]float64(nil), values...)
	sort.Float64s(ordered)
	middle := len(ordered) / 2
	if len(ordered)%2 == 1 {
		return ordered[middle]
	}
	return (ordered[middle-1] + ordered[middle]) / 2
}

func boundedCalibrationMultiplier(value float64) float64 {
	value = math.Max(0.5, math.Min(2, value))
	return roundedCalibrationValue(value)
}

func roundedCalibrationValue(value float64) float64 {
	return math.Round(value*1000) / 1000
}

func medianAbsoluteDeviation(values []float64, center float64) float64 {
	deviations := make([]float64, 0, len(values))
	for _, value := range values {
		deviations = append(deviations, math.Abs(value-center))
	}
	return median(deviations)
}

func calibrationConfidence(sampleCount int, dispersion float64) float64 {
	value := 0.5 + math.Min(float64(sampleCount), 10)*0.04 - math.Min(dispersion, 1)*0.2
	return roundedCalibrationValue(math.Max(0.5, math.Min(0.9, value)))
}

func materialCalibrationChange(current, proposed float64) bool {
	return math.Abs(current-proposed) >= 0.1
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
