package application

import (
	"context"
	"errors"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// ErrSambaChanged means a copy of Samba changed, or left No asignados, before it was taken: the change
// was not made and the next reading takes the copy as it is (RF-27a).
var ErrSambaChanged = errors.New("the Samba copy changed before it was taken")

// SambaSection is what the ingestion asks of No asignados for the compressed files that Samba copied
// (RF-27a). The catalog context implements it, with the section (RF-27).
type SambaSection interface {
	// Files lists the files of the entries that Samba copied, as the section reads them now (RF-27).
	// A file that is still being copied is listed as copying.
	Files(ctx context.Context) ([]domain.SambaFile, error)
	// Take moves files of one entry into their staging paths, as one change of the library (RNF-04).
	// A file that is copying or gone refuses the whole change with ErrSambaChanged, and nothing moves.
	Take(ctx context.Context, entry int64, moves []SambaMove) error
}

// SambaMove is one file that a Take moves: its place inside its entry, and where it goes.
type SambaMove struct {
	// Rel is the file's place inside the entry, with "/" separators; "" for a loose entry's own file.
	Rel string
	// Dst is the staging path the file goes to, built by the server.
	Dst string
}

// ArchiveProbes remembers, for a copy of Samba, whether it is an archive, with the size and modification
// time it had when it was read (RF-27a). A reading that still matches is used, so that the header of a
// settled copy is not read again at every reading of the section; a file that changed is read again.
type ArchiveProbes interface {
	// Lookup returns what the file at path was when it was read with this size and modification time, and
	// whether such a reading exists. A reading with another size or time is not used (ok is false).
	Lookup(ctx context.Context, path string, size, mtime int64) (archive, ok bool, err error)
	// Store records what the file at path is, with the size and modification time it has now.
	Store(ctx context.Context, path string, size, mtime int64, archive bool) error
	// Prune forgets the readings of the paths that are not in keep: the copies that left _unassigned/ (RF-27a).
	Prune(ctx context.Context, keep []string) error
}
