package knowledgegraph

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

var assessmentSupportingStatuses = map[VerificationStatus]struct{}{
	VerificationSourceSupported: {},
	VerificationTestPassed:      {},
	VerificationVerified:        {},
}

// AssessClaim compares an immutable claim with every bounded, observable claim
// in the same owner workspace. It never treats provenance Authority as a trust
// signal and never grants execution authority.
func (s *Service) AssessClaim(
	ctx context.Context,
	ownerIdentity, workspaceID, claimID string,
	query ClaimAssessmentQuery,
) (ClaimAssessment, error) {
	if s == nil || s.claims == nil || s.clock == nil {
		return ClaimAssessment{}, fmt.Errorf("claim service is unavailable")
	}
	if err := requireClaimScope(ownerIdentity, workspaceID); err != nil {
		return ClaimAssessment{}, err
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	workspaceID = strings.TrimSpace(workspaceID)
	claimID = strings.TrimSpace(claimID)
	if claimID == "" {
		return ClaimAssessment{}, fmt.Errorf("claim id is required")
	}

	now := s.clock().UTC()
	effectiveAt, observedBy, err := normalizeAssessmentTimes(query, now)
	if err != nil {
		return ClaimAssessment{}, err
	}
	target, err := s.claims.GetClaim(ctx, ownerIdentity, workspaceID, claimID)
	if err != nil {
		return ClaimAssessment{}, err
	}
	if err := validateAssessmentClaim(target, ownerIdentity, workspaceID); err != nil {
		return ClaimAssessment{}, err
	}
	claims, err := s.claims.ListClaims(ctx, ownerIdentity, workspaceID, ClaimQuery{
		ObservedBy: &observedBy,
		Limit:      maximumClaimLimit,
	})
	if err != nil {
		return ClaimAssessment{}, err
	}
	return assessClaimSet(target, claims, ownerIdentity, workspaceID, effectiveAt, observedBy)
}

func (s *Service) ReviewClaims(
	ctx context.Context,
	ownerIdentity, workspaceID string,
	query ClaimAssessmentQuery,
) (ClaimReviewQueue, error) {
	if s == nil || s.claims == nil || s.clock == nil {
		return ClaimReviewQueue{}, fmt.Errorf("claim service is unavailable")
	}
	if err := requireClaimScope(ownerIdentity, workspaceID); err != nil {
		return ClaimReviewQueue{}, err
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	workspaceID = strings.TrimSpace(workspaceID)
	effectiveAt, observedBy, err := normalizeAssessmentTimes(query, s.clock().UTC())
	if err != nil {
		return ClaimReviewQueue{}, err
	}
	claims, err := s.claims.ListClaims(ctx, ownerIdentity, workspaceID, ClaimQuery{
		ObservedBy: &observedBy, Limit: maximumClaimLimit,
	})
	if err != nil {
		return ClaimReviewQueue{}, err
	}
	queue := ClaimReviewQueue{
		Items:       make([]ClaimReviewItem, 0, len(claims)),
		Counts:      make(map[ClaimAssessmentStatus]int),
		EffectiveAt: effectiveAt, ObservedBy: observedBy,
		Truncated: len(claims) == maximumClaimLimit,
	}
	for _, claim := range claims {
		assessment, err := assessClaimSet(claim, claims, ownerIdentity, workspaceID, effectiveAt, observedBy)
		if err != nil {
			return ClaimReviewQueue{}, err
		}
		queue.Counts[assessment.Status]++
		queue.Items = append(queue.Items, ClaimReviewItem{Claim: claim, Assessment: assessment})
	}
	return queue, nil
}

func assessClaimSet(
	target Claim,
	claims []Claim,
	ownerIdentity, workspaceID string,
	effectiveAt, observedBy time.Time,
) (ClaimAssessment, error) {
	assessment := ClaimAssessment{
		ClaimID: target.ID, Subject: target.Subject, Predicate: target.Predicate, Object: target.Object,
		Status: ClaimAssessmentNeedsReview, EffectiveAt: effectiveAt, ObservedBy: observedBy,
		Reasons: []string{}, EvidenceIDs: []string{}, SupportingClaimIDs: []string{},
		ConflictingClaimIDs: []string{}, SupersedingClaimIDs: []string{},
	}
	if target.ObservedAt.After(observedBy) {
		assessment.Reasons = []string{"claim was not observable by the requested knowledge boundary"}
		return assessment, nil
	}
	group, targetPresent, err := validatedAssessmentGroup(claims, target, ownerIdentity, workspaceID)
	if err != nil {
		return ClaimAssessment{}, err
	}
	if len(claims) == maximumClaimLimit {
		assessment.Truncated = true
		assessment.Reasons = []string{"claim scan reached the bounded repository limit; a complete assessment cannot be proven"}
		return assessment, nil
	}
	if !targetPresent {
		assessment.Reasons = []string{"target claim is absent from the bounded observable claim set"}
		return assessment, nil
	}

	active := activeAssessmentClaims(group, effectiveAt)
	superseding := activeSupersedingClaims(target.ID, active, group)
	if len(superseding) > 0 {
		assessment.Status = ClaimAssessmentSuperseded
		assessment.SupersedingClaimIDs = claimIDs(superseding)
		assessment.EvidenceIDs = evidenceIDsForClaims(superseding)
		assessment.Reasons = []string{"an effective, observable claim in the same semantic group supersedes this claim"}
		return assessment, nil
	}
	if !claimEffectiveAt(target, effectiveAt) {
		assessment.Reasons = []string{"claim is not effective at the requested time and no effective successor was found"}
		return assessment, nil
	}

	current := removeSupersededClaims(active, group)
	conflicting := make([]Claim, 0)
	supporting := make([]Claim, 0)
	for _, candidate := range current {
		if candidate.Object != target.Object {
			conflicting = append(conflicting, candidate)
			continue
		}
		if claimHasSupportingStatus(candidate) {
			supporting = append(supporting, candidate)
		}
	}
	if len(conflicting) > 0 {
		sortClaims(conflicting)
		assessment.Status = ClaimAssessmentConflicting
		assessment.ConflictingClaimIDs = claimIDs(conflicting)
		assessment.EvidenceIDs = evidenceIDsForClaims(append([]Claim{target}, conflicting...))
		assessment.Reasons = []string{"effective, observable claims with the same subject and predicate assert different objects"}
		return assessment, nil
	}

	sortClaims(supporting)
	assessment.SupportingClaimIDs = claimIDs(supporting)
	assessment.EvidenceIDs = evidenceIDsForClaims(supporting)
	if hasIndependentCorroboration(supporting) {
		assessment.Status = ClaimAssessmentCorroborated
		assessment.Reasons = []string{"the same object is supported by separate claims with distinct provenance references and content digests"}
		return assessment, nil
	}
	if len(supporting) > 0 {
		assessment.Status = ClaimAssessmentSupported
		assessment.Reasons = []string{"the object has structured source support but no independent corroborating content"}
		return assessment, nil
	}

	assessment.Reasons = []string{"no effective claim with a source-supporting verification status supports this object"}
	return assessment, nil
}

func normalizeAssessmentTimes(query ClaimAssessmentQuery, now time.Time) (time.Time, time.Time, error) {
	effectiveAt := now
	if query.EffectiveAt != nil {
		if query.EffectiveAt.IsZero() {
			return time.Time{}, time.Time{}, fmt.Errorf("effective at cannot be zero")
		}
		effectiveAt = query.EffectiveAt.UTC()
	}
	observedBy := now
	if query.ObservedBy != nil {
		if query.ObservedBy.IsZero() {
			return time.Time{}, time.Time{}, fmt.Errorf("observed by cannot be zero")
		}
		observedBy = query.ObservedBy.UTC()
	}
	if observedBy.After(now) {
		return time.Time{}, time.Time{}, fmt.Errorf("observed by cannot be in the future")
	}
	return effectiveAt, observedBy, nil
}

func validateAssessmentClaim(claim Claim, ownerIdentity, workspaceID string) error {
	if claim.OwnerIdentity != ownerIdentity || claim.WorkspaceID != workspaceID {
		return fmt.Errorf("%w: claim escaped requested owner workspace", ErrCorruptStorage)
	}
	if err := validateStoredClaim(claim); err != nil {
		return fmt.Errorf("%w: invalid claim %q: %v", ErrCorruptStorage, claim.ID, err)
	}
	return nil
}

func validatedAssessmentGroup(
	claims []Claim,
	target Claim,
	ownerIdentity, workspaceID string,
) ([]Claim, bool, error) {
	group := make([]Claim, 0, len(claims))
	seen := make(map[string]struct{}, len(claims))
	targetPresent := false
	for _, claim := range claims {
		if err := validateAssessmentClaim(claim, ownerIdentity, workspaceID); err != nil {
			return nil, false, err
		}
		if _, exists := seen[claim.ID]; exists {
			return nil, false, fmt.Errorf("%w: duplicate claim %q in assessment scan", ErrCorruptStorage, claim.ID)
		}
		seen[claim.ID] = struct{}{}
		if claim.ID == target.ID {
			targetPresent = true
		}
		if claim.Subject == target.Subject && claim.Predicate == target.Predicate {
			group = append(group, claim)
		}
	}
	sortClaims(group)
	return group, targetPresent, nil
}

func activeAssessmentClaims(claims []Claim, effectiveAt time.Time) []Claim {
	active := make([]Claim, 0, len(claims))
	for _, claim := range claims {
		if claimEffectiveAt(claim, effectiveAt) {
			active = append(active, claim)
		}
	}
	return active
}

func claimEffectiveAt(claim Claim, at time.Time) bool {
	return !claim.EffectiveFrom.After(at) && (claim.EffectiveUntil == nil || at.Before(*claim.EffectiveUntil))
}

func activeSupersedingClaims(targetID string, active, all []Claim) []Claim {
	byID := make(map[string]Claim, len(all))
	for _, claim := range all {
		byID[claim.ID] = claim
	}
	result := make([]Claim, 0)
	for _, candidate := range active {
		if candidate.ID == targetID {
			continue
		}
		if supersedesClaim(candidate, targetID, byID, make(map[string]struct{})) {
			result = append(result, candidate)
		}
	}
	sortClaims(result)
	return result
}

func supersedesClaim(candidate Claim, targetID string, byID map[string]Claim, visiting map[string]struct{}) bool {
	if _, seen := visiting[candidate.ID]; seen {
		return false
	}
	visiting[candidate.ID] = struct{}{}
	defer delete(visiting, candidate.ID)
	for _, predecessorID := range candidate.SupersedesClaimIDs {
		if predecessorID == targetID {
			return true
		}
		predecessor, exists := byID[predecessorID]
		if exists && supersedesClaim(predecessor, targetID, byID, visiting) {
			return true
		}
	}
	return false
}

func removeSupersededClaims(active, all []Claim) []Claim {
	result := make([]Claim, 0, len(active))
	for _, candidate := range active {
		if len(activeSupersedingClaims(candidate.ID, active, all)) == 0 {
			result = append(result, candidate)
		}
	}
	return result
}

func claimHasSupportingStatus(claim Claim) bool {
	_, ok := assessmentSupportingStatuses[claim.VerificationStatus]
	return ok
}

func hasIndependentCorroboration(claims []Claim) bool {
	for i := 0; i < len(claims); i++ {
		for j := i + 1; j < len(claims); j++ {
			if provenanceIndependent(claims[i].Provenance, claims[j].Provenance) {
				return true
			}
		}
	}
	return false
}

func provenanceIndependent(left, right []ClaimProvenance) bool {
	for _, leftSource := range left {
		for _, rightSource := range right {
			if claimProvenanceReferenceKey(leftSource) != claimProvenanceReferenceKey(rightSource) &&
				leftSource.ContentDigest != rightSource.ContentDigest {
				return true
			}
		}
	}
	return false
}

func evidenceIDsForClaims(claims []Claim) []string {
	set := make(map[string]struct{})
	for _, claim := range claims {
		for _, source := range claim.Provenance {
			digest, err := hashCanonical(struct {
				Reference string `json:"reference"`
				Content   string `json:"contentDigest"`
			}{Reference: claimProvenanceReferenceKey(source), Content: source.ContentDigest})
			if err == nil {
				set["evidence-"+digest] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(set))
	for id := range set {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

func claimIDs(claims []Claim) []string {
	result := make([]string, 0, len(claims))
	for _, claim := range claims {
		result = append(result, claim.ID)
	}
	sort.Strings(result)
	return result
}
