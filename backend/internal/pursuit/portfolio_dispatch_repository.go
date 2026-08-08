package pursuit

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type pursuitPortfolioDispatchRepository interface {
	LoadPortfolioDispatchProposal(string, uuid.UUID) (*portfolioDispatchProposalEvidence, error)
	FindOrCreatePortfolioDispatchRun(*models.PursuitPortfolioDispatchRun) (*models.PursuitPortfolioDispatchRun, bool, error)
	ListPortfolioDispatchRuns(string, uuid.UUID, int) ([]models.PursuitPortfolioDispatchRun, error)
	ListPortfolioDispatchItemResults(string, uuid.UUID) ([]models.PursuitPortfolioDispatchItemResult, error)
	ListLatestPortfolioDispatchItemResults(string, uuid.UUID) ([]models.PursuitPortfolioDispatchItemResult, error)
	AppendPortfolioDispatchItemResult(*models.PursuitPortfolioDispatchItemResult) (*models.PursuitPortfolioDispatchItemResult, bool, error)
}

type portfolioDispatchProposalEvidence struct {
	Allocation      models.PursuitPortfolioAllocation
	AllocationItems []models.PursuitPortfolioAllocationItem
	Proposal        models.PursuitPortfolioExecutionProposal
	Items           []models.PursuitPortfolioExecutionProposalItem
}

type portfolioDispatchCoordinationEvidence struct {
	Allocation        models.PursuitPortfolioAllocation
	Proposal          models.PursuitPortfolioExecutionProposal
	Items             []models.PursuitPortfolioExecutionProposalItem
	DispatchRuns      []models.PursuitPortfolioDispatchRun
	LatestDispatch    map[uuid.UUID]models.PursuitPortfolioDispatchItemResult
	ApprovalSnapshots map[uuid.UUID]*PortfolioWorkflowEffectApprovalSnapshot
}

type pursuitPortfolioDispatchCoordinationRepository interface {
	LoadPortfolioDispatchCoordinationEvidence(
		context.Context,
		string,
		[]uuid.UUID,
		int,
	) (map[uuid.UUID]portfolioDispatchCoordinationEvidence, error)
}

func (r *GormRepository) LoadPortfolioDispatchCoordinationEvidence(
	ctx context.Context,
	ownerIdentity string,
	proposalIDs []uuid.UUID,
	runLimit int,
) (map[uuid.UUID]portfolioDispatchCoordinationEvidence, error) {
	if r == nil || r.DB == nil {
		return nil, fmt.Errorf("pursuit portfolio coordination repository is unavailable")
	}
	if ctx == nil {
		return nil, fmt.Errorf("context is required")
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	if ownerIdentity == "" || len(proposalIDs) == 0 || len(proposalIDs) > PortfolioDispatchMaxItems || runLimit < 1 || runLimit > 100 {
		return nil, fmt.Errorf("valid bounded portfolio coordination scope is required")
	}
	db := r.DB.WithContext(ctx)
	proposals := []models.PursuitPortfolioExecutionProposal{}
	if err := db.Where("owner_identity = ? AND id IN ?", ownerIdentity, proposalIDs).
		Order("prepared_at DESC, id DESC").Find(&proposals).Error; err != nil {
		return nil, err
	}
	result := make(map[uuid.UUID]portfolioDispatchCoordinationEvidence, len(proposals))
	if len(proposals) == 0 {
		return result, nil
	}
	foundProposalIDs := make([]uuid.UUID, 0, len(proposals))
	allocationIDs := make([]uuid.UUID, 0, len(proposals))
	for _, proposal := range proposals {
		foundProposalIDs = append(foundProposalIDs, proposal.ID)
		allocationIDs = append(allocationIDs, proposal.AllocationID)
		result[proposal.ID] = portfolioDispatchCoordinationEvidence{
			Proposal:          proposal,
			LatestDispatch:    make(map[uuid.UUID]models.PursuitPortfolioDispatchItemResult),
			ApprovalSnapshots: make(map[uuid.UUID]*PortfolioWorkflowEffectApprovalSnapshot),
		}
	}
	allocations := []models.PursuitPortfolioAllocation{}
	if err := db.Where("owner_identity = ? AND id IN ?", ownerIdentity, allocationIDs).Find(&allocations).Error; err != nil {
		return nil, err
	}
	parentAllocationByID := make(map[uuid.UUID]models.PursuitPortfolioAllocation, len(allocations))
	for _, allocation := range allocations {
		parentAllocationByID[allocation.ID] = allocation
	}
	for _, proposal := range proposals {
		allocation, exists := parentAllocationByID[proposal.AllocationID]
		if !exists || allocation.RecordDigest != proposal.AllocationRecordDigest {
			return nil, fmt.Errorf("portfolio coordination parent allocation evidence is unavailable or changed")
		}
		evidence := result[proposal.ID]
		evidence.Allocation = allocation
		result[proposal.ID] = evidence
	}

	items := []models.PursuitPortfolioExecutionProposalItem{}
	if err := db.Where("owner_identity = ? AND proposal_id IN ?", ownerIdentity, foundProposalIDs).
		Order("proposal_id ASC, prepared_at ASC, id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	if len(items) > PortfolioCoordinationMaxItems {
		return nil, fmt.Errorf("portfolio coordination exceeds the %d-item read limit", PortfolioCoordinationMaxItems)
	}
	itemIDs := make([]uuid.UUID, 0, len(items))
	allocationItemIDs := make([]uuid.UUID, 0, len(items))
	pursuitIDs := make([]uuid.UUID, 0, len(items))
	reservationIDs := make([]uuid.UUID, 0, len(items))
	for _, item := range items {
		evidence, exists := result[item.ProposalID]
		if !exists {
			return nil, fmt.Errorf("portfolio coordination item crossed its proposal boundary")
		}
		evidence.Items = append(evidence.Items, item)
		result[item.ProposalID] = evidence
		itemIDs = append(itemIDs, item.ID)
		allocationItemIDs = append(allocationItemIDs, item.AllocationItemID)
		pursuitIDs = append(pursuitIDs, item.PursuitID)
		reservationIDs = append(reservationIDs, item.ReservationID)
	}
	if len(items) == 0 {
		return result, nil
	}

	runs := []models.PursuitPortfolioDispatchRun{}
	if err := db.Raw(`
		SELECT * FROM (
			SELECT pursuit_portfolio_dispatch_runs.*,
				ROW_NUMBER() OVER (PARTITION BY proposal_id ORDER BY requested_at DESC, id DESC) AS coordination_row
			FROM pursuit_portfolio_dispatch_runs
			WHERE owner_identity = ? AND proposal_id IN ?
		) ranked
		WHERE coordination_row <= ?
		ORDER BY proposal_id, requested_at DESC, id DESC
	`, ownerIdentity, foundProposalIDs, runLimit).Scan(&runs).Error; err != nil {
		return nil, err
	}
	for _, run := range runs {
		evidence, exists := result[run.ProposalID]
		if !exists {
			return nil, fmt.Errorf("portfolio dispatch history crossed its proposal boundary")
		}
		evidence.DispatchRuns = append(evidence.DispatchRuns, run)
		result[run.ProposalID] = evidence
	}

	latestDispatch := []models.PursuitPortfolioDispatchItemResult{}
	if err := db.Raw(`
		SELECT DISTINCT ON (proposal_item_id) *
		FROM pursuit_portfolio_dispatch_item_results
		WHERE owner_identity = ? AND proposal_id IN ?
		ORDER BY proposal_item_id, attempted_at DESC, attempt_number DESC, id DESC
	`, ownerIdentity, foundProposalIDs).Scan(&latestDispatch).Error; err != nil {
		return nil, err
	}
	for _, record := range latestDispatch {
		evidence, exists := result[record.ProposalID]
		if !exists {
			return nil, fmt.Errorf("portfolio dispatch result crossed its proposal boundary")
		}
		evidence.LatestDispatch[record.ProposalItemID] = record
		result[record.ProposalID] = evidence
	}

	latestDecisions := []models.PursuitPortfolioExecutionProposalDecision{}
	if err := db.Raw(`
		SELECT DISTINCT ON (proposal_item_id) *
		FROM pursuit_portfolio_execution_proposal_decisions
		WHERE owner_identity = ? AND proposal_item_id IN ?
		ORDER BY proposal_item_id, decided_at DESC, id DESC
	`, ownerIdentity, itemIDs).Scan(&latestDecisions).Error; err != nil {
		return nil, err
	}
	decisionsByItem := make(map[uuid.UUID]models.PursuitPortfolioExecutionProposalDecision, len(latestDecisions))
	for _, decision := range latestDecisions {
		decisionsByItem[decision.ProposalItemID] = decision
	}

	allocationItems := []models.PursuitPortfolioAllocationItem{}
	if err := db.Where("owner_identity = ? AND id IN ?", ownerIdentity, allocationItemIDs).Find(&allocationItems).Error; err != nil {
		return nil, err
	}
	allocationByID := make(map[uuid.UUID]models.PursuitPortfolioAllocationItem, len(allocationItems))
	for _, allocationItem := range allocationItems {
		allocationByID[allocationItem.ID] = allocationItem
	}
	pursuits := []models.Pursuit{}
	if err := db.Where("owner_identity = ? AND id IN ?", ownerIdentity, pursuitIDs).Find(&pursuits).Error; err != nil {
		return nil, err
	}
	pursuitByID := make(map[uuid.UUID]models.Pursuit, len(pursuits))
	for _, record := range pursuits {
		pursuitByID[record.ID] = record
	}
	settlements := []models.PursuitResourceReservationSettlement{}
	if err := db.Where("owner_identity = ? AND reservation_id IN ?", ownerIdentity, reservationIDs).Find(&settlements).Error; err != nil {
		return nil, err
	}
	settled := make(map[uuid.UUID]struct{}, len(settlements))
	for _, settlement := range settlements {
		settled[settlement.ReservationID] = struct{}{}
	}

	for _, item := range items {
		evidence := result[item.ProposalID]
		allocationItem, allocationExists := allocationByID[item.AllocationItemID]
		pursuitRecord, pursuitExists := pursuitByID[item.PursuitID]
		if !allocationExists || !pursuitExists || allocationItem.PursuitID != item.PursuitID ||
			allocationItem.ReservationID != item.ReservationID || allocationItem.RecordDigest != item.AllocationItemDigest {
			return nil, fmt.Errorf("portfolio coordination source evidence is incomplete or changed")
		}
		snapshot := &PortfolioWorkflowEffectApprovalSnapshot{
			Allocation: evidence.Allocation, Proposal: evidence.Proposal, Item: item, AllocationItem: allocationItem,
			Pursuit: pursuitRecord,
		}
		if _, exists := settled[item.ReservationID]; exists {
			snapshot.Settled = true
		}
		if decision, exists := decisionsByItem[item.ID]; exists {
			decisionCopy := decision
			snapshot.Decision = decisionCopy
			snapshot.LatestDecision = &decisionCopy
		}
		evidence.ApprovalSnapshots[item.ID] = snapshot
		result[item.ProposalID] = evidence
	}
	return result, nil
}

func (r *GormRepository) LoadPortfolioDispatchProposal(
	ownerIdentity string,
	proposalID uuid.UUID,
) (*portfolioDispatchProposalEvidence, error) {
	if r == nil || r.DB == nil {
		return nil, fmt.Errorf("pursuit portfolio dispatch repository is unavailable")
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	if ownerIdentity == "" || proposalID == uuid.Nil {
		return nil, fmt.Errorf("valid portfolio dispatch owner and proposal id are required")
	}
	var proposal models.PursuitPortfolioExecutionProposal
	if err := r.DB.Where("id = ? AND owner_identity = ?", proposalID, ownerIdentity).First(&proposal).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	var allocation models.PursuitPortfolioAllocation
	if err := r.DB.Where(
		"id = ? AND owner_identity = ? AND record_digest = ?",
		proposal.AllocationID, ownerIdentity, proposal.AllocationRecordDigest,
	).First(&allocation).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("portfolio dispatch parent allocation evidence is unavailable or changed")
		}
		return nil, err
	}
	items := []models.PursuitPortfolioExecutionProposalItem{}
	if err := r.DB.Where("proposal_id = ? AND owner_identity = ?", proposalID, ownerIdentity).
		Order("prepared_at ASC, id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	allocationItems := []models.PursuitPortfolioAllocationItem{}
	if err := r.DB.Where("allocation_id = ? AND owner_identity = ?", allocation.ID, ownerIdentity).
		Order("created_at ASC, id ASC").Find(&allocationItems).Error; err != nil {
		return nil, err
	}
	return &portfolioDispatchProposalEvidence{
		Allocation: allocation, AllocationItems: allocationItems,
		Proposal: proposal, Items: items,
	}, nil
}

func (r *GormRepository) FindOrCreatePortfolioDispatchRun(
	wanted *models.PursuitPortfolioDispatchRun,
) (*models.PursuitPortfolioDispatchRun, bool, error) {
	if r == nil || r.DB == nil {
		return nil, false, fmt.Errorf("pursuit portfolio dispatch repository is unavailable")
	}
	if wanted == nil || wanted.ID == uuid.Nil || wanted.ProposalID == uuid.Nil ||
		strings.TrimSpace(wanted.OwnerIdentity) == "" || !validPortfolioRecordDigest(wanted.RecordDigest) {
		return nil, false, fmt.Errorf("valid portfolio dispatch run evidence is required")
	}
	stored := models.PursuitPortfolioDispatchRun{}
	created := false
	err := r.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "owner_identity"}, {Name: "request_digest"}},
			DoNothing: true,
		}).Create(wanted)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 {
			stored = *wanted
			created = true
			return nil
		}
		if err := tx.Where("owner_identity = ? AND request_digest = ?", wanted.OwnerIdentity, wanted.RequestDigest).
			First(&stored).Error; err != nil {
			return err
		}
		if stored.ProposalID != wanted.ProposalID ||
			stored.ProposalDigest != wanted.ProposalDigest || stored.SelectedItemsDigest != wanted.SelectedItemsDigest ||
			stored.Actor != wanted.Actor || stored.Confirmation != wanted.Confirmation ||
			!slices.Equal(stored.SelectedItemIDs, wanted.SelectedItemIDs) {
			return fmt.Errorf("portfolio dispatch request digest already exists with different immutable evidence")
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return &stored, created, nil
}

func (r *GormRepository) ListPortfolioDispatchRuns(
	ownerIdentity string,
	proposalID uuid.UUID,
	limit int,
) ([]models.PursuitPortfolioDispatchRun, error) {
	if r == nil || r.DB == nil {
		return nil, fmt.Errorf("pursuit portfolio dispatch repository is unavailable")
	}
	records := []models.PursuitPortfolioDispatchRun{}
	if err := r.DB.Where("owner_identity = ? AND proposal_id = ?", strings.TrimSpace(ownerIdentity), proposalID).
		Order("requested_at DESC, id DESC").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (r *GormRepository) ListPortfolioDispatchItemResults(
	ownerIdentity string,
	runID uuid.UUID,
) ([]models.PursuitPortfolioDispatchItemResult, error) {
	if r == nil || r.DB == nil {
		return nil, fmt.Errorf("pursuit portfolio dispatch repository is unavailable")
	}
	records := []models.PursuitPortfolioDispatchItemResult{}
	if err := r.DB.Where("owner_identity = ? AND dispatch_run_id = ?", strings.TrimSpace(ownerIdentity), runID).
		Order("proposal_item_id ASC, attempt_number DESC, attempted_at DESC, id DESC").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (r *GormRepository) ListLatestPortfolioDispatchItemResults(
	ownerIdentity string,
	proposalID uuid.UUID,
) ([]models.PursuitPortfolioDispatchItemResult, error) {
	if r == nil || r.DB == nil {
		return nil, fmt.Errorf("pursuit portfolio dispatch repository is unavailable")
	}
	records := []models.PursuitPortfolioDispatchItemResult{}
	err := r.DB.Raw(`
		SELECT DISTINCT ON (proposal_item_id) *
		FROM pursuit_portfolio_dispatch_item_results
		WHERE owner_identity = ? AND proposal_id = ?
		ORDER BY proposal_item_id, attempted_at DESC, attempt_number DESC, id DESC
	`, strings.TrimSpace(ownerIdentity), proposalID).Scan(&records).Error
	if err != nil {
		return nil, err
	}
	return records, nil
}

func (r *GormRepository) AppendPortfolioDispatchItemResult(
	wanted *models.PursuitPortfolioDispatchItemResult,
) (*models.PursuitPortfolioDispatchItemResult, bool, error) {
	if r == nil || r.DB == nil {
		return nil, false, fmt.Errorf("pursuit portfolio dispatch repository is unavailable")
	}
	if wanted == nil || wanted.ID == uuid.Nil || wanted.DispatchRunID == uuid.Nil ||
		wanted.ProposalID == uuid.Nil || wanted.ProposalItemID == uuid.Nil ||
		strings.TrimSpace(wanted.OwnerIdentity) == "" || !validPortfolioRecordDigest(wanted.ProposalItemDigest) {
		return nil, false, fmt.Errorf("valid portfolio dispatch item result evidence is required")
	}
	stored := models.PursuitPortfolioDispatchItemResult{}
	created := false
	err := r.DB.Transaction(func(tx *gorm.DB) error {
		var run models.PursuitPortfolioDispatchRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(
			"id = ? AND owner_identity = ? AND proposal_id = ?",
			wanted.DispatchRunID, wanted.OwnerIdentity, wanted.ProposalID,
		).First(&run).Error; err != nil {
			return fmt.Errorf("load immutable portfolio dispatch run: %w", err)
		}
		var latest models.PursuitPortfolioDispatchItemResult
		latestErr := tx.Where(
			"dispatch_run_id = ? AND proposal_item_id = ? AND owner_identity = ?",
			wanted.DispatchRunID, wanted.ProposalItemID, wanted.OwnerIdentity,
		).Order("attempt_number DESC, attempted_at DESC, id DESC").First(&latest).Error
		if latestErr != nil && latestErr != gorm.ErrRecordNotFound {
			return latestErr
		}
		if latestErr == nil && portfolioDispatchOutcomeTerminal(latest.Outcome) {
			stored = latest
			return nil
		}
		wanted.AttemptNumber = 1
		if latestErr == nil {
			wanted.AttemptNumber = latest.AttemptNumber + 1
		}
		if wanted.AttemptNumber > PortfolioDispatchMaxAttemptsPerItem {
			return fmt.Errorf("portfolio dispatch retry limit reached for proposal item")
		}
		wanted.RecordDigest, latestErr = digestPortfolioDispatchItemResult(wanted)
		if latestErr != nil {
			return latestErr
		}
		if err := tx.Create(wanted).Error; err != nil {
			return err
		}
		stored = *wanted
		created = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return &stored, created, nil
}
