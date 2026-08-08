package proactivity

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"

	"automation-hub-backend/internal/infra"

	"gorm.io/gorm"
)

var ErrCorruptStorage = errors.New("proactivity storage is corrupt")

const (
	idempotencyKindPolicy    = "policy"
	idempotencyKindSignals   = "signals"
	idempotencyKindDecisions = "decisions"
)

// PostgresRepository is the durable owner-scoped advisory ledger. It has no
// delivery, execution, approval, or authority-granting capability.
//
// Schema creation belongs to a future versioned migration. The exact required
// table, column, constraint, and index contract is documented in
// postgres_schema_contract.go and enforced by package-local tests.
type PostgresRepository struct {
	DB *gorm.DB
}

var _ Repository = (*PostgresRepository)(nil)

func NewPostgresRepository(db *gorm.DB) *PostgresRepository {
	return &PostgresRepository{DB: db}
}

// DefaultRepository opens the configured, migrated database. It deliberately
// has no in-memory fallback: missing schema or storage is a closed failure.
func DefaultRepository() (Repository, error) {
	db, err := infra.GetDefaultDB()
	if err != nil {
		return nil, fmt.Errorf("open proactivity database: %w", err)
	}
	if err := validatePostgresSchema(db); err != nil {
		return nil, err
	}
	return NewPostgresRepository(db), nil
}

func (r *PostgresRepository) RecordPolicy(ctx context.Context, owner, key, digest string, record PolicyRecord) (PolicyRecord, bool, error) {
	if err := r.ready(ctx); err != nil {
		return PolicyRecord{}, false, err
	}
	owner, key, digest, err := normalizePostgresCommand(owner, key, digest)
	if err != nil {
		return PolicyRecord{}, false, err
	}
	record, err = validateStoredPolicyRecord(owner, record)
	if err != nil {
		return PolicyRecord{}, false, err
	}
	if err := validatePostgresPayloadDigest(idempotencyKindPolicy, owner, record.Policy, digest); err != nil {
		return PolicyRecord{}, false, err
	}
	payload, err := marshalPostgresPayload("policy", record)
	if err != nil {
		return PolicyRecord{}, false, err
	}

	var stored PolicyRecord
	created := false
	err = r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		reserved, reserveErr := reservePostgresIdempotency(tx, owner, key, idempotencyKindPolicy, digest, record.RecordedAt)
		if reserveErr != nil {
			return reserveErr
		}
		if !reserved {
			stored, reserveErr = loadPostgresPolicy(tx, owner, key)
			return reserveErr
		}
		result := tx.Exec(`
			INSERT INTO public.proactivity_policy_records (
				owner_identity, idempotency_key, record_kind, payload_digest,
				recorded_at, payload
			) VALUES (?, ?, ?, ?, ?, CAST(? AS jsonb))`,
			owner, key, idempotencyKindPolicy, digest, record.RecordedAt.UTC(), string(payload),
		)
		if result.Error != nil {
			return fmt.Errorf("record proactivity policy: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("%w: policy insert affected %d rows", ErrCorruptStorage, result.RowsAffected)
		}
		stored = record
		created = true
		return nil
	})
	if err != nil {
		return PolicyRecord{}, false, err
	}
	return clonePolicyRecord(stored), created, nil
}

func (r *PostgresRepository) CurrentPolicy(ctx context.Context, owner string) (PolicyRecord, error) {
	if err := r.ready(ctx); err != nil {
		return PolicyRecord{}, err
	}
	owner, err := repositoryOwner(owner)
	if err != nil {
		return PolicyRecord{}, err
	}
	var row postgresPolicyRow
	err = r.DB.WithContext(ctx).Raw(`
		SELECT owner_identity, idempotency_key, payload_digest, recorded_at,
			payload::text AS payload
		FROM public.proactivity_policy_records
		WHERE owner_identity = ?
		ORDER BY recorded_at DESC, idempotency_key DESC
		LIMIT 1`, owner).Scan(&row).Error
	if err != nil {
		return PolicyRecord{}, fmt.Errorf("read current proactivity policy: %w", err)
	}
	if row.OwnerIdentity == "" {
		return PolicyRecord{}, ErrNotFound
	}
	return decodePostgresPolicyRow(row, owner)
}

func (r *PostgresRepository) ListPolicies(ctx context.Context, owner string, limit int) ([]PolicyRecord, error) {
	if err := r.ready(ctx); err != nil {
		return nil, err
	}
	owner, err := repositoryOwner(owner)
	if err != nil {
		return nil, err
	}
	var rows []postgresPolicyRow
	err = r.DB.WithContext(ctx).Raw(`
		SELECT owner_identity, idempotency_key, payload_digest, recorded_at,
			payload::text AS payload
		FROM public.proactivity_policy_records
		WHERE owner_identity = ?
		ORDER BY recorded_at DESC, idempotency_key DESC
		LIMIT ?`, owner, boundedLimit(limit, MaxPolicyHistory)).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list proactivity policies: %w", err)
	}
	result := make([]PolicyRecord, 0, len(rows))
	for _, row := range rows {
		record, decodeErr := decodePostgresPolicyRow(row, owner)
		if decodeErr != nil {
			return nil, decodeErr
		}
		result = append(result, record)
	}
	return result, nil
}

func (r *PostgresRepository) RecordSignals(ctx context.Context, owner, key, digest string, records []SignalRecord) ([]SignalRecord, bool, error) {
	if err := r.ready(ctx); err != nil {
		return nil, false, err
	}
	owner, key, digest, err := normalizePostgresCommand(owner, key, digest)
	if err != nil {
		return nil, false, err
	}
	records, err = validateStoredSignalRecords(owner, records)
	if err != nil {
		return nil, false, err
	}
	signals := make([]OpenLoopSignal, len(records))
	for index := range records {
		signals[index] = records[index].Signal
	}
	if err := validatePostgresPayloadDigest(idempotencyKindSignals, owner, signals, digest); err != nil {
		return nil, false, err
	}
	batchPayload, err := marshalPostgresPayload("signal batch", records)
	if err != nil {
		return nil, false, err
	}

	var stored []SignalRecord
	created := false
	err = r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		reserved, reserveErr := reservePostgresIdempotency(tx, owner, key, idempotencyKindSignals, digest, records[0].RecordedAt)
		if reserveErr != nil {
			return reserveErr
		}
		if !reserved {
			stored, reserveErr = loadPostgresSignalBatch(tx, owner, key)
			return reserveErr
		}
		batchResult := tx.Exec(`
			INSERT INTO public.proactivity_signal_batches (
				owner_identity, idempotency_key, record_kind, payload_digest, signal_count,
				recorded_at, payload
			) VALUES (?, ?, ?, ?, ?, ?, CAST(? AS jsonb))`,
			owner, key, idempotencyKindSignals, digest, len(records), records[0].RecordedAt.UTC(), string(batchPayload),
		)
		if batchResult.Error != nil {
			return fmt.Errorf("record proactivity signal batch: %w", batchResult.Error)
		}
		if batchResult.RowsAffected != 1 {
			return fmt.Errorf("%w: signal batch insert affected %d rows", ErrCorruptStorage, batchResult.RowsAffected)
		}
		for ordinal, record := range records {
			payload, marshalErr := marshalPostgresPayload("signal record", record)
			if marshalErr != nil {
				return marshalErr
			}
			result := tx.Exec(`
				INSERT INTO public.proactivity_signal_records (
					owner_identity, batch_idempotency_key, ordinal, signal_id,
					recorded_at, payload
				) VALUES (?, ?, ?, ?, ?, CAST(? AS jsonb))`,
				owner, key, ordinal, record.Signal.ID, record.RecordedAt.UTC(), string(payload),
			)
			if result.Error != nil {
				return fmt.Errorf("record proactivity signal %d: %w", ordinal, result.Error)
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("%w: signal insert affected %d rows", ErrCorruptStorage, result.RowsAffected)
			}
		}
		stored = cloneSignalRecords(records)
		created = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return cloneSignalRecords(stored), created, nil
}

func (r *PostgresRepository) ListSignals(ctx context.Context, owner string, limit int) ([]SignalRecord, error) {
	if err := r.ready(ctx); err != nil {
		return nil, err
	}
	owner, err := repositoryOwner(owner)
	if err != nil {
		return nil, err
	}
	rows, err := queryPostgresSignalRows(r.DB.WithContext(ctx), `
		SELECT owner_identity, batch_idempotency_key, ordinal, signal_id,
			recorded_at, payload::text AS payload
		FROM public.proactivity_signal_records
		WHERE owner_identity = ?
		ORDER BY recorded_at DESC, batch_idempotency_key DESC, ordinal DESC
		LIMIT ?`, owner, boundedLimit(limit, MaxSignalHistory))
	if err != nil {
		return nil, fmt.Errorf("list proactivity signals: %w", err)
	}
	result := make([]SignalRecord, 0, len(rows))
	for _, row := range rows {
		record, decodeErr := decodePostgresSignalRow(row, owner)
		if decodeErr != nil {
			return nil, decodeErr
		}
		result = append(result, record)
	}
	return result, nil
}

func (r *PostgresRepository) LatestSignals(ctx context.Context, owner string, limit int) ([]OpenLoopSignal, error) {
	if err := r.ready(ctx); err != nil {
		return nil, err
	}
	owner, err := repositoryOwner(owner)
	if err != nil {
		return nil, err
	}
	rows, err := queryPostgresSignalRows(r.DB.WithContext(ctx), `
		SELECT owner_identity, batch_idempotency_key, ordinal, signal_id,
			recorded_at, payload
		FROM (
			SELECT DISTINCT ON (signal_id)
				owner_identity, batch_idempotency_key, ordinal, signal_id,
				recorded_at, payload::text AS payload
			FROM public.proactivity_signal_records
			WHERE owner_identity = ?
			ORDER BY signal_id, recorded_at DESC, batch_idempotency_key DESC, ordinal DESC
		) AS latest
		ORDER BY signal_id ASC
		LIMIT ?`, owner, boundedLimit(limit, MaxSignals))
	if err != nil {
		return nil, fmt.Errorf("read latest proactivity signals: %w", err)
	}
	result := make([]OpenLoopSignal, 0, len(rows))
	for _, row := range rows {
		record, decodeErr := decodePostgresSignalRow(row, owner)
		if decodeErr != nil {
			return nil, decodeErr
		}
		result = append(result, cloneSignal(record.Signal))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (r *PostgresRepository) FindDecisionBatch(ctx context.Context, owner, key, digest string) (DecisionBatch, bool, error) {
	if err := r.ready(ctx); err != nil {
		return DecisionBatch{}, false, err
	}
	owner, key, digest, err := normalizePostgresCommand(owner, key, digest)
	if err != nil {
		return DecisionBatch{}, false, err
	}
	entry, found, err := findPostgresIdempotency(r.DB.WithContext(ctx), owner, key)
	if err != nil {
		return DecisionBatch{}, false, err
	}
	if !found {
		return DecisionBatch{}, false, nil
	}
	if entry.RecordKind != idempotencyKindDecisions || entry.PayloadDigest != digest {
		return DecisionBatch{}, false, ErrIdempotencyConflict
	}
	batch, err := loadPostgresDecisionBatch(r.DB.WithContext(ctx), owner, key)
	if err != nil {
		return DecisionBatch{}, false, err
	}
	return batch, true, nil
}

func (r *PostgresRepository) RecordDecisionBatch(ctx context.Context, owner, key, digest string, batch DecisionBatch) (DecisionBatch, bool, error) {
	if err := r.ready(ctx); err != nil {
		return DecisionBatch{}, false, err
	}
	owner, key, digest, err := normalizePostgresCommand(owner, key, digest)
	if err != nil {
		return DecisionBatch{}, false, err
	}
	batch, err = validateStoredDecisionBatch(owner, batch)
	if err != nil {
		return DecisionBatch{}, false, err
	}
	expectedDigest, err := decisionBatchPayloadDigest(owner, batch)
	if err != nil {
		return DecisionBatch{}, false, fmt.Errorf("digest proactivity decision payload: %w", err)
	}
	if digest != expectedDigest {
		return DecisionBatch{}, false, ErrIdempotencyConflict
	}
	batchPayload, err := marshalPostgresPayload("decision batch", batch)
	if err != nil {
		return DecisionBatch{}, false, err
	}

	var stored DecisionBatch
	created := false
	err = r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		reserved, reserveErr := reservePostgresIdempotency(tx, owner, key, idempotencyKindDecisions, digest, batch.RecordedAt)
		if reserveErr != nil {
			return reserveErr
		}
		if !reserved {
			stored, reserveErr = loadPostgresDecisionBatch(tx, owner, key)
			return reserveErr
		}
		batchResult := tx.Exec(`
			INSERT INTO public.proactivity_decision_batches (
				owner_identity, idempotency_key, record_kind, payload_digest, decision_count,
				recorded_at, payload
			) VALUES (?, ?, ?, ?, ?, ?, CAST(? AS jsonb))`,
			owner, key, idempotencyKindDecisions, digest, len(batch.Result.Decisions), batch.RecordedAt.UTC(), string(batchPayload),
		)
		if batchResult.Error != nil {
			return fmt.Errorf("record proactivity decision batch: %w", batchResult.Error)
		}
		if batchResult.RowsAffected != 1 {
			return fmt.Errorf("%w: decision batch insert affected %d rows", ErrCorruptStorage, batchResult.RowsAffected)
		}
		for ordinal, decision := range batch.Result.Decisions {
			record := DecisionRecord{
				ContractVersion: ContractVersion,
				OwnerIdentity:   owner,
				Decision:        cloneDecision(decision),
				RecordedAt:      batch.RecordedAt.UTC(),
			}
			payload, marshalErr := marshalPostgresPayload("decision record", record)
			if marshalErr != nil {
				return marshalErr
			}
			result := tx.Exec(`
				INSERT INTO public.proactivity_decision_records (
					owner_identity, batch_idempotency_key, ordinal, signal_id,
					open_loop_key, outcome, recorded_at, payload
				) VALUES (?, ?, ?, ?, ?, ?, ?, CAST(? AS jsonb))`,
				owner, key, ordinal, decision.SignalID, decision.OpenLoopKey,
				string(decision.Outcome), record.RecordedAt, string(payload),
			)
			if result.Error != nil {
				return fmt.Errorf("record proactivity decision %d: %w", ordinal, result.Error)
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("%w: decision insert affected %d rows", ErrCorruptStorage, result.RowsAffected)
			}
		}
		stored = cloneDecisionBatch(batch)
		created = true
		return nil
	})
	if err != nil {
		return DecisionBatch{}, false, err
	}
	return cloneDecisionBatch(stored), created, nil
}

func (r *PostgresRepository) ListDecisions(ctx context.Context, owner string, limit int) ([]DecisionRecord, error) {
	if err := r.ready(ctx); err != nil {
		return nil, err
	}
	owner, err := repositoryOwner(owner)
	if err != nil {
		return nil, err
	}
	var rows []postgresDecisionRow
	err = r.DB.WithContext(ctx).Raw(`
		SELECT owner_identity, batch_idempotency_key, ordinal, signal_id,
			open_loop_key, outcome, recorded_at, payload::text AS payload
		FROM public.proactivity_decision_records
		WHERE owner_identity = ?
		ORDER BY recorded_at DESC, batch_idempotency_key DESC, ordinal DESC
		LIMIT ?`, owner, boundedLimit(limit, MaxDecisionHistory)).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list proactivity decisions: %w", err)
	}
	result := make([]DecisionRecord, 0, len(rows))
	for _, row := range rows {
		record, decodeErr := decodePostgresDecisionRow(row, owner)
		if decodeErr != nil {
			return nil, decodeErr
		}
		result = append(result, record)
	}
	return result, nil
}

func (r *PostgresRepository) RecordFeedback(ctx context.Context, owner, key, digest string, record FeedbackRecord) (FeedbackRecord, bool, error) {
	if err := r.ready(ctx); err != nil {
		return FeedbackRecord{}, false, err
	}
	owner, key, digest, err := normalizePostgresCommand(owner, key, digest)
	if err != nil {
		return FeedbackRecord{}, false, err
	}
	record, err = validateStoredFeedbackRecord(owner, record)
	if err != nil {
		return FeedbackRecord{}, false, err
	}
	// Resolve durable replays before INSERT. PostgreSQL executes BEFORE INSERT
	// triggers before ON CONFLICT, so a replay made after a successor record
	// exists would otherwise fail the append-chain guard before reaching the
	// idempotency conflict handler.
	row, found, err := loadPostgresFeedbackByIdempotency(r.DB.WithContext(ctx), owner, key)
	if err != nil {
		return FeedbackRecord{}, false, err
	}
	if found {
		if row.RequestDigest != digest {
			return FeedbackRecord{}, false, ErrIdempotencyConflict
		}
		stored, decodeErr := decodePostgresFeedbackRow(row, owner)
		if decodeErr != nil {
			return FeedbackRecord{}, false, decodeErr
		}
		return cloneFeedbackRecord(stored), false, nil
	}
	payload, err := marshalPostgresPayload("feedback record", record)
	if err != nil {
		return FeedbackRecord{}, false, err
	}

	created := false
	stored := FeedbackRecord{}
	err = r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Exec(`
			INSERT INTO public.proactivity_feedback_records (
				owner_identity, feedback_id, idempotency_key, request_digest,
				signal_id, open_loop_key, signal_digest, source_outcome,
				source_decision_at, action, snoozed_until, previous_record_digest,
				record_digest, recorded_at, authority, can_execute,
				delivery_authorized, execution_authorized, payload
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, CAST(? AS jsonb))
			ON CONFLICT (owner_identity, idempotency_key) DO NOTHING`,
			owner, record.ID, key, digest, record.SignalID, record.OpenLoopKey,
			record.SignalDigest, string(record.SourceOutcome), record.SourceDecisionAt,
			string(record.Action), record.SnoozedUntil, record.PreviousRecordDigest,
			record.RecordDigest, record.RecordedAt, record.Authority, record.CanExecute,
			record.DeliveryAuthorized, record.ExecutionAuthorized, string(payload),
		)
		if result.Error != nil {
			return fmt.Errorf("record proactivity feedback: %w", result.Error)
		}
		if result.RowsAffected == 1 {
			stored = cloneFeedbackRecord(record)
			created = true
			return nil
		}
		row, found, loadErr := loadPostgresFeedbackByIdempotency(tx, owner, key)
		if loadErr != nil {
			return loadErr
		}
		if !found || row.RequestDigest != digest {
			return ErrIdempotencyConflict
		}
		stored, loadErr = decodePostgresFeedbackRow(row, owner)
		return loadErr
	})
	if err != nil {
		return FeedbackRecord{}, false, err
	}
	return cloneFeedbackRecord(stored), created, nil
}

func (r *PostgresRepository) LatestFeedback(ctx context.Context, owner, openLoopKey string) (FeedbackRecord, bool, error) {
	if err := r.ready(ctx); err != nil {
		return FeedbackRecord{}, false, err
	}
	owner, err := repositoryOwner(owner)
	if err != nil {
		return FeedbackRecord{}, false, err
	}
	openLoopKey = strings.TrimSpace(openLoopKey)
	if err := validateIdentifier("open loop key", openLoopKey); err != nil {
		return FeedbackRecord{}, false, err
	}
	var row postgresFeedbackRow
	err = r.DB.WithContext(ctx).Raw(`
		SELECT owner_identity, feedback_id, idempotency_key, request_digest,
			signal_id, open_loop_key, signal_digest, source_outcome, source_decision_at,
			action, snoozed_until, COALESCE(previous_record_digest, '') AS previous_record_digest,
			record_digest, recorded_at, authority, can_execute, delivery_authorized,
			execution_authorized, payload::text AS payload
		FROM public.proactivity_feedback_records
		WHERE owner_identity = ? AND open_loop_key = ?
		ORDER BY recorded_at DESC, feedback_id DESC
		LIMIT 1`, owner, openLoopKey).Scan(&row).Error
	if err != nil {
		return FeedbackRecord{}, false, fmt.Errorf("read latest proactivity feedback: %w", err)
	}
	if row.OwnerIdentity == "" {
		return FeedbackRecord{}, false, nil
	}
	record, err := decodePostgresFeedbackRow(row, owner)
	return record, err == nil, err
}

func (r *PostgresRepository) ListFeedback(ctx context.Context, owner string, limit int) ([]FeedbackRecord, error) {
	if err := r.ready(ctx); err != nil {
		return nil, err
	}
	owner, err := repositoryOwner(owner)
	if err != nil {
		return nil, err
	}
	var rows []postgresFeedbackRow
	err = r.DB.WithContext(ctx).Raw(`
		SELECT owner_identity, feedback_id, idempotency_key, request_digest,
			signal_id, open_loop_key, signal_digest, source_outcome, source_decision_at,
			action, snoozed_until, COALESCE(previous_record_digest, '') AS previous_record_digest,
			record_digest, recorded_at, authority, can_execute, delivery_authorized,
			execution_authorized, payload::text AS payload
		FROM public.proactivity_feedback_records
		WHERE owner_identity = ?
		ORDER BY recorded_at DESC, feedback_id DESC
		LIMIT ?`, owner, boundedLimit(limit, MaxFeedbackHistory)).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list proactivity feedback: %w", err)
	}
	result := make([]FeedbackRecord, 0, len(rows))
	for _, row := range rows {
		record, decodeErr := decodePostgresFeedbackRow(row, owner)
		if decodeErr != nil {
			return nil, decodeErr
		}
		result = append(result, record)
	}
	return result, nil
}

func (r *PostgresRepository) ready(ctx context.Context) error {
	if err := repositoryContext(ctx); err != nil {
		return err
	}
	if r == nil || r.DB == nil {
		return ErrRepositoryUnavailable
	}
	return nil
}

func validatePostgresSchema(db *gorm.DB) error {
	if db == nil {
		return ErrRepositoryUnavailable
	}
	for _, relation := range []string{
		"public.proactivity_idempotency",
		"public.proactivity_policy_records",
		"public.proactivity_signal_batches",
		"public.proactivity_signal_records",
		"public.proactivity_decision_batches",
		"public.proactivity_decision_records",
		"public.proactivity_feedback_records",
	} {
		var resolved sql.NullString
		if err := db.Raw(`SELECT to_regclass(?)::text`, relation).Row().Scan(&resolved); err != nil {
			return fmt.Errorf("verify proactivity schema %s: %w", relation, err)
		}
		if !resolved.Valid || strings.TrimSpace(resolved.String) == "" {
			return fmt.Errorf("%w: required relation %s is missing", ErrRepositoryUnavailable, relation)
		}
	}
	return nil
}

func validatePostgresPayloadDigest(kind, owner string, value any, actual string) error {
	expected, err := advisoryDigest(kind, owner, value)
	if err != nil {
		return fmt.Errorf("digest proactivity %s payload: %w", kind, err)
	}
	if actual != expected {
		return fmt.Errorf("%w: %s payload digest mismatch", ErrIdempotencyConflict, kind)
	}
	return nil
}

type postgresIdempotencyRow struct {
	OwnerIdentity  string
	IdempotencyKey string
	RecordKind     string
	PayloadDigest  string
	RecordedAt     time.Time
}

type postgresPolicyRow struct {
	OwnerIdentity  string
	IdempotencyKey string
	PayloadDigest  string
	RecordedAt     time.Time
	Payload        string
}

type postgresSignalBatchRow struct {
	OwnerIdentity  string
	IdempotencyKey string
	PayloadDigest  string
	SignalCount    int
	RecordedAt     time.Time
	Payload        string
}

type postgresSignalRow struct {
	OwnerIdentity       string
	BatchIdempotencyKey string
	Ordinal             int
	SignalID            string
	RecordedAt          time.Time
	Payload             string
}

type postgresDecisionBatchRow struct {
	OwnerIdentity  string
	IdempotencyKey string
	PayloadDigest  string
	DecisionCount  int
	RecordedAt     time.Time
	Payload        string
}

type postgresDecisionRow struct {
	OwnerIdentity       string
	BatchIdempotencyKey string
	Ordinal             int
	SignalID            string
	OpenLoopKey         string
	Outcome             Outcome
	RecordedAt          time.Time
	Payload             string
}

type postgresFeedbackRow struct {
	OwnerIdentity        string
	FeedbackID           string
	IdempotencyKey       string
	RequestDigest        string
	SignalID             string
	OpenLoopKey          string
	SignalDigest         string
	SourceOutcome        Outcome
	SourceDecisionAt     time.Time
	Action               FeedbackAction
	SnoozedUntil         sql.NullTime
	PreviousRecordDigest string
	RecordDigest         string
	RecordedAt           time.Time
	Authority            string
	CanExecute           bool
	DeliveryAuthorized   bool
	ExecutionAuthorized  bool
	Payload              string
}

func loadPostgresFeedbackByIdempotency(db *gorm.DB, owner, key string) (postgresFeedbackRow, bool, error) {
	var row postgresFeedbackRow
	err := db.Raw(`
		SELECT owner_identity, feedback_id, idempotency_key, request_digest,
			signal_id, open_loop_key, signal_digest, source_outcome, source_decision_at,
			action, snoozed_until, COALESCE(previous_record_digest, '') AS previous_record_digest,
			record_digest, recorded_at, authority, can_execute, delivery_authorized,
			execution_authorized, payload::text AS payload
		FROM public.proactivity_feedback_records
		WHERE owner_identity = ? AND idempotency_key = ?`, owner, key).Scan(&row).Error
	if err != nil {
		return postgresFeedbackRow{}, false, fmt.Errorf("read proactivity feedback replay: %w", err)
	}
	return row, row.OwnerIdentity != "", nil
}

func decodePostgresFeedbackRow(row postgresFeedbackRow, expectedOwner string) (FeedbackRecord, error) {
	var record FeedbackRecord
	if err := decodePostgresPayload("feedback record", row.Payload, &record); err != nil {
		return FeedbackRecord{}, err
	}
	cleaned, err := validateStoredFeedbackRecord(expectedOwner, record)
	if err != nil {
		return FeedbackRecord{}, corruptPostgresRecord("feedback record", err)
	}
	var snoozed *time.Time
	if row.SnoozedUntil.Valid {
		value := row.SnoozedUntil.Time.UTC()
		snoozed = &value
	}
	if row.OwnerIdentity != expectedOwner || row.FeedbackID != cleaned.ID ||
		strings.TrimSpace(row.IdempotencyKey) == "" || !digestPattern.MatchString(row.RequestDigest) ||
		row.SignalID != cleaned.SignalID || row.OpenLoopKey != cleaned.OpenLoopKey ||
		row.SignalDigest != cleaned.SignalDigest || row.SourceOutcome != cleaned.SourceOutcome ||
		!postgresTimestampEqual(row.SourceDecisionAt, cleaned.SourceDecisionAt) || row.Action != cleaned.Action ||
		!timePointersEqual(snoozed, cleaned.SnoozedUntil) || row.PreviousRecordDigest != cleaned.PreviousRecordDigest ||
		row.RecordDigest != cleaned.RecordDigest || !postgresTimestampEqual(row.RecordedAt, cleaned.RecordedAt) ||
		row.Authority != FeedbackAuthority || row.CanExecute || row.DeliveryAuthorized || row.ExecutionAuthorized {
		return FeedbackRecord{}, corruptPostgresRecord("feedback metadata", nil)
	}
	return cleaned, nil
}

func validateStoredFeedbackRecord(owner string, record FeedbackRecord) (FeedbackRecord, error) {
	cleaned, err := sanitizeFeedbackRecord(owner, cloneFeedbackRecord(record))
	if err != nil {
		return FeedbackRecord{}, err
	}
	if !reflect.DeepEqual(cleaned, record) {
		return FeedbackRecord{}, errors.New("proactivity feedback record is not canonical")
	}
	return cleaned, nil
}

func timePointersEqual(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return postgresTimestampEqual(*left, *right)
}

func reservePostgresIdempotency(tx *gorm.DB, owner, key, kind, digest string, recordedAt time.Time) (bool, error) {
	result := tx.Exec(`
		INSERT INTO public.proactivity_idempotency (
			owner_identity, idempotency_key, record_kind, payload_digest, recorded_at
		) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (owner_identity, idempotency_key) DO NOTHING`,
		owner, key, kind, digest, recordedAt.UTC(),
	)
	if result.Error != nil {
		return false, fmt.Errorf("reserve proactivity idempotency: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return true, nil
	}
	entry, found, err := findPostgresIdempotency(tx, owner, key)
	if err != nil {
		return false, err
	}
	if !found {
		return false, fmt.Errorf("%w: idempotency reservation disappeared", ErrCorruptStorage)
	}
	if entry.RecordKind != kind || entry.PayloadDigest != digest {
		return false, ErrIdempotencyConflict
	}
	return false, nil
}

func findPostgresIdempotency(db *gorm.DB, owner, key string) (postgresIdempotencyRow, bool, error) {
	var row postgresIdempotencyRow
	err := db.Raw(`
		SELECT owner_identity, idempotency_key, record_kind, payload_digest, recorded_at
		FROM public.proactivity_idempotency
		WHERE owner_identity = ? AND idempotency_key = ?`, owner, key).Scan(&row).Error
	if err != nil {
		return postgresIdempotencyRow{}, false, fmt.Errorf("read proactivity idempotency: %w", err)
	}
	if row.OwnerIdentity == "" {
		return postgresIdempotencyRow{}, false, nil
	}
	if row.OwnerIdentity != owner || row.IdempotencyKey != key ||
		!validIdempotencyKind(row.RecordKind) || !digestPattern.MatchString(row.PayloadDigest) || row.RecordedAt.IsZero() {
		return postgresIdempotencyRow{}, false, fmt.Errorf("%w: invalid idempotency record", ErrCorruptStorage)
	}
	return row, true, nil
}

func loadPostgresPolicy(db *gorm.DB, owner, key string) (PolicyRecord, error) {
	var row postgresPolicyRow
	err := db.Raw(`
		SELECT owner_identity, idempotency_key, payload_digest, recorded_at,
			payload::text AS payload
		FROM public.proactivity_policy_records
		WHERE owner_identity = ? AND idempotency_key = ?`, owner, key).Scan(&row).Error
	if err != nil {
		return PolicyRecord{}, fmt.Errorf("read proactivity policy replay: %w", err)
	}
	if row.OwnerIdentity == "" {
		return PolicyRecord{}, fmt.Errorf("%w: policy replay record missing", ErrCorruptStorage)
	}
	return decodePostgresPolicyRow(row, owner)
}

func loadPostgresSignalBatch(db *gorm.DB, owner, key string) ([]SignalRecord, error) {
	var row postgresSignalBatchRow
	err := db.Raw(`
		SELECT owner_identity, idempotency_key, payload_digest, signal_count,
			recorded_at, payload::text AS payload
		FROM public.proactivity_signal_batches
		WHERE owner_identity = ? AND idempotency_key = ?`, owner, key).Scan(&row).Error
	if err != nil {
		return nil, fmt.Errorf("read proactivity signal replay: %w", err)
	}
	if row.OwnerIdentity == "" {
		return nil, fmt.Errorf("%w: signal replay record missing", ErrCorruptStorage)
	}
	batch, err := decodePostgresSignalBatchRow(row, owner)
	if err != nil {
		return nil, err
	}
	children, err := queryPostgresSignalRows(db, `
		SELECT owner_identity, batch_idempotency_key, ordinal, signal_id,
			recorded_at, payload::text AS payload
		FROM public.proactivity_signal_records
		WHERE owner_identity = ? AND batch_idempotency_key = ?
		ORDER BY ordinal ASC`, owner, key)
	if err != nil {
		return nil, fmt.Errorf("read proactivity signal replay records: %w", err)
	}
	if len(children) != len(batch) {
		return nil, corruptPostgresRecord("signal batch child count", nil)
	}
	for index, child := range children {
		record, decodeErr := decodePostgresSignalRow(child, owner)
		if decodeErr != nil {
			return nil, decodeErr
		}
		if child.Ordinal != index || !reflect.DeepEqual(record, batch[index]) {
			return nil, corruptPostgresRecord("signal batch child mismatch", nil)
		}
	}
	return batch, nil
}

func loadPostgresDecisionBatch(db *gorm.DB, owner, key string) (DecisionBatch, error) {
	var row postgresDecisionBatchRow
	err := db.Raw(`
		SELECT owner_identity, idempotency_key, payload_digest, decision_count,
			recorded_at, payload::text AS payload
		FROM public.proactivity_decision_batches
		WHERE owner_identity = ? AND idempotency_key = ?`, owner, key).Scan(&row).Error
	if err != nil {
		return DecisionBatch{}, fmt.Errorf("read proactivity decision replay: %w", err)
	}
	if row.OwnerIdentity == "" {
		return DecisionBatch{}, fmt.Errorf("%w: decision replay record missing", ErrCorruptStorage)
	}
	batch, err := decodePostgresDecisionBatchRow(row, owner)
	if err != nil {
		return DecisionBatch{}, err
	}
	var children []postgresDecisionRow
	if err := db.Raw(`
		SELECT owner_identity, batch_idempotency_key, ordinal, signal_id,
			open_loop_key, outcome, recorded_at, payload::text AS payload
		FROM public.proactivity_decision_records
		WHERE owner_identity = ? AND batch_idempotency_key = ?
		ORDER BY ordinal ASC`, owner, key).Scan(&children).Error; err != nil {
		return DecisionBatch{}, fmt.Errorf("read proactivity decision replay records: %w", err)
	}
	if len(children) != len(batch.Result.Decisions) {
		return DecisionBatch{}, corruptPostgresRecord("decision batch child count", nil)
	}
	for index, child := range children {
		record, decodeErr := decodePostgresDecisionRow(child, owner)
		if decodeErr != nil {
			return DecisionBatch{}, decodeErr
		}
		if child.Ordinal != index || !reflect.DeepEqual(record.Decision, batch.Result.Decisions[index]) ||
			!record.RecordedAt.Equal(batch.RecordedAt) {
			return DecisionBatch{}, corruptPostgresRecord("decision batch child mismatch", nil)
		}
	}
	return batch, nil
}

func queryPostgresSignalRows(db *gorm.DB, statement string, args ...any) ([]postgresSignalRow, error) {
	var rows []postgresSignalRow
	if err := db.Raw(statement, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func decodePostgresPolicyRow(row postgresPolicyRow, expectedOwner string) (PolicyRecord, error) {
	var record PolicyRecord
	if err := decodePostgresPayload("policy", row.Payload, &record); err != nil {
		return PolicyRecord{}, err
	}
	cleaned, err := validateStoredPolicyRecord(expectedOwner, record)
	if err != nil {
		return PolicyRecord{}, corruptPostgresRecord("policy", err)
	}
	if row.OwnerIdentity != expectedOwner || row.OwnerIdentity != cleaned.OwnerIdentity ||
		strings.TrimSpace(row.IdempotencyKey) == "" || !digestPattern.MatchString(row.PayloadDigest) ||
		!postgresTimestampEqual(row.RecordedAt, cleaned.RecordedAt) {
		return PolicyRecord{}, corruptPostgresRecord("policy metadata", nil)
	}
	expectedDigest, err := advisoryDigest(idempotencyKindPolicy, expectedOwner, cleaned.Policy)
	if err != nil || row.PayloadDigest != expectedDigest {
		return PolicyRecord{}, corruptPostgresRecord("policy digest", err)
	}
	return cleaned, nil
}

func decodePostgresSignalBatchRow(row postgresSignalBatchRow, expectedOwner string) ([]SignalRecord, error) {
	var records []SignalRecord
	if err := decodePostgresPayload("signal batch", row.Payload, &records); err != nil {
		return nil, err
	}
	cleaned, err := validateStoredSignalRecords(expectedOwner, records)
	if err != nil {
		return nil, corruptPostgresRecord("signal batch", err)
	}
	if row.OwnerIdentity != expectedOwner || strings.TrimSpace(row.IdempotencyKey) == "" ||
		!digestPattern.MatchString(row.PayloadDigest) || row.SignalCount != len(cleaned) ||
		len(cleaned) == 0 || !postgresTimestampEqual(row.RecordedAt, cleaned[0].RecordedAt) {
		return nil, corruptPostgresRecord("signal batch metadata", nil)
	}
	signals := make([]OpenLoopSignal, len(cleaned))
	for index := range cleaned {
		signals[index] = cleaned[index].Signal
	}
	expectedDigest, err := advisoryDigest(idempotencyKindSignals, expectedOwner, signals)
	if err != nil || row.PayloadDigest != expectedDigest {
		return nil, corruptPostgresRecord("signal batch digest", err)
	}
	return cleaned, nil
}

func decodePostgresSignalRow(row postgresSignalRow, expectedOwner string) (SignalRecord, error) {
	var record SignalRecord
	if err := decodePostgresPayload("signal record", row.Payload, &record); err != nil {
		return SignalRecord{}, err
	}
	cleaned, err := validateStoredSignalRecords(expectedOwner, []SignalRecord{record})
	if err != nil {
		return SignalRecord{}, corruptPostgresRecord("signal record", err)
	}
	if row.OwnerIdentity != expectedOwner || strings.TrimSpace(row.BatchIdempotencyKey) == "" ||
		row.Ordinal < 0 || row.SignalID != cleaned[0].Signal.ID ||
		!postgresTimestampEqual(row.RecordedAt, cleaned[0].RecordedAt) {
		return SignalRecord{}, corruptPostgresRecord("signal metadata", nil)
	}
	return cleaned[0], nil
}

func decodePostgresDecisionBatchRow(row postgresDecisionBatchRow, expectedOwner string) (DecisionBatch, error) {
	var batch DecisionBatch
	if err := decodePostgresPayload("decision batch", row.Payload, &batch); err != nil {
		return DecisionBatch{}, err
	}
	cleaned, err := validateStoredDecisionBatch(expectedOwner, batch)
	if err != nil {
		return DecisionBatch{}, corruptPostgresRecord("decision batch", err)
	}
	if row.OwnerIdentity != expectedOwner || strings.TrimSpace(row.IdempotencyKey) == "" ||
		!digestPattern.MatchString(row.PayloadDigest) || row.DecisionCount != len(cleaned.Result.Decisions) ||
		!postgresTimestampEqual(row.RecordedAt, cleaned.RecordedAt) {
		return DecisionBatch{}, corruptPostgresRecord("decision batch metadata", nil)
	}
	expectedDigest, err := decisionBatchPayloadDigest(expectedOwner, cleaned)
	if err != nil || row.PayloadDigest != expectedDigest {
		return DecisionBatch{}, corruptPostgresRecord("decision batch digest", err)
	}
	return cleaned, nil
}

func decodePostgresDecisionRow(row postgresDecisionRow, expectedOwner string) (DecisionRecord, error) {
	var record DecisionRecord
	if err := decodePostgresPayload("decision record", row.Payload, &record); err != nil {
		return DecisionRecord{}, err
	}
	cleaned, err := validateStoredDecisionRecord(expectedOwner, record)
	if err != nil {
		return DecisionRecord{}, corruptPostgresRecord("decision record", err)
	}
	if row.OwnerIdentity != expectedOwner || strings.TrimSpace(row.BatchIdempotencyKey) == "" || row.Ordinal < 0 ||
		row.SignalID != cleaned.Decision.SignalID || row.OpenLoopKey != cleaned.Decision.OpenLoopKey ||
		row.Outcome != cleaned.Decision.Outcome || !postgresTimestampEqual(row.RecordedAt, cleaned.RecordedAt) {
		return DecisionRecord{}, corruptPostgresRecord("decision metadata", nil)
	}
	return cleaned, nil
}

func normalizePostgresCommand(owner, key, digest string) (string, string, string, error) {
	owner = strings.TrimSpace(owner)
	key = strings.TrimSpace(key)
	digest = strings.TrimSpace(digest)
	if err := validateRepositoryCommand(owner, key, digest); err != nil {
		return "", "", "", err
	}
	return owner, key, digest, nil
}

func validateStoredPolicyRecord(owner string, record PolicyRecord) (PolicyRecord, error) {
	cleaned, err := sanitizePolicyRecord(owner, clonePolicyRecord(record))
	if err != nil {
		return PolicyRecord{}, err
	}
	if !reflect.DeepEqual(cleaned, record) {
		return PolicyRecord{}, errors.New("proactivity policy record is not canonical")
	}
	return cleaned, nil
}

func validateStoredSignalRecords(owner string, records []SignalRecord) ([]SignalRecord, error) {
	if len(records) == 0 || len(records) > MaxSignals {
		return nil, errors.New("proactivity signal batch is invalid")
	}
	cleaned := make([]SignalRecord, len(records))
	seen := make(map[string]struct{}, len(records))
	var recordedAt time.Time
	for index, record := range records {
		if err := validateSignalRecordOwner(owner, record); err != nil {
			return nil, err
		}
		if index == 0 {
			recordedAt = record.RecordedAt
		} else if !recordedAt.Equal(record.RecordedAt) {
			return nil, errors.New("signal batch timestamps do not match")
		}
		normalized, err := normalizeSignal(owner, cloneSignal(record.Signal), record.RecordedAt.UTC())
		if err != nil {
			return nil, err
		}
		canonical := cloneSignalRecord(record)
		canonical.RecordedAt = canonical.RecordedAt.UTC()
		canonical.Signal = normalized
		if !reflect.DeepEqual(canonical, record) {
			return nil, errors.New("proactivity signal record is not canonical")
		}
		if _, exists := seen[record.Signal.ID]; exists {
			return nil, errors.New("signal ids must be unique within a batch")
		}
		seen[record.Signal.ID] = struct{}{}
		cleaned[index] = canonical
	}
	return cleaned, nil
}

func validateStoredDecisionBatch(owner string, batch DecisionBatch) (DecisionBatch, error) {
	if err := validateDecisionBatchOwner(owner, batch); err != nil {
		return DecisionBatch{}, err
	}
	if batch.Result.DecidedAt.IsZero() || batch.RecordedAt.IsZero() ||
		batch.Result.InterruptionsUsed < 0 || batch.Result.InterruptionsUsed > MaxHistoryEntries+MaxSignals ||
		batch.Result.InterruptionsRemaining < 0 || batch.Result.InterruptionsRemaining > 100 {
		return DecisionBatch{}, errors.New("proactivity decision batch metadata is invalid")
	}
	if (batch.SnapshotInputDigest != "" && !digestPattern.MatchString(batch.SnapshotInputDigest)) ||
		(batch.AdditionalSignalsDigest != "" && !digestPattern.MatchString(batch.AdditionalSignalsDigest)) {
		return DecisionBatch{}, errors.New("proactivity decision snapshot digest is invalid")
	}
	if _, err := time.LoadLocation(strings.TrimSpace(batch.Result.TimeZone)); err != nil {
		return DecisionBatch{}, errors.New("proactivity decision batch time zone is invalid")
	}
	cleaned := cloneDecisionBatch(batch)
	cleaned.RecordedAt = cleaned.RecordedAt.UTC()
	cleaned.Result.DecidedAt = cleaned.Result.DecidedAt.UTC()
	seen := make(map[string]struct{}, len(cleaned.Result.Decisions))
	for index, decision := range cleaned.Result.Decisions {
		validated, err := validateStoredDecision(owner, decision, cleaned.Result.DecidedAt)
		if err != nil {
			return DecisionBatch{}, fmt.Errorf("decision %d: %w", index, err)
		}
		if _, exists := seen[validated.SignalID]; exists {
			return DecisionBatch{}, errors.New("decision signal ids must be unique within a batch")
		}
		seen[validated.SignalID] = struct{}{}
		cleaned.Result.Decisions[index] = validated
	}
	if !reflect.DeepEqual(cleaned, batch) {
		return DecisionBatch{}, errors.New("proactivity decision batch is not canonical")
	}
	return cleaned, nil
}

func validateStoredDecisionRecord(owner string, record DecisionRecord) (DecisionRecord, error) {
	if record.ContractVersion != ContractVersion || record.OwnerIdentity != owner || record.RecordedAt.IsZero() {
		return DecisionRecord{}, ErrOwnerScopeViolation
	}
	decision, err := validateStoredDecision(owner, record.Decision, record.Decision.DecidedAt)
	if err != nil {
		return DecisionRecord{}, err
	}
	cleaned := cloneDecisionRecord(record)
	cleaned.RecordedAt = cleaned.RecordedAt.UTC()
	cleaned.Decision = decision
	if !reflect.DeepEqual(cleaned, record) {
		return DecisionRecord{}, errors.New("proactivity decision record is not canonical")
	}
	return cleaned, nil
}

func validateStoredDecision(owner string, decision Decision, decidedAt time.Time) (Decision, error) {
	if decision.ContractVersion != ContractVersion || decision.OwnerIdentity != owner ||
		decision.ExecutionAuthorized || decision.DeliveryAuthorized || decision.AuthorityGranted {
		return Decision{}, ErrOwnerScopeViolation
	}
	if err := validateIdentifier("decision signal id", strings.TrimSpace(decision.SignalID)); err != nil {
		return Decision{}, err
	}
	if err := validateIdentifier("decision open-loop key", strings.TrimSpace(decision.OpenLoopKey)); err != nil {
		return Decision{}, err
	}
	if !digestPattern.MatchString(strings.TrimSpace(decision.SignalDigest)) || !validOutcome(decision.Outcome) ||
		decision.DecidedAt.IsZero() || !decision.DecidedAt.Equal(decidedAt) || decision.BudgetCost < 0 || decision.BudgetCost > 1 {
		return Decision{}, errors.New("proactivity decision fields are invalid")
	}
	if err := validateUnit("decision score", decision.Score); err != nil {
		return Decision{}, err
	}
	if decision.Title == "" || decision.Summary == "" ||
		redactAndBound(decision.Title, maxTitleLength) != decision.Title ||
		redactAndBound(decision.Summary, maxSummaryLength) != decision.Summary ||
		len(decision.Components) == 0 || len(decision.Components) > 32 || len(decision.Reasons) == 0 || len(decision.Reasons) > 32 {
		return Decision{}, errors.New("proactivity decision content is invalid")
	}
	for _, component := range decision.Components {
		if strings.TrimSpace(component.Name) == "" || redactAndBound(component.Name, 80) != component.Name ||
			strings.TrimSpace(component.Explanation) == "" || redactAndBound(component.Explanation, 300) != component.Explanation ||
			math.IsNaN(component.Value) || math.IsInf(component.Value, 0) ||
			math.IsNaN(component.Weight) || math.IsInf(component.Weight, 0) ||
			math.IsNaN(component.Contribution) || math.IsInf(component.Contribution, 0) {
			return Decision{}, errors.New("proactivity decision score component is invalid")
		}
	}
	for _, reason := range decision.Reasons {
		if strings.TrimSpace(reason) == "" || redactAndBound(reason, 500) != reason {
			return Decision{}, errors.New("proactivity decision reason is invalid")
		}
	}
	seenChannels := make(map[Channel]struct{}, len(decision.RecommendedChannels))
	for _, channel := range decision.RecommendedChannels {
		if !validChannel(channel) {
			return Decision{}, errors.New("proactivity decision channel is invalid")
		}
		if _, exists := seenChannels[channel]; exists {
			return Decision{}, errors.New("proactivity decision channels must be unique")
		}
		seenChannels[channel] = struct{}{}
	}
	cleaned := advisoryDecision(cloneDecision(decision))
	cleaned.DecidedAt = cleaned.DecidedAt.UTC()
	if !reflect.DeepEqual(cleaned, decision) {
		return Decision{}, errors.New("proactivity decision is not canonical")
	}
	return cleaned, nil
}

func marshalPostgresPayload(kind string, value any) ([]byte, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode proactivity %s: %w", kind, err)
	}
	return payload, nil
}

func decodePostgresPayload(kind, payload string, target any) error {
	decoder := json.NewDecoder(bytes.NewBufferString(strings.TrimSpace(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return corruptPostgresRecord(kind, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return corruptPostgresRecord(kind, errors.New("persisted JSON must contain exactly one value"))
	}
	return nil
}

func corruptPostgresRecord(kind string, cause error) error {
	if cause == nil {
		return fmt.Errorf("%w: %s", ErrCorruptStorage, kind)
	}
	return fmt.Errorf("%w: %s: %v", ErrCorruptStorage, kind, cause)
}

func validIdempotencyKind(kind string) bool {
	return kind == idempotencyKindPolicy || kind == idempotencyKindSignals || kind == idempotencyKindDecisions
}

func postgresTimestampEqual(left, right time.Time) bool {
	return left.UTC().Truncate(time.Microsecond).Equal(right.UTC().Truncate(time.Microsecond))
}
