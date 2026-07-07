package session

import (
	"testing"
	"time"
)

func TestValidWithinTTL(t *testing.T) {
	issued := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := New("tok", issued, time.Hour)
	if !s.Valid(issued.Add(30 * time.Minute)) {
		t.Fatalf("session should be valid within ttl")
	}
	if s.Valid(issued.Add(2 * time.Hour)) {
		t.Fatalf("session should be expired past ttl")
	}
}

func TestEmptyTokenNeverValid(t *testing.T) {
	s := New("", time.Now(), time.Hour)
	if s.Valid(time.Now()) {
		t.Fatalf("empty token must never be valid")
	}
}

func TestRemainingClampsAtZero(t *testing.T) {
	issued := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := New("tok", issued, time.Hour)
	if s.Remaining(issued.Add(30*time.Minute)) != 30*time.Minute {
		t.Fatalf("remaining wrong")
	}
	if s.Remaining(issued.Add(2*time.Hour)) != 0 {
		t.Fatalf("expired remaining should clamp to 0")
	}
}
