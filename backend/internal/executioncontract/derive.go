package executioncontract

import (
	"fmt"
	"strings"
)

// DeriveChildAttempt creates a child that cannot expand the parent's
// authority, deadline, tools, resource operations, or approval boundary.
func DeriveChildAttempt(parent Envelope, child ChildAttempt) (Envelope, error) {
	if err := validateDigest("parent contract digest", parent.ContractDigest); err != nil {
		return Envelope{}, err
	}
	computedParentDigest, err := ComputeDigest(parent)
	if err != nil {
		return Envelope{}, fmt.Errorf("compute parent digest: %w", err)
	}
	if !strings.EqualFold(parent.ContractDigest, computedParentDigest) {
		return Envelope{}, fmt.Errorf("parent contract digest does not match the parent envelope")
	}
	if err := requireUUID("child attempt ID", child.AttemptID); err != nil {
		return Envelope{}, err
	}
	if child.AttemptID == parent.AttemptID {
		return Envelope{}, fmt.Errorf("child attempt ID must differ from parent attempt ID")
	}
	if err := validateIdempotencyKey(child.IdempotencyKey); err != nil {
		return Envelope{}, err
	}
	if child.CreatedAt.IsZero() || child.CreatedAt.Before(parent.CreatedAt) {
		return Envelope{}, fmt.Errorf("child creation time cannot precede the parent")
	}
	deadline := child.Deadline
	if deadline.IsZero() {
		deadline = parent.Deadline
	}
	if deadline.After(parent.Deadline) {
		return Envelope{}, fmt.Errorf("child deadline cannot exceed parent deadline")
	}
	action := cloneAction(parent.Action)
	if child.Action != nil {
		action = cloneAction(*child.Action)
		if err := ensureActionSubset(parent.Action, action); err != nil {
			return Envelope{}, err
		}
	}
	resources := cloneResources(parent.Resources)
	if child.Resources != nil {
		resources = cloneResources(child.Resources)
		if err := ensureResourceSubset(parent.Resources, resources); err != nil {
			return Envelope{}, err
		}
	}
	autonomy := parent.AutonomyCeiling
	if child.AutonomyCeiling != nil {
		if *child.AutonomyCeiling > parent.AutonomyCeiling {
			return Envelope{}, fmt.Errorf("child autonomy ceiling cannot exceed parent ceiling")
		}
		autonomy = *child.AutonomyCeiling
	}
	result := Envelope{
		SchemaVersion:        parent.SchemaVersion,
		OwnerID:              parent.OwnerID,
		RunID:                parent.RunID,
		AttemptID:            child.AttemptID,
		ParentAttemptID:      parent.AttemptID,
		ParentContractDigest: parent.ContractDigest,
		AttemptNumber:        parent.AttemptNumber + 1,
		CorrelationID:        parent.CorrelationID,
		IdempotencyKey:       child.IdempotencyKey,
		TraceID:              parent.TraceID,
		CreatedAt:            child.CreatedAt.UTC(),
		Deadline:             deadline.UTC(),
		Action:               action,
		Resources:            resources,
		PolicyReferences:     append([]PolicyReference(nil), parent.PolicyReferences...),
		ApprovalReferences:   append([]ApprovalReference(nil), parent.ApprovalReferences...),
		AutonomyCeiling:      autonomy,
		EvidenceRequirements: mergeEvidence(
			parent.EvidenceRequirements,
			child.EvidenceRequirements,
		),
		SourceProvenance: mergeProvenance(
			parent.SourceProvenance,
			child.SourceProvenance,
		),
	}
	result.RedactedMetadata, err = mergeMetadata(
		parent.RedactedMetadata,
		child.RedactedMetadata,
	)
	if err != nil {
		return Envelope{}, err
	}
	result.ContractDigest, err = ComputeDigest(result)
	if err != nil {
		return Envelope{}, fmt.Errorf("compute child digest: %w", err)
	}
	return result, nil
}

func ensureActionSubset(parent, child ActionScope) error {
	if child.Mode != parent.Mode {
		if !(parent.Mode == ModeExecute &&
			(child.Mode == ModeDraft || child.Mode == ModeRecommend || child.Mode == ModePlanOnly)) {
			return fmt.Errorf("child execution mode cannot expand or alter parent execution authority")
		}
	}
	if riskRank(child.Risk) > riskRank(parent.Risk) {
		return fmt.Errorf("child risk cannot exceed parent risk")
	}
	if parent.RequiresApproval && !child.RequiresApproval {
		return fmt.Errorf("child cannot remove parent approval requirement")
	}
	if !stringSubset(child.AllowedTools, parent.AllowedTools) {
		return fmt.Errorf("child allowed tools must be a subset of parent tools")
	}
	if !stringSubset(parent.ProhibitedActions, child.ProhibitedActions) {
		return fmt.Errorf("child cannot remove parent prohibited actions")
	}
	if !stringSubset(child.ExpectedSideEffects, parent.ExpectedSideEffects) {
		return fmt.Errorf("child expected side effects must be a subset of parent side effects")
	}
	return nil
}

func ensureResourceSubset(parent, child []ResourceScope) error {
	parentByResource := make(map[string]map[ResourceOperation]struct{}, len(parent))
	for _, resource := range parent {
		key := resourceKey(resource)
		operations := make(map[ResourceOperation]struct{}, len(resource.Operations))
		for _, operation := range resource.Operations {
			operations[operation] = struct{}{}
		}
		parentByResource[key] = operations
	}
	for _, resource := range child {
		allowed, exists := parentByResource[resourceKey(resource)]
		if !exists {
			return fmt.Errorf("child resource %q is outside parent scope", resource.Identifier)
		}
		for _, operation := range resource.Operations {
			if _, exists := allowed[operation]; !exists {
				return fmt.Errorf(
					"child operation %q exceeds parent scope for %q",
					operation,
					resource.Identifier,
				)
			}
		}
	}
	return nil
}

func resourceKey(value ResourceScope) string {
	return strings.TrimSpace(value.Kind) + "\x00" + strings.TrimSpace(value.Identifier)
}

func riskRank(value RiskLevel) int {
	switch value {
	case RiskLow:
		return 1
	case RiskMedium:
		return 2
	case RiskHigh:
		return 3
	case RiskCritical:
		return 4
	default:
		return 100
	}
}

func stringSubset(subset, superset []string) bool {
	allowed := map[string]struct{}{}
	for _, value := range normalizeStrings(superset) {
		allowed[value] = struct{}{}
	}
	for _, value := range normalizeStrings(subset) {
		if _, exists := allowed[value]; !exists {
			return false
		}
	}
	return true
}

func mergeEvidence(parent, child []EvidenceRequirement) []EvidenceRequirement {
	result := append([]EvidenceRequirement(nil), parent...)
	seen := map[string]struct{}{}
	for _, value := range parent {
		seen[strings.TrimSpace(value.ID)] = struct{}{}
	}
	for _, value := range child {
		if _, exists := seen[strings.TrimSpace(value.ID)]; exists {
			continue
		}
		result = append(result, value)
	}
	return result
}

func mergeProvenance(parent, child []SourceProvenance) []SourceProvenance {
	result := append([]SourceProvenance(nil), parent...)
	seen := map[string]struct{}{}
	for _, value := range parent {
		seen[value.SourceID+"\x00"+value.SourceVersion] = struct{}{}
	}
	for _, value := range child {
		key := value.SourceID + "\x00" + value.SourceVersion
		if _, exists := seen[key]; exists {
			continue
		}
		result = append(result, value)
	}
	return result
}

func mergeMetadata(parent, child map[string]string) (map[string]string, error) {
	if len(parent) == 0 && len(child) == 0 {
		return nil, nil
	}
	result := make(map[string]string, len(parent)+len(child))
	for key, value := range parent {
		result[key] = value
	}
	for key, value := range child {
		if inherited, exists := result[key]; exists && inherited != value {
			return nil, fmt.Errorf("child cannot overwrite inherited metadata %q", key)
		}
		result[key] = value
	}
	return result, nil
}

func cloneAction(value ActionScope) ActionScope {
	value.AllowedTools = append([]string(nil), value.AllowedTools...)
	value.ProhibitedActions = append([]string(nil), value.ProhibitedActions...)
	value.ExpectedSideEffects = append([]string(nil), value.ExpectedSideEffects...)
	return value
}

func cloneResources(values []ResourceScope) []ResourceScope {
	result := append([]ResourceScope(nil), values...)
	for index := range result {
		result[index].Operations = append([]ResourceOperation(nil), result[index].Operations...)
	}
	return result
}
