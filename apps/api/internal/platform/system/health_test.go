package system_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/system"
)

func TestGetHealth(t *testing.T) {
	t.Parallel()

	resp, err := system.NewHandler().GetHealth(t.Context(), httpapi.GetHealthRequestObject{})
	if err != nil {
		t.Fatalf("GetHealth: %v", err)
	}
	rec := httptest.NewRecorder()
	if err := resp.VisitGetHealthResponse(rec); err != nil {
		t.Fatalf("write response: %v", err)
	}

	tests := []struct {
		name string
		ok   bool
	}{
		{name: "status code is 200", ok: rec.Code == http.StatusOK},
		{name: "content type is JSON", ok: rec.Header().Get("Content-Type") == "application/json"},
		{name: "answer is not cacheable", ok: rec.Header().Get("Cache-Control") == "no-store"},
		{name: "status is ok", ok: strings.Contains(rec.Body.String(), `"status":"ok"`)},
		{name: "checks is an empty array, not null", ok: strings.Contains(rec.Body.String(), `"checks":[]`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if !tt.ok {
				t.Fatalf("failed; response was %d %v: %s", rec.Code, rec.Header(), rec.Body.String())
			}
		})
	}
}
