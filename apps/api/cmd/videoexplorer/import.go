package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	catalogapplication "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	catalogdomain "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/libraryfs"
	catalogsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite"
	ingestiondomain "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/config"
)

// newLibraryImport wires the scan of Movies/ and TV Shows/ (RF-29) over the catalog's engine, registry and No
// asignados, TMDB, and the names and archives the ingestion reads. Each is adapted here, so that the catalog
// keeps the types of the other contexts out.
func newLibraryImport(cfg config.Config, conn *sql.DB, catalog catalogWiring, meta metadataWiring, log *slog.Logger) (*catalogapplication.LibraryImport, error) {
	files := libraryfs.New(cfg.LibraryPath)
	library := catalogsqlite.NewLibraryRepository(conn)
	imp, err := catalogapplication.NewLibraryImport(catalogapplication.ImportDeps{
		Files:    files,
		Engine:   catalog.Engine,
		Library:  library,
		Seen:     library,
		Aside:    catalog.Unassigned,
		Archives: archiveProbe{files: files},
		TMDB:     tmdbOf{svc: meta.Service},
		Names:    readingOf,
		Roots:    catalogapplication.Roots{Movies: cfg.MoviesDir, Series: cfg.SeriesDir},
		Media:    catalogdomain.MediaExts{Video: cfg.VideoExtensions, Subtitle: cfg.SubtitleExtensions},
		Now:      time.Now,
		Log:      log,
	})
	if err != nil {
		return nil, fmt.Errorf("library import: %w", err)
	}
	return imp, nil
}

// archiveProbe tells whether a file of the library is an archive from its first bytes (RF-27a). The same
// signatures as the ingestion's (domain.DetectArchive) decide.
type archiveProbe struct {
	files *libraryfs.FS
}

// IsArchive implements catalogapplication.ArchiveProbe.
func (a archiveProbe) IsArchive(path string) (bool, error) {
	header, err := a.files.ReadHeader(path, 8)
	if err != nil {
		return false, err
	}
	_, archive := ingestiondomain.DetectArchive(header)
	return archive, nil
}

// readingOf reads what a file of a title folder says about itself with the ingestion's analysis (RF-20,
// RF-23, RF-29), in the terms of the catalog's import.
func readingOf(rel string, subtitle bool) catalogdomain.FileReading {
	kind := ingestiondomain.KindVideo
	if subtitle {
		kind = ingestiondomain.KindSubtitle
	}
	a := ingestiondomain.AnalyzeFile(rel, kind)
	return catalogdomain.FileReading{
		Season: a.Season, Chapter: a.Chapter, Last: a.LastChapter,
		Language: a.Language, Forced: a.Forced, SDH: a.SDH,
		TMDBID: a.TMDBID, IMDbID: a.IMDbID,
	}
}
