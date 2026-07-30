package domainpack

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const minimumClassificationScore = 30

func (registry *Registry) Classify(request ClassificationRequest, preferences PreferenceRepository) (ClassificationResult, error) {
	if registry == nil {
		return ClassificationResult{}, fmt.Errorf("domain pack registry is required")
	}
	text := normalizeText(request.Text)
	explicitIDs := make(map[PackID]struct{}, len(request.ExplicitPackIDs))
	for _, id := range request.ExplicitPackIDs {
		if _, exists := registry.packs[id]; !exists {
			return ClassificationResult{}, fmt.Errorf("unknown explicit domain pack %q", id)
		}
		explicitIDs[id] = struct{}{}
	}

	var ownerPreferences map[PackID]PackPreference
	if preferences != nil && strings.TrimSpace(request.OwnerIdentity) != "" {
		stored, err := preferences.List(request.OwnerIdentity)
		if err != nil {
			return ClassificationResult{}, fmt.Errorf("load domain pack preferences: %w", err)
		}
		ownerPreferences = make(map[PackID]PackPreference, len(stored))
		for _, preference := range stored {
			ownerPreferences[preference.PackID] = preference
		}
	}

	result := ClassificationResult{}
	for _, id := range registry.ids {
		pack := registry.packs[id]
		preference, hasPreference := ownerPreferences[id]
		enabled := pack.DefaultEnabled
		if hasPreference && preference.Enabled != nil {
			enabled = *preference.Enabled
		}
		if !enabled {
			result.Suppressed = append(result.Suppressed, SuppressedMatch{
				PackID: id,
				Reason: "disabled by owner-scoped preference",
			})
			continue
		}

		_, explicitID := explicitIDs[id]
		explicitValues := explicitSignalValues(request.ExplicitSignals, id)
		explicit := explicitID || len(explicitValues) > 0
		match := ClassificationMatch{PackID: id, Sensitive: pack.Sensitive, Explicit: explicit}
		hasStrongSignal := false
		weakSignals := make([]string, 0)

		if explicitID {
			match.Score += 100
			match.Reasons = append(match.Reasons, "domain pack was explicitly selected")
		}
		for _, value := range explicitValues {
			match.Score += 80
			match.Reasons = append(match.Reasons, "explicit domain signal: "+value)
			match.Signals = append(match.Signals, SignalMatch{
				Signal: value, Strength: SignalStrong, Score: 80,
				Reason: "caller supplied an explicit domain signal",
			})
			hasStrongSignal = true
		}
		for _, signal := range pack.ClassificationSignals {
			if text == "" || !containsPhrase(text, normalizeText(signal.Phrase)) {
				continue
			}
			score := signalScore(signal.Strength)
			match.Score += score
			match.Signals = append(match.Signals, SignalMatch{
				Signal: signal.Phrase, Strength: signal.Strength, Score: score, Reason: signal.Reason,
			})
			match.Reasons = append(match.Reasons, fmt.Sprintf("matched %s signal %q", signalStrengthName(signal.Strength), signal.Phrase))
			if signal.Strength == SignalStrong {
				hasStrongSignal = true
			} else {
				weakSignals = append(weakSignals, signal.Phrase)
			}
		}
		if hasPreference && preference.ClassificationBoost != 0 {
			match.Score += preference.ClassificationBoost
			match.Reasons = append(match.Reasons, fmt.Sprintf("owner preference adjusted score by %+d", preference.ClassificationBoost))
		}

		if pack.Sensitive && !explicit && !hasStrongSignal && match.Score > 0 {
			result.Suppressed = append(result.Suppressed, SuppressedMatch{
				PackID:  id,
				Reason:  "sensitive domain requires explicit selection or a strong unambiguous signal",
				Signals: weakSignals,
			})
			continue
		}
		if match.Score < minimumClassificationScore {
			continue
		}
		sort.SliceStable(match.Signals, func(i, j int) bool {
			if match.Signals[i].Score != match.Signals[j].Score {
				return match.Signals[i].Score > match.Signals[j].Score
			}
			return match.Signals[i].Signal < match.Signals[j].Signal
		})
		match.Reasons = uniqueSorted(match.Reasons)
		result.Matches = append(result.Matches, match)
	}

	sort.SliceStable(result.Matches, func(i, j int) bool {
		if result.Matches[i].Score != result.Matches[j].Score {
			return result.Matches[i].Score > result.Matches[j].Score
		}
		return result.Matches[i].PackID < result.Matches[j].PackID
	})
	sort.SliceStable(result.Suppressed, func(i, j int) bool {
		return result.Suppressed[i].PackID < result.Suppressed[j].PackID
	})
	return result, nil
}

func explicitSignalValues(signals map[string][]string, id PackID) []string {
	if len(signals) == 0 {
		return nil
	}
	values := signals[string(id)]
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return uniqueSorted(result)
}

func signalScore(strength SignalStrength) int {
	switch strength {
	case SignalStrong:
		return 70
	case SignalModerate:
		return 35
	default:
		return 10
	}
}

func signalStrengthName(strength SignalStrength) string {
	switch strength {
	case SignalStrong:
		return "strong"
	case SignalModerate:
		return "moderate"
	default:
		return "weak"
	}
}

func containsPhrase(text, phrase string) bool {
	if text == "" || phrase == "" {
		return false
	}
	pattern := `(^|[^a-z0-9])` + regexp.QuoteMeta(phrase) + `([^a-z0-9]|$)`
	return regexp.MustCompile(pattern).FindStringIndex(text) != nil
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
