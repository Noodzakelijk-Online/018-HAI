package knowledgegraph

import (
	"context"
	"time"
)

const (
	claimSchemaVersion = "temporal-claim.v1"
	defaultClaimLimit  = 50
	maximumClaimLimit  = 100
)

// ClaimProvenance is an immutable pointer to source material. ContentDigest is
// the lowercase, bare SHA-256 digest of the source content, not of this record.
type ClaimProvenance struct {
	ReferenceID   string    `json:"referenceId,omitempty"`
	URI           string    `json:"uri,omitempty"`
	SourceNodeID  string    `json:"sourceNodeId,omitempty"`
	ContentDigest string    `json:"contentDigest"`
	Authority     string    `json:"authority,omitempty"`
	CapturedAt    time.Time `json:"capturedAt"`
	LocalOnly     bool      `json:"localOnly"`
}

// Claim is one atomic subject-predicate-object assertion observed at a known
// time and effective over a bounded interval. Claim records are append-only.
// They describe knowledge only and never grant approval or execution authority.
type Claim struct {
	ID                 string             `json:"id"`
	OwnerIdentity      string             `json:"ownerIdentity"`
	WorkspaceID        string             `json:"workspaceId"`
	Subject            string             `json:"subject"`
	Predicate          string             `json:"predicate"`
	Object             string             `json:"object"`
	EffectiveFrom      time.Time          `json:"effectiveFrom"`
	EffectiveUntil     *time.Time         `json:"effectiveUntil,omitempty"`
	ObservedAt         time.Time          `json:"observedAt"`
	VerificationStatus VerificationStatus `json:"verificationStatus"`
	Provenance         []ClaimProvenance  `json:"provenance"`
	ProvenanceDigest   string             `json:"provenanceDigest"`
	SupersedesClaimIDs []string           `json:"supersedesClaimIds,omitempty"`
	ConflictsWithIDs   []string           `json:"conflictsWithIds,omitempty"`
	Sensitivity        Sensitivity        `json:"sensitivity"`
	LocalOnly          bool               `json:"localOnly"`
	ClaimDigest        string             `json:"claimDigest"`
	CreatedAt          time.Time          `json:"createdAt"`
}

type RecordClaimRequest struct {
	OwnerIdentity      string
	WorkspaceID        string
	Subject            string
	Predicate          string
	Object             string
	EffectiveFrom      time.Time
	EffectiveUntil     *time.Time
	ObservedAt         time.Time
	VerificationStatus VerificationStatus
	Provenance         []ClaimProvenance
	SupersedesClaimIDs []string
	ConflictsWithIDs   []string
	Sensitivity        Sensitivity
	LocalOnly          bool
}

// CorrectClaimRequest is an authenticated human correction command. The
// service derives identity, verification status, provenance, and observation
// time; callers cannot use this shape to self-assert semantic verification.
type CorrectClaimRequest struct {
	RequestID       string
	CorrectedObject string
	Reason          string
	EffectiveFrom   *time.Time
}

// ClaimQuery is deliberately bounded. EffectiveAt uses a half-open interval:
// effectiveFrom <= t < effectiveUntil. ObservedBy is an as-of knowledge bound.
type ClaimQuery struct {
	EffectiveAt          *time.Time
	ObservedBy           *time.Time
	VerificationStatuses []VerificationStatus
	Limit                int
}

type ClaimLifecycle struct {
	Claim        Claim   `json:"claim"`
	Supersedes   []Claim `json:"supersedes"`
	SupersededBy []Claim `json:"supersededBy"`
	Conflicts    []Claim `json:"conflicts"`
	Truncated    bool    `json:"truncated"`
}

// ClaimAssessmentStatus describes deterministic evidence relationships for one
// atomic claim. It is advisory knowledge state, not execution authority.
type ClaimAssessmentStatus string

const (
	ClaimAssessmentSupported    ClaimAssessmentStatus = "supported"
	ClaimAssessmentCorroborated ClaimAssessmentStatus = "corroborated"
	ClaimAssessmentConflicting  ClaimAssessmentStatus = "conflicting"
	ClaimAssessmentSuperseded   ClaimAssessmentStatus = "superseded"
	ClaimAssessmentNeedsReview  ClaimAssessmentStatus = "needs_review"
)

// ClaimAssessmentQuery evaluates what was knowable by ObservedBy and what was
// true for the requested effective time. Nil values default to the service
// clock. Assessment always uses the repository's maximum bounded read.
type ClaimAssessmentQuery struct {
	EffectiveAt *time.Time
	ObservedBy  *time.Time
}

// ClaimAssessment is a deterministic comparison of claims with the same
// subject and predicate. Evidence IDs derive only from immutable provenance
// references and content digests; free-form authority labels are excluded.
type ClaimAssessment struct {
	ClaimID             string                `json:"claimId"`
	Subject             string                `json:"subject"`
	Predicate           string                `json:"predicate"`
	Object              string                `json:"object"`
	Status              ClaimAssessmentStatus `json:"status"`
	EffectiveAt         time.Time             `json:"effectiveAt"`
	ObservedBy          time.Time             `json:"observedBy"`
	Reasons             []string              `json:"reasons"`
	EvidenceIDs         []string              `json:"evidenceIds"`
	SupportingClaimIDs  []string              `json:"supportingClaimIds"`
	ConflictingClaimIDs []string              `json:"conflictingClaimIds"`
	SupersedingClaimIDs []string              `json:"supersedingClaimIds"`
	Truncated           bool                  `json:"truncated"`
}

type ClaimReviewItem struct {
	Claim      Claim           `json:"claim"`
	Assessment ClaimAssessment `json:"assessment"`
}

// ClaimReviewQueue is a bounded, point-in-time read model for operator review.
// Counts are derived from deterministic assessments, not model confidence.
type ClaimReviewQueue struct {
	Items       []ClaimReviewItem             `json:"items"`
	Counts      map[ClaimAssessmentStatus]int `json:"counts"`
	EffectiveAt time.Time                     `json:"effectiveAt"`
	ObservedBy  time.Time                     `json:"observedBy"`
	Truncated   bool                          `json:"truncated"`
}

// ClaimRepository exposes append-only, owner/workspace-scoped claim storage.
// It intentionally has no update or execution methods.
type ClaimRepository interface {
	AppendClaim(context.Context, Claim) (Claim, error)
	GetClaim(context.Context, string, string, string) (Claim, error)
	ListClaims(context.Context, string, string, ClaimQuery) ([]Claim, error)
}
