// Package sqlite stores the metadata context's data in the shared SQLite
// database (spec §6): the TMDB answers. Its typed queries come from
// queries/ through sqlc (`task gen:sql`).
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/infrastructure/sqlite/sqlcgen"
)

// ResponseStore implements application.ResponseStore on the metadata_cache table.
type ResponseStore struct {
	q *sqlcgen.Queries
}

var _ application.ResponseStore = (*ResponseStore)(nil)

// NewResponseStore builds the store over an open database (see platform/database).
func NewResponseStore(db *sql.DB) *ResponseStore {
	return &ResponseStore{q: sqlcgen.New(db)}
}

// Get returns the entry under key. found is false when there is none.
func (s *ResponseStore) Get(ctx context.Context, key string) (application.CachedResponse, bool, error) {
	row, err := s.q.GetCachedResponse(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return application.CachedResponse{}, false, nil
	}
	if err != nil {
		return application.CachedResponse{}, false, fmt.Errorf("metadata cache: get: %w", err)
	}
	return application.CachedResponse{Payload: row.Payload, ExpiresAt: time.Unix(row.ExpiresAt, 0)}, true, nil
}

// Put stores entry under key, replacing any previous one. Expiry has one-second precision.
func (s *ResponseStore) Put(ctx context.Context, key string, entry application.CachedResponse) error {
	err := s.q.PutCachedResponse(ctx, sqlcgen.PutCachedResponseParams{
		Key:       key,
		Payload:   entry.Payload,
		ExpiresAt: entry.ExpiresAt.Unix(),
	})
	if err != nil {
		return fmt.Errorf("metadata cache: put: %w", err)
	}
	return nil
}
