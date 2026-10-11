package application

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"sync"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// CachedProber answers a probe from the store when the file still has the content that
// was probed. It is a decorator of the Prober port: the identity of the file is read
// first, so a file changed while it is probed is probed again next time. A cache that
// fails is never fatal: the file is probed instead, and a probe that cannot be stored
// is only logged.
type CachedProber struct {
	prober Prober
	ids    Identifier
	store  ProbeStore
	log    *slog.Logger
}

var _ Prober = (*CachedProber)(nil)

// NewCachedProber builds the decorator. log may be nil.
func NewCachedProber(prober Prober, ids Identifier, store ProbeStore, log *slog.Logger) *CachedProber {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &CachedProber{prober: prober, ids: ids, store: store, log: log}
}

// Probe implements Prober.
func (c *CachedProber) Probe(ctx context.Context, path string) (domain.MediaInfo, error) {
	id, err := c.ids.Identify(path)
	if errors.Is(err, ErrNoFileIdentity) {
		return c.prober.Probe(ctx, path) // nothing to key the cache with: no caching
	}
	if err != nil {
		return domain.MediaInfo{}, fmt.Errorf("identify video: %w", err)
	}

	stored, info, found, err := c.store.Find(ctx, id.Device, id.Inode)
	switch {
	case err != nil:
		c.log.Warn("probe cache not read", "error", err)
	case found && stored.Matches(id):
		return info, nil
	}

	info, err = c.prober.Probe(ctx, path)
	if err != nil {
		return domain.MediaInfo{}, err
	}
	if err := c.store.Store(ctx, id, info); err != nil {
		c.log.Warn("probe not cached", "error", err)
	}
	return info, nil
}

// probing is the background reading of the kept videos (RF-24, RF-44). One worker reads
// them, one at a time, so that the reading never competes with the extractions for the
// disk. Each video is probed (when WithProbing is set) and its partial hash is cached
// (when WithPartialHashing is set). A path is queued once until it is read.
type probing struct {
	// prober is nil until WithProbing.
	prober Prober

	mu     sync.Mutex
	queue  []probeRequest
	queued map[string]bool
	// wake tells the worker that the queue has something: it holds one signal at most.
	wake chan struct{}
}

// probeRequest asks for the video at path to be read, for the job it came from.
type probeRequest struct {
	job  domain.JobID
	path string
}

// WithProbing enables the background reading of the kept videos (RF-24, RF-44): once the
// filter has kept a video, it is read with prober (normally a CachedProber). Without it,
// nothing is read. It must be set before RunProbing.
func (s *Service) WithProbing(prober Prober) *Service {
	s.probeQueue().prober = prober
	return s
}

// probeQueue returns the queue of the background reading, creating it on first use: the
// probing and the partial hash may be enabled in either order.
func (s *Service) probeQueue() *probing {
	if s.probes == nil {
		s.probes = &probing{queued: map[string]bool{}, wake: make(chan struct{}, 1)}
	}
	return s.probes
}

// RunProbing reads the queued videos, one at a time, until ctx ends. It runs in its own
// goroutine. A read that fails is only logged: it never changes the state of its job (RF-24).
func (s *Service) RunProbing(ctx context.Context) {
	if s.probes == nil {
		return
	}
	for {
		req, ok := s.probes.next()
		if !ok {
			select {
			case <-ctx.Done():
				return
			case <-s.probes.wake:
			}
			continue
		}
		s.readVideo(ctx, req)
		if ctx.Err() != nil {
			return
		}
	}
}

// readVideo reads one queued video: ffprobe reads it when probing is wired, then its partial
// hash is cached when hashing is wired (RF-24). Each failure is only logged, and it never
// changes the state of the job. A file that already left staging (it was moved before its
// turn came) is not an error, and its other reading is skipped.
func (s *Service) readVideo(ctx context.Context, req probeRequest) {
	if s.probes.prober != nil {
		_, err := s.probes.prober.Probe(ctx, req.path)
		if s.logRead(ctx, req, "video not read", err) {
			return
		}
	}
	if s.hashes != nil {
		_, err := s.hashes.hasher.PartialHash(ctx, req.path)
		s.logRead(ctx, req, "video not hashed", err)
	}
}

// logRead logs the failure of one reading of a queued video, if there is one. It reports
// whether the file has left staging.
func (s *Service) logRead(ctx context.Context, req probeRequest, msg string, err error) (gone bool) {
	switch {
	case ctx.Err() != nil:
		return false
	case errors.Is(err, fs.ErrNotExist):
		s.log.Info("video left staging before its turn", "job", req.job)
		return true
	case err != nil:
		s.log.Warn(msg, "job", req.job, "error", err)
	}
	return false
}

// queueKeptVideos queues the videos the filter kept of a work unit (RF-24). Only videos
// are read: subtitles have no video track. The job itself does not change.
func (s *Service) queueKeptVideos(unit []*domain.UploadJob, extracted bool, entries []domain.FilterEntry) {
	if s.probes == nil || s.work == nil {
		return
	}
	job := unit[0]
	for _, e := range entries {
		if e.Kind != domain.KindVideo {
			continue
		}
		path := s.work.workspace.ArchivePath(job.ID)
		if extracted {
			path = s.work.workspace.ExtractedPath(leaderOf(job), e.Path)
		}
		s.probes.enqueue(probeRequest{job: job.ID, path: path})
	}
}

// enqueue queues a read. It never blocks the caller (a filter worker calls it).
func (p *probing) enqueue(req probeRequest) {
	p.mu.Lock()
	if p.queued[req.path] {
		p.mu.Unlock()
		return
	}
	p.queued[req.path] = true
	p.queue = append(p.queue, req)
	p.mu.Unlock()

	select {
	case p.wake <- struct{}{}:
	default: // the worker has been signalled already
	}
}

// next takes the oldest queued read, if any.
func (p *probing) next() (probeRequest, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.queue) == 0 {
		return probeRequest{}, false
	}
	req := p.queue[0]
	p.queue = p.queue[1:]
	delete(p.queued, req.path)
	return req, true
}
