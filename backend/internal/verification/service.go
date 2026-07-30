package verification

import (
	"automation-hub-backend/internal/memory"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/ragflow"
	"automation-hub-backend/internal/research"
	"automation-hub-backend/internal/source"
	"context"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	ModeDraft    = "draft"
	ModeGrounded = "grounded"
	ModeStrict   = "strict"
	ModeAction   = "action"

	StatusVerified        = "verified"
	StatusSourceSupported = "source_supported"
	StatusSchemaValidated = "schema_validated"
	StatusTestPassed      = "test_passed"
	StatusHumanApproved   = "human_approved"
	StatusUncertain       = "uncertain"
	StatusConflicting     = "conflicting"
	StatusUnsupported     = "unsupported"
	StatusNeedsReview     = "needs_review"
)

type EvidenceInput struct {
	SourceType  string `json:"sourceType"`
	SourceID    string `json:"sourceId,omitempty"`
	SourceURI   string `json:"sourceUri,omitempty"`
	SourceLabel string `json:"sourceLabel,omitempty"`
	Snippet     string `json:"snippet"`
	Authority   string `json:"authority,omitempty"`
	Freshness   string `json:"freshness,omitempty"`
	Official    bool   `json:"official,omitempty"`
	Primary     bool   `json:"primary,omitempty"`
	Generated   bool   `json:"generated,omitempty"`
}

type AnswerRequest struct {
	OwnerIdentity            string          `json:"-"`
	Question                 string          `json:"question"`
	ProjectKey               string          `json:"projectKey,omitempty"`
	PursuitID                string          `json:"pursuitId,omitempty"`
	Mode                     string          `json:"mode,omitempty"`
	DraftAnswer              string          `json:"draftAnswer,omitempty"`
	ExternalEvidence         []EvidenceInput `json:"externalEvidence,omitempty"`
	IncludeSensitive         bool            `json:"includeSensitive,omitempty"`
	IncludeRAGFlowCandidates bool            `json:"includeRagflowCandidates,omitempty"`
	IncludeResearchCandidates bool           `json:"includeResearchCandidates,omitempty"`
	HumanApproved            bool            `json:"humanApproved,omitempty"`
	AllowMemoryUpdate        bool            `json:"allowMemoryUpdate,omitempty"`
}

// CandidateEvidence is deliberately separate from VerificationEvidence. It is
// a retrieval preview and is never persisted, used to support a claim, or
// allowed to update memory or trigger an action until the operator explicitly
// attaches it as ordinary evidence in a later verification request.
type CandidateEvidence struct {
	SourceType   string  `json:"sourceType"`
	SourceURI    string  `json:"sourceUri"`
	SourceLabel  string  `json:"sourceLabel"`
	Snippet      string  `json:"snippet"`
	DatasetID    string  `json:"datasetId"`
	DocumentID   string  `json:"documentId,omitempty"`
	ChunkID      string  `json:"chunkId"`
	Similarity   float64 `json:"similarity,omitempty"`
	Status       string  `json:"status"`
	Restrictions string  `json:"restrictions"`
}

// EvidenceConflict is a deterministic review signal, not an assertion that one
// source is true. It names the conflicting source records so an operator can
// resolve the disagreement with the underlying evidence.
type EvidenceConflict struct {
	Topic        string   `json:"topic"`
	Status       string   `json:"status"`
	Reason       string   `json:"reason"`
	EvidenceRefs []string `json:"evidenceRefs"`
}

type VerificationResult struct {
	Run               models.VerificationRun        `json:"run"`
	PursuitID         string                        `json:"pursuitId,omitempty"`
	PursuitLinked     bool                          `json:"pursuitLinked,omitempty"`
	PursuitLinkError  string                        `json:"pursuitLinkError,omitempty"`
	Claims            []models.VerificationClaim    `json:"claims"`
	Evidence          []models.VerificationEvidence `json:"evidence"`
	Conflicts         []EvidenceConflict            `json:"conflicts"`
	UnsupportedClaims []models.VerificationClaim    `json:"unsupportedClaims"`
	RAGFlowCandidates []CandidateEvidence           `json:"ragflowCandidates"`
	ResearchCandidates []CandidateEvidence          `json:"researchCandidates"`
	ResearchQuestions []string                      `json:"researchQuestions"`
	Logs              []string                      `json:"logs"`
}

type Service interface {
	Answer(request AnswerRequest) (*VerificationResult, error)
	Runs() ([]models.VerificationRun, error)
	RunsForOwner(ownerIdentity string) ([]models.VerificationRun, error)
	RunDetails(id uuid.UUID) (*VerificationResult, error)
	RunDetailsForOwner(ownerIdentity string, id uuid.UUID) (*VerificationResult, error)
}

type PursuitLinker interface {
	LinkVerificationForOwner(ownerIdentity string, pursuitID, verificationID uuid.UUID) error
}

type service struct {
	repo           Repository
	sourceService  source.Service
	memoryService  memory.Service
	ragflowService ragflow.Service
	researchService research.Service
	pursuitLinker  PursuitLinker
}

func NewService(repo Repository, sourceService source.Service, memoryService memory.Service, pursuitLinkers ...PursuitLinker) Service {
	return NewServiceWithRAGFlow(repo, sourceService, memoryService, nil, pursuitLinkers...)
}

// NewServiceWithRAGFlow keeps retrieval separate from evidence verification.
// Callers must opt in per request before this service contacts RAGFlow.
func NewServiceWithRAGFlow(repo Repository, sourceService source.Service, memoryService memory.Service, ragflowService ragflow.Service, pursuitLinkers ...PursuitLinker) Service {
	return NewServiceWithCandidateRetrieval(repo, sourceService, memoryService, ragflowService, nil, pursuitLinkers...)
}

// NewServiceWithCandidateRetrieval keeps local RAGFlow and SearXNG discovery
// separate from evidence verification. Each must be explicitly requested by
// the operator and only returns previews that cannot affect facts, memory, or
// actions until re-submitted as ordinary evidence and verified again.
func NewServiceWithCandidateRetrieval(repo Repository, sourceService source.Service, memoryService memory.Service, ragflowService ragflow.Service, researchService research.Service, pursuitLinkers ...PursuitLinker) Service {
	var pursuitLinker PursuitLinker
	if len(pursuitLinkers) > 0 {
		pursuitLinker = pursuitLinkers[0]
	}
	return &service{repo: repo, sourceService: sourceService, memoryService: memoryService, ragflowService: ragflowService, researchService: researchService, pursuitLinker: pursuitLinker}
}

func DefaultService() Service {
	return NewServiceWithRAGFlow(DefaultRepository(), source.DefaultService(), memory.DefaultService(), ragflow.DefaultService())
}

func (s *service) Answer(request AnswerRequest) (*VerificationResult, error) {
	pursuitID, err := requestedPursuitID(request.PursuitID)
	if err != nil {
		return nil, err
	}
	if pursuitID != uuid.Nil && s.pursuitLinker == nil {
		return nil, fmt.Errorf("pursuit linking is not configured")
	}
	mode := normalizeMode(request.Mode)
	questions := researchQuestions(request.Question, request.ProjectKey)
	logs := []string{"converted request into research questions"}
	run, err := s.repo.CreateRun(&models.VerificationRun{
		OwnerIdentity:     strings.TrimSpace(request.OwnerIdentity),
		Mode:              mode,
		Question:          strings.TrimSpace(request.Question),
		ProjectKey:        strings.TrimSpace(request.ProjectKey),
		Status:            StatusUncertain,
		ResearchQuestions: joinValues(questions),
		SourcesSearched:   "connected_sources,provided_evidence",
	})
	if err != nil {
		return nil, err
	}

	evidence := s.collectEvidence(run.ID, request, questions, &logs)
	ragflowCandidates, ragflowSearched := s.collectRAGFlowCandidates(request, mode, questions, &logs)
	researchCandidates, researchSearched := s.collectResearchCandidates(request, mode, questions, &logs)
	searchScopes := []string{"connected_sources", "provided_evidence"}
	if ragflowSearched {
		searchScopes = append(searchScopes, "ragflow_candidate_datasets")
	}
	if researchSearched {
		searchScopes = append(searchScopes, "local_research_candidates")
	}
	run.SourcesSearched = strings.Join(searchScopes, ",")
	answer := buildAnswer(request, mode, evidence)
	claims := decomposeClaims(run.ID, answer, mode, request)
	conflicts := detectEvidenceConflicts(evidence)
	verifiedClaims := verifyClaims(claims, evidence, conflicts, request, mode)
	unsupported := unsupportedClaims(verifiedClaims)
	status := runStatus(verifiedClaims, mode, request)

	run.Answer = answer
	run.Status = status
	run.SourcesUsed = sourceLabels(filterEvidence(evidence, true))
	run.SourcesRejected = sourceLabels(filterRejectedEvidence(evidence))
	run.MissingSources = missingSources(request, evidence)
	run, err = s.repo.UpdateRun(run)
	if err != nil {
		return nil, err
	}
	for _, item := range evidence {
		_, _ = s.repo.CreateEvidence(&item)
	}
	for _, claim := range verifiedClaims {
		_, _ = s.repo.CreateClaim(&claim)
	}
	s.audit(run.ID, "verification.completed", "important claims decomposed and verified before acceptance")
	if len(conflicts) > 0 {
		s.audit(run.ID, "verification.conflicts_detected", fmt.Sprintf("%d source-linked evidence conflict(s) require review", len(conflicts)))
		logs = append(logs, "detected source-linked evidence conflicts; no conflict was resolved automatically")
	}
	if request.AllowMemoryUpdate {
		s.storeVerifiedMemory(request, run, verifiedClaims)
	}
	result := &VerificationResult{
		Run:               *run,
		Claims:            verifiedClaims,
		Evidence:          evidence,
		Conflicts:         conflicts,
		UnsupportedClaims: unsupported,
		RAGFlowCandidates: ragflowCandidates,
		ResearchCandidates: researchCandidates,
		ResearchQuestions: questions,
		Logs:              append(logs, "verification status logged for every important claim"),
	}
	if pursuitID != uuid.Nil {
		result.PursuitID = pursuitID.String()
		if err := s.pursuitLinker.LinkVerificationForOwner(request.OwnerIdentity, pursuitID, run.ID); err != nil {
			result.PursuitLinkError = err.Error()
			result.Logs = append(result.Logs, "verification was saved but could not be linked to the requested pursuit")
			s.audit(run.ID, "verification.pursuit_link_failed", err.Error())
		} else {
			result.PursuitLinked = true
			result.Logs = append(result.Logs, "verification linked to the requested pursuit")
			s.audit(run.ID, "verification.pursuit_linked", "verification run linked to requested pursuit "+pursuitID.String())
		}
	}
	return result, nil
}

func (s *service) collectResearchCandidates(request AnswerRequest, mode string, questions []string, logs *[]string) ([]CandidateEvidence, bool) {
	if !request.IncludeResearchCandidates {
		return []CandidateEvidence{}, false
	}
	if mode == ModeAction {
		*logs = append(*logs, "local public-source candidate retrieval is not available in action mode")
		return []CandidateEvidence{}, false
	}
	if s.researchService == nil || !s.researchService.Status().Configured {
		*logs = append(*logs, "local public-source discovery is not configured; no candidate evidence was added")
		return []CandidateEvidence{}, false
	}

	const maxQuestions = 2
	const maxCandidatesPerQuestion = 3
	seen := map[string]bool{}
	candidates := []CandidateEvidence{}
	searched := false
	for index, question := range questions {
		if index >= maxQuestions {
			break
		}
		response, err := s.researchService.Search(context.Background(), research.Request{Query: question, Limit: maxCandidatesPerQuestion})
		if err != nil {
			*logs = append(*logs, "local public-source discovery was unavailable; no candidate evidence was added for one research question")
			continue
		}
		searched = true
		for _, item := range response.Results {
			key := strings.TrimSpace(item.SourceURI)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			candidates = append(candidates, CandidateEvidence{
				SourceType:   "searxng_candidate",
				SourceURI:    item.SourceURI,
				SourceLabel:  firstNonEmpty(item.Title, item.SourceURI),
				Snippet:      compact(item.Snippet, 1200),
				Status:       "unverified_candidate",
				Restrictions: "Discovery preview only. This result has not been fetched or checked for source support, freshness, authority, or conflicts. It is not persisted as evidence and cannot support claims, update memory, or authorize actions until explicitly attached and re-verified.",
			})
		}
	}
	if searched {
		*logs = append(*logs, "retrieved local public-source candidates without using them for verification")
	}
	return candidates, searched
}

func (s *service) collectRAGFlowCandidates(request AnswerRequest, mode string, questions []string, logs *[]string) ([]CandidateEvidence, bool) {
	if !request.IncludeRAGFlowCandidates {
		return []CandidateEvidence{}, false
	}
	if mode == ModeAction {
		*logs = append(*logs, "RAGFlow candidate retrieval is not available in action mode")
		return []CandidateEvidence{}, false
	}
	if s.ragflowService == nil || !s.ragflowService.Status().Configured {
		*logs = append(*logs, "local RAGFlow candidate retrieval is not configured; no candidate evidence was added")
		return []CandidateEvidence{}, false
	}

	const maxQuestions = 2
	const maxCandidatesPerQuestion = 3
	seen := map[string]bool{}
	candidates := []CandidateEvidence{}
	searched := false
	for index, question := range questions {
		if index >= maxQuestions {
			break
		}
		response, err := s.ragflowService.Retrieve(context.Background(), ragflow.Request{Query: question, Limit: maxCandidatesPerQuestion})
		if err != nil {
			*logs = append(*logs, "local RAGFlow candidate retrieval was unavailable; no candidate evidence was added for one research question")
			continue
		}
		searched = true
		for _, item := range response.Results {
			key := item.DatasetID + "\x00" + item.DocumentID + "\x00" + item.ChunkID
			if seen[key] {
				continue
			}
			seen[key] = true
			candidates = append(candidates, CandidateEvidence{
				SourceType:   "ragflow_candidate",
				SourceURI:    ragflowCandidateURI(item),
				SourceLabel:  firstNonEmpty(item.DocumentName, "RAGFlow candidate"),
				Snippet:      compact(item.Content, 1200),
				DatasetID:    item.DatasetID,
				DocumentID:   item.DocumentID,
				ChunkID:      item.ChunkID,
				Similarity:   item.Similarity,
				Status:       "unverified_candidate",
				Restrictions: "Preview only. This chunk is not persisted as verification evidence and cannot support claims, update memory, or authorize actions until explicitly attached and re-verified.",
			})
		}
	}
	if searched {
		*logs = append(*logs, "retrieved local RAGFlow candidate evidence without using it for verification")
	}
	return candidates, searched
}

func ragflowCandidateURI(item ragflow.Result) string {
	segment := func(value string) string { return url.PathEscape(firstNonEmpty(value, "unknown")) }
	return "ragflow://dataset/" + segment(item.DatasetID) + "/document/" + segment(item.DocumentID) + "/chunk/" + segment(item.ChunkID)
}

func requestedPursuitID(value string) (uuid.UUID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return uuid.Nil, nil
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid pursuitId")
	}
	return id, nil
}

func (s *service) Runs() ([]models.VerificationRun, error) {
	return s.repo.FindRuns()
}

func (s *service) RunsForOwner(ownerIdentity string) ([]models.VerificationRun, error) {
	return s.repo.FindRunsForOwner(strings.TrimSpace(ownerIdentity))
}

func (s *service) RunDetails(id uuid.UUID) (*VerificationResult, error) {
	return s.runDetailsForOwner("", id)
}

func (s *service) RunDetailsForOwner(ownerIdentity string, id uuid.UUID) (*VerificationResult, error) {
	return s.runDetailsForOwner(strings.TrimSpace(ownerIdentity), id)
}

func (s *service) runDetailsForOwner(ownerIdentity string, id uuid.UUID) (*VerificationResult, error) {
	runs, err := s.RunsForOwner(ownerIdentity)
	if err != nil {
		return nil, err
	}
	var run *models.VerificationRun
	for index := range runs {
		if runs[index].ID == id {
			copy := runs[index]
			run = &copy
			break
		}
	}
	if run == nil {
		return nil, fmt.Errorf("verification run not found")
	}
	claims, err := s.repo.FindClaims(id)
	if err != nil {
		return nil, err
	}
	evidence, err := s.repo.FindEvidence(id)
	if err != nil {
		return nil, err
	}
	conflicts := detectEvidenceConflicts(evidence)
	logs := []string{"loaded persisted verification details"}
	if len(conflicts) > 0 {
		logs = append(logs, "source-linked evidence conflicts require review")
	}
	return &VerificationResult{
		Run:               *run,
		Claims:            claims,
		Evidence:          evidence,
		Conflicts:         conflicts,
		UnsupportedClaims: unsupportedClaims(claims),
		RAGFlowCandidates: []CandidateEvidence{},
		ResearchCandidates: []CandidateEvidence{},
		Logs:              logs,
	}, nil
}

func (s *service) collectEvidence(runID uuid.UUID, request AnswerRequest, questions []string, logs *[]string) []models.VerificationEvidence {
	evidence := []models.VerificationEvidence{}
	if s.sourceService != nil {
		for _, question := range questions {
			result, err := s.sourceService.Search(source.SearchRequest{
				OwnerIdentity:    request.OwnerIdentity,
				Query:            question,
				ProjectKey:       request.ProjectKey,
				Limit:            6,
				IncludeSensitive: request.IncludeSensitive,
			})
			if err == nil {
				for _, ranked := range result.UsedContext {
					evidence = append(evidence, models.VerificationEvidence{
						RunID:        runID,
						SourceType:   "connected_source",
						SourceID:     ranked.Extraction.ID.String(),
						SourceURI:    ranked.Extraction.SourceURI,
						SourceLabel:  ranked.Extraction.SourceLabel,
						Snippet:      firstNonEmpty(ranked.Extraction.Summary, ranked.Extraction.Text),
						Authority:    "connected_account",
						Freshness:    freshnessLabel(ranked.Extraction.UpdatedAt),
						QualityScore: math.Min(1, 0.62+ranked.Score/2),
						Used:         true,
					})
				}
				*logs = append(*logs, "searched connected-source index")
			}
		}
	} else {
		*logs = append(*logs, "connected-source index is not configured; using only supplied evidence")
	}
	for _, input := range request.ExternalEvidence {
		score := evidenceQuality(input, request.Question)
		evidence = append(evidence, models.VerificationEvidence{
			RunID:        runID,
			SourceType:   firstNonEmpty(input.SourceType, "external"),
			SourceID:     input.SourceID,
			SourceURI:    input.SourceURI,
			SourceLabel:  input.SourceLabel,
			Snippet:      input.Snippet,
			Authority:    input.Authority,
			Freshness:    input.Freshness,
			QualityScore: score,
			Used:         score >= 0.35,
			Rejected:     score < 0.35,
			RejectReason: rejectReason(score, input),
		})
	}
	sort.SliceStable(evidence, func(i, j int) bool {
		return evidence[i].QualityScore > evidence[j].QualityScore
	})
	return evidence
}

func buildAnswer(request AnswerRequest, mode string, evidence []models.VerificationEvidence) string {
	if mode == ModeDraft {
		return "Draft hypothesis: " + strings.TrimSpace(request.Question)
	}
	if strings.TrimSpace(request.DraftAnswer) != "" {
		return strings.TrimSpace(request.DraftAnswer)
	}
	used := filterEvidence(evidence, true)
	if len(used) == 0 {
		return "No grounded answer can be produced because no supporting evidence was found."
	}
	lines := []string{}
	for _, item := range used {
		lines = append(lines, compact(item.Snippet, 260))
		if len(lines) >= 5 {
			break
		}
	}
	return strings.Join(lines, ". ")
}

func decomposeClaims(runID uuid.UUID, answer, mode string, request AnswerRequest) []models.VerificationClaim {
	claims := []models.VerificationClaim{}
	for _, sentence := range splitClaims(answer) {
		if sentence == "" {
			continue
		}
		claims = append(claims, models.VerificationClaim{
			RunID:     runID,
			ClaimText: sentence,
			Status:    StatusUncertain,
			HighRisk:  highRisk(request.Question + " " + sentence),
		})
	}
	if len(claims) == 0 {
		claims = append(claims, models.VerificationClaim{
			RunID:       runID,
			ClaimText:   "No answer claim was generated.",
			Status:      StatusNeedsReview,
			NeedsReview: true,
		})
	}
	return claims
}

func verifyClaims(claims []models.VerificationClaim, evidence []models.VerificationEvidence, conflicts []EvidenceConflict, request AnswerRequest, mode string) []models.VerificationClaim {
	for i := range claims {
		claim := &claims[i]
		best, score := bestEvidenceForClaim(claim.ClaimText, evidence)
		if mode == ModeDraft {
			claim.Status = StatusUncertain
			claim.NeedsReview = true
			claim.SupportExplanation = "draft mode does not grant factual confidence"
			claim.Confidence = 0.25
			continue
		}
		if claim.HighRisk && !request.HumanApproved {
			claim.Status = StatusNeedsReview
			claim.NeedsReview = true
			claim.SupportExplanation = "high-risk output requires human approval"
			claim.Confidence = 0.2
			continue
		}
		if ok, passed := deterministicCalculationCheck(claim.ClaimText); ok {
			if passed {
				claim.Status = StatusVerified
				claim.Confidence = 1
				claim.SupportExplanation = "arithmetic claim passed deterministic calculation"
			} else {
				claim.Status = StatusUnsupported
				claim.NeedsReview = true
				claim.Confidence = 0
				claim.SupportExplanation = "arithmetic claim failed deterministic calculation"
			}
			continue
		}
		if best == nil || score < 0.22 {
			claim.Status = StatusUnsupported
			claim.NeedsReview = mode == ModeStrict || mode == ModeAction || mode == ModeGrounded
			claim.SupportExplanation = "no source precisely supports this claim"
			claim.Confidence = math.Round(score*100) / 100
			continue
		}
		claim.SourceRefs = firstNonEmpty(best.SourceURI, best.SourceID, best.SourceLabel)
		claim.Confidence = math.Round((score+best.QualityScore)/2*100) / 100
		claim.SupportExplanation = "claim overlaps supporting evidence and has source provenance"
		claim.Status = StatusSourceSupported
		if claim.Confidence >= 0.72 {
			claim.Status = StatusVerified
		}
		if best.SourceType == "test_result" && containsAny(strings.ToLower(best.Snippet), "pass", "passed", "ok") {
			claim.Status = StatusTestPassed
		}
		if request.HumanApproved && claim.HighRisk {
			claim.Status = StatusHumanApproved
		}
		if conflict := conflictForClaim(claim.ClaimText, conflicts); conflict != nil {
			claim.Status = StatusConflicting
			claim.NeedsReview = true
			claim.SourceRefs = joinValues(conflict.EvidenceRefs)
			claim.SupportExplanation = "separate source records disagree about " + conflict.Topic
		}
	}
	return claims
}

func bestEvidenceForClaim(claim string, evidence []models.VerificationEvidence) (*models.VerificationEvidence, float64) {
	var best *models.VerificationEvidence
	bestScore := 0.0
	claimTokens := tokenSet(claim)
	for i := range evidence {
		if evidence[i].Rejected || evidence[i].Snippet == "" {
			continue
		}
		score := overlapScore(claimTokens, tokenSet(evidence[i].Snippet))
		score = score*0.75 + evidence[i].QualityScore*0.25
		if score > bestScore {
			bestScore = score
			best = &evidence[i]
		}
	}
	return best, bestScore
}

func runStatus(claims []models.VerificationClaim, mode string, request AnswerRequest) string {
	if len(claims) == 0 {
		return StatusNeedsReview
	}
	hasUnsupported := false
	hasReview := false
	for _, claim := range claims {
		if claim.Status == StatusUnsupported {
			hasUnsupported = true
		}
		if claim.NeedsReview || claim.Status == StatusNeedsReview || claim.Status == StatusConflicting || claim.Status == StatusUncertain {
			hasReview = true
		}
	}
	if hasReview {
		return StatusNeedsReview
	}
	if hasUnsupported {
		return StatusUnsupported
	}
	if request.HumanApproved && mode == ModeAction {
		return StatusHumanApproved
	}
	return StatusVerified
}

func (s *service) storeVerifiedMemory(request AnswerRequest, run *models.VerificationRun, claims []models.VerificationClaim) {
	for _, claim := range claims {
		if claim.Status != StatusVerified && claim.Status != StatusSourceSupported && claim.Status != StatusHumanApproved {
			continue
		}
		_, _ = memory.CreateForOwner(s.memoryService, request.OwnerIdentity, memory.CreateRequest{
			ProjectKey:  request.ProjectKey,
			Kind:        "verified_fact",
			Content:     claim.ClaimText,
			Summary:     compact(claim.ClaimText, 240),
			Tags:        []string{"verified", claim.Status},
			Confidence:  claim.Confidence,
			SourceURI:   claim.SourceRefs,
			SourceLabel: "verification-run:" + run.ID.String(),
		})
	}
}

func (s *service) audit(runID uuid.UUID, action, message string) {
	_, _ = s.repo.CreateAuditLog(&models.VerificationAuditLog{
		RunID:   runID,
		Action:  action,
		Message: message,
	})
}

func normalizeMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ModeDraft, ModeGrounded, ModeStrict, ModeAction:
		return strings.ToLower(strings.TrimSpace(mode))
	default:
		return ModeGrounded
	}
}

func researchQuestions(question, projectKey string) []string {
	base := strings.TrimSpace(question)
	questions := []string{base}
	if projectKey != "" {
		questions = append(questions, projectKey+" "+base)
	}
	if needsExternal(question) {
		questions = append(questions, "current official source for "+base)
	}
	return uniqueStrings(questions)
}

func needsExternal(text string) bool {
	return containsAny(strings.ToLower(text), "latest", "current", "today", "public", "official", "legal", "government", "financial", "medical")
}

func evidenceQuality(input EvidenceInput, question string) float64 {
	score := 0.35
	if input.Official {
		score += 0.25
	}
	if input.Primary {
		score += 0.2
	}
	if input.SourceURI != "" {
		score += 0.1
	}
	if input.Generated {
		score -= 0.25
	}
	if overlapScore(tokenSet(question), tokenSet(input.Snippet)) > 0.2 {
		score += 0.1
	}
	if score > 1 {
		return 1
	}
	if score < 0 {
		return 0
	}
	return math.Round(score*100) / 100
}

func rejectReason(score float64, input EvidenceInput) string {
	if score >= 0.35 {
		return ""
	}
	if input.Generated {
		return "low-quality generated source"
	}
	return "source authority or relevance too weak"
}

func missingSources(request AnswerRequest, evidence []models.VerificationEvidence) string {
	if len(filterEvidence(evidence, true)) > 0 {
		return ""
	}
	if needsExternal(request.Question) {
		return "authoritative external source required but not available"
	}
	return "connected-source or provided evidence required but not available"
}

func unsupportedClaims(claims []models.VerificationClaim) []models.VerificationClaim {
	result := []models.VerificationClaim{}
	for _, claim := range claims {
		if claim.Status == StatusUnsupported || claim.Status == StatusUncertain || claim.Status == StatusNeedsReview || claim.Status == StatusConflicting {
			result = append(result, claim)
		}
	}
	return result
}

func filterEvidence(evidence []models.VerificationEvidence, used bool) []models.VerificationEvidence {
	result := []models.VerificationEvidence{}
	for _, item := range evidence {
		if item.Used == used && !item.Rejected {
			result = append(result, item)
		}
	}
	return result
}

func filterRejectedEvidence(evidence []models.VerificationEvidence) []models.VerificationEvidence {
	result := []models.VerificationEvidence{}
	for _, item := range evidence {
		if item.Rejected {
			result = append(result, item)
		}
	}
	return result
}

func sourceLabels(evidence []models.VerificationEvidence) string {
	values := []string{}
	for _, item := range evidence {
		values = append(values, firstNonEmpty(item.SourceLabel, item.SourceURI, item.SourceID, item.SourceType))
	}
	return joinValues(values)
}

func detectEvidenceConflicts(evidence []models.VerificationEvidence) []EvidenceConflict {
	conflicts := []EvidenceConflict{}
	seen := map[string]bool{}
	for left := 0; left < len(evidence); left++ {
		if evidence[left].Rejected || strings.TrimSpace(evidence[left].Snippet) == "" {
			continue
		}
		leftState := evidenceAssertionState(evidence[left].Snippet)
		if leftState == "" {
			continue
		}
		for right := left + 1; right < len(evidence); right++ {
			if evidence[right].Rejected || strings.TrimSpace(evidence[right].Snippet) == "" {
				continue
			}
			rightState := evidenceAssertionState(evidence[right].Snippet)
			if rightState == "" || rightState == leftState || evidenceReference(evidence[left]) == evidenceReference(evidence[right]) {
				continue
			}
			topicTokens := sharedTopicTokens(evidence[left].Snippet, evidence[right].Snippet)
			if len(topicTokens) < 2 {
				continue
			}
			refs := []string{evidenceReference(evidence[left]), evidenceReference(evidence[right])}
			sort.Strings(refs)
			key := strings.Join(topicTokens, " ") + "\x00" + strings.Join(refs, "\x00")
			if seen[key] {
				continue
			}
			seen[key] = true
			conflicts = append(conflicts, EvidenceConflict{
				Topic:        strings.Join(topicTokens, " "),
				Status:       StatusNeedsReview,
				Reason:       "separate source records carry opposite status assertions",
				EvidenceRefs: refs,
			})
		}
	}
	return conflicts
}

func conflictForClaim(claim string, conflicts []EvidenceConflict) *EvidenceConflict {
	claimTokens := tokenSet(claim)
	for index := range conflicts {
		matches := 0
		for _, token := range strings.Fields(conflicts[index].Topic) {
			if claimTokens[token] {
				matches++
			}
		}
		if matches >= 2 {
			return &conflicts[index]
		}
	}
	return nil
}

func evidenceAssertionState(text string) string {
	lower := strings.ToLower(text)
	if containsAny(lower, "not approved", "not accepted", "not confirmed", "rejected", "denied", "disabled", "cancelled", "canceled", "revoked", "incomplete") {
		return "negative"
	}
	if containsAny(lower, "approved", "accepted", "enabled", "confirmed", "completed", "scheduled", "granted") {
		return "positive"
	}
	return ""
}

func evidenceReference(evidence models.VerificationEvidence) string {
	return firstNonEmpty(evidence.SourceURI, evidence.SourceID, evidence.SourceLabel, evidence.SourceType)
}

func sharedTopicTokens(left, right string) []string {
	leftTokens := topicTokenSet(left)
	rightTokens := topicTokenSet(right)
	shared := []string{}
	for token := range leftTokens {
		if rightTokens[token] {
			shared = append(shared, token)
		}
	}
	sort.Strings(shared)
	if len(shared) > 5 {
		return shared[:5]
	}
	return shared
}

func topicTokenSet(text string) map[string]bool {
	ignored := map[string]bool{
		"a": true, "an": true, "and": true, "are": true, "as": true, "at": true, "be": true, "by": true,
		"for": true, "from": true, "has": true, "have": true, "in": true, "is": true, "it": true, "of": true,
		"on": true, "or": true, "the": true, "to": true, "was": true, "were": true, "with": true,
		"approved": true, "accepted": true, "enabled": true, "confirmed": true, "completed": true, "scheduled": true, "granted": true,
		"rejected": true, "denied": true, "disabled": true, "cancelled": true, "canceled": true, "revoked": true, "incomplete": true, "not": true,
	}
	cleaned := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return ' '
	}, strings.ToLower(text))
	tokens := map[string]bool{}
	for _, token := range strings.Fields(cleaned) {
		if len(token) >= 3 && !ignored[token] {
			tokens[token] = true
		}
	}
	return tokens
}

func splitClaims(answer string) []string {
	raw := strings.NewReplacer("\n", ". ", ";", ".").Replace(answer)
	claims := []string{}
	for _, part := range strings.Split(raw, ".") {
		part = strings.TrimSpace(part)
		if part != "" {
			claims = append(claims, part)
		}
	}
	return claims
}

func highRisk(text string) bool {
	return containsAny(strings.ToLower(text), "email", "send", "delete", "financial", "legal", "government", "medical", "account", "public posting", "contract")
}

func deterministicCalculationCheck(claim string) (bool, bool) {
	value := strings.ReplaceAll(claim, " ", "")
	if !strings.Contains(value, "=") {
		return false, false
	}
	parts := strings.Split(value, "=")
	if len(parts) != 2 {
		return false, false
	}
	expected, err := strconv.ParseFloat(trimNumber(parts[1]), 64)
	if err != nil {
		return false, false
	}
	left := parts[0]
	operator := ""
	for _, candidate := range []string{"+", "-", "*", "/"} {
		if strings.Contains(left, candidate) {
			operator = candidate
			break
		}
	}
	if operator == "" {
		return false, false
	}
	numbers := strings.Split(left, operator)
	if len(numbers) != 2 {
		return false, false
	}
	a, errA := strconv.ParseFloat(trimNumber(numbers[0]), 64)
	b, errB := strconv.ParseFloat(trimNumber(numbers[1]), 64)
	if errA != nil || errB != nil {
		return false, false
	}
	result := 0.0
	switch operator {
	case "+":
		result = a + b
	case "-":
		result = a - b
	case "*":
		result = a * b
	case "/":
		if b == 0 {
			return true, false
		}
		result = a / b
	}
	return true, math.Abs(result-expected) < 0.0001
}

func trimNumber(value string) string {
	return strings.Trim(value, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ€$,%:;,.")
}

func freshnessLabel(value time.Time) string {
	days := time.Since(value).Hours() / 24
	if days <= 1 {
		return "fresh"
	}
	if days <= 30 {
		return "recent"
	}
	return "stale"
}

func tokenSet(value string) map[string]bool {
	set := map[string]bool{}
	replacer := strings.NewReplacer(",", " ", ".", " ", ";", " ", ":", " ", "/", " ", "\\", " ", "\n", " ", "\t", " ", "(", " ", ")", " ", "-", " ")
	for _, token := range strings.Fields(strings.ToLower(replacer.Replace(value))) {
		if len(token) >= 3 {
			if _, err := strconv.ParseFloat(token, 64); err == nil {
				set[token] = true
				continue
			}
			set[token] = true
		}
	}
	return set
}

func overlapScore(left, right map[string]bool) float64 {
	if len(left) == 0 {
		return 0
	}
	matches := 0
	for token := range left {
		if right[token] {
			matches++
		}
	}
	return float64(matches) / float64(len(left))
}

func compact(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= limit {
		return value
	}
	return value[:limit-3] + "..."
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func joinValues(values []string) string {
	return strings.Join(uniqueStrings(values), ",")
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
