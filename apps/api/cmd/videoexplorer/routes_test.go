package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRoutes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		method          string
		path            string
		wantStatus      int
		wantContentType string
		wantBodyPart    string
		forbidBodyPart  string
	}{
		{
			name:            "health answers ok as JSON",
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

			rec := httptest.NewRecorder()
			routes(slog.New(slog.DiscardHandler)).ServeHTTP(rec,
				httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, nil))

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
