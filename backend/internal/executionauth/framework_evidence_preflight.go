package executionauth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"automation-hub-backend/internal/sourceevidence"
)

const frameworkEvidencePreflightPassed = "passed"

func (s *Service) verifyFrameworkEvidencePreflight(
	ctx context.Context,
	request Request,
	receipt *Receipt,
) error {
	if request.Governance == nil ||
		request.Governance.FrameworkSelectorAlgorithmVersion != frameworkSelectorV5 {
		return nil
	}

	governance := request.Governance
	digest := governance.FrameworkEvidencePreflightDigest
	if digest == "" || digest != strings.ToLower(strings.TrimSpace(digest)) ||
		!validDigest(digest) {
		return fmt.Errorf("selector-v5 framework evidence preflight digest is missing or invalid")
	}
	if s.preflights == nil {
		return fmt.Errorf("framework evidence preflight resolver is unavailable")
	}

	resolved, err := s.preflights.ResolveFrameworkEvidencePreflight(
		ctx,
		request.OwnerIdentity,
		governance.TaskPlanID,
		governance.FrameworkSelectionID,
		digest,
	)
	if err != nil {
		return fmt.Errorf("resolve framework evidence preflight: %w", err)
	}
	if resolved.OwnerIdentity != request.OwnerIdentity ||
		resolved.TaskPlanID != governance.TaskPlanID ||
		resolved.FrameworkSelectionID != governance.FrameworkSelectionID ||
		resolved.PreflightDigest != digest ||
		resolved.Status != frameworkEvidencePreflightPassed {
		return fmt.Errorf("framework evidence preflight does not match the immutable passed record")
	}
	verifiedSourceClaims, sourceClaimsDigest, err := s.verifySourceEvidenceClaims(
		ctx,
		request.OwnerIdentity,
		resolved.AssertionsJSON,
		monotonicNow(s.now),
	)
	if err != nil {
		return fmt.Errorf("verify source evidence claims: %w", err)
	}

	receipt.Evidence.FrameworkEvidencePreflight =
		FrameworkEvidencePreflightVerificationEvidence{
			Digest:               resolved.PreflightDigest,
			OwnerScoped:          true,
			Verified:             true,
			SourceClaimsVerified: verifiedSourceClaims,
			SourceClaimsDigest:   sourceClaimsDigest,
		}
	return nil
}

type sourceEvidenceAssertion struct {
	RequirementID string                 `json:"requirementId"`
	Validator     string                 `json:"validator"`
	Status        string                 `json:"status"`
	SourceClaims  []sourceevidence.Claim `json:"sourceClaims,omitempty"`
}

func (s *Service) verifySourceEvidenceClaims(
	ctx context.Context,
	ownerIdentity string,
	assertionsJSON json.RawMessage,
	now time.Time,
) (int, string, error) {
	var assertions []sourceEvidenceAssertion
	if len(assertionsJSON) == 0 || json.Unmarshal(assertionsJSON, &assertions) != nil {
		return 0, "", fmt.Errorf("%w: framework evidence assertions are missing or malformed", ErrSourceEvidenceUnverified)
	}
	claims := []sourceevidence.Claim{}
	for _, assertion := range assertions {
		if !isSourceEvidenceValidator(assertion.Validator) || assertion.Status != "verified" {
			continue
		}
		if len(assertion.SourceClaims) == 0 {
			return 0, "", fmt.Errorf("%w: verified source assertion %q has no independently resolvable claim", ErrSourceEvidenceUnverified, assertion.RequirementID)
		}
		for _, claim := range assertion.SourceClaims {
			if claim.RequirementID != assertion.RequirementID || claim.Validator != assertion.Validator {
				return 0, "", fmt.Errorf("%w: source claim does not match assertion %q", ErrSourceEvidenceUnverified, assertion.RequirementID)
			}
			claims = append(claims, claim)
		}
	}
	if len(claims) == 0 {
		return 0, "", nil
	}
	if s.sourceEvidence == nil {
		return 0, "", fmt.Errorf("%w: resolver is unavailable", ErrSourceEvidenceUnverified)
	}
	sort.Slice(claims, func(i, j int) bool {
		if claims[i].RequirementID != claims[j].RequirementID {
			return claims[i].RequirementID < claims[j].RequirementID
		}
		return claims[i].ExtractionID < claims[j].ExtractionID
	})
	for _, claim := range claims {
		snapshot, err := s.sourceEvidence.Resolve(ctx, ownerIdentity, claim.ExtractionID)
		if err != nil {
			return 0, "", fmt.Errorf("%w: %v", ErrSourceEvidenceUnverified, err)
		}
		if err := sourceevidence.VerifyClaim(snapshot, claim, ownerIdentity, now); err != nil {
			return 0, "", fmt.Errorf("%w: %v", ErrSourceEvidenceUnverified, err)
		}
	}
	encoded, err := json.Marshal(claims)
	if err != nil {
		return 0, "", fmt.Errorf("%w: encode verified claims", ErrSourceEvidenceUnverified)
	}
	digest := sha256.Sum256(encoded)
	return len(claims), hex.EncodeToString(digest[:]), nil
}

func isSourceEvidenceValidator(value string) bool {
	switch strings.TrimSpace(value) {
	case sourceevidence.ValidatorPrimarySource, sourceevidence.ValidatorFreshSource, sourceevidence.ValidatorSourceContext:
		return true
	default:
		return false
	}
}
