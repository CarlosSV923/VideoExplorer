package main

import (
	"database/sql"
	"log/slog"
	"time"

	catalogapplication "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/libraryfs"
	catalogsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite"
	catalogshttp "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/interfaces/http"
	ingestionapplication "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/ffprobe"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/fileid"
	ingestionsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/config"
)

// newLibrary wires the title pages (RF-40, RF-43) and the edit of a title (RF-44) over the registry, the engine,
// TMDB (spec §6), the file system and ffprobe, the same adapters that the assignment uses (newAssign).
func newLibrary(cfg config.Config, conn *sql.DB, catalog catalogWiring, meta metadataWiring, ingest *ingestionapplication.Service, log *slog.Logger) *catalogshttp.LibraryHandler {
	files := libraryfs.New(cfg.LibraryPath)
	library := catalogsqlite.NewLibraryRepository(conn)
	prober := ingestionapplication.NewCachedProber(ffprobe.New(""), fileid.Identifier{}, ingestionsqlite.NewMediaProbeRepository(conn), log)
	catalog.Trash.ConfigureRestore(catalogapplication.TrashRestoreDeps{
		Repository: catalogsqlite.NewTrashRepository(conn), Library: library, Files: files, Engine: catalog.Engine,
		Finder: catalogapplication.NewDuplicateFinder(library, files, hasherOf{ingest: ingest}, cfg.VideoExtensions),
		Media:  mediaOf{prober: prober, log: log}, SubtitleExts: cfg.SubtitleExtensions,
		Roots: catalogapplication.Roots{Movies: cfg.MoviesDir, Series: cfg.SeriesDir}, Now: time.Now,
	})
	edits := catalogapplication.NewEdits(catalogapplication.EditDeps{
		Library:      library,
		Files:        files,
		Engine:       catalog.Engine,
		Finder:       catalogapplication.NewDuplicateFinder(library, files, hasherOf{ingest: ingest}, cfg.VideoExtensions),
		Media:        mediaOf{prober: prober, log: log},
		TMDB:         tmdbOf{svc: meta.Service},
		Roots:        catalogapplication.Roots{Movies: cfg.MoviesDir, Series: cfg.SeriesDir},
		SubtitleExts: cfg.SubtitleExtensions,
		Now:          time.Now,
		Log:          log,
	})
	views := catalogapplication.NewLibraryViews(library, tmdbOf{svc: meta.Service}, log)
	return catalogshttp.NewLibraryHandler(views, edits, log)
}
