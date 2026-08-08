package frameworkregistry

import (
	"reflect"
	"strings"
	"testing"
)

func TestEvaluateConstitutionExecutionPolicyUsesActiveRulesAndNeverGrantsAuthority(t *testing.T) {
	repository := NewMemoryRepository()
	service, err := NewService(repository)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	draft, err := service.CreateConstitutionDraft("alice", ConstitutionDraftRequest{
		BaseVersion:   1,
		ChangeSummary: "Restrict local execution and require review for web access.",
		Prohibitions: []string{
			"HAI-RULE v1 deny-capability capability=local-execution",
			"HAI-RULE v1 require-approval capability=web-access",
			"HAI-RULE v1 authority-ceiling level=3",
		},
	})
	if err != nil {
		t.Fatalf("CreateConstitutionDraft: %v", err)
	}
	active, err := service.ActivateConstitution(
		"alice",
		draft.ID,
		"alice",
		ActivateConstitutionRequest{
			Confirmation: "ACTIVATE CONSTITUTION",
			ApprovalNote: "Reviewed this restrictive execution policy.",
		},
	)
	if err != nil {
		t.Fatalf("ActivateConstitution: %v", err)
	}

	decision, err := service.EvaluateConstitutionExecutionPolicy(
		ConstitutionExecutionPolicyRequest{
			OwnerIdentity: " alice ",
			RequestedCapabilities: []string{
				"web-access",
				"LOCAL-EXECUTION",
				"web-access",
				"memory-read",
				"financial-action",
			},
			RequiredAuthority: 4,
		},
	)
	if err != nil {
		t.Fatalf("EvaluateConstitutionExecutionPolicy: %v", err)
	}

	if !reflect.DeepEqual(decision.RequestedCapabilities, []string{
		"financial-action",
		"local-execution",
		"memory-read",
		"web-access",
	}) {
		t.Fatalf("requested capabilities = %#v", decision.RequestedCapabilities)
	}
	if !reflect.DeepEqual(decision.AllowedCapabilities, []string{
		"financial-action",
		"memory-read",
		"web-access",
	}) {
		t.Fatalf("allowed capabilities = %#v", decision.AllowedCapabilities)
	}
	if !reflect.DeepEqual(decision.DeniedCapabilities, []string{"local-execution"}) {
		t.Fatalf("denied capabilities = %#v", decision.DeniedCapabilities)
	}
	if !reflect.DeepEqual(decision.ApprovalRequiredCapabilities, []string{
		"financial-action",
		"web-access",
	}) {
		t.Fatalf(
			"approval-required capabilities = %#v",
			decision.ApprovalRequiredCapabilities,
		)
	}
	if decision.RequiredAuthorityWithinCeiling ||
		decision.ConstitutionSatisfied ||
		decision.GrantsAuthority {
		t.Fatalf("unsafe authority result: %#v", decision)
	}
	if decision.EffectiveAuthorityCeiling != 3 ||
		decision.RequiredAuthority != 4 {
		t.Fatalf("authority result = %#v", decision)
	}
	if decision.ConstitutionID != active.ID ||
		decision.ConstitutionVersion != active.Version ||
		decision.ConstitutionSource != active.ID+":v2" ||
		len(decision.ConstitutionDigest) != 64 {
		t.Fatalf("Constitution provenance = %#v", decision)
	}
}

func TestEvaluateConstitutionExecutionPolicyIsDeterministicAndMatchesSelectionDigest(t *testing.T) {
	service, err := NewService(NewMemoryRepository())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	first, err := service.EvaluateConstitutionExecutionPolicy(
		ConstitutionExecutionPolicyRequest{
			OwnerIdentity: "alice",
			RequestedCapabilities: []string{
				"tool-execution",
				"memory-read",
			},
			RequiredAuthority: 2,
		},
	)
	if err != nil {
		t.Fatalf("first evaluation: %v", err)
	}
	second, err := service.EvaluateConstitutionExecutionPolicy(
		ConstitutionExecutionPolicyRequest{
			OwnerIdentity: "alice",
			RequestedCapabilities: []string{
				" memory-read ",
				"TOOL-EXECUTION",
				"memory-read",
			},
			RequiredAuthority: 2,
		},
	)
	if err != nil {
		t.Fatalf("second evaluation: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("equivalent policy inputs were not deterministic:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if !first.ConstitutionSatisfied ||
		!first.RequiredAuthorityWithinCeiling ||
		first.GrantsAuthority {
		t.Fatalf("unexpected built-in policy result: %#v", first)
	}

	views, err := service.List("alice")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	constitution, _, err := service.ActiveConstitution("alice")
	if err != nil {
		t.Fatalf("ActiveConstitution: %v", err)
	}
	metadata, err := service.selectionReproducibilityMetadata(views, constitution)
	if err != nil {
		t.Fatalf("selectionReproducibilityMetadata: %v", err)
	}
	if first.ConstitutionDigest != metadata.constitutionDigest {
		t.Fatalf(
			"evaluator digest %q differs from selection digest %q",
			first.ConstitutionDigest,
			metadata.constitutionDigest,
		)
	}
}

func TestEvaluateConstitutionExecutionPolicyRejectsInvalidInputs(t *testing.T) {
	service, err := NewService(nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	tests := []struct {
		name      string
		request   ConstitutionExecutionPolicyRequest
		wantError string
	}{
		{
			name: "owner required",
			request: ConstitutionExecutionPolicyRequest{
				RequestedCapabilities: []string{"memory-read"},
			},
			wantError: "owner identity is required",
		},
		{
			name: "authority below range",
			request: ConstitutionExecutionPolicyRequest{
				OwnerIdentity:         "alice",
				RequestedCapabilities: []string{"memory-read"},
				RequiredAuthority:     -1,
			},
			wantError: "between 0 and 10",
		},
		{
			name: "authority above range",
			request: ConstitutionExecutionPolicyRequest{
				OwnerIdentity:         "alice",
				RequestedCapabilities: []string{"memory-read"},
				RequiredAuthority:     11,
			},
			wantError: "between 0 and 10",
		},
		{
			name: "unknown capability",
			request: ConstitutionExecutionPolicyRequest{
				OwnerIdentity:         "alice",
				RequestedCapabilities: []string{"z-unknown", "authority-grant"},
				RequiredAuthority:     1,
			},
			wantError: `unknown Constitution capability "authority-grant"`,
		},
		{
			name: "blank capability",
			request: ConstitutionExecutionPolicyRequest{
				OwnerIdentity:         "alice",
				RequestedCapabilities: []string{" "},
				RequiredAuthority:     1,
			},
			wantError: `unknown Constitution capability ""`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.EvaluateConstitutionExecutionPolicy(test.request)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("error = %v, want substring %q", err, test.wantError)
			}
		})
	}
}

func TestEvaluateConstitutionExecutionPolicyReportsProtectedApprovals(t *testing.T) {
	service, err := NewService(nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	decision, err := service.EvaluateConstitutionExecutionPolicy(
		ConstitutionExecutionPolicyRequest{
			OwnerIdentity: "alice",
			RequestedCapabilities: []string{
				"public-posting",
				"legal-government-action",
				"document-read",
			},
			RequiredAuthority: 0,
		},
	)
	if err != nil {
		t.Fatalf("EvaluateConstitutionExecutionPolicy: %v", err)
	}
	if !reflect.DeepEqual(decision.ApprovalRequiredCapabilities, []string{
		"legal-government-action",
		"public-posting",
	}) {
		t.Fatalf("protected approvals = %#v", decision.ApprovalRequiredCapabilities)
	}
	if !decision.ConstitutionSatisfied || decision.GrantsAuthority {
		t.Fatalf("protected policy classification granted authority: %#v", decision)
	}
}
