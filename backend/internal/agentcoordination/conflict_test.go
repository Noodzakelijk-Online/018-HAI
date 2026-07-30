package agentcoordination

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDetectConflictsFindsResourceAndDecisionConflicts(t *testing.T) {
	t.Parallel()

	now := fixedNow()
	left := validDelegation(now)
	right := validDelegation(now)
	right.CorrelationID = left.CorrelationID
	right.Delegate = AgentRef{ID: "other-worker", Role: "bounded_worker", AuthorityCeiling: 3}
	right.ResourceClaims = []ResourceClaim{
		{Resource: "backend/internal/agentcoordination", Access: ResourceWrite, Exclusive: true},
	}

	firstDecision := validMessage(t, now)
	firstDecision.Type = MessageTypeDecision
	firstDecision.CorrelationID = left.CorrelationID
	firstDecision.Payload.Subject = "implementation strategy"
	firstDecision.Payload.Data = json.RawMessage(`{"choice":"serialize"}`)
	firstDecision.EvidenceRefs = []string{"test://one"}
	firstDecision = withDigest(t, firstDecision)

	secondDecision := firstDecision
	secondDecision.ID = uuid.NewString()
	secondDecision.IdempotencyKey = uuid.NewString()
	secondDecision.Sender = AgentRef{ID: "critic", Role: "critic", AuthorityCeiling: 4}
	secondDecision.Payload.Data = json.RawMessage(`{"choice":"parallel"}`)
	secondDecision.EvidenceRefs = []string{"test://two"}
	secondDecision = withDigest(t, secondDecision)

	conflicts := DetectConflicts(
		[]Message{firstDecision, secondDecision},
		[]DelegationEnvelope{left, right},
		now,
	)
	if len(conflicts) != 2 {
		t.Fatalf("conflict count = %d, want 2: %#v", len(conflicts), conflicts)
	}

	var resourceConflict Conflict
	for _, conflict := range conflicts {
		if conflict.Type == ConflictResourceContention {
			resourceConflict = conflict
		}
	}
	if resourceConflict.ID == "" || resourceConflict.Severity != ConflictSeverityHigh {
		t.Fatalf("resource conflict was not detected correctly: %#v", resourceConflict)
	}
	proposed, err := TransitionConflict(
		resourceConflict,
		ConflictTransition{
			To:        ConflictResolutionProposed,
			ActorID:   "orchestrator",
			Strategy:  ResolutionSerialize,
			Rationale: "serialize the exclusive writes",
		},
		now.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("propose resolution: %v", err)
	}
	resolved, err := TransitionConflict(
		proposed,
		ConflictTransition{
			To:        ConflictResolved,
			ActorID:   "human-reviewer",
			Strategy:  ResolutionSerialize,
			Rationale: "ordered execution approved",
		},
		now.Add(2*time.Minute),
	)
	if err != nil {
		t.Fatalf("resolve conflict: %v", err)
	}
	if resolved.State != ConflictResolved || len(resolved.Events) != 2 {
		t.Fatalf("unexpected resolved conflict: %#v", resolved)
	}
}

func TestConflictTerminalStateCannotBeReopened(t *testing.T) {
	t.Parallel()

	now := fixedNow()
	conflict := Conflict{
		ID:            uuid.NewString(),
		CorrelationID: uuid.NewString(),
		Type:          ConflictResourceContention,
		State:         ConflictDismissed,
		Severity:      ConflictSeverityMedium,
		Subject:       "resource",
		DetectedAt:    now,
		UpdatedAt:     now,
	}
	_, err := TransitionConflict(
		conflict,
		ConflictTransition{
			To:        ConflictUnderReview,
			ActorID:   "orchestrator",
			Rationale: "reopen",
		},
		now.Add(time.Minute),
	)
	if err == nil {
		t.Fatal("terminal conflict state was reopened")
	}
}

func TestDetectConflictsAcceptsEquivalentDecisionsFromDifferentAgents(t *testing.T) {
	t.Parallel()

	now := fixedNow()
	first := validMessage(t, now)
	first.Type = MessageTypeDecision
	first.Payload.Subject = "route"
	first.Payload.Data = json.RawMessage(`{"tier":"free","model":"local"}`)
	first.EvidenceRefs = []string{"source://policy"}
	first = withDigest(t, first)

	second := first
	second.ID = uuid.NewString()
	second.IdempotencyKey = uuid.NewString()
	second.Sender = AgentRef{ID: "second-reviewer", Role: "reviewer", AuthorityCeiling: 4}
	second.Payload.Data = json.RawMessage(`{"model":"local","tier":"free"}`)
	second = withDigest(t, second)

	conflicts := DetectConflicts([]Message{first, second}, nil, now)
	if len(conflicts) != 0 {
		t.Fatalf("equivalent decisions were marked conflicting: %#v", conflicts)
	}
}
