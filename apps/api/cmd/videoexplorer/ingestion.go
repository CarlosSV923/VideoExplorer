package main

import (
	"database/sql"
	"log/slog"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/broker"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
	ingestionhttp "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/interfaces/http"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/config"
)

// purgeInterval is how often the upload purge runs, after the one at start (RF-09, RF-10b).
const purgeInterval = time.Hour

// ingestionWiring is the ingestion context as the composition root builds it.
type ingestionWiring struct {
	// Service is what the upload endpoint, the purge and the batch endpoint use.
	Service *application.Service
	// Handler answers the batch, job and event operations of the generated contract.
	Handler *ingestionhttp.Handler
}

// newIngestion wires the batches, the upload jobs and their event stream (phase 3)
// over the shared database. shutdown closes when the server stops, so the open
// event streams end with it.
func newIngestion(conn *sql.DB, log *slog.Logger, shutdown <-chan struct{}) ingestionWiring {
	events := broker.New(time.Now)
	svc := application.NewService(sqlite.NewBatchRepository(conn), sqlite.NewJobRepository(conn), events, log, time.Now)
	return ingestionWiring{Service: svc, Handler: ingestionhttp.NewHandler(svc, events, shutdown, log)}
}

// withFiltering enables the filter of RF-07 on the ingestion service: what it finds is
// recorded in the shared database, and the rules are VIDEO_EXTENSIONS and SUBTITLE_EXTENSIONS.
func withFiltering(svc *application.Service, conn *sql.DB, cfg config.Config) {
	svc.WithFiltering(sqlite.NewFilterRepository(conn), domain.NewFilterRules(cfg.VideoExtensions, cfg.SubtitleExtensions))
}
