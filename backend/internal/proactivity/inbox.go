package proactivity

import (
	"context"
	"time"
)

const InboxAuthority = "attention_inbox_only"

type InboxItem struct {
	Decision            DecisionRecord  `json:"decision"`
	LatestFeedback      *FeedbackRecord `json:"latestFeedback,omitempty"`
	Authority           string          `json:"authority"`
	CanExecute          bool            `json:"canExecute"`
	DeliveryAuthorized  bool            `json:"deliveryAuthorized"`
	ExecutionAuthorized bool            `json:"executionAuthorized"`
}

type InboxSummary struct {
	AsOf         time.Time   `json:"asOf"`
	Items        []InboxItem `json:"items"`
	Acknowledged int         `json:"acknowledged"`
	Dismissed    int         `json:"dismissed"`
	Snoozed      int         `json:"snoozed"`
	Suppressed   int         `json:"suppressed"`
	Authority    string      `json:"authority"`
	CanExecute   bool        `json:"canExecute"`
}

// Inbox derives the current owner-visible attention queue from immutable
// decisions and feedback. It is a projection only and grants no authority.
func (s *Service) Inbox(ctx context.Context, owner string, limit int) (InboxSummary, error) {
	owner, err := validateServiceIdentity(owner)
	if err != nil {
		return InboxSummary{}, err
	}
	if err := s.available(); err != nil {
		return InboxSummary{}, err
	}
	if limit < 1 || limit > maxAdvisoryLimit {
		return InboxSummary{}, ErrInvalidLimit
	}

	now := s.now().UTC().Truncate(time.Microsecond)
	decisions, err := s.Decisions(ctx, owner, MaxDecisionHistory)
	if err != nil {
		return InboxSummary{}, err
	}
	feedback, err := s.Feedback(ctx, owner, MaxFeedbackHistory)
	if err != nil {
		return InboxSummary{}, err
	}

	latestFeedback := make(map[string]FeedbackRecord, len(feedback))
	for _, record := range feedback {
		if _, exists := latestFeedback[record.OpenLoopKey]; !exists {
			latestFeedback[record.OpenLoopKey] = record
		}
	}

	result := InboxSummary{
		AsOf:       now,
		Items:      make([]InboxItem, 0, limit),
		Authority:  InboxAuthority,
		CanExecute: false,
	}
	seen := make(map[string]struct{}, len(decisions))
	for _, record := range decisions {
		key := record.Decision.OpenLoopKey
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		if record.Decision.Outcome == OutcomeSuppress {
			result.Suppressed++
			continue
		}

		var latest *FeedbackRecord
		if value, exists := latestFeedback[key]; exists {
			copy := cloneFeedbackRecord(value)
			latest = &copy
			sameRevision := value.SignalDigest == record.Decision.SignalDigest
			switch value.Action {
			case FeedbackSuppress:
				result.Suppressed++
				continue
			case FeedbackSnooze:
				if value.SnoozedUntil != nil && value.SnoozedUntil.After(now) {
					result.Snoozed++
					continue
				}
			case FeedbackDismiss:
				if sameRevision {
					result.Dismissed++
					continue
				}
			case FeedbackAccept:
				if sameRevision {
					result.Acknowledged++
					continue
				}
			}
		}

		result.Items = append(result.Items, InboxItem{
			Decision: record, LatestFeedback: latest,
			Authority: InboxAuthority, CanExecute: false,
			DeliveryAuthorized: false, ExecutionAuthorized: false,
		})
		if len(result.Items) == limit {
			break
		}
	}
	return result, nil
}
