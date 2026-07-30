package executioncontract

import (
	"sort"
	"strings"
)

// SafeLog returns a projection suitable for structured operational logs. It
// keeps correlation fields while excluding personal and resource-level data.
func SafeLog(envelope Envelope) SafeLogProjection {
	resourceKinds := make([]string, 0, len(envelope.Resources))
	operations := make([]ResourceOperation, 0)
	operationSeen := map[ResourceOperation]struct{}{}
	for _, resource := range envelope.Resources {
		resourceKinds = append(resourceKinds, strings.TrimSpace(resource.Kind))
		for _, operation := range resource.Operations {
			if _, exists := operationSeen[operation]; exists {
				continue
			}
			operationSeen[operation] = struct{}{}
			operations = append(operations, operation)
		}
	}
	resourceKinds = normalizeStrings(resourceKinds)
	sort.Slice(operations, func(left, right int) bool {
		return operations[left] < operations[right]
	})

	policyIDs := make([]string, 0, len(envelope.PolicyReferences))
	for _, reference := range envelope.PolicyReferences {
		policyIDs = append(policyIDs, strings.TrimSpace(reference.ID))
	}
	evidenceIDs := make([]string, 0, len(envelope.EvidenceRequirements))
	for _, requirement := range envelope.EvidenceRequirements {
		evidenceIDs = append(evidenceIDs, strings.TrimSpace(requirement.ID))
	}
	sourceIDs := make([]string, 0, len(envelope.SourceProvenance))
	for _, source := range envelope.SourceProvenance {
		sourceIDs = append(sourceIDs, strings.TrimSpace(source.SourceID))
	}

	metadata := make(map[string]string, len(envelope.RedactedMetadata))
	for key, value := range envelope.RedactedMetadata {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if isSensitiveKey(key) || containsSecretText(value) {
			metadata[key] = RedactedValue
		} else {
			metadata[key] = value
		}
	}
	if len(metadata) == 0 {
		metadata = nil
	}

	return SafeLogProjection{
		SchemaVersion:      envelope.SchemaVersion,
		ContractDigest:     envelope.ContractDigest,
		OwnerRef:           shortHash(envelope.OwnerID),
		RunID:              envelope.RunID,
		AttemptID:          envelope.AttemptID,
		ParentAttemptID:    envelope.ParentAttemptID,
		AttemptNumber:      envelope.AttemptNumber,
		CorrelationID:      envelope.CorrelationID,
		TraceID:            envelope.TraceID,
		Deadline:           envelope.Deadline.UTC(),
		Action:             envelope.Action.Name,
		Mode:               envelope.Action.Mode,
		Risk:               envelope.Action.Risk,
		AutonomyCeiling:    envelope.AutonomyCeiling,
		ResourceKinds:      resourceKinds,
		ResourceOperations: operations,
		PolicyIDs:          normalizeStrings(policyIDs),
		ApprovalCount:      len(envelope.ApprovalReferences),
		EvidenceIDs:        normalizeStrings(evidenceIDs),
		SourceIDs:          normalizeStrings(sourceIDs),
		Metadata:           metadata,
	}
}
