package domain

import "context"

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
	// Save stores the state of an existing job (status, error and update time).
	Save(ctx context.Context, job *UploadJob) error
}
