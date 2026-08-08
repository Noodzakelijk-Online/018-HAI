package domainpack

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

type expectedMethodGroup struct {
	packID  PackID
	section string
	names   string
}

var expectedWholeLifeMethods = map[string]expectedMethodGroup{
	"health_personal_care": {
		packID: PackHealthWellbeing, section: "40",
		names: "Biopsychosocial assessment|SOAP|SBAR|PQRST symptom description|Red-flag triage|Shared decision-making|Medication reconciliation|Medication-rights checklist|Care-plan cycles|ADL/IADL monitoring|Sleep hygiene|Pacing|Energy envelope|Pain diary|Trigger tracking|Symptom trend analysis|Preventive-care schedules|Escalation thresholds|Emergency contacts|Caregiver coordination|Accessibility accommodations",
	},
	"financial_management": {
		packID: PackFinancial, section: "41",
		names: "Zero-based budgeting|Envelope budgeting|50/30/20|Pay-yourself-first|Cash-flow forecasting|Sinking funds|Emergency-fund ladder|Debt snowball|Debt avalanche|Net-worth tracking|Asset-liability management|Liquidity management|Expected-value decisions|Scenario analysis|Sensitivity analysis|Total Cost of Ownership|Life-cycle costing|Opportunity cost|Risk-adjusted return|Fraud controls|Four-eyes payments|Spending mandates|Subscription review|Tax-calendar management|Financial document retention",
	},
	"home_garden_assets": {
		packID: PackHomeAssets, section: "42",
		names: "Asset registry|Preventive maintenance|Predictive maintenance|Reliability-Centred Maintenance|FMEA|Seasonal maintenance|5S|Visual management|ABC inventory|XYZ inventory|Reorder points|Min-max stocking|Bill of materials|Tool tracking|Warranty tracking|Inspection checklists|Energy monitoring|Water monitoring|Waste hierarchy|Circular-economy principles|Integrated Pest Management|Plant-health monitoring|Weather-aware planning|Work-order management|Contractor and quotation comparison",
	},
	"work_service_delivery": {
		packID: PackWorkVenture, section: "43",
		names: "Standard operating procedures|Checklists|Job breakdown|Work packages|Critical Path|PERT|Lean|5S|Kaizen|Theory of Constraints|Value-stream mapping|DMAIC|Root-cause analysis|Quality gates|Safety briefings|Toolbox talks|Resource planning|Capacity planning|Skills matrices|RACI|Service blueprints|Customer journey mapping|CRM pipelines|Quote-accept-deliver-invoice-follow-up|Scope-change control|Time-and-material tracking|Cost estimation|Lessons learned",
	},
	"entrepreneurship_venture": {
		packID: PackWorkVenture, section: "44",
		names: "Business Model Canvas|Lean Canvas|Value Proposition Canvas|Jobs to Be Done|Lean Startup|Build-Measure-Learn|Customer Development|Design Thinking|Double Diamond|Effectuation|Causation versus effectuation|Blue Ocean Strategy|Strategy Canvas|SWOT|TOWS|PESTEL|Porter's Five Forces|VRIO|Ansoff Matrix|BCG Matrix|GE-McKinsey Matrix|Three Horizons|Wardley Mapping|Crossing the Chasm|Technology Adoption Lifecycle|Stage-Gate|Product-market fit|North Star Metric|AARRR|Unit economics|Cohort analysis|TAM/SAM/SOM|Scenario planning|Theory of Change|Stakeholder mapping|OKRs|Balanced Scorecard",
	},
	"legal_government_case": {
		packID: PackLegalGovernment, section: "45",
		names: "IRAC|CRAC|CREAC|FIRAC|Claim-issue-evidence matrices|Chronology|Procedural timeline|Deadline and limitation tracking|Burden-of-proof mapping|Elements-of-claim analysis|Authority hierarchy|Primary-source verification|Evidence chain of custody|Document authenticity|Contradiction matrix|Damage schedules|Remedy mapping|Stakeholder mapping|Escalation ladders|Complaint-objection-appeal workflows|Freedom-of-information workflows|GDPR access and correction workflows|Legal-hold management|Correspondence registers|Promise and commitment tracking",
	},
	"communication": {
		packID: PackCommunication, section: "46",
		names: "BLUF|Pyramid Principle|SCQA|7Cs of Communication|Audience-Purpose-Message|AIDA|PAS|FAB|Situation-Behaviour-Impact|DESC|Nonviolent Communication|Active listening|Reflective listening|Motivational interviewing|Steelmanning|Argument mapping|Negotiation preparation|BATNA|ZOPA|Interest-based negotiation|Difficult-conversation framework|Formal-government correspondence|Email triage|Thread and commitment extraction|Approval-before-send|Recipient verification",
	},
	"relationships_care": {
		packID: PackRelationshipsCare, section: "47",
		names: "Nonviolent Communication|Gottman principles|Active-constructive responding|Attachment-informed communication|Family-systems thinking|Transactional Analysis|Drama Triangle and Empowerment Dynamic|Circle of control|Boundary-setting|Love Languages as a conversational heuristic|Conflict de-escalation|Restorative practices|Interest-based negotiation|Care plans|Shared calendars|Household responsibility matrices|Reciprocity tracking without scorekeeping|Important-date reminders|Social-capacity management|Safeguarding escalation",
	},
	"learning_competence": {
		packID: PackLearningGrowth, section: "48",
		names: "Bloom's Taxonomy|Revised Bloom|Feynman Technique|Active recall|Spaced repetition|Interleaving|Retrieval practice|Deliberate practice|Mastery learning|Kolb learning cycle|Experiential learning|Zone of Proximal Development|Scaffolding|Cognitive Load Theory|Dual coding|Worked examples|Testing effect|Leitner system|70-20-10|Competency matrices|Skills-gap analysis|Learning objectives|Formative and summative assessment|Teach-back|Portfolio-based evidence",
	},
	"travel_mobility": {
		packID: PackTravelMobility, section: "49",
		names: "Door-to-door journey planning|Time-dependent routing|Multi-objective route optimisation|Travelling-salesperson variants|Accessibility constraints|Transfer-risk scoring|Buffer planning|Weather-aware routing|Fare optimisation|Energy and fatigue planning|Last-mile planning|Contingency routes|Geofenced reminders|Packing checklists|Live disruption management",
	},
	"emergency_continuity": {
		packID: PackEmergencyContinuity, section: "50",
		names: "Personal emergency plan|Incident Command System|Triage|Severity classification|Contact trees|Evacuation plans|Shelter-in-place plans|Medical-information card|Emergency funds|Go-bags|Pet emergency plans|Utility outage plans|Communication fallback|Data backup and recovery|Account-recovery kits|Decision authority during incapacity|After-action review",
	},
}

func TestWholeLifePlaybooksExactCoverageAndStablePlacement(t *testing.T) {
	t.Parallel()
	packs := BuiltinPacks()
	actual := map[string]expectedMethodGroup{}
	globalIDs := map[string]PackID{}
	total := 0
	for _, pack := range packs {
		for _, method := range pack.Playbook.Methods {
			total++
			if prior, exists := globalIDs[method.ID]; exists {
				t.Fatalf("method id %q duplicated in %s and %s", method.ID, prior, pack.ID)
			}
			globalIDs[method.ID] = pack.ID
			group := actual[method.Group]
			if group.packID != "" && group.packID != pack.ID {
				t.Fatalf("group %q crosses stable packs %s and %s", method.Group, group.packID, pack.ID)
			}
			if group.section != "" && group.section != method.Provenance.Section {
				t.Fatalf("group %q crosses specification sections", method.Group)
			}
			group.packID = pack.ID
			group.section = method.Provenance.Section
			if group.names == "" {
				group.names = method.Name
			} else {
				group.names += "|" + method.Name
			}
			actual[method.Group] = group
		}
	}
	if total != 264 {
		t.Fatalf("whole-life method count = %d, want 264", total)
	}
	if len(actual) != len(expectedWholeLifeMethods) {
		t.Fatalf("method group count = %d, want %d", len(actual), len(expectedWholeLifeMethods))
	}
	for groupID, expected := range expectedWholeLifeMethods {
		got, exists := actual[groupID]
		if !exists {
			t.Errorf("missing method group %q", groupID)
			continue
		}
		if got.packID != expected.packID || got.section != expected.section {
			t.Errorf("%s placement = pack %s section %s, want pack %s section %s",
				groupID, got.packID, got.section, expected.packID, expected.section)
		}
		gotNames := strings.Split(got.names, "|")
		expectedNames := strings.Split(expected.names, "|")
		sort.Strings(gotNames)
		sort.Strings(expectedNames)
		if !reflect.DeepEqual(gotNames, expectedNames) {
			t.Errorf("%s names differ:\ngot  %q\nwant %q", groupID, gotNames, expectedNames)
		}
	}
	work, ok := findPack(packs, PackWorkVenture)
	if !ok {
		t.Fatal("work_venture pack missing")
	}
	groups := map[string]bool{}
	for _, method := range work.Playbook.Methods {
		groups[method.Group] = true
	}
	if !groups["work_service_delivery"] || !groups["entrepreneurship_venture"] || len(groups) != 2 {
		t.Fatalf("work_venture groups = %#v", groups)
	}
}

func TestEveryPlaybookMethodIsCompleteAdvisoryAndTraceable(t *testing.T) {
	t.Parallel()
	for _, pack := range BuiltinPacks() {
		if pack.Playbook.Version != PlaybookVersion {
			t.Errorf("%s playbook version = %q", pack.ID, pack.Playbook.Version)
		}
		if got := mustPlaybookDigest(pack.Playbook); got != pack.Playbook.Digest {
			t.Errorf("%s playbook digest = %q, want %q", pack.ID, pack.Playbook.Digest, got)
		}
		for _, method := range pack.Playbook.Methods {
			if method.Version != PlaybookVersion || method.LifecycleStatus != MethodLifecycleActive {
				t.Errorf("%s version/lifecycle = %q/%q", method.ID, method.Version, method.LifecycleStatus)
			}
			for field, count := range map[string]int{
				"triggers": len(method.TriggerConditions), "inputs": len(method.RequiredInputs),
				"outputs": len(method.ProducedOutputs), "authority": len(method.AuthorityRequirements),
				"safety": len(method.SafetyInvariants), "evidence": len(method.EvidenceRequirements),
				"evaluation": len(method.Evaluation.Criteria),
			} {
				if count == 0 {
					t.Errorf("%s has empty %s", method.ID, field)
				}
			}
			if !containsNormalizedValue(method.AuthorityRequirements,
				"method selection is advisory and grants no execution authority") {
				t.Errorf("%s does not deny execution authority", method.ID)
			}
			if !containsNormalizedValue(method.SafetyInvariants,
				"all execution still passes HAI constitution, mandate, risk, approval, and final-effect authorization") {
				t.Errorf("%s weakens execution gates", method.ID)
			}
			if method.Provenance.SourceType != "owner_provided_design_specification" ||
				method.Provenance.Title != wholeLifeSpecificationTitle ||
				!strings.HasSuffix(method.Provenance.Reference, "/section-"+method.Provenance.Section) {
				t.Errorf("%s provenance = %#v", method.ID, method.Provenance)
			}
			if method.Evaluation.FailureDisposition == "" ||
				!strings.Contains(method.Evaluation.FailureDisposition, "do not execute") {
				t.Errorf("%s failure disposition is not fail-closed", method.ID)
			}
		}
	}
}

func TestPlaybookCatalogAndRegistryAreDeeplyImmutable(t *testing.T) {
	t.Parallel()
	first := BuiltinPacks()
	health, ok := findPack(first, PackHealthWellbeing)
	if !ok || len(health.Playbook.Methods) == 0 {
		t.Fatal("health methods missing")
	}
	health.Playbook.Methods[0].Name = "mutated"
	health.Playbook.Methods[0].RequiredInputs[0] = "mutated"
	for index := range first {
		if first[index].ID == PackHealthWellbeing {
			first[index] = health
		}
	}
	second, _ := findPack(BuiltinPacks(), PackHealthWellbeing)
	if second.Playbook.Methods[0].Name == "mutated" ||
		second.Playbook.Methods[0].RequiredInputs[0] == "mutated" {
		t.Fatal("BuiltinPacks exposed shared playbook state")
	}

	registry := mustBuiltinRegistry(t)
	lookup, _ := registry.Lookup(PackHealthWellbeing)
	lookup.Playbook.Methods[0].SafetyInvariants[0] = "mutated"
	again, _ := registry.Lookup(PackHealthWellbeing)
	if again.Playbook.Methods[0].SafetyInvariants[0] == "mutated" {
		t.Fatal("registry lookup exposed shared playbook state")
	}
}

func TestPlaybookValidationRejectsTamperingAndSafetyWeakening(t *testing.T) {
	t.Parallel()
	pack, _ := findPack(BuiltinPacks(), PackLegalGovernment)
	pack.Playbook.Methods[0].AuthorityRequirements = []string{"agent decides"}
	pack.Playbook.Digest = mustPlaybookDigest(pack.Playbook)
	if err := ValidatePack(pack); err == nil || !strings.Contains(err.Error(), "deny execution authority") {
		t.Fatalf("unsafe authority error = %v", err)
	}

	pack, _ = findPack(BuiltinPacks(), PackHealthWellbeing)
	pack.Playbook.Methods[0].SafetyInvariants = []string{"optimize speed"}
	pack.Playbook.Digest = mustPlaybookDigest(pack.Playbook)
	if err := ValidatePack(pack); err == nil || !strings.Contains(err.Error(), "preserve execution safety gates") {
		t.Fatalf("unsafe safety error = %v", err)
	}

	pack, _ = findPack(BuiltinPacks(), PackEmergencyContinuity)
	pack.Playbook.Methods[0].Purpose = "tampered"
	if err := ValidatePack(pack); err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("tamper digest error = %v", err)
	}
}

func TestSensitivePlaybookSafeguardsAndConservativeAdaptation(t *testing.T) {
	t.Parallel()
	for _, id := range []PackID{
		PackHealthWellbeing, PackFinancial, PackLegalGovernment,
		PackRelationshipsCare, PackEmergencyContinuity,
	} {
		pack, _ := findPack(BuiltinPacks(), id)
		if !pack.Sensitive || !pack.Retention.LocalOnly {
			t.Errorf("%s is not sensitive local-only", id)
		}
		for _, method := range pack.Playbook.Methods {
			if method.RiskCeiling != RiskHigh && method.RiskCeiling != RiskCritical {
				t.Errorf("%s sensitive risk ceiling = %q", method.ID, method.RiskCeiling)
			}
		}
	}

	base, _ := findPack(BuiltinPacks(), PackWorkVenture)
	effective, err := applyAdaptation(base, PackAdaptation{
		AdditionalClassificationSignals: []ClassificationSignal{{
			Phrase: "owner service phrase", Strength: SignalStrong, Reason: "owner reviewed",
		}},
	})
	if err != nil {
		t.Fatalf("applyAdaptation: %v", err)
	}
	if effective.Playbook.Digest != base.Playbook.Digest ||
		!reflect.DeepEqual(effective.Playbook.Methods, base.Playbook.Methods) {
		t.Fatal("owner adaptation changed immutable playbook methods")
	}
}

func TestLegacyCatalogPreferenceUpgradesWithoutChangingOwnerScope(t *testing.T) {
	t.Parallel()
	repository := NewMemoryPreferenceRepository(nil)
	disabled := false
	value, err := repository.Upsert(PackPreference{
		OwnerIdentity:  "alice",
		PackID:         PackWorkVenture,
		CatalogVersion: "1.1.0",
		Enabled:        &disabled,
	})
	if err != nil {
		t.Fatalf("legacy preference upgrade: %v", err)
	}
	if value.CatalogVersion != CatalogVersion || value.OwnerIdentity != "alice" {
		t.Fatalf("upgraded preference = %#v", value)
	}
	_, exists, err := repository.Get("bob", PackWorkVenture)
	if err != nil || exists {
		t.Fatalf("legacy preference leaked to bob: exists=%v err=%v", exists, err)
	}
}

func findPack(packs []DomainPack, id PackID) (DomainPack, bool) {
	for _, pack := range packs {
		if pack.ID == id {
			return pack, true
		}
	}
	return DomainPack{}, false
}
