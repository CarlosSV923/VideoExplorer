package application

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// Service implements the ingestion use cases on the batch and job repositories.
type Service struct {
	batches domain.BatchRepository
	jobs    domain.JobRepository
	now     func() time.Time
	newID   func() string
}

var _ Uploads = (*Service)(nil)

// NewService builds the service. now may be nil (time.Now).
func NewService(batches domain.BatchRepository, jobs domain.JobRepository, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{batches: batches, jobs: jobs, now: now, newID: randomID}
}

// randomID returns 16 random bytes in URL-safe base64 (22 characters), which
// is valid both as a batch id and as a tus upload id.
func randomID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}

// CreateBatch starts a batch of files dropped together (RF-03). The name is
// provisional and may be empty.
func (s *Service) CreateBatch(ctx context.Context, name string, relation domain.Relation) (*domain.Batch, error) {
	batch, err := domain.NewBatch(domain.BatchID(s.newID()), name, relation, s.now())
	if err != nil {
		return nil, err
	}
	if err := s.batches.Create(ctx, batch); err != nil {
		return nil, fmt.Errorf("create batch: %w", err)
	}
	return batch, nil
}

// Admit implements Uploads. An upload that names a batch must name one that
// exists (domain.ErrBatchNotFound otherwise); an upload without batch is a job
// without batch.
func (s *Service) Admit(ctx context.Context, a Admission) (domain.JobID, error) {
	if a.Meta.BatchID != "" {
		if _, err := s.batches.Get(ctx, domain.BatchID(a.Meta.BatchID)); err != nil {
			return "", err
		}
	}
	job, err := domain.NewUploadJob(domain.JobID(s.newID()), a.Meta, a.Size, s.now())
	if err != nil {
		return "", err
	}
	if err := s.jobs.Create(ctx, job); err != nil {
		return "", fmt.Errorf("create upload job: %w", err)
	}
	return job.ID, nil
}

// Completed implements Uploads.
func (s *Service) Completed(ctx context.Context, id domain.JobID) error {
	job, err := s.jobs.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := job.MarkUploaded(s.now()); err != nil {
		return err
	}
	return s.jobs.Save(ctx, job)
}

// Terminated implements Uploads. A job that already ended keeps its state.
func (s *Service) Terminated(ctx context.Context, id domain.JobID) error {
	job, err := s.jobs.Get(ctx, id)
	if err != nil {
		return err
	}
	if job.Status.Terminal() {
		return nil
	}
	if err := job.Cancel(s.now()); err != nil {
		return err
	}
	return s.jobs.Save(ctx, job)
}

// Job returns one upload job.
func (s *Service) Job(ctx context.Context, id domain.JobID) (*domain.UploadJob, error) {
	return s.jobs.Get(ctx, id)
}
