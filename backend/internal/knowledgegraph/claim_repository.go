package knowledgegraph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"time"
)

const (
	claimPropertySchema           = "claim_schema"
	claimPropertyWorkspace        = "claim_workspace"
	claimPropertySubject          = "claim_subject"
	claimPropertyPredicate        = "claim_predicate"
	claimPropertyObject           = "claim_object"
	claimPropertyObservedAt       = "claim_observed_at"
	claimPropertyProvenanceDigest = "claim_provenance_digest"
	claimPropertyDigest           = "claim_digest"
	claimPropertySupersedes       = "claim_supersedes"
	claimPropertyConflicts        = "claim_conflicts"
)

// GraphClaimRepository persists immutable claim envelopes through the existing
// graph repository. One claim maps to one node, so creation is atomic even when
// it carries several lifecycle links.
type GraphClaimRepository struct {
	repo Repository
}

func NewGraphClaimRepository(repo Repository) *GraphClaimRepository {
	return &GraphClaimRepository{repo: repo}
}

func (r *GraphClaimRepository) AppendClaim(ctx context.Context, claim Claim) (Claim, error) {
	if r == nil || r.repo == nil {
		return Claim{}, fmt.Errorf("claim repository is unavailable")
	}
	node, err := claimToNode(claim)
	if err != nil {
		return Claim{}, err
	}
	created, err := r.repo.CreateNode(ctx, node)
	if err != nil {
		return Claim{}, err
	}
	return claimFromNode(created)
}

func (r *GraphClaimRepository) GetClaim(ctx context.Context, ownerIdentity, workspaceID, id string) (Claim, error) {
	if r == nil || r.repo == nil {
		return Claim{}, fmt.Errorf("claim repository is unavailable")
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	workspaceID = strings.TrimSpace(workspaceID)
	id = strings.TrimSpace(id)
	if ownerIdentity == "" || workspaceID == "" || id == "" {
		return Claim{}, ErrNotFound
	}
	node, err := r.repo.GetNode(ctx, ownerIdentity, id)
	if err != nil {
		return Claim{}, err
	}
	claim, err := claimFromNode(node)
	if err != nil {
		return Claim{}, err
	}
	if claim.WorkspaceID != workspaceID {
		return Claim{}, ErrNotFound
	}
	return claim, nil
}

func (r *GraphClaimRepository) ListClaims(ctx context.Context, ownerIdentity, workspaceID string, query ClaimQuery) ([]Claim, error) {
	if r == nil || r.repo == nil {
		return nil, fmt.Errorf("claim repository is unavailable")
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	workspaceID = strings.TrimSpace(workspaceID)
	if ownerIdentity == "" || workspaceID == "" {
		return nil, fmt.Errorf("owner identity and workspace id are required")
	}
	limit, err := normalizedClaimLimit(query.Limit)
	if err != nil {
		return nil, err
	}
	statuses, err := claimStatusSet(query.VerificationStatuses)
	if err != nil {
		return nil, err
	}
	nodes, err := r.repo.ListNodes(ctx, ownerIdentity, ListOptions{})
	if err != nil {
		return nil, err
	}
	claims := make([]Claim, 0, min(limit, len(nodes)))
	for _, node := range nodes {
		if !isTemporalClaimNode(node) {
			continue
		}
		claim, err := claimFromNode(node)
		if err != nil {
			return nil, err
		}
		if claim.WorkspaceID != workspaceID || !claimMatchesQuery(claim, query, statuses) {
			continue
		}
		claims = append(claims, claim)
	}
	sort.Slice(claims, func(i, j int) bool {
		if claims[i].ObservedAt.Equal(claims[j].ObservedAt) {
			return claims[i].ID < claims[j].ID
		}
		return claims[i].ObservedAt.After(claims[j].ObservedAt)
	})
	if len(claims) > limit {
		claims = claims[:limit]
	}
	return claims, nil
}

func claimToNode(claim Claim) (Node, error) {
	if err := validateStoredClaim(claim); err != nil {
		return Node{}, err
	}
	supersedes, err := json.Marshal(claim.SupersedesClaimIDs)
	if err != nil {
		return Node{}, err
	}
	conflicts, err := json.Marshal(claim.ConflictsWithIDs)
	if err != nil {
		return Node{}, err
	}
	sources := make([]SourceReference, 0, len(claim.Provenance))
	for _, source := range claim.Provenance {
		sources = append(sources, SourceReference{
			ID:           source.ReferenceID,
			URI:          source.URI,
			SourceNodeID: source.SourceNodeID,
			ContentHash:  source.ContentDigest,
			Authority:    source.Authority,
			CapturedAt:   source.CapturedAt,
			LocalOnly:    source.LocalOnly,
		})
	}
	return Node{
		ID:               claim.ID,
		OwnerIdentity:    claim.OwnerIdentity,
		Kind:             NodeClaim,
		DeduplicationKey: claimSchemaVersion + "|" + claim.WorkspaceID + "|" + claim.ClaimDigest,
		Label:            compact(claim.Subject + " " + claim.Predicate),
		Content:          claim.Object,
		Properties: map[string]string{
			claimPropertySchema:           claimSchemaVersion,
			claimPropertyWorkspace:        claim.WorkspaceID,
			claimPropertySubject:          claim.Subject,
			claimPropertyPredicate:        claim.Predicate,
			claimPropertyObject:           claim.Object,
			claimPropertyObservedAt:       claim.ObservedAt.Format(time.RFC3339Nano),
			claimPropertyProvenanceDigest: claim.ProvenanceDigest,
			claimPropertyDigest:           claim.ClaimDigest,
			claimPropertySupersedes:       string(supersedes),
			claimPropertyConflicts:        string(conflicts),
		},
		Confidence:         1,
		VerificationStatus: claim.VerificationStatus,
		Sources:            sources,
		ValidFrom:          timePointer(claim.EffectiveFrom),
		ValidUntil:         cloneTime(claim.EffectiveUntil),
		Sensitivity:        claim.Sensitivity,
		LocalOnly:          claim.LocalOnly,
		CreatedAt:          claim.CreatedAt,
		UpdatedAt:          claim.CreatedAt,
	}, nil
}

func claimFromNode(node Node) (Claim, error) {
	if !isTemporalClaimNode(node) {
		return Claim{}, ErrNotFound
	}
	required := []string{
		claimPropertyWorkspace, claimPropertySubject, claimPropertyPredicate,
		claimPropertyObject, claimPropertyObservedAt, claimPropertyProvenanceDigest,
		claimPropertyDigest, claimPropertySupersedes, claimPropertyConflicts,
	}
	for _, key := range required {
		if _, ok := node.Properties[key]; !ok {
			return Claim{}, fmt.Errorf("%w: temporal claim property %q is missing", ErrCorruptStorage, key)
		}
	}
	observedAt, err := time.Parse(time.RFC3339Nano, node.Properties[claimPropertyObservedAt])
	if err != nil {
		return Claim{}, fmt.Errorf("%w: invalid claim observed time", ErrCorruptStorage)
	}
	var supersedes []string
	if err := decodeClaimLinks(node.Properties[claimPropertySupersedes], &supersedes); err != nil {
		return Claim{}, fmt.Errorf("%w: invalid supersedes links: %v", ErrCorruptStorage, err)
	}
	var conflicts []string
	if err := decodeClaimLinks(node.Properties[claimPropertyConflicts], &conflicts); err != nil {
		return Claim{}, fmt.Errorf("%w: invalid conflict links: %v", ErrCorruptStorage, err)
	}
	provenance := make([]ClaimProvenance, 0, len(node.Sources))
	for _, source := range node.Sources {
		provenance = append(provenance, ClaimProvenance{
			ReferenceID: source.ID, URI: source.URI, SourceNodeID: source.SourceNodeID,
			ContentDigest: source.ContentHash, Authority: source.Authority,
			CapturedAt: source.CapturedAt, LocalOnly: source.LocalOnly,
		})
	}
	if node.ValidFrom == nil {
		return Claim{}, fmt.Errorf("%w: temporal claim has no effective start", ErrCorruptStorage)
	}
	claim := Claim{
		ID: node.ID, OwnerIdentity: node.OwnerIdentity,
		WorkspaceID:   node.Properties[claimPropertyWorkspace],
		Subject:       node.Properties[claimPropertySubject],
		Predicate:     node.Properties[claimPropertyPredicate],
		Object:        node.Properties[claimPropertyObject],
		EffectiveFrom: *node.ValidFrom, EffectiveUntil: cloneTime(node.ValidUntil),
		ObservedAt: observedAt, VerificationStatus: node.VerificationStatus,
		Provenance:         provenance,
		ProvenanceDigest:   node.Properties[claimPropertyProvenanceDigest],
		SupersedesClaimIDs: supersedes, ConflictsWithIDs: conflicts,
		Sensitivity: node.Sensitivity, LocalOnly: node.LocalOnly,
		ClaimDigest: node.Properties[claimPropertyDigest], CreatedAt: node.CreatedAt,
	}
	if err := validateStoredClaim(claim); err != nil {
		return Claim{}, fmt.Errorf("%w: %v", ErrCorruptStorage, err)
	}
	expectedProvenance, err := deterministicProvenanceDigest(claim.Provenance)
	if err != nil || expectedProvenance != claim.ProvenanceDigest {
		return Claim{}, fmt.Errorf("%w: provenance digest mismatch", ErrCorruptStorage)
	}
	expectedClaim, err := deterministicClaimDigest(claim)
	if err != nil || expectedClaim != claim.ClaimDigest || claim.ID != claimID(expectedClaim) {
		return Claim{}, fmt.Errorf("%w: claim digest mismatch", ErrCorruptStorage)
	}
	if err := validateClaimNodeProjection(node, claim); err != nil {
		return Claim{}, fmt.Errorf("%w: %v", ErrCorruptStorage, err)
	}
	return claim, nil
}

func validateClaimNodeProjection(node Node, claim Claim) error {
	if len(node.Properties) != 10 {
		return fmt.Errorf("temporal claim contains unsigned properties")
	}
	expectedDeduplicationKey := claimSchemaVersion + "|" + claim.WorkspaceID + "|" + claim.ClaimDigest
	if node.DeduplicationKey != expectedDeduplicationKey ||
		node.Label != compact(claim.Subject+" "+claim.Predicate) ||
		node.Content != claim.Object || node.Confidence != 1 {
		return fmt.Errorf("temporal claim projection does not match signed envelope")
	}
	if len(node.ProjectKeys) != 0 || len(node.Tags) != 0 ||
		node.ConflictGroupID != "" || node.SupersedesID != "" || node.CorrectedByID != "" {
		return fmt.Errorf("temporal claim contains unsupported generic graph metadata")
	}
	return nil
}

func decodeClaimLinks(encoded string, target *[]string) error {
	decoder := json.NewDecoder(strings.NewReader(encoded))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if *target == nil {
		return fmt.Errorf("links must be an array")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing content")
		}
		return err
	}
	return nil
}

func isTemporalClaimNode(node Node) bool {
	return node.Kind == NodeClaim && node.Properties[claimPropertySchema] == claimSchemaVersion
}

// sameImmutableClaimEnvelope permits archive state and transaction timestamp
// changes while preventing mutation of signed claim content or provenance.
func sameImmutableClaimEnvelope(current, next Node) bool {
	current.Archived, next.Archived = false, false
	current.UpdatedAt, next.UpdatedAt = time.Time{}, time.Time{}
	current.DeletedAt, next.DeletedAt = nil, nil
	return reflect.DeepEqual(current, next)
}

func claimMatchesQuery(claim Claim, query ClaimQuery, statuses map[VerificationStatus]struct{}) bool {
	if query.ObservedBy != nil && claim.ObservedAt.After(query.ObservedBy.UTC()) {
		return false
	}
	if query.EffectiveAt != nil {
		at := query.EffectiveAt.UTC()
		if claim.EffectiveFrom.After(at) || (claim.EffectiveUntil != nil && !at.Before(*claim.EffectiveUntil)) {
			return false
		}
	}
	if len(statuses) > 0 {
		_, ok := statuses[claim.VerificationStatus]
		return ok
	}
	return true
}
