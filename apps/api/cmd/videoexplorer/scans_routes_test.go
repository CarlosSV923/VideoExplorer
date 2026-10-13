package main

import (
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	catalogapplication "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	catalogdomain "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	catalogshttp "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/interfaces/http"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/infrastructure/imagecache"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/config"
)

// fixedScans answers the state of the scans of the library without running them.
type fixedScans struct{}

func (fixedScans) Status() []catalogapplication.ScanStatus {
	return []catalogapplication.ScanStatus{
		{Kind: catalogdomain.ScanUnassigned, Finished: time.Date(2026, time.October, 13, 10, 0, 0, 0, time.UTC)},
		{Kind: catalogdomain.ScanLibrary},
	}
}

func (fixedScans) Trigger() []catalogapplication.ScanStatus {
	return []catalogapplication.ScanStatus{{Kind: catalogdomain.ScanUnassigned, Running: true}, {Kind: catalogdomain.ScanLibrary, Running: true}}
}

func TestTheScansAnswerOnlyWithASession(t *testing.T) {
	t.Parallel()

	ingest, _ := newTestIngestion(t)
	uploads := newUploads(config.Config{LibraryPath: t.TempDir()}, slog.New(slog.DiscardHandler), ingest.Service)
	h := newTestRoutesWithUploads(t, false, imagecache.New(t.TempDir(), nil), ingest.Handler, uploads.handler, catalogshttp.NewScanHandler(fixedScans{}))

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		rec := do(t, h, method, "/api/scans", "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s /api/scans without a session = %d, want 401", method, rec.Code)
		}
	}

	rec := do(t, h, http.MethodPost, "/api/auth/login", `{"password":"`+testPassword+`"}`)
	cookie := sessionCookie(rec)
	if cookie == nil {
		t.Fatal("login did not set the session cookie")
	}

	rec = do(t, h, http.MethodGet, "/api/scans", "", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/scans = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"kind":"unassigned"`) || !strings.Contains(body, `"lastFinishedAt":"2026-10-13T10:00:00Z"`) {
		t.Fatalf("GET /api/scans body = %s, want both scans and the time the unassigned scan finished", body)
	}

	rec = do(t, h, http.MethodPost, "/api/scans", "", cookie)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST /api/scans = %d, want 202", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"running":true`) {
		t.Fatalf("POST /api/scans body = %s, want the scans running", body)
	}
}
