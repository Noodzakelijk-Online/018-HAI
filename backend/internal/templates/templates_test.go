package templates

import "testing"

func TestDefaultRegistryHasSeededPresets(t *testing.T) {
	r := DefaultRegistry()
	if list := r.ListMemory(); len(list) != 3 || list[0].Name != "contact" {
		t.Fatalf("expected 3 sorted presets, got %+v", list)
	}
	if _, ok := r.Memory("PREFERENCE"); !ok {
		t.Fatalf("lookup should be case-insensitive")
	}
	if _, ok := r.Memory("nope"); ok {
		t.Fatalf("unknown template should not resolve")
	}
}

func TestApplyFillsOnlyEmptyFields(t *testing.T) {
	tmpl, _ := DefaultRegistry().Memory("preference")

	// Empty draft is fully seeded from the template.
	seeded := tmpl.Apply(Draft{})
	if seeded.Kind != "preference" || seeded.Confidence != 0.8 || len(seeded.Tags) != 1 {
		t.Fatalf("empty draft not seeded: %+v", seeded)
	}

	// Explicit values are preserved, not overwritten.
	custom := tmpl.Apply(Draft{Kind: "custom", Confidence: 0.5, Tags: []string{"mine"}})
	if custom.Kind != "custom" || custom.Confidence != 0.5 || custom.Tags[0] != "mine" {
		t.Fatalf("explicit values overwritten: %+v", custom)
	}
	// Summary was empty, so it gets the template value.
	if custom.Summary != "User preference" {
		t.Fatalf("empty summary not filled: %+v", custom)
	}
}
