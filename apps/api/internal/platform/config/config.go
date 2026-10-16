// Package config loads and validates the process configuration from
// environment variables. Nothing else in the codebase reads os.Getenv.
//
// The variables, their defaults and their rules are listed in docs/spec.md §7
// ("Valores por defecto") and in .env.example.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/caarlos0/env/v11"
)

// MinSessionSecretBytes is the minimum length of SESSION_SECRET.
const MinSessionSecretBytes = 32

// reservedFolder is the unassigned area of the library (spec §5); a library
// root cannot take its name.
const reservedFolder = "_unassigned"

// Config is the validated process configuration.
type Config struct {
	Port     int        `env:"PORT" envDefault:"8080"`
	LogLevel slog.Level `env:"LOG_LEVEL" envDefault:"info"`

	// LibraryPath is the library root: the SMB dataset in production (/library),
	// a local folder in development.
	LibraryPath string `env:"LIBRARY_PATH,required"`
	// DataPath holds the SQLite database, the image cache, staging, trash and the journal.
	DataPath string `env:"DATA_PATH,required"`
	// MoviesDir and SeriesDir are the names of the two library roots inside LibraryPath.
	MoviesDir string `env:"MOVIES_DIR" envDefault:"Movies"`
	SeriesDir string `env:"SERIES_DIR" envDefault:"TV Shows"`

	// PasswordHash is the argon2id PHC string of the shared password (recommended).
	PasswordHash string `env:"APP_PASSWORD_HASH"`
	// Password is a plain-text alternative to PasswordHash, hashed in memory at startup.
	Password string `env:"APP_PASSWORD"`

	SessionSecret string        `env:"SESSION_SECRET,required"`
	SessionTTL    time.Duration `env:"SESSION_TTL" envDefault:"720h"`
	// CookieSecure must be true only when the app is served over HTTPS.
	CookieSecure bool `env:"COOKIE_SECURE" envDefault:"false"`

	// TMDB credentials are optional: the v3 API key or the v4 read token, not both.
	// Without them the app works with free names (spec §6).
	TMDBAPIKey    string `env:"TMDB_API_KEY"`
	TMDBReadToken string `env:"TMDB_READ_TOKEN"`
	// TMDBLanguage is the language of TMDB data; English is the fallback.
	TMDBLanguage string `env:"TMDB_LANGUAGE" envDefault:"es-MX"`

	// Jellyfin is optional: both the URL and the API key, or neither (RF-50).
	JellyfinURL         string `env:"JELLYFIN_URL"`
	JellyfinAPIKey      string `env:"JELLYFIN_API_KEY"`
	JellyfinLibraryPath string `env:"JELLYFIN_LIBRARY_PATH"`

	// TrashRetentionDays is how long trashed items are kept before they are purged (RF-30).
	TrashRetentionDays int `env:"TRASH_RETENTION_DAYS" envDefault:"30"`
	// ExtractConcurrency is how many archives are extracted at once. One is right
	// for spinning disks: parallel extractions only make them seek.
	ExtractConcurrency int `env:"EXTRACT_CONCURRENCY" envDefault:"1"`
	// UnassignedScanInterval and LibraryScanInterval set how often the scans run (RF-28).
	UnassignedScanInterval time.Duration `env:"UNASSIGNED_SCAN_INTERVAL" envDefault:"5m"`
	LibraryScanInterval    time.Duration `env:"LIBRARY_SCAN_INTERVAL" envDefault:"1h"`

	// VideoExtensions and SubtitleExtensions are normalized to lowercase with a
	// leading dot (RF-07).
	VideoExtensions    []string `env:"VIDEO_EXTENSIONS" envSeparator:"," envDefault:".mkv,.mp4,.avi,.m4v,.ts,.m2ts,.mov,.wmv,.webm,.mpg,.mpeg"`
	SubtitleExtensions []string `env:"SUBTITLE_EXTENSIONS" envSeparator:"," envDefault:".srt,.ass,.ssa,.sub,.idx,.vtt"`
}

// Load reads the configuration from environ and validates it. A nil environ
// means the process environment. A blank variable counts as unset: its default
// applies, or the required check fails.
func Load(environ map[string]string) (Config, error) {
	if environ == nil {
		environ = env.ToMap(os.Environ())
	}
	set := make(map[string]string, len(environ))
	for key, value := range environ {
		if strings.TrimSpace(value) != "" {
			set[key] = value
		}
	}

	var cfg Config
	if err := env.ParseWithOptions(&cfg, env.Options{Environment: set}); err != nil {
		return Config{}, fmt.Errorf("config: %w", explainParseError(err))
	}

	var err error
	if cfg.VideoExtensions, err = normalizeExtensions("VIDEO_EXTENSIONS", cfg.VideoExtensions); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	if cfg.SubtitleExtensions, err = normalizeExtensions("SUBTITLE_EXTENSIONS", cfg.SubtitleExtensions); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	return cfg, nil
}

// explainParseError rewrites the parser's errors, which name the Go field, so
// they name the environment variable instead (PORT rather than Port).
func explainParseError(err error) error {
	var agg env.AggregateError
	if errors.As(err, &agg) {
		errs := make([]error, 0, len(agg.Errors))
		for _, e := range agg.Errors {
			errs = append(errs, explainParseError(e))
		}
		return errors.Join(errs...)
	}
	var pe env.ParseError
	if errors.As(err, &pe) {
		return fmt.Errorf("%s: %w", envName(pe.Name), pe.Err)
	}
	return err
}

// envName returns the environment variable of a Config field.
func envName(field string) string {
	sf, ok := reflect.TypeFor[Config]().FieldByName(field)
	if !ok {
		return field
	}
	name, _, _ := strings.Cut(sf.Tag.Get("env"), ",")
	return name
}

// validate checks the rules that cannot be expressed as a parsing error.
func (c Config) validate() error {
	var errs []error
	switch {
	case c.PasswordHash == "" && c.Password == "":
		errs = append(errs, errors.New("set APP_PASSWORD_HASH (recommended) or APP_PASSWORD"))
	case c.PasswordHash != "" && c.Password != "":
		errs = append(errs, errors.New("set only one of APP_PASSWORD_HASH and APP_PASSWORD"))
	}
	if len(c.SessionSecret) < MinSessionSecretBytes {
		errs = append(errs, fmt.Errorf("SESSION_SECRET must be at least %d bytes", MinSessionSecretBytes))
	}
	if c.SessionTTL <= 0 {
		errs = append(errs, errors.New("SESSION_TTL must be positive"))
	}
	if c.Port < 1 || c.Port > 65535 {
		errs = append(errs, fmt.Errorf("PORT %d is out of range", c.Port))
	}
	if c.ExtractConcurrency < 1 || c.ExtractConcurrency > 8 {
		errs = append(errs, fmt.Errorf("EXTRACT_CONCURRENCY must be between 1 and 8, got %d", c.ExtractConcurrency))
	}
	if c.TrashRetentionDays < 1 || c.TrashRetentionDays > 3650 {
		errs = append(errs, fmt.Errorf("TRASH_RETENTION_DAYS must be between 1 and 3650, got %d", c.TrashRetentionDays))
	}
	if c.UnassignedScanInterval < time.Minute {
		errs = append(errs, fmt.Errorf("UNASSIGNED_SCAN_INTERVAL must be at least 1m, got %s", c.UnassignedScanInterval))
	}
	if c.LibraryScanInterval < time.Minute {
		errs = append(errs, fmt.Errorf("LIBRARY_SCAN_INTERVAL must be at least 1m, got %s", c.LibraryScanInterval))
	}
	if c.TMDBAPIKey != "" && c.TMDBReadToken != "" {
		errs = append(errs, errors.New("set only one of TMDB_API_KEY and TMDB_READ_TOKEN, or neither"))
	}
	if (c.JellyfinURL == "") != (c.JellyfinAPIKey == "") {
		errs = append(errs, errors.New("set both JELLYFIN_URL and JELLYFIN_API_KEY, or neither"))
	}
	if c.JellyfinURL != "" && !validHTTPURL(c.JellyfinURL) {
		errs = append(errs, fmt.Errorf("JELLYFIN_URL %q must be an http or https URL", c.JellyfinURL))
	}
	if c.JellyfinLibraryPath != "" && !path.IsAbs(c.JellyfinLibraryPath) {
		errs = append(errs, fmt.Errorf("JELLYFIN_LIBRARY_PATH %q must be an absolute path", c.JellyfinLibraryPath))
	}
	errs = append(errs, validateFolderName("MOVIES_DIR", c.MoviesDir), validateFolderName("SERIES_DIR", c.SeriesDir))
	if c.MoviesDir == c.SeriesDir {
		errs = append(errs, errors.New("MOVIES_DIR and SERIES_DIR must be different"))
	}
	for _, ext := range c.VideoExtensions {
		if slices.Contains(c.SubtitleExtensions, ext) {
			errs = append(errs, fmt.Errorf("%s is listed in both VIDEO_EXTENSIONS and SUBTITLE_EXTENSIONS", ext))
		}
	}
	return errors.Join(errs...)
}

// normalizeExtensions lowercases each extension, drops blank items and removes
// duplicates, keeping the order. Each item must look like .mkv.
func normalizeExtensions(name string, items []string) ([]string, error) {
	out := make([]string, 0, len(items))
	for _, item := range items {
		ext := strings.ToLower(strings.TrimSpace(item))
		if ext == "" {
			continue
		}
		if ext == "." || strings.Contains(ext, "..") || strings.ContainsAny(ext, " \t/\\") {
			return nil, fmt.Errorf("%s: %q is not an extension; use the form .mkv", name, item)
		}
		if !strings.HasPrefix(ext, ".") {
			return nil, fmt.Errorf("%s: %q must start with a dot, for example .mkv", name, item)
		}
		if !slices.Contains(out, ext) {
			out = append(out, ext)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s must list at least one extension", name)
	}
	return out, nil
}

// validateFolderName accepts one plain folder name for a library root: it must
// not be empty, start with a dot, contain separators or control characters,
// or be the reserved unassigned folder.
func validateFolderName(name, value string) error {
	if strings.HasPrefix(value, ".") || value == reservedFolder ||
		strings.ContainsAny(value, `/\`) || strings.ContainsFunc(value, unicode.IsControl) {
		return fmt.Errorf("%s %q must be a single folder name that does not start with a dot and is not %s", name, value, reservedFolder)
	}
	return nil
}

func validHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
