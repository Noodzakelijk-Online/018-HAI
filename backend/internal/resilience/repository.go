package resilience

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

const (
	DefaultHistoryLimit = 100
	MaxHistoryLimit     = 1000
)

var (
	ErrStateNotFound = errors.New("resilience state not found")
	ErrStateConflict = errors.New("resilience state conflict")
	ErrStaleFence    = errors.New("resilience stale fencing token")
)

// EventRecord is a validated, hash-chained advisory control-plane event.
type EventRecord struct {
	Event     ControlEvent      `json:"event"`
	Hash      string            `json:"hash"`
	Authority AuthorityBoundary `json:"authority"`
}

// RecoveryRecord retains the durable evidence and recommendation used for a
// recovery assessment. It is evidence only and carries no execution authority.
type RecoveryRecord struct {
	ContractVersion int               `json:"contractVersion"`
	Scope           Scope             `json:"scope"`
	WorkID          string            `json:"workId"`
	Sequence        uint64            `json:"sequence"`
	RequestedAt     time.Time         `json:"requestedAt"`
	Request         RecoveryRequest   `json:"request"`
	Decision        RecoveryDecision  `json:"decision"`
	Authority       AuthorityBoundary `json:"authority"`
}

// RetryRecord retains an immutable retry/dead-letter recommendation. It is
// advisory evidence only and is never a scheduled job.
type RetryRecord struct {
	ContractVersion int               `json:"contractVersion"`
	Scope           Scope             `json:"scope"`
	WorkID          string            `json:"workId"`
	Sequence        uint64            `json:"sequence"`
	RequestedAt     time.Time         `json:"requestedAt"`
	Policy          RetryPolicy       `json:"policy"`
	Decision        RetryDecision     `json:"decision"`
	Authority       AuthorityBoundary `json:"authority"`
}

// Repository is the persistence boundary for advisory resilience state.
// Implementations must make all compare-and-swap operations atomic and must
// never return mutable aliases to durable state.
type Repository interface {
	LookupIdempotency(context.Context, Scope, string) (*IdempotencyRecord, error)
	CreateIdempotency(context.Context, IdempotencyRecord) (*IdempotencyRecord, bool, error)

	GetLease(context.Context, Scope, string) (*WorkLease, error)
	ListLeases(context.Context, Scope, int) ([]WorkLease, error)
	CompareAndSwapLease(context.Context, Scope, string, *WorkLease, WorkLease) error

	GetWorkerHeartbeat(context.Context, Scope, string) (*WorkerHeartbeat, error)
	ListWorkerHeartbeats(context.Context, Scope, int) ([]WorkerHeartbeat, error)
	CompareAndSwapWorkerHeartbeat(context.Context, Scope, string, *WorkerHeartbeat, WorkerHeartbeat) error

	GetCircuit(context.Context, Scope, string) (*CircuitState, error)
	ListCircuits(context.Context, Scope, int) ([]CircuitState, error)
	CompareAndSwapCircuit(context.Context, Scope, string, uint64, CircuitState) error

	LatestRetry(context.Context, Scope, string) (*RetryRecord, error)
	AppendRetry(context.Context, uint64, RetryRecord) error
	ListRetries(context.Context, Scope, string, int) ([]RetryRecord, error)
	ListAllRetries(context.Context, Scope, int) ([]RetryRecord, error)

	LatestEvent(context.Context, Scope) (*EventRecord, error)
	AppendEvent(context.Context, EventRecord) error
	ListEvents(context.Context, Scope, int) ([]EventRecord, error)

	LatestRecovery(context.Context, Scope, string) (*RecoveryRecord, error)
	AppendRecovery(context.Context, uint64, RecoveryRecord) error
	ListRecoveries(context.Context, Scope, string, int) ([]RecoveryRecord, error)
	ListAllRecoveries(context.Context, Scope, int) ([]RecoveryRecord, error)
}

type scopedID struct {
	scope Scope
	id    string
}

// MemoryRepository is a concurrency-safe reference repository. It is intended
// for tests and single-process deployments, not as a substitute for durable
// cross-process storage.
type MemoryRepository struct {
	mu              sync.RWMutex
	historyLimit    int
	idempotency     map[scopedID]IdempotencyRecord
	leases          map[scopedID]WorkLease
	heartbeats      map[scopedID]WorkerHeartbeat
	circuits        map[scopedID]CircuitState
	retryHistory    map[scopedID][]RetryRecord
	events          map[Scope][]EventRecord
	recoveryHistory map[scopedID][]RecoveryRecord
}

// NewMemoryRepository returns a bounded in-memory repository. A missing or
// non-positive limit uses DefaultHistoryLimit; values above MaxHistoryLimit are
// clamped so a caller cannot accidentally create unbounded histories.
func NewMemoryRepository(historyLimit ...int) *MemoryRepository {
	limit := DefaultHistoryLimit
	if len(historyLimit) > 0 && historyLimit[0] > 0 {
		limit = historyLimit[0]
	}
	if limit > MaxHistoryLimit {
		limit = MaxHistoryLimit
	}
	return &MemoryRepository{
		historyLimit:    limit,
		idempotency:     make(map[scopedID]IdempotencyRecord),
		leases:          make(map[scopedID]WorkLease),
		heartbeats:      make(map[scopedID]WorkerHeartbeat),
		circuits:        make(map[scopedID]CircuitState),
		retryHistory:    make(map[scopedID][]RetryRecord),
		events:          make(map[Scope][]EventRecord),
		recoveryHistory: make(map[scopedID][]RecoveryRecord),
	}
}

func (r *MemoryRepository) ListLeases(ctx context.Context, scope Scope, limit int) ([]WorkLease, error) {
	if err := repositoryInput(ctx, scope); err != nil {
		return nil, err
	}
	limit, err := boundedListLimit(limit, MaxHistoryLimit)
	if err != nil {
		return nil, err
	}
	r.mu.RLock()
	result := make([]WorkLease, 0, len(r.leases))
	for key, lease := range r.leases {
		if key.scope == scope {
			result = append(result, *cloneLease(&lease))
		}
	}
	r.mu.RUnlock()
	sort.Slice(result, func(i, j int) bool { return result[i].WorkID < result[j].WorkID })
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func NewInMemoryRepository(historyLimit ...int) *MemoryRepository {
	return NewMemoryRepository(historyLimit...)
}

func (r *MemoryRepository) LookupIdempotency(ctx context.Context, scope Scope, key string) (*IdempotencyRecord, error) {
	if err := repositoryInput(ctx, scope); err != nil {
		return nil, err
	}
	if err := validateHash("idempotency key", key, false); err != nil {
		return nil, err
	}
	r.mu.RLock()
	record, ok := r.idempotency[scopedID{scope: scope, id: key}]
	r.mu.RUnlock()
	if !ok {
		return nil, ErrStateNotFound
	}
	if err := validateIdempotencyRecord(record); err != nil {
		return nil, fmt.Errorf("%w: idempotency", ErrStateConflict)
	}
	return cloneIdempotencyRecord(&record), nil
}

func (r *MemoryRepository) CreateIdempotency(ctx context.Context, record IdempotencyRecord) (*IdempotencyRecord, bool, error) {
	if err := repositoryInput(ctx, record.Scope); err != nil {
		return nil, false, err
	}
	if err := validateIdempotencyRecord(record); err != nil {
		return nil, false, err
	}
	key := scopedID{scope: record.Scope, id: record.IdempotencyKey}
	r.mu.Lock()
	defer r.mu.Unlock()
	if current, ok := r.idempotency[key]; ok {
		if current.PayloadHash != record.PayloadHash {
			return nil, false, ErrStateConflict
		}
		return cloneIdempotencyRecord(&current), false, nil
	}
	r.idempotency[key] = *cloneIdempotencyRecord(&record)
	return cloneIdempotencyRecord(&record), true, nil
}

func (r *MemoryRepository) GetLease(ctx context.Context, scope Scope, workID string) (*WorkLease, error) {
	if err := repositoryScopedID(ctx, scope, "work id", workID); err != nil {
		return nil, err
	}
	r.mu.RLock()
	lease, ok := r.leases[scopedID{scope: scope, id: workID}]
	r.mu.RUnlock()
	if !ok {
		return nil, ErrStateNotFound
	}
	if err := validateLease(lease); err != nil {
		return nil, fmt.Errorf("%w: lease", ErrStateConflict)
	}
	return cloneLease(&lease), nil
}

func (r *MemoryRepository) CompareAndSwapLease(ctx context.Context, scope Scope, workID string, expected *WorkLease, next WorkLease) error {
	if err := repositoryScopedID(ctx, scope, "work id", workID); err != nil {
		return err
	}
	if err := validateLease(next); err != nil {
		return err
	}
	if err := requireSameScope(scope, next.Scope); err != nil || next.WorkID != workID {
		return ErrStateConflict
	}
	if expected != nil {
		if err := validateLease(*expected); err != nil {
			return err
		}
		if err := requireSameScope(scope, expected.Scope); err != nil || expected.WorkID != workID {
			return ErrStateConflict
		}
	}

	key := scopedID{scope: scope, id: workID}
	r.mu.Lock()
	defer r.mu.Unlock()
	current, exists := r.leases[key]
	if !exists {
		if expected != nil || next.Generation != 1 {
			return ErrStaleFence
		}
		r.leases[key] = *cloneLease(&next)
		return nil
	}
	if expected == nil || !sameLease(current, *expected) {
		return ErrStaleFence
	}
	if err := validLeaseTransition(current, next); err != nil {
		return err
	}
	r.leases[key] = *cloneLease(&next)
	return nil
}

func (r *MemoryRepository) GetWorkerHeartbeat(ctx context.Context, scope Scope, workerID string) (*WorkerHeartbeat, error) {
	if err := repositoryScopedID(ctx, scope, "worker id", workerID); err != nil {
		return nil, err
	}
	r.mu.RLock()
	heartbeat, ok := r.heartbeats[scopedID{scope: scope, id: workerID}]
	r.mu.RUnlock()
	if !ok {
		return nil, ErrStateNotFound
	}
	if err := validateWorkerHeartbeat(heartbeat); err != nil {
		return nil, fmt.Errorf("%w: heartbeat", ErrStateConflict)
	}
	copyHeartbeat := heartbeat
	return &copyHeartbeat, nil
}

func (r *MemoryRepository) ListWorkerHeartbeats(ctx context.Context, scope Scope, limit int) ([]WorkerHeartbeat, error) {
	if err := repositoryInput(ctx, scope); err != nil {
		return nil, err
	}
	limit, err := boundedListLimit(limit, MaxHistoryLimit)
	if err != nil {
		return nil, err
	}
	r.mu.RLock()
	result := make([]WorkerHeartbeat, 0, len(r.heartbeats))
	for key, heartbeat := range r.heartbeats {
		if key.scope == scope {
			result = append(result, heartbeat)
		}
	}
	r.mu.RUnlock()
	sort.Slice(result, func(i, j int) bool { return result[i].WorkerID < result[j].WorkerID })
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (r *MemoryRepository) CompareAndSwapWorkerHeartbeat(ctx context.Context, scope Scope, workerID string, expected *WorkerHeartbeat, next WorkerHeartbeat) error {
	if err := repositoryScopedID(ctx, scope, "worker id", workerID); err != nil {
		return err
	}
	if err := validateWorkerHeartbeat(next); err != nil {
		return err
	}
	if err := requireSameScope(scope, next.Scope); err != nil || next.WorkerID != workerID {
		return ErrStateConflict
	}
	if expected != nil {
		if err := validateWorkerHeartbeat(*expected); err != nil {
			return err
		}
		if err := requireSameScope(scope, expected.Scope); err != nil || expected.WorkerID != workerID {
			return ErrStateConflict
		}
	}
	key := scopedID{scope: scope, id: workerID}
	r.mu.Lock()
	defer r.mu.Unlock()
	current, exists := r.heartbeats[key]
	if !exists {
		if expected != nil {
			return ErrStaleFence
		}
		r.heartbeats[key] = next
		return nil
	}
	if expected == nil || current != *expected {
		return ErrStaleFence
	}
	if next.Sequence <= current.Sequence || !next.ObservedAt.After(current.ObservedAt) {
		return ErrStaleFence
	}
	r.heartbeats[key] = next
	return nil
}

func (r *MemoryRepository) GetCircuit(ctx context.Context, scope Scope, circuitID string) (*CircuitState, error) {
	if err := repositoryScopedID(ctx, scope, "circuit id", circuitID); err != nil {
		return nil, err
	}
	r.mu.RLock()
	state, ok := r.circuits[scopedID{scope: scope, id: circuitID}]
	r.mu.RUnlock()
	if !ok {
		return nil, ErrStateNotFound
	}
	if err := validateCircuitState(state); err != nil {
		return nil, fmt.Errorf("%w: circuit", ErrStateConflict)
	}
	copyState := cloneCircuit(state)
	return &copyState, nil
}

func (r *MemoryRepository) ListCircuits(ctx context.Context, scope Scope, limit int) ([]CircuitState, error) {
	if err := repositoryInput(ctx, scope); err != nil {
		return nil, err
	}
	limit, err := boundedListLimit(limit, MaxHistoryLimit)
	if err != nil {
		return nil, err
	}
	r.mu.RLock()
	result := make([]CircuitState, 0, len(r.circuits))
	for key, state := range r.circuits {
		if key.scope == scope {
			result = append(result, cloneCircuit(state))
		}
	}
	r.mu.RUnlock()
	sort.Slice(result, func(i, j int) bool { return result[i].CircuitID < result[j].CircuitID })
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (r *MemoryRepository) LatestRetry(ctx context.Context, scope Scope, workID string) (*RetryRecord, error) {
	if err := repositoryScopedID(ctx, scope, "work id", workID); err != nil {
		return nil, err
	}
	r.mu.RLock()
	history := r.retryHistory[scopedID{scope: scope, id: workID}]
	if len(history) == 0 {
		r.mu.RUnlock()
		return nil, ErrStateNotFound
	}
	record := cloneRetryRecord(history[len(history)-1])
	r.mu.RUnlock()
	return &record, nil
}

func (r *MemoryRepository) AppendRetry(ctx context.Context, expectedSequence uint64, record RetryRecord) error {
	if err := validateRetryRecord(record); err != nil {
		return err
	}
	if err := repositoryInput(ctx, record.Scope); err != nil {
		return err
	}
	key := scopedID{scope: record.Scope, id: record.WorkID}
	r.mu.Lock()
	defer r.mu.Unlock()
	history := r.retryHistory[key]
	currentSequence := uint64(0)
	if len(history) > 0 {
		currentSequence = history[len(history)-1].Sequence
	}
	if currentSequence != expectedSequence || record.Sequence != expectedSequence+1 {
		return ErrStaleFence
	}
	history = append(history, cloneRetryRecord(record))
	r.retryHistory[key] = trimRetries(history, r.historyLimit)
	return nil
}

func (r *MemoryRepository) ListRetries(ctx context.Context, scope Scope, workID string, limit int) ([]RetryRecord, error) {
	if err := repositoryScopedID(ctx, scope, "work id", workID); err != nil {
		return nil, err
	}
	limit, err := boundedListLimit(limit, r.historyLimit)
	if err != nil {
		return nil, err
	}
	r.mu.RLock()
	history := r.retryHistory[scopedID{scope: scope, id: workID}]
	start := max(0, len(history)-limit)
	result := make([]RetryRecord, len(history)-start)
	for i := range result {
		result[i] = cloneRetryRecord(history[start+i])
	}
	r.mu.RUnlock()
	return result, nil
}

func (r *MemoryRepository) ListAllRetries(ctx context.Context, scope Scope, limit int) ([]RetryRecord, error) {
	if err := repositoryInput(ctx, scope); err != nil {
		return nil, err
	}
	limit, err := boundedListLimit(limit, MaxHistoryLimit)
	if err != nil {
		return nil, err
	}
	r.mu.RLock()
	result := make([]RetryRecord, 0)
	for key, history := range r.retryHistory {
		if key.scope == scope {
			for i := range history {
				result = append(result, cloneRetryRecord(history[i]))
			}
		}
	}
	r.mu.RUnlock()
	sort.Slice(result, func(i, j int) bool {
		if result[i].RequestedAt.Equal(result[j].RequestedAt) {
			return result[i].WorkID < result[j].WorkID
		}
		return result[i].RequestedAt.After(result[j].RequestedAt)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (r *MemoryRepository) CompareAndSwapCircuit(ctx context.Context, scope Scope, circuitID string, expectedRevision uint64, next CircuitState) error {
	if err := repositoryScopedID(ctx, scope, "circuit id", circuitID); err != nil {
		return err
	}
	if err := validateCircuitState(next); err != nil {
		return err
	}
	if err := requireSameScope(scope, next.Scope); err != nil || next.CircuitID != circuitID {
		return ErrStateConflict
	}
	key := scopedID{scope: scope, id: circuitID}
	r.mu.Lock()
	defer r.mu.Unlock()
	current, exists := r.circuits[key]
	if !exists {
		if expectedRevision != 0 || next.Revision != 1 {
			return ErrStaleFence
		}
		r.circuits[key] = cloneCircuit(next)
		return nil
	}
	if current.Revision != expectedRevision || next.Revision != expectedRevision+1 {
		return ErrStaleFence
	}
	if err := validCircuitTransition(current, next); err != nil {
		return err
	}
	r.circuits[key] = cloneCircuit(next)
	return nil
}

func (r *MemoryRepository) LatestEvent(ctx context.Context, scope Scope) (*EventRecord, error) {
	if err := repositoryInput(ctx, scope); err != nil {
		return nil, err
	}
	r.mu.RLock()
	history := r.events[scope]
	if len(history) == 0 {
		r.mu.RUnlock()
		return nil, ErrStateNotFound
	}
	record := cloneEventRecord(history[len(history)-1])
	r.mu.RUnlock()
	return &record, nil
}

func (r *MemoryRepository) AppendEvent(ctx context.Context, record EventRecord) error {
	if err := repositoryInput(ctx, record.Event.Scope); err != nil {
		return err
	}
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
	r.mu.Lock()
	defer r.mu.Unlock()
	history := r.events[record.Event.Scope]
	if len(history) == 0 {
		if record.Event.Sequence != 1 || record.Event.PreviousHash != "" {
			return ErrStaleFence
		}
	} else {
		latest := history[len(history)-1]
		if record.Event.Sequence != latest.Event.Sequence+1 || record.Event.PreviousHash != latest.Hash {
			return ErrStaleFence
		}
	}
	history = append(history, cloneEventRecord(record))
	r.events[record.Event.Scope] = trimEvents(history, r.historyLimit)
	return nil
}

func (r *MemoryRepository) ListEvents(ctx context.Context, scope Scope, limit int) ([]EventRecord, error) {
	if err := repositoryInput(ctx, scope); err != nil {
		return nil, err
	}
	limit, err := boundedListLimit(limit, r.historyLimit)
	if err != nil {
		return nil, err
	}
	r.mu.RLock()
	history := r.events[scope]
	start := max(0, len(history)-limit)
	result := make([]EventRecord, len(history)-start)
	for i := range result {
		result[i] = cloneEventRecord(history[start+i])
	}
	r.mu.RUnlock()
	return result, nil
}

func (r *MemoryRepository) LatestRecovery(ctx context.Context, scope Scope, workID string) (*RecoveryRecord, error) {
	if err := repositoryScopedID(ctx, scope, "work id", workID); err != nil {
		return nil, err
	}
	r.mu.RLock()
	history := r.recoveryHistory[scopedID{scope: scope, id: workID}]
	if len(history) == 0 {
		r.mu.RUnlock()
		return nil, ErrStateNotFound
	}
	record := cloneRecoveryRecord(history[len(history)-1])
	r.mu.RUnlock()
	return &record, nil
}

func (r *MemoryRepository) AppendRecovery(ctx context.Context, expectedSequence uint64, record RecoveryRecord) error {
	if err := validateRecoveryRecord(record); err != nil {
		return err
	}
	if err := repositoryInput(ctx, record.Scope); err != nil {
		return err
	}
	key := scopedID{scope: record.Scope, id: record.WorkID}
	r.mu.Lock()
	defer r.mu.Unlock()
	history := r.recoveryHistory[key]
	currentSequence := uint64(0)
	if len(history) > 0 {
		currentSequence = history[len(history)-1].Sequence
	}
	if currentSequence != expectedSequence || record.Sequence != expectedSequence+1 {
		return ErrStaleFence
	}
	history = append(history, cloneRecoveryRecord(record))
	r.recoveryHistory[key] = trimRecoveries(history, r.historyLimit)
	return nil
}

func (r *MemoryRepository) ListRecoveries(ctx context.Context, scope Scope, workID string, limit int) ([]RecoveryRecord, error) {
	if err := repositoryScopedID(ctx, scope, "work id", workID); err != nil {
		return nil, err
	}
	limit, err := boundedListLimit(limit, r.historyLimit)
	if err != nil {
		return nil, err
	}
	r.mu.RLock()
	history := r.recoveryHistory[scopedID{scope: scope, id: workID}]
	start := max(0, len(history)-limit)
	result := make([]RecoveryRecord, len(history)-start)
	for i := range result {
		result[i] = cloneRecoveryRecord(history[start+i])
	}
	r.mu.RUnlock()
	return result, nil
}

func (r *MemoryRepository) ListAllRecoveries(ctx context.Context, scope Scope, limit int) ([]RecoveryRecord, error) {
	if err := repositoryInput(ctx, scope); err != nil {
		return nil, err
	}
	limit, err := boundedListLimit(limit, MaxHistoryLimit)
	if err != nil {
		return nil, err
	}
	r.mu.RLock()
	result := make([]RecoveryRecord, 0)
	for key, history := range r.recoveryHistory {
		if key.scope == scope {
			for i := range history {
				result = append(result, cloneRecoveryRecord(history[i]))
			}
		}
	}
	r.mu.RUnlock()
	sort.Slice(result, func(i, j int) bool {
		if result[i].RequestedAt.Equal(result[j].RequestedAt) {
			return result[i].WorkID < result[j].WorkID
		}
		return result[i].RequestedAt.After(result[j].RequestedAt)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func repositoryInput(ctx context.Context, scope Scope) error {
	if ctx == nil {
		return fmt.Errorf("resilience: context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return validateScope(scope)
}

func repositoryScopedID(ctx context.Context, scope Scope, name, id string) error {
	if err := repositoryInput(ctx, scope); err != nil {
		return err
	}
	return validateID(name, id)
}

func validLeaseTransition(current, next WorkLease) error {
	if current.Scope != next.Scope || current.WorkID != next.WorkID || current.IdempotencyKey != next.IdempotencyKey || current.PayloadHash != next.PayloadHash {
		return ErrStateConflict
	}
	if next.Generation == current.Generation+1 {
		if current.Generation == ^uint64(0) || next.State != LeaseActive || next.ReleasedAt != nil {
			return ErrStaleFence
		}
		if current.State == LeaseActive && next.AcquiredAt.Before(current.ExpiresAt) {
			return ErrStaleFence
		}
		if current.State == LeaseReleased && (current.ReleasedAt == nil || next.AcquiredAt.Before(*current.ReleasedAt)) {
			return ErrStaleFence
		}
		return nil
	}
	if next.Generation != current.Generation || next.WorkerID != current.WorkerID || next.AcquiredAt != current.AcquiredAt {
		return ErrStaleFence
	}
	if current.State == LeaseReleased {
		return ErrStaleFence
	}
	if next.State == LeaseReleased {
		if next.LastHeartbeatAt != current.LastHeartbeatAt || next.ExpiresAt != current.ExpiresAt || next.ReleasedAt == nil {
			return ErrStaleFence
		}
		return nil
	}
	if next.State != LeaseActive || !next.LastHeartbeatAt.After(current.LastHeartbeatAt) || !next.ExpiresAt.After(current.ExpiresAt) {
		return ErrStaleFence
	}
	return nil
}

func validCircuitTransition(current, next CircuitState) error {
	if current.Scope != next.Scope || current.CircuitID != next.CircuitID {
		return ErrStateConflict
	}
	switch current.Phase {
	case CircuitClosed:
		switch next.Phase {
		case CircuitClosed:
			if next.ConsecutiveFailures != 0 && next.ConsecutiveFailures != incrementFailureCount(current.ConsecutiveFailures) {
				return ErrStaleFence
			}
		case CircuitOpen:
			if next.ConsecutiveFailures != incrementFailureCount(current.ConsecutiveFailures) {
				return ErrStaleFence
			}
		default:
			return ErrStaleFence
		}
	case CircuitOpen:
		if next.Phase != CircuitHalfOpen || next.ConsecutiveFailures != current.ConsecutiveFailures || next.ProbesInFlight != current.ProbesInFlight+1 ||
			next.OpenedAt == nil || current.OpenedAt == nil || !next.OpenedAt.Equal(*current.OpenedAt) ||
			next.RetryAfter == nil || current.RetryAfter == nil || !next.RetryAfter.Equal(*current.RetryAfter) {
			return ErrStaleFence
		}
	case CircuitHalfOpen:
		switch next.Phase {
		case CircuitHalfOpen:
			if next.ConsecutiveFailures != current.ConsecutiveFailures || next.ProbesInFlight != current.ProbesInFlight+1 {
				return ErrStaleFence
			}
		case CircuitClosed:
			if next.ConsecutiveFailures != 0 || next.ProbesInFlight != 0 {
				return ErrStaleFence
			}
		case CircuitOpen:
			if next.ConsecutiveFailures != incrementFailureCount(current.ConsecutiveFailures) || next.ProbesInFlight != 0 {
				return ErrStaleFence
			}
		default:
			return ErrStaleFence
		}
	default:
		return ErrStateConflict
	}
	return nil
}

func sameLease(left, right WorkLease) bool {
	if left.ContractVersion != right.ContractVersion || left.Scope != right.Scope || left.WorkID != right.WorkID ||
		left.IdempotencyKey != right.IdempotencyKey || left.PayloadHash != right.PayloadHash || left.WorkerID != right.WorkerID ||
		left.Generation != right.Generation || left.State != right.State || left.AcquiredAt != right.AcquiredAt ||
		left.LastHeartbeatAt != right.LastHeartbeatAt || left.ExpiresAt != right.ExpiresAt {
		return false
	}
	if left.ReleasedAt == nil || right.ReleasedAt == nil {
		return left.ReleasedAt == nil && right.ReleasedAt == nil
	}
	return *left.ReleasedAt == *right.ReleasedAt
}

func validateRecoveryRecord(record RecoveryRecord) error {
	if err := validateContract(record.ContractVersion); err != nil {
		return err
	}
	if err := validateScope(record.Scope); err != nil {
		return err
	}
	if err := validateID("work id", record.WorkID); err != nil {
		return err
	}
	if record.Sequence == 0 || record.RequestedAt.IsZero() {
		return fmt.Errorf("resilience: recovery sequence and request time are required")
	}
	if err := requireSameScope(record.Scope, record.Request.Scope); err != nil || record.Request.WorkID != record.WorkID {
		return ErrStateConflict
	}
	if !isAdvisory(record.Authority) || !isAdvisory(record.Decision.Authority) {
		return ErrStateConflict
	}
	if !record.RequestedAt.Equal(record.Request.Now) {
		return ErrStateConflict
	}
	decision, err := DecideRecovery(record.Request)
	if err != nil || !sameRecoveryDecision(decision, record.Decision) {
		return ErrStateConflict
	}
	return nil
}

func sameRecoveryDecision(left, right RecoveryDecision) bool {
	if left.Action != right.Action || left.DeadLetterClass != right.DeadLetterClass || left.Reason != right.Reason || left.Authority != right.Authority {
		return false
	}
	if left.NotBefore == nil || right.NotBefore == nil {
		return left.NotBefore == nil && right.NotBefore == nil
	}
	return left.NotBefore.Equal(*right.NotBefore)
}

func validateRetryRecord(record RetryRecord) error {
	if err := validateContract(record.ContractVersion); err != nil {
		return err
	}
	if err := validateScope(record.Scope); err != nil {
		return err
	}
	if err := validateID("work id", record.WorkID); err != nil {
		return err
	}
	if record.Sequence == 0 || record.RequestedAt.IsZero() {
		return fmt.Errorf("resilience: retry sequence and request time are required")
	}
	if err := validateRetryPolicy(record.Policy); err != nil {
		return err
	}
	if err := requireSameScope(record.Scope, record.Decision.Scope); err != nil || record.Decision.WorkID != record.WorkID {
		return ErrStateConflict
	}
	if !isAdvisory(record.Authority) || !isAdvisory(record.Decision.Authority) {
		return ErrStateConflict
	}
	decision, err := DecideRetry(record.Scope, record.WorkID, record.Decision.AttemptsCompleted, record.Decision.Failure, record.Policy, record.RequestedAt)
	if err != nil || !sameRetryDecision(decision, record.Decision) {
		return ErrStateConflict
	}
	return nil
}

func sameRetryDecision(left, right RetryDecision) bool {
	if left.Scope != right.Scope || left.WorkID != right.WorkID || left.Disposition != right.Disposition ||
		left.AttemptsCompleted != right.AttemptsCompleted || left.DeadLetterClass != right.DeadLetterClass ||
		left.Failure != right.Failure || left.Reason != right.Reason || left.Authority != right.Authority {
		return false
	}
	if left.RetryAt == nil || right.RetryAt == nil {
		return left.RetryAt == nil && right.RetryAt == nil
	}
	return left.RetryAt.Equal(*right.RetryAt)
}

func isAdvisory(authority AuthorityBoundary) bool {
	return authority == advisoryBoundary()
}

func boundedListLimit(limit, storedLimit int) (int, error) {
	if limit <= 0 || limit > MaxHistoryLimit {
		return 0, fmt.Errorf("resilience: history limit is out of bounds")
	}
	if limit > storedLimit {
		return storedLimit, nil
	}
	return limit, nil
}

func trimEvents(history []EventRecord, limit int) []EventRecord {
	if len(history) <= limit {
		return history
	}
	return append([]EventRecord(nil), history[len(history)-limit:]...)
}

func trimRecoveries(history []RecoveryRecord, limit int) []RecoveryRecord {
	if len(history) <= limit {
		return history
	}
	return append([]RecoveryRecord(nil), history[len(history)-limit:]...)
}

func trimRetries(history []RetryRecord, limit int) []RetryRecord {
	if len(history) <= limit {
		return history
	}
	return append([]RetryRecord(nil), history[len(history)-limit:]...)
}

func cloneRetryRecord(record RetryRecord) RetryRecord {
	copyRecord := record
	if record.Decision.RetryAt != nil {
		retryAt := *record.Decision.RetryAt
		copyRecord.Decision.RetryAt = &retryAt
	}
	return copyRecord
}

func cloneEventRecord(record EventRecord) EventRecord {
	copyRecord := record
	if record.Event.Attributes != nil {
		copyRecord.Event.Attributes = make(map[string]string, len(record.Event.Attributes))
		for key, value := range record.Event.Attributes {
			copyRecord.Event.Attributes[key] = value
		}
	}
	return copyRecord
}

func cloneRecoveryRecord(record RecoveryRecord) RecoveryRecord {
	copyRecord := record
	copyRecord.Request = cloneRecoveryRequest(record.Request)
	copyRecord.Decision = cloneRecoveryDecision(record.Decision)
	return copyRecord
}

func cloneRecoveryRequest(request RecoveryRequest) RecoveryRequest {
	copyRequest := request
	copyRequest.Lease = cloneLease(request.Lease)
	if request.Heartbeat != nil {
		heartbeat := *request.Heartbeat
		copyRequest.Heartbeat = &heartbeat
	}
	if request.Circuit != nil {
		circuit := cloneCircuit(*request.Circuit)
		copyRequest.Circuit = &circuit
	}
	if request.Failure != nil {
		failure := *request.Failure
		copyRequest.Failure = &failure
	}
	return copyRequest
}

func cloneRecoveryDecision(decision RecoveryDecision) RecoveryDecision {
	copyDecision := decision
	if decision.NotBefore != nil {
		notBefore := *decision.NotBefore
		copyDecision.NotBefore = &notBefore
	}
	return copyDecision
}
