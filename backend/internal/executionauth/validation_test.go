package executionauth

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNormalizeRequestRequiresExactLowercaseEffectDigest(t *testing.T) {
	for _, value := range []string{
		"",
		strings.Repeat("A", 64),
		" " + strings.Repeat("a", 64),
		strings.Repeat("a", 63),
	} {
		request := baseRequest("invalid-effect-digest")
		request.EffectDigest = value
		if _, err := normalizeRequest(request); err == nil {
			t.Fatalf("normalizeRequest accepted effect digest %q", value)
		}
	}

	request := baseRequest("valid-effect-digest")
	normalized, err := normalizeRequest(request)
	if err != nil {
		t.Fatalf("normalizeRequest: %v", err)
	}
	if normalized.EffectDigest != request.EffectDigest {
		t.Fatalf("effect digest changed: %q", normalized.EffectDigest)
	}

	service := newTestService(
		t,
		NewMemoryRepository(),
		permissiveConstitution(),
		nil,
		nil,
	)
	if _, err := service.Authorize(context.Background(), Request{
		OwnerIdentity:     "alice",
		IdempotencyKey:    "missing-effect",
		ActorIdentity:     "hai",
		ActorKind:         ActorSystem,
		TaskID:            "task-1",
		Action:            "test.execute",
		Stage:             StageExecution,
		ResourceType:      "test",
		RequiredAuthority: 1,
		RequestedAutonomy: 8,
		Risk:              RiskLow,
		Reversible:        true,
	}); err == nil {
		t.Fatal("Authorize accepted a request without an effect digest")
	}
}

func TestNormalizeRequestBoundsLifeDomain(t *testing.T) {
	request := baseRequest("bounded-life-domain")
	request.Domain = strings.Repeat("d", 65)
	if _, err := normalizeRequest(request); err == nil {
		t.Fatal("normalizeRequest accepted a life domain over 64 characters")
	}

	request.Domain = " legal_government "
	normalized, err := normalizeRequest(request)
	if err != nil {
		t.Fatalf("normalizeRequest: %v", err)
	}
	if normalized.Domain != "legal_government" {
		t.Fatalf("normalized domain = %q", normalized.Domain)
	}
}

func TestCleanFoldersDeduplicatesAfterCanonicalization(t *testing.T) {
	values := []string{
		filepath.Join("C:", "HAI", "workspace", "."),
		filepath.Join("C:", "HAI", "workspace"),
		filepath.Join("C:", "HAI", "workspace", "nested", ".."),
	}
	got := cleanFolders(values)
	if len(got) != 1 || got[0] != filepath.Join("C:", "HAI", "workspace") {
		t.Fatalf("cleanFolders = %#v", got)
	}
}

func TestCleanFactsTruncatesAtRuneBoundaries(t *testing.T) {
	key := strings.Repeat("é", 130)
	value := strings.Repeat("界", 520)
	got := cleanFacts(map[string]string{key: value})
	if len(got) != 1 {
		t.Fatalf("cleanFacts = %#v", got)
	}
	for cleanKey, cleanValue := range got {
		if !utf8.ValidString(cleanKey) || !utf8.ValidString(cleanValue) {
			t.Fatalf("cleanFacts produced invalid UTF-8: %q=%q", cleanKey, cleanValue)
		}
		if len([]rune(cleanKey)) != 128 || len([]rune(cleanValue)) != 512 {
			t.Fatalf(
				"cleanFacts rune lengths = %d/%d",
				len([]rune(cleanKey)),
				len([]rune(cleanValue)),
			)
		}
	}
}

func TestNormalizeRequestRequiresCompleteExactGovernanceEvidence(t *testing.T) {
	digest := func(value string) string {
		return strings.Repeat(value, 64)
	}
	request := baseRequest("governance-valid")
	request.Governance = &GovernanceEvidence{
		TaskPlanID:                       "plan-1",
		TaskPlanDigest:                   digest("a"),
		FrameworkSelectionID:             "selection-1",
		FrameworkCatalogVersion:          "framework-catalog-v1",
		FrameworkCatalogDigest:           digest("b"),
		FrameworkPreferenceDigest:        digest("c"),
		FrameworkConstitutionDigest:      digest("d"),
		FrameworkOperatingContractDigest: digest("e"),
		DomainPackDecisionID:             "domain-decision-1",
		DomainPackCatalogVersion:         "domain-pack-catalog-v1",
		DomainPackCatalogDigest:          digest("f"),
		DomainPackDecisionDigest:         digest("1"),
		EvidenceReferences: []string{
			"source://record-2",
			"source://record-1",
			"source://record-1",
		},
	}
	normalized, err := normalizeRequest(request)
	if err != nil {
		t.Fatalf("normalizeRequest: %v", err)
	}
	if normalized.Governance == nil || len(normalized.Governance.EvidenceReferences) != 2 ||
		normalized.Governance.EvidenceReferences[0] != "source://record-1" {
		t.Fatalf("governance evidence references = %#v", normalized.Governance.EvidenceReferences)
	}

	for name, mutate := range map[string]func(*Request){
		"missing plan digest": func(value *Request) {
			value.Governance.TaskPlanDigest = ""
		},
		"partial framework": func(value *Request) {
			value.Governance.FrameworkOperatingContractDigest = ""
		},
		"partial domain pack": func(value *Request) {
			value.Governance.DomainPackDecisionDigest = ""
		},
		"non-exact digest": func(value *Request) {
			value.Governance.TaskPlanDigest = strings.ToUpper(value.Governance.TaskPlanDigest)
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := request
			governance := *request.Governance
			candidate.Governance = &governance
			mutate(&candidate)
			if _, err := normalizeRequest(candidate); err == nil {
				t.Fatal("normalizeRequest accepted incomplete or non-exact governance evidence")
			}
		})
	}
}

func TestNormalizeRequestAcceptsOnlyExactFrameworkPreflightDigest(t *testing.T) {
	request := baseRequest("framework-preflight-digest")
	request.Governance = &GovernanceEvidence{
		TaskPlanID: "plan-1", TaskPlanDigest: strings.Repeat("a", 64),
		FrameworkEvidencePreflightDigest: strings.Repeat("b", 64),
	}
	if _, err := normalizeRequest(request); err != nil {
		t.Fatalf("normalizeRequest rejected exact preflight digest: %v", err)
	}
	request.Governance.FrameworkEvidencePreflightDigest = strings.Repeat("B", 64)
	if _, err := normalizeRequest(request); err == nil {
		t.Fatal("normalizeRequest accepted a non-canonical preflight digest")
	}
}

func TestNormalizeRequestEnforcesSelectorV5RiskContract(t *testing.T) {
	digest := func(value string) string { return strings.Repeat(value, 64) }
	base := func() Request {
		request := baseRequest("selector-v5-risk")
		request.Risk = RiskMedium
		maximumAutonomy := request.RequestedAutonomy
		requiresApproval := false
		request.Governance = &GovernanceEvidence{
			TaskPlanID: "plan-1", TaskPlanDigest: digest("a"),
			FrameworkSelectionID: "selection-1", FrameworkCatalogVersion: "framework-catalog-v2",
			FrameworkSelectorAlgorithmVersion: "selector-v5",
			FrameworkTaskRiskLevel:            RiskMedium, FrameworkEffectiveRiskCeiling: RiskHigh,
			FrameworkMaximumAutonomyLevel: &maximumAutonomy,
			FrameworkRequiresApproval:     &requiresApproval,
			FrameworkCatalogDigest:        digest("b"), FrameworkPreferenceDigest: digest("c"),
			FrameworkConstitutionDigest: digest("d"), FrameworkOperatingContractDigest: digest("e"),
		}
		return request
	}

	valid := base()
	normalized, err := normalizeRequest(valid)
	if err != nil {
		t.Fatalf("normalizeRequest valid v5 contract: %v", err)
	}
	if normalized.Governance == nil ||
		normalized.Governance.FrameworkSelectorAlgorithmVersion != "selector-v5" ||
		normalized.Governance.FrameworkTaskRiskLevel != RiskMedium ||
		normalized.Governance.FrameworkEffectiveRiskCeiling != RiskHigh {
		t.Fatalf("normalized v5 governance = %#v", normalized.Governance)
	}

	for name, mutate := range map[string]func(*Request){
		"missing task risk": func(request *Request) {
			request.Governance.FrameworkTaskRiskLevel = ""
		},
		"missing ceiling": func(request *Request) {
			request.Governance.FrameworkEffectiveRiskCeiling = ""
		},
		"missing autonomy ceiling": func(request *Request) {
			request.Governance.FrameworkMaximumAutonomyLevel = nil
		},
		"missing approval contract": func(request *Request) {
			request.Governance.FrameworkRequiresApproval = nil
		},
		"autonomy exceeds ceiling": func(request *Request) {
			value := request.RequestedAutonomy - 1
			request.Governance.FrameworkMaximumAutonomyLevel = &value
		},
		"invalid task risk": func(request *Request) {
			request.Governance.FrameworkTaskRiskLevel = RiskCritical
		},
		"invalid ceiling": func(request *Request) {
			request.Governance.FrameworkEffectiveRiskCeiling = "severe"
		},
		"task risk exceeds ceiling": func(request *Request) {
			request.Governance.FrameworkTaskRiskLevel = RiskHigh
			request.Governance.FrameworkEffectiveRiskCeiling = RiskMedium
		},
		"execution risk exceeds ceiling": func(request *Request) {
			request.Risk = RiskHigh
			request.Governance.FrameworkEffectiveRiskCeiling = RiskMedium
		},
		"execution risk understates task risk": func(request *Request) {
			request.Risk = RiskLow
		},
		"unknown selector version": func(request *Request) {
			request.Governance.FrameworkSelectorAlgorithmVersion = "selector-v99"
		},
	} {
		t.Run(name, func(t *testing.T) {
			request := base()
			mutate(&request)
			if _, err := normalizeRequest(request); err == nil {
				t.Fatal("normalizeRequest accepted invalid selector-v5 risk governance")
			}
		})
	}
}

func TestNormalizeRequestPreservesLegacyFrameworkGovernanceWithoutRiskInference(t *testing.T) {
	digest := strings.Repeat("a", 64)
	for _, version := range []string{"", "selector-v4"} {
		t.Run(firstNonEmpty(version, "blank-version"), func(t *testing.T) {
			request := baseRequest("legacy-" + firstNonEmpty(version, "blank"))
			request.Governance = &GovernanceEvidence{
				TaskPlanID: "plan-1", TaskPlanDigest: digest,
				FrameworkSelectionID: "selection-1", FrameworkCatalogVersion: "framework-catalog-v1",
				FrameworkSelectorAlgorithmVersion: version,
				FrameworkCatalogDigest:            digest, FrameworkPreferenceDigest: digest,
				FrameworkConstitutionDigest: digest, FrameworkOperatingContractDigest: digest,
			}
			normalized, err := normalizeRequest(request)
			if err != nil {
				t.Fatalf("normalizeRequest legacy governance: %v", err)
			}
			if normalized.Governance.FrameworkTaskRiskLevel != "" ||
				normalized.Governance.FrameworkEffectiveRiskCeiling != "" ||
				normalized.Governance.FrameworkMaximumAutonomyLevel != nil ||
				normalized.Governance.FrameworkRequiresApproval != nil {
				t.Fatalf("legacy risk contract was inferred: %#v", normalized.Governance)
			}
		})
	}
}

func TestNormalizeRequestRedactsCredentialsFromEvidenceReferences(t *testing.T) {
	request := baseRequest("redacted-references")
	request.SourceReferences = []string{
		"https://user:password@example.test/source?token=secret-value&record=42",
	}
	request.Governance = &GovernanceEvidence{
		TaskPlanID:     "plan-1",
		TaskPlanDigest: strings.Repeat("a", 64),
		EvidenceReferences: []string{
			"https://operator:secret@example.test/evidence?api_key=hidden&item=7",
		},
	}

	normalized, err := normalizeRequest(request)
	if err != nil {
		t.Fatalf("normalizeRequest: %v", err)
	}
	for _, reference := range append(
		append([]string{}, normalized.SourceReferences...),
		normalized.Governance.EvidenceReferences...,
	) {
		if strings.Contains(reference, "user:") ||
			strings.Contains(reference, "operator:") ||
			strings.Contains(reference, "secret-value") ||
			strings.Contains(reference, "hidden") {
			t.Fatalf("reference retained credential material: %q", reference)
		}
		if !strings.Contains(reference, "record=42") &&
			!strings.Contains(reference, "item=7") {
			t.Fatalf("reference lost its non-sensitive identity: %q", reference)
		}
	}
}
