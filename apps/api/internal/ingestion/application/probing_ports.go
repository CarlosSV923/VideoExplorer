package application

import (
	"context"
	"errors"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// Errors a Prober returns.
var (
	// ErrNotVideo means the file cannot be read as a video: ffprobe refuses it, or it
	// has no video track.
	ErrNotVideo = errors.New("not a valid video")
	// ErrProbeTimeout means ffprobe did not answer in time.
	ErrProbeTimeout = errors.New("probe timed out")
	// ErrNoFileIdentity means the platform gives no device and inode for a file, so the
	// probes of its files cannot be cached.
	ErrNoFileIdentity = errors.New("the platform gives no file identity")
)

// Prober reads the technical information of a video file (RF-24, RF-44). The bytes of
// the video never enter the process: the tool reads the file (RNF-01).
type Prober interface {
	// Probe returns what the file at path says about itself. A file that cannot be read
	// as a video fails with ErrNotVideo; one that takes too long fails with ErrProbeTimeout.
	Probe(ctx context.Context, path string) (domain.MediaInfo, error)
}

// Identifier reads the identity of the file at path. The adapter reads it from the
// file system.
type Identifier interface {
	// Identify returns the identity of the regular file at path. It returns
	// ErrNoFileIdentity when the platform has no device and inode to give.
	Identify(path string) (domain.FileIdentity, error)
}

// ProbeStore keeps the probes of videos, one per file (RF-24 cache). A file is found by
// its device and inode, so a probe survives a rename inside one file system.
type ProbeStore interface {
	// Find returns the identity and the probe stored for the file with this device and
	// inode. found is false when none is stored.
	Find(ctx context.Context, device, inode uint64) (stored domain.FileIdentity, info domain.MediaInfo, found bool, err error)
	// Store keeps the probe taken from the file with identity id, replacing the one of
	// the same device and inode.
	Store(ctx context.Context, id domain.FileIdentity, info domain.MediaInfo) error
}
