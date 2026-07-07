package reminders

import (
	"testing"
	"time"
)

func at(min int) time.Time {
	return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC).Add(time.Duration(min) * time.Minute)
}

func TestDueReturnsOverdueSorted(t *testing.T) {
	now := at(0)
	rs := []Reminder{
		{ID: "future", DueAt: at(10)},
		{ID: "now", DueAt: at(0)},
		{ID: "old", DueAt: at(-30)},
	}
	due := Due(rs, now)
	if len(due) != 2 {
		t.Fatalf("due = %d, want 2 (old + now)", len(due))
	}
	if due[0].ID != "old" || due[1].ID != "now" {
		t.Fatalf("due order wrong: %s, %s", due[0].ID, due[1].ID)
	}
}

func TestNextDue(t *testing.T) {
	now := at(0)
	rs := []Reminder{{ID: "a", DueAt: at(30)}, {ID: "b", DueAt: at(5)}, {ID: "past", DueAt: at(-5)}}
	next, ok := NextDue(rs, now)
	if !ok || next.ID != "b" {
		t.Fatalf("next due = %v (%v), want b", next.ID, ok)
	}
	if _, ok := NextDue([]Reminder{{ID: "past", DueAt: at(-1)}}, now); ok {
		t.Fatalf("no upcoming reminder should be found")
	}
}
