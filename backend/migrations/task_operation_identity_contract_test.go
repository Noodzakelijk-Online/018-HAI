package migrations

import (
	"strings"
	"testing"
)

func TestTaskOperationIdentityMigrationContract(t *testing.T) {
	upBytes, err := Files.ReadFile("pre/0037_task_operation_identity.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := string(upBytes)
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS public.task_operations",
		"CONSTRAINT uq_task_operations_owner_key UNIQUE (owner_identity, idempotency_key)",
		"request_digest ~ '^[0-9a-f]{64}$'",
		"mode IN ('plan', 'run')",
		"status IN ('running', 'completed', 'needs_review')",
		"task operation identity is immutable",
		"terminal task operation state is immutable",
		"task_operations_reject_delete",
		"task_operations_reject_truncate",
	} {
		if !strings.Contains(up, required) {
			t.Errorf("task operation migration missing %q", required)
		}
	}
	if strings.Contains(strings.ToUpper(up), " CASCADE") {
		t.Fatal("task operation migration must not use CASCADE")
	}

	downBytes, err := Files.ReadFile("pre/0037_task_operation_identity.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	down := string(downBytes)
	if !strings.Contains(down, "refusing to remove non-empty task operation audit state") ||
		!strings.Contains(down, "EXISTS (SELECT 1 FROM public.task_operations LIMIT 1)") {
		t.Fatal("task operation rollback must refuse to discard operation records")
	}
	if strings.Contains(strings.ToUpper(down), " CASCADE") {
		t.Fatal("task operation rollback must not use CASCADE")
	}
}
