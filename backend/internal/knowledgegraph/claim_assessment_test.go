package knowledgegraph

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func assessmentRequest(object, reference, content string, observedAt time.Time) RecordClaimRequest {
	request := claimTestRequest(object)
	request.ObservedAt = observedAt
	request.Provenance = []ClaimProvenance{{
		ReferenceID: reference, ContentDigest: claimTestDigest(content),
		CapturedAt: observedAt.Add(-time.Minute),
	}}
	return request
}

func assessClaim(t *testing.T, service *Service, claimID string, query ClaimAssessmentQuery) ClaimAssessment {
	t.Helper()
	assessment, err := service.AssessClaim(context.Background(), "robert", "hai", claimID, query)
	if err != nil {
		t.Fatalf("AssessClaim: %v", err)
	}
	if assessment.ClaimID != claimID || len(assessment.Reasons) == 0 {
		t.Fatalf("incomplete assessment: %#v", assessment)
	}
	return assessment
}

func TestAssessClaimIsOwnerWorkspaceScoped(t *testing.T) {
	service := claimTestService(NewMemoryRepository())
	claim := recordClaim(t, service, claimTestRequest("scoped"))

	for _, test := range []struct {
		owner, workspace string
	}{
		{owner: "other", workspace: "hai"},
		{owner: "robert", workspace: "other"},
	} {
		_, err := service.AssessClaim(context.Background(), test.owner, test.workspace, claim.ID, ClaimAssessmentQuery{})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("scope %q/%q assessment error = %v, want ErrNotFound", test.owner, test.workspace, err)
		}
	}
	if _, err := service.AssessClaim(context.Background(), "", "hai", claim.ID, ClaimAssessmentQuery{}); err == nil {
		t.Fatal("blank owner was accepted")
	}
	if _, err := service.AssessClaim(context.Background(), "robert", "hai", "", ClaimAssessmentQuery{}); err == nil {
		t.Fatal("blank claim id was accepted")
	}
}

func TestAssessClaimDoesNotTreatAuthorityAsTruth(t *testing.T) {
	service := claimTestService(NewMemoryRepository())
	request := assessmentRequest("claimed", "forum-post", "unverified post", claimTestNow.Add(-time.Hour))
	request.VerificationStatus = VerificationUnverified
	request.Provenance[0].Authority = "official infallible primary source"
	claim := recordClaim(t, service, request)

	assessment := assessClaim(t, service, claim.ID, ClaimAssessmentQuery{})
	if assessment.Status != ClaimAssessmentNeedsReview || len(assessment.SupportingClaimIDs) != 0 {
		t.Fatalf("free-form authority elevated claim: %#v", assessment)
	}

	supportedService := claimTestService(NewMemoryRepository())
	supportedRequest := assessmentRequest("supported", "record-1", "source payload", claimTestNow.Add(-50*time.Minute))
	supported := recordClaim(t, supportedService, supportedRequest)
	assessment = assessClaim(t, supportedService, supported.ID, ClaimAssessmentQuery{})
	if assessment.Status != ClaimAssessmentSupported || !reflect.DeepEqual(assessment.SupportingClaimIDs, []string{supported.ID}) {
		t.Fatalf("source-supported claim assessment = %#v", assessment)
	}
	if len(assessment.EvidenceIDs) != 1 || !strings.HasPrefix(assessment.EvidenceIDs[0], "evidence-") {
		t.Fatalf("deterministic evidence id missing: %#v", assessment.EvidenceIDs)
	}
}

func TestAssessClaimCorroboratesOnlyIndependentReferencesAndContent(t *testing.T) {
	service := claimTestService(NewMemoryRepository())
	first := recordClaim(t, service, assessmentRequest("green", "record-a", "payload-a", claimTestNow.Add(-3*time.Hour)))

	copyRequest := assessmentRequest("green", "record-b", "payload-a", claimTestNow.Add(-2*time.Hour))
	copyClaim := recordClaim(t, service, copyRequest)
	assessment := assessClaim(t, service, first.ID, ClaimAssessmentQuery{})
	if assessment.Status != ClaimAssessmentSupported {
		t.Fatalf("copied content counted as independent corroboration: %#v", assessment)
	}
	if len(assessment.SupportingClaimIDs) != 2 || !containsString(assessment.SupportingClaimIDs, copyClaim.ID) {
		t.Fatalf("same-object support was not reported: %#v", assessment.SupportingClaimIDs)
	}

	independent := recordClaim(t, service, assessmentRequest("green", "record-c", "payload-c", claimTestNow.Add(-time.Hour)))
	assessment = assessClaim(t, service, first.ID, ClaimAssessmentQuery{})
	if assessment.Status != ClaimAssessmentCorroborated || !containsString(assessment.SupportingClaimIDs, independent.ID) {
		t.Fatalf("independent corroboration not detected: %#v", assessment)
	}
	if len(assessment.EvidenceIDs) != 3 {
		t.Fatalf("evidence ids = %#v, want three distinct references", assessment.EvidenceIDs)
	}

	repeated := assessClaim(t, service, first.ID, ClaimAssessmentQuery{})
	if !reflect.DeepEqual(assessment, repeated) {
		t.Fatalf("assessment is not deterministic:\nfirst=%#v\nsecond=%#v", assessment, repeated)
	}
}

func TestAssessClaimDetectsConflictingObjectsWithoutAuthorityRanking(t *testing.T) {
	service := claimTestService(NewMemoryRepository())
	firstRequest := assessmentRequest("approved", "source-a", "approved payload", claimTestNow.Add(-2*time.Hour))
	firstRequest.Provenance[0].Authority = "unknown"
	first := recordClaim(t, service, firstRequest)
	conflictRequest := assessmentRequest("rejected", "source-b", "rejected payload", claimTestNow.Add(-time.Hour))
	conflictRequest.VerificationStatus = VerificationUnverified
	conflictRequest.Provenance[0].Authority = "supreme authority"
	conflict := recordClaim(t, service, conflictRequest)

	assessment := assessClaim(t, service, first.ID, ClaimAssessmentQuery{})
	if assessment.Status != ClaimAssessmentConflicting || !reflect.DeepEqual(assessment.ConflictingClaimIDs, []string{conflict.ID}) {
		t.Fatalf("conflicting object assessment = %#v", assessment)
	}
	if len(assessment.EvidenceIDs) != 2 {
		t.Fatalf("conflict evidence = %#v, want both source records", assessment.EvidenceIDs)
	}
}

func TestAssessClaimRespectsObservedAndEffectiveBoundaries(t *testing.T) {
	service := claimTestService(NewMemoryRepository())
	targetRequest := assessmentRequest("open", "source-old", "old payload", claimTestNow.Add(-3*time.Hour))
	targetRequest.EffectiveFrom = claimTestNow.Add(-4 * time.Hour)
	target := recordClaim(t, service, targetRequest)

	conflictRequest := assessmentRequest("closed", "source-new", "new payload", claimTestNow.Add(-30*time.Minute))
	conflictRequest.EffectiveFrom = claimTestNow.Add(-time.Hour)
	recordClaim(t, service, conflictRequest)

	beforeObservation := claimTestNow.Add(-2 * time.Hour)
	assessment := assessClaim(t, service, target.ID, ClaimAssessmentQuery{ObservedBy: &beforeObservation})
	if assessment.Status != ClaimAssessmentSupported {
		t.Fatalf("later observation leaked into as-of assessment: %#v", assessment)
	}

	beforeEffective := claimTestNow.Add(-2 * time.Hour)
	assessment = assessClaim(t, service, target.ID, ClaimAssessmentQuery{EffectiveAt: &beforeEffective})
	if assessment.Status != ClaimAssessmentSupported {
		t.Fatalf("future-effective conflict leaked into assessment: %#v", assessment)
	}

	assessment = assessClaim(t, service, target.ID, ClaimAssessmentQuery{})
	if assessment.Status != ClaimAssessmentConflicting {
		t.Fatalf("current conflict was not detected: %#v", assessment)
	}

	beforeTarget := claimTestNow.Add(-4 * time.Hour)
	assessment = assessClaim(t, service, target.ID, ClaimAssessmentQuery{ObservedBy: &beforeTarget})
	if assessment.Status != ClaimAssessmentNeedsReview || !strings.Contains(assessment.Reasons[0], "not observable") {
		t.Fatalf("unobserved target assessment = %#v", assessment)
	}
}

func TestAssessClaimRespectsDirectAndTransitiveSupersession(t *testing.T) {
	service := claimTestService(NewMemoryRepository())
	firstRequest := assessmentRequest("draft", "source-1", "draft", claimTestNow.Add(-4*time.Hour))
	firstRequest.EffectiveFrom = claimTestNow.Add(-5 * time.Hour)
	first := recordClaim(t, service, firstRequest)
	middleRequest := assessmentRequest("reviewed", "source-2", "reviewed", claimTestNow.Add(-3*time.Hour))
	middleRequest.SupersedesClaimIDs = []string{first.ID}
	middle := recordClaim(t, service, middleRequest)
	finalRequest := assessmentRequest("final", "source-3", "final", claimTestNow.Add(-time.Hour))
	finalRequest.SupersedesClaimIDs = []string{middle.ID}
	final := recordClaim(t, service, finalRequest)

	assessment := assessClaim(t, service, first.ID, ClaimAssessmentQuery{})
	if assessment.Status != ClaimAssessmentSuperseded ||
		!containsString(assessment.SupersedingClaimIDs, middle.ID) ||
		!containsString(assessment.SupersedingClaimIDs, final.ID) {
		t.Fatalf("transitive supersession assessment = %#v", assessment)
	}

	beforeMiddle := claimTestNow.Add(-3*time.Hour - 30*time.Minute)
	assessment = assessClaim(t, service, first.ID, ClaimAssessmentQuery{ObservedBy: &beforeMiddle, EffectiveAt: &beforeMiddle})
	if assessment.Status != ClaimAssessmentSupported {
		t.Fatalf("later successor leaked into historical assessment: %#v", assessment)
	}
}

func TestAssessClaimFailsClosedWhenScanMayBeTruncated(t *testing.T) {
	service := claimTestService(NewMemoryRepository())
	var target Claim
	for i := 0; i < maximumClaimLimit; i++ {
		request := assessmentRequest(
			fmt.Sprintf("value-%03d", i),
			fmt.Sprintf("source-%03d", i),
			fmt.Sprintf("payload-%03d", i),
			claimTestNow.Add(-time.Duration(i+1)*time.Second),
		)
		claim := recordClaim(t, service, request)
		if i == 0 {
			target = claim
		}
	}

	assessment := assessClaim(t, service, target.ID, ClaimAssessmentQuery{})
	if assessment.Status != ClaimAssessmentNeedsReview || !assessment.Truncated ||
		!strings.Contains(assessment.Reasons[0], "bounded repository limit") {
		t.Fatalf("truncated assessment did not fail closed: %#v", assessment)
	}
}

func TestAssessClaimRejectsInvalidRepositoryData(t *testing.T) {
	base := claimTestService(NewMemoryRepository())
	target := recordClaim(t, base, assessmentRequest("valid", "source", "payload", claimTestNow.Add(-time.Hour)))
	corrupt := target
	corrupt.OwnerIdentity = "other"

	service := &Service{
		claims: assessmentRepository{target: target, claims: []Claim{corrupt}},
		clock:  func() time.Time { return claimTestNow },
	}
	_, err := service.AssessClaim(context.Background(), "robert", "hai", target.ID, ClaimAssessmentQuery{})
	if !errors.Is(err, ErrCorruptStorage) {
		t.Fatalf("invalid repository data error = %v, want ErrCorruptStorage", err)
	}
}

func TestAssessClaimValidatesTimeBounds(t *testing.T) {
	service := claimTestService(NewMemoryRepository())
	claim := recordClaim(t, service, claimTestRequest("time bounds"))
	zero := time.Time{}
	future := claimTestNow.Add(time.Second)
	for _, query := range []ClaimAssessmentQuery{
		{EffectiveAt: &zero},
		{ObservedBy: &zero},
		{ObservedBy: &future},
	} {
		if _, err := service.AssessClaim(context.Background(), "robert", "hai", claim.ID, query); err == nil {
			t.Fatalf("invalid assessment query was accepted: %#v", query)
		}
	}
}

type assessmentRepository struct {
	target Claim
	claims []Claim
	err    error
}

func (r assessmentRepository) AppendClaim(context.Context, Claim) (Claim, error) {
	return Claim{}, errors.New("not implemented")
}

func (r assessmentRepository) GetClaim(_ context.Context, owner, workspace, id string) (Claim, error) {
	if r.err != nil {
		return Claim{}, r.err
	}
	if r.target.OwnerIdentity != owner || r.target.WorkspaceID != workspace || r.target.ID != id {
		return Claim{}, ErrNotFound
	}
	return r.target, nil
}

func (r assessmentRepository) ListClaims(context.Context, string, string, ClaimQuery) ([]Claim, error) {
	if r.err != nil {
		return nil, r.err
	}
	return append([]Claim(nil), r.claims...), nil
}
