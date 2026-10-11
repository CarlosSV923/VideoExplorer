package main

import (
	"database/sql"
	"log/slog"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/broker"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
	ingestionhttp "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/interfaces/http"
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
