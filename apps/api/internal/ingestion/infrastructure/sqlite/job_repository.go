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
		PartsTotal:   int64(j.PartsTotal),
		VolumeSet:    j.VolumeSet,
		VolumeIndex:  int64(j.VolumeIndex),
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
	return toDomain(jobRow{
		ID: row.ID, BatchID: row.BatchID, FileName: row.FileName, RelativePath: row.RelativePath,
		Size: row.Size, Received: row.Received, Progress: row.Progress, Status: row.Status, Error: row.Error,
		PartsTotal: row.PartsTotal, VolumeSet: row.VolumeSet, VolumeIndex: row.VolumeIndex, VolumeGroup: row.VolumeGroup,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	})
}

// Save implements domain.JobRepository. The job's identity and declared values never change after creation.
func (r *JobRepository) Save(ctx context.Context, j *domain.UploadJob) error {
	err := r.q.SaveUploadJob(ctx, sqlcgen.SaveUploadJobParams{
		Status:      string(j.Status),
		Error:       string(j.Error),
		Received:    j.Received,
		Progress:    int64(j.Progress),
		VolumeGroup: j.VolumeGroup,
		UpdatedAt:   formatTime(j.UpdatedAt),
		ID:          string(j.ID),
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
		job, err := toDomain(jobRow{
			ID: row.ID, BatchID: row.BatchID, FileName: row.FileName, RelativePath: row.RelativePath,
			Size: row.Size, Received: row.Received, Progress: row.Progress, Status: row.Status, Error: row.Error,
			PartsTotal: row.PartsTotal, VolumeSet: row.VolumeSet, VolumeIndex: row.VolumeIndex, VolumeGroup: row.VolumeGroup,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		})
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

// ByVolumeGroup implements domain.JobRepository.
func (r *JobRepository) ByVolumeGroup(ctx context.Context, group string) ([]*domain.UploadJob, error) {
	rows, err := r.q.ListUploadJobsByVolumeGroup(ctx, group)
	if err != nil {
		return nil, fmt.Errorf("ingestion: list volumes of a set: %w", err)
	}
	return toDomainAll(rows)
}

// ByBatch implements domain.JobRepository.
func (r *JobRepository) ByBatch(ctx context.Context, batch domain.BatchID) ([]*domain.UploadJob, error) {
	rows, err := r.q.ListUploadJobsByBatch(ctx, string(batch))
	if err != nil {
		return nil, fmt.Errorf("ingestion: list jobs of a batch: %w", err)
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

// jobRow is a stored job, with the columns of every query that reads jobs.
type jobRow struct {
	ID, BatchID, FileName, RelativePath string
	Size, Received, Progress            int64
	Status, Error                       string
	PartsTotal, VolumeIndex             int64
	VolumeSet, VolumeGroup              string
	CreatedAt, UpdatedAt                string
}

func toDomainAll(rows []sqlcgen.UploadJob) ([]*domain.UploadJob, error) {
	out := make([]*domain.UploadJob, 0, len(rows))
	for _, row := range rows {
		j, err := toDomain(jobRow{
			ID: row.ID, BatchID: row.BatchID, FileName: row.FileName, RelativePath: row.RelativePath,
			Size: row.Size, Received: row.Received, Progress: row.Progress, Status: row.Status, Error: row.Error,
			PartsTotal: row.PartsTotal, VolumeSet: row.VolumeSet, VolumeIndex: row.VolumeIndex, VolumeGroup: row.VolumeGroup,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, nil
}

// toDomain maps a stored job to the domain model.
func toDomain(row jobRow) (*domain.UploadJob, error) {
	created, err1 := parseTime(row.CreatedAt)
	updated, err2 := parseTime(row.UpdatedAt)
	if err := errors.Join(err1, err2); err != nil {
		return nil, fmt.Errorf("ingestion: upload job %s: %w", row.ID, err)
	}
	return &domain.UploadJob{
		ID:           domain.JobID(row.ID),
		BatchID:      domain.BatchID(row.BatchID),
		FileName:     row.FileName,
		RelativePath: row.RelativePath,
		Size:         row.Size,
		Received:     row.Received,
		Progress:     int(row.Progress),
		Status:       domain.Status(row.Status),
		Error:        domain.FailureReason(row.Error),
		PartsTotal:   int(row.PartsTotal),
		VolumeSet:    row.VolumeSet,
		VolumeIndex:  int(row.VolumeIndex),
		VolumeGroup:  row.VolumeGroup,
		CreatedAt:    created,
		UpdatedAt:    updated,
	}, nil
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(timeLayout, s)
}
