package controlledlearning

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	maxIdentityLength   = 256
	maxIdentifierLength = 256
	maxSummaryLength    = 4000
	maxDetailLength     = 12000
	maxCollectionSize   = 100
	futureTolerance     = 5 * time.Minute
)

type Service struct {
	repository            Repository
	applicationRepository ApplicationRepository
	promoter              ProposalPromoter
	now                   func() time.Time
	newID                 func() string
}

func NewService(repository Repository, now func() time.Time, newID func() string) (*Service, error) {
	if err := validateRepository(repository); err != nil {
		return nil, err
	}
	if now == nil {
		now = time.Now
	}
	if newID == nil {
		newID = uuid.NewString
	}
	applicationRepository, _ := repository.(ApplicationRepository)
	return &Service{
		repository:            repository,
		applicationRepository: applicationRepository,
		promoter:              defaultProposalPromoter(applicationRepository, now),
		now:                   now,
		newID:                 newID,
	}, nil
}

func defaultProposalPromoter(
	repository ApplicationRepository,
	now func() time.Time,
) ProposalPromoter {
	if repository == nil {
		return nil
	}
	return NewLedgerPromoter(now)
}

func NewServiceWithPromoter(
	repository Repository,
	promoter ProposalPromoter,
	now func() time.Time,
	newID func() string,
) (*Service, error) {
	service, err := NewService(repository, now, newID)
	if err != nil {
		return nil, err
	}
	return service.WithPromoter(promoter)
}

// WithPromoter returns a configured copy so callers can assemble the service
// before concurrent use without mutating a live authorization boundary.
func (service *Service) WithPromoter(promoter ProposalPromoter) (*Service, error) {
	if service == nil {
		return nil, fmt.Errorf("controlled learning service is required")
	}
	if promoter == nil {
		return nil, ErrPromoterUnavailable
	}
	if err := validateRequired("controlled learning promoter id", promoter.ID(), maxIdentifierLength); err != nil {
		return nil, err
	}
	configured := *service
	configured.promoter = promoter
	return &configured, nil
}

func (service *Service) RecordOutcome(ctx context.Context, request RecordOutcomeRequest) (OutcomeRecord, error) {
	if err := ctx.Err(); err != nil {
		return OutcomeRecord{}, err
	}
	now := service.now().UTC()
	record, err := normalizeOutcomeRequest(request, now)
	if err != nil {
		return OutcomeRecord{}, err
	}
	record.ID = service.newID()
	record.ProtocolVersion = ProtocolVersion
	record.RecordedAt = now
	record.Reconciliation = reconcile(record)
	record.EvidenceDigest, err = outcomeDigest(record)
	if err != nil {
		return OutcomeRecord{}, err
	}
	return service.repository.CreateOutcome(ctx, record)
}

func (service *Service) Propose(ctx context.Context, request ProposeRequest) (LearningProposal, error) {
	if err := ctx.Err(); err != nil {
		return LearningProposal{}, err
	}
	proposal, err := normalizeProposalRequest(request)
	if err != nil {
		return LearningProposal{}, err
	}
	for _, evidenceID := range proposal.EvidenceIDs {
		evidence, err := service.repository.GetOutcome(ctx, proposal.OwnerIdentity, evidenceID)
		if err != nil {
			return LearningProposal{}, fmt.Errorf("load learning evidence %q: %w", evidenceID, err)
		}
		if err := verifyOutcomeIntegrity(evidence); err != nil {
			return LearningProposal{}, fmt.Errorf("verify learning evidence %q: %w", evidenceID, err)
		}
		if !eligibleEvidence(evidence) {
			return LearningProposal{}, fmt.Errorf("%w: %s", ErrUnsupportedEvidence, evidenceID)
		}
	}
	now := service.now().UTC()
	proposal.ID = service.newID()
	proposal.ProtocolVersion = ProtocolVersion
	proposal.Revision = 1
	proposal.ProtectedTarget = isProtectedTarget(proposal.Target) ||
		proposalSemanticallyImpactsAuthority(request)
	if proposal.ProtectedTarget {
		proposal.Status = ProposalGovernanceRequired
	} else {
		proposal.Status = ProposalReviewRequired
	}
	proposal.CreatedAt = now
	proposal.UpdatedAt = now
	proposal.ProposalDigest, err = proposalDigest(proposal)
	if err != nil {
		return LearningProposal{}, err
	}
	return service.repository.CreateProposal(ctx, proposal)
}

func (service *Service) Decide(ctx context.Context, request DecideRequest) (LearningProposal, error) {
	result, err := service.DecideAndApply(ctx, request)
	return result.Proposal, err
}

func (service *Service) decideWithoutApplication(
	ctx context.Context,
	proposal LearningProposal,
	request DecideRequest,
	nextStatus ProposalStatus,
) (LearningProposal, error) {
	now := service.now().UTC()
	decision, err := service.buildReviewDecision(proposal, request, "", now)
	if err != nil {
		return LearningProposal{}, err
	}
	return service.repository.DecideProposal(
		ctx,
		proposal.OwnerIdentity,
		proposal.ID,
		request.ExpectedRevision,
		decision,
		nextStatus,
	)
}

func (service *Service) buildReviewDecision(
	proposal LearningProposal,
	request DecideRequest,
	applicationID string,
	now time.Time,
) (ReviewDecision, error) {
	decision := ReviewDecision{
		ID:                  service.newID(),
		ProposalID:          proposal.ID,
		OwnerIdentity:       proposal.OwnerIdentity,
		ProposalRevision:    request.ExpectedRevision,
		Kind:                request.Kind,
		ActorIdentity:       strings.TrimSpace(request.ActorIdentity),
		HumanConfirmed:      true,
		Rationale:           strings.TrimSpace(request.Rationale),
		GovernanceReference: strings.TrimSpace(request.GovernanceReference),
		ProposalDigest:      proposal.ProposalDigest,
		ApplicationID:       strings.TrimSpace(applicationID),
		DecidedAt:           now,
	}
	var err error
	decision.DecisionDigest, err = reviewDecisionDigest(decision)
	return decision, err
}

func (service *Service) validateDecisionRequest(ctx context.Context, request DecideRequest) (LearningProposal, error) {
	if err := ctx.Err(); err != nil {
		return LearningProposal{}, err
	}
	if err := validateRequired("owner identity", request.OwnerIdentity, maxIdentityLength); err != nil {
		return LearningProposal{}, err
	}
	if err := validateRequired("proposal id", request.ProposalID, maxIdentifierLength); err != nil {
		return LearningProposal{}, err
	}
	if request.ExpectedRevision <= 0 {
		return LearningProposal{}, fmt.Errorf("expected proposal revision must be positive")
	}
	if err := validateRequired("actor identity", request.ActorIdentity, maxIdentityLength); err != nil {
		return LearningProposal{}, err
	}
	if strings.TrimSpace(request.ActorIdentity) != strings.TrimSpace(request.OwnerIdentity) {
		return LearningProposal{}, fmt.Errorf(
			"%w: decision actor must match the proposal owner",
			ErrOwnerScopeViolation,
		)
	}
	if !request.HumanConfirmed {
		return LearningProposal{}, fmt.Errorf("controlled learning decisions require explicit human confirmation")
	}
	if err := validateRequired("decision rationale", request.Rationale, maxDetailLength); err != nil {
		return LearningProposal{}, err
	}
	proposal, err := service.repository.GetProposal(ctx, request.OwnerIdentity, request.ProposalID)
	if err != nil {
		return LearningProposal{}, err
	}
	if err := verifyProposalIntegrity(proposal); err != nil {
		return LearningProposal{}, err
	}
	return proposal, nil
}

func reviewDecisionDigest(decision ReviewDecision) (string, error) {
	return digestValue(struct {
		ID                  string
		ProposalID          string
		OwnerIdentity       string
		ProposalRevision    int64
		Kind                DecisionKind
		ActorIdentity       string
		Rationale           string
		GovernanceReference string
		ProposalDigest      string
		ApplicationID       string
		DecidedAt           time.Time
	}{
		decision.ID, decision.ProposalID, decision.OwnerIdentity, decision.ProposalRevision,
		decision.Kind, decision.ActorIdentity, decision.Rationale, decision.GovernanceReference,
		decision.ProposalDigest, decision.ApplicationID, decision.DecidedAt,
	})
}

func verifyReviewDecisionIntegrity(decision ReviewDecision) error {
	expected, err := reviewDecisionDigest(decision)
	if err != nil {
		return err
	}
	if decision.DecisionDigest == "" || decision.DecisionDigest != expected {
		return ErrIntegrityViolation
	}
	return nil
}

func normalizeOutcomeRequest(request RecordOutcomeRequest, now time.Time) (OutcomeRecord, error) {
	if err := validateRequired("owner identity", request.OwnerIdentity, maxIdentityLength); err != nil {
		return OutcomeRecord{}, err
	}
	if err := validateRequired("idempotency key", request.IdempotencyKey, maxIdentifierLength); err != nil {
		return OutcomeRecord{}, err
	}
	if err := validateRequired("operation id", request.OperationID, maxIdentifierLength); err != nil {
		return OutcomeRecord{}, err
	}
	if err := validateRequired("outcome summary", request.Summary, maxSummaryLength); err != nil {
		return OutcomeRecord{}, err
	}
	if !validOutcomeStatus(request.Status) {
		return OutcomeRecord{}, fmt.Errorf("unsupported outcome status %q", request.Status)
	}
	if !validVerification(request.Verification) {
		return OutcomeRecord{}, fmt.Errorf("unsupported verification status %q", request.Verification)
	}
	if request.OccurredAt.IsZero() {
		return OutcomeRecord{}, fmt.Errorf("outcome occurred time is required")
	}
	if request.OccurredAt.After(now.Add(futureTolerance)) {
		return OutcomeRecord{}, fmt.Errorf("outcome occurred time is in the future")
	}
	if len(request.Sources) > maxCollectionSize || len(request.Criteria) > maxCollectionSize ||
		len(request.Metrics) > maxCollectionSize || len(request.Tags) > maxCollectionSize ||
		len(request.DomainPackIDs) > maxCollectionSize {
		return OutcomeRecord{}, fmt.Errorf("controlled learning collections may contain at most %d items", maxCollectionSize)
	}
	record := OutcomeRecord{
		OwnerIdentity:  strings.TrimSpace(request.OwnerIdentity),
		IdempotencyKey: strings.TrimSpace(request.IdempotencyKey),
		OperationID:    strings.TrimSpace(request.OperationID),
		ProjectKey:     strings.TrimSpace(request.ProjectKey),
		DomainPackIDs:  canonicalStrings(request.DomainPackIDs),
		Basis:          request.Basis,
		Status:         request.Status,
		Summary:        strings.TrimSpace(request.Summary),
		ActorIdentity:  strings.TrimSpace(request.ActorIdentity),
		HumanConfirmed: request.HumanConfirmed,
		Correction:     strings.TrimSpace(request.Correction),
		Verification:   request.Verification,
		Sources:        canonicalSources(request.Sources),
		Criteria:       canonicalCriteria(request.Criteria),
		Metrics:        canonicalMetrics(request.Metrics),
		Tags:           canonicalStrings(request.Tags),
		OccurredAt:     request.OccurredAt.UTC(),
	}
	if err := validateEvidenceBasis(record); err != nil {
		return OutcomeRecord{}, err
	}
	if err := validateSources(record.Sources, now); err != nil {
		return OutcomeRecord{}, err
	}
	if err := validateCriteria(record.Criteria, record.Sources); err != nil {
		return OutcomeRecord{}, err
	}
	if err := validateMetrics(record.Metrics); err != nil {
		return OutcomeRecord{}, err
	}
	return record, nil
}

func validateEvidenceBasis(record OutcomeRecord) error {
	switch record.Basis {
	case EvidenceVerifiedOutcome:
		if record.Verification != VerificationVerified &&
			record.Verification != VerificationSourceSupported &&
			record.Verification != VerificationTestPassed &&
			record.Verification != VerificationHumanApproved {
			return fmt.Errorf("%w: verified outcomes require substantive verification", ErrUnsupportedEvidence)
		}
		if len(record.Sources) == 0 {
			return fmt.Errorf("%w: verified outcomes require provenance", ErrUnsupportedEvidence)
		}
	case EvidenceHumanCorrection:
		if !record.HumanConfirmed || record.ActorIdentity == "" || record.Correction == "" ||
			record.Verification != VerificationHumanApproved {
			return fmt.Errorf("%w: corrections require an identified human, correction text, and human approval", ErrUnsupportedEvidence)
		}
		if record.ActorIdentity != record.OwnerIdentity {
			return fmt.Errorf(
				"%w: correction actor must match the outcome owner",
				ErrOwnerScopeViolation,
			)
		}
	default:
		return fmt.Errorf("%w: unsupported evidence basis %q", ErrUnsupportedEvidence, record.Basis)
	}
	return nil
}

func normalizeProposalRequest(request ProposeRequest) (LearningProposal, error) {
	if err := validateRequired("owner identity", request.OwnerIdentity, maxIdentityLength); err != nil {
		return LearningProposal{}, err
	}
	if err := validateRequired("idempotency key", request.IdempotencyKey, maxIdentifierLength); err != nil {
		return LearningProposal{}, err
	}
	if !validLearningMethod(request.Method) {
		return LearningProposal{}, fmt.Errorf("unsupported learning method %q", request.Method)
	}
	if !validTarget(request.Target) {
		return LearningProposal{}, fmt.Errorf("unsupported learning target %q", request.Target)
	}
	if request.Method == MethodPolicyVersioning &&
		!isProtectedTarget(request.Target) &&
		!proposalSemanticallyImpactsAuthority(request) {
		return LearningProposal{}, fmt.Errorf("policy versioning requires a protected policy target")
	}
	for name, value := range map[string]string{
		"title": request.Title, "hypothesis": request.Hypothesis, "proposed change": request.ProposedChange,
		"current version": request.CurrentVersion, "proposed version": request.ProposedVersion,
		"rollback plan": request.RollbackPlan, "evaluation plan": request.EvaluationPlan,
	} {
		limit := maxDetailLength
		if name == "title" || strings.Contains(name, "version") {
			limit = maxIdentifierLength
		}
		if err := validateRequired(name, value, limit); err != nil {
			return LearningProposal{}, err
		}
	}
	if strings.TrimSpace(request.CurrentVersion) == strings.TrimSpace(request.ProposedVersion) {
		return LearningProposal{}, fmt.Errorf("proposed version must differ from current version")
	}
	evidenceIDs := canonicalStrings(request.EvidenceIDs)
	if len(evidenceIDs) == 0 {
		return LearningProposal{}, fmt.Errorf("at least one learning evidence id is required")
	}
	if len(evidenceIDs) > maxCollectionSize {
		return LearningProposal{}, fmt.Errorf("at most %d evidence records may support a proposal", maxCollectionSize)
	}
	return LearningProposal{
		OwnerIdentity:   strings.TrimSpace(request.OwnerIdentity),
		IdempotencyKey:  strings.TrimSpace(request.IdempotencyKey),
		Method:          request.Method,
		Target:          request.Target,
		Title:           strings.TrimSpace(request.Title),
		Hypothesis:      strings.TrimSpace(request.Hypothesis),
		ProposedChange:  strings.TrimSpace(request.ProposedChange),
		CurrentVersion:  strings.TrimSpace(request.CurrentVersion),
		ProposedVersion: strings.TrimSpace(request.ProposedVersion),
		RollbackPlan:    strings.TrimSpace(request.RollbackPlan),
		EvaluationPlan:  strings.TrimSpace(request.EvaluationPlan),
		EvidenceIDs:     evidenceIDs,
	}, nil
}

func nextProposalStatus(proposal LearningProposal, request DecideRequest) (ProposalStatus, error) {
	switch request.Kind {
	case DecisionApprove:
		if proposal.ProtectedTarget {
			return "", ErrProtectedTarget
		}
		if proposal.Status != ProposalReviewRequired && proposal.Status != ProposalChangesRequested {
			return "", ErrInvalidStateChange
		}
		return ProposalApproved, nil
	case DecisionReject:
		if terminalProposalStatus(proposal.Status) {
			return "", ErrInvalidStateChange
		}
		return ProposalRejected, nil
	case DecisionRequestChanges:
		if proposal.ProtectedTarget || proposal.Status != ProposalReviewRequired {
			return "", ErrInvalidStateChange
		}
		return ProposalChangesRequested, nil
	case DecisionEscalateGovernance:
		if !proposal.ProtectedTarget || proposal.Status != ProposalGovernanceRequired {
			return "", ErrInvalidStateChange
		}
		if strings.TrimSpace(request.GovernanceReference) == "" {
			return "", fmt.Errorf("governance reference is required for protected learning targets")
		}
		return ProposalGovernanceReview, nil
	default:
		return "", fmt.Errorf("unsupported learning decision %q", request.Kind)
	}
}

func reconcile(record OutcomeRecord) Reconciliation {
	result := Reconciliation{Status: ReconciliationMatched}
	for _, criterion := range record.Criteria {
		if criterion.Passed {
			result.PassedCriteria++
		} else {
			result.FailedCriteria = append(result.FailedCriteria, criterion.ID)
		}
	}
	for _, metric := range record.Metrics {
		if metricDrifted(metric) {
			result.DriftSignals = append(result.DriftSignals, metric.Name)
		}
	}
	switch {
	case record.Status == OutcomeFailed || len(result.FailedCriteria) > 0:
		result.Status = ReconciliationDiverged
	case record.Status == OutcomePartial || len(result.DriftSignals) > 0:
		result.Status = ReconciliationPartial
	}
	methods := []LearningMethod{MethodOutcomeReconciliation, MethodAfterActionReview}
	if record.Basis == EvidenceHumanCorrection {
		methods = append(methods, MethodHumanCorrection, MethodDoubleLoop)
	}
	if result.Status == ReconciliationDiverged {
		methods = append(methods, MethodErrorAnalysis, MethodRootCauseAnalysis, MethodFiveWhys)
	}
	if len(result.DriftSignals) > 0 {
		methods = append(methods, MethodDriftDetection)
	}
	result.SuggestedMethods = uniqueMethods(methods)
	return result
}

func metricDrifted(metric MetricResult) bool {
	switch metric.Direction {
	case MetricExact:
		return math.Abs(metric.Actual-metric.Expected) > metric.Tolerance
	case MetricAtLeast:
		return metric.Actual+metric.Tolerance < metric.Expected
	case MetricAtMost:
		return metric.Actual-metric.Tolerance > metric.Expected
	default:
		return true
	}
}

func outcomeDigest(record OutcomeRecord) (string, error) {
	copy := cloneOutcome(record)
	copy.ID = ""
	copy.RecordedAt = time.Time{}
	copy.EvidenceDigest = ""
	return digestValue(copy)
}

func proposalDigest(proposal LearningProposal) (string, error) {
	copy := cloneProposal(proposal)
	copy.ID = ""
	copy.Revision = 0
	copy.Status = ""
	copy.CreatedAt = time.Time{}
	copy.UpdatedAt = time.Time{}
	copy.ProposalDigest = ""
	return digestValue(copy)
}

func verifyOutcomeIntegrity(record OutcomeRecord) error {
	expected, err := outcomeDigest(record)
	if err != nil {
		return err
	}
	if record.EvidenceDigest == "" || record.EvidenceDigest != expected {
		return ErrIntegrityViolation
	}
	return nil
}

func verifyProposalIntegrity(proposal LearningProposal) error {
	expected, err := proposalDigest(proposal)
	if err != nil {
		return err
	}
	if proposal.ProposalDigest == "" || proposal.ProposalDigest != expected {
		return ErrIntegrityViolation
	}
	return nil
}

func eligibleEvidence(record OutcomeRecord) bool {
	if record.ProtocolVersion != ProtocolVersion || record.EvidenceDigest == "" {
		return false
	}
	switch record.Basis {
	case EvidenceHumanCorrection:
		return record.HumanConfirmed && record.ActorIdentity != "" &&
			record.Correction != "" && record.Verification == VerificationHumanApproved
	case EvidenceVerifiedOutcome:
		return len(record.Sources) > 0 &&
			(record.Verification == VerificationVerified ||
				record.Verification == VerificationSourceSupported ||
				record.Verification == VerificationTestPassed ||
				record.Verification == VerificationHumanApproved)
	default:
		return false
	}
}

func validateSources(sources []SourceReference, now time.Time) error {
	seen := map[string]struct{}{}
	for _, source := range sources {
		if err := validateRequired("source id", source.ID, maxIdentifierLength); err != nil {
			return err
		}
		if err := validateRequired("source kind", source.Kind, maxIdentifierLength); err != nil {
			return err
		}
		if err := validateRequired("source URI", source.URI, maxDetailLength); err != nil {
			return err
		}
		if err := validateSourceURI(source.URI); err != nil {
			return fmt.Errorf("source %q: %w", source.ID, err)
		}
		if source.RetrievedAt.IsZero() || source.RetrievedAt.After(now.Add(futureTolerance)) {
			return fmt.Errorf("source %q requires a valid retrieval time", source.ID)
		}
		if _, exists := seen[source.ID]; exists {
			return fmt.Errorf("duplicate source id %q", source.ID)
		}
		seen[source.ID] = struct{}{}
	}
	return nil
}

func validateCriteria(criteria []CriterionResult, sources []SourceReference) error {
	sourceIDs := map[string]struct{}{}
	for _, source := range sources {
		sourceIDs[source.ID] = struct{}{}
	}
	seen := map[string]struct{}{}
	for _, criterion := range criteria {
		if err := validateRequired("criterion id", criterion.ID, maxIdentifierLength); err != nil {
			return err
		}
		if err := validateRequired("criterion description", criterion.Description, maxSummaryLength); err != nil {
			return err
		}
		if _, exists := seen[criterion.ID]; exists {
			return fmt.Errorf("duplicate criterion id %q", criterion.ID)
		}
		seen[criterion.ID] = struct{}{}
		for _, sourceID := range criterion.SourceIDs {
			if _, exists := sourceIDs[sourceID]; !exists {
				return fmt.Errorf("criterion %q references unknown source %q", criterion.ID, sourceID)
			}
		}
	}
	return nil
}

func validateMetrics(metrics []MetricResult) error {
	seen := map[string]struct{}{}
	for _, metric := range metrics {
		if err := validateRequired("metric name", metric.Name, maxIdentifierLength); err != nil {
			return err
		}
		if metric.Direction != MetricExact && metric.Direction != MetricAtLeast && metric.Direction != MetricAtMost {
			return fmt.Errorf("metric %q has unsupported direction %q", metric.Name, metric.Direction)
		}
		if math.IsNaN(metric.Expected) || math.IsInf(metric.Expected, 0) ||
			math.IsNaN(metric.Actual) || math.IsInf(metric.Actual, 0) ||
			math.IsNaN(metric.Tolerance) || math.IsInf(metric.Tolerance, 0) || metric.Tolerance < 0 {
			return fmt.Errorf("metric %q contains invalid numbers", metric.Name)
		}
		if _, exists := seen[metric.Name]; exists {
			return fmt.Errorf("duplicate metric %q", metric.Name)
		}
		seen[metric.Name] = struct{}{}
	}
	return nil
}

func validOutcomeStatus(status OutcomeStatus) bool {
	return status == OutcomeSucceeded || status == OutcomePartial ||
		status == OutcomeFailed || status == OutcomeCorrected
}

func validVerification(status VerificationStatus) bool {
	return status == VerificationVerified || status == VerificationSourceSupported ||
		status == VerificationSchemaValidated || status == VerificationTestPassed ||
		status == VerificationHumanApproved
}

func validTarget(target TargetKind) bool {
	switch target {
	case TargetRecommendation, TargetSkill, TargetReusablePlan, TargetPrompt,
		TargetPreference, TargetEvaluation, TargetExperiment, TargetPlanningEstimateCalibration,
		TargetConstitution, TargetPermission, TargetSafetyBoundary,
		TargetApprovalPolicy, TargetAutonomyPolicy, TargetProviderBudget,
		TargetMandate, TargetExecutionPolicy:
		return true
	default:
		return false
	}
}

func isProtectedTarget(target TargetKind) bool {
	return target == TargetConstitution || target == TargetPermission ||
		target == TargetSafetyBoundary || target == TargetApprovalPolicy ||
		target == TargetAutonomyPolicy || target == TargetProviderBudget ||
		target == TargetMandate || target == TargetExecutionPolicy
}

func proposalSemanticallyImpactsAuthority(request ProposeRequest) bool {
	text := strings.ToLower(strings.Join([]string{
		request.Title,
		request.Hypothesis,
		request.ProposedChange,
	}, " "))
	for _, phrase := range []string{
		"constitution",
		"permission",
		"tool allowlist",
		"folder allowlist",
		"denylist",
		"whitelist",
		"safety boundary",
		"secret redaction",
		"emergency stop",
		"kill switch",
		"approval policy",
		"approval gate",
		"require approval",
		"without approval",
		"bypass approval",
		"autonomy policy",
		"autonomy level",
		"autonomous execution",
		"provider budget",
		"paid budget",
		"daily budget",
		"spending limit",
		"cost limit",
		"paid usage",
		"standing mandate",
		"delegated mandate",
		"execution mandate",
		"execution policy",
		"execution authorization",
		"execution authority",
		"grant authority",
		"revoke authority",
		"broaden authority",
		"expand authority",
		"execute without",
	} {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

func validLearningMethod(method LearningMethod) bool {
	switch method {
	case MethodPDCA, MethodOODA, MethodDoubleLoop, MethodTripleLoop,
		MethodAfterActionReview, MethodBlamelessPostmortem, MethodBayesianUpdate,
		MethodCaseBasedLearning, MethodExperienceReplay, MethodReflection,
		MethodErrorAnalysis, MethodRootCauseAnalysis, MethodFiveWhys, MethodFishbone,
		MethodDMAIC, MethodHypothesisDriven, MethodMultiArmedBandit, MethodSafeExperiment,
		MethodShadowDeployment, MethodChampionChallenger, MethodSkillMining,
		MethodReusablePlanExtraction, MethodPromptVersioning, MethodPolicyVersioning,
		MethodEvalDriven, MethodPreferenceLearning,
		MethodFeedbackLearning, MethodHumanCorrection, MethodDriftDetection,
		MethodOutcomeReconciliation:
		return true
	default:
		return false
	}
}

func terminalProposalStatus(status ProposalStatus) bool {
	return status == ProposalApproved || status == ProposalRejected ||
		status == ProposalGovernanceReview
}

func canonicalStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func canonicalSources(values []SourceReference) []SourceReference {
	result := append([]SourceReference(nil), values...)
	for index := range result {
		result[index].ID = strings.TrimSpace(result[index].ID)
		result[index].Kind = strings.TrimSpace(result[index].Kind)
		result[index].URI = strings.TrimSpace(result[index].URI)
		result[index].ContentHash = strings.TrimSpace(result[index].ContentHash)
		result[index].RetrievedAt = result[index].RetrievedAt.UTC()
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func canonicalCriteria(values []CriterionResult) []CriterionResult {
	result := make([]CriterionResult, len(values))
	for index := range values {
		result[index] = values[index]
		result[index].ID = strings.TrimSpace(result[index].ID)
		result[index].Description = strings.TrimSpace(result[index].Description)
		result[index].SourceIDs = canonicalStrings(result[index].SourceIDs)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func canonicalMetrics(values []MetricResult) []MetricResult {
	result := append([]MetricResult(nil), values...)
	for index := range result {
		result[index].Name = strings.TrimSpace(result[index].Name)
		result[index].Unit = strings.TrimSpace(result[index].Unit)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func uniqueMethods(values []LearningMethod) []LearningMethod {
	seen := map[LearningMethod]struct{}{}
	result := make([]LearningMethod, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

// SupportedLearningMethods returns the complete section-37 method vocabulary.
func SupportedLearningMethods() []LearningMethod {
	return []LearningMethod{
		MethodPDCA, MethodOODA, MethodDoubleLoop, MethodTripleLoop,
		MethodAfterActionReview, MethodBlamelessPostmortem, MethodBayesianUpdate,
		MethodCaseBasedLearning, MethodExperienceReplay, MethodReflection,
		MethodErrorAnalysis, MethodRootCauseAnalysis, MethodFiveWhys, MethodFishbone,
		MethodDMAIC, MethodHypothesisDriven, MethodMultiArmedBandit, MethodSafeExperiment,
		MethodShadowDeployment, MethodChampionChallenger, MethodSkillMining,
		MethodReusablePlanExtraction, MethodPromptVersioning, MethodPolicyVersioning,
		MethodEvalDriven, MethodPreferenceLearning, MethodFeedbackLearning,
		MethodHumanCorrection, MethodDriftDetection, MethodOutcomeReconciliation,
	}
}

func validateSourceURI(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" {
		return fmt.Errorf("source URI must be absolute")
	}
	if parsed.User != nil {
		return fmt.Errorf("source URI must not contain credentials")
	}
	for key := range parsed.Query() {
		lower := strings.ToLower(key)
		for _, fragment := range []string{"token", "key", "secret", "password", "credential", "signature", "auth"} {
			if strings.Contains(lower, fragment) {
				return fmt.Errorf("source URI must not contain credential query parameters")
			}
		}
	}
	return nil
}

func validateRequired(name, value string, maxRunes int) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > maxRunes {
		return fmt.Errorf("%s exceeds %d characters", name, maxRunes)
	}
	return nil
}
