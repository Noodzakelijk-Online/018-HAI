package router

import (
	"context"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/knowledgegraph"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/verification"

	"github.com/google/uuid"
)

func TestKnowledgeClaimProjectorPersistsOnlyExactSourceBackedProjectClaims(t *testing.T) {
	now := time.Date(2026, time.August, 4, 12, 0, 0, 0, time.UTC)
	service := knowledgegraph.NewService(knowledgegraph.NewMemoryRepository(), func() time.Time { return now })
	projector := knowledgeClaimProjector{service: service}
	run := models.VerificationRun{ID: uuid.New(), CreatedAt: now.Add(-time.Minute), UpdatedAt: now}
	request := verification.AnswerRequest{OwnerIdentity: "robert", ProjectKey: "hai"}
	claims := []models.VerificationClaim{
		{ClaimText: "The runtime safety gate passed.", Status: verification.StatusTestPassed, SourceRefs: "test-42"},
		{ClaimText: "Unsupported draft.", Status: verification.StatusUnsupported, SourceRefs: "test-42"},
		{ClaimText: "Missing source.", Status: verification.StatusSourceSupported, SourceRefs: "missing"},
	}
	evidence := []models.VerificationEvidence{{
		SourceType: "test_result", SourceID: "test-42", SourceURI: "file:///test-42.json",
		Snippet: "The runtime safety gate passed.", Authority: "trusted_external:test_runner", Used: true,
	}}

	ids, err := projector.ProjectClaims(context.Background(), request, run, claims, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 {
		t.Fatalf("projected ids = %#v", ids)
	}
	stored, err := service.GetClaim(context.Background(), "robert", "hai", ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if stored.Object != claims[0].ClaimText || stored.Predicate != verificationClaimPredicate ||
		stored.VerificationStatus != knowledgegraph.VerificationTestPassed || !stored.LocalOnly ||
		len(stored.Provenance) != 1 || !stored.Provenance[0].LocalOnly ||
		len(stored.Provenance[0].ContentDigest) != 64 {
		t.Fatalf("stored semantic claim = %#v", stored)
	}
	if strings.Contains(stored.ClaimDigest, stored.Provenance[0].Authority) {
		t.Fatal("claim identity unexpectedly exposes authority text")
	}

	replayed, err := projector.ProjectClaims(context.Background(), request, run, claims, evidence)
	if err != nil || len(replayed) != 1 || replayed[0] != ids[0] {
		t.Fatalf("idempotent projection = %#v err=%v", replayed, err)
	}
}

func TestKnowledgeClaimProjectorSkipsUnscopedOrRejectedEvidence(t *testing.T) {
	now := time.Date(2026, time.August, 4, 12, 0, 0, 0, time.UTC)
	service := knowledgegraph.NewService(knowledgegraph.NewMemoryRepository(), func() time.Time { return now })
	projector := knowledgeClaimProjector{service: service}
	run := models.VerificationRun{CreatedAt: now, UpdatedAt: now}
	claim := []models.VerificationClaim{{ClaimText: "Claim", Status: verification.StatusSourceSupported, SourceRefs: "record"}}
	rejected := []models.VerificationEvidence{{SourceID: "record", Snippet: "Claim", Rejected: true}}

	ids, err := projector.ProjectClaims(context.Background(), verification.AnswerRequest{OwnerIdentity: "robert"}, run, claim, rejected)
	if err != nil || len(ids) != 0 {
		t.Fatalf("unscoped projection = %#v err=%v", ids, err)
	}
	ids, err = projector.ProjectClaims(context.Background(), verification.AnswerRequest{OwnerIdentity: "robert", ProjectKey: "hai"}, run, claim, rejected)
	if err != nil || len(ids) != 0 {
		t.Fatalf("rejected evidence projection = %#v err=%v", ids, err)
	}
}
