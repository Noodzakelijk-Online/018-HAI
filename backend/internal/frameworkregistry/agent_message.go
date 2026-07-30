package frameworkregistry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"automation-hub-backend/internal/safety"

	"github.com/google/uuid"
)

// NormalizeAgentMessage validates the typed, authority-bounded envelope used
// between HAI agents. A valid message communicates evidence or a proposal; it
// never grants authority or proves that the recipient executed anything.
func NormalizeAgentMessage(
	contract CommunicationContract,
	input AgentMessage,
	now time.Time,
) (AgentMessage, error) {
	if strings.TrimSpace(contract.SchemaVersion) == "" ||
		strings.TrimSpace(contract.CorrelationID) == "" {
		return AgentMessage{}, fmt.Errorf("communication contract is incomplete")
	}
	if contract.MaximumAuthority < 0 || contract.MaximumAuthority > 10 {
		return AgentMessage{}, fmt.Errorf("communication contract authority must be between 0 and 10")
	}
	if contract.RedactionRequired && messageContainsSecret(input) {
		return AgentMessage{}, fmt.Errorf("agent message contains secret material")
	}
	if _, err := uuid.Parse(strings.TrimSpace(input.ID)); err != nil {
		return AgentMessage{}, fmt.Errorf("agent message ID must be a UUID")
	}
	input.ID = strings.TrimSpace(input.ID)
	if contract.IdempotencyRequired {
		if _, err := uuid.Parse(strings.TrimSpace(input.IdempotencyKey)); err != nil {
			return AgentMessage{}, fmt.Errorf("agent message idempotency key must be a UUID")
		}
	}
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.SchemaVersion = strings.TrimSpace(input.SchemaVersion)
	input.CorrelationID = strings.TrimSpace(input.CorrelationID)
	input.Sender = normalizeIdentifier(input.Sender)
	input.Recipient = normalizeIdentifier(input.Recipient)
	input.MessageType = normalizeIdentifier(input.MessageType)
	input.Confidentiality = normalizeIdentifier(input.Confidentiality)
	input.PayloadSummary = strings.Join(strings.Fields(strings.TrimSpace(input.PayloadSummary)), " ")
	input.EvidenceRefs = sortedUnique(input.EvidenceRefs)
	input.PayloadDigest = strings.ToLower(strings.TrimSpace(input.PayloadDigest))
	input.Provenance = compactContractText(input.Provenance)
	input.SignatureDigest = strings.ToLower(strings.TrimSpace(input.SignatureDigest))

	if input.SchemaVersion != contract.SchemaVersion {
		return AgentMessage{}, fmt.Errorf("agent message schema does not match the communication contract")
	}
	if input.CorrelationID != contract.CorrelationID {
		return AgentMessage{}, fmt.Errorf("agent message correlation does not match the communication contract")
	}
	if input.Sender == "" || input.Recipient == "" {
		return AgentMessage{}, fmt.Errorf("agent message sender and recipient are required")
	}
	if !containsExact(contract.AllowedMessageTypes, input.MessageType) {
		return AgentMessage{}, fmt.Errorf("agent message type %q is not allowed", input.MessageType)
	}
	if !containsExact(contract.AllowedConfidentiality, input.Confidentiality) {
		return AgentMessage{}, fmt.Errorf("agent message confidentiality %q is not allowed", input.Confidentiality)
	}
	if input.AuthorityCeiling < 0 || input.AuthorityCeiling > contract.MaximumAuthority {
		return AgentMessage{}, fmt.Errorf("agent message exceeds the communication authority ceiling")
	}
	if input.CreatedAt.IsZero() {
		return AgentMessage{}, fmt.Errorf("agent message creation time is required")
	}
	input.CreatedAt = input.CreatedAt.UTC()
	if input.CreatedAt.After(now.UTC().Add(5 * time.Minute)) {
		return AgentMessage{}, fmt.Errorf("agent message creation time cannot be in the future")
	}
	if input.ExpiresAt == nil {
		return AgentMessage{}, fmt.Errorf("agent message expiry is required")
	}
	expiresAt := input.ExpiresAt.UTC()
	input.ExpiresAt = &expiresAt
	if !expiresAt.After(input.CreatedAt) {
		return AgentMessage{}, fmt.Errorf("agent message expiry must be after creation")
	}
	if !expiresAt.After(now.UTC()) {
		return AgentMessage{}, fmt.Errorf("agent message has expired")
	}
	maxTTL := time.Duration(contract.MaximumTTLSeconds) * time.Second
	if contract.MaximumTTLSeconds <= 0 || expiresAt.Sub(input.CreatedAt) > maxTTL {
		return AgentMessage{}, fmt.Errorf("agent message expiry exceeds the communication contract")
	}
	if contract.MaximumPayloadChars <= 0 ||
		len([]rune(input.PayloadSummary)) > contract.MaximumPayloadChars {
		return AgentMessage{}, fmt.Errorf(
			"agent message payload summary exceeds %d characters",
			contract.MaximumPayloadChars,
		)
	}
	if contract.ProvenanceRequired && input.Provenance == "" {
		return AgentMessage{}, fmt.Errorf("agent message provenance is required")
	}
	expectedDigest, err := agentMessagePayloadDigest(input)
	if err != nil {
		return AgentMessage{}, fmt.Errorf("digest agent message payload: %w", err)
	}
	if input.PayloadDigest != expectedDigest {
		return AgentMessage{}, fmt.Errorf("agent message payload digest does not match its content")
	}
	if input.SignatureDigest != "" {
		if len(input.SignatureDigest) != sha256.Size*2 {
			return AgentMessage{}, fmt.Errorf("agent message signature digest must be a SHA-256 digest")
		}
		if _, err := hex.DecodeString(input.SignatureDigest); err != nil {
			return AgentMessage{}, fmt.Errorf("agent message signature digest must be a SHA-256 digest")
		}
	}
	if input.SignatureVerified {
		return AgentMessage{}, fmt.Errorf("agent messages cannot self-assert signature verification")
	}
	if contract.RedactionRequired && messageContainsSecret(input) {
		return AgentMessage{}, fmt.Errorf("agent message contains secret material")
	}
	return input, nil
}

func messageContainsSecret(message AgentMessage) bool {
	values := append([]string{
		message.Sender,
		message.Recipient,
		message.PayloadSummary,
		message.Provenance,
	}, message.EvidenceRefs...)
	for _, value := range values {
		if safety.RedactSecrets(value) != value {
			return true
		}
	}
	return false
}

func agentMessagePayloadDigest(message AgentMessage) (string, error) {
	payload, err := json.Marshal(struct {
		Sender           string   `json:"sender"`
		Recipient        string   `json:"recipient"`
		MessageType      string   `json:"messageType"`
		Confidentiality  string   `json:"confidentiality"`
		AuthorityCeiling int      `json:"authorityCeiling"`
		EvidenceRefs     []string `json:"evidenceRefs"`
		PayloadSummary   string   `json:"payloadSummary"`
		Provenance       string   `json:"provenance"`
	}{
		Sender:           message.Sender,
		Recipient:        message.Recipient,
		MessageType:      message.MessageType,
		Confidentiality:  message.Confidentiality,
		AuthorityCeiling: message.AuthorityCeiling,
		EvidenceRefs:     message.EvidenceRefs,
		PayloadSummary:   message.PayloadSummary,
		Provenance:       message.Provenance,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func containsExact(values []string, target string) bool {
	for _, value := range values {
		if normalizeIdentifier(value) == target {
			return true
		}
	}
	return false
}
