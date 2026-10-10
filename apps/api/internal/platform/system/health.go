// Package system implements process-level operations: the liveness probe and,
// from phase 1, the startup diagnostics (library, 7zz, ffprobe, TMDB).
package system

import (
	"encoding/json"
	"net/http"
)

// StatusOK is the health status of a process whose checks all pass.
const StatusOK = "ok"

// Health is the body of GET /api/health; its schema is Health in api/openapi.yaml.
// Until the diagnostics arrive in phase 1 the list of checks is empty.
type Health struct {
	Status string  `json:"status"`
	Checks []Check `json:"checks"`
}

// Check is one named diagnostic. Phase 1 fills the list.
type Check struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

// RegisterRoutes mounts the system routes on mux.
func RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/health", healthHandler())
}

func healthHandler() http.Handler {
	body := Health{Status: StatusOK, Checks: []Check{}}
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(body)
	})
}
