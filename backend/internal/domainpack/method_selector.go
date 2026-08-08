package domainpack

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

const (
	defaultMethodSelectionLimit = 8
	maxMethodSelectionLimit     = 50
)

// SelectMethods resolves advisory methods only from packs that a classifier
// has already selected. It never returns an authorization receipt, mandate,
// approval, or executable action.
func (registry *Registry) SelectMethods(
	request MethodSelectionRequest,
	preferences PreferenceRepository,
) (MethodSelectionResult, error) {
	if registry == nil {
		return MethodSelectionResult{}, fmt.Errorf("domain pack registry is required")
	}
	result := MethodSelectionResult{
		CatalogVersion:            registry.version,
		CatalogDigest:             registry.digest,
		AdvisoryOnly:              true,
		ExecutionAuthorityGranted: false,
	}
	if len(request.ClassifiedPackIDs) == 0 {
		return result, fmt.Errorf("at least one classified domain pack is required")
	}
	if preferences != nil && strings.TrimSpace(request.OwnerIdentity) == "" {
		return result, fmt.Errorf("owner identity is required when resolving domain pack preferences")
	}
	limit := request.Limit
	if limit == 0 {
		limit = defaultMethodSelectionLimit
	}
	if limit < 1 || limit > maxMethodSelectionLimit {
		return result, fmt.Errorf("method selection limit must be between 1 and %d", maxMethodSelectionLimit)
	}

	classified := make(map[PackID]struct{}, len(request.ClassifiedPackIDs))
	packViews := make(map[PackID]PackView, len(request.ClassifiedPackIDs))
	for _, packID := range request.ClassifiedPackIDs {
		if _, duplicate := classified[packID]; duplicate {
			continue
		}
		classified[packID] = struct{}{}
		view, err := registry.Resolve(request.OwnerIdentity, packID, preferences)
		if err != nil {
			return result, err
		}
		if !view.Enabled {
			result.Suppressed = append(result.Suppressed, SuppressedMethodGroup{
				PackID: packID,
				Reason: "disabled by owner-scoped preference",
			})
			continue
		}
		packViews[packID] = view
	}

	methodIndex := make(map[string]struct {
		packID PackID
		method PlaybookMethod
	})
	for packID, view := range packViews {
		for _, method := range view.Pack.Playbook.Methods {
			methodIndex[method.ID] = struct {
				packID PackID
				method PlaybookMethod
			}{packID: packID, method: method}
		}
	}
	explicit := make(map[string]struct{}, len(request.ExplicitMethodIDs))
	for _, rawID := range request.ExplicitMethodIDs {
		id := normalizeIdentifier(rawID)
		if id == "" {
			return result, fmt.Errorf("explicit playbook method id cannot be empty")
		}
		if _, duplicate := explicit[id]; duplicate {
			continue
		}
		if _, exists := methodIndex[id]; !exists {
			return result, fmt.Errorf("explicit playbook method %q is not available in the classified enabled packs", id)
		}
		explicit[id] = struct{}{}
	}

	text := normalizeText(request.Text)
	taskTokens := lexicalTokens(text)
	for packID, view := range packViews {
		for _, method := range view.Pack.Playbook.Methods {
			_, explicitlySelected := explicit[method.ID]
			if method.LifecycleStatus == MethodLifecycleRetired ||
				(method.LifecycleStatus == MethodLifecycleDeprecated && !explicitlySelected) {
				continue
			}
			score, reasons := methodSuitability(method, text, taskTokens)
			if explicitlySelected {
				score += 1000
				reasons = append(reasons, "method was explicitly selected within a classified enabled pack")
			}
			if score == 0 {
				continue
			}
			reasons = append(reasons,
				"method belongs to classified pack "+string(packID),
				"selection is advisory and grants no execution authority")
			result.Selections = append(result.Selections, MethodSelection{
				PackID:   packID,
				Method:   clonePlaybook(DomainPlaybook{Methods: []PlaybookMethod{method}}).Methods[0],
				Score:    score,
				Explicit: explicitlySelected,
				Reasons:  uniqueSorted(reasons),
			})
		}
	}

	sort.SliceStable(result.Selections, func(i, j int) bool {
		if result.Selections[i].Score != result.Selections[j].Score {
			return result.Selections[i].Score > result.Selections[j].Score
		}
		if result.Selections[i].PackID != result.Selections[j].PackID {
			return result.Selections[i].PackID < result.Selections[j].PackID
		}
		return result.Selections[i].Method.ID < result.Selections[j].Method.ID
	})
	if len(result.Selections) > limit {
		result.Selections = result.Selections[:limit]
	}
	sort.Slice(result.Suppressed, func(i, j int) bool {
		return result.Suppressed[i].PackID < result.Suppressed[j].PackID
	})
	return result, nil
}

func methodSuitability(method PlaybookMethod, text string, taskTokens map[string]struct{}) (int, []string) {
	if text == "" {
		return 0, nil
	}
	score := 0
	reasons := make([]string, 0)
	name := normalizeText(method.Name)
	if containsPhrase(text, name) {
		score += 160
		reasons = append(reasons, "task directly names "+method.Name)
	}
	methodTokens := lexicalTokens(name)
	matches := make([]string, 0)
	for token := range methodTokens {
		if _, exists := taskTokens[token]; exists {
			matches = append(matches, token)
		}
	}
	sort.Strings(matches)
	for _, token := range matches {
		score += 24
		reasons = append(reasons, "task matches method term "+token)
	}
	if len(matches) == len(methodTokens) && len(methodTokens) > 1 {
		score += 35
		reasons = append(reasons, "task covers all distinctive method terms")
	}
	return score, reasons
}

func lexicalTokens(value string) map[string]struct{} {
	stopWords := map[string]struct{}{
		"a": {}, "an": {}, "and": {}, "as": {}, "at": {}, "be": {}, "for": {},
		"from": {}, "in": {}, "of": {}, "on": {}, "or": {}, "the": {}, "to": {},
		"versus": {}, "with": {}, "without": {},
	}
	result := map[string]struct{}{}
	var token strings.Builder
	flush := func() {
		value := lexicalStem(token.String())
		token.Reset()
		if len(value) < 2 {
			return
		}
		if _, stop := stopWords[value]; stop {
			return
		}
		result[value] = struct{}{}
	}
	for _, value := range strings.ToLower(value) {
		if unicode.IsLetter(value) || unicode.IsDigit(value) {
			token.WriteRune(value)
			continue
		}
		flush()
	}
	flush()
	return result
}

func lexicalStem(value string) string {
	switch {
	case len(value) > 6 && strings.HasSuffix(value, "isation"):
		return strings.TrimSuffix(value, "isation")
	case len(value) > 6 && strings.HasSuffix(value, "ization"):
		return strings.TrimSuffix(value, "ization")
	case len(value) > 5 && strings.HasSuffix(value, "ment"):
		return strings.TrimSuffix(value, "ment")
	case len(value) > 5 && strings.HasSuffix(value, "ing"):
		return strings.TrimSuffix(value, "ing")
	case len(value) > 4 && strings.HasSuffix(value, "ies"):
		return strings.TrimSuffix(value, "ies") + "y"
	case len(value) > 4 && strings.HasSuffix(value, "s"):
		return strings.TrimSuffix(value, "s")
	default:
		return value
	}
}
