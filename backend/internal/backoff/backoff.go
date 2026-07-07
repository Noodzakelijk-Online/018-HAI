// Package backoff computes exponential-backoff retry delays. Pure and
// deterministic (no jitter) so worker retry timing is testable.
package backoff

import (
	"math"
	"time"
)

// Policy configures exponential backoff.
type Policy struct {
	Base   time.Duration // delay for the first retry
	Factor float64       // growth factor per attempt (>= 1)
	Max    time.Duration // cap on any single delay (0 = uncapped)
}

// DefaultPolicy is a sensible default: 1s base, doubling, capped at 5m.
func DefaultPolicy() Policy {
	return Policy{Base: time.Second, Factor: 2, Max: 5 * time.Minute}
}

// Delay returns the backoff delay for a 1-based attempt number. Attempt <= 0
// returns 0 (no wait before the first try).
func (p Policy) Delay(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}
	factor := p.Factor
	if factor < 1 {
		factor = 1
	}
	delay := float64(p.Base) * math.Pow(factor, float64(attempt-1))
	if p.Max > 0 && delay > float64(p.Max) {
		return p.Max
	}
	if delay > math.MaxInt64 {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(delay)
}
