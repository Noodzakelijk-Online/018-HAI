package knowledgegraph

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"automation-hub-backend/internal/safety"
)

var bareSHA256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type canonicalProvenance struct {
	ReferenceID   string `json:"referenceId"`
	URI           string `json:"uri"`
	SourceNodeID  string `json:"sourceNodeId"`
	ContentDigest string `json:"contentDigest"`
	Authority     string `json:"authority"`
	CapturedAt    string `json:"capturedAt"`
	LocalOnly     bool   `json:"localOnly"`
}

type canonicalClaim struct {
	SchemaVersion      string   `json:"schemaVersion"`
	OwnerIdentity      string   `json:"ownerIdentity"`
	WorkspaceID        string   `json:"workspaceId"`
	Subject            string   `json:"subject"`
	Predicate          string   `json:"predicate"`
	Object             string   `json:"object"`
	EffectiveFrom      string   `json:"effectiveFrom"`
	EffectiveUntil     string   `json:"effectiveUntil"`
	ObservedAt         string   `json:"observedAt"`
	VerificationStatus string   `json:"verificationStatus"`
	ProvenanceDigest   string   `json:"provenanceDigest"`
	Supersedes         []string `json:"supersedes"`
	Conflicts          []string `json:"conflicts"`
	Sensitivity        string   `json:"sensitivity"`
	LocalOnly          bool     `json:"localOnly"`
}

func normalizeClaimRequest(request RecordClaimRequest, createdAt time.Time) (Claim, error) {
	claim := Claim{
		OwnerIdentity: strings.TrimSpace(request.OwnerIdentity),
		WorkspaceID:   strings.TrimSpace(request.WorkspaceID),
		Subject:       compact(request.Subject), Predicate: compact(request.Predicate),
		Object:             compact(request.Object),
		EffectiveFrom:      request.EffectiveFrom.UTC(),
		EffectiveUntil:     cloneTime(request.EffectiveUntil),
		ObservedAt:         request.ObservedAt.UTC(),
		VerificationStatus: request.VerificationStatus,
		Provenance:         normalizeClaimProvenance(request.Provenance),
		SupersedesClaimIDs: normalizeClaimIDs(request.SupersedesClaimIDs),
		ConflictsWithIDs:   normalizeClaimIDs(request.ConflictsWithIDs),
		Sensitivity:        request.Sensitivity,
		LocalOnly:          request.LocalOnly,
		CreatedAt:          createdAt.UTC(),
	}
	if claim.EffectiveUntil != nil {
		utc := claim.EffectiveUntil.UTC()
		claim.EffectiveUntil = &utc
	}
	if claim.VerificationStatus == "" {
		claim.VerificationStatus = VerificationUnverified
	}
	if claim.Sensitivity == "" {
		claim.Sensitivity = SensitivityInternal
	}
	for _, source := range claim.Provenance {
		claim.LocalOnly = claim.LocalOnly || source.LocalOnly
	}
	if err := validateClaimShape(claim, createdAt.UTC()); err != nil {
		return Claim{}, err
	}
	var err error
	claim.ProvenanceDigest, err = deterministicProvenanceDigest(claim.Provenance)
	if err != nil {
		return Claim{}, err
	}
	claim.ClaimDigest, err = deterministicClaimDigest(claim)
	if err != nil {
		return Claim{}, err
	}
	claim.ID = claimID(claim.ClaimDigest)
	if containsString(claim.SupersedesClaimIDs, claim.ID) || containsString(claim.ConflictsWithIDs, claim.ID) {
		return Claim{}, fmt.Errorf("claim cannot link to itself")
	}
	return claim, nil
}

func validateClaimShape(claim Claim, now time.Time) error {
	if err := requireOwner(claim.OwnerIdentity); err != nil {
		return err
	}
	if strings.TrimSpace(claim.WorkspaceID) == "" {
		return fmt.Errorf("workspace id is required")
	}
	if claim.Subject == "" || claim.Predicate == "" || claim.Object == "" {
		return fmt.Errorf("atomic claim requires subject, predicate, and object")
	}
	for label, value := range map[string]string{
		"subject": claim.Subject, "predicate": claim.Predicate, "object": claim.Object,
	} {
		if safety.RedactSecrets(value) != value {
			return fmt.Errorf("claim %s contains secret material", label)
		}
	}
	if len(claim.Subject) > 512 || len(claim.Predicate) > 256 || len(claim.Object) > 4096 {
		return fmt.Errorf("atomic claim exceeds bounded field length")
	}
	if claim.EffectiveFrom.IsZero() {
		return fmt.Errorf("effective from is required")
	}
	if claim.ObservedAt.IsZero() {
		return fmt.Errorf("observed at is required")
	}
	if claim.ObservedAt.After(now) {
		return fmt.Errorf("observed at cannot be in the future")
	}
	if claim.EffectiveUntil != nil && !claim.EffectiveUntil.After(claim.EffectiveFrom) {
		return fmt.Errorf("effective until must be after effective from")
	}
	if _, ok := validVerificationStatuses[claim.VerificationStatus]; !ok {
		return fmt.Errorf("invalid verification status %q", claim.VerificationStatus)
	}
	if _, ok := validSensitivities[claim.Sensitivity]; !ok {
		return fmt.Errorf("invalid sensitivity %q", claim.Sensitivity)
	}
	if len(claim.Provenance) == 0 || len(claim.Provenance) > 32 {
		return fmt.Errorf("claim requires between 1 and 32 provenance references")
	}
	for i, source := range claim.Provenance {
		if source.ReferenceID == "" && source.URI == "" && source.SourceNodeID == "" {
			return fmt.Errorf("provenance %d requires reference id, uri, or source node id", i)
		}
		if !bareSHA256Pattern.MatchString(source.ContentDigest) {
			return fmt.Errorf("provenance %d content digest must be a lowercase bare SHA-256", i)
		}
		if source.CapturedAt.IsZero() {
			return fmt.Errorf("provenance %d captured at is required", i)
		}
		if source.CapturedAt.After(now) {
			return fmt.Errorf("provenance %d captured at cannot be in the future", i)
		}
		for label, value := range map[string]string{
			"reference id":   source.ReferenceID,
			"source node id": source.SourceNodeID,
			"authority":      source.Authority,
		} {
			if safety.RedactSecrets(value) != value {
				return fmt.Errorf("provenance %d %s contains secret material", i, label)
			}
		}
	}
	seenReferences := make(map[string]string, len(claim.Provenance))
	for _, source := range claim.Provenance {
		key := claimProvenanceReferenceKey(source)
		if previous, exists := seenReferences[key]; exists && previous != source.ContentDigest {
			return fmt.Errorf("provenance reference %q has conflicting content digests", key)
		}
		seenReferences[key] = source.ContentDigest
	}
	if len(claim.SupersedesClaimIDs) > 32 || len(claim.ConflictsWithIDs) > 32 {
		return fmt.Errorf("claim lifecycle links exceed limit of 32")
	}
	for _, id := range append(append([]string(nil), claim.SupersedesClaimIDs...), claim.ConflictsWithIDs...) {
		if !validClaimID(id) {
			return fmt.Errorf("invalid claim link id %q", id)
		}
	}
	for _, id := range claim.SupersedesClaimIDs {
		if containsString(claim.ConflictsWithIDs, id) {
			return fmt.Errorf("claim %q cannot be both superseded and conflicting", id)
		}
	}
	return nil
}

func validateStoredClaim(claim Claim) error {
	if claim.CreatedAt.IsZero() {
		return fmt.Errorf("claim created at is required")
	}
	if err := validateClaimShape(claim, claim.CreatedAt.UTC()); err != nil {
		return err
	}
	if !validClaimID(claim.ID) || !bareSHA256Pattern.MatchString(claim.ProvenanceDigest) || !bareSHA256Pattern.MatchString(claim.ClaimDigest) {
		return fmt.Errorf("claim identifiers and digests must be canonical SHA-256 values")
	}
	if !reflect.DeepEqual(claim.Provenance, normalizeClaimProvenance(claim.Provenance)) ||
		!reflect.DeepEqual(claim.SupersedesClaimIDs, normalizeClaimIDs(claim.SupersedesClaimIDs)) ||
		!reflect.DeepEqual(claim.ConflictsWithIDs, normalizeClaimIDs(claim.ConflictsWithIDs)) {
		return fmt.Errorf("claim envelope is not in canonical order")
	}
	return nil
}

func deterministicProvenanceDigest(provenance []ClaimProvenance) (string, error) {
	canonical := make([]canonicalProvenance, 0, len(provenance))
	for _, source := range normalizeClaimProvenance(provenance) {
		canonical = append(canonical, canonicalProvenance{
			ReferenceID: source.ReferenceID, URI: source.URI,
			SourceNodeID: source.SourceNodeID, ContentDigest: source.ContentDigest,
			Authority: source.Authority, CapturedAt: source.CapturedAt.UTC().Format(time.RFC3339Nano),
			LocalOnly: source.LocalOnly,
		})
	}
	return hashCanonical(canonical)
}

func deterministicClaimDigest(claim Claim) (string, error) {
	effectiveUntil := ""
	if claim.EffectiveUntil != nil {
		effectiveUntil = claim.EffectiveUntil.UTC().Format(time.RFC3339Nano)
	}
	return hashCanonical(canonicalClaim{
		SchemaVersion: claimSchemaVersion,
		OwnerIdentity: strings.TrimSpace(claim.OwnerIdentity),
		WorkspaceID:   strings.TrimSpace(claim.WorkspaceID),
		Subject:       compact(claim.Subject), Predicate: compact(claim.Predicate), Object: compact(claim.Object),
		EffectiveFrom:      claim.EffectiveFrom.UTC().Format(time.RFC3339Nano),
		EffectiveUntil:     effectiveUntil,
		ObservedAt:         claim.ObservedAt.UTC().Format(time.RFC3339Nano),
		VerificationStatus: string(claim.VerificationStatus),
		ProvenanceDigest:   claim.ProvenanceDigest,
		Supersedes:         normalizeClaimIDs(claim.SupersedesClaimIDs),
		Conflicts:          normalizeClaimIDs(claim.ConflictsWithIDs),
		Sensitivity:        string(claim.Sensitivity), LocalOnly: claim.LocalOnly,
	})
}

func hashCanonical(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func normalizeClaimProvenance(values []ClaimProvenance) []ClaimProvenance {
	result := append([]ClaimProvenance(nil), values...)
	for i := range result {
		result[i].ReferenceID = compact(result[i].ReferenceID)
		result[i].URI = strings.TrimSpace(safety.RedactURL(result[i].URI))
		result[i].SourceNodeID = compact(result[i].SourceNodeID)
		result[i].ContentDigest = strings.TrimSpace(result[i].ContentDigest)
		result[i].Authority = compact(result[i].Authority)
		result[i].CapturedAt = result[i].CapturedAt.UTC()
	}
	byIdentity := make(map[string]ClaimProvenance, len(result))
	for _, source := range result {
		byIdentity[claimProvenanceIdentity(source)] = source
	}
	result = result[:0]
	for _, source := range byIdentity {
		result = append(result, source)
	}
	sort.Slice(result, func(i, j int) bool { return claimProvenanceIdentity(result[i]) < claimProvenanceIdentity(result[j]) })
	return result
}

func claimProvenanceIdentity(source ClaimProvenance) string {
	return strings.Join([]string{
		source.ReferenceID, source.URI, source.SourceNodeID, source.ContentDigest,
		source.Authority, source.CapturedAt.Format(time.RFC3339Nano), fmt.Sprint(source.LocalOnly),
	}, "|")
}

func claimProvenanceReferenceKey(source ClaimProvenance) string {
	if source.ReferenceID != "" {
		return "id:" + source.ReferenceID
	}
	if source.URI != "" {
		return "uri:" + source.URI
	}
	return "node:" + source.SourceNodeID
}

func normalizeClaimIDs(values []string) []string {
	set := make(map[string]struct{})
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func normalizedClaimLimit(limit int) (int, error) {
	if limit == 0 {
		return defaultClaimLimit, nil
	}
	if limit < 1 || limit > maximumClaimLimit {
		return 0, fmt.Errorf("claim limit must be between 1 and %d", maximumClaimLimit)
	}
	return limit, nil
}

func claimStatusSet(statuses []VerificationStatus) (map[VerificationStatus]struct{}, error) {
	result := make(map[VerificationStatus]struct{}, len(statuses))
	for _, status := range statuses {
		if _, ok := validVerificationStatuses[status]; !ok {
			return nil, fmt.Errorf("invalid verification status %q", status)
		}
		result[status] = struct{}{}
	}
	return result, nil
}

func claimID(digest string) string { return "claim-" + digest }

func validClaimID(id string) bool {
	return strings.HasPrefix(id, "claim-") && bareSHA256Pattern.MatchString(strings.TrimPrefix(id, "claim-"))
}

func containsString(values []string, wanted string) bool {
	i := sort.SearchStrings(values, wanted)
	return i < len(values) && values[i] == wanted
}
