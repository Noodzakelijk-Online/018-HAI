package migrations

import (
	"strings"
	"testing"
)

func TestOperationalLifeGraphMigrationExpandsAndSafelyNarrowsOntology(t *testing.T) {
	upBytes, err := Files.ReadFile("pre/0021_operational_life_graph.up.sql")
	if err != nil {
		t.Fatalf("read operational life graph up migration: %v", err)
	}
	downBytes, err := Files.ReadFile("pre/0021_operational_life_graph.down.sql")
	if err != nil {
		t.Fatalf("read operational life graph down migration: %v", err)
	}

	up := strings.ToLower(string(upBytes))
	down := strings.ToLower(string(downBytes))
	for _, entityType := range []string{
		"source", "document", "pursuit", "workflow", "task", "memory",
		"commitment", "cost", "outcome",
	} {
		if !strings.Contains(up, "'"+entityType+"'") {
			t.Errorf("up migration is missing operational entity type %q", entityType)
		}
	}
	for _, relationType := range []string{
		"derived_from", "documents", "produces", "fulfills", "assigned_to",
		"requires", "incurs_cost", "belongs_to_pursuit", "belongs_to_workflow",
	} {
		if !strings.Contains(up, "'"+relationType+"'") {
			t.Errorf("up migration is missing operational relation type %q", relationType)
		}
	}
	if !strings.Contains(down, "refusing to narrow life ontology constraints") {
		t.Error("down migration must fail closed when operational records exist")
	}
	if strings.Contains(strings.ToUpper(down), " CASCADE") {
		t.Error("down migration must not use CASCADE")
	}
}
