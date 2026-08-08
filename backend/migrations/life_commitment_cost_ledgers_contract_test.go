package migrations

import (
	"strings"
	"testing"
)

func TestLifeCommitmentCostLedgerMigrationIsAppendOnlyAndFailClosed(t *testing.T) {
	t.Parallel()
	upBytes, err := Files.ReadFile("pre/0026_life_commitment_cost_ledgers.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	downBytes, err := Files.ReadFile("pre/0026_life_commitment_cost_ledgers.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, down := strings.ToLower(string(upBytes)), strings.ToLower(string(downBytes))
	for _, fragment := range []string{
		"life_ledger_commitment_revisions", "life_ledger_cost_entries",
		"unique (owner_identity, idempotency_key)", "amount_minor > 0",
		"payload ->> 'owneridentity' = owner_identity",
		"payload ->> 'recorddigest' = record_digest",
		"payload ->> 'localonly' = 'true'",
		"before update or delete", "before truncate", "append-only",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("0026 up migration missing %q", fragment)
		}
	}
	if !strings.Contains(down, "refusing to discard immutable life commitment or cost history") {
		t.Fatal("0026 rollback must fail closed when immutable ledger history exists")
	}
}
