package migrations

import (
	"strings"
	"testing"
)

func TestExecutionAuthorizationCompatibilityMigrationPreservesHistory(t *testing.T) {
	t.Parallel()

	upBytes, err := Files.ReadFile("pre/0024_execution_authorization_schema_compatibility.up.sql")
	if err != nil {
		t.Fatalf("read execution authorization compatibility migration: %v", err)
	}
	downBytes, err := Files.ReadFile("pre/0024_execution_authorization_schema_compatibility.down.sql")
	if err != nil {
		t.Fatalf("read execution authorization compatibility rollback: %v", err)
	}
	up := strings.ToLower(string(upBytes))
	down := strings.ToLower(string(downBytes))

	for _, fragment := range []string{
		"uq_task_review_decision_owner_id_source",
		"uq_workflow_item_owner_id",
		"add column if not exists owner_identity",
		"uq_workflow_decision_owner_id",
		"fk_workflow_decision_owner_workflow",
		"hai_bind_workflow_decision_owner()",
		"add column if not exists project_key",
		"add column if not exists runtime_id",
		"add column if not exists approval_source_id",
		"add column if not exists effect_digest",
		"add column if not exists task_review_decision_id",
		"add column if not exists workflow_decision_id",
		"effect_digest = coalesce(effect_digest, request_digest)",
		"drop constraint if exists chk_execution_authorization_receipt_effect_digest",
		"evidence_json = jsonb_set",
		"uq_execution_authorization_receipt_owner_id_effect",
		"fk_execution_authorization_receipt_task_review",
		"fk_execution_authorization_receipt_workflow_decision",
		"uq_execution_authorization_consumption_receipt_digest",
		"create table if not exists public.execution_authorization_final_effect_exercises",
		"hai_reject_execution_authorization_mutation()",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("0024 up migration missing %q", fragment)
		}
	}
	if strings.Contains(up, "truncate public.execution_authorization") ||
		strings.Contains(up, "delete from public.execution_authorization") ||
		strings.Contains(up, " cascade") || strings.Contains(down, " cascade") {
		t.Fatal("0024 must preserve historical authorization records")
	}
	if !strings.Contains(down, "compatibility schema remains in place") {
		t.Fatal("0024 rollback must explain why the current 0014 schema is retained")
	}
}
