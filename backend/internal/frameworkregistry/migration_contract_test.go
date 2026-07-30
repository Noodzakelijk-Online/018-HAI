package frameworkregistry

import (
	"strings"
	"testing"

	"automation-hub-backend/migrations"
)

func TestFrameworkRegistryMigrationDeclaresVersionedImmutableConstitutionContract(t *testing.T) {
	t.Parallel()

	upBytes, err := migrations.Files.ReadFile("pre/0003_framework_registry.up.sql")
	if err != nil {
		t.Fatalf("read framework registry up migration: %v", err)
	}
	downBytes, err := migrations.Files.ReadFile("pre/0003_framework_registry.down.sql")
	if err != nil {
		t.Fatalf("read framework registry down migration: %v", err)
	}
	up := string(upBytes)
	down := string(downBytes)

	requiredUpFragments := []string{
		"base_version integer DEFAULT 0 NOT NULL",
		"CONSTRAINT chk_robert_constitution_base_version CHECK",
		"NEW.base_version IS DISTINCT FROM OLD.base_version",
		"stale constitution activation",
		"(NEW.version > 1 AND NEW.base_version = 1)",
		"status IN ('active', 'superseded')",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_robert_constitution_active_owner",
		"CREATE TRIGGER trg_framework_selection_records_immutable",
		"CREATE TRIGGER trg_robert_constitution_versions_lifecycle",
		"CREATE CONSTRAINT TRIGGER trg_robert_constitution_active_history",
		"DEFERRABLE INITIALLY DEFERRED",
		"CREATE TRIGGER trg_robert_constitution_versions_no_truncate",
	}
	for _, fragment := range requiredUpFragments {
		if !strings.Contains(up, fragment) {
			t.Errorf("framework registry up migration is missing %q", fragment)
		}
	}

	requiredDownFragments := []string{
		"DROP TABLE IF EXISTS public.robert_constitution_versions",
		"DROP FUNCTION IF EXISTS public.hai_enforce_constitution_lifecycle",
		"DROP FUNCTION IF EXISTS public.hai_require_active_constitution_after_history",
		"DROP TABLE IF EXISTS public.framework_selection_records",
		"DROP FUNCTION IF EXISTS public.hai_reject_framework_selection_mutation",
		"DROP FUNCTION IF EXISTS public.hai_reject_framework_registry_truncate",
		"DROP TABLE IF EXISTS public.framework_preferences",
	}
	for _, fragment := range requiredDownFragments {
		if !strings.Contains(down, fragment) {
			t.Errorf("framework registry down migration is missing %q", fragment)
		}
	}
}
