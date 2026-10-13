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

// ScanRepository implements application.ScanStore: the time each scan of the library last finished
// (RF-28), kept so that it survives a restart.
type ScanRepository struct {
	q *sqlcgen.Queries
}

var _ application.ScanStore = (*ScanRepository)(nil)

// NewScanRepository builds the repository over an open database.
func NewScanRepository(db *sql.DB) *ScanRepository {
	return &ScanRepository{q: sqlcgen.New(db)}
}

// Finished implements application.ScanStore: the scans that finished, with their times.
func (r *ScanRepository) Finished(ctx context.Context) (map[domain.ScanKind]time.Time, error) {
	rows, err := r.q.ListScanRuns(ctx)
	if err != nil {
		return nil, fmt.Errorf("catalog: list scan times: %w", err)
	}
	out := make(map[domain.ScanKind]time.Time, len(rows))
	for _, row := range rows {
		at, err := time.Parse(timeLayout, row.FinishedAt)
		if err != nil {
			return nil, fmt.Errorf("catalog: scan time of %s: %w", row.Kind, err)
		}
		out[domain.ScanKind(row.Kind)] = at
	}
	return out, nil
}

// SetFinished implements application.ScanStore: it records when a scan finished, replacing its last time.
func (r *ScanRepository) SetFinished(ctx context.Context, kind domain.ScanKind, at time.Time) error {
	err := r.q.UpsertScanRun(ctx, sqlcgen.UpsertScanRunParams{Kind: string(kind), FinishedAt: at.UTC().Format(timeLayout)})
	if err != nil {
		return fmt.Errorf("catalog: save scan time of %s: %w", kind, err)
	}
	return nil
}
