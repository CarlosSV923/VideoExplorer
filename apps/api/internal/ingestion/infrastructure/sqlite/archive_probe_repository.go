package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite/sqlcgen"
)

// ArchiveProbeRepository implements application.ArchiveProbes: whether a copy of Samba is an archive, kept
// with the size and modification time of its reading (RF-27a).
type ArchiveProbeRepository struct {
	q *sqlcgen.Queries
}

var _ application.ArchiveProbes = (*ArchiveProbeRepository)(nil)

// NewArchiveProbeRepository builds the repository over an open database.
func NewArchiveProbeRepository(db *sql.DB) *ArchiveProbeRepository {
	return &ArchiveProbeRepository{q: sqlcgen.New(db)}
}

// Lookup implements application.ArchiveProbes. A reading with another size or modification time is not
// returned: the file changed since it was read.
func (r *ArchiveProbeRepository) Lookup(ctx context.Context, path string, size, mtime int64) (bool, bool, error) {
	row, err := r.q.GetArchiveProbe(ctx, path)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("ingestion: read archive probe: %w", err)
	}
	if row.Size != size || row.MtimeMs != mtime {
		return false, false, nil
	}
	return row.Archive == 1, true, nil
}

// Store implements application.ArchiveProbes: it replaces the reading kept for the path.
func (r *ArchiveProbeRepository) Store(ctx context.Context, path string, size, mtime int64, archive bool) error {
	var flag int64
	if archive {
		flag = 1
	}
	err := r.q.UpsertArchiveProbe(ctx, sqlcgen.UpsertArchiveProbeParams{Path: path, Size: size, MtimeMs: mtime, Archive: flag})
	if err != nil {
		return fmt.Errorf("ingestion: save archive probe: %w", err)
	}
	return nil
}

// Prune implements application.ArchiveProbes: it deletes the readings of the paths that are not in keep.
func (r *ArchiveProbeRepository) Prune(ctx context.Context, keep []string) error {
	paths, err := r.q.ListArchiveProbePaths(ctx)
	if err != nil {
		return fmt.Errorf("ingestion: list archive probes: %w", err)
	}
	kept := make(map[string]bool, len(keep))
	for _, p := range keep {
		kept[p] = true
	}
	for _, p := range paths {
		if kept[p] {
			continue
		}
		if err := r.q.DeleteArchiveProbe(ctx, p); err != nil {
			return fmt.Errorf("ingestion: forget archive probe: %w", err)
		}
	}
	return nil
}
