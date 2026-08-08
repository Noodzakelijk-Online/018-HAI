package outcomeevaluation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// postgres0023SchemaContract documents the exact minimum schema provided by
// migration 0023. It is test-only documentation, not executable DDL.
// The migration must additionally make all three tables append-only (deny
// UPDATE/DELETE/TRUNCATE) without CASCADE-based rollback.
const postgres0023SchemaContract = `
CREATE TABLE public.outcome_evaluation_outcome_revisions (
  owner_identity text NOT NULL CHECK (char_length(owner_identity) BETWEEN 1 AND 256),
  workspace_id text NOT NULL CHECK (char_length(workspace_id) BETWEEN 1 AND 256),
  outcome_id text NOT NULL CHECK (char_length(outcome_id) BETWEEN 1 AND 256),
  revision bigint NOT NULL CHECK (revision > 0),
  idempotency_key text NOT NULL CHECK (char_length(idempotency_key) BETWEEN 1 AND 256),
  request_digest text NOT NULL CHECK (request_digest ~ '^[0-9a-f]{64}$'),
  audit_digest text NOT NULL CHECK (audit_digest ~ '^[0-9a-f]{64}$'),
  recorded_at timestamptz NOT NULL,
  payload jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
  PRIMARY KEY (owner_identity, workspace_id, outcome_id, revision),
  UNIQUE (owner_identity, workspace_id, outcome_id, idempotency_key)
);
CREATE INDEX outcome_evaluation_outcome_revisions_latest_idx
  ON public.outcome_evaluation_outcome_revisions
  (owner_identity, workspace_id, outcome_id, revision DESC);

CREATE TABLE public.outcome_evaluation_evaluations (
  owner_identity text NOT NULL CHECK (char_length(owner_identity) BETWEEN 1 AND 256),
  workspace_id text NOT NULL CHECK (char_length(workspace_id) BETWEEN 1 AND 256),
  outcome_id text NOT NULL CHECK (char_length(outcome_id) BETWEEN 1 AND 256),
  evaluation_id text NOT NULL CHECK (char_length(evaluation_id) BETWEEN 1 AND 320),
  outcome_revision bigint NOT NULL CHECK (outcome_revision > 0),
  idempotency_key text NOT NULL CHECK (char_length(idempotency_key) BETWEEN 1 AND 256),
  request_digest text NOT NULL CHECK (request_digest ~ '^[0-9a-f]{64}$'),
  evaluation_audit_digest text NOT NULL CHECK (evaluation_audit_digest ~ '^[0-9a-f]{64}$'),
  record_digest text NOT NULL CHECK (record_digest ~ '^[0-9a-f]{64}$'),
  recorded_at timestamptz NOT NULL,
  payload jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
  PRIMARY KEY (owner_identity, workspace_id, outcome_id, evaluation_id),
  UNIQUE (owner_identity, workspace_id, outcome_id, idempotency_key),
  FOREIGN KEY (owner_identity, workspace_id, outcome_id, outcome_revision)
    REFERENCES public.outcome_evaluation_outcome_revisions
    (owner_identity, workspace_id, outcome_id, revision)
);
CREATE INDEX outcome_evaluation_evaluations_history_idx
  ON public.outcome_evaluation_evaluations
  (owner_identity, workspace_id, outcome_id, recorded_at DESC, evaluation_id DESC);

CREATE TABLE public.outcome_evaluation_corrections (
  owner_identity text NOT NULL CHECK (char_length(owner_identity) BETWEEN 1 AND 256),
  workspace_id text NOT NULL CHECK (char_length(workspace_id) BETWEEN 1 AND 256),
  outcome_id text NOT NULL CHECK (char_length(outcome_id) BETWEEN 1 AND 256),
  correction_id text NOT NULL CHECK (char_length(correction_id) BETWEEN 1 AND 256),
  observation_id text NOT NULL CHECK (char_length(observation_id) BETWEEN 1 AND 256),
  outcome_revision bigint NOT NULL CHECK (outcome_revision > 0),
  idempotency_key text NOT NULL CHECK (char_length(idempotency_key) BETWEEN 1 AND 256),
  request_digest text NOT NULL CHECK (request_digest ~ '^[0-9a-f]{64}$'),
  audit_digest text NOT NULL CHECK (audit_digest ~ '^[0-9a-f]{64}$'),
  recorded_at timestamptz NOT NULL,
  payload jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
  PRIMARY KEY (owner_identity, workspace_id, outcome_id, correction_id),
  UNIQUE (owner_identity, workspace_id, outcome_id, idempotency_key),
  FOREIGN KEY (owner_identity, workspace_id, outcome_id, outcome_revision)
    REFERENCES public.outcome_evaluation_outcome_revisions
    (owner_identity, workspace_id, outcome_id, revision)
);
CREATE INDEX outcome_evaluation_corrections_history_idx
  ON public.outcome_evaluation_corrections
  (owner_identity, workspace_id, outcome_id, recorded_at DESC, correction_id DESC);
`

func TestPostgres0023SchemaContractDocumentsScopeAndAtomicity(t *testing.T) {
	required := []string{
		"outcome_evaluation_outcome_revisions",
		"outcome_evaluation_evaluations",
		"outcome_evaluation_corrections",
		"PRIMARY KEY (owner_identity, workspace_id, outcome_id, revision)",
		"UNIQUE (owner_identity, workspace_id, outcome_id, idempotency_key)",
		"FOREIGN KEY (owner_identity, workspace_id, outcome_id, outcome_revision)",
		"jsonb_typeof(payload) = 'object'",
		"request_digest ~ '^[0-9a-f]{64}$'",
	}
	for _, fragment := range required {
		if !strings.Contains(postgres0023SchemaContract, fragment) {
			t.Fatalf("0023 schema contract is missing %q", fragment)
		}
	}
}

func TestPostgresRepositoryFailsClosedWithoutDatabase(t *testing.T) {
	repositories := []*PostgresRepository{nil, NewPostgresRepository(nil)}
	for _, repository := range repositories {
		if _, err := repository.GetOutcome(context.Background(), "owner-1", "workspace-1", "outcome-1"); err == nil {
			t.Fatal("GetOutcome accepted an unavailable database")
		}
		if _, err := repository.ResolveOutcomeRevision(context.Background(), "owner-1", "workspace-1", "outcome-1", 1, strings.Repeat("a", 64)); err == nil {
			t.Fatal("ResolveOutcomeRevision accepted an unavailable database")
		}
	}
	if _, err := NewPostgresRepositoryWithLimits(nil, HistoryLimits{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid limits error = %v, want ErrInvalidInput", err)
	}
}

func TestDecodeStrictPostgresJSONRejectsSchemaDrift(t *testing.T) {
	var record OutcomeRevision
	for _, payload := range []string{
		`null`,
		`{}` + `{}`,
		`{"outcome":{},"revision":1,"recordedAt":"2026-01-01T00:00:00Z","auditDigest":"x","unexpected":true}`,
	} {
		if err := decodeStrictPostgresJSON(payload, &record); err == nil {
			t.Fatalf("decodeStrictPostgresJSON(%q) unexpectedly succeeded", payload)
		}
	}
}

func TestPostgresRowDecodersRevalidateScopeMetadataAndAuditDigests(t *testing.T) {
	outcome, evaluation, correction := postgresDecoderFixtures(t)
	outcomePayload, _ := marshalPostgresRecord("outcome", outcome)
	evaluationPayload, _ := marshalPostgresRecord("evaluation", evaluation)
	correctionPayload, _ := marshalPostgresRecord("correction", correction)
	requestDigest := strings.Repeat("a", 64)

	outcomeRow := postgresOutcomeRow{
		OwnerIdentity: "owner-1", WorkspaceID: "workspace-1", OutcomeID: "outcome-1",
		Revision: outcome.Revision, IdempotencyKey: "outcome-write", RequestDigest: requestDigest,
		AuditDigest: outcome.AuditDigest, RecordedAt: outcome.RecordedAt, Payload: string(outcomePayload),
	}
	if _, err := decodeOutcomeRow(outcomeRow, "owner-1", "workspace-1", "outcome-1"); err != nil {
		t.Fatalf("decodeOutcomeRow(valid) error = %v", err)
	}
	tamperedOutcome := outcomeRow
	tamperedOutcome.WorkspaceID = "other-workspace"
	if _, err := decodeOutcomeRow(tamperedOutcome, "owner-1", "workspace-1", "outcome-1"); !errors.Is(err, ErrIntegrityViolation) {
		t.Fatalf("tampered outcome metadata error = %v", err)
	}

	evaluationRow := postgresEvaluationRow{
		OwnerIdentity: "owner-1", WorkspaceID: "workspace-1", OutcomeID: "outcome-1",
		EvaluationID: evaluation.Evaluation.ID, OutcomeRevision: evaluation.OutcomeRevision,
		IdempotencyKey: "evaluation-write", RequestDigest: requestDigest,
		EvaluationAuditDigest: evaluation.Evaluation.AuditDigest, RecordDigest: evaluation.RecordDigest,
		RecordedAt: evaluation.RecordedAt, Payload: string(evaluationPayload),
	}
	if _, err := decodeEvaluationRow(evaluationRow, "owner-1", "workspace-1", "outcome-1"); err != nil {
		t.Fatalf("decodeEvaluationRow(valid) error = %v", err)
	}
	tamperedEvaluation := evaluationRow
	tamperedEvaluation.RecordDigest = strings.Repeat("b", 64)
	if _, err := decodeEvaluationRow(tamperedEvaluation, "owner-1", "workspace-1", "outcome-1"); !errors.Is(err, ErrIntegrityViolation) {
		t.Fatalf("tampered evaluation digest error = %v", err)
	}

	correctionRow := postgresCorrectionRow{
		OwnerIdentity: "owner-1", WorkspaceID: "workspace-1", OutcomeID: "outcome-1",
		CorrectionID: correction.Correction.ID, ObservationID: correction.Observation.ID,
		OutcomeRevision: correction.OutcomeRevision, IdempotencyKey: "correction-write", RequestDigest: requestDigest,
		AuditDigest: correction.AuditDigest, RecordedAt: correction.RecordedAt, Payload: string(correctionPayload),
	}
	if _, err := decodeCorrectionRow(correctionRow, "owner-1", "workspace-1", "outcome-1"); err != nil {
		t.Fatalf("decodeCorrectionRow(valid) error = %v", err)
	}
	tamperedCorrection := correctionRow
	tamperedCorrection.ObservationID = "other-observation"
	if _, err := decodeCorrectionRow(tamperedCorrection, "owner-1", "workspace-1", "outcome-1"); !errors.Is(err, ErrIntegrityViolation) {
		t.Fatalf("tampered correction metadata error = %v", err)
	}
}

func postgresDecoderFixtures(t *testing.T) (OutcomeRevision, EvaluationRecord, CorrectionRecord) {
	t.Helper()
	now := time.Date(2026, time.February, 2, 12, 0, 0, 0, time.UTC)
	service := newService(NewMemoryRepository(), func() time.Time { return now })
	outcome, _, err := service.StoreOutcome(context.Background(), "owner-1", "workspace-1", "outcome-1", StoreOutcomeRequest{
		IdempotencyKey: "fixture-outcome", ExpectedRevision: 0, Outcome: validRequest().Outcome,
	})
	if err != nil {
		t.Fatal(err)
	}
	evaluation, _, err := service.CreateEvaluation(context.Background(), "owner-1", "workspace-1", "outcome-1", CreateEvaluationRequest{
		IdempotencyKey: "fixture-evaluation", OutcomeRevision: 1,
		Observations: []Observation{
			observation("fixture-observation-1", 12, testStart.Add(5*24*time.Hour)),
			observation("fixture-observation-2", 16, testStart.Add(15*24*time.Hour)),
		},
		AsOf: testAsOf,
	})
	if err != nil {
		t.Fatal(err)
	}
	original := observation("fixture-corrected-observation", 16, testStart.Add(15*24*time.Hour))
	correction, _, err := service.StoreCorrection(context.Background(), "owner-1", "workspace-1", "outcome-1", StoreCorrectionRequest{
		IdempotencyKey: "fixture-correction", OutcomeRevision: 1, Observation: original,
		Correction: UserCorrection{
			ID: "fixture-correction", Scope: original.Scope, ObservationID: original.ID,
			ActorID: "owner-1", UserConfirmed: true, CorrectedValue: 13,
			CorrectedVerification: VerificationUserConfirmed,
			Reason:                "Owner-confirmed fixture correction.", CorrectedAt: original.RecordedAt.Add(time.Hour),
		},
		AsOf: testAsOf,
	})
	if err != nil {
		t.Fatal(err)
	}
	return outcome, evaluation, correction
}
