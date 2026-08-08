package verification

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"automation-hub-backend/internal/models"
)

type capturingClaimProjector struct {
	called   int
	request  AnswerRequest
	claims   []models.VerificationClaim
	evidence []models.VerificationEvidence
	ids      []string
	err      error
}

func (p *capturingClaimProjector) ProjectClaims(
	_ context.Context,
	request AnswerRequest,
	_ models.VerificationRun,
	claims []models.VerificationClaim,
	evidence []models.VerificationEvidence,
) ([]string, error) {
	p.called++
	p.request = request
	p.claims = append([]models.VerificationClaim(nil), claims...)
	p.evidence = append([]models.VerificationEvidence(nil), evidence...)
	return append([]string(nil), p.ids...), p.err
}

func TestVerificationProjectsGroundedClaimsAndReturnsIdentifiers(t *testing.T) {
	resolver := EvidenceAuthorityResolverFunc(func(AnswerRequest, EvidenceInput) EvidenceAuthorityResolution {
		return EvidenceAuthorityResolution{Trusted: true, Authority: "verified_registry"}
	})
	base := NewServiceWithAuthorityResolver(&fakeVerificationRepository{}, nil, nil, resolver)
	projector := &capturingClaimProjector{ids: []string{"claim-1"}}
	service, err := WithClaimProjector(base, projector)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Answer(AnswerRequest{
		OwnerIdentity: "alice", ProjectKey: "hai", Question: "What is ready?",
		DraftAnswer: "The evidence boundary is ready.", Mode: ModeGrounded,
		ExternalEvidence: []EvidenceInput{{
			SourceType: "test_result", SourceID: "test-42",
			Snippet: "The evidence boundary is ready and passed.",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if projector.called != 1 || projector.request.ProjectKey != "hai" || len(projector.claims) != 1 || len(projector.evidence) != 1 {
		t.Fatalf("projector input was incomplete: %#v", projector)
	}
	if !reflect.DeepEqual(result.KnowledgeClaimIDs, []string{"claim-1"}) || result.KnowledgeError != "" {
		t.Fatalf("projection result = %#v", result)
	}
}

func TestVerificationProjectionFailureIsExplicitAndRedacted(t *testing.T) {
	base := NewService(&fakeVerificationRepository{}, nil, nil)
	projector := &capturingClaimProjector{err: errors.New("postgres password=hunter2")}
	service, err := WithClaimProjector(base, projector)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Answer(AnswerRequest{OwnerIdentity: "alice", ProjectKey: "hai", Question: "Draft", DraftAnswer: "Draft", Mode: ModeDraft})
	if err != nil {
		t.Fatal(err)
	}
	if result.KnowledgeError != "semantic claim projection failed" || projector.called != 1 {
		t.Fatalf("projection failure was not surfaced: %#v", result)
	}
	payload, _ := json.Marshal(result)
	if strings.Contains(string(payload), "hunter2") || strings.Contains(string(payload), "postgres") {
		t.Fatal("projection internals leaked into result")
	}
}

func TestWithClaimProjectorRejectsInvalidComposition(t *testing.T) {
	if _, err := WithClaimProjector(nil, &capturingClaimProjector{}); err == nil {
		t.Fatal("nil verification service was accepted")
	}
	if _, err := WithClaimProjector(NewService(&fakeVerificationRepository{}, nil, nil), nil); err == nil {
		t.Fatal("nil projector was accepted")
	}
}
