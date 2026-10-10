package application_test

import (
	"errors"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/identity/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/identity/domain"
)

type fakeVerifier string

func (f fakeVerifier) Verify(p string) bool { return p == string(f) }

// fakeCodec stores the expiry in the token itself, so tests can pick the clock.
type fakeCodec struct{}

func (fakeCodec) Encode(s domain.Session) string { return s.ExpiresAt.Format(time.RFC3339) }

func (fakeCodec) Decode(tok string) (domain.Session, error) {
	exp, err := time.Parse(time.RFC3339, tok)
	if err != nil {
		return domain.Session{}, domain.ErrInvalidSession
	}
	return domain.Session{IssuedAt: exp.Add(-time.Hour), ExpiresAt: exp}, nil
}

type fakeThrottle struct{ allow bool }

func (f fakeThrottle) Allow(string) bool { return f.allow }

var clock = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

func newService(now time.Time, allow bool) *application.Service {
	return application.NewService(fakeVerifier("hunter2"), fakeCodec{}, fakeThrottle{allow}, time.Hour,
		func() time.Time { return now })
}

func TestLogin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		allow    bool
		password string
		wantErr  error
	}{
		{name: "right password", allow: true, password: "hunter2"},
		{name: "wrong password", allow: true, password: "wrong", wantErr: domain.ErrInvalidCredentials},
		{name: "throttled even with the right password", allow: false, password: "hunter2", wantErr: domain.ErrTooManyAttempts},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			token, session, err := newService(clock, tt.allow).Login(t.Context(), "ip", tt.password)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if token == "" || !session.ExpiresAt.Equal(clock.Add(time.Hour)) {
				t.Fatalf("token=%q session=%+v", token, session)
			}
		})
	}
}

func TestAuthenticate(t *testing.T) {
	t.Parallel()

	svc := newService(clock, true)
	token, _, err := svc.Login(t.Context(), "ip", "hunter2")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		svc     *application.Service
		token   string
		wantErr bool
	}{
		{name: "valid token", svc: svc, token: token},
		{name: "expired token", svc: newService(clock.Add(2*time.Hour), true), token: token, wantErr: true},
		{name: "empty token", svc: svc, token: "", wantErr: true},
		{name: "garbage token", svc: svc, token: "garbage", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := tt.svc.Authenticate(t.Context(), tt.token)
			if tt.wantErr && !errors.Is(err, domain.ErrInvalidSession) {
				t.Fatalf("err = %v, want ErrInvalidSession", err)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("err = %v", err)
			}
		})
	}
}
