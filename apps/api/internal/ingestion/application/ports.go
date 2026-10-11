package application

import (
	"context"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// StagedUpload is what the staging area holds for one upload.
type StagedUpload struct {
	// Exists is false when the upload has no staged data: its file is missing or was never written.
	Exists bool
	// Size is the size the upload declared; Offset is how many bytes it holds.
	Size, Offset int64
}

// Staging is the area where uploads wait until the library takes them (RF-02,
// RF-09a). The tus adapter implements it.
type Staging interface {
	// Inspect reports the staged data of an upload.
	Inspect(ctx context.Context, id domain.JobID) (StagedUpload, error)
	// Remove deletes everything staged for an upload, partial or complete.
	// Removing what is not there is not an error.
	Remove(ctx context.Context, id domain.JobID) error
	// IDs lists the upload ids that have anything in the staging area.
	IDs(ctx context.Context) ([]domain.JobID, error)
}

// Publisher broadcasts job changes to the event stream (RF-10b). Publish never blocks.
type Publisher interface {
	Publish(view domain.JobView)
}
