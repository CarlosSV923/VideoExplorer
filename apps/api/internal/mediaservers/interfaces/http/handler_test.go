package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/mediaservers/domain"
	mediaservershttp "github.com/CarlosSV923/VideoExplorer/apps/api/internal/mediaservers/interfaces/http"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
)

type fakeStatus struct{ value domain.Status }

func (f fakeStatus) Status() domain.Status { return f.value }

func TestGetSettingsTable(t *testing.T) {
	t.Parallel()

	when := time.Date(2026, time.October, 16, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		status      domain.Status
		wantState   httpapi.JellyfinSettingsState
		wantSource  httpapi.JellyfinSettingsTranslationSource
		wantLibrary string
		wantMovies  string
		wantSeries  string
		wantError   bool
		wantNotify  bool
	}{
		{
			name:       "unconfigured",
			status:     domain.Status{State: domain.StateUnconfigured, Translation: domain.Translation{Source: domain.TranslationNone}},
			wantState:  httpapi.JellyfinSettingsStateUnconfigured,
			wantSource: httpapi.JellyfinSettingsTranslationSourceNone,
		},
		{
			name: "detected translation and last notification",
			status: domain.Status{
				State:              domain.StateConnected,
				Translation:        domain.Translation{Source: domain.TranslationDetected, LibraryRoot: "/media", MoviesRoot: "/media/Movies", SeriesRoot: "/media/TV Shows"},
				LastNotificationAt: when,
			},
			wantState: httpapi.JellyfinSettingsStateConnected, wantSource: httpapi.JellyfinSettingsTranslationSourceDetected,
			wantLibrary: "/media",
			wantMovies:  "/media/Movies", wantSeries: "/media/TV Shows", wantNotify: true,
		},
		{
			name: "configured translation",
			status: domain.Status{
				State:       domain.StateConnected,
				Translation: domain.Translation{Forced: true, Source: domain.TranslationConfigured, LibraryRoot: "/media", MoviesRoot: "/media/Movies", SeriesRoot: "/media/TV Shows"},
			},
			wantState: httpapi.JellyfinSettingsStateConnected, wantSource: httpapi.JellyfinSettingsTranslationSourceConfigured,
			wantLibrary: "/media",
			wantMovies:  "/media/Movies", wantSeries: "/media/TV Shows",
		},
		{
			name: "error keeps code message and times",
			status: domain.Status{
				State: domain.StateError, Translation: domain.Translation{Source: domain.TranslationNone},
				LastErrorCode: "discovery_failed", LastError: "returned 401: unauthorized", LastErrorAt: when,
			},
			wantState: httpapi.JellyfinSettingsStateError, wantSource: httpapi.JellyfinSettingsTranslationSourceNone, wantError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			response, err := mediaservershttp.NewHandler(fakeStatus{value: tt.status}).GetSettings(t.Context(), httpapi.GetSettingsRequestObject{})
			if err != nil {
				t.Fatalf("GetSettings: %v", err)
			}
			body := response.(httpapi.GetSettings200JSONResponse)
			got := body.Jellyfin
			if got.State != tt.wantState || got.TranslationSource != tt.wantSource || got.Translation.LibraryPath != tt.wantLibrary || got.Translation.Movies != tt.wantMovies || got.Translation.Series != tt.wantSeries {
				t.Fatalf("jellyfin = %+v, want state/source/routes %q/%q/%q/%q/%q", got, tt.wantState, tt.wantSource, tt.wantLibrary, tt.wantMovies, tt.wantSeries)
			}
			if (got.LastError != nil) != tt.wantError || (got.LastNotificationAt != nil) != tt.wantNotify {
				t.Fatalf("optional status = lastError:%v lastNotificationAt:%v", got.LastError != nil, got.LastNotificationAt != nil)
			}
			if tt.wantError && (got.LastError.Code != httpapi.DiscoveryFailed || got.LastError.Message != tt.status.LastError || got.LastErrorAt == nil || !got.LastErrorAt.Equal(when)) {
				t.Fatalf("last error = %+v at %v", got.LastError, got.LastErrorAt)
			}
			if tt.wantNotify && !got.LastNotificationAt.Equal(when) {
				t.Fatalf("last notification = %v, want %v", got.LastNotificationAt, when)
			}

			rec := httptest.NewRecorder()
			if err := response.VisitGetSettingsResponse(rec); err != nil {
				t.Fatalf("write response: %v", err)
			}
			if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("response = %d %q", rec.Code, rec.Header().Get("Content-Type"))
			}
			var raw map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if strings.Contains(rec.Body.String(), "secret") || strings.Contains(rec.Body.String(), "jellyfin.local") {
				t.Fatalf("response leaked credentials or upstream URL: %s", rec.Body.String())
			}
		})
	}
}
