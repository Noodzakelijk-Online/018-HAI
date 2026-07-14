package verification

import (
	"testing"

	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
)

func TestRequestedPursuitID(t *testing.T) {
	valid := uuid.New()

	parsed, err := requestedPursuitID(valid.String())
	if err != nil || parsed != valid {
		t.Fatalf("requestedPursuitID(valid) = %v, %v", parsed, err)
	}

	parsed, err = requestedPursuitID("  ")
	if err != nil || parsed != uuid.Nil {
		t.Fatalf("requestedPursuitID(empty) = %v, %v", parsed, err)
	}

	if _, err := requestedPursuitID("not-a-uuid"); err == nil {
		t.Fatal("requestedPursuitID accepted an invalid id")
	}
}

func TestAnswerLinksExplicitPursuitEvidence(t *testing.T) {
	repo := &fakeVerificationRepository{}
	linker := &capturingPursuitLinker{}
	pursuitID := uuid.New()
	service := NewService(repo, nil, nil, linker)

	result, err := service.Answer(AnswerRequest{
		Question:  "What does the supplied legal record establish?",
		PursuitID: pursuitID.String(),
		Mode:      ModeGrounded,
		ExternalEvidence: []EvidenceInput{{
			SourceType:  "legal_record",
			SourceURI:   "local://vivare/record-1",
			SourceLabel: "Legal record",
			Snippet:     "The hearing date is 9 September.",
			Authority:   "primary_document",
			Primary:     true,
		}},
	})
	if err != nil {
		t.Fatalf("Answer returned error: %v", err)
	}
	if !result.PursuitLinked || result.PursuitID != pursuitID.String() || result.PursuitLinkError != "" {
		t.Fatalf("pursuit link result = %#v", result)
	}
	if linker.pursuitID != pursuitID || linker.verificationID != result.Run.ID {
		t.Fatalf("linker received pursuit=%s verification=%s; want pursuit=%s verification=%s", linker.pursuitID, linker.verificationID, pursuitID, result.Run.ID)
	}
}

type capturingPursuitLinker struct {
	pursuitID      uuid.UUID
	verificationID uuid.UUID
}

func (l *capturingPursuitLinker) LinkVerification(pursuitID, verificationID uuid.UUID) error {
	l.pursuitID = pursuitID
	l.verificationID = verificationID
	return nil
}

type fakeVerificationRepository struct {
	runs     []models.VerificationRun
	claims   []models.VerificationClaim
	evidence []models.VerificationEvidence
}

func (r *fakeVerificationRepository) CreateRun(run *models.VerificationRun) (*models.VerificationRun, error) {
	if run.ID == uuid.Nil {
		run.ID = uuid.New()
	}
	r.runs = append(r.runs, *run)
	return run, nil
}

func (r *fakeVerificationRepository) UpdateRun(run *models.VerificationRun) (*models.VerificationRun, error) {
	for index := range r.runs {
		if r.runs[index].ID == run.ID {
			r.runs[index] = *run
		}
	}
	return run, nil
}

func (r *fakeVerificationRepository) CreateEvidence(evidence *models.VerificationEvidence) (*models.VerificationEvidence, error) {
	if evidence.ID == uuid.Nil {
		evidence.ID = uuid.New()
	}
	r.evidence = append(r.evidence, *evidence)
	return evidence, nil
}

func (r *fakeVerificationRepository) CreateClaim(claim *models.VerificationClaim) (*models.VerificationClaim, error) {
	if claim.ID == uuid.Nil {
		claim.ID = uuid.New()
	}
	r.claims = append(r.claims, *claim)
	return claim, nil
}

func (r *fakeVerificationRepository) CreateAuditLog(*models.VerificationAuditLog) (*models.VerificationAuditLog, error) {
	return nil, nil
}

func (r *fakeVerificationRepository) FindRuns() ([]models.VerificationRun, error) {
	return r.runs, nil
}

func (r *fakeVerificationRepository) FindClaims(runID uuid.UUID) ([]models.VerificationClaim, error) {
	claims := []models.VerificationClaim{}
	for _, claim := range r.claims {
		if claim.RunID == runID {
			claims = append(claims, claim)
		}
	}
	return claims, nil
}

func (r *fakeVerificationRepository) FindEvidence(runID uuid.UUID) ([]models.VerificationEvidence, error) {
	evidence := []models.VerificationEvidence{}
	for _, item := range r.evidence {
		if item.RunID == runID {
			evidence = append(evidence, item)
		}
	}
	return evidence, nil
}
