package secretrotation

import (
	"testing"
	"time"
)

func TestDueWhenPastMaxAge(t *testing.T) {
	p := Policy{MaxAgeDays: 90}
	last := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if p.Due(last, last.AddDate(0, 0, 89)) {
		t.Fatalf("should not be due before deadline")
	}
	if !p.Due(last, last.AddDate(0, 0, 90)) {
		t.Fatalf("should be due at deadline")
	}
	if !p.Due(last, last.AddDate(0, 0, 200)) {
		t.Fatalf("should be due well past deadline")
	}
}

func TestDaysUntilDue(t *testing.T) {
	p := Policy{MaxAgeDays: 30}
	last := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if d := p.DaysUntilDue(last, last.AddDate(0, 0, 10)); d != 20 {
		t.Fatalf("days until due = %d, want 20", d)
	}
	if d := p.DaysUntilDue(last, last.AddDate(0, 0, 40)); d != 0 {
		t.Fatalf("overdue should report 0, got %d", d)
	}
}

func TestDisabledPolicy(t *testing.T) {
	p := Policy{MaxAgeDays: 0}
	now := time.Now()
	if p.Due(now.AddDate(-10, 0, 0), now) {
		t.Fatalf("disabled policy is never due")
	}
	if p.DaysUntilDue(now, now) != -1 {
		t.Fatalf("disabled policy should report -1")
	}
}
