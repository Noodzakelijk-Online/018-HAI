package migrations

import (
	"strings"
	"testing"
)

func TestPursuitResourceLedgerMigrationContract(t *testing.T) {
	upBytes, err := Files.ReadFile("pre/0034_pursuit_resource_ledger.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := string(upBytes)
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS pursuit_resource_events",
		"owner_identity VARCHAR(255) NOT NULL",
		"kind IN ('effort_recorded', 'spend_incurred', 'spend_refund')",
		"pursuit_resource_events_owner_idempotency_idx",
		"pursuit_resource_events_value_check",
		"pg_advisory_xact_lock",
		"refund exceeds recorded net spend",
		"pursuit resource events are append-only",
		"BEFORE TRUNCATE",
	} {
		if !strings.Contains(up, required) {
			t.Errorf("pursuit resource ledger migration missing %q", required)
		}
	}
	if strings.Contains(strings.ToUpper(up), " CASCADE") {
		t.Fatal("pursuit resource ledger migration must not use CASCADE")
	}

	downBytes, err := Files.ReadFile("pre/0034_pursuit_resource_ledger.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	down := string(downBytes)
	if !strings.Contains(down, "refusing to remove non-empty pursuit resource ledger") {
		t.Fatal("pursuit resource ledger rollback must refuse to discard accounting records")
	}
	if strings.Contains(strings.ToUpper(down), " CASCADE") {
		t.Fatal("pursuit resource ledger rollback must not use CASCADE")
	}
}
