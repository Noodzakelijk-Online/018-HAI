// Package featureflags provides a small, dependency-free feature-flag store
// with boolean toggles and deterministic percentage rollouts. Rollout decisions
// are a stable hash of (flag, subject), so the same subject always lands the
// same way without persisting per-subject state.
package featureflags

import (
	"hash/fnv"
	"sort"
	"sync"
)

// Flag is the state of a single feature flag.
type Flag struct {
	Key             string `json:"key"`
	Enabled         bool   `json:"enabled"`
	RolloutPercent  int    `json:"rolloutPercent"` // 0-100; used only when Enabled and < 100
	Description     string `json:"description,omitempty"`
}

// Store holds feature flags. It is safe for concurrent use.
type Store struct {
	mu    sync.RWMutex
	flags map[string]Flag
}

// New returns an empty store.
func New() *Store {
	return &Store{flags: map[string]Flag{}}
}

// Set inserts or replaces a flag, clamping the rollout percent to [0,100].
func (s *Store) Set(flag Flag) {
	if flag.RolloutPercent < 0 {
		flag.RolloutPercent = 0
	}
	if flag.RolloutPercent > 100 {
		flag.RolloutPercent = 100
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flags[flag.Key] = flag
}

// IsEnabled reports whether a flag is globally on. A flag with a partial
// rollout percent is not considered globally enabled.
func (s *Store) IsEnabled(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, ok := s.flags[key]
	return ok && f.Enabled && f.RolloutPercent >= 100
}

// IsEnabledFor reports whether a flag is on for a specific subject (e.g. a user
// or workspace id), honoring percentage rollout deterministically.
func (s *Store) IsEnabledFor(key, subject string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, ok := s.flags[key]
	if !ok || !f.Enabled {
		return false
	}
	if f.RolloutPercent >= 100 {
		return true
	}
	if f.RolloutPercent <= 0 {
		return false
	}
	return bucket(key, subject) < f.RolloutPercent
}

// List returns all flags sorted by key for stable output.
func (s *Store) List() []Flag {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Flag, 0, len(s.flags))
	for _, f := range s.flags {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// bucket maps (key, subject) to a stable value in [0,100).
func bucket(key, subject string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key + ":" + subject))
	return int(h.Sum32() % 100)
}
