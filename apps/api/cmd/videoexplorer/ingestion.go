package main

import (
	"database/sql"
	"log/slog"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
	ingestionhttp "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/interfaces/http"
)

// ingestionWiring is the ingestion context as the composition root builds it.
type ingestionWiring struct {
	// Service is what the upload endpoint and the batch endpoint use.
	Service *application.Service
	// Handler answers the batch operations of the generated contract.
	Handler *ingestionhttp.Handler
}

// newIngestion wires the batches and the upload jobs (phase 3) over the shared database.
func newIngestion(conn *sql.DB, log *slog.Logger) ingestionWiring {
	svc := application.NewService(sqlite.NewBatchRepository(conn), sqlite.NewJobRepository(conn), time.Now)
	return ingestionWiring{Service: svc, Handler: ingestionhttp.NewHandler(svc, log)}
}
