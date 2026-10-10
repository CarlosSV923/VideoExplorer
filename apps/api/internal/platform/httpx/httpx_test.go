package httpx_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpx"
)

func TestWriteProblem(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	httpx.WriteProblem(rec, http.StatusTeapot, "Short", "Longer detail")

	if rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("content-type = %q", ct)
	}
	var p httpx.Problem
	if err := json.NewDecoder(rec.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	if p.Status != http.StatusTeapot || p.Title != "Short" || p.Detail != "Longer detail" {
		t.Fatalf("problem = %+v", p)
	}
}

func TestWithRequestStoresTheRequest(t *testing.T) {
	t.Parallel()

	var got *http.Request
	h := httpx.WithRequest(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got, _ = httpx.Request(r.Context())
	}))

	in := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/images/poster/a.jpg", nil)
	in.Header.Set("Range", "bytes=0-9")
	h.ServeHTTP(httptest.NewRecorder(), in)

	if got == nil {
		t.Fatal("Request found no request in the context")
	}
	if got.Header.Get("Range") != "bytes=0-9" || got.URL.Path != "/api/images/poster/a.jpg" {
		t.Errorf("stored request = %s %s (Range %q), want the original one", got.Method, got.URL.Path, got.Header.Get("Range"))
	}
}

func TestRequestMissingFromContext(t *testing.T) {
	t.Parallel()

	if r, ok := httpx.Request(t.Context()); ok || r != nil {
		t.Fatalf("Request on a bare context = (%v, %v), want (nil, false)", r, ok)
	}
}

func TestWithClientIP(t *testing.T) {
	t.Parallel()

	var got string
	h := httpx.WithClientIP(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = httpx.ClientIP(r.Context())
	}))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.8.20:51234"
	req.Header.Set("X-Forwarded-For", "1.2.3.4") // must be ignored
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got != "192.168.8.20" {
		t.Fatalf("ClientIP = %q, want 192.168.8.20", got)
	}
}

func TestRecoverReturnsProblem(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := httpx.Recover(log)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/x", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("content-type = %q", ct)
	}
}

func TestSecurityHeaders(t *testing.T) {
	t.Parallel()

	const wantCSP = "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; " +
		"connect-src 'self'; frame-ancestors 'none'; base-uri 'self'"

	tests := []struct {
		name      string
		path      string
		wantCache string
	}{
		{name: "api answers are not stored", path: "/api/health", wantCache: "no-store"},
		{name: "unknown api path too", path: "/api/loquesea", wantCache: "no-store"},
		{name: "spa assets are not forced to no-store", path: "/assets/app.js", wantCache: ""},
		{name: "spa root", path: "/", wantCache: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := httpx.SecurityHeaders(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, tt.path, nil))

			want := map[string]string{
				"X-Content-Type-Options":  "nosniff",
				"Referrer-Policy":         "no-referrer",
				"X-Frame-Options":         "DENY",
				"Content-Security-Policy": wantCSP,
				"Cache-Control":           tt.wantCache,
			}
			for header, value := range want {
				if got := rec.Header().Get(header); got != value {
					t.Errorf("%s = %q, want %q", header, got, value)
				}
			}
		})
	}
}

func TestChainRunsTheFirstMiddlewareOutermost(t *testing.T) {
	t.Parallel()

	var order []string
	mark := func(name string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}
	h := httpx.Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		order = append(order, "handler")
	}), mark("first"), mark("second"))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

	if got, want := len(order), 3; got != want {
		t.Fatalf("calls = %v", order)
	}
	if order[0] != "first" || order[1] != "second" || order[2] != "handler" {
		t.Fatalf("order = %v, want [first second handler]", order)
	}
}
