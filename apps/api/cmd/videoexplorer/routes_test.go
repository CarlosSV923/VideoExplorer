package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	catalogshttp "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/interfaces/http"
	identityinfra "github.com/CarlosSV923/VideoExplorer/apps/api/internal/identity/infrastructure"
	identityhttp "github.com/CarlosSV923/VideoExplorer/apps/api/internal/identity/interfaces/http"
	ingestionhttp "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/interfaces/http"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/infrastructure/imagecache"
	metadatahttp "github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/interfaces/http"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/config"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/system"
)

const (
	testPassword = "correct horse battery"
	testSecret   = "0123456789abcdef0123456789abcdef" // 32 bytes, as SESSION_SECRET requires
	wantCSP      = "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; " +
		"connect-src 'self'; frame-ancestors 'none'; base-uri 'self'"
)

// newTestRoutes builds the production routes with a real password hash and a
// session of one hour. Images come from an empty cache with no TMDB source.
func newTestRoutes(t *testing.T, cookieSecure bool) http.Handler {
	t.Helper()

	return newTestRoutesWith(t, cookieSecure, imagecache.New(t.TempDir(), nil))
}

// newTestRoutesWith is newTestRoutes with the image store of the test. Uploads
// go to a fresh library folder, and batches to a fresh database.
func newTestRoutesWith(t *testing.T, cookieSecure bool, images application.ImageStore) http.Handler {
	t.Helper()

	log := slog.New(slog.DiscardHandler)
	ingest, _ := newTestIngestion(t)
	uploads := newUploads(config.Config{LibraryPath: t.TempDir()}, log, ingest.Service)
	return newTestRoutesWithUploads(t, cookieSecure, images, ingest.Handler, uploads.handler, nil)
}

// newTestRoutesWithUploads is newTestRoutesWith with the batch and upload
// endpoints of the test.
func newTestRoutesWithUploads(t *testing.T, cookieSecure bool, images application.ImageStore, ingest *ingestionhttp.Handler, uploads http.Handler, scans *catalogshttp.ScanHandler) http.Handler {
	t.Helper()

	hash, err := identityinfra.HashPassword(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := newIdentity(config.Config{
		PasswordHash:  hash,
		SessionSecret: testSecret,
		SessionTTL:    time.Hour,
		CookieSecure:  cookieSecure,
	})
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.DiscardHandler)
	return routes(log, identity, system.NewHandler(), metadatahttp.NewHandler(images, log), ingest, uploads, nil, nil, scans, nil)
}

func do(t *testing.T, h http.Handler, method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()

	var in io.Reader
	if body != "" {
		in = strings.NewReader(body)
	}
	req := httptest.NewRequestWithContext(t.Context(), method, path, in)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func sessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == identityhttp.CookieName {
			return c
		}
	}
	return nil
}

func TestRoutes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		method          string
		path            string
		body            string
		wantStatus      int
		wantContentType string
		wantBodyPart    string
		forbidBodyPart  string
	}{
		{
			name:            "health answers ok as JSON without a session",
			method:          http.MethodGet,
			path:            "/api/health",
			wantStatus:      http.StatusOK,
			wantContentType: "application/json",
			wantBodyPart:    `"status":"ok"`,
		},
		{
			name:            "unknown API path answers a JSON problem",
			method:          http.MethodGet,
			path:            "/api/loquesea",
			wantStatus:      http.StatusNotFound,
			wantContentType: "application/problem+json",
			forbidBodyPart:  "<html",
		},
		{
			name:            "unknown API path never falls back to the SPA",
			method:          http.MethodGet,
			path:            "/api/",
			wantStatus:      http.StatusNotFound,
			wantContentType: "application/problem+json",
			forbidBodyPart:  "<!doctype html",
		},
		{
			name:            "session check without a cookie needs sign-in",
			method:          http.MethodGet,
			path:            "/api/auth/session",
			wantStatus:      http.StatusUnauthorized,
			wantContentType: "application/problem+json",
			wantBodyPart:    `"status":401`,
			forbidBodyPart:  "<html",
		},
		{
			name:            "logout without a cookie needs sign-in",
			method:          http.MethodPost,
			path:            "/api/auth/logout",
			wantStatus:      http.StatusUnauthorized,
			wantContentType: "application/problem+json",
			wantBodyPart:    `"status":401`,
		},
		{
			name:            "login with a wrong password is 401 and public",
			method:          http.MethodPost,
			path:            "/api/auth/login",
			body:            `{"password":"wrong"}`,
			wantStatus:      http.StatusUnauthorized,
			wantContentType: "application/problem+json",
			wantBodyPart:    `"status":401`,
		},
		{
			name:            "root serves the SPA",
			method:          http.MethodGet,
			path:            "/",
			wantStatus:      http.StatusOK,
			wantContentType: "text/html",
		},
		{
			name:            "client route falls back to the SPA",
			method:          http.MethodGet,
			path:            "/peliculas/iron-man",
			wantStatus:      http.StatusOK,
			wantContentType: "text/html",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := do(t, newTestRoutes(t, false), tt.method, tt.path, tt.body)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, tt.wantContentType) {
				t.Errorf("Content-Type = %q, want prefix %q", got, tt.wantContentType)
			}
			body, err := io.ReadAll(rec.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if tt.wantBodyPart != "" && !strings.Contains(string(body), tt.wantBodyPart) {
				t.Errorf("body = %q, want it to contain %q", body, tt.wantBodyPart)
			}
			if tt.forbidBodyPart != "" && strings.Contains(string(body), tt.forbidBodyPart) {
				t.Errorf("body = %q, must not contain %q", body, tt.forbidBodyPart)
			}
		})
	}
}

func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		method    string
		path      string
		wantCache string // "" means the header is not checked
	}{
		{name: "spa", method: http.MethodGet, path: "/"},
		{name: "health", method: http.MethodGet, path: "/api/health", wantCache: "no-store"},
		{name: "unknown api path", method: http.MethodGet, path: "/api/loquesea", wantCache: "no-store"},
		{name: "protected api answer", method: http.MethodGet, path: "/api/auth/session", wantCache: "no-store"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := do(t, newTestRoutes(t, false), tt.method, tt.path, "")

			want := map[string]string{
				"X-Content-Type-Options":  "nosniff",
				"Referrer-Policy":         "no-referrer",
				"X-Frame-Options":         "DENY",
				"Content-Security-Policy": wantCSP,
			}
			for header, value := range want {
				if got := rec.Header().Get(header); got != value {
					t.Errorf("%s = %q, want %q", header, got, value)
				}
			}
			if tt.wantCache != "" {
				if got := rec.Header().Get("Cache-Control"); got != tt.wantCache {
					t.Errorf("Cache-Control = %q, want %q", got, tt.wantCache)
				}
			}
		})
	}
}

func TestSessionFlow(t *testing.T) {
	t.Parallel()

	h := newTestRoutes(t, false)

	rec := do(t, h, http.MethodPost, "/api/auth/login", `{"password":"nope"}`)
	if rec.Code != http.StatusUnauthorized || sessionCookie(rec) != nil {
		t.Fatalf("wrong password: status %d, cookie %v; want 401 and no cookie", rec.Code, sessionCookie(rec))
	}

	rec = do(t, h, http.MethodPost, "/api/auth/login", `{"password":"`+testPassword+`"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("right password: status = %d, want 204", rec.Code)
	}
	cookie := sessionCookie(rec)
	if cookie == nil {
		t.Fatal("login did not set the session cookie")
	}
	if !cookie.HttpOnly || cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" {
		t.Errorf("cookie attributes = HttpOnly:%v Secure:%v SameSite:%v Path:%q; want HttpOnly, not Secure (COOKIE_SECURE=false), Strict, /",
			cookie.HttpOnly, cookie.Secure, cookie.SameSite, cookie.Path)
	}
	if cookie.MaxAge != int(time.Hour.Seconds()) {
		t.Errorf("MaxAge = %d, want %d (SESSION_TTL)", cookie.MaxAge, int(time.Hour.Seconds()))
	}

	rec = do(t, h, http.MethodGet, "/api/auth/session", "", cookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"expiresAt"`) {
		t.Fatalf("session with cookie: status %d, body %q; want 200 with expiresAt", rec.Code, rec.Body.String())
	}

	tampered := &http.Cookie{Name: identityhttp.CookieName, Value: cookie.Value + "x"}
	for name, c := range map[string]*http.Cookie{"no cookie": nil, "tampered cookie": tampered} {
		var cookies []*http.Cookie
		if c != nil {
			cookies = append(cookies, c)
		}
		rec = do(t, h, http.MethodGet, "/api/auth/session", "", cookies...)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("session with %s: status = %d, want 401", name, rec.Code)
		}
	}

	rec = do(t, h, http.MethodPost, "/api/auth/logout", "", cookie)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout: status = %d, want 204", rec.Code)
	}
	cleared := sessionCookie(rec)
	if cleared == nil || cleared.Value != "" || cleared.MaxAge >= 0 {
		t.Errorf("logout cookie = %+v, want an empty cookie that expires now", cleared)
	}
}

func TestLoginIsSecureOnlyWhenConfigured(t *testing.T) {
	t.Parallel()

	rec := do(t, newTestRoutes(t, true), http.MethodPost, "/api/auth/login", `{"password":"`+testPassword+`"}`)
	cookie := sessionCookie(rec)
	if cookie == nil || !cookie.Secure {
		t.Fatalf("COOKIE_SECURE=true: cookie = %+v, want Secure", cookie)
	}
}

func TestLoginThrottle(t *testing.T) {
	t.Parallel()

	h := newTestRoutes(t, false)
	const wrong = `{"password":"nope"}`

	for attempt := 1; attempt <= 5; attempt++ {
		if rec := do(t, h, http.MethodPost, "/api/auth/login", wrong); rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401 within the burst", attempt, rec.Code)
		}
	}

	// The sixth attempt is throttled even with the right password (5 at once, then 1 every 12 s).
	rec := do(t, h, http.MethodPost, "/api/auth/login", `{"password":"`+testPassword+`"}`)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("attempt 6: status = %d, want 429", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
	if sessionCookie(rec) != nil {
		t.Error("throttled login must not set a cookie")
	}
}
