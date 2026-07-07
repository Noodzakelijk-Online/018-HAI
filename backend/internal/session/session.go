// Package session models a bearer session with issue/expiry times and pure
// validity checks. Clock-injected so expiry logic is deterministic in tests.
package session

import "time"

// Session is an issued session/token.
type Session struct {
	Token     string    `json:"token"`
	IssuedAt  time.Time `json:"issuedAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// New issues a session valid for ttl from issuedAt.
func New(token string, issuedAt time.Time, ttl time.Duration) Session {
	return Session{Token: token, IssuedAt: issuedAt.UTC(), ExpiresAt: issuedAt.Add(ttl).UTC()}
}

// Valid reports whether the session is non-empty and not expired at now.
func (s Session) Valid(now time.Time) bool {
	if s.Token == "" {
		return false
	}
	return now.Before(s.ExpiresAt)
}

// Remaining returns the time left before expiry, clamped at zero.
func (s Session) Remaining(now time.Time) time.Duration {
	if d := s.ExpiresAt.Sub(now); d > 0 {
		return d
	}
	return 0
}
