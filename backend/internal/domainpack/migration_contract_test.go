package domainpack

import (
	"strings"
	"testing"

	"automation-hub-backend/migrations"
)

func TestDomainPackPreferenceMigrationContract(t *testing.T) {
	up, err := migrations.Files.ReadFile("pre/0010_domain_pack_preferences.up.sql")
	if err != nil {
		t.Fatalf("read up migration: %v", err)
	}
	down, err := migrations.Files.ReadFile("pre/0010_domain_pack_preferences.down.sql")
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS public.domain_pack_preferences",
		"UNIQUE (owner_identity, pack_id)",
		"catalog_version",
		"revision bigint",
		"adaptations_json jsonb",
		"classification_boost BETWEEN -25 AND 25",
	} {
		if !strings.Contains(string(up), fragment) {
			t.Errorf("up migration missing %q", fragment)
		}
	}
	if !strings.Contains(string(down), "DROP TABLE IF EXISTS public.domain_pack_preferences") {
		t.Fatal("down migration does not drop domain pack preferences")
	}
}
