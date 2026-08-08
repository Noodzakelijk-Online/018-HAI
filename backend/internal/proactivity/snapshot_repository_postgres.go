package proactivity

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"time"

	"gorm.io/gorm"
)

type postgresSnapshotSignalRow struct {
	OwnerIdentity       string
	BatchIdempotencyKey string
	Ordinal             int
	SignalID            string
	PayloadDigest       string
	SignalCount         int
	BatchRecordedAt     time.Time
	BatchPayload        string
	RecordedAt          time.Time
	Payload             string
}

type postgresSnapshotDecisionRow struct {
	OwnerIdentity       string
	BatchIdempotencyKey string
	Ordinal             int
	SignalID            string
	OpenLoopKey         string
	Outcome             Outcome
	PayloadDigest       string
	DecisionCount       int
	BatchRecordedAt     time.Time
	BatchPayload        string
	RecordedAt          time.Time
	Payload             string
}

func (r *PostgresRepository) captureEvaluationSnapshotState(ctx context.Context, owner string, at time.Time) (evaluationSnapshotState, error) {
	if err := r.ready(ctx); err != nil {
		return evaluationSnapshotState{}, err
	}
	owner, err := repositoryOwner(owner)
	if err != nil {
		return evaluationSnapshotState{}, err
	}
	at = snapshotTime(at)
	var state evaluationSnapshotState
	err = withPostgresSnapshot(r.DB.WithContext(ctx), func(tx *gorm.DB) error {
		captured, captureErr := capturePostgresEvaluationSnapshotState(tx, owner, at)
		if captureErr != nil {
			return captureErr
		}
		state = captured
		return validateEvaluationSnapshotState(owner, at, state)
	})
	if err != nil {
		return evaluationSnapshotState{}, err
	}
	return canonicalEvaluationSnapshotState(state), nil
}

func (r *PostgresRepository) resolveEvaluationSnapshotState(ctx context.Context, owner string, snapshot EvaluationSnapshot) (evaluationSnapshotState, error) {
	if err := r.ready(ctx); err != nil {
		return evaluationSnapshotState{}, err
	}
	owner, err := repositoryOwner(owner)
	if err != nil {
		return evaluationSnapshotState{}, err
	}
	snapshot, err = validateEvaluationSnapshot(owner, snapshot)
	if err != nil {
		return evaluationSnapshotState{}, err
	}
	var state evaluationSnapshotState
	err = withPostgresSnapshot(r.DB.WithContext(ctx), func(tx *gorm.DB) error {
		resolved, resolveErr := resolvePostgresEvaluationSnapshotState(tx, owner, snapshot)
		if resolveErr != nil {
			return resolveErr
		}
		state = resolved
		if validateErr := validateEvaluationSnapshotState(owner, snapshot.CapturedAt, state); validateErr != nil {
			return validateErr
		}
		return verifySnapshotState(owner, snapshot, state)
	})
	if err != nil {
		return evaluationSnapshotState{}, err
	}
	return canonicalEvaluationSnapshotState(state), nil
}

func withPostgresSnapshot(db *gorm.DB, operation func(*gorm.DB) error) error {
	if db == nil || operation == nil {
		return ErrRepositoryUnavailable
	}
	if db.Statement != nil {
		if _, nested := db.Statement.ConnPool.(*sql.Tx); nested {
			return operation(db)
		}
	}
	tx := db.Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if tx.Error != nil {
		return fmt.Errorf("begin proactivity snapshot transaction: %w", tx.Error)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback().Error
		}
	}()
	if err := operation(tx); err != nil {
		return err
	}
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("commit proactivity snapshot transaction: %w", err)
	}
	committed = true
	return nil
}

func capturePostgresEvaluationSnapshotState(db *gorm.DB, owner string, at time.Time) (evaluationSnapshotState, error) {
	policy, err := capturePostgresPolicy(db, owner, at)
	if err != nil {
		return evaluationSnapshotState{}, err
	}
	signals, err := queryPostgresSnapshotSignals(db, owner, at, nil, MaxSignalHistory)
	if err != nil {
		return evaluationSnapshotState{}, err
	}
	decisions, err := queryPostgresSnapshotDecisions(db, owner, at, nil, MaxDecisionHistory)
	if err != nil {
		return evaluationSnapshotState{}, err
	}
	feedback, err := queryPostgresSnapshotFeedback(db, owner, at, nil, MaxFeedbackHistory)
	if err != nil {
		return evaluationSnapshotState{}, err
	}
	return evaluationSnapshotState{Policy: policy, Signals: signals, Decisions: decisions, Feedback: feedback}, nil
}

func resolvePostgresEvaluationSnapshotState(db *gorm.DB, owner string, snapshot EvaluationSnapshot) (evaluationSnapshotState, error) {
	policy, err := resolvePostgresSnapshotPolicy(db, owner, snapshot.Policy)
	if err != nil {
		return evaluationSnapshotState{}, err
	}
	signals, err := queryPostgresSnapshotSignals(db, owner, snapshot.CapturedAt, snapshot.Signals.Cursor, snapshot.Signals.Count)
	if err != nil {
		return evaluationSnapshotState{}, err
	}
	decisions, err := queryPostgresSnapshotDecisions(db, owner, snapshot.CapturedAt, snapshot.Decisions.Cursor, snapshot.Decisions.Count)
	if err != nil {
		return evaluationSnapshotState{}, err
	}
	feedback, err := queryPostgresSnapshotFeedback(db, owner, snapshot.CapturedAt, snapshot.Feedback.Cursor, snapshot.Feedback.Count)
	if err != nil {
		return evaluationSnapshotState{}, err
	}
	return evaluationSnapshotState{Policy: policy, Signals: signals, Decisions: decisions, Feedback: feedback}, nil
}

func capturePostgresPolicy(db *gorm.DB, owner string, at time.Time) (snapshotPolicyMaterial, error) {
	var row postgresPolicyRow
	if err := db.Raw(`
		SELECT owner_identity, idempotency_key, payload_digest, recorded_at,
			payload::text AS payload
		FROM public.proactivity_policy_records
		WHERE owner_identity = ? AND recorded_at <= ?
		ORDER BY recorded_at DESC, idempotency_key DESC
		LIMIT 1`, owner, at).Scan(&row).Error; err != nil {
		return snapshotPolicyMaterial{}, fmt.Errorf("capture proactivity snapshot policy: %w", err)
	}
	if row.OwnerIdentity == "" {
		return snapshotPolicyMaterial{}, ErrNotFound
	}
	record, err := decodePostgresPolicyRow(row, owner)
	if err != nil {
		return snapshotPolicyMaterial{}, err
	}
	return snapshotPolicyMaterial{
		Reference: PolicySnapshotReference{IdempotencyKey: row.IdempotencyKey, PayloadDigest: row.PayloadDigest, RecordedAt: snapshotTime(row.RecordedAt)},
		Record:    record,
	}, nil
}

func resolvePostgresSnapshotPolicy(db *gorm.DB, owner string, reference PolicySnapshotReference) (snapshotPolicyMaterial, error) {
	var row postgresPolicyRow
	if err := db.Raw(`
		SELECT owner_identity, idempotency_key, payload_digest, recorded_at,
			payload::text AS payload
		FROM public.proactivity_policy_records
		WHERE owner_identity = ? AND idempotency_key = ? AND payload_digest = ? AND recorded_at = ?`,
		owner, reference.IdempotencyKey, reference.PayloadDigest, reference.RecordedAt).Scan(&row).Error; err != nil {
		return snapshotPolicyMaterial{}, fmt.Errorf("resolve exact proactivity snapshot policy: %w", err)
	}
	if row.OwnerIdentity == "" {
		return snapshotPolicyMaterial{}, fmt.Errorf("%w: exact policy was not found", ErrSnapshotUnavailable)
	}
	record, err := decodePostgresPolicyRow(row, owner)
	if err != nil {
		return snapshotPolicyMaterial{}, err
	}
	material := snapshotPolicyMaterial{
		Reference: PolicySnapshotReference{IdempotencyKey: row.IdempotencyKey, PayloadDigest: row.PayloadDigest, RecordedAt: snapshotTime(row.RecordedAt)},
		Record:    record,
	}
	if !policySnapshotReferenceEqual(material.Reference, reference) {
		return snapshotPolicyMaterial{}, fmt.Errorf("%w: exact policy metadata mismatch", ErrSnapshotUnavailable)
	}
	return material, nil
}

func queryPostgresSnapshotSignals(db *gorm.DB, owner string, at time.Time, cursor *SnapshotRecordCursor, limit int) ([]snapshotSignalMaterial, error) {
	if limit == 0 {
		return nil, nil
	}
	query := `
		SELECT r.owner_identity, r.batch_idempotency_key, r.ordinal, r.signal_id,
			b.payload_digest, b.signal_count, b.recorded_at AS batch_recorded_at,
			b.payload::text AS batch_payload, r.recorded_at, r.payload::text AS payload
		FROM public.proactivity_signal_records r
		JOIN public.proactivity_signal_batches b
		  ON b.owner_identity = r.owner_identity AND b.idempotency_key = r.batch_idempotency_key
		WHERE r.owner_identity = ? AND r.recorded_at <= ?`
	args := []any{owner, at}
	if cursor != nil {
		query += ` AND (r.recorded_at, r.batch_idempotency_key, r.ordinal) <= (?, ?, ?)`
		args = append(args, cursor.RecordedAt, cursor.IdempotencyKey, cursor.Ordinal)
	}
	query += ` ORDER BY r.recorded_at DESC, r.batch_idempotency_key DESC, r.ordinal DESC LIMIT ?`
	args = append(args, limit)
	var rows []postgresSnapshotSignalRow
	if err := db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("read bounded proactivity snapshot signals: %w", err)
	}
	result := make([]snapshotSignalMaterial, 0, len(rows))
	batches := make(map[string][]SignalRecord)
	for _, row := range rows {
		record, err := decodePostgresSignalRow(postgresSignalRow{
			OwnerIdentity: row.OwnerIdentity, BatchIdempotencyKey: row.BatchIdempotencyKey,
			Ordinal: row.Ordinal, SignalID: row.SignalID, RecordedAt: row.RecordedAt, Payload: row.Payload,
		}, owner)
		if err != nil {
			return nil, err
		}
		batch, found := batches[row.BatchIdempotencyKey]
		if !found {
			decoded, decodeErr := decodePostgresSignalBatchRow(postgresSignalBatchRow{
				OwnerIdentity: owner, IdempotencyKey: row.BatchIdempotencyKey,
				PayloadDigest: row.PayloadDigest, SignalCount: row.SignalCount,
				RecordedAt: row.BatchRecordedAt, Payload: row.BatchPayload,
			}, owner)
			if decodeErr != nil {
				return nil, decodeErr
			}
			batch = decoded
			batches[row.BatchIdempotencyKey] = batch
		}
		if row.Ordinal < 0 || row.Ordinal >= len(batch) || !reflect.DeepEqual(batch[row.Ordinal], record) {
			return nil, corruptPostgresRecord("snapshot signal batch binding", nil)
		}
		result = append(result, snapshotSignalMaterial{
			Cursor: SnapshotRecordCursor{RecordedAt: snapshotTime(row.RecordedAt), IdempotencyKey: row.BatchIdempotencyKey, Ordinal: row.Ordinal, PayloadDigest: row.PayloadDigest},
			Record: record,
		})
	}
	return result, nil
}

func queryPostgresSnapshotDecisions(db *gorm.DB, owner string, at time.Time, cursor *SnapshotRecordCursor, limit int) ([]snapshotDecisionMaterial, error) {
	if limit == 0 {
		return nil, nil
	}
	query := `
		SELECT r.owner_identity, r.batch_idempotency_key, r.ordinal, r.signal_id,
			r.open_loop_key, r.outcome, b.payload_digest, b.decision_count,
			b.recorded_at AS batch_recorded_at, b.payload::text AS batch_payload,
			r.recorded_at, r.payload::text AS payload
		FROM public.proactivity_decision_records r
		JOIN public.proactivity_decision_batches b
		  ON b.owner_identity = r.owner_identity AND b.idempotency_key = r.batch_idempotency_key
		WHERE r.owner_identity = ? AND r.recorded_at <= ?`
	args := []any{owner, at}
	if cursor != nil {
		query += ` AND (r.recorded_at, r.batch_idempotency_key, r.ordinal) <= (?, ?, ?)`
		args = append(args, cursor.RecordedAt, cursor.IdempotencyKey, cursor.Ordinal)
	}
	query += ` ORDER BY r.recorded_at DESC, r.batch_idempotency_key DESC, r.ordinal DESC LIMIT ?`
	args = append(args, limit)
	var rows []postgresSnapshotDecisionRow
	if err := db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("read bounded proactivity snapshot decisions: %w", err)
	}
	result := make([]snapshotDecisionMaterial, 0, len(rows))
	batches := make(map[string]DecisionBatch)
	for _, row := range rows {
		record, err := decodePostgresDecisionRow(postgresDecisionRow{
			OwnerIdentity: row.OwnerIdentity, BatchIdempotencyKey: row.BatchIdempotencyKey,
			Ordinal: row.Ordinal, SignalID: row.SignalID, OpenLoopKey: row.OpenLoopKey,
			Outcome: row.Outcome, RecordedAt: row.RecordedAt, Payload: row.Payload,
		}, owner)
		if err != nil {
			return nil, err
		}
		batch, found := batches[row.BatchIdempotencyKey]
		if !found {
			decoded, decodeErr := decodePostgresDecisionBatchRow(postgresDecisionBatchRow{
				OwnerIdentity: owner, IdempotencyKey: row.BatchIdempotencyKey,
				PayloadDigest: row.PayloadDigest, DecisionCount: row.DecisionCount,
				RecordedAt: row.BatchRecordedAt, Payload: row.BatchPayload,
			}, owner)
			if decodeErr != nil {
				return nil, decodeErr
			}
			batch = decoded
			batches[row.BatchIdempotencyKey] = batch
		}
		if row.Ordinal < 0 || row.Ordinal >= len(batch.Result.Decisions) {
			return nil, corruptPostgresRecord("snapshot decision batch ordinal", nil)
		}
		expected := DecisionRecord{
			ContractVersion: ContractVersion, OwnerIdentity: owner,
			Decision: cloneDecision(batch.Result.Decisions[row.Ordinal]), RecordedAt: batch.RecordedAt,
		}
		if !reflect.DeepEqual(expected, record) {
			return nil, corruptPostgresRecord("snapshot decision batch binding", nil)
		}
		result = append(result, snapshotDecisionMaterial{
			Cursor: SnapshotRecordCursor{RecordedAt: snapshotTime(row.RecordedAt), IdempotencyKey: row.BatchIdempotencyKey, Ordinal: row.Ordinal, PayloadDigest: row.PayloadDigest},
			Record: record,
		})
	}
	return result, nil
}

func queryPostgresSnapshotFeedback(db *gorm.DB, owner string, at time.Time, cursor *FeedbackSnapshotCursor, limit int) ([]snapshotFeedbackMaterial, error) {
	if limit == 0 {
		return nil, nil
	}
	query := `
		SELECT owner_identity, feedback_id, idempotency_key, request_digest,
			signal_id, open_loop_key, signal_digest, source_outcome, source_decision_at,
			action, snoozed_until, COALESCE(previous_record_digest, '') AS previous_record_digest,
			record_digest, recorded_at, authority, can_execute, delivery_authorized,
			execution_authorized, payload::text AS payload
		FROM public.proactivity_feedback_records
		WHERE owner_identity = ? AND recorded_at <= ?`
	args := []any{owner, at}
	if cursor != nil {
		query += ` AND (recorded_at, feedback_id) <= (?, CAST(? AS uuid))`
		args = append(args, cursor.RecordedAt, cursor.FeedbackID)
	}
	query += ` ORDER BY recorded_at DESC, feedback_id DESC LIMIT ?`
	args = append(args, limit)
	var rows []postgresFeedbackRow
	if err := db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("read bounded proactivity snapshot feedback: %w", err)
	}
	result := make([]snapshotFeedbackMaterial, 0, len(rows))
	for _, row := range rows {
		record, err := decodePostgresFeedbackRow(row, owner)
		if err != nil {
			return nil, err
		}
		result = append(result, snapshotFeedbackMaterial{
			Cursor: FeedbackSnapshotCursor{
				RecordedAt: snapshotTime(row.RecordedAt), FeedbackID: row.FeedbackID,
				IdempotencyKey: row.IdempotencyKey, PayloadDigest: row.RequestDigest,
				RecordDigest: row.RecordDigest,
			},
			Record: record,
		})
	}
	return result, nil
}
