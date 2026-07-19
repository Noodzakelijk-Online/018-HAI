package braincatalog

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

const maxCapabilityRecommendations = 6

type CapabilityRecommendation struct {
	Recommendation
	UpstreamURL      string   `json:"upstreamUrl"`
	SourceCatalogURL string   `json:"sourceCatalogUrl"`
	SourceCollection string   `json:"sourceCollection,omitempty"`
	VerifiedAt       string   `json:"verifiedAt"`
	VerificationNote string   `json:"verificationNote"`
	Score            int      `json:"score"`
	Reasons          []string `json:"reasons"`
	NextStep         string   `json:"nextStep"`
}

type CapabilityRecommendationResponse struct {
	Need            string                     `json:"need"`
	Recommendations []CapabilityRecommendation `json:"recommendations"`
	Message         string                     `json:"message"`
}

// RecommendForNeed ranks existing HAI catalog records using only the supplied
// need and reviewed catalog text. It neither probes upstreams nor changes an
// entry's adoption status, configuration, or runtime state.
func RecommendForNeed(need string) (CapabilityRecommendationResponse, error) {
	need = strings.TrimSpace(need)
	tokens := capabilityTokens(need)
	if len(tokens) == 0 {
		return CapabilityRecommendationResponse{}, fmt.Errorf("describe a capability need using at least one specific word")
	}
	response := CapabilityRecommendationResponse{Need: need}
	for _, entry := range Entries() {
		if entry.Status == StatusExcluded || entry.Status == StatusReferenceOnly || entry.Status == StatusLicenseReview {
			continue
		}
		recommendation := recommendationForEntry(entry, tokens)
		if recommendation.Score > 0 {
			response.Recommendations = append(response.Recommendations, recommendation)
		}
	}
	sort.SliceStable(response.Recommendations, func(i, j int) bool {
		left, right := response.Recommendations[i], response.Recommendations[j]
		if left.Score != right.Score {
			return left.Score > right.Score
		}
		if left.Status != right.Status {
			return left.Status == StatusIntegrated
		}
		return left.Name < right.Name
	})
	if len(response.Recommendations) > maxCapabilityRecommendations {
		response.Recommendations = response.Recommendations[:maxCapabilityRecommendations]
	}
	if len(response.Recommendations) == 0 {
		response.Message = "No reviewed HAI profile or review-first candidate matched this need. HAI did not search, install, or activate external projects."
	} else {
		response.Message = "Results are ranked from HAI's reviewed catalog. A recommendation is planning context only: profile configuration, adapter review, and existing approval gates remain required."
	}
	return response, nil
}

func recommendationForEntry(entry Entry, tokens []string) CapabilityRecommendation {
	recommendation := CapabilityRecommendation{Recommendation: Recommendation{
		ID: entry.ID, Name: entry.Name, Status: entry.Status, Role: entry.Category, Rationale: entry.Rationale,
		RequiresApproval: entry.RequiresApproval, Activation: entry.Activation, ControlMappings: append([]ControlMapping(nil), entry.ControlMappings...),
	}, UpstreamURL: entry.UpstreamURL, SourceCatalogURL: entry.SourceCatalogURL, SourceCollection: entry.SourceCollection,
		VerifiedAt: entry.VerifiedAt, VerificationNote: entry.VerificationNote}
	for _, token := range tokens {
		if matchesCapabilityText(token, entry.Name) || matchesCapabilityText(token, entry.Category) {
			recommendation.Score += 5
			recommendation.Reasons = append(recommendation.Reasons, "matches profile or role: "+token)
		}
		for _, capability := range entry.Capabilities {
			if matchesCapabilityText(token, capability) {
				recommendation.Score += 3
				recommendation.Reasons = append(recommendation.Reasons, "matches capability: "+capability)
			}
		}
		for _, use := range entry.RecommendedFor {
			if matchesCapabilityText(token, use) {
				recommendation.Score += 4
				recommendation.Reasons = append(recommendation.Reasons, "matches intended use: "+use)
			}
		}
	}
	if recommendation.Score == 0 {
		return recommendation
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

func capabilityTokens(value string) []string {
	stopWords := map[string]bool{"a": true, "an": true, "and": true, "for": true, "from": true, "have": true, "i": true, "in": true, "is": true, "me": true, "my": true, "need": true, "of": true, "or": true, "the": true, "to": true, "use": true, "with": true}
	parts := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	seen := map[string]bool{}
	result := []string{}
	for _, part := range parts {
		if len(part) < 3 || stopWords[part] || seen[part] {
			continue
		}
		seen[part] = true
		result = append(result, part)
	}
	return result
}

func matchesCapabilityText(token, value string) bool {
	return strings.Contains(strings.ToLower(value), token)
}
