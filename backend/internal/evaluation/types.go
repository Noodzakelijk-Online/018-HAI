// Package evaluation defines a bounded, provider-neutral evaluation contract.
//
// It deliberately owns no HTTP routes, model calls, or promotion side effects.
// Integrators execute an Evaluator, construct immutable records, persist them
// through Repository, and separately apply a fail-closed PromotionDecision.
package evaluation

import (
	"context"
	"encoding/json"
	"time"
)

const (
	DatasetSchemaVersion           uint32 = 1
	RunSchemaVersion               uint32 = 1
	ComparisonReceiptSchemaVersion uint32 = 1
	PromotionReceiptSchemaVersion  uint32 = 1
)

type RunMode string

const (
	RunModeShadow RunMode = "shadow"
	RunModeCanary RunMode = "canary"
)

type RunStatus string

const (
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
)

type CriterionStatus string

const (
	CriterionPassed CriterionStatus = "passed"
	CriterionFailed CriterionStatus = "failed"
	CriterionError  CriterionStatus = "error"
)

// Dataset is a content-addressed, append-only dataset version.
type Dataset struct {
	SchemaVersion uint32           `json:"schemaVersion"`
	ID            string           `json:"id"`
	Version       uint32           `json:"version"`
	Name          string           `json:"name"`
	Description   string           `json:"description,omitempty"`
	Cases         []EvaluationCase `json:"cases"`
	CreatedAt     time.Time        `json:"createdAt"`
	Digest        string           `json:"digest"`
}

type DatasetSpec struct {
	ID          string
	Version     uint32
	Name        string
	Description string
	Cases       []EvaluationCase
	CreatedAt   time.Time
}

// EvaluationCase has its own version so a dataset can pin exact case revisions.
type EvaluationCase struct {
	ID       string          `json:"id"`
	Version  uint32          `json:"version"`
	Input    json.RawMessage `json:"input"`
	Expected json.RawMessage `json:"expected"`
	Criteria []Criterion     `json:"criteria"`
}

// Criterion is snapshotted into every run result. Score values are normalized
// to [0,1]; Weight controls aggregation and MinScore controls pass/fail.
type Criterion struct {
	ID          string  `json:"id"`
	Description string  `json:"description,omitempty"`
	Required    bool    `json:"required"`
	Weight      float64 `json:"weight"`
	MinScore    float64 `json:"minScore"`
}

type EvaluatorDescriptor struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

type SubjectDescriptor struct {
	ID             string `json:"id"`
	Version        string `json:"version"`
	ArtifactDigest string `json:"artifactDigest"`
}

// EvaluationRequest is the bounded input an evaluator may inspect. Config is
// explicit and becomes part of the reproducibility digest.
type EvaluationRequest struct {
	Dataset           Dataset
	Subject           SubjectDescriptor
	Mode              RunMode
	CanaryPercent     float64
	Seed              int64
	Config            json.RawMessage
	EnvironmentDigest string
}

// Evaluator returns observations only. It cannot create a trusted RunRecord or
// make a promotion decision; those are deterministic package responsibilities.
type Evaluator interface {
	Descriptor() EvaluatorDescriptor
	Evaluate(context.Context, EvaluationRequest) ([]CaseObservation, error)
}

type CriterionObservation struct {
	CriterionID    string
	Status         CriterionStatus
	Score          float64
	EvidenceDigest string
	Detail         string
}

type CaseObservation struct {
	CaseID      string
	CaseVersion uint32
	Criteria    []CriterionObservation
}

type RunSpec struct {
	ID                string
	Dataset           Dataset
	Evaluator         EvaluatorDescriptor
	Subject           SubjectDescriptor
	Mode              RunMode
	CanaryPercent     float64
	BaselineRunID     string
	Seed              int64
	Config            json.RawMessage
	EnvironmentDigest string
	StartedAt         time.Time
	CompletedAt       time.Time
	Status            RunStatus
	FailureCode       string
	Observations      []CaseObservation
}

type DatasetRef struct {
	ID      string `json:"id"`
	Version uint32 `json:"version"`
	Digest  string `json:"digest"`
}

type ReproducibilityManifest struct {
	Dataset           DatasetRef          `json:"dataset"`
	Evaluator         EvaluatorDescriptor `json:"evaluator"`
	Subject           SubjectDescriptor   `json:"subject"`
	Seed              int64               `json:"seed"`
	ConfigDigest      string              `json:"configDigest"`
	EnvironmentDigest string              `json:"environmentDigest"`
}

type CriterionResult struct {
	CriterionID    string          `json:"criterionId"`
	Status         CriterionStatus `json:"status"`
	Score          float64         `json:"score"`
	Required       bool            `json:"required"`
	Weight         float64         `json:"weight"`
	MinScore       float64         `json:"minScore"`
	EvidenceDigest string          `json:"evidenceDigest,omitempty"`
	Detail         string          `json:"detail,omitempty"`
}

type CaseResult struct {
	CaseID      string            `json:"caseId"`
	CaseVersion uint32            `json:"caseVersion"`
	Passed      bool              `json:"passed"`
	Score       float64           `json:"score"`
	Criteria    []CriterionResult `json:"criteria"`
}

// RunRecord is append-only. Repository exposes no update method and must
// return defensive copies so a caller cannot mutate persisted evidence.
type RunRecord struct {
	SchemaVersion         uint32                  `json:"schemaVersion"`
	ID                    string                  `json:"id"`
	Dataset               DatasetRef              `json:"dataset"`
	Evaluator             EvaluatorDescriptor     `json:"evaluator"`
	Subject               SubjectDescriptor       `json:"subject"`
	Mode                  RunMode                 `json:"mode"`
	CanaryPercent         float64                 `json:"canaryPercent"`
	BaselineRunID         string                  `json:"baselineRunId,omitempty"`
	StartedAt             time.Time               `json:"startedAt"`
	CompletedAt           time.Time               `json:"completedAt"`
	Status                RunStatus               `json:"status"`
	FailureCode           string                  `json:"failureCode,omitempty"`
	CaseResults           []CaseResult            `json:"caseResults,omitempty"`
	OverallScore          float64                 `json:"overallScore"`
	CasePassRate          float64                 `json:"casePassRate"`
	RequiredFailureCount  int                     `json:"requiredFailureCount"`
	CriterionErrorCount   int                     `json:"criterionErrorCount"`
	Reproducibility       ReproducibilityManifest `json:"reproducibility"`
	ReproducibilityDigest string                  `json:"reproducibilityDigest"`
	RecordDigest          string                  `json:"recordDigest"`
}

// BaselineComparisonReceipt is immutable evidence of a deterministic
// comparison. It binds the policy and result to exact run record digests.
type BaselineComparisonReceipt struct {
	SchemaVersion         uint32               `json:"schemaVersion"`
	ID                    string               `json:"id"`
	CandidateRunID        string               `json:"candidateRunId"`
	CandidateRecordDigest string               `json:"candidateRecordDigest"`
	BaselineRunID         string               `json:"baselineRunId"`
	BaselineRecordDigest  string               `json:"baselineRecordDigest"`
	Thresholds            RegressionThresholds `json:"thresholds"`
	Comparison            BaselineComparison   `json:"comparison"`
	CreatedAt             time.Time            `json:"createdAt"`
	ReceiptDigest         string               `json:"receiptDigest"`
}

type BaselineComparisonReceiptSpec struct {
	ID         string
	Candidate  RunRecord
	Baseline   RunRecord
	Thresholds RegressionThresholds
	CreatedAt  time.Time
}

// PromotionDecisionReceipt records a fail-closed decision but never applies a
// deployment, routing, or configuration side effect.
type PromotionDecisionReceipt struct {
	SchemaVersion         uint32               `json:"schemaVersion"`
	ID                    string               `json:"id"`
	CandidateRunID        string               `json:"candidateRunId"`
	CandidateRecordDigest string               `json:"candidateRecordDigest"`
	BaselineRunID         string               `json:"baselineRunId,omitempty"`
	BaselineRecordDigest  string               `json:"baselineRecordDigest,omitempty"`
	ComparisonReceiptID   string               `json:"comparisonReceiptId,omitempty"`
	Thresholds            RegressionThresholds `json:"thresholds"`
	Decision              PromotionDecision    `json:"decision"`
	CreatedAt             time.Time            `json:"createdAt"`
	ReceiptDigest         string               `json:"receiptDigest"`
}

type PromotionDecisionReceiptSpec struct {
	ID                  string
	Candidate           RunRecord
	Baseline            *RunRecord
	ComparisonReceiptID string
	Thresholds          RegressionThresholds
	CreatedAt           time.Time
}
