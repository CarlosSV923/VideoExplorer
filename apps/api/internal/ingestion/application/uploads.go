// Package application holds the ingestion use cases and the ports they need.
package application

import (
	"context"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// Admission is a new upload as the browser declared it, after the tus adapter
// checked its size and the room the library has for it.
type Admission struct {
	Meta domain.UploadMeta
	Size int64
}

// Uploads is what the tus adapter asks of the ingestion context. The service
// implements it: it registers each upload as a job before its bytes are accepted
// and hears how the upload ends (RF-10a).
type Uploads interface {
	// Admit checks the batch the upload names and registers its job in uploading.
	// It returns the job id, which becomes the upload's tus id. It returns
	// domain.ErrBatchNotFound for a batch that does not exist.
	Admit(ctx context.Context, a Admission) (domain.JobID, error)
	// Completed records that every byte of the upload arrived (uploaded).
	Completed(ctx context.Context, id domain.JobID) error
	// Terminated records that the browser deleted the upload (cancelled).
	Terminated(ctx context.Context, id domain.JobID) error
}
