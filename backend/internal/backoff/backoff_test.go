package backoff

import (
	"testing"
	"time"
)

func TestExponentialGrowth(t *testing.T) {
	p := Policy{Base: time.Second, Factor: 2, Max: time.Minute}
	want := []time.Duration{0, time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}
	for attempt, expected := range want {
		if got := p.Delay(attempt); got != expected {
			t.Fatalf("Delay(%d) = %v, want %v", attempt, got, expected)
		}
	}
}

func TestDelayIsCapped(t *testing.T) {
	p := Policy{Base: time.Second, Factor: 2, Max: 5 * time.Second}
	if got := p.Delay(10); got != 5*time.Second {
		t.Fatalf("Delay(10) = %v, want cap 5s", got)
	}
}

func TestZeroAndDefault(t *testing.T) {
	if DefaultPolicy().Delay(0) != 0 {
		t.Fatalf("attempt 0 must be no delay")
	}
	if DefaultPolicy().Delay(1) != time.Second {
		t.Fatalf("first retry should be base")
	}
}
