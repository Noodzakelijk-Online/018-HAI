package migrations

import (
	"strings"
	"testing"
)

func TestWorkflowCoordinationPlanBindingMigrationContract(t *testing.T) {
	upBytes, err := Files.ReadFile("pre/0053_workflow_coordination_plan_binding.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := string(upBytes)
	for _, required := range []string{
		"uq_plan_graph_owner_plan_revision_digest",
		"ADD COLUMN coordination_plan_id uuid",
		"coordination_plan_revision bigint NOT NULL DEFAULT 0",
		"coordination_plan_digest ~ '^[0-9a-f]{64}$'",
		"coordination_plan_node_id <> ''",
		"FOREIGN KEY (owner_identity, coordination_plan_id, coordination_plan_revision, coordination_plan_digest)",
		"REFERENCES public.plan_graph_revisions (owner_identity, plan_id, revision, digest)",
		"ON UPDATE RESTRICT",
		"ON DELETE RESTRICT",
		"grants no execution authority",
	} {
		if !strings.Contains(up, required) {
			t.Errorf("workflow coordination plan binding migration missing %q", required)
		}
	}
	if strings.Contains(strings.ToUpper(up), " CASCADE") {
		t.Fatal("workflow coordination plan binding must not cascade immutable evidence")
	}

	downBytes, err := Files.ReadFile("pre/0053_workflow_coordination_plan_binding.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	down := string(downBytes)
	if !strings.Contains(down, "refusing to remove non-empty workflow coordination plan bindings") {
		t.Fatal("rollback must refuse to discard durable workflow plan provenance")
	}
	if strings.Contains(strings.ToUpper(down), " CASCADE") {
		t.Fatal("workflow coordination plan rollback must not cascade")
	}
}
