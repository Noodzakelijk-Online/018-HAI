package verification

import (
	"context"
	"strings"
	"testing"

	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/ragflow"
	"automation-hub-backend/internal/research"

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
		OwnerIdentity: "alice",
		Question:      "What does the supplied legal record establish?",
		PursuitID:     pursuitID.String(),
		Mode:          ModeGrounded,
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
	if linker.ownerIdentity != "alice" || linker.pursuitID != pursuitID || linker.verificationID != result.Run.ID {
		t.Fatalf("linker received pursuit=%s verification=%s; want pursuit=%s verification=%s", linker.pursuitID, linker.verificationID, pursuitID, result.Run.ID)
	}
}

func TestVerificationRunsAndDetailsAreScopedToOwner(t *testing.T) {
	repo := &fakeVerificationRepository{}
	service := NewService(repo, nil, nil)

	alice, err := service.Answer(AnswerRequest{
		OwnerIdentity:    "alice",
		Question:         "What does Alice's document establish?",
		ExternalEvidence: []EvidenceInput{{SourceType: "document", Snippet: "Alice evidence", Primary: true}},
	})
	if err != nil {
		t.Fatalf("create Alice verification: %v", err)
	}
	bob, err := service.Answer(AnswerRequest{
		OwnerIdentity:    "bob",
		Question:         "What does Bob's document establish?",
		ExternalEvidence: []EvidenceInput{{SourceType: "document", Snippet: "Bob evidence", Primary: true}},
	})
	if err != nil {
		t.Fatalf("create Bob verification: %v", err)
	}

	runs, err := service.RunsForOwner("alice")
	if err != nil {
		t.Fatalf("RunsForOwner: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != alice.Run.ID || runs[0].OwnerIdentity != "alice" {
		t.Fatalf("Alice-visible runs = %#v", runs)
	}
	if _, err := service.RunDetailsForOwner("alice", bob.Run.ID); err == nil {
		t.Fatal("Alice could load Bob's verification run")
	}
	detail, err := service.RunDetailsForOwner("alice", alice.Run.ID)
	if err != nil {
		t.Fatalf("Alice RunDetailsForOwner: %v", err)
	}
	if detail.Run.ID != alice.Run.ID || detail.Run.OwnerIdentity != "alice" {
		t.Fatalf("Alice detail run = %#v", detail.Run)
	}
	if detail.RAGFlowCandidates == nil || detail.ResearchCandidates == nil {
		t.Fatalf("persisted detail must return empty candidate lists, got ragflow=%#v research=%#v", detail.RAGFlowCandidates, detail.ResearchCandidates)
	}
}

func TestAnswerOffersOptInRAGFlowCandidatesWithoutUsingOrPersistingThem(t *testing.T) {
	repo := &fakeVerificationRepository{}
	retriever := &fakeRAGFlowService{results: []ragflow.Result{{
		DatasetID:    "legal-records",
		DocumentID:   "vivare-2026-09",
		DocumentName: "Vivare hearing correspondence",
		ChunkID:      "chunk-1",
		Content:      "Unverified RAGFlow preview: the hearing is tomorrow.",
		Similarity:   0.91,
	}}}
	service := NewServiceWithRAGFlow(repo, nil, nil, retriever)

	result, err := service.Answer(AnswerRequest{
		OwnerIdentity:            "alice",
		Question:                 "What does the supplied case record establish?",
		Mode:                     ModeGrounded,
		IncludeRAGFlowCandidates: true,
		ExternalEvidence: []EvidenceInput{{
			SourceType:  "case_record",
			SourceURI:   "local://case/record-1",
			SourceLabel: "Supplied case record",
			Snippet:     "The supplied case record says the hearing is scheduled for 9 September.",
			Primary:     true,
		}},
	})
	if err != nil {
		t.Fatalf("Answer returned error: %v", err)
	}
	if retriever.retrieveCalls != 1 {
		t.Fatalf("RAGFlow retrieve calls = %d, want 1", retriever.retrieveCalls)
	}
	if len(result.RAGFlowCandidates) != 1 || result.RAGFlowCandidates[0].Status != "unverified_candidate" {
		t.Fatalf("RAGFlow candidates = %#v", result.RAGFlowCandidates)
	}
	if !strings.Contains(result.RAGFlowCandidates[0].SourceURI, "ragflow://dataset/legal-records") {
		t.Fatalf("candidate source uri = %q", result.RAGFlowCandidates[0].SourceURI)
	}
	if strings.Contains(result.Run.Answer, "Unverified RAGFlow preview") || strings.Contains(result.Run.SourcesUsed, "RAGFlow") {
		t.Fatalf("candidate preview influenced answer or sources used: %#v", result.Run)
	}
	for _, evidence := range repo.evidence {
		if evidence.SourceType == "ragflow_candidate" || strings.HasPrefix(evidence.SourceURI, "ragflow://") {
			t.Fatalf("RAGFlow preview was persisted as evidence: %#v", evidence)
		}
	}
	for _, claim := range result.Claims {
		if strings.Contains(claim.SourceRefs, "ragflow://") {
			t.Fatalf("RAGFlow preview supported a claim: %#v", claim)
		}
	}
}

func TestAnswerDoesNotRetrieveRAGFlowCandidatesForActionMode(t *testing.T) {
	repo := &fakeVerificationRepository{}
	retriever := &fakeRAGFlowService{results: []ragflow.Result{{DatasetID: "approved", ChunkID: "chunk-1", Content: "candidate"}}}
	service := NewServiceWithRAGFlow(repo, nil, nil, retriever)

	result, err := service.Answer(AnswerRequest{
		OwnerIdentity:            "alice",
		Question:                 "Send the legal reply.",
		Mode:                     ModeAction,
		IncludeRAGFlowCandidates: true,
	})
	if err != nil {
		t.Fatalf("Answer returned error: %v", err)
	}
	if retriever.retrieveCalls != 0 || len(result.RAGFlowCandidates) != 0 {
		t.Fatalf("action mode retrieved candidates: calls=%d candidates=%#v", retriever.retrieveCalls, result.RAGFlowCandidates)
	}
	if !strings.Contains(strings.Join(result.Logs, " "), "not available in action mode") {
		t.Fatalf("action-mode guard was not logged: %#v", result.Logs)
	}
}

func TestAnswerOffersOptInResearchCandidatesWithoutUsingOrPersistingThem(t *testing.T) {
	repo := &fakeVerificationRepository{}
	discovery := &fakeResearchService{results: []research.Result{{
		Title:     "Official case update",
		SourceURI: "https://court.example.test/cases/vivare",
		Snippet:   "Unverified discovery preview: a case update is available.",
	}}}
	service := NewServiceWithCandidateRetrieval(repo, nil, nil, nil, discovery)

	result, err := service.Answer(AnswerRequest{
		OwnerIdentity:            "alice",
		Question:                 "What does the supplied case record establish?",
		Mode:                     ModeGrounded,
		IncludeResearchCandidates: true,
		ExternalEvidence: []EvidenceInput{{
			SourceType:  "case_record",
			SourceURI:   "local://case/record-1",
			SourceLabel: "Supplied case record",
			Snippet:     "The supplied case record says the hearing is scheduled for 9 September.",
			Primary:     true,
		}},
	})
	if err != nil {
		t.Fatalf("Answer returned error: %v", err)
	}
	if discovery.searchCalls != 1 || len(result.ResearchCandidates) != 1 {
		t.Fatalf("research calls=%d candidates=%#v", discovery.searchCalls, result.ResearchCandidates)
	}
	candidate := result.ResearchCandidates[0]
	if candidate.SourceType != "searxng_candidate" || candidate.Status != "unverified_candidate" || candidate.SourceURI != "https://court.example.test/cases/vivare" {
		t.Fatalf("research candidate = %#v", candidate)
	}
	if strings.Contains(result.Run.Answer, "Unverified discovery preview") || strings.Contains(result.Run.SourcesUsed, "court.example.test") || !strings.Contains(result.Run.SourcesSearched, "local_research_candidates") {
		t.Fatalf("candidate preview influenced answer or source record: %#v", result.Run)
	}
	for _, evidence := range repo.evidence {
		if evidence.SourceType == "searxng_candidate" || strings.Contains(evidence.SourceURI, "court.example.test") {
			t.Fatalf("research preview was persisted as evidence: %#v", evidence)
		}
	}
	for _, claim := range result.Claims {
		if strings.Contains(claim.SourceRefs, "court.example.test") {
			t.Fatalf("research preview supported a claim: %#v", claim)
		}
	}
}

func TestAnswerDoesNotRetrieveResearchCandidatesForActionMode(t *testing.T) {
	repo := &fakeVerificationRepository{}
	discovery := &fakeResearchService{results: []research.Result{{Title: "Candidate", SourceURI: "https://example.test", Snippet: "candidate"}}}
	service := NewServiceWithCandidateRetrieval(repo, nil, nil, nil, discovery)

	result, err := service.Answer(AnswerRequest{
		OwnerIdentity:            "alice",
		Question:                 "Send the legal reply.",
		Mode:                     ModeAction,
		IncludeResearchCandidates: true,
	})
	if err != nil {
		t.Fatalf("Answer returned error: %v", err)
	}
	if discovery.searchCalls != 0 || len(result.ResearchCandidates) != 0 {
		t.Fatalf("action mode searched=%d candidates=%#v", discovery.searchCalls, result.ResearchCandidates)
	}
	if !strings.Contains(strings.Join(result.Logs, " "), "not available in action mode") {
		t.Fatalf("action-mode guard was not logged: %#v", result.Logs)
	}
}

func TestAnswerFlagsSourceLinkedContradictionsWithoutResolvingThem(t *testing.T) {
	repo := &fakeVerificationRepository{}
	service := NewService(repo, nil, nil)

	result, err := service.Answer(AnswerRequest{
		OwnerIdentity: "alice",
		Question:      "What is the current Vivare hearing status?",
		Mode:          ModeGrounded,
		DraftAnswer:   "The Vivare hearing date was confirmed.",
		ExternalEvidence: []EvidenceInput{
			{SourceType: "email", SourceURI: "local://mail/1", Snippet: "Vivare hearing date was confirmed for 9 September.", Primary: true},
			{SourceType: "court_letter", SourceURI: "local://letter/2", Snippet: "Vivare hearing date was cancelled by the court.", Primary: true},
		},
	})
	if err != nil {
		t.Fatalf("Answer returned error: %v", err)
	}
	if len(result.Conflicts) != 1 || result.Conflicts[0].Status != StatusNeedsReview {
		t.Fatalf("conflicts = %#v", result.Conflicts)
	}
	if len(result.Conflicts[0].EvidenceRefs) != 2 {
		t.Fatalf("conflict evidence refs = %#v", result.Conflicts[0].EvidenceRefs)
	}
	if len(result.Claims) != 1 || result.Claims[0].Status != StatusConflicting || !result.Claims[0].NeedsReview {
		t.Fatalf("claims = %#v", result.Claims)
	}
	detail, err := service.RunDetailsForOwner("alice", result.Run.ID)
	if err != nil {
		t.Fatalf("RunDetailsForOwner: %v", err)
	}
	if len(detail.Conflicts) != 1 || len(detail.Conflicts[0].EvidenceRefs) != 2 {
		t.Fatalf("persisted conflict detail = %#v", detail.Conflicts)
	}
}

func TestConflictScanRequiresSeparateSourcesAndConcreteSharedTopic(t *testing.T) {
	conflicts := detectEvidenceConflicts([]models.VerificationEvidence{
		{SourceURI: "local://same", Snippet: "Vivare hearing date was confirmed."},
		{SourceURI: "local://same", Snippet: "Vivare hearing date was cancelled."},
		{SourceURI: "local://other", Snippet: "Account access was denied."},
	})
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %#v; expected no conflict for duplicate source or unrelated topic", conflicts)
	}
}

type capturingPursuitLinker struct {
	ownerIdentity  string
	pursuitID      uuid.UUID
	verificationID uuid.UUID
}

func (l *capturingPursuitLinker) LinkVerificationForOwner(ownerIdentity string, pursuitID, verificationID uuid.UUID) error {
	l.ownerIdentity = ownerIdentity
	l.pursuitID = pursuitID
	l.verificationID = verificationID
	return nil
}

type fakeVerificationRepository struct {
	runs     []models.VerificationRun
	claims   []models.VerificationClaim
	evidence []models.VerificationEvidence
}

type fakeRAGFlowService struct {
	retrieveCalls int
	results       []ragflow.Result
}

type fakeResearchService struct {
	searchCalls int
	results     []research.Result
}

func (s *fakeResearchService) Status() research.Status {
	return research.Status{Enabled: true, Configured: true}
}

func (s *fakeResearchService) Probe(context.Context) (*research.ProbeResult, error) {
	return &research.ProbeResult{Reachable: true}, nil
}

func (s *fakeResearchService) Search(context.Context, research.Request) (*research.Response, error) {
	s.searchCalls++
	return &research.Response{Results: s.results}, nil
}

func (s *fakeRAGFlowService) Status() ragflow.Status {
	return ragflow.Status{Enabled: true, Configured: true}
}

func (s *fakeRAGFlowService) Probe(context.Context) (*ragflow.ProbeResult, error) {
	return &ragflow.ProbeResult{Reachable: true}, nil
}

func (s *fakeRAGFlowService) Retrieve(context.Context, ragflow.Request) (*ragflow.Response, error) {
	s.retrieveCalls++
	return &ragflow.Response{Results: s.results}, nil
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

func (r *fakeVerificationRepository) FindRunsForOwner(ownerIdentity string) ([]models.VerificationRun, error) {
	result := []models.VerificationRun{}
	for _, run := range r.runs {
		if ownerIdentity == "" || run.OwnerIdentity == "" || run.OwnerIdentity == ownerIdentity {
			result = append(result, run)
		}
	}
	return result, nil
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
