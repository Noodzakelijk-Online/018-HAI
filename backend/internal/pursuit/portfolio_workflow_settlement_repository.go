package pursuit

import (
	"errors"
	"fmt"
	"strings"

	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/workflow"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type portfolioWorkflowSettlementCommit struct {
	Settlement *models.PursuitResourceReservationSettlement
	Proof      *models.PursuitPortfolioWorkflowSettlementProof
	Events     []models.PursuitResourceEvent
	Activities []models.PursuitActivity
}

type portfolioWorkflowSettlementRepository interface {
	FindPortfolioWorkflowSettlementProof(ownerIdentity string, itemID, workflowID uuid.UUID) (*models.PursuitPortfolioWorkflowSettlementProof, *models.PursuitResourceReservationSettlement, error)
	FindWorkflowCompletionAttestation(ownerIdentity string, workflowID uuid.UUID) (*models.WorkflowCompletionAttestation, error)
	SettleVerifiedPortfolioWorkflow(commit portfolioWorkflowSettlementCommit) (*models.PursuitPortfolioWorkflowSettlementProof, *models.PursuitResourceReservationSettlement, bool, error)
}

func (r *GormRepository) FindWorkflowCompletionAttestation(ownerIdentity string, workflowID uuid.UUID) (*models.WorkflowCompletionAttestation, error) {
	var attestation models.WorkflowCompletionAttestation
	err := r.DB.Where("owner_identity = ? AND workflow_id = ?", strings.TrimSpace(ownerIdentity), workflowID).First(&attestation).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &attestation, nil
}

func (r *GormRepository) FindPortfolioWorkflowSettlementProof(
	ownerIdentity string,
	itemID, workflowID uuid.UUID,
) (*models.PursuitPortfolioWorkflowSettlementProof, *models.PursuitResourceReservationSettlement, error) {
	var proof models.PursuitPortfolioWorkflowSettlementProof
	err := r.DB.Where(
		"owner_identity = ? AND proposal_item_id = ? AND workflow_id = ?",
		strings.TrimSpace(ownerIdentity), itemID, workflowID,
	).First(&proof).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var settlement models.PursuitResourceReservationSettlement
	if err := r.DB.Where("id = ? AND owner_identity = ?", proof.SettlementID, proof.OwnerIdentity).First(&settlement).Error; err != nil {
		return nil, nil, err
	}
	return &proof, &settlement, nil
}

func (r *GormRepository) SettleVerifiedPortfolioWorkflow(
	commit portfolioWorkflowSettlementCommit,
) (*models.PursuitPortfolioWorkflowSettlementProof, *models.PursuitResourceReservationSettlement, bool, error) {
	if err := validatePortfolioWorkflowSettlementCommit(commit); err != nil {
		return nil, nil, false, err
	}
	proof := commit.Proof
	settlement := commit.Settlement
	storedProof := models.PursuitPortfolioWorkflowSettlementProof{}
	storedSettlement := models.PursuitResourceReservationSettlement{}
	created := false
	err := r.DB.Transaction(func(tx *gorm.DB) error {
		var existing models.PursuitPortfolioWorkflowSettlementProof
		existingErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("reservation_id = ?", proof.ReservationID).First(&existing).Error
		if existingErr == nil {
			if existing.RequestDigest != proof.RequestDigest || existing.RecordDigest != proof.RecordDigest {
				return fmt.Errorf("portfolio workflow reservation was already settled with different proof")
			}
			if err := tx.Where("id = ?", existing.SettlementID).First(&storedSettlement).Error; err != nil {
				return err
			}
			storedProof = existing
			return nil
		}
		if !errors.Is(existingErr, gorm.ErrRecordNotFound) {
			return existingErr
		}

		var item models.PursuitPortfolioExecutionProposalItem
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where(
			"id = ? AND owner_identity = ? AND pursuit_id = ? AND reservation_id = ? AND record_digest = ?",
			proof.ProposalItemID, proof.OwnerIdentity, proof.PursuitID, proof.ReservationID, proof.ProposalItemDigest,
		).First(&item).Error; err != nil {
			return fmt.Errorf("lock portfolio proposal item: %w", err)
		}
		if item.Status == PortfolioExecutionProposalItemBlocked || len(item.BlockedReasons) > 0 {
			return fmt.Errorf("blocked portfolio proposal work cannot be settled")
		}

		var decision models.PursuitPortfolioExecutionProposalDecision
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where(
			"id = ? AND owner_identity = ? AND proposal_item_id = ? AND pursuit_id = ? AND proposal_item_digest = ? AND record_digest = ? AND decision = ?",
			proof.ApprovalDecisionID, proof.OwnerIdentity, proof.ProposalItemID, proof.PursuitID,
			proof.ProposalItemDigest, proof.ApprovalDecisionDigest, PortfolioExecutionDecisionApproved,
		).First(&decision).Error; err != nil {
			return fmt.Errorf("lock portfolio approval decision: %w", err)
		}

		var reservation models.PursuitResourceReservation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(
			"id = ? AND owner_identity = ? AND pursuit_id = ?",
			proof.ReservationID, proof.OwnerIdentity, proof.PursuitID,
		).First(&reservation).Error; err != nil {
			return fmt.Errorf("lock portfolio resource reservation: %w", err)
		}

		var workflowItem models.WorkflowItem
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where(
			"id = ? AND owner_identity = ? AND current_state = ?",
			proof.WorkflowID, proof.OwnerIdentity, workflow.StateCompleted,
		).First(&workflowItem).Error; err != nil {
			return fmt.Errorf("lock completed portfolio workflow: %w", err)
		}
		receiptURI := "hai://execution-authorization-receipts/" + proof.AuthorizationReceiptID.String()
		if workflowItem.SourceType != PortfolioWorkflowEffectSourceType ||
			workflowItem.SourceID != proof.AuthorizationReceiptID.String() ||
			strings.TrimSpace(workflowItem.SourceURI) != receiptURI {
			return fmt.Errorf("portfolio workflow no longer matches its immutable authorization receipt")
		}

		var attestation models.WorkflowCompletionAttestation
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where(
			"id = ? AND workflow_id = ? AND owner_identity = ? AND record_digest = ?",
			proof.CompletionAttestationID, proof.WorkflowID, proof.OwnerIdentity, proof.CompletionAttestationDigest,
		).First(&attestation).Error; err != nil {
			return fmt.Errorf("lock workflow completion attestation: %w", err)
		}
		if attestation.CompletionStatus != workflow.StateCompleted ||
			!portfolioSettlementAcceptsVerification(attestation.VerificationStatus) ||
			workflowItem.LastTaskPlanID != attestation.TaskPlanID ||
			workflowItem.CompletedAt == nil || !workflowItem.CompletedAt.Equal(attestation.CompletedAt) ||
			workflowItem.VerificationStatus != attestation.VerificationStatus {
			return fmt.Errorf("workflow projection does not match immutable verified completion")
		}

		var receipt struct {
			OwnerIdentity, ResourceID, ApprovalSourceID, Outcome, DecisionDigest string
		}
		row := tx.Raw(`SELECT owner_identity, resource_id, approval_source_id, outcome, decision_digest
			FROM public.execution_authorization_receipts
			WHERE owner_identity = ? AND id = ? FOR SHARE`, proof.OwnerIdentity, proof.AuthorizationReceiptID).Row()
		if err := row.Scan(&receipt.OwnerIdentity, &receipt.ResourceID, &receipt.ApprovalSourceID, &receipt.Outcome, &receipt.DecisionDigest); err != nil {
			return fmt.Errorf("lock portfolio authorization receipt: %w", err)
		}
		if receipt.ResourceID != proof.ProposalItemID.String() ||
			receipt.ApprovalSourceID != PortfolioWorkflowEffectApprovalSourcePrefix+proof.ApprovalDecisionID.String() ||
			receipt.Outcome != "authorized" || receipt.DecisionDigest != proof.AuthorizationReceiptDigest {
			return fmt.Errorf("portfolio authorization receipt does not match settlement proof")
		}

		var consumption struct {
			OwnerIdentity, Consumer, ExecutionTarget, ReceiptDigest string
		}
		row = tx.Raw(`SELECT owner_identity, consumer, execution_target, receipt_digest
			FROM public.execution_authorization_consumptions
			WHERE owner_identity = ? AND receipt_id = ? FOR SHARE`, proof.OwnerIdentity, proof.AuthorizationReceiptID).Row()
		if err := row.Scan(&consumption.OwnerIdentity, &consumption.Consumer, &consumption.ExecutionTarget, &consumption.ReceiptDigest); err != nil {
			return fmt.Errorf("lock portfolio authorization consumption: %w", err)
		}
		if consumption.Consumer != PortfolioWorkflowEffectConsumer ||
			consumption.ExecutionTarget != proof.AuthorizationTarget ||
			consumption.ReceiptDigest != proof.AuthorizationConsumptionKey {
			return fmt.Errorf("portfolio authorization consumption does not match settlement proof")
		}

		var receiptLinks int64
		if err := tx.Model(&models.PursuitLink{}).Where(
			"pursuit_id = ? AND link_type = ? AND relationship = ? AND source_uri = ?",
			proof.PursuitID, LinkWorkflow, PortfolioWorkflowEffectRelationship, receiptURI,
		).Count(&receiptLinks).Error; err != nil {
			return err
		}
		if receiptLinks != 1 {
			return fmt.Errorf("authorization receipt is linked to conflicting workflows")
		}
		var exactLinks int64
		if err := tx.Model(&models.PursuitLink{}).Where(
			"pursuit_id = ? AND link_type = ? AND link_id = ? AND relationship = ? AND source_uri = ?",
			proof.PursuitID, LinkWorkflow, proof.WorkflowID.String(), PortfolioWorkflowEffectRelationship, receiptURI,
		).Count(&exactLinks).Error; err != nil || exactLinks != 1 {
			return fmt.Errorf("exact receipt-bound portfolio workflow link is unavailable")
		}

		if err := tx.Create(settlement).Error; err != nil {
			return err
		}
		if err := tx.Create(proof).Error; err != nil {
			return err
		}
		for index := range commit.Events {
			if err := tx.Create(&commit.Events[index]).Error; err != nil {
				return err
			}
		}
		if err := appendResourceActivities(tx, settlement.PursuitID, commit.Activities); err != nil {
			return err
		}
		storedProof = *proof
		storedSettlement = *settlement
		created = true
		return nil
	})
	if err != nil {
		return nil, nil, false, err
	}
	return &storedProof, &storedSettlement, created, nil
}

func validatePortfolioWorkflowSettlementCommit(commit portfolioWorkflowSettlementCommit) error {
	if commit.Settlement == nil || commit.Proof == nil {
		return fmt.Errorf("portfolio workflow settlement and proof are required")
	}
	settlement, proof := commit.Settlement, commit.Proof
	if settlement.ID == uuid.Nil || proof.ID == uuid.Nil || proof.SettlementID != settlement.ID ||
		proof.SettlementDigest != settlement.RecordDigest || proof.ReservationID != settlement.ReservationID ||
		proof.PursuitID != settlement.PursuitID || proof.OwnerIdentity != settlement.OwnerIdentity ||
		proof.ActualEffortMinutes != settlement.ActualEffortMinutes || proof.ActualCostMicros != settlement.ActualCostMicros ||
		proof.Currency != settlement.Currency || proof.Actor != settlement.Actor || !proof.CreatedAt.Equal(settlement.SettledAt) ||
		settlement.Disposition != ResourceReservationConsumed {
		return fmt.Errorf("portfolio workflow settlement proof does not match its accounting record")
	}
	if proof.ProposalItemID == uuid.Nil || proof.ApprovalDecisionID == uuid.Nil ||
		proof.AuthorizationReceiptID == uuid.Nil || proof.WorkflowID == uuid.Nil ||
		proof.CompletionAttestationID == uuid.Nil || proof.RecordDigest == "" || proof.RequestDigest == "" {
		return fmt.Errorf("complete portfolio workflow settlement proof is required")
	}
	return nil
}
