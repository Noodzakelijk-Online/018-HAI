package migrations

import (
	"strings"
	"testing"
)

func TestPursuitGoalContractMigrationContract(t *testing.T) {
	upBytes, err := Files.ReadFile("pre/0033_pursuit_goal_contract.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := string(upBytes)
	for _, required := range []string{
		"ADD COLUMN success_criteria jsonb",
		"ADD COLUMN stop_conditions jsonb",
		"ADD COLUMN dependencies jsonb",
		"ADD COLUMN resource_limits jsonb",
		"ADD COLUMN target_at timestamptz",
		"ADD COLUMN review_cadence_days integer",
		"jsonb_typeof(success_criteria) = 'array'",
		"idx_pursuits_target_at",
	} {
		if !strings.Contains(up, required) {
			t.Errorf("pursuit goal contract migration missing %q", required)
		}
	}
	if strings.Contains(strings.ToUpper(up), " CASCADE") {
		t.Fatal("pursuit goal contract migration must not use CASCADE")
	}

	downBytes, err := Files.ReadFile("pre/0033_pursuit_goal_contract.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	down := string(downBytes)
	if !strings.Contains(down, "refusing to remove non-empty pursuit goal contracts") {
		t.Fatal("pursuit goal contract rollback must refuse to discard durable contracts")
	}
	if strings.Contains(strings.ToUpper(down), " CASCADE") {
		t.Fatal("pursuit goal contract rollback must not use CASCADE")
	}
}
