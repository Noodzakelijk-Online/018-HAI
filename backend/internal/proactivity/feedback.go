package proactivity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	FeedbackAuthority  = "attention_feedback_only"
	MaxFeedbackHistory = 2048
)

type FeedbackAction string

const (
	FeedbackAccept   FeedbackAction = "accept"
	FeedbackDismiss  FeedbackAction = "dismiss"
	FeedbackSnooze   FeedbackAction = "snooze"
	FeedbackSuppress FeedbackAction = "suppress"
	FeedbackResume   FeedbackAction = "resume"
)

type FeedbackRequest struct {
	IdempotencyKey string         `json:"idempotencyKey"`
	SignalID       string         `json:"signalId"`
	OpenLoopKey    string         `json:"openLoopKey"`
	SignalDigest   string         `json:"signalDigest"`
	Action         FeedbackAction `json:"action"`
	Reason         string         `json:"reason"`
	SnoozedUntil   *time.Time     `json:"snoozedUntil,omitempty"`
}

type FeedbackRecord struct {
	ContractVersion      int            `json:"contractVersion"`
	ID                   string         `json:"id"`
	OwnerIdentity        string         `json:"ownerIdentity"`
	SignalID             string         `json:"signalId"`
	OpenLoopKey          string         `json:"openLoopKey"`
	SignalDigest         string         `json:"signalDigest"`
	SourceOutcome        Outcome        `json:"sourceOutcome"`
	SourceDecisionAt     time.Time      `json:"sourceDecisionAt"`
	Action               FeedbackAction `json:"action"`
	Reason               string         `json:"reason"`
	SnoozedUntil         *time.Time     `json:"snoozedUntil,omitempty"`
	PreviousRecordDigest string         `json:"previousRecordDigest,omitempty"`
	RecordDigest         string         `json:"recordDigest"`
	RecordedAt           time.Time      `json:"recordedAt"`
	Authority            string         `json:"authority"`
	CanExecute           bool           `json:"canExecute"`
	DeliveryAuthorized   bool           `json:"deliveryAuthorized"`
	ExecutionAuthorized  bool           `json:"executionAuthorized"`
}

type AttentionControl struct {
	OpenLoopKey  string         `json:"openLoopKey"`
	SignalDigest string         `json:"signalDigest"`
	Action       FeedbackAction `json:"action"`
	SnoozedUntil *time.Time     `json:"snoozedUntil,omitempty"`
	RecordedAt   time.Time      `json:"recordedAt"`
}

func (s *Service) RecordFeedback(ctx context.Context, owner string, request FeedbackRequest) (FeedbackRecord, bool, error) {
	owner, err := validateServiceIdentity(owner)
	if err != nil {
		return FeedbackRecord{}, false, err
	}
	if err := validateIdempotencyKey(request.IdempotencyKey); err != nil {
		return FeedbackRecord{}, false, err
	}
	if err := s.available(); err != nil {
		return FeedbackRecord{}, false, err
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	request, err = normalizeFeedbackRequest(request, now)
	if err != nil {
		return FeedbackRecord{}, false, err
	}

	decisions, err := s.repository.ListDecisions(ctx, owner, MaxDecisionHistory)
	if err != nil {
		return FeedbackRecord{}, false, err
	}
	decision, found := latestDecisionForOpenLoop(decisions, request.OpenLoopKey)
	if !found || decision.Decision.SignalID != request.SignalID || decision.Decision.SignalDigest != request.SignalDigest {
		return FeedbackRecord{}, false, ErrNotFound
	}
	if decision.Decision.Outcome == OutcomeSuppress {
		return FeedbackRecord{}, false, errors.New("suppressed advisory decisions do not accept owner feedback")
	}

	latest, found, err := s.repository.LatestFeedback(ctx, owner, request.OpenLoopKey)
	if err != nil {
		return FeedbackRecord{}, false, err
	}
	previousDigest := ""
	if found {
		previousDigest = latest.RecordDigest
		if !now.After(latest.RecordedAt) {
			now = latest.RecordedAt.Add(time.Microsecond)
		}
	}
	record := FeedbackRecord{
		ContractVersion:      ContractVersion,
		ID:                   uuid.NewString(),
		OwnerIdentity:        owner,
		SignalID:             request.SignalID,
		OpenLoopKey:          request.OpenLoopKey,
		SignalDigest:         request.SignalDigest,
		SourceOutcome:        decision.Decision.Outcome,
		SourceDecisionAt:     decision.Decision.DecidedAt.UTC().Truncate(time.Microsecond),
		Action:               request.Action,
		Reason:               request.Reason,
		SnoozedUntil:         cloneTimePointer(request.SnoozedUntil),
		PreviousRecordDigest: previousDigest,
		RecordedAt:           now,
		Authority:            FeedbackAuthority,
		CanExecute:           false,
		DeliveryAuthorized:   false,
		ExecutionAuthorized:  false,
	}
	record.RecordDigest, err = feedbackRecordDigest(record)
	if err != nil {
		return FeedbackRecord{}, false, fmt.Errorf("digest proactivity feedback: %w", err)
	}
	requestDigest, err := feedbackRequestDigest(owner, request)
	if err != nil {
		return FeedbackRecord{}, false, fmt.Errorf("digest proactivity feedback request: %w", err)
	}
	stored, created, err := s.repository.RecordFeedback(
		ctx, owner, strings.TrimSpace(request.IdempotencyKey), requestDigest, record,
	)
	if err != nil {
		return FeedbackRecord{}, false, err
	}
	cleaned, err := sanitizeFeedbackRecord(owner, stored)
	return cleaned, created, err
}

func (s *Service) Feedback(ctx context.Context, owner string, limit int) ([]FeedbackRecord, error) {
	owner, err := validateServiceIdentity(owner)
	if err != nil {
		return nil, err
	}
	if err := s.available(); err != nil {
		return nil, err
	}
	records, err := s.repository.ListFeedback(ctx, owner, limit)
	if err != nil {
		return nil, err
	}
	result := make([]FeedbackRecord, len(records))
	for index, record := range records {
		cleaned, cleanErr := sanitizeFeedbackRecord(owner, record)
		if cleanErr != nil {
			return nil, cleanErr
		}
		result[index] = cleaned
	}
	return result, nil
}

func normalizeFeedbackRequest(request FeedbackRequest, now time.Time) (FeedbackRequest, error) {
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	request.SignalID = strings.TrimSpace(request.SignalID)
	request.OpenLoopKey = strings.TrimSpace(request.OpenLoopKey)
	request.SignalDigest = strings.ToLower(strings.TrimSpace(request.SignalDigest))
	request.Reason = strings.Join(strings.Fields(strings.TrimSpace(request.Reason)), " ")
	if err := validateIdentifier("signal id", request.SignalID); err != nil {
		return FeedbackRequest{}, err
	}
	if err := validateIdentifier("open loop key", request.OpenLoopKey); err != nil {
		return FeedbackRequest{}, err
	}
	if !digestPattern.MatchString(request.SignalDigest) {
		return FeedbackRequest{}, errors.New("signal digest is invalid")
	}
	if !validFeedbackAction(request.Action) {
		return FeedbackRequest{}, errors.New("feedback action is unsupported")
	}
	if request.Reason == "" || len([]rune(request.Reason)) > 1000 {
		return FeedbackRequest{}, errors.New("feedback reason must contain between 1 and 1000 characters")
	}
	if containsSecret(request.Reason) {
		return FeedbackRequest{}, errors.New("feedback reason contains secret material")
	}
	if request.Action == FeedbackSnooze {
		if request.SnoozedUntil == nil {
			return FeedbackRequest{}, errors.New("snoozed until is required for snooze feedback")
		}
		value := request.SnoozedUntil.UTC().Truncate(time.Microsecond)
		if value.Before(now.Add(5*time.Minute)) || value.After(now.Add(30*24*time.Hour)) {
			return FeedbackRequest{}, errors.New("snoozed until must be between 5 minutes and 30 days in the future")
		}
		request.SnoozedUntil = &value
	} else if request.SnoozedUntil != nil {
		return FeedbackRequest{}, errors.New("snoozed until is only allowed for snooze feedback")
	}
	return request, nil
}

func latestDecisionForOpenLoop(records []DecisionRecord, key string) (DecisionRecord, bool) {
	for _, record := range records {
		if record.Decision.OpenLoopKey == key {
			return record, true
		}
	}
	return DecisionRecord{}, false
}

func sanitizeFeedbackRecord(owner string, record FeedbackRecord) (FeedbackRecord, error) {
	if record.OwnerIdentity != owner {
		return FeedbackRecord{}, ErrOwnerScopeViolation
	}
	if record.ContractVersion != ContractVersion || record.ID == "" || record.RecordedAt.IsZero() ||
		record.Authority != FeedbackAuthority || record.CanExecute || record.DeliveryAuthorized || record.ExecutionAuthorized ||
		!validFeedbackAction(record.Action) || !digestPattern.MatchString(record.SignalDigest) ||
		!digestPattern.MatchString(record.RecordDigest) {
		return FeedbackRecord{}, errors.New("proactivity feedback record is invalid")
	}
	expected, err := feedbackRecordDigest(record)
	if err != nil || expected != record.RecordDigest {
		return FeedbackRecord{}, errors.New("proactivity feedback record digest is invalid")
	}
	record.Reason = redactAndBound(record.Reason, 1000)
	record.RecordedAt = record.RecordedAt.UTC()
	record.SourceDecisionAt = record.SourceDecisionAt.UTC()
	record.SnoozedUntil = cloneTimePointer(record.SnoozedUntil)
	return record, nil
}

func validFeedbackAction(value FeedbackAction) bool {
	switch value {
	case FeedbackAccept, FeedbackDismiss, FeedbackSnooze, FeedbackSuppress, FeedbackResume:
		return true
	default:
		return false
	}
}

func feedbackRequestDigest(owner string, request FeedbackRequest) (string, error) {
	return advisoryDigest("attention-feedback-request", owner, request)
}

func feedbackRecordDigest(record FeedbackRecord) (string, error) {
	projection := record
	projection.RecordDigest = ""
	return advisoryDigest("attention-feedback-record", record.OwnerIdentity, projection)
}

func cloneFeedbackRecord(value FeedbackRecord) FeedbackRecord {
	value.SnoozedUntil = cloneTimePointer(value.SnoozedUntil)
	return value
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}
