package pursuit

import (
	"automation-hub-backend/internal/models"
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *GormRepository) LoadPortfolioWorkflowEffectApprovalSnapshot(
	ctx context.Context,
	ownerIdentity string,
	decisionID uuid.UUID,
) (*PortfolioWorkflowEffectApprovalSnapshot, error) {
	if r == nil || r.DB == nil {
		return nil, fmt.Errorf("pursuit portfolio workflow approval repository is unavailable")
	}
	if ctx == nil {
		return nil, fmt.Errorf("context is required")
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	if ownerIdentity == "" || decisionID == uuid.Nil {
		return nil, fmt.Errorf("valid portfolio workflow approval owner and decision id are required")
	}
	db := r.DB.WithContext(ctx)
	var decision models.PursuitPortfolioExecutionProposalDecision
	if err := db.Where("id = ? AND owner_identity = ?", decisionID, ownerIdentity).First(&decision).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	snapshot, err := loadPortfolioExecutionProposalDecisionSnapshot(
		db, ownerIdentity, decision.ProposalItemID, false,
	)
	if err != nil || snapshot == nil {
		return nil, err
	}
	return &PortfolioWorkflowEffectApprovalSnapshot{
		Allocation: snapshot.Allocation, Proposal: snapshot.Proposal, Item: snapshot.Item,
		AllocationItem: snapshot.AllocationItem, Pursuit: snapshot.Pursuit,
		Decision: decision, LatestDecision: snapshot.LatestDecision,
		Settled: snapshot.Settled,
	}, nil
}

func (r *GormRepository) LoadPortfolioExecutionProposalDecisionSnapshot(
	ownerIdentity string,
	itemID uuid.UUID,
) (*portfolioExecutionProposalDecisionSnapshot, error) {
	if r == nil || r.DB == nil {
		return nil, fmt.Errorf("pursuit portfolio execution proposal decision repository is unavailable")
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	if ownerIdentity == "" || itemID == uuid.Nil {
		return nil, fmt.Errorf("valid proposal decision owner and item id are required")
	}
	return loadPortfolioExecutionProposalDecisionSnapshot(r.DB, ownerIdentity, itemID, false)
}

func loadPortfolioExecutionProposalDecisionSnapshot(
	db *gorm.DB,
	ownerIdentity string,
	itemID uuid.UUID,
	lock bool,
) (*portfolioExecutionProposalDecisionSnapshot, error) {
	query := db.Where("id = ? AND owner_identity = ?", itemID, ownerIdentity)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var item models.PursuitPortfolioExecutionProposalItem
	if err := query.First(&item).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	var proposal models.PursuitPortfolioExecutionProposal
	if err := db.Where("id = ? AND owner_identity = ?", item.ProposalID, ownerIdentity).First(&proposal).Error; err != nil {
		return nil, err
	}
	var allocation models.PursuitPortfolioAllocation
	if err := db.Where(
		"id = ? AND owner_identity = ? AND record_digest = ?",
		proposal.AllocationID, ownerIdentity, proposal.AllocationRecordDigest,
	).First(&allocation).Error; err != nil {
		return nil, err
	}
	var allocationItem models.PursuitPortfolioAllocationItem
	if err := db.Where(
		"id = ? AND owner_identity = ? AND pursuit_id = ? AND reservation_id = ? AND record_digest = ?",
		item.AllocationItemID, ownerIdentity, item.PursuitID, item.ReservationID, item.AllocationItemDigest,
	).First(&allocationItem).Error; err != nil {
		return nil, err
	}
	var pursuit models.Pursuit
	if err := db.Where("id = ? AND owner_identity = ?", item.PursuitID, ownerIdentity).First(&pursuit).Error; err != nil {
		return nil, err
	}
	var settlementCount int64
	if err := db.Model(&models.PursuitResourceReservationSettlement{}).Where(
		"reservation_id = ? AND owner_identity = ?", item.ReservationID, ownerIdentity,
	).Count(&settlementCount).Error; err != nil {
		return nil, err
	}
	var latest models.PursuitPortfolioExecutionProposalDecision
	latestQuery := db.Where("proposal_item_id = ? AND owner_identity = ?", itemID, ownerIdentity).
		Order("decided_at DESC, id DESC")
	if lock {
		latestQuery = latestQuery.Clauses(clause.Locking{Strength: "SHARE"})
	}
	latestErr := latestQuery.First(&latest).Error
	if latestErr != nil && latestErr != gorm.ErrRecordNotFound {
		return nil, latestErr
	}
	snapshot := &portfolioExecutionProposalDecisionSnapshot{
		Allocation: allocation, Proposal: proposal, Item: item, AllocationItem: allocationItem,
		Pursuit: pursuit, Settled: settlementCount > 0,
	}
	if latestErr == nil {
		snapshot.LatestDecision = &latest
	}
	return snapshot, nil
}

func (r *GormRepository) SavePortfolioExecutionProposalDecision(
	wantedSnapshot *portfolioExecutionProposalDecisionSnapshot,
	decision *models.PursuitPortfolioExecutionProposalDecision,
	activity models.PursuitActivity,
) (*models.PursuitPortfolioExecutionProposalDecision, bool, error) {
	if r == nil || r.DB == nil {
		return nil, false, fmt.Errorf("pursuit portfolio execution proposal decision repository is unavailable")
	}
	if wantedSnapshot == nil || decision == nil {
		return nil, false, fmt.Errorf("portfolio execution proposal decision evidence is required")
	}
	if err := validatePortfolioExecutionDecisionEvidence(decision.OwnerIdentity, wantedSnapshot.Item, decision); err != nil {
		return nil, false, err
	}
	if activity.ID == uuid.Nil || activity.PursuitID != decision.PursuitID ||
		activity.EventType != "pursuit.portfolio_execution_proposal_decided" ||
		activity.Actor != decision.Actor || activity.SourceType != "pursuit_portfolio_execution_proposal_decision" ||
		activity.SourceID != decision.ID.String() || activity.CreatedAt.IsZero() {
		return nil, false, fmt.Errorf("portfolio execution proposal decision activity is invalid")
	}
	stored := models.PursuitPortfolioExecutionProposalDecision{}
	created := false
	err := r.DB.Transaction(func(tx *gorm.DB) error {
		current, err := loadPortfolioExecutionProposalDecisionSnapshot(tx, decision.OwnerIdentity, decision.ProposalItemID, true)
		if err != nil {
			return err
		}
		if current == nil {
			return fmt.Errorf("portfolio execution proposal item changed or is unavailable")
		}
		if err := validatePortfolioExecutionDecisionSource(decision.OwnerIdentity, current); err != nil {
			return err
		}
		settled := map[uuid.UUID]struct{}{}
		if current.Settled {
			settled[current.Item.ReservationID] = struct{}{}
		}
		stateDigest, err := digestPortfolioExecutionState(current.Pursuit, current.AllocationItem, settled)
		if err != nil || stateDigest != decision.StateDigest || current.Item.RecordDigest != decision.ProposalItemDigest {
			return fmt.Errorf("portfolio execution proposal state changed before decision persistence")
		}
		if current.Item.Status == PortfolioExecutionProposalItemBlocked || len(current.Item.BlockedReasons) > 0 {
			return fmt.Errorf("blocked portfolio execution proposal items cannot be decided")
		}
		if current.LatestDecision != nil && current.LatestDecision.RequestDigest == decision.RequestDigest {
			if err := validatePortfolioExecutionDecisionEvidence(decision.OwnerIdentity, current.Item, current.LatestDecision); err != nil {
				return err
			}
			stored = *current.LatestDecision
			return nil
		}
		currentPrevious := uuid.Nil
		if current.LatestDecision != nil {
			currentPrevious = current.LatestDecision.ID
		}
		wantedPrevious := uuid.Nil
		if decision.PreviousDecisionID != nil {
			wantedPrevious = *decision.PreviousDecisionID
		}
		if currentPrevious != wantedPrevious {
			return fmt.Errorf("portfolio execution proposal decision chain changed; retry from current history")
		}
		if decision.Decision == PortfolioExecutionDecisionRevoked &&
			(current.LatestDecision == nil || current.LatestDecision.Decision != PortfolioExecutionDecisionApproved) {
			return fmt.Errorf("only the latest approved proposal item decision can be revoked")
		}
		if err := tx.Create(decision).Error; err != nil {
			return err
		}
		if err := appendResourceActivities(tx, activity.PursuitID, []models.PursuitActivity{activity}); err != nil {
			return fmt.Errorf("create portfolio execution proposal decision activity: %w", err)
		}
		stored = *decision
		created = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return &stored, created, nil
}

func (r *GormRepository) ListPortfolioExecutionProposalDecisions(
	ownerIdentity string,
	itemID uuid.UUID,
	limit int,
) ([]models.PursuitPortfolioExecutionProposalDecision, error) {
	if r == nil || r.DB == nil {
		return nil, fmt.Errorf("pursuit portfolio execution proposal decision repository is unavailable")
	}
	records := []models.PursuitPortfolioExecutionProposalDecision{}
	if err := r.DB.Where("owner_identity = ? AND proposal_item_id = ?", strings.TrimSpace(ownerIdentity), itemID).
		Order("decided_at DESC, id DESC").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}
