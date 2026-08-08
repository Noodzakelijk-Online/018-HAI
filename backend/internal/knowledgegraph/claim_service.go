package knowledgegraph

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"automation-hub-backend/internal/safety"
)

var correctionRequestIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

func (s *Service) RecordClaim(ctx context.Context, request RecordClaimRequest) (Claim, error) {
	if s == nil || s.claims == nil || s.repo == nil || s.clock == nil {
		return Claim{}, fmt.Errorf("claim service is unavailable")
	}
	now := s.clock().UTC()
	claim, err := normalizeClaimRequest(request, now)
	if err != nil {
		return Claim{}, err
	}
	if err := s.validateClaimSources(ctx, claim); err != nil {
		return Claim{}, err
	}
	if err := s.validateClaimLinks(ctx, claim); err != nil {
		return Claim{}, err
	}
	created, err := s.claims.AppendClaim(ctx, claim)
	if errors.Is(err, ErrExists) {
		existing, getErr := s.claims.GetClaim(ctx, claim.OwnerIdentity, claim.WorkspaceID, claim.ID)
		if getErr == nil && existing.ClaimDigest == claim.ClaimDigest {
			return existing, nil
		}
	}
	return created, err
}

// CorrectClaim preserves the original claim and appends a human-approved
// successor. The authenticated route owns this command; verification status,
// observation time, and local provenance are derived by the service.
func (s *Service) CorrectClaim(
	ctx context.Context,
	ownerIdentity, workspaceID, claimID string,
	request CorrectClaimRequest,
) (Claim, error) {
	if s == nil || s.claims == nil || s.repo == nil || s.clock == nil {
		return Claim{}, fmt.Errorf("claim service is unavailable")
	}
	if err := requireClaimScope(ownerIdentity, workspaceID); err != nil {
		return Claim{}, err
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	workspaceID = strings.TrimSpace(workspaceID)
	claimID = strings.TrimSpace(claimID)
	target, err := s.GetClaim(ctx, ownerIdentity, workspaceID, claimID)
	if err != nil {
		return Claim{}, err
	}
	requestID := strings.TrimSpace(request.RequestID)
	if !correctionRequestIDPattern.MatchString(requestID) {
		return Claim{}, fmt.Errorf("correction request id must be 1-128 safe identifier characters")
	}
	object := compact(request.CorrectedObject)
	reason := strings.TrimSpace(request.Reason)
	if object == "" || object == target.Object {
		return Claim{}, fmt.Errorf("correction must provide a changed object")
	}
	if reason == "" || len(reason) > 2048 {
		return Claim{}, fmt.Errorf("correction reason is required and limited to 2048 characters")
	}
	if safety.RedactSecrets(reason) != reason {
		return Claim{}, fmt.Errorf("correction reason contains secret material")
	}

	referenceID := correctionReferenceID(target.ID, requestID)
	claims, err := s.claims.ListClaims(ctx, ownerIdentity, workspaceID, ClaimQuery{Limit: maximumClaimLimit})
	if err != nil {
		return Claim{}, err
	}
	for _, existing := range claims {
		if !claimHasProvenanceReference(existing, referenceID) {
			continue
		}
		if existing.Object == object && containsString(existing.SupersedesClaimIDs, target.ID) {
			return existing, nil
		}
		return Claim{}, fmt.Errorf("correction request id has already been used with different content")
	}
	if len(claims) == maximumClaimLimit {
		return Claim{}, fmt.Errorf("correction idempotency scan reached its safe bound; review or archive claim history")
	}

	source, err := s.correctionSource(ctx, target, requestID, object, reason)
	if err != nil {
		return Claim{}, err
	}
	effectiveFrom := source.CreatedAt.UTC()
	if request.EffectiveFrom != nil {
		effectiveFrom = request.EffectiveFrom.UTC()
		if effectiveFrom.IsZero() || effectiveFrom.After(source.CreatedAt.UTC()) {
			return Claim{}, fmt.Errorf("correction effective time must be valid and no later than confirmation")
		}
	}
	digest, err := hashCanonical(struct {
		TargetDigest string `json:"targetDigest"`
		Object       string `json:"object"`
		Reason       string `json:"reason"`
		RequestID    string `json:"requestId"`
		Owner        string `json:"owner"`
	}{target.ClaimDigest, object, reason, requestID, ownerIdentity})
	if err != nil {
		return Claim{}, err
	}
	return s.RecordClaim(ctx, RecordClaimRequest{
		OwnerIdentity: ownerIdentity, WorkspaceID: workspaceID,
		Subject: target.Subject, Predicate: target.Predicate, Object: object,
		EffectiveFrom: effectiveFrom, ObservedAt: source.CreatedAt.UTC(),
		VerificationStatus: VerificationHumanApproved,
		Provenance: []ClaimProvenance{{
			ReferenceID:  referenceID,
			URI:          "local://knowledge/claims/" + target.ID + "/correction",
			SourceNodeID: source.ID, ContentDigest: digest,
			Authority:  "authenticated_owner_confirmation",
			CapturedAt: source.CreatedAt.UTC(), LocalOnly: true,
		}},
		SupersedesClaimIDs: []string{target.ID},
		Sensitivity:        target.Sensitivity, LocalOnly: true,
	})
}

func (s *Service) correctionSource(
	ctx context.Context,
	target Claim,
	requestID, object, reason string,
) (Node, error) {
	deduplicationKey := strings.Join([]string{
		"claim-correction", target.WorkspaceID, target.ID, requestID,
	}, "|")
	nodes, err := s.repo.ListNodes(ctx, target.OwnerIdentity, ListOptions{})
	if err != nil {
		return Node{}, err
	}
	for _, candidate := range nodes {
		if candidate.DeduplicationKey != deduplicationKey {
			continue
		}
		if candidate.Kind != NodeSource || candidate.Content != reason ||
			candidate.Properties["targetClaimId"] != target.ID ||
			candidate.Properties["correctedObject"] != object || candidate.DeletedAt != nil {
			return Node{}, fmt.Errorf("correction request id has already been used with different audit content")
		}
		return candidate, nil
	}
	result, err := s.CreateNode(ctx, CreateNodeRequest{
		OwnerIdentity: target.OwnerIdentity,
		Kind:          NodeSource, DeduplicationKey: deduplicationKey,
		Label:   "Authenticated claim correction",
		Content: reason,
		Properties: map[string]string{
			"workspaceId":      target.WorkspaceID,
			"targetClaimId":    target.ID,
			"correctedObject":  object,
			"requestId":        requestID,
			"confirmationType": "authenticated_owner",
		},
		ProjectKeys: []string{target.WorkspaceID},
		Confidence:  1, VerificationStatus: VerificationHumanApproved,
		Sensitivity: target.Sensitivity, LocalOnly: true,
	})
	if err != nil {
		return Node{}, err
	}
	return result.Node, nil
}

func correctionReferenceID(claimID, requestID string) string {
	return "claim-correction:" + claimID + ":" + requestID
}

func claimHasProvenanceReference(claim Claim, referenceID string) bool {
	for _, source := range claim.Provenance {
		if source.ReferenceID == referenceID {
			return true
		}
	}
	return false
}

func (s *Service) GetClaim(ctx context.Context, ownerIdentity, workspaceID, id string) (Claim, error) {
	if s == nil || s.claims == nil {
		return Claim{}, fmt.Errorf("claim service is unavailable")
	}
	if err := requireClaimScope(ownerIdentity, workspaceID); err != nil {
		return Claim{}, err
	}
	return s.claims.GetClaim(ctx, strings.TrimSpace(ownerIdentity), strings.TrimSpace(workspaceID), strings.TrimSpace(id))
}

func (s *Service) ListClaims(ctx context.Context, ownerIdentity, workspaceID string, query ClaimQuery) ([]Claim, error) {
	if s == nil || s.claims == nil {
		return nil, fmt.Errorf("claim service is unavailable")
	}
	if err := requireClaimScope(ownerIdentity, workspaceID); err != nil {
		return nil, err
	}
	return s.claims.ListClaims(ctx, strings.TrimSpace(ownerIdentity), strings.TrimSpace(workspaceID), query)
}

func (s *Service) GetClaimLifecycle(ctx context.Context, ownerIdentity, workspaceID, id string) (ClaimLifecycle, error) {
	target, err := s.GetClaim(ctx, ownerIdentity, workspaceID, id)
	if err != nil {
		return ClaimLifecycle{}, err
	}
	claims, err := s.ListClaims(ctx, ownerIdentity, workspaceID, ClaimQuery{Limit: maximumClaimLimit})
	if err != nil {
		return ClaimLifecycle{}, err
	}
	lifecycle := ClaimLifecycle{Claim: target, Truncated: len(claims) == maximumClaimLimit}
	for _, predecessorID := range target.SupersedesClaimIDs {
		predecessor, err := s.GetClaim(ctx, ownerIdentity, workspaceID, predecessorID)
		if err != nil {
			return ClaimLifecycle{}, err
		}
		lifecycle.Supersedes = append(lifecycle.Supersedes, predecessor)
	}
	for _, conflictID := range target.ConflictsWithIDs {
		conflict, err := s.GetClaim(ctx, ownerIdentity, workspaceID, conflictID)
		if err != nil {
			return ClaimLifecycle{}, err
		}
		lifecycle.Conflicts = append(lifecycle.Conflicts, conflict)
	}
	for _, candidate := range claims {
		if containsString(candidate.SupersedesClaimIDs, target.ID) {
			lifecycle.SupersededBy = append(lifecycle.SupersededBy, candidate)
		}
		if containsString(candidate.ConflictsWithIDs, target.ID) && !claimSliceContains(lifecycle.Conflicts, candidate.ID) {
			lifecycle.Conflicts = append(lifecycle.Conflicts, candidate)
		}
	}
	sortClaims(lifecycle.Supersedes)
	sortClaims(lifecycle.SupersededBy)
	sortClaims(lifecycle.Conflicts)
	return lifecycle, nil
}

func (s *Service) validateClaimSources(ctx context.Context, claim Claim) error {
	for _, source := range claim.Provenance {
		if source.SourceNodeID == "" {
			continue
		}
		node, err := s.repo.GetNode(ctx, claim.OwnerIdentity, source.SourceNodeID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return fmt.Errorf("source node %q is not available to owner", source.SourceNodeID)
			}
			return err
		}
		if node.Kind != NodeSource || node.DeletedAt != nil {
			return fmt.Errorf("source node %q must be an active source node", source.SourceNodeID)
		}
	}
	return nil
}

func (s *Service) validateClaimLinks(ctx context.Context, claim Claim) error {
	for _, id := range append(append([]string(nil), claim.SupersedesClaimIDs...), claim.ConflictsWithIDs...) {
		referenced, err := s.claims.GetClaim(ctx, claim.OwnerIdentity, claim.WorkspaceID, id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return fmt.Errorf("claim link %q is not available in owner workspace", id)
			}
			return err
		}
		if referenced.ObservedAt.After(claim.ObservedAt) {
			return fmt.Errorf("claim link %q was observed after the new claim", id)
		}
	}
	return nil
}

func requireClaimScope(ownerIdentity, workspaceID string) error {
	if err := requireOwner(ownerIdentity); err != nil {
		return err
	}
	if strings.TrimSpace(workspaceID) == "" {
		return fmt.Errorf("workspace id is required")
	}
	return nil
}

func claimSliceContains(claims []Claim, id string) bool {
	for _, claim := range claims {
		if claim.ID == id {
			return true
		}
	}
	return false
}

func sortClaims(claims []Claim) {
	sort.Slice(claims, func(i, j int) bool { return claims[i].ID < claims[j].ID })
}
