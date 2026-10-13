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
