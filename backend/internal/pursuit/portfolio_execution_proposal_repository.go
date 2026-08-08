package pursuit

import (
	"automation-hub-backend/internal/models"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	portfolioExecutionProposalActivityType   = "pursuit.portfolio_execution_proposed"
	portfolioExecutionProposalActivitySource = "pursuit_portfolio_execution_proposal"
)

func (r *GormRepository) LoadPortfolioExecutionProposalSnapshot(
	ownerIdentity string,
	allocationID uuid.UUID,
) (*portfolioExecutionProposalSnapshot, error) {
	if r == nil || r.DB == nil {
		return nil, fmt.Errorf("pursuit portfolio execution proposal repository is unavailable")
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	if ownerIdentity == "" || allocationID == uuid.Nil {
		return nil, fmt.Errorf("valid portfolio execution proposal owner and allocation id are required")
	}
	var allocation models.PursuitPortfolioAllocation
	if err := r.DB.Where("id = ? AND owner_identity = ?", allocationID, ownerIdentity).First(&allocation).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	items := []models.PursuitPortfolioAllocationItem{}
	if err := r.DB.Where("allocation_id = ? AND owner_identity = ?", allocationID, ownerIdentity).
		Order("scheduled_start ASC, pursuit_id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return &portfolioExecutionProposalSnapshot{
			Allocation: &allocation, AllocationItems: items,
			Pursuits: map[uuid.UUID]models.Pursuit{}, SettledReservationIDs: map[uuid.UUID]struct{}{},
		}, nil
	}
	pursuitIDs := make([]uuid.UUID, 0, len(items))
	reservationIDs := make([]uuid.UUID, 0, len(items))
	for _, item := range items {
		pursuitIDs = append(pursuitIDs, item.PursuitID)
		reservationIDs = append(reservationIDs, item.ReservationID)
	}
	pursuits := []models.Pursuit{}
	if err := r.DB.Where("owner_identity = ? AND id IN ?", ownerIdentity, pursuitIDs).Find(&pursuits).Error; err != nil {
		return nil, err
	}
	pursuitsByID := make(map[uuid.UUID]models.Pursuit, len(pursuits))
	for _, record := range pursuits {
		pursuitsByID[record.ID] = record
	}
	settlements := []models.PursuitResourceReservationSettlement{}
	if err := r.DB.Where("owner_identity = ? AND reservation_id IN ?", ownerIdentity, reservationIDs).
		Find(&settlements).Error; err != nil {
		return nil, err
	}
	settled := make(map[uuid.UUID]struct{}, len(settlements))
	for _, settlement := range settlements {
		settled[settlement.ReservationID] = struct{}{}
	}
	return &portfolioExecutionProposalSnapshot{
		Allocation: &allocation, AllocationItems: items,
		Pursuits: pursuitsByID, SettledReservationIDs: settled,
	}, nil
}

func (r *GormRepository) FindPortfolioExecutionProposalForSnapshot(
	ownerIdentity string,
	allocationID uuid.UUID,
	snapshotDigest string,
) (*models.PursuitPortfolioExecutionProposal, []models.PursuitPortfolioExecutionProposalItem, error) {
	if r == nil || r.DB == nil {
		return nil, nil, fmt.Errorf("pursuit portfolio execution proposal repository is unavailable")
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	if ownerIdentity == "" || allocationID == uuid.Nil || !validPortfolioRecordDigest(snapshotDigest) {
		return nil, nil, fmt.Errorf("valid portfolio execution proposal replay identity is required")
	}
	var proposal models.PursuitPortfolioExecutionProposal
	err := r.DB.Where(
		"owner_identity = ? AND allocation_id = ? AND snapshot_digest = ?",
		ownerIdentity, allocationID, snapshotDigest,
	).First(&proposal).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	items := []models.PursuitPortfolioExecutionProposalItem{}
	if err := r.DB.Where("owner_identity = ? AND proposal_id = ?", ownerIdentity, proposal.ID).
		Order("allocation_item_id ASC").Find(&items).Error; err != nil {
		return nil, nil, err
	}
	return &proposal, items, nil
}

func (r *GormRepository) ListLatestPortfolioExecutionProposals(
	ownerIdentity string,
	allocationIDs []uuid.UUID,
) ([]models.PursuitPortfolioExecutionProposal, map[uuid.UUID][]models.PursuitPortfolioExecutionProposalItem, error) {
	if r == nil || r.DB == nil {
		return nil, nil, fmt.Errorf("pursuit portfolio execution proposal repository is unavailable")
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	if ownerIdentity == "" || len(allocationIDs) == 0 || len(allocationIDs) > 20 {
		return nil, nil, fmt.Errorf("valid portfolio execution proposal history scope is required")
	}
	proposals := []models.PursuitPortfolioExecutionProposal{}
	if err := r.DB.Raw(`
		SELECT DISTINCT ON (allocation_id) *
		FROM pursuit_portfolio_execution_proposals
		WHERE owner_identity = ? AND allocation_id IN ?
		ORDER BY allocation_id, prepared_at DESC, id DESC
	`, ownerIdentity, allocationIDs).Scan(&proposals).Error; err != nil {
		return nil, nil, err
	}
	itemsByProposal := make(map[uuid.UUID][]models.PursuitPortfolioExecutionProposalItem, len(proposals))
	if len(proposals) == 0 {
		return proposals, itemsByProposal, nil
	}
	proposalIDs := make([]uuid.UUID, 0, len(proposals))
	for _, proposal := range proposals {
		proposalIDs = append(proposalIDs, proposal.ID)
	}
	items := []models.PursuitPortfolioExecutionProposalItem{}
	if err := r.DB.Where("owner_identity = ? AND proposal_id IN ?", ownerIdentity, proposalIDs).
		Order("proposal_id ASC, allocation_item_id ASC").Find(&items).Error; err != nil {
		return nil, nil, err
	}
	for _, item := range items {
		itemsByProposal[item.ProposalID] = append(itemsByProposal[item.ProposalID], item)
	}
	return proposals, itemsByProposal, nil
}

func (r *GormRepository) SavePortfolioExecutionProposal(
	proposal *models.PursuitPortfolioExecutionProposal,
	items []models.PursuitPortfolioExecutionProposalItem,
	activities []models.PursuitActivity,
) (*models.PursuitPortfolioExecutionProposal, []models.PursuitPortfolioExecutionProposalItem, bool, error) {
	if r == nil || r.DB == nil {
		return nil, nil, false, fmt.Errorf("pursuit portfolio execution proposal repository is unavailable")
	}
	if err := validatePortfolioExecutionProposalAggregate(proposal, items, activities); err != nil {
		return nil, nil, false, err
	}
	var stored models.PursuitPortfolioExecutionProposal
	storedItems := []models.PursuitPortfolioExecutionProposalItem{}
	created := false
	err := r.DB.Transaction(func(tx *gorm.DB) error {
		if err := verifyPortfolioExecutionProposalBindings(tx, proposal, items); err != nil {
			return err
		}
		result := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "owner_identity"}, {Name: "allocation_id"}, {Name: "snapshot_digest"}},
			DoNothing: true,
		}).Create(proposal)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return loadAndVerifyPortfolioExecutionProposalReplay(tx, proposal, items, activities, &stored, &storedItems)
		}
		created = true
		stored = *proposal
		for index := range items {
			if err := tx.Create(&items[index]).Error; err != nil {
				return fmt.Errorf("create portfolio execution proposal item: %w", err)
			}
			storedItems = append(storedItems, items[index])
		}
		for index := range activities {
			if err := appendResourceActivities(tx, activities[index].PursuitID, []models.PursuitActivity{activities[index]}); err != nil {
				return fmt.Errorf("create portfolio execution proposal activity: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, false, err
	}
	sortPortfolioExecutionProposalItems(storedItems)
	return &stored, storedItems, created, nil
}

func validatePortfolioExecutionProposalAggregate(
	proposal *models.PursuitPortfolioExecutionProposal,
	items []models.PursuitPortfolioExecutionProposalItem,
	activities []models.PursuitActivity,
) error {
	if proposal == nil || proposal.ID == uuid.Nil || proposal.AllocationID == uuid.Nil ||
		strings.TrimSpace(proposal.OwnerIdentity) == "" || strings.TrimSpace(proposal.OwnerIdentity) != proposal.OwnerIdentity ||
		proposal.Actor != proposal.OwnerIdentity || proposal.Confirmation != PortfolioExecutionProposalConfirmation ||
		proposal.Authority != PortfolioExecutionProposalAuthority || proposal.PreparedAt.IsZero() ||
		!validPortfolioRecordDigest(proposal.AllocationRecordDigest) || !validPortfolioRecordDigest(proposal.SnapshotDigest) ||
		!validPortfolioRecordDigest(proposal.RecordDigest) {
		return fmt.Errorf("portfolio execution proposal parent evidence is invalid")
	}
	if len(items) == 0 || len(items) > 500 || len(activities) != len(items) {
		return fmt.Errorf("portfolio execution proposal requires one audit activity per item")
	}
	if err := validatePortfolioExecutionProposalEvidence(proposal.OwnerIdentity, proposal, items); err != nil {
		return err
	}
	activitiesByPursuit := make(map[uuid.UUID]models.PursuitActivity, len(activities))
	for _, activity := range activities {
		if activity.ID == uuid.Nil || activity.PursuitID == uuid.Nil || activity.CreatedAt.IsZero() ||
			activity.EventType != portfolioExecutionProposalActivityType || activity.Actor != proposal.Actor ||
			activity.SourceType != portfolioExecutionProposalActivitySource || activity.SourceID != proposal.ID.String() {
			return fmt.Errorf("portfolio execution proposal audit activity is invalid")
		}
		if _, duplicate := activitiesByPursuit[activity.PursuitID]; duplicate {
			return fmt.Errorf("portfolio execution proposal contains duplicate pursuit activity")
		}
		activitiesByPursuit[activity.PursuitID] = activity
	}
	for _, item := range items {
		if _, ok := activitiesByPursuit[item.PursuitID]; !ok {
			return fmt.Errorf("portfolio execution proposal item is missing its audit activity")
		}
	}
	return nil
}

func verifyPortfolioExecutionProposalBindings(
	tx *gorm.DB,
	proposal *models.PursuitPortfolioExecutionProposal,
	items []models.PursuitPortfolioExecutionProposalItem,
) error {
	var allocationCount int64
	if err := tx.Model(&models.PursuitPortfolioAllocation{}).Where(
		"id = ? AND owner_identity = ? AND record_digest = ?",
		proposal.AllocationID, proposal.OwnerIdentity, proposal.AllocationRecordDigest,
	).Count(&allocationCount).Error; err != nil {
		return err
	}
	if allocationCount != 1 {
		return fmt.Errorf("portfolio execution proposal allocation evidence changed or is unavailable")
	}
	for _, item := range items {
		var allocationItem models.PursuitPortfolioAllocationItem
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where(
			"id = ? AND allocation_id = ? AND pursuit_id = ? AND reservation_id = ? AND owner_identity = ? AND record_digest = ?",
			item.AllocationItemID, proposal.AllocationID, item.PursuitID, item.ReservationID,
			proposal.OwnerIdentity, item.AllocationItemDigest,
		).First(&allocationItem).Error; err != nil {
			return fmt.Errorf("portfolio execution proposal item evidence changed or is unavailable")
		}
		var pursuit models.Pursuit
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where(
			"id = ? AND owner_identity = ?", item.PursuitID, proposal.OwnerIdentity,
		).First(&pursuit).Error; err != nil {
			return fmt.Errorf("portfolio execution proposal pursuit state changed or is unavailable")
		}
		var settlementCount int64
		if err := tx.Model(&models.PursuitResourceReservationSettlement{}).Where(
			"reservation_id = ? AND owner_identity = ?", item.ReservationID, proposal.OwnerIdentity,
		).Count(&settlementCount).Error; err != nil {
			return err
		}
		settled := map[uuid.UUID]struct{}{}
		if settlementCount > 0 {
			settled[item.ReservationID] = struct{}{}
		}
		currentStateDigest, err := digestPortfolioExecutionState(pursuit, allocationItem, settled)
		if err != nil || currentStateDigest != item.StateDigest {
			return fmt.Errorf("portfolio execution proposal pursuit state changed before persistence")
		}
	}
	return nil
}

func loadAndVerifyPortfolioExecutionProposalReplay(
	tx *gorm.DB,
	wanted *models.PursuitPortfolioExecutionProposal,
	wantedItems []models.PursuitPortfolioExecutionProposalItem,
	wantedActivities []models.PursuitActivity,
	stored *models.PursuitPortfolioExecutionProposal,
	storedItems *[]models.PursuitPortfolioExecutionProposalItem,
) error {
	if err := tx.Where(
		"owner_identity = ? AND allocation_id = ? AND snapshot_digest = ?",
		wanted.OwnerIdentity, wanted.AllocationID, wanted.SnapshotDigest,
	).First(stored).Error; err != nil {
		return err
	}
	if stored.RecordDigest != wanted.RecordDigest || stored.AllocationRecordDigest != wanted.AllocationRecordDigest || stored.Authority != PortfolioExecutionProposalAuthority {
		return fmt.Errorf("portfolio execution proposal snapshot already exists with different parent evidence")
	}
	if err := tx.Where("owner_identity = ? AND proposal_id = ?", wanted.OwnerIdentity, stored.ID).
		Order("allocation_item_id ASC").Find(storedItems).Error; err != nil {
		return err
	}
	if len(*storedItems) != len(wantedItems) {
		return fmt.Errorf("stored portfolio execution proposal does not match the replayed item set")
	}
	wantedByAllocationItem := make(map[uuid.UUID]models.PursuitPortfolioExecutionProposalItem, len(wantedItems))
	for _, item := range wantedItems {
		wantedByAllocationItem[item.AllocationItemID] = item
	}
	for _, persisted := range *storedItems {
		expected, ok := wantedByAllocationItem[persisted.AllocationItemID]
		if !ok || persisted.RecordDigest != expected.RecordDigest || persisted.StateDigest != expected.StateDigest {
			return fmt.Errorf("portfolio execution proposal snapshot already exists with different item evidence")
		}
	}
	var activityCount int64
	if err := tx.Table("pursuit_activities AS a").Joins("JOIN pursuits AS p ON p.id = a.pursuit_id").Where(
		"p.owner_identity = ? AND a.source_type = ? AND a.source_id = ? AND a.event_type = ?",
		wanted.OwnerIdentity, portfolioExecutionProposalActivitySource, stored.ID.String(), portfolioExecutionProposalActivityType,
	).Count(&activityCount).Error; err != nil {
		return fmt.Errorf("verify portfolio execution proposal activities: %w", err)
	}
	if activityCount != int64(len(wantedActivities)) {
		return fmt.Errorf("stored portfolio execution proposal audit activity is incomplete")
	}
	return nil
}

func sortPortfolioExecutionProposalItems(items []models.PursuitPortfolioExecutionProposalItem) {
	sort.Slice(items, func(left, right int) bool {
		return items[left].AllocationItemID.String() < items[right].AllocationItemID.String()
	})
}
