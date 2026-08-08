package migrations

import (
	"strings"
	"testing"
)

func TestApprovalProofConsumptionMigrationIsOwnerScopedAtomicAndImmutable(t *testing.T) {
	upBytes, err := Files.ReadFile("pre/0028_automation_approval_proof_consumptions.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := string(upBytes)
	for _, required := range []string{
		"automation_approval_proof_consumptions",
		"PRIMARY KEY (owner_identity, proof_id)",
		"automation-approval-proof-consumption.v1",
		"automation.agent-runtime.execute",
		"nonce_digest character(64)",
		"signature_digest character(64)",
		"record_digest character(64)",
		"BEFORE UPDATE OR DELETE",
		"BEFORE TRUNCATE",
		"hai_reject_execution_authorization_mutation",
	} {
		if !strings.Contains(up, required) {
			t.Errorf("approval proof consumption migration missing %q", required)
		}
	}
	if strings.Contains(strings.ToUpper(up), " CASCADE") {
		t.Fatal("approval proof consumption migration must not use CASCADE")
	}
	if strings.Contains(up, "IF NOT EXISTS") || strings.Contains(up, "OR REPLACE") {
		t.Fatal("versioned approval proof migration must fail on schema drift")
	}
	downBytes, err := Files.ReadFile("pre/0028_automation_approval_proof_consumptions.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(downBytes), "refusing to remove non-empty immutable approval proof consumption ledger") {
		t.Fatal("approval proof rollback must refuse to discard replay evidence")
	}
}
