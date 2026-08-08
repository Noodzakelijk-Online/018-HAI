package lifeledger

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"automation-hub-backend/internal/lifeontology"
	"automation-hub-backend/internal/safety"
)

func (s *Service) projectCommitment(ctx context.Context, record *CommitmentRevision) {
	if s == nil || s.projector == nil || record == nil {
		return
	}
	links := make([]lifeontology.OperationalLinkRequest, 0, 1)
	if record.ProjectKey != "" {
		links = append(links, lifeontology.OperationalLinkRequest{
			Type: lifeontology.EntityProject, RecordID: record.ProjectKey,
			Name: "Project " + record.ProjectKey, Relation: lifeontology.RelationBelongsToProject,
			Status: lifeontology.StatusActive,
		})
	}
	result, err := s.projector.ProjectOperationalRecord(ctx, lifeontology.OperationalProjectionRequest{
		OwnerIdentity: record.OwnerIdentity, Type: lifeontology.EntityCommitment,
		RecordID: commitmentProjectionID(record.CommitmentKey, record.Revision), Domain: record.Domain,
		Name: record.Title, Summary: record.Summary, Status: commitmentLifecycle(record.Status),
		Priority: commitmentPriority(record.Status), DueAt: record.DueAt, ObservedAt: record.ObservedAt,
		Confidence: verificationConfidence(record.Verification), VerificationStatus: ontologyVerification(record.Verification),
		Attributes: map[string]string{
			"record_kind": "commitment_revision", "commitment_key": record.CommitmentKey,
			"revision": strconv.FormatUint(record.Revision, 10), "status": string(record.Status),
			"counterparty": record.Counterparty, "project_key": record.ProjectKey,
		},
		Provenance:  ontologyProvenance(record.Evidence, record.RecordDigest),
		Sensitivity: lifeontology.SensitivitySensitive, LocalOnly: true, Links: links,
	})
	if err != nil {
		record.LifeGraphWarning = projectionWarning(err)
		return
	}
	if !result.AdvisoryOnly || result.CanExecute || result.GrantsAuthority {
		record.LifeGraphWarning = "whole-life graph projection crossed its advisory-only authority boundary"
		return
	}
	record.LifeGraph = &result
}

func (s *Service) projectCost(ctx context.Context, record *CostEntry) {
	if s == nil || s.projector == nil || record == nil {
		return
	}
	links := make([]lifeontology.OperationalLinkRequest, 0, 2)
	if record.CommitmentKey != "" {
		commitment, err := s.repository.GetCommitment(ctx, record.OwnerIdentity, record.CommitmentKey)
		if err != nil {
			record.LifeGraphWarning = projectionWarning(fmt.Errorf("resolve linked commitment: %w", err))
			return
		}
		links = append(links, lifeontology.OperationalLinkRequest{
			Type:     lifeontology.EntityCommitment,
			RecordID: commitmentProjectionID(commitment.CommitmentKey, commitment.Revision),
			Name:     commitment.Title, Summary: commitment.Summary,
			Relation: lifeontology.RelationIncursCost, Direction: "related_to_primary",
			Status: commitmentLifecycle(commitment.Status), Priority: commitmentPriority(commitment.Status),
			VerificationStatus: ontologyVerification(commitment.Verification),
			Confidence:         verificationConfidence(commitment.Verification),
			Sensitivity:        lifeontology.SensitivitySensitive, LocalOnly: true,
		})
	}
	if record.ProjectKey != "" {
		links = append(links, lifeontology.OperationalLinkRequest{
			Type: lifeontology.EntityProject, RecordID: record.ProjectKey,
			Name: "Project " + record.ProjectKey, Relation: lifeontology.RelationBelongsToProject,
			Status: lifeontology.StatusActive,
		})
	}
	result, err := s.projector.ProjectOperationalRecord(ctx, lifeontology.OperationalProjectionRequest{
		OwnerIdentity: record.OwnerIdentity, Type: lifeontology.EntityCost,
		RecordID: record.ID.String(), Domain: record.Domain, Name: record.Title, Summary: record.Summary,
		Status: costLifecycle(record.Kind), Priority: costPriority(record.Kind), ObservedAt: record.ObservedAt,
		Confidence: verificationConfidence(record.Verification), VerificationStatus: ontologyVerification(record.Verification),
		Attributes: map[string]string{
			"record_kind": "cost_event", "cost_kind": string(record.Kind),
			"amount_minor": strconv.FormatInt(record.AmountMinor, 10), "currency": record.Currency,
			"commitment_key": record.CommitmentKey, "project_key": record.ProjectKey,
			"incurred": strconv.FormatBool(record.Kind == CostIncurred || record.Kind == CostPaid),
			"paid":     strconv.FormatBool(record.Kind == CostPaid),
		},
		Provenance:  ontologyProvenance(record.Evidence, record.RecordDigest),
		Sensitivity: lifeontology.SensitivityRestricted, LocalOnly: true, Links: links,
	})
	if err != nil {
		record.LifeGraphWarning = projectionWarning(err)
		return
	}
	if !result.AdvisoryOnly || result.CanExecute || result.GrantsAuthority {
		record.LifeGraphWarning = "whole-life graph projection crossed its advisory-only authority boundary"
		return
	}
	record.LifeGraph = &result
}

func commitmentProjectionID(key string, revision uint64) string {
	return fmt.Sprintf("%s/revision/%d", key, revision)
}

func ontologyProvenance(evidence []EvidenceReference, fallbackDigest string) []lifeontology.Provenance {
	result := make([]lifeontology.Provenance, 0, len(evidence))
	for _, reference := range evidence {
		digest := reference.ContentDigest
		if digest == "" {
			digest = fallbackDigest
		}
		result = append(result, lifeontology.Provenance{
			ReferenceID: reference.ID, URI: reference.URI, ContentDigest: digest,
			Authority: reference.Authority, CapturedAt: reference.ObservedAt, LocalOnly: true,
		})
	}
	return result
}

func ontologyVerification(value VerificationStatus) lifeontology.VerificationStatus {
	switch value {
	case VerificationVerified:
		return lifeontology.VerificationVerified
	case VerificationHumanConfirmed:
		return lifeontology.VerificationHumanApproved
	case VerificationSourceSupported:
		return lifeontology.VerificationSourceSupported
	case VerificationDisputed:
		return lifeontology.VerificationConflicting
	default:
		return lifeontology.VerificationNeedsReview
	}
}

func verificationConfidence(value VerificationStatus) float64 {
	switch value {
	case VerificationVerified:
		return 1
	case VerificationHumanConfirmed:
		return .95
	case VerificationSourceSupported:
		return .85
	case VerificationDisputed:
		return .35
	default:
		return .5
	}
}

func commitmentLifecycle(value CommitmentStatus) lifeontology.LifecycleStatus {
	switch value {
	case CommitmentActive:
		return lifeontology.StatusActive
	case CommitmentWaiting, CommitmentProposed, CommitmentDisputed:
		return lifeontology.StatusWaiting
	case CommitmentFulfilled:
		return lifeontology.StatusCompleted
	case CommitmentCancelled:
		return lifeontology.StatusArchived
	default:
		return lifeontology.StatusOpen
	}
}

func costLifecycle(value CostKind) lifeontology.LifecycleStatus {
	if value == CostEstimate {
		return lifeontology.StatusOpen
	}
	return lifeontology.StatusCompleted
}

func commitmentPriority(value CommitmentStatus) int {
	switch value {
	case CommitmentBreached:
		return 100
	case CommitmentDisputed:
		return 95
	case CommitmentWaiting:
		return 80
	case CommitmentActive:
		return 70
	default:
		return 40
	}
}

func costPriority(value CostKind) int {
	if value == CostEstimate {
		return 45
	}
	return 70
}

func projectionWarning(err error) string {
	message := strings.Join(strings.Fields(safety.RedactSecrets(err.Error())), " ")
	if len([]rune(message)) > 500 {
		message = string([]rune(message)[:500])
	}
	return message
}
