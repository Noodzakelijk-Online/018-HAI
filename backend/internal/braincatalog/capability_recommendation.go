package braincatalog

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

const maxCapabilityRecommendations = 6

type capabilityTerm struct {
	Value    string
	Expanded bool
}

type CapabilityRecommendation struct {
	Recommendation
	UpstreamURL      string            `json:"upstreamUrl"`
	SourceCatalogURL string            `json:"sourceCatalogUrl"`
	SourceCollection string            `json:"sourceCollection,omitempty"`
	VerifiedAt       string            `json:"verifiedAt"`
	VerificationNote string            `json:"verificationNote"`
	Score            int               `json:"score"`
	MatchedTerms     []string          `json:"matchedTerms"`
	Reasons          []string          `json:"reasons"`
	NextStep         string            `json:"nextStep"`
	RoadmapPriority  int               `json:"roadmapPriority"`
	RoadmapReason    string            `json:"roadmapReason"`
	CapabilityPlanes []CapabilityPlane `json:"capabilityPlanes"`
}

type CapabilityRecommendationResponse struct {
	Need            string                     `json:"need"`
	ExpandedTerms   []string                   `json:"expandedTerms"`
	Recommendations []CapabilityRecommendation `json:"recommendations"`
	Message         string                     `json:"message"`
}

// RecommendForNeed ranks existing HAI catalog records using only the supplied
// need and reviewed catalog text. It neither probes upstreams nor changes an
// entry's adoption status, configuration, or runtime state.
func RecommendForNeed(need string) (CapabilityRecommendationResponse, error) {
	need = strings.TrimSpace(need)
	terms := capabilityTerms(need)
	if len(terms) == 0 {
		return CapabilityRecommendationResponse{}, fmt.Errorf("describe a capability need using at least one specific word")
	}
	response := CapabilityRecommendationResponse{Need: need, ExpandedTerms: capabilityTermValues(terms)}
	roadmapByID := map[string]AdoptionPlanItem{}
	for _, item := range AdoptionPlanReport().Items {
		roadmapByID[item.ID] = item
	}
	for _, entry := range Entries() {
		if entry.Status == StatusExcluded || entry.Status == StatusReferenceOnly || entry.Status == StatusLicenseReview {
			continue
		}
		recommendation := recommendationForEntry(entry, terms, roadmapByID[entry.ID])
		if recommendation.Score > 0 {
			response.Recommendations = append(response.Recommendations, recommendation)
		}
	}
	sort.SliceStable(response.Recommendations, func(i, j int) bool {
		left, right := response.Recommendations[i], response.Recommendations[j]
		if left.Score != right.Score {
			return left.Score > right.Score
		}
		if left.RoadmapPriority != right.RoadmapPriority {
			return left.RoadmapPriority > right.RoadmapPriority
		}
		if left.Status != right.Status {
			return left.Status == StatusIntegrated
		}
		return left.Name < right.Name
	})
	response.Recommendations = selectCapabilityRecommendations(response.Recommendations, terms)
	if len(response.Recommendations) == 0 {
		response.Message = "No reviewed HAI profile or review-first candidate matched this need. HAI did not search, install, or activate external projects."
	} else {
		response.Message = "Results are ranked by task relevance, then HAI's reviewed roadmap priority. A recommendation is planning context only: profile configuration, adapter review, and existing approval gates remain required."
	}
	return response, nil
}

func recommendationForEntry(entry Entry, terms []capabilityTerm, roadmap AdoptionPlanItem) CapabilityRecommendation {
	recommendation := CapabilityRecommendation{Recommendation: Recommendation{
		ID: entry.ID, Name: entry.Name, Status: entry.Status, Role: entry.Category, Rationale: entry.Rationale,
		RequiresApproval: entry.RequiresApproval, Activation: entry.Activation, ControlMappings: append([]ControlMapping(nil), entry.ControlMappings...),
	}, UpstreamURL: entry.UpstreamURL, SourceCatalogURL: entry.SourceCatalogURL, SourceCollection: entry.SourceCollection,
		VerifiedAt: entry.VerifiedAt, VerificationNote: entry.VerificationNote,
		RoadmapPriority: roadmap.Priority, RoadmapReason: roadmap.PriorityReason,
		CapabilityPlanes: append([]CapabilityPlane(nil), roadmap.Planes...)}
	for _, term := range terms {
		token := term.Value
		weight := capabilityTermWeight(term)
		if matchesCapabilityText(token, entry.Name) || matchesCapabilityText(token, entry.Category) {
			recommendation.Score += 5 * weight
			recommendation.MatchedTerms = appendUniqueTerm(recommendation.MatchedTerms, token)
			recommendation.Reasons = append(recommendation.Reasons, "matches profile or role: "+token)
		}
		for _, capability := range entry.Capabilities {
			if matchesCapabilityText(token, capability) {
				recommendation.Score += 3 * weight
				recommendation.MatchedTerms = appendUniqueTerm(recommendation.MatchedTerms, token)
				recommendation.Reasons = append(recommendation.Reasons, "matches capability: "+capability)
			}
		}
		for _, use := range entry.RecommendedFor {
			if matchesCapabilityText(token, use) {
				recommendation.Score += 4 * weight
				recommendation.MatchedTerms = appendUniqueTerm(recommendation.MatchedTerms, token)
				recommendation.Reasons = append(recommendation.Reasons, "matches intended use: "+use)
			}
		}
	}
	if recommendation.Score == 0 {
		return recommendation
	}
	if recommendation.RoadmapPriority > 0 && recommendation.RoadmapReason != "" {
		recommendation.Reasons = append(recommendation.Reasons, fmt.Sprintf("roadmap priority %d: %s", recommendation.RoadmapPriority, recommendation.RoadmapReason))
	}
	switch entry.Status {
	case StatusIntegrated:
		recommendation.Score += 2
		recommendation.NextStep = "Open the existing HAI profile and confirm local configuration and live health before using it."
	case StatusCandidate:
		recommendation.NextStep = "Create a manual adapter review. Do not install, configure, or run the candidate from this recommendation."
	case StatusCompatibility:
		recommendation.NextStep = "Review the compatibility boundary and approval model before designing a bridge."
	}
	return recommendation
}

func selectCapabilityRecommendations(ranked []CapabilityRecommendation, terms []capabilityTerm) []CapabilityRecommendation {
	if len(ranked) <= maxCapabilityRecommendations {
		return ranked
	}
	selected := make([]CapabilityRecommendation, 0, maxCapabilityRecommendations)
	selectedIDs := map[string]bool{}
	for _, term := range terms {
		if term.Expanded || isGenericCapabilityTerm(term.Value) {
			continue
		}
		for _, recommendation := range ranked {
			if !containsCapabilityTerm(recommendation.MatchedTerms, term.Value) || selectedIDs[recommendation.ID] {
				continue
			}
			selected = append(selected, recommendation)
			selectedIDs[recommendation.ID] = true
			break
		}
		if len(selected) == maxCapabilityRecommendations {
			return selected
		}
	}
	for _, recommendation := range ranked {
		if selectedIDs[recommendation.ID] {
			continue
		}
		selected = append(selected, recommendation)
		selectedIDs[recommendation.ID] = true
		if len(selected) == maxCapabilityRecommendations {
			break
		}
	}
	return selected
}

func appendUniqueTerm(values []string, value string) []string {
	if containsCapabilityTerm(values, value) {
		return values
	}
	return append(values, value)
}

func containsCapabilityTerm(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func capabilityTerms(value string) []capabilityTerm {
	stopWords := map[string]bool{"a": true, "an": true, "and": true, "for": true, "from": true, "have": true, "i": true, "in": true, "is": true, "me": true, "my": true, "need": true, "of": true, "or": true, "the": true, "to": true, "use": true, "with": true}
	aliases := map[string][]string{
		"audio":      {"speech", "transcription"},
		"automation": {"workflow"},
		"browser":    {"web"},
		"code":       {"coding", "repository"},
		"crawl":      {"browser", "web"},
		"github":     {"repository"},
		"llm":        {"model", "inference"},
		"pii":        {"sensitive", "redaction"},
		"privacy":    {"sensitive", "redaction"},
		"scraping":   {"browser", "web"},
		"voice":      {"speech", "transcription"},
		"website":    {"browser", "web"},
	}
	parts := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	seen := map[string]bool{}
	result := []capabilityTerm{}
	for _, part := range parts {
		part = singularCapabilityTerm(part)
		if len(part) < 3 || stopWords[part] || seen[part] {
			continue
		}
		seen[part] = true
		result = append(result, capabilityTerm{Value: part})
		for _, alias := range aliases[part] {
			if !seen[alias] {
				seen[alias] = true
				result = append(result, capabilityTerm{Value: alias, Expanded: true})
			}
		}
	}
	return result
}

func capabilityTermValues(terms []capabilityTerm) []string {
	values := make([]string, 0, len(terms))
	for _, term := range terms {
		values = append(values, term.Value)
	}
	return values
}

func capabilityTermWeight(term capabilityTerm) int {
	if term.Expanded || isGenericCapabilityTerm(term.Value) {
		return 1
	}
	return 4
}

func isGenericCapabilityTerm(value string) bool {
	switch value {
	case "agent", "browser", "code", "coding", "inference", "llm", "local", "model", "repository", "tool", "web", "workflow":
		return true
	default:
		return false
	}
}

func singularCapabilityTerm(value string) string {
	switch {
	case strings.HasSuffix(value, "ies") && len(value) > 4:
		return strings.TrimSuffix(value, "ies") + "y"
	case strings.HasSuffix(value, "ses") && len(value) > 4:
		return strings.TrimSuffix(value, "es")
	case strings.HasSuffix(value, "s") && !strings.HasSuffix(value, "ss") && len(value) > 3:
		return strings.TrimSuffix(value, "s")
	default:
		return value
	}
}

func matchesCapabilityText(token, value string) bool {
	return strings.Contains(strings.ToLower(value), token)
}
