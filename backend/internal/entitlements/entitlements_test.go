package entitlements

import "testing"

func TestAllCoreFeaturesAreFree(t *testing.T) {
	for _, f := range FreeTier() {
		if !Available(f) {
			t.Fatalf("free-tier feature %q not available", f)
		}
		if RequiresPayment(f) {
			t.Fatalf("core feature %q must not require payment (no forced billing)", f)
		}
	}
}

func TestCriticalPathFeaturesPresent(t *testing.T) {
	for _, f := range []Feature{Memory, Search, Workflows, Approvals, Export, Automation, Verification} {
		if !Available(f) {
			t.Fatalf("critical feature %q should be available free", f)
		}
	}
}

func TestFreeTierIsSortedAndComplete(t *testing.T) {
	ft := FreeTier()
	if len(ft) != 7 {
		t.Fatalf("free tier size = %d, want 7", len(ft))
	}
	for i := 1; i < len(ft); i++ {
		if ft[i-1] > ft[i] {
			t.Fatalf("free tier not sorted: %v", ft)
		}
	}
}
