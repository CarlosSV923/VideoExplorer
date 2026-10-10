package application

import (
	"context"
	"io"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/domain"
)

// ImageSource downloads images from TMDB's image CDN. It is not the API: it
// needs no credential, but the app only calls it from the server, never from
// the browser (RNF-10).
type ImageSource interface {
	// Fetch returns the body of the image at the size of ref.Kind. The caller
	// closes it. domain.ErrNotFound when TMDB has no such image, domain.ErrUpstream
	// when the download fails.
	Fetch(ctx context.Context, ref domain.ImageRef) (io.ReadCloser, error)
}

// StoredImage is a cached image ready to be served. File supports seeking, so
// the HTTP layer can answer Range and conditional requests with http.ServeContent.
type StoredImage struct {
	File    io.ReadSeekCloser
	ModTime time.Time
}

// ImageStore keeps the title images on disk (spec §6). Images never expire;
// the catalog removes the ones no title uses through Prune.
type ImageStore interface {
	// Open returns the cached image of ref, downloading it first when it is not
	// cached. domain.ErrNotConfigured when it is not cached and TMDB is not
	// configured, domain.ErrNotFound when TMDB has no such image,
	// domain.ErrUpstream when the download fails.
	Open(ctx context.Context, ref domain.ImageRef) (StoredImage, error)
	// Prune deletes every cached image that is not in keep and returns how many
	// it removed. An empty keep deletes them all, so callers must pass the full
	// set of images in use.
	Prune(ctx context.Context, keep []domain.ImageRef) (removed int, err error)
}
