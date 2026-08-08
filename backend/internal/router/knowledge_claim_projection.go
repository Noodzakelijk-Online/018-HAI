package router

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"automation-hub-backend/internal/knowledgegraph"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/verification"
)

const verificationClaimPredicate = "verification claim"

type knowledgeClaimProjector struct {
	service *knowledgegraph.Service
}

func (p knowledgeClaimProjector) ProjectClaims(
	ctx context.Context,
	request verification.AnswerRequest,
	run models.VerificationRun,
	claims []models.VerificationClaim,
	evidence []models.VerificationEvidence,
) ([]string, error) {
	if p.service == nil {
		return nil, fmt.Errorf("knowledge claim service is unavailable")
	}
	owner := strings.TrimSpace(request.OwnerIdentity)
	workspace := strings.TrimSpace(request.ProjectKey)
	if owner == "" || workspace == "" {
		return nil, nil
	}
	observedAt := run.UpdatedAt.UTC()
	if observedAt.IsZero() {
		observedAt = run.CreatedAt.UTC()
	}
	if observedAt.IsZero() {
		return nil, fmt.Errorf("verification run has no durable timestamp")
	}

	ids := make([]string, 0, len(claims))
	for _, claim := range claims {
		status, ok := semanticVerificationStatus(claim.Status)
		if !ok || strings.TrimSpace(claim.SourceRefs) == "" {
			continue
		}
		source, ok := matchingVerificationEvidence(claim.SourceRefs, evidence)
		if !ok {
			continue
		}
		contentDigest := sha256Hex(source.Snippet)
		referenceID := strings.TrimSpace(source.SourceID)
		if referenceID == "" {
			referenceID = "verification-evidence-" + sha256Hex(claim.SourceRefs+"\x00"+source.Snippet)
		}
		created, err := p.service.RecordClaim(ctx, knowledgegraph.RecordClaimRequest{
			OwnerIdentity:      owner,
			WorkspaceID:        workspace,
			Subject:            workspace,
			Predicate:          verificationClaimPredicate,
			Object:             strings.TrimSpace(claim.ClaimText),
			EffectiveFrom:      observedAt,
			ObservedAt:         observedAt,
			VerificationStatus: status,
			Provenance: []knowledgegraph.ClaimProvenance{{
				ReferenceID:   referenceID,
				URI:           strings.TrimSpace(source.SourceURI),
				ContentDigest: contentDigest,
				Authority:     strings.TrimSpace(source.Authority),
				CapturedAt:    observedAt,
				LocalOnly:     true,
			}},
			Sensitivity: knowledgegraph.SensitivityInternal,
			LocalOnly:   true,
		})
		if err != nil {
			return ids, fmt.Errorf("project verification claim: %w", err)
		}
		ids = append(ids, created.ID)
	}
	sort.Strings(ids)
	return ids, nil
}

func semanticVerificationStatus(status string) (knowledgegraph.VerificationStatus, bool) {
	switch status {
	case verification.StatusSourceSupported:
		return knowledgegraph.VerificationSourceSupported, true
	case verification.StatusTestPassed:
		return knowledgegraph.VerificationTestPassed, true
	case verification.StatusHumanApproved:
		return knowledgegraph.VerificationHumanApproved, true
	case verification.StatusVerified:
		return knowledgegraph.VerificationVerified, true
	default:
		return "", false
	}
}

func matchingVerificationEvidence(reference string, evidence []models.VerificationEvidence) (models.VerificationEvidence, bool) {
	reference = strings.TrimSpace(reference)
	for _, candidate := range evidence {
		if candidate.Rejected || !candidate.Used || strings.TrimSpace(candidate.Snippet) == "" {
			continue
		}
		if reference == strings.TrimSpace(candidate.SourceURI) ||
			reference == strings.TrimSpace(candidate.SourceID) ||
			reference == strings.TrimSpace(candidate.SourceLabel) {
			return candidate, true
		}
	}
	return models.VerificationEvidence{}, false
}

func sha256Hex(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

var _ verification.ClaimProjector = knowledgeClaimProjector{}
