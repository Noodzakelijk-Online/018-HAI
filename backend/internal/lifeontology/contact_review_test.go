package lifeontology

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestContactCandidatePromotionCreatesHumanApprovedCanonicalRecord(t *testing.T) {
	service := NewService(nil, func() time.Time { return fixedNow() })
	candidate := mustContactCandidate(t, service, "Candidate Robert", "contact-a")
	request := DecideContactCandidateRequest{
		OwnerIdentity: "owner-1", CandidateID: candidate.ID, Action: ContactReviewCorrect,
		CanonicalName: "Robert Velhorst", CanonicalSummary: "Confirmed contact identity",
		Reason: "Robert confirmed the corrected contact name", IdempotencyKey: "contact-review-0001",
	}
	result, err := service.DecideContactCandidate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.CanonicalEntity == nil || result.CanonicalEntity.Name != "Robert Velhorst" ||
		result.CanonicalEntity.VerificationStatus != VerificationHumanApproved ||
		result.CanonicalEntity.Confidence != 1 || !result.CanonicalEntity.LocalOnly ||
		result.Decision.CanExecute || result.Decision.GrantsAuthority || !result.Decision.LocalOnly {
		t.Fatalf("contact promotion crossed trust boundary: %#v", result)
	}
	storedCandidate, err := service.GetEntity(context.Background(), "owner-1", candidate.ID)
	if err != nil || storedCandidate.VerificationStatus != VerificationNeedsReview || storedCandidate.Name != "Candidate Robert" {
		t.Fatalf("source candidate was mutated: %#v err=%v", storedCandidate, err)
	}
	replay, err := service.DecideContactCandidate(context.Background(), request)
	if err != nil || !replay.AlreadyExisted || replay.Decision.ID != result.Decision.ID {
		t.Fatalf("idempotent review replay = %#v err=%v", replay, err)
	}
	request.Reason = "Different decision under reused idempotency key"
	if _, err := service.DecideContactCandidate(context.Background(), request); !errors.Is(err, ErrContactReviewConflict) {
		t.Fatalf("conflicting replay error = %v", err)
	}
}

func TestContactCandidateRejectIsAuditedWithoutCanonicalPromotion(t *testing.T) {
	service := NewService(nil, func() time.Time { return fixedNow() })
	candidate := mustContactCandidate(t, service, "Not a person", "contact-reject")
	result, err := service.DecideContactCandidate(context.Background(), DecideContactCandidateRequest{
		OwnerIdentity: "owner-1", CandidateID: candidate.ID, Action: ContactReviewReject,
		Reason: "The extraction is not a real contact", IdempotencyKey: "contact-review-reject",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.CanonicalEntity != nil || result.Decision.CanonicalEntityID != "" || result.Decision.Action != ContactReviewReject {
		t.Fatalf("rejected contact was promoted: %#v", result)
	}
	decisions, err := service.ListContactReviewDecisions(context.Background(), "owner-1", 10)
	if err != nil || len(decisions) != 1 || decisions[0].ID != result.Decision.ID {
		t.Fatalf("contact review history = %#v err=%v", decisions, err)
	}
}

func TestContactMergeRequiresCandidateRecordsAndOneFinalDecision(t *testing.T) {
	service := NewService(nil, func() time.Time { return fixedNow() })
	first := mustContactCandidate(t, service, "Joyce", "contact-merge-a")
	secondRequest := contactCandidateRequest("Joyce", "contact-merge-b")
	secondRequest.ExternalKeys = append([]ExternalKey(nil), first.ExternalKeys...)
	secondRequest.Summary = "Second independent source record"
	secondRequest.Provenance = []Provenance{source("contact-merge-b")}
	second := mustEntity(t, service, secondRequest)
	proposals, err := service.ListMergeProposals(context.Background(), "owner-1", 10)
	if err != nil || len(proposals) != 1 {
		t.Fatalf("merge proposals = %#v err=%v", proposals, err)
	}
	result, err := service.DecideContactMerge(context.Background(), DecideContactMergeRequest{
		OwnerIdentity: "owner-1", ProposalID: proposals[0].ID, Action: ContactReviewMerge,
		CanonicalName: "Joyce", CanonicalSummary: "Confirmed merged contact",
		Reason: "Both source records refer to the same person", IdempotencyKey: "contact-merge-review",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.CanonicalEntity == nil || len(result.Decision.CandidateEntityIDs) != 2 ||
		result.Decision.CandidateEntityIDs[0] != minString(first.ID, second.ID) {
		t.Fatalf("merged contact result = %#v", result)
	}
	_, err = service.DecideContactMerge(context.Background(), DecideContactMergeRequest{
		OwnerIdentity: "owner-1", ProposalID: proposals[0].ID, Action: ContactReviewKeepDistinct,
		Reason: "Attempt a conflicting second decision", IdempotencyKey: "contact-merge-second",
	})
	if !errors.Is(err, ErrContactReviewConflict) {
		t.Fatalf("second final decision error = %v", err)
	}
}

func TestContactReviewRejectsOrdinaryPersonEntity(t *testing.T) {
	service := NewService(nil, func() time.Time { return fixedNow() })
	person := mustEntity(t, service, entityRequest(EntityPerson, "Ordinary person"))
	_, err := service.DecideContactCandidate(context.Background(), DecideContactCandidateRequest{
		OwnerIdentity: "owner-1", CandidateID: person.ID, Action: ContactReviewPromote,
		Reason: "Should not be accepted", IdempotencyKey: "contact-review-unsafe",
	})
	if err == nil {
		t.Fatal("ordinary person entity was accepted as a source contact candidate")
	}
}

func TestContactReviewUsesServerTimeAndRejectsSecretMaterial(t *testing.T) {
	service := NewService(nil, func() time.Time { return fixedNow() })
	candidate := mustContactCandidate(t, service, "Candidate", "contact-server-time")
	result, err := service.DecideContactCandidate(context.Background(), DecideContactCandidateRequest{
		OwnerIdentity: "owner-1", CandidateID: candidate.ID, Action: ContactReviewReject,
		Reason: "The source text is not a real contact", IdempotencyKey: "contact-server-time",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Decision.DecidedAt.Equal(fixedNow()) || !result.Decision.RecordedAt.Equal(fixedNow()) {
		t.Fatalf("decision timestamps are not server authoritative: %#v", result.Decision)
	}
	other := mustContactCandidate(t, service, "Other candidate", "contact-secret")
	_, err = service.DecideContactCandidate(context.Background(), DecideContactCandidateRequest{
		OwnerIdentity: "owner-1", CandidateID: other.ID, Action: ContactReviewReject,
		Reason: "Leaked credential sk-abcdefghijklmnopqrstuvwxyz123456", IdempotencyKey: "contact-secret-review",
	})
	if err == nil {
		t.Fatal("secret material was accepted into immutable contact review history")
	}
}

func contactCandidateRequest(name, key string) RecordEntityRequest {
	request := entityRequest(EntityPerson, name)
	request.Domain = DomainRelationships
	request.Attributes = map[string]string{"candidate": "true", "review_required": "true"}
	request.ExternalKeys = []ExternalKey{{Namespace: "source/contact-candidate", Value: key}}
	request.Confidence = 0.35
	request.VerificationStatus = VerificationNeedsReview
	request.Sensitivity = SensitivitySensitive
	request.LocalOnly = true
	return request
}

func mustContactCandidate(t *testing.T, service *Service, name, key string) Entity {
	t.Helper()
	return mustEntity(t, service, contactCandidateRequest(name, key))
}

func minString(left, right string) string {
	if left < right {
		return left
	}
	return right
}
