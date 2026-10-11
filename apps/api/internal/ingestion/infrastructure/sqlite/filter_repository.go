package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite/sqlcgen"
)

// FilterRepository implements domain.FilterRepository.
type FilterRepository struct {
	db *sql.DB
	q  *sqlcgen.Queries
}

var _ domain.FilterRepository = (*FilterRepository)(nil)

// NewFilterRepository builds the repository over an open database.
func NewFilterRepository(db *sql.DB) *FilterRepository {
	return &FilterRepository{db: db, q: sqlcgen.New(db)}
}

// Entries implements domain.FilterRepository.
func (r *FilterRepository) Entries(ctx context.Context, unit domain.JobID) ([]domain.FilterEntry, error) {
	rows, err := r.q.ListFilterFiles(ctx, string(unit))
	if err != nil {
		return nil, fmt.Errorf("ingestion: list filter entries: %w", err)
	}
	out := make([]domain.FilterEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.FilterEntry{
			Path:    row.Path,
			Size:    row.Size,
			Kind:    domain.FileKind(row.Kind),
			Trashed: row.Trashed != 0,
		})
	}
	return out, nil
}

// Record implements domain.FilterRepository. The entries are stored in one
// transaction, and a path already recorded keeps its entry.
func (r *FilterRepository) Record(ctx context.Context, unit domain.JobID, entries []domain.FilterEntry) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ingestion: begin filter entries: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	q := r.q.WithTx(tx)
	for _, e := range entries {
		if err := q.InsertFilterFile(ctx, sqlcgen.InsertFilterFileParams{
			JobID:   string(unit),
			Path:    e.Path,
			Size:    e.Size,
			Kind:    string(e.Kind),
			Trashed: boolInt(e.Trashed),
		}); err != nil {
			return fmt.Errorf("ingestion: record filter entry: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ingestion: commit filter entries: %w", err)
	}
	return nil
}

// MarkTrashed implements domain.FilterRepository.
func (r *FilterRepository) MarkTrashed(ctx context.Context, unit domain.JobID, path string) error {
	if err := r.q.MarkFilterFileTrashed(ctx, sqlcgen.MarkFilterFileTrashedParams{JobID: string(unit), Path: path}); err != nil {
		return fmt.Errorf("ingestion: mark filter entry trashed: %w", err)
	}
	return nil
}
