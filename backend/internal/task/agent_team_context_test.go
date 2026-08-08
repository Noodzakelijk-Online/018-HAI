package task

import (
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/frameworkregistry"
)

type fakeAgentTeamReader struct {
	teams []frameworkregistry.AgentTeamContract
	err   error
}

func (f fakeAgentTeamReader) ListTeams(string) ([]frameworkregistry.AgentTeamContract, error) {
	return append([]frameworkregistry.AgentTeamContract(nil), f.teams...), f.err
}

func TestAgentTeamContextSelectsOnlyActiveRiskCompatibleAdvisoryTeams(t *testing.T) {
	now := time.Now().UTC()
	bridge := NewAgentTeamContextProvider(fakeAgentTeamReader{teams: []frameworkregistry.AgentTeamContract{
		{ID: "draft", Status: frameworkregistry.AgentTeamDraft, AdvisoryOnly: true, ExecutionAuthorizationRequired: true, RiskCeiling: frameworkregistry.TeamRiskCritical},
		{ID: "low", Status: frameworkregistry.AgentTeamActive, AdvisoryOnly: true, ExecutionAuthorizationRequired: true, RiskCeiling: frameworkregistry.TeamRiskLow},
		{
			ID: "legal-team", Name: "Legal evidence team", Purpose: "Review legal evidence", Status: frameworkregistry.AgentTeamActive,
			AdvisoryOnly: true, ExecutionAuthorizationRequired: true, RiskCeiling: frameworkregistry.TeamRiskHigh, UpdatedAt: now,
			Capabilities: []frameworkregistry.TeamCapabilityContract{{ID: "evidence", Name: "Evidence review", Description: "legal source checks"}},
			Members:      []frameworkregistry.TeamMembership{{ID: "member-1", Status: frameworkregistry.TeamMemberActive}},
		},
		{
			ID: "general-team", Name: "General planning", Purpose: "General work", Status: frameworkregistry.AgentTeamActive,
			AdvisoryOnly: true, ExecutionAuthorizationRequired: true, RiskCeiling: frameworkregistry.TeamRiskCritical, UpdatedAt: now.Add(-time.Hour),
		},
	}})

	teams, err := bridge.ActiveTeams("owner-1", "review legal evidence", frameworkregistry.TeamRiskHigh, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != 2 || teams[0].ID != "legal-team" || teams[1].ID != "general-team" {
		t.Fatalf("selected teams = %#v", teams)
	}
}

func TestAgentTeamContextRejectsAuthorityEscalation(t *testing.T) {
	bridge := NewAgentTeamContextProvider(fakeAgentTeamReader{teams: []frameworkregistry.AgentTeamContract{{
		ID: "unsafe", Status: frameworkregistry.AgentTeamActive, RiskCeiling: frameworkregistry.TeamRiskCritical,
		AdvisoryOnly: false, GrantsExecutionAuthority: true, ExecutionAuthorizationRequired: false,
	}}})
	if _, err := bridge.ActiveTeams("owner-1", "task", "low", 1); err == nil || !strings.Contains(err.Error(), "authority boundary") {
		t.Fatalf("err = %v", err)
	}
}

func TestAgentTeamContextEnrichesExecutionPlanWithoutAuthority(t *testing.T) {
	team := frameworkregistry.AgentTeamContract{
		ID: "team-1", Status: frameworkregistry.AgentTeamActive, AdvisoryOnly: true,
		ExecutionAuthorizationRequired: true, RiskCeiling: frameworkregistry.TeamRiskHigh,
	}
	plan := applyAgentTeamExecution(ExecutionPlan{}, []frameworkregistry.AgentTeamContract{team})
	if len(plan.AgentTeams) != 1 || plan.AgentTeamAuthorityBoundary != agentTeamTaskAuthorityBoundary {
		t.Fatalf("execution plan = %#v", plan)
	}
	if !strings.Contains(strings.Join(plan.AuditEvents, " "), "authority boundary") {
		t.Fatalf("audit events = %#v", plan.AuditEvents)
	}
}

func TestWithAgentTeamContextRequiresBuiltInServiceAndProvider(t *testing.T) {
	provider := NewAgentTeamContextProvider(fakeAgentTeamReader{})
	if _, err := WithAgentTeamContext(nil, provider); err == nil {
		t.Fatal("expected built-in service requirement")
	}
	if _, err := WithAgentTeamContext(&service{}, nil); err == nil {
		t.Fatal("expected provider requirement")
	}
}
