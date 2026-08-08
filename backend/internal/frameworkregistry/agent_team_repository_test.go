package frameworkregistry

import (
	"testing"
	"time"
)

func TestMemoryAgentTeamRepositoryPersistsAcrossServicesAndScopesOwners(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 31, 15, 0, 0, 0, time.UTC)
	repo := NewMemoryAgentTeamRepository()
	writer := newAgentTeamService(repo, func() time.Time { return now }, deterministicTeamIDs("shared"))
	team, err := writer.CreateTeam("robert", testTeamRequest(now))
	if err != nil {
		t.Fatal(err)
	}
	reader := newAgentTeamService(repo, func() time.Time { return now }, deterministicTeamIDs("reader"))
	stored, err := reader.GetTeam("robert", team.ID, team.Version)
	if err != nil || stored.ContractDigest != team.ContractDigest {
		t.Fatalf("shared repository did not preserve team: %#v, err %v", stored, err)
	}
	if _, err := reader.GetTeam("other-owner", team.ID, team.Version); err != ErrAgentTeamNotFound {
		t.Fatalf("owner scope leaked team: %v", err)
	}

	stored.Roles[0].Name = "mutated outside repository"
	storedAgain, err := reader.GetTeam("robert", team.ID, team.Version)
	if err != nil {
		t.Fatal(err)
	}
	if storedAgain.Roles[0].Name == stored.Roles[0].Name {
		t.Fatal("repository returned aliased team state")
	}
	events, err := reader.Events("robert", team.ID, team.Version)
	if err != nil || len(events) != 1 || events[0].Type != TeamEventCreated {
		t.Fatalf("repository event history = %#v, err %v", events, err)
	}
}
