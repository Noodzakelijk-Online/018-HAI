package analytics

import (
	"testing"
	"time"
)

func ev(typ string, day int) Event {
	return Event{Type: typ, At: time.Date(2026, 1, day, 12, 0, 0, 0, time.UTC)}
}

func TestAggregateCountsByTypeAndDay(t *testing.T) {
	events := []Event{
		ev("memory.create", 1),
		ev("memory.create", 1),
		ev("workflow.run", 1),
		ev("memory.create", 2),
		{Type: "", At: time.Now()}, // empty type ignored
	}
	s := Aggregate(events)
	if s.Total != 4 {
		t.Fatalf("total = %d, want 4 (empty type ignored)", s.Total)
	}
	if s.DistinctT != 2 {
		t.Fatalf("distinct types = %d, want 2", s.DistinctT)
	}
	// Sorted by count desc: memory.create (3) before workflow.run (1).
	if s.ByType[0].Type != "memory.create" || s.ByType[0].Count != 3 {
		t.Fatalf("byType top = %+v, want memory.create/3", s.ByType[0])
	}
	// Two distinct days, ascending.
	if len(s.ByDay) != 2 || s.ByDay[0].Type != "2026-01-01" || s.ByDay[0].Count != 3 {
		t.Fatalf("byDay wrong: %+v", s.ByDay)
	}
}

func TestAggregateTracksFirstAndLast(t *testing.T) {
	s := Aggregate([]Event{ev("a", 5), ev("a", 2), ev("a", 9)})
	if s.FirstEvent == nil || s.LastEvent == nil {
		t.Fatalf("first/last not set")
	}
	if s.FirstEvent.Day() != 2 || s.LastEvent.Day() != 9 {
		t.Fatalf("first/last = %v/%v, want day 2/9", s.FirstEvent.Day(), s.LastEvent.Day())
	}
}

func TestAggregateEmptyIsZeroValue(t *testing.T) {
	s := Aggregate(nil)
	if s.Total != 0 || s.DistinctT != 0 || s.FirstEvent != nil {
		t.Fatalf("empty aggregate should be zero-valued: %+v", s)
	}
}
