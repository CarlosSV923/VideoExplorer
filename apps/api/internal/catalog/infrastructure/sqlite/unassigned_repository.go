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
// by the commit of the change that moves them in (see records); the lock of an assignment is
// written by Lock and Unlock, outside any change.
type UnassignedRepository struct {
	db *sql.DB
	q  *sqlcgen.Queries
}

var _ application.UnassignedRepository = (*UnassignedRepository)(nil)

// NewUnassignedRepository builds the repository over an open database.
func NewUnassignedRepository(db *sql.DB) *UnassignedRepository {
	return &UnassignedRepository{db: db, q: sqlcgen.New(db)}
}

// List implements application.UnassignedRepository: every entry with its files, oldest first.
func (r *UnassignedRepository) List(ctx context.Context) ([]domain.UnassignedEntry, error) {
	rows, err := r.q.ListAllUnassignedEntries(ctx)
	if err != nil {
		return nil, fmt.Errorf("catalog: list unassigned entries: %w", err)
	}
	out := make([]domain.UnassignedEntry, 0, len(rows))
	for _, row := range rows {
		entry, _, err := r.withFiles(ctx, entryFields{
			id: row.ID, source: row.Source, name: row.Name, loose: row.Loose, batchID: row.BatchID,
			relation: row.Relation, origin: row.Origin, createdAt: row.CreatedAt,
			lockToken: row.LockToken, lockUntil: row.LockUntil,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, nil
}

// Apply implements application.UnassignedRepository: fn writes its records in one transaction.
func (r *UnassignedRepository) Apply(ctx context.Context, fn func(application.Records) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("catalog: begin unassigned transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(records{q: r.q.WithTx(tx)}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("catalog: commit unassigned transaction: %w", err)
	}
	return nil
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
	return r.withFiles(ctx, entryFields{
		id: row.ID, source: row.Source, name: row.Name, loose: row.Loose, batchID: row.BatchID,
		relation: row.Relation, origin: row.Origin, createdAt: row.CreatedAt,
		lockToken: row.LockToken, lockUntil: row.LockUntil,
	})
}

// ByID implements application.UnassignedRepository.
func (r *UnassignedRepository) ByID(ctx context.Context, id int64) (domain.UnassignedEntry, bool, error) {
	row, err := r.q.GetUnassignedEntryByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.UnassignedEntry{}, false, nil
	}
	if err != nil {
		return domain.UnassignedEntry{}, false, fmt.Errorf("catalog: read unassigned entry: %w", err)
	}
	return r.withFiles(ctx, entryFields{
		id: row.ID, source: row.Source, name: row.Name, loose: row.Loose, batchID: row.BatchID,
		relation: row.Relation, origin: row.Origin, createdAt: row.CreatedAt,
		lockToken: row.LockToken, lockUntil: row.LockUntil,
	})
}

// entryFields are the columns of an entry, whichever query read them.
type entryFields struct {
	id                        int64
	source, name              string
	loose                     int64
	batchID, relation, origin string
	createdAt                 string
	lockToken                 string
	lockUntil                 int64
}

// withFiles builds the entry from its columns and reads its files.
func (r *UnassignedRepository) withFiles(ctx context.Context, f entryFields) (domain.UnassignedEntry, bool, error) {
	created, err := time.Parse(timeLayout, f.createdAt)
	if err != nil {
		return domain.UnassignedEntry{}, false, fmt.Errorf("catalog: unassigned entry time: %w", err)
	}
	entry := domain.UnassignedEntry{
		ID: f.id, Source: f.source, Name: f.name, Loose: f.loose != 0,
		BatchID: f.batchID, Relation: f.relation, Origin: f.origin, CreatedAt: created,
		Lock: lockOf(f.lockToken, f.lockUntil),
	}
	files, err := r.q.ListUnassignedFiles(ctx, f.id)
	if err != nil {
		return domain.UnassignedEntry{}, false, fmt.Errorf("catalog: list unassigned files: %w", err)
	}
	for _, file := range files {
		f := domain.UnassignedFile{Source: file.Source, Rel: file.Rel, Size: file.Size}
		if file.MtimeMs.Valid {
			f.Mtime = file.MtimeMs.Int64
		}
		entry.Files = append(entry.Files, f)
	}
	return entry, true, nil
}

// lockOf reads the lock columns: an empty token is a free entry.
func lockOf(token string, untilMillis int64) domain.AssignLock {
	if token == "" {
		return domain.AssignLock{}
	}
	return domain.AssignLock{Token: token, Until: time.UnixMilli(untilMillis)}
}

// Lock implements application.UnassignedRepository: it takes the lock for token until until
// when the entry is free, its lock has expired at now, or token already holds it (a renewal).
func (r *UnassignedRepository) Lock(ctx context.Context, id int64, token string, now, until time.Time) (bool, error) {
	n, err := r.q.TakeUnassignedLock(ctx, sqlcgen.TakeUnassignedLockParams{
		LockToken:   token,
		LockUntil:   until.UnixMilli(),
		ID:          id,
		LockUntil_2: now.UnixMilli(),
		LockToken_2: token,
	})
	if err != nil {
		return false, fmt.Errorf("catalog: take assign lock: %w", err)
	}
	return n == 1, nil
}

// Unlock implements application.UnassignedRepository: it frees the lock only when token holds it.
func (r *UnassignedRepository) Unlock(ctx context.Context, id int64, token string) error {
	if err := r.q.ReleaseUnassignedLock(ctx, sqlcgen.ReleaseUnassignedLockParams{ID: id, LockToken: token}); err != nil {
		return fmt.Errorf("catalog: release assign lock: %w", err)
	}
	return nil
}

// Names implements application.UnassignedRepository.
func (r *UnassignedRepository) Names(ctx context.Context) ([]string, error) {
	names, err := r.q.ListUnassignedNames(ctx)
	if err != nil {
		return nil, fmt.Errorf("catalog: list unassigned names: %w", err)
	}
	return names, nil
}
