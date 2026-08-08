package controlledlearning

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestEstimateCalibrationRequiresThreeComparableVerifiedSettlements(t *testing.T) {
	t.Parallel()
	service := newTestService(t)
	recordPortfolioCalibrationOutcome(t, service, "sample-1", "project:hai", 100, 120, 1_000_000, 1_100_000, 2*time.Hour)
	recordPortfolioCalibrationOutcome(t, service, "sample-2", "project:hai", 100, 130, 1_000_000, 1_200_000, time.Hour)

	result, err := service.ProposeEstimateCalibration(context.Background(), "robert", "project:hai")
	if err != nil {
		t.Fatalf("ProposeEstimateCalibration: %v", err)
	}
	if result.Status != EstimateCalibrationInsufficientEvidence || result.SampleCount != 2 || result.Proposal != nil {
		t.Fatalf("insufficient calibration result = %#v", result)
	}
}

func TestEstimateCalibrationUsesRobustEvidenceAndDeterministicReplay(t *testing.T) {
	t.Parallel()
	service := newTestService(t)
	recordPortfolioCalibrationOutcome(t, service, "sample-1", "project:hai", 100, 120, 1_000_000, 1_100_000, 3*time.Hour)
	recordPortfolioCalibrationOutcome(t, service, "sample-2", "project:hai", 100, 125, 1_000_000, 1_200_000, 2*time.Hour)
	recordPortfolioCalibrationOutcome(t, service, "sample-outlier", "project:hai", 100, 1_000, 1_000_000, 9_000_000, time.Hour)
	recordPortfolioCalibrationOutcome(t, service, "other-scope", "project:other", 100, 200, 1_000_000, 2_000_000, time.Hour)
	recordNonComparablePortfolioOutcome(t, service)

	first, err := service.ProposeEstimateCalibration(context.Background(), "robert", "project:hai")
	if err != nil {
		t.Fatalf("ProposeEstimateCalibration: %v", err)
	}
	second, err := service.ProposeEstimateCalibration(context.Background(), "robert", "project:hai")
	if err != nil {
		t.Fatalf("deterministic replay: %v", err)
	}
	if first.Status != string(ProposalReviewRequired) || first.SampleCount != 3 || first.Proposal == nil ||
		second.Proposal == nil || second.Proposal.ID != first.Proposal.ID {
		t.Fatalf("review proposal replay first=%#v second=%#v", first, second)
	}
	definition, err := decodeEstimateCalibration(first.Proposal.ProposedChange)
	if err != nil {
		t.Fatalf("decode calibration: %v", err)
	}
	if definition.EffortMultiplier != 1.25 || definition.CostMultiplier != 1.2 ||
		definition.SampleCount != 3 || definition.CostSampleCount != 3 ||
		definition.AlgorithmVersion != calibrationAlgorithm || definition.EvidenceDigest == "" ||
		definition.Confidence < 0.5 || definition.Confidence > 0.9 || definition.ObservedFrom.IsZero() {
		t.Fatalf("robust calibration definition = %#v", definition)
	}
	if first.Proposal.Target != TargetPlanningEstimateCalibration || first.Proposal.ProtectedTarget {
		t.Fatalf("calibration target boundary = %#v", first.Proposal)
	}
}

func TestApprovedEstimateCalibrationIsExactStableAndRollbackAware(t *testing.T) {
	t.Parallel()
	repository := NewMemoryRepository()
	service, err := NewService(repository, func() time.Time { return fixedNow }, sequenceIDs())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	for index, actual := range []float64{120, 125, 130} {
		recordPortfolioCalibrationOutcome(
			t, service, "approved-sample-"+string(rune('a'+index)), "project:hai",
			100, actual, 1_000_000, 1_200_000, time.Duration(3-index)*time.Hour,
		)
	}
	proposed, err := service.ProposeEstimateCalibration(context.Background(), "robert", "project:hai")
	if err != nil || proposed.Proposal == nil {
		t.Fatalf("proposal result=%#v err=%v", proposed, err)
	}
	decision := approvedDecision(*proposed.Proposal)
	decision.IdempotencyKey = "approve-estimate-calibration"
	approved, err := service.DecideAndApply(context.Background(), decision)
	if err != nil || approved.Application == nil {
		t.Fatalf("DecideAndApply result=%#v err=%v", approved, err)
	}
	latest, err := service.LatestAppliedEstimateCalibration(context.Background(), "robert", "project:hai")
	if err != nil || latest == nil || latest.ProposalID != proposed.Proposal.ID ||
		latest.ApplicationID != approved.Application.ID || latest.EffortMultiplier != 1.25 {
		t.Fatalf("latest calibration=%#v err=%v", latest, err)
	}
	exact, err := service.AppliedEstimateCalibration(
		context.Background(), "robert", "project:hai", latest.ProposalVersion,
	)
	if err != nil || exact == nil || exact.ApplicationID != latest.ApplicationID {
		t.Fatalf("exact calibration=%#v err=%v", exact, err)
	}
	monitoring, err := service.ProposeEstimateCalibration(context.Background(), "robert", "project:hai")
	if err != nil || monitoring.Status != EstimateCalibrationMonitoring || monitoring.Proposal != nil ||
		monitoring.SampleCount != 0 || monitoring.Calibration == nil {
		t.Fatalf("post-approval monitoring result=%#v err=%v", monitoring, err)
	}
	_, err = service.Rollback(context.Background(), RollbackRequest{
		OwnerIdentity: "robert", ApplicationID: approved.Application.ID,
		IdempotencyKey: "rollback-estimate-calibration", ActorIdentity: "robert",
		HumanConfirmed: true, Rationale: "The reviewed estimate calibration should no longer influence new plans.",
		ExpectedVersion: latest.ProposalVersion,
	})
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if current, err := service.LatestAppliedEstimateCalibration(context.Background(), "robert", "project:hai"); err != nil || current != nil {
		t.Fatalf("rolled-back latest=%#v err=%v", current, err)
	}
	if _, err := service.AppliedEstimateCalibration(context.Background(), "robert", "project:hai", latest.ProposalVersion); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rolled-back exact lookup error=%v", err)
	}
}

func TestEstimateCalibrationUsesFreshPostReviewCohortForDrift(t *testing.T) {
	t.Parallel()
	service := newTestService(t)
	for index, actual := range []float64{120, 125, 130} {
		recordPortfolioCalibrationOutcome(
			t, service, "baseline-"+string(rune('a'+index)), "project:hai",
			100, actual, 1_000_000, 1_200_000, time.Duration(3-index)*time.Hour,
		)
	}
	baseline, err := service.ProposeEstimateCalibration(context.Background(), "robert", "project:hai")
	if err != nil || baseline.Proposal == nil {
		t.Fatalf("baseline proposal=%#v err=%v", baseline, err)
	}
	baselineDefinition, err := decodeEstimateCalibration(baseline.Proposal.ProposedChange)
	if err != nil {
		t.Fatalf("decode baseline proposal: %v", err)
	}
	decision := approvedDecision(*baseline.Proposal)
	decision.IdempotencyKey = "approve-drift-baseline"
	applied, err := service.DecideAndApply(context.Background(), decision)
	if err != nil || applied.Application == nil {
		t.Fatalf("apply baseline=%#v err=%v", applied, err)
	}

	recordPortfolioCalibrationOutcome(t, service, "drift-a", "project:hai", 100, 160, 1_000_000, 1_600_000, 45*time.Minute)
	recordPortfolioCalibrationOutcome(t, service, "drift-b", "project:hai", 100, 165, 1_000_000, 1_650_000, 30*time.Minute)
	monitoring, err := service.ProposeEstimateCalibration(context.Background(), "robert", "project:hai")
	if err != nil || monitoring.Status != EstimateCalibrationMonitoring || monitoring.SampleCount != 2 || monitoring.Proposal != nil {
		t.Fatalf("two-sample monitoring=%#v err=%v", monitoring, err)
	}

	recordPortfolioCalibrationOutcome(t, service, "drift-c", "project:hai", 100, 170, 1_000_000, 1_700_000, 15*time.Minute)
	drift, err := service.ProposeEstimateCalibration(context.Background(), "robert", "project:hai")
	if err != nil || drift.Status != string(ProposalReviewRequired) || drift.Proposal == nil ||
		drift.SampleCount != 3 || !drift.DriftDetected {
		t.Fatalf("drift proposal=%#v err=%v", drift, err)
	}
	definition, err := decodeEstimateCalibration(drift.Proposal.ProposedChange)
	if err != nil {
		t.Fatalf("decode drift proposal: %v", err)
	}
	if definition.Version != 2 || definition.AlgorithmVersion != calibrationAlgorithm ||
		definition.EvaluationMode != calibrationEvaluationPostReview ||
		definition.ReviewAnchorVersion != baseline.Proposal.ProposedVersion ||
		definition.ReviewAnchorEvidenceDigest == "" || definition.SampleCount != 3 ||
		!definition.ObservedFrom.After(baselineDefinition.ObservedThrough) ||
		definition.EffortMultiplier != 1.65 || definition.CostMultiplier != 1.65 {
		t.Fatalf("drift definition=%#v", definition)
	}
	if len(drift.Proposal.EvidenceIDs) != 3 {
		t.Fatalf("drift evidence must contain only fresh cohort: %#v", drift.Proposal.EvidenceIDs)
	}
}

func TestDecodeEstimateCalibrationAcceptsLegacyV1Proposal(t *testing.T) {
	t.Parallel()
	legacy := EstimateCalibrationDefinition{
		Kind: "portfolio_estimate_calibration", Version: 1, AlgorithmVersion: calibrationAlgorithmV1,
		ScopeKey: "project:hai", SampleCount: 3, CostSampleCount: 3,
		EffortMultiplier: 1.2, CostMultiplier: 1.1,
		EffortDispersion: 0.05, CostDispersion: 0.04, Confidence: 0.62,
		EvidenceDigest: "sha256:" + strings.Repeat("a", 64),
		ObservedFrom:   fixedNow.Add(-3 * time.Hour), ObservedThrough: fixedNow.Add(-time.Hour),
	}
	payload, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy calibration: %v", err)
	}
	decoded, err := decodeEstimateCalibration(string(payload))
	if err != nil {
		t.Fatalf("decode legacy calibration: %v", err)
	}
	if decoded.Version != 1 || decoded.AlgorithmVersion != calibrationAlgorithmV1 ||
		decoded.EvaluationMode != "" || decoded.ReviewAnchorVersion != "" {
		t.Fatalf("legacy calibration changed during decode: %#v", decoded)
	}
}

func TestEstimateCalibrationKeepsOnePendingReviewPerScope(t *testing.T) {
	t.Parallel()
	service := newTestService(t)
	for index, actual := range []float64{120, 125, 130} {
		recordPortfolioCalibrationOutcome(
			t, service, "pending-"+string(rune('a'+index)), "project:hai",
			100, actual, 1_000_000, 1_200_000, time.Duration(3-index)*time.Hour,
		)
	}
	first, err := service.ProposeEstimateCalibration(context.Background(), "robert", "project:hai")
	if err != nil || first.Proposal == nil {
		t.Fatalf("first proposal=%#v err=%v", first, err)
	}
	for index, actual := range []float64{160, 165, 170} {
		recordPortfolioCalibrationOutcome(
			t, service, "pending-new-"+string(rune('a'+index)), "project:hai",
			100, actual, 1_000_000, 1_600_000, time.Duration(45-index*10)*time.Minute,
		)
	}
	again, err := service.ProposeEstimateCalibration(context.Background(), "robert", "project:hai")
	if err != nil || again.Proposal == nil || again.Proposal.ID != first.Proposal.ID ||
		again.NewEvidenceCount != 3 || !again.DriftDetected {
		t.Fatalf("pending review suppression=%#v err=%v", again, err)
	}
	proposals, err := service.repository.ListProposals(context.Background(), ProposalQuery{OwnerIdentity: "robert"})
	if err != nil || len(proposals) != 1 {
		t.Fatalf("pending proposal count=%d err=%v", len(proposals), err)
	}
}

func TestRejectedCalibrationAnchorsEvidenceAndNeedsFreshCohort(t *testing.T) {
	t.Parallel()
	service := newTestService(t)
	for index := 0; index < 3; index++ {
		recordPortfolioCalibrationOutcome(
			t, service, "rejected-"+string(rune('a'+index)), "project:hai",
			100, 130, 1_000_000, 1_300_000, time.Duration(3-index)*time.Hour,
		)
	}
	proposed, err := service.ProposeEstimateCalibration(context.Background(), "robert", "project:hai")
	if err != nil || proposed.Proposal == nil {
		t.Fatalf("proposal=%#v err=%v", proposed, err)
	}
	_, err = service.DecideAndApply(context.Background(), DecideRequest{
		OwnerIdentity: "robert", ProposalID: proposed.Proposal.ID, ExpectedRevision: proposed.Proposal.Revision,
		IdempotencyKey: "reject-calibration", Kind: DecisionReject, ActorIdentity: "robert",
		HumanConfirmed: true, Rationale: "This cohort should not change future estimates.",
	})
	if err != nil {
		t.Fatalf("reject calibration: %v", err)
	}
	monitoring, err := service.ProposeEstimateCalibration(context.Background(), "robert", "project:hai")
	if err != nil || monitoring.Status != EstimateCalibrationMonitoring || monitoring.SampleCount != 0 || monitoring.Proposal != nil {
		t.Fatalf("rejected evidence replay=%#v err=%v", monitoring, err)
	}
	for index := 0; index < 3; index++ {
		recordPortfolioCalibrationOutcome(
			t, service, "post-rejection-"+string(rune('a'+index)), "project:hai",
			100, 150, 1_000_000, 1_500_000, time.Duration(45-index*10)*time.Minute,
		)
	}
	fresh, err := service.ProposeEstimateCalibration(context.Background(), "robert", "project:hai")
	if err != nil || fresh.Proposal == nil || fresh.Proposal.ID == proposed.Proposal.ID || fresh.SampleCount != 3 {
		t.Fatalf("fresh post-rejection proposal=%#v err=%v", fresh, err)
	}
}

func TestEstimateCalibrationSuppressesNeutralHistory(t *testing.T) {
	t.Parallel()
	service := newTestService(t)
	for index := 0; index < 3; index++ {
		recordPortfolioCalibrationOutcome(
			t, service, "neutral-sample-"+string(rune('a'+index)), "project:hai",
			100, 100, 1_000_000, 1_000_000, time.Duration(3-index)*time.Hour,
		)
	}
	result, err := service.ProposeEstimateCalibration(context.Background(), "robert", "project:hai")
	if err != nil || result.Status != EstimateCalibrationStable || result.Proposal != nil {
		t.Fatalf("neutral calibration result=%#v err=%v", result, err)
	}
}

func recordPortfolioCalibrationOutcome(
	t *testing.T,
	service *Service,
	id string,
	scope string,
	expectedEffort float64,
	actualEffort float64,
	expectedCost float64,
	actualCost float64,
	age time.Duration,
) {
	t.Helper()
	request := verifiedOutcomeRequest(id)
	request.ProjectKey = scope
	request.Status = OutcomeSucceeded
	request.Verification = VerificationVerified
	request.Tags = []string{"portfolio-settlement", "outcome-reconciliation"}
	request.Criteria = []CriterionResult{{
		ID: "settled", Description: "The verified workflow usage was settled.", Passed: true,
		SourceIDs: []string{request.Sources[0].ID},
	}}
	request.Metrics = []MetricResult{
		{Name: portfolioEffortMetric, Expected: expectedEffort, Actual: actualEffort, Direction: MetricExact, Unit: "minutes"},
		{Name: portfolioCostMetric, Expected: expectedCost, Actual: actualCost, Direction: MetricExact, Unit: "EUR_micros"},
	}
	request.OccurredAt = fixedNow.Add(-age)
	request.Sources[0].RetrievedAt = request.OccurredAt
	if _, err := service.RecordOutcome(context.Background(), request); err != nil {
		t.Fatalf("RecordOutcome(%s): %v", id, err)
	}
}

func recordNonComparablePortfolioOutcome(t *testing.T, service *Service) {
	t.Helper()
	request := verifiedOutcomeRequest("non-comparable")
	request.ProjectKey = "project:hai"
	request.Status = OutcomeFailed
	request.Tags = []string{"portfolio-settlement"}
	request.OccurredAt = fixedNow.Add(-time.Hour)
	request.Sources[0].RetrievedAt = request.OccurredAt
	if _, err := service.RecordOutcome(context.Background(), request); err != nil {
		t.Fatalf("RecordOutcome(non-comparable): %v", err)
	}
}
