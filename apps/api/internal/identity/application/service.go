// Package application holds the identity use cases and the ports they need.
package application

import (
	"context"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/identity/domain"
)

// PasswordVerifier checks the shared password.
type PasswordVerifier interface {
	Verify(password string) bool
}

// TokenCodec turns sessions into tamper-proof tokens and back.
type TokenCodec interface {
	Encode(domain.Session) string
	// Decode returns domain.ErrInvalidSession for forged or malformed tokens.
	Decode(token string) (domain.Session, error)
}

// Throttle limits login attempts per client key.
type Throttle interface {
	Allow(key string) bool
}

// Service implements login and session checks.
type Service struct {
	verifier PasswordVerifier
	codec    TokenCodec
	throttle Throttle
	ttl      time.Duration
	now      func() time.Time
}

// NewService builds the service. now may be nil (time.Now).
func NewService(v PasswordVerifier, c TokenCodec, t Throttle, ttl time.Duration, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{verifier: v, codec: c, throttle: t, ttl: ttl, now: now}
}

// Login checks the password for clientKey and returns a session token.
func (s *Service) Login(_ context.Context, clientKey, password string) (string, domain.Session, error) {
	if !s.throttle.Allow(clientKey) {
		return "", domain.Session{}, domain.ErrTooManyAttempts
	}
	if !s.verifier.Verify(password) {
		return "", domain.Session{}, domain.ErrInvalidCredentials
	}
	session := domain.NewSession(s.now(), s.ttl)
	return s.codec.Encode(session), session, nil
}

// Authenticate validates a session token.
func (s *Service) Authenticate(_ context.Context, token string) (domain.Session, error) {
	if token == "" {
		return domain.Session{}, domain.ErrInvalidSession
	}
	session, err := s.codec.Decode(token)
	if err != nil {
		return domain.Session{}, domain.ErrInvalidSession
	}
	if !session.ActiveAt(s.now()) {
		return domain.Session{}, domain.ErrInvalidSession
	}
	return session, nil
}

// TTL is the configured session lifetime.
func (s *Service) TTL() time.Duration { return s.ttl }
