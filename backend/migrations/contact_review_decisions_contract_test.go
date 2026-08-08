package migrations

import (
	"strings"
	"testing"
)

func TestContactReviewDecisionMigrationPreservesImmutableHumanAuthorityBoundary(t *testing.T) {
	upBytes, err := Files.ReadFile("pre/0027_contact_review_decisions.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := string(upBytes)
	for _, required := range []string{
		"life_ontology_contact_review_decisions",
		"UNIQUE (owner_identity, idempotency_key)",
		"UNIQUE (owner_identity, subject_kind, subject_id)",
		"FOREIGN KEY (owner_identity, candidate_left_id)",
		"FOREIGN KEY (owner_identity, candidate_right_id)",
		"FOREIGN KEY (owner_identity, merge_proposal_id)",
		"hai_validate_contact_review_decision",
		"contact review candidates do not match merge proposal",
		"human-approved local person",
		"life-contact-review.v1",
		"payload #>> '{localOnly}' = 'true'",
		"payload #>> '{canExecute}' = 'false'",
		"payload #>> '{grantsAuthority}' = 'false'",
		"BEFORE UPDATE OR DELETE",
		"BEFORE TRUNCATE",
	} {
		if !strings.Contains(up, required) {
			t.Errorf("contact review migration missing %q", required)
		}
	}
	if strings.Contains(strings.ToUpper(up), " CASCADE") {
		t.Fatal("contact review migration must not use CASCADE")
	}
	if strings.Contains(up, "IF NOT EXISTS") || strings.Contains(up, "OR REPLACE") {
		t.Fatal("versioned contact review migration must fail on schema drift")
	}
	downBytes, err := Files.ReadFile("pre/0027_contact_review_decisions.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(downBytes), "refusing to remove non-empty immutable contact review ledger") {
		t.Fatal("contact review rollback must refuse to discard immutable evidence")
	}
}
