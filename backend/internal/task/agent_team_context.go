package task

import (
	"fmt"
	"sort"
	"strings"

	"automation-hub-backend/internal/frameworkregistry"
)

const (
	maxTaskAgentTeams              = 3
	agentTeamTaskAuthorityBoundary = "agent teams provide advisory coordination context only; consensus, membership, and team status cannot grant execution authority or consume approval"
)

type AgentTeamContextProvider interface {
	ActiveTeams(ownerIdentity, task, risk string, limit int) ([]frameworkregistry.AgentTeamContract, error)
}

type agentTeamServiceReader interface {
	ListTeams(owner string) ([]frameworkregistry.AgentTeamContract, error)
}

type agentTeamContextBridge struct {
	service agentTeamServiceReader
}

func NewAgentTeamContextProvider(service agentTeamServiceReader) AgentTeamContextProvider {
	return &agentTeamContextBridge{service: service}
}

func WithAgentTeamContext(base Service, provider AgentTeamContextProvider) (Service, error) {
	implementation, ok := base.(*service)
	if !ok {
		return nil, fmt.Errorf("agent-team context requires the built-in task service")
	}
	if provider == nil {
		return nil, fmt.Errorf("agent-team context provider is required")
	}
	implementation.agentTeams = provider
	return implementation, nil
}

func (p *agentTeamContextBridge) ActiveTeams(ownerIdentity, task, risk string, limit int) ([]frameworkregistry.AgentTeamContract, error) {
	if p == nil || p.service == nil {
		return nil, fmt.Errorf("agent-team context provider is unavailable")
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	if ownerIdentity == "" {
		return nil, fmt.Errorf("agent-team context requires a verified owner")
	}
	if limit < 1 || limit > maxTaskAgentTeams {
		limit = maxTaskAgentTeams
	}
	teams, err := p.service.ListTeams(ownerIdentity)
	if err != nil {
		return nil, err
	}
	type scoredTeam struct {
		team  frameworkregistry.AgentTeamContract
		score int
	}
	words := teamTaskWords(task)
	scored := make([]scoredTeam, 0, len(teams))
	for _, team := range teams {
		if team.Status != frameworkregistry.AgentTeamActive {
			continue
		}
		if !team.AdvisoryOnly || team.GrantsExecutionAuthority || !team.ExecutionAuthorizationRequired {
			return nil, fmt.Errorf("active agent team %s crossed its advisory authority boundary", team.ID)
		}
		if domainRiskRank(team.RiskCeiling) < domainRiskRank(risk) {
			continue
		}
		score := activeTeamMemberCount(team) * 2
		searchable := strings.ToLower(team.Name + " " + team.Purpose)
		for _, capability := range team.Capabilities {
			searchable += " " + strings.ToLower(capability.ID+" "+capability.Name+" "+capability.Description)
		}
		for _, word := range words {
			if strings.Contains(searchable, word) {
				score += 5
			}
		}
		scored = append(scored, scoredTeam{team: team, score: score})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		if !scored[i].team.UpdatedAt.Equal(scored[j].team.UpdatedAt) {
			return scored[i].team.UpdatedAt.After(scored[j].team.UpdatedAt)
		}
		return scored[i].team.ID < scored[j].team.ID
	})
	if len(scored) > limit {
		scored = scored[:limit]
	}
	result := make([]frameworkregistry.AgentTeamContract, 0, len(scored))
	for _, selected := range scored {
		result = append(result, selected.team)
	}
	return result, nil
}

func (s *service) selectTaskAgentTeams(request IntakeRequest, risk RiskAssessment) ([]frameworkregistry.AgentTeamContract, string, error) {
	if s == nil || s.agentTeams == nil {
		return nil, "Agent-team coordination context is not configured.", nil
	}
	if strings.TrimSpace(request.OwnerIdentity) == "" {
		return nil, "Agent-team coordination was skipped because no verified owner is available.", nil
	}
	teams, err := s.agentTeams.ActiveTeams(request.OwnerIdentity, request.Request, risk.Level, maxTaskAgentTeams)
	if err != nil {
		return nil, "", err
	}
	if len(teams) == 0 {
		return nil, "No active advisory agent team matched the task and risk ceiling.", nil
	}
	return teams, fmt.Sprintf("Selected %d active advisory agent team contracts; separate final execution authorization remains required.", len(teams)), nil
}

func activeTeamMemberCount(team frameworkregistry.AgentTeamContract) int {
	count := 0
	for _, member := range team.Members {
		if member.Status == frameworkregistry.TeamMemberActive {
			count++
		}
	}
	return count
}

func teamTaskWords(task string) []string {
	set := make(map[string]struct{})
	for _, word := range strings.Fields(strings.ToLower(task)) {
		word = strings.Trim(word, " .,:;!?()[]{}\"'")
		if len(word) >= 4 {
			set[word] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for word := range set {
		result = append(result, word)
	}
	sort.Strings(result)
	return result
}

func applyAgentTeamExecution(plan ExecutionPlan, teams []frameworkregistry.AgentTeamContract) ExecutionPlan {
	if len(teams) == 0 {
		return plan
	}
	plan.AgentTeams = append([]frameworkregistry.AgentTeamContract(nil), teams...)
	plan.AgentTeamAuthorityBoundary = agentTeamTaskAuthorityBoundary
	plan.AuditEvents = uniqueStrings(append(plan.AuditEvents,
		"active advisory agent teams selected",
		"agent team authority boundary evaluated",
	))
	return plan
}
