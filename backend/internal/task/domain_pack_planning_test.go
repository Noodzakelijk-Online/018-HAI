package task

import (
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/domainpack"
)

func TestDomainPackPlanningPersistsOwnerScopedAdvisoryDecision(t *testing.T) {
	registry, err := domainpack.NewBuiltinRegistry()
	if err != nil {
		t.Fatalf("NewBuiltinRegistry: %v", err)
	}
	preferences := domainpack.NewMemoryPreferenceRepository(func() time.Time {
		return time.Date(2026, time.July, 31, 10, 0, 0, 0, time.UTC)
	})
	enabled := true
	preference, err := preferences.Upsert(domainpack.PackPreference{
		OwnerIdentity:       "owner-alice",
		PackID:              domainpack.PackFinancial,
		Enabled:             &enabled,
		ClassificationBoost: 12,
		ForceLocalOnly:      true,
		Adaptation: domainpack.PackAdaptation{
			Notes:                       "Owner wants deterministic cash-flow review.",
			AdditionalAgentCapabilities: []string{"owner-scenario-review"},
		},
	})
	if err != nil {
		t.Fatalf("Upsert preference: %v", err)
	}

	configured, err := WithDomainPackPlanning(
		NewService(&fakeMemoryService{}, newTaskTestLLMService(t)),
		registry,
		preferences,
	)
	if err != nil {
		t.Fatalf("WithDomainPackPlanning: %v", err)
	}
	plan, err := configured.Plan(IntakeRequest{
		OwnerIdentity: "owner-alice",
		ProjectKey:    "personal-finance",
		Request:       "Review the bank account, apply cash-flow forecasting, and prepare to pay invoice 104 without moving money yet.",
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	decision := plan.DomainPackDecision
	if decision == nil {
		t.Fatal("expected persisted domain-pack decision")
	}
	if !decision.AdvisoryOnly || decision.ExecutionAuthorityGranted {
		t.Fatalf("authority boundary = advisory %t / granted %t", decision.AdvisoryOnly, decision.ExecutionAuthorityGranted)
	}
	if decision.AuthorityBoundary != domainPackAuthorityBoundary {
		t.Fatalf("authority boundary = %q", decision.AuthorityBoundary)
	}
	if !strings.HasPrefix(decision.Digest, "sha256:") || !strings.HasPrefix(decision.RequestDigest, "sha256:") {
		t.Fatalf("decision digests = %q / %q", decision.Digest, decision.RequestDigest)
	}
	classified, found := domainClassification(decision, domainpack.PackFinancial)
	if !found {
		t.Fatalf("financial pack not classified: %#v", decision.Classified)
	}
	if classified.PlaybookDigest == "" || classified.PlaybookProvenanceDigest == "" || classified.GuidanceDigest == "" {
		t.Fatalf("pack digest bindings incomplete: %#v", classified)
	}
	if !classified.LocalOnly || !decision.LocalOnly || !plan.ExecutionPlan.DomainPackLocalOnly {
		t.Fatalf("local-only guidance was not retained: %#v / %#v", classified, plan.ExecutionPlan)
	}
	if len(decision.Preferences) != 1 || decision.Preferences[0].Revision != preference.Revision || decision.Preferences[0].Digest == "" {
		t.Fatalf("preference binding = %#v, want revision %d", decision.Preferences, preference.Revision)
	}
	if !containsDomainString(decision.AgentCapabilities, "owner-scenario-review") ||
		!containsDomainString(plan.ExecutionPlan.AdvisoryAgentCapabilities, "owner-scenario-review") {
		t.Fatalf("owner capability adaptation was not applied: %#v", decision.AgentCapabilities)
	}
	method, found := domainMethodByName(decision, "Cash-flow forecasting")
	if !found {
		t.Fatalf("cash-flow playbook method not selected: %#v", decision.Methods)
	}
	if method.ProvenanceDigest == "" || method.Provenance.Reference == "" || method.Score <= 0 {
		t.Fatalf("method provenance binding incomplete: %#v", method)
	}
	if !decision.RequiresApproval || decision.AdvisoryRiskLevel != domainpack.RiskCritical {
		t.Fatalf("financial action guidance = approval %t / risk %q", decision.RequiresApproval, decision.AdvisoryRiskLevel)
	}
	if plan.RiskAssessment.AllowedNow || !plan.RiskAssessment.ApprovalRequired || plan.RiskAssessment.ApprovalGranted {
		t.Fatalf("domain guidance did not safely tighten task risk: %#v", plan.RiskAssessment)
	}
	if len(plan.ValidationPlan.DomainPackEvidenceRequirements) == 0 ||
		len(plan.ValidationPlan.DomainPackSuccessCriteria) == 0 ||
		len(plan.ValidationPlan.DomainPackValidators) == 0 ||
		len(plan.ValidationPlan.DomainPackMethodEvaluation) == 0 {
		t.Fatalf("domain validation guidance incomplete: %#v", plan.ValidationPlan)
	}
	if plan.ExecutionPlan.DomainPackAuthorityBoundary == "" || len(plan.ExecutionPlan.StopConditions) == 0 {
		t.Fatalf("domain execution constraints incomplete: %#v", plan.ExecutionPlan)
	}

	logs := configured.Logs()
	if len(logs) == 0 || logs[0].DomainPackDecision == nil || logs[0].DomainPackDecision.Digest != decision.Digest {
		t.Fatalf("domain decision was not persisted in task history: %#v", logs)
	}
}

func TestDomainPackPlanningIsOwnerScopedAndDisabledPreferenceSuppressesPack(t *testing.T) {
	registry, err := domainpack.NewBuiltinRegistry()
	if err != nil {
		t.Fatalf("NewBuiltinRegistry: %v", err)
	}
	preferences := domainpack.NewMemoryPreferenceRepository(time.Now)
	disabled := false
	if _, err := preferences.Upsert(domainpack.PackPreference{
		OwnerIdentity: "owner-alice",
		PackID:        domainpack.PackFinancial,
		Enabled:       &disabled,
	}); err != nil {
		t.Fatalf("disable financial pack: %v", err)
	}
	configured, err := WithDomainPackPlanning(
		NewService(&fakeMemoryService{}, newTaskTestLLMService(t)),
		registry,
		preferences,
	)
	if err != nil {
		t.Fatalf("WithDomainPackPlanning: %v", err)
	}
	request := "Review the bank account and prepare to pay invoice 104 without making the payment."
	alice, err := configured.Plan(IntakeRequest{OwnerIdentity: "owner-alice", Request: request})
	if err != nil {
		t.Fatalf("alice Plan: %v", err)
	}
	if _, found := domainClassification(alice.DomainPackDecision, domainpack.PackFinancial); found {
		t.Fatalf("disabled financial pack was selected for Alice: %#v", alice.DomainPackDecision.Classified)
	}
	if !containsSuppressedPack(alice.DomainPackDecision, domainpack.PackFinancial) {
		t.Fatalf("disabled financial pack missing from suppression evidence: %#v", alice.DomainPackDecision.Suppressed)
	}

	bob, err := configured.Plan(IntakeRequest{OwnerIdentity: "owner-bob", Request: request})
	if err != nil {
		t.Fatalf("bob Plan: %v", err)
	}
	if _, found := domainClassification(bob.DomainPackDecision, domainpack.PackFinancial); !found {
		t.Fatalf("Alice preference leaked into Bob plan: %#v", bob.DomainPackDecision)
	}
	if alice.DomainPackDecision.Digest == bob.DomainPackDecision.Digest {
		t.Fatal("owner-scoped preference state did not change the deterministic decision digest")
	}
	if alice.DomainPackDecision.ExecutionAuthorityGranted || bob.DomainPackDecision.ExecutionAuthorityGranted {
		t.Fatal("domain-pack classification granted execution authority")
	}
}

func TestDomainPackDecisionIsDeterministicAndPreferenceRevisionBound(t *testing.T) {
	registry, err := domainpack.NewBuiltinRegistry()
	if err != nil {
		t.Fatalf("NewBuiltinRegistry: %v", err)
	}
	preferences := domainpack.NewMemoryPreferenceRepository(func() time.Time {
		return time.Date(2026, time.July, 31, 11, 0, 0, 0, time.UTC)
	})
	enabled := true
	stored, err := preferences.Upsert(domainpack.PackPreference{
		OwnerIdentity: "owner-alice",
		PackID:        domainpack.PackFinancial,
		Enabled:       &enabled,
	})
	if err != nil {
		t.Fatalf("create preference: %v", err)
	}
	configured, err := WithDomainPackPlanning(
		NewService(&fakeMemoryService{}, newTaskTestLLMService(t)),
		registry,
		preferences,
	)
	if err != nil {
		t.Fatalf("WithDomainPackPlanning: %v", err)
	}
	request := IntakeRequest{OwnerIdentity: "owner-alice", Request: "Review bank account cash-flow forecasting for pay invoice planning."}
	first, err := configured.Plan(request)
	if err != nil {
		t.Fatalf("first Plan: %v", err)
	}
	second, err := configured.Plan(request)
	if err != nil {
		t.Fatalf("second Plan: %v", err)
	}
	if first.ID == second.ID {
		t.Fatal("task plan IDs should remain unique")
	}
	if first.DomainPackDecision.ID != second.DomainPackDecision.ID || first.DomainPackDecision.Digest != second.DomainPackDecision.Digest {
		t.Fatalf("equivalent domain decisions are not deterministic: %q/%q vs %q/%q",
			first.DomainPackDecision.ID, first.DomainPackDecision.Digest,
			second.DomainPackDecision.ID, second.DomainPackDecision.Digest)
	}
	stored.Revision = first.DomainPackDecision.Preferences[0].Revision
	stored.ClassificationBoost = 7
	if _, err := preferences.Upsert(stored); err != nil {
		t.Fatalf("revise preference: %v", err)
	}
	third, err := configured.Plan(request)
	if err != nil {
		t.Fatalf("third Plan: %v", err)
	}
	if third.DomainPackDecision.Digest == first.DomainPackDecision.Digest {
		t.Fatal("preference revision did not change domain decision digest")
	}
	if third.DomainPackDecision.Preferences[0].Revision != 2 {
		t.Fatalf("preference revision = %d, want 2", third.DomainPackDecision.Preferences[0].Revision)
	}
}

func TestDomainPackActionInferenceDoesNotTreatDraftAsExternalSend(t *testing.T) {
	draft := inferDomainPackActions(DomainPackPlanningRequest{
		Text: "Draft a reply email for owner review, but do not send it.",
	})
	if draft["external_send"] {
		t.Fatal("draft-only communication was misclassified as an external send")
	}
	send := inferDomainPackActions(DomainPackPlanningRequest{
		Text: "Send the approved email to the recipient.",
	})
	if !send["external_send"] {
		t.Fatal("explicit send was not classified as an external effect")
	}
}

func TestDomainPackAuthorityBoundaryFailsClosed(t *testing.T) {
	risk := applyDomainPackRisk(RiskAssessment{
		Level:      "low",
		AllowedNow: true,
	}, &DomainPackDecision{
		AdvisoryOnly:              true,
		ExecutionAuthorityGranted: true,
	})
	if risk.AllowedNow {
		t.Fatal("forged domain-pack execution authority did not fail closed")
	}
	if !containsDomainString(risk.Reasons, "invalid domain pack decision attempted to cross the advisory-only authority boundary") {
		t.Fatalf("authority failure reason missing: %#v", risk.Reasons)
	}
}

func domainClassification(decision *DomainPackDecision, packID domainpack.PackID) (DomainPackClassificationDecision, bool) {
	if decision == nil {
		return DomainPackClassificationDecision{}, false
	}
	for _, classified := range decision.Classified {
		if classified.PackID == packID {
			return classified, true
		}
	}
	return DomainPackClassificationDecision{}, false
}

func domainMethodByName(decision *DomainPackDecision, name string) (DomainPackMethodDecision, bool) {
	if decision == nil {
		return DomainPackMethodDecision{}, false
	}
	for _, method := range decision.Methods {
		if method.Name == name {
			return method, true
		}
	}
	return DomainPackMethodDecision{}, false
}

func containsSuppressedPack(decision *DomainPackDecision, packID domainpack.PackID) bool {
	if decision == nil {
		return false
	}
	for _, suppressed := range decision.Suppressed {
		if suppressed.PackID == packID {
			return true
		}
	}
	return false
}

func containsDomainString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
