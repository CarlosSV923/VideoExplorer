package main

import (
	"context"
	"log/slog"
	"net/http"
	"path/filepath"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/tus"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/config"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/diskspace"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpx"
)

// uploadsPath is where the browser starts and resumes uploads (RF-02). The
// contract describes the protocol in its description, not as operations.
const uploadsPath = "/api/uploads/"

// uploadsWiring is the upload endpoint as the composition root builds it.
type uploadsWiring struct {
	// handler serves uploadsPath without its prefix; routes puts the session check in front.
	handler http.Handler
	// adapter runs the event loop of the finished uploads. Nil when the library cannot take uploads.
	adapter *tus.Adapter
}

// stagingDir holds the uploads until they are taken by the library. It lives
// in LIBRARY_PATH, on the same file system, so the move is a rename (RNF-01, RNF-04).
func stagingDir(libraryPath string) string {
	return filepath.Join(libraryPath, ".videoexplorer", "staging")
}

// newUploads wires the resumable uploads (RF-02). When the staging folder
// cannot be created, the endpoint answers 503 and the rest of the app keeps
// working: the library can still be browsed (RF-63).
func newUploads(cfg config.Config, log *slog.Logger) uploadsWiring {
	adapter, err := tus.New(stagingDir(cfg.LibraryPath), uploadsPath, log, diskspace.Free, logCompleted{log})
	if err != nil {
		log.Error("uploads disabled: library not writable", "error", err)
		return uploadsWiring{handler: uploadsUnavailable()}
	}
	return uploadsWiring{
		handler: http.StripPrefix(uploadsPath, adapter.Handler()),
		adapter: adapter,
	}
}

// uploadsUnavailable answers the upload endpoint when the library is not writable.
func uploadsUnavailable() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteProblem(w, http.StatusServiceUnavailable, "Service Unavailable",
			"Uploads are disabled because the library is not writable. See /api/health.")
	})
}

// logCompleted receives the finished uploads until the batch model is
// connected (phase 3, task 2). For now it only logs them.
type logCompleted struct{ log *slog.Logger }

func (l logCompleted) HandleUploadCompleted(ctx context.Context, u application.CompletedUpload) error {
	l.log.InfoContext(ctx, "upload completed", "upload", u.ID, "size", u.Size, "file", u.Meta.FileName, "batch", u.Meta.BatchID)
	return nil
}
