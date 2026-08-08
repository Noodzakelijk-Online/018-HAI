package proactivity

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"time"
)

func (r *MemoryRepository) captureEvaluationSnapshotState(ctx context.Context, owner string, at time.Time) (evaluationSnapshotState, error) {
	if err := repositoryContext(ctx); err != nil {
		return evaluationSnapshotState{}, err
	}
	if r == nil {
		return evaluationSnapshotState{}, ErrRepositoryUnavailable
	}
	owner, err := repositoryOwner(owner)
	if err != nil {
		return evaluationSnapshotState{}, err
	}
	at = snapshotTime(at)
	r.mu.RLock()
	defer r.mu.RUnlock()

	state, err := r.memorySnapshotStateLocked(owner, at, nil, nil, nil)
	if err != nil {
		return evaluationSnapshotState{}, err
	}
	if err := r.validateMemorySnapshotMetadataLocked(owner, state); err != nil {
		return evaluationSnapshotState{}, err
	}
	if err := validateEvaluationSnapshotState(owner, at, state); err != nil {
		return evaluationSnapshotState{}, err
	}
	return canonicalEvaluationSnapshotState(state), nil
}

func (r *MemoryRepository) resolveEvaluationSnapshotState(ctx context.Context, owner string, snapshot EvaluationSnapshot) (evaluationSnapshotState, error) {
	if err := repositoryContext(ctx); err != nil {
		return evaluationSnapshotState{}, err
	}
	if r == nil {
		return evaluationSnapshotState{}, ErrRepositoryUnavailable
	}
	owner, err := repositoryOwner(owner)
	if err != nil {
		return evaluationSnapshotState{}, err
	}
	snapshot, err = validateEvaluationSnapshot(owner, snapshot)
	if err != nil {
		return evaluationSnapshotState{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	state, err := r.memorySnapshotStateLocked(owner, snapshot.CapturedAt, &snapshot.Signals, &snapshot.Decisions, &snapshot.Feedback)
	if err != nil {
		return evaluationSnapshotState{}, err
	}
	policyFound := false
	for index, reference := range r.policyReferences[owner] {
		if index >= len(r.policies[owner]) {
			return evaluationSnapshotState{}, fmt.Errorf("%w: policy metadata is incomplete", ErrSnapshotUnavailable)
		}
		if policySnapshotReferenceEqual(reference, snapshot.Policy) {
			state.Policy = snapshotPolicyMaterial{Reference: reference, Record: clonePolicyRecord(r.policies[owner][index])}
			policyFound = true
			break
		}
	}
	if !policyFound {
		return evaluationSnapshotState{}, fmt.Errorf("%w: exact policy was not found", ErrSnapshotUnavailable)
	}
	if err := r.validateMemorySnapshotMetadataLocked(owner, state); err != nil {
		return evaluationSnapshotState{}, err
	}
	if err := validateEvaluationSnapshotState(owner, snapshot.CapturedAt, state); err != nil {
		return evaluationSnapshotState{}, err
	}
	if err := verifySnapshotState(owner, snapshot, state); err != nil {
		return evaluationSnapshotState{}, err
	}
	return canonicalEvaluationSnapshotState(state), nil
}

func (r *MemoryRepository) validateMemorySnapshotMetadataLocked(owner string, state evaluationSnapshotState) error {
	entries := r.idempotency[owner]
	policyEntry, found := entries[state.Policy.Reference.IdempotencyKey]
	if !found || policyEntry.kind != idempotencyKindPolicy || policyEntry.digest != state.Policy.Reference.PayloadDigest ||
		policyEntry.policy == nil || !reflect.DeepEqual(*policyEntry.policy, state.Policy.Record) {
		return fmt.Errorf("%w: policy idempotency metadata mismatch", ErrSnapshotUnavailable)
	}
	for index, item := range state.Signals {
		entry, exists := entries[item.Cursor.IdempotencyKey]
		if !exists || entry.kind != idempotencyKindSignals || entry.digest != item.Cursor.PayloadDigest ||
			item.Cursor.Ordinal < 0 || item.Cursor.Ordinal >= len(entry.signals) ||
			!reflect.DeepEqual(entry.signals[item.Cursor.Ordinal], item.Record) {
			return fmt.Errorf("%w: signal %d payload metadata mismatch", ErrSnapshotUnavailable, index)
		}
	}
	for index, item := range state.Decisions {
		entry, exists := entries[item.Cursor.IdempotencyKey]
		if !exists || entry.kind != idempotencyKindDecisions || entry.digest != item.Cursor.PayloadDigest ||
			entry.decisions == nil || item.Cursor.Ordinal < 0 || item.Cursor.Ordinal >= len(entry.decisions.Result.Decisions) {
			return fmt.Errorf("%w: decision %d payload metadata mismatch", ErrSnapshotUnavailable, index)
		}
		expected := DecisionRecord{
			ContractVersion: ContractVersion, OwnerIdentity: owner,
			Decision:   cloneDecision(entry.decisions.Result.Decisions[item.Cursor.Ordinal]),
			RecordedAt: entry.decisions.RecordedAt.UTC(),
		}
		if !reflect.DeepEqual(expected, item.Record) {
			return fmt.Errorf("%w: decision %d payload record mismatch", ErrSnapshotUnavailable, index)
		}
	}
	for index, item := range state.Feedback {
		entry, exists := entries[item.Cursor.IdempotencyKey]
		if !exists || entry.kind != "feedback" || entry.digest != item.Cursor.PayloadDigest ||
			entry.feedback == nil || entry.feedback.RecordDigest != item.Cursor.RecordDigest ||
			!reflect.DeepEqual(*entry.feedback, item.Record) {
			return fmt.Errorf("%w: feedback %d payload metadata mismatch", ErrSnapshotUnavailable, index)
		}
	}
	return nil
}

func (r *MemoryRepository) memorySnapshotStateLocked(
	owner string,
	at time.Time,
	signals *SnapshotWatermark,
	decisions *SnapshotWatermark,
	feedback *FeedbackSnapshotWatermark,
) (evaluationSnapshotState, error) {
	if len(r.policies[owner]) != len(r.policyReferences[owner]) ||
		len(r.signals[owner]) != len(r.signalCursors[owner]) ||
		len(r.decisions[owner]) != len(r.decisionCursors[owner]) ||
		len(r.feedback[owner]) != len(r.feedbackCursors[owner]) {
		return evaluationSnapshotState{}, fmt.Errorf("%w: ledger metadata is incomplete", ErrSnapshotUnavailable)
	}

	state := evaluationSnapshotState{}
	for index, record := range r.policies[owner] {
		reference := r.policyReferences[owner][index]
		if snapshotTime(record.RecordedAt).After(at) || snapshotTime(reference.RecordedAt).After(at) {
			continue
		}
		candidate := snapshotPolicyMaterial{Reference: reference, Record: clonePolicyRecord(record)}
		if state.Policy.Record.OwnerIdentity == "" || policyReferenceAfter(candidate.Reference, state.Policy.Reference) {
			state.Policy = candidate
		}
	}
	if state.Policy.Record.OwnerIdentity == "" {
		return evaluationSnapshotState{}, ErrNotFound
	}

	for index, record := range r.signals[owner] {
		cursor := r.signalCursors[owner][index]
		if snapshotTime(cursor.RecordedAt).After(at) || (signals != nil && !snapshotCursorWithin(cursor, signals.Cursor)) {
			continue
		}
		state.Signals = append(state.Signals, snapshotSignalMaterial{Cursor: cursor, Record: cloneSignalRecord(record)})
	}
	sort.Slice(state.Signals, func(i, j int) bool {
		return compareSnapshotRecordCursor(state.Signals[i].Cursor, state.Signals[j].Cursor) > 0
	})
	signalLimit := MaxSignalHistory
	if signals != nil {
		signalLimit = signals.Count
	}
	if len(state.Signals) > signalLimit {
		state.Signals = state.Signals[:signalLimit]
	}

	for index, record := range r.decisions[owner] {
		cursor := r.decisionCursors[owner][index]
		if snapshotTime(cursor.RecordedAt).After(at) || (decisions != nil && !snapshotCursorWithin(cursor, decisions.Cursor)) {
			continue
		}
		state.Decisions = append(state.Decisions, snapshotDecisionMaterial{Cursor: cursor, Record: cloneDecisionRecord(record)})
	}
	sort.Slice(state.Decisions, func(i, j int) bool {
		return compareSnapshotRecordCursor(state.Decisions[i].Cursor, state.Decisions[j].Cursor) > 0
	})
	decisionLimit := MaxDecisionHistory
	if decisions != nil {
		decisionLimit = decisions.Count
	}
	if len(state.Decisions) > decisionLimit {
		state.Decisions = state.Decisions[:decisionLimit]
	}

	for index, record := range r.feedback[owner] {
		cursor := r.feedbackCursors[owner][index]
		if snapshotTime(cursor.RecordedAt).After(at) || (feedback != nil && !feedbackCursorWithin(cursor, feedback.Cursor)) {
			continue
		}
		state.Feedback = append(state.Feedback, snapshotFeedbackMaterial{Cursor: cursor, Record: cloneFeedbackRecord(record)})
	}
	sort.Slice(state.Feedback, func(i, j int) bool {
		return compareFeedbackSnapshotCursor(state.Feedback[i].Cursor, state.Feedback[j].Cursor) > 0
	})
	feedbackLimit := MaxFeedbackHistory
	if feedback != nil {
		feedbackLimit = feedback.Count
	}
	if len(state.Feedback) > feedbackLimit {
		state.Feedback = state.Feedback[:feedbackLimit]
	}
	return state, nil
}

func snapshotCursorWithin(value SnapshotRecordCursor, upper *SnapshotRecordCursor) bool {
	return upper != nil && compareSnapshotRecordCursor(value, *upper) <= 0
}

func feedbackCursorWithin(value FeedbackSnapshotCursor, upper *FeedbackSnapshotCursor) bool {
	return upper != nil && compareFeedbackSnapshotCursor(value, *upper) <= 0
}

func policyReferenceAfter(left, right PolicySnapshotReference) bool {
	left = canonicalPolicySnapshotReference(left)
	right = canonicalPolicySnapshotReference(right)
	if left.RecordedAt.After(right.RecordedAt) {
		return true
	}
	return left.RecordedAt.Equal(right.RecordedAt) && left.IdempotencyKey > right.IdempotencyKey
}

func policySnapshotReferenceEqual(left, right PolicySnapshotReference) bool {
	left = canonicalPolicySnapshotReference(left)
	right = canonicalPolicySnapshotReference(right)
	return left.IdempotencyKey == right.IdempotencyKey && left.PayloadDigest == right.PayloadDigest && left.RecordedAt.Equal(right.RecordedAt)
}

func cloneEvaluationSnapshotState(value evaluationSnapshotState) evaluationSnapshotState {
	result := evaluationSnapshotState{
		Policy:    snapshotPolicyMaterial{Reference: value.Policy.Reference, Record: clonePolicyRecord(value.Policy.Record)},
		Signals:   make([]snapshotSignalMaterial, len(value.Signals)),
		Decisions: make([]snapshotDecisionMaterial, len(value.Decisions)),
		Feedback:  make([]snapshotFeedbackMaterial, len(value.Feedback)),
	}
	for index, item := range value.Signals {
		result.Signals[index] = snapshotSignalMaterial{Cursor: item.Cursor, Record: cloneSignalRecord(item.Record)}
	}
	for index, item := range value.Decisions {
		result.Decisions[index] = snapshotDecisionMaterial{Cursor: item.Cursor, Record: cloneDecisionRecord(item.Record)}
	}
	for index, item := range value.Feedback {
		result.Feedback[index] = snapshotFeedbackMaterial{Cursor: item.Cursor, Record: cloneFeedbackRecord(item.Record)}
	}
	return result
}
