package application

import (
	"context"
	"errors"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// Errors of the partial hash (RF-24).
var (
	// ErrHashingDisabled means the partial hash is not wired: no copy can be compared.
	ErrHashingDisabled = errors.New("partial hashing is disabled")
	// ErrOutsideLibrary means the file to hash is not under the library.
	ErrOutsideLibrary = errors.New("the file is not under the library")
	// ErrSymbolicLink means the file to hash, or a folder between the library and it, is a
	// symbolic link: nothing is read through it (RNF-02).
	ErrSymbolicLink = errors.New("the path goes through a symbolic link")
)

// PartialHasher computes the partial hash of the file at path (RF-24). It reads only the
// ranges of domain.PartialHashRanges, through a small buffer: the bytes of the file never
// enter memory whole (RNF-01).
type PartialHasher interface {
	// PartialHash returns domain.FormatPartialHash of the file at path. A file that cannot be
	// read fails, and a cancelled ctx returns its error.
	PartialHash(ctx context.Context, path string) (string, error)
}

// HashStore keeps the partial hash of each file (RF-24 cache). A file is found by its device
// and inode, so a hash survives a rename inside one file system.
type HashStore interface {
	// Find returns the identity and the hash stored for the file with this device and inode.
	// found is false when none is stored.
	Find(ctx context.Context, device, inode uint64) (stored domain.FileIdentity, hash string, found bool, err error)
	// Store keeps the hash read from the file with identity id, replacing the one of the same
	// device and inode.
	Store(ctx context.Context, id domain.FileIdentity, hash string) error
}

// LinkChecker tells whether a path goes through a symbolic link (RNF-02). The adapter reads
// the file system; the application only asks.
type LinkChecker interface {
	// NoLinks returns ErrSymbolicLink when a folder between root and path, or path itself, is a
	// symbolic link. root itself is the library and is not checked. A part that does not exist
	// yet is not a link, and nil is returned for it.
	NoLinks(root, path string) error
}
