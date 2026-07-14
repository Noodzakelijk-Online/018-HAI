package autonomypolicy

import (
	"strings"

	"automation-hub-backend/internal/operations"
)

// Always-high-risk domains from Robert's taxonomy (§25). Any hit forces high
// risk and mandatory approval.
var highRiskKeywords = []string{
	"government", "municipality", "court", "lawyer", "landlord",
	"housing corporation", "debt collection", "insurance", "medical",
	"banking", "bank ", "payment", "invoice", "tax", "taxes", "contract",
	"legal", "hiring", "firing", "dispute", "deletion", "credential",
	"account settings", "password", "deadline commitment", "public posting",
}

// Medium-risk indicators (drafts, outreach, visible updates).
var mediumRiskKeywords = []string{
	"draft", "follow-up", "follow up", "reply", "github comment", "comment",
	"calendar", "outreach", "upwork", "trello", "review", "pull request",
}

// ClassifyRisk maps text to a Robert-taxonomy risk level (§25). High wins over
// medium wins over low.
func ClassifyRisk(text string) operations.RiskLevel {
	t := strings.ToLower(text)
	for _, k := range highRiskKeywords {
		if strings.Contains(t, k) {
			return operations.RiskHigh
		}
	}
	for _, k := range mediumRiskKeywords {
		if strings.Contains(t, k) {
			return operations.RiskMedium
		}
	}
	return operations.RiskLow
}
