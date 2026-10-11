package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite/sqlcgen"
)

// UnassignedRepository implements application.UnassignedRepository. The entries are written
// by the commit of the change that moves them in (see records).
type UnassignedRepository struct {
	q *sqlcgen.Queries
}

var _ application.UnassignedRepository = (*UnassignedRepository)(nil)

// NewUnassignedRepository builds the repository over an open database.
func NewUnassignedRepository(db *sql.DB) *UnassignedRepository {
	return &UnassignedRepository{q: sqlcgen.New(db)}
}

// BySource implements application.UnassignedRepository.
func (r *UnassignedRepository) BySource(ctx context.Context, source string) (domain.UnassignedEntry, bool, error) {
	row, err := r.q.GetUnassignedEntryBySource(ctx, source)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.UnassignedEntry{}, false, nil
	}
	if err != nil {
		return domain.UnassignedEntry{}, false, fmt.Errorf("catalog: read unassigned entry: %w", err)
	}
	created, err := time.Parse(timeLayout, row.CreatedAt)
	if err != nil {
		return domain.UnassignedEntry{}, false, fmt.Errorf("catalog: unassigned entry time: %w", err)
	}
	entry := domain.UnassignedEntry{
		ID: row.ID, Source: row.Source, Name: row.Name, Loose: row.Loose != 0,
		BatchID: row.BatchID, Relation: row.Relation, Origin: row.Origin, CreatedAt: created,
	}
	files, err := r.q.ListUnassignedFiles(ctx, row.ID)
	if err != nil {
		return domain.UnassignedEntry{}, false, fmt.Errorf("catalog: list unassigned files: %w", err)
	}
	for _, f := range files {
		entry.Files = append(entry.Files, domain.UnassignedFile{Source: f.Source, Rel: f.Rel, Size: f.Size})
	}
	return entry, true, nil
}

// Names implements application.UnassignedRepository.
func (r *UnassignedRepository) Names(ctx context.Context) ([]string, error) {
	names, err := r.q.ListUnassignedNames(ctx)
	if err != nil {
		return nil, fmt.Errorf("catalog: list unassigned names: %w", err)
	}
	return names, nil
}
