package migrations

import (
	"strings"
	"testing"
)

func TestPursuitPortfolioCoordinationPlanBindingMigrationContract(t *testing.T) {
	upBytes, err := Files.ReadFile("pre/0054_pursuit_portfolio_coordination_plan_binding.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := string(upBytes)
	for _, required := range []string{
		"ALTER TABLE public.pursuit_portfolio_allocations",
		"ADD COLUMN coordination_plan_id uuid",
		"coordination_plan_revision bigint NOT NULL DEFAULT 0",
		"coordination_plan_digest character(64) NOT NULL DEFAULT ''",
		"coordination_plan_node_id character varying(160) NOT NULL DEFAULT ''",
		"coordination_plan_id IS NULL",
		"coordination_plan_revision = 0",
		"coordination_plan_id IS NOT NULL",
		"coordination_plan_revision > 0",
		"coordination_plan_digest ~ '^[0-9a-f]{64}$'",
		"length(btrim(coordination_plan_node_id)) > 0",
		"FOREIGN KEY (owner_identity, coordination_plan_id, coordination_plan_revision, coordination_plan_digest)",
		"REFERENCES public.plan_graph_revisions (owner_identity, plan_id, revision, digest)",
		"ON UPDATE RESTRICT",
		"ON DELETE RESTRICT",
		"WHERE coordination_plan_id IS NOT NULL",
		"CREATE OR REPLACE FUNCTION public.hai_validate_pursuit_portfolio_coordination_plan_insert()",
		"status = 'accepted'",
		"pursuit portfolio coordination plan must reference an exact accepted revision",
		"BEFORE INSERT ON public.pursuit_portfolio_allocations",
		"grants no execution authority",
	} {
		if !strings.Contains(up, required) {
			t.Errorf("pursuit portfolio coordination plan binding migration missing %q", required)
		}
	}
	if strings.Contains(strings.ToUpper(up), " CASCADE") {
		t.Fatal("pursuit portfolio coordination plan binding must not cascade immutable evidence")
	}

	downBytes, err := Files.ReadFile("pre/0054_pursuit_portfolio_coordination_plan_binding.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	down := string(downBytes)
	if !strings.Contains(down, "refusing to remove non-empty pursuit portfolio coordination plan bindings") {
		t.Fatal("rollback must refuse to discard durable pursuit portfolio plan provenance")
	}
	if !strings.Contains(down, "WHERE coordination_plan_id IS NOT NULL") {
		t.Fatal("rollback guard must detect bound allocations")
	}
	for _, required := range []string{
		"DROP TRIGGER IF EXISTS pursuit_portfolio_coordination_plan_validate_insert",
		"DROP FUNCTION IF EXISTS public.hai_validate_pursuit_portfolio_coordination_plan_insert()",
	} {
		if !strings.Contains(down, required) {
			t.Errorf("pursuit portfolio coordination plan rollback missing %q", required)
		}
	}
	if strings.Contains(strings.ToUpper(down), " CASCADE") {
		t.Fatal("pursuit portfolio coordination plan rollback must not cascade")
	}
}
