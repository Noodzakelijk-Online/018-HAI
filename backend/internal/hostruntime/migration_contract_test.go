package hostruntime

import (
	"strings"
	"testing"

	"automation-hub-backend/migrations"
)

func TestHostRuntimeJobMigrationCreatesLeasedJobLedger(t *testing.T) {
	up, err := migrations.Files.ReadFile("pre/0065_host_runtime_jobs.up.sql")
	if err != nil {
		t.Fatalf("read up migration: %v", err)
	}
	down, err := migrations.Files.ReadFile("pre/0065_host_runtime_jobs.down.sql")
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}
	for _, fragment := range []string{"CREATE TABLE IF NOT EXISTS host_runtime_jobs", "lease_digest", "lease_expires", "task_id", "reconciled_at", "idx_host_runtime_jobs_completed_unreconciled", "chk_host_runtime_jobs_status"} {
		if !strings.Contains(string(up), fragment) {
			t.Fatalf("up migration missing %q", fragment)
		}
	}
	if !strings.Contains(string(down), "DROP TABLE IF EXISTS host_runtime_jobs") {
		t.Fatal("down migration does not remove host runtime jobs")
	}
}
