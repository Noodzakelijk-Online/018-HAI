package resilience

import (
	"fmt"
	"strings"
	"time"
)

// DeriveIdempotencyKey binds duplicate detection to an owner/workspace scope,
// operation, source reference, and already-computed payload digest.
func DeriveIdempotencyKey(scope Scope, operation, sourceRef, payloadHash string) (string, error) {
	if err := validateScope(scope); err != nil {
		return "", err
	}
	operation = strings.TrimSpace(operation)
	sourceRef = strings.TrimSpace(sourceRef)
	if err := validateID("operation", operation); err != nil {
		return "", err
	}
	if err := validateID("source reference", sourceRef); err != nil {
		return "", err
	}
	if err := validateHash("payload hash", payloadHash, false); err != nil {
		return "", err
	}
	return hashFields(
		"resilience-idempotency/v1",
		scope.OwnerID,
		scope.WorkspaceID,
		operation,
		sourceRef,
		payloadHash,
	), nil
}

// DecideIdempotency recommends accepting new work or treating it as a
// duplicate. The existing record must come from an exact scoped lookup.
func DecideIdempotency(work WorkDescriptor, existing *IdempotencyRecord) (IdempotencyDecision, error) {
	if err := validateWorkDescriptor(work); err != nil {
		return IdempotencyDecision{}, err
	}
	decision := IdempotencyDecision{Authority: advisoryBoundary()}
	if existing == nil {
		record := IdempotencyRecord{
			ContractVersion: ContractVersion,
			Scope:           work.Scope,
			WorkID:          work.WorkID,
			IdempotencyKey:  work.IdempotencyKey,
			PayloadHash:     work.PayloadHash,
			RecordedAt:      work.CreatedAt.UTC(),
		}
		decision.Disposition = IdempotencyAccept
		decision.CanonicalWorkID = work.WorkID
		decision.Record = &record
		decision.Reason = "no existing scoped idempotency record"
		return decision, nil
	}
	if err := validateIdempotencyRecord(*existing); err != nil {
		return IdempotencyDecision{}, err
	}
	if err := requireSameScope(work.Scope, existing.Scope); err != nil {
		return IdempotencyDecision{}, err
	}
	if work.IdempotencyKey != existing.IdempotencyKey {
		return IdempotencyDecision{}, fmt.Errorf("resilience: existing idempotency record does not match requested key")
	}
	if work.PayloadHash != existing.PayloadHash {
		return IdempotencyDecision{}, fmt.Errorf("resilience: idempotency key is already bound to a different payload")
	}
	decision.Disposition = IdempotencyDuplicate
	decision.CanonicalWorkID = existing.WorkID
	decision.Record = cloneIdempotencyRecord(existing)
	decision.Reason = "matching scoped idempotency key and payload already recorded"
	return decision, nil
}

// DecideLease recommends granting, reclaiming, or declining a durable lease.
// A reclaimed lease increments Generation to fence stale workers.
func DecideLease(request LeaseRequest, current *WorkLease) (LeaseDecision, error) {
	if err := validateLeaseRequest(request); err != nil {
		return LeaseDecision{}, err
	}
	decision := LeaseDecision{Authority: advisoryBoundary()}
	if current == nil {
		lease := newLease(request, 1)
		decision.Disposition = LeaseGrant
		decision.Lease = &lease
		decision.Reason = "no current lease"
		return decision, nil
	}
	if err := validateLease(*current); err != nil {
		return LeaseDecision{}, err
	}
	if err := requireSameScope(request.Scope, current.Scope); err != nil {
		return LeaseDecision{}, err
	}
	if request.WorkID != current.WorkID || request.IdempotencyKey != current.IdempotencyKey || request.PayloadHash != current.PayloadHash {
		return LeaseDecision{}, fmt.Errorf("resilience: current lease binding does not match requested work")
	}
	if request.Now.Before(current.AcquiredAt) {
		return LeaseDecision{}, fmt.Errorf("resilience: lease decision time predates current lease acquisition")
	}
	if current.State == LeaseActive && request.Now.Before(current.ExpiresAt) {
		decision.Lease = cloneLease(current)
		if current.WorkerID == request.WorkerID {
			decision.Disposition = LeaseDuplicate
			decision.Reason = "worker already holds the active lease; use a fenced heartbeat to renew"
		} else {
			decision.Disposition = LeaseBusy
			decision.Reason = "another worker holds the active lease"
		}
		return decision, nil
	}
	if current.Generation == ^uint64(0) {
		return LeaseDecision{}, fmt.Errorf("resilience: lease generation is exhausted")
	}
	lease := newLease(request, current.Generation+1)
	decision.Lease = &lease
	decision.Disposition = LeaseReclaim
	if current.State == LeaseReleased {
		decision.Reason = "previous lease was released"
	} else {
		decision.Reason = "previous lease expired and generation was advanced"
	}
	return decision, nil
}

// DecideLeaseHeartbeat renews only an unexpired lease whose full fencing tuple
// matches the heartbeat. Late or stale-generation heartbeats fail closed.
func DecideLeaseHeartbeat(current WorkLease, heartbeat LeaseHeartbeat) (LeaseDecision, error) {
	if err := validateLease(current); err != nil {
		return LeaseDecision{}, err
	}
	if err := validateContract(heartbeat.ContractVersion); err != nil {
		return LeaseDecision{}, err
	}
	if err := requireSameScope(current.Scope, heartbeat.Scope); err != nil {
		return LeaseDecision{}, err
	}
	if err := validateID("heartbeat work id", heartbeat.WorkID); err != nil {
		return LeaseDecision{}, err
	}
	if err := validateID("heartbeat worker id", heartbeat.WorkerID); err != nil {
		return LeaseDecision{}, err
	}
	if err := validateTime("heartbeat observation time", heartbeat.ObservedAt); err != nil {
		return LeaseDecision{}, err
	}
	if err := validateLeaseTTL(heartbeat.TTL); err != nil {
		return LeaseDecision{}, err
	}
	if current.State != LeaseActive {
		return LeaseDecision{}, fmt.Errorf("resilience: released lease cannot be renewed")
	}
	if heartbeat.WorkID != current.WorkID || heartbeat.WorkerID != current.WorkerID || heartbeat.Generation != current.Generation {
		return LeaseDecision{}, fmt.Errorf("resilience: lease heartbeat fencing tuple does not match")
	}
	if heartbeat.ObservedAt.Before(current.LastHeartbeatAt) {
		return LeaseDecision{}, fmt.Errorf("resilience: lease heartbeat is older than the durable heartbeat")
	}
	if !heartbeat.ObservedAt.Before(current.ExpiresAt) {
		return LeaseDecision{}, fmt.Errorf("resilience: lease heartbeat arrived after lease expiry")
	}
	lease := *cloneLease(&current)
	lease.LastHeartbeatAt = heartbeat.ObservedAt.UTC()
	lease.ExpiresAt = heartbeat.ObservedAt.UTC().Add(heartbeat.TTL)
	return LeaseDecision{
		Disposition: LeaseRenew,
		Lease:       &lease,
		Reason:      "matching heartbeat renewed the fenced lease",
		Authority:   advisoryBoundary(),
	}, nil
}

// DecideLeaseRelease marks an exactly fenced active lease as released.
func DecideLeaseRelease(current WorkLease, scope Scope, workerID string, generation uint64, now time.Time) (LeaseDecision, error) {
	if err := validateLease(current); err != nil {
		return LeaseDecision{}, err
	}
	if err := requireSameScope(current.Scope, scope); err != nil {
		return LeaseDecision{}, err
	}
	if err := validateID("worker id", workerID); err != nil {
		return LeaseDecision{}, err
	}
	if err := validateTime("release time", now); err != nil {
		return LeaseDecision{}, err
	}
	if current.State != LeaseActive || current.WorkerID != workerID || current.Generation != generation {
		return LeaseDecision{}, fmt.Errorf("resilience: lease release fencing tuple does not match an active lease")
	}
	if now.Before(current.AcquiredAt) || !now.Before(current.ExpiresAt) {
		return LeaseDecision{}, fmt.Errorf("resilience: release time must fall within the active lease interval")
	}
	releasedAt := now.UTC()
	lease := *cloneLease(&current)
	lease.State = LeaseReleased
	lease.ReleasedAt = &releasedAt
	return LeaseDecision{
		Disposition: LeaseRelease,
		Lease:       &lease,
		Reason:      "matching fenced lease may be persisted as released",
		Authority:   advisoryBoundary(),
	}, nil
}

// DecideWorkerHeartbeat accepts only monotonically increasing heartbeat
// sequences for the same worker and exact owner/workspace scope.
func DecideWorkerHeartbeat(next WorkerHeartbeat, current *WorkerHeartbeat) (WorkerHeartbeat, error) {
	if err := validateWorkerHeartbeat(next); err != nil {
		return WorkerHeartbeat{}, err
	}
	next.ObservedAt = next.ObservedAt.UTC()
	if current == nil {
		return next, nil
	}
	if err := validateWorkerHeartbeat(*current); err != nil {
		return WorkerHeartbeat{}, err
	}
	if err := requireSameScope(current.Scope, next.Scope); err != nil {
		return WorkerHeartbeat{}, err
	}
	if current.WorkerID != next.WorkerID {
		return WorkerHeartbeat{}, fmt.Errorf("resilience: heartbeat worker does not match durable worker")
	}
	if next.Sequence <= current.Sequence || !next.ObservedAt.After(current.ObservedAt) {
		return WorkerHeartbeat{}, fmt.Errorf("resilience: worker heartbeat is stale or replayed")
	}
	return next, nil
}

// AssessHeartbeat classifies worker liveness without contacting or directing
// the worker.
func AssessHeartbeat(scope Scope, heartbeat *WorkerHeartbeat, now time.Time, maxAge time.Duration) (HeartbeatDecision, error) {
	if err := validateScope(scope); err != nil {
		return HeartbeatDecision{}, err
	}
	if err := validateTime("assessment time", now); err != nil {
		return HeartbeatDecision{}, err
	}
	if err := validateHeartbeatAge(maxAge); err != nil {
		return HeartbeatDecision{}, err
	}
	decision := HeartbeatDecision{Authority: advisoryBoundary()}
	if heartbeat == nil {
		decision.Status = HeartbeatMissing
		decision.Reason = "no durable worker heartbeat"
		return decision, nil
	}
	if err := validateWorkerHeartbeat(*heartbeat); err != nil {
		return HeartbeatDecision{}, err
	}
	if err := requireSameScope(scope, heartbeat.Scope); err != nil {
		return HeartbeatDecision{}, err
	}
	if heartbeat.ObservedAt.After(now) {
		return HeartbeatDecision{}, fmt.Errorf("resilience: heartbeat observation is in the future")
	}
	decision.Age = now.Sub(heartbeat.ObservedAt)
	if decision.Age > maxAge {
		decision.Status = HeartbeatStale
		decision.Reason = "durable worker heartbeat exceeded maximum age"
	} else {
		decision.Status = HeartbeatHealthy
		decision.Reason = "durable worker heartbeat is within maximum age"
	}
	return decision, nil
}

func validateWorkDescriptor(work WorkDescriptor) error {
	if err := validateContract(work.ContractVersion); err != nil {
		return err
	}
	if err := validateScope(work.Scope); err != nil {
		return err
	}
	if err := validateID("work id", work.WorkID); err != nil {
		return err
	}
	if err := validateHash("idempotency key", work.IdempotencyKey, false); err != nil {
		return err
	}
	if err := validateHash("payload hash", work.PayloadHash, false); err != nil {
		return err
	}
	return validateTime("work creation time", work.CreatedAt)
}

func validateIdempotencyRecord(record IdempotencyRecord) error {
	work := WorkDescriptor{
		ContractVersion: record.ContractVersion,
		Scope:           record.Scope,
		WorkID:          record.WorkID,
		IdempotencyKey:  record.IdempotencyKey,
		PayloadHash:     record.PayloadHash,
		CreatedAt:       record.RecordedAt,
	}
	return validateWorkDescriptor(work)
}

func validateLeaseRequest(request LeaseRequest) error {
	work := WorkDescriptor{
		ContractVersion: request.ContractVersion,
		Scope:           request.Scope,
		WorkID:          request.WorkID,
		IdempotencyKey:  request.IdempotencyKey,
		PayloadHash:     request.PayloadHash,
		CreatedAt:       request.Now,
	}
	if err := validateWorkDescriptor(work); err != nil {
		return err
	}
	if err := validateID("worker id", request.WorkerID); err != nil {
		return err
	}
	return validateLeaseTTL(request.TTL)
}

func validateLease(lease WorkLease) error {
	if err := validateContract(lease.ContractVersion); err != nil {
		return err
	}
	if err := validateScope(lease.Scope); err != nil {
		return err
	}
	if err := validateID("work id", lease.WorkID); err != nil {
		return err
	}
	if err := validateHash("idempotency key", lease.IdempotencyKey, false); err != nil {
		return err
	}
	if err := validateHash("payload hash", lease.PayloadHash, false); err != nil {
		return err
	}
	if err := validateID("worker id", lease.WorkerID); err != nil {
		return err
	}
	if lease.Generation == 0 {
		return fmt.Errorf("resilience: lease generation must be positive")
	}
	if lease.State != LeaseActive && lease.State != LeaseReleased {
		return fmt.Errorf("resilience: lease state is unsupported")
	}
	if lease.AcquiredAt.IsZero() || lease.LastHeartbeatAt.Before(lease.AcquiredAt) || !lease.ExpiresAt.After(lease.LastHeartbeatAt) {
		return fmt.Errorf("resilience: lease timestamps are inconsistent")
	}
	if lease.ExpiresAt.Sub(lease.LastHeartbeatAt) > maxLeaseTTL {
		return fmt.Errorf("resilience: durable lease ttl exceeds maximum")
	}
	if lease.State == LeaseReleased {
		if lease.ReleasedAt == nil || lease.ReleasedAt.Before(lease.AcquiredAt) || !lease.ReleasedAt.Before(lease.ExpiresAt) {
			return fmt.Errorf("resilience: released lease requires a valid release time")
		}
	} else if lease.ReleasedAt != nil {
		return fmt.Errorf("resilience: active lease cannot have a release time")
	}
	return nil
}

func validateWorkerHeartbeat(heartbeat WorkerHeartbeat) error {
	if err := validateContract(heartbeat.ContractVersion); err != nil {
		return err
	}
	if err := validateScope(heartbeat.Scope); err != nil {
		return err
	}
	if err := validateID("worker id", heartbeat.WorkerID); err != nil {
		return err
	}
	if heartbeat.Sequence == 0 {
		return fmt.Errorf("resilience: heartbeat sequence must be positive")
	}
	return validateTime("heartbeat observation time", heartbeat.ObservedAt)
}

func newLease(request LeaseRequest, generation uint64) WorkLease {
	now := request.Now.UTC()
	return WorkLease{
		ContractVersion: ContractVersion,
		Scope:           request.Scope,
		WorkID:          request.WorkID,
		IdempotencyKey:  request.IdempotencyKey,
		PayloadHash:     request.PayloadHash,
		WorkerID:        request.WorkerID,
		Generation:      generation,
		State:           LeaseActive,
		AcquiredAt:      now,
		LastHeartbeatAt: now,
		ExpiresAt:       now.Add(request.TTL),
	}
}

func cloneLease(lease *WorkLease) *WorkLease {
	if lease == nil {
		return nil
	}
	copyLease := *lease
	if lease.ReleasedAt != nil {
		releasedAt := *lease.ReleasedAt
		copyLease.ReleasedAt = &releasedAt
	}
	return &copyLease
}

func cloneIdempotencyRecord(record *IdempotencyRecord) *IdempotencyRecord {
	if record == nil {
		return nil
	}
	copyRecord := *record
	return &copyRecord
}
