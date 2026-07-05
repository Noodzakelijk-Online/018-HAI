package featureflags

import "testing"

func TestBooleanToggle(t *testing.T) {
	s := New()
	s.Set(Flag{Key: "new_ui", Enabled: true, RolloutPercent: 100})
	s.Set(Flag{Key: "beta", Enabled: false, RolloutPercent: 100})
	if !s.IsEnabled("new_ui") {
		t.Fatalf("new_ui should be enabled")
	}
	if s.IsEnabled("beta") {
		t.Fatalf("beta should be disabled")
	}
	if s.IsEnabled("missing") {
		t.Fatalf("unknown flag must be disabled")
	}
}

func TestRolloutIsDeterministicPerSubject(t *testing.T) {
	s := New()
	s.Set(Flag{Key: "gradual", Enabled: true, RolloutPercent: 50})
	// Same subject → same answer every time.
	first := s.IsEnabledFor("gradual", "user-42")
	for i := 0; i < 100; i++ {
		if s.IsEnabledFor("gradual", "user-42") != first {
			t.Fatalf("rollout decision not stable for a subject")
		}
	}
	// A partial rollout is not "globally enabled".
	if s.IsEnabled("gradual") {
		t.Fatalf("50%% rollout must not report globally enabled")
	}
}

func TestRolloutBoundaries(t *testing.T) {
	s := New()
	s.Set(Flag{Key: "off", Enabled: true, RolloutPercent: 0})
	s.Set(Flag{Key: "full", Enabled: true, RolloutPercent: 100})
	if s.IsEnabledFor("off", "anyone") {
		t.Fatalf("0%% rollout should be off for everyone")
	}
	if !s.IsEnabledFor("full", "anyone") {
		t.Fatalf("100%% rollout should be on for everyone")
	}
}

func TestSetClampsPercentAndListsSorted(t *testing.T) {
	s := New()
	s.Set(Flag{Key: "b", Enabled: true, RolloutPercent: 250})
	s.Set(Flag{Key: "a", Enabled: true, RolloutPercent: -5})
	list := s.List()
	if len(list) != 2 || list[0].Key != "a" || list[1].Key != "b" {
		t.Fatalf("List not sorted: %+v", list)
	}
	if list[1].RolloutPercent != 100 || list[0].RolloutPercent != 0 {
		t.Fatalf("percent not clamped: %+v", list)
	}
}
