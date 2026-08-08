package frameworkregistry

import (
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const expectedBuiltinFamilyTaxonomyV11Digest = "00a0102b1373f82b58c711ec1d90b6f633267a57fcd29b6504854f762439a228"

func TestBuiltinFamilyTaxonomyCoversExactly55SequentialUniqueFamilies(t *testing.T) {
	t.Parallel()

	taxonomy, err := BuiltinFamilyTaxonomy()
	if err != nil {
		t.Fatalf("BuiltinFamilyTaxonomy: %v", err)
	}
	if got, want := len(taxonomy.Families), 55; got != want {
		t.Fatalf("family count = %d, want %d", got, want)
	}

	ids := make(map[string]struct{}, len(taxonomy.Families))
	sections := make(map[int]struct{}, len(taxonomy.Families))
	for index, family := range taxonomy.Families {
		expectedSection := index + 1
		if family.Section != expectedSection {
			t.Errorf("family[%d].Section = %d, want %d", index, family.Section, expectedSection)
		}
		if _, exists := sections[family.Section]; exists {
			t.Errorf("duplicate section %d", family.Section)
		}
		sections[family.Section] = struct{}{}
		if _, exists := ids[family.ID]; exists {
			t.Errorf("duplicate stable ID %q", family.ID)
		}
		ids[family.ID] = struct{}{}
		if got, want := family.ID, expectedFrameworkIDsBySection[index]; got != want {
			t.Errorf("section %d ID = %q, want %q", family.Section, got, want)
		}
		if !strings.Contains(family.Source, "section "+strconv.Itoa(family.Section)) {
			t.Errorf("section %d source %q does not retain numbered provenance", family.Section, family.Source)
		}
	}
}

func TestBuiltinFamilyTaxonomyHasRequiredGovernanceMetadata(t *testing.T) {
	t.Parallel()

	taxonomy, err := BuiltinFamilyTaxonomy()
	if err != nil {
		t.Fatalf("BuiltinFamilyTaxonomy: %v", err)
	}
	if err := ValidateFamilyTaxonomy(*taxonomy); err != nil {
		t.Fatalf("ValidateFamilyTaxonomy: %v", err)
	}

	semanticVersion := regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	for _, family := range taxonomy.Families {
		requiredScalars := map[string]string{
			"stable ID":             family.ID,
			"semantic version":      family.Version,
			"family category":       family.Family,
			"purpose":               family.Purpose,
			"authority requirement": family.AuthorityRequirement,
			"risk ceiling":          family.RiskCeiling,
			"source":                family.Source,
			"provenance":            family.Provenance,
			"status":                family.Status,
		}
		for field, value := range requiredScalars {
			if strings.TrimSpace(value) == "" {
				t.Errorf("section %d %s is blank", family.Section, field)
			}
		}
		if !semanticVersion.MatchString(family.Version) {
			t.Errorf("section %d framework version %q is not semantic", family.Section, family.Version)
		}
		requiredSlices := map[string][]string{
			"trigger summaries":     family.TriggerConditions,
			"input summaries":       family.RequiredInputs,
			"output summaries":      family.ProducedOutputs,
			"evidence requirements": family.EvidenceRequirements,
			"evaluation method":     family.EvaluationMethod,
			"safety invariants":     family.SafetyInvariants,
		}
		for field, values := range requiredSlices {
			if len(values) == 0 {
				t.Errorf("section %d %s is empty", family.Section, field)
			}
		}
		if family.MaximumAutonomyLevel < 0 || family.MaximumAutonomyLevel > 10 {
			t.Errorf("section %d autonomy level = %d", family.Section, family.MaximumAutonomyLevel)
		}
	}
}

func TestBuiltinFamilyTaxonomyDigestAndVersionAreDeterministic(t *testing.T) {
	t.Parallel()

	first, err := BuiltinFamilyTaxonomy()
	if err != nil {
		t.Fatalf("first BuiltinFamilyTaxonomy: %v", err)
	}
	second, err := BuiltinFamilyTaxonomy()
	if err != nil {
		t.Fatalf("second BuiltinFamilyTaxonomy: %v", err)
	}
	if FrameworkFamilyTaxonomyVersion != "1.1.0" {
		t.Fatalf("taxonomy version constant = %q, want 1.1.0", FrameworkFamilyTaxonomyVersion)
	}
	if first.Version != FrameworkFamilyTaxonomyVersion {
		t.Fatalf("taxonomy version = %q, want 1.1.0", first.Version)
	}
	if first.Digest != second.Digest {
		t.Fatalf("taxonomy digest is not deterministic: %q != %q", first.Digest, second.Digest)
	}
	if first.Digest != expectedBuiltinFamilyTaxonomyV11Digest {
		t.Fatalf("taxonomy v1.1 digest = %q, want %q", first.Digest, expectedBuiltinFamilyTaxonomyV11Digest)
	}
}

func TestFamilyTaxonomyReturnsImmutableCopies(t *testing.T) {
	t.Parallel()

	first, err := BuiltinFamilyTaxonomy()
	if err != nil {
		t.Fatalf("first BuiltinFamilyTaxonomy: %v", err)
	}
	originalID := first.Families[0].ID
	originalTrigger := first.Families[0].TriggerConditions[0]
	first.Families[0].ID = "mutated"
	first.Families[0].TriggerConditions[0] = "mutated"
	first.Families = first.Families[:1]

	second, err := BuiltinFamilyTaxonomy()
	if err != nil {
		t.Fatalf("second BuiltinFamilyTaxonomy: %v", err)
	}
	if got := second.Families[0].ID; got != originalID {
		t.Fatalf("stable ID leaked mutation: got %q, want %q", got, originalID)
	}
	if got := second.Families[0].TriggerConditions[0]; got != originalTrigger {
		t.Fatalf("nested metadata leaked mutation: got %q, want %q", got, originalTrigger)
	}
	if got := len(second.Families); got != 55 {
		t.Fatalf("family slice leaked mutation: got %d records", got)
	}
}

func TestServiceExposesFamilyTaxonomyWithoutMutableAliases(t *testing.T) {
	t.Parallel()

	service, err := NewService(NewMemoryRepository())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	taxonomy, err := service.FamilyTaxonomy()
	if err != nil {
		t.Fatalf("FamilyTaxonomy: %v", err)
	}
	if got, want := len(taxonomy.Families), 55; got != want {
		t.Fatalf("service family count = %d, want %d", got, want)
	}
	expected := taxonomy.Families[0].TriggerConditions[0]
	taxonomy.Families[0].TriggerConditions[0] = "caller mutation"

	again, err := service.FamilyTaxonomy()
	if err != nil {
		t.Fatalf("second FamilyTaxonomy: %v", err)
	}
	if got := again.Families[0].TriggerConditions[0]; got != expected {
		t.Fatalf("service taxonomy leaked caller mutation: got %q, want %q", got, expected)
	}

	views, err := service.List("owner@example.com")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	before := append([]string(nil), views[0].SafetyInvariants...)
	views[0].SafetyInvariants[0] = "caller mutation"
	viewsAgain, err := service.List("owner@example.com")
	if err != nil {
		t.Fatalf("second List: %v", err)
	}
	if !reflect.DeepEqual(viewsAgain[0].SafetyInvariants, before) {
		t.Fatalf("service list leaked nested framework metadata mutation")
	}
}

func TestValidateFamilyTaxonomyRejectsSequenceMetadataAndDigestDrift(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*FrameworkFamilyTaxonomy)
		want   string
	}{
		{
			name: "non semantic taxonomy version",
			mutate: func(value *FrameworkFamilyTaxonomy) {
				value.Version = "v1"
			},
			want: "invalid semantic version",
		},
		{
			name: "non sequential section",
			mutate: func(value *FrameworkFamilyTaxonomy) {
				value.Families[1].Section = 55
			},
			want: "want sequential section 2",
		},
		{
			name: "missing governance metadata",
			mutate: func(value *FrameworkFamilyTaxonomy) {
				value.Families[0].AuthorityRequirement = ""
			},
			want: "missing required scalar metadata",
		},
		{
			name: "content digest mismatch",
			mutate: func(value *FrameworkFamilyTaxonomy) {
				value.Families[0].Purpose += " changed"
			},
			want: "digest does not match",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			taxonomy, err := BuiltinFamilyTaxonomy()
			if err != nil {
				t.Fatalf("BuiltinFamilyTaxonomy: %v", err)
			}
			test.mutate(taxonomy)
			err = ValidateFamilyTaxonomy(*taxonomy)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateFamilyTaxonomy error = %v, want containing %q", err, test.want)
			}
		})
	}
}
