package outcomeevaluation

import (
	"time"

	"automation-hub-backend/internal/lifeontology"
)

// OutcomeRevision is an immutable revision of an intended outcome definition.
type OutcomeRevision struct {
	Outcome                    IntendedOutcome                           `json:"outcome"`
	Revision                   int64                                     `json:"revision"`
	RecordedAt                 time.Time                                 `json:"recordedAt"`
	AuditDigest                string                                    `json:"auditDigest"`
	LifeGraphProjection        *lifeontology.OperationalProjectionResult `json:"lifeGraphProjection,omitempty"`
	LifeGraphProjectionWarning string                                    `json:"lifeGraphProjectionWarning,omitempty"`
}

// EvaluationRecord persists an advisory evaluation against one exact outcome
// revision. Its nested Evaluation has its own immutable audit digest.
type EvaluationRecord struct {
	Evaluation                 Evaluation                                `json:"evaluation"`
	OutcomeRevision            int64                                     `json:"outcomeRevision"`
	RecordedAt                 time.Time                                 `json:"recordedAt"`
	RecordDigest               string                                    `json:"recordDigest"`
	LifeGraphProjection        *lifeontology.OperationalProjectionResult `json:"lifeGraphProjection,omitempty"`
	LifeGraphProjectionWarning string                                    `json:"lifeGraphProjectionWarning,omitempty"`
}

// CorrectionRecord preserves both a user correction and the observation it
// corrects. Corrections remain evidence; they never mutate the original.
type CorrectionRecord struct {
	OutcomeID       string         `json:"outcomeId"`
	OutcomeRevision int64          `json:"outcomeRevision"`
	Observation     Observation    `json:"observation"`
	Correction      UserCorrection `json:"correction"`
	RecordedAt      time.Time      `json:"recordedAt"`
	AuditDigest     string         `json:"auditDigest"`
}

type StoreOutcomeRequest struct {
	IdempotencyKey   string          `json:"idempotencyKey"`
	ExpectedRevision int64           `json:"expectedRevision"`
	Outcome          IntendedOutcome `json:"outcome"`
}

type CreateEvaluationRequest struct {
	IdempotencyKey     string           `json:"idempotencyKey"`
	OutcomeRevision    int64            `json:"outcomeRevision"`
	OutcomeAuditDigest string           `json:"outcomeAuditDigest,omitempty"`
	Observations       []Observation    `json:"observations"`
	Corrections        []UserCorrection `json:"corrections,omitempty"`
	AsOf               time.Time        `json:"asOf"`
}

type StoreCorrectionRequest struct {
	IdempotencyKey  string         `json:"idempotencyKey"`
	OutcomeRevision int64          `json:"outcomeRevision"`
	Observation     Observation    `json:"observation"`
	Correction      UserCorrection `json:"correction"`
	AsOf            time.Time      `json:"asOf"`
}

// WriteToken binds one idempotency key to one canonical request payload.
// Repository implementations must return the original record for a matching
// retry and ErrIdempotencyConflict when the payload digest differs.
type WriteToken struct {
	Key                string
	RequestDigest      string
	OutcomeAuditDigest string
}

func outcomeRevisionDigest(value OutcomeRevision) (string, error) {
	value.AuditDigest = ""
	value.LifeGraphProjection = nil
	value.LifeGraphProjectionWarning = ""
	return hashValue(value)
}

func VerifyOutcomeRevisionDigest(value OutcomeRevision) error {
	expected, err := outcomeRevisionDigest(value)
	if err != nil {
		return err
	}
	if !equalSHA256(value.AuditDigest, expected) {
		return ErrIntegrityViolation
	}
	return nil
}

func evaluationRecordDigest(value EvaluationRecord) (string, error) {
	value.RecordDigest = ""
	value.LifeGraphProjection = nil
	value.LifeGraphProjectionWarning = ""
	return hashValue(value)
}

func VerifyEvaluationRecordDigest(value EvaluationRecord) error {
	if err := VerifyAuditDigest(value.Evaluation); err != nil {
		return err
	}
	expected, err := evaluationRecordDigest(value)
	if err != nil {
		return err
	}
	if !equalSHA256(value.RecordDigest, expected) {
		return ErrIntegrityViolation
	}
	return nil
}

func correctionRecordDigest(value CorrectionRecord) (string, error) {
	value.AuditDigest = ""
	return hashValue(value)
}

func VerifyCorrectionRecordDigest(value CorrectionRecord) error {
	expected, err := correctionRecordDigest(value)
	if err != nil {
		return err
	}
	if !equalSHA256(value.AuditDigest, expected) {
		return ErrIntegrityViolation
	}
	return nil
}
