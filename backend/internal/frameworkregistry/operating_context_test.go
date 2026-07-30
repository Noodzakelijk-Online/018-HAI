package frameworkregistry

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBuildSelectionCreatesMultiDomainWholeLifeContract(t *testing.T) {
	decision, err := BuildSelection(
		testCatalog(t),
		testConstitution(),
		SelectionRequest{
			Request:   "Plan a doctor appointment, review the invoice budget, and schedule the home repair.",
			TaskType:  "cross-domain planning",
			RiskLevel: "medium",
		},
		time.Date(2026, time.July, 30, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("BuildSelection: %v", err)
	}

	for _, domainID := range []string{"financial", "health_wellbeing", "home_assets"} {
		if !hasLifeDomain(decision.LifeDomains, domainID) {
			t.Errorf("life domains %v do not contain %q", decision.LifeDomains, domainID)
		}
	}
	if len(decision.NeedsState) < 3 {
		t.Fatalf("needs state did not preserve cross-domain context: %#v", decision.NeedsState)
	}
	if decision.Capacity.Status != "unknown" || !decision.Capacity.NeedsReview {
		t.Fatalf("absent capacity was treated as known: %#v", decision.Capacity)
	}
	if decision.ChiefOfStaff.NeedsAttention == "" ||
		decision.ChiefOfStaff.ContextNeeded == "" ||
		decision.ChiefOfStaff.CompletionProof == "" {
		t.Fatalf("chief-of-staff questions were not answered: %#v", decision.ChiefOfStaff)
	}
	if len(decision.OperatingContractDigest) != 64 {
		t.Fatalf("operating contract digest = %q", decision.OperatingContractDigest)
	}
}

func TestBuildSelectionCoversAdditionalWholeLifeDomains(t *testing.T) {
	t.Parallel()

	decision, err := BuildSelection(
		testCatalog(t),
		testConstitution(),
		SelectionRequest{
			Request: "Plan groceries and nutrition, secure my online account, inventory the tools, schedule pet care, and organize a community sustainability event.",
		},
		time.Date(2026, time.July, 30, 12, 30, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("BuildSelection: %v", err)
	}
	for _, domainID := range []string{
		"animals_dependants",
		"community_civic",
		"digital_accounts",
		"environment_sustainability",
		"food_nutrition",
		"possessions_inventory",
	} {
		if !hasLifeDomain(decision.LifeDomains, domainID) {
			t.Errorf("life domains %v do not contain %q", decision.LifeDomains, domainID)
		}
	}
}

func TestBuildSelectionMarksStaleCapacityAndConstrainsPlanning(t *testing.T) {
	now := time.Date(2026, time.July, 30, 13, 0, 0, 0, time.UTC)
	captured := now.Add(-25 * time.Hour)
	decision, err := BuildSelection(
		testCatalog(t),
		testConstitution(),
		SelectionRequest{
			Request:  "Plan the next project steps around my available time.",
			TaskType: "planning",
			Capacity: &CapacitySnapshot{
				Status:               "available",
				Energy:               70,
				Attention:            60,
				TimeAvailableMinutes: 90,
				SourceLabel:          "operator capacity check-in",
				CapturedAt:           &captured,
				Confidence:           0.9,
			},
		},
		now,
	)
	if err != nil {
		t.Fatalf("BuildSelection: %v", err)
	}
	if decision.Capacity.Status != "stale" ||
		decision.Capacity.Fresh ||
		!decision.Capacity.NeedsReview ||
		decision.Capacity.PlanningStepLimit != 3 {
		t.Fatalf("stale capacity was not constrained: %#v", decision.Capacity)
	}
	if !containsStringFragment(decision.Capacity.Constraints, "older than 24 hours") {
		t.Fatalf("stale capacity reason missing: %#v", decision.Capacity.Constraints)
	}
}

func TestBuildSelectionClampsAgentAuthorityAndUsesOnlyFreshVerifiedAgents(t *testing.T) {
	now := time.Date(2026, time.July, 30, 14, 0, 0, 0, time.UTC)
	verifiedAt := now.Add(-time.Hour)
	decision, err := BuildSelection(
		testCatalog(t),
		testConstitution(),
		SelectionRequest{
			Request:    "Create a dependency-aware implementation plan.",
			TaskType:   "architecture",
			Difficulty: 5,
			AvailableAgents: []AgentCard{{
				ID:               "planner-runtime",
				Name:             "Planner runtime",
				Role:             "planner",
				Capabilities:     []string{"planning"},
				AllowedTools:     []string{"read-only repository inspection"},
				AuthorityCeiling: 10,
				Status:           "available",
				Provenance:       "runtime-health://planner-runtime",
				LastVerifiedAt:   &verifiedAt,
			}},
			PreferredCoordinationMode: "hierarchical",
		},
		now,
	)
	if err != nil {
		t.Fatalf("BuildSelection: %v", err)
	}
	planner := findAgentCard(decision.AgentCards, "planner_runtime")
	if planner == nil || !planner.Verified {
		t.Fatalf("fresh runtime agent was not verified: %#v", decision.AgentCards)
	}
	if planner.AuthorityCeiling > decision.MaximumAutonomyLevel {
		t.Fatalf("agent authority %d exceeded selection ceiling %d", planner.AuthorityCeiling, decision.MaximumAutonomyLevel)
	}
	if planner.Owner == "" ||
		planner.Purpose == "" ||
		len(planner.DomainCompetence) == 0 ||
		len(planner.DataAccessBoundaries) == 0 ||
		planner.CostProfile == "" ||
		len(planner.ReliabilityHistory) == 0 ||
		planner.InputSchema == "" ||
		planner.OutputSchema == "" ||
		len(planner.ExpectedEvidence) == 0 ||
		planner.EscalationRoute == "" ||
		planner.Availability == "" ||
		planner.Version == "" ||
		planner.HealthStatus == "" {
		t.Fatalf("agent card did not receive a complete bounded identity contract: %#v", planner)
	}
	if decision.Coordination.Mode != "hierarchical" {
		t.Fatalf("coordination mode = %q, want hierarchical", decision.Coordination.Mode)
	}
	for _, delegation := range decision.Delegations {
		if delegation.AuthorityCeiling > decision.MaximumAutonomyLevel {
			t.Fatalf("delegation exceeded authority ceiling: %#v", delegation)
		}
	}
}

func TestBuildSelectionNeverLeaksSecretsIntoOperatingContract(t *testing.T) {
	decision, err := BuildSelection(
		testCatalog(t),
		testConstitution(),
		SelectionRequest{
			Request:         "Draft a reply with api_key=do-not-expose and require approval.",
			NeedsApproval:   true,
			SuccessCriteria: []string{"send token=also-secret only after review"},
			ObservedNeeds: []NeedStateAssessment{{
				ID:          "safety",
				Level:       "api_key=need-secret",
				State:       "attention",
				Priority:    90,
				Confidence:  0.8,
				Evidence:    []string{"password=need-evidence-secret"},
				Source:      "token=need-source-secret",
				NeedsReview: true,
			}},
			AvailableAgents: []AgentCard{{
				ID:               "planner",
				Name:             "Planner api_key=agent-name-secret",
				Role:             "planner",
				Capabilities:     []string{"token=agent-capability-secret"},
				DomainCompetence: []string{"api_key=agent-domain-secret"},
				AllowedTools:     []string{"password=agent-tool-secret"},
				RequiredPermissions: []string{
					"token=agent-permission-secret",
				},
				DataAccessBoundaries: []string{"password=agent-boundary-secret"},
				CostProfile:          "api_key=agent-cost-secret",
				ReliabilityHistory:   []string{"token=agent-history-secret"},
				ExpectedEvidence:     []string{"password=agent-evidence-secret"},
				EscalationRoute:      "token=agent-escalation-secret",
				AuthorityCeiling: 3,
				Status:           "available",
				Provenance:       "token=agent-provenance-secret",
			}},
		},
		time.Date(2026, time.July, 30, 15, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("BuildSelection: %v", err)
	}
	encoded, err := json.Marshal(struct {
		Needs        []NeedStateAssessment `json:"needs"`
		Agents       []AgentCard           `json:"agents"`
		Delegations  []DelegationContract  `json:"delegations"`
		ChiefOfStaff ChiefOfStaffDecision  `json:"chiefOfStaff"`
	}{
		Needs:        decision.NeedsState,
		Agents:       decision.AgentCards,
		Delegations:  decision.Delegations,
		ChiefOfStaff: decision.ChiefOfStaff,
	})
	if err != nil {
		t.Fatalf("marshal operating contract: %v", err)
	}
	if strings.Contains(string(encoded), "do-not-expose") {
		t.Fatalf("operating contract exposed secret: %s", encoded)
	}
	for _, secret := range []string{
		"also-secret",
		"need-secret",
		"need-evidence-secret",
		"need-source-secret",
		"agent-name-secret",
		"agent-capability-secret",
		"agent-domain-secret",
		"agent-tool-secret",
		"agent-permission-secret",
		"agent-boundary-secret",
		"agent-cost-secret",
		"agent-history-secret",
		"agent-evidence-secret",
		"agent-escalation-secret",
		"agent-provenance-secret",
	} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("operating contract exposed %q: %s", secret, encoded)
		}
	}
}

func TestAutonomyLevelNamesMatchTheConstitutionalZeroToTenLadder(t *testing.T) {
	t.Parallel()

	want := []string{
		"observe_only",
		"inform",
		"recommend",
		"draft",
		"plan_and_simulate",
		"prepare_action",
		"execute_after_case_specific_approval",
		"execute_under_standing_approval",
		"execute_reversible_low_risk_automatically",
		"execute_and_notify",
		"fully_autonomous_inside_bounded_mandate",
	}
	for level, name := range want {
		if got := autonomyLevelName(level); got != name {
			t.Errorf("autonomy level %d = %q, want %q", level, got, name)
		}
	}
}

func TestDelegationContractCarriesBudgetDeadlineAndConstraints(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 30, 17, 0, 0, 0, time.UTC)
	deadline := now.Add(48 * time.Hour)
	decision, err := BuildSelection(
		testCatalog(t),
		testConstitution(),
		SelectionRequest{
			Request:  "Prepare the source-backed project plan.",
			Deadline: &deadline,
			Capacity: &CapacitySnapshot{
				Status:               "available",
				Energy:               70,
				Attention:            70,
				TimeAvailableMinutes: 90,
				Constraints:          []string{"keep the plan within one work session"},
				SourceLabel:          "operator check-in",
				CapturedAt:           &now,
				Confidence:           1,
			},
		},
		now,
	)
	if err != nil {
		t.Fatalf("BuildSelection: %v", err)
	}
	if len(decision.Delegations) == 0 {
		t.Fatal("selection did not produce delegation contracts")
	}
	delegation := decision.Delegations[0]
	if delegation.BudgetLimitEUR != 0 || delegation.BudgetPolicy != "no_spend_authorized" {
		t.Fatalf("delegation manufactured financial authority: %#v", delegation)
	}
	if delegation.Deadline == nil || !delegation.Deadline.Equal(deadline) ||
		delegation.DeadlineStatus != "scheduled" {
		t.Fatalf("delegation deadline was not preserved: %#v", delegation)
	}
	if !containsStringFragment(delegation.Constraints, "one work session") ||
		!containsStringFragment(delegation.Constraints, "no financial expenditure") {
		t.Fatalf("delegation constraints are incomplete: %#v", delegation.Constraints)
	}
}

func hasLifeDomain(domains []LifeDomainAssignment, id string) bool {
	for _, domain := range domains {
		if domain.ID == id {
			return true
		}
	}
	return false
}

func findAgentCard(cards []AgentCard, id string) *AgentCard {
	for index := range cards {
		if cards[index].ID == id {
			return &cards[index]
		}
	}
	return nil
}
