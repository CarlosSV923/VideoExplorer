package application_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/ffprobe"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/fileid"
	ingestionsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/workspace"
)

// fakeProber counts the probes it is asked for and answers with info or err.
type fakeProber struct {
	mu    sync.Mutex
	calls int
	info  domain.MediaInfo
	err   error
}

func (f *fakeProber) Probe(_ context.Context, _ string) (domain.MediaInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.info, f.err
}

func (f *fakeProber) probes() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeProber) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

// fakeIdentifier answers with the identity of each path, which a test may change.
type fakeIdentifier struct {
	ids  map[string]domain.FileIdentity
	errs map[string]error
}

func (f *fakeIdentifier) Identify(path string) (domain.FileIdentity, error) {
	if err := f.errs[path]; err != nil {
		return domain.FileIdentity{}, err
	}
	return f.ids[path], nil
}

// fakeStore keeps one probe per device and inode, and can be made to fail.
type fakeStore struct {
	entries  map[[2]uint64]storedProbe
	findErr  error
	storeErr error
}

type storedProbe struct {
	id   domain.FileIdentity
	info domain.MediaInfo
}

func (s *fakeStore) Find(_ context.Context, device, inode uint64) (domain.FileIdentity, domain.MediaInfo, bool, error) {
	if s.findErr != nil {
		return domain.FileIdentity{}, domain.MediaInfo{}, false, s.findErr
	}
	e, ok := s.entries[[2]uint64{device, inode}]
	return e.id, e.info, ok, nil
}

func (s *fakeStore) Store(_ context.Context, id domain.FileIdentity, info domain.MediaInfo) error {
	if s.storeErr != nil {
		return s.storeErr
	}
	s.entries[[2]uint64{id.Device, id.Inode}] = storedProbe{id: id, info: info}
	return nil
}

// probeEnv is what one case of the cache table runs against.
type probeEnv struct {
	prober *fakeProber
	ids    *fakeIdentifier
	store  *fakeStore
}

func newProbeEnv() probeEnv {
	return probeEnv{
		prober: &fakeProber{info: domain.MediaInfo{Container: "matroska,webm"}},
		ids: &fakeIdentifier{
			ids:  map[string]domain.FileIdentity{},
			errs: map[string]error{},
		},
		store: &fakeStore{entries: map[[2]uint64]storedProbe{}},
	}
}

var movieAt = domain.FileIdentity{Device: 5, Inode: 77, Size: 8147, ModTime: time.Unix(1_700_000_000, 1)}

// probeStep is one read through the cache. change runs before the read.
type probeStep struct {
	path       string
	change     func(env probeEnv)
	wantErr    error // nil: the read must succeed
	wantProbes int   // the probes the real prober has run after this read
}

func TestCachedProberAnswersFromTheStoreOnlyForTheSameContent(t *testing.T) {
	t.Parallel()

	withMovie := func(env probeEnv) {
		env.ids.ids["a.mkv"] = movieAt
		env.ids.ids["moved.mkv"] = movieAt // a rename: same device and inode, same size and mtime
	}
	tests := []struct {
		name  string
		setup func(env probeEnv)
		steps []probeStep
	}{
		{
			name:  "the second read of the same file is answered by the cache",
			setup: withMovie,
			steps: []probeStep{
				{path: "a.mkv", wantProbes: 1},
				{path: "a.mkv", wantProbes: 1},
			},
		},
		{
			name:  "a rename keeps the cached probe",
			setup: withMovie,
			steps: []probeStep{
				{path: "a.mkv", wantProbes: 1},
				{path: "moved.mkv", wantProbes: 1},
			},
		},
		{
			name:  "a changed size probes the file again, and the new probe replaces the old",
			setup: withMovie,
			steps: []probeStep{
				{path: "a.mkv", wantProbes: 1},
				{path: "a.mkv", change: func(env probeEnv) {
					changed := movieAt
					changed.Size++
					env.ids.ids["a.mkv"] = changed
				}, wantProbes: 2},
				{path: "a.mkv", wantProbes: 2},
			},
		},
		{
			name:  "a changed modification time probes the file again",
			setup: withMovie,
			steps: []probeStep{
				{path: "a.mkv", wantProbes: 1},
				{path: "a.mkv", change: func(env probeEnv) {
					changed := movieAt
					changed.ModTime = movieAt.ModTime.Add(time.Second)
					env.ids.ids["a.mkv"] = changed
				}, wantProbes: 2},
			},
		},
		{
			name: "without a file identity nothing is cached",
			setup: func(env probeEnv) {
				env.ids.errs["a.mkv"] = application.ErrNoFileIdentity
			},
			steps: []probeStep{
				{path: "a.mkv", wantProbes: 1},
				{path: "a.mkv", wantProbes: 2},
			},
		},
		{
			name: "a file that cannot be identified is not probed at all",
			setup: func(env probeEnv) {
				env.ids.errs["gone.mkv"] = fs.ErrNotExist
			},
			steps: []probeStep{
				{path: "gone.mkv", wantErr: fs.ErrNotExist, wantProbes: 0},
			},
		},
		{
			name:  "a probe that fails is not stored: the next read probes again",
			setup: withMovie,
			steps: []probeStep{
				{
					path: "a.mkv", change: func(env probeEnv) { env.prober.setErr(application.ErrNotVideo) },
					wantErr: application.ErrNotVideo, wantProbes: 1,
				},
				{path: "a.mkv", change: func(env probeEnv) { env.prober.setErr(nil) }, wantProbes: 2},
				{path: "a.mkv", wantProbes: 2},
			},
		},
		{
			name: "a cache that cannot be read is bypassed, not fatal",
			setup: func(env probeEnv) {
				withMovie(env)
				env.store.findErr = errors.New("database is locked")
			},
			steps: []probeStep{
				{path: "a.mkv", wantProbes: 1},
				{path: "a.mkv", wantProbes: 2},
			},
		},
		{
			name: "a probe that cannot be stored still gives its answer",
			setup: func(env probeEnv) {
				withMovie(env)
				env.store.storeErr = errors.New("disk full")
			},
			steps: []probeStep{
				{path: "a.mkv", wantProbes: 1},
				{path: "a.mkv", wantProbes: 2},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			env := newProbeEnv()
			tt.setup(env)
			cached := application.NewCachedProber(env.prober, env.ids, env.store, nil)
			for i, step := range tt.steps {
				if step.change != nil {
					step.change(env)
				}
				_, err := cached.Probe(t.Context(), step.path)
				if step.wantErr == nil && err != nil {
					t.Fatalf("step %d: err = %v, want none", i, err)
				}
				if step.wantErr != nil && !errors.Is(err, step.wantErr) {
					t.Fatalf("step %d: err = %v, want %v", i, err, step.wantErr)
				}
				if got := env.prober.probes(); got != step.wantProbes {
					t.Fatalf("step %d: probes = %d, want %d", i, got, step.wantProbes)
				}
			}
		})
	}
}

// countingProber counts the reads of the real ffprobe, under the cache: started and finished.
type countingProber struct {
	inner application.Prober
	mu    sync.Mutex
	began int
	ended int
}

func (c *countingProber) Probe(ctx context.Context, path string) (domain.MediaInfo, error) {
	c.mu.Lock()
	c.began++
	c.mu.Unlock()
	info, err := c.inner.Probe(ctx, path)
	c.mu.Lock()
	c.ended++
	c.mu.Unlock()
	return info, err
}

func (c *countingProber) finished() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ended
}

// newProbingRig is newRig with the background reading of the kept videos: the real
// ffprobe under the cache, over the rig's database and by file identity.
func newProbingRig(t *testing.T) (*rig, *countingProber, application.ProbeStore) {
	t.Helper()

	r := newRig(t, realExtractor(), 1<<40, true, nil)
	counting := &countingProber{inner: ffprobe.New("")}
	store := ingestionsqlite.NewMediaProbeRepository(r.conn)
	r.svc.WithProbing(application.NewCachedProber(counting, fileid.Identifier{}, store, nil))

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	go r.svc.RunProbing(ctx)
	return r, counting, store
}

// needFFprobe skips without ffprobe in PATH, except where REQUIRE_FFPROBE is set.
func needFFprobe(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath(ffprobe.Binary); err != nil {
		if os.Getenv("REQUIRE_FFPROBE") != "" {
			t.Fatal("ffprobe not found but REQUIRE_FFPROBE is set")
		}
		t.Skip("ffprobe not installed")
	}
}

// stagedPath is the file a loose upload waits in, which filtering reads.
func (r *rig) stagedPath(id domain.JobID) string {
	return filepath.Join(r.staging, string(id))
}

// cachedProbeOf waits until the cache holds the probe of the file at path and returns it.
func cachedProbeOf(t *testing.T, store application.ProbeStore, path string) domain.MediaInfo {
	t.Helper()

	id, err := fileid.Identifier{}.Identify(path)
	if err != nil {
		t.Fatalf("Identify %s: %v", path, err)
	}
	deadline := time.Now().Add(extractionGuard)
	for {
		_, info, found, err := store.Find(t.Context(), id.Device, id.Inode)
		if err != nil {
			t.Fatalf("Find: %v", err)
		}
		if found {
			return info
		}
		if time.Now().After(deadline) {
			t.Fatalf("no probe of %s was cached", path)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// fixtureVideo is the real 1080p video of the ffprobe fixtures: two audio tracks, two subtitles.
const fixtureVideo = "../infrastructure/ffprobe/testdata/h264_1080p_two_audio_subs.mkv"

func TestAKeptVideoIsReadInTheBackgroundAndCachedByItsFile(t *testing.T) {
	t.Parallel()
	needFFprobe(t)

	r, _, store := newProbingRig(t)
	data, err := os.ReadFile(fixtureVideo)
	if err != nil {
		t.Fatal(err)
	}
	id := r.drop(t, "Movie.mkv", data)

	job := r.reachedFilter(t, id)
	if job.Status != domain.StatusFiltering || !job.FilterDone {
		t.Fatalf("job = %s (filter done %v), want filtering with the filter done", job.Status, job.FilterDone)
	}
	info := cachedProbeOf(t, store, r.stagedPath(id))
	if info.Container != "matroska,webm" || info.Video.Codec != "h264" || info.Video.Width != 1920 ||
		len(info.Audio) != 2 || len(info.Subtitles) != 2 || !info.Subtitles[0].Forced {
		t.Fatalf("cached probe = %+v, want the fixture's tracks", info)
	}
	if after, err := r.svc.Job(t.Context(), id); err != nil || after.Status != domain.StatusFiltering {
		t.Fatalf("after the reading the job is %+v, %v; the reading must not change it", after, err)
	}
}

func TestAVideoKeptFromAnArchiveIsReadWhereItWasExtracted(t *testing.T) {
	t.Parallel()
	needFFprobe(t)
	need7zz(t)

	r, _, store := newProbingRig(t)
	data, err := os.ReadFile(fixtureVideo)
	if err != nil {
		t.Fatal(err)
	}
	id := r.upload(t, "Movie.zip", zipBytes(t, zipEntry{name: "Movie/Movie.mkv", content: string(data), mode: 0o644}))

	r.reachedFilter(t, id)
	extracted := workspace.New(r.staging, nil).ExtractedPath(id, "Movie/Movie.mkv")
	if info := cachedProbeOf(t, store, extracted); info.Video.Codec != "h264" {
		t.Fatalf("cached probe = %+v, want the extracted video", info)
	}
}

func TestAFileThatIsNotAVideoFailsOnlyInTheLog(t *testing.T) {
	t.Parallel()
	needFFprobe(t)

	r, counting, store := newProbingRig(t)
	id := r.drop(t, "Broken.mkv", []byte("this is not a video, only text\n"))

	job := r.reachedFilter(t, id)
	if job.Status != domain.StatusFiltering || !job.FilterDone {
		t.Fatalf("job = %s, want it kept in filtering: an unreadable video does not change its state", job.Status)
	}
	deadline := time.Now().Add(extractionGuard)
	for counting.finished() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the video was never read")
		}
		time.Sleep(5 * time.Millisecond)
	}
	identity, err := fileid.Identifier{}.Identify(r.stagedPath(id))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, found, _ := store.Find(t.Context(), identity.Device, identity.Inode); found {
		t.Fatal("a probe was cached for a file that is not a video")
	}
	if after, err := r.svc.Job(t.Context(), id); err != nil || after.Status != domain.StatusFiltering {
		t.Fatalf("job = %+v, %v; want it still filtering", after, err)
	}
}
