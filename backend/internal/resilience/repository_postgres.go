package resilience

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"automation-hub-backend/internal/infra"

	"gorm.io/gorm"
)

// Migration 0023 schema contract
//
// Migration 0023 creates these owner/workspace-scoped tables in the
// public schema. Every payload column is jsonb, NOT NULL, constrained with
// jsonb_typeof(payload) = 'object', and should be bounded to 1 MiB with
// octet_length(payload::text) <= 1048576. All timestamps are timestamptz and
// all counters are NUMERIC(20,0) so the complete uint64 fencing domain is
// representable. Text identifiers are NOT NULL and bounded to 200 characters.
//
//	resilience_idempotency_records
//	  owner_id, workspace_id, idempotency_key, work_id, payload_hash,
//	  contract_version, recorded_at, payload
//	  PK (owner_id, workspace_id, idempotency_key)
//
//	resilience_leases
//	  owner_id, workspace_id, work_id, idempotency_key, payload_hash,
//	  worker_id, generation, lease_state, acquired_at, last_heartbeat_at,
//	  expires_at, released_at, contract_version, payload
//	  PK (owner_id, workspace_id, work_id); generation >= 1
//
//	resilience_worker_heartbeats
//	  owner_id, workspace_id, worker_id, sequence, observed_at,
//	  contract_version, payload
//	  PK (owner_id, workspace_id, worker_id); sequence >= 1
//
//	resilience_circuits
//	  owner_id, workspace_id, circuit_id, revision, circuit_phase,
//	  updated_at, contract_version, payload
//	  PK (owner_id, workspace_id, circuit_id); revision >= 1
//
//	resilience_retry_records / resilience_recovery_records
//	  owner_id, workspace_id, work_id, sequence, requested_at,
//	  contract_version, payload
//	  PK (owner_id, workspace_id, work_id, sequence); sequence >= 1
//
//	resilience_event_records
//	  owner_id, workspace_id, sequence, event_hash, previous_hash, event_type,
//	  subject_id, occurred_at, contract_version, payload
//	  PK (owner_id, workspace_id, sequence)
//	  UNIQUE (owner_id, workspace_id, event_hash)
//
// Required bounded-list indexes are:
//
//	resilience_leases_scope_work_idx (owner_id, workspace_id, work_id)
//	resilience_worker_heartbeats_scope_worker_idx (owner_id, workspace_id, worker_id)
//	resilience_circuits_scope_circuit_idx (owner_id, workspace_id, circuit_id)
//	resilience_retries_scope_work_sequence_idx (owner_id, workspace_id, work_id, sequence DESC)
//	resilience_retries_scope_requested_idx (owner_id, workspace_id, requested_at DESC, work_id)
//	resilience_recoveries_scope_work_sequence_idx (owner_id, workspace_id, work_id, sequence DESC)
//	resilience_recoveries_scope_requested_idx (owner_id, workspace_id, requested_at DESC, work_id)
//	resilience_events_scope_sequence_idx (owner_id, workspace_id, sequence DESC)
//
// The migration must deny UPDATE/DELETE/TRUNCATE on retry, recovery, and event
// tables with triggers, constrain contract_version = 1, and must not add any
// dispatch, execution, approval-consumption, or authority-granting columns.
const postgresSchemaContract = `
migration=0023
tables=resilience_idempotency_records,resilience_leases,resilience_worker_heartbeats,resilience_circuits,resilience_retry_records,resilience_recovery_records,resilience_event_records
scope=owner_id+workspace_id
primary-keys=idempotency(owner_id,workspace_id,idempotency_key);lease(owner_id,workspace_id,work_id);heartbeat(owner_id,workspace_id,worker_id);circuit(owner_id,workspace_id,circuit_id);retry(owner_id,workspace_id,work_id,sequence);recovery(owner_id,workspace_id,work_id,sequence);event(owner_id,workspace_id,sequence)
indexes=resilience_leases_scope_work_idx,resilience_worker_heartbeats_scope_worker_idx,resilience_circuits_scope_circuit_idx,resilience_retries_scope_work_sequence_idx,resilience_retries_scope_requested_idx,resilience_recoveries_scope_work_sequence_idx,resilience_recoveries_scope_requested_idx,resilience_events_scope_sequence_idx
constraints=contract_version=1;identifiers<=200;hashes=lowercase-sha256;payload=jsonb-object<=1MiB;counters=numeric(20,0);event-hash-unique-per-scope
append-only=retry+recovery+event
authority=advisory-only
`

// PostgresRepository is the durable, cross-process implementation of
// Repository. It persists only advisory control-plane state and never runs or
// dispatches work.
type PostgresRepository struct {
	DB *gorm.DB
}

func NewPostgresRepository(db *gorm.DB) *PostgresRepository {
	return &PostgresRepository{DB: db}
}

// DefaultRepository opens the configured database and applies the application's
// versioned migration chain. It deliberately has no in-memory fallback.
func DefaultRepository() (Repository, error) {
	db, err := infra.GetDefaultDB()
	if err != nil {
		return nil, err
	}
	return NewPostgresRepository(db), nil
}

type postgresIdempotencyRow struct {
	OwnerID         string
	WorkspaceID     string
	IdempotencyKey  string
	WorkID          string
	PayloadHash     string
	ContractVersion int
	RecordedAt      time.Time
	Payload         string
}

type postgresLeaseRow struct {
	OwnerID         string
	WorkspaceID     string
	WorkID          string
	IdempotencyKey  string
	PayloadHash     string
	WorkerID        string
	Generation      uint64
	LeaseState      string
	AcquiredAt      time.Time
	LastHeartbeatAt time.Time
	ExpiresAt       time.Time
	ReleasedAt      *time.Time
	ContractVersion int
	Payload         string
}

type postgresHeartbeatRow struct {
	OwnerID         string
	WorkspaceID     string
	WorkerID        string
	Sequence        uint64
	ObservedAt      time.Time
	ContractVersion int
	Payload         string
}

type postgresCircuitRow struct {
	OwnerID         string
	WorkspaceID     string
	CircuitID       string
	Revision        uint64
	CircuitPhase    string
	ContractVersion int
	Payload         string
}

type postgresHistoryRow struct {
	OwnerID         string
	WorkspaceID     string
	WorkID          string
	Sequence        uint64
	RequestedAt     time.Time
	ContractVersion int
	Payload         string
}

type postgresEventRow struct {
	OwnerID         string
	WorkspaceID     string
	Sequence        uint64
	EventHash       string
	PreviousHash    string
	EventType       string
	SubjectID       string
	OccurredAt      time.Time
	ContractVersion int
	Payload         string
}

func (r *PostgresRepository) ready(ctx context.Context, scope Scope) error {
	if r == nil || r.DB == nil {
		return fmt.Errorf("resilience: postgres repository is unavailable")
	}
	return repositoryInput(ctx, scope)
}

func (r *PostgresRepository) transaction(ctx context.Context, fn func(*gorm.DB) error) error {
	if r == nil || r.DB == nil {
		return fmt.Errorf("resilience: postgres repository is unavailable")
	}
	if ctx == nil {
		return fmt.Errorf("resilience: context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	err := r.DB.WithContext(ctx).Transaction(fn, &sql.TxOptions{Isolation: sql.LevelSerializable})
	return translatePostgresWriteError(err)
}

func (r *PostgresRepository) LookupIdempotency(ctx context.Context, scope Scope, key string) (*IdempotencyRecord, error) {
	if err := r.ready(ctx, scope); err != nil {
		return nil, err
	}
	if err := validateHash("idempotency key", key, false); err != nil {
		return nil, err
	}
	return loadPostgresIdempotency(r.DB.WithContext(ctx), scope, key)
}

func (r *PostgresRepository) CreateIdempotency(ctx context.Context, record IdempotencyRecord) (*IdempotencyRecord, bool, error) {
	if err := r.ready(ctx, record.Scope); err != nil {
		return nil, false, err
	}
	if err := validateIdempotencyRecord(record); err != nil {
		return nil, false, err
	}
	payload, err := marshalPostgresRecord("idempotency record", record)
	if err != nil {
		return nil, false, err
	}
	result := r.DB.WithContext(ctx).Exec(`INSERT INTO public.resilience_idempotency_records
			(owner_id, workspace_id, idempotency_key, work_id, payload_hash, contract_version, recorded_at, payload)
			VALUES (?, ?, ?, ?, ?, ?, ?, CAST(? AS jsonb))
			ON CONFLICT (owner_id, workspace_id, idempotency_key) DO NOTHING`,
		record.Scope.OwnerID, record.Scope.WorkspaceID, record.IdempotencyKey,
		record.WorkID, record.PayloadHash, record.ContractVersion, record.RecordedAt.UTC(), string(payload))
	if result.Error != nil {
		return nil, false, result.Error
	}
	stored, err := loadPostgresIdempotency(r.DB.WithContext(ctx), record.Scope, record.IdempotencyKey)
	if err != nil {
		return nil, false, err
	}
	if stored.PayloadHash != record.PayloadHash {
		return nil, false, ErrStateConflict
	}
	return cloneIdempotencyRecord(stored), result.RowsAffected == 1, nil
}

func loadPostgresIdempotency(db *gorm.DB, scope Scope, key string) (*IdempotencyRecord, error) {
	var rows []postgresIdempotencyRow
	err := db.Raw(`SELECT owner_id, workspace_id, idempotency_key, work_id, payload_hash,
		contract_version, recorded_at, payload::text AS payload
		FROM public.resilience_idempotency_records
		WHERE owner_id = ? AND workspace_id = ? AND idempotency_key = ? LIMIT 2`,
		scope.OwnerID, scope.WorkspaceID, key).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("read resilience idempotency record: %w", err)
	}
	if len(rows) == 0 {
		return nil, ErrStateNotFound
	}
	if len(rows) != 1 {
		return nil, fmt.Errorf("%w: duplicate idempotency rows", ErrStateConflict)
	}
	record, err := decodePostgresIdempotency(rows[0], scope)
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *PostgresRepository) GetLease(ctx context.Context, scope Scope, workID string) (*WorkLease, error) {
	if err := repositoryScopedID(ctx, scope, "work id", workID); err != nil {
		return nil, err
	}
	if err := r.ready(ctx, scope); err != nil {
		return nil, err
	}
	return loadPostgresLease(r.DB.WithContext(ctx), scope, workID, false)
}

func (r *PostgresRepository) ListLeases(ctx context.Context, scope Scope, limit int) ([]WorkLease, error) {
	if err := r.ready(ctx, scope); err != nil {
		return nil, err
	}
	limit, err := boundedListLimit(limit, MaxHistoryLimit)
	if err != nil {
		return nil, err
	}
	var rows []postgresLeaseRow
	if err := r.DB.WithContext(ctx).Raw(`SELECT owner_id, workspace_id, work_id, idempotency_key,
		payload_hash, worker_id, generation, lease_state, acquired_at, last_heartbeat_at,
		expires_at, released_at, contract_version, payload::text AS payload
		FROM public.resilience_leases WHERE owner_id = ? AND workspace_id = ?
		ORDER BY work_id ASC LIMIT ?`, scope.OwnerID, scope.WorkspaceID, limit).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list resilience leases: %w", err)
	}
	result := make([]WorkLease, 0, len(rows))
	for _, row := range rows {
		lease, decodeErr := decodePostgresLease(row, scope)
		if decodeErr != nil {
			return nil, decodeErr
		}
		result = append(result, lease)
	}
	return result, nil
}

func (r *PostgresRepository) CompareAndSwapLease(ctx context.Context, scope Scope, workID string, expected *WorkLease, next WorkLease) error {
	if err := repositoryScopedID(ctx, scope, "work id", workID); err != nil {
		return err
	}
	if err := r.ready(ctx, scope); err != nil {
		return err
	}
	if err := validateLease(next); err != nil {
		return err
	}
	if next.Scope != scope || next.WorkID != workID {
		return ErrStateConflict
	}
	if expected != nil {
		if err := validateLease(*expected); err != nil {
			return err
		}
		if expected.Scope != scope || expected.WorkID != workID {
			return ErrStateConflict
		}
	}
	payload, err := marshalPostgresRecord("lease", next)
	if err != nil {
		return err
	}
	return r.transaction(ctx, func(tx *gorm.DB) error {
		current, loadErr := loadPostgresLease(tx, scope, workID, true)
		if errors.Is(loadErr, ErrStateNotFound) {
			if expected != nil || next.Generation != 1 {
				return ErrStaleFence
			}
			result := tx.Exec(`INSERT INTO public.resilience_leases
				(owner_id, workspace_id, work_id, idempotency_key, payload_hash, worker_id,
				 generation, lease_state, acquired_at, last_heartbeat_at, expires_at,
				 released_at, contract_version, payload)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CAST(? AS jsonb))
				ON CONFLICT (owner_id, workspace_id, work_id) DO NOTHING`,
				scope.OwnerID, scope.WorkspaceID, next.WorkID, next.IdempotencyKey,
				next.PayloadHash, next.WorkerID, next.Generation, next.State,
				next.AcquiredAt.UTC(), next.LastHeartbeatAt.UTC(), next.ExpiresAt.UTC(),
				utcTimePointer(next.ReleasedAt), next.ContractVersion, string(payload))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrStaleFence
			}
			return nil
		}
		if loadErr != nil {
			return loadErr
		}
		if expected == nil || !sameLease(*current, *expected) {
			return ErrStaleFence
		}
		if err := validLeaseTransition(*current, next); err != nil {
			return err
		}
		result := tx.Exec(`UPDATE public.resilience_leases SET
			worker_id = ?, generation = ?, lease_state = ?, acquired_at = ?,
			last_heartbeat_at = ?, expires_at = ?, released_at = ?, contract_version = ?,
			payload = CAST(? AS jsonb)
			WHERE owner_id = ? AND workspace_id = ? AND work_id = ? AND generation = ?`,
			next.WorkerID, next.Generation, next.State, next.AcquiredAt.UTC(),
			next.LastHeartbeatAt.UTC(), next.ExpiresAt.UTC(), utcTimePointer(next.ReleasedAt),
			next.ContractVersion, string(payload), scope.OwnerID, scope.WorkspaceID, workID,
			current.Generation)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrStaleFence
		}
		return nil
	})
}

func loadPostgresLease(db *gorm.DB, scope Scope, workID string, lock bool) (*WorkLease, error) {
	query := `SELECT owner_id, workspace_id, work_id, idempotency_key, payload_hash,
		worker_id, generation, lease_state, acquired_at, last_heartbeat_at, expires_at,
		released_at, contract_version, payload::text AS payload
		FROM public.resilience_leases
		WHERE owner_id = ? AND workspace_id = ? AND work_id = ?`
	if lock {
		query += ` FOR UPDATE`
	}
	var rows []postgresLeaseRow
	if err := db.Raw(query, scope.OwnerID, scope.WorkspaceID, workID).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("read resilience lease: %w", err)
	}
	if len(rows) == 0 {
		return nil, ErrStateNotFound
	}
	if len(rows) != 1 {
		return nil, fmt.Errorf("%w: duplicate lease rows", ErrStateConflict)
	}
	lease, err := decodePostgresLease(rows[0], scope)
	if err != nil {
		return nil, err
	}
	return &lease, nil
}

func (r *PostgresRepository) GetWorkerHeartbeat(ctx context.Context, scope Scope, workerID string) (*WorkerHeartbeat, error) {
	if err := repositoryScopedID(ctx, scope, "worker id", workerID); err != nil {
		return nil, err
	}
	if err := r.ready(ctx, scope); err != nil {
		return nil, err
	}
	return loadPostgresHeartbeat(r.DB.WithContext(ctx), scope, workerID, false)
}

func (r *PostgresRepository) ListWorkerHeartbeats(ctx context.Context, scope Scope, limit int) ([]WorkerHeartbeat, error) {
	if err := r.ready(ctx, scope); err != nil {
		return nil, err
	}
	limit, err := boundedListLimit(limit, MaxHistoryLimit)
	if err != nil {
		return nil, err
	}
	var rows []postgresHeartbeatRow
	if err := r.DB.WithContext(ctx).Raw(`SELECT owner_id, workspace_id, worker_id, sequence,
		observed_at, contract_version, payload::text AS payload
		FROM public.resilience_worker_heartbeats
		WHERE owner_id = ? AND workspace_id = ? ORDER BY worker_id ASC LIMIT ?`,
		scope.OwnerID, scope.WorkspaceID, limit).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list resilience worker heartbeats: %w", err)
	}
	result := make([]WorkerHeartbeat, 0, len(rows))
	for _, row := range rows {
		heartbeat, decodeErr := decodePostgresHeartbeat(row, scope)
		if decodeErr != nil {
			return nil, decodeErr
		}
		result = append(result, heartbeat)
	}
	return result, nil
}

func (r *PostgresRepository) CompareAndSwapWorkerHeartbeat(ctx context.Context, scope Scope, workerID string, expected *WorkerHeartbeat, next WorkerHeartbeat) error {
	if err := repositoryScopedID(ctx, scope, "worker id", workerID); err != nil {
		return err
	}
	if err := r.ready(ctx, scope); err != nil {
		return err
	}
	if err := validateWorkerHeartbeat(next); err != nil {
		return err
	}
	if next.Scope != scope || next.WorkerID != workerID {
		return ErrStateConflict
	}
	if expected != nil {
		if err := validateWorkerHeartbeat(*expected); err != nil {
			return err
		}
		if expected.Scope != scope || expected.WorkerID != workerID {
			return ErrStateConflict
		}
	}
	payload, err := marshalPostgresRecord("worker heartbeat", next)
	if err != nil {
		return err
	}
	return r.transaction(ctx, func(tx *gorm.DB) error {
		current, loadErr := loadPostgresHeartbeat(tx, scope, workerID, true)
		if errors.Is(loadErr, ErrStateNotFound) {
			if expected != nil {
				return ErrStaleFence
			}
			result := tx.Exec(`INSERT INTO public.resilience_worker_heartbeats
				(owner_id, workspace_id, worker_id, sequence, observed_at, contract_version, payload)
				VALUES (?, ?, ?, ?, ?, ?, CAST(? AS jsonb))
				ON CONFLICT (owner_id, workspace_id, worker_id) DO NOTHING`,
				scope.OwnerID, scope.WorkspaceID, workerID, next.Sequence,
				next.ObservedAt.UTC(), next.ContractVersion, string(payload))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrStaleFence
			}
			return nil
		}
		if loadErr != nil {
			return loadErr
		}
		if expected == nil || *current != *expected || next.Sequence <= current.Sequence || !next.ObservedAt.After(current.ObservedAt) {
			return ErrStaleFence
		}
		result := tx.Exec(`UPDATE public.resilience_worker_heartbeats
			SET sequence = ?, observed_at = ?, contract_version = ?, payload = CAST(? AS jsonb)
			WHERE owner_id = ? AND workspace_id = ? AND worker_id = ? AND sequence = ?`,
			next.Sequence, next.ObservedAt.UTC(), next.ContractVersion, string(payload),
			scope.OwnerID, scope.WorkspaceID, workerID, current.Sequence)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrStaleFence
		}
		return nil
	})
}

func loadPostgresHeartbeat(db *gorm.DB, scope Scope, workerID string, lock bool) (*WorkerHeartbeat, error) {
	query := `SELECT owner_id, workspace_id, worker_id, sequence, observed_at,
		contract_version, payload::text AS payload
		FROM public.resilience_worker_heartbeats
		WHERE owner_id = ? AND workspace_id = ? AND worker_id = ?`
	if lock {
		query += ` FOR UPDATE`
	}
	var rows []postgresHeartbeatRow
	if err := db.Raw(query, scope.OwnerID, scope.WorkspaceID, workerID).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("read resilience worker heartbeat: %w", err)
	}
	if len(rows) == 0 {
		return nil, ErrStateNotFound
	}
	if len(rows) != 1 {
		return nil, fmt.Errorf("%w: duplicate worker heartbeat rows", ErrStateConflict)
	}
	heartbeat, err := decodePostgresHeartbeat(rows[0], scope)
	if err != nil {
		return nil, err
	}
	return &heartbeat, nil
}

func (r *PostgresRepository) GetCircuit(ctx context.Context, scope Scope, circuitID string) (*CircuitState, error) {
	if err := repositoryScopedID(ctx, scope, "circuit id", circuitID); err != nil {
		return nil, err
	}
	if err := r.ready(ctx, scope); err != nil {
		return nil, err
	}
	return loadPostgresCircuit(r.DB.WithContext(ctx), scope, circuitID, false)
}

func (r *PostgresRepository) ListCircuits(ctx context.Context, scope Scope, limit int) ([]CircuitState, error) {
	if err := r.ready(ctx, scope); err != nil {
		return nil, err
	}
	limit, err := boundedListLimit(limit, MaxHistoryLimit)
	if err != nil {
		return nil, err
	}
	var rows []postgresCircuitRow
	if err := r.DB.WithContext(ctx).Raw(`SELECT owner_id, workspace_id, circuit_id, revision,
		circuit_phase, contract_version, payload::text AS payload
		FROM public.resilience_circuits WHERE owner_id = ? AND workspace_id = ?
		ORDER BY circuit_id ASC LIMIT ?`, scope.OwnerID, scope.WorkspaceID, limit).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list resilience circuits: %w", err)
	}
	result := make([]CircuitState, 0, len(rows))
	for _, row := range rows {
		state, decodeErr := decodePostgresCircuit(row, scope)
		if decodeErr != nil {
			return nil, decodeErr
		}
		result = append(result, state)
	}
	return result, nil
}

func (r *PostgresRepository) CompareAndSwapCircuit(ctx context.Context, scope Scope, circuitID string, expectedRevision uint64, next CircuitState) error {
	if err := repositoryScopedID(ctx, scope, "circuit id", circuitID); err != nil {
		return err
	}
	if err := r.ready(ctx, scope); err != nil {
		return err
	}
	if err := validateCircuitState(next); err != nil {
		return err
	}
	if next.Scope != scope || next.CircuitID != circuitID {
		return ErrStateConflict
	}
	payload, err := marshalPostgresRecord("circuit", next)
	if err != nil {
		return err
	}
	return r.transaction(ctx, func(tx *gorm.DB) error {
		current, loadErr := loadPostgresCircuit(tx, scope, circuitID, true)
		if errors.Is(loadErr, ErrStateNotFound) {
			if expectedRevision != 0 || next.Revision != 1 {
				return ErrStaleFence
			}
			result := tx.Exec(`INSERT INTO public.resilience_circuits
				(owner_id, workspace_id, circuit_id, revision, circuit_phase, updated_at,
				 contract_version, payload)
				VALUES (?, ?, ?, ?, ?, ?, ?, CAST(? AS jsonb))
				ON CONFLICT (owner_id, workspace_id, circuit_id) DO NOTHING`,
				scope.OwnerID, scope.WorkspaceID, circuitID, next.Revision, next.Phase,
				time.Now().UTC(), next.ContractVersion, string(payload))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrStaleFence
			}
			return nil
		}
		if loadErr != nil {
			return loadErr
		}
		if current.Revision != expectedRevision || next.Revision != expectedRevision+1 {
			return ErrStaleFence
		}
		if err := validCircuitTransition(*current, next); err != nil {
			return err
		}
		result := tx.Exec(`UPDATE public.resilience_circuits SET revision = ?, circuit_phase = ?,
			updated_at = ?, contract_version = ?, payload = CAST(? AS jsonb)
			WHERE owner_id = ? AND workspace_id = ? AND circuit_id = ? AND revision = ?`,
			next.Revision, next.Phase, time.Now().UTC(), next.ContractVersion, string(payload),
			scope.OwnerID, scope.WorkspaceID, circuitID, current.Revision)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrStaleFence
		}
		return nil
	})
}

func loadPostgresCircuit(db *gorm.DB, scope Scope, circuitID string, lock bool) (*CircuitState, error) {
	query := `SELECT owner_id, workspace_id, circuit_id, revision, circuit_phase,
		contract_version, payload::text AS payload FROM public.resilience_circuits
		WHERE owner_id = ? AND workspace_id = ? AND circuit_id = ?`
	if lock {
		query += ` FOR UPDATE`
	}
	var rows []postgresCircuitRow
	if err := db.Raw(query, scope.OwnerID, scope.WorkspaceID, circuitID).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("read resilience circuit: %w", err)
	}
	if len(rows) == 0 {
		return nil, ErrStateNotFound
	}
	if len(rows) != 1 {
		return nil, fmt.Errorf("%w: duplicate circuit rows", ErrStateConflict)
	}
	state, err := decodePostgresCircuit(rows[0], scope)
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func (r *PostgresRepository) LatestRetry(ctx context.Context, scope Scope, workID string) (*RetryRecord, error) {
	if err := repositoryScopedID(ctx, scope, "work id", workID); err != nil {
		return nil, err
	}
	if err := r.ready(ctx, scope); err != nil {
		return nil, err
	}
	return loadLatestPostgresRetry(r.DB.WithContext(ctx), scope, workID, false)
}

func (r *PostgresRepository) AppendRetry(ctx context.Context, expectedSequence uint64, record RetryRecord) error {
	if err := r.ready(ctx, record.Scope); err != nil {
		return err
	}
	if err := validateRetryRecord(record); err != nil {
		return err
	}
	payload, err := marshalPostgresRecord("retry record", record)
	if err != nil {
		return err
	}
	return r.transaction(ctx, func(tx *gorm.DB) error {
		if err := postgresAdvisoryLock(tx, "retry", record.Scope, record.WorkID); err != nil {
			return err
		}
		current, loadErr := loadLatestPostgresRetry(tx, record.Scope, record.WorkID, true)
		sequence := uint64(0)
		if loadErr == nil {
			sequence = current.Sequence
		} else if !errors.Is(loadErr, ErrStateNotFound) {
			return loadErr
		}
		if sequence != expectedSequence || record.Sequence != expectedSequence+1 {
			return ErrStaleFence
		}
		result := tx.Exec(`INSERT INTO public.resilience_retry_records
			(owner_id, workspace_id, work_id, sequence, requested_at, contract_version, payload)
			VALUES (?, ?, ?, ?, ?, ?, CAST(? AS jsonb))`, record.Scope.OwnerID,
			record.Scope.WorkspaceID, record.WorkID, record.Sequence, record.RequestedAt.UTC(),
			record.ContractVersion, string(payload))
		return result.Error
	})
}

func (r *PostgresRepository) ListRetries(ctx context.Context, scope Scope, workID string, limit int) ([]RetryRecord, error) {
	if err := repositoryScopedID(ctx, scope, "work id", workID); err != nil {
		return nil, err
	}
	if err := r.ready(ctx, scope); err != nil {
		return nil, err
	}
	limit, err := boundedListLimit(limit, MaxHistoryLimit)
	if err != nil {
		return nil, err
	}
	var rows []postgresHistoryRow
	if err := r.DB.WithContext(ctx).Raw(`SELECT owner_id, workspace_id, work_id, sequence,
		requested_at, contract_version, payload FROM (
			SELECT owner_id, workspace_id, work_id, sequence, requested_at, contract_version,
				payload::text AS payload FROM public.resilience_retry_records
			WHERE owner_id = ? AND workspace_id = ? AND work_id = ?
			ORDER BY sequence DESC LIMIT ?
		) recent ORDER BY sequence ASC`, scope.OwnerID, scope.WorkspaceID, workID, limit).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list resilience retries: %w", err)
	}
	return decodePostgresRetries(rows, scope)
}

func (r *PostgresRepository) ListAllRetries(ctx context.Context, scope Scope, limit int) ([]RetryRecord, error) {
	if err := r.ready(ctx, scope); err != nil {
		return nil, err
	}
	limit, err := boundedListLimit(limit, MaxHistoryLimit)
	if err != nil {
		return nil, err
	}
	var rows []postgresHistoryRow
	if err := r.DB.WithContext(ctx).Raw(`SELECT owner_id, workspace_id, work_id, sequence,
		requested_at, contract_version, payload::text AS payload
		FROM public.resilience_retry_records WHERE owner_id = ? AND workspace_id = ?
		ORDER BY requested_at DESC, work_id ASC, sequence DESC LIMIT ?`,
		scope.OwnerID, scope.WorkspaceID, limit).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list all resilience retries: %w", err)
	}
	return decodePostgresRetries(rows, scope)
}

func loadLatestPostgresRetry(db *gorm.DB, scope Scope, workID string, lock bool) (*RetryRecord, error) {
	query := `SELECT owner_id, workspace_id, work_id, sequence, requested_at,
		contract_version, payload::text AS payload FROM public.resilience_retry_records
		WHERE owner_id = ? AND workspace_id = ? AND work_id = ?
		ORDER BY sequence DESC LIMIT 1`
	if lock {
		query += ` FOR UPDATE`
	}
	var rows []postgresHistoryRow
	if err := db.Raw(query, scope.OwnerID, scope.WorkspaceID, workID).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("read latest resilience retry: %w", err)
	}
	if len(rows) == 0 {
		return nil, ErrStateNotFound
	}
	record, err := decodePostgresRetry(rows[0], scope)
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *PostgresRepository) LatestEvent(ctx context.Context, scope Scope) (*EventRecord, error) {
	if err := r.ready(ctx, scope); err != nil {
		return nil, err
	}
	return loadLatestPostgresEvent(r.DB.WithContext(ctx), scope, false)
}

func (r *PostgresRepository) AppendEvent(ctx context.Context, record EventRecord) error {
	if err := r.ready(ctx, record.Event.Scope); err != nil {
		return err
	}
	if err := validatePostgresEventRecord(record); err != nil {
		return err
	}
	payload, err := marshalPostgresRecord("event record", record)
	if err != nil {
		return err
	}
	return r.transaction(ctx, func(tx *gorm.DB) error {
		if err := postgresAdvisoryLock(tx, "event", record.Event.Scope, "scope"); err != nil {
			return err
		}
		latest, loadErr := loadLatestPostgresEvent(tx, record.Event.Scope, true)
		if errors.Is(loadErr, ErrStateNotFound) {
			if record.Event.Sequence != 1 || record.Event.PreviousHash != "" {
				return ErrStaleFence
			}
		} else if loadErr != nil {
			return loadErr
		} else if record.Event.Sequence != latest.Event.Sequence+1 || record.Event.PreviousHash != latest.Hash {
			return ErrStaleFence
		}
		result := tx.Exec(`INSERT INTO public.resilience_event_records
			(owner_id, workspace_id, sequence, event_hash, previous_hash, event_type,
			 subject_id, occurred_at, contract_version, payload)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CAST(? AS jsonb))`,
			record.Event.Scope.OwnerID, record.Event.Scope.WorkspaceID, record.Event.Sequence,
			record.Hash, record.Event.PreviousHash, record.Event.Type, record.Event.SubjectID,
			record.Event.OccurredAt.UTC(), record.Event.ContractVersion, string(payload))
		return result.Error
	})
}

func (r *PostgresRepository) ListEvents(ctx context.Context, scope Scope, limit int) ([]EventRecord, error) {
	if err := r.ready(ctx, scope); err != nil {
		return nil, err
	}
	limit, err := boundedListLimit(limit, MaxHistoryLimit)
	if err != nil {
		return nil, err
	}
	queryLimit := limit + 1
	var rows []postgresEventRow
	if err := r.DB.WithContext(ctx).Raw(`SELECT owner_id, workspace_id, sequence,
		event_hash, previous_hash, event_type, subject_id, occurred_at,
		contract_version, payload FROM (
			SELECT owner_id, workspace_id, sequence, event_hash, previous_hash,
				event_type, subject_id, occurred_at, contract_version, payload::text AS payload
			FROM public.resilience_event_records
			WHERE owner_id = ? AND workspace_id = ? ORDER BY sequence DESC LIMIT ?
		) recent ORDER BY sequence ASC`, scope.OwnerID, scope.WorkspaceID, queryLimit).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list resilience events: %w", err)
	}
	result := make([]EventRecord, 0, len(rows))
	var previous *EventRecord
	for _, row := range rows {
		record, decodeErr := decodePostgresEvent(row, scope)
		if decodeErr != nil {
			return nil, decodeErr
		}
		if previous != nil && (record.Event.Sequence != previous.Event.Sequence+1 || record.Event.PreviousHash != previous.Hash) {
			return nil, fmt.Errorf("%w: stored event chain is discontinuous", ErrStateConflict)
		}
		result = append(result, record)
		previous = &result[len(result)-1]
	}
	if len(result) > limit {
		result = append([]EventRecord(nil), result[len(result)-limit:]...)
	}
	return result, nil
}

func loadLatestPostgresEvent(db *gorm.DB, scope Scope, lock bool) (*EventRecord, error) {
	query := `SELECT owner_id, workspace_id, sequence, event_hash, previous_hash,
		event_type, subject_id, occurred_at, contract_version, payload::text AS payload
		FROM public.resilience_event_records WHERE owner_id = ? AND workspace_id = ?
		ORDER BY sequence DESC LIMIT 2`
	if lock {
		query += ` FOR UPDATE`
	}
	var rows []postgresEventRow
	if err := db.Raw(query, scope.OwnerID, scope.WorkspaceID).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("read latest resilience event: %w", err)
	}
	if len(rows) == 0 {
		return nil, ErrStateNotFound
	}
	record, err := decodePostgresEvent(rows[0], scope)
	if err != nil {
		return nil, err
	}
	if len(rows) == 1 && record.Event.Sequence != 1 {
		return nil, fmt.Errorf("%w: stored event predecessor is missing", ErrStateConflict)
	}
	if len(rows) == 2 {
		previous, decodeErr := decodePostgresEvent(rows[1], scope)
		if decodeErr != nil {
			return nil, decodeErr
		}
		if record.Event.Sequence != previous.Event.Sequence+1 || record.Event.PreviousHash != previous.Hash {
			return nil, fmt.Errorf("%w: stored event chain is discontinuous", ErrStateConflict)
		}
	}
	return &record, nil
}

func (r *PostgresRepository) LatestRecovery(ctx context.Context, scope Scope, workID string) (*RecoveryRecord, error) {
	if err := repositoryScopedID(ctx, scope, "work id", workID); err != nil {
		return nil, err
	}
	if err := r.ready(ctx, scope); err != nil {
		return nil, err
	}
	return loadLatestPostgresRecovery(r.DB.WithContext(ctx), scope, workID, false)
}

func (r *PostgresRepository) AppendRecovery(ctx context.Context, expectedSequence uint64, record RecoveryRecord) error {
	if err := r.ready(ctx, record.Scope); err != nil {
		return err
	}
	if err := validateRecoveryRecord(record); err != nil {
		return err
	}
	payload, err := marshalPostgresRecord("recovery record", record)
	if err != nil {
		return err
	}
	return r.transaction(ctx, func(tx *gorm.DB) error {
		if err := postgresAdvisoryLock(tx, "recovery", record.Scope, record.WorkID); err != nil {
			return err
		}
		current, loadErr := loadLatestPostgresRecovery(tx, record.Scope, record.WorkID, true)
		sequence := uint64(0)
		if loadErr == nil {
			sequence = current.Sequence
		} else if !errors.Is(loadErr, ErrStateNotFound) {
			return loadErr
		}
		if sequence != expectedSequence || record.Sequence != expectedSequence+1 {
			return ErrStaleFence
		}
		result := tx.Exec(`INSERT INTO public.resilience_recovery_records
			(owner_id, workspace_id, work_id, sequence, requested_at, contract_version, payload)
			VALUES (?, ?, ?, ?, ?, ?, CAST(? AS jsonb))`, record.Scope.OwnerID,
			record.Scope.WorkspaceID, record.WorkID, record.Sequence, record.RequestedAt.UTC(),
			record.ContractVersion, string(payload))
		return result.Error
	})
}

func (r *PostgresRepository) ListRecoveries(ctx context.Context, scope Scope, workID string, limit int) ([]RecoveryRecord, error) {
	if err := repositoryScopedID(ctx, scope, "work id", workID); err != nil {
		return nil, err
	}
	if err := r.ready(ctx, scope); err != nil {
		return nil, err
	}
	limit, err := boundedListLimit(limit, MaxHistoryLimit)
	if err != nil {
		return nil, err
	}
	var rows []postgresHistoryRow
	if err := r.DB.WithContext(ctx).Raw(`SELECT owner_id, workspace_id, work_id, sequence,
		requested_at, contract_version, payload FROM (
			SELECT owner_id, workspace_id, work_id, sequence, requested_at, contract_version,
				payload::text AS payload FROM public.resilience_recovery_records
			WHERE owner_id = ? AND workspace_id = ? AND work_id = ?
			ORDER BY sequence DESC LIMIT ?
		) recent ORDER BY sequence ASC`, scope.OwnerID, scope.WorkspaceID, workID, limit).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list resilience recoveries: %w", err)
	}
	return decodePostgresRecoveries(rows, scope)
}

func (r *PostgresRepository) ListAllRecoveries(ctx context.Context, scope Scope, limit int) ([]RecoveryRecord, error) {
	if err := r.ready(ctx, scope); err != nil {
		return nil, err
	}
	limit, err := boundedListLimit(limit, MaxHistoryLimit)
	if err != nil {
		return nil, err
	}
	var rows []postgresHistoryRow
	if err := r.DB.WithContext(ctx).Raw(`SELECT owner_id, workspace_id, work_id, sequence,
		requested_at, contract_version, payload::text AS payload
		FROM public.resilience_recovery_records WHERE owner_id = ? AND workspace_id = ?
		ORDER BY requested_at DESC, work_id ASC, sequence DESC LIMIT ?`,
		scope.OwnerID, scope.WorkspaceID, limit).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list all resilience recoveries: %w", err)
	}
	return decodePostgresRecoveries(rows, scope)
}

func loadLatestPostgresRecovery(db *gorm.DB, scope Scope, workID string, lock bool) (*RecoveryRecord, error) {
	query := `SELECT owner_id, workspace_id, work_id, sequence, requested_at,
		contract_version, payload::text AS payload FROM public.resilience_recovery_records
		WHERE owner_id = ? AND workspace_id = ? AND work_id = ?
		ORDER BY sequence DESC LIMIT 1`
	if lock {
		query += ` FOR UPDATE`
	}
	var rows []postgresHistoryRow
	if err := db.Raw(query, scope.OwnerID, scope.WorkspaceID, workID).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("read latest resilience recovery: %w", err)
	}
	if len(rows) == 0 {
		return nil, ErrStateNotFound
	}
	record, err := decodePostgresRecovery(rows[0], scope)
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func decodePostgresIdempotency(row postgresIdempotencyRow, scope Scope) (IdempotencyRecord, error) {
	var record IdempotencyRecord
	if err := decodeStrictPostgresRecord("idempotency record", row.Payload, &record); err != nil {
		return IdempotencyRecord{}, err
	}
	if row.OwnerID != scope.OwnerID || row.WorkspaceID != scope.WorkspaceID ||
		record.Scope != scope || row.IdempotencyKey != record.IdempotencyKey ||
		row.WorkID != record.WorkID || row.PayloadHash != record.PayloadHash ||
		row.ContractVersion != record.ContractVersion || !postgresTimesEqual(row.RecordedAt, record.RecordedAt) {
		return IdempotencyRecord{}, corruptPostgresRecord("idempotency record")
	}
	if err := validateIdempotencyRecord(record); err != nil {
		return IdempotencyRecord{}, corruptPostgresRecord("idempotency record")
	}
	return record, nil
}

func decodePostgresLease(row postgresLeaseRow, scope Scope) (WorkLease, error) {
	var lease WorkLease
	if err := decodeStrictPostgresRecord("lease", row.Payload, &lease); err != nil {
		return WorkLease{}, err
	}
	if row.OwnerID != scope.OwnerID || row.WorkspaceID != scope.WorkspaceID || lease.Scope != scope ||
		row.WorkID != lease.WorkID || row.IdempotencyKey != lease.IdempotencyKey ||
		row.PayloadHash != lease.PayloadHash || row.WorkerID != lease.WorkerID ||
		row.Generation != lease.Generation || row.LeaseState != string(lease.State) ||
		row.ContractVersion != lease.ContractVersion || !postgresTimesEqual(row.AcquiredAt, lease.AcquiredAt) ||
		!postgresTimesEqual(row.LastHeartbeatAt, lease.LastHeartbeatAt) || !postgresTimesEqual(row.ExpiresAt, lease.ExpiresAt) ||
		!postgresOptionalTimesEqual(row.ReleasedAt, lease.ReleasedAt) {
		return WorkLease{}, corruptPostgresRecord("lease")
	}
	if err := validateLease(lease); err != nil {
		return WorkLease{}, corruptPostgresRecord("lease")
	}
	return *cloneLease(&lease), nil
}

func decodePostgresHeartbeat(row postgresHeartbeatRow, scope Scope) (WorkerHeartbeat, error) {
	var heartbeat WorkerHeartbeat
	if err := decodeStrictPostgresRecord("worker heartbeat", row.Payload, &heartbeat); err != nil {
		return WorkerHeartbeat{}, err
	}
	if row.OwnerID != scope.OwnerID || row.WorkspaceID != scope.WorkspaceID || heartbeat.Scope != scope ||
		row.WorkerID != heartbeat.WorkerID || row.Sequence != heartbeat.Sequence ||
		row.ContractVersion != heartbeat.ContractVersion || !postgresTimesEqual(row.ObservedAt, heartbeat.ObservedAt) {
		return WorkerHeartbeat{}, corruptPostgresRecord("worker heartbeat")
	}
	if err := validateWorkerHeartbeat(heartbeat); err != nil {
		return WorkerHeartbeat{}, corruptPostgresRecord("worker heartbeat")
	}
	return heartbeat, nil
}

func decodePostgresCircuit(row postgresCircuitRow, scope Scope) (CircuitState, error) {
	var state CircuitState
	if err := decodeStrictPostgresRecord("circuit", row.Payload, &state); err != nil {
		return CircuitState{}, err
	}
	if row.OwnerID != scope.OwnerID || row.WorkspaceID != scope.WorkspaceID || state.Scope != scope ||
		row.CircuitID != state.CircuitID || row.Revision != state.Revision ||
		row.CircuitPhase != string(state.Phase) || row.ContractVersion != state.ContractVersion {
		return CircuitState{}, corruptPostgresRecord("circuit")
	}
	if err := validateCircuitState(state); err != nil {
		return CircuitState{}, corruptPostgresRecord("circuit")
	}
	return cloneCircuit(state), nil
}

func decodePostgresRetry(row postgresHistoryRow, scope Scope) (RetryRecord, error) {
	var record RetryRecord
	if err := decodeStrictPostgresRecord("retry record", row.Payload, &record); err != nil {
		return RetryRecord{}, err
	}
	if row.OwnerID != scope.OwnerID || row.WorkspaceID != scope.WorkspaceID || record.Scope != scope ||
		row.WorkID != record.WorkID || row.Sequence != record.Sequence ||
		row.ContractVersion != record.ContractVersion || !postgresTimesEqual(row.RequestedAt, record.RequestedAt) {
		return RetryRecord{}, corruptPostgresRecord("retry record")
	}
	if err := validateRetryRecord(record); err != nil {
		return RetryRecord{}, corruptPostgresRecord("retry record")
	}
	return cloneRetryRecord(record), nil
}

func decodePostgresRetries(rows []postgresHistoryRow, scope Scope) ([]RetryRecord, error) {
	result := make([]RetryRecord, 0, len(rows))
	for _, row := range rows {
		record, err := decodePostgresRetry(row, scope)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, nil
}

func decodePostgresRecovery(row postgresHistoryRow, scope Scope) (RecoveryRecord, error) {
	var record RecoveryRecord
	if err := decodeStrictPostgresRecord("recovery record", row.Payload, &record); err != nil {
		return RecoveryRecord{}, err
	}
	if row.OwnerID != scope.OwnerID || row.WorkspaceID != scope.WorkspaceID || record.Scope != scope ||
		row.WorkID != record.WorkID || row.Sequence != record.Sequence ||
		row.ContractVersion != record.ContractVersion || !postgresTimesEqual(row.RequestedAt, record.RequestedAt) {
		return RecoveryRecord{}, corruptPostgresRecord("recovery record")
	}
	if err := validateRecoveryRecord(record); err != nil {
		return RecoveryRecord{}, corruptPostgresRecord("recovery record")
	}
	return cloneRecoveryRecord(record), nil
}

func decodePostgresRecoveries(rows []postgresHistoryRow, scope Scope) ([]RecoveryRecord, error) {
	result := make([]RecoveryRecord, 0, len(rows))
	for _, row := range rows {
		record, err := decodePostgresRecovery(row, scope)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, nil
}

func decodePostgresEvent(row postgresEventRow, scope Scope) (EventRecord, error) {
	var record EventRecord
	if err := decodeStrictPostgresRecord("event record", row.Payload, &record); err != nil {
		return EventRecord{}, err
	}
	if row.OwnerID != scope.OwnerID || row.WorkspaceID != scope.WorkspaceID || record.Event.Scope != scope ||
		row.Sequence != record.Event.Sequence || row.EventHash != record.Hash ||
		row.PreviousHash != record.Event.PreviousHash || row.EventType != record.Event.Type ||
		row.SubjectID != record.Event.SubjectID || row.ContractVersion != record.Event.ContractVersion ||
		!postgresTimesEqual(row.OccurredAt, record.Event.OccurredAt) {
		return EventRecord{}, corruptPostgresRecord("event record")
	}
	if err := validatePostgresEventRecord(record); err != nil {
		return EventRecord{}, corruptPostgresRecord("event record")
	}
	return cloneEventRecord(record), nil
}

func validatePostgresEventRecord(record EventRecord) error {
	if !isAdvisory(record.Authority) {
		return ErrStateConflict
	}
	hash, err := EventHash(record.Event)
	if err != nil {
		return err
	}
	if hash != record.Hash {
		return ErrStateConflict
	}
	return nil
}

func marshalPostgresRecord(name string, value any) ([]byte, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode resilience %s: %w", name, err)
	}
	if len(payload) < 2 || len(payload) > 1<<20 || payload[0] != '{' || !json.Valid(payload) {
		return nil, fmt.Errorf("encode resilience %s: bounded JSON object required", name)
	}
	return payload, nil
}

func decodeStrictPostgresRecord(name, payload string, target any) error {
	payload = strings.TrimSpace(payload)
	if len(payload) < 2 || len(payload) > 1<<20 || payload[0] != '{' || payload == "null" {
		return corruptPostgresRecord(name)
	}
	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return corruptPostgresRecord(name)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return corruptPostgresRecord(name)
	}
	return nil
}

func corruptPostgresRecord(name string) error {
	return fmt.Errorf("%w: stored postgres %s is invalid", ErrStateConflict, name)
}

func postgresAdvisoryLock(tx *gorm.DB, stream string, scope Scope, subjectID string) error {
	key := strings.Join([]string{"hai-resilience/v1", stream, scope.OwnerID, scope.WorkspaceID, subjectID}, "\x1f")
	if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`, key).Error; err != nil {
		return fmt.Errorf("lock resilience stream: %w", err)
	}
	return nil
}

func translatePostgresWriteError(err error) error {
	if err == nil || errors.Is(err, ErrStateNotFound) || errors.Is(err, ErrStateConflict) || errors.Is(err, ErrStaleFence) {
		return err
	}
	var state interface{ SQLState() string }
	if errors.As(err, &state) {
		switch state.SQLState() {
		case "23505", "40001", "40P01":
			return fmt.Errorf("%w: concurrent postgres resilience write", ErrStaleFence)
		}
	}
	return err
}

func postgresTimesEqual(left, right time.Time) bool {
	difference := left.UTC().Sub(right.UTC())
	if difference < 0 {
		difference = -difference
	}
	return difference <= time.Microsecond
}

func postgresOptionalTimesEqual(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return postgresTimesEqual(*left, *right)
}

func utcTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	utc := value.UTC()
	return &utc
}

var _ Repository = (*PostgresRepository)(nil)
