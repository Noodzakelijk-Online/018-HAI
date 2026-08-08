// Package outcomeevaluation evaluates longitudinal user outcomes without
// granting authority to execute actions or mutate operational policy.
package outcomeevaluation

import (
	"errors"
	"time"

	"automation-hub-backend/internal/lifeontology"
)

const SchemaVersion = "1.0.0"

var (
	ErrInvalidInput        = errors.New("invalid outcome evaluation input")
	ErrScopeViolation      = errors.New("owner or workspace scope violation")
	ErrSecretMaterial      = errors.New("secret material is not accepted")
	ErrInvalidTimeWindow   = errors.New("invalid longitudinal time window")
	ErrMissingProvenance   = errors.New("verified evidence requires provenance")
	ErrIntegrityViolation  = errors.New("outcome evaluation integrity violation")
	ErrNotFound            = errors.New("outcome evaluation record not found")
	ErrRevisionConflict    = errors.New("outcome evaluation revision conflict")
	ErrIdempotencyConflict = errors.New("outcome evaluation idempotency conflict")
)

type Scope struct {
	OwnerID     string `json:"ownerId"`
	WorkspaceID string `json:"workspaceId"`
}

type LongitudinalWindow struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type DesiredDirection string

const (
	DirectionHigher   DesiredDirection = "higher"
	DirectionLower    DesiredDirection = "lower"
	DirectionMaintain DesiredDirection = "maintain"
)

type VerificationStatus string

const (
	VerificationUnverified      VerificationStatus = "unverified"
	VerificationUserConfirmed   VerificationStatus = "user_confirmed"
	VerificationSourceSupported VerificationStatus = "source_supported"
	VerificationVerified        VerificationStatus = "verified"
	VerificationDisputed        VerificationStatus = "disputed"
)

type SourceStatus string

const (
	SourceUnreviewed SourceStatus = "unreviewed"
	SourceSupported  SourceStatus = "supported"
	SourceVerified   SourceStatus = "verified"
	SourceDisputed   SourceStatus = "disputed"
)

type SourceReference struct {
	ID            string       `json:"id"`
	URI           string       `json:"uri"`
	ContentDigest string       `json:"contentDigest,omitempty"`
	RetrievedAt   time.Time    `json:"retrievedAt"`
	Status        SourceStatus `json:"status"`
}

type AttributionMethod string

const (
	AttributionUnknown         AttributionMethod = "unknown"
	AttributionUserReport      AttributionMethod = "user_report"
	AttributionCorrelation     AttributionMethod = "correlation"
	AttributionControlledStudy AttributionMethod = "controlled_study"
	AttributionModelEstimate   AttributionMethod = "model_estimate"
)

// Attribution records a bounded assessment, not a fact or verification state.
type Attribution struct {
	Method     AttributionMethod `json:"method"`
	Confidence float64           `json:"confidence"`
	Rationale  string            `json:"rationale"`
}

type Baseline struct {
	ID           string             `json:"id"`
	Scope        Scope              `json:"scope"`
	Value        float64            `json:"value"`
	ObservedAt   time.Time          `json:"observedAt"`
	Verification VerificationStatus `json:"verification"`
	Sources      []SourceReference  `json:"sources,omitempty"`
}

type Indicator struct {
	ID                   string           `json:"id"`
	Name                 string           `json:"name"`
	Unit                 string           `json:"unit"`
	Direction            DesiredDirection `json:"direction"`
	TargetValue          float64          `json:"targetValue"`
	TargetTolerance      float64          `json:"targetTolerance"`
	TrendThresholdPerDay float64          `json:"trendThresholdPerDay"`
	RegressionThreshold  float64          `json:"regressionThreshold"`
	MinimumObservations  int              `json:"minimumObservations"`
	Baseline             Baseline         `json:"baseline"`
}

type IntendedOutcome struct {
	ID         string              `json:"id"`
	Scope      Scope               `json:"scope"`
	Statement  string              `json:"statement"`
	LifeDomain lifeontology.Domain `json:"lifeDomain,omitempty"`
	Window     LongitudinalWindow  `json:"window"`
	Indicators []Indicator         `json:"indicators"`
}

type Observation struct {
	ID           string             `json:"id"`
	Scope        Scope              `json:"scope"`
	IndicatorID  string             `json:"indicatorId"`
	Value        float64            `json:"value"`
	ObservedAt   time.Time          `json:"observedAt"`
	RecordedAt   time.Time          `json:"recordedAt"`
	Verification VerificationStatus `json:"verification"`
	Sources      []SourceReference  `json:"sources,omitempty"`
	Attribution  Attribution        `json:"attribution"`
}

type UserCorrection struct {
	ID                    string             `json:"id"`
	Scope                 Scope              `json:"scope"`
	ObservationID         string             `json:"observationId"`
	ActorID               string             `json:"actorId"`
	UserConfirmed         bool               `json:"userConfirmed"`
	CorrectedValue        float64            `json:"correctedValue"`
	CorrectedVerification VerificationStatus `json:"correctedVerification"`
	Sources               []SourceReference  `json:"sources,omitempty"`
	Reason                string             `json:"reason"`
	CorrectedAt           time.Time          `json:"correctedAt"`
}

type EvaluationRequest struct {
	Outcome      IntendedOutcome  `json:"outcome"`
	Observations []Observation    `json:"observations"`
	Corrections  []UserCorrection `json:"corrections,omitempty"`
	AsOf         time.Time        `json:"asOf"`
}

type EvidenceStatus string

const (
	EvidenceSufficient   EvidenceStatus = "sufficient"
	EvidenceInsufficient EvidenceStatus = "insufficient"
	EvidenceConflicting  EvidenceStatus = "conflicting"
)

type TrendClassification string

const (
	TrendUnknown   TrendClassification = "unknown"
	TrendImproving TrendClassification = "improving"
	TrendStable    TrendClassification = "stable"
	TrendDeclining TrendClassification = "declining"
)

type RegressionClassification string

const (
	RegressionUnknown  RegressionClassification = "unknown"
	RegressionNone     RegressionClassification = "none"
	RegressionDetected RegressionClassification = "detected"
)

type TargetStatus string

const (
	TargetUnknown TargetStatus = "unknown"
	TargetMet     TargetStatus = "met"
	TargetNotMet  TargetStatus = "not_met"
)

type EffectiveObservation struct {
	ObservationID       string             `json:"observationId"`
	AppliedCorrectionID string             `json:"appliedCorrectionId,omitempty"`
	Value               float64            `json:"value"`
	ObservedAt          time.Time          `json:"observedAt"`
	Verification        VerificationStatus `json:"verification"`
	SourceIDs           []string           `json:"sourceIds,omitempty"`
	Attribution         Attribution        `json:"attribution"`
}

type IndicatorEvaluation struct {
	IndicatorID       string                   `json:"indicatorId"`
	Evidence          EvidenceStatus           `json:"evidence"`
	BaselineValue     float64                  `json:"baselineValue"`
	CurrentValue      *float64                 `json:"currentValue,omitempty"`
	DeltaFromBaseline *float64                 `json:"deltaFromBaseline,omitempty"`
	Trend             TrendClassification      `json:"trend"`
	TrendPerDay       *float64                 `json:"trendPerDay,omitempty"`
	Regression        RegressionClassification `json:"regression"`
	Target            TargetStatus             `json:"target"`
	Effective         []EffectiveObservation   `json:"effectiveObservations"`
	ReviewRequired    bool                     `json:"reviewRequired"`
	ReviewReasons     []string                 `json:"reviewReasons,omitempty"`
}

type RecommendationKind string

const (
	RecommendationCollectEvidence     RecommendationKind = "collect_evidence"
	RecommendationReconcileEvidence   RecommendationKind = "reconcile_evidence"
	RecommendationReviewRegression    RecommendationKind = "review_regression"
	RecommendationReviewTrend         RecommendationKind = "review_declining_trend"
	RecommendationReviewCorrection    RecommendationKind = "review_user_correction"
	RecommendationValidateAttribution RecommendationKind = "validate_attribution"
)

type RecommendationControl struct {
	AdvisoryOnly       bool   `json:"advisoryOnly"`
	ReviewRequired     bool   `json:"reviewRequired"`
	ExecutionAuthority string `json:"executionAuthority"`
	MayExecute         bool   `json:"mayExecute"`
	MayChangePolicy    bool   `json:"mayChangePolicy"`
}

type LearningRecommendation struct {
	ID          string                `json:"id"`
	Kind        RecommendationKind    `json:"kind"`
	IndicatorID string                `json:"indicatorId"`
	Summary     string                `json:"summary"`
	EvidenceIDs []string              `json:"evidenceIds,omitempty"`
	Control     RecommendationControl `json:"control"`
}

type OutcomeState string

const (
	OutcomeInsufficientEvidence OutcomeState = "insufficient_evidence"
	OutcomeOnTrack              OutcomeState = "on_track"
	OutcomeAchieved             OutcomeState = "achieved"
	OutcomeRegression           OutcomeState = "regression"
	OutcomeReviewRequired       OutcomeState = "review_required"
)

type Evaluation struct {
	ID              string                   `json:"id"`
	SchemaVersion   string                   `json:"schemaVersion"`
	Scope           Scope                    `json:"scope"`
	OutcomeID       string                   `json:"outcomeId"`
	Window          LongitudinalWindow       `json:"window"`
	AsOf            time.Time                `json:"asOf"`
	State           OutcomeState             `json:"state"`
	Indicators      []IndicatorEvaluation    `json:"indicators"`
	Recommendations []LearningRecommendation `json:"recommendations"`
	ReviewRequired  bool                     `json:"reviewRequired"`
	ReviewReasons   []string                 `json:"reviewReasons,omitempty"`
	AuditDigest     string                   `json:"auditDigest"`
}
