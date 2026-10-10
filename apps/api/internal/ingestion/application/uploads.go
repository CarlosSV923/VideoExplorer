// Package application holds the ingestion use cases and the ports they need.
package application

import (
	"context"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// CompletedUpload is a finished upload, handed to the next step of ingestion.
// It is the event the batch and job model (phase 3, task 2) will build on.
type CompletedUpload struct {
	// ID is the upload id given by the tus store. The server builds every path
	// from it; the browser never sends a path.
	ID string
	// Size is the number of bytes received, equal to the declared length.
	Size int64
	// Meta is the form data the upload declared, already checked.
	Meta domain.UploadMeta
	// StoredAt is the data file in the staging folder, built on the server.
	StoredAt string
}

// UploadCompletedHandler receives every finished upload, in arrival order. The
// tus adapter calls it once per upload; it must not keep the file open.
type UploadCompletedHandler interface {
	HandleUploadCompleted(ctx context.Context, upload CompletedUpload) error
}
