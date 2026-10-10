package main

import (
	"log/slog"
	"net/http"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpx"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/system"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/web"
)

// routes mounts the generated API contract, the JSON fallback for unknown
// /api paths and the embedded SPA on everything else. The order matters: the
// /api/ fallback is registered before "/" so the SPA never answers API paths.
func routes(log *slog.Logger) http.Handler {
	server := system.NewHandler()

	mux := http.NewServeMux()
	strict := httpapi.NewStrictHandlerWithOptions(server, nil, httpapi.StrictHTTPServerOptions{
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

	return mux
}
