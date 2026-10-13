package domain

import (
	"context"
	"time"
)

// BatchRepository persists batches.
type BatchRepository interface {
	Create(ctx context.Context, batch *Batch) error
	// Get returns ErrBatchNotFound when no batch has the id.
	Get(ctx context.Context, id BatchID) (*Batch, error)
}

// JobRepository persists upload jobs.
type JobRepository interface {
	Create(ctx context.Context, job *UploadJob) error
	// Get returns ErrJobNotFound when no job has the id.
	Get(ctx context.Context, id JobID) (*UploadJob, error)
	// Save stores the state of an existing job (status, failure reason, received bytes and update time).
	Save(ctx context.Context, job *UploadJob) error
	// Recent returns the most recent jobs with their batches, newest first (RF-10b).
	Recent(ctx context.Context, limit int) ([]JobView, error)
	// ByStatus returns every job in status, oldest first.
	ByStatus(ctx context.Context, status Status) ([]*UploadJob, error)
	// Stale returns the jobs in status whose last change is before t, oldest first.
	Stale(ctx context.Context, status Status, before time.Time) ([]*UploadJob, error)
	// ByVolumeGroup returns the volumes of one claimed set, in volume order (RF-03).
	ByVolumeGroup(ctx context.Context, group string) ([]*UploadJob, error)
	// ByBatch returns every job of a batch, in the order they were admitted (RF-03).
	ByBatch(ctx context.Context, batch BatchID) ([]*UploadJob, error)
	// Unsettled reports whether a job of the batch is still on its way (uploading, waiting for
	// its parts or its password, extracting, or filtering without its filter done): the batch is
	// then not complete and cannot be decided (RF-07, RF-08a). It reads no job but the batch's
	// own, by index, whatever the size of the batch.
	Unsettled(ctx context.Context, batch BatchID) (bool, error)
	// Exists reports whether a job has the id.
	Exists(ctx context.Context, id JobID) (bool, error)
}

// FilterRepository persists what the filter found, so that a restart goes on where it
// stopped without sending a discarded file to the trash twice (RF-07). A work unit is
// keyed by the job that leads it: a single file, or the first volume of a set.
type FilterRepository interface {
	// Entries returns what filtering recorded for the unit, by path. It is empty until Record.
	Entries(ctx context.Context, unit JobID) ([]FilterEntry, error)
	// Record stores what filtering found for the unit. A path already recorded keeps
	// its entry, so recording again changes nothing.
	Record(ctx context.Context, unit JobID, entries []FilterEntry) error
	// MarkTrashed records that the discarded file at path went to the trash.
	MarkTrashed(ctx context.Context, unit JobID, path string) error
}
