// Package jellyfin implements the media-server port using Jellyfin's library
// notification API (RF-50, RF-50a).
package jellyfin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/mediaservers/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/mediaservers/domain"
)

const (
	// DefaultRequestTimeout bounds one HTTP attempt.
	DefaultRequestTimeout = 10 * time.Second
	// DefaultMaxAttempts includes the first HTTP attempt.
	DefaultMaxAttempts = 3
	// DefaultRetryDelay is doubled for each retry.
	DefaultRetryDelay = 500 * time.Millisecond
	maxBodyBytes      = 4 << 20
	redacted          = "[redacted]"
)

// Config contains only adapter configuration. The API key is kept here and is
// sent only as an Authorization token; it is never put in a URL or an error.
type Config struct {
	URL               string
	APIKey            string
	LibraryPath       string
	MoviesDir         string
	SeriesDir         string
	ForcedLibraryPath string
	HTTPClient        *http.Client
	RequestTimeout    time.Duration
	MaxAttempts       int
	RetryDelay        time.Duration
}

// Client is the Jellyfin adapter. Its detected roots are replaced atomically
// after each successful discovery and read while sending a notification.
type Client struct {
	cfg        Config
	http       *http.Client
	baseURL    string
	configured bool

	mu          sync.RWMutex
	translation domain.Translation
}

var _ application.Backend = (*Client)(nil)

// New builds a Jellyfin adapter. Empty URL and key are the valid disabled
// configuration used when Jellyfin is not configured.
func New(cfg Config) (*Client, error) {
	if (cfg.URL == "") != (cfg.APIKey == "") {
		return nil, errors.New("jellyfin: set both URL and API key, or neither")
	}
	if cfg.URL != "" {
		u, err := url.Parse(cfg.URL)
		if err != nil || u.Scheme == "" || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, errors.New("jellyfin: URL must be an http or https URL")
		}
	}
	if cfg.ForcedLibraryPath != "" && !path.IsAbs(cfg.ForcedLibraryPath) {
		return nil, errors.New("jellyfin: forced library path must be absolute")
	}
	if cfg.MoviesDir == "" || cfg.SeriesDir == "" || cfg.MoviesDir == cfg.SeriesDir {
		return nil, errors.New("jellyfin: movie and series directories must be distinct")
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = DefaultRequestTimeout
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = DefaultMaxAttempts
	}
	if cfg.RetryDelay <= 0 {
		cfg.RetryDelay = DefaultRetryDelay
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{}
	}
	return &Client{
		cfg:        cfg,
		http:       hc,
		baseURL:    strings.TrimRight(cfg.URL, "/"),
		configured: cfg.URL != "",
		translation: domain.Translation{
			Forced: cfg.ForcedLibraryPath != "",
		},
	}, nil
}

// Configured reports whether both Jellyfin credentials are present.
func (c *Client) Configured() bool { return c.configured }

// Discover queries Jellyfin's virtual folders unless the explicit library
// path was configured. Matching roots are those whose path ends in the
// configured MoviesDir or SeriesDir.
func (c *Client) Discover(ctx context.Context) (domain.Connection, error) {
	if !c.configured {
		return domain.Connection{}, nil
	}
	if c.cfg.ForcedLibraryPath != "" {
		root := cleanRemotePath(c.cfg.ForcedLibraryPath)
		translation := domain.Translation{
			Forced:      true,
			Source:      domain.TranslationConfigured,
			LibraryRoot: root,
			MoviesRoot:  path.Join(root, c.cfg.MoviesDir),
			SeriesRoot:  path.Join(root, c.cfg.SeriesDir),
		}
		c.setTranslation(translation)
		return domain.Connection{Translation: translation}, nil
	}

	var folders []virtualFolder
	if err := c.request(ctx, http.MethodGet, "/Library/VirtualFolders", nil, &folders); err != nil {
		return domain.Connection{}, err
	}
	translation := domain.Translation{Source: domain.TranslationNone}
	for _, folder := range folders {
		for _, location := range folder.Locations {
			if translation.MoviesRoot == "" && hasRootSuffix(location, c.cfg.MoviesDir) {
				translation.MoviesRoot = cleanRemotePath(location)
			}
			if translation.SeriesRoot == "" && hasRootSuffix(location, c.cfg.SeriesDir) {
				translation.SeriesRoot = cleanRemotePath(location)
			}
		}
	}
	if translation.MoviesRoot != "" || translation.SeriesRoot != "" {
		translation.Source = domain.TranslationDetected
	}
	translation.LibraryRoot = commonRoot(translation.MoviesRoot, translation.SeriesRoot)
	c.setTranslation(translation)
	return domain.Connection{Translation: translation}, nil
}

// Refresh sends one request for all translatable folders. If any folder has
// no detected root, the complete library refresh is the safe Jellyfin action.
func (c *Client) Refresh(ctx context.Context, folders []string) error {
	if !c.configured || len(folders) == 0 {
		return nil
	}
	c.mu.RLock()
	translation := c.translation
	c.mu.RUnlock()
	paths := make([]string, 0, len(folders))
	for _, folder := range folders {
		translated, ok := c.translate(translation, folder)
		if !ok {
			return c.request(ctx, http.MethodPost, "/Library/Refresh", nil, nil)
		}
		paths = append(paths, translated)
	}
	updates := make([]mediaUpdate, 0, len(paths))
	for _, p := range paths {
		updates = append(updates, mediaUpdate{Path: p, UpdateType: "Modified"})
	}
	return c.request(ctx, http.MethodPost, "/Library/Media/Updated", map[string]any{"Updates": updates}, nil)
}

type virtualFolder struct {
	Locations []string `json:"Locations"`
}

type mediaUpdate struct {
	Path       string `json:"Path"`
	UpdateType string `json:"UpdateType"`
}

func (c *Client) translate(t domain.Translation, folder string) (string, bool) {
	rel := strings.Trim(strings.ReplaceAll(folder, "\\", "/"), "/")
	if rel == "" || rel == "." || strings.HasPrefix(rel, "../") || rel == ".." {
		return "", false
	}
	if t.Forced {
		return path.Join(cleanRemotePath(c.cfg.ForcedLibraryPath), rel), true
	}
	parts := strings.Split(rel, "/")
	if len(parts) >= 2 && equalPathPart(parts[0], c.cfg.MoviesDir) && t.MoviesRoot != "" {
		return path.Join(t.MoviesRoot, strings.Join(parts[1:], "/")), true
	}
	if len(parts) >= 2 && equalPathPart(parts[0], c.cfg.SeriesDir) && t.SeriesRoot != "" {
		return path.Join(t.SeriesRoot, strings.Join(parts[1:], "/")), true
	}
	return "", false
}

func (c *Client) setTranslation(translation domain.Translation) {
	c.mu.Lock()
	c.translation = translation
	c.mu.Unlock()
}

func hasRootSuffix(raw, root string) bool {
	clean := cleanRemotePath(raw)
	return equalPathPart(path.Base(clean), root)
}

func cleanRemotePath(raw string) string {
	clean := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if clean == "" {
		return ""
	}
	clean = path.Clean(clean)
	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(clean, "/") {
		return "/" + clean
	}
	return clean
}

func equalPathPart(a, b string) bool { return strings.EqualFold(a, b) }

func commonRoot(first, second string) string {
	if first == "" && second == "" {
		return ""
	}
	if first == "" {
		return path.Dir(second)
	}
	if second == "" {
		return path.Dir(first)
	}
	firstParts := strings.Split(strings.Trim(path.Clean(first), "/"), "/")
	secondParts := strings.Split(strings.Trim(path.Clean(second), "/"), "/")
	length := len(firstParts)
	if len(secondParts) < length {
		length = len(secondParts)
	}
	common := make([]string, 0, length)
	for i := 0; i < length && equalPathPart(firstParts[i], secondParts[i]); i++ {
		common = append(common, firstParts[i])
	}
	if len(common) == 0 {
		return ""
	}
	root := strings.Join(common, "/")
	if strings.HasPrefix(first, "/") {
		return "/" + root
	}
	return root
}

func (c *Client) request(ctx context.Context, method, endpoint string, body any, out any) error {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return c.upstream(endpoint, "encode request: "+err.Error())
		}
	}

	for attempt := 1; ; attempt++ {
		requestCtx, cancel := context.WithTimeout(ctx, c.cfg.RequestTimeout)
		req, reqErr := http.NewRequestWithContext(requestCtx, method, c.baseURL+endpoint, bytes.NewReader(payload))
		if reqErr != nil {
			cancel()
			return c.upstream(endpoint, "invalid request: "+reqErr.Error())
		}
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if c.cfg.APIKey != "" {
			req.Header.Set("Authorization", `MediaBrowser Token="`+c.cfg.APIKey+`"`)
		}
		res, doErr := c.http.Do(req)
		if doErr != nil {
			cancel()
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if attempt < c.cfg.MaxAttempts {
				if err := sleep(ctx, c.cfg.RetryDelay<<(attempt-1)); err != nil {
					return err
				}
				continue
			}
			return c.upstream(endpoint, "request failed: "+unwrapURL(doErr).Error())
		}

		message := ""
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			message = readMessage(res.Body)
			_ = res.Body.Close()
			cancel()
			retry := res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500
			if retry && attempt < c.cfg.MaxAttempts {
				if err := sleep(ctx, c.cfg.RetryDelay<<(attempt-1)); err != nil {
					return err
				}
				continue
			}
			if message == "" {
				message = "no message"
			}
			return c.upstream(endpoint, "returned "+strconv.Itoa(res.StatusCode)+": "+message)
		}
		if out != nil && res.StatusCode != http.StatusNoContent {
			err = json.NewDecoder(io.LimitReader(res.Body, maxBodyBytes)).Decode(out)
		}
		_ = res.Body.Close()
		cancel()
		if err != nil {
			return c.upstream(endpoint, "malformed response: "+err.Error())
		}
		return nil
	}
}

func (c *Client) upstream(endpoint, message string) error {
	return fmt.Errorf("jellyfin: %s: %s", endpoint, c.redact(message))
}

func (c *Client) redact(message string) string {
	if c.cfg.APIKey != "" {
		message = strings.ReplaceAll(message, c.cfg.APIKey, redacted)
	}
	return message
}

func readMessage(body io.Reader) string {
	b, err := io.ReadAll(io.LimitReader(body, 64<<10))
	if err != nil || len(b) == 0 {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func unwrapURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

func sleep(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
