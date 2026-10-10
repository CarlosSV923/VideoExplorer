// Package config loads the process configuration from environment variables.
//
// Phase 0 reads only what the skeleton needs (PORT and LOG_LEVEL). The full
// set (library, data, passwords, TMDB…) arrives with phase 1.
package config

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
)

// DefaultPort is the port the API listens on when PORT is not set. It is the
// same default as GameExplorer, so the dev proxy and the docs match.
const DefaultPort = 8080

// Config is the validated process configuration.
type Config struct {
	Port     int
	LogLevel slog.Level
}

// Load reads the configuration through getenv (os.Getenv in production) and
// validates it. An unset or blank variable keeps its default.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{Port: DefaultPort, LogLevel: slog.LevelInfo}

	if raw := strings.TrimSpace(getenv("PORT")); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil || port < 1 || port > 65535 {
			return Config{}, fmt.Errorf("config: PORT %q must be a port between 1 and 65535", raw)
		}
		cfg.Port = port
	}

	if raw := strings.TrimSpace(getenv("LOG_LEVEL")); raw != "" {
		if err := cfg.LogLevel.UnmarshalText([]byte(raw)); err != nil {
			return Config{}, fmt.Errorf("config: LOG_LEVEL %q: use debug, info, warn or error", raw)
		}
	}

	return cfg, nil
}
