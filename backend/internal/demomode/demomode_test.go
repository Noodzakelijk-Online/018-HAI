package demomode

import "testing"

func TestParseDefaultsToProduction(t *testing.T) {
	for _, in := range []string{"", "unknown", "PROD", "production"} {
		if Parse(in) != Production {
			t.Fatalf("Parse(%q) should be production", in)
		}
	}
	if Parse("Demo") != Demo || Parse("test") != Test {
		t.Fatalf("demo/test parsing wrong")
	}
}

func TestOnlyProductionAllowsSideEffects(t *testing.T) {
	if !Production.AllowsRealSideEffects() {
		t.Fatalf("production must allow side effects")
	}
	if Demo.AllowsRealSideEffects() || Test.AllowsRealSideEffects() {
		t.Fatalf("non-production must not allow real side effects")
	}
}

func TestLabelsFlagNonProduction(t *testing.T) {
	if Production.Label() != "" {
		t.Fatalf("production should have no banner")
	}
	if Demo.Label() == "" || Test.Label() == "" {
		t.Fatalf("demo/test must be labelled")
	}
}
