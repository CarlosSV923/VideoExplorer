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
		Error:        string(j.Error),
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
	return toDomain(row)
}

// Save implements domain.JobRepository. The job's identity and declared values never change after creation.
func (r *JobRepository) Save(ctx context.Context, j *domain.UploadJob) error {
	err := r.q.SaveUploadJob(ctx, sqlcgen.SaveUploadJobParams{
		Status:    string(j.Status),
		Error:     string(j.Error),
		Received:  j.Received,
		UpdatedAt: formatTime(j.UpdatedAt),
		ID:        string(j.ID),
	})
	if err != nil {
		return fmt.Errorf("ingestion: save upload job: %w", err)
	}
	return nil
}

// Recent implements domain.JobRepository.
func (r *JobRepository) Recent(ctx context.Context, limit int) ([]domain.JobView, error) {
	rows, err := r.q.ListRecentUploadJobs(ctx, int64(limit))
	if err != nil {
		return nil, fmt.Errorf("ingestion: list upload jobs: %w", err)
	}
	out := make([]domain.JobView, 0, len(rows))
	for _, row := range rows {
		job, err := parseJob(row.ID, row.BatchID, row.FileName, row.RelativePath, row.Size, row.Received,
			row.Status, row.Error, row.CreatedAt, row.UpdatedAt)
		if err != nil {
			return nil, err
		}
		created, err := parseTime(row.BatchCreatedAt)
		if err != nil {
			return nil, fmt.Errorf("ingestion: batch of job %s: %w", row.ID, err)
		}
		out = append(out, domain.JobView{
			Job: *job,
			Batch: domain.Batch{
				ID:        job.BatchID,
				Name:      row.BatchName,
				Relation:  domain.Relation(row.BatchRelation),
				CreatedAt: created,
			},
		})
	}
	return out, nil
}

// ByStatus implements domain.JobRepository.
func (r *JobRepository) ByStatus(ctx context.Context, status domain.Status) ([]*domain.UploadJob, error) {
	rows, err := r.q.ListUploadJobsByStatus(ctx, string(status))
	if err != nil {
		return nil, fmt.Errorf("ingestion: list upload jobs by status: %w", err)
	}
	return toDomainAll(rows)
}

// Stale implements domain.JobRepository.
func (r *JobRepository) Stale(ctx context.Context, status domain.Status, before time.Time) ([]*domain.UploadJob, error) {
	rows, err := r.q.ListStaleUploadJobs(ctx, sqlcgen.ListStaleUploadJobsParams{
		Status:    string(status),
		UpdatedAt: formatTime(before),
	})
	if err != nil {
		return nil, fmt.Errorf("ingestion: list stale upload jobs: %w", err)
	}
	return toDomainAll(rows)
}

// Exists implements domain.JobRepository.
func (r *JobRepository) Exists(ctx context.Context, id domain.JobID) (bool, error) {
	ok, err := r.q.UploadJobExists(ctx, string(id))
	if err != nil {
		return false, fmt.Errorf("ingestion: check upload job: %w", err)
	}
	return ok, nil
}

func toDomainAll(rows []sqlcgen.UploadJob) ([]*domain.UploadJob, error) {
	out := make([]*domain.UploadJob, 0, len(rows))
	for _, row := range rows {
		j, err := toDomain(row)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, nil
}

func toDomain(row sqlcgen.UploadJob) (*domain.UploadJob, error) {
	return parseJob(row.ID, row.BatchID, row.FileName, row.RelativePath, row.Size, row.Received,
		row.Status, row.Error, row.CreatedAt, row.UpdatedAt)
}

// parseJob maps the stored columns of a job to the domain model.
func parseJob(id, batchID, fileName, relativePath string, size, received int64, status, failure, createdAt, updatedAt string) (*domain.UploadJob, error) {
	created, err1 := parseTime(createdAt)
	updated, err2 := parseTime(updatedAt)
	if err := errors.Join(err1, err2); err != nil {
		return nil, fmt.Errorf("ingestion: upload job %s: %w", id, err)
	}
	return &domain.UploadJob{
		ID:           domain.JobID(id),
		BatchID:      domain.BatchID(batchID),
		FileName:     fileName,
		RelativePath: relativePath,
		Size:         size,
		Received:     received,
		Status:       domain.Status(status),
		Error:        domain.FailureReason(failure),
		CreatedAt:    created,
		UpdatedAt:    updated,
	}, nil
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(timeLayout, s)
}
