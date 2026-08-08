package migrations

import (
	"strings"
	"testing"
)

func TestWorkflowStandingMandateBindingMigrationContract(t *testing.T) {
	upBytes, err := Files.ReadFile("pre/0032_pursuit_workflow_standing_mandate_binding.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := string(upBytes)
	for _, required := range []string{
		"ALTER TABLE public.workflow_items",
		"ADD COLUMN mandate_id uuid",
		"FOREIGN KEY (owner_identity, mandate_id)",
		"REFERENCES public.standing_mandates (owner_identity, id)",
		"ON UPDATE RESTRICT",
		"ON DELETE RESTRICT",
		"idx_workflow_items_owner_mandate",
		"ALTER TABLE public.pursuits",
		"fk_pursuits_owner_mandate",
		"idx_pursuits_owner_mandate",
		"WHERE mandate_id IS NOT NULL",
	} {
		if !strings.Contains(up, required) {
			t.Errorf("workflow standing mandate binding migration missing %q", required)
		}
	}
	if strings.Contains(strings.ToUpper(up), " CASCADE") {
		t.Fatal("workflow standing mandate binding migration must not use CASCADE")
	}

	downBytes, err := Files.ReadFile("pre/0032_pursuit_workflow_standing_mandate_binding.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	down := string(downBytes)
	if !strings.Contains(down, "refusing to remove non-empty workflow standing mandate bindings") {
		t.Fatal("workflow standing mandate rollback must refuse to discard durable bindings")
	}
	if strings.Contains(strings.ToUpper(down), " CASCADE") {
		t.Fatal("workflow standing mandate rollback must not use CASCADE")
	}
}
