package main

import (
	"fmt"
	"time"

	identityapp "github.com/CarlosSV923/VideoExplorer/apps/api/internal/identity/application"
	identityinfra "github.com/CarlosSV923/VideoExplorer/apps/api/internal/identity/infrastructure"
	identityhttp "github.com/CarlosSV923/VideoExplorer/apps/api/internal/identity/interfaces/http"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/config"
)

// Login throttle (RF-60): 5 attempts at once, then one more every 12 seconds (5/min).
const (
	loginBurst  = 5
	loginRefill = 12 * time.Second
)

// newIdentity wires the single-password login: the verifier of APP_PASSWORD_HASH
// (or APP_PASSWORD), the signed session tokens and the login throttle.
func newIdentity(cfg config.Config) (*identityhttp.Handler, error) {
	verifier, err := passwordVerifier(cfg)
	if err != nil {
		return nil, err
	}
	svc := identityapp.NewService(
		verifier,
		identityinfra.NewHMACTokenCodec([]byte(cfg.SessionSecret)),
		identityinfra.NewMemoryThrottle(loginRefill, loginBurst, nil),
		cfg.SessionTTL,
		nil,
	)
	return identityhttp.NewHandler(svc, cfg.CookieSecure), nil
}

func passwordVerifier(cfg config.Config) (identityapp.PasswordVerifier, error) {
	if cfg.PasswordHash != "" {
		v, err := identityinfra.NewArgon2idVerifier(cfg.PasswordHash)
		if err != nil {
			return nil, fmt.Errorf("APP_PASSWORD_HASH: %w", err)
		}
		return v, nil
	}
	return identityinfra.NewArgon2idVerifierFromPlain(cfg.Password)
}
