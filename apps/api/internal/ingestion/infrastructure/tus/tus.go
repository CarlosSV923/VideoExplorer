// Package tus adapts tusd (the reference tus server, spec RF-02) to the
// ingestion context: resumable uploads of any size, streamed straight to disk
// and never held in memory (RNF-01).
package tus

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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
	dir       string
	store     filestore.FileStore
	handler   *tusd.Handler
	freeSpace FreeSpaceFunc
	uploads   application.Uploads
	log       *slog.Logger
}

// New creates the store in dir and the tus handler served under basePath.
// dir must be on the library's file system, so that a finished upload can be
// moved into the library with a rename (RNF-01, RNF-04). Every new upload is
// admitted through uploads, which registers its job and gives the upload its
// id; finished and deleted uploads are reported to it. freeSpace may be nil
// and log may be nil.
func New(dir, basePath string, log *slog.Logger, freeSpace FreeSpaceFunc, uploads application.Uploads) (*Adapter, error) {
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

	a := &Adapter{dir: dir, store: store, freeSpace: freeSpace, uploads: uploads, log: log}
	h, err := tusd.NewHandler(tusd.Config{
		BasePath:                basePath,
		StoreComposer:           composer,
		NotifyTerminatedUploads: true,
		DisableDownload:         true, // finished files are served by the catalog, never by tus
		DisableConcatenation:    true,
		// tusd logs every request at info level; keep only what needs attention.
		Logger: tusLogger(log),
		PreUploadCreateCallback: func(ev tusd.HookEvent) (tusd.HTTPResponse, tusd.FileInfoChanges, error) {
			id, err := a.admit(ev.Context, ev.Upload)
			if err != nil {
				return tusd.HTTPResponse{}, tusd.FileInfoChanges{}, err
			}
			// The upload takes the id of its job, so the browser and the server name it the same way.
			return tusd.HTTPResponse{}, tusd.FileInfoChanges{ID: string(id)}, nil
		},
		// The job is marked uploaded before the last chunk is answered, so the
		// browser never sees a finished upload whose job is still uploading.
		PreFinishResponseCallback: func(ev tusd.HookEvent) (tusd.HTTPResponse, error) {
			if err := uploads.Completed(ev.Context, domain.JobID(ev.Upload.ID)); err != nil {
				return tusd.HTTPResponse{}, fmt.Errorf("tus: record finished upload: %w", err)
			}
			return tusd.HTTPResponse{}, nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("tus: %w", err)
	}
	a.handler = h
	return a, nil
}

// admit refuses an upload before any byte is accepted, or registers it. The
// form data must be valid, the size declared and not zero, the library must
// have room for it with the margin of RF-05 and the bytes of the uploads in
// progress reserved (RF-09a), and the batch it names, which is required, must exist.
func (a *Adapter) admit(ctx context.Context, info tusd.FileInfo) (domain.JobID, error) {
	meta, err := parseMeta(info.MetaData)
	if err != nil {
		return "", tusd.NewError("ERR_INVALID_METADATA", err.Error(), http.StatusBadRequest)
	}
	if info.SizeIsDeferred {
		return "", tusd.NewError("ERR_SIZE_REQUIRED", "the upload size must be declared up front in Upload-Length", http.StatusBadRequest)
	}
	if info.Size == 0 {
		return "", tusd.NewError("ERR_EMPTY_FILE", "an empty file cannot be uploaded", http.StatusBadRequest)
	}
	if err := a.checkRoom(ctx, info.Size); err != nil {
		return "", err
	}
	id, err := a.uploads.Admit(ctx, application.Admission{Meta: meta, Size: info.Size})
	if errors.Is(err, domain.ErrBatchNotFound) {
		return "", tusd.NewError("ERR_INVALID_METADATA", "batchId does not name a batch of this library", http.StatusBadRequest)
	}
	if err != nil {
		return "", fmt.Errorf("tus: admit upload: %w", err)
	}
	return id, nil
}

// checkRoom answers 507 when the library cannot take size bytes with the margin
// of RF-05, counting the bytes that uploads in progress still have to receive
// (RF-09a). When the free space cannot be measured, the upload goes ahead and
// the failure is logged, as in GameExplorer.
func (a *Adapter) checkRoom(ctx context.Context, size int64) error {
	if a.freeSpace == nil {
		return nil
	}
	free, err := a.freeSpace(a.dir)
	if err != nil {
		a.log.Warn("free space of the library not measured", "error", err)
		return nil
	}
	pending, err := a.pendingBytes(ctx)
	if err != nil {
		return fmt.Errorf("tus: bytes of uploads in progress: %w", err)
	}
	if err := domain.CheckFreeSpace(size, free, pending); err != nil {
		return tusd.NewError("ERR_INSUFFICIENT_STORAGE", err.Error(), http.StatusInsufficientStorage)
	}
	return nil
}

// pendingBytes sums the bytes that the uploads in the staging folder still have
// to receive. A finished upload contributes nothing; an upload that was removed
// while the sum was taken is skipped.
func (a *Adapter) pendingBytes(ctx context.Context) (uint64, error) {
	infos, err := filepath.Glob(filepath.Join(a.dir, "*.info"))
	if err != nil {
		return 0, err
	}
	var pending uint64
	for _, path := range infos {
		id := strings.TrimSuffix(filepath.Base(path), ".info")
		up, err := a.store.GetUpload(ctx, id)
		if errors.Is(err, tusd.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return 0, err
		}
		info, err := up.GetInfo(ctx)
		if errors.Is(err, tusd.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return 0, err
		}
		pending += uint64(domain.PendingBytes(info.Size, info.Offset)) //nolint:gosec // never negative, see PendingBytes
	}
	return pending, nil
}

// parseMeta reads the form data of an upload from its Upload-Metadata.
func parseMeta(m map[string]string) (domain.UploadMeta, error) {
	return domain.NewUploadMeta(m[MetaFileName], m[MetaRelativePath], m[MetaBatchID])
}

// Handler is the tus HTTP endpoint. Mount it at basePath with http.StripPrefix.
func (a *Adapter) Handler() http.Handler { return a.handler }

// Run reports the deleted uploads to the ingestion context until ctx ends. It
// must run for the adapter to work: tusd blocks the request that deletes an
// upload until the event is received. Finished uploads are reported before
// their last chunk is answered (see New), not here.
func (a *Adapter) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-a.handler.TerminatedUploads:
			id := ev.Upload.ID
			if err := a.uploads.Terminated(ctx, domain.JobID(id)); err != nil && ctx.Err() == nil {
				a.log.Error("deleted upload not recorded", "upload", id, "error", err)
			}
		}
	}
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
