package agentregistry

import (
	"strings"
	"testing"

	"automation-hub-backend/migrations"
)

func TestAgentRegistryMigrationContract(t *testing.T) {
	up, err := migrations.Files.ReadFile("pre/0013_agent_registry.up.sql")
	if err != nil {
		t.Fatalf("read up migration: %v", err)
	}
	down, err := migrations.Files.ReadFile("pre/0013_agent_registry.down.sql")
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}
	upSQL := string(up)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS public.agent_registry_agents",
		"PRIMARY KEY (owner_identity, id)",
		"revision bigint NOT NULL",
		"payload jsonb NOT NULL",
		"payload ?& ARRAY[",
		"CREATE TABLE IF NOT EXISTS public.agent_registry_revisions",
		"PRIMARY KEY (owner_identity, agent_id, revision)",
		"CREATE TABLE IF NOT EXISTS public.agent_registry_transitions",
		"UNIQUE (owner_identity, agent_id, revision)",
		"CREATE TABLE IF NOT EXISTS public.agent_registry_assignments",
		"FOREIGN KEY (owner_identity, agent_id)",
		"FOREIGN KEY (owner_identity, agent_id, agent_revision)",
		"CREATE TABLE IF NOT EXISTS public.agent_registry_assignment_outcomes",
		"FOREIGN KEY (owner_identity, assignment_id, agent_id)",
		"NEW.revision <> OLD.revision + 1",
		"DROP TRIGGER IF EXISTS trg_agent_registry_agent_revision",
		"trg_agent_registry_revisions_immutable",
		"trg_agent_registry_revisions_no_truncate",
		"trg_agent_registry_transitions_immutable",
		"trg_agent_registry_transitions_no_truncate",
		"trg_agent_registry_assignments_immutable",
		"trg_agent_registry_assignments_no_truncate",
		"trg_agent_registry_outcomes_immutable",
		"trg_agent_registry_outcomes_no_truncate",
		"agent registry audit records are append-only",
	} {
		if !strings.Contains(upSQL, fragment) {
			t.Errorf("up migration missing %q", fragment)
		}
	}
	downSQL := string(down)
	for _, fragment := range []string{
		"DROP TRIGGER IF EXISTS trg_agent_registry_outcomes_no_truncate",
		"DROP TRIGGER IF EXISTS trg_agent_registry_outcomes_immutable",
		"DROP TRIGGER IF EXISTS trg_agent_registry_assignments_no_truncate",
		"DROP TRIGGER IF EXISTS trg_agent_registry_transitions_no_truncate",
		"DROP TRIGGER IF EXISTS trg_agent_registry_revisions_no_truncate",
		"DROP FUNCTION IF EXISTS public.hai_reject_agent_registry_audit_mutation()",
		"DROP FUNCTION IF EXISTS public.hai_enforce_agent_registry_revision()",
		"DROP TABLE IF EXISTS public.agent_registry_assignment_outcomes",
		"DROP TABLE IF EXISTS public.agent_registry_assignments",
		"DROP TABLE IF EXISTS public.agent_registry_transitions",
		"DROP TABLE IF EXISTS public.agent_registry_revisions",
		"DROP TABLE IF EXISTS public.agent_registry_agents",
	} {
		if !strings.Contains(downSQL, fragment) {
			t.Errorf("down migration missing %q", fragment)
		}
	}
}
