package providerfallback

import "testing"

func TestPrefersFirstAvailableFreeProvider(t *testing.T) {
	providers := []Provider{
		{Name: "ollama-down", Available: false, Paid: false},
		{Name: "ollama-up", Available: true, Paid: false},
		{Name: "openai", Available: true, Paid: true},
	}
	got, ok := Select(providers, true)
	if !ok || got.Name != "ollama-up" {
		t.Fatalf("selected %q (ok=%v), want ollama-up", got.Name, ok)
	}
}

func TestNeverSelectsPaidWhenDisallowed(t *testing.T) {
	providers := []Provider{
		{Name: "local", Available: false, Paid: false},
		{Name: "openai", Available: true, Paid: true},
	}
	if _, ok := Select(providers, false); ok {
		t.Fatalf("paid provider selected while paid usage disallowed")
	}
}

func TestFallsBackToPaidOnlyWhenAllowed(t *testing.T) {
	providers := []Provider{
		{Name: "local", Available: false, Paid: false},
		{Name: "openai", Available: true, Paid: true},
	}
	got, ok := Select(providers, true)
	if !ok || got.Name != "openai" {
		t.Fatalf("selected %q (ok=%v), want openai fallback", got.Name, ok)
	}
}

func TestDeterministicAndEmpty(t *testing.T) {
	if _, ok := Select(nil, true); ok {
		t.Fatalf("empty provider list should select nothing")
	}
	providers := []Provider{{Name: "a", Available: true}, {Name: "b", Available: true}}
	for i := 0; i < 50; i++ {
		got, _ := Select(providers, true)
		if got.Name != "a" {
			t.Fatalf("selection not deterministic: got %q", got.Name)
		}
	}
}
