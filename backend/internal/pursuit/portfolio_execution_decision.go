package pursuit

import (
	"automation-hub-backend/internal/models"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	PortfolioExecutionDecisionApproved           = "approved"
	PortfolioExecutionDecisionRejected           = "rejected"
	PortfolioExecutionDecisionNeedsClarification = "needs_clarification"
	PortfolioExecutionDecisionRevoked            = "revoked"

	PortfolioExecutionDecisionApproveConfirmation = "APPROVE EXECUTION PROPOSAL ITEM"
	PortfolioExecutionDecisionRejectConfirmation  = "REJECT EXECUTION PROPOSAL ITEM"
	PortfolioExecutionDecisionClarifyConfirmation = "REQUEST CLARIFICATION FOR EXECUTION PROPOSAL ITEM"
	PortfolioExecutionDecisionRevokeConfirmation  = "REVOKE EXECUTION PROPOSAL ITEM"
	PortfolioExecutionDecisionAuthority           = "approval_decision_only"

	portfolioExecutionDecisionApprovalLifetime = 30 * time.Minute
)

type PortfolioExecutionProposalDecisionRequest struct {
	ExpectedItemDigest string `json:"expectedItemDigest"`
	Decision           string `json:"decision"`
	Reason             string `json:"reason"`
	Confirmation       string `json:"confirmation"`
}

type PortfolioExecutionProposalDecisionResult struct {
	Decision   *models.PursuitPortfolioExecutionProposalDecision `json:"decision"`
	Replayed   bool                                              `json:"replayed"`
	Authority  string                                            `json:"authority"`
	CanExecute bool                                              `json:"canExecute"`
}

type PortfolioExecutionProposalDecisionHistoryResult struct {
	Decisions  []models.PursuitPortfolioExecutionProposalDecision `json:"decisions"`
	Authority  string                                             `json:"authority"`
	CanExecute bool                                               `json:"canExecute"`
}

type portfolioExecutionProposalDecisionSnapshot struct {
	Allocation     models.PursuitPortfolioAllocation
	Proposal       models.PursuitPortfolioExecutionProposal
	Item           models.PursuitPortfolioExecutionProposalItem
	AllocationItem models.PursuitPortfolioAllocationItem
	Pursuit        models.Pursuit
	Settled        bool
	LatestDecision *models.PursuitPortfolioExecutionProposalDecision
}

type pursuitPortfolioExecutionProposalDecisionRepository interface {
	LoadPortfolioExecutionProposalDecisionSnapshot(string, uuid.UUID) (*portfolioExecutionProposalDecisionSnapshot, error)
	SavePortfolioExecutionProposalDecision(
		*portfolioExecutionProposalDecisionSnapshot,
		*models.PursuitPortfolioExecutionProposalDecision,
		models.PursuitActivity,
	) (*models.PursuitPortfolioExecutionProposalDecision, bool, error)
	ListPortfolioExecutionProposalDecisions(string, uuid.UUID, int) ([]models.PursuitPortfolioExecutionProposalDecision, error)
}

// DecidePortfolioExecutionProposalItemForOwner records one append-only owner
// decision. It does not consume approval, enqueue work, settle a reservation,
// call a tool/runtime, or authorize a concrete external effect.
func (s *service) DecidePortfolioExecutionProposalItemForOwner(
	ownerIdentity, actor string,
	itemID uuid.UUID,
	request PortfolioExecutionProposalDecisionRequest,
) (*PortfolioExecutionProposalDecisionResult, error) {
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	actor = strings.TrimSpace(actor)
	request.ExpectedItemDigest = strings.TrimSpace(request.ExpectedItemDigest)
	request.Decision = strings.ToLower(strings.TrimSpace(request.Decision))
	request.Reason = strings.TrimSpace(request.Reason)
	request.Confirmation = strings.TrimSpace(request.Confirmation)
	if ownerIdentity == "" || actor == "" || actor != ownerIdentity {
		return nil, fmt.Errorf("the authenticated owner must decide a portfolio execution proposal item")
	}
	if itemID == uuid.Nil {
		return nil, fmt.Errorf("a valid portfolio execution proposal item id is required")
	}
	if !validPortfolioRecordDigest(request.ExpectedItemDigest) {
		return nil, fmt.Errorf("a valid expected proposal item digest is required")
	}
	if utf8.RuneCountInString(request.Reason) == 0 || utf8.RuneCountInString(request.Reason) > 2000 {
		return nil, fmt.Errorf("a decision reason between 1 and 2000 characters is required")
	}
	expectedConfirmation, ok := portfolioExecutionDecisionConfirmation(request.Decision)
	if !ok || request.Confirmation != expectedConfirmation {
		return nil, fmt.Errorf("the exact confirmation for this proposal item decision is required")
	}
	repository, ok := s.repo.(pursuitPortfolioExecutionProposalDecisionRepository)
	if !ok {
		return nil, fmt.Errorf("durable portfolio execution proposal decision storage is unavailable")
	}
	snapshot, err := repository.LoadPortfolioExecutionProposalDecisionSnapshot(ownerIdentity, itemID)
	if err != nil {
		return nil, fmt.Errorf("load portfolio execution proposal decision state: %w", err)
	}
	if snapshot == nil {
		return nil, fmt.Errorf("portfolio execution proposal item is unavailable to this owner")
	}
	if err := validatePortfolioExecutionDecisionAllocation(ownerIdentity, snapshot); err != nil {
		return nil, err
	}
	if err := validatePortfolioExecutionDecisionSource(ownerIdentity, snapshot); err != nil {
		return nil, err
	}
	if snapshot.Item.RecordDigest != request.ExpectedItemDigest {
		return nil, fmt.Errorf("portfolio execution proposal item changed; inspect the immutable item before continuing")
	}
	if snapshot.Item.Status == PortfolioExecutionProposalItemBlocked || len(snapshot.Item.BlockedReasons) > 0 {
		return nil, fmt.Errorf("blocked portfolio execution proposal items cannot be decided")
	}
	settled := map[uuid.UUID]struct{}{}
	if snapshot.Settled {
		settled[snapshot.Item.ReservationID] = struct{}{}
	}
	currentStateDigest, err := digestPortfolioExecutionState(snapshot.Pursuit, snapshot.AllocationItem, settled)
	if err != nil || currentStateDigest != snapshot.Item.StateDigest {
		return nil, fmt.Errorf("portfolio execution proposal pursuit state changed; prepare and inspect a fresh proposal")
	}
	if request.Decision == PortfolioExecutionDecisionRevoked {
		if snapshot.LatestDecision == nil || snapshot.LatestDecision.Decision != PortfolioExecutionDecisionApproved {
			return nil, fmt.Errorf("only the latest approved proposal item decision can be revoked")
		}
	}
	if request.Decision == PortfolioExecutionDecisionApproved {
		if _, err := s.revalidatePortfolioAllocationCoordinationPlan(
			ownerIdentity,
			&snapshot.Allocation,
			[]models.PursuitPortfolioAllocationItem{snapshot.AllocationItem},
		); err != nil {
			return nil, fmt.Errorf("revalidate portfolio allocation coordination plan before approval: %w", err)
		}
	}

	requestDigest, err := digestPortfolioPayload(struct {
		ItemID, ItemDigest, Decision, Reason, Confirmation string
	}{itemID.String(), snapshot.Item.RecordDigest, request.Decision, request.Reason, request.Confirmation})
	if err != nil {
		return nil, err
	}
	if snapshot.LatestDecision != nil && snapshot.LatestDecision.RequestDigest == requestDigest {
		if err := validatePortfolioExecutionDecisionEvidence(ownerIdentity, snapshot.Item, snapshot.LatestDecision); err != nil {
			return nil, err
		}
		return &PortfolioExecutionProposalDecisionResult{
			Decision: snapshot.LatestDecision, Replayed: true,
			Authority: PortfolioExecutionDecisionAuthority, CanExecute: false,
		}, nil
	}

	decidedAt := time.Now().UTC().Truncate(time.Second)
	decision := &models.PursuitPortfolioExecutionProposalDecision{
		ID: uuid.New(), ProposalItemID: snapshot.Item.ID, ProposalID: snapshot.Proposal.ID,
		PursuitID: snapshot.Item.PursuitID, OwnerIdentity: ownerIdentity,
		Decision: request.Decision, Reason: request.Reason, Actor: actor,
		Confirmation: request.Confirmation, ProposalItemDigest: snapshot.Item.RecordDigest,
		StateDigest: snapshot.Item.StateDigest, Authority: PortfolioExecutionDecisionAuthority,
		RequestDigest: requestDigest, DecidedAt: decidedAt,
	}
	if snapshot.LatestDecision != nil {
		previousID := snapshot.LatestDecision.ID
		decision.PreviousDecisionID = &previousID
	}
	if decision.Decision == PortfolioExecutionDecisionApproved {
		expiresAt := decidedAt.Add(portfolioExecutionDecisionApprovalLifetime)
		decision.ExpiresAt = &expiresAt
	}
	decision.RecordDigest, err = digestPortfolioExecutionDecision(decision)
	if err != nil {
		return nil, err
	}
	activity := newPursuitResourceActivity(
		decision.PursuitID,
		"pursuit.portfolio_execution_proposal_decided",
		fmt.Sprintf("Recorded %s proposal item decision; concrete effect authorization remains separate.", decision.Decision),
		actor,
		"pursuit_portfolio_execution_proposal_decision",
		decision.ID.String(),
		"hai://pursuits/"+decision.PursuitID.String()+"/portfolio-execution-proposal-decisions/"+decision.ID.String(),
		decidedAt,
	)
	stored, created, err := repository.SavePortfolioExecutionProposalDecision(snapshot, decision, activity)
	if err != nil {
		return nil, fmt.Errorf("save portfolio execution proposal decision: %w", err)
	}
	if err := validatePortfolioExecutionDecisionEvidence(ownerIdentity, snapshot.Item, stored); err != nil {
		return nil, err
	}
	return &PortfolioExecutionProposalDecisionResult{
		Decision: stored, Replayed: !created,
		Authority: PortfolioExecutionDecisionAuthority, CanExecute: false,
	}, nil
}

func (s *service) PortfolioExecutionProposalDecisionHistoryForOwner(
	ownerIdentity string,
	itemID uuid.UUID,
	limit int,
) (*PortfolioExecutionProposalDecisionHistoryResult, error) {
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	if ownerIdentity == "" || itemID == uuid.Nil || limit < 1 || limit > 100 {
		return nil, fmt.Errorf("valid owner, proposal item, and history limit are required")
	}
	repository, ok := s.repo.(pursuitPortfolioExecutionProposalDecisionRepository)
	if !ok {
		return nil, fmt.Errorf("durable portfolio execution proposal decision storage is unavailable")
	}
	snapshot, err := repository.LoadPortfolioExecutionProposalDecisionSnapshot(ownerIdentity, itemID)
	if err != nil {
		return nil, err
	}
	if snapshot == nil {
		return nil, fmt.Errorf("portfolio execution proposal item is unavailable to this owner")
	}
	records, err := repository.ListPortfolioExecutionProposalDecisions(ownerIdentity, itemID, limit)
	if err != nil {
		return nil, err
	}
	for index := range records {
		if err := validatePortfolioExecutionDecisionEvidence(ownerIdentity, snapshot.Item, &records[index]); err != nil {
			return nil, err
		}
		if index+1 < len(records) {
			if records[index].PreviousDecisionID == nil || *records[index].PreviousDecisionID != records[index+1].ID {
				return nil, fmt.Errorf("portfolio execution proposal decision history chain verification failed")
			}
		}
	}
	return &PortfolioExecutionProposalDecisionHistoryResult{
		Decisions: records, Authority: PortfolioExecutionDecisionAuthority, CanExecute: false,
	}, nil
}

func portfolioExecutionDecisionConfirmation(decision string) (string, bool) {
	switch decision {
	case PortfolioExecutionDecisionApproved:
		return PortfolioExecutionDecisionApproveConfirmation, true
	case PortfolioExecutionDecisionRejected:
		return PortfolioExecutionDecisionRejectConfirmation, true
	case PortfolioExecutionDecisionNeedsClarification:
		return PortfolioExecutionDecisionClarifyConfirmation, true
	case PortfolioExecutionDecisionRevoked:
		return PortfolioExecutionDecisionRevokeConfirmation, true
	default:
		return "", false
	}
}

func validatePortfolioExecutionDecisionSource(
	ownerIdentity string,
	snapshot *portfolioExecutionProposalDecisionSnapshot,
) error {
	if snapshot == nil || snapshot.Proposal.ID == uuid.Nil || snapshot.Item.ID == uuid.Nil ||
		snapshot.Item.ProposalID != snapshot.Proposal.ID || snapshot.Item.PursuitID == uuid.Nil ||
		snapshot.Proposal.OwnerIdentity != ownerIdentity || snapshot.Item.OwnerIdentity != ownerIdentity ||
		snapshot.AllocationItem.ID != snapshot.Item.AllocationItemID ||
		snapshot.AllocationItem.PursuitID != snapshot.Item.PursuitID ||
		snapshot.AllocationItem.ReservationID != snapshot.Item.ReservationID ||
		snapshot.AllocationItem.OwnerIdentity != ownerIdentity || snapshot.Pursuit.ID != snapshot.Item.PursuitID ||
		snapshot.Pursuit.OwnerIdentity != ownerIdentity ||
		!validPortfolioRecordDigest(snapshot.Proposal.RecordDigest) ||
		!validPortfolioRecordDigest(snapshot.Item.RecordDigest) ||
		!validPortfolioRecordDigest(snapshot.Item.StateDigest) {
		return fmt.Errorf("portfolio execution proposal decision source evidence is invalid")
	}
	expectedItem, err := digestPortfolioExecutionProposalItem(snapshot.Proposal.SnapshotDigest, snapshot.Item)
	if err != nil || expectedItem != snapshot.Item.RecordDigest {
		return fmt.Errorf("portfolio execution proposal item digest verification failed")
	}
	expectedProposal, err := digestPortfolioExecutionProposal(&snapshot.Proposal)
	if err != nil || expectedProposal != snapshot.Proposal.RecordDigest {
		return fmt.Errorf("portfolio execution proposal parent digest verification failed")
	}
	if snapshot.LatestDecision != nil {
		if err := validatePortfolioExecutionDecisionEvidence(ownerIdentity, snapshot.Item, snapshot.LatestDecision); err != nil {
			return err
		}
	}
	return nil
}

func validatePortfolioExecutionDecisionAllocation(
	ownerIdentity string,
	snapshot *portfolioExecutionProposalDecisionSnapshot,
) error {
	if snapshot == nil || snapshot.Allocation.ID == uuid.Nil ||
		snapshot.Allocation.OwnerIdentity != ownerIdentity ||
		snapshot.Proposal.AllocationID != snapshot.Allocation.ID ||
		snapshot.Proposal.AllocationRecordDigest != snapshot.Allocation.RecordDigest ||
		!validPortfolioRecordDigest(snapshot.Allocation.RecordDigest) {
		return fmt.Errorf("portfolio execution proposal allocation evidence is invalid")
	}
	return nil
}

func digestPortfolioExecutionDecision(value *models.PursuitPortfolioExecutionProposalDecision) (string, error) {
	previous := ""
	if value.PreviousDecisionID != nil {
		previous = value.PreviousDecisionID.String()
	}
	expires := ""
	if value.ExpiresAt != nil {
		expires = value.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	return digestPortfolioPayload(struct {
		ID, ProposalItemID, ProposalID, PursuitID, OwnerIdentity, Decision, Reason, Actor string
		Confirmation, ProposalItemDigest, StateDigest, Authority, RequestDigest           string
		PreviousDecisionID, DecidedAt, ExpiresAt                                          string
	}{
		value.ID.String(), value.ProposalItemID.String(), value.ProposalID.String(), value.PursuitID.String(),
		value.OwnerIdentity, value.Decision, value.Reason, value.Actor, value.Confirmation,
		value.ProposalItemDigest, value.StateDigest, value.Authority, value.RequestDigest,
		previous, value.DecidedAt.UTC().Format(time.RFC3339Nano), expires,
	})
}

func validatePortfolioExecutionDecisionEvidence(
	ownerIdentity string,
	item models.PursuitPortfolioExecutionProposalItem,
	decision *models.PursuitPortfolioExecutionProposalDecision,
) error {
	if decision == nil || decision.ID == uuid.Nil || decision.ProposalItemID != item.ID ||
		decision.ProposalID != item.ProposalID || decision.PursuitID != item.PursuitID ||
		decision.OwnerIdentity != ownerIdentity || decision.Actor != ownerIdentity ||
		decision.ProposalItemDigest != item.RecordDigest || decision.StateDigest != item.StateDigest ||
		decision.Authority != PortfolioExecutionDecisionAuthority || decision.DecidedAt.IsZero() ||
		!validPortfolioRecordDigest(decision.RequestDigest) || !validPortfolioRecordDigest(decision.RecordDigest) {
		return fmt.Errorf("portfolio execution proposal decision contains invalid owner-scoped evidence")
	}
	expectedConfirmation, ok := portfolioExecutionDecisionConfirmation(decision.Decision)
	if !ok || decision.Confirmation != expectedConfirmation || strings.TrimSpace(decision.Reason) != decision.Reason ||
		utf8.RuneCountInString(decision.Reason) == 0 || utf8.RuneCountInString(decision.Reason) > 2000 {
		return fmt.Errorf("portfolio execution proposal decision contains invalid decision evidence")
	}
	if decision.Decision == PortfolioExecutionDecisionApproved {
		if decision.ExpiresAt == nil || !decision.ExpiresAt.After(decision.DecidedAt) {
			return fmt.Errorf("approved portfolio execution proposal decision is missing bounded expiry")
		}
	} else if decision.ExpiresAt != nil {
		return fmt.Errorf("non-approval portfolio execution proposal decision has an invalid expiry")
	}
	expectedDigest, err := digestPortfolioExecutionDecision(decision)
	if err != nil || expectedDigest != decision.RecordDigest {
		return fmt.Errorf("portfolio execution proposal decision digest verification failed")
	}
	return nil
}
