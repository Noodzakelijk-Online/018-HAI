package proactivity

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	MaxPolicyHistory   = 32
	MaxSignalHistory   = 512
	MaxDecisionHistory = MaxHistoryEntries
)

var (
	ErrNotFound              = errors.New("proactivity record not found")
	ErrIdempotencyConflict   = errors.New("proactivity idempotency conflict")
	ErrOwnerScopeViolation   = errors.New("proactivity owner scope violation")
	ErrRepositoryUnavailable = errors.New("proactivity repository unavailable")
	ErrInvalidLimit          = errors.New("proactivity limit is invalid")
)

type PolicyRecord struct {
	ContractVersion int         `json:"contractVersion"`
	OwnerIdentity   string      `json:"ownerIdentity"`
	Policy          Preferences `json:"policy"`
	RecordedAt      time.Time   `json:"recordedAt"`
}

type SignalRecord struct {
	ContractVersion int            `json:"contractVersion"`
	OwnerIdentity   string         `json:"ownerIdentity"`
	Signal          OpenLoopSignal `json:"signal"`
	RecordedAt      time.Time      `json:"recordedAt"`
}

type DecisionRecord struct {
	ContractVersion int       `json:"contractVersion"`
	OwnerIdentity   string    `json:"ownerIdentity"`
	Decision        Decision  `json:"decision"`
	RecordedAt      time.Time `json:"recordedAt"`
}

type DecisionBatch struct {
	ContractVersion         int              `json:"contractVersion"`
	OwnerIdentity           string           `json:"ownerIdentity"`
	SnapshotInputDigest     string           `json:"snapshotInputDigest,omitempty"`
	AdditionalSignalsDigest string           `json:"additionalSignalsDigest,omitempty"`
	Result                  EvaluationResult `json:"result"`
	RecordedAt              time.Time        `json:"recordedAt"`
}

// Repository is an owner-scoped advisory ledger. Record methods must apply
// idempotency atomically and return the original value on an exact replay.
type Repository interface {
	RecordPolicy(context.Context, string, string, string, PolicyRecord) (PolicyRecord, bool, error)
	CurrentPolicy(context.Context, string) (PolicyRecord, error)
	ListPolicies(context.Context, string, int) ([]PolicyRecord, error)
	RecordSignals(context.Context, string, string, string, []SignalRecord) ([]SignalRecord, bool, error)
	ListSignals(context.Context, string, int) ([]SignalRecord, error)
	LatestSignals(context.Context, string, int) ([]OpenLoopSignal, error)
	FindDecisionBatch(context.Context, string, string, string) (DecisionBatch, bool, error)
	RecordDecisionBatch(context.Context, string, string, string, DecisionBatch) (DecisionBatch, bool, error)
	ListDecisions(context.Context, string, int) ([]DecisionRecord, error)
	RecordFeedback(context.Context, string, string, string, FeedbackRecord) (FeedbackRecord, bool, error)
	LatestFeedback(context.Context, string, string) (FeedbackRecord, bool, error)
	ListFeedback(context.Context, string, int) ([]FeedbackRecord, error)
}

type idempotencyEntry struct {
	kind      string
	digest    string
	policy    *PolicyRecord
	signals   []SignalRecord
	decisions *DecisionBatch
	feedback  *FeedbackRecord
}

// MemoryRepository is a bounded, concurrency-safe repository intended for
// local operation and tests. It validates owner boundaries independently of
// the service and never exposes its mutable backing slices.
type MemoryRepository struct {
	mu               sync.RWMutex
	policies         map[string][]PolicyRecord
	policyReferences map[string][]PolicySnapshotReference
	signals          map[string][]SignalRecord
	signalCursors    map[string][]SnapshotRecordCursor
	decisions        map[string][]DecisionRecord
	decisionCursors  map[string][]SnapshotRecordCursor
	feedback         map[string][]FeedbackRecord
	feedbackCursors  map[string][]FeedbackSnapshotCursor
	idempotency      map[string]map[string]idempotencyEntry
	idempotencyOrder map[string][]string
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		policies:         make(map[string][]PolicyRecord),
		policyReferences: make(map[string][]PolicySnapshotReference),
		signals:          make(map[string][]SignalRecord),
		signalCursors:    make(map[string][]SnapshotRecordCursor),
		decisions:        make(map[string][]DecisionRecord),
		decisionCursors:  make(map[string][]SnapshotRecordCursor),
		feedback:         make(map[string][]FeedbackRecord),
		feedbackCursors:  make(map[string][]FeedbackSnapshotCursor),
		idempotency:      make(map[string]map[string]idempotencyEntry),
		idempotencyOrder: make(map[string][]string),
	}
}

func (r *MemoryRepository) RecordFeedback(ctx context.Context, owner, key, digest string, record FeedbackRecord) (FeedbackRecord, bool, error) {
	if err := repositoryContext(ctx); err != nil {
		return FeedbackRecord{}, false, err
	}
	if r == nil {
		return FeedbackRecord{}, false, ErrRepositoryUnavailable
	}
	owner = strings.TrimSpace(owner)
	key = strings.TrimSpace(key)
	if err := validateRepositoryCommand(owner, key, digest); err != nil {
		return FeedbackRecord{}, false, err
	}
	if _, err := sanitizeFeedbackRecord(owner, record); err != nil {
		return FeedbackRecord{}, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, found, err := r.lookupLocked(owner, key, digest, "feedback"); err != nil {
		return FeedbackRecord{}, false, err
	} else if found {
		return cloneFeedbackRecord(*existing.feedback), false, nil
	}
	for _, existing := range r.feedback[owner] {
		if existing.OpenLoopKey == record.OpenLoopKey && existing.RecordDigest == record.RecordDigest {
			return FeedbackRecord{}, false, ErrIdempotencyConflict
		}
	}
	stored := cloneFeedbackRecord(record)
	r.feedback[owner] = appendBounded(r.feedback[owner], stored, MaxFeedbackHistory)
	r.feedbackCursors[owner] = appendBounded(r.feedbackCursors[owner], FeedbackSnapshotCursor{
		RecordedAt: record.RecordedAt.UTC(), FeedbackID: record.ID, IdempotencyKey: key,
		PayloadDigest: digest, RecordDigest: record.RecordDigest,
	}, MaxFeedbackHistory)
	r.storeIdempotencyLocked(owner, key, idempotencyEntry{kind: "feedback", digest: digest, feedback: &stored})
	return cloneFeedbackRecord(stored), true, nil
}

func (r *MemoryRepository) LatestFeedback(ctx context.Context, owner, openLoopKey string) (FeedbackRecord, bool, error) {
	if err := repositoryContext(ctx); err != nil {
		return FeedbackRecord{}, false, err
	}
	if r == nil {
		return FeedbackRecord{}, false, ErrRepositoryUnavailable
	}
	owner = strings.TrimSpace(owner)
	openLoopKey = strings.TrimSpace(openLoopKey)
	if _, err := repositoryOwner(owner); err != nil {
		return FeedbackRecord{}, false, err
	}
	if err := validateIdentifier("open loop key", openLoopKey); err != nil {
		return FeedbackRecord{}, false, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := r.feedback[owner]
	for index := len(items) - 1; index >= 0; index-- {
		if items[index].OpenLoopKey == openLoopKey {
			return cloneFeedbackRecord(items[index]), true, nil
		}
	}
	return FeedbackRecord{}, false, nil
}

func (r *MemoryRepository) ListFeedback(ctx context.Context, owner string, limit int) ([]FeedbackRecord, error) {
	if err := repositoryContext(ctx); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, ErrRepositoryUnavailable
	}
	owner = strings.TrimSpace(owner)
	if _, err := repositoryOwner(owner); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := r.feedback[owner]
	maximum := boundedLimit(limit, MaxFeedbackHistory)
	if len(items) < maximum {
		maximum = len(items)
	}
	result := make([]FeedbackRecord, 0, maximum)
	for index := len(items) - 1; index >= 0 && len(result) < maximum; index-- {
		result = append(result, cloneFeedbackRecord(items[index]))
	}
	return result, nil
}

func (r *MemoryRepository) RecordPolicy(ctx context.Context, owner, key, digest string, record PolicyRecord) (PolicyRecord, bool, error) {
	if err := repositoryContext(ctx); err != nil {
		return PolicyRecord{}, false, err
	}
	if r == nil {
		return PolicyRecord{}, false, ErrRepositoryUnavailable
	}
	owner = strings.TrimSpace(owner)
	key = strings.TrimSpace(key)
	if err := validateRepositoryCommand(owner, key, digest); err != nil {
		return PolicyRecord{}, false, err
	}
	if err := validatePolicyRecordOwner(owner, record); err != nil {
		return PolicyRecord{}, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, found, err := r.lookupLocked(owner, key, digest, "policy"); err != nil {
		return PolicyRecord{}, false, err
	} else if found {
		return clonePolicyRecord(*existing.policy), false, nil
	}
	stored := clonePolicyRecord(record)
	r.policies[owner] = appendBounded(r.policies[owner], stored, MaxPolicyHistory)
	r.policyReferences[owner] = appendBounded(r.policyReferences[owner], PolicySnapshotReference{
		IdempotencyKey: key, PayloadDigest: digest, RecordedAt: record.RecordedAt.UTC(),
	}, MaxPolicyHistory)
	r.storeIdempotencyLocked(owner, key, idempotencyEntry{kind: "policy", digest: digest, policy: &stored})
	return clonePolicyRecord(stored), true, nil
}

func (r *MemoryRepository) CurrentPolicy(ctx context.Context, owner string) (PolicyRecord, error) {
	if err := repositoryContext(ctx); err != nil {
		return PolicyRecord{}, err
	}
	if r == nil {
		return PolicyRecord{}, ErrRepositoryUnavailable
	}
	owner, err := repositoryOwner(owner)
	if err != nil {
		return PolicyRecord{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := r.policies[owner]
	if len(items) == 0 {
		return PolicyRecord{}, ErrNotFound
	}
	return clonePolicyRecord(items[len(items)-1]), nil
}

func (r *MemoryRepository) ListPolicies(ctx context.Context, owner string, limit int) ([]PolicyRecord, error) {
	if err := repositoryContext(ctx); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, ErrRepositoryUnavailable
	}
	owner, err := repositoryOwner(owner)
	if err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := r.policies[owner]
	limit = boundedLimit(limit, MaxPolicyHistory)
	result := make([]PolicyRecord, 0, min(limit, len(items)))
	for index := len(items) - 1; index >= 0 && len(result) < limit; index-- {
		result = append(result, clonePolicyRecord(items[index]))
	}
	return result, nil
}

func (r *MemoryRepository) RecordSignals(ctx context.Context, owner, key, digest string, records []SignalRecord) ([]SignalRecord, bool, error) {
	if err := repositoryContext(ctx); err != nil {
		return nil, false, err
	}
	if r == nil {
		return nil, false, ErrRepositoryUnavailable
	}
	owner = strings.TrimSpace(owner)
	key = strings.TrimSpace(key)
	if err := validateRepositoryCommand(owner, key, digest); err != nil {
		return nil, false, err
	}
	if len(records) == 0 || len(records) > MaxSignals {
		return nil, false, errors.New("proactivity signal batch is invalid")
	}
	for _, record := range records {
		if err := validateSignalRecordOwner(owner, record); err != nil {
			return nil, false, err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, found, err := r.lookupLocked(owner, key, digest, "signals"); err != nil {
		return nil, false, err
	} else if found {
		return cloneSignalRecords(existing.signals), false, nil
	}
	stored := cloneSignalRecords(records)
	for ordinal, record := range stored {
		r.signals[owner] = appendBounded(r.signals[owner], record, MaxSignalHistory)
		r.signalCursors[owner] = appendBounded(r.signalCursors[owner], SnapshotRecordCursor{
			RecordedAt: record.RecordedAt.UTC(), IdempotencyKey: key,
			Ordinal: ordinal, PayloadDigest: digest,
		}, MaxSignalHistory)
	}
	r.storeIdempotencyLocked(owner, key, idempotencyEntry{kind: "signals", digest: digest, signals: stored})
	return cloneSignalRecords(stored), true, nil
}

func (r *MemoryRepository) ListSignals(ctx context.Context, owner string, limit int) ([]SignalRecord, error) {
	if err := repositoryContext(ctx); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, ErrRepositoryUnavailable
	}
	owner, err := repositoryOwner(owner)
	if err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := r.signals[owner]
	limit = boundedLimit(limit, MaxSignalHistory)
	result := make([]SignalRecord, 0, min(limit, len(items)))
	for index := len(items) - 1; index >= 0 && len(result) < limit; index-- {
		result = append(result, cloneSignalRecord(items[index]))
	}
	return result, nil
}

func (r *MemoryRepository) LatestSignals(ctx context.Context, owner string, limit int) ([]OpenLoopSignal, error) {
	if err := repositoryContext(ctx); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, ErrRepositoryUnavailable
	}
	owner, err := repositoryOwner(owner)
	if err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := r.signals[owner]
	limit = boundedLimit(limit, MaxSignals)
	seen := make(map[string]struct{}, limit)
	result := make([]OpenLoopSignal, 0, min(limit, len(items)))
	for index := len(items) - 1; index >= 0 && len(result) < limit; index-- {
		signal := items[index].Signal
		if _, exists := seen[signal.ID]; exists {
			continue
		}
		seen[signal.ID] = struct{}{}
		result = append(result, cloneSignal(signal))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (r *MemoryRepository) FindDecisionBatch(ctx context.Context, owner, key, digest string) (DecisionBatch, bool, error) {
	if err := repositoryContext(ctx); err != nil {
		return DecisionBatch{}, false, err
	}
	if r == nil {
		return DecisionBatch{}, false, ErrRepositoryUnavailable
	}
	owner = strings.TrimSpace(owner)
	key = strings.TrimSpace(key)
	if err := validateRepositoryCommand(owner, key, digest); err != nil {
		return DecisionBatch{}, false, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	existing, found, err := r.lookupLocked(owner, key, digest, "decisions")
	if err != nil || !found {
		return DecisionBatch{}, found, err
	}
	return cloneDecisionBatch(*existing.decisions), true, nil
}

func (r *MemoryRepository) RecordDecisionBatch(ctx context.Context, owner, key, digest string, batch DecisionBatch) (DecisionBatch, bool, error) {
	if err := repositoryContext(ctx); err != nil {
		return DecisionBatch{}, false, err
	}
	if r == nil {
		return DecisionBatch{}, false, ErrRepositoryUnavailable
	}
	owner = strings.TrimSpace(owner)
	key = strings.TrimSpace(key)
	if err := validateRepositoryCommand(owner, key, digest); err != nil {
		return DecisionBatch{}, false, err
	}
	if err := validateDecisionBatchOwner(owner, batch); err != nil {
		return DecisionBatch{}, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, found, err := r.lookupLocked(owner, key, digest, "decisions"); err != nil {
		return DecisionBatch{}, false, err
	} else if found {
		return cloneDecisionBatch(*existing.decisions), false, nil
	}
	stored := cloneDecisionBatch(batch)
	for ordinal, decision := range stored.Result.Decisions {
		record := DecisionRecord{
			ContractVersion: ContractVersion,
			OwnerIdentity:   owner,
			Decision:        cloneDecision(decision),
			RecordedAt:      stored.RecordedAt,
		}
		r.decisions[owner] = appendBounded(r.decisions[owner], record, MaxDecisionHistory)
		r.decisionCursors[owner] = appendBounded(r.decisionCursors[owner], SnapshotRecordCursor{
			RecordedAt: batch.RecordedAt.UTC(), IdempotencyKey: key,
			Ordinal: ordinal, PayloadDigest: digest,
		}, MaxDecisionHistory)
	}
	r.storeIdempotencyLocked(owner, key, idempotencyEntry{kind: "decisions", digest: digest, decisions: &stored})
	return cloneDecisionBatch(stored), true, nil
}

func (r *MemoryRepository) ListDecisions(ctx context.Context, owner string, limit int) ([]DecisionRecord, error) {
	if err := repositoryContext(ctx); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, ErrRepositoryUnavailable
	}
	owner, err := repositoryOwner(owner)
	if err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := r.decisions[owner]
	limit = boundedLimit(limit, MaxDecisionHistory)
	result := make([]DecisionRecord, 0, min(limit, len(items)))
	for index := len(items) - 1; index >= 0 && len(result) < limit; index-- {
		result = append(result, cloneDecisionRecord(items[index]))
	}
	return result, nil
}

func (r *MemoryRepository) lookupLocked(owner, key, digest, kind string) (idempotencyEntry, bool, error) {
	entries := r.idempotency[strings.TrimSpace(owner)]
	entry, found := entries[strings.TrimSpace(key)]
	if !found {
		return idempotencyEntry{}, false, nil
	}
	if entry.kind != kind || entry.digest != digest {
		return idempotencyEntry{}, false, ErrIdempotencyConflict
	}
	return entry, true, nil
}

func (r *MemoryRepository) storeIdempotencyLocked(owner, key string, entry idempotencyEntry) {
	if r.idempotency[owner] == nil {
		r.idempotency[owner] = make(map[string]idempotencyEntry)
	}
	if _, exists := r.idempotency[owner][key]; !exists {
		r.idempotencyOrder[owner] = append(r.idempotencyOrder[owner], key)
	}
	r.idempotency[owner][key] = entry
	if len(r.idempotencyOrder[owner]) > MaxHistoryEntries {
		oldest := r.idempotencyOrder[owner][0]
		delete(r.idempotency[owner], oldest)
		r.idempotencyOrder[owner] = append([]string(nil), r.idempotencyOrder[owner][1:]...)
	}
}

func repositoryContext(ctx context.Context) error {
	if ctx == nil {
		return errors.New("proactivity context is required")
	}
	return ctx.Err()
}

func repositoryOwner(owner string) (string, error) {
	owner = strings.TrimSpace(owner)
	if err := validateIdentity(owner); err != nil {
		return "", ErrOwnerScopeViolation
	}
	return owner, nil
}

func validateRepositoryCommand(owner, key, digest string) error {
	if _, err := repositoryOwner(owner); err != nil {
		return err
	}
	if err := validateIdentifier("idempotency key", key); err != nil {
		return err
	}
	if !digestPattern.MatchString(strings.TrimSpace(digest)) {
		return errors.New("proactivity payload digest is invalid")
	}
	return nil
}

func validatePolicyRecordOwner(owner string, record PolicyRecord) error {
	owner = strings.TrimSpace(owner)
	if owner == "" || record.OwnerIdentity != owner || record.Policy.OwnerIdentity != owner {
		return ErrOwnerScopeViolation
	}
	if record.ContractVersion != ContractVersion || record.Policy.ContractVersion != ContractVersion || record.RecordedAt.IsZero() {
		return errors.New("proactivity policy record is invalid")
	}
	return nil
}

func validateSignalRecordOwner(owner string, record SignalRecord) error {
	owner = strings.TrimSpace(owner)
	if owner == "" || record.OwnerIdentity != owner || record.Signal.OwnerIdentity != owner {
		return ErrOwnerScopeViolation
	}
	if record.ContractVersion != ContractVersion || record.Signal.ContractVersion != ContractVersion || record.RecordedAt.IsZero() {
		return errors.New("proactivity signal record is invalid")
	}
	return nil
}

func validateDecisionBatchOwner(owner string, batch DecisionBatch) error {
	owner = strings.TrimSpace(owner)
	if owner == "" || batch.OwnerIdentity != owner || batch.Result.OwnerIdentity != owner {
		return ErrOwnerScopeViolation
	}
	if batch.ContractVersion != ContractVersion || batch.Result.ContractVersion != ContractVersion || batch.RecordedAt.IsZero() || len(batch.Result.Decisions) > MaxSignals {
		return errors.New("proactivity decision batch is invalid")
	}
	if (batch.SnapshotInputDigest != "" && !digestPattern.MatchString(batch.SnapshotInputDigest)) ||
		(batch.AdditionalSignalsDigest != "" && !digestPattern.MatchString(batch.AdditionalSignalsDigest)) {
		return errors.New("proactivity decision snapshot digest is invalid")
	}
	for _, decision := range batch.Result.Decisions {
		if decision.ContractVersion != ContractVersion || decision.OwnerIdentity != owner || decision.ExecutionAuthorized || decision.DeliveryAuthorized || decision.AuthorityGranted {
			return ErrOwnerScopeViolation
		}
	}
	return nil
}

func appendBounded[T any](items []T, value T, maximum int) []T {
	items = append(items, value)
	if len(items) <= maximum {
		return items
	}
	trimmed := make([]T, maximum)
	copy(trimmed, items[len(items)-maximum:])
	return trimmed
}

func boundedLimit(limit, maximum int) int {
	if limit <= 0 || limit > maximum {
		return maximum
	}
	return limit
}

func clonePolicyRecord(value PolicyRecord) PolicyRecord {
	value.Policy = clonePreferences(value.Policy)
	return value
}

func clonePreferences(value Preferences) Preferences {
	value.Channels = append([]ChannelPreference(nil), value.Channels...)
	return value
}

func cloneSignalRecords(values []SignalRecord) []SignalRecord {
	result := make([]SignalRecord, len(values))
	for index, value := range values {
		result[index] = cloneSignalRecord(value)
	}
	return result
}

func cloneSignalRecord(value SignalRecord) SignalRecord {
	value.Signal = cloneSignal(value.Signal)
	return value
}

func cloneSignal(value OpenLoopSignal) OpenLoopSignal {
	if value.Deadline != nil {
		deadline := *value.Deadline
		value.Deadline = &deadline
	}
	value.Evidence = append([]EvidenceReference(nil), value.Evidence...)
	return value
}

func cloneDecisionBatch(value DecisionBatch) DecisionBatch {
	value.Result.Decisions = cloneDecisions(value.Result.Decisions)
	return value
}

func cloneDecisionRecord(value DecisionRecord) DecisionRecord {
	value.Decision = cloneDecision(value.Decision)
	return value
}

func cloneDecisions(values []Decision) []Decision {
	result := make([]Decision, len(values))
	for index, value := range values {
		result[index] = cloneDecision(value)
	}
	return result
}

func cloneDecision(value Decision) Decision {
	value.Components = append([]ScoreComponent(nil), value.Components...)
	value.Reasons = append([]string(nil), value.Reasons...)
	value.RecommendedChannels = append([]Channel(nil), value.RecommendedChannels...)
	if value.NextEligibleAt != nil {
		next := *value.NextEligibleAt
		value.NextEligibleAt = &next
	}
	return value
}
