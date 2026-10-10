// Package tus adapts tusd (the reference tus server, spec RF-02) to the
// ingestion context: resumable uploads of any size, streamed straight to disk
// and never held in memory (RNF-01).
package tus

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/tus/tusd/v2/pkg/filelocker"
	"github.com/tus/tusd/v2/pkg/filestore"
	tusd "github.com/tus/tusd/v2/pkg/handler"
	expslog "golang.org/x/exp/slog"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// Keys of the Upload-Metadata that the browser sends with every upload.
const (
	MetaFileName     = "filename"
	MetaRelativePath = "relativePath"
	MetaBatchID      = "batchId"
)

// FreeSpaceFunc reports the free bytes of the file system that holds path.
type FreeSpaceFunc func(path string) (uint64, error)

// Adapter owns the tusd handler and its file store.
type Adapter struct {
	dir     string
	handler *tusd.Handler
	next    application.UploadCompletedHandler
	log     *slog.Logger
}

// New creates the store in dir and the tus handler served under basePath.
// dir must be on the library's file system, so that a finished upload can be
// moved into the library with a rename (RNF-01, RNF-04). next receives each
// finished upload; it may be nil, and then finished uploads are only dropped
// from the event queue. log may be nil.
func New(dir, basePath string, log *slog.Logger, freeSpace FreeSpaceFunc, next application.UploadCompletedHandler) (*Adapter, error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("tus: create staging dir: %w", err)
	}
	store := filestore.New(dir)

	composer := tusd.NewStoreComposer()
	store.UseIn(composer)
	filelocker.New(dir).UseIn(composer)

	h, err := tusd.NewHandler(tusd.Config{
		BasePath:              basePath,
		StoreComposer:         composer,
		NotifyCompleteUploads: true,
		DisableDownload:       true, // finished files are served by the catalog, never by tus
		DisableConcatenation:  true,
		// tusd logs every request at info level; keep only what needs attention.
		Logger: tusLogger(log),
		PreUploadCreateCallback: func(ev tusd.HookEvent) (tusd.HTTPResponse, tusd.FileInfoChanges, error) {
			if err := checkNewUpload(ev.Upload, dir, freeSpace, log); err != nil {
				return tusd.HTTPResponse{}, tusd.FileInfoChanges{}, err
			}
			return tusd.HTTPResponse{}, tusd.FileInfoChanges{}, nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("tus: %w", err)
	}
	return &Adapter{dir: dir, handler: h, next: next, log: log}, nil
}

// checkNewUpload refuses an upload before any byte is accepted: its form data
// must be valid, its size must be declared, and the library must have room
// for it with the margin of RF-05 (answered as 507).
func checkNewUpload(info tusd.FileInfo, dir string, freeSpace FreeSpaceFunc, log *slog.Logger) error {
	if _, err := parseMeta(info.MetaData); err != nil {
		return tusd.NewError("ERR_INVALID_METADATA", err.Error(), http.StatusBadRequest)
	}
	if info.SizeIsDeferred {
		return tusd.NewError("ERR_SIZE_REQUIRED", "the upload size must be declared up front in Upload-Length", http.StatusBadRequest)
	}
	if freeSpace == nil {
		return nil
	}
	free, err := freeSpace(dir)
	if err != nil {
		// Like GameExplorer: when the file system cannot be measured, the upload
		// goes ahead and the failure is logged. The write itself will still fail
		// loudly if the disk is full.
		log.Warn("free space of the library not measured", "error", err)
		return nil
	}
	if err := domain.CheckFreeSpace(info.Size, free); err != nil {
		return tusd.NewError("ERR_INSUFFICIENT_STORAGE", err.Error(), http.StatusInsufficientStorage)
	}
	return nil
}

// parseMeta reads the form data of an upload from its Upload-Metadata.
func parseMeta(m map[string]string) (domain.UploadMeta, error) {
	return domain.NewUploadMeta(m[MetaFileName], m[MetaRelativePath], m[MetaBatchID])
}

// Handler is the tus HTTP endpoint. Mount it at basePath with http.StripPrefix.
func (a *Adapter) Handler() http.Handler { return a.handler }

// Run hands every finished upload to the next handler until ctx ends. It must
// run for the adapter to work: tusd blocks the request that finishes an upload
// until the event is received.
func (a *Adapter) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-a.handler.CompleteUploads:
			if a.next == nil {
				continue
			}
			if err := a.complete(ctx, ev.Upload); err != nil && ctx.Err() == nil {
				a.log.Error("finished upload not handled", "upload", ev.Upload.ID, "error", err)
			}
		}
	}
}

func (a *Adapter) complete(ctx context.Context, info tusd.FileInfo) error {
	meta, err := parseMeta(info.MetaData)
	if err != nil {
		return err
	}
	return a.next.HandleUploadCompleted(ctx, application.CompletedUpload{
		ID:       info.ID,
		Size:     info.Size,
		Meta:     meta,
		StoredAt: filepath.Join(a.dir, filepath.Base(info.ID)),
	})
}

// tusLogger returns the logger tusd writes to. tusd v2 takes the logger of
// golang.org/x/exp/slog and logs every request at info level, so this bridge
// forwards only warnings and errors to the application's standard logger.
func tusLogger(log *slog.Logger) *expslog.Logger {
	return expslog.New(warningsOnly{next: log.Handler().WithAttrs([]slog.Attr{slog.String("component", "tus")})})
}

// warningsOnly is an x/exp/slog handler that forwards warnings and above to a
// standard library handler.
type warningsOnly struct{ next slog.Handler }

func (w warningsOnly) Enabled(_ context.Context, level expslog.Level) bool {
	return level >= expslog.LevelWarn
}

func (w warningsOnly) Handle(ctx context.Context, r expslog.Record) error {
	std := slog.NewRecord(r.Time, slog.Level(r.Level), r.Message, r.PC)
	r.Attrs(func(a expslog.Attr) bool {
		std.AddAttrs(slog.Any(a.Key, a.Value.Any()))
		return true
	})
	return w.next.Handle(ctx, std)
}

func (w warningsOnly) WithAttrs(attrs []expslog.Attr) expslog.Handler {
	return warningsOnly{next: w.next.WithAttrs(stdAttrs(attrs))}
}

func (w warningsOnly) WithGroup(name string) expslog.Handler {
	return warningsOnly{next: w.next.WithGroup(name)}
}

func stdAttrs(attrs []expslog.Attr) []slog.Attr {
	out := make([]slog.Attr, 0, len(attrs))
	for _, a := range attrs {
		out = append(out, slog.Any(a.Key, a.Value.Any()))
	}
	return out
}
