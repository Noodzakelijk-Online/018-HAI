package proactivity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"
)

const (
	urgencyWeight    = 0.27
	impactWeight     = 0.25
	deadlineWeight   = 0.22
	stalenessWeight  = 0.16
	confidenceWeight = 0.10
)

type candidate struct {
	signal           OpenLoopSignal
	digest           string
	decision         Decision
	notifyCandidate  bool
	suppressed       bool
	deadlineSortTime time.Time
}

// Evaluate deterministically ranks a bounded owner-scoped snapshot. The
// returned channels are recommendations for a separate governed delivery
// system; this package has no delivery or execution capability.
func Evaluate(input EvaluationRequest) (EvaluationResult, error) {
	request, location, err := normalizeAndValidate(input)
	if err != nil {
		return EvaluationResult{}, err
	}
	preferences := request.Preferences
	historyInterruptions := interruptionsUsedToday(request.History, request.Now, location)
	remaining := preferences.AttentionBudget.MaxInterruptionsPerDay - historyInterruptions
	if remaining < 0 {
		remaining = 0
	}

	candidates := make([]candidate, 0, len(request.Signals))
	for _, signal := range request.Signals {
		digest, digestErr := digestSignal(signal)
		if digestErr != nil {
			return EvaluationResult{}, fmt.Errorf("digest signal: %w", digestErr)
		}
		item := buildCandidate(request, signal, digest)
		candidates = append(candidates, item)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		if left.decision.Score != right.decision.Score {
			return left.decision.Score > right.decision.Score
		}
		if !left.deadlineSortTime.Equal(right.deadlineSortTime) {
			return left.deadlineSortTime.Before(right.deadlineSortTime)
		}
		if left.signal.OpenLoopKey != right.signal.OpenLoopKey {
			return left.signal.OpenLoopKey < right.signal.OpenLoopKey
		}
		return left.signal.ID < right.signal.ID
	})

	batchInterruptions := 0
	for index := range candidates {
		item := &candidates[index]
		if !item.notifyCandidate || item.suppressed {
			continue
		}
		if inQuietHours(preferences.QuietHours, request.Now) {
			item.decision.Outcome = OutcomeDailyBrief
			item.decision.BudgetCost = 0
			item.decision.RecommendedChannels = recommendedChannels(preferences, OutcomeDailyBrief)
			item.decision.Reasons = append(item.decision.Reasons,
				"quiet hours defer interruption to the daily brief")
			continue
		}
		if batchInterruptions >= remaining {
			item.decision.Outcome = OutcomeDailyBrief
			item.decision.BudgetCost = 0
			item.decision.RecommendedChannels = recommendedChannels(preferences, OutcomeDailyBrief)
			item.decision.Reasons = append(item.decision.Reasons,
				"daily interruption budget is exhausted; retained for the daily brief")
			continue
		}
		channels := recommendedChannels(preferences, OutcomeNotify)
		if len(channels) == 0 {
			item.decision.Outcome = OutcomeDailyBrief
			item.decision.BudgetCost = 0
			item.decision.RecommendedChannels = recommendedChannels(preferences, OutcomeDailyBrief)
			item.decision.Reasons = append(item.decision.Reasons,
				"no explicitly enabled notification channel is available")
			continue
		}
		item.decision.RecommendedChannels = channels
		item.decision.BudgetCost = 1
		batchInterruptions++
	}

	decisions := make([]Decision, len(candidates))
	for index := range candidates {
		decisions[index] = candidates[index].decision
	}
	used := historyInterruptions + batchInterruptions
	resultRemaining := preferences.AttentionBudget.MaxInterruptionsPerDay - used
	if resultRemaining < 0 {
		resultRemaining = 0
	}
	return EvaluationResult{
		ContractVersion:        ContractVersion,
		OwnerIdentity:          request.OwnerIdentity,
		DecidedAt:              request.Now,
		TimeZone:               preferences.TimeZone,
		InterruptionsUsed:      used,
		InterruptionsRemaining: resultRemaining,
		Decisions:              decisions,
	}, nil
}

func buildCandidate(request EvaluationRequest, signal OpenLoopSignal, digest string) candidate {
	score, components := scoreSignal(signal, request.Now)
	decision := Decision{
		ContractVersion:     ContractVersion,
		OwnerIdentity:       request.OwnerIdentity,
		SignalID:            signal.ID,
		OpenLoopKey:         signal.OpenLoopKey,
		SignalDigest:        digest,
		Title:               signal.Title,
		Summary:             signal.Summary,
		Score:               score,
		Components:          components,
		ExecutionAuthorized: false,
		DeliveryAuthorized:  false,
		AuthorityGranted:    false,
		DecidedAt:           request.Now,
	}
	item := candidate{
		signal:           signal,
		digest:           digest,
		decision:         decision,
		deadlineSortTime: time.Unix(1<<62, 0),
	}
	if signal.Deadline != nil {
		item.deadlineSortTime = *signal.Deadline
	}

	if signal.Status == StatusResolved {
		item.suppress("the open loop is already resolved", nil)
		return item
	}
	if control, found := currentAttentionControl(request.Controls, signal.OpenLoopKey); found {
		switch control.Action {
		case FeedbackSuppress:
			item.suppress("the owner suppressed this open loop", nil)
			return item
		case FeedbackSnooze:
			if control.SnoozedUntil != nil && request.Now.Before(*control.SnoozedUntil) {
				item.suppress("the owner snoozed this open loop", control.SnoozedUntil)
				return item
			}
		case FeedbackDismiss:
			if control.SignalDigest == digest {
				item.suppress("the owner dismissed this exact signal revision", nil)
				return item
			}
		}
	}
	if duplicateHistory(request.History, signal.OpenLoopKey, digest) {
		item.suppress("an identical signal decision already exists", nil)
		return item
	}
	if next, coolingDown := cooldownEnd(request.History, signal.OpenLoopKey, request.Now, request.Preferences.Cooldown); coolingDown {
		item.suppress("the open loop is inside its notification cooldown", &next)
		return item
	}

	reviewRequired := signal.Sensitive || signal.HumanReviewRequired ||
		signal.Risk == RiskHigh || signal.Risk == RiskCritical
	if signal.Confidence < request.Preferences.MinimumConfidence {
		if reviewRequired {
			item.decision.Outcome = OutcomeRequireReview
			item.decision.Reasons = []string{
				"confidence is below the owner threshold",
				"risk or sensitivity requires human review instead of silent suppression",
			}
			item.decision.RecommendedChannels = recommendedChannels(request.Preferences, OutcomeRequireReview)
			return item
		}
		item.suppress("confidence is below the owner threshold", nil)
		return item
	}
	if reviewRequired || score >= request.Preferences.ReviewThreshold {
		item.decision.Outcome = OutcomeRequireReview
		item.decision.Reasons = reviewReasons(signal, score >= request.Preferences.ReviewThreshold)
		item.decision.RecommendedChannels = recommendedChannels(request.Preferences, OutcomeRequireReview)
		return item
	}
	if score >= request.Preferences.NotifyThreshold {
		item.decision.Outcome = OutcomeNotify
		item.decision.Reasons = []string{"rank exceeds the owner's notification threshold"}
		item.notifyCandidate = true
		return item
	}
	if score >= request.Preferences.DailyBriefThreshold {
		item.decision.Outcome = OutcomeDailyBrief
		item.decision.Reasons = []string{"rank is material but does not justify an interruption"}
		item.decision.RecommendedChannels = recommendedChannels(request.Preferences, OutcomeDailyBrief)
		return item
	}
	if score >= request.Preferences.AmbientThreshold {
		item.decision.Outcome = OutcomeAmbient
		item.decision.Reasons = []string{"rank is suitable for passive ambient visibility"}
		item.decision.RecommendedChannels = recommendedChannels(request.Preferences, OutcomeAmbient)
		return item
	}
	item.suppress("rank is below the owner's ambient threshold", nil)
	return item
}

func currentAttentionControl(controls []AttentionControl, openLoopKey string) (AttentionControl, bool) {
	for _, control := range controls {
		if control.OpenLoopKey == openLoopKey {
			return control, true
		}
	}
	return AttentionControl{}, false
}

func (item *candidate) suppress(reason string, next *time.Time) {
	item.decision.Outcome = OutcomeSuppress
	item.decision.Reasons = []string{reason}
	item.decision.NextEligibleAt = next
	item.decision.RecommendedChannels = nil
	item.decision.BudgetCost = 0
	item.suppressed = true
}

func reviewReasons(signal OpenLoopSignal, scoreTriggered bool) []string {
	reasons := make([]string, 0, 4)
	if signal.Sensitive {
		reasons = append(reasons, "sensitive context requires human review")
	}
	if signal.HumanReviewRequired {
		reasons = append(reasons, "the source marked this open loop for human review")
	}
	if signal.Risk == RiskHigh || signal.Risk == RiskCritical {
		reasons = append(reasons, "high-risk work cannot be escalated without human review")
	}
	if scoreTriggered {
		reasons = append(reasons, "rank exceeds the owner's review threshold")
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "policy requires human review")
	}
	return reasons
}

func scoreSignal(signal OpenLoopSignal, now time.Time) (float64, []ScoreComponent) {
	deadlineValue, deadlineReason := deadlineScore(signal.Deadline, now)
	stalenessValue, stalenessReason := stalenessScore(signal.LastActivityAt, signal.StaleAfter, now)
	components := []ScoreComponent{
		component("urgency", signal.Urgency, urgencyWeight, "source-assessed urgency"),
		component("impact", signal.Impact, impactWeight, "source-assessed consequence if left open"),
		component("deadline", deadlineValue, deadlineWeight, deadlineReason),
		component("staleness", stalenessValue, stalenessWeight, stalenessReason),
		component("confidence", signal.Confidence, confidenceWeight, "confidence in the source signal"),
	}
	total := 0.0
	for _, value := range components {
		total += value.Contribution
	}
	return round(total), components
}

func component(name string, value, weight float64, explanation string) ScoreComponent {
	return ScoreComponent{
		Name:         name,
		Value:        round(value),
		Weight:       weight,
		Contribution: round(value * weight),
		Explanation:  explanation,
	}
}

func deadlineScore(deadline *time.Time, now time.Time) (float64, string) {
	if deadline == nil {
		return 0, "no deadline is attached"
	}
	remaining := deadline.Sub(now)
	switch {
	case remaining <= 0:
		return 1, "deadline is overdue"
	case remaining <= 2*time.Hour:
		return 0.98, "deadline is within two hours"
	case remaining <= 24*time.Hour:
		return 0.90, "deadline is within one day"
	case remaining <= 3*24*time.Hour:
		return 0.72, "deadline is within three days"
	case remaining <= 7*24*time.Hour:
		return 0.52, "deadline is within one week"
	case remaining <= 30*24*time.Hour:
		return 0.25, "deadline is within thirty days"
	default:
		return 0.08, "deadline is more than thirty days away"
	}
}

func stalenessScore(lastActivity time.Time, staleAfter time.Duration, now time.Time) (float64, string) {
	age := now.Sub(lastActivity)
	if age <= 0 {
		return 0, "activity is current"
	}
	ratio := float64(age) / float64(staleAfter)
	switch {
	case ratio < 0.5:
		return 0.05, "activity is well inside its freshness window"
	case ratio < 1:
		return round(0.1 + 0.4*(ratio-0.5)/0.5), "activity is approaching its staleness threshold"
	case ratio < 2:
		return round(0.6 + 0.3*(ratio-1)), "open loop is stale"
	default:
		return 1, "open loop is substantially stale"
	}
}

func digestSignal(signal OpenLoopSignal) (string, error) {
	projection := struct {
		ID                  string
		OwnerIdentity       string
		OpenLoopKey         string
		Title               string
		Summary             string
		Status              SignalStatus
		Risk                RiskLevel
		ObservedAt          time.Time
		LastActivityAt      time.Time
		Deadline            *time.Time
		StaleAfter          time.Duration
		Impact              float64
		Urgency             float64
		Confidence          float64
		Sensitive           bool
		HumanReviewRequired bool
		Evidence            []EvidenceReference
	}{
		signal.ID, signal.OwnerIdentity, signal.OpenLoopKey, signal.Title, signal.Summary,
		signal.Status, signal.Risk, signal.ObservedAt, signal.LastActivityAt, signal.Deadline,
		signal.StaleAfter, signal.Impact, signal.Urgency, signal.Confidence, signal.Sensitive,
		signal.HumanReviewRequired, signal.Evidence,
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func duplicateHistory(history []DecisionHistory, openLoopKey, digest string) bool {
	for _, entry := range history {
		if entry.OpenLoopKey == openLoopKey && entry.SignalDigest == digest {
			return true
		}
	}
	return false
}

func cooldownEnd(history []DecisionHistory, openLoopKey string, now time.Time, cooldown time.Duration) (time.Time, bool) {
	if cooldown <= 0 {
		return time.Time{}, false
	}
	var latest time.Time
	for _, entry := range history {
		if entry.OpenLoopKey != openLoopKey || entry.Outcome == OutcomeSuppress {
			continue
		}
		if entry.DecidedAt.After(latest) {
			latest = entry.DecidedAt
		}
	}
	if latest.IsZero() {
		return time.Time{}, false
	}
	next := latest.Add(cooldown)
	return next, now.Before(next)
}

func interruptionsUsedToday(history []DecisionHistory, now time.Time, location *time.Location) int {
	localNow := now.In(location)
	year, month, day := localNow.Date()
	count := 0
	for _, entry := range history {
		if entry.Outcome != OutcomeNotify {
			continue
		}
		entryYear, entryMonth, entryDay := entry.DecidedAt.In(location).Date()
		if entryYear == year && entryMonth == month && entryDay == day {
			count++
		}
	}
	return count
}

func inQuietHours(value QuietHours, now time.Time) bool {
	if !value.Enabled {
		return false
	}
	location, err := time.LoadLocation(value.TimeZone)
	if err != nil {
		return true
	}
	local := now.In(location)
	minute := local.Hour()*60 + local.Minute()
	if value.StartMinute < value.EndMinute {
		return minute >= value.StartMinute && minute < value.EndMinute
	}
	return minute >= value.StartMinute || minute < value.EndMinute
}

func recommendedChannels(preferences Preferences, outcome Outcome) []Channel {
	if outcome == OutcomeSuppress {
		return nil
	}
	result := make([]Channel, 0, 3)
	for _, preference := range preferences.Channels {
		if !preference.Enabled {
			continue
		}
		if !preference.Channel.Local() && !preferences.AllowExternalChannels {
			continue
		}
		if outcome != OutcomeNotify && !preference.Channel.Local() {
			continue
		}
		result = append(result, preference.Channel)
		if len(result) == 3 {
			break
		}
	}
	return result
}

func round(value float64) float64 {
	return math.Round(value*1_000_000) / 1_000_000
}
