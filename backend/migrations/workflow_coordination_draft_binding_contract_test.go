package migrations

import (
	"strings"
	"testing"
)

func TestWorkflowCoordinationDraftBindingMigrationContract(t *testing.T) {
	upBytes, err := Files.ReadFile("pre/0058_workflow_coordination_draft_binding.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := string(upBytes)
	for _, required := range []string{
		"ADD COLUMN coordination_draft_plan_id uuid",
		"coordination_draft_revision = 1",
		"FOREIGN KEY (owner_identity, coordination_draft_plan_id, coordination_draft_revision, coordination_draft_digest)",
		"REFERENCES public.plan_graph_revisions (owner_identity, plan_id, revision, digest)",
		"bound_status IS DISTINCT FROM 'draft'",
		"node -> 'bindings' ->> 'workflowId' = NEW.id::text",
		"node ->> 'owner' = NEW.owner_identity",
		"DEFERRABLE INITIALLY IMMEDIATE",
		"grants no approval or execution authority",
	} {
		if !strings.Contains(up, required) {
			t.Errorf("workflow coordination draft migration missing %q", required)
		}
	}
	if strings.Contains(strings.ToUpper(up), " CASCADE") {
		t.Fatal("workflow coordination draft binding must not cascade immutable evidence")
	}

	downBytes, err := Files.ReadFile("pre/0058_workflow_coordination_draft_binding.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	down := string(downBytes)
	if !strings.Contains(down, "refusing to remove non-empty workflow coordination draft bindings") {
		t.Fatal("rollback must refuse to discard durable workflow draft provenance")
	}
	if strings.Contains(strings.ToUpper(down), " CASCADE") {
		t.Fatal("workflow coordination draft rollback must not cascade")
	}
}
