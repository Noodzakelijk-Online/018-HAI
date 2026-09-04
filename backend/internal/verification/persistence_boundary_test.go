package verification

import (
	"errors"
	"strings"
	"testing"

	"automation-hub-backend/internal/memory"
	"automation-hub-backend/internal/models"
)

func TestAnswerFailsClosedWhenEvidenceCannotBePersisted(t *testing.T) {
	repository := &failingVerificationRepository{
		fakeVerificationRepository: &fakeVerificationRepository{},
		evidenceErr:                errors.New("evidence database unavailable"),
	}
	memoryRepository := &capturingMemoryRepository{}
	service := NewService(repository, nil, memory.NewService(memoryRepository))

	_, err := service.Answer(AnswerRequest{
		OwnerIdentity:     "robert",
		Question:          "What does the record say?",
		DraftAnswer:       "The record says the work passed.",
		Mode:              ModeGrounded,
		AllowMemoryUpdate: true,
		ExternalEvidence: []EvidenceInput{{
			SourceType: "local_record", SourceID: "record-1", SourceURI: "file:///record-1",
			SourceLabel: "record", Snippet: "The record says the work passed.",
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "persist verification evidence") {
		t.Fatalf("Answer error = %v, want evidence persistence failure", err)
	}
	if len(repository.runs) != 1 || repository.runs[0].Status != StatusUncertain {
		t.Fatalf("run was finalized after failed evidence persistence: %#v", repository.runs)
	}
	if len(repository.claims) != 0 || len(memoryRepository.created) != 0 {
		t.Fatalf("failed verification created claims or memory: claims=%d memory=%d", len(repository.claims), len(memoryRepository.created))
	}
}

func TestAnswerFailsClosedWhenClaimCannotBePersisted(t *testing.T) {
	repository := &failingVerificationRepository{
		fakeVerificationRepository: &fakeVerificationRepository{},
		claimErr:                   errors.New("claim database unavailable"),
	}
	service := NewService(repository, nil, nil)

	_, err := service.Answer(AnswerRequest{
		OwnerIdentity: "robert",
		Question:      "What does the record say?",
		DraftAnswer:   "The record says the work passed.",
		Mode:          ModeGrounded,
		ExternalEvidence: []EvidenceInput{{
			SourceType: "local_record", SourceID: "record-1", SourceURI: "file:///record-1",
			SourceLabel: "record", Snippet: "The record says the work passed.",
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "persist verification claim") {
		t.Fatalf("Answer error = %v, want claim persistence failure", err)
	}
	if len(repository.runs) != 1 || repository.runs[0].Status != StatusUncertain {
		t.Fatalf("run was finalized after failed claim persistence: %#v", repository.runs)
	}
	if len(repository.claims) != 0 {
		t.Fatalf("claim persistence failure retained a claim: %#v", repository.claims)
	}
}

func TestAtomicRepositoryRollsBackEvidenceWhenClaimCannotBePersisted(t *testing.T) {
	repository := &transactionalFailingVerificationRepository{
		failingVerificationRepository: &failingVerificationRepository{
			fakeVerificationRepository: &fakeVerificationRepository{},
			claimErr:                   errors.New("claim database unavailable"),
		},
	}
	service := NewService(repository, nil, nil)

	_, err := service.Answer(AnswerRequest{
		OwnerIdentity: "robert",
		Question:      "What does the record say?",
		DraftAnswer:   "The record says the work passed.",
		Mode:          ModeGrounded,
		ExternalEvidence: []EvidenceInput{{
			SourceType: "local_record", SourceID: "record-1", SourceURI: "file:///record-1",
			SourceLabel: "record", Snippet: "The record says the work passed.",
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "persist verification claim") {
		t.Fatalf("Answer error = %v, want claim persistence failure", err)
	}
	if len(repository.evidence) != 0 || len(repository.claims) != 0 {
		t.Fatalf("atomic failure retained partial verification records: evidence=%d claims=%d", len(repository.evidence), len(repository.claims))
	}
	if len(repository.runs) != 1 || repository.runs[0].Status != StatusUncertain {
		t.Fatalf("atomic failure finalized the run: %#v", repository.runs)
	}
}

type failingVerificationRepository struct {
	*fakeVerificationRepository
	evidenceErr error
	claimErr    error
}

func (r *failingVerificationRepository) CreateEvidence(evidence *models.VerificationEvidence) (*models.VerificationEvidence, error) {
	if r.evidenceErr != nil {
		return nil, r.evidenceErr
	}
	return r.fakeVerificationRepository.CreateEvidence(evidence)
}

func (r *failingVerificationRepository) CreateClaim(claim *models.VerificationClaim) (*models.VerificationClaim, error) {
	if r.claimErr != nil {
		return nil, r.claimErr
	}
	return r.fakeVerificationRepository.CreateClaim(claim)
}

type transactionalFailingVerificationRepository struct {
	*failingVerificationRepository
}

func (r *transactionalFailingVerificationRepository) WithinTransaction(action func(Repository) error) error {
	staged := &fakeVerificationRepository{
		runs:     append([]models.VerificationRun(nil), r.runs...),
		claims:   append([]models.VerificationClaim(nil), r.claims...),
		evidence: append([]models.VerificationEvidence(nil), r.evidence...),
	}
	transactional := &failingVerificationRepository{
		fakeVerificationRepository: staged,
		evidenceErr:                r.evidenceErr,
		claimErr:                   r.claimErr,
	}
	if err := action(transactional); err != nil {
		return err
	}
	r.runs = staged.runs
	r.claims = staged.claims
	r.evidence = staged.evidence
	return nil
}
