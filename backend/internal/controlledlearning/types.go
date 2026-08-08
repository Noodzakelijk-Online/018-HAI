package controlledlearning

import "time"

const ProtocolVersion = "1.0.0"

type EvidenceBasis string

const (
	EvidenceVerifiedOutcome EvidenceBasis = "verified_outcome"
	EvidenceHumanCorrection EvidenceBasis = "human_correction"
)

type VerificationStatus string

const (
	VerificationVerified        VerificationStatus = "verified"
	VerificationSourceSupported VerificationStatus = "source_supported"
	VerificationSchemaValidated VerificationStatus = "schema_validated"
	VerificationTestPassed      VerificationStatus = "test_passed"
	VerificationHumanApproved   VerificationStatus = "human_approved"
)

type OutcomeStatus string

const (
	OutcomeSucceeded OutcomeStatus = "succeeded"
	OutcomePartial   OutcomeStatus = "partial"
	OutcomeFailed    OutcomeStatus = "failed"
	OutcomeCorrected OutcomeStatus = "corrected"
)

type MetricDirection string

const (
	MetricExact   MetricDirection = "exact"
	MetricAtLeast MetricDirection = "at_least"
	MetricAtMost  MetricDirection = "at_most"
)

type SourceReference struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	URI         string    `json:"uri"`
	RetrievedAt time.Time `json:"retrievedAt"`
	ContentHash string    `json:"contentHash,omitempty"`
}

type CriterionResult struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Passed      bool     `json:"passed"`
	SourceIDs   []string `json:"sourceIds,omitempty"`
}

type MetricResult struct {
	Name      string          `json:"name"`
	Expected  float64         `json:"expected"`
	Actual    float64         `json:"actual"`
	Tolerance float64         `json:"tolerance"`
	Direction MetricDirection `json:"direction"`
	Unit      string          `json:"unit,omitempty"`
}

type ReconciliationStatus string

const (
	ReconciliationMatched  ReconciliationStatus = "matched"
	ReconciliationPartial  ReconciliationStatus = "partial"
	ReconciliationDiverged ReconciliationStatus = "diverged"
)

type LearningMethod string

const (
	MethodPDCA                   LearningMethod = "pdca"
	MethodOODA                   LearningMethod = "ooda"
	MethodDoubleLoop             LearningMethod = "double_loop"
	MethodTripleLoop             LearningMethod = "triple_loop"
	MethodAfterActionReview      LearningMethod = "after_action_review"
	MethodBlamelessPostmortem    LearningMethod = "blameless_postmortem"
	MethodBayesianUpdate         LearningMethod = "bayesian_update"
	MethodCaseBasedLearning      LearningMethod = "case_based_learning"
	MethodExperienceReplay       LearningMethod = "experience_replay"
	MethodReflection             LearningMethod = "reflection"
	MethodErrorAnalysis          LearningMethod = "error_analysis"
	MethodRootCauseAnalysis      LearningMethod = "root_cause_analysis"
	MethodFiveWhys               LearningMethod = "five_whys"
	MethodFishbone               LearningMethod = "fishbone"
	MethodDMAIC                  LearningMethod = "dmaic"
	MethodHypothesisDriven       LearningMethod = "hypothesis_driven"
	MethodMultiArmedBandit       LearningMethod = "multi_armed_bandit"
	MethodSafeExperiment         LearningMethod = "safe_experiment"
	MethodShadowDeployment       LearningMethod = "shadow_deployment"
	MethodChampionChallenger     LearningMethod = "champion_challenger"
	MethodSkillMining            LearningMethod = "skill_mining"
	MethodReusablePlanExtraction LearningMethod = "reusable_plan_extraction"
	MethodPromptVersioning       LearningMethod = "prompt_versioning"
	MethodPolicyVersioning       LearningMethod = "policy_versioning"
	MethodEvalDriven             LearningMethod = "eval_driven"
	MethodPreferenceLearning     LearningMethod = "preference_learning"
	MethodFeedbackLearning       LearningMethod = "feedback_learning"
	MethodHumanCorrection        LearningMethod = "human_correction"
	MethodDriftDetection         LearningMethod = "drift_detection"
	MethodOutcomeReconciliation  LearningMethod = "outcome_reconciliation"
)

type Reconciliation struct {
	Status           ReconciliationStatus `json:"status"`
	PassedCriteria   int                  `json:"passedCriteria"`
	FailedCriteria   []string             `json:"failedCriteria,omitempty"`
	DriftSignals     []string             `json:"driftSignals,omitempty"`
	SuggestedMethods []LearningMethod     `json:"suggestedMethods"`
}

type OutcomeRecord struct {
	ID              string             `json:"id"`
	ProtocolVersion string             `json:"protocolVersion"`
	OwnerIdentity   string             `json:"ownerIdentity"`
	IdempotencyKey  string             `json:"idempotencyKey"`
	OperationID     string             `json:"operationId"`
	ProjectKey      string             `json:"projectKey,omitempty"`
	DomainPackIDs   []string           `json:"domainPackIds,omitempty"`
	Basis           EvidenceBasis      `json:"basis"`
	Status          OutcomeStatus      `json:"status"`
	Summary         string             `json:"summary"`
	ActorIdentity   string             `json:"actorIdentity,omitempty"`
	HumanConfirmed  bool               `json:"humanConfirmed"`
	Correction      string             `json:"correction,omitempty"`
	Verification    VerificationStatus `json:"verification"`
	Sources         []SourceReference  `json:"sources,omitempty"`
	Criteria        []CriterionResult  `json:"criteria,omitempty"`
	Metrics         []MetricResult     `json:"metrics,omitempty"`
	Tags            []string           `json:"tags,omitempty"`
	Reconciliation  Reconciliation     `json:"reconciliation"`
	EvidenceDigest  string             `json:"evidenceDigest"`
	OccurredAt      time.Time          `json:"occurredAt"`
	RecordedAt      time.Time          `json:"recordedAt"`
}

type RecordOutcomeRequest struct {
	OwnerIdentity  string
	IdempotencyKey string
	OperationID    string
	ProjectKey     string
	DomainPackIDs  []string
	Basis          EvidenceBasis
	Status         OutcomeStatus
	Summary        string
	ActorIdentity  string
	HumanConfirmed bool
	Correction     string
	Verification   VerificationStatus
	Sources        []SourceReference
	Criteria       []CriterionResult
	Metrics        []MetricResult
	Tags           []string
	OccurredAt     time.Time
}

type TargetKind string

const (
	TargetRecommendation              TargetKind = "recommendation"
	TargetSkill                       TargetKind = "skill"
	TargetReusablePlan                TargetKind = "reusable_plan"
	TargetPrompt                      TargetKind = "prompt"
	TargetPreference                  TargetKind = "preference"
	TargetEvaluation                  TargetKind = "evaluation"
	TargetExperiment                  TargetKind = "experiment"
	TargetPlanningEstimateCalibration TargetKind = "planning_estimate_calibration"
	TargetEstimateCalibration                    = TargetPlanningEstimateCalibration

	TargetConstitution    TargetKind = "constitution"
	TargetPermission      TargetKind = "permission"
	TargetSafetyBoundary  TargetKind = "safety_boundary"
	TargetApprovalPolicy  TargetKind = "approval_policy"
	TargetAutonomyPolicy  TargetKind = "autonomy_policy"
	TargetProviderBudget  TargetKind = "provider_budget"
	TargetMandate         TargetKind = "mandate"
	TargetExecutionPolicy TargetKind = "execution_policy"
)

type ProposalStatus string

const (
	ProposalReviewRequired     ProposalStatus = "review_required"
	ProposalGovernanceRequired ProposalStatus = "governance_required"
	ProposalGovernanceReview   ProposalStatus = "governance_review"
	ProposalApproved           ProposalStatus = "approved"
	ProposalRejected           ProposalStatus = "rejected"
	ProposalChangesRequested   ProposalStatus = "changes_requested"
)

type LearningProposal struct {
	ID              string         `json:"id"`
	ProtocolVersion string         `json:"protocolVersion"`
	OwnerIdentity   string         `json:"ownerIdentity"`
	IdempotencyKey  string         `json:"idempotencyKey"`
	Revision        int64          `json:"revision"`
	Status          ProposalStatus `json:"status"`
	Method          LearningMethod `json:"method"`
	Target          TargetKind     `json:"target"`
	ProtectedTarget bool           `json:"protectedTarget"`
	Title           string         `json:"title"`
	Hypothesis      string         `json:"hypothesis"`
	ProposedChange  string         `json:"proposedChange"`
	CurrentVersion  string         `json:"currentVersion"`
	ProposedVersion string         `json:"proposedVersion"`
	RollbackPlan    string         `json:"rollbackPlan"`
	EvaluationPlan  string         `json:"evaluationPlan"`
	EvidenceIDs     []string       `json:"evidenceIds"`
	ProposalDigest  string         `json:"proposalDigest"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
}

type ProposeRequest struct {
	OwnerIdentity   string
	IdempotencyKey  string
	Method          LearningMethod
	Target          TargetKind
	Title           string
	Hypothesis      string
	ProposedChange  string
	CurrentVersion  string
	ProposedVersion string
	RollbackPlan    string
	EvaluationPlan  string
	EvidenceIDs     []string
}

type DecisionKind string

const (
	DecisionApprove            DecisionKind = "approve"
	DecisionReject             DecisionKind = "reject"
	DecisionRequestChanges     DecisionKind = "request_changes"
	DecisionEscalateGovernance DecisionKind = "escalate_governance"
)

type ReviewDecision struct {
	ID                  string       `json:"id"`
	ProposalID          string       `json:"proposalId"`
	OwnerIdentity       string       `json:"ownerIdentity"`
	ProposalRevision    int64        `json:"proposalRevision"`
	Kind                DecisionKind `json:"kind"`
	ActorIdentity       string       `json:"actorIdentity"`
	HumanConfirmed      bool         `json:"humanConfirmed"`
	Rationale           string       `json:"rationale"`
	GovernanceReference string       `json:"governanceReference,omitempty"`
	ProposalDigest      string       `json:"proposalDigest"`
	ApplicationID       string       `json:"applicationId,omitempty"`
	DecisionDigest      string       `json:"decisionDigest"`
	DecidedAt           time.Time    `json:"decidedAt"`
}

type DecideRequest struct {
	OwnerIdentity       string
	ProposalID          string
	ExpectedRevision    int64
	IdempotencyKey      string
	Kind                DecisionKind
	ActorIdentity       string
	HumanConfirmed      bool
	Rationale           string
	GovernanceReference string
}

type OutcomeQuery struct {
	OwnerIdentity string
	OperationID   string
	Limit         int
}

type ProposalQuery struct {
	OwnerIdentity string
	Status        ProposalStatus
	Limit         int
}
