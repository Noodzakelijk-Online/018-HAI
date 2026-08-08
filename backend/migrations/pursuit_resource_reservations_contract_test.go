package migrations

import (
	"strings"
	"testing"
)

func TestPursuitResourceReservationsMigrationContract(t *testing.T) {
	upBytes, err := Files.ReadFile("pre/0035_pursuit_resource_reservations.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := string(upBytes)
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS pursuit_resource_reservations",
		"CREATE TABLE IF NOT EXISTS pursuit_resource_reservation_settlements",
		"estimated_effort_minutes BIGINT NOT NULL",
		"estimated_cost_micros BIGINT NOT NULL",
		"UNIQUE REFERENCES pursuit_resource_reservations(id) ON DELETE RESTRICT",
		"disposition IN ('consumed', 'released')",
		"pg_advisory_xact_lock",
		"recorded_effort + held_effort + NEW.estimated_effort_minutes",
		"recorded_spend_micros + held_spend_micros + NEW.estimated_cost_micros",
		"s.id IS NULL",
		"pursuit resource reservations and settlements are append-only",
		"pursuit_resource_reservations_reject_update",
		"pursuit_resource_reservations_reject_delete",
		"pursuit_resource_reservations_reject_truncate",
		"pursuit_resource_reservation_settlements_reject_update",
		"pursuit_resource_reservation_settlements_reject_delete",
		"pursuit_resource_reservation_settlements_reject_truncate",
	} {
		if !strings.Contains(up, required) {
			t.Errorf("pursuit resource reservations migration missing %q", required)
		}
	}
	if strings.Contains(strings.ToUpper(up), " CASCADE") {
		t.Fatal("pursuit resource reservations migration must not use CASCADE")
	}

	downBytes, err := Files.ReadFile("pre/0035_pursuit_resource_reservations.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	down := string(downBytes)
	if !strings.Contains(down, "refusing to remove non-empty pursuit resource reservation ledger") {
		t.Fatal("pursuit resource reservations rollback must refuse to discard reservation records")
	}
	for _, table := range []string{
		"pursuit_resource_reservation_settlements",
		"pursuit_resource_reservations",
	} {
		if !strings.Contains(down, "EXISTS (SELECT 1 FROM "+table+" LIMIT 1)") {
			t.Errorf("rollback must refuse a non-empty %s table", table)
		}
	}
	if strings.Contains(strings.ToUpper(down), " CASCADE") {
		t.Fatal("pursuit resource reservations rollback must not use CASCADE")
	}
}
