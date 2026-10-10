package system_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/system"
)

// serveHealth answers GET /health the way the server does: through the
// generated response visitor.
func serveHealth(t *testing.T, h *system.Handler) *httptest.ResponseRecorder {
	t.Helper()

	resp, err := h.GetHealth(t.Context(), httpapi.GetHealthRequestObject{})
	if err != nil {
		t.Fatalf("GetHealth: %v", err)
	}
	rec := httptest.NewRecorder()
	if err := resp.VisitGetHealthResponse(rec); err != nil {
		t.Fatalf("write response: %v", err)
	}
	return rec
}

// healthBody decodes the answer. Each check keeps its raw keys so the tests can
// check the JSON shape, not only the Go values.
type healthBody struct {
	Status string           `json:"status"`
	Checks []map[string]any `json:"checks"`
}

func decodeHealth(t *testing.T, rec *httptest.ResponseRecorder) healthBody {
	t.Helper()

	var body healthBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return body
}

func passingCheck(name string) system.Check {
	return system.Check{
		Name: name,
		Code: httpapi.LibraryNotWritable,
		Run:  func(context.Context) *system.Failure { return nil },
	}
}

func failingCheck(name string, code httpapi.CheckCode, failure system.Failure) system.Check {
	return system.Check{
		Name: name,
		Code: code,
		Run:  func(context.Context) *system.Failure { return &failure },
	}
}

func TestGetHealthAnswersOKAsCacheableJSON(t *testing.T) {
	t.Parallel()

	rec := serveHealth(t, system.NewHandler())

	tests := []struct {
		name string
		ok   bool
	}{
		{name: "status code is 200", ok: rec.Code == http.StatusOK},
		{name: "content type is JSON", ok: rec.Header().Get("Content-Type") == "application/json"},
		{name: "answer is not cacheable", ok: rec.Header().Get("Cache-Control") == "no-store"},
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

func TestGetHealthAggregatesChecks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		checks     []system.Check
		wantStatus string
		wantNames  []string
		wantOK     []bool
	}{
		{
			name:       "no checks is ok with an empty array, not null",
			wantStatus: "ok",
			wantNames:  []string{},
			wantOK:     []bool{},
		},
		{
			name:       "every check passing is ok",
			checks:     []system.Check{passingCheck("library"), passingCheck("data")},
			wantStatus: "ok",
			wantNames:  []string{"library", "data"},
			wantOK:     []bool{true, true},
		},
		{
			name: "one failing check makes the status degraded and keeps the order",
			checks: []system.Check{
				passingCheck("library"),
				failingCheck("data", httpapi.DataNotWritable, system.Failure{Message: "no"}),
				passingCheck("7zz"),
			},
			wantStatus: "degraded",
			wantNames:  []string{"library", "data", "7zz"},
			wantOK:     []bool{true, false, true},
		},
		{
			name: "every check failing is still a 200 answer, degraded",
			checks: []system.Check{
				failingCheck("7zz", httpapi.SevenzipMissing, system.Failure{Message: "no"}),
				failingCheck("ffprobe", httpapi.FfprobeMissing, system.Failure{Message: "no"}),
			},
			wantStatus: "degraded",
			wantNames:  []string{"7zz", "ffprobe"},
			wantOK:     []bool{false, false},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := serveHealth(t, system.NewHandler(tt.checks...))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			body := decodeHealth(t, rec)
			if body.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", body.Status, tt.wantStatus)
			}
			if body.Checks == nil {
				t.Fatalf("checks is null, want an array: %s", rec.Body.String())
			}
			if len(body.Checks) != len(tt.wantNames) {
				t.Fatalf("got %d checks, want %d: %s", len(body.Checks), len(tt.wantNames), rec.Body.String())
			}
			for i, c := range body.Checks {
				if c["name"] != tt.wantNames[i] || c["ok"] != tt.wantOK[i] {
					t.Errorf("check %d = %v, want name %q ok %v", i, c, tt.wantNames[i], tt.wantOK[i])
				}
			}
		})
	}
}

func TestGetHealthCheckShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		check    system.Check
		wantKeys []string
	}{
		{
			name:     "a passing check carries only its name and ok",
			check:    passingCheck("library"),
			wantKeys: []string{"name", "ok"},
		},
		{
			name: "a failing check carries its code and message",
			check: failingCheck("7zz", httpapi.SevenzipMissing,
				system.Failure{Message: "7zz is not in PATH"}),
			wantKeys: []string{"code", "message", "name", "ok"},
		},
		{
			name: "a failing folder check adds the path, uid and gid",
			check: failingCheck("library", httpapi.LibraryNotWritable, system.Failure{
				Message: "cannot create and remove a file",
				Path:    "/library",
				UID:     intPtr(1000),
				GID:     intPtr(100),
			}),
			wantKeys: []string{"code", "gid", "message", "name", "ok", "path", "uid"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := serveHealth(t, system.NewHandler(tt.check))
			checks := decodeHealth(t, rec).Checks
			if len(checks) != 1 {
				t.Fatalf("got %d checks, want 1: %s", len(checks), rec.Body.String())
			}
			var keys []string
			for k := range checks[0] {
				keys = append(keys, k)
			}
			slices.Sort(keys)
			if !slices.Equal(keys, tt.wantKeys) {
				t.Errorf("keys = %v, want %v: %s", keys, tt.wantKeys, rec.Body.String())
			}
		})
	}
}

func TestGetHealthFailureKeepsItsDataAndMessage(t *testing.T) {
	t.Parallel()

	rec := serveHealth(t, system.NewHandler(failingCheck("data", httpapi.DataNotWritable, system.Failure{
		Message: "cannot create and remove a file in /data as uid=1000 gid=100",
		Path:    "/data",
		UID:     intPtr(1000),
		GID:     intPtr(100),
	})))

	body := decodeHealth(t, rec)
	if body.Status != "degraded" {
		t.Fatalf("status = %q, want degraded", body.Status)
	}
	c := body.Checks[0]
	tests := []struct {
		field string
		want  any
	}{
		{field: "code", want: "data_not_writable"},
		{field: "path", want: "/data"},
		{field: "uid", want: float64(1000)},
		{field: "gid", want: float64(100)},
		{field: "message", want: "cannot create and remove a file in /data as uid=1000 gid=100"},
	}
	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			t.Parallel()
			if c[tt.field] != tt.want {
				t.Errorf("%s = %v, want %v", tt.field, c[tt.field], tt.want)
			}
		})
	}
	if msg, _ := c["message"].(string); strings.Contains(msg, "\n") {
		t.Errorf("message must be one line: %q", msg)
	}
}

func intPtr(v int) *int { return &v }
