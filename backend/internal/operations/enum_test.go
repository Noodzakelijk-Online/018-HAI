package operations

import "testing"

func TestEnumsValidateAndParse(t *testing.T) {
	// Valid round-trips.
	if s, err := ParseOperationStatus("AWAITING_APPROVAL"); err != nil || s != StatusAwaitingApproval {
		t.Fatalf("status parse = %v %v", s, err)
	}
	if r, err := ParseRiskLevel(" High "); err != nil || r != RiskHigh {
		t.Fatalf("risk parse = %v %v", r, err)
	}
	if a, err := ParseAutonomyLevel("automatic"); err != nil || a != AutonomyAuto {
		t.Fatalf("autonomy parse = %v %v", a, err)
	}
	if o, err := ParseOwnerType("robert"); err != nil || o != OwnerRobert {
		t.Fatalf("owner parse = %v %v", o, err)
	}
	if d, err := ParseCurrentDecision("run_safe_local_worker"); err != nil || d != DecisionRunSafeLocalWorker {
		t.Fatalf("decision parse = %v %v", d, err)
	}
	if v, err := ParseVerificationStatus("not_required"); err != nil || v != VerificationNotRequired {
		t.Fatalf("verification parse = %v %v", v, err)
	}
}

func TestEnumsRejectInvalid(t *testing.T) {
	if _, err := ParseOperationStatus("nope"); err == nil {
		t.Fatalf("invalid status should error")
	}
	if _, err := ParseRiskLevel("critical"); err == nil {
		t.Fatalf("unknown risk should error")
	}
	if _, err := ParseCurrentDecision(""); err == nil {
		t.Fatalf("empty decision should error")
	}
	if RiskLevel("").IsValid() {
		t.Fatalf("empty risk must be invalid")
	}
}

func TestDecisionApprovalAndAutoSemantics(t *testing.T) {
	// External-action decisions require approval.
	for _, d := range []CurrentDecision{DecisionRunRuntimeAfterApprv, DecisionExecuteAPIAfterApprv, DecisionStageBrowserAction, DecisionAskRobert} {
		if !d.RequiresApproval() {
			t.Fatalf("%s should require approval", d)
		}
	}
	// Only safe local worker and internal model pipeline auto-execute in 2A.
	if !DecisionRunSafeLocalWorker.IsAutoExecutable() || !DecisionRunModelPipeline.IsAutoExecutable() {
		t.Fatalf("safe worker / model pipeline should be auto-executable")
	}
	if DecisionExecuteAPIAfterApprv.IsAutoExecutable() {
		t.Fatalf("external API execution must not be auto-executable")
	}
	if DecisionBlock.RequiresApproval() || DecisionBlock.IsAutoExecutable() {
		t.Fatalf("block is neither approvable nor auto-executable")
	}
}
