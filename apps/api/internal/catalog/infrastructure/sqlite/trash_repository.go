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

// TrashRepository implements application.TrashRepository.
type TrashRepository struct {
	db *sql.DB
	q  *sqlcgen.Queries
}

var _ application.TrashRepository = (*TrashRepository)(nil)

// NewTrashRepository builds the repository over an open database.
func NewTrashRepository(db *sql.DB) *TrashRepository {
	return &TrashRepository{db: db, q: sqlcgen.New(db)}
}

// Insert implements application.TrashRepository. The entry and its files are
// stored in one transaction.
func (r *TrashRepository) Insert(ctx context.Context, entry domain.TrashEntry) (domain.TrashEntry, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.TrashEntry{}, fmt.Errorf("catalog: begin trash entry: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	q := r.q.WithTx(tx)
	id, err := q.InsertTrashEntry(ctx, sqlcgen.InsertTrashEntryParams{
		Origin:       string(entry.Origin),
		Reason:       string(entry.Reason),
		Dir:          entry.Dir,
		TrashedAt:    formatTime(entry.TrashedAt),
		BatchID:      entry.BatchID,
		RelativePath: entry.RelativePath,
	})
	if err != nil {
		return domain.TrashEntry{}, fmt.Errorf("catalog: insert trash entry: %w", err)
	}
	for _, f := range entry.Files {
		if err := q.InsertTrashFile(ctx, sqlcgen.InsertTrashFileParams{EntryID: id, Name: f.Name, Size: f.Size}); err != nil {
			return domain.TrashEntry{}, fmt.Errorf("catalog: insert trash file: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.TrashEntry{}, fmt.Errorf("catalog: commit trash entry: %w", err)
	}
	entry.ID = id
	return entry, nil
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
