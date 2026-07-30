package proactive

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Clock func() time.Time

type Service struct {
	repository Repository
	clock      Clock
}

func NewService(repository Repository, clock Clock) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("proactive repository is required")
	}
	if clock == nil {
		clock = time.Now
	}
	return &Service{repository: repository, clock: clock}, nil
}

func (s *Service) PutRule(ctx context.Context, rule TriggerRule) (TriggerRule, error) {
	now := s.clock().UTC()
	normalized, err := normalizeRule(rule, now)
	if err != nil {
		return TriggerRule{}, err
	}
	if err := s.repository.PutRule(ctx, normalized); err != nil {
		return TriggerRule{}, err
	}
	return cloneRule(normalized), nil
}

func (s *Service) Evaluate(ctx context.Context, owner, ruleID string, signal Signal) (EvaluationResult, error) {
	now := s.clock().UTC()
	if err := validateIdentity(owner); err != nil {
		return EvaluationResult{}, err
	}
	if err := validateIdentifier("rule id", ruleID); err != nil {
		return EvaluationResult{}, err
	}
	signal.OwnerIdentity = strings.TrimSpace(signal.OwnerIdentity)
	if signal.OwnerIdentity != strings.TrimSpace(owner) {
		return EvaluationResult{}, fmt.Errorf("signal owner does not match authenticated owner")
	}
	if err := validateSignal(signal, now); err != nil {
		return EvaluationResult{}, err
	}
	rule, err := s.repository.GetRule(ctx, owner, ruleID)
	if err != nil {
		return EvaluationResult{}, err
	}
	if !rule.Enabled {
		return suppressed(SuppressionRuleDisabled, "the trigger rule is disabled"), nil
	}
	if !containsSignalType(rule.SignalTypes, signal.Type) {
		return suppressed(SuppressionTypeMismatch, "the trigger rule does not accept this signal type"), nil
	}
	if signal.ResolvedAt != nil {
		return suppressed(SuppressionResolved, "the signal is already resolved"), nil
	}
	resolved, err := s.repository.IsOpenLoopResolved(ctx, owner, signal.OpenLoopKey)
	if err != nil {
		return EvaluationResult{}, err
	}
	if resolved {
		return suppressed(SuppressionResolved, "the open loop was already resolved"), nil
	}
	if signal.Sensitivity != SensitivityStandard {
		return suppressed(SuppressionSensitive, "sensitive signals require explicit review outside proactive proposal generation"), nil
	}
	if signal.Confidence < rule.MinimumConfidence {
		return suppressed(SuppressionLowConfidence, "signal confidence is below the rule threshold"), nil
	}
	evidence, sourceSuppression := evaluateEvidence(signal.Sources, rule.MaximumSourceAge, now)
	if sourceSuppression != SuppressionNone {
		reason := "source evidence is not fresh enough"
		if sourceSuppression == SuppressionUncertain {
			reason = "source evidence is uncertain, conflicting, or unsupported"
		}
		return suppressed(sourceSuppression, reason), nil
	}
	signalDigest, err := digestValue(signal)
	if err != nil {
		return EvaluationResult{}, err
	}
	existing, err := s.repository.FindByIdempotency(ctx, owner, signal.IdempotencyKey)
	if err == nil {
		if existing.SignalDigest != signalDigest {
			return EvaluationResult{}, ErrIdempotencyConflict
		}
		copy := cloneProposal(existing)
		return EvaluationResult{
			Proposal:         &copy,
			Reason:           "the original proposal was returned for an exact idempotent replay",
			IdempotentReplay: true,
			Deferred:         copy.NotifyAfter.After(now),
		}, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return EvaluationResult{}, err
	}
	latest, err := s.repository.LatestByOpenLoop(ctx, owner, signal.OpenLoopKey)
	if err == nil && rule.Cooldown > 0 && now.Before(latest.CreatedAt.Add(rule.Cooldown)) {
		return suppressed(SuppressionCooldown, "a related proposal is still within its cooldown window"), nil
	}
	if err != nil && !errors.Is(err, ErrNotFound) {
		return EvaluationResult{}, err
	}
	weights := rule.Weights
	if learned, weightsErr := s.repository.GetWeights(ctx, owner); weightsErr == nil {
		weights = learned
	} else if !errors.Is(weightsErr, ErrNotFound) {
		return EvaluationResult{}, weightsErr
	}
	score := scoreSignal(signal, weights, now)
	notifyAfter, deferred, err := nextNotificationTime(rule.QuietHours, now)
	if err != nil {
		return EvaluationResult{}, err
	}
	approvalRequired, approvalReason := approvalRequirement(signal)
	action := recommendedAction(signal)
	if action.ExternalEffect {
		return EvaluationResult{}, fmt.Errorf("proactive actions may not produce external effects")
	}
	proposalID := deterministicID(owner, signal.IdempotencyKey, signalDigest, rule.ID, rule.Digest)
	expiresAt := now.Add(rule.ProposalTTL)
	nextReview := now.Add(rule.Retry.Intervals[0])
	if signal.DueAt != nil && signal.DueAt.Before(nextReview) && signal.DueAt.After(now) {
		nextReview = *signal.DueAt
	}
	proposal := Proposal{
		ContractVersion:  ContractVersion,
		ID:               proposalID,
		OwnerIdentity:    owner,
		SignalID:         signal.ID,
		SignalDigest:     signalDigest,
		IdempotencyKey:   signal.IdempotencyKey,
		OpenLoopKey:      signal.OpenLoopKey,
		RuleID:           rule.ID,
		RuleVersion:      rule.Version,
		RuleDigest:       rule.Digest,
		Title:            signal.Title,
		Summary:          signal.Summary,
		Status:           StatusProposed,
		Responsible:      signal.Responsible,
		Risk:             signal.Risk,
		Domain:           signal.Domain,
		ApprovalRequired: approvalRequired,
		ApprovalReason:   approvalReason,
		ExecutionAllowed: false,
		Action:           action,
		Evidence:         evidence,
		Score:            score,
		CreatedAt:        now,
		UpdatedAt:        now,
		ExpiresAt:        expiresAt,
		NotifyAfter:      notifyAfter,
		NextReviewAt:     &nextReview,
		Revision:         1,
	}
	if err := s.repository.CreateProposal(ctx, proposal); err != nil {
		if errors.Is(err, ErrAlreadyExists) {
			existing, getErr := s.repository.FindByIdempotency(ctx, owner, signal.IdempotencyKey)
			if getErr != nil {
				return EvaluationResult{}, getErr
			}
			copy := cloneProposal(existing)
			return EvaluationResult{Proposal: &copy, IdempotentReplay: true, Reason: "concurrent replay returned the existing proposal"}, nil
		}
		return EvaluationResult{}, err
	}
	copy := cloneProposal(proposal)
	return EvaluationResult{
		Proposal: &copy,
		Reason:   "fresh, supported signal produced a reviewable proposal",
		Deferred: deferred,
	}, nil
}

func (s *Service) GetProposal(ctx context.Context, owner, id string) (Proposal, error) {
	if err := validateIdentity(owner); err != nil {
		return Proposal{}, err
	}
	if err := validateIdentifier("proposal id", id); err != nil {
		return Proposal{}, err
	}
	return s.repository.GetProposal(ctx, strings.TrimSpace(owner), strings.TrimSpace(id))
}

func (s *Service) ListProposals(ctx context.Context, owner string, filter ProposalFilter) ([]Proposal, error) {
	if err := validateIdentity(owner); err != nil {
		return nil, err
	}
	if filter.Limit < 0 || filter.Limit > 1000 {
		return nil, fmt.Errorf("proposal limit must be between 0 and 1000")
	}
	return s.repository.ListProposals(ctx, strings.TrimSpace(owner), filter)
}

func (s *Service) Transition(ctx context.Context, owner, id string, expectedRevision uint64, to ProposalStatus) (Proposal, error) {
	current, err := s.GetProposal(ctx, owner, id)
	if err != nil {
		return Proposal{}, err
	}
	if current.Revision != expectedRevision {
		return Proposal{}, ErrConflict
	}
	if !transitionAllowed(current.Status, to) {
		return Proposal{}, fmt.Errorf("%w: %s to %s", ErrInvalidTransition, current.Status, to)
	}
	now := s.clock().UTC()
	updated := cloneProposal(current)
	updated.Status = to
	updated.UpdatedAt = now
	updated.Revision++
	if to == StatusResolved {
		updated.NextReviewAt = nil
		if err := s.repository.MarkOpenLoopResolved(ctx, owner, current.OpenLoopKey); err != nil {
			return Proposal{}, err
		}
	}
	if to.Terminal() {
		updated.NextReviewAt = nil
	}
	return s.repository.CompareAndSwapProposal(ctx, updated, expectedRevision)
}

func (s *Service) Snooze(ctx context.Context, owner, id string, expectedRevision uint64, until time.Time) (Proposal, error) {
	current, err := s.GetProposal(ctx, owner, id)
	if err != nil {
		return Proposal{}, err
	}
	now := s.clock().UTC()
	until = until.UTC()
	if current.Revision != expectedRevision {
		return Proposal{}, ErrConflict
	}
	if !transitionAllowed(current.Status, StatusSnoozed) {
		return Proposal{}, fmt.Errorf("%w: %s to %s", ErrInvalidTransition, current.Status, StatusSnoozed)
	}
	if !until.After(now) || until.After(current.ExpiresAt) {
		return Proposal{}, fmt.Errorf("snooze time must be after now and before proposal expiry")
	}
	updated := cloneProposal(current)
	updated.Status = StatusSnoozed
	updated.SnoozedUntil = &until
	updated.NextReviewAt = &until
	updated.UpdatedAt = now
	updated.Revision++
	return s.repository.CompareAndSwapProposal(ctx, updated, expectedRevision)
}

func (s *Service) RecordReviewAttempt(ctx context.Context, owner, id string, expectedRevision uint64) (Proposal, error) {
	current, err := s.GetProposal(ctx, owner, id)
	if err != nil {
		return Proposal{}, err
	}
	if current.Revision != expectedRevision {
		return Proposal{}, ErrConflict
	}
	if current.Status.Terminal() {
		return Proposal{}, ErrScheduleExhausted
	}
	rule, err := s.repository.GetRule(ctx, owner, current.RuleID)
	if err != nil {
		return Proposal{}, err
	}
	if rule.Version != current.RuleVersion || rule.Digest != current.RuleDigest {
		return Proposal{}, fmt.Errorf("proposal rule snapshot no longer matches the configured rule")
	}
	now := s.clock().UTC()
	updated := cloneProposal(current)
	updated.ReviewAttempts++
	updated.UpdatedAt = now
	updated.Revision++
	if updated.ReviewAttempts >= rule.Retry.MaxAttempts {
		if updated.EscalationCount >= rule.Retry.MaxEscalations {
			updated.Status = StatusExpired
			updated.NextReviewAt = nil
			saved, saveErr := s.repository.CompareAndSwapProposal(ctx, updated, expectedRevision)
			if saveErr != nil {
				return Proposal{}, saveErr
			}
			return saved, ErrScheduleExhausted
		}
		updated.EscalationCount++
		updated.ReviewAttempts = 0
	}
	intervalIndex := updated.ReviewAttempts
	if intervalIndex >= len(rule.Retry.Intervals) {
		intervalIndex = len(rule.Retry.Intervals) - 1
	}
	next := now.Add(rule.Retry.Intervals[intervalIndex])
	if next.After(updated.ExpiresAt) {
		next = updated.ExpiresAt
	}
	updated.NextReviewAt = &next
	return s.repository.CompareAndSwapProposal(ctx, updated, expectedRevision)
}

func (s *Service) ResolveOpenLoop(ctx context.Context, owner, openLoopKey string) error {
	if err := validateIdentity(owner); err != nil {
		return err
	}
	if err := validateIdentifier("open loop key", openLoopKey); err != nil {
		return err
	}
	return s.repository.MarkOpenLoopResolved(ctx, strings.TrimSpace(owner), strings.TrimSpace(openLoopKey))
}

func (s *Service) RecordFeedback(ctx context.Context, feedback Feedback) (ScoreWeights, error) {
	if err := validateIdentity(feedback.OwnerIdentity); err != nil {
		return ScoreWeights{}, err
	}
	if err := validateIdentifier("proposal id", feedback.ProposalID); err != nil {
		return ScoreWeights{}, err
	}
	switch feedback.Outcome {
	case FeedbackUseful, FeedbackNotUseful:
	default:
		return ScoreWeights{}, fmt.Errorf("unsupported feedback outcome %q", feedback.Outcome)
	}
	if !validScoreComponent(feedback.Component) {
		return ScoreWeights{}, fmt.Errorf("unsupported feedback component %q", feedback.Component)
	}
	if _, err := s.repository.GetProposal(ctx, feedback.OwnerIdentity, feedback.ProposalID); err != nil {
		return ScoreWeights{}, err
	}
	if feedback.OccurredAt.IsZero() {
		feedback.OccurredAt = s.clock().UTC()
	}
	weights, err := s.repository.GetWeights(ctx, feedback.OwnerIdentity)
	if errors.Is(err, ErrNotFound) {
		weights = DefaultScoreWeights()
	} else if err != nil {
		return ScoreWeights{}, err
	}
	delta := 0.02
	if feedback.Outcome == FeedbackNotUseful {
		delta = -delta
	}
	switch feedback.Component {
	case ComponentRelevance:
		weights.Relevance += delta
	case ComponentUrgency:
		weights.Urgency += delta
	case ComponentImportance:
		weights.Importance += delta
	case ComponentRisk:
		weights.Risk += delta
	}
	weights = normalizeLearnedWeights(weights)
	if err := validateWeights(weights); err != nil {
		return ScoreWeights{}, err
	}
	if err := s.repository.PutWeights(ctx, feedback.OwnerIdentity, weights); err != nil {
		return ScoreWeights{}, err
	}
	if err := s.repository.AppendFeedback(ctx, feedback); err != nil {
		return ScoreWeights{}, err
	}
	return weights, nil
}

func evaluateEvidence(sources []SourceReference, maximumAge time.Duration, now time.Time) ([]EvidenceSnapshot, SuppressionReason) {
	result := make([]EvidenceSnapshot, 0, len(sources))
	hasFreshSupport := false
	for _, source := range sources {
		age := now.Sub(source.ObservedAt)
		fresh := age >= 0 && age <= maximumAge
		if fresh && (source.Verification == VerificationVerified || source.Verification == VerificationSourceSupported) {
			hasFreshSupport = true
		}
		result = append(result, EvidenceSnapshot{SourceReference: source, Age: age, IsFresh: fresh})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	if hasFreshSupport {
		return result, SuppressionNone
	}
	for _, source := range result {
		if source.IsFresh {
			return result, SuppressionUncertain
		}
	}
	return result, SuppressionStale
}

func approvalRequirement(signal Signal) (bool, string) {
	if signal.Domain == DomainLegal || signal.Domain == DomainMedical || signal.Domain == DomainFinancial {
		return true, fmt.Sprintf("%s decisions always require explicit human approval", signal.Domain)
	}
	if signal.Risk == RiskHigh || signal.Risk == RiskCritical {
		return true, fmt.Sprintf("%s-risk proposals require explicit human approval", signal.Risk)
	}
	if signal.Type == SignalCapacityConstraint {
		return true, "health and safety capacity changes require human review"
	}
	return false, ""
}

func recommendedAction(signal Signal) RecommendedAction {
	action := RecommendedAction{ExternalEffect: false}
	switch signal.Type {
	case SignalDeadline:
		action.Kind = "review_deadline"
		action.Label = "Review deadline"
		action.Description = "Review the evidence and choose the next safe preparation step."
	case SignalCommitment:
		action.Kind = "review_commitment"
		action.Label = "Review commitment"
		action.Description = "Confirm the commitment and prepare a follow-up for human review."
	case SignalWaitingState:
		action.Kind = "prepare_follow_up"
		action.Label = "Prepare follow-up"
		action.Description = "Review what is outstanding and prepare, but do not send, a follow-up."
	case SignalStaleWork:
		action.Kind = "review_stale_work"
		action.Label = "Review stale work"
		action.Description = "Decide whether to resume, replan, archive, or close the work."
	case SignalSourceChange:
		action.Kind = "review_source_change"
		action.Label = "Review source change"
		action.Description = "Compare the source change with dependent plans before continuing."
	case SignalRecurringObligation:
		action.Kind = "prepare_recurring_obligation"
		action.Label = "Prepare next occurrence"
		action.Description = "Review the recurring obligation and prepare its next safe step."
	case SignalCapacityConstraint:
		action.Kind = "review_capacity"
		action.Label = "Review capacity"
		action.Description = "Review the capacity constraint and decide how the plan should be adjusted."
	case SignalReviewQueue:
		action.Kind = "review_queue_item"
		action.Label = "Review queued item"
		action.Description = "Inspect the evidence and record a human decision."
	}
	return action
}

func nextNotificationTime(quiet QuietHours, now time.Time) (time.Time, bool, error) {
	if !quiet.Enabled {
		return now, false, nil
	}
	location, err := time.LoadLocation(quiet.TimeZone)
	if err != nil {
		return time.Time{}, false, err
	}
	local := now.In(location)
	minute := local.Hour()*60 + local.Minute()
	inQuiet := false
	if quiet.StartMinute < quiet.EndMinute {
		inQuiet = minute >= quiet.StartMinute && minute < quiet.EndMinute
	} else {
		inQuiet = minute >= quiet.StartMinute || minute < quiet.EndMinute
	}
	if !inQuiet {
		return now, false, nil
	}
	endDay := local
	if quiet.StartMinute > quiet.EndMinute && minute >= quiet.StartMinute {
		endDay = endDay.AddDate(0, 0, 1)
	}
	end := time.Date(endDay.Year(), endDay.Month(), endDay.Day(), quiet.EndMinute/60, quiet.EndMinute%60, 0, 0, location)
	return end.UTC(), true, nil
}

func transitionAllowed(from, to ProposalStatus) bool {
	switch from {
	case StatusProposed:
		return to == StatusAccepted || to == StatusDismissed || to == StatusSnoozed || to == StatusExpired || to == StatusResolved
	case StatusAccepted:
		return to == StatusSnoozed || to == StatusDismissed || to == StatusExpired || to == StatusResolved
	case StatusSnoozed:
		return to == StatusProposed || to == StatusAccepted || to == StatusDismissed || to == StatusExpired || to == StatusResolved
	default:
		return false
	}
}

func containsSignalType(values []SignalType, target SignalType) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func suppressed(reason SuppressionReason, detail string) EvaluationResult {
	return EvaluationResult{Suppressed: true, Suppression: reason, Reason: detail}
}

func validScoreComponent(value ScoreComponentName) bool {
	return value == ComponentRelevance || value == ComponentUrgency || value == ComponentImportance || value == ComponentRisk
}
