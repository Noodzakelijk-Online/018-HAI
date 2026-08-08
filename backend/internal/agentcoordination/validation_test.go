package agentcoordination

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestValidateMessageEnforcesTypedSafeEnvelope(t *testing.T) {
	t.Parallel()

	now := fixedNow()
	policy := DefaultValidationPolicy()
	message := validMessage(t, now)
	if err := ValidateMessage(policy, message, now); err != nil {
		t.Fatalf("ValidateMessage(valid): %v", err)
	}

	tests := map[string]func(Message) Message{
		"invalid correlation": func(value Message) Message {
			value.CorrelationID = "not-a-uuid"
			return value
		},
		"unknown type": func(value Message) Message {
			value.Type = MessageType("authority_grant")
			return withDigest(t, value)
		},
		"authority too high": func(value Message) Message {
			value.AuthorityLevel = 5
			return withDigest(t, value)
		},
		"expired": func(value Message) Message {
			value.ExpiresAt = now.Add(-time.Minute)
			return value
		},
		"invalid payload": func(value Message) Message {
			value.Payload.Data = json.RawMessage(`{`)
			return value
		},
		"decision without evidence": func(value Message) Message {
			value.Type = MessageTypeDecision
			value.EvidenceRefs = nil
			return withDigest(t, value)
		},
		"secret": func(value Message) Message {
			value.Payload.Data = json.RawMessage(`{"api_key":"do-not-store"}`)
			return withDigest(t, value)
		},
		"tampered digest": func(value Message) Message {
			value.Payload.Subject = "tampered"
			return value
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			err := ValidateMessage(policy, mutate(message), now)
			if err == nil {
				t.Fatal("invalid message was accepted")
			}
			if strings.Contains(err.Error(), "do-not-store") {
				t.Fatalf("error leaked secret: %v", err)
			}
		})
	}
}

func TestValidateDelegationAndStateTransitions(t *testing.T) {
	t.Parallel()

	now := fixedNow()
	delegation := validDelegation(now)
	if err := ValidateDelegation(DefaultValidationPolicy(), delegation, now); err != nil {
		t.Fatalf("ValidateDelegation(valid): %v", err)
	}
	accepted, err := TransitionDelegation(
		delegation,
		DelegationTransition{
			To:      DelegationAccepted,
			ActorID: delegation.Delegate.ID,
			Reason:  "bounded task accepted",
		},
		now.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("accept delegation: %v", err)
	}
	if err := ValidateDelegation(DefaultValidationPolicy(), accepted, now.Add(time.Minute)); err != nil {
		t.Fatalf("ValidateDelegation(accepted without execution approval): %v", err)
	}
	inProgress, err := TransitionDelegation(
		accepted,
		DelegationTransition{
			To:               DelegationInProgress,
			ActorID:          delegation.Delegate.ID,
			Reason:           "approved execution started",
			HumanApprovalRef: "approval://human/42",
		},
		now.Add(2*time.Minute),
	)
	if err != nil {
		t.Fatalf("start delegation: %v", err)
	}
	completed, err := TransitionDelegation(
		inProgress,
		DelegationTransition{
			To:                 DelegationCompleted,
			ActorID:            delegation.Delegate.ID,
			Reason:             "success criteria verified",
			CompletionEvidence: []string{"test://agentcoordination"},
		},
		now.Add(3*time.Minute),
	)
	if err != nil {
		t.Fatalf("complete delegation: %v", err)
	}
	if completed.Status != DelegationCompleted ||
		len(completed.CompletionEvidence) != 1 ||
		len(completed.DelegationTransitions) != 3 {
		t.Fatalf("unexpected completed delegation: %#v", completed)
	}

	if _, err := TransitionDelegation(
		delegation,
		DelegationTransition{
			To:                 DelegationCompleted,
			ActorID:            delegation.Delegate.ID,
			Reason:             "skip",
			CompletionEvidence: []string{"claim"},
		},
		now,
	); err == nil {
		t.Fatal("invalid direct completion transition was accepted")
	}
}

func TestValidateAcknowledgmentBindsMessageAndRecipient(t *testing.T) {
	t.Parallel()

	now := fixedNow()
	message := validMessage(t, now)
	acknowledgment := Acknowledgment{
		ID:             uuid.NewString(),
		MessageID:      message.ID,
		CorrelationID:  message.CorrelationID,
		RecipientID:    message.Recipient.ID,
		Status:         AcknowledgmentAccepted,
		CreatedAt:      now.Add(time.Minute),
		IdempotencyKey: uuid.NewString(),
	}
	if err := ValidateAcknowledgment(message, acknowledgment, now.Add(time.Minute)); err != nil {
		t.Fatalf("ValidateAcknowledgment(valid): %v", err)
	}
	acknowledgment.RecipientID = "another-agent"
	if err := ValidateAcknowledgment(message, acknowledgment, now.Add(time.Minute)); err == nil {
		t.Fatal("acknowledgment from the wrong recipient was accepted")
	}
}

func TestComputeAcknowledgmentDigestIsStableAndContentBound(t *testing.T) {
	t.Parallel()

	now := fixedNow().Add(time.Minute)
	acknowledgment := Acknowledgment{
		ID:             uuid.NewString(),
		MessageID:      uuid.NewString(),
		CorrelationID:  uuid.NewString(),
		RecipientID:    " Reviewer ",
		Status:         AcknowledgmentRejected,
		Reason:         " Source was not available. ",
		CreatedAt:      now,
		IdempotencyKey: uuid.NewString(),
	}
	first, err := ComputeAcknowledgmentDigest(acknowledgment)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ComputeAcknowledgmentDigest(acknowledgment)
	if err != nil || first != second || len(first) != 64 {
		t.Fatalf("stable acknowledgment digest = %q / %q, err %v", first, second, err)
	}
	acknowledgment.Status = AcknowledgmentAccepted
	changed, err := ComputeAcknowledgmentDigest(acknowledgment)
	if err != nil || changed == first {
		t.Fatalf("content-bound acknowledgment digest = %q, err %v", changed, err)
	}
}

func validMessage(t *testing.T, now time.Time) Message {
	t.Helper()
	message := Message{
		ID:              uuid.NewString(),
		IdempotencyKey:  uuid.NewString(),
		CorrelationID:   uuid.NewString(),
		SchemaVersion:   DefaultValidationPolicy().SchemaVersion,
		Type:            MessageTypeProposal,
		Sender:          AgentRef{ID: "planner", Role: "planner", AuthorityCeiling: 4},
		Recipient:       AgentRef{ID: "reviewer", Role: "reviewer", AuthorityCeiling: 4},
		Confidentiality: ConfidentialityInternal,
		AuthorityLevel:  2,
		Payload: MessagePayload{
			Schema:  "hai.proposal.v1",
			Subject: "bounded implementation",
			Data:    json.RawMessage(`{"summary":"review this plan"}`),
		},
		EvidenceRefs:      []string{"source://task/1"},
		RequiresAck:       true,
		CreatedAt:         now,
		ExpiresAt:         now.Add(time.Hour),
		ProvenanceSummary: "task planner output",
	}
	return withDigest(t, message)
}

func validDelegation(now time.Time) DelegationEnvelope {
	return DelegationEnvelope{
		ID:                uuid.NewString(),
		TaskID:            uuid.NewString(),
		IdempotencyKey:    uuid.NewString(),
		CorrelationID:     uuid.NewString(),
		Principal:         AgentRef{ID: "orchestrator", Role: "orchestrator", AuthorityCeiling: 4},
		Delegate:          AgentRef{ID: "worker", Role: "bounded_worker", AuthorityCeiling: 3},
		Objective:         "implement and test the isolated package",
		SuccessCriteria:   []string{"focused tests pass"},
		StopConditions:    []string{"scope expansion is required"},
		AllowedTools:      []string{"filesystem"},
		ProhibitedActions: []string{"modify files outside assigned package", "self approve"},
		ResourceClaims: []ResourceClaim{
			{Resource: "backend/internal/agentcoordination", Access: ResourceWrite, Exclusive: true},
		},
		ExecutionMode:     ExecutionModeExecuteLowRisk,
		ApprovalMode:      ApprovalBeforeExecution,
		RequiredAuthority: 3,
		Status:            DelegationProposed,
		CreatedAt:         now,
		DueAt:             now.Add(time.Hour),
		UpdatedAt:         now,
	}
}

func withDigest(t *testing.T, message Message) Message {
	t.Helper()
	digest, err := ComputeMessageDigest(message)
	if err != nil {
		t.Fatalf("ComputeMessageDigest: %v", err)
	}
	message.PayloadDigest = digest
	return message
}

func fixedNow() time.Time {
	return time.Date(2026, time.July, 30, 18, 0, 0, 0, time.UTC)
}
