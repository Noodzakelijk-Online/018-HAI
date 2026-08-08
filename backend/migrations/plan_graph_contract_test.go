package migrations

import (
	"strings"
	"testing"
)

func TestPlanGraphMigrationCreatesImmutableOwnerScopedRevisionLedger(t *testing.T) {
	upBytes, err := Files.ReadFile("pre/0052_plan_graph_contract.up.sql")
	if err != nil {
		t.Fatalf("read plan graph up migration: %v", err)
	}
	up := string(upBytes)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS public.plan_graph_revisions",
		"owner_identity character varying(255) NOT NULL",
		"UNIQUE (owner_identity, plan_id, revision)",
		"uq_plan_graph_owner_idempotency",
		"WHERE idempotency_key <> ''",
		"status IN ('draft', 'accepted')",
		"COALESCE(payload -> 'canExecute', 'true'::jsonb) = 'false'::jsonb",
		"payload ?& ARRAY['id', 'title', 'status', 'revision', 'digest', 'nodes', 'edges', 'createdBy', 'createdAt', 'canExecute']",
		"hai_validate_plan_graph_revision_insert",
		"plan graph revisions must be contiguous and append-only",
		"plan graph parent digest must bind the previous revision",
		"BEFORE UPDATE OR DELETE",
		"BEFORE TRUNCATE",
		"plan graph revisions are immutable",
		"Acceptance never grants execution authority",
		"hai-plan-graph-v1",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("plan graph migration missing %q", fragment)
		}
	}
	if strings.Contains(strings.ToUpper(up), " ON DELETE CASCADE") {
		t.Error("immutable plan graph history must not use cascading deletion")
	}

	downBytes, err := Files.ReadFile("pre/0052_plan_graph_contract.down.sql")
	if err != nil {
		t.Fatalf("read plan graph down migration: %v", err)
	}
	down := string(downBytes)
	for _, fragment := range []string{
		"refusing to roll back non-empty immutable plan graph revision history",
		"DROP TRIGGER IF EXISTS trg_plan_graph_revision_no_truncate",
		"DROP TRIGGER IF EXISTS trg_plan_graph_revision_immutable",
		"DROP TRIGGER IF EXISTS trg_plan_graph_revision_insert",
		"DROP TABLE IF EXISTS public.plan_graph_revisions",
	} {
		if !strings.Contains(down, fragment) {
			t.Errorf("plan graph rollback missing %q", fragment)
		}
	}
	if strings.Contains(strings.ToUpper(down), " CASCADE") {
		t.Error("plan graph rollback must not use CASCADE")
	}
}
