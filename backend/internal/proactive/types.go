package proactive

import "time"

const ContractVersion = 1

type SignalType string

const (
	SignalDeadline            SignalType = "deadline"
	SignalCommitment          SignalType = "commitment"
	SignalWaitingState        SignalType = "waiting_state"
	SignalStaleWork           SignalType = "stale_work"
	SignalSourceChange        SignalType = "source_change"
	SignalRecurringObligation SignalType = "recurring_obligation"
	SignalCapacityConstraint  SignalType = "capacity_constraint"
	SignalReviewQueue         SignalType = "review_queue"
)

type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

type Sensitivity string

const (
	SensitivityStandard   Sensitivity = "standard"
	SensitivitySensitive  Sensitivity = "sensitive"
	SensitivityRestricted Sensitivity = "restricted"
)

type VerificationStatus string

const (
	VerificationVerified        VerificationStatus = "verified"
	VerificationSourceSupported VerificationStatus = "source_supported"
	VerificationUncertain       VerificationStatus = "uncertain"
	VerificationConflicting     VerificationStatus = "conflicting"
	VerificationUnsupported     VerificationStatus = "unsupported"
)

type DecisionDomain string

const (
	DomainGeneral   DecisionDomain = "general"
	DomainLegal     DecisionDomain = "legal"
	DomainMedical   DecisionDomain = "medical"
	DomainFinancial DecisionDomain = "financial"
)

type ResponsibleParty string

const (
	ResponsibleRobert   ResponsibleParty = "robert"
	ResponsibleHAI      ResponsibleParty = "hai"
	ResponsibleExternal ResponsibleParty = "external"
)

type ProposalStatus string

const (
	StatusProposed  ProposalStatus = "proposed"
	StatusAccepted  ProposalStatus = "accepted"
	StatusDismissed ProposalStatus = "dismissed"
	StatusSnoozed   ProposalStatus = "snoozed"
	StatusExpired   ProposalStatus = "expired"
	StatusResolved  ProposalStatus = "resolved"
)

func (s ProposalStatus) Terminal() bool {
	return s == StatusDismissed || s == StatusExpired || s == StatusResolved
}

type SuppressionReason string

const (
	SuppressionNone          SuppressionReason = ""
	SuppressionRuleDisabled  SuppressionReason = "rule_disabled"
	SuppressionTypeMismatch  SuppressionReason = "type_mismatch"
	SuppressionResolved      SuppressionReason = "already_resolved"
	SuppressionStale         SuppressionReason = "stale_source"
	SuppressionUncertain     SuppressionReason = "uncertain_source"
	SuppressionSensitive     SuppressionReason = "sensitive_signal"
	SuppressionLowConfidence SuppressionReason = "low_confidence"
	SuppressionCooldown      SuppressionReason = "cooldown"
)

type FeedbackOutcome string

const (
	FeedbackUseful    FeedbackOutcome = "useful"
	FeedbackNotUseful FeedbackOutcome = "not_useful"
)

type ScoreComponentName string

const (
	ComponentRelevance  ScoreComponentName = "relevance"
	ComponentUrgency    ScoreComponentName = "urgency"
	ComponentImportance ScoreComponentName = "importance"
	ComponentRisk       ScoreComponentName = "risk"
)

type SourceReference struct {
	ID           string             `json:"id"`
	Kind         string             `json:"kind"`
	Locator      string             `json:"locator"`
	ContentHash  string             `json:"contentHash"`
	ObservedAt   time.Time          `json:"observedAt"`
	RetrievedAt  time.Time          `json:"retrievedAt"`
	Verification VerificationStatus `json:"verification"`
}

type Signal struct {
	ContractVersion int               `json:"contractVersion"`
	ID              string            `json:"id"`
	IdempotencyKey  string            `json:"idempotencyKey"`
	OwnerIdentity   string            `json:"ownerIdentity"`
	Type            SignalType        `json:"type"`
	OpenLoopKey     string            `json:"openLoopKey"`
	Title           string            `json:"title"`
	Summary         string            `json:"summary"`
	Responsible     ResponsibleParty  `json:"responsible"`
	Domain          DecisionDomain    `json:"domain"`
	Risk            RiskLevel         `json:"risk"`
	Sensitivity     Sensitivity       `json:"sensitivity"`
	Confidence      float64           `json:"confidence"`
	Relevance       float64           `json:"relevance"`
	Importance      float64           `json:"importance"`
	OccurredAt      time.Time         `json:"occurredAt"`
	DueAt           *time.Time        `json:"dueAt,omitempty"`
	ResolvedAt      *time.Time        `json:"resolvedAt,omitempty"`
	Sources         []SourceReference `json:"sources"`
	Attributes      map[string]string `json:"attributes,omitempty"`
}

type ScoreWeights struct {
	Relevance  float64 `json:"relevance"`
	Urgency    float64 `json:"urgency"`
	Importance float64 `json:"importance"`
	Risk       float64 `json:"risk"`
}

func DefaultScoreWeights() ScoreWeights {
	return ScoreWeights{Relevance: 0.30, Urgency: 0.30, Importance: 0.25, Risk: 0.15}
}

type QuietHours struct {
	Enabled     bool   `json:"enabled"`
	StartMinute int    `json:"startMinute"`
	EndMinute   int    `json:"endMinute"`
	TimeZone    string `json:"timeZone"`
}

type RetryPolicy struct {
	Intervals      []time.Duration `json:"intervals"`
	MaxAttempts    int             `json:"maxAttempts"`
	MaxEscalations int             `json:"maxEscalations"`
}

type TriggerRule struct {
	ContractVersion   int           `json:"contractVersion"`
	ID                string        `json:"id"`
	OwnerIdentity     string        `json:"ownerIdentity"`
	Version           uint64        `json:"version"`
	Digest            string        `json:"digest"`
	Name              string        `json:"name"`
	Enabled           bool          `json:"enabled"`
	SignalTypes       []SignalType  `json:"signalTypes"`
	MinimumConfidence float64       `json:"minimumConfidence"`
	MaximumSourceAge  time.Duration `json:"maximumSourceAge"`
	Cooldown          time.Duration `json:"cooldown"`
	ProposalTTL       time.Duration `json:"proposalTtl"`
	QuietHours        QuietHours    `json:"quietHours"`
	Weights           ScoreWeights  `json:"weights"`
	Retry             RetryPolicy   `json:"retry"`
	CreatedAt         time.Time     `json:"createdAt"`
	UpdatedAt         time.Time     `json:"updatedAt"`
}

type EvidenceSnapshot struct {
	SourceReference
	Age     time.Duration `json:"age"`
	IsFresh bool          `json:"isFresh"`
}

type ScoreComponent struct {
	Name         ScoreComponentName `json:"name"`
	Value        float64            `json:"value"`
	Weight       float64            `json:"weight"`
	Contribution float64            `json:"contribution"`
	Reason       string             `json:"reason"`
}

type ScoreExplanation struct {
	Total      float64          `json:"total"`
	Components []ScoreComponent `json:"components"`
	Summary    string           `json:"summary"`
}

type RecommendedAction struct {
	Kind           string `json:"kind"`
	Label          string `json:"label"`
	Description    string `json:"description"`
	ExternalEffect bool   `json:"externalEffect"`
}

type Proposal struct {
	ContractVersion  int                `json:"contractVersion"`
	ID               string             `json:"id"`
	OwnerIdentity    string             `json:"ownerIdentity"`
	SignalID         string             `json:"signalId"`
	SignalDigest     string             `json:"signalDigest"`
	IdempotencyKey   string             `json:"idempotencyKey"`
	OpenLoopKey      string             `json:"openLoopKey"`
	RuleID           string             `json:"ruleId"`
	RuleVersion      uint64             `json:"ruleVersion"`
	RuleDigest       string             `json:"ruleDigest"`
	Title            string             `json:"title"`
	Summary          string             `json:"summary"`
	Status           ProposalStatus     `json:"status"`
	Responsible      ResponsibleParty   `json:"responsible"`
	Risk             RiskLevel          `json:"risk"`
	Domain           DecisionDomain     `json:"domain"`
	ApprovalRequired bool               `json:"approvalRequired"`
	ApprovalReason   string             `json:"approvalReason,omitempty"`
	ExecutionAllowed bool               `json:"executionAllowed"`
	Action           RecommendedAction  `json:"action"`
	Evidence         []EvidenceSnapshot `json:"evidence"`
	Score            ScoreExplanation   `json:"score"`
	CreatedAt        time.Time          `json:"createdAt"`
	UpdatedAt        time.Time          `json:"updatedAt"`
	ExpiresAt        time.Time          `json:"expiresAt"`
	NotifyAfter      time.Time          `json:"notifyAfter"`
	SnoozedUntil     *time.Time         `json:"snoozedUntil,omitempty"`
	ReviewAttempts   int                `json:"reviewAttempts"`
	EscalationCount  int                `json:"escalationCount"`
	NextReviewAt     *time.Time         `json:"nextReviewAt,omitempty"`
	Revision         uint64             `json:"revision"`
}

type EvaluationResult struct {
	Proposal         *Proposal         `json:"proposal,omitempty"`
	Suppressed       bool              `json:"suppressed"`
	Suppression      SuppressionReason `json:"suppression,omitempty"`
	Reason           string            `json:"reason"`
	IdempotentReplay bool              `json:"idempotentReplay"`
	Deferred         bool              `json:"deferred"`
}

type ProposalFilter struct {
	Statuses []ProposalStatus
	Limit    int
}

type Feedback struct {
	OwnerIdentity string             `json:"ownerIdentity"`
	ProposalID    string             `json:"proposalId"`
	Outcome       FeedbackOutcome    `json:"outcome"`
	Component     ScoreComponentName `json:"component"`
	OccurredAt    time.Time          `json:"occurredAt"`
}
