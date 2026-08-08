package migrations

import (
	"strings"
	"testing"
)

func TestLifeOntologyTimestampPrecisionMigrationIsBoundedAndReversible(t *testing.T) {
	upBytes, err := Files.ReadFile("pre/0022_life_ontology_timestamp_precision.up.sql")
	if err != nil {
		t.Fatalf("read timestamp precision up migration: %v", err)
	}
	downBytes, err := Files.ReadFile("pre/0022_life_ontology_timestamp_precision.down.sql")
	if err != nil {
		t.Fatalf("read timestamp precision down migration: %v", err)
	}
	up := strings.ToLower(string(upBytes))
	down := strings.ToLower(string(downBytes))
	if !strings.Contains(up, "<= 0.000001") {
		t.Error("timestamp comparison must permit no more than one microsecond")
	}
	for _, constraint := range []string{"chk_life_ontology_entity_payload", "chk_life_ontology_relation_payload"} {
		if !strings.Contains(up, constraint) || !strings.Contains(down, constraint) {
			t.Errorf("migration must replace and restore %s", constraint)
		}
	}
	if !strings.Contains(down, "refusing to restore exact timestamp checks") {
		t.Error("down migration must fail closed when exact checks cannot be restored")
	}
	if strings.Contains(strings.ToUpper(down), " CASCADE") {
		t.Error("down migration must not use CASCADE")
	}
}
