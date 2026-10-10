// Package domain holds the identity model: a single shared password grants a
// time-limited session per device.
package domain

import (
	"errors"
	"time"
)

var (
	// ErrInvalidCredentials means the password did not match.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrTooManyAttempts means the client exceeded the login rate limit.
	ErrTooManyAttempts = errors.New("too many login attempts")
	// ErrInvalidSession means the session token is missing, forged or expired.
	ErrInvalidSession = errors.New("invalid session")
)

// Session is an authenticated period for one device.
type Session struct {
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// NewSession starts a session at now that lasts ttl.
func NewSession(now time.Time, ttl time.Duration) Session {
	now = now.UTC().Truncate(time.Second)
	return Session{IssuedAt: now, ExpiresAt: now.Add(ttl)}
}

// ActiveAt reports whether the session is usable at t.
func (s Session) ActiveAt(t time.Time) bool {
	return !t.Before(s.IssuedAt.Add(-time.Minute)) && t.Before(s.ExpiresAt)
}
