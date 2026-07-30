package agentcoordination

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ConflictType string

const (
	ConflictResourceContention    ConflictType = "resource_contention"
	ConflictContradictoryDecision ConflictType = "contradictory_decision"
)

type ConflictState string

const (
	ConflictDetected           ConflictState = "detected"
	ConflictUnderReview        ConflictState = "under_review"
	ConflictResolutionProposed ConflictState = "resolution_proposed"
	ConflictResolved           ConflictState = "resolved"
	ConflictEscalated          ConflictState = "escalated"
	ConflictDismissed          ConflictState = "dismissed"
)

type ConflictSeverity string

const (
	ConflictSeverityMedium ConflictSeverity = "medium"
	ConflictSeverityHigh   ConflictSeverity = "high"
)

type ResolutionStrategy string

const (
	ResolutionSerialize   ResolutionStrategy = "serialize"
	ResolutionReassign    ResolutionStrategy = "reassign"
	ResolutionMerge       ResolutionStrategy = "merge"
	ResolutionHumanReview ResolutionStrategy = "human_review"
	ResolutionAcceptOne   ResolutionStrategy = "accept_one"
)

type Conflict struct {
	ID              string             `json:"id"`
	CorrelationID   string             `json:"correlationId"`
	Type            ConflictType       `json:"type"`
	State           ConflictState      `json:"state"`
	Severity        ConflictSeverity   `json:"severity"`
	Subject         string             `json:"subject"`
	ParticipantIDs  []string           `json:"participantIds"`
	MessageIDs      []string           `json:"messageIds,omitempty"`
	DelegationIDs   []string           `json:"delegationIds,omitempty"`
	DetectedAt      time.Time          `json:"detectedAt"`
	UpdatedAt       time.Time          `json:"updatedAt"`
	Resolution      ResolutionStrategy `json:"resolution,omitempty"`
	ResolutionNotes string             `json:"resolutionNotes,omitempty"`
	Events          []ConflictEvent    `json:"events"`
}

type ConflictEvent struct {
	From      ConflictState      `json:"from"`
	To        ConflictState      `json:"to"`
	ActorID   string             `json:"actorId"`
	Strategy  ResolutionStrategy `json:"strategy,omitempty"`
	Rationale string             `json:"rationale"`
	CreatedAt time.Time          `json:"createdAt"`
}

type ConflictTransition struct {
	To        ConflictState
	ActorID   string
	Strategy  ResolutionStrategy
	Rationale string
}

// DetectConflicts performs deterministic, bounded checks. It reports competing
// resource claims and contradictory decisions; it does not choose a winner.
func DetectConflicts(
	messages []Message,
	delegations []DelegationEnvelope,
	now time.Time,
) []Conflict {
	conflicts := detectResourceConflicts(delegations, now)
	conflicts = append(conflicts, detectDecisionConflicts(messages, now)...)
	sort.Slice(conflicts, func(i, j int) bool {
		return conflicts[i].ID < conflicts[j].ID
	})
	return conflicts
}

func TransitionConflict(
	conflict Conflict,
	transition ConflictTransition,
	now time.Time,
) (Conflict, error) {
	if !allowedConflictTransition(conflict.State, transition.To) {
		return Conflict{}, fmt.Errorf(
			"conflict cannot transition from %q to %q",
			conflict.State,
			transition.To,
		)
	}
	actorID := normalizeID(transition.ActorID)
	rationale := normalizeText(transition.Rationale)
	if actorID == "" || rationale == "" {
		return Conflict{}, fmt.Errorf("conflict transition requires actor and rationale")
	}
	if now.IsZero() || now.Before(conflict.DetectedAt) {
		return Conflict{}, fmt.Errorf("conflict transition time is invalid")
	}
	if (transition.To == ConflictResolutionProposed || transition.To == ConflictResolved) &&
		!validResolutionStrategy(transition.Strategy) {
		return Conflict{}, fmt.Errorf("conflict resolution requires a valid strategy")
	}
	if transition.To == ConflictEscalated && transition.Strategy != ResolutionHumanReview {
		return Conflict{}, fmt.Errorf("escalated conflict must request human review")
	}
	conflict.Events = append(
		append([]ConflictEvent(nil), conflict.Events...),
		ConflictEvent{
			From:      conflict.State,
			To:        transition.To,
			ActorID:   actorID,
			Strategy:  transition.Strategy,
			Rationale: rationale,
			CreatedAt: now.UTC(),
		},
	)
	conflict.State = transition.To
	conflict.UpdatedAt = now.UTC()
	if transition.Strategy != "" {
		conflict.Resolution = transition.Strategy
	}
	conflict.ResolutionNotes = rationale
	return conflict, nil
}

func detectResourceConflicts(delegations []DelegationEnvelope, now time.Time) []Conflict {
	var conflicts []Conflict
	for leftIndex := 0; leftIndex < len(delegations); leftIndex++ {
		left := delegations[leftIndex]
		if terminalDelegation(left.Status) {
			continue
		}
		for rightIndex := leftIndex + 1; rightIndex < len(delegations); rightIndex++ {
			right := delegations[rightIndex]
			if terminalDelegation(right.Status) ||
				left.CorrelationID != right.CorrelationID ||
				normalizeID(left.Delegate.ID) == normalizeID(right.Delegate.ID) ||
				!delegationWindowsOverlap(left, right) {
				continue
			}
			for _, leftClaim := range left.ResourceClaims {
				for _, rightClaim := range right.ResourceClaims {
					if normalizeID(leftClaim.Resource) != normalizeID(rightClaim.Resource) ||
						!claimsConflict(leftClaim, rightClaim) {
						continue
					}
					subject := normalizeID(leftClaim.Resource)
					severity := ConflictSeverityMedium
					if leftClaim.Exclusive && rightClaim.Exclusive &&
						leftClaim.Access == ResourceWrite && rightClaim.Access == ResourceWrite {
						severity = ConflictSeverityHigh
					}
					conflicts = append(conflicts, newConflict(
						left.CorrelationID,
						ConflictResourceContention,
						severity,
						subject,
						[]string{left.Delegate.ID, right.Delegate.ID},
						nil,
						[]string{left.ID, right.ID},
						now,
					))
				}
			}
		}
	}
	return deduplicateConflicts(conflicts)
}

func detectDecisionConflicts(messages []Message, now time.Time) []Conflict {
	type decisionKey struct {
		correlation string
		subject     string
	}
	grouped := map[decisionKey][]Message{}
	for _, message := range messages {
		if message.Type != MessageTypeDecision {
			continue
		}
		key := decisionKey{
			correlation: message.CorrelationID,
			subject:     normalizeText(message.Payload.Subject),
		}
		grouped[key] = append(grouped[key], message)
	}
	var conflicts []Conflict
	for key, decisions := range grouped {
		for leftIndex := 0; leftIndex < len(decisions); leftIndex++ {
			for rightIndex := leftIndex + 1; rightIndex < len(decisions); rightIndex++ {
				left := decisions[leftIndex]
				right := decisions[rightIndex]
				if normalizeID(left.Sender.ID) == normalizeID(right.Sender.ID) ||
					decisionPayloadDigest(left) == decisionPayloadDigest(right) {
					continue
				}
				conflicts = append(conflicts, newConflict(
					key.correlation,
					ConflictContradictoryDecision,
					ConflictSeverityHigh,
					key.subject,
					[]string{left.Sender.ID, right.Sender.ID},
					[]string{left.ID, right.ID},
					nil,
					now,
				))
			}
		}
	}
	return deduplicateConflicts(conflicts)
}

func decisionPayloadDigest(message Message) string {
	var payload any
	decoder := json.NewDecoder(bytes.NewReader(message.Payload.Data))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		payload = string(bytes.TrimSpace(message.Payload.Data))
	}
	raw, err := json.Marshal(struct {
		Schema  string `json:"schema"`
		Subject string `json:"subject"`
		Data    any    `json:"data"`
	}{
		Schema:  strings.TrimSpace(message.Payload.Schema),
		Subject: normalizeText(message.Payload.Subject),
		Data:    payload,
	})
	if err != nil {
		raw = bytes.TrimSpace(message.Payload.Data)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func newConflict(
	correlationID string,
	conflictType ConflictType,
	severity ConflictSeverity,
	subject string,
	participantIDs []string,
	messageIDs []string,
	delegationIDs []string,
	now time.Time,
) Conflict {
	participantIDs = normalizeStrings(participantIDs)
	messageIDs = normalizeStrings(messageIDs)
	delegationIDs = normalizeStrings(delegationIDs)
	name := strings.Join([]string{
		correlationID,
		string(conflictType),
		subject,
		strings.Join(participantIDs, ","),
		strings.Join(messageIDs, ","),
		strings.Join(delegationIDs, ","),
	}, "|")
	return Conflict{
		ID:             uuid.NewSHA1(uuid.NameSpaceOID, []byte(name)).String(),
		CorrelationID:  correlationID,
		Type:           conflictType,
		State:          ConflictDetected,
		Severity:       severity,
		Subject:        subject,
		ParticipantIDs: participantIDs,
		MessageIDs:     messageIDs,
		DelegationIDs:  delegationIDs,
		DetectedAt:     now.UTC(),
		UpdatedAt:      now.UTC(),
		Events:         []ConflictEvent{},
	}
}

func claimsConflict(left, right ResourceClaim) bool {
	if left.Access == ResourceRead && right.Access == ResourceRead {
		return false
	}
	return left.Exclusive || right.Exclusive ||
		(left.Access == ResourceWrite && right.Access == ResourceWrite)
}

func delegationWindowsOverlap(left, right DelegationEnvelope) bool {
	return left.CreatedAt.Before(right.DueAt) && right.CreatedAt.Before(left.DueAt)
}

func terminalDelegation(status DelegationStatus) bool {
	return status == DelegationCompleted ||
		status == DelegationCancelled ||
		status == DelegationRejected ||
		status == DelegationExpired
}

func deduplicateConflicts(conflicts []Conflict) []Conflict {
	seen := map[string]Conflict{}
	for _, conflict := range conflicts {
		seen[conflict.ID] = conflict
	}
	result := make([]Conflict, 0, len(seen))
	for _, conflict := range seen {
		result = append(result, conflict)
	}
	return result
}

func allowedConflictTransition(from, to ConflictState) bool {
	allowed := map[ConflictState]map[ConflictState]bool{
		ConflictDetected: {
			ConflictUnderReview:        true,
			ConflictResolutionProposed: true,
			ConflictEscalated:          true,
			ConflictDismissed:          true,
		},
		ConflictUnderReview: {
			ConflictResolutionProposed: true,
			ConflictEscalated:          true,
			ConflictDismissed:          true,
		},
		ConflictResolutionProposed: {
			ConflictResolved:    true,
			ConflictUnderReview: true,
			ConflictEscalated:   true,
			ConflictDismissed:   true,
		},
		ConflictEscalated: {
			ConflictUnderReview:        true,
			ConflictResolutionProposed: true,
			ConflictResolved:           true,
			ConflictDismissed:          true,
		},
	}
	return allowed[from][to]
}

func validResolutionStrategy(strategy ResolutionStrategy) bool {
	switch strategy {
	case ResolutionSerialize,
		ResolutionReassign,
		ResolutionMerge,
		ResolutionHumanReview,
		ResolutionAcceptOne:
		return true
	default:
		return false
	}
}
