package pursuit

import (
	"automation-hub-backend/internal/models"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	PortfolioExecutionProposalConfirmation = "PREPARE EXECUTION PROPOSALS"
	PortfolioExecutionProposalAuthority    = "proposal_only"

	PortfolioExecutionProposalPrepared              = "prepared"
	PortfolioExecutionProposalPreparedNeedsApproval = "prepared_needs_approval"
	PortfolioExecutionProposalPreparedBlocked       = "prepared_blocked"

	PortfolioExecutionProposalItemProposed      = "proposed"
	PortfolioExecutionProposalItemNeedsApproval = "needs_approval"
	PortfolioExecutionProposalItemBlocked       = "blocked"

	PortfolioExecutionProposalFreshnessPrepared  = "prepared_snapshot"
	PortfolioExecutionProposalFreshnessRecovered = "recovered_snapshot"
)

type PortfolioExecutionProposalRequest struct {
	ExpectedAllocationDigest string `json:"expectedAllocationDigest"`
	Confirmation             string `json:"confirmation"`
}

type PortfolioExecutionProposalResult struct {
	Proposal   *models.PursuitPortfolioExecutionProposal      `json:"proposal"`
	Items      []models.PursuitPortfolioExecutionProposalItem `json:"items"`
	Replayed   bool                                           `json:"replayed"`
	Authority  string                                         `json:"authority"`
	CanExecute bool                                           `json:"canExecute"`
	Freshness  PortfolioExecutionProposalFreshness            `json:"freshness"`
}

type PortfolioExecutionProposalFreshness struct {
	Status               string    `json:"status"`
	RevalidationRequired bool      `json:"revalidationRequired"`
	CheckedAt            time.Time `json:"checkedAt"`
	Reason               string    `json:"reason"`
}

type portfolioExecutionProposalSnapshot struct {
	Allocation            *models.PursuitPortfolioAllocation
	AllocationItems       []models.PursuitPortfolioAllocationItem
	Pursuits              map[uuid.UUID]models.Pursuit
	SettledReservationIDs map[uuid.UUID]struct{}
}

type pursuitPortfolioExecutionProposalRepository interface {
	LoadPortfolioExecutionProposalSnapshot(ownerIdentity string, allocationID uuid.UUID) (*portfolioExecutionProposalSnapshot, error)
	FindPortfolioExecutionProposalForSnapshot(ownerIdentity string, allocationID uuid.UUID, snapshotDigest string) (*models.PursuitPortfolioExecutionProposal, []models.PursuitPortfolioExecutionProposalItem, error)
	SavePortfolioExecutionProposal(
		proposal *models.PursuitPortfolioExecutionProposal,
		items []models.PursuitPortfolioExecutionProposalItem,
		activities []models.PursuitActivity,
	) (*models.PursuitPortfolioExecutionProposal, []models.PursuitPortfolioExecutionProposalItem, bool, error)
}

type pursuitPortfolioExecutionProposalHistoryRepository interface {
	ListLatestPortfolioExecutionProposals(
		ownerIdentity string,
		allocationIDs []uuid.UUID,
	) ([]models.PursuitPortfolioExecutionProposal, map[uuid.UUID][]models.PursuitPortfolioExecutionProposalItem, error)
}

// PortfolioExecutionProposalHistoryForOwner restores the newest immutable
// proposal for each explicitly requested allocation. It is read-only and does
// not refresh snapshots, approve work, or grant execution authority.
func (s *service) PortfolioExecutionProposalHistoryForOwner(
	ownerIdentity string,
	allocationIDs []uuid.UUID,
) ([]PortfolioExecutionProposalResult, error) {
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	if ownerIdentity == "" {
		return nil, fmt.Errorf("an authenticated owner is required to read portfolio execution proposals")
	}
	if len(allocationIDs) == 0 || len(allocationIDs) > 20 {
		return nil, fmt.Errorf("between 1 and 20 portfolio allocation ids are required")
	}
	requested := make(map[uuid.UUID]struct{}, len(allocationIDs))
	for _, allocationID := range allocationIDs {
		if allocationID == uuid.Nil {
			return nil, fmt.Errorf("portfolio allocation ids must be valid")
		}
		if _, duplicate := requested[allocationID]; duplicate {
			return nil, fmt.Errorf("portfolio allocation ids must be unique")
		}
		requested[allocationID] = struct{}{}
	}
	repository, ok := s.repo.(pursuitPortfolioExecutionProposalHistoryRepository)
	if !ok {
		return nil, fmt.Errorf("durable portfolio execution proposal history is unavailable")
	}
	proposals, itemsByProposal, err := repository.ListLatestPortfolioExecutionProposals(ownerIdentity, allocationIDs)
	if err != nil {
		return nil, fmt.Errorf("list portfolio execution proposals: %w", err)
	}
	checkedAt := time.Now().UTC()
	byAllocation := make(map[uuid.UUID]PortfolioExecutionProposalResult, len(proposals))
	for index := range proposals {
		proposal := proposals[index]
		if _, expected := requested[proposal.AllocationID]; !expected {
			return nil, fmt.Errorf("portfolio execution proposal history crossed its requested allocation boundary")
		}
		items := append([]models.PursuitPortfolioExecutionProposalItem(nil), itemsByProposal[proposal.ID]...)
		if err := validatePortfolioExecutionProposalEvidence(ownerIdentity, &proposal, items); err != nil {
			return nil, fmt.Errorf("validate portfolio execution proposal history: %w", err)
		}
		if _, duplicate := byAllocation[proposal.AllocationID]; duplicate {
			return nil, fmt.Errorf("portfolio execution proposal history returned duplicate allocations")
		}
		proposalCopy := proposal
		byAllocation[proposal.AllocationID] = PortfolioExecutionProposalResult{
			Proposal: &proposalCopy, Items: items, Replayed: true,
			Authority: PortfolioExecutionProposalAuthority, CanExecute: false,
			Freshness: recoveredPortfolioExecutionProposalFreshness(checkedAt),
		}
	}
	results := make([]PortfolioExecutionProposalResult, 0, len(byAllocation))
	for _, allocationID := range allocationIDs {
		if result, exists := byAllocation[allocationID]; exists {
			results = append(results, result)
		}
	}
	return results, nil
}

// PreparePortfolioExecutionProposalsForOwner translates an immutable accepted
// allocation into immutable, per-item proposals. This method deliberately does
// not consume approvals, enqueue jobs, mutate pursuits, settle reservations,
// call a runtime, or grant execution authority.
func (s *service) PreparePortfolioExecutionProposalsForOwner(
	ownerIdentity, actor string,
	allocationID uuid.UUID,
	request PortfolioExecutionProposalRequest,
) (*PortfolioExecutionProposalResult, error) {
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	actor = strings.TrimSpace(actor)
	request.ExpectedAllocationDigest = strings.TrimSpace(request.ExpectedAllocationDigest)
	request.Confirmation = strings.TrimSpace(request.Confirmation)
	if ownerIdentity == "" || actor == "" || actor != ownerIdentity {
		return nil, fmt.Errorf("the authenticated owner must prepare portfolio execution proposals")
	}
	if allocationID == uuid.Nil {
		return nil, fmt.Errorf("a valid portfolio allocation id is required")
	}
	if request.Confirmation != PortfolioExecutionProposalConfirmation {
		return nil, fmt.Errorf("exact portfolio execution proposal confirmation is required")
	}
	if !validPortfolioRecordDigest(request.ExpectedAllocationDigest) {
		return nil, fmt.Errorf("a valid expected allocation digest is required")
	}
	repository, ok := s.repo.(pursuitPortfolioExecutionProposalRepository)
	if !ok {
		return nil, fmt.Errorf("durable portfolio execution proposal storage is unavailable")
	}

	snapshot, err := repository.LoadPortfolioExecutionProposalSnapshot(ownerIdentity, allocationID)
	if err != nil {
		return nil, fmt.Errorf("load portfolio execution proposal state: %w", err)
	}
	if snapshot == nil || snapshot.Allocation == nil {
		return nil, fmt.Errorf("portfolio allocation is unavailable to this owner")
	}
	if err := validatePortfolioAllocationHistoryEvidence(ownerIdentity, snapshot.Allocation, snapshot.AllocationItems); err != nil {
		return nil, fmt.Errorf("validate accepted allocation evidence: %w", err)
	}
	if _, err := s.revalidatePortfolioAllocationCoordinationPlan(ownerIdentity, snapshot.Allocation, snapshot.AllocationItems); err != nil {
		return nil, fmt.Errorf("revalidate accepted coordination plan: %w", err)
	}
	if snapshot.Allocation.ID != allocationID || snapshot.Allocation.RecordDigest != request.ExpectedAllocationDigest {
		return nil, fmt.Errorf("portfolio allocation changed; inspect the current immutable allocation before continuing")
	}

	preparedAt := time.Now().UTC()
	items, snapshotDigest, proposalStatus, err := buildPortfolioExecutionProposalItems(snapshot, preparedAt)
	if err != nil {
		return nil, err
	}
	if existing, existingItems, findErr := repository.FindPortfolioExecutionProposalForSnapshot(ownerIdentity, allocationID, snapshotDigest); findErr != nil {
		return nil, fmt.Errorf("read portfolio execution proposal replay state: %w", findErr)
	} else if existing != nil {
		if err := validatePortfolioExecutionProposalEvidence(ownerIdentity, existing, existingItems); err != nil {
			return nil, err
		}
		return &PortfolioExecutionProposalResult{
			Proposal: existing, Items: existingItems, Replayed: true,
			Authority: PortfolioExecutionProposalAuthority, CanExecute: false,
			Freshness: recoveredPortfolioExecutionProposalFreshness(time.Now().UTC()),
		}, nil
	}

	proposalID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("create time-ordered portfolio execution proposal id: %w", err)
	}
	proposal := &models.PursuitPortfolioExecutionProposal{
		ID: proposalID, AllocationID: allocationID, OwnerIdentity: ownerIdentity,
		AllocationRecordDigest: snapshot.Allocation.RecordDigest, SnapshotDigest: snapshotDigest,
		Status: proposalStatus, Actor: actor, Confirmation: PortfolioExecutionProposalConfirmation,
		Authority: PortfolioExecutionProposalAuthority, PreparedAt: preparedAt,
	}
	proposal.RecordDigest, err = digestPortfolioExecutionProposal(proposal)
	if err != nil {
		return nil, err
	}
	activities := make([]models.PursuitActivity, 0, len(items))
	for index := range items {
		items[index].ID = uuid.New()
		items[index].ProposalID = proposalID
		items[index].OwnerIdentity = ownerIdentity
		items[index].PreparedAt = preparedAt
		items[index].RecordDigest, err = digestPortfolioExecutionProposalItem(snapshotDigest, items[index])
		if err != nil {
			return nil, err
		}
		activities = append(activities, newPursuitResourceActivity(
			items[index].PursuitID,
			"pursuit.portfolio_execution_proposed",
			fmt.Sprintf("Prepared %s portfolio execution proposal; approval and execution remain separate.", items[index].Status),
			actor,
			"pursuit_portfolio_execution_proposal",
			proposalID.String(),
			"hai://pursuits/"+items[index].PursuitID.String()+"/portfolio-execution-proposals/"+proposalID.String(),
			preparedAt,
		))
	}
	stored, storedItems, created, err := repository.SavePortfolioExecutionProposal(proposal, items, activities)
	if err != nil {
		return nil, fmt.Errorf("save portfolio execution proposal: %w", err)
	}
	return &PortfolioExecutionProposalResult{
		Proposal: stored, Items: storedItems, Replayed: !created,
		Authority: PortfolioExecutionProposalAuthority, CanExecute: false,
		Freshness: portfolioExecutionProposalFreshness(created, time.Now().UTC()),
	}, nil
}

func portfolioExecutionProposalFreshness(created bool, checkedAt time.Time) PortfolioExecutionProposalFreshness {
	if !created {
		return recoveredPortfolioExecutionProposalFreshness(checkedAt)
	}
	return PortfolioExecutionProposalFreshness{
		Status:               PortfolioExecutionProposalFreshnessPrepared,
		RevalidationRequired: true,
		CheckedAt:            checkedAt.UTC(),
		Reason:               "Proposal evidence reflects its newly prepared immutable snapshot; current approvals and dispatch eligibility still require separate revalidation.",
	}
}

func recoveredPortfolioExecutionProposalFreshness(checkedAt time.Time) PortfolioExecutionProposalFreshness {
	return PortfolioExecutionProposalFreshness{
		Status:               PortfolioExecutionProposalFreshnessRecovered,
		RevalidationRequired: true,
		CheckedAt:            checkedAt.UTC(),
		Reason:               "Recovered immutable evidence does not prove current approval, reservation, workflow, or runtime eligibility; refresh eligibility before any governed dispatch review.",
	}
}

func buildPortfolioExecutionProposalItems(
	snapshot *portfolioExecutionProposalSnapshot,
	preparedAt time.Time,
) ([]models.PursuitPortfolioExecutionProposalItem, string, string, error) {
	if snapshot == nil || snapshot.Allocation == nil || len(snapshot.AllocationItems) == 0 {
		return nil, "", "", fmt.Errorf("accepted allocation contains no proposal candidates")
	}
	items := make([]models.PursuitPortfolioExecutionProposalItem, 0, len(snapshot.AllocationItems))
	stateDigests := make([]string, 0, len(snapshot.AllocationItems))
	anyBlocked := false
	anyApproval := false
	for _, allocationItem := range snapshot.AllocationItems {
		pursuit, exists := snapshot.Pursuits[allocationItem.PursuitID]
		if !exists || pursuit.ID == uuid.Nil || pursuit.OwnerIdentity != snapshot.Allocation.OwnerIdentity {
			return nil, "", "", fmt.Errorf("portfolio execution proposal crossed its pursuit owner boundary")
		}
		actionSummary := strings.Join(strings.Fields(pursuit.NextRecommendedAction), " ")
		approvalReasons := append([]string(nil), allocationItem.ApprovalReasons...)
		blockedReasons := portfolioExecutionBlockedReasons(pursuit, allocationItem.ReservationID, snapshot.SettledReservationIDs, actionSummary)
		approvalReasons = append(approvalReasons, portfolioExecutionApprovalReasons(pursuit, allocationItem)...)
		approvalReasons = uniqueSortedPortfolioReasons(approvalReasons)
		if actionSummary == "" {
			actionSummary = "Define and verify the next action before execution."
		}
		if len(actionSummary) > 4000 {
			return nil, "", "", fmt.Errorf("portfolio execution proposal action summary is too long")
		}
		stateDigest, err := digestPortfolioExecutionState(pursuit, allocationItem, snapshot.SettledReservationIDs)
		if err != nil {
			return nil, "", "", err
		}
		status := PortfolioExecutionProposalItemProposed
		if len(blockedReasons) > 0 {
			status = PortfolioExecutionProposalItemBlocked
			anyBlocked = true
		} else if len(approvalReasons) > 0 {
			status = PortfolioExecutionProposalItemNeedsApproval
			anyApproval = true
		}
		items = append(items, models.PursuitPortfolioExecutionProposalItem{
			AllocationItemID: allocationItem.ID, PursuitID: allocationItem.PursuitID,
			ReservationID: allocationItem.ReservationID, ActionSummary: actionSummary,
			PursuitStatus: strings.ToLower(strings.TrimSpace(pursuit.Status)),
			RiskLevel:     strings.ToLower(firstNonEmpty(strings.TrimSpace(pursuit.RiskLevel), "unknown")),
			AutonomyLevel: strings.ToLower(firstNonEmpty(strings.TrimSpace(pursuit.AutonomyLevel), "unknown")),
			Status:        status, RequiresApproval: len(approvalReasons) > 0,
			ApprovalReasons: approvalReasons, BlockedReasons: blockedReasons,
			AllocationItemDigest: allocationItem.RecordDigest, StateDigest: stateDigest,
			PreparedAt: preparedAt,
		})
		stateDigests = append(stateDigests, allocationItem.ID.String()+":"+stateDigest)
	}
	sort.Slice(items, func(left, right int) bool {
		return items[left].AllocationItemID.String() < items[right].AllocationItemID.String()
	})
	sort.Strings(stateDigests)
	snapshotDigest, err := digestPortfolioPayload(struct {
		AllocationID, AllocationDigest string
		ItemStateDigests               []string
	}{snapshot.Allocation.ID.String(), snapshot.Allocation.RecordDigest, stateDigests})
	if err != nil {
		return nil, "", "", err
	}
	status := PortfolioExecutionProposalPrepared
	if anyBlocked {
		status = PortfolioExecutionProposalPreparedBlocked
	} else if anyApproval {
		status = PortfolioExecutionProposalPreparedNeedsApproval
	}
	return items, snapshotDigest, status, nil
}

func portfolioExecutionBlockedReasons(
	pursuit models.Pursuit,
	reservationID uuid.UUID,
	settled map[uuid.UUID]struct{},
	actionSummary string,
) []string {
	reasons := []string{}
	status := strings.ToLower(strings.TrimSpace(pursuit.Status))
	completion := strings.ToLower(strings.TrimSpace(pursuit.CompletionState))
	if pursuit.Archived || status == StatusArchived {
		reasons = append(reasons, "The pursuit is archived.")
	}
	if status == StatusCompleted || completion == "completed" || completion == "verified" {
		reasons = append(reasons, "The pursuit is already complete.")
	}
	if status == StatusBlocked {
		reasons = append(reasons, "The pursuit is currently blocked.")
	}
	if actionSummary == "" {
		reasons = append(reasons, "No verified next action is defined.")
	}
	if _, isSettled := settled[reservationID]; isSettled {
		reasons = append(reasons, "The accepted resource reservation is already settled.")
	}
	for _, condition := range pursuit.StopConditions {
		conditionStatus := strings.ToLower(strings.TrimSpace(condition.Status))
		if condition.ResolvedAt == nil && (condition.TriggeredAt != nil || conditionStatus == "triggered" || conditionStatus == "active" || conditionStatus == "blocked") {
			reason := strings.Join(strings.Fields(firstNonEmpty(condition.Reason, condition.Description, "A pursuit stop condition is active.")), " ")
			reasons = append(reasons, reason)
		}
	}
	for _, dependency := range pursuit.Dependencies {
		status := strings.ToLower(strings.TrimSpace(dependency.Status))
		if status == "" || status == "open" || status == "waiting" || status == "blocked" || status == "pending" {
			reason := "Dependency is unresolved: " + strings.Join(strings.Fields(firstNonEmpty(dependency.Label, dependency.Reason, dependency.ID)), " ")
			reasons = append(reasons, reason)
		}
	}
	return uniqueSortedPortfolioReasons(reasons)
}

func portfolioExecutionApprovalReasons(
	pursuit models.Pursuit,
	allocationItem models.PursuitPortfolioAllocationItem,
) []string {
	reasons := []string{}
	risk := strings.ToLower(strings.TrimSpace(pursuit.RiskLevel))
	if risk == "" || risk == "unknown" || risk == "high" || risk == "critical" {
		reasons = append(reasons, "Current pursuit risk requires explicit owner approval.")
	}
	autonomy := strings.ToLower(strings.TrimSpace(pursuit.AutonomyLevel))
	switch autonomy {
	case "autonomous_safe", "autonomous_full_local_only":
		// Later authorization still verifies the concrete tool and scope.
	default:
		reasons = append(reasons, "Current autonomy level requires explicit owner approval.")
	}
	if allocationItem.EstimatedCostMicros > 0 {
		reasons = append(reasons, "Estimated paid usage requires a separate budget approval.")
	}
	return reasons
}

func digestPortfolioExecutionState(
	pursuit models.Pursuit,
	allocationItem models.PursuitPortfolioAllocationItem,
	settled map[uuid.UUID]struct{},
) (string, error) {
	_, reservationSettled := settled[allocationItem.ReservationID]
	payload := struct {
		PursuitID, AllocationItemDigest, Status, RiskLevel, AutonomyLevel, NextAction, CompletionState string
		Archived                                                                                       bool
		StopConditions                                                                                 []models.PursuitStopCondition
		Dependencies                                                                                   []models.PursuitDependency
		ReservationSettled                                                                             bool
	}{
		pursuit.ID.String(), allocationItem.RecordDigest,
		strings.ToLower(strings.TrimSpace(pursuit.Status)), strings.ToLower(strings.TrimSpace(pursuit.RiskLevel)),
		strings.ToLower(strings.TrimSpace(pursuit.AutonomyLevel)), strings.Join(strings.Fields(pursuit.NextRecommendedAction), " "),
		strings.ToLower(strings.TrimSpace(pursuit.CompletionState)), pursuit.Archived,
		pursuit.StopConditions, pursuit.Dependencies, reservationSettled,
	}
	return digestPortfolioPayload(payload)
}

func digestPortfolioExecutionProposal(value *models.PursuitPortfolioExecutionProposal) (string, error) {
	payload := struct {
		AllocationID, OwnerIdentity, AllocationRecordDigest, SnapshotDigest, Status, Actor, Confirmation, Authority string
	}{
		value.AllocationID.String(), value.OwnerIdentity, value.AllocationRecordDigest, value.SnapshotDigest,
		value.Status, value.Actor, value.Confirmation, value.Authority,
	}
	return digestPortfolioPayload(payload)
}

func digestPortfolioExecutionProposalItem(snapshotDigest string, value models.PursuitPortfolioExecutionProposalItem) (string, error) {
	payload := struct {
		SnapshotDigest, AllocationItemID, PursuitID, ReservationID, OwnerIdentity, ActionSummary string
		PursuitStatus, RiskLevel, AutonomyLevel, Status                                          string
		RequiresApproval                                                                         bool
		ApprovalReasons, BlockedReasons                                                          []string
		AllocationItemDigest, StateDigest                                                        string
	}{
		snapshotDigest, value.AllocationItemID.String(), value.PursuitID.String(), value.ReservationID.String(), value.OwnerIdentity,
		value.ActionSummary, value.PursuitStatus, value.RiskLevel, value.AutonomyLevel, value.Status,
		value.RequiresApproval, value.ApprovalReasons, value.BlockedReasons, value.AllocationItemDigest, value.StateDigest,
	}
	return digestPortfolioPayload(payload)
}

func validatePortfolioExecutionProposalEvidence(
	ownerIdentity string,
	proposal *models.PursuitPortfolioExecutionProposal,
	items []models.PursuitPortfolioExecutionProposalItem,
) error {
	if proposal == nil || proposal.ID == uuid.Nil || proposal.AllocationID == uuid.Nil || proposal.OwnerIdentity != ownerIdentity ||
		proposal.Actor != ownerIdentity || proposal.Confirmation != PortfolioExecutionProposalConfirmation ||
		proposal.Authority != PortfolioExecutionProposalAuthority || proposal.PreparedAt.IsZero() ||
		!validPortfolioRecordDigest(proposal.AllocationRecordDigest) || !validPortfolioRecordDigest(proposal.SnapshotDigest) ||
		!validPortfolioRecordDigest(proposal.RecordDigest) {
		return fmt.Errorf("portfolio execution proposal contains invalid owner-scoped evidence")
	}
	expectedParent, err := digestPortfolioExecutionProposal(proposal)
	if err != nil || expectedParent != proposal.RecordDigest {
		return fmt.Errorf("portfolio execution proposal parent digest verification failed")
	}
	if len(items) == 0 || len(items) > 500 {
		return fmt.Errorf("portfolio execution proposal is missing item evidence")
	}
	anyBlocked := false
	anyApproval := false
	seenAllocationItems := map[uuid.UUID]struct{}{}
	for _, item := range items {
		if item.ID == uuid.Nil || item.ProposalID != proposal.ID || item.AllocationItemID == uuid.Nil || item.PursuitID == uuid.Nil ||
			item.ReservationID == uuid.Nil || item.OwnerIdentity != ownerIdentity || strings.TrimSpace(item.ActionSummary) == "" ||
			item.PreparedAt.IsZero() || !validPortfolioRecordDigest(item.AllocationItemDigest) ||
			!validPortfolioRecordDigest(item.StateDigest) || !validPortfolioRecordDigest(item.RecordDigest) {
			return fmt.Errorf("portfolio execution proposal contains invalid item evidence")
		}
		if _, duplicate := seenAllocationItems[item.AllocationItemID]; duplicate {
			return fmt.Errorf("portfolio execution proposal contains duplicate allocation item evidence")
		}
		seenAllocationItems[item.AllocationItemID] = struct{}{}
		if len(item.ApprovalReasons) > 40 || len(item.BlockedReasons) > 40 || item.RequiresApproval != (len(item.ApprovalReasons) > 0) {
			return fmt.Errorf("portfolio execution proposal contains inconsistent policy reasons")
		}
		for _, reason := range append(append([]string{}, item.ApprovalReasons...), item.BlockedReasons...) {
			if strings.TrimSpace(reason) == "" || strings.TrimSpace(reason) != reason || len(reason) > 1000 {
				return fmt.Errorf("portfolio execution proposal contains invalid policy evidence")
			}
		}
		switch item.Status {
		case PortfolioExecutionProposalItemBlocked:
			if len(item.BlockedReasons) == 0 {
				return fmt.Errorf("blocked portfolio execution proposal item is missing a reason")
			}
			anyBlocked = true
		case PortfolioExecutionProposalItemNeedsApproval:
			if !item.RequiresApproval || len(item.BlockedReasons) != 0 {
				return fmt.Errorf("approval-required portfolio execution proposal item is inconsistent")
			}
			anyApproval = true
		case PortfolioExecutionProposalItemProposed:
			if item.RequiresApproval || len(item.BlockedReasons) != 0 {
				return fmt.Errorf("proposed portfolio execution item contains unresolved policy gates")
			}
		default:
			return fmt.Errorf("portfolio execution proposal item status is invalid")
		}
		expectedItem, digestErr := digestPortfolioExecutionProposalItem(proposal.SnapshotDigest, item)
		if digestErr != nil || expectedItem != item.RecordDigest {
			return fmt.Errorf("portfolio execution proposal item digest verification failed")
		}
	}
	expectedStatus := PortfolioExecutionProposalPrepared
	if anyBlocked {
		expectedStatus = PortfolioExecutionProposalPreparedBlocked
	} else if anyApproval {
		expectedStatus = PortfolioExecutionProposalPreparedNeedsApproval
	}
	if proposal.Status != expectedStatus {
		return fmt.Errorf("portfolio execution proposal status does not match its item evidence")
	}
	return nil
}
