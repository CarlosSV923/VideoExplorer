package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	catalogapplication "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	catalogsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite"
	catalogshttp "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/interfaces/http"
	ingestionapplication "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/config"
)

// scansWiring is the two scans of the library as the composition root builds them (RF-28).
type scansWiring struct {
	// Scanner runs the scans: at start, at their intervals and on demand.
	Scanner *catalogapplication.Scanner
	// Handler answers GET /scans and POST /scans.
	Handler *catalogshttp.ScanHandler
}

// newScans wires the scans over the section, the Samba arrivals of the ingestion and the store of the
// times (RF-28). The library scan has no work until the import of RF-29 arrives: until then it only
// records its time. ctx is the context the scans run under: its end stops them.
func newScans(ctx context.Context, cfg config.Config, conn *sql.DB, catalog catalogWiring, ingest *ingestionapplication.Service, log *slog.Logger) (scansWiring, error) {
	scanner, err := catalogapplication.NewScanner(ctx, catalogapplication.ScanDeps{
		Section:         catalog.Section,
		Arrivals:        arrivalsOf{ingest: ingest},
		Store:           catalogsqlite.NewScanRepository(conn),
		UnassignedEvery: cfg.UnassignedScanInterval,
		LibraryEvery:    cfg.LibraryScanInterval,
		Now:             time.Now,
		After:           time.After,
		Log:             log,
	})
	if err != nil {
		return scansWiring{}, fmt.Errorf("start the scans: %w", err)
	}
	return scansWiring{Scanner: scanner, Handler: catalogshttp.NewScanHandler(scanner)}, nil
}

// stopScans ends the scans and waits for them, for at most shutdownTimeout, so that the database is not
// closed under a scan. done is what Scanner.Start returns.
func stopScans(stop context.CancelFunc, done <-chan struct{}, log *slog.Logger) {
	stop()
	select {
	case <-done:
	case <-time.After(shutdownTimeout):
		log.Warn("scans still running at shutdown")
	}
}
