package migrations

import (
	"strings"
	"testing"
)

func TestAgentTeamLifecycleMigrationIsOwnerScopedAppendOnlyAndReversible(t *testing.T) {
	t.Parallel()

	upBytes, err := Files.ReadFile("pre/0018_agent_team_lifecycle.up.sql")
	if err != nil {
		t.Fatalf("read agent team lifecycle migration: %v", err)
	}
	downBytes, err := Files.ReadFile("pre/0018_agent_team_lifecycle.down.sql")
	if err != nil {
		t.Fatalf("read agent team lifecycle rollback: %v", err)
	}
	up := strings.ToLower(string(upBytes))
	down := strings.ToLower(string(downBytes))

	for _, fragment := range []string{
		"primary key (owner_identity, team_id, team_version)",
		"unique (owner_identity, team_id, team_version, idempotency_key)",
		"previous_event_digest",
		"new.previous_event_digest <> prior_digest",
		"new.revision <> old.revision + 1",
		"deferrable initially deferred",
		"agent team revision requires a matching lifecycle event",
		"consensus outcome revision does not match current team contract",
		"agent team ledger records are append-only",
		"payload #>> '{advisoryonly}' = 'true'",
		"payload #>> '{grantsexecutionauthority}' = 'false'",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("agent team up migration missing %q", fragment)
		}
	}
	if strings.Contains(up, " cascade") || strings.Contains(down, " cascade") {
		t.Fatal("agent team migration must not use CASCADE")
	}
	for _, fragment := range []string{
		"refusing to roll back non-empty agent team lifecycle tables",
		"drop table if exists public.agent_team_consensus_outcomes",
		"drop table if exists public.agent_team_coordination_messages",
		"drop table if exists public.agent_team_lifecycle_events",
		"drop table if exists public.agent_team_contracts",
		"drop table if exists public.agent_teams",
		"drop function if exists public.hai_reject_agent_team_append_only_mutation()",
	} {
		if !strings.Contains(down, fragment) {
			t.Errorf("agent team down migration missing %q", fragment)
		}
	}
}
