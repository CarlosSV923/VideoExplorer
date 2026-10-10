package system_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/system"
)

func TestHealthRoute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{name: "get health answers ok", method: http.MethodGet, path: "/api/health", wantStatus: http.StatusOK},
		{name: "post health is not allowed", method: http.MethodPost, path: "/api/health", wantStatus: http.StatusMethodNotAllowed},
		{name: "unknown route is not found", method: http.MethodGet, path: "/api/unknown", wantStatus: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mux := http.NewServeMux()
			system.RegisterRoutes(mux)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantStatus != http.StatusOK {
				return
			}

			var body system.Health
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body.Status != system.StatusOK {
				t.Errorf("status field = %q, want %q", body.Status, system.StatusOK)
			}
			if body.Checks == nil {
				t.Error("checks must be an empty array, not null")
			}
			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", got)
			}
		})
	}
}
