package main

import (
	"strings"
	"testing"

	identityinfra "github.com/CarlosSV923/VideoExplorer/apps/api/internal/identity/infrastructure"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/config"
)

func TestPasswordVerifier(t *testing.T) {
	t.Parallel()

	hash, err := identityinfra.HashPassword("from-hash")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		cfg     config.Config
		wantErr string
		accepts string
		rejects string
	}{
		{
			name:    "argon2id hash",
			cfg:     config.Config{PasswordHash: hash},
			accepts: "from-hash",
			rejects: "from-plain",
		},
		{
			name:    "plain password is hashed in memory",
			cfg:     config.Config{Password: "from-plain"},
			accepts: "from-plain",
			rejects: "from-hash",
		},
		{
			name:    "malformed hash names the variable",
			cfg:     config.Config{PasswordHash: "not-a-hash"},
			wantErr: "APP_PASSWORD_HASH",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v, err := passwordVerifier(tt.cfg)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want one that mentions %s", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !v.Verify(tt.accepts) || v.Verify(tt.rejects) {
				t.Fatalf("Verify(%q) and Verify(%q) must be true and false", tt.accepts, tt.rejects)
			}
		})
	}
}
