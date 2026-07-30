package models

import (
	"time"

	"github.com/google/uuid"
)

// EvaluationDataset stores immutable owner-scoped dataset metadata. Dataset
// cases are normalized into EvaluationDatasetCase so individual case versions
// remain inspectable without weakening the dataset-level content digest.
type EvaluationDataset struct {
	ID             uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	OwnerIdentity  string    `gorm:"type:varchar(255);not null;uniqueIndex:uq_evaluation_datasets_owner_key,priority:1;uniqueIndex:uq_evaluation_datasets_owner_record,priority:1"`
	DatasetID      string    `gorm:"type:varchar(128);not null;uniqueIndex:uq_evaluation_datasets_owner_key,priority:2"`
	DatasetVersion uint32    `gorm:"not null;uniqueIndex:uq_evaluation_datasets_owner_key,priority:3"`
	SchemaVersion  uint32    `gorm:"not null"`
	Name           string    `gorm:"type:varchar(255);not null"`
	Description    string    `gorm:"type:text;not null;default:''"`
	CreatedAtValue string    `gorm:"column:created_at_value;type:varchar(64);not null"`
	Digest         string    `gorm:"type:char(64);not null;index"`
	RecordedAt     time.Time `gorm:"not null;index:idx_evaluation_datasets_owner_recorded,priority:2,sort:desc"`
}

func (EvaluationDataset) TableName() string { return "evaluation_datasets" }

// EvaluationDatasetCase stores the exact canonical JSON bytes used to produce
// the parent dataset digest. Text storage is intentional: jsonb may reorder
// object keys and would make an unchanged content-addressed record appear
// tampered with after a round trip.
type EvaluationDatasetCase struct {
	ID              uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	OwnerIdentity   string    `gorm:"type:varchar(255);not null;uniqueIndex:uq_evaluation_dataset_cases_owner_case,priority:1;uniqueIndex:uq_evaluation_dataset_cases_owner_ordinal,priority:1"`
	DatasetRecordID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_evaluation_dataset_cases_owner_case,priority:2;uniqueIndex:uq_evaluation_dataset_cases_owner_ordinal,priority:2;index"`
	Ordinal         int       `gorm:"not null;uniqueIndex:uq_evaluation_dataset_cases_owner_ordinal,priority:3"`
	CaseID          string    `gorm:"type:varchar(128);not null;uniqueIndex:uq_evaluation_dataset_cases_owner_case,priority:3"`
	CaseVersion     uint32    `gorm:"not null;uniqueIndex:uq_evaluation_dataset_cases_owner_case,priority:4"`
	InputJSON       string    `gorm:"type:text;not null"`
	ExpectedJSON    string    `gorm:"type:text;not null"`
	CriteriaJSON    string    `gorm:"type:text;not null"`
}

func (EvaluationDatasetCase) TableName() string { return "evaluation_dataset_cases" }

// EvaluationRun is an immutable run receipt. Case-level observations are kept
// in EvaluationRunCaseResult; reproducibility fields and both trusted digests
// stay on the run for cheap fail-closed validation.
type EvaluationRun struct {
	ID                    uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	OwnerIdentity         string     `gorm:"type:varchar(255);not null;uniqueIndex:uq_evaluation_runs_owner_key,priority:1;uniqueIndex:uq_evaluation_runs_owner_record,priority:1"`
	RunID                 string     `gorm:"type:varchar(128);not null;uniqueIndex:uq_evaluation_runs_owner_key,priority:2"`
	SchemaVersion         uint32     `gorm:"not null"`
	DatasetRecordID       uuid.UUID  `gorm:"type:uuid;not null;index"`
	DatasetID             string     `gorm:"type:varchar(128);not null;index:idx_evaluation_runs_dataset,priority:1"`
	DatasetVersion        uint32     `gorm:"not null;index:idx_evaluation_runs_dataset,priority:2"`
	DatasetDigest         string     `gorm:"type:char(64);not null"`
	EvaluatorID           string     `gorm:"type:varchar(128);not null"`
	EvaluatorVersion      string     `gorm:"type:varchar(128);not null"`
	SubjectID             string     `gorm:"type:varchar(128);not null;index"`
	SubjectVersion        string     `gorm:"type:varchar(128);not null"`
	SubjectArtifactDigest string     `gorm:"type:char(64);not null"`
	Mode                  string     `gorm:"type:varchar(32);not null;index"`
	CanaryPercent         float64    `gorm:"type:double precision;not null"`
	BaselineRunRecordID   *uuid.UUID `gorm:"type:uuid;index"`
	BaselineRunID         string     `gorm:"type:varchar(128);not null;default:'';index"`
	StartedAtValue        string     `gorm:"column:started_at_value;type:varchar(64);not null"`
	CompletedAtValue      string     `gorm:"column:completed_at_value;type:varchar(64);not null"`
	CompletedAtIndex      time.Time  `gorm:"column:completed_at_index;not null;index:idx_evaluation_runs_owner_completed,priority:2,sort:desc"`
	Status                string     `gorm:"type:varchar(32);not null;index"`
	FailureCode           string     `gorm:"type:varchar(160);not null;default:''"`
	OverallScore          float64    `gorm:"type:double precision;not null"`
	CasePassRate          float64    `gorm:"type:double precision;not null"`
	RequiredFailureCount  int        `gorm:"not null"`
	CriterionErrorCount   int        `gorm:"not null"`
	ReproducibilityJSON   string     `gorm:"type:text;not null"`
	ReproducibilityDigest string     `gorm:"type:char(64);not null;index"`
	RecordDigest          string     `gorm:"type:char(64);not null;index"`
	RecordedAt            time.Time  `gorm:"not null"`
}

func (EvaluationRun) TableName() string { return "evaluation_runs" }

// EvaluationRunCaseResult stores an ordered snapshot of one run case result.
type EvaluationRunCaseResult struct {
	ID            uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	OwnerIdentity string    `gorm:"type:varchar(255);not null;uniqueIndex:uq_evaluation_run_results_owner_case,priority:1;uniqueIndex:uq_evaluation_run_results_owner_ordinal,priority:1"`
	RunRecordID   uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_evaluation_run_results_owner_case,priority:2;uniqueIndex:uq_evaluation_run_results_owner_ordinal,priority:2;index"`
	Ordinal       int       `gorm:"not null;uniqueIndex:uq_evaluation_run_results_owner_ordinal,priority:3"`
	CaseID        string    `gorm:"type:varchar(128);not null;uniqueIndex:uq_evaluation_run_results_owner_case,priority:3"`
	CaseVersion   uint32    `gorm:"not null;uniqueIndex:uq_evaluation_run_results_owner_case,priority:4"`
	Passed        bool      `gorm:"not null"`
	Score         float64   `gorm:"type:double precision;not null"`
	CriteriaJSON  string    `gorm:"type:text;not null"`
}

func (EvaluationRunCaseResult) TableName() string { return "evaluation_run_case_results" }

// EvaluationComparisonReceipt binds a deterministic baseline comparison to
// exact immutable run digests and the regression policy used.
type EvaluationComparisonReceipt struct {
	ID                    uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	OwnerIdentity         string    `gorm:"type:varchar(255);not null;uniqueIndex:uq_evaluation_comparison_receipts_owner_key,priority:1;uniqueIndex:uq_evaluation_comparison_receipts_owner_record,priority:1"`
	ReceiptID             string    `gorm:"type:varchar(128);not null;uniqueIndex:uq_evaluation_comparison_receipts_owner_key,priority:2"`
	SchemaVersion         uint32    `gorm:"not null"`
	CandidateRunID        string    `gorm:"type:varchar(128);not null;index"`
	CandidateRecordDigest string    `gorm:"type:char(64);not null"`
	BaselineRunID         string    `gorm:"type:varchar(128);not null;index"`
	BaselineRecordDigest  string    `gorm:"type:char(64);not null"`
	ThresholdsJSON        string    `gorm:"type:text;not null"`
	ComparisonJSON        string    `gorm:"type:text;not null"`
	CreatedAtValue        string    `gorm:"column:created_at_value;type:varchar(64);not null"`
	ReceiptDigest         string    `gorm:"type:char(64);not null;index"`
	RecordedAt            time.Time `gorm:"not null;index:idx_evaluation_comparison_owner_recorded,priority:2,sort:desc"`
}

func (EvaluationComparisonReceipt) TableName() string {
	return "evaluation_comparison_receipts"
}

// EvaluationPromotionDecisionReceipt is evidence of a fail-closed decision.
// It does not execute or authorize a deployment side effect.
type EvaluationPromotionDecisionReceipt struct {
	ID                    uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	OwnerIdentity         string     `gorm:"type:varchar(255);not null;uniqueIndex:uq_evaluation_promotion_receipts_owner_key,priority:1;uniqueIndex:uq_evaluation_promotion_receipts_owner_record,priority:1"`
	ReceiptID             string     `gorm:"type:varchar(128);not null;uniqueIndex:uq_evaluation_promotion_receipts_owner_key,priority:2"`
	SchemaVersion         uint32     `gorm:"not null"`
	CandidateRunID        string     `gorm:"type:varchar(128);not null;index"`
	CandidateRecordDigest string     `gorm:"type:char(64);not null"`
	BaselineRunID         *string    `gorm:"type:varchar(128);index"`
	BaselineRecordDigest  *string    `gorm:"type:char(64)"`
	ComparisonReceiptID   *uuid.UUID `gorm:"type:uuid;index"`
	ComparisonReceiptKey  string     `gorm:"type:varchar(128);not null;default:''"`
	ThresholdsJSON        string     `gorm:"type:text;not null"`
	DecisionJSON          string     `gorm:"type:text;not null"`
	CreatedAtValue        string     `gorm:"column:created_at_value;type:varchar(64);not null"`
	ReceiptDigest         string     `gorm:"type:char(64);not null;index"`
	RecordedAt            time.Time  `gorm:"not null;index:idx_evaluation_promotion_owner_recorded,priority:2,sort:desc"`
}

func (EvaluationPromotionDecisionReceipt) TableName() string {
	return "evaluation_promotion_decision_receipts"
}
