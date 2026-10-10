package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/infrastructure/imagecache"
)

// jpegImage starts with the JPEG signature, so the type check of the download accepts it.
var jpegImage = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{0x11}, 508)...)

// fakeSource stands in for TMDB's image CDN: it counts the downloads and
// answers with fixed bytes or an error.
type fakeSource struct {
	mu    sync.Mutex
	calls int
	body  []byte
	err   error
}

func (f *fakeSource) Fetch(_ context.Context, _ domain.ImageRef) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return io.NopCloser(bytes.NewReader(f.body)), nil
}

func (f *fakeSource) downloads() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// signIn logs in with the test password and returns the session cookie.
func signIn(t *testing.T, h http.Handler) *http.Cookie {
	t.Helper()

	rec := do(t, h, http.MethodPost, "/api/auth/login", `{"password":"`+testPassword+`"}`)
	cookie := sessionCookie(rec)
	if cookie == nil {
		t.Fatalf("login: status %d, no session cookie", rec.Code)
	}
	return cookie
}

func TestImagesNeedASession(t *testing.T) {
	t.Parallel()

	h := newTestRoutesWith(t, false, imagecache.New(t.TempDir(), &fakeSource{body: jpegImage}))
	rec := do(t, h, http.MethodGet, "/api/images/poster/9ZmWNo1wq1y9vSIt7iGlVAtsgXB.jpg", "")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control of a 401 = %q, want no-store: errors must never be cached", got)
	}
}

func TestImageRouteAnswers(t *testing.T) {
	t.Parallel()

	const file = "9ZmWNo1wq1y9vSIt7iGlVAtsgXB.jpg"
	tests := []struct {
		name       string
		path       string
		source     *fakeSource // nil: TMDB is not configured
		prepare    func(t *testing.T, dir string)
		wantStatus int
		wantType   string
		wantBody   []byte // nil: not checked
		wantFetch  int    // downloads from TMDB expected
	}{
		{
			name:       "unknown kind is 400",
			path:       "/api/images/avatar/" + file,
			source:     &fakeSource{body: jpegImage},
			wantStatus: http.StatusBadRequest,
			wantType:   "application/problem+json",
		},
		{
			name:       "file with another extension is 400",
			path:       "/api/images/poster/" + "a.gif",
			source:     &fakeSource{body: jpegImage},
			wantStatus: http.StatusBadRequest,
			wantType:   "application/problem+json",
		},
		{
			name:       "encoded path separator is 400 and never reaches the disk",
			path:       "/api/images/poster/..%2F..%2Fsecret.jpg",
			source:     &fakeSource{body: jpegImage},
			wantStatus: http.StatusBadRequest,
			wantType:   "application/problem+json",
		},
		{
			name:       "not cached and TMDB not configured is 404 JSON",
			path:       "/api/images/poster/" + file,
			wantStatus: http.StatusNotFound,
			wantType:   "application/problem+json",
		},
		{
			name:       "TMDB has no such image is 404",
			path:       "/api/images/poster/" + file,
			source:     &fakeSource{err: domain.ErrNotFound},
			wantStatus: http.StatusNotFound,
			wantType:   "application/problem+json",
			wantFetch:  1,
		},
		{
			name:       "TMDB failing is 502 when the image is not cached",
			path:       "/api/images/poster/" + file,
			source:     &fakeSource{err: domain.ErrUpstream},
			wantStatus: http.StatusBadGateway,
			wantType:   "application/problem+json",
			wantFetch:  1,
		},
		{
			name:       "missing image is downloaded and served as JPEG",
			path:       "/api/images/poster/" + file,
			source:     &fakeSource{body: jpegImage},
			wantStatus: http.StatusOK,
			wantType:   "image/jpeg",
			wantBody:   jpegImage,
			wantFetch:  1,
		},
		{
			name:   "cached image is served without asking TMDB",
			path:   "/api/images/poster/" + file,
			source: &fakeSource{err: domain.ErrUpstream},
			prepare: func(t *testing.T, dir string) {
				writeFile(t, filepath.Join(dir, "poster", file), jpegImage)
			},
			wantStatus: http.StatusOK,
			wantType:   "image/jpeg",
			wantBody:   jpegImage,
		},
		{
			name:   "cached image works without TMDB configured",
			path:   "/api/images/poster/" + file,
			source: nil,
			prepare: func(t *testing.T, dir string) {
				writeFile(t, filepath.Join(dir, "poster", file), jpegImage)
			},
			wantStatus: http.StatusOK,
			wantType:   "image/jpeg",
			wantBody:   jpegImage,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			if tt.prepare != nil {
				tt.prepare(t, dir)
			}
			var source application.ImageSource
			if tt.source != nil {
				source = tt.source
			}
			h := newTestRoutesWith(t, false, imagecache.New(dir, source))
			cookie := signIn(t, h)

			rec := do(t, h, http.MethodGet, tt.path, "", cookie)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body %q", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, tt.wantType) {
				t.Errorf("Content-Type = %q, want prefix %q", got, tt.wantType)
			}
			if tt.wantBody != nil && !bytes.Equal(rec.Body.Bytes(), tt.wantBody) {
				t.Errorf("body has %d bytes, want the %d bytes of the image", rec.Body.Len(), len(tt.wantBody))
			}
			if tt.source != nil && tt.source.downloads() != tt.wantFetch {
				t.Errorf("TMDB downloads = %d, want %d", tt.source.downloads(), tt.wantFetch)
			}
			// RNF-02: images are cached by the browser; every other answer of /api/* is not.
			wantCache := "no-store"
			if tt.wantStatus == http.StatusOK {
				wantCache = "private, max-age=2592000, immutable"
			}
			if got := rec.Header().Get("Cache-Control"); got != wantCache {
				t.Errorf("Cache-Control = %q, want %q", got, wantCache)
			}
		})
	}
}

func TestImageIsDownloadedOnceThenServedFromDisk(t *testing.T) {
	t.Parallel()

	source := &fakeSource{body: jpegImage}
	h := newTestRoutesWith(t, false, imagecache.New(t.TempDir(), source))
	cookie := signIn(t, h)
	const path = "/api/images/backdrop/9ZmWNo1wq1y9vSIt7iGlVAtsgXB.jpg"

	for i := range 2 {
		rec := do(t, h, http.MethodGet, path, "", cookie)
		if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), jpegImage) {
			t.Fatalf("request %d: status %d, body of %d bytes", i+1, rec.Code, rec.Body.Len())
		}
	}
	if got := source.downloads(); got != 1 {
		t.Errorf("TMDB downloads = %d, want 1: the second request must come from the disk", got)
	}
}

func TestImageRangeRequestAnswersPartialContent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	const file = "logo.png"
	writeFile(t, filepath.Join(dir, "logo", file), []byte("0123456789"))
	h := newTestRoutesWith(t, false, imagecache.New(dir, nil))
	cookie := signIn(t, h)

	full := do(t, h, http.MethodGet, "/api/images/logo/"+file, "", cookie)
	if full.Code != http.StatusOK || full.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("full request: status %d, Content-Type %q; want 200 image/png", full.Code, full.Header().Get("Content-Type"))
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/images/logo/"+file, nil)
	req.AddCookie(cookie)
	req.Header.Set("Range", "bytes=2-5")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", rec.Code)
	}
	if got := rec.Body.String(); got != "2345" {
		t.Errorf("body = %q, want %q", got, "2345")
	}
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
