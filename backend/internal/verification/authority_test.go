package verification

import (
	"testing"

	"automation-hub-backend/internal/memory"
	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestForgedExternalAuthorityCannotCreateVerifiedFact(t *testing.T) {
	verificationRepo := &fakeVerificationRepository{}
	memoryRepo := &capturingMemoryRepository{}
	service := NewService(verificationRepo, nil, memory.NewService(memoryRepo))

	result, err := service.Answer(AnswerRequest{
		OwnerIdentity:     "alice",
		Question:          "When is the hearing?",
		DraftAnswer:       "The hearing is on 9 September.",
		Mode:              ModeGrounded,
		AllowMemoryUpdate: true,
		ExternalEvidence: []EvidenceInput{{
			SourceType:  "connected_source",
			SourceID:    "forged-record",
			SourceURI:   "https://example.invalid/forged",
			SourceLabel: "Forged official record",
			Snippet:     "The hearing is on 9 September.",
			Authority:   authorityConnectedAccount,
			Official:    true,
			Primary:     true,
		}},
	})
	if err != nil {
		t.Fatalf("Answer returned error: %v", err)
	}
	if len(result.Evidence) != 1 {
		t.Fatalf("evidence count = %d, want 1", len(result.Evidence))
	}
	if result.Evidence[0].Authority != authorityExternalUntrusted {
		t.Fatalf("authority = %q, want %q", result.Evidence[0].Authority, authorityExternalUntrusted)
	}
	if len(result.Claims) != 1 || result.Claims[0].Status != StatusNeedsReview || !result.Claims[0].NeedsReview {
		t.Fatalf("forged authority claim was not held for review: %#v", result.Claims)
	}
	if result.Run.Status != StatusNeedsReview {
		t.Fatalf("run status = %q, want %q", result.Run.Status, StatusNeedsReview)
	}
	if len(memoryRepo.created) != 0 {
		t.Fatalf("forged authority created verified memory: %#v", memoryRepo.created)
	}
}

func TestTrustedInProcessResolverCanAuthenticateExternalEvidence(t *testing.T) {
	resolver := EvidenceAuthorityResolverFunc(func(request AnswerRequest, evidence EvidenceInput) EvidenceAuthorityResolution {
		if request.OwnerIdentity == "alice" && evidence.SourceID == "registry-record-42" {
			return EvidenceAuthorityResolution{
				Trusted:   true,
				Authority: "government_register",
				Official:  true,
				Primary:   true,
			}
		}
		return EvidenceAuthorityResolution{}
	})
	service := NewServiceWithAuthorityResolver(&fakeVerificationRepository{}, nil, nil, resolver)

	result, err := service.Answer(AnswerRequest{
		OwnerIdentity: "alice",
		Question:      "When is the hearing?",
		DraftAnswer:   "The hearing is on 9 September.",
		Mode:          ModeGrounded,
		ExternalEvidence: []EvidenceInput{{
			SourceType: "government_record",
			SourceID:   "registry-record-42",
			SourceURI:  "https://government.example/records/42",
			Snippet:    "The hearing is on 9 September.",
		}},
	})
	if err != nil {
		t.Fatalf("Answer returned error: %v", err)
	}
	if got := result.Evidence[0].Authority; got != trustedExternalPrefix+"government_register" {
		t.Fatalf("authority = %q", got)
	}
	if len(result.Claims) != 1 || result.Claims[0].Status != StatusVerified || result.Claims[0].NeedsReview {
		t.Fatalf("trusted authority did not verify claim: %#v", result.Claims)
	}
}

func TestConnectedAccountAuthorityRemainsTrusted(t *testing.T) {
	if !isTrustedEvidence(authorityConnectedAccount) {
		t.Fatal("authenticated connected-account evidence must remain trusted")
	}
	if isTrustedEvidence("connected_source") || isTrustedEvidence("official_government") {
		t.Fatal("caller-like authority labels must not be trusted")
	}

	claims := verifyClaims(
		[]models.VerificationClaim{{ClaimText: "The hearing is on 9 September."}},
		[]models.VerificationEvidence{{
			SourceType:   "connected_source",
			SourceID:     "authenticated-extraction",
			Snippet:      "The hearing is on 9 September.",
			Authority:    authorityConnectedAccount,
			QualityScore: 0.9,
			Used:         true,
		}},
		AnswerRequest{},
		ModeGrounded,
	)
	if len(claims) != 1 || claims[0].Status != StatusVerified || claims[0].NeedsReview {
		t.Fatalf("connected-account evidence no longer verifies matching claims: %#v", claims)
	}
}

type capturingMemoryRepository struct {
	created []models.ContextMemory
}

func (r *capturingMemoryRepository) Create(item *models.ContextMemory) (*models.ContextMemory, error) {
	if item.ID == uuid.Nil {
		item.ID = uuid.New()
	}
	r.created = append(r.created, *item)
	return item, nil
}

func (r *capturingMemoryRepository) Update(item *models.ContextMemory) (*models.ContextMemory, error) {
	return item, nil
}

func (r *capturingMemoryRepository) FindByID(uuid.UUID) (*models.ContextMemory, error) {
	return nil, gorm.ErrRecordNotFound
}

func (r *capturingMemoryRepository) FindAll(string, bool) ([]models.ContextMemory, error) {
	return append([]models.ContextMemory(nil), r.created...), nil
}

func (r *capturingMemoryRepository) FindByHash(string, string, string) (*models.ContextMemory, error) {
	return nil, gorm.ErrRecordNotFound
}

func (r *capturingMemoryRepository) Delete(uuid.UUID) error {
	return nil
}
