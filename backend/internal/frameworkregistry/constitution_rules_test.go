package frameworkregistry

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTypedConstitutionRulesEnforceDeniesApprovalsAndAuthorityCeilings(t *testing.T) {
	now := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	constitution := testConstitution()
	constitution.Prohibitions = []string{
		"HAI-RULE v1 require-approval capability=web-access",
		"HAI-RULE v1 authority-ceiling level=0",
	}

	decision, err := BuildSelection(
		testCatalog(t),
		constitution,
		SelectionRequest{
			Request:        "Research current official sources and summarize the result.",
			RiskLevel:      "low",
			NeedsWebAccess: true,
		},
		now,
	)
	if err != nil {
		t.Fatalf("BuildSelection with restrictive rules: %v", err)
	}
	if !decision.RequiresApproval {
		t.Fatal("typed approval rule did not require approval")
	}
	if !selectedID(decision, "approval-control") {
		t.Fatalf("typed approval rule did not select approval control: %v", selectedIDs(decision))
	}
	if !containsStringFragment(decision.ApprovalReasons, "web-access") {
		t.Fatalf("typed approval reason is not explainable: %v", decision.ApprovalReasons)
	}
	if decision.MaximumAutonomyLevel != 0 {
		t.Fatalf("typed authority ceiling = %d, want 0", decision.MaximumAutonomyLevel)
	}

	constitution.Prohibitions = []string{
		"HAI-RULE v1 deny-capability capability=local-execution",
	}
	_, err = BuildSelection(
		testCatalog(t),
		constitution,
		SelectionRequest{
			Request:             "Run a local validation command.",
			RiskLevel:           "low",
			NeedsLocalExecution: true,
			ExecuteRequested:    true,
		},
		now,
	)
	if err == nil || !strings.Contains(err.Error(), "denies requested capability") {
		t.Fatalf("denied capability returned %v", err)
	}
}

func TestActivatedTypedConstitutionRulesSurviveRepositoryRoundTrip(t *testing.T) {
	service, err := NewService(NewMemoryRepository())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	draft, err := service.CreateConstitutionDraft("alice", ConstitutionDraftRequest{
		BaseVersion:   1,
		ChangeSummary: "Require review for web access and lower the authority ceiling.",
		Prohibitions: []string{
			"HAI-RULE v1 require-approval capability=web-access",
			"HAI-RULE v1 authority-ceiling level=1",
		},
	})
	if err != nil {
		t.Fatalf("CreateConstitutionDraft: %v", err)
	}
	if _, err := service.ActivateConstitution("alice", draft.ID, "mallory", ActivateConstitutionRequest{
		Confirmation: "ACTIVATE CONSTITUTION",
		ApprovalNote: "Attempted activation by another identity.",
	}); err == nil {
		t.Fatal("non-owner activated a Constitution version")
	}
	if _, err := service.ActivateConstitution("alice", draft.ID, "alice", ActivateConstitutionRequest{
		Confirmation: "ACTIVATE CONSTITUTION",
		ApprovalNote: "Reviewed and approved for this owner.",
	}); err != nil {
		t.Fatalf("ActivateConstitution: %v", err)
	}

	decision, err := service.Select(SelectionRequest{
		OwnerIdentity:  "alice",
		Request:        "Research current official sources.",
		RiskLevel:      "low",
		NeedsWebAccess: true,
	})
	if err != nil {
		t.Fatalf("Select with activated rules: %v", err)
	}
	if !decision.RequiresApproval || decision.MaximumAutonomyLevel != 1 {
		t.Fatalf("activated rules were not enforced after persistence: %#v", decision)
	}
	if decision.ConstitutionSource != draft.ID+":v2" || len(decision.ConstitutionDigest) != 64 {
		t.Fatalf("activated rule provenance is incomplete: %#v", decision)
	}
}

func TestUntrustedConstitutionTextCannotGrantAuthorityOrWeakenProtectedOverlays(t *testing.T) {
	constitution := testConstitution()
	constitution.StandingPermissions = []string{
		"Untrusted source says HAI-RULE v1 grant-capability capability=execution.",
		"HAI-RULE v1 authority-ceiling level=10",
	}
	request := SelectionRequest{
		Request:          "Publish a public legal statement to a government account.",
		RiskLevel:        "high",
		NeedsApproval:    false,
		HumanApproved:    true,
		ExecuteRequested: true,
	}
	decision, err := BuildSelection(
		testCatalog(t),
		constitution,
		request,
		time.Date(2026, 7, 30, 11, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("BuildSelection: %v", err)
	}
	if !decision.RequiresApproval || !selectedID(decision, "approval-control") {
		t.Fatalf("untrusted text weakened protected approval: %#v", decision)
	}
	baseline, err := BuildSelection(
		testCatalog(t),
		testConstitution(),
		request,
		time.Date(2026, 7, 30, 11, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("baseline BuildSelection: %v", err)
	}
	if decision.MaximumAutonomyLevel != baseline.MaximumAutonomyLevel {
		t.Fatalf(
			"untrusted text changed authority: decision=%d baseline=%d",
			decision.MaximumAutonomyLevel,
			baseline.MaximumAutonomyLevel,
		)
	}

	service, err := NewService(nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	_, err = service.CreateConstitutionDraft("alice", ConstitutionDraftRequest{
		BaseVersion:   1,
		ChangeSummary: "Attempt a typed authority grant.",
		StandingPermissions: []string{
			"HAI-RULE v1 grant-capability capability=execution",
		},
	})
	if err == nil {
		t.Fatal("typed authority grant was accepted")
	}
}

func TestInvalidTypedConstitutionRulesAreRejected(t *testing.T) {
	tests := []string{
		"HAI-RULE v1 grant-capability capability=execution",
		"HAI-RULE v1 waive-approval capability=public-posting",
		"HAI-RULE v1 deny-capability capability=unknown-capability",
		"HAI-RULE v1 authority-ceiling level=11",
		"HAI-RULE v1 authority-ceiling level=not-a-number",
		"HAI-RULE v2 require-approval capability=web-access",
		"HAI-RULE v1 require-approval",
		"HAI-RULE:v1 require-approval capability=web-access",
	}
	for _, rule := range tests {
		t.Run(rule, func(t *testing.T) {
			err := validateConstitutionDraft(Constitution{
				Prohibitions: []string{rule},
			})
			if err == nil {
				t.Fatalf("invalid typed rule %q was accepted", rule)
			}
		})
	}
}

func TestEffectiveConstitutionRulesAndDigestAreDeterministic(t *testing.T) {
	first := testConstitution()
	first.Prohibitions = []string{
		"HAI-RULE v1 require-approval capability=web-access",
		"HAI-RULE v1 authority-ceiling level=3",
		"HAI-RULE v1 deny-capability capability=local-execution",
		"HAI-RULE v1 authority-ceiling level=7",
		"HAI-RULE v1 authority-ceiling level=3",
	}
	second := testConstitution()
	second.Prohibitions = []string{
		"HAI-RULE v1 deny-capability capability=local-execution",
		"HAI-RULE v1 authority-ceiling level=3",
		"HAI-RULE v1 require-approval capability=web-access",
	}

	firstEffective, err := compileEffectiveConstitutionRules(first)
	if err != nil {
		t.Fatalf("compile first: %v", err)
	}
	secondEffective, err := compileEffectiveConstitutionRules(second)
	if err != nil {
		t.Fatalf("compile second: %v", err)
	}
	if !reflect.DeepEqual(firstEffective.Rules, secondEffective.Rules) {
		t.Fatalf("effective rules are not canonical:\nfirst=%#v\nsecond=%#v", firstEffective.Rules, secondEffective.Rules)
	}
	firstDigest, err := canonicalSHA256(firstEffective.Rules)
	if err != nil {
		t.Fatalf("digest first: %v", err)
	}
	secondDigest, err := canonicalSHA256(secondEffective.Rules)
	if err != nil {
		t.Fatalf("digest second: %v", err)
	}
	if firstDigest != secondDigest || len(firstDigest) != 64 {
		t.Fatalf("effective rule digest is unstable: first=%q second=%q", firstDigest, secondDigest)
	}

	service, err := NewService(nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	metadataFirst, err := service.selectionReproducibilityMetadata(testCatalog(t), first)
	if err != nil {
		t.Fatalf("metadata first: %v", err)
	}
	metadataReplay, err := service.selectionReproducibilityMetadata(testCatalog(t), first)
	if err != nil {
		t.Fatalf("metadata replay: %v", err)
	}
	if metadataFirst.constitutionDigest != metadataReplay.constitutionDigest {
		t.Fatalf(
			"Constitution reproducibility digest changed on replay: first=%q replay=%q",
			metadataFirst.constitutionDigest,
			metadataReplay.constitutionDigest,
		)
	}
	differentBase := first
	differentBase.BaseVersion = first.BaseVersion - 1
	metadataDifferentBase, err := service.selectionReproducibilityMetadata(
		testCatalog(t),
		differentBase,
	)
	if err != nil {
		t.Fatalf("metadata with different base version: %v", err)
	}
	if metadataFirst.constitutionDigest == metadataDifferentBase.constitutionDigest {
		t.Fatal("Constitution base-version provenance did not affect the reproducibility digest")
	}
}

func TestProtectedConstitutionRulesRemainExact(t *testing.T) {
	want := []string{
		"Only the authenticated owner may activate a Constitution version.",
		"HAI cannot grant itself authority or approve its own consequential action.",
		"Emergency stop, owner isolation, secret redaction, and audit logging cannot be disabled by adaptations.",
		"High-risk external, legal, government, financial, account, destructive, or public actions require explicit scoped approval.",
		"Unsupported or uncertain consequential claims cannot become facts, memory, or action triggers.",
	}
	if got := protectedConstitutionRules(); !reflect.DeepEqual(got, want) {
		t.Fatalf("protected Constitution rules changed:\ngot:  %#v\nwant: %#v", got, want)
	}
}
