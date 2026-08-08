package ambientmonitor

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Repository is an owner/workspace-scoped persistence seam. Complete and Fail
// must atomically fence the lease, append immutable records, advance the next
// run, and apply exact idempotency.
type Repository interface {
	CreateTarget(context.Context, string, string, string, MonitorTarget) (MonitorTarget, bool, error)
	SetEnabled(context.Context, string, string, string, string, string, bool, time.Time) (MonitorTarget, bool, error)
	FindCompletion(context.Context, string, string, string) (Completion, bool, error)
	GetTarget(context.Context, string, string, string) (MonitorTarget, error)
	ListTargets(context.Context, string, string) ([]MonitorTarget, error)
	ListDueScopes(context.Context, time.Time, int) ([]Scope, error)
	ClaimDue(context.Context, string, string, string, time.Time, time.Duration, int) ([]MonitorTarget, error)
	Complete(context.Context, string, string, string, string, string, string, uint64, string, ObservationRecord, MonitorRun, CompositionSnapshot, time.Time) (ObservationRecord, MonitorRun, bool, error)
	Fail(context.Context, string, string, string, string, string, string, uint64, MonitorRun, time.Time) (MonitorRun, bool, error)
	RecoverExpiredLeases(context.Context, string, string, time.Time) (int, error)
	ListObservations(context.Context, string, string, string, int) ([]ObservationRecord, error)
	ListObservationsAt(context.Context, string, string, string, time.Time, int) ([]ObservationRecord, error)
	ListRuns(context.Context, string, string, string, int) ([]MonitorRun, error)
	GetCompositionByRun(context.Context, string, string, string) (CompositionDelivery, error)
	GetComposition(context.Context, string, string, string) (CompositionDelivery, error)
	LoadCompositionSignal(context.Context, string, string, string) (AdvisorySignal, error)
	ListPendingCompositionScopes(context.Context, time.Time, int) ([]Scope, error)
	ClaimDueCompositions(context.Context, string, string, string, time.Time, time.Duration, int) ([]CompositionDelivery, error)
	CompleteComposition(context.Context, string, string, string, string, uint64, CompositionAttempt, time.Time) (CompositionDelivery, CompositionAttempt, error)
	FailComposition(context.Context, string, string, string, string, uint64, CompositionAttempt, time.Time, bool) (CompositionDelivery, CompositionAttempt, error)
	RecoverExpiredCompositionLeases(context.Context, string, string, time.Time) (int, error)
	ListCompositions(context.Context, string, string, string, int) ([]CompositionDelivery, error)
	ListCompositionAttempts(context.Context, string, string, string, int) ([]CompositionAttempt, error)
}

type idempotencyEntry struct {
	operation   string
	digest      string
	target      *MonitorTarget
	observation *ObservationRecord
	run         *MonitorRun
}

// MemoryRepository is a concurrency-safe reference implementation. Returned
// values are copies, so accepted observations and runs cannot be modified
// through repository aliases.
type MemoryRepository struct {
	mu           sync.RWMutex
	targets      map[string]MonitorTarget
	observations map[string][]ObservationRecord
	runs         map[string][]MonitorRun
	compositions map[string]CompositionDelivery
	attempts     map[string][]CompositionAttempt
	idempotency  map[string]map[string]idempotencyEntry
}

var _ Repository = (*MemoryRepository)(nil)

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		targets: make(map[string]MonitorTarget), observations: make(map[string][]ObservationRecord),
		runs: make(map[string][]MonitorRun), compositions: make(map[string]CompositionDelivery),
		attempts: make(map[string][]CompositionAttempt), idempotency: make(map[string]map[string]idempotencyEntry),
	}
}

func (r *MemoryRepository) CreateTarget(ctx context.Context, owner, workspace, key string, target MonitorTarget) (MonitorTarget, bool, error) {
	if err := checkContext(ctx); err != nil {
		return MonitorTarget{}, false, err
	}
	if r == nil {
		return MonitorTarget{}, false, ErrRepositoryUnavailable
	}
	if err := validateRepositoryScope(owner, workspace); err != nil {
		return MonitorTarget{}, false, err
	}
	if err := validateIdempotency(key, targetDigest(target)); err != nil {
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
	r.mu.Lock()
	defer r.mu.Unlock()
	if item, found, lookupErr := r.lookupLocked(owner, workspace, key, "create_target", digest); lookupErr != nil {
		return MonitorTarget{}, false, lookupErr
	} else if found {
		return *item.target, false, nil
	}
	storageKey := scopedTargetKey(owner, workspace, clean.ID)
	if _, exists := r.targets[storageKey]; exists {
		return MonitorTarget{}, false, ErrIdempotencyConflict
	}
	r.targets[storageKey] = clean
	r.storeIdempotencyLocked(owner, workspace, key, idempotencyEntry{operation: "create_target", digest: digest, target: &clean})
	return clean, true, nil
}

func (r *MemoryRepository) SetEnabled(ctx context.Context, owner, workspace, key, digest, targetID string, enabled bool, at time.Time) (MonitorTarget, bool, error) {
	if err := checkContext(ctx); err != nil {
		return MonitorTarget{}, false, err
	}
	if r == nil {
		return MonitorTarget{}, false, ErrRepositoryUnavailable
	}
	if err := validateRepositoryScope(owner, workspace); err != nil {
		return MonitorTarget{}, false, err
	}
	if err := validateIdentifier("target id", targetID); err != nil {
		return MonitorTarget{}, false, err
	}
	if err := validateIdempotency(key, digest); err != nil {
		return MonitorTarget{}, false, err
	}
	var err error
	if at, err = validateTime("target update time", at); err != nil {
		return MonitorTarget{}, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if item, found, lookupErr := r.lookupLocked(owner, workspace, key, "set_enabled", digest); lookupErr != nil {
		return MonitorTarget{}, false, lookupErr
	} else if found {
		return *item.target, false, nil
	}
	storageKey := scopedTargetKey(owner, workspace, targetID)
	target, exists := r.targets[storageKey]
	if !exists {
		return MonitorTarget{}, false, ErrNotFound
	}
	if at.Before(target.UpdatedAt) {
		return MonitorTarget{}, false, fmt.Errorf("%w: target update time moved backwards", ErrInvalidInput)
	}
	target.Enabled = enabled
	target.UpdatedAt = at
	if !enabled {
		target.Lease = Lease{Generation: target.Lease.Generation}
	}
	r.targets[storageKey] = target
	r.storeIdempotencyLocked(owner, workspace, key, idempotencyEntry{operation: "set_enabled", digest: digest, target: &target})
	return target, true, nil
}

func (r *MemoryRepository) FindCompletion(ctx context.Context, owner, workspace, key string) (Completion, bool, error) {
	if err := checkContext(ctx); err != nil {
		return Completion{}, false, err
	}
	if r == nil {
		return Completion{}, false, ErrRepositoryUnavailable
	}
	if err := validateRepositoryScope(owner, workspace); err != nil {
		return Completion{}, false, err
	}
	if err := validateIdentifier("idempotency key", strings.TrimSpace(key)); err != nil {
		return Completion{}, false, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, found := r.idempotency[scopeKey(owner, workspace)][strings.TrimSpace(key)]
	if !found {
		return Completion{}, false, nil
	}
	if entry.operation != "complete" || entry.observation == nil || entry.run == nil {
		return Completion{}, false, ErrIdempotencyConflict
	}
	if err := validateObservationRecord(*entry.observation, owner, workspace, entry.run.TargetID); err != nil {
		return Completion{}, false, err
	}
	if err := validateRunRecord(*entry.run, owner, workspace, entry.run.TargetID); err != nil {
		return Completion{}, false, err
	}
	return Completion{Observation: *entry.observation, Run: *entry.run, Created: false, Authority: advisoryAuthority()}, true, nil
}

func (r *MemoryRepository) GetTarget(ctx context.Context, owner, workspace, targetID string) (MonitorTarget, error) {
	if err := checkContext(ctx); err != nil {
		return MonitorTarget{}, err
	}
	if r == nil {
		return MonitorTarget{}, ErrRepositoryUnavailable
	}
	if err := validateRepositoryScope(owner, workspace); err != nil {
		return MonitorTarget{}, err
	}
	if err := validateIdentifier("target id", targetID); err != nil {
		return MonitorTarget{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	target, exists := r.targets[scopedTargetKey(owner, workspace, targetID)]
	if !exists {
		return MonitorTarget{}, ErrNotFound
	}
	return target, nil
}

func (r *MemoryRepository) ListTargets(ctx context.Context, owner, workspace string) ([]MonitorTarget, error) {
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
	items := make([]MonitorTarget, 0)
	for _, target := range r.targets {
		if target.Scope == (Scope{OwnerID: owner, WorkspaceID: workspace}) {
			items = append(items, target)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].NextRunAt.Equal(items[j].NextRunAt) {
			return items[i].NextRunAt.Before(items[j].NextRunAt)
		}
		return items[i].ID < items[j].ID
	})
	return items, nil
}

func (r *MemoryRepository) ListDueScopes(ctx context.Context, now time.Time, limit int) ([]Scope, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, ErrRepositoryUnavailable
	}
	var err error
	if now, err = validateTime("due scope time", now); err != nil {
		return nil, err
	}
	if limit < 1 || limit > maxClaimLimit {
		return nil, fmt.Errorf("%w: due scope limit must be between 1 and %d", ErrInvalidInput, maxClaimLimit)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	unique := make(map[Scope]struct{})
	for _, target := range r.targets {
		if !target.Enabled || target.NextRunAt.After(now) || (target.Lease.Active() && target.Lease.ExpiresAt.After(now)) {
			continue
		}
		unique[target.Scope] = struct{}{}
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

func (r *MemoryRepository) ClaimDue(ctx context.Context, owner, workspace, worker string, now time.Time, leaseDuration time.Duration, limit int) ([]MonitorTarget, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, ErrRepositoryUnavailable
	}
	if err := validateRepositoryScope(owner, workspace); err != nil {
		return nil, err
	}
	if err := validateIdentifier("worker id", worker); err != nil {
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
	r.mu.Lock()
	defer r.mu.Unlock()
	candidates := make([]MonitorTarget, 0)
	for _, target := range r.targets {
		if target.Scope != (Scope{OwnerID: owner, WorkspaceID: workspace}) || !target.Enabled || target.NextRunAt.After(now) {
			continue
		}
		if target.Lease.Active() && target.Lease.ExpiresAt.After(now) {
			continue
		}
		candidates = append(candidates, target)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if !candidates[i].NextRunAt.Equal(candidates[j].NextRunAt) {
			return candidates[i].NextRunAt.Before(candidates[j].NextRunAt)
		}
		return candidates[i].ID < candidates[j].ID
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	for index := range candidates {
		target := candidates[index]
		target.Lease = Lease{WorkerID: worker, Generation: target.Lease.Generation + 1, ClaimedAt: now, ExpiresAt: now.Add(leaseDuration)}
		target.UpdatedAt = now
		r.targets[scopedTargetKey(owner, workspace, target.ID)] = target
		candidates[index] = target
	}
	return candidates, nil
}

func (r *MemoryRepository) Complete(ctx context.Context, owner, workspace, key, digest, targetID, worker string, generation uint64, expectedSourceDigest string, observation ObservationRecord, run MonitorRun, snapshot CompositionSnapshot, next time.Time) (ObservationRecord, MonitorRun, bool, error) {
	if err := checkContext(ctx); err != nil {
		return ObservationRecord{}, MonitorRun{}, false, err
	}
	if r == nil {
		return ObservationRecord{}, MonitorRun{}, false, ErrRepositoryUnavailable
	}
	if err := validateRepositoryScope(owner, workspace); err != nil {
		return ObservationRecord{}, MonitorRun{}, false, err
	}
	if err := validateIdempotency(key, digest); err != nil {
		return ObservationRecord{}, MonitorRun{}, false, err
	}
	if err := validateIdentifier("target id", targetID); err != nil {
		return ObservationRecord{}, MonitorRun{}, false, err
	}
	if err := validateIdentifier("worker id", worker); err != nil {
		return ObservationRecord{}, MonitorRun{}, false, err
	}
	if generation == 0 {
		return ObservationRecord{}, MonitorRun{}, false, fmt.Errorf("%w: lease generation is required", ErrInvalidInput)
	}
	if expectedSourceDigest, _ = validateDigest("expected source digest", expectedSourceDigest); expectedSourceDigest == "" {
		return ObservationRecord{}, MonitorRun{}, false, fmt.Errorf("%w: expected source digest is invalid", ErrInvalidInput)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if item, found, lookupErr := r.lookupLocked(owner, workspace, key, "complete", digest); lookupErr != nil {
		return ObservationRecord{}, MonitorRun{}, false, lookupErr
	} else if found {
		return *item.observation, *item.run, false, nil
	}
	storageKey := scopedTargetKey(owner, workspace, targetID)
	target, exists := r.targets[storageKey]
	if !exists {
		return ObservationRecord{}, MonitorRun{}, false, ErrNotFound
	}
	if err := ownedLease(target, worker, generation, run.FinishedAt); err != nil {
		return ObservationRecord{}, MonitorRun{}, false, err
	}
	if err := validateObservationRecord(observation, owner, workspace, targetID); err != nil {
		return ObservationRecord{}, MonitorRun{}, false, err
	}
	if err := validateRunRecord(run, owner, workspace, targetID); err != nil {
		return ObservationRecord{}, MonitorRun{}, false, err
	}
	if observation.SourceDigest != expectedSourceDigest || run.ObservationID != observation.ID || run.ObservationDigest != observation.RecordDigest || run.Status != RunCompleted || run.IdempotencyDigest != digest {
		return ObservationRecord{}, MonitorRun{}, false, fmt.Errorf("%w: completion record binding is invalid", ErrInvalidInput)
	}
	var err error
	if next, err = validateTime("next run time", next); err != nil {
		return ObservationRecord{}, MonitorRun{}, false, err
	}
	if target.OutcomeID != observation.OutcomeID || target.IndicatorID != observation.IndicatorID || target.SourceKind != observation.SourceKind || target.OutcomeID != run.OutcomeID || target.IndicatorID != run.IndicatorID || target.SourceKind != run.SourceKind {
		return ObservationRecord{}, MonitorRun{}, false, ErrScopeViolation
	}
	if hasObservationDigest(r.observations[storageKey], observation.RecordDigest) || hasRunDigest(r.runs[storageKey], run.RecordDigest) {
		return ObservationRecord{}, MonitorRun{}, false, ErrIdempotencyConflict
	}
	delivery, err := initialCompositionDelivery(observation, run, snapshot)
	if err != nil {
		return ObservationRecord{}, MonitorRun{}, false, err
	}
	if _, exists := r.compositions[scopedCompositionKey(owner, workspace, delivery.ID)]; exists {
		return ObservationRecord{}, MonitorRun{}, false, ErrIdempotencyConflict
	}
	target.NextRunAt = next
	target.Lease = Lease{Generation: target.Lease.Generation}
	target.UpdatedAt = run.FinishedAt
	r.targets[storageKey] = target
	r.observations[storageKey] = appendBoundedObservation(r.observations[storageKey], observation)
	r.runs[storageKey] = appendBoundedRun(r.runs[storageKey], run)
	r.compositions[scopedCompositionKey(owner, workspace, delivery.ID)] = delivery
	r.storeIdempotencyLocked(owner, workspace, key, idempotencyEntry{operation: "complete", digest: digest, observation: &observation, run: &run})
	return observation, run, true, nil
}

func (r *MemoryRepository) Fail(ctx context.Context, owner, workspace, key, digest, targetID, worker string, generation uint64, run MonitorRun, next time.Time) (MonitorRun, bool, error) {
	if err := checkContext(ctx); err != nil {
		return MonitorRun{}, false, err
	}
	if r == nil {
		return MonitorRun{}, false, ErrRepositoryUnavailable
	}
	if err := validateRepositoryScope(owner, workspace); err != nil {
		return MonitorRun{}, false, err
	}
	if err := validateIdempotency(key, digest); err != nil {
		return MonitorRun{}, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if item, found, lookupErr := r.lookupLocked(owner, workspace, key, "fail", digest); lookupErr != nil {
		return MonitorRun{}, false, lookupErr
	} else if found {
		return *item.run, false, nil
	}
	storageKey := scopedTargetKey(owner, workspace, targetID)
	target, exists := r.targets[storageKey]
	if !exists {
		return MonitorRun{}, false, ErrNotFound
	}
	if err := ownedLease(target, worker, generation, run.FinishedAt); err != nil {
		return MonitorRun{}, false, err
	}
	if err := validateRunRecord(run, owner, workspace, targetID); err != nil {
		return MonitorRun{}, false, err
	}
	if run.Status != RunFailed || run.IdempotencyDigest != digest {
		return MonitorRun{}, false, fmt.Errorf("%w: failure record binding is invalid", ErrInvalidInput)
	}
	var err error
	if next, err = validateTime("next run time", next); err != nil {
		return MonitorRun{}, false, err
	}
	if target.OutcomeID != run.OutcomeID || target.IndicatorID != run.IndicatorID || target.SourceKind != run.SourceKind {
		return MonitorRun{}, false, ErrScopeViolation
	}
	if hasRunDigest(r.runs[storageKey], run.RecordDigest) {
		return MonitorRun{}, false, ErrIdempotencyConflict
	}
	target.NextRunAt = next
	target.Lease = Lease{Generation: target.Lease.Generation}
	target.UpdatedAt = run.FinishedAt
	r.targets[storageKey] = target
	r.runs[storageKey] = appendBoundedRun(r.runs[storageKey], run)
	r.storeIdempotencyLocked(owner, workspace, key, idempotencyEntry{operation: "fail", digest: digest, run: &run})
	return run, true, nil
}

func (r *MemoryRepository) RecoverExpiredLeases(ctx context.Context, owner, workspace string, now time.Time) (int, error) {
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
	if now, err = validateTime("recovery time", now); err != nil {
		return 0, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	recovered := 0
	for key, target := range r.targets {
		if target.Scope == (Scope{OwnerID: owner, WorkspaceID: workspace}) && target.Lease.Active() && !target.Lease.ExpiresAt.After(now) {
			target.Lease = Lease{Generation: target.Lease.Generation}
			target.UpdatedAt = now
			r.targets[key] = target
			recovered++
		}
	}
	return recovered, nil
}

func (r *MemoryRepository) ListObservations(ctx context.Context, owner, workspace, targetID string, limit int) ([]ObservationRecord, error) {
	return r.ListObservationsAt(ctx, owner, workspace, targetID, time.Date(2200, 12, 31, 23, 59, 59, 0, time.UTC), limit)
}

func (r *MemoryRepository) ListObservationsAt(ctx context.Context, owner, workspace, targetID string, cutoff time.Time, limit int) ([]ObservationRecord, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, ErrRepositoryUnavailable
	}
	if err := validateRepositoryScope(owner, workspace); err != nil {
		return nil, err
	}
	if err := validateIdentifier("target id", targetID); err != nil {
		return nil, err
	}
	var err error
	if cutoff, err = validateTime("observation history cutoff", cutoff); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	source := r.observations[scopedTargetKey(owner, workspace, targetID)]
	limit = boundedHistoryLimit(limit)
	result := make([]ObservationRecord, 0, min(limit, len(source)))
	for index := len(source) - 1; index >= 0 && len(result) < limit; index-- {
		if source[index].RecordedAt.After(cutoff) {
			continue
		}
		result = append(result, source[index])
	}
	return result, nil
}

func (r *MemoryRepository) ListRuns(ctx context.Context, owner, workspace, targetID string, limit int) ([]MonitorRun, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, ErrRepositoryUnavailable
	}
	if err := validateRepositoryScope(owner, workspace); err != nil {
		return nil, err
	}
	if err := validateIdentifier("target id", targetID); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	source := r.runs[scopedTargetKey(owner, workspace, targetID)]
	limit = boundedHistoryLimit(limit)
	result := make([]MonitorRun, 0, min(limit, len(source)))
	for index := len(source) - 1; index >= 0 && len(result) < limit; index-- {
		result = append(result, source[index])
	}
	return result, nil
}

func (r *MemoryRepository) lookupLocked(owner, workspace, key, operation, digest string) (idempotencyEntry, bool, error) {
	entry, found := r.idempotency[scopeKey(owner, workspace)][key]
	if !found {
		return idempotencyEntry{}, false, nil
	}
	if entry.operation != operation || entry.digest != digest {
		return idempotencyEntry{}, false, ErrIdempotencyConflict
	}
	return entry, true, nil
}

func (r *MemoryRepository) storeIdempotencyLocked(owner, workspace, key string, entry idempotencyEntry) {
	scope := scopeKey(owner, workspace)
	if r.idempotency[scope] == nil {
		r.idempotency[scope] = make(map[string]idempotencyEntry)
	}
	r.idempotency[scope][key] = entry
}

func validateRepositoryScope(owner, workspace string) error {
	scope, err := validateScope(Scope{OwnerID: strings.TrimSpace(owner), WorkspaceID: strings.TrimSpace(workspace)})
	if err != nil {
		return err
	}
	if scope.OwnerID != owner || scope.WorkspaceID != workspace {
		return fmt.Errorf("%w: repository scope is not canonical", ErrInvalidInput)
	}
	return nil
}
func checkContext(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("%w: context is required", ErrInvalidInput)
	}
	return ctx.Err()
}
func scopeKey(owner, workspace string) string { return owner + "\x00" + workspace }
func scopedTargetKey(owner, workspace, target string) string {
	return scopeKey(owner, workspace) + "\x00" + target
}
func targetDigest(target MonitorTarget) string {
	digest, _ := exactDigest("create_target", struct {
		Scope                      Scope
		ID, OutcomeID, IndicatorID string
		SourceKind                 SourceKind
		Enabled                    bool
		Cadence                    time.Duration
		NextRunAt                  time.Time
	}{target.Scope, target.ID, target.OutcomeID, target.IndicatorID, target.SourceKind, target.Enabled, target.Cadence, target.NextRunAt})
	return digest
}
func boundedHistoryLimit(limit int) int {
	if limit <= 0 || limit > maxHistory {
		return maxHistory
	}
	return limit
}
func appendBoundedObservation(items []ObservationRecord, value ObservationRecord) []ObservationRecord {
	items = append(items, value)
	if len(items) > maxHistory {
		items = append([]ObservationRecord(nil), items[len(items)-maxHistory:]...)
	}
	return items
}
func appendBoundedRun(items []MonitorRun, value MonitorRun) []MonitorRun {
	items = append(items, value)
	if len(items) > maxHistory {
		items = append([]MonitorRun(nil), items[len(items)-maxHistory:]...)
	}
	return items
}
func hasObservationDigest(items []ObservationRecord, digest string) bool {
	for _, item := range items {
		if item.RecordDigest == digest {
			return true
		}
	}
	return false
}
func hasRunDigest(items []MonitorRun, digest string) bool {
	for _, item := range items {
		if item.RecordDigest == digest {
			return true
		}
	}
	return false
}
func ownedLease(target MonitorTarget, worker string, generation uint64, at time.Time) error {
	if !target.Enabled || !target.Lease.Active() || target.Lease.WorkerID != worker || target.Lease.Generation != generation || !target.Lease.ExpiresAt.After(at) {
		return ErrLeaseLost
	}
	return nil
}

func validateObservationRecord(value ObservationRecord, owner, workspace, target string) error {
	if value.ContractVersion != ContractVersion || value.Scope != (Scope{OwnerID: owner, WorkspaceID: workspace}) || value.TargetID != target {
		return ErrScopeViolation
	}
	for name, item := range map[string]string{"observation id": value.ID, "outcome id": value.OutcomeID, "indicator id": value.IndicatorID, "target id": value.TargetID} {
		if err := validateIdentifier(name, item); err != nil {
			return err
		}
	}
	if err := validateSourceKind(value.SourceKind); err != nil {
		return err
	}
	if _, err := validateTime("observed at", value.ObservedAt); err != nil {
		return err
	}
	if _, err := validateTime("recorded at", value.RecordedAt); err != nil {
		return err
	}
	if _, err := validateDigest("source digest", value.SourceDigest); err != nil {
		return err
	}
	if _, err := validateDigest("record digest", value.RecordDigest); err != nil {
		return err
	}
	return validateAuthority(value.Authority)
}
func validateRunRecord(value MonitorRun, owner, workspace, target string) error {
	if value.ContractVersion != ContractVersion || value.Scope != (Scope{OwnerID: owner, WorkspaceID: workspace}) || value.TargetID != target {
		return ErrScopeViolation
	}
	for name, item := range map[string]string{"run id": value.ID, "target id": value.TargetID, "outcome id": value.OutcomeID, "indicator id": value.IndicatorID} {
		if err := validateIdentifier(name, item); err != nil {
			return err
		}
	}
	if value.LeaseGeneration == 0 {
		return fmt.Errorf("%w: lease generation is required", ErrInvalidInput)
	}
	if err := validateSourceKind(value.SourceKind); err != nil {
		return err
	}
	if _, err := validateTime("run start time", value.StartedAt); err != nil {
		return err
	}
	if _, err := validateTime("run finish time", value.FinishedAt); err != nil {
		return err
	}
	if value.FinishedAt.Before(value.StartedAt) {
		return fmt.Errorf("%w: run finishes before it starts", ErrInvalidInput)
	}
	if _, err := validateDigest("idempotency digest", value.IdempotencyDigest); err != nil {
		return err
	}
	if _, err := validateDigest("run record digest", value.RecordDigest); err != nil {
		return err
	}
	if value.Status == RunCompleted {
		if err := validateIdentifier("observation id", value.ObservationID); err != nil {
			return err
		}
		if _, err := validateDigest("observation digest", value.ObservationDigest); err != nil {
			return err
		}
		if value.FailureCode != "" || value.FailureSummary != "" {
			return fmt.Errorf("%w: completed run contains failure data", ErrInvalidInput)
		}
	} else if value.Status == RunFailed {
		if value.ObservationID != "" || value.ObservationDigest != "" {
			return fmt.Errorf("%w: failed run contains observation data", ErrInvalidInput)
		}
		if err := validateIdentifier("failure code", value.FailureCode); err != nil {
			return err
		}
		if err := validateBoundedText("failure summary", value.FailureSummary, maxFailureLength, true); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("%w: run status is unsupported", ErrInvalidInput)
	}
	return validateAuthority(value.Authority)
}
