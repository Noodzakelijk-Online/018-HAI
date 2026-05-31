package verification

import (
	"automation-hub-backend/internal/memory"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/source"
	"math"
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
	Question          string          `json:"question"`
	ProjectKey        string          `json:"projectKey,omitempty"`
	Mode              string          `json:"mode,omitempty"`
	DraftAnswer       string          `json:"draftAnswer,omitempty"`
	ExternalEvidence  []EvidenceInput `json:"externalEvidence,omitempty"`
	IncludeSensitive  bool            `json:"includeSensitive,omitempty"`
	HumanApproved     bool            `json:"humanApproved,omitempty"`
	AllowMemoryUpdate bool            `json:"allowMemoryUpdate,omitempty"`
}

type VerificationResult struct {
	Run               models.VerificationRun        `json:"run"`
	Claims            []models.VerificationClaim    `json:"claims"`
	Evidence          []models.VerificationEvidence `json:"evidence"`
	UnsupportedClaims []models.VerificationClaim    `json:"unsupportedClaims"`
	ResearchQuestions []string                      `json:"researchQuestions"`
	Logs              []string                      `json:"logs"`
}

type Service interface {
	Answer(request AnswerRequest) (*VerificationResult, error)
	Runs() ([]models.VerificationRun, error)
	RunDetails(id uuid.UUID) (*VerificationResult, error)
}

type service struct {
	repo          Repository
	sourceService source.Service
	memoryService memory.Service
}

func NewService(repo Repository, sourceService source.Service, memoryService memory.Service) Service {
	return &service{repo: repo, sourceService: sourceService, memoryService: memoryService}
}

func DefaultService() Service {
	return NewService(DefaultRepository(), source.DefaultService(), memory.DefaultService())
}

func (s *service) Answer(request AnswerRequest) (*VerificationResult, error) {
	mode := normalizeMode(request.Mode)
	questions := researchQuestions(request.Question, request.ProjectKey)
	logs := []string{"converted request into research questions"}
	run, err := s.repo.CreateRun(&models.VerificationRun{
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
	answer := buildAnswer(request, mode, evidence)
	claims := decomposeClaims(run.ID, answer, mode, request)
	verifiedClaims := verifyClaims(claims, evidence, request, mode)
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
	if request.AllowMemoryUpdate {
		s.storeVerifiedMemory(request, run, verifiedClaims)
	}
	return &VerificationResult{
		Run:               *run,
		Claims:            verifiedClaims,
		Evidence:          evidence,
		UnsupportedClaims: unsupported,
		ResearchQuestions: questions,
		Logs:              append(logs, "verification status logged for every important claim"),
	}, nil
}

func (s *service) Runs() ([]models.VerificationRun, error) {
	return s.repo.FindRuns()
}

func (s *service) RunDetails(id uuid.UUID) (*VerificationResult, error) {
	claims, err := s.repo.FindClaims(id)
	if err != nil {
		return nil, err
	}
	evidence, err := s.repo.FindEvidence(id)
	if err != nil {
		return nil, err
	}
	return &VerificationResult{
		Claims:            claims,
		Evidence:          evidence,
		UnsupportedClaims: unsupportedClaims(claims),
		Logs:              []string{"loaded persisted verification details"},
	}, nil
}

func (s *service) collectEvidence(runID uuid.UUID, request AnswerRequest, questions []string, logs *[]string) []models.VerificationEvidence {
	evidence := []models.VerificationEvidence{}
	for _, question := range questions {
		result, err := s.sourceService.Search(source.SearchRequest{
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

func verifyClaims(claims []models.VerificationClaim, evidence []models.VerificationEvidence, request AnswerRequest, mode string) []models.VerificationClaim {
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
		if containsContradiction(claim.ClaimText, evidence) {
			claim.Status = StatusConflicting
			claim.NeedsReview = true
			claim.SupportExplanation = "supporting sources appear to disagree"
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
		_, _ = s.memoryService.Create(memory.CreateRequest{
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

func containsContradiction(claim string, evidence []models.VerificationEvidence) bool {
	lower := strings.ToLower(claim)
	for _, item := range evidence {
		snippet := strings.ToLower(item.Snippet)
		if containsAny(lower, "approved", "yes", "enabled") && containsAny(snippet, "rejected", "no", "disabled") {
			return true
		}
		if containsAny(lower, "rejected", "no", "disabled") && containsAny(snippet, "approved", "yes", "enabled") {
			return true
		}
	}
	return false
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
