package migrations

import (
	"strings"
	"testing"
)

func TestExecutionAuthorizationLifeDomainMigrationPreservesProvenance(t *testing.T) {
	t.Parallel()

	upBytes, err := Files.ReadFile("pre/0025_execution_authorization_life_domain.up.sql")
	if err != nil {
		t.Fatalf("read execution authorization life-domain migration: %v", err)
	}
	downBytes, err := Files.ReadFile("pre/0025_execution_authorization_life_domain.down.sql")
	if err != nil {
		t.Fatalf("read execution authorization life-domain rollback: %v", err)
	}
	up := strings.ToLower(string(upBytes))
	down := strings.ToLower(string(downBytes))

	for _, fragment := range []string{
		"add column if not exists domain",
		"character varying(64) not null default ''",
		"chk_execution_authorization_receipt_domain_length",
		"check (char_length(domain) <= 64)",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("0025 up migration missing %q", fragment)
		}
	}
	if !strings.Contains(down, "refusing to discard execution authorization life domains") ||
		!strings.Contains(down, "where domain <> ''") {
		t.Fatal("0025 rollback must fail closed when immutable domain provenance exists")
	}
	if strings.Contains(up, "delete from") || strings.Contains(up, "truncate") {
		t.Fatal("0025 migration must not rewrite or remove authorization history")
	}
}
