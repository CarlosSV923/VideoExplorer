package config_test

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/config"
)

func TestLoad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		env       map[string]string
		wantPort  int
		wantLevel slog.Level
		wantErr   string
	}{
		{name: "defaults when nothing is set", env: nil, wantPort: 8080, wantLevel: slog.LevelInfo},
		{
			name:      "custom port and level",
			env:       map[string]string{"PORT": "9000", "LOG_LEVEL": "debug"},
			wantPort:  9000,
			wantLevel: slog.LevelDebug,
		},
		{
			name:      "level is case insensitive",
			env:       map[string]string{"LOG_LEVEL": "WARN"},
			wantPort:  8080,
			wantLevel: slog.LevelWarn,
		},
		{
			name:      "blank values keep the defaults",
			env:       map[string]string{"PORT": "  ", "LOG_LEVEL": ""},
			wantPort:  8080,
			wantLevel: slog.LevelInfo,
		},
		{name: "port is not a number", env: map[string]string{"PORT": "abc"}, wantErr: "PORT"},
		{name: "port zero", env: map[string]string{"PORT": "0"}, wantErr: "PORT"},
		{name: "port too large", env: map[string]string{"PORT": "70000"}, wantErr: "PORT"},
		{name: "unknown level", env: map[string]string{"LOG_LEVEL": "verbose"}, wantErr: "LOG_LEVEL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := config.Load(func(key string) string { return tt.env[key] })

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load() error = %v, want an error mentioning %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}
			if cfg.Port != tt.wantPort {
				t.Errorf("Port = %d, want %d", cfg.Port, tt.wantPort)
			}
			if cfg.LogLevel != tt.wantLevel {
				t.Errorf("LogLevel = %v, want %v", cfg.LogLevel, tt.wantLevel)
			}
		})
	}
}
