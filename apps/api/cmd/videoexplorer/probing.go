package main

import (
	"database/sql"
	"log/slog"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/ffprobe"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/fileid"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
)

// withProbing reads the kept videos in the background (RF-24, RF-44): ffprobe reads
// each one, and the reading is cached in the shared database by the file's identity.
func withProbing(svc *application.Service, conn *sql.DB, log *slog.Logger) {
	prober := application.NewCachedProber(ffprobe.New(""), fileid.Identifier{}, sqlite.NewMediaProbeRepository(conn), log)
	svc.WithProbing(prober)
}
