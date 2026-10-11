package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
)

// CachedPartialHasher answers a partial hash from the store when the file still has the
// content that was hashed (RF-24). It is a decorator of PartialHasher, like CachedProber:
// the identity of the file is read first, so a file changed while it is read is hashed
// again next time. A cache that fails is never fatal, and a hash that cannot be stored is
// only logged.
type CachedPartialHasher struct {
	hasher PartialHasher
	ids    Identifier
	store  HashStore
	log    *slog.Logger
}

var _ PartialHasher = (*CachedPartialHasher)(nil)

// NewCachedPartialHasher builds the decorator. log may be nil.
func NewCachedPartialHasher(hasher PartialHasher, ids Identifier, store HashStore, log *slog.Logger) *CachedPartialHasher {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &CachedPartialHasher{hasher: hasher, ids: ids, store: store, log: log}
}

// PartialHash implements PartialHasher.
func (c *CachedPartialHasher) PartialHash(ctx context.Context, path string) (string, error) {
	id, err := c.ids.Identify(path)
	if errors.Is(err, ErrNoFileIdentity) {
		return c.hasher.PartialHash(ctx, path) // nothing to key the cache with: no caching
	}
	if err != nil {
		return "", fmt.Errorf("identify file: %w", err)
	}

	stored, hash, found, err := c.store.Find(ctx, id.Device, id.Inode)
	switch {
	case err != nil:
		c.log.Warn("partial hash cache not read", "error", err)
	case found && stored.Matches(id):
		return hash, nil
	}

	hash, err = c.hasher.PartialHash(ctx, path)
	if err != nil {
		return "", err
	}
	if err := c.store.Store(ctx, id, hash); err != nil {
		c.log.Warn("partial hash not cached", "error", err)
	}
	return hash, nil
}

// hashing holds the port of the partial hash and the library that on-demand hashes must be
// under (RF-24). It is nil until WithPartialHashing.
type hashing struct {
	hasher  PartialHasher
	library string
}

// WithPartialHashing enables the partial hash of RF-24. The kept videos are hashed in the
// background, after their reading and without changing their jobs; PartialHashOf hashes any
// file under library on demand. It must be set before RunProbing.
func (s *Service) WithPartialHashing(hasher PartialHasher, library string) *Service {
	s.hashes = &hashing{hasher: hasher, library: library}
	s.probeQueue() // the kept videos are queued for this reading too
	return s
}

// PartialHashOf returns the partial hash of a file under the library, computed on demand
// (RF-24, through the cache). Phase 5 uses it to tell a copy apart from an existing file. A
// path outside the library fails with ErrOutsideLibrary, and nothing is read from it.
func (s *Service) PartialHashOf(ctx context.Context, path string) (string, error) {
	if s.hashes == nil {
		return "", ErrHashingDisabled
	}
	if !within(s.hashes.library, path) {
		return "", ErrOutsideLibrary
	}
	return s.hashes.hasher.PartialHash(ctx, path)
}

// within reports whether path lies strictly inside root. Both are cleaned first, so a
// path that climbs out with ".." is outside.
func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
