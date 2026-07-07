package idempotency

import (
	"testing"
	"time"
)

func TestFirstSeenDetectsDuplicates(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := New(time.Minute)
	if !s.FirstSeen("k1", now) {
		t.Fatalf("first occurrence should be fresh")
	}
	if s.FirstSeen("k1", now) {
		t.Fatalf("second occurrence should be a duplicate")
	}
	if !s.IsDuplicate("k1", now) {
		t.Fatalf("IsDuplicate should report true for a seen key")
	}
	if s.IsDuplicate("other", now) {
		t.Fatalf("unseen key must not be a duplicate")
	}
}

func TestKeyExpiresAfterTTL(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := New(time.Minute)
	s.FirstSeen("k", start)
	if s.FirstSeen("k", start.Add(30*time.Second)) {
		t.Fatalf("within TTL should still be duplicate")
	}
	if !s.FirstSeen("k", start.Add(61*time.Second)) {
		t.Fatalf("after TTL the key should be fresh again")
	}
}

func TestZeroTTLNeverExpires(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := New(0)
	s.FirstSeen("k", start)
	if s.FirstSeen("k", start.Add(1000*time.Hour)) {
		t.Fatalf("zero TTL should remember keys indefinitely")
	}
}
