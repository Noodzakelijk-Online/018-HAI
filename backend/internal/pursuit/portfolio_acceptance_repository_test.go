package pursuit

import (
	"automation-hub-backend/migrations"
	"strings"
	"testing"
)

var _ pursuitPortfolioAllocationRepository = (*GormRepository)(nil)
var _ pursuitPortfolioAllocationHistoryRepository = (*GormRepository)(nil)

func TestPortfolioAllocationMigrationContract(t *testing.T) {
	upBytes, err := migrations.Files.ReadFile("pre/0038_pursuit_portfolio_allocations.up.sql")
	if err != nil {
		t.Fatalf("read portfolio allocation up migration: %v", err)
	}
	downBytes, err := migrations.Files.ReadFile("pre/0038_pursuit_portfolio_allocations.down.sql")
	if err != nil {
		t.Fatalf("read portfolio allocation down migration: %v", err)
	}
	up := string(upBytes)
	down := string(downBytes)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS pursuit_portfolio_allocations",
		"CREATE TABLE IF NOT EXISTS pursuit_portfolio_allocation_items",
		"UNIQUE (owner_identity, plan_id)",
		"UNIQUE (allocation_id, pursuit_id)",
		"request_digest CHAR(64) NOT NULL",
		"decision_digest CHAR(64) NOT NULL",
		"status IN ('accepted', 'accepted_needs_approval')",
		"approval_reasons JSONB NOT NULL",
		"reservation_id UUID NOT NULL UNIQUE",
		"portfolio allocation item does not match its owner-scoped allocation",
		"portfolio allocation item does not match its resource reservation",
		"pursuit_portfolio_allocations_reject_update",
		"pursuit_portfolio_allocation_items_reject_update",
		"BEFORE TRUNCATE ON pursuit_portfolio_allocations",
		"BEFORE TRUNCATE ON pursuit_portfolio_allocation_items",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("portfolio allocation migration missing %q", fragment)
		}
	}
	for _, fragment := range []string{
		"refusing to remove non-empty pursuit portfolio allocation audit state",
		"DROP TABLE IF EXISTS pursuit_portfolio_allocation_items",
		"DROP TABLE IF EXISTS pursuit_portfolio_allocations",
	} {
		if !strings.Contains(down, fragment) {
			t.Errorf("portfolio allocation down migration missing %q", fragment)
		}
	}
}
