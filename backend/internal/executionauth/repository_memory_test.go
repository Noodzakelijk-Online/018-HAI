package executionauth

import (
	"context"
	"strings"
	"testing"
)

func TestMemoryRepositoryReceiptGovernanceIsImmutableByCopy(t *testing.T) {
	repository := NewMemoryRepository()
	service := newTestService(t, repository, permissiveConstitution(), nil, nil)
	request := selectorV5Request("immutable-governance-copy")
	withMatchingFrameworkSelection(t, service, *request.Governance)

	receipt, err := service.Authorize(context.Background(), request)
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if receipt.Outcome != OutcomeAuthorized {
		t.Fatalf("receipt = %#v", receipt)
	}
	receipt.Evidence.Governance.EvidenceReferences = append(
		receipt.Evidence.Governance.EvidenceReferences,
		"mutated://reference",
	)
	*receipt.Evidence.Governance.FrameworkMaximumAutonomyLevel = 0
	*receipt.Evidence.Governance.FrameworkRequiresApproval = true
	receipt.Evidence.ReasonCodes = append(receipt.Evidence.ReasonCodes, "mutated")

	stored, err := service.Get(context.Background(), "alice", receipt.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if *stored.Evidence.Governance.FrameworkMaximumAutonomyLevel != request.RequestedAutonomy ||
		*stored.Evidence.Governance.FrameworkRequiresApproval ||
		containsFold(stored.Evidence.Governance.EvidenceReferences, "mutated://reference") ||
		containsFold(stored.Evidence.ReasonCodes, "mutated") {
		t.Fatalf("stored receipt was mutated through returned evidence: %#v", stored.Evidence)
	}
	if stored.DecisionDigest == "" || !strings.EqualFold(stored.DecisionDigest, receipt.DecisionDigest) {
		t.Fatalf("stored decision digest changed: stored=%q returned=%q", stored.DecisionDigest, receipt.DecisionDigest)
	}
}
