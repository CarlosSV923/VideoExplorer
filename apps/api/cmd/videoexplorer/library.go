package main

import (
	"database/sql"
	"log/slog"

	catalogapplication "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	catalogsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite"
	catalogshttp "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/interfaces/http"
)

// newLibrary wires the reads of the title pages (RF-40, RF-43) over the registry and TMDB (spec §6).
func newLibrary(conn *sql.DB, meta metadataWiring, log *slog.Logger) *catalogshttp.LibraryHandler {
	views := catalogapplication.NewLibraryViews(catalogsqlite.NewLibraryRepository(conn), tmdbOf{svc: meta.Service}, log)
	return catalogshttp.NewLibraryHandler(views, log)
}
