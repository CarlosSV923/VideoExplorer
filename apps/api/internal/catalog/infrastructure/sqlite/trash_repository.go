// Package sqlite implements the catalog repositories on the shared SQLite
// database. The typed queries come from queries/ through sqlc (`task gen:sql`).
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite/sqlcgen"
)

// timeLayout is fixed-width UTC, so string comparison in SQL equals time order.
const timeLayout = "2006-01-02T15:04:05.000000000Z"

// TrashRepository implements application.TrashRepository. The entries are written by the
// commit of the change that trashes them (see records).
type TrashRepository struct {
	q *sqlcgen.Queries
}

var _ application.TrashRepository = (*TrashRepository)(nil)

// NewTrashRepository builds the repository over an open database.
func NewTrashRepository(db *sql.DB) *TrashRepository {
	return &TrashRepository{q: sqlcgen.New(db)}
}

// Entries implements application.TrashRepository.
func (r *TrashRepository) Entries(ctx context.Context) ([]domain.TrashEntry, error) {
	rows, err := r.q.ListTrashEntries(ctx)
	if err != nil {
		return nil, fmt.Errorf("catalog: list trash entries: %w", err)
	}
	files, err := r.q.ListTrashFiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("catalog: list trash files: %w", err)
	}
	byEntry := make(map[int64][]domain.TrashFile, len(rows))
	for _, f := range files {
		byEntry[f.EntryID] = append(byEntry[f.EntryID], domain.TrashFile{Name: f.Name, Size: f.Size})
	}
	out := make([]domain.TrashEntry, 0, len(rows))
	for _, row := range rows {
		at, err := time.Parse(timeLayout, row.TrashedAt)
		if err != nil {
			return nil, fmt.Errorf("catalog: trash entry %d: %w", row.ID, err)
		}
		out = append(out, domain.TrashEntry{
			ID:           row.ID,
			Origin:       domain.TrashOrigin(row.Origin),
			Reason:       domain.TrashReason(row.Reason),
			Dir:          row.Dir,
			TrashedAt:    at,
			BatchID:      row.BatchID,
			RelativePath: row.RelativePath,
			Files:        byEntry[row.ID],
		})
	}
	return out, nil
}

func formatTime(t time.Time) string { return t.UTC().Format(timeLayout) }
