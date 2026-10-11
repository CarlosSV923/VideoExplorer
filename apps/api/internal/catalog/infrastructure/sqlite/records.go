package sqlite

import (
	"context"
	"fmt"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite/sqlcgen"
)

// records implements application.Records inside the transaction of a commit.
type records struct {
	q *sqlcgen.Queries
}

var _ application.Records = records{}

// InsertTrash implements application.Records.
func (r records) InsertTrash(ctx context.Context, entry domain.TrashEntry) (int64, error) {
	id, err := r.q.InsertTrashEntry(ctx, sqlcgen.InsertTrashEntryParams{
		Origin:       string(entry.Origin),
		Reason:       string(entry.Reason),
		Dir:          entry.Dir,
		TrashedAt:    formatTime(entry.TrashedAt),
		BatchID:      entry.BatchID,
		RelativePath: entry.RelativePath,
	})
	if err != nil {
		return 0, fmt.Errorf("catalog: insert trash entry: %w", err)
	}
	for _, f := range entry.Files {
		if err := r.q.InsertTrashFile(ctx, sqlcgen.InsertTrashFileParams{EntryID: id, Name: f.Name, Size: f.Size}); err != nil {
			return 0, fmt.Errorf("catalog: insert trash file: %w", err)
		}
	}
	return id, nil
}

// InsertUnassigned implements application.Records.
func (r records) InsertUnassigned(ctx context.Context, entry domain.UnassignedEntry) (int64, error) {
	id, err := r.q.InsertUnassignedEntry(ctx, sqlcgen.InsertUnassignedEntryParams{
		Source:    entry.Source,
		Name:      entry.Name,
		Loose:     boolInt(entry.Loose),
		BatchID:   entry.BatchID,
		Relation:  entry.Relation,
		Origin:    entry.Origin,
		CreatedAt: formatTime(entry.CreatedAt),
	})
	if err != nil {
		return 0, fmt.Errorf("catalog: insert unassigned entry: %w", err)
	}
	if err := insertFiles(ctx, r.q, id, entry.Files); err != nil {
		return 0, err
	}
	return id, nil
}

// AddUnassignedFiles implements application.Records.
func (r records) AddUnassignedFiles(ctx context.Context, entryID int64, files []domain.UnassignedFile) error {
	return insertFiles(ctx, r.q, entryID, files)
}

// insertFiles stores the files of an entry, as the queries of the transaction q.
func insertFiles(ctx context.Context, q *sqlcgen.Queries, entryID int64, files []domain.UnassignedFile) error {
	for _, f := range files {
		if err := q.InsertUnassignedFile(ctx, sqlcgen.InsertUnassignedFileParams{
			EntryID: entryID, Source: f.Source, Rel: f.Rel, Size: f.Size,
		}); err != nil {
			return fmt.Errorf("catalog: insert unassigned file: %w", err)
		}
	}
	return nil
}

// boolInt stores a flag as the 0 or 1 the schema checks.
func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
