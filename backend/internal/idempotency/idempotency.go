// Package idempotency provides a small, dependency-free store that detects
// duplicate operations keyed by an idempotency key within a time window. The
// current time is passed in so behaviour is deterministic and unit-testable.
package idempotency

import (
	"sync"
	"time"
)

// Store records seen idempotency keys and reports duplicates within a TTL.
// It is safe for concurrent use.
type Store struct {
	mu   sync.Mutex
	ttl  time.Duration
	seen map[string]time.Time
}

// New returns a Store whose keys expire after ttl. A non-positive ttl disables
// expiry (keys are remembered for the process lifetime).
func New(ttl time.Duration) *Store {
	return &Store{ttl: ttl, seen: make(map[string]time.Time)}
}

// FirstSeen records key at time now and reports whether this is the first time
// the key has been seen within the TTL. It returns true for a fresh key and
// false for a duplicate. Expired keys are treated as fresh again.
func (s *Store) FirstSeen(key string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if at, ok := s.seen[key]; ok {
		if s.ttl <= 0 || now.Sub(at) < s.ttl {
			return false
		}
	}
	s.seen[key] = now
	return true
}

// IsDuplicate reports whether key was already seen within the TTL, without
// recording anything.
func (s *Store) IsDuplicate(key string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	at, ok := s.seen[key]
	if !ok {
		return false
	}
	if s.ttl <= 0 {
		return true
	}
	return now.Sub(at) < s.ttl
}
