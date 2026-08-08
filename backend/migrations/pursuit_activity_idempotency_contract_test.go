package migrations

import (
	"strings"
	"testing"
)

func TestPursuitActivityIdempotencyMigrationPreservesHistoryAndPreventsFutureDuplicates(t *testing.T) {
	upBytes, err := Files.ReadFile("pre/0043_pursuit_activity_idempotency.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	downBytes, err := Files.ReadFile("pre/0043_pursuit_activity_idempotency.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := strings.ToLower(string(upBytes))
	down := strings.ToLower(string(downBytes))
	for _, required := range []string{
		"alter table public.pursuit_activities",
		"add column idempotency_key",
		"create unique index idx_pursuit_activities_idempotency",
		"pursuit_id, idempotency_key",
		"where idempotency_key is not null",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("up migration missing %q", required)
		}
	}
	if strings.Contains(up, "delete from") || strings.Contains(up, "update public.pursuit_activities") {
		t.Fatal("idempotency migration must not rewrite or delete historical audit records")
	}
	for _, required := range []string{
		"drop index if exists public.idx_pursuit_activities_idempotency",
		"drop column if exists idempotency_key",
	} {
		if !strings.Contains(down, required) {
			t.Fatalf("down migration missing %q", required)
		}
	}
}
