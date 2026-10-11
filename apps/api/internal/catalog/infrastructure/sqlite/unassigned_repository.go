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

// UnassignedRepository implements application.UnassignedRepository.
type UnassignedRepository struct {
	db *sql.DB
	q  *sqlcgen.Queries
}

var _ application.UnassignedRepository = (*UnassignedRepository)(nil)

// NewUnassignedRepository builds the repository over an open database.
func NewUnassignedRepository(db *sql.DB) *UnassignedRepository {
	return &UnassignedRepository{db: db, q: sqlcgen.New(db)}
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
		entry.Files = append(entry.Files, domain.UnassignedFile{Source: f.Source, Rel: f.Rel, Size: f.Size, Moved: f.Moved != 0})
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

// Record implements application.UnassignedRepository. The entry and its files are
// stored in one transaction, so a failure leaves neither of them behind.
func (r *UnassignedRepository) Record(ctx context.Context, entry domain.UnassignedEntry) (domain.UnassignedEntry, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.UnassignedEntry{}, fmt.Errorf("catalog: begin unassigned entry: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	q := r.q.WithTx(tx)
	id, err := q.InsertUnassignedEntry(ctx, sqlcgen.InsertUnassignedEntryParams{
		Source:    entry.Source,
		Name:      entry.Name,
		Loose:     boolInt(entry.Loose),
		BatchID:   entry.BatchID,
		Relation:  entry.Relation,
		Origin:    entry.Origin,
		CreatedAt: formatTime(entry.CreatedAt),
	})
	if err != nil {
		return domain.UnassignedEntry{}, fmt.Errorf("catalog: insert unassigned entry: %w", err)
	}
	if err := insertFiles(ctx, q, id, entry.Files); err != nil {
		return domain.UnassignedEntry{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.UnassignedEntry{}, fmt.Errorf("catalog: commit unassigned entry: %w", err)
	}
	entry.ID = id
	return entry, nil
}

// AddFiles implements application.UnassignedRepository.
func (r *UnassignedRepository) AddFiles(ctx context.Context, entryID int64, files []domain.UnassignedFile) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("catalog: begin unassigned files: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := insertFiles(ctx, r.q.WithTx(tx), entryID, files); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("catalog: commit unassigned files: %w", err)
	}
	return nil
}

// MarkMoved implements application.UnassignedRepository.
func (r *UnassignedRepository) MarkMoved(ctx context.Context, entryID int64, source string) error {
	if err := r.q.MarkUnassignedFileMoved(ctx, sqlcgen.MarkUnassignedFileMovedParams{EntryID: entryID, Source: source}); err != nil {
		return fmt.Errorf("catalog: mark unassigned file moved: %w", err)
	}
	return nil
}

// insertFiles stores the files of an entry, as the queries of the transaction q.
func insertFiles(ctx context.Context, q *sqlcgen.Queries, entryID int64, files []domain.UnassignedFile) error {
	for _, f := range files {
		if err := q.InsertUnassignedFile(ctx, sqlcgen.InsertUnassignedFileParams{
			EntryID: entryID, Source: f.Source, Rel: f.Rel, Size: f.Size, Moved: boolInt(f.Moved),
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
