// Command videoexplorer runs the VideoExplorer HTTP API.
//
// This is the composition root: configuration, adapters and bounded contexts
// are wired here and nowhere else. It opens the database and serves the API
// contract (routes.go) and the embedded SPA. Sign-in (identity.go), the
// startup diagnostics (health.go), TMDB with its caches (metadata.go), the
// upload batches and jobs (ingestion.go) and the resumable uploads (uploads.go)
// are wired here; the other bounded contexts arrive in later phases.
//
// Usage:
//
//	videoexplorer                run the server (configured by environment, see .env.example)
//	videoexplorer hash-password  read a password from stdin, print its argon2id hash
package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	identityinfra "github.com/CarlosSV923/VideoExplorer/apps/api/internal/identity/infrastructure"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sevenzip"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/workspace"
	metadatahttp "github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/interfaces/http"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/config"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/database"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpserver"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/system"
)

const shutdownTimeout = 30 * time.Second

func main() {
	if len(os.Args) > 1 && os.Args[1] == "hash-password" {
		if err := hashPassword(os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	bootLog := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load(nil)
	if err != nil {
		bootLog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))

	// Before the database is opened: without a writable DATA_PATH the app
	// cannot keep its state, so it stops here and says why (RNF-03).
	if !dataPathWritable(cfg.DataPath, log) {
		os.Exit(1)
	}

	if err := run(cfg, log); err != nil {
		log.Error("server stopped with error", "error", err)
		os.Exit(1)
	}
}

// dataPathWritable reports whether this process can write in DATA_PATH. When
// it cannot, it logs an error that names the folder, the uid and gid of the
// process and the Modify permission to grant on the dataset (RNF-03, RF-63).
// It never opens the database.
func dataPathWritable(dataPath string, log *slog.Logger) bool {
	failure := system.WritableDir("data", httpapi.DataNotWritable, dataPath).Run(context.Background())
	if failure == nil {
		return true
	}
	attrs := []any{"path", failure.Path, "error", failure.Message}
	if failure.UID != nil {
		attrs = append(attrs, "uid", *failure.UID)
	}
	if failure.GID != nil {
		attrs = append(attrs, "gid", *failure.GID)
	}
	log.Error("DATA_PATH is not writable, the app does not start", attrs...)
	return false
}

// run opens the database, serves until SIGINT or SIGTERM, then shuts down in order.
func run(cfg config.Config, log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	conn, err := database.Open(ctx, cfg.DataPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer closeDatabase(conn, log)
	log.Info("database ready", "data", cfg.DataPath)

	identity, err := newIdentity(cfg)
	if err != nil {
		return err
	}

	meta, err := newMetadata(cfg, conn, log)
	if err != nil {
		return err
	}
	log.Info("metadata ready", "tmdb", meta.Service.Configured(), "language", cfg.TMDBLanguage)

	ingest := newIngestion(conn, log, ctx.Done())
	uploads := newUploads(cfg, log, ingest.Service)
	if uploads.adapter != nil {
		// The staging area is known only now: the adapter needs the service first.
		ingest.Service.WithStaging(uploads.adapter)
		// Archives are extracted in the staging area and their originals go to the trash (RF-04, RF-06).
		catalog := newCatalog(conn, cfg)
		ingest.Service.WithExtraction(workspace.New(stagingDir(cfg.LibraryPath), nil), sevenzip.New(""), trashForIngestion{trash: catalog.Trash})
		withFiltering(ingest.Service, conn, cfg)
		withProbing(ingest.Service, conn, cfg.LibraryPath, log)
		// Reconciled before the upload endpoint serves anything, so no upload is in flight (RF-10b).
		if res, err := ingest.Service.Recover(ctx); err != nil {
			log.Error("uploads not reconciled", "error", err)
		} else if res != (application.PurgeResult{}) {
			log.Info("uploads reconciled", "completed", res.Completed, "missing", res.Missing)
		}
		go uploads.adapter.Run(ctx)
		go ingest.Service.RunExtraction(ctx, cfg.ExtractConcurrency)
		go ingest.Service.RunProbing(ctx) // one reading at a time, behind the extractions (RF-24)
		go ingest.Service.RunPurge(ctx, purgeInterval)
	}

	srv := httpserver.New(net.JoinHostPort("", strconv.Itoa(cfg.Port)),
		routes(log, identity, newHealth(cfg), metadatahttp.NewHandler(meta.Images, log), ingest.Handler, uploads.handler))

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", srv.Addr, err)
	}
	log.Info("listening", "addr", ln.Addr().String())

	serveErr := make(chan error, 1)
	go func() { serveErr <- httpserver.Serve(ctx, srv, ln, shutdownTimeout) }()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
	}
	if err := <-serveErr; err != nil {
		return err
	}
	log.Info("stopped")
	return nil
}

func closeDatabase(conn *sql.DB, log *slog.Logger) {
	if err := conn.Close(); err != nil {
		log.Warn("close database", "error", err)
	}
}

// hashPassword reads one line from in and writes its argon2id PHC hash to out.
func hashPassword(in io.Reader, out io.Writer) error {
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	password := strings.TrimRight(line, "\r\n")
	if password == "" {
		return errors.New("empty password: pipe it on stdin, e.g. echo 'my password' | videoexplorer hash-password")
	}
	phc, err := identityinfra.HashPassword(password)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, phc)
	return err
}
