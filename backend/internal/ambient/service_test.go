package ambient

import (
	"automation-hub-backend/internal/models"
	"testing"
)

func TestOpportunityScoreRewardsNeedGapAndCapability(t *testing.T) {
	need := models.AmbientNeed{CurrentLevel: 20, TargetLevel: 90, PriorityWeight: 90}
	strong := models.AmbientOpportunity{Urgency: 80, Impact: 85, Effort: 20, Confidence: 90, Risk: 20}
	weak := models.AmbientOpportunity{Urgency: 30, Impact: 35, Effort: 80, Confidence: 40, Risk: 60}
	if opportunityScore(strong, need) <= opportunityScore(weak, need) {
		t.Fatalf("expected capable high-impact opportunity to rank higher")
	}
}

func TestFingerprintIsStableAndSourceScoped(t *testing.T) {
	first := fingerprint("workflow", "123", "growth")
	second := fingerprint("workflow", "123", "growth")
	other := fingerprint("workflow", "124", "growth")
	if first != second {
		t.Fatalf("expected stable fingerprint")
	}
	if first == other {
		t.Fatalf("expected source identity to change fingerprint")
	}
}

func TestNeedForWorkflowPrioritizesSafety(t *testing.T) {
	item := models.WorkflowItem{Title: "Reply to lawyer", RiskLevel: "high"}
	if got := needForWorkflow(item); got != "safety" {
		t.Fatalf("expected safety, got %s", got)
	}
}
