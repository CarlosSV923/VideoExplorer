package main

import (
	"log/slog"
	"net/http"

	identityhttp "github.com/CarlosSV923/VideoExplorer/apps/api/internal/identity/interfaces/http"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpx"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/system"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/web"
)

// Aliases give each embedded handler its own field name (they are all "Handler").
type (
	systemAPI   = system.Handler
	identityAPI = identityhttp.Handler
)

// api assembles the bounded contexts' handlers into the generated interface.
// Each embedded handler implements a disjoint subset of the operations.
type api struct {
	*systemAPI
	*identityAPI
}

var _ httpapi.StrictServerInterface = api{}

// routes mounts the generated API contract, the JSON fallback for unknown
// /api paths and the embedded SPA on everything else. The session check
// protects every operation except the public ones (RF-60). The order matters:
// the /api/ fallback is registered before "/" so the SPA never answers API paths.
// Security headers, request logging and panic recovery wrap everything (RNF-02).
func routes(log *slog.Logger, identity *identityhttp.Handler, health *system.Handler) http.Handler {
	server := api{
		systemAPI:   health,
		identityAPI: identity,
	}

	mux := http.NewServeMux()
	strict := httpapi.NewStrictHandlerWithOptions(server,
		[]httpapi.StrictMiddlewareFunc{identity.Middleware()},
		httpapi.StrictHTTPServerOptions{
			RequestErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
				httpx.WriteProblem(w, http.StatusBadRequest, "Bad Request", err.Error())
			},
			ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
				log.ErrorContext(r.Context(), "handler error", "path", r.URL.Path, "error", err)
				httpx.WriteProblem(w, http.StatusInternalServerError, "Internal Server Error", "")
			},
		})
	httpapi.HandlerWithOptions(strict, httpapi.StdHTTPServerOptions{
		BaseURL:    "/api",
		BaseRouter: mux,
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			httpx.WriteProblem(w, http.StatusBadRequest, "Bad Request", err.Error())
		},
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteProblem(w, http.StatusNotFound, "Not Found", "")
	})
	mux.Handle("/", web.Handler())

	return httpx.Chain(mux,
		httpx.Recover(log),
		httpx.Logging(log),
		httpx.SecurityHeaders,
		httpx.WithClientIP,
	)
}
