package domainpack

import (
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestBuiltinCatalogCompletenessAndUniqueness(t *testing.T) {
	t.Parallel()
	packs := BuiltinPacks()
	expected := []PackID{
		PackLegalGovernment, PackEmergencyContinuity, PackHealthWellbeing, PackFinancial,
		PackWorkVenture, PackHomeAssets, PackRelationshipsCare, PackLearningGrowth,
		PackTravelMobility, PackPersonalProductivity, PackIdentityRoles, PackFamilyHousehold, PackFoodNutrition,
		PackCommunication, PackDigitalAccounts, PackPossessionsInventory, PackAnimalsDependants,
		PackCommunityCivic, PackLeisure, PackCreativity, PackMeaningValues,
		PackEnvironmentSustainability, PackLegacyLongTerm, PackSafetySecurity,
	}
	if len(packs) != len(expected) {
		t.Fatalf("pack count = %d, want %d", len(packs), len(expected))
	}

	seen := map[PackID]bool{}
	for _, pack := range packs {
		if seen[pack.ID] {
			t.Fatalf("duplicate pack %q", pack.ID)
		}
		seen[pack.ID] = true
		if err := ValidatePack(pack); err != nil {
			t.Fatalf("ValidatePack(%s): %v", pack.ID, err)
		}
		for field, length := range map[string]int{
			"signals": len(pack.ClassificationSignals), "questions": len(pack.IntakeQuestions),
			"entities": len(pack.CommonEntities), "risk": len(pack.RiskTriggers),
			"approval": len(pack.ApprovalRules), "prohibited": len(pack.ProhibitedAutonomousActions),
			"authority": len(pack.SourceAuthorityRules), "evidence": len(pack.EvidenceRequirements),
			"validators": len(pack.DeterministicValidators), "success": len(pack.SuccessCriteriaTemplates),
			"stops": len(pack.StopEscalationConditions), "capabilities": len(pack.SuitableAgentCapabilities),
			"audit": len(pack.AuditEvents),
		} {
			if length == 0 {
				t.Fatalf("%s has empty %s", pack.ID, field)
			}
		}
	}
	for _, id := range expected {
		if !seen[id] {
			t.Errorf("missing pack %q", id)
		}
	}
}

func TestBuiltinCatalogMatchesCanonicalWholeLifeDomainIDs(t *testing.T) {
	t.Parallel()
	expected := map[PackID]struct{}{
		"legal_government": {}, "emergency_continuity": {}, "health_wellbeing": {},
		"financial": {}, "work_venture": {}, "home_assets": {}, "relationships_care": {},
		"learning_growth": {}, "travel_mobility": {}, "personal_productivity": {},
		"identity_roles": {}, "family_household": {}, "food_nutrition": {},
		"communication_correspondence": {}, "digital_accounts": {},
		"possessions_inventory": {}, "animals_dependants": {}, "community_civic": {},
		"leisure_recreation": {}, "creativity_expression": {}, "meaning_values": {},
		"environment_sustainability": {}, "legacy_long_term": {}, "safety_security": {},
	}
	packs := BuiltinPacks()
	if len(packs) != len(expected) {
		t.Fatalf("pack count = %d, want canonical %d", len(packs), len(expected))
	}
	for _, pack := range packs {
		if _, exists := expected[pack.ID]; !exists {
			t.Fatalf("non-canonical or duplicate domain pack id %q", pack.ID)
		}
		delete(expected, pack.ID)
	}
	if len(expected) != 0 {
		t.Fatalf("missing canonical pack ids: %#v", expected)
	}
}

func TestEveryPackRequiresApprovalForConsequentialActions(t *testing.T) {
	t.Parallel()
	for _, pack := range BuiltinPacks() {
		rules := map[string]bool{}
		for _, rule := range pack.ApprovalRules {
			rules[rule.Action] = rule.Required
		}
		for _, action := range []string{
			"paid_model_usage", "financial_transaction", "legal_or_government_action",
			"medical_action", "public_post",
		} {
			if !rules[action] {
				t.Errorf("%s does not require approval for %s", pack.ID, action)
			}
		}
	}
}

func TestClassificationSupportsMultipleExplainablePacks(t *testing.T) {
	t.Parallel()
	registry := mustBuiltinRegistry(t)
	result, err := registry.Classify(ClassificationRequest{
		Text: "Draft email to my lawyer about the legal filing and pay invoice after review.",
	}, nil)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	for _, id := range []PackID{PackCommunication, PackLegalGovernment, PackFinancial} {
		match, ok := findMatch(result.Matches, id)
		if !ok {
			t.Fatalf("expected %s in %#v", id, result.Matches)
		}
		if match.Score < 70 || len(match.Signals) == 0 || len(match.Reasons) == 0 {
			t.Fatalf("%s match is not explainable: %#v", id, match)
		}
	}
}

func TestWeakSignalsDoNotInferSensitiveDomains(t *testing.T) {
	t.Parallel()
	registry := mustBuiltinRegistry(t)
	result, err := registry.Classify(ClassificationRequest{
		Text: "This case mentions money, stress, family, a login, a dog, diet, and legacy.",
	}, nil)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	sensitive := []PackID{
		PackLegalGovernment, PackHealthWellbeing, PackFinancial, PackFamilyHousehold,
		PackDigitalAccounts, PackAnimalsDependants, PackFoodNutrition, PackLegacyLongTerm,
	}
	for _, id := range sensitive {
		if _, ok := findMatch(result.Matches, id); ok {
			t.Errorf("weak evidence inferred sensitive pack %s", id)
		}
		suppressed, ok := findSuppressed(result.Suppressed, id)
		if !ok || !strings.Contains(suppressed.Reason, "strong unambiguous signal") {
			t.Errorf("missing explainable suppression for %s: %#v", id, result.Suppressed)
		}
	}
}

func TestExplicitSensitiveClassificationIsAllowedAndMarked(t *testing.T) {
	t.Parallel()
	registry := mustBuiltinRegistry(t)
	result, err := registry.Classify(ClassificationRequest{
		Text:            "Help me organize this matter.",
		ExplicitPackIDs: []PackID{PackHealthWellbeing},
	}, nil)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	match, ok := findMatch(result.Matches, PackHealthWellbeing)
	if !ok || !match.Explicit || !match.Sensitive || match.Score < 100 {
		t.Fatalf("explicit health match = %#v", match)
	}
}

func TestPreferenceRepositoryIsOwnerScopedAndDeterministic(t *testing.T) {
	t.Parallel()
	fixed := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	repository := NewMemoryPreferenceRepository(func() time.Time { return fixed })
	disabled := false
	enabled := true
	if _, err := repository.Upsert(PackPreference{
		OwnerIdentity: "alice", PackID: PackWorkVenture, Enabled: &disabled, ClassificationBoost: -10,
	}); err != nil {
		t.Fatalf("Upsert alice: %v", err)
	}
	if _, err := repository.Upsert(PackPreference{
		OwnerIdentity: "bob", PackID: PackWorkVenture, Enabled: &enabled, ClassificationBoost: 10,
	}); err != nil {
		t.Fatalf("Upsert bob: %v", err)
	}
	alice, ok, err := repository.Get("alice", PackWorkVenture)
	if err != nil || !ok || alice.Enabled == nil || *alice.Enabled || alice.UpdatedAt != fixed {
		t.Fatalf("alice preference = %#v, ok=%v err=%v", alice, ok, err)
	}
	bob, ok, err := repository.Get("bob", PackWorkVenture)
	if err != nil || !ok || bob.Enabled == nil || !*bob.Enabled {
		t.Fatalf("bob preference = %#v, ok=%v err=%v", bob, ok, err)
	}
	alice.Enabled = &enabled
	stored, _, _ := repository.Get("alice", PackWorkVenture)
	if stored.Enabled == nil || *stored.Enabled {
		t.Fatal("returned preference mutated stored owner state")
	}
	if _, _, err := repository.Get("", PackWorkVenture); err == nil {
		t.Fatal("ownerless preference access must fail")
	}
	list, err := repository.List("alice")
	if err != nil || len(list) != 1 || list[0].OwnerIdentity != "alice" {
		t.Fatalf("alice list = %#v, err=%v", list, err)
	}
}

func TestClassificationRespectsOwnerPreferenceWithoutLeaking(t *testing.T) {
	t.Parallel()
	registry := mustBuiltinRegistry(t)
	repository := NewMemoryPreferenceRepository(nil)
	disabled := false
	if _, err := repository.Upsert(PackPreference{OwnerIdentity: "alice", PackID: PackWorkVenture, Enabled: &disabled}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	alice, err := registry.Classify(ClassificationRequest{
		OwnerIdentity: "alice", Text: "Review this github repository and client deliverable.",
	}, repository)
	if err != nil {
		t.Fatalf("Classify alice: %v", err)
	}
	if _, ok := findMatch(alice.Matches, PackWorkVenture); ok {
		t.Fatal("disabled pack classified for alice")
	}
	if suppressed, ok := findSuppressed(alice.Suppressed, PackWorkVenture); !ok || !strings.Contains(suppressed.Reason, "owner-scoped") {
		t.Fatalf("missing preference suppression: %#v", alice.Suppressed)
	}
	bob, err := registry.Classify(ClassificationRequest{
		OwnerIdentity: "bob", Text: "Review this github repository and client deliverable.",
	}, repository)
	if err != nil {
		t.Fatalf("Classify bob: %v", err)
	}
	if _, ok := findMatch(bob.Matches, PackWorkVenture); !ok {
		t.Fatal("alice preference leaked into bob classification")
	}
}

func TestCatalogAndRegistryAreImmutableToCallers(t *testing.T) {
	t.Parallel()
	first := BuiltinPacks()
	originalName := first[0].Name
	originalQuestion := first[0].IntakeQuestions[0].Question
	first[0].Name = "mutated"
	first[0].IntakeQuestions[0].Question = "mutated"
	first[0].SourceAuthorityRules[0].AcceptedSources[0] = "mutated"
	second := BuiltinPacks()
	if second[0].Name != originalName || second[0].IntakeQuestions[0].Question != originalQuestion {
		t.Fatal("BuiltinPacks returned shared mutable state")
	}

	registry := mustBuiltinRegistry(t)
	pack, ok := registry.Lookup(PackLegalGovernment)
	if !ok {
		t.Fatal("legal pack missing")
	}
	pack.Name = "mutated"
	pack.SuccessCriteriaTemplates[0].Criteria[0] = "mutated"
	again, _ := registry.Lookup(PackLegalGovernment)
	if again.Name == "mutated" || again.SuccessCriteriaTemplates[0].Criteria[0] == "mutated" {
		t.Fatal("registry lookup exposed mutable state")
	}
	list := registry.List()
	list[0].AuditEvents[0] = "mutated"
	againList := registry.List()
	if againList[0].AuditEvents[0] == "mutated" {
		t.Fatal("registry list exposed mutable state")
	}
}

func TestRegistryDigestIsDeterministicAndVersioned(t *testing.T) {
	t.Parallel()
	first := mustBuiltinRegistry(t)
	packs := BuiltinPacks()
	sort.Slice(packs, func(i, j int) bool { return packs[i].ID > packs[j].ID })
	second, err := NewRegistry(CatalogVersion, packs)
	if err != nil {
		t.Fatalf("NewRegistry reversed: %v", err)
	}
	if first.Metadata() != second.Metadata() {
		t.Fatalf("metadata differs: %#v != %#v", first.Metadata(), second.Metadata())
	}
	if !strings.HasPrefix(first.Metadata().Digest, "sha256:") || len(first.Metadata().Digest) != len("sha256:")+64 {
		t.Fatalf("unexpected digest %q", first.Metadata().Digest)
	}
	if first.Metadata().Version != CatalogVersion {
		t.Fatalf("version = %q", first.Metadata().Version)
	}
}

func TestPolicyConflictsAndUnsafeOmissionsAreRejected(t *testing.T) {
	t.Parallel()
	base := BuiltinPacks()[0]
	for index := range base.ApprovalRules {
		if base.ApprovalRules[index].Action == "legal_or_government_action" {
			base.ApprovalRules[index].Required = false
		}
	}
	if err := ValidatePack(base); err == nil || !strings.Contains(err.Error(), "must require approval") {
		t.Fatalf("unsafe legal policy error = %v", err)
	}

	conflict := BuiltinPacks()[0]
	conflict.ApprovalRules = append(conflict.ApprovalRules,
		ApprovalRule{Action: "public_post", Required: false, MinimumRisk: RiskLow, Reason: "unsafe override"})
	if err := ValidatePack(conflict); err == nil || !strings.Contains(err.Error(), "conflicting approval rules") {
		t.Fatalf("conflicting policy error = %v", err)
	}

	sensitive := BuiltinPacks()[0]
	sensitive.Retention.LocalOnly = false
	if err := ValidatePack(sensitive); err == nil || !strings.Contains(err.Error(), "local-only") {
		t.Fatalf("sensitive retention error = %v", err)
	}
}

func TestResolveAppliesConservativePreferenceOverlay(t *testing.T) {
	t.Parallel()
	registry := mustBuiltinRegistry(t)
	repository := NewMemoryPreferenceRepository(nil)
	enabled := true
	if _, err := repository.Upsert(PackPreference{
		OwnerIdentity: "alice", PackID: PackWorkVenture, Enabled: &enabled, ForceLocalOnly: true,
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	view, err := registry.Resolve("alice", PackWorkVenture, repository)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !view.Enabled || !view.LocalOnly || view.Preference == nil {
		t.Fatalf("resolved view = %#v", view)
	}
	view.Pack.Name = "mutated"
	again, _ := registry.Resolve("alice", PackWorkVenture, repository)
	if again.Pack.Name == "mutated" {
		t.Fatal("resolved view exposed registry state")
	}
}

func TestActiveAdaptationAffectsClassificationAndDraftDoesNot(t *testing.T) {
	t.Parallel()
	registry := mustBuiltinRegistry(t)
	repository := NewMemoryPreferenceRepository(nil)
	adaptation := PackAdaptation{
		AdditionalClassificationSignals: []ClassificationSignal{{
			Phrase: "north star review", Strength: SignalStrong, Reason: "owner-specific planning phrase",
		}},
		AdditionalIntakeQuestions: []IntakeQuestion{{
			ID: "owner_outcome", Question: "What owner outcome is expected?", Required: true,
		}},
		AdditionalStopConditions: []StopCondition{{
			ID: "owner_stop", Condition: "owner pauses the work", EscalateTo: "owner_review", Level: RiskHigh,
		}},
	}
	if _, err := repository.Upsert(PackPreference{
		OwnerIdentity: "alice", PackID: PackPersonalProductivity,
		Status: PreferenceStatusDraft, Adaptation: adaptation,
	}); err != nil {
		t.Fatalf("Upsert draft: %v", err)
	}
	draft, err := registry.Classify(ClassificationRequest{
		OwnerIdentity: "alice", Text: "Run my north star review.",
	}, repository)
	if err != nil {
		t.Fatalf("Classify draft: %v", err)
	}
	if _, exists := findMatch(draft.Matches, PackPersonalProductivity); exists {
		t.Fatal("draft adaptation affected classification")
	}
	stored, _, err := repository.Get("alice", PackPersonalProductivity)
	if err != nil {
		t.Fatalf("Get draft: %v", err)
	}
	stored.Status = PreferenceStatusActive
	if _, err := repository.Upsert(stored); err != nil {
		t.Fatalf("activate adaptation: %v", err)
	}
	active, err := registry.Classify(ClassificationRequest{
		OwnerIdentity: "alice", Text: "Run my north star review.",
	}, repository)
	if err != nil {
		t.Fatalf("Classify active: %v", err)
	}
	match, exists := findMatch(active.Matches, PackPersonalProductivity)
	if !exists || match.Score < 70 {
		t.Fatalf("active adaptation match = %#v", match)
	}
	view, err := registry.Resolve("alice", PackPersonalProductivity, repository)
	if err != nil {
		t.Fatalf("Resolve active: %v", err)
	}
	if len(view.Pack.IntakeQuestions) != len(BuiltinPacks()[9].IntakeQuestions)+1 {
		t.Fatalf("effective intake questions were not adapted: %#v", view.Pack.IntakeQuestions)
	}
}

func TestPreferenceRevisionConflictAndRoundTrip(t *testing.T) {
	t.Parallel()
	repository := NewMemoryPreferenceRepository(nil)
	first, err := repository.Upsert(PackPreference{
		OwnerIdentity: "alice", PackID: PackWorkVenture,
		Status:     PreferenceStatusActive,
		Adaptation: PackAdaptation{Notes: "owner-reviewed"},
	})
	if err != nil {
		t.Fatalf("first Upsert: %v", err)
	}
	if first.Revision != 1 || first.CatalogVersion != CatalogVersion ||
		first.CreatedAt.IsZero() || first.UpdatedAt.IsZero() {
		t.Fatalf("first preference metadata = %#v", first)
	}
	stale := first
	stale.Revision = first.Revision + 1
	if _, err := repository.Upsert(stale); !errors.Is(err, ErrPreferenceConflict) {
		t.Fatalf("stale Upsert error = %v, want ErrPreferenceConflict", err)
	}
	current, ok, err := repository.Get("alice", PackWorkVenture)
	if err != nil || !ok || current.Adaptation.Notes != "owner-reviewed" {
		t.Fatalf("round trip = %#v, ok=%v, err=%v", current, ok, err)
	}
}

func TestClassificationOutputIsStable(t *testing.T) {
	t.Parallel()
	registry := mustBuiltinRegistry(t)
	request := ClassificationRequest{Text: "Draft email for a client deliverable in the github repository."}
	first, err := registry.Classify(request, nil)
	if err != nil {
		t.Fatalf("first Classify: %v", err)
	}
	second, err := registry.Classify(request, nil)
	if err != nil {
		t.Fatalf("second Classify: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("classification is not deterministic:\n%#v\n%#v", first, second)
	}
}

func mustBuiltinRegistry(t *testing.T) *Registry {
	t.Helper()
	registry, err := NewBuiltinRegistry()
	if err != nil {
		t.Fatalf("NewBuiltinRegistry: %v", err)
	}
	return registry
}

func findMatch(matches []ClassificationMatch, id PackID) (ClassificationMatch, bool) {
	for _, match := range matches {
		if match.PackID == id {
			return match, true
		}
	}
	return ClassificationMatch{}, false
}

func findSuppressed(matches []SuppressedMatch, id PackID) (SuppressedMatch, bool) {
	for _, match := range matches {
		if match.PackID == id {
			return match, true
		}
	}
	return SuppressedMatch{}, false
}
