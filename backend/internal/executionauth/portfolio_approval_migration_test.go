package executionauth

import (
	"strings"
	"testing"

	"automation-hub-backend/migrations"
	"github.com/google/uuid"
)

func TestPortfolioApprovalMigrationPreservesExactOwnerScopedProvenance(t *testing.T) {
	up, err := migrations.Files.ReadFile("pre/0041_execution_authorization_portfolio_approval.up.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(up)
	for _, required := range []string{
		"portfolio_proposal_decision_id uuid",
		"FOREIGN KEY (owner_identity, portfolio_proposal_decision_id)",
		"'portfolio-decision:' || portfolio_proposal_decision_id::text",
		"evidence_json #>> '{approval,decisionId}'",
		"idx_execution_authorization_receipts_portfolio_decision",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration is missing %q", required)
		}
	}
}

func TestPortfolioApprovalDownMigrationRefusesToEraseReferencedProvenance(t *testing.T) {
	down, err := migrations.Files.ReadFile("pre/0041_execution_authorization_portfolio_approval.down.sql")
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}
	sql := string(down)
	for _, required := range []string{
		"cannot remove portfolio approval provenance",
		"approval_source_id LIKE 'portfolio-decision:%'",
		"DROP COLUMN portfolio_proposal_decision_id",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("down migration is missing %q", required)
		}
	}
}

func TestReceiptReferencesAcceptExactPortfolioDecisionSource(t *testing.T) {
	decisionID := uuid.New()
	references, err := receiptReferencesFromEvidence(DecisionEvidence{
		Approval: ApprovalEvidence{
			SourceID:   "portfolio-decision:" + decisionID.String(),
			DecisionID: decisionID.String(),
		},
	})
	if err != nil {
		t.Fatalf("resolve portfolio approval reference: %v", err)
	}
	if got := referenceString(references.portfolioProposalDecisionID); got != decisionID.String() {
		t.Fatalf("portfolio decision id = %q, want %q", got, decisionID)
	}
	if references.taskReviewDecisionID != nil || references.workflowDecisionID != nil {
		t.Fatal("portfolio approval populated another polymorphic approval column")
	}
}

func TestReceiptReferencesRejectMismatchedPortfolioDecisionSource(t *testing.T) {
	_, err := receiptReferencesFromEvidence(DecisionEvidence{
		Approval: ApprovalEvidence{
			SourceID:   "portfolio-decision:" + uuid.NewString(),
			DecisionID: uuid.NewString(),
		},
	})
	if err == nil || !strings.Contains(err.Error(), "does not match its decision") {
		t.Fatalf("mismatched portfolio source error = %v", err)
	}
}
