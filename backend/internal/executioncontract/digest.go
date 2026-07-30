package executioncontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

type canonicalEnvelope struct {
	SchemaVersion        string                `json:"schemaVersion"`
	OwnerID              string                `json:"ownerId"`
	RunID                string                `json:"runId"`
	AttemptID            string                `json:"attemptId"`
	ParentAttemptID      string                `json:"parentAttemptId,omitempty"`
	ParentContractDigest string                `json:"parentContractDigest,omitempty"`
	AttemptNumber        uint32                `json:"attemptNumber"`
	CorrelationID        string                `json:"correlationId"`
	IdempotencyKey       string                `json:"idempotencyKey"`
	TraceID              string                `json:"traceId"`
	CreatedAt            string                `json:"createdAt"`
	Deadline             string                `json:"deadline"`
	Action               ActionScope           `json:"action"`
	Resources            []ResourceScope       `json:"resources,omitempty"`
	PolicyReferences     []PolicyReference     `json:"policyReferences"`
	ApprovalReferences   []ApprovalReference   `json:"approvalReferences,omitempty"`
	AutonomyCeiling      int                   `json:"autonomyCeiling"`
	EvidenceRequirements []EvidenceRequirement `json:"evidenceRequirements"`
	SourceProvenance     []SourceProvenance    `json:"sourceProvenance"`
	RedactedMetadata     map[string]string     `json:"redactedMetadata,omitempty"`
}

// ComputeDigest produces a deterministic SHA-256 digest over the execution
// contract. Set-like slices are normalized so equivalent scope declarations
// have the same digest.
func ComputeDigest(envelope Envelope) (string, error) {
	canonical := canonicalEnvelope{
		SchemaVersion:        strings.TrimSpace(envelope.SchemaVersion),
		OwnerID:              strings.TrimSpace(envelope.OwnerID),
		RunID:                strings.ToLower(strings.TrimSpace(envelope.RunID)),
		AttemptID:            strings.ToLower(strings.TrimSpace(envelope.AttemptID)),
		ParentAttemptID:      strings.ToLower(strings.TrimSpace(envelope.ParentAttemptID)),
		ParentContractDigest: strings.ToLower(strings.TrimSpace(envelope.ParentContractDigest)),
		AttemptNumber:        envelope.AttemptNumber,
		CorrelationID:        strings.ToLower(strings.TrimSpace(envelope.CorrelationID)),
		IdempotencyKey:       strings.TrimSpace(envelope.IdempotencyKey),
		TraceID:              strings.ToLower(strings.TrimSpace(envelope.TraceID)),
		CreatedAt:            envelope.CreatedAt.UTC().Format(timeFormat),
		Deadline:             envelope.Deadline.UTC().Format(timeFormat),
		Action:               canonicalAction(envelope.Action),
		Resources:            canonicalResources(envelope.Resources),
		PolicyReferences:     canonicalPolicyReferences(envelope.PolicyReferences),
		ApprovalReferences:   canonicalApprovalReferences(envelope.ApprovalReferences),
		AutonomyCeiling:      envelope.AutonomyCeiling,
		EvidenceRequirements: canonicalEvidenceRequirements(envelope.EvidenceRequirements),
		SourceProvenance:     canonicalSourceProvenance(envelope.SourceProvenance),
		RedactedMetadata:     canonicalMetadata(envelope.RedactedMetadata),
	}
	raw, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

const timeFormat = "2006-01-02T15:04:05.999999999Z07:00"

func canonicalAction(value ActionScope) ActionScope {
	value.Name = strings.TrimSpace(value.Name)
	value.Purpose = strings.Join(strings.Fields(value.Purpose), " ")
	value.AllowedTools = normalizeStrings(value.AllowedTools)
	value.ProhibitedActions = normalizeStrings(value.ProhibitedActions)
	value.ExpectedSideEffects = normalizeStrings(value.ExpectedSideEffects)
	return value
}

func canonicalResources(values []ResourceScope) []ResourceScope {
	result := append([]ResourceScope(nil), values...)
	for index := range result {
		result[index].Kind = strings.TrimSpace(result[index].Kind)
		result[index].Identifier = strings.TrimSpace(result[index].Identifier)
		result[index].Operations = append(
			[]ResourceOperation(nil),
			result[index].Operations...,
		)
		sort.Slice(result[index].Operations, func(left, right int) bool {
			return result[index].Operations[left] < result[index].Operations[right]
		})
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Kind == result[right].Kind {
			return result[left].Identifier < result[right].Identifier
		}
		return result[left].Kind < result[right].Kind
	})
	return result
}

func canonicalPolicyReferences(values []PolicyReference) []PolicyReference {
	result := append([]PolicyReference(nil), values...)
	for index := range result {
		result[index].ID = strings.TrimSpace(result[index].ID)
		result[index].Version = strings.TrimSpace(result[index].Version)
		result[index].DecisionID = strings.TrimSpace(result[index].DecisionID)
		result[index].DecisionDigest = strings.ToLower(strings.TrimSpace(result[index].DecisionDigest))
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].ID == result[right].ID {
			return result[left].DecisionID < result[right].DecisionID
		}
		return result[left].ID < result[right].ID
	})
	return result
}

func canonicalApprovalReferences(values []ApprovalReference) []ApprovalReference {
	result := append([]ApprovalReference(nil), values...)
	for index := range result {
		result[index].ID = strings.TrimSpace(result[index].ID)
		result[index].GrantedBy = strings.TrimSpace(result[index].GrantedBy)
		result[index].ScopeDigest = strings.ToLower(strings.TrimSpace(result[index].ScopeDigest))
		result[index].GrantedAt = result[index].GrantedAt.UTC()
		result[index].ExpiresAt = result[index].ExpiresAt.UTC()
	}
	sort.Slice(result, func(left, right int) bool {
		return result[left].ID < result[right].ID
	})
	return result
}

func canonicalEvidenceRequirements(values []EvidenceRequirement) []EvidenceRequirement {
	result := append([]EvidenceRequirement(nil), values...)
	for index := range result {
		result[index].ID = strings.TrimSpace(result[index].ID)
		result[index].Description = strings.Join(strings.Fields(result[index].Description), " ")
		result[index].Verifier = strings.TrimSpace(result[index].Verifier)
	}
	sort.Slice(result, func(left, right int) bool {
		return result[left].ID < result[right].ID
	})
	return result
}

func canonicalSourceProvenance(values []SourceProvenance) []SourceProvenance {
	result := append([]SourceProvenance(nil), values...)
	for index := range result {
		result[index].SourceID = strings.TrimSpace(result[index].SourceID)
		result[index].SourceType = strings.TrimSpace(result[index].SourceType)
		result[index].SourceVersion = strings.TrimSpace(result[index].SourceVersion)
		result[index].URI = strings.TrimSpace(result[index].URI)
		result[index].ContentDigest = strings.ToLower(strings.TrimSpace(result[index].ContentDigest))
		result[index].RetrievedAt = result[index].RetrievedAt.UTC()
		result[index].Authority = strings.Join(strings.Fields(result[index].Authority), " ")
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].SourceID == result[right].SourceID {
			return result[left].SourceVersion < result[right].SourceVersion
		}
		return result[left].SourceID < result[right].SourceID
	})
	return result
}

func canonicalMetadata(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return result
}
