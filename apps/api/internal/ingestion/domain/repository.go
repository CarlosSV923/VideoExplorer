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
	// Exists reports whether a job has the id.
	Exists(ctx context.Context, id JobID) (bool, error)
}
