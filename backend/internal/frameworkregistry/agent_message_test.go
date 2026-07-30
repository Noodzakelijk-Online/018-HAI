package frameworkregistry

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNormalizeAgentMessageEnforcesSchemaCorrelationAuthorityAndRedaction(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 30, 16, 0, 0, 0, time.UTC)
	contract := CommunicationContract{
		SchemaVersion:          "hai-agent-message-v1",
		AllowedMessageTypes:    []string{"proposal", "evidence"},
		AllowedConfidentiality: []string{"internal", "restricted"},
		MaximumAuthority:       4,
		MaximumPayloadChars:    4000,
		MaximumTTLSeconds:      3600,
		RedactionRequired:      true,
		IdempotencyRequired:    true,
		ProvenanceRequired:     true,
		CorrelationID:          uuid.NewString(),
	}
	valid := AgentMessage{
		ID:               uuid.NewString(),
		IdempotencyKey:   uuid.NewString(),
		SchemaVersion:    contract.SchemaVersion,
		CorrelationID:    contract.CorrelationID,
		Sender:           "planning_agent",
		Recipient:        "critic",
		MessageType:      "proposal",
		Confidentiality:  "internal",
		AuthorityCeiling: 4,
		EvidenceRefs:     []string{"source://project/018"},
		PayloadSummary:   "Review the bounded implementation plan.",
		Provenance:       "verified task planner output",
		CreatedAt:        now,
		ExpiresAt:        timePointer(now.Add(30 * time.Minute)),
	}
	digest, err := agentMessagePayloadDigest(valid)
	if err != nil {
		t.Fatalf("agentMessagePayloadDigest: %v", err)
	}
	valid.PayloadDigest = digest
	normalized, err := NormalizeAgentMessage(contract, valid, now)
	if err != nil {
		t.Fatalf("NormalizeAgentMessage(valid): %v", err)
	}
	if normalized.Sender != "planning_agent" || normalized.Recipient != "critic" {
		t.Fatalf("message identity was not normalized: %#v", normalized)
	}

	tests := map[string]func(AgentMessage) AgentMessage{
		"wrong schema": func(value AgentMessage) AgentMessage {
			value.SchemaVersion = "other"
			return value
		},
		"wrong correlation": func(value AgentMessage) AgentMessage {
			value.CorrelationID = uuid.NewString()
			return value
		},
		"unsupported type": func(value AgentMessage) AgentMessage {
			value.MessageType = "authority_grant"
			return value
		},
		"unsupported confidentiality": func(value AgentMessage) AgentMessage {
			value.Confidentiality = "public"
			return value
		},
		"excess authority": func(value AgentMessage) AgentMessage {
			value.AuthorityCeiling = 5
			return value
		},
		"expired": func(value AgentMessage) AgentMessage {
			value.ExpiresAt = timePointer(now.Add(-time.Second))
			return value
		},
		"tampered payload": func(value AgentMessage) AgentMessage {
			value.PayloadSummary = "Changed after digest."
			return value
		},
		"self asserted signature": func(value AgentMessage) AgentMessage {
			value.SignatureDigest = strings.Repeat("a", 64)
			value.SignatureVerified = true
			return value
		},
		"secret payload": func(value AgentMessage) AgentMessage {
			value.PayloadSummary = "Use api_key=never-persist-this"
			return value
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := NormalizeAgentMessage(contract, mutate(valid), now)
			if err == nil {
				t.Fatal("invalid agent message was accepted")
			}
			if strings.Contains(err.Error(), "never-persist-this") {
				t.Fatalf("validation error leaked secret: %v", err)
			}
		})
	}
}

func timePointer(value time.Time) *time.Time {
	return &value
}
