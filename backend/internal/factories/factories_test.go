package factories

import (
	"testing"

	"automation-hub-backend/internal/invariants"
	"automation-hub-backend/internal/models"
)

func TestMemoryDefaultsAreValid(t *testing.T) {
	if v := invariants.ValidateMemory(Memory()); !invariants.Valid(v) {
		t.Fatalf("default memory should satisfy invariants, got %+v", v)
	}
}

func TestMemoryOverridesApply(t *testing.T) {
	m := Memory(func(m *models.ContextMemory) { m.Kind = "preference"; m.Confidence = 0.5 })
	if m.Kind != "preference" || m.Confidence != 0.5 {
		t.Fatalf("overrides not applied: %+v", m)
	}
}

func TestMemoriesAreDistinctAndValid(t *testing.T) {
	list := Memories(5)
	if len(list) != 5 {
		t.Fatalf("want 5, got %d", len(list))
	}
	seen := map[string]bool{}
	for _, m := range list {
		if seen[m.ID.String()] {
			t.Fatalf("duplicate id generated")
		}
		seen[m.ID.String()] = true
		if !invariants.Valid(invariants.ValidateMemory(m)) {
			t.Fatalf("generated memory invalid: %+v", m)
		}
	}
}
