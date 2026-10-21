package web_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/web"
)

const indexHTML = "<!doctype html><title>app</title>"

func builtSPA() fstest.MapFS {
	return fstest.MapFS{
		"index.html":    {Data: []byte(indexHTML)},
		"assets/app.js": {Data: []byte("console.log('app')")},
		"favicon.svg":   {Data: []byte("<svg></svg>")},
	}
}

func TestSPAHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		fsys       fstest.MapFS
		method     string
		path       string
		wantStatus int
		wantBody   string
		wantCache  string
	}{
		{name: "root serves index", fsys: builtSPA(), method: http.MethodGet, path: "/", wantStatus: http.StatusOK, wantBody: indexHTML, wantCache: "no-cache"},
		{name: "explicit index", fsys: builtSPA(), method: http.MethodGet, path: "/index.html", wantStatus: http.StatusOK, wantBody: indexHTML, wantCache: "no-cache"},
		{name: "static asset", fsys: builtSPA(), method: http.MethodGet, path: "/favicon.svg", wantStatus: http.StatusOK, wantBody: "<svg></svg>"},
		{name: "fingerprinted asset is immutable", fsys: builtSPA(), method: http.MethodGet, path: "/assets/app.js", wantStatus: http.StatusOK, wantBody: "console.log('app')", wantCache: "public, max-age=31536000, immutable"},
		{name: "client route falls back to index", fsys: builtSPA(), method: http.MethodGet, path: "/movies/iron-man", wantStatus: http.StatusOK, wantBody: indexHTML, wantCache: "no-cache"},
		{name: "path traversal falls back to index", fsys: builtSPA(), method: http.MethodGet, path: "/../../etc/passwd", wantStatus: http.StatusOK, wantBody: indexHTML, wantCache: "no-cache"},
		{name: "head of a client route", fsys: builtSPA(), method: http.MethodHead, path: "/series", wantStatus: http.StatusOK},
		{name: "post is not allowed", fsys: builtSPA(), method: http.MethodPost, path: "/", wantStatus: http.StatusMethodNotAllowed},
		{name: "without a build a dev hint is served", fsys: fstest.MapFS{".gitkeep": {}}, method: http.MethodGet, path: "/", wantStatus: http.StatusOK, wantBody: "task dev:web"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := web.NewHandler(tt.fsys)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantBody != "" && !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want it to contain %q", rec.Body.String(), tt.wantBody)
			}
			if tt.wantCache != "" && rec.Header().Get("Cache-Control") != tt.wantCache {
				t.Errorf("Cache-Control = %q, want %q", rec.Header().Get("Cache-Control"), tt.wantCache)
			}
		})
	}
}
