package ambientmonitor

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"automation-hub-backend/internal/infra"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// ErrCorruptStorage means durable monitor state no longer satisfies the
// package contract. Callers must not continue from a partially decoded row.
var ErrCorruptStorage = errors.New("ambient monitor storage is corrupt")

// PostgresRepository persists the advisory monitor contract created by
// migration 0049. It deliberately exposes no migration or schema-repair path.
type PostgresRepository struct {
	DB *gorm.DB
}

var _ Repository = (*PostgresRepository)(nil)

func NewPostgresRepository(db *gorm.DB) *PostgresRepository {
	return &PostgresRepository{DB: db}
}

// DefaultRepository opens the configured, migrated database. Missing storage
// is a closed failure; ambient monitoring never falls back to process memory.
func DefaultRepository() (Repository, error) {
	db, err := infra.GetDefaultDB()
	if err != nil {
		return nil, fmt.Errorf("open ambient monitor database: %w", err)
	}
	return NewPostgresRepository(db), nil
}

func (r *PostgresRepository) CreateTarget(ctx context.Context, owner, workspace, key string, target MonitorTarget) (MonitorTarget, bool, error) {
	if err := r.ready(ctx); err != nil {
		return MonitorTarget{}, false, err
	}
	if err := validateRepositoryScope(owner, workspace); err != nil {
		return MonitorTarget{}, false, err
	}
	clean, err := validateTarget(target)
	if err != nil {
		return MonitorTarget{}, false, err
	}
	if clean.Scope != (Scope{OwnerID: owner, WorkspaceID: workspace}) {
		return MonitorTarget{}, false, ErrScopeViolation
	}
	digest := targetDigest(clean)
	if err := validateIdempotency(key, digest); err != nil {
		return MonitorTarget{}, false, err
	}
	targetUUID, err := postgresTargetUUID(clean.ID)
	if err != nil {
		return MonitorTarget{}, false, err
	}
	if clean.Lease.Active() || clean.Lease.Generation != 0 || !clean.CreatedAt.Equal(clean.UpdatedAt) {
		return MonitorTarget{}, false, fmt.Errorf("%w: a durable target must start unclaimed", ErrInvalidInput)
	}

	var stored MonitorTarget
	created := false
	err = r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := postgresCommandLock(tx, owner, workspace, "create_target", key); err != nil {
			return err
		}
		command, found, loadErr := loadPostgresCommand(tx, owner, workspace, "create_target", key)
		if loadErr != nil {
			return loadErr
		}
		if found {
			if command.RequestDigest != digest || command.TargetID != targetUUID.String() {
				return ErrIdempotencyConflict
			}
			stored, loadErr = decodePostgresCommandTarget(tx, command, owner, workspace)
			return loadErr
		}
		row, found, loadErr := loadPostgresTarget(tx, owner, workspace, targetUUID.String(), true)
		if loadErr != nil {
			return loadErr
		}
		if found {
			stored, loadErr = decodePostgresTarget(row, owner, workspace)
			if loadErr != nil {
				return loadErr
			}
			if !sameTargetIdentity(stored, clean) {
				return ErrIdempotencyConflict
			}
			return appendPostgresCommand(tx, owner, workspace, "create_target", key, digest, row.Revision, stored)
		}

		result := tx.Exec(`
			INSERT INTO public.outcome_monitor_targets (
				target_id, owner_identity, workspace_key, outcome_key, indicator_key,
				source_kind, cadence_seconds, next_run_at, enabled, revision,
				lease_id, lease_owner, lease_until, last_run_at, last_result, last_digest,
				created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, NULL, NULL, NULL, NULL, NULL, NULL, ?, ?)
			ON CONFLICT DO NOTHING`,
			targetUUID, owner, workspace, clean.OutcomeID, clean.IndicatorID,
			string(clean.SourceKind), int64(clean.Cadence/time.Second), clean.NextRunAt.UTC(), clean.Enabled,
			clean.CreatedAt.UTC(), clean.UpdatedAt.UTC(),
		)
		if result.Error != nil {
			return mapPostgresMonitorError("create target", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrIdempotencyConflict
		}
		clean.ID = targetUUID.String()
		stored = clean
		if err := appendPostgresCommand(tx, owner, workspace, "create_target", key, digest, 1, stored); err != nil {
			return err
		}
		created = true
		return nil
	})
	if err != nil {
		return MonitorTarget{}, false, err
	}
	return stored, created, nil
}

func (r *PostgresRepository) SetEnabled(ctx context.Context, owner, workspace, key, digest, targetID string, enabled bool, at time.Time) (MonitorTarget, bool, error) {
	if err := r.ready(ctx); err != nil {
		return MonitorTarget{}, false, err
	}
	if err := validateRepositoryScope(owner, workspace); err != nil {
		return MonitorTarget{}, false, err
	}
	if err := validateIdempotency(key, digest); err != nil {
		return MonitorTarget{}, false, err
	}
	targetUUID, err := postgresTargetUUID(targetID)
	if err != nil {
		return MonitorTarget{}, false, err
	}
	if at, err = validateTime("target update time", at); err != nil {
		return MonitorTarget{}, false, err
	}
	expectedDigest, digestErr := exactDigest("set_enabled", struct {
		Scope       Scope
		TargetID    string
		Enabled     bool
		RequestedAt time.Time
	}{Scope{OwnerID: owner, WorkspaceID: workspace}, targetID, enabled, at})
	if digestErr != nil || expectedDigest != digest {
		return MonitorTarget{}, false, ErrIdempotencyConflict
	}

	var stored MonitorTarget
	changed := false
	err = r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := postgresCommandLock(tx, owner, workspace, "set_enabled", key); err != nil {
			return err
		}
		command, found, loadErr := loadPostgresCommand(tx, owner, workspace, "set_enabled", key)
		if loadErr != nil {
			return loadErr
		}
		if found {
			if command.RequestDigest != digest || command.TargetID != targetUUID.String() {
				return ErrIdempotencyConflict
			}
			stored, loadErr = decodePostgresCommandTarget(tx, command, owner, workspace)
			return loadErr
		}
		row, found, loadErr := loadPostgresTarget(tx, owner, workspace, targetUUID.String(), true)
		if loadErr != nil {
			return loadErr
		}
		if !found {
			return ErrNotFound
		}
		stored, loadErr = decodePostgresTarget(row, owner, workspace)
		if loadErr != nil {
			return loadErr
		}
		if stored.Enabled == enabled {
			return appendPostgresCommand(tx, owner, workspace, "set_enabled", key, digest, row.Revision, stored)
		}
		if !at.After(row.UpdatedAt) {
			return fmt.Errorf("%w: target update time must advance", ErrInvalidInput)
		}
		result := tx.Exec(`
			UPDATE public.outcome_monitor_targets
			SET enabled = ?, revision = revision + 1,
				lease_id = CASE WHEN ? THEN lease_id ELSE NULL END,
				lease_owner = CASE WHEN ? THEN lease_owner ELSE NULL END,
				lease_until = CASE WHEN ? THEN lease_until ELSE NULL END,
				updated_at = ?
			WHERE owner_identity = ? AND workspace_key = ? AND target_id = ? AND revision = ?`,
			enabled, enabled, enabled, enabled, at.UTC(), owner, workspace, targetUUID, row.Revision,
		)
		if result.Error != nil {
			return mapPostgresMonitorError("set target enabled state", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrLeaseLost
		}
		updated, found, loadErr := loadPostgresTarget(tx, owner, workspace, targetUUID.String(), false)
		if loadErr != nil {
			return loadErr
		}
		if !found {
			return ErrCorruptStorage
		}
		stored, loadErr = decodePostgresTarget(updated, owner, workspace)
		if loadErr != nil {
			return loadErr
		}
		if err := appendPostgresCommand(tx, owner, workspace, "set_enabled", key, digest, updated.Revision, stored); err != nil {
			return err
		}
		changed = true
		return nil
	})
	if err != nil {
		return MonitorTarget{}, false, err
	}
	return stored, changed, nil
}

func (r *PostgresRepository) GetTarget(ctx context.Context, owner, workspace, targetID string) (MonitorTarget, error) {
	if err := r.ready(ctx); err != nil {
		return MonitorTarget{}, err
	}
	if err := validateRepositoryScope(owner, workspace); err != nil {
		return MonitorTarget{}, err
	}
	targetUUID, err := postgresTargetUUID(targetID)
	if err != nil {
		return MonitorTarget{}, err
	}
	row, found, err := loadPostgresTarget(r.DB.WithContext(ctx), owner, workspace, targetUUID.String(), false)
	if err != nil {
		return MonitorTarget{}, err
	}
	if !found {
		return MonitorTarget{}, ErrNotFound
	}
	return decodePostgresTarget(row, owner, workspace)
}

func (r *PostgresRepository) ListTargets(ctx context.Context, owner, workspace string) ([]MonitorTarget, error) {
	if err := r.ready(ctx); err != nil {
		return nil, err
	}
	if err := validateRepositoryScope(owner, workspace); err != nil {
		return nil, err
	}
	var rows []postgresTargetRow
	if err := r.DB.WithContext(ctx).Raw(postgresTargetSelect+`
		WHERE owner_identity = ? AND workspace_key = ?
		ORDER BY next_run_at ASC, target_id ASC`, owner, workspace).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list ambient monitor targets: %w", err)
	}
	result := make([]MonitorTarget, 0, len(rows))
	for _, row := range rows {
		target, err := decodePostgresTarget(row, owner, workspace)
		if err != nil {
			return nil, err
		}
		result = append(result, target)
	}
	return result, nil
}

func (r *PostgresRepository) ListDueScopes(ctx context.Context, now time.Time, limit int) ([]Scope, error) {
	if err := r.ready(ctx); err != nil {
		return nil, err
	}
	var err error
	if now, err = validateTime("due scope time", now); err != nil {
		return nil, err
	}
	if limit < 1 || limit > maxClaimLimit {
		return nil, fmt.Errorf("%w: due scope limit must be between 1 and %d", ErrInvalidInput, maxClaimLimit)
	}
	var rows []struct {
		OwnerID     string
		WorkspaceID string
	}
	if err := r.DB.WithContext(ctx).Raw(`
		SELECT owner_identity AS owner_id, workspace_key AS workspace_id
		FROM public.outcome_monitor_targets
		WHERE enabled = true AND next_run_at <= ?
			AND (lease_until IS NULL OR lease_until <= ?)
		GROUP BY owner_identity, workspace_key
		ORDER BY owner_identity ASC, workspace_key ASC
		LIMIT ?`, now.UTC(), now.UTC(), limit).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list due ambient monitor scopes: %w", err)
	}
	result := make([]Scope, 0, len(rows))
	for _, row := range rows {
		scope, err := validateScope(Scope{OwnerID: row.OwnerID, WorkspaceID: row.WorkspaceID})
		if err != nil {
			return nil, fmt.Errorf("%w: invalid stored due scope", ErrCorruptStorage)
		}
		result = append(result, scope)
	}
	return result, nil
}

func (r *PostgresRepository) FindCompletion(ctx context.Context, owner, workspace, key string) (Completion, bool, error) {
	if err := r.ready(ctx); err != nil {
		return Completion{}, false, err
	}
	if err := validateRepositoryScope(owner, workspace); err != nil {
		return Completion{}, false, err
	}
	if err := validateIdentifier("idempotency key", key); err != nil {
		return Completion{}, false, err
	}
	var row postgresRunRow
	if err := r.DB.WithContext(ctx).Raw(postgresRunSelect+`
		WHERE r.owner_identity = ? AND r.workspace_key = ? AND r.idempotency_key = ?
		ORDER BY r.completed_at DESC, r.run_id DESC LIMIT 1`,
		owner, workspace, key).Scan(&row).Error; err != nil {
		return Completion{}, false, fmt.Errorf("find ambient monitor completion: %w", err)
	}
	if row.RunID == "" || row.Status != "succeeded" {
		return Completion{}, false, nil
	}
	run, err := decodePostgresRun(row, owner, workspace, row.TargetID)
	if err != nil {
		return Completion{}, false, err
	}
	var observationRow postgresObservationRow
	if err := r.DB.WithContext(ctx).Raw(postgresObservationSelect+`
		WHERE owner_identity = ? AND workspace_key = ? AND source_key = ? AND idempotency_key = ?`,
		owner, workspace, row.TargetID, key).Scan(&observationRow).Error; err != nil {
		return Completion{}, false, fmt.Errorf("find ambient monitor completion observation: %w", err)
	}
	if observationRow.ObservationID == "" {
		return Completion{}, false, ErrCorruptStorage
	}
	observation, err := decodePostgresObservation(observationRow, owner, workspace, row.TargetID)
	if err != nil {
		return Completion{}, false, err
	}
	if run.ObservationID != observation.ID || run.ObservationDigest != observation.RecordDigest ||
		run.OutcomeID != observation.OutcomeID || run.IndicatorID != observation.IndicatorID || run.SourceKind != observation.SourceKind {
		return Completion{}, false, ErrCorruptStorage
	}
	return Completion{Observation: observation, Run: run, Created: false, Composed: false, Authority: advisoryAuthority()}, true, nil
}

func (r *PostgresRepository) ClaimDue(ctx context.Context, owner, workspace, worker string, now time.Time, leaseDuration time.Duration, limit int) ([]MonitorTarget, error) {
	if err := r.ready(ctx); err != nil {
		return nil, err
	}
	if err := validateRepositoryScope(owner, workspace); err != nil {
		return nil, err
	}
	if err := validatePostgresWorker(worker); err != nil {
		return nil, err
	}
	var err error
	if now, err = validateTime("claim time", now); err != nil {
		return nil, err
	}
	if err := validateLeaseDuration(leaseDuration); err != nil {
		return nil, err
	}
	if limit < 1 || limit > maxClaimLimit {
		return nil, fmt.Errorf("%w: claim limit must be between 1 and %d", ErrInvalidInput, maxClaimLimit)
	}

	claimed := make([]MonitorTarget, 0, limit)
	err = r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []postgresTargetRow
		query := postgresTargetSelect + `
			WHERE owner_identity = ? AND workspace_key = ? AND enabled = true
				AND next_run_at <= ? AND (lease_until IS NULL OR lease_until <= ?)
			ORDER BY next_run_at ASC, target_id ASC
			FOR UPDATE SKIP LOCKED LIMIT ?`
		if err := tx.Raw(query, owner, workspace, now.UTC(), now.UTC(), limit).Scan(&rows).Error; err != nil {
			return fmt.Errorf("lock due ambient monitor targets: %w", err)
		}
		for _, row := range rows {
			claimAt := now
			if !claimAt.After(row.UpdatedAt) {
				claimAt = row.UpdatedAt.Add(time.Microsecond)
			}
			generation := uint64(row.Revision + 1)
			claimID := postgresClaimID(owner, workspace, row.TargetID, worker, generation)
			result := tx.Exec(`
				UPDATE public.outcome_monitor_targets
				SET lease_id = ?, lease_owner = ?, lease_until = ?, revision = revision + 1, updated_at = ?
				WHERE owner_identity = ? AND workspace_key = ? AND target_id = ?
					AND revision = ? AND enabled = true
					AND (lease_until IS NULL OR lease_until <= ?)`,
				claimID, worker, claimAt.Add(leaseDuration), claimAt, owner, workspace, row.TargetID, row.Revision, now.UTC(),
			)
			if result.Error != nil {
				return mapPostgresMonitorError("claim ambient monitor target", result.Error)
			}
			if result.RowsAffected != 1 {
				continue
			}
			updated, found, loadErr := loadPostgresTarget(tx, owner, workspace, row.TargetID, false)
			if loadErr != nil || !found {
				if loadErr != nil {
					return loadErr
				}
				return ErrCorruptStorage
			}
			target, decodeErr := decodePostgresTarget(updated, owner, workspace)
			if decodeErr != nil {
				return decodeErr
			}
			target.Lease.WorkerID = worker
			claimed = append(claimed, target)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return claimed, nil
}

func (r *PostgresRepository) Complete(ctx context.Context, owner, workspace, key, digest, targetID, worker string, generation uint64, expectedSourceDigest string, observation ObservationRecord, run MonitorRun, snapshot CompositionSnapshot, next time.Time) (ObservationRecord, MonitorRun, bool, error) {
	if err := r.ready(ctx); err != nil {
		return ObservationRecord{}, MonitorRun{}, false, err
	}
	if err := validateCompletionInput(owner, workspace, key, digest, targetID, worker, generation, expectedSourceDigest, observation, run, next); err != nil {
		return ObservationRecord{}, MonitorRun{}, false, err
	}
	targetUUID, _ := postgresTargetUUID(targetID)
	observationUUID, err := postgresRecordUUID(observation.ID, "obs")
	if err != nil {
		return ObservationRecord{}, MonitorRun{}, false, err
	}
	runUUID, err := postgresRecordUUID(run.ID, "run")
	if err != nil {
		return ObservationRecord{}, MonitorRun{}, false, err
	}
	delivery, err := initialCompositionDelivery(observation, run, snapshot)
	if err != nil {
		return ObservationRecord{}, MonitorRun{}, false, err
	}
	deliveryUUID, err := postgresRecordUUID(delivery.ID, "cmp")
	if err != nil {
		return ObservationRecord{}, MonitorRun{}, false, err
	}
	snapshotColumns, err := postgresCompositionSnapshotColumns(delivery.Snapshot)
	if err != nil {
		return ObservationRecord{}, MonitorRun{}, false, err
	}
	var deliveryFailure, deliveryCompleted any
	if delivery.LastFailureCode != "" {
		deliveryFailure = delivery.LastFailureCode
	}
	if !delivery.CompletedAt.IsZero() {
		deliveryCompleted = delivery.CompletedAt.UTC()
	}

	storedObservation := ObservationRecord{}
	storedRun := MonitorRun{}
	created := false
	err = r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		replayedObservation, replayedRun, found, replayErr := loadCompletionReplay(tx, owner, workspace, targetUUID.String(), key, digest)
		if replayErr != nil {
			return replayErr
		}
		if found {
			storedObservation, storedRun = replayedObservation, replayedRun
			return nil
		}
		row, found, loadErr := loadPostgresTarget(tx, owner, workspace, targetUUID.String(), true)
		if loadErr != nil {
			return loadErr
		}
		if !found {
			return ErrNotFound
		}
		if err := verifyPostgresLease(row, owner, workspace, worker, generation, run.FinishedAt); err != nil {
			return err
		}
		if !run.StartedAt.Equal(row.UpdatedAt) {
			return fmt.Errorf("%w: completion start does not match claim", ErrLeaseLost)
		}
		if row.OutcomeID != observation.OutcomeID || row.IndicatorID != observation.IndicatorID || SourceKind(row.SourceKind) != observation.SourceKind {
			return ErrScopeViolation
		}
		result := tx.Exec(`
			INSERT INTO public.outcome_observation_records (
				observation_id, owner_identity, workspace_key, outcome_key, indicator_key,
				source_kind, source_key, source_digest, numeric_value, unit,
				idempotency_key, request_digest, record_digest, observed_at, recorded_at,
				authority, can_execute, delivery_authorized, execution_authorized
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'count', ?, ?, ?, ?, ?, ?, false, false, false)`,
			observationUUID, owner, workspace, observation.OutcomeID, observation.IndicatorID,
			string(observation.SourceKind), targetUUID.String(), observation.SourceDigest, observation.Value,
			key, digest, observation.RecordDigest, observation.ObservedAt.UTC(), observation.RecordedAt.UTC(), AuthorityLabel,
		)
		if result.Error != nil {
			return mapPostgresMonitorError("append ambient monitor observation", result.Error)
		}
		if delivery.Snapshot.Status == CompositionSnapshotPinned {
			if result = tx.Exec(`SELECT set_config('hai.outcome_monitor_pinned_enqueue', 'on', true)`); result.Error != nil {
				return mapPostgresMonitorError("prepare pinned ambient composition", result.Error)
			}
		}
		claimID := postgresClaimID(owner, workspace, targetUUID.String(), worker, generation)
		result = tx.Exec(`
			INSERT INTO public.outcome_monitor_runs (
				run_id, target_id, owner_identity, workspace_key, target_revision, claim_id, claimed_at,
				status, observation_count, signal_count, error_message_redacted,
				error_was_redacted, idempotency_key, request_digest, record_digest,
				started_at, completed_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, 'succeeded', 1, 0, NULL, false, ?, ?, ?, ?, ?)`,
			runUUID, targetUUID, owner, workspace, generation, claimID, row.UpdatedAt.UTC(),
			key, digest, run.RecordDigest, run.StartedAt.UTC(), run.FinishedAt.UTC(),
		)
		if result.Error != nil {
			return mapPostgresMonitorError("append ambient monitor run", result.Error)
		}
		if delivery.Snapshot.Status == CompositionSnapshotPinned {
			result = tx.Exec(`
			INSERT INTO public.outcome_monitor_composition_deliveries (
				delivery_id, owner_identity, workspace_key, target_id, run_id, run_digest,
				observation_id, observation_digest, status, revision, attempt_count,
				max_attempts, next_attempt_at, lease_generation, lease_id, lease_owner,
				lease_until, last_attempt_at, last_failure_code, created_at, updated_at,
				completed_at, binding_digest, snapshot_status, composer_version,
				snapshot_captured_at, outcome_revision, outcome_audit_digest,
				policy_idempotency_key, policy_payload_digest, policy_recorded_at,
				signal_watermark_at, signal_watermark_key,
				decision_watermark_at, decision_watermark_key,
				feedback_watermark_at, feedback_watermark_id, feedback_watermark_digest,
				attention_snapshot, attention_snapshot_digest, snapshot_digest,
				authority, can_execute,
				delivery_authorized, execution_authorized
			) VALUES (
				?, ?, ?, ?, ?, ?, ?, ?, ?,
				1, 0, ?, ?, 0, NULL, NULL, NULL, NULL,
				?, ?, ?, ?, ?,
				?, ?, ?, ?, ?,
				?, ?, ?,
				?, ?,
				?, ?,
				?, ?, ?,
				CAST(? AS jsonb), ?, ?,
				?, false, false, false
			)`,
				deliveryUUID, owner, workspace, targetUUID, runUUID, delivery.RunDigest,
				observationUUID, delivery.ObservationDigest, string(delivery.Status), delivery.MaxAttempts,
				delivery.NextAttemptAt.UTC(), deliveryFailure, delivery.CreatedAt.UTC(), delivery.UpdatedAt.UTC(), deliveryCompleted,
				delivery.BindingDigest, string(delivery.Snapshot.Status), delivery.Snapshot.ComposerVersion,
				delivery.Snapshot.CapturedAt.UTC(), snapshotColumns.OutcomeRevision, snapshotColumns.OutcomeAuditDigest,
				snapshotColumns.PolicyKey, snapshotColumns.PolicyDigest, snapshotColumns.PolicyAt,
				snapshotColumns.SignalAt, snapshotColumns.SignalKey,
				snapshotColumns.DecisionAt, snapshotColumns.DecisionKey,
				snapshotColumns.FeedbackAt, snapshotColumns.FeedbackID, snapshotColumns.FeedbackDigest,
				snapshotColumns.AttentionJSON, snapshotColumns.AttentionDigest, delivery.Snapshot.SnapshotDigest,
				AuthorityLabel,
			)
			if result.Error != nil {
				return mapPostgresMonitorError("enqueue ambient monitor composition", result.Error)
			}
		}
		result = tx.Exec(`
			UPDATE public.outcome_monitor_targets
			SET next_run_at = ?, revision = revision + 1, lease_id = NULL, lease_owner = NULL, lease_until = NULL,
				last_run_at = ?, last_result = 'succeeded', last_digest = ?, updated_at = ?
			WHERE owner_identity = ? AND workspace_key = ? AND target_id = ?
				AND revision = ? AND lease_id = ?`,
			next.UTC(), run.FinishedAt.UTC(), run.RecordDigest, run.FinishedAt.UTC(),
			owner, workspace, targetUUID, generation, claimID,
		)
		if result.Error != nil {
			return mapPostgresMonitorError("complete ambient monitor target", result.Error)
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("%w: completion projection was not advanced", ErrLeaseLost)
		}
		storedObservation, storedRun, created = observation, run, true
		return nil
	})
	if err != nil {
		return ObservationRecord{}, MonitorRun{}, false, err
	}
	return storedObservation, storedRun, created, nil
}

func (r *PostgresRepository) Fail(ctx context.Context, owner, workspace, key, digest, targetID, worker string, generation uint64, run MonitorRun, next time.Time) (MonitorRun, bool, error) {
	if err := r.ready(ctx); err != nil {
		return MonitorRun{}, false, err
	}
	if err := validateFailureInput(owner, workspace, key, digest, targetID, worker, generation, run, next); err != nil {
		return MonitorRun{}, false, err
	}
	targetUUID, _ := postgresTargetUUID(targetID)
	runUUID, err := postgresRecordUUID(run.ID, "run")
	if err != nil {
		return MonitorRun{}, false, err
	}
	stored := MonitorRun{}
	created := false
	err = r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		replay, found, replayErr := loadFailureReplay(tx, owner, workspace, targetUUID.String(), key, digest)
		if replayErr != nil {
			return replayErr
		}
		if found {
			stored = replay
			return nil
		}
		row, found, loadErr := loadPostgresTarget(tx, owner, workspace, targetUUID.String(), true)
		if loadErr != nil {
			return loadErr
		}
		if !found {
			return ErrNotFound
		}
		if err := verifyPostgresLease(row, owner, workspace, worker, generation, run.FinishedAt); err != nil {
			return err
		}
		if !run.StartedAt.Equal(row.UpdatedAt) {
			return fmt.Errorf("%w: failure start does not match claim", ErrLeaseLost)
		}
		if row.OutcomeID != run.OutcomeID || row.IndicatorID != run.IndicatorID || SourceKind(row.SourceKind) != run.SourceKind {
			return ErrScopeViolation
		}
		claimID := postgresClaimID(owner, workspace, targetUUID.String(), worker, generation)
		redacted := encodePostgresFailure(run.FailureCode, run.FailureSummary)
		result := tx.Exec(`
			INSERT INTO public.outcome_monitor_runs (
				run_id, target_id, owner_identity, workspace_key, target_revision, claim_id, claimed_at,
				status, observation_count, signal_count, error_message_redacted,
				error_was_redacted, idempotency_key, request_digest, record_digest,
				started_at, completed_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, 'failed', 0, 0, ?, true, ?, ?, ?, ?, ?)`,
			runUUID, targetUUID, owner, workspace, generation, claimID, row.UpdatedAt.UTC(), redacted,
			key, digest, run.RecordDigest, run.StartedAt.UTC(), run.FinishedAt.UTC(),
		)
		if result.Error != nil {
			return mapPostgresMonitorError("append failed ambient monitor run", result.Error)
		}
		result = tx.Exec(`
			UPDATE public.outcome_monitor_targets
			SET next_run_at = ?, revision = revision + 1, lease_id = NULL, lease_owner = NULL, lease_until = NULL,
				last_run_at = ?, last_result = 'failed', last_digest = ?, updated_at = ?
			WHERE owner_identity = ? AND workspace_key = ? AND target_id = ?
				AND revision = ? AND lease_id = ?`,
			next.UTC(), run.FinishedAt.UTC(), run.RecordDigest, run.FinishedAt.UTC(),
			owner, workspace, targetUUID, generation, claimID,
		)
		if result.Error != nil {
			return mapPostgresMonitorError("fail ambient monitor target", result.Error)
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("%w: failure projection was not advanced", ErrLeaseLost)
		}
		stored, created = run, true
		return nil
	})
	if err != nil {
		return MonitorRun{}, false, err
	}
	return stored, created, nil
}

func (r *PostgresRepository) RecoverExpiredLeases(ctx context.Context, owner, workspace string, now time.Time) (int, error) {
	if err := r.ready(ctx); err != nil {
		return 0, err
	}
	if err := validateRepositoryScope(owner, workspace); err != nil {
		return 0, err
	}
	var err error
	if now, err = validateTime("recovery time", now); err != nil {
		return 0, err
	}
	result := r.DB.WithContext(ctx).Exec(`
		UPDATE public.outcome_monitor_targets
		SET lease_id = NULL, lease_owner = NULL, lease_until = NULL, revision = revision + 1, updated_at = ?
		WHERE owner_identity = ? AND workspace_key = ?
			AND lease_id IS NOT NULL AND lease_until IS NOT NULL AND lease_until <= ?`,
		now.UTC(), owner, workspace, now.UTC(),
	)
	if result.Error != nil {
		return 0, mapPostgresMonitorError("recover expired ambient monitor leases", result.Error)
	}
	return int(result.RowsAffected), nil
}

func (r *PostgresRepository) ListObservations(ctx context.Context, owner, workspace, targetID string, limit int) ([]ObservationRecord, error) {
	return r.ListObservationsAt(ctx, owner, workspace, targetID, time.Date(2200, 12, 31, 23, 59, 59, 0, time.UTC), limit)
}

func (r *PostgresRepository) ListObservationsAt(ctx context.Context, owner, workspace, targetID string, cutoff time.Time, limit int) ([]ObservationRecord, error) {
	if err := r.ready(ctx); err != nil {
		return nil, err
	}
	if err := validateRepositoryScope(owner, workspace); err != nil {
		return nil, err
	}
	targetUUID, err := postgresTargetUUID(targetID)
	if err != nil {
		return nil, err
	}
	if cutoff, err = validateTime("observation history cutoff", cutoff); err != nil {
		return nil, err
	}
	var rows []postgresObservationRow
	if err := r.DB.WithContext(ctx).Raw(postgresObservationSelect+`
		WHERE owner_identity = ? AND workspace_key = ? AND source_key = ? AND recorded_at <= ?
		ORDER BY observed_at DESC, observation_id DESC LIMIT ?`,
		owner, workspace, targetUUID.String(), cutoff.UTC(), boundedHistoryLimit(limit),
	).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list ambient monitor observations: %w", err)
	}
	result := make([]ObservationRecord, 0, len(rows))
	for _, row := range rows {
		record, err := decodePostgresObservation(row, owner, workspace, targetUUID.String())
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, nil
}

func (r *PostgresRepository) ListRuns(ctx context.Context, owner, workspace, targetID string, limit int) ([]MonitorRun, error) {
	if err := r.ready(ctx); err != nil {
		return nil, err
	}
	if err := validateRepositoryScope(owner, workspace); err != nil {
		return nil, err
	}
	targetUUID, err := postgresTargetUUID(targetID)
	if err != nil {
		return nil, err
	}
	var rows []postgresRunRow
	if err := r.DB.WithContext(ctx).Raw(postgresRunSelect+`
		WHERE r.owner_identity = ? AND r.workspace_key = ? AND r.target_id = ?
		ORDER BY r.started_at DESC, r.run_id DESC LIMIT ?`,
		owner, workspace, targetUUID, boundedHistoryLimit(limit),
	).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list ambient monitor runs: %w", err)
	}
	result := make([]MonitorRun, 0, len(rows))
	for _, row := range rows {
		run, err := decodePostgresRun(row, owner, workspace, targetUUID.String())
		if err != nil {
			return nil, err
		}
		result = append(result, run)
	}
	return result, nil
}

const postgresTargetSelect = `
	SELECT target_id::text AS target_id, owner_identity, workspace_key,
		outcome_key AS outcome_id, indicator_key AS indicator_id, source_kind,
		cadence_seconds, next_run_at, enabled, revision,
		lease_id::text AS lease_id, lease_owner, lease_until, last_run_at, last_result,
		last_digest, created_at, updated_at
	FROM public.outcome_monitor_targets `

const postgresObservationSelect = `
	SELECT observation_id::text AS observation_id, owner_identity, workspace_key,
		outcome_key AS outcome_id, indicator_key AS indicator_id, source_kind,
		source_key, source_digest, numeric_value, unit, idempotency_key,
		request_digest, record_digest, observed_at, recorded_at, authority,
		can_execute, delivery_authorized, execution_authorized
	FROM public.outcome_observation_records `

const postgresRunSelect = `
	SELECT r.run_id::text AS run_id, r.target_id::text AS target_id,
		r.owner_identity, r.workspace_key, t.outcome_key AS outcome_id,
		t.indicator_key AS indicator_id, t.source_kind, r.target_revision,
		r.claim_id::text AS claim_id, r.claimed_at, r.status,
		r.error_message_redacted, r.error_was_redacted, r.idempotency_key,
		r.request_digest, r.record_digest, r.started_at, r.completed_at,
		o.observation_id::text AS observation_id, o.record_digest AS observation_digest
	FROM public.outcome_monitor_runs r
	JOIN public.outcome_monitor_targets t
		ON t.target_id = r.target_id AND t.owner_identity = r.owner_identity
		AND t.workspace_key = r.workspace_key
	LEFT JOIN public.outcome_observation_records o
		ON o.owner_identity = r.owner_identity AND o.workspace_key = r.workspace_key
		AND o.idempotency_key = r.idempotency_key `

type postgresTargetRow struct {
	TargetID, OwnerIdentity, WorkspaceKey, OutcomeID, IndicatorID, SourceKind string
	CadenceSeconds                                                            int64
	NextRunAt                                                                 time.Time
	Enabled                                                                   bool
	Revision                                                                  int64
	LeaseID, LeaseOwner, LastResult, LastDigest                               sql.NullString
	LeaseUntil, LastRunAt                                                     sql.NullTime
	CreatedAt, UpdatedAt                                                      time.Time
}

type postgresObservationRow struct {
	ObservationID, OwnerIdentity, WorkspaceKey, OutcomeID, IndicatorID string
	SourceKind, SourceKey, SourceDigest                                string
	NumericValue                                                       float64
	Unit, IdempotencyKey, RequestDigest, RecordDigest, Authority       string
	ObservedAt, RecordedAt                                             time.Time
	CanExecute, DeliveryAuthorized, ExecutionAuthorized                bool
}

type postgresRunRow struct {
	RunID, TargetID, OwnerIdentity, WorkspaceKey, OutcomeID, IndicatorID string
	SourceKind, ClaimID, Status, IdempotencyKey                          string
	RequestDigest, RecordDigest                                          string
	TargetRevision                                                       int64
	ClaimedAt, StartedAt, CompletedAt                                    time.Time
	ErrorMessageRedacted, ObservationID, ObservationDigest               sql.NullString
	ErrorWasRedacted                                                     bool
}

type postgresCommandRow struct {
	OwnerIdentity, WorkspaceKey, Operation, IdempotencyKey, RequestDigest string
	TargetID                                                              string
	ResultRevision, ResultLeaseGeneration                                 int64
	ResultLeaseID, ResultLeaseOwner                                       sql.NullString
	ResultLeaseUntil                                                      sql.NullTime
	ResultEnabled                                                         bool
	ResultNextRunAt, ResultUpdatedAt, RecordedAt                          time.Time
}

func loadPostgresCommand(db *gorm.DB, owner, workspace, operation, key string) (postgresCommandRow, bool, error) {
	var row postgresCommandRow
	if err := db.Raw(`
		SELECT owner_identity, workspace_key, operation, idempotency_key, request_digest,
			target_id::text AS target_id, result_revision, result_lease_generation,
			result_lease_id::text AS result_lease_id, result_lease_owner, result_lease_until,
			result_enabled, result_next_run_at,
			result_updated_at, recorded_at
		FROM public.outcome_monitor_commands
		WHERE owner_identity = ? AND workspace_key = ? AND operation = ? AND idempotency_key = ?`,
		owner, workspace, operation, key).Scan(&row).Error; err != nil {
		return postgresCommandRow{}, false, fmt.Errorf("load ambient monitor command: %w", err)
	}
	return row, row.TargetID != "", nil
}

func appendPostgresCommand(db *gorm.DB, owner, workspace, operation, key, digest string, revision int64, target MonitorTarget) error {
	targetUUID, err := postgresTargetUUID(target.ID)
	if err != nil {
		return err
	}
	var leaseID, leaseOwner, leaseUntil any
	if target.Lease.Active() {
		claimID := postgresClaimID(owner, workspace, target.ID, target.Lease.WorkerID, target.Lease.Generation)
		leaseID = claimID
		leaseOwner = target.Lease.WorkerID
		leaseUntil = target.Lease.ExpiresAt.UTC()
	}
	result := db.Exec(`
		INSERT INTO public.outcome_monitor_commands (
			owner_identity, workspace_key, operation, idempotency_key, request_digest,
			target_id, result_revision, result_lease_generation,
			result_lease_id, result_lease_owner, result_lease_until,
			result_enabled, result_next_run_at,
			result_updated_at, recorded_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		owner, workspace, operation, key, digest, targetUUID, revision,
		target.Lease.Generation, leaseID, leaseOwner, leaseUntil,
		target.Enabled, target.NextRunAt.UTC(),
		target.UpdatedAt.UTC(), target.UpdatedAt.UTC(),
	)
	if result.Error != nil {
		return mapPostgresMonitorError("append ambient monitor command", result.Error)
	}
	return nil
}

func decodePostgresCommandTarget(db *gorm.DB, command postgresCommandRow, owner, workspace string) (MonitorTarget, error) {
	if command.OwnerIdentity != owner || command.WorkspaceKey != workspace || command.ResultRevision < 1 || command.ResultLeaseGeneration < 0 {
		return MonitorTarget{}, ErrCorruptStorage
	}
	current, found, err := loadPostgresTarget(db, owner, workspace, command.TargetID, false)
	if err != nil {
		return MonitorTarget{}, err
	}
	if !found {
		return MonitorTarget{}, ErrCorruptStorage
	}
	current.Enabled = command.ResultEnabled
	current.Revision = command.ResultRevision
	current.NextRunAt = command.ResultNextRunAt
	current.LeaseID = command.ResultLeaseID
	current.LeaseOwner = command.ResultLeaseOwner
	current.LeaseUntil = command.ResultLeaseUntil
	current.LastRunAt = sql.NullTime{}
	current.LastResult = sql.NullString{}
	current.LastDigest = sql.NullString{}
	current.UpdatedAt = command.ResultUpdatedAt
	target, err := decodePostgresTarget(current, owner, workspace)
	if err != nil {
		return MonitorTarget{}, err
	}
	target.Lease.Generation = uint64(command.ResultLeaseGeneration)
	return target, nil
}

func loadPostgresTarget(db *gorm.DB, owner, workspace, targetID string, lock bool) (postgresTargetRow, bool, error) {
	query := postgresTargetSelect + `WHERE owner_identity = ? AND workspace_key = ? AND target_id = ?`
	if lock {
		query += ` FOR UPDATE`
	}
	var row postgresTargetRow
	if err := db.Raw(query, owner, workspace, targetID).Scan(&row).Error; err != nil {
		return postgresTargetRow{}, false, fmt.Errorf("load ambient monitor target: %w", err)
	}
	return row, row.TargetID != "", nil
}

func decodePostgresTarget(row postgresTargetRow, owner, workspace string) (MonitorTarget, error) {
	if row.OwnerIdentity != owner || row.WorkspaceKey != workspace || row.Revision < 1 || row.CadenceSeconds < 1 {
		return MonitorTarget{}, ErrCorruptStorage
	}
	target := MonitorTarget{
		ContractVersion: ContractVersion,
		ID:              row.TargetID,
		Scope:           Scope{OwnerID: row.OwnerIdentity, WorkspaceID: row.WorkspaceKey},
		OutcomeID:       row.OutcomeID, IndicatorID: row.IndicatorID, SourceKind: SourceKind(row.SourceKind),
		Enabled: row.Enabled, Cadence: time.Duration(row.CadenceSeconds) * time.Second,
		NextRunAt: row.NextRunAt.UTC(), CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC(),
		Authority: advisoryAuthority(),
	}
	if row.LeaseID.Valid || row.LeaseOwner.Valid || row.LeaseUntil.Valid {
		if !row.LeaseID.Valid || !row.LeaseOwner.Valid || !row.LeaseUntil.Valid {
			return MonitorTarget{}, ErrCorruptStorage
		}
		if err := validatePostgresWorker(row.LeaseOwner.String); err != nil {
			return MonitorTarget{}, fmt.Errorf("%w: invalid stored lease owner", ErrCorruptStorage)
		}
		target.Lease = Lease{WorkerID: row.LeaseOwner.String, Generation: uint64(row.Revision), ClaimedAt: row.UpdatedAt.UTC(), ExpiresAt: row.LeaseUntil.Time.UTC()}
	} else if row.LastRunAt.Valid && row.Revision > 1 {
		target.Lease.Generation = uint64(row.Revision - 1)
	}
	clean, err := validateTarget(target)
	if err != nil {
		return MonitorTarget{}, fmt.Errorf("%w: invalid stored target: %v", ErrCorruptStorage, err)
	}
	return clean, nil
}

func decodePostgresObservation(row postgresObservationRow, owner, workspace, targetID string) (ObservationRecord, error) {
	if row.OwnerIdentity != owner || row.WorkspaceKey != workspace || row.SourceKey != targetID || row.Unit != "count" ||
		row.Authority != AuthorityLabel || row.CanExecute || row.DeliveryAuthorized || row.ExecutionAuthorized {
		return ObservationRecord{}, ErrCorruptStorage
	}
	id, err := domainRecordID(row.ObservationID, "obs")
	if err != nil {
		return ObservationRecord{}, err
	}
	record := ObservationRecord{
		ContractVersion: ContractVersion, ID: id,
		Scope: Scope{OwnerID: owner, WorkspaceID: workspace}, TargetID: targetID,
		OutcomeID: row.OutcomeID, IndicatorID: row.IndicatorID, SourceKind: SourceKind(row.SourceKind),
		Value: row.NumericValue, ObservedAt: row.ObservedAt.UTC(), RecordedAt: row.RecordedAt.UTC(),
		SourceDigest: row.SourceDigest, RecordDigest: row.RecordDigest, Authority: advisoryAuthority(),
	}
	if err := validateObservationRecord(record, owner, workspace, targetID); err != nil {
		return ObservationRecord{}, fmt.Errorf("%w: invalid stored observation: %v", ErrCorruptStorage, err)
	}
	return record, nil
}

func decodePostgresRun(row postgresRunRow, owner, workspace, targetID string) (MonitorRun, error) {
	if row.OwnerIdentity != owner || row.WorkspaceKey != workspace || row.TargetID != targetID || row.TargetRevision < 1 {
		return MonitorRun{}, ErrCorruptStorage
	}
	id, err := domainRecordID(row.RunID, "run")
	if err != nil {
		return MonitorRun{}, err
	}
	run := MonitorRun{
		ContractVersion: ContractVersion, ID: id,
		Scope: Scope{OwnerID: owner, WorkspaceID: workspace}, TargetID: targetID,
		OutcomeID: row.OutcomeID, IndicatorID: row.IndicatorID, SourceKind: SourceKind(row.SourceKind),
		LeaseGeneration: uint64(row.TargetRevision), StartedAt: row.StartedAt.UTC(), FinishedAt: row.CompletedAt.UTC(),
		IdempotencyDigest: row.RequestDigest, RecordDigest: row.RecordDigest, Authority: advisoryAuthority(),
	}
	switch row.Status {
	case "succeeded":
		run.Status = RunCompleted
		if !row.ObservationID.Valid || !row.ObservationDigest.Valid {
			return MonitorRun{}, ErrCorruptStorage
		}
		run.ObservationID, err = domainRecordID(row.ObservationID.String, "obs")
		if err != nil {
			return MonitorRun{}, err
		}
		run.ObservationDigest = row.ObservationDigest.String
	case "failed":
		run.Status = RunFailed
		if !row.ErrorWasRedacted || !row.ErrorMessageRedacted.Valid {
			return MonitorRun{}, ErrCorruptStorage
		}
		run.FailureCode, run.FailureSummary = decodePostgresFailure(row.ErrorMessageRedacted.String)
	default:
		return MonitorRun{}, fmt.Errorf("%w: unsupported stored run status", ErrCorruptStorage)
	}
	if err := validateRunRecord(run, owner, workspace, targetID); err != nil {
		return MonitorRun{}, fmt.Errorf("%w: invalid stored run: %v", ErrCorruptStorage, err)
	}
	return run, nil
}

func loadCompletionReplay(tx *gorm.DB, owner, workspace, targetID, key, digest string) (ObservationRecord, MonitorRun, bool, error) {
	row, found, err := loadPostgresRunByKey(tx, owner, workspace, targetID, key)
	if err != nil || !found {
		return ObservationRecord{}, MonitorRun{}, false, err
	}
	if row.RequestDigest != digest || row.Status != "succeeded" {
		return ObservationRecord{}, MonitorRun{}, false, ErrIdempotencyConflict
	}
	run, err := decodePostgresRun(row, owner, workspace, targetID)
	if err != nil {
		return ObservationRecord{}, MonitorRun{}, false, err
	}
	var observationRow postgresObservationRow
	if err := tx.Raw(postgresObservationSelect+`
		WHERE owner_identity = ? AND workspace_key = ? AND source_key = ? AND idempotency_key = ?`,
		owner, workspace, targetID, key).Scan(&observationRow).Error; err != nil {
		return ObservationRecord{}, MonitorRun{}, false, fmt.Errorf("load replayed observation: %w", err)
	}
	if observationRow.ObservationID == "" {
		return ObservationRecord{}, MonitorRun{}, false, ErrCorruptStorage
	}
	observation, err := decodePostgresObservation(observationRow, owner, workspace, targetID)
	return observation, run, err == nil, err
}

func loadFailureReplay(tx *gorm.DB, owner, workspace, targetID, key, digest string) (MonitorRun, bool, error) {
	row, found, err := loadPostgresRunByKey(tx, owner, workspace, targetID, key)
	if err != nil || !found {
		return MonitorRun{}, false, err
	}
	if row.RequestDigest != digest || row.Status != "failed" {
		return MonitorRun{}, false, ErrIdempotencyConflict
	}
	run, err := decodePostgresRun(row, owner, workspace, targetID)
	return run, err == nil, err
}

func loadPostgresRunByKey(db *gorm.DB, owner, workspace, targetID, key string) (postgresRunRow, bool, error) {
	var row postgresRunRow
	if err := db.Raw(postgresRunSelect+`
		WHERE r.owner_identity = ? AND r.workspace_key = ? AND r.target_id = ? AND r.idempotency_key = ?`,
		owner, workspace, targetID, key).Scan(&row).Error; err != nil {
		return postgresRunRow{}, false, fmt.Errorf("load ambient monitor replay: %w", err)
	}
	return row, row.RunID != "", nil
}

func validateCompletionInput(owner, workspace, key, digest, targetID, worker string, generation uint64, expectedSourceDigest string, observation ObservationRecord, run MonitorRun, next time.Time) error {
	if err := validateRepositoryScope(owner, workspace); err != nil {
		return err
	}
	if err := validateIdempotency(key, digest); err != nil {
		return err
	}
	if _, err := postgresTargetUUID(targetID); err != nil {
		return err
	}
	if err := validatePostgresWorker(worker); err != nil {
		return err
	}
	if generation == 0 {
		return fmt.Errorf("%w: lease generation is required", ErrInvalidInput)
	}
	if _, err := validateDigest("expected source digest", expectedSourceDigest); err != nil {
		return err
	}
	if err := validateObservationRecord(observation, owner, workspace, targetID); err != nil {
		return err
	}
	collected, err := validateCollected(CollectedObservation{
		Value: observation.Value, ObservedAt: observation.ObservedAt, SourceDigest: observation.SourceDigest,
	}, observation.RecordedAt)
	if err != nil || collected.Value != observation.Value || !collected.ObservedAt.Equal(observation.ObservedAt) || collected.SourceDigest != observation.SourceDigest {
		return fmt.Errorf("%w: observation value or source snapshot is invalid", ErrInvalidInput)
	}
	if err := validateRunRecord(run, owner, workspace, targetID); err != nil {
		return err
	}
	if observation.SourceDigest != expectedSourceDigest || run.Status != RunCompleted || run.ObservationID != observation.ID || run.ObservationDigest != observation.RecordDigest || run.IdempotencyDigest != digest {
		return fmt.Errorf("%w: completion record binding is invalid", ErrInvalidInput)
	}
	if _, err := validateTime("next run time", next); err != nil {
		return err
	}
	if !next.After(run.FinishedAt) {
		return fmt.Errorf("%w: next run time must follow completion", ErrInvalidInput)
	}
	return nil
}

func validateFailureInput(owner, workspace, key, digest, targetID, worker string, generation uint64, run MonitorRun, next time.Time) error {
	if err := validateRepositoryScope(owner, workspace); err != nil {
		return err
	}
	if err := validateIdempotency(key, digest); err != nil {
		return err
	}
	if _, err := postgresTargetUUID(targetID); err != nil {
		return err
	}
	if err := validatePostgresWorker(worker); err != nil {
		return err
	}
	if generation == 0 {
		return fmt.Errorf("%w: lease generation is required", ErrInvalidInput)
	}
	if err := validateRunRecord(run, owner, workspace, targetID); err != nil {
		return err
	}
	if run.Status != RunFailed || run.IdempotencyDigest != digest {
		return fmt.Errorf("%w: failure record binding is invalid", ErrInvalidInput)
	}
	if _, err := validateTime("next run time", next); err != nil {
		return err
	}
	if !next.After(run.FinishedAt) {
		return fmt.Errorf("%w: next run time must follow failure", ErrInvalidInput)
	}
	return nil
}

func verifyPostgresLease(row postgresTargetRow, owner, workspace, worker string, generation uint64, finishedAt time.Time) error {
	if !row.Enabled {
		return fmt.Errorf("%w: target is disabled", ErrLeaseLost)
	}
	if row.Revision != int64(generation) {
		return fmt.Errorf("%w: claim generation changed", ErrLeaseLost)
	}
	if !row.LeaseID.Valid || !row.LeaseOwner.Valid || !row.LeaseUntil.Valid {
		return fmt.Errorf("%w: claim is no longer active", ErrLeaseLost)
	}
	if !row.LeaseUntil.Time.After(finishedAt) {
		return fmt.Errorf("%w: claim expired", ErrLeaseLost)
	}
	expected := postgresClaimID(owner, workspace, row.TargetID, worker, generation)
	if row.LeaseOwner.String != worker || row.LeaseID.String != expected.String() {
		return fmt.Errorf("%w: claim belongs to another worker", ErrLeaseLost)
	}
	return nil
}

func sameTargetIdentity(stored, requested MonitorTarget) bool {
	return stored.Scope == requested.Scope && stored.OutcomeID == requested.OutcomeID &&
		stored.IndicatorID == requested.IndicatorID && stored.SourceKind == requested.SourceKind &&
		stored.Cadence == requested.Cadence && stored.CreatedAt.Equal(requested.CreatedAt)
}

func postgresTargetUUID(value string) (uuid.UUID, error) {
	trimmed := strings.TrimSpace(value)
	parsed, err := uuid.Parse(trimmed)
	if err != nil || parsed == uuid.Nil || value != trimmed || parsed.String() != value {
		return uuid.Nil, fmt.Errorf("%w: durable target id must be a canonical UUID", ErrInvalidInput)
	}
	return parsed, nil
}

func postgresRecordUUID(value, prefix string) (uuid.UUID, error) {
	trimmed := strings.TrimSpace(value)
	if parsed, err := uuid.Parse(trimmed); err == nil && parsed != uuid.Nil {
		return parsed, nil
	}
	hexValue := strings.TrimPrefix(trimmed, prefix+"-")
	if len(hexValue) != 32 {
		return uuid.Nil, fmt.Errorf("%w: %s record id is not persistable", ErrInvalidInput, prefix)
	}
	parsed, err := uuid.Parse(hexValue)
	if err != nil || parsed == uuid.Nil {
		return uuid.Nil, fmt.Errorf("%w: %s record id is not persistable", ErrInvalidInput, prefix)
	}
	return parsed, nil
}

func domainRecordID(value, prefix string) (string, error) {
	parsed, err := uuid.Parse(value)
	if err != nil || parsed == uuid.Nil {
		return "", fmt.Errorf("%w: invalid stored %s id", ErrCorruptStorage, prefix)
	}
	return prefix + "-" + strings.ReplaceAll(parsed.String(), "-", ""), nil
}

func postgresClaimID(owner, workspace, targetID, worker string, generation uint64) uuid.UUID {
	value := fmt.Sprintf("%s|%s|%s|%s|%d", owner, workspace, targetID, worker, generation)
	digest := sha256.Sum256([]byte(value))
	var claim uuid.UUID
	copy(claim[:], digest[:16])
	claim[6] = (claim[6] & 0x0f) | 0x50
	claim[8] = (claim[8] & 0x3f) | 0x80
	return claim
}

func validatePostgresWorker(worker string) error {
	return validateIdentifier("worker id", worker)
}

func postgresCommandLock(tx *gorm.DB, owner, workspace, operation, key string) error {
	// The identifier grammar excludes '|', making this composition unambiguous,
	// while unlike the in-memory NUL delimiter it remains valid PostgreSQL text.
	if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`, owner+"|"+workspace+"|"+operation+"|"+key).Error; err != nil {
		return fmt.Errorf("lock ambient monitor command: %w", err)
	}
	return nil
}

func encodePostgresFailure(code, summary string) string {
	return strings.TrimSpace(code) + ": " + strings.TrimSpace(summary)
}

func decodePostgresFailure(value string) (string, string) {
	code, summary, found := strings.Cut(value, ": ")
	if !found {
		return "stored_failure", strings.TrimSpace(value)
	}
	return strings.TrimSpace(code), strings.TrimSpace(summary)
}

func mapPostgresMonitorError(operation string, err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrIdempotencyConflict
		case "23503", "55P03", "40001":
			return fmt.Errorf("%w: %s", ErrLeaseLost, operation)
		case "23514", "23000":
			return fmt.Errorf("%w: postgres rejected monitor state", ErrInvalidInput)
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func (r *PostgresRepository) ready(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("%w: context is required", ErrInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if r == nil || r.DB == nil {
		return ErrRepositoryUnavailable
	}
	return nil
}
