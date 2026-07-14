package ratelimit

import (
	"context"
	"time"
)

// Enforcer is the rate-limiting surface the HTTP middleware depends on. Both the
// in-process limiter and the Redis-backed limiter satisfy it, so the middleware
// is identical regardless of where the counters live.
type Enforcer interface {
	// Enabled reports whether any limit is enforced. A disabled enforcer is a
	// pass-through, preserving the default (unlimited) behaviour.
	Enabled() bool
	// Allow records one request for key and returns the decision. It takes a
	// context so a backing store (Redis) can honour request cancellation and
	// timeouts.
	Allow(ctx context.Context, key string) Decision
}

// Decision is the outcome of a single Allow call.
type Decision struct {
	// Allowed is false when the request exceeds the limit.
	Allowed bool
	// Remaining is how many requests key may still make in the current window.
	// -1 means unbounded (limiter disabled or backing store unavailable).
	Remaining int
	// RetryAfter is how long until the window resets — the Retry-After hint.
	RetryAfter time.Duration
}

// memoryEnforcer adapts the in-process Limiter to the Enforcer interface. It is
// the default when no shared store is configured: correct for a single instance,
// but its counters are per-process and reset on restart.
type memoryEnforcer struct {
	limiter *Limiter
}

// Memory returns an in-process Enforcer allowing limit requests per window.
func Memory(limit int, window time.Duration) Enforcer {
	return memoryEnforcer{limiter: New(limit, window)}
}

func (m memoryEnforcer) Enabled() bool { return m.limiter.Enabled() }

func (m memoryEnforcer) Allow(_ context.Context, key string) Decision {
	now := time.Now()
	allowed := m.limiter.Allow(key, now)
	return Decision{
		Allowed:    allowed,
		Remaining:  m.limiter.Remaining(key, now),
		RetryAfter: m.limiter.window,
	}
}
