package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite/sqlcgen"
)

// JobRepository implements domain.JobRepository.
type JobRepository struct {
	q *sqlcgen.Queries
}

var _ domain.JobRepository = (*JobRepository)(nil)

// NewJobRepository builds the repository over an open database.
func NewJobRepository(db *sql.DB) *JobRepository {
	return &JobRepository{q: sqlcgen.New(db)}
}

// Create implements domain.JobRepository.
func (r *JobRepository) Create(ctx context.Context, j *domain.UploadJob) error {
	err := r.q.CreateUploadJob(ctx, sqlcgen.CreateUploadJobParams{
		ID:           string(j.ID),
		BatchID:      string(j.BatchID),
		FileName:     j.FileName,
		RelativePath: j.RelativePath,
		Size:         j.Size,
		Status:       string(j.Status),
		Error:        j.Error,
		CreatedAt:    formatTime(j.CreatedAt),
		UpdatedAt:    formatTime(j.UpdatedAt),
	})
	if err != nil {
		return fmt.Errorf("ingestion: create upload job: %w", err)
	}
	return nil
}

// Get implements domain.JobRepository.
func (r *JobRepository) Get(ctx context.Context, id domain.JobID) (*domain.UploadJob, error) {
	row, err := r.q.GetUploadJob(ctx, string(id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrJobNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("ingestion: get upload job: %w", err)
	}
	created, err1 := time.Parse(timeLayout, row.CreatedAt)
	updated, err2 := time.Parse(timeLayout, row.UpdatedAt)
	if err := errors.Join(err1, err2); err != nil {
		return nil, fmt.Errorf("ingestion: upload job %s: %w", row.ID, err)
	}
	return &domain.UploadJob{
		ID:           domain.JobID(row.ID),
		BatchID:      domain.BatchID(row.BatchID),
		FileName:     row.FileName,
		RelativePath: row.RelativePath,
		Size:         row.Size,
		Status:       domain.Status(row.Status),
		Error:        row.Error,
		CreatedAt:    created,
		UpdatedAt:    updated,
	}, nil
}

// Save implements domain.JobRepository. Only the state changes after creation.
func (r *JobRepository) Save(ctx context.Context, j *domain.UploadJob) error {
	err := r.q.SaveUploadJob(ctx, sqlcgen.SaveUploadJobParams{
		Status:    string(j.Status),
		Error:     j.Error,
		UpdatedAt: formatTime(j.UpdatedAt),
		ID:        string(j.ID),
	})
	if err != nil {
		return fmt.Errorf("ingestion: save upload job: %w", err)
	}
	return nil
}
