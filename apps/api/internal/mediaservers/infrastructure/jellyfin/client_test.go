package jellyfin_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/mediaservers/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/mediaservers/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/mediaservers/infrastructure/jellyfin"
)

const secret = "jellyfin-secret-123"

func newClient(t *testing.T, url string) *jellyfin.Client {
	t.Helper()
	client, err := jellyfin.New(jellyfin.Config{
		URL:            url,
		APIKey:         secret,
		MoviesDir:      "Movies",
		SeriesDir:      "TV Shows",
		RequestTimeout: time.Second,
		MaxAttempts:    1,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return client
}

func jsonResponse(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}
}

func TestDiscoverPathDetectionTable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		forcedPath string
		want       domain.Translation
		wantGets   int32
	}{
		{
			name:     "finds movie and series roots",
			body:     `[{"Locations":["/srv/media/Movies"]},{"Locations":["/srv/media/TV Shows"]}]`,
			want:     domain.Translation{Source: domain.TranslationDetected, LibraryRoot: "/srv/media", MoviesRoot: "/srv/media/Movies", SeriesRoot: "/srv/media/TV Shows"},
			wantGets: 1,
		},
		{
			name:     "finds no configured roots",
			body:     `[{"Locations":["/srv/music"]}]`,
			want:     domain.Translation{Source: domain.TranslationNone},
			wantGets: 1,
		},
		{
			name:       "forced path skips detection",
			forcedPath: "/srv/media",
			want:       domain.Translation{Forced: true, Source: domain.TranslationConfigured, LibraryRoot: "/srv/media", MoviesRoot: "/srv/media/Movies", SeriesRoot: "/srv/media/TV Shows"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gets atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/Library/VirtualFolders" {
					t.Errorf("path = %q, want virtual folders", r.URL.Path)
					return
				}
				gets.Add(1)
				jsonResponse(tt.body)(w, r)
			}))
			t.Cleanup(srv.Close)
			client, err := jellyfin.New(jellyfin.Config{
				URL:               srv.URL,
				APIKey:            secret,
				MoviesDir:         "Movies",
				SeriesDir:         "TV Shows",
				ForcedLibraryPath: tt.forcedPath,
				MaxAttempts:       1,
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			got, err := client.Discover(t.Context())
			if err != nil {
				t.Fatalf("Discover: %v", err)
			}
			if got.Translation != tt.want {
				t.Errorf("translation = %+v, want %+v", got.Translation, tt.want)
			}
			if got := gets.Load(); got != tt.wantGets {
				t.Errorf("virtual-folder requests = %d, want %d", got, tt.wantGets)
			}
		})
	}
}

func TestDetectsJellyfinRootsAndTranslatesFolders(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotBody map[string][]struct {
		Path       string `json:"Path"`
		UpdateType string `json:"UpdateType"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantAuth := `MediaBrowser Token="` + secret + `"`
		if r.Header.Get("Authorization") != wantAuth {
			t.Errorf("Authorization = %q, want %q", r.Header.Get("Authorization"), wantAuth)
		}
		if got := r.Header.Get("X-Emby-Token"); got != "" {
			t.Errorf("X-Emby-Token = %q, want it absent", got)
		}
		if r.URL.RawQuery != "" {
			t.Errorf("query = %q, want no credentials in the URL", r.URL.RawQuery)
		}
		switch r.URL.Path {
		case "/Library/VirtualFolders":
			jsonResponse(`[{"Name":"Películas","Locations":["/srv/media/Movies"]},{"Name":"Series","Locations":["/srv/media/TV Shows"]}]`)(w, r)
		case "/Library/Media/Updated":
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Errorf("decode update: %v", err)
			}
			gotPath = r.URL.Path
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client := newClient(t, srv.URL)
	if _, err := client.Discover(t.Context()); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if err := client.Refresh(t.Context(), []string{"Movies/Iron Man", "TV Shows/Breaking Bad/Season 01"}); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if gotPath != "/Library/Media/Updated" {
		t.Fatalf("path = %q, want media updated", gotPath)
	}
	updates := gotBody["Updates"]
	if len(updates) != 2 || updates[0].Path != "/srv/media/Movies/Iron Man" || updates[1].Path != "/srv/media/TV Shows/Breaking Bad/Season 01" {
		t.Fatalf("updates = %+v, want translated movie and season paths", updates)
	}
	for _, update := range updates {
		if update.UpdateType != "Modified" {
			t.Errorf("update type = %q, want Modified", update.UpdateType)
		}
	}
}

func TestMissingRootsUsesFullLibraryRefresh(t *testing.T) {
	t.Parallel()

	var refreshes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Library/VirtualFolders":
			jsonResponse(`[{"Locations":["/srv/music"]}]`)(w, r)
		case "/Library/Refresh":
			refreshes.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client := newClient(t, srv.URL)
	if _, err := client.Discover(t.Context()); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if err := client.Refresh(t.Context(), []string{"Movies/Unknown"}); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if got := refreshes.Load(); got != 1 {
		t.Fatalf("full refreshes = %d, want 1", got)
	}
}

func TestForcedLibraryPathSkipsDetection(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/Library/Media/Updated" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		var body struct {
			Updates []struct {
				Path string `json:"Path"`
			} `json:"Updates"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode update: %v", err)
		} else if len(body.Updates) == 1 {
			gotPath = body.Updates[0].Path
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	client, err := jellyfin.New(jellyfin.Config{
		URL:               srv.URL,
		APIKey:            secret,
		MoviesDir:         "Movies",
		SeriesDir:         "TV Shows",
		ForcedLibraryPath: "/srv/media",
		MaxAttempts:       1,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	connection, err := client.Discover(t.Context())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !connection.Translation.Forced {
		t.Fatal("forced translation was not reported")
	}
	if err := client.Refresh(t.Context(), []string{"Movies/Iron Man"}); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if requests.Load() != 1 || gotPath != "/srv/media/Movies/Iron Man" {
		t.Fatalf("requests/path = %d/%q, want one forced update", requests.Load(), gotPath)
	}
}

func TestJellyfinFailureRedactsKeyFromErrorAndLog(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "failure "+secret)
	}))
	t.Cleanup(srv.Close)
	client := newClient(t, srv.URL)
	var logs bytes.Buffer
	service := application.NewService(client, nil, slog.New(slog.NewTextHandler(&logs, nil)))
	service.RefreshConfiguration(t.Context())

	if status := service.Status(); status.State != domain.StateError || !status.LastErrorAt.After(time.Time{}) {
		t.Fatalf("status = %+v, want an error with a timestamp", status)
	}
	if strings.Contains(service.Status().LastError, secret) || strings.Contains(logs.String(), secret) {
		t.Fatalf("secret leaked in status/log: %q / %q", service.Status().LastError, logs.String())
	}
}

func TestUnconfiguredJellyfinMakesNoRequests(t *testing.T) {
	t.Parallel()

	client, err := jellyfin.New(jellyfin.Config{MoviesDir: "Movies", SeriesDir: "TV Shows"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if client.Configured() {
		t.Fatal("empty configuration is configured")
	}
	if _, err := client.Discover(t.Context()); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if err := client.Refresh(t.Context(), []string{"Movies/Movie"}); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
}

func TestJellyfinRetriesTransientFailure(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Library/Refresh" {
			http.NotFound(w, r)
			return
		}
		if attempts.Add(1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	client, err := jellyfin.New(jellyfin.Config{
		URL:            srv.URL,
		APIKey:         secret,
		MoviesDir:      "Movies",
		SeriesDir:      "TV Shows",
		MaxAttempts:    3,
		RetryDelay:     time.Millisecond,
		RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := client.Refresh(t.Context(), []string{"Movies/Movie"}); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if got := attempts.Load(); got != 3 {
		t.Fatalf("attempts = %d, want 3", got)
	}
}
