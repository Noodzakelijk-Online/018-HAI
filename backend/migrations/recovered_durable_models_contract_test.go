package migrations

import (
	"strings"
	"testing"
)

func TestRecoveredDurableModelsMigrationContract(t *testing.T) {
	upBytes, err := Files.ReadFile("pre/0012_recovered_durable_models.up.sql")
	if err != nil {
		t.Fatalf("read recovered durable models up migration: %v", err)
	}
	downBytes, err := Files.ReadFile("pre/0012_recovered_durable_models.down.sql")
	if err != nil {
		t.Fatalf("read recovered durable models down migration: %v", err)
	}

	up := string(upBytes)
	for _, table := range []string{
		"brain_catalog_upstream_reviews",
		"brain_catalog_collection_reviews",
		"brain_catalog_repository_discovery_reviews",
		"llm_model_maintenances",
		"llm_generation_records",
		"mini_swe_patch_proposals",
	} {
		if !strings.Contains(up, "CREATE TABLE IF NOT EXISTS public."+table) {
			t.Errorf("up migration does not create %s", table)
		}
		if !strings.Contains(string(downBytes), "DROP TABLE IF EXISTS public."+table) {
			t.Errorf("down migration does not drop %s", table)
		}
	}

	for _, fragment := range []string{
		"idx_brain_catalog_upstream_reviews_entry_checked",
		"idx_brain_catalog_repository_reviews_scope_checked",
		"idx_llm_model_maintenances_model_checked",
		"idx_llm_generation_records_logged",
		"idx_mini_swe_patch_proposals_owner_created",
		"configuration_fingerprint ~ '^[0-9a-f]{64}$'",
		"jsonb_typeof(fallback_path_json::jsonb)",
		"changed_files BETWEEN 0 AND 2000",
		"hai_reject_recovered_audit_mutation",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("up migration is missing contract fragment %q", fragment)
		}
	}
}
