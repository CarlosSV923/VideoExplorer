package main

import (
	"database/sql"
	"log/slog"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/ffprobe"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/fileid"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/partialhash"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
)

// withProbing reads the kept videos in the background (RF-24, RF-44): ffprobe reads
// each one, and the reading is cached in the shared database by the file's identity.
// Each kept video also gets its partial hash cached (RF-24), and on-demand hashes are
// allowed under library.
func withProbing(svc *application.Service, conn *sql.DB, library string, log *slog.Logger) {
	ids := fileid.Identifier{}
	prober := application.NewCachedProber(ffprobe.New(""), ids, sqlite.NewMediaProbeRepository(conn), log)
	svc.WithProbing(prober)
	hasher := application.NewCachedPartialHasher(partialhash.Hasher{}, ids, sqlite.NewPartialHashRepository(conn), log)
	svc.WithPartialHashing(hasher, fileid.Links{}, library)
}
