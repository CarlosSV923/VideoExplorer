package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite/sqlcgen"
)

// PartialHashRepository implements application.HashStore: one partial hash per file, kept
// in partial_hashes (RF-24).
type PartialHashRepository struct {
	q *sqlcgen.Queries
}

var _ application.HashStore = (*PartialHashRepository)(nil)

// NewPartialHashRepository builds the repository over an open database.
func NewPartialHashRepository(db *sql.DB) *PartialHashRepository {
	return &PartialHashRepository{q: sqlcgen.New(db)}
}

// Find implements application.HashStore.
func (r *PartialHashRepository) Find(ctx context.Context, device, inode uint64) (domain.FileIdentity, string, bool, error) {
	row, err := r.q.GetPartialHash(ctx, sqlcgen.GetPartialHashParams{Device: toSQL(device), Inode: toSQL(inode)})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.FileIdentity{}, "", false, nil
	}
	if err != nil {
		return domain.FileIdentity{}, "", false, fmt.Errorf("ingestion: get partial hash: %w", err)
	}
	stored := domain.FileIdentity{Device: device, Inode: inode, Size: row.Size, ModTime: time.Unix(0, row.MtimeNs)}
	return stored, row.Hash, true, nil
}

// Store implements application.HashStore.
func (r *PartialHashRepository) Store(ctx context.Context, id domain.FileIdentity, hash string) error {
	if err := r.q.UpsertPartialHash(ctx, sqlcgen.UpsertPartialHashParams{
		Device:  toSQL(id.Device),
		Inode:   toSQL(id.Inode),
		Size:    id.Size,
		MtimeNs: id.ModTime.UnixNano(),
		Hash:    hash,
	}); err != nil {
		return fmt.Errorf("ingestion: store partial hash: %w", err)
	}
	return nil
}
