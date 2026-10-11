package main

import (
	"encoding/base64"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/infrastructure/imagecache"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/config"
)

// tusCreate sends the create request of the tus protocol, with the session cookie if given.
func tusCreate(t *testing.T, h http.Handler, fileName string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, uploadsPath, nil)
	req.Header.Set("Tus-Resumable", "1.0.0")
	req.Header.Set("Upload-Length", "10")
	req.Header.Set("Upload-Metadata", "filename "+base64.StdEncoding.EncodeToString([]byte(fileName)))
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestUploadsNeedASession(t *testing.T) {
	t.Parallel()

	h := newTestRoutes(t, false)
	tests := []struct {
		name, method, path string
	}{
		{name: "create", method: http.MethodPost, path: uploadsPath},
		{name: "resume point", method: http.MethodHead, path: uploadsPath + "0123abcd"},
		{name: "chunk", method: http.MethodPatch, path: uploadsPath + "0123abcd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := do(t, h, tt.method, tt.path, "")
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
				t.Errorf("Content-Type = %q, want the RFC 9457 problem", ct)
			}
		})
	}
}

func TestUploadsWithSessionCreateAnUpload(t *testing.T) {
	t.Parallel()

	h := newTestRoutes(t, false)
	login := do(t, h, http.MethodPost, "/api/auth/login", `{"password":"`+testPassword+`"}`)
	cookie := sessionCookie(login)
	if cookie == nil {
		t.Fatal("login did not set the session cookie")
	}

	batchID := newBatchID(t, h, cookie)
	rec := tusCreateWith(t, h, "filename "+b64("Iron Man (2008).mkv")+",batchId "+b64(batchID), cookie)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, uploadsPath) {
		t.Errorf("Location = %q, want a URL under %s", loc, uploadsPath)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store on the API (RNF-02)", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff (RNF-02)", got)
	}
}

func TestUploadsWithoutAWritableLibraryAnswer503AfterTheSessionCheck(t *testing.T) {
	t.Parallel()

	// A file where the library folder should be: the staging folder cannot be created.
	library := filepath.Join(t.TempDir(), "library-is-a-file")
	if err := os.WriteFile(library, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	ingest, _ := newTestIngestion(t)
	uploads := newUploads(config.Config{LibraryPath: library}, slog.New(slog.DiscardHandler), ingest.Service)
	if uploads.adapter != nil {
		t.Fatal("uploads enabled without a writable library")
	}
	h := newTestRoutesWithUploads(t, false, imagecache.New(t.TempDir(), nil), ingest.Handler, uploads.handler)

	if rec := tusCreate(t, h, "a.mkv"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("without a session: status = %d, want 401", rec.Code)
	}
	login := do(t, h, http.MethodPost, "/api/auth/login", `{"password":"`+testPassword+`"}`)
	rec := tusCreate(t, h, "a.mkv", sessionCookie(login))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("with a session: status = %d, want 503", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want the RFC 9457 problem", ct)
	}
}

func TestStagingIsInsideTheLibrary(t *testing.T) {
	t.Parallel()

	// Same file system as the library, so a finished upload moves with a rename (RNF-04).
	got := stagingDir("/library")
	want := filepath.Join("/library", ".videoexplorer", "staging")
	if got != want {
		t.Errorf("stagingDir = %q, want %q", got, want)
	}
}
