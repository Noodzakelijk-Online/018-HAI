package knowledgegraph

import (
	"fmt"
	"strings"
	"time"
)

var validNodeKinds = map[NodeKind]struct{}{
	NodePerson: {}, NodeOrganization: {}, NodeProject: {}, NodeGoal: {},
	NodeTask: {}, NodeEvent: {}, NodeDocument: {}, NodeSource: {},
	NodeClaim: {}, NodePreference: {}, NodeDecision: {}, NodeObligation: {},
	NodeDeadline: {}, NodePlace: {}, NodeAccount: {}, NodeCapability: {},
}

var validRelationships = map[RelationshipKind]struct{}{
	RelationRelatedTo: {}, RelationMemberOf: {}, RelationOwns: {},
	RelationWorksOn: {}, RelationSupports: {}, RelationDependsOn: {},
	RelationParentOf: {}, RelationAssignedTo: {}, RelationCausedBy: {},
	RelationDerivedFrom: {}, RelationEvidencedBy: {}, RelationContradicts: {},
	RelationConfirms: {}, RelationPrefers: {}, RelationDecided: {},
	RelationObligatedTo: {}, RelationDueAt: {}, RelationLocatedAt: {},
	RelationCapableOf: {}, RelationMentions: {}, RelationSupersedes: {},
	RelationCorrectedBy: {},
}

var validVerificationStatuses = map[VerificationStatus]struct{}{
	VerificationUnverified: {}, VerificationSourceSupported: {},
	VerificationSchemaValidated: {}, VerificationTestPassed: {},
	VerificationHumanApproved: {}, VerificationVerified: {},
	VerificationUncertain: {}, VerificationConflicting: {},
	VerificationUnsupported: {}, VerificationNeedsReview: {},
}

var validSensitivities = map[Sensitivity]struct{}{
	SensitivityPublic: {}, SensitivityInternal: {},
	SensitivitySensitive: {}, SensitivityRestricted: {},
}

func validateNode(node Node) error {
	if strings.TrimSpace(node.OwnerIdentity) == "" {
		return fmt.Errorf("owner identity is required")
	}
	if _, ok := validNodeKinds[node.Kind]; !ok {
		return fmt.Errorf("invalid node kind %q", node.Kind)
	}
	if strings.TrimSpace(node.Label) == "" && strings.TrimSpace(node.Content) == "" {
		return fmt.Errorf("node label or content is required")
	}
	if err := validateCommon(node.Confidence, node.VerificationStatus, node.Sensitivity, node.ValidFrom, node.ValidUntil, node.Sources); err != nil {
		return fmt.Errorf("node: %w", err)
	}
	return nil
}

func validateEdge(edge Edge) error {
	if strings.TrimSpace(edge.OwnerIdentity) == "" {
		return fmt.Errorf("owner identity is required")
	}
	if strings.TrimSpace(edge.FromNodeID) == "" || strings.TrimSpace(edge.ToNodeID) == "" {
		return fmt.Errorf("edge endpoints are required")
	}
	if edge.FromNodeID == edge.ToNodeID {
		return fmt.Errorf("self-referential edges are not allowed")
	}
	if _, ok := validRelationships[edge.Relationship]; !ok {
		return fmt.Errorf("invalid relationship %q", edge.Relationship)
	}
	if err := validateCommon(edge.Confidence, edge.VerificationStatus, edge.Sensitivity, edge.ValidFrom, edge.ValidUntil, edge.Sources); err != nil {
		return fmt.Errorf("edge: %w", err)
	}
	return nil
}

func validateCommon(confidence float64, verification VerificationStatus, sensitivity Sensitivity, validFrom, validUntil *time.Time, sources []SourceReference) error {
	if confidence < 0 || confidence > 1 {
		return fmt.Errorf("confidence must be between 0 and 1")
	}
	if _, ok := validVerificationStatuses[verification]; !ok {
		return fmt.Errorf("invalid verification status %q", verification)
	}
	if _, ok := validSensitivities[sensitivity]; !ok {
		return fmt.Errorf("invalid sensitivity %q", sensitivity)
	}

	if validFrom != nil && validUntil != nil && validUntil.Before(*validFrom) {
		return fmt.Errorf("valid until must not precede valid from")
	}

	for i, source := range sources {
		if strings.TrimSpace(source.ID) == "" &&
			strings.TrimSpace(source.URI) == "" &&
			strings.TrimSpace(source.SourceNodeID) == "" {
			return fmt.Errorf("source reference %d requires id, uri, or source node id", i)
		}
	}
	return nil
}
