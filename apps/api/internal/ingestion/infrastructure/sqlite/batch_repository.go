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

// BatchRepository implements domain.BatchRepository.
type BatchRepository struct {
	q *sqlcgen.Queries
}

var _ domain.BatchRepository = (*BatchRepository)(nil)

// NewBatchRepository builds the repository over an open database.
func NewBatchRepository(db *sql.DB) *BatchRepository {
	return &BatchRepository{q: sqlcgen.New(db)}
}

// Create implements domain.BatchRepository.
func (r *BatchRepository) Create(ctx context.Context, b *domain.Batch) error {
	err := r.q.CreateBatch(ctx, sqlcgen.CreateBatchParams{
		ID:        string(b.ID),
		Name:      b.Name,
		Relation:  string(b.Relation),
		CreatedAt: formatTime(b.CreatedAt),
	})
	if err != nil {
		return fmt.Errorf("ingestion: create batch: %w", err)
	}
	return nil
}

// Get implements domain.BatchRepository.
func (r *BatchRepository) Get(ctx context.Context, id domain.BatchID) (*domain.Batch, error) {
	row, err := r.q.GetBatch(ctx, string(id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrBatchNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("ingestion: get batch: %w", err)
	}
	created, err := time.Parse(timeLayout, row.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("ingestion: batch %s: %w", row.ID, err)
	}
	return &domain.Batch{
		ID:        domain.BatchID(row.ID),
		Name:      row.Name,
		Relation:  domain.Relation(row.Relation),
		CreatedAt: created,
	}, nil
}
