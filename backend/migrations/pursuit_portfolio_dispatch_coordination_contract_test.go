package migrations

import (
	"strings"
	"testing"
)

func TestPursuitPortfolioDispatchCoordinationMigrationIsBoundedImmutableAndReversible(t *testing.T) {
	upBytes, err := Files.ReadFile("pre/0045_pursuit_portfolio_dispatch_coordination.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := string(upBytes)
	for _, fragment := range []string{
		"CREATE TABLE public.pursuit_portfolio_dispatch_runs",
		"CREATE TABLE public.pursuit_portfolio_dispatch_item_results",
		"jsonb_array_length(selected_item_ids) BETWEEN 1 AND 20",
		"DISPATCH APPROVED PORTFOLIO WORKFLOWS",
		"portfolio dispatch does not match its immutable proposal",
		"portfolio dispatch result does not match an approved decision",
		"portfolio dispatch result does not match its authorization receipt",
		"portfolio dispatch result does not match its receipt-bound workflow",
		"portfolio dispatch coordination records are append-only",
		"BEFORE TRUNCATE ON public.pursuit_portfolio_dispatch_runs",
		"BEFORE TRUNCATE ON public.pursuit_portfolio_dispatch_item_results",
	} {
		if !strings.Contains(up, fragment) {
			t.Fatalf("up migration missing %q", fragment)
		}
	}
	downBytes, err := Files.ReadFile("pre/0045_pursuit_portfolio_dispatch_coordination.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	down := string(downBytes)
	for _, fragment := range []string{
		"refusing to remove non-empty portfolio dispatch coordination ledgers",
		"DROP TABLE IF EXISTS public.pursuit_portfolio_dispatch_item_results",
		"DROP TABLE IF EXISTS public.pursuit_portfolio_dispatch_runs",
	} {
		if !strings.Contains(down, fragment) {
			t.Fatalf("down migration missing %q", fragment)
		}
	}
}
