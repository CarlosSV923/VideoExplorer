package config_test

import (
	"log/slog"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/config"
)

const (
	defaultVideoExts    = ".mkv,.mp4,.avi,.m4v,.ts,.m2ts,.mov,.wmv,.webm,.mpg,.mpeg"
	defaultSubtitleExts = ".srt,.ass,.ssa,.sub,.idx,.vtt"
)

// validEnv is the smallest environment that starts the app.
func validEnv() map[string]string {
	return map[string]string{
		"LIBRARY_PATH":   "/library",
		"DATA_PATH":      "/data",
		"APP_PASSWORD":   "secret",
		"SESSION_SECRET": strings.Repeat("x", config.MinSessionSecretBytes),
	}
}

func splitList(s string) []string { return strings.Split(s, ",") }

func TestLoadAppliesDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(validEnv())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"Port", cfg.Port, 8080},
		{"LogLevel", cfg.LogLevel, slog.LevelInfo},
		{"LibraryPath", cfg.LibraryPath, "/library"},
		{"DataPath", cfg.DataPath, "/data"},
		{"MoviesDir", cfg.MoviesDir, "Movies"},
		{"SeriesDir", cfg.SeriesDir, "TV Shows"},
		{"SessionTTL", cfg.SessionTTL, 720 * time.Hour},
		{"CookieSecure", cfg.CookieSecure, false},
		{"TMDBLanguage", cfg.TMDBLanguage, "es-MX"},
		{"JellyfinLibraryPath", cfg.JellyfinLibraryPath, ""},
		{"TrashRetentionDays", cfg.TrashRetentionDays, 30},
		{"ExtractConcurrency", cfg.ExtractConcurrency, 1},
		{"UnassignedScanInterval", cfg.UnassignedScanInterval, 5 * time.Minute},
		{"LibraryScanInterval", cfg.LibraryScanInterval, time.Hour},
		{"VideoExtensions", cfg.VideoExtensions, splitList(defaultVideoExts)},
		{"SubtitleExtensions", cfg.SubtitleExtensions, splitList(defaultSubtitleExts)},
	}
	for _, c := range checks {
		if !equal(c.got, c.want) {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

// equal compares the values the table uses: scalars with ==, slices element by element.
func equal(got, want any) bool {
	if g, ok := got.([]string); ok {
		w, _ := want.([]string)
		return slices.Equal(g, w)
	}
	return got == want
}

func TestLoadReadsOverrides(t *testing.T) {
	t.Parallel()

	env := validEnv()
	env["PORT"] = "9000"
	env["LOG_LEVEL"] = "WARN"
	env["MOVIES_DIR"] = "Peliculas"
	env["SERIES_DIR"] = "Series"
	env["SESSION_TTL"] = "24h"
	env["COOKIE_SECURE"] = "true"
	env["TMDB_READ_TOKEN"] = "token"
	env["TMDB_LANGUAGE"] = "en-US"
	env["JELLYFIN_URL"] = "http://jellyfin:8096"
	env["JELLYFIN_API_KEY"] = "key"
	env["JELLYFIN_LIBRARY_PATH"] = "/mnt/media"
	env["TRASH_RETENTION_DAYS"] = "7"
	env["EXTRACT_CONCURRENCY"] = "2"
	env["UNASSIGNED_SCAN_INTERVAL"] = "2m"
	env["LIBRARY_SCAN_INTERVAL"] = "30m"
	env["VIDEO_EXTENSIONS"] = ".mkv,.mp4"
	env["SUBTITLE_EXTENSIONS"] = ".srt"

	cfg, err := config.Load(env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 9000 || cfg.LogLevel != slog.LevelWarn || cfg.MoviesDir != "Peliculas" || cfg.SeriesDir != "Series" {
		t.Errorf("port, level or folders not read: %+v", cfg)
	}
	if cfg.SessionTTL != 24*time.Hour || !cfg.CookieSecure || cfg.TMDBReadToken != "token" || cfg.TMDBLanguage != "en-US" {
		t.Errorf("session or TMDB values not read: %+v", cfg)
	}
	if cfg.JellyfinURL != "http://jellyfin:8096" || cfg.JellyfinAPIKey != "key" || cfg.JellyfinLibraryPath != "/mnt/media" {
		t.Errorf("Jellyfin values not read: %+v", cfg)
	}
	if cfg.TrashRetentionDays != 7 || cfg.ExtractConcurrency != 2 || cfg.UnassignedScanInterval != 2*time.Minute || cfg.LibraryScanInterval != 30*time.Minute {
		t.Errorf("processing values not read: %+v", cfg)
	}
	if !equal(cfg.VideoExtensions, []string{".mkv", ".mp4"}) || !equal(cfg.SubtitleExtensions, []string{".srt"}) {
		t.Errorf("extensions = %v / %v", cfg.VideoExtensions, cfg.SubtitleExtensions)
	}
}

func TestLoadAcceptsOptionalCombinations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(map[string]string)
	}{
		{"password hash only", func(e map[string]string) {
			delete(e, "APP_PASSWORD")
			e["APP_PASSWORD_HASH"] = "$argon2id$v=19$m=65536,t=3,p=4$salt$hash"
		}},
		{"TMDB v3 key only", func(e map[string]string) { e["TMDB_API_KEY"] = "v3" }},
		{"TMDB v4 token only", func(e map[string]string) { e["TMDB_READ_TOKEN"] = "v4" }},
		{"Jellyfin both values", func(e map[string]string) {
			e["JELLYFIN_URL"] = "https://jellyfin.local"
			e["JELLYFIN_API_KEY"] = "key"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := validEnv()
			tt.mutate(env)

			if _, err := config.Load(env); err != nil {
				t.Fatalf("Load: unexpected error: %v", err)
			}
		})
	}
}

func TestLoadTreatsBlankAsUnset(t *testing.T) {
	t.Parallel()

	env := validEnv()
	env["PORT"] = "  "
	env["TMDB_LANGUAGE"] = ""
	env["VIDEO_EXTENSIONS"] = " "
	env["APP_PASSWORD_HASH"] = ""
	env["TMDB_API_KEY"] = "   "
	env["TMDB_READ_TOKEN"] = "v4"

	cfg, err := config.Load(env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 8080 || cfg.TMDBLanguage != "es-MX" {
		t.Errorf("blank values should keep defaults: port %d, language %q", cfg.Port, cfg.TMDBLanguage)
	}
	if !equal(cfg.VideoExtensions, splitList(defaultVideoExts)) {
		t.Errorf("blank VIDEO_EXTENSIONS should keep the defaults, got %v", cfg.VideoExtensions)
	}
	if cfg.TMDBAPIKey != "" {
		t.Errorf("blank TMDB_API_KEY should be unset, got %q", cfg.TMDBAPIKey)
	}
}

func TestLoadNormalizesExtensions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		want    []string
		wantErr string
	}{
		{name: "lowercased and trimmed", value: " .MKV , .Mp4 ", want: []string{".mkv", ".mp4"}},
		{name: "duplicates removed in order", value: ".mkv,.mp4,.MKV", want: []string{".mkv", ".mp4"}},
		{name: "empty items ignored", value: ".mkv,,.mp4,", want: []string{".mkv", ".mp4"}},
		{name: "missing dot", value: "mkv", wantErr: "must start with a dot"},
		{name: "lone dot", value: ".", wantErr: "is not an extension"},
		{name: "double dot", value: "..mkv", wantErr: "is not an extension"},
		{name: "inner space", value: ".m k v", wantErr: "is not an extension"},
		{name: "separator in name", value: ".mk/v", wantErr: "is not an extension"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := validEnv()
			env["VIDEO_EXTENSIONS"] = tt.value

			cfg, err := config.Load(env)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load() error = %v, want it to mention %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if !equal(cfg.VideoExtensions, tt.want) {
				t.Errorf("VideoExtensions = %v, want %v", cfg.VideoExtensions, tt.want)
			}
		})
	}
}

func TestLoadRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(map[string]string)
		wantErr string
	}{
		{"missing library", func(e map[string]string) { delete(e, "LIBRARY_PATH") }, "LIBRARY_PATH"},
		{"blank library", func(e map[string]string) { e["LIBRARY_PATH"] = " " }, "LIBRARY_PATH"},
		{"missing data", func(e map[string]string) { delete(e, "DATA_PATH") }, "DATA_PATH"},
		{"missing session secret", func(e map[string]string) { delete(e, "SESSION_SECRET") }, "SESSION_SECRET"},
		{"no password", func(e map[string]string) { delete(e, "APP_PASSWORD") }, "APP_PASSWORD_HASH (recommended) or APP_PASSWORD"},
		{"both passwords", func(e map[string]string) { e["APP_PASSWORD_HASH"] = "$argon2id$..." }, "only one of APP_PASSWORD_HASH"},
		{"short secret", func(e map[string]string) { e["SESSION_SECRET"] = "short" }, "SESSION_SECRET must be at least 32 bytes"},
		{"zero ttl", func(e map[string]string) { e["SESSION_TTL"] = "0s" }, "SESSION_TTL must be positive"},
		{"bad ttl", func(e map[string]string) { e["SESSION_TTL"] = "720" }, "SESSION_TTL"},
		{"bad cookie flag", func(e map[string]string) { e["COOKIE_SECURE"] = "yes" }, "COOKIE_SECURE"},
		{"bad port", func(e map[string]string) { e["PORT"] = "70000" }, "PORT"},
		{"port is not a number", func(e map[string]string) { e["PORT"] = "abc" }, "PORT"},
		{"unknown log level", func(e map[string]string) { e["LOG_LEVEL"] = "loud" }, `"loud"`},
		{"zero extract concurrency", func(e map[string]string) { e["EXTRACT_CONCURRENCY"] = "0" }, "EXTRACT_CONCURRENCY"},
		{"too many extractions", func(e map[string]string) { e["EXTRACT_CONCURRENCY"] = "9" }, "EXTRACT_CONCURRENCY"},
		{"zero trash retention", func(e map[string]string) { e["TRASH_RETENTION_DAYS"] = "0" }, "TRASH_RETENTION_DAYS"},
		{"unassigned scan too frequent", func(e map[string]string) { e["UNASSIGNED_SCAN_INTERVAL"] = "30s" }, "UNASSIGNED_SCAN_INTERVAL must be at least 1m"},
		{"library scan too frequent", func(e map[string]string) { e["LIBRARY_SCAN_INTERVAL"] = "59s" }, "LIBRARY_SCAN_INTERVAL must be at least 1m"},
		{"both TMDB credentials", func(e map[string]string) {
			e["TMDB_API_KEY"] = "v3"
			e["TMDB_READ_TOKEN"] = "v4"
		}, "only one of TMDB_API_KEY and TMDB_READ_TOKEN"},
		{"Jellyfin URL without key", func(e map[string]string) { e["JELLYFIN_URL"] = "http://jellyfin:8096" }, "JELLYFIN_URL and JELLYFIN_API_KEY"},
		{"Jellyfin key without URL", func(e map[string]string) { e["JELLYFIN_API_KEY"] = "key" }, "JELLYFIN_URL and JELLYFIN_API_KEY"},
		{"Jellyfin URL not http", func(e map[string]string) {
			e["JELLYFIN_URL"] = "ftp://jellyfin"
			e["JELLYFIN_API_KEY"] = "key"
		}, "must be an http or https URL"},
		{"Jellyfin library path relative", func(e map[string]string) {
			e["JELLYFIN_LIBRARY_PATH"] = "media"
		}, "JELLYFIN_LIBRARY_PATH"},
		{"movies dir with separator", func(e map[string]string) { e["MOVIES_DIR"] = "Movies/HD" }, "MOVIES_DIR"},
		{"series dir hidden", func(e map[string]string) { e["SERIES_DIR"] = ".videoexplorer" }, "SERIES_DIR"},
		{"movies dir reserved", func(e map[string]string) { e["MOVIES_DIR"] = "_unassigned" }, "MOVIES_DIR"},
		{"same folder for both", func(e map[string]string) {
			e["MOVIES_DIR"] = "Media"
			e["SERIES_DIR"] = "Media"
		}, "must be different"},
		{"extension in both lists", func(e map[string]string) {
			e["VIDEO_EXTENSIONS"] = ".mkv,.srt"
			e["SUBTITLE_EXTENSIONS"] = ".srt"
		}, "both VIDEO_EXTENSIONS and SUBTITLE_EXTENSIONS"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := validEnv()
			tt.mutate(env)

			_, err := config.Load(env)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadDoesNotModifyInput(t *testing.T) {
	t.Parallel()

	env := validEnv()
	env["PORT"] = ""
	before := maps.Clone(env)

	if _, err := config.Load(env); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !maps.Equal(env, before) {
		t.Errorf("Load changed its input: %v", env)
	}
}
