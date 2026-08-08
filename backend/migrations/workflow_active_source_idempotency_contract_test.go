package migrations

import (
	"strings"
	"testing"
)

func TestWorkflowActiveSourceIdempotencyMigrationContract(t *testing.T) {
	upBytes, err := Files.ReadFile("pre/0042_workflow_active_source_idempotency.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	downBytes, err := Files.ReadFile("pre/0042_workflow_active_source_idempotency.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := string(upBytes)
	down := string(downBytes)

	for _, fragment := range []string{
		"having count(*) > 1",
		"raise exception 'cannot enforce workflow source idempotency",
		"create unique index idx_workflow_items_active_owner_source_identity",
		"on public.workflow_items (owner_identity, source_type, source_id)",
		"where archived = false",
		"btrim(owner_identity) <> ''",
		"btrim(source_type) <> ''",
		"btrim(source_id) <> ''",
	} {
		if !strings.Contains(strings.ToLower(up), fragment) {
			t.Fatalf("0042 up migration missing %q", fragment)
		}
	}
	if !strings.Contains(strings.ToLower(down), "drop index if exists public.idx_workflow_items_active_owner_source_identity") {
		t.Fatal("0042 down migration does not remove the exact idempotency index")
	}
}
