package migrations

import (
	"strings"
	"testing"
)

func TestPursuitResourceReservationReconciliationMigrationContract(t *testing.T) {
	upBytes, err := Files.ReadFile("pre/0036_pursuit_resource_reservation_reconciliation.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := string(upBytes)
	for _, required := range []string{
		"ADD COLUMN IF NOT EXISTS reason TEXT NOT NULL",
		"pursuit_resource_settlement_reason_check",
		"disposition <> 'released' OR length(btrim(reason)) >= 12",
		"Immutable operator or engine reason",
	} {
		if !strings.Contains(up, required) {
			t.Errorf("reconciliation migration missing %q", required)
		}
	}

	downBytes, err := Files.ReadFile("pre/0036_pursuit_resource_reservation_reconciliation.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	down := string(downBytes)
	if !strings.Contains(down, "refusing to discard pursuit resource reservation reconciliation reasons") {
		t.Fatal("rollback must refuse to discard immutable settlement reasons")
	}
	if strings.Contains(strings.ToUpper(up+down), " CASCADE") {
		t.Fatal("reconciliation migration must not use CASCADE")
	}
}
