package agentcoordination

import (
	"fmt"
	"strings"
	"time"
)

type DelegationTransition struct {
	To                 DelegationStatus
	ActorID            string
	Reason             string
	CompletionEvidence []string
	HumanApprovalRef   string
}

func TransitionDelegation(
	delegation DelegationEnvelope,
	transition DelegationTransition,
	now time.Time,
) (DelegationEnvelope, error) {
	if !allowedDelegationTransition(delegation.Status, transition.To) {
		return DelegationEnvelope{}, fmt.Errorf(
			"delegation cannot transition from %q to %q",
			delegation.Status,
			transition.To,
		)
	}
	actorID := normalizeID(transition.ActorID)
	reason := normalizeText(transition.Reason)
	if actorID == "" || reason == "" {
		return DelegationEnvelope{}, fmt.Errorf("delegation transition requires actor and reason")
	}
	if now.IsZero() || now.Before(delegation.CreatedAt) {
		return DelegationEnvelope{}, fmt.Errorf("delegation transition time is invalid")
	}
	if transition.To == DelegationCompleted {
		evidence := normalizeStrings(transition.CompletionEvidence)
		if len(evidence) == 0 {
			return DelegationEnvelope{}, fmt.Errorf("completed delegation requires completion evidence")
		}
		delegation.CompletionEvidence = evidence
	}
	if delegation.ApprovalMode == ApprovalBeforeExecution &&
		(transition.To == DelegationInProgress || transition.To == DelegationCompleted) {
		approvalRef := strings.TrimSpace(transition.HumanApprovalRef)
		if approvalRef == "" {
			approvalRef = strings.TrimSpace(delegation.HumanApprovalRef)
		}
		if approvalRef == "" {
			return DelegationEnvelope{}, fmt.Errorf("delegation requires external human approval before execution")
		}
		delegation.HumanApprovalRef = approvalRef
	}
	delegation.DelegationTransitions = append(
		append([]DelegationEvent(nil), delegation.DelegationTransitions...),
		DelegationEvent{
			From:      delegation.Status,
			To:        transition.To,
			ActorID:   actorID,
			Reason:    reason,
			CreatedAt: now.UTC(),
		},
	)
	delegation.Status = transition.To
	delegation.StatusReason = reason
	delegation.UpdatedAt = now.UTC()
	return delegation, nil
}

func allowedDelegationTransition(from, to DelegationStatus) bool {
	allowed := map[DelegationStatus]map[DelegationStatus]bool{
		DelegationProposed: {
			DelegationAccepted:  true,
			DelegationRejected:  true,
			DelegationCancelled: true,
			DelegationExpired:   true,
			DelegationEscalated: true,
		},
		DelegationAccepted: {
			DelegationInProgress: true,
			DelegationBlocked:    true,
			DelegationCancelled:  true,
			DelegationExpired:    true,
			DelegationEscalated:  true,
		},
		DelegationInProgress: {
			DelegationBlocked:   true,
			DelegationCompleted: true,
			DelegationCancelled: true,
			DelegationExpired:   true,
			DelegationEscalated: true,
		},
		DelegationBlocked: {
			DelegationInProgress: true,
			DelegationCancelled:  true,
			DelegationExpired:    true,
			DelegationEscalated:  true,
		},
		DelegationEscalated: {
			DelegationAccepted:   true,
			DelegationInProgress: true,
			DelegationBlocked:    true,
			DelegationCompleted:  true,
			DelegationCancelled:  true,
			DelegationExpired:    true,
		},
	}
	return allowed[from][to]
}
