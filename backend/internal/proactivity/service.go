package proactivity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
)

type Service struct {
	repository Repository
	now        func() time.Time
}

type EvaluateStoredRequest struct {
	IdempotencyKey string    `json:"idempotencyKey"`
	Now            time.Time `json:"now"`
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository, now: time.Now}
}

// NewServiceWithClock supports deterministic embedding and tests without
// coupling append-only policy timestamps to the host wall clock.
func NewServiceWithClock(repository Repository, now func() time.Time) *Service {
	return newService(repository, now)
}

func newService(repository Repository, now func() time.Time) *Service {
	service := NewService(repository)
	if now != nil {
		service.now = now
	}
	return service
}

func (s *Service) RecordPolicy(ctx context.Context, owner, idempotencyKey string, policy Preferences) (PolicyRecord, bool, error) {
	owner, err := validateServiceIdentity(owner)
	if err != nil {
		return PolicyRecord{}, false, err
	}
	if err := validateIdempotencyKey(idempotencyKey); err != nil {
		return PolicyRecord{}, false, err
	}
	if err := s.available(); err != nil {
		return PolicyRecord{}, false, err
	}
	policy, _, err = normalizePreferences(owner, clonePreferences(policy))
	if err != nil {
		return PolicyRecord{}, false, err
	}
	recordedAt := s.now().UTC().Truncate(time.Microsecond)
	record := PolicyRecord{
		ContractVersion: ContractVersion,
		OwnerIdentity:   owner,
		Policy:          policy,
		RecordedAt:      recordedAt,
	}
	digest, err := advisoryDigest("policy", owner, policy)
	if err != nil {
		return PolicyRecord{}, false, fmt.Errorf("digest proactivity policy: %w", err)
	}
	stored, created, err := s.repository.RecordPolicy(ctx, owner, strings.TrimSpace(idempotencyKey), digest, record)
	if err != nil {
		return PolicyRecord{}, false, err
	}
	cleaned, err := sanitizePolicyRecord(owner, stored)
	return cleaned, created, err
}

func (s *Service) CurrentPolicy(ctx context.Context, owner string) (PolicyRecord, error) {
	owner, err := validateServiceIdentity(owner)
	if err != nil {
		return PolicyRecord{}, err
	}
	if err := s.available(); err != nil {
		return PolicyRecord{}, err
	}
	record, err := s.repository.CurrentPolicy(ctx, owner)
	if err != nil {
		return PolicyRecord{}, err
	}
	record, err = sanitizePolicyRecord(owner, record)
	return record, err
}

func (s *Service) PolicyHistory(ctx context.Context, owner string, limit int) ([]PolicyRecord, error) {
	owner, err := validateServiceIdentity(owner)
	if err != nil {
		return nil, err
	}
	if err := s.available(); err != nil {
		return nil, err
	}
	items, err := s.repository.ListPolicies(ctx, owner, limit)
	if err != nil {
		return nil, err
	}
	result := make([]PolicyRecord, 0, len(items))
	for _, item := range items {
		cleaned, cleanErr := sanitizePolicyRecord(owner, item)
		if cleanErr != nil {
			return nil, cleanErr
		}
		result = append(result, cleaned)
	}
	return result, nil
}

func (s *Service) RecordSignals(ctx context.Context, owner, idempotencyKey string, signals []OpenLoopSignal) ([]SignalRecord, bool, error) {
	owner, err := validateServiceIdentity(owner)
	if err != nil {
		return nil, false, err
	}
	if err := validateIdempotencyKey(idempotencyKey); err != nil {
		return nil, false, err
	}
	if err := s.available(); err != nil {
		return nil, false, err
	}
	if len(signals) == 0 {
		return nil, false, errors.New("at least one proactivity signal is required")
	}
	if len(signals) > MaxSignals {
		return nil, false, fmt.Errorf("signal count exceeds %d", MaxSignals)
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	records := make([]SignalRecord, len(signals))
	seen := make(map[string]struct{}, len(signals))
	for index, signal := range signals {
		normalized, normalizeErr := normalizeSignal(owner, cloneSignal(signal), now)
		if normalizeErr != nil {
			return nil, false, fmt.Errorf("signal %d: %w", index, normalizeErr)
		}
		if _, exists := seen[normalized.ID]; exists {
			return nil, false, errors.New("signal ids must be unique")
		}
		seen[normalized.ID] = struct{}{}
		records[index] = SignalRecord{
			ContractVersion: ContractVersion,
			OwnerIdentity:   owner,
			Signal:          normalized,
			RecordedAt:      now,
		}
	}
	normalizedSignals := make([]OpenLoopSignal, len(records))
	for index := range records {
		normalizedSignals[index] = records[index].Signal
	}
	digest, err := advisoryDigest("signals", owner, normalizedSignals)
	if err != nil {
		return nil, false, fmt.Errorf("digest proactivity signals: %w", err)
	}
	stored, created, err := s.repository.RecordSignals(ctx, owner, strings.TrimSpace(idempotencyKey), digest, records)
	if err != nil {
		return nil, false, err
	}
	cleaned, err := sanitizeSignalRecords(owner, stored)
	return cleaned, created, err
}

func (s *Service) Signals(ctx context.Context, owner string, limit int) ([]SignalRecord, error) {
	owner, err := validateServiceIdentity(owner)
	if err != nil {
		return nil, err
	}
	if err := s.available(); err != nil {
		return nil, err
	}
	items, err := s.repository.ListSignals(ctx, owner, limit)
	if err != nil {
		return nil, err
	}
	return sanitizeSignalRecords(owner, items)
}

// EvaluateStored evaluates persisted owner-scoped state and records advisory
// decisions. It never invokes a delivery, execution, task, or approval system.
func (s *Service) EvaluateStored(ctx context.Context, owner string, request EvaluateStoredRequest) (DecisionBatch, bool, error) {
	owner, err := validateServiceIdentity(owner)
	if err != nil {
		return DecisionBatch{}, false, err
	}
	if err := validateIdempotencyKey(request.IdempotencyKey); err != nil {
		return DecisionBatch{}, false, err
	}
	if request.Now.IsZero() {
		return DecisionBatch{}, false, errors.New("evaluation time is required")
	}
	request.Now = request.Now.UTC().Truncate(time.Microsecond)
	if err := s.available(); err != nil {
		return DecisionBatch{}, false, err
	}
	digest, err := advisoryDigest("decisions", owner, request.Now)
	if err != nil {
		return DecisionBatch{}, false, fmt.Errorf("digest proactivity evaluation: %w", err)
	}
	key := strings.TrimSpace(request.IdempotencyKey)
	if existing, found, findErr := s.repository.FindDecisionBatch(ctx, owner, key, digest); findErr != nil {
		return DecisionBatch{}, false, findErr
	} else if found {
		cleaned, cleanErr := sanitizeDecisionBatch(owner, existing)
		return cleaned, false, cleanErr
	}

	policyRecord, err := s.repository.CurrentPolicy(ctx, owner)
	if err != nil {
		return DecisionBatch{}, false, err
	}
	policyRecord, err = sanitizePolicyRecord(owner, policyRecord)
	if err != nil {
		return DecisionBatch{}, false, err
	}
	signals, err := s.repository.LatestSignals(ctx, owner, MaxSignals)
	if err != nil {
		return DecisionBatch{}, false, err
	}
	for index := range signals {
		if signals[index].OwnerIdentity != owner {
			return DecisionBatch{}, false, ErrOwnerScopeViolation
		}
		signals[index] = cloneSignal(signals[index])
	}
	historyRecords, err := s.repository.ListDecisions(ctx, owner, MaxDecisionHistory)
	if err != nil {
		return DecisionBatch{}, false, err
	}
	history := make([]DecisionHistory, 0, len(historyRecords))
	for _, record := range historyRecords {
		if record.OwnerIdentity != owner || record.Decision.OwnerIdentity != owner {
			return DecisionBatch{}, false, ErrOwnerScopeViolation
		}
		history = append(history, DecisionHistory{
			ContractVersion: ContractVersion,
			OwnerIdentity:   owner,
			OpenLoopKey:     record.Decision.OpenLoopKey,
			SignalDigest:    record.Decision.SignalDigest,
			Outcome:         record.Decision.Outcome,
			DecidedAt:       record.Decision.DecidedAt,
		})
	}
	feedbackRecords, err := s.repository.ListFeedback(ctx, owner, MaxFeedbackHistory)
	if err != nil {
		return DecisionBatch{}, false, err
	}
	controls := make([]AttentionControl, 0, min(len(feedbackRecords), MaxSignals))
	seenControls := make(map[string]struct{}, MaxSignals)
	for _, record := range feedbackRecords {
		cleaned, cleanErr := sanitizeFeedbackRecord(owner, record)
		if cleanErr != nil {
			return DecisionBatch{}, false, cleanErr
		}
		if _, exists := seenControls[cleaned.OpenLoopKey]; exists {
			continue
		}
		seenControls[cleaned.OpenLoopKey] = struct{}{}
		controls = append(controls, AttentionControl{
			OpenLoopKey: cleaned.OpenLoopKey, SignalDigest: cleaned.SignalDigest,
			Action: cleaned.Action, SnoozedUntil: cloneTimePointer(cleaned.SnoozedUntil),
			RecordedAt: cleaned.RecordedAt,
		})
		if len(controls) == MaxSignals {
			break
		}
	}
	result, err := Evaluate(EvaluationRequest{
		ContractVersion: ContractVersion,
		OwnerIdentity:   owner,
		Now:             request.Now,
		Preferences:     policyRecord.Policy,
		Signals:         signals,
		History:         history,
		Controls:        controls,
	})
	if err != nil {
		return DecisionBatch{}, false, err
	}
	for index := range result.Decisions {
		result.Decisions[index] = advisoryDecision(result.Decisions[index])
	}
	batch := DecisionBatch{
		ContractVersion: ContractVersion,
		OwnerIdentity:   owner,
		Result:          result,
		RecordedAt:      s.now().UTC().Truncate(time.Microsecond),
	}
	stored, created, err := s.repository.RecordDecisionBatch(ctx, owner, key, digest, batch)
	if err != nil {
		return DecisionBatch{}, false, err
	}
	cleaned, err := sanitizeDecisionBatch(owner, stored)
	return cleaned, created, err
}

func (s *Service) Decisions(ctx context.Context, owner string, limit int) ([]DecisionRecord, error) {
	owner, err := validateServiceIdentity(owner)
	if err != nil {
		return nil, err
	}
	if err := s.available(); err != nil {
		return nil, err
	}
	items, err := s.repository.ListDecisions(ctx, owner, limit)
	if err != nil {
		return nil, err
	}
	result := make([]DecisionRecord, 0, len(items))
	for _, item := range items {
		cleaned, cleanErr := sanitizeDecisionRecord(owner, item)
		if cleanErr != nil {
			return nil, cleanErr
		}
		result = append(result, cleaned)
	}
	return result, nil
}

func (s *Service) available() error {
	if s == nil || repositoryIsNil(s.repository) || s.now == nil {
		return ErrRepositoryUnavailable
	}
	return nil
}

func repositoryIsNil(repository Repository) bool {
	if repository == nil {
		return true
	}
	value := reflect.ValueOf(repository)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func validateServiceIdentity(owner string) (string, error) {
	owner = strings.TrimSpace(owner)
	if err := validateIdentity(owner); err != nil {
		return "", err
	}
	return owner, nil
}

func validateIdempotencyKey(value string) error {
	if err := validateIdentifier("idempotency key", strings.TrimSpace(value)); err != nil {
		return err
	}
	return nil
}

func advisoryDigest(kind, owner string, value any) (string, error) {
	payload := struct {
		Kind  string `json:"kind"`
		Owner string `json:"owner"`
		Value any    `json:"value"`
	}{kind, owner, value}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func sanitizePolicyRecord(owner string, record PolicyRecord) (PolicyRecord, error) {
	if err := validatePolicyRecordOwner(owner, record); err != nil {
		return PolicyRecord{}, err
	}
	if record.ContractVersion != ContractVersion || record.RecordedAt.IsZero() {
		return PolicyRecord{}, errors.New("proactivity policy record is invalid")
	}
	policy, _, err := normalizePreferences(owner, clonePreferences(record.Policy))
	if err != nil {
		return PolicyRecord{}, err
	}
	record.Policy = policy
	record.RecordedAt = record.RecordedAt.UTC()
	return record, nil
}

func sanitizeSignalRecords(owner string, records []SignalRecord) ([]SignalRecord, error) {
	result := make([]SignalRecord, len(records))
	for index, record := range records {
		if err := validateSignalRecordOwner(owner, record); err != nil {
			return nil, err
		}
		if record.ContractVersion != ContractVersion || record.RecordedAt.IsZero() {
			return nil, errors.New("proactivity signal record is invalid")
		}
		record = cloneSignalRecord(record)
		record.Signal.Title = redactAndBound(record.Signal.Title, maxTitleLength)
		record.Signal.Summary = redactAndBound(record.Signal.Summary, maxSummaryLength)
		record.RecordedAt = record.RecordedAt.UTC()
		result[index] = record
	}
	return result, nil
}

func sanitizeDecisionBatch(owner string, batch DecisionBatch) (DecisionBatch, error) {
	if batch.OwnerIdentity != owner || batch.Result.OwnerIdentity != owner {
		return DecisionBatch{}, ErrOwnerScopeViolation
	}
	if batch.ContractVersion != ContractVersion || batch.Result.ContractVersion != ContractVersion || batch.RecordedAt.IsZero() {
		return DecisionBatch{}, errors.New("proactivity decision batch is invalid")
	}
	if (batch.SnapshotInputDigest != "" && !digestPattern.MatchString(batch.SnapshotInputDigest)) ||
		(batch.AdditionalSignalsDigest != "" && !digestPattern.MatchString(batch.AdditionalSignalsDigest)) {
		return DecisionBatch{}, errors.New("proactivity decision snapshot digest is invalid")
	}
	batch = cloneDecisionBatch(batch)
	for index := range batch.Result.Decisions {
		if batch.Result.Decisions[index].OwnerIdentity != owner {
			return DecisionBatch{}, ErrOwnerScopeViolation
		}
		batch.Result.Decisions[index] = advisoryDecision(batch.Result.Decisions[index])
	}
	batch.RecordedAt = batch.RecordedAt.UTC()
	return batch, nil
}

func sanitizeDecisionRecord(owner string, record DecisionRecord) (DecisionRecord, error) {
	if record.OwnerIdentity != owner || record.Decision.OwnerIdentity != owner {
		return DecisionRecord{}, ErrOwnerScopeViolation
	}
	if record.ContractVersion != ContractVersion || record.Decision.ContractVersion != ContractVersion || record.RecordedAt.IsZero() {
		return DecisionRecord{}, errors.New("proactivity decision record is invalid")
	}
	record = cloneDecisionRecord(record)
	record.Decision = advisoryDecision(record.Decision)
	record.RecordedAt = record.RecordedAt.UTC()
	return record, nil
}

func advisoryDecision(value Decision) Decision {
	value = cloneDecision(value)
	value.Title = redactAndBound(value.Title, maxTitleLength)
	value.Summary = redactAndBound(value.Summary, maxSummaryLength)
	for index := range value.Components {
		value.Components[index].Name = redactAndBound(value.Components[index].Name, 80)
		value.Components[index].Explanation = redactAndBound(value.Components[index].Explanation, 300)
	}
	for index := range value.Reasons {
		value.Reasons[index] = redactAndBound(value.Reasons[index], 500)
	}
	value.ExecutionAuthorized = false
	value.DeliveryAuthorized = false
	value.AuthorityGranted = false
	value.DecidedAt = value.DecidedAt.UTC()
	return value
}
