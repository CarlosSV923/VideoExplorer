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

// Library moves the content a batch kept into No asignados (RF-08, RF-08a). The catalog
// context implements it, with the unassigned section.
type Library interface {
	// PutAside moves the files of an entry into No asignados and returns the place of each
	// file, by its source. A call that fails can be repeated: it goes on from what it recorded.
	PutAside(ctx context.Context, req AsideRequest) (map[string]string, error)
}

// AsideRequest is the content of one entry of No asignados.
type AsideRequest struct {
	// Source is what the move is for: a repeated request with it goes to the same entry.
	Source string
	// Name is the name before sanitizing (RF-08); Loose keeps a single file as it is.
	Name  string
	Loose bool
	// BatchID and Relation are the batch the entry came from (RF-08a).
	BatchID  string
	Relation string
	Files    []AsideFile
}

// AsideFile is one file of an AsideRequest.
type AsideFile struct {
	// Source is the file's path in staging, and Rel its place in the entry ("/" separators).
	Source string
	Rel    string
	Size   int64
	// Video says the file is a video, which is read again in its new place (RF-24).
	Video bool
}
