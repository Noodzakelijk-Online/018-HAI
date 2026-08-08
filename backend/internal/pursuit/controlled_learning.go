package pursuit

import (
	"context"
	"fmt"
	"strings"
	"time"

	"automation-hub-backend/internal/controlledlearning"
	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
)

const portfolioControlledLearningTimeout = 5 * time.Second

const (
	PortfolioLearningRecorded            = "evidence_recorded"
	PortfolioLearningUnavailable         = "unavailable"
	PortfolioLearningFailed              = "recording_failed"
	PortfolioLearningProposalUnavailable = "proposal_unavailable"
	PortfolioLearningProposalFailed      = "proposal_failed"
)

// ControlledLearningRecorder is deliberately narrower than the controlled
// learning service. Pursuit settlement can add verified evidence, but it
// cannot approve or apply a change.
type ControlledLearningRecorder interface {
	RecordOutcome(
		context.Context,
		controlledlearning.RecordOutcomeRequest,
	) (controlledlearning.OutcomeRecord, error)
}

type PortfolioCalibrationReader interface {
	LatestAppliedEstimateCalibration(
		context.Context,
		string,
		string,
	) (*controlledlearning.AppliedEstimateCalibration, error)
	AppliedEstimateCalibration(
		context.Context,
		string,
		string,
		string,
	) (*controlledlearning.AppliedEstimateCalibration, error)
}

type controlledLearningCalibration interface {
	PortfolioCalibrationReader
	ProposeEstimateCalibration(
		context.Context,
		string,
		string,
	) (controlledlearning.EstimateCalibrationProposalResult, error)
}

type portfolioSettlementLearningResult struct {
	OutcomeID        string
	Status           string
	ProposalID       string
	ProposalStatus   string
	SampleCount      int
	NewEvidenceCount int
	DriftDetected    bool
	ReviewRequired   bool
}

// WithControlledLearning attaches the governed outcome ledger to the
// canonical pursuit service. Unsupported service implementations remain
// side-effect free for protocol previews and test doubles.
func WithControlledLearning(value Service, recorder ControlledLearningRecorder) Service {
	concrete, ok := value.(*service)
	if !ok || concrete == nil || recorder == nil {
		return value
	}
	concrete.controlledLearning = recorder
	if calibration, supported := recorder.(controlledLearningCalibration); supported {
		concrete.portfolioCalibration = calibration
	}
	return concrete
}

func (s *service) recordPortfolioSettlementOutcome(
	proof *models.PursuitPortfolioWorkflowSettlementProof,
	settlement *models.PursuitResourceReservationSettlement,
	attestation *models.WorkflowCompletionAttestation,
) portfolioSettlementLearningResult {
	if s == nil || s.controlledLearning == nil {
		return portfolioSettlementLearningResult{Status: PortfolioLearningUnavailable, ProposalStatus: PortfolioLearningProposalUnavailable}
	}
	if proof == nil || settlement == nil || attestation == nil ||
		proof.ID.String() == "" || strings.TrimSpace(proof.OwnerIdentity) == "" ||
		proof.RecordDigest == "" || settlement.RecordDigest == "" ||
		attestation.RecordDigest == "" {
		return portfolioSettlementLearningResult{Status: PortfolioLearningFailed, ProposalStatus: PortfolioLearningProposalUnavailable}
	}

	verification, ok := portfolioControlledLearningVerification(attestation.VerificationStatus)
	if !ok {
		return portfolioSettlementLearningResult{Status: PortfolioLearningFailed, ProposalStatus: PortfolioLearningProposalUnavailable}
	}
	projectKey := ""
	if pursuitRecord, err := s.repo.FindByID(proof.PursuitID); err == nil &&
		pursuitRecord != nil && pursuitRecord.OwnerIdentity == proof.OwnerIdentity {
		projectKey = strings.TrimSpace(pursuitRecord.ProjectKey)
	}
	occurredAt := proof.CreatedAt.UTC()
	if occurredAt.IsZero() {
		occurredAt = settlement.SettledAt.UTC()
	}
	if occurredAt.IsZero() {
		return portfolioSettlementLearningResult{Status: PortfolioLearningFailed, ProposalStatus: PortfolioLearningProposalUnavailable}
	}
	metrics := []controlledlearning.MetricResult{}
	if reservations, ok := s.repo.(pursuitResourceReservationRepository); ok {
		if reservation, err := reservations.FindResourceReservationByID(
			proof.OwnerIdentity, proof.PursuitID, proof.ReservationID,
		); err == nil && reservation != nil {
			metrics = append(metrics,
				controlledlearning.MetricResult{
					Name: "portfolio_effort_minutes", Expected: float64(reservation.EstimatedEffortMinutes),
					Actual: float64(settlement.ActualEffortMinutes), Direction: controlledlearning.MetricExact,
					Unit: "minutes",
				},
				controlledlearning.MetricResult{
					Name: "portfolio_cost_micros", Expected: float64(reservation.EstimatedCostMicros),
					Actual: float64(settlement.ActualCostMicros), Direction: controlledlearning.MetricExact,
					Unit: "EUR_micros",
				},
			)
		}
	}

	scopeKey := portfolioCalibrationScope(projectKey, proof.PursuitID)
	ctx, cancel := context.WithTimeout(context.Background(), portfolioControlledLearningTimeout)
	defer cancel()
	outcome, err := s.controlledLearning.RecordOutcome(ctx, controlledlearning.RecordOutcomeRequest{
		OwnerIdentity:  proof.OwnerIdentity,
		IdempotencyKey: "portfolio-workflow-settlement:" + proof.ID.String(),
		OperationID:    proof.WorkflowID.String(),
		ProjectKey:     scopeKey,
		Basis:          controlledlearning.EvidenceVerifiedOutcome,
		Status:         controlledlearning.OutcomeSucceeded,
		Summary: fmt.Sprintf(
			"Receipt-bound portfolio workflow %s completed with %s verification; %d effort minute(s) and EUR %.6f were reconciled.",
			proof.WorkflowID, attestation.VerificationStatus, settlement.ActualEffortMinutes,
			float64(settlement.ActualCostMicros)/1_000_000,
		),
		Verification: verification,
		Sources: []controlledlearning.SourceReference{
			{
				ID: "portfolio-settlement-proof:" + proof.ID.String(), Kind: "portfolio_workflow_settlement_proof",
				URI:         "hai://pursuit-portfolio-workflow-settlement-proofs/" + proof.ID.String(),
				RetrievedAt: occurredAt, ContentHash: proof.RecordDigest,
			},
			{
				ID: "workflow-attestation:" + attestation.ID.String(), Kind: "workflow_completion_attestation",
				URI:         "hai://workflow-completion-attestations/" + attestation.ID.String(),
				RetrievedAt: occurredAt, ContentHash: attestation.RecordDigest,
			},
			{
				ID: "authorization-receipt:" + proof.AuthorizationReceiptID.String(), Kind: "execution_authorization_receipt",
				URI:         "hai://execution-authorization-receipts/" + proof.AuthorizationReceiptID.String(),
				RetrievedAt: occurredAt, ContentHash: proof.AuthorizationReceiptDigest,
			},
		},
		Criteria: []controlledlearning.CriterionResult{
			{ID: "receipt_consumed", Description: "The approved execution receipt was consumed for one exact workflow.", Passed: true, SourceIDs: []string{"authorization-receipt:" + proof.AuthorizationReceiptID.String()}},
			{ID: "completion_attested", Description: "The workflow has immutable verified completion evidence.", Passed: true, SourceIDs: []string{"workflow-attestation:" + attestation.ID.String()}},
			{ID: "usage_settled", Description: "Measured usage was settled against the reserved portfolio allocation.", Passed: true, SourceIDs: []string{"portfolio-settlement-proof:" + proof.ID.String()}},
		},
		Metrics:    metrics,
		Tags:       []string{"pursuit-engine", "portfolio-settlement", "outcome-reconciliation", attestation.VerificationStatus},
		OccurredAt: occurredAt,
	})
	if err != nil {
		return portfolioSettlementLearningResult{Status: PortfolioLearningFailed, ProposalStatus: PortfolioLearningProposalUnavailable}
	}
	result := portfolioSettlementLearningResult{
		OutcomeID:      outcome.ID,
		Status:         PortfolioLearningRecorded,
		ProposalStatus: PortfolioLearningProposalUnavailable,
	}
	calibration, ok := s.controlledLearning.(controlledLearningCalibration)
	if !ok {
		return result
	}
	proposal, err := calibration.ProposeEstimateCalibration(ctx, proof.OwnerIdentity, scopeKey)
	if err != nil {
		result.ProposalStatus = PortfolioLearningProposalFailed
		return result
	}
	result.ProposalStatus = proposal.Status
	result.SampleCount = proposal.SampleCount
	result.NewEvidenceCount = proposal.NewEvidenceCount
	result.DriftDetected = proposal.DriftDetected
	if proposal.Proposal != nil {
		result.ProposalID = proposal.Proposal.ID
		result.ReviewRequired = portfolioCalibrationProposalNeedsReview(proposal.Proposal.Status)
	}
	return result
}

func portfolioCalibrationProposalNeedsReview(status controlledlearning.ProposalStatus) bool {
	switch status {
	case controlledlearning.ProposalReviewRequired,
		controlledlearning.ProposalChangesRequested,
		controlledlearning.ProposalGovernanceRequired:
		return true
	default:
		return false
	}
}

func portfolioCalibrationScope(projectKey string, pursuitID uuid.UUID) string {
	if project := strings.TrimSpace(projectKey); project != "" {
		return "project:" + project
	}
	return "pursuit:" + pursuitID.String()
}

func portfolioControlledLearningVerification(value string) (controlledlearning.VerificationStatus, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(controlledlearning.VerificationVerified):
		return controlledlearning.VerificationVerified, true
	case string(controlledlearning.VerificationTestPassed):
		return controlledlearning.VerificationTestPassed, true
	default:
		return "", false
	}
}
