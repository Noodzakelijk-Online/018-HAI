package resilience

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"
)

var ErrRepositoryUnavailable = errors.New("resilience repository unavailable")

type Service struct {
	repository Repository
	clock      func() time.Time
}

type Status struct {
	ContractVersion int               `json:"contractVersion"`
	Scope           Scope             `json:"scope"`
	GeneratedAt     time.Time         `json:"generatedAt"`
	LeaseCount      int               `json:"leaseCount"`
	WorkerCount     int               `json:"workerCount"`
	RetryCount      int               `json:"retryCount"`
	CircuitCount    int               `json:"circuitCount"`
	RecoveryCount   int               `json:"recoveryCount"`
	LatestEvent     *EventRecord      `json:"latestEvent,omitempty"`
	Authority       AuthorityBoundary `json:"authority"`
}

type LeaseAcquireInput struct {
	WorkID         string
	WorkerID       string
	IdempotencyKey string
	PayloadHash    string
	TTL            time.Duration
}

type WorkRegistrationInput struct {
	WorkID      string
	Operation   string
	SourceRef   string
	PayloadHash string
}

type LeaseAdvisory struct {
	Idempotency IdempotencyDecision `json:"idempotency"`
	Lease       *LeaseDecision      `json:"lease,omitempty"`
	Authority   AuthorityBoundary   `json:"authority"`
}

type LeaseHeartbeatInput struct {
	WorkID     string
	WorkerID   string
	Generation uint64
	TTL        time.Duration
}

type LeaseReleaseInput struct {
	WorkID     string
	WorkerID   string
	Generation uint64
}

type WorkerHeartbeatInput struct {
	WorkerID string
	Sequence uint64
}

type RetryAdvisoryInput struct {
	WorkID            string
	AttemptsCompleted uint32
	FailureCode       string
	FailureClass      FailureClass
	FailureMessage    string
	Policy            RetryPolicy
}

type CircuitBeforeInput struct {
	CircuitID string
	Policy    CircuitPolicy
}

type CircuitObservationInput struct {
	CircuitID string
	Outcome   AttemptOutcome
	Policy    CircuitPolicy
}

type RecoveryAdvisoryInput struct {
	WorkID            string
	WorkerID          string
	CircuitID         string
	HeartbeatMaxAge   time.Duration
	AttemptsCompleted uint32
	FailureCode       string
	FailureClass      FailureClass
	FailureMessage    string
	RetryPolicy       RetryPolicy
}

func NewService(repository Repository, clocks ...func() time.Time) *Service {
	clock := time.Now
	if len(clocks) > 0 && clocks[0] != nil {
		clock = clocks[0]
	}
	return &Service{repository: repository, clock: clock}
}

func newServiceWithClock(repository Repository, clock func() time.Time) *Service {
	service := NewService(repository)
	if clock != nil {
		service.clock = clock
	}
	return service
}

func (s *Service) scope(ownerID, workspaceID string) (Scope, error) {
	if s == nil || s.repository == nil || s.clock == nil {
		return Scope{}, ErrRepositoryUnavailable
	}
	scope := Scope{OwnerID: ownerID, WorkspaceID: workspaceID}
	if err := validateScope(scope); err != nil {
		return Scope{}, err
	}
	return scope, nil
}

func (s *Service) Status(ctx context.Context, ownerID, workspaceID string) (Status, error) {
	scope, err := s.scope(ownerID, workspaceID)
	if err != nil {
		return Status{}, err
	}
	leases, err := s.repository.ListLeases(ctx, scope, MaxHistoryLimit)
	if err != nil {
		return Status{}, err
	}
	workers, err := s.repository.ListWorkerHeartbeats(ctx, scope, MaxHistoryLimit)
	if err != nil {
		return Status{}, err
	}
	retries, err := s.repository.ListAllRetries(ctx, scope, MaxHistoryLimit)
	if err != nil {
		return Status{}, err
	}
	circuits, err := s.repository.ListCircuits(ctx, scope, MaxHistoryLimit)
	if err != nil {
		return Status{}, err
	}
	recoveries, err := s.repository.ListAllRecoveries(ctx, scope, MaxHistoryLimit)
	if err != nil {
		return Status{}, err
	}
	latest, err := s.repository.LatestEvent(ctx, scope)
	if errors.Is(err, ErrStateNotFound) {
		latest = nil
	} else if err != nil {
		return Status{}, err
	}
	return Status{
		ContractVersion: ContractVersion, Scope: scope, GeneratedAt: s.now(),
		LeaseCount: len(leases), WorkerCount: len(workers), RetryCount: len(retries),
		CircuitCount: len(circuits), RecoveryCount: len(recoveries), LatestEvent: latest,
		Authority: advisoryBoundary(),
	}, nil
}

func (s *Service) ListLeases(ctx context.Context, ownerID, workspaceID string, limit int) ([]WorkLease, error) {
	scope, err := s.scope(ownerID, workspaceID)
	if err != nil {
		return nil, err
	}
	return s.repository.ListLeases(ctx, scope, limit)
}

func (s *Service) GetLease(ctx context.Context, ownerID, workspaceID, workID string) (*WorkLease, error) {
	scope, err := s.scope(ownerID, workspaceID)
	if err != nil {
		return nil, err
	}
	return s.repository.GetLease(ctx, scope, workID)
}

func (s *Service) ListWorkers(ctx context.Context, ownerID, workspaceID string, limit int) ([]WorkerHeartbeat, error) {
	scope, err := s.scope(ownerID, workspaceID)
	if err != nil {
		return nil, err
	}
	return s.repository.ListWorkerHeartbeats(ctx, scope, limit)
}

func (s *Service) GetWorker(ctx context.Context, ownerID, workspaceID, workerID string) (*WorkerHeartbeat, error) {
	scope, err := s.scope(ownerID, workspaceID)
	if err != nil {
		return nil, err
	}
	return s.repository.GetWorkerHeartbeat(ctx, scope, workerID)
}

func (s *Service) ListRetries(ctx context.Context, ownerID, workspaceID, workID string, limit int) ([]RetryRecord, error) {
	scope, err := s.scope(ownerID, workspaceID)
	if err != nil {
		return nil, err
	}
	if workID == "" {
		return s.repository.ListAllRetries(ctx, scope, limit)
	}
	return s.repository.ListRetries(ctx, scope, workID, limit)
}

func (s *Service) GetRetry(ctx context.Context, ownerID, workspaceID, workID string) (*RetryRecord, error) {
	scope, err := s.scope(ownerID, workspaceID)
	if err != nil {
		return nil, err
	}
	return s.repository.LatestRetry(ctx, scope, workID)
}

func (s *Service) ListCircuits(ctx context.Context, ownerID, workspaceID string, limit int) ([]CircuitState, error) {
	scope, err := s.scope(ownerID, workspaceID)
	if err != nil {
		return nil, err
	}
	return s.repository.ListCircuits(ctx, scope, limit)
}

func (s *Service) GetCircuit(ctx context.Context, ownerID, workspaceID, circuitID string) (*CircuitState, error) {
	scope, err := s.scope(ownerID, workspaceID)
	if err != nil {
		return nil, err
	}
	return s.repository.GetCircuit(ctx, scope, circuitID)
}

func (s *Service) ListRecoveries(ctx context.Context, ownerID, workspaceID, workID string, limit int) ([]RecoveryRecord, error) {
	scope, err := s.scope(ownerID, workspaceID)
	if err != nil {
		return nil, err
	}
	if workID == "" {
		return s.repository.ListAllRecoveries(ctx, scope, limit)
	}
	return s.repository.ListRecoveries(ctx, scope, workID, limit)
}

func (s *Service) GetRecovery(ctx context.Context, ownerID, workspaceID, workID string) (*RecoveryRecord, error) {
	scope, err := s.scope(ownerID, workspaceID)
	if err != nil {
		return nil, err
	}
	return s.repository.LatestRecovery(ctx, scope, workID)
}

func (s *Service) ListEvents(ctx context.Context, ownerID, workspaceID string, limit int) ([]EventRecord, error) {
	scope, err := s.scope(ownerID, workspaceID)
	if err != nil {
		return nil, err
	}
	return s.repository.ListEvents(ctx, scope, limit)
}

func (s *Service) RegisterWork(ctx context.Context, ownerID, workspaceID string, input WorkRegistrationInput) (IdempotencyDecision, error) {
	scope, err := s.scope(ownerID, workspaceID)
	if err != nil {
		return IdempotencyDecision{}, err
	}
	key, err := DeriveIdempotencyKey(scope, input.Operation, input.SourceRef, input.PayloadHash)
	if err != nil {
		return IdempotencyDecision{}, err
	}
	now := s.now()
	existing, err := s.repository.LookupIdempotency(ctx, scope, key)
	if errors.Is(err, ErrStateNotFound) {
		existing, err = nil, nil
	}
	if err != nil {
		return IdempotencyDecision{}, err
	}
	work := WorkDescriptor{ContractVersion: ContractVersion, Scope: scope, WorkID: input.WorkID, IdempotencyKey: key, PayloadHash: input.PayloadHash, CreatedAt: now}
	decision, err := DecideIdempotency(work, existing)
	if err != nil {
		return IdempotencyDecision{}, err
	}
	if decision.Disposition == IdempotencyAccept {
		persisted, created, createErr := s.repository.CreateIdempotency(ctx, *decision.Record)
		if createErr != nil {
			return IdempotencyDecision{}, createErr
		}
		if !created {
			decision, err = DecideIdempotency(work, persisted)
			if err != nil {
				return IdempotencyDecision{}, err
			}
		} else {
			decision.Record = persisted
			if err := s.appendEvent(ctx, scope, "work.registered", input.WorkID, map[string]string{"operation": input.Operation, "sourceRef": input.SourceRef}); err != nil {
				return IdempotencyDecision{}, err
			}
		}
	}
	return decision, nil
}

func (s *Service) AcquireLease(ctx context.Context, ownerID, workspaceID string, input LeaseAcquireInput) (LeaseAdvisory, error) {
	scope, err := s.scope(ownerID, workspaceID)
	if err != nil {
		return LeaseAdvisory{}, err
	}
	now := s.now()
	request := LeaseRequest{ContractVersion: ContractVersion, Scope: scope, WorkID: input.WorkID, IdempotencyKey: input.IdempotencyKey, PayloadHash: input.PayloadHash, WorkerID: input.WorkerID, Now: now, TTL: input.TTL}
	if err := validateLeaseRequest(request); err != nil {
		return LeaseAdvisory{}, err
	}

	existingID, err := s.repository.LookupIdempotency(ctx, scope, input.IdempotencyKey)
	if errors.Is(err, ErrStateNotFound) {
		existingID, err = nil, nil
	}
	if err != nil {
		return LeaseAdvisory{}, err
	}
	work := WorkDescriptor{ContractVersion: ContractVersion, Scope: scope, WorkID: input.WorkID, IdempotencyKey: input.IdempotencyKey, PayloadHash: input.PayloadHash, CreatedAt: now}
	idDecision, err := DecideIdempotency(work, existingID)
	if err != nil {
		return LeaseAdvisory{}, err
	}
	if idDecision.Disposition == IdempotencyAccept {
		persisted, created, createErr := s.repository.CreateIdempotency(ctx, *idDecision.Record)
		if createErr != nil {
			return LeaseAdvisory{}, createErr
		}
		if !created {
			idDecision, err = DecideIdempotency(work, persisted)
			if err != nil {
				return LeaseAdvisory{}, err
			}
		} else {
			idDecision.Record = persisted
		}
	}
	result := LeaseAdvisory{Idempotency: idDecision, Authority: advisoryBoundary()}
	if idDecision.CanonicalWorkID != input.WorkID {
		return result, nil
	}
	for attempt := 0; attempt < 32; attempt++ {
		current, getErr := s.repository.GetLease(ctx, scope, input.WorkID)
		if errors.Is(getErr, ErrStateNotFound) {
			current, getErr = nil, nil
		}
		if getErr != nil {
			return LeaseAdvisory{}, getErr
		}
		decision, decideErr := DecideLease(request, current)
		if decideErr != nil {
			return LeaseAdvisory{}, decideErr
		}
		result.Lease = &decision
		if decision.Disposition == LeaseGrant || decision.Disposition == LeaseReclaim {
			if decision.Lease == nil {
				return LeaseAdvisory{}, ErrStateConflict
			}
			if swapErr := s.repository.CompareAndSwapLease(ctx, scope, input.WorkID, current, *decision.Lease); errors.Is(swapErr, ErrStaleFence) {
				continue
			} else if swapErr != nil {
				return LeaseAdvisory{}, swapErr
			}
		}
		if err := s.appendEvent(ctx, scope, "lease.advised", input.WorkID, map[string]string{"disposition": string(decision.Disposition), "workerId": input.WorkerID}); err != nil {
			return LeaseAdvisory{}, err
		}
		return result, nil
	}
	return LeaseAdvisory{}, fmt.Errorf("%w: lease compare-and-set contention", ErrStateConflict)
}

func (s *Service) HeartbeatLease(ctx context.Context, ownerID, workspaceID string, input LeaseHeartbeatInput) (LeaseDecision, error) {
	scope, err := s.scope(ownerID, workspaceID)
	if err != nil {
		return LeaseDecision{}, err
	}
	current, err := s.repository.GetLease(ctx, scope, input.WorkID)
	if err != nil {
		return LeaseDecision{}, err
	}
	decision, err := DecideLeaseHeartbeat(*current, LeaseHeartbeat{ContractVersion: ContractVersion, Scope: scope, WorkID: input.WorkID, WorkerID: input.WorkerID, Generation: input.Generation, ObservedAt: s.now(), TTL: input.TTL})
	if err != nil {
		return LeaseDecision{}, err
	}
	if decision.Lease == nil {
		return LeaseDecision{}, ErrStateConflict
	}
	if err := s.repository.CompareAndSwapLease(ctx, scope, input.WorkID, current, *decision.Lease); err != nil {
		return LeaseDecision{}, err
	}
	if err := s.appendEvent(ctx, scope, "lease.heartbeat_advised", input.WorkID, map[string]string{"generation": strconv.FormatUint(input.Generation, 10)}); err != nil {
		return LeaseDecision{}, err
	}
	return decision, nil
}

func (s *Service) ReleaseLease(ctx context.Context, ownerID, workspaceID string, input LeaseReleaseInput) (LeaseDecision, error) {
	scope, err := s.scope(ownerID, workspaceID)
	if err != nil {
		return LeaseDecision{}, err
	}
	current, err := s.repository.GetLease(ctx, scope, input.WorkID)
	if err != nil {
		return LeaseDecision{}, err
	}
	decision, err := DecideLeaseRelease(*current, scope, input.WorkerID, input.Generation, s.now())
	if err != nil {
		return LeaseDecision{}, err
	}
	if decision.Lease == nil {
		return LeaseDecision{}, ErrStateConflict
	}
	if err := s.repository.CompareAndSwapLease(ctx, scope, input.WorkID, current, *decision.Lease); err != nil {
		return LeaseDecision{}, err
	}
	if err := s.appendEvent(ctx, scope, "lease.release_advised", input.WorkID, map[string]string{"generation": strconv.FormatUint(input.Generation, 10)}); err != nil {
		return LeaseDecision{}, err
	}
	return decision, nil
}

func (s *Service) RecordWorkerHeartbeat(ctx context.Context, ownerID, workspaceID string, input WorkerHeartbeatInput) (WorkerHeartbeat, error) {
	scope, err := s.scope(ownerID, workspaceID)
	if err != nil {
		return WorkerHeartbeat{}, err
	}
	current, err := s.repository.GetWorkerHeartbeat(ctx, scope, input.WorkerID)
	if errors.Is(err, ErrStateNotFound) {
		current, err = nil, nil
	}
	if err != nil {
		return WorkerHeartbeat{}, err
	}
	next, err := DecideWorkerHeartbeat(WorkerHeartbeat{ContractVersion: ContractVersion, Scope: scope, WorkerID: input.WorkerID, Sequence: input.Sequence, ObservedAt: s.now()}, current)
	if err != nil {
		return WorkerHeartbeat{}, err
	}
	if err := s.repository.CompareAndSwapWorkerHeartbeat(ctx, scope, input.WorkerID, current, next); err != nil {
		return WorkerHeartbeat{}, err
	}
	if err := s.appendEvent(ctx, scope, "worker.heartbeat_observed", input.WorkerID, map[string]string{"sequence": strconv.FormatUint(input.Sequence, 10)}); err != nil {
		return WorkerHeartbeat{}, err
	}
	return next, nil
}

func (s *Service) AdviseRetry(ctx context.Context, ownerID, workspaceID string, input RetryAdvisoryInput) (RetryRecord, error) {
	scope, err := s.scope(ownerID, workspaceID)
	if err != nil {
		return RetryRecord{}, err
	}
	failure, err := NewFailure(input.FailureCode, input.FailureClass, input.FailureMessage)
	if err != nil {
		return RetryRecord{}, err
	}
	now := s.now()
	decision, err := DecideRetry(scope, input.WorkID, input.AttemptsCompleted, failure, input.Policy, now)
	if err != nil {
		return RetryRecord{}, err
	}
	latest, err := s.repository.LatestRetry(ctx, scope, input.WorkID)
	if errors.Is(err, ErrStateNotFound) {
		latest, err = nil, nil
	}
	if err != nil {
		return RetryRecord{}, err
	}
	sequence := uint64(1)
	if latest != nil {
		sequence = latest.Sequence + 1
	}
	record := RetryRecord{ContractVersion: ContractVersion, Scope: scope, WorkID: input.WorkID, Sequence: sequence, RequestedAt: now, Policy: input.Policy, Decision: decision, Authority: advisoryBoundary()}
	if err := s.repository.AppendRetry(ctx, sequence-1, record); err != nil {
		return RetryRecord{}, err
	}
	if err := s.appendEvent(ctx, scope, "retry.advised", input.WorkID, map[string]string{"disposition": string(decision.Disposition), "attemptsCompleted": strconv.FormatUint(uint64(input.AttemptsCompleted), 10)}); err != nil {
		return RetryRecord{}, err
	}
	return record, nil
}

func (s *Service) BeforeCircuit(ctx context.Context, ownerID, workspaceID string, input CircuitBeforeInput) (CircuitDecision, error) {
	scope, err := s.scope(ownerID, workspaceID)
	if err != nil {
		return CircuitDecision{}, err
	}
	current, err := s.ensureCircuit(ctx, scope, input.CircuitID)
	if err != nil {
		return CircuitDecision{}, err
	}
	decision, err := BeforeCircuitAttempt(scope, *current, s.now(), input.Policy)
	if err != nil {
		return CircuitDecision{}, err
	}
	if decision.State.Revision != current.Revision {
		if err := s.repository.CompareAndSwapCircuit(ctx, scope, input.CircuitID, current.Revision, decision.State); err != nil {
			return CircuitDecision{}, err
		}
	}
	if err := s.appendEvent(ctx, scope, "circuit.before_attempt_advised", input.CircuitID, map[string]string{"recommendation": string(decision.Recommendation)}); err != nil {
		return CircuitDecision{}, err
	}
	return decision, nil
}

func (s *Service) ObserveCircuit(ctx context.Context, ownerID, workspaceID string, input CircuitObservationInput) (CircuitDecision, error) {
	scope, err := s.scope(ownerID, workspaceID)
	if err != nil {
		return CircuitDecision{}, err
	}
	current, err := s.ensureCircuit(ctx, scope, input.CircuitID)
	if err != nil {
		return CircuitDecision{}, err
	}
	decision, err := AfterCircuitAttempt(scope, *current, input.Outcome, s.now(), input.Policy)
	if err != nil {
		return CircuitDecision{}, err
	}
	if err := s.repository.CompareAndSwapCircuit(ctx, scope, input.CircuitID, current.Revision, decision.State); err != nil {
		return CircuitDecision{}, err
	}
	if err := s.appendEvent(ctx, scope, "circuit.observation_advised", input.CircuitID, map[string]string{"outcome": string(input.Outcome), "recommendation": string(decision.Recommendation)}); err != nil {
		return CircuitDecision{}, err
	}
	return decision, nil
}

func (s *Service) AdviseRecovery(ctx context.Context, ownerID, workspaceID string, input RecoveryAdvisoryInput) (RecoveryRecord, error) {
	scope, err := s.scope(ownerID, workspaceID)
	if err != nil {
		return RecoveryRecord{}, err
	}
	now := s.now()
	request := RecoveryRequest{ContractVersion: ContractVersion, Scope: scope, WorkID: input.WorkID, Now: now, HeartbeatMaxAge: input.HeartbeatMaxAge, AttemptsCompleted: input.AttemptsCompleted, RetryPolicy: input.RetryPolicy}
	if lease, getErr := s.repository.GetLease(ctx, scope, input.WorkID); getErr == nil {
		request.Lease = lease
	} else if !errors.Is(getErr, ErrStateNotFound) {
		return RecoveryRecord{}, getErr
	}
	if input.WorkerID != "" {
		heartbeat, getErr := s.repository.GetWorkerHeartbeat(ctx, scope, input.WorkerID)
		if getErr != nil {
			return RecoveryRecord{}, getErr
		}
		request.Heartbeat = heartbeat
	}
	if input.CircuitID != "" {
		circuit, getErr := s.repository.GetCircuit(ctx, scope, input.CircuitID)
		if getErr != nil {
			return RecoveryRecord{}, getErr
		}
		request.Circuit = circuit
	}
	if input.FailureCode != "" || input.FailureMessage != "" || input.FailureClass != "" {
		failure, failureErr := NewFailure(input.FailureCode, input.FailureClass, input.FailureMessage)
		if failureErr != nil {
			return RecoveryRecord{}, failureErr
		}
		request.Failure = &failure
	}
	decision, err := DecideRecovery(request)
	if err != nil {
		return RecoveryRecord{}, err
	}
	latest, err := s.repository.LatestRecovery(ctx, scope, input.WorkID)
	if errors.Is(err, ErrStateNotFound) {
		latest, err = nil, nil
	}
	if err != nil {
		return RecoveryRecord{}, err
	}
	sequence := uint64(1)
	if latest != nil {
		sequence = latest.Sequence + 1
	}
	record := RecoveryRecord{ContractVersion: ContractVersion, Scope: scope, WorkID: input.WorkID, Sequence: sequence, RequestedAt: now, Request: request, Decision: decision, Authority: advisoryBoundary()}
	if err := s.repository.AppendRecovery(ctx, sequence-1, record); err != nil {
		return RecoveryRecord{}, err
	}
	if err := s.appendEvent(ctx, scope, "recovery.advised", input.WorkID, map[string]string{"action": string(decision.Action)}); err != nil {
		return RecoveryRecord{}, err
	}
	return record, nil
}

func (s *Service) ensureCircuit(ctx context.Context, scope Scope, circuitID string) (*CircuitState, error) {
	current, err := s.repository.GetCircuit(ctx, scope, circuitID)
	if err == nil {
		return current, nil
	}
	if !errors.Is(err, ErrStateNotFound) {
		return nil, err
	}
	initial, err := NewCircuitState(scope, circuitID)
	if err != nil {
		return nil, err
	}
	if err := s.repository.CompareAndSwapCircuit(ctx, scope, circuitID, 0, initial); err != nil {
		if !errors.Is(err, ErrStaleFence) {
			return nil, err
		}
		return s.repository.GetCircuit(ctx, scope, circuitID)
	}
	return &initial, nil
}

func (s *Service) appendEvent(ctx context.Context, scope Scope, eventType, subjectID string, attributes map[string]string) error {
	for attempt := 0; attempt < 32; attempt++ {
		latest, err := s.repository.LatestEvent(ctx, scope)
		if errors.Is(err, ErrStateNotFound) {
			latest, err = nil, nil
		}
		if err != nil {
			return err
		}
		sequence := uint64(1)
		previousHash := ""
		if latest != nil {
			sequence, previousHash = latest.Event.Sequence+1, latest.Hash
		}
		event := ControlEvent{ContractVersion: ContractVersion, Scope: scope, Type: eventType, SubjectID: subjectID, OccurredAt: s.now(), Sequence: sequence, PreviousHash: previousHash, Attributes: attributes}
		hash, err := EventHash(event)
		if err != nil {
			return err
		}
		err = s.repository.AppendEvent(ctx, EventRecord{Event: event, Hash: hash, Authority: advisoryBoundary()})
		if err == nil {
			return nil
		}
		if !errors.Is(err, ErrStaleFence) {
			return err
		}
	}
	return fmt.Errorf("%w: event append contention", ErrStateConflict)
}

func (s *Service) now() time.Time { return s.clock().UTC() }
