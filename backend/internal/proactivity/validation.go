package proactivity

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	maxIdentityLength   = 320
	maxIdentifierLength = 200
	maxTitleLength      = 300
	maxSummaryLength    = 4000
	maxPolicyDuration   = 365 * 24 * time.Hour
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:@/-]{0,199}$`)
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func normalizeAndValidate(request EvaluationRequest) (EvaluationRequest, *time.Location, error) {
	request.OwnerIdentity = strings.TrimSpace(request.OwnerIdentity)
	if request.ContractVersion != ContractVersion {
		return EvaluationRequest{}, nil, fmt.Errorf("unsupported evaluation contract version")
	}
	if err := validateIdentity(request.OwnerIdentity); err != nil {
		return EvaluationRequest{}, nil, err
	}
	if request.Now.IsZero() {
		return EvaluationRequest{}, nil, fmt.Errorf("evaluation time is required")
	}
	request.Now = request.Now.UTC()

	prefs, location, err := normalizePreferences(request.OwnerIdentity, request.Preferences)
	if err != nil {
		return EvaluationRequest{}, nil, err
	}
	request.Preferences = prefs
	if len(request.Signals) > MaxSignals {
		return EvaluationRequest{}, nil, fmt.Errorf("signal count exceeds %d", MaxSignals)
	}
	if len(request.History) > MaxHistoryEntries {
		return EvaluationRequest{}, nil, fmt.Errorf("history count exceeds %d", MaxHistoryEntries)
	}
	if len(request.Controls) > MaxSignals {
		return EvaluationRequest{}, nil, fmt.Errorf("attention control count exceeds %d", MaxSignals)
	}

	seenSignals := make(map[string]struct{}, len(request.Signals))
	for index := range request.Signals {
		normalized, signalErr := normalizeSignal(request.OwnerIdentity, request.Signals[index], request.Now)
		if signalErr != nil {
			return EvaluationRequest{}, nil, fmt.Errorf("signal %d: %w", index, signalErr)
		}
		if _, exists := seenSignals[normalized.ID]; exists {
			return EvaluationRequest{}, nil, fmt.Errorf("signal ids must be unique")
		}
		seenSignals[normalized.ID] = struct{}{}
		request.Signals[index] = normalized
	}
	for index := range request.History {
		normalized, historyErr := normalizeHistory(request.OwnerIdentity, request.History[index], request.Now)
		if historyErr != nil {
			return EvaluationRequest{}, nil, fmt.Errorf("history %d: %w", index, historyErr)
		}
		request.History[index] = normalized
	}
	seenControls := make(map[string]struct{}, len(request.Controls))
	for index := range request.Controls {
		normalized, controlErr := normalizeAttentionControl(request.Controls[index], request.Now)
		if controlErr != nil {
			return EvaluationRequest{}, nil, fmt.Errorf("attention control %d: %w", index, controlErr)
		}
		if _, exists := seenControls[normalized.OpenLoopKey]; exists {
			return EvaluationRequest{}, nil, fmt.Errorf("attention controls must contain one current record per open loop")
		}
		seenControls[normalized.OpenLoopKey] = struct{}{}
		request.Controls[index] = normalized
	}

	return request, location, nil
}

func normalizeAttentionControl(value AttentionControl, now time.Time) (AttentionControl, error) {
	value.OpenLoopKey = strings.TrimSpace(value.OpenLoopKey)
	value.SignalDigest = strings.ToLower(strings.TrimSpace(value.SignalDigest))
	if err := validateIdentifier("attention control open-loop key", value.OpenLoopKey); err != nil {
		return AttentionControl{}, err
	}
	if !digestPattern.MatchString(value.SignalDigest) {
		return AttentionControl{}, fmt.Errorf("attention control signal digest is invalid")
	}
	if !validFeedbackAction(value.Action) {
		return AttentionControl{}, fmt.Errorf("attention control action is unsupported")
	}
	if value.RecordedAt.IsZero() || value.RecordedAt.After(now.Add(time.Minute)) {
		return AttentionControl{}, fmt.Errorf("attention control time must be present and not in the future")
	}
	value.RecordedAt = value.RecordedAt.UTC()
	if value.Action == FeedbackSnooze {
		if value.SnoozedUntil == nil {
			return AttentionControl{}, fmt.Errorf("snoozed attention control requires an end time")
		}
		until := value.SnoozedUntil.UTC()
		value.SnoozedUntil = &until
	} else if value.SnoozedUntil != nil {
		return AttentionControl{}, fmt.Errorf("only snoozed attention controls may have an end time")
	}
	return value, nil
}

func normalizePreferences(owner string, value Preferences) (Preferences, *time.Location, error) {
	value.OwnerIdentity = strings.TrimSpace(value.OwnerIdentity)
	value.TimeZone = strings.TrimSpace(value.TimeZone)
	if value.ContractVersion != ContractVersion {
		return Preferences{}, nil, fmt.Errorf("unsupported preferences contract version")
	}
	if value.OwnerIdentity != owner {
		return Preferences{}, nil, fmt.Errorf("preferences owner does not match authenticated owner")
	}
	if value.TimeZone == "" {
		return Preferences{}, nil, fmt.Errorf("preference time zone is required")
	}
	location, err := time.LoadLocation(value.TimeZone)
	if err != nil {
		return Preferences{}, nil, fmt.Errorf("preference time zone is invalid")
	}
	if value.QuietHours.TimeZone == "" {
		value.QuietHours.TimeZone = value.TimeZone
	}
	if err := validateQuietHours(value.QuietHours); err != nil {
		return Preferences{}, nil, err
	}
	for _, item := range []struct {
		name  string
		value float64
	}{
		{"minimum confidence", value.MinimumConfidence},
		{"ambient threshold", value.AmbientThreshold},
		{"daily brief threshold", value.DailyBriefThreshold},
		{"notify threshold", value.NotifyThreshold},
		{"review threshold", value.ReviewThreshold},
	} {
		if err := validateUnit(item.name, item.value); err != nil {
			return Preferences{}, nil, err
		}
	}
	if !(value.AmbientThreshold <= value.DailyBriefThreshold &&
		value.DailyBriefThreshold <= value.NotifyThreshold &&
		value.NotifyThreshold <= value.ReviewThreshold) {
		return Preferences{}, nil, fmt.Errorf("attention thresholds must be ordered from ambient through review")
	}
	if value.Cooldown < 0 || value.Cooldown > maxPolicyDuration {
		return Preferences{}, nil, fmt.Errorf("cooldown must be between zero and 365 days")
	}
	if value.AttentionBudget.MaxInterruptionsPerDay < 0 || value.AttentionBudget.MaxInterruptionsPerDay > 100 {
		return Preferences{}, nil, fmt.Errorf("daily interruption budget must be between 0 and 100")
	}

	seenChannels := make(map[Channel]struct{}, len(value.Channels))
	for index, channel := range value.Channels {
		if !validChannel(channel.Channel) {
			return Preferences{}, nil, fmt.Errorf("channel %d is unsupported", index)
		}
		if channel.Order < 0 || channel.Order > 1000 {
			return Preferences{}, nil, fmt.Errorf("channel order must be between 0 and 1000")
		}
		if _, exists := seenChannels[channel.Channel]; exists {
			return Preferences{}, nil, fmt.Errorf("channel preferences must be unique")
		}
		seenChannels[channel.Channel] = struct{}{}
	}
	sort.Slice(value.Channels, func(i, j int) bool {
		left, right := value.Channels[i], value.Channels[j]
		if left.Channel.Local() != right.Channel.Local() {
			return left.Channel.Local()
		}
		if left.Order != right.Order {
			return left.Order < right.Order
		}
		return left.Channel < right.Channel
	})
	return value, location, nil
}

func normalizeSignal(owner string, value OpenLoopSignal, now time.Time) (OpenLoopSignal, error) {
	value.OwnerIdentity = strings.TrimSpace(value.OwnerIdentity)
	value.ID = strings.TrimSpace(value.ID)
	value.OpenLoopKey = strings.TrimSpace(value.OpenLoopKey)
	if len(value.Title) > maxTitleLength*4 {
		return OpenLoopSignal{}, fmt.Errorf("signal title exceeds the bounded input size")
	}
	if len(value.Summary) > maxSummaryLength*4 {
		return OpenLoopSignal{}, fmt.Errorf("signal summary exceeds the bounded input size")
	}
	value.Title = redactAndBound(value.Title, maxTitleLength)
	value.Summary = redactAndBound(value.Summary, maxSummaryLength)
	if value.ContractVersion != ContractVersion {
		return OpenLoopSignal{}, fmt.Errorf("unsupported signal contract version")
	}
	if value.OwnerIdentity != owner {
		return OpenLoopSignal{}, fmt.Errorf("signal owner does not match authenticated owner")
	}
	if err := validateIdentifier("signal id", value.ID); err != nil {
		return OpenLoopSignal{}, err
	}
	if err := validateIdentifier("open-loop key", value.OpenLoopKey); err != nil {
		return OpenLoopSignal{}, err
	}
	if value.Title == "" {
		return OpenLoopSignal{}, fmt.Errorf("signal title is required")
	}
	if value.Summary == "" {
		return OpenLoopSignal{}, fmt.Errorf("signal summary is required")
	}
	if value.Status != StatusOpen && value.Status != StatusResolved {
		return OpenLoopSignal{}, fmt.Errorf("signal status is unsupported")
	}
	if !validRisk(value.Risk) {
		return OpenLoopSignal{}, fmt.Errorf("signal risk is unsupported")
	}
	if value.ObservedAt.IsZero() || value.ObservedAt.After(now.Add(time.Minute)) {
		return OpenLoopSignal{}, fmt.Errorf("signal observation time must be present and not in the future")
	}
	if value.LastActivityAt.IsZero() || value.LastActivityAt.After(now.Add(time.Minute)) {
		return OpenLoopSignal{}, fmt.Errorf("last activity time must be present and not in the future")
	}
	value.ObservedAt = value.ObservedAt.UTC()
	value.LastActivityAt = value.LastActivityAt.UTC()
	if value.Deadline != nil {
		deadline := value.Deadline.UTC()
		value.Deadline = &deadline
	}
	if value.StaleAfter <= 0 || value.StaleAfter > maxPolicyDuration {
		return OpenLoopSignal{}, fmt.Errorf("staleness window must be positive and no longer than 365 days")
	}
	for _, item := range []struct {
		name  string
		value float64
	}{
		{"impact", value.Impact},
		{"urgency", value.Urgency},
		{"confidence", value.Confidence},
	} {
		if err := validateUnit(item.name, item.value); err != nil {
			return OpenLoopSignal{}, err
		}
	}
	if len(value.Evidence) > MaxEvidencePerItem {
		return OpenLoopSignal{}, fmt.Errorf("evidence count exceeds %d", MaxEvidencePerItem)
	}
	seenEvidence := make(map[string]struct{}, len(value.Evidence))
	for index := range value.Evidence {
		evidence := &value.Evidence[index]
		evidence.ID = strings.TrimSpace(evidence.ID)
		evidence.Kind = strings.TrimSpace(evidence.Kind)
		evidence.Digest = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(evidence.Digest)), "sha256:")
		if err := validateIdentifier("evidence id", evidence.ID); err != nil {
			return OpenLoopSignal{}, err
		}
		if err := validateIdentifier("evidence kind", evidence.Kind); err != nil {
			return OpenLoopSignal{}, err
		}
		if !digestPattern.MatchString(evidence.Digest) {
			return OpenLoopSignal{}, fmt.Errorf("evidence digest must be a lowercase SHA-256 digest")
		}
		if evidence.ObservedAt.IsZero() || evidence.ObservedAt.After(now.Add(time.Minute)) {
			return OpenLoopSignal{}, fmt.Errorf("evidence observation time must be present and not in the future")
		}
		evidence.ObservedAt = evidence.ObservedAt.UTC()
		if _, exists := seenEvidence[evidence.ID]; exists {
			return OpenLoopSignal{}, fmt.Errorf("evidence ids must be unique within a signal")
		}
		seenEvidence[evidence.ID] = struct{}{}
	}
	sort.Slice(value.Evidence, func(i, j int) bool {
		if value.Evidence[i].ID != value.Evidence[j].ID {
			return value.Evidence[i].ID < value.Evidence[j].ID
		}
		return value.Evidence[i].Digest < value.Evidence[j].Digest
	})
	return value, nil
}

func normalizeHistory(owner string, value DecisionHistory, now time.Time) (DecisionHistory, error) {
	value.OwnerIdentity = strings.TrimSpace(value.OwnerIdentity)
	value.OpenLoopKey = strings.TrimSpace(value.OpenLoopKey)
	value.SignalDigest = strings.ToLower(strings.TrimSpace(value.SignalDigest))
	if value.ContractVersion != ContractVersion {
		return DecisionHistory{}, fmt.Errorf("unsupported history contract version")
	}
	if value.OwnerIdentity != owner {
		return DecisionHistory{}, fmt.Errorf("history owner does not match authenticated owner")
	}
	if err := validateIdentifier("history open-loop key", value.OpenLoopKey); err != nil {
		return DecisionHistory{}, err
	}
	if !digestPattern.MatchString(value.SignalDigest) {
		return DecisionHistory{}, fmt.Errorf("history signal digest must be a lowercase SHA-256 digest")
	}
	if !validOutcome(value.Outcome) {
		return DecisionHistory{}, fmt.Errorf("history outcome is unsupported")
	}
	if value.DecidedAt.IsZero() || value.DecidedAt.After(now.Add(time.Minute)) {
		return DecisionHistory{}, fmt.Errorf("history decision time must be present and not in the future")
	}
	value.DecidedAt = value.DecidedAt.UTC()
	return value, nil
}

func validateIdentity(value string) error {
	if value == "" || len(value) > maxIdentityLength || containsControl(value) || containsSecret(value) {
		return fmt.Errorf("owner identity is invalid")
	}
	return nil
}

func validateIdentifier(name, value string) error {
	if len(value) > maxIdentifierLength || !identifierPattern.MatchString(value) || containsSecret(value) {
		return fmt.Errorf("%s is invalid", name)
	}
	return nil
}

func validateUnit(name string, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
		return fmt.Errorf("%s must be between 0 and 1", name)
	}
	return nil
}

func validateQuietHours(value QuietHours) error {
	if !value.Enabled {
		return nil
	}
	if value.StartMinute < 0 || value.StartMinute >= 24*60 || value.EndMinute < 0 || value.EndMinute >= 24*60 {
		return fmt.Errorf("quiet-hour minutes must be within a day")
	}
	if value.StartMinute == value.EndMinute {
		return fmt.Errorf("quiet hours cannot cover an ambiguous full day")
	}
	if _, err := time.LoadLocation(strings.TrimSpace(value.TimeZone)); err != nil {
		return fmt.Errorf("quiet-hour time zone is invalid")
	}
	return nil
}

func containsControl(value string) bool {
	for _, char := range value {
		if unicode.IsControl(char) {
			return true
		}
	}
	return false
}

func validOutcome(value Outcome) bool {
	switch value {
	case OutcomeSuppress, OutcomeAmbient, OutcomeDailyBrief, OutcomeNotify, OutcomeRequireReview:
		return true
	default:
		return false
	}
}

func validChannel(value Channel) bool {
	switch value {
	case ChannelInApp, ChannelDesktop, ChannelEmail, ChannelSMS, ChannelWebhook:
		return true
	default:
		return false
	}
}

func validRisk(value RiskLevel) bool {
	return value == RiskLow || value == RiskMedium || value == RiskHigh || value == RiskCritical
}
