package ambientmonitor

import (
	"context"
	"fmt"
	"sort"
	"time"
)

func scopedCompositionKey(owner, workspace, deliveryID string) string {
	return scopeKey(owner, workspace) + "\x00" + deliveryID
}

func (r *MemoryRepository) GetCompositionByRun(ctx context.Context, owner, workspace, runID string) (CompositionDelivery, error) {
	if err := checkContext(ctx); err != nil {
		return CompositionDelivery{}, err
	}
	if r == nil {
		return CompositionDelivery{}, ErrRepositoryUnavailable
	}
	if err := validateRepositoryScope(owner, workspace); err != nil {
		return CompositionDelivery{}, err
	}
	if err := validateIdentifier("run id", runID); err != nil {
		return CompositionDelivery{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, item := range r.compositions {
		if item.Scope == (Scope{OwnerID: owner, WorkspaceID: workspace}) && item.RunID == runID {
			return validateCompositionDelivery(item)
		}
	}
	return CompositionDelivery{}, ErrNotFound
}

func (r *MemoryRepository) GetComposition(ctx context.Context, owner, workspace, deliveryID string) (CompositionDelivery, error) {
	if err := checkContext(ctx); err != nil {
		return CompositionDelivery{}, err
	}
	if r == nil {
		return CompositionDelivery{}, ErrRepositoryUnavailable
	}
	if err := validateRepositoryScope(owner, workspace); err != nil {
		return CompositionDelivery{}, err
	}
	if err := validateIdentifier("composition id", deliveryID); err != nil {
		return CompositionDelivery{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, found := r.compositions[scopedCompositionKey(owner, workspace, deliveryID)]
	if !found {
		return CompositionDelivery{}, ErrNotFound
	}
	return validateCompositionDelivery(item)
}

func (r *MemoryRepository) LoadCompositionSignal(ctx context.Context, owner, workspace, deliveryID string) (AdvisorySignal, error) {
	if err := checkContext(ctx); err != nil {
		return AdvisorySignal{}, err
	}
	if r == nil {
		return AdvisorySignal{}, ErrRepositoryUnavailable
	}
	if err := validateRepositoryScope(owner, workspace); err != nil {
		return AdvisorySignal{}, err
	}
	if err := validateIdentifier("composition id", deliveryID); err != nil {
		return AdvisorySignal{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	delivery, found := r.compositions[scopedCompositionKey(owner, workspace, deliveryID)]
	if !found {
		return AdvisorySignal{}, ErrNotFound
	}
	storageKey := scopedTargetKey(owner, workspace, delivery.TargetID)
	var run MonitorRun
	for _, item := range r.runs[storageKey] {
		if item.ID == delivery.RunID {
			run = item
			break
		}
	}
	var observation ObservationRecord
	for _, item := range r.observations[storageKey] {
		if item.ID == delivery.ObservationID {
			observation = item
			break
		}
	}
	if run.ID == "" || observation.ID == "" || run.RecordDigest != delivery.RunDigest || observation.RecordDigest != delivery.ObservationDigest {
		return AdvisorySignal{}, ErrCorruptStorage
	}
	return AdvisorySignal{Observation: observation, Run: run, Snapshot: delivery.Snapshot, Authority: advisoryAuthority()}, nil
}

func (r *MemoryRepository) ListPendingCompositionScopes(ctx context.Context, now time.Time, limit int) ([]Scope, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, ErrRepositoryUnavailable
	}
	var err error
	if now, err = validateTime("composition scope time", now); err != nil {
		return nil, err
	}
	if limit < 1 || limit > maxClaimLimit {
		return nil, fmt.Errorf("%w: composition scope limit is invalid", ErrInvalidInput)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	unique := map[Scope]struct{}{}
	for _, item := range r.compositions {
		if item.Status != CompositionPending || item.NextAttemptAt.After(now) || (item.Lease.Active() && item.Lease.ExpiresAt.After(now)) {
			continue
		}
		unique[item.Scope] = struct{}{}
	}
	items := make([]Scope, 0, len(unique))
	for scope := range unique {
		items = append(items, scope)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].OwnerID != items[j].OwnerID {
			return items[i].OwnerID < items[j].OwnerID
		}
		return items[i].WorkspaceID < items[j].WorkspaceID
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *MemoryRepository) ClaimDueCompositions(ctx context.Context, owner, workspace, worker string, now time.Time, leaseDuration time.Duration, limit int) ([]CompositionDelivery, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, ErrRepositoryUnavailable
	}
	if err := validateRepositoryScope(owner, workspace); err != nil {
		return nil, err
	}
	if err := validateIdentifier("composition worker id", worker); err != nil {
		return nil, err
	}
	var err error
	if now, err = validateTime("composition claim time", now); err != nil {
		return nil, err
	}
	if err := validateLeaseDuration(leaseDuration); err != nil {
		return nil, err
	}
	if limit < 1 || limit > maxClaimLimit {
		return nil, fmt.Errorf("%w: composition claim limit is invalid", ErrInvalidInput)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]CompositionDelivery, 0)
	for _, item := range r.compositions {
		if item.Scope != (Scope{OwnerID: owner, WorkspaceID: workspace}) || item.Status != CompositionPending || item.NextAttemptAt.After(now) || (item.Lease.Active() && item.Lease.ExpiresAt.After(now)) {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].NextAttemptAt.Equal(items[j].NextAttemptAt) {
			return items[i].NextAttemptAt.Before(items[j].NextAttemptAt)
		}
		return items[i].ID < items[j].ID
	})
	if len(items) > limit {
		items = items[:limit]
	}
	for i := range items {
		item := items[i]
		claimAt := now
		if !claimAt.After(item.UpdatedAt) {
			claimAt = item.UpdatedAt.Add(time.Microsecond)
		}
		item.Revision++
		item.Lease = Lease{WorkerID: worker, Generation: item.Lease.Generation + 1, ClaimedAt: claimAt, ExpiresAt: claimAt.Add(leaseDuration)}
		item.UpdatedAt = claimAt
		r.compositions[scopedCompositionKey(owner, workspace, item.ID)] = item
		items[i] = item
	}
	return items, nil
}

func (r *MemoryRepository) CompleteComposition(ctx context.Context, owner, workspace, deliveryID, worker string, generation uint64, attempt CompositionAttempt, completedAt time.Time) (CompositionDelivery, CompositionAttempt, error) {
	return r.finishComposition(ctx, owner, workspace, deliveryID, worker, generation, attempt, completedAt, time.Time{}, false)
}

func (r *MemoryRepository) FailComposition(ctx context.Context, owner, workspace, deliveryID, worker string, generation uint64, attempt CompositionAttempt, next time.Time, deadLetter bool) (CompositionDelivery, CompositionAttempt, error) {
	return r.finishComposition(ctx, owner, workspace, deliveryID, worker, generation, attempt, attempt.FinishedAt, next, deadLetter)
}

func (r *MemoryRepository) finishComposition(ctx context.Context, owner, workspace, deliveryID, worker string, generation uint64, attempt CompositionAttempt, at, next time.Time, deadLetter bool) (CompositionDelivery, CompositionAttempt, error) {
	if err := checkContext(ctx); err != nil {
		return CompositionDelivery{}, CompositionAttempt{}, err
	}
	if r == nil {
		return CompositionDelivery{}, CompositionAttempt{}, ErrRepositoryUnavailable
	}
	if err := validateRepositoryScope(owner, workspace); err != nil {
		return CompositionDelivery{}, CompositionAttempt{}, err
	}
	cleanAttempt, err := validateCompositionAttempt(attempt)
	if err != nil {
		return CompositionDelivery{}, CompositionAttempt{}, err
	}
	if at, err = validateTime("composition finish time", at); err != nil {
		return CompositionDelivery{}, CompositionAttempt{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := scopedCompositionKey(owner, workspace, deliveryID)
	delivery, found := r.compositions[key]
	if !found {
		return CompositionDelivery{}, CompositionAttempt{}, ErrNotFound
	}
	if delivery.Scope != (Scope{OwnerID: owner, WorkspaceID: workspace}) || cleanAttempt.Scope != delivery.Scope || cleanAttempt.DeliveryID != delivery.ID || cleanAttempt.TargetID != delivery.TargetID || cleanAttempt.RunID != delivery.RunID || cleanAttempt.RunDigest != delivery.RunDigest || cleanAttempt.SnapshotDigest != delivery.Snapshot.SnapshotDigest {
		return CompositionDelivery{}, CompositionAttempt{}, ErrScopeViolation
	}
	if delivery.Status != CompositionPending || !delivery.Lease.Active() || delivery.Lease.WorkerID != worker || delivery.Lease.Generation != generation || at.After(delivery.Lease.ExpiresAt) {
		return CompositionDelivery{}, CompositionAttempt{}, ErrLeaseLost
	}
	if cleanAttempt.WorkerID != worker || cleanAttempt.LeaseGeneration != generation || cleanAttempt.AttemptNumber != delivery.AttemptCount+1 || !cleanAttempt.StartedAt.Equal(delivery.Lease.ClaimedAt) || !cleanAttempt.FinishedAt.Equal(at) {
		return CompositionDelivery{}, CompositionAttempt{}, fmt.Errorf("%w: composition attempt does not match lease", ErrInvalidInput)
	}
	for _, previous := range r.attempts[key] {
		if previous.ID == cleanAttempt.ID || previous.RecordDigest == cleanAttempt.RecordDigest {
			return CompositionDelivery{}, CompositionAttempt{}, ErrIdempotencyConflict
		}
	}
	delivery.AttemptCount++
	delivery.Revision++
	delivery.LastAttemptAt = at
	delivery.UpdatedAt = at
	delivery.Lease = Lease{Generation: generation}
	if cleanAttempt.Status == CompositionAttemptSucceeded {
		delivery.Status = CompositionSucceeded
		delivery.CompletedAt = at
		delivery.LastFailureCode = ""
	} else {
		delivery.LastFailureCode = cleanAttempt.FailureCode
		if deadLetter || delivery.AttemptCount >= delivery.MaxAttempts {
			delivery.Status = CompositionDeadLettered
			delivery.CompletedAt = at
		} else {
			if next, err = validateTime("composition retry time", next); err != nil || !next.After(at) {
				return CompositionDelivery{}, CompositionAttempt{}, fmt.Errorf("%w: composition retry time is invalid", ErrInvalidInput)
			}
			delivery.NextAttemptAt = next
		}
	}
	if _, err := validateCompositionDelivery(delivery); err != nil {
		return CompositionDelivery{}, CompositionAttempt{}, err
	}
	r.attempts[key] = append(r.attempts[key], cleanAttempt)
	r.compositions[key] = delivery
	return delivery, cleanAttempt, nil
}

func (r *MemoryRepository) RecoverExpiredCompositionLeases(ctx context.Context, owner, workspace string, now time.Time) (int, error) {
	if err := checkContext(ctx); err != nil {
		return 0, err
	}
	if r == nil {
		return 0, ErrRepositoryUnavailable
	}
	if err := validateRepositoryScope(owner, workspace); err != nil {
		return 0, err
	}
	var err error
	if now, err = validateTime("composition recovery time", now); err != nil {
		return 0, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for key, item := range r.compositions {
		if item.Scope == (Scope{OwnerID: owner, WorkspaceID: workspace}) && item.Status == CompositionPending && item.Lease.Active() && !item.Lease.ExpiresAt.After(now) {
			updateAt := now
			if !updateAt.After(item.UpdatedAt) {
				updateAt = item.UpdatedAt.Add(time.Microsecond)
			}
			item.Revision++
			item.Lease = Lease{Generation: item.Lease.Generation}
			item.UpdatedAt = updateAt
			r.compositions[key] = item
			count++
		}
	}
	return count, nil
}

func (r *MemoryRepository) ListCompositions(ctx context.Context, owner, workspace, targetID string, limit int) ([]CompositionDelivery, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, ErrRepositoryUnavailable
	}
	if err := validateRepositoryScope(owner, workspace); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]CompositionDelivery, 0)
	for _, item := range r.compositions {
		if item.Scope == (Scope{OwnerID: owner, WorkspaceID: workspace}) && item.TargetID == targetID {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	limit = boundedHistoryLimit(limit)
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *MemoryRepository) ListCompositionAttempts(ctx context.Context, owner, workspace, deliveryID string, limit int) ([]CompositionAttempt, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, ErrRepositoryUnavailable
	}
	if err := validateRepositoryScope(owner, workspace); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := append([]CompositionAttempt(nil), r.attempts[scopedCompositionKey(owner, workspace, deliveryID)]...)
	sort.Slice(items, func(i, j int) bool { return items[i].AttemptNumber > items[j].AttemptNumber })
	limit = boundedHistoryLimit(limit)
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}
