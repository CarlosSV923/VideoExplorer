package application_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/fileid"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/partialhash"
	ingestionsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/database"
)

// fakeHasher counts the hashes it is asked for and answers with hash or err.
type fakeHasher struct {
	mu    sync.Mutex
	calls int
	paths []string
	hash  string
	err   error
}

func (f *fakeHasher) PartialHash(ctx context.Context, path string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	f.calls++
	f.paths = append(f.paths, path)
	return f.hash, f.err
}

func (f *fakeHasher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeHasher) set(hash string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hash, f.err = hash, err
}

// fakeLinks answers the link check with err: nil when no link is on the way.
type fakeLinks struct {
	err error
}

func (f fakeLinks) NoLinks(_, _ string) error { return f.err }

// fakeHashStore keeps one hash per device and inode, and can be made to fail.
type fakeHashStore struct {
	entries  map[[2]uint64]storedHash
	findErr  error
	storeErr error
}

type storedHash struct {
	id   domain.FileIdentity
	hash string
}

func (s *fakeHashStore) Find(_ context.Context, device, inode uint64) (domain.FileIdentity, string, bool, error) {
	if s.findErr != nil {
		return domain.FileIdentity{}, "", false, s.findErr
	}
	e, ok := s.entries[[2]uint64{device, inode}]
	return e.id, e.hash, ok, nil
}

func (s *fakeHashStore) Store(_ context.Context, id domain.FileIdentity, hash string) error {
	if s.storeErr != nil {
		return s.storeErr
	}
	s.entries[[2]uint64{id.Device, id.Inode}] = storedHash{id: id, hash: hash}
	return nil
}

type hashEnv struct {
	hasher *fakeHasher
	ids    *fakeIdentifier
	store  *fakeHashStore
}

func newHashEnv() hashEnv {
	return hashEnv{
		hasher: &fakeHasher{hash: "ph1:first"},
		ids: &fakeIdentifier{
			ids:  map[string]domain.FileIdentity{},
			errs: map[string]error{},
		},
		store: &fakeHashStore{entries: map[[2]uint64]storedHash{}},
	}
}

// hashStep is one read through the cache. change runs before the read; cancelled reads with a
// context that is already done.
type hashStep struct {
	path       string
	change     func(env hashEnv)
	cancelled  bool
	wantErr    error // nil: the read must succeed
	wantHash   string
	wantHashes int // the hashes the real hasher has run after this read
}

func TestCachedPartialHasherAnswersFromTheStoreOnlyForTheSameContent(t *testing.T) {
	t.Parallel()

	withMovie := func(env hashEnv) {
		env.ids.ids["a.mkv"] = movieAt
		env.ids.ids["moved.mkv"] = movieAt // a rename: same device and inode, same size and mtime
	}
	tests := []struct {
		name  string
		setup func(env hashEnv)
		steps []hashStep
	}{
		{
			name:  "the second read of the same file is answered by the cache",
			setup: withMovie,
			steps: []hashStep{
				{path: "a.mkv", wantHash: "ph1:first", wantHashes: 1},
				{path: "a.mkv", wantHash: "ph1:first", wantHashes: 1},
			},
		},
		{
			name:  "a rename keeps the cached hash",
			setup: withMovie,
			steps: []hashStep{
				{path: "a.mkv", wantHash: "ph1:first", wantHashes: 1},
				{path: "moved.mkv", wantHash: "ph1:first", wantHashes: 1},
			},
		},
		{
			name:  "a changed size hashes the file again, and the new hash replaces the old",
			setup: withMovie,
			steps: []hashStep{
				{path: "a.mkv", wantHash: "ph1:first", wantHashes: 1},
				{path: "a.mkv", change: func(env hashEnv) {
					changed := movieAt
					changed.Size++
					env.ids.ids["a.mkv"] = changed
					env.hasher.set("ph1:second", nil)
				}, wantHash: "ph1:second", wantHashes: 2},
				{path: "a.mkv", wantHash: "ph1:second", wantHashes: 2},
			},
		},
		{
			name:  "a changed modification time hashes the file again",
			setup: withMovie,
			steps: []hashStep{
				{path: "a.mkv", wantHash: "ph1:first", wantHashes: 1},
				{path: "a.mkv", change: func(env hashEnv) {
					changed := movieAt
					changed.ModTime = movieAt.ModTime.Add(time.Second)
					env.ids.ids["a.mkv"] = changed
					env.hasher.set("ph1:second", nil)
				}, wantHash: "ph1:second", wantHashes: 2},
			},
		},
		{
			name: "without a file identity nothing is cached",
			setup: func(env hashEnv) {
				env.ids.errs["a.mkv"] = application.ErrNoFileIdentity
			},
			steps: []hashStep{
				{path: "a.mkv", wantHash: "ph1:first", wantHashes: 1},
				{path: "a.mkv", wantHash: "ph1:first", wantHashes: 2},
			},
		},
		{
			name: "a file that cannot be identified is not hashed at all",
			setup: func(env hashEnv) {
				env.ids.errs["gone.mkv"] = fs.ErrNotExist
			},
			steps: []hashStep{
				{path: "gone.mkv", wantErr: fs.ErrNotExist, wantHashes: 0},
			},
		},
		{
			name:  "a hash that fails is not stored: the next read hashes again",
			setup: withMovie,
			steps: []hashStep{
				{
					path: "a.mkv", change: func(env hashEnv) { env.hasher.set("", application.ErrNotVideo) },
					wantErr: application.ErrNotVideo, wantHashes: 1,
				},
				{path: "a.mkv", change: func(env hashEnv) { env.hasher.set("ph1:first", nil) }, wantHash: "ph1:first", wantHashes: 2},
				{path: "a.mkv", wantHash: "ph1:first", wantHashes: 2},
			},
		},
		{
			name:  "a read cancelled before it starts stores nothing",
			setup: withMovie,
			steps: []hashStep{
				{path: "a.mkv", cancelled: true, wantErr: context.Canceled, wantHashes: 0},
				{path: "a.mkv", wantHash: "ph1:first", wantHashes: 1},
			},
		},
		{
			name: "a cache that cannot be read is bypassed, not fatal",
			setup: func(env hashEnv) {
				withMovie(env)
				env.store.findErr = errors.New("database is locked")
			},
			steps: []hashStep{
				{path: "a.mkv", wantHash: "ph1:first", wantHashes: 1},
				{path: "a.mkv", wantHash: "ph1:first", wantHashes: 2},
			},
		},
		{
			name: "a hash that cannot be stored still gives its answer",
			setup: func(env hashEnv) {
				withMovie(env)
				env.store.storeErr = errors.New("disk full")
			},
			steps: []hashStep{
				{path: "a.mkv", wantHash: "ph1:first", wantHashes: 1},
				{path: "a.mkv", wantHash: "ph1:first", wantHashes: 2},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			env := newHashEnv()
			tt.setup(env)
			cached := application.NewCachedPartialHasher(env.hasher, env.ids, env.store, nil)
			for i, step := range tt.steps {
				if step.change != nil {
					step.change(env)
				}
				ctx := t.Context()
				if step.cancelled {
					c, cancel := context.WithCancel(ctx)
					cancel()
					ctx = c
				}
				hash, err := cached.PartialHash(ctx, step.path)
				if step.wantErr == nil && err != nil {
					t.Fatalf("step %d: err = %v, want none", i, err)
				}
				if step.wantErr != nil && !errors.Is(err, step.wantErr) {
					t.Fatalf("step %d: err = %v, want %v", i, err, step.wantErr)
				}
				if step.wantHash != "" && hash != step.wantHash {
					t.Fatalf("step %d: hash = %q, want %q", i, hash, step.wantHash)
				}
				if got := env.hasher.count(); got != step.wantHashes {
					t.Fatalf("step %d: hashes = %d, want %d", i, got, step.wantHashes)
				}
			}
		})
	}
}

// partialHashOfBytes is the partial hash of data, from the domain's ranges.
func partialHashOfBytes(data []byte) string {
	h := domain.NewPartialDigest(int64(len(data)))
	for _, rg := range domain.PartialHashRanges(int64(len(data))) {
		_, _ = h.Write(data[rg.Offset : rg.Offset+rg.Length])
	}
	return domain.FormatPartialHash(h.Sum(nil))
}

// counting counts the hashes of the real hasher under the cache.
type counting struct {
	inner application.PartialHasher
	mu    sync.Mutex
	calls int
}

func (c *counting) PartialHash(ctx context.Context, path string) (string, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return c.inner.PartialHash(ctx, path)
}

func (c *counting) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// TestPartialHashSurvivesARenameAndChangesWithTheContent runs the cache over the real file
// identity, the real hasher and a real database: a rename keeps the hash, a change of the
// content computes it again (RF-24, cache by file identity).
func TestPartialHashSurvivesARenameAndChangesWithTheContent(t *testing.T) {
	t.Parallel()

	conn, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	dir := t.TempDir()
	path := filepath.Join(dir, "Movie.mkv")
	data := make([]byte, 13<<20)
	for i := range data {
		data[i] = byte(i*7 + i>>12)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	hasher := &counting{inner: partialhash.Hasher{}}
	cached := application.NewCachedPartialHasher(hasher, fileid.Identifier{}, ingestionsqlite.NewPartialHashRepository(conn), nil)
	want := partialHashOfBytes(data)

	got, err := cached.PartialHash(t.Context(), path)
	if err != nil || got != want {
		t.Fatalf("first hash = %q, %v; want %q", got, err, want)
	}
	renamed := filepath.Join(dir, "renamed.mkv")
	if err := os.Rename(path, renamed); err != nil {
		t.Fatal(err)
	}
	if got, err := cached.PartialHash(t.Context(), renamed); err != nil || got != want {
		t.Fatalf("hash after the rename = %q, %v; want %q from the cache", got, err, want)
	}
	if hasher.count() != 1 {
		t.Fatalf("the file was hashed %d times, want once: the rename keeps the cache", hasher.count())
	}

	// Changing the content (its size and modification time) makes the hash stale.
	appended := append(append([]byte(nil), data...), 'x')
	if err := os.WriteFile(renamed, appended, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := cached.PartialHash(t.Context(), renamed); err != nil || got != partialHashOfBytes(appended) {
		t.Fatalf("hash after a change = %q, %v; want the new content's hash", got, err)
	}
	if hasher.count() != 2 {
		t.Fatalf("the file was hashed %d times, want twice after a change", hasher.count())
	}
}

// waitForHash waits until the cache holds the partial hash of the file at path and returns it.
func waitForHash(t *testing.T, store application.HashStore, path string) string {
	t.Helper()

	id, err := fileid.Identifier{}.Identify(path)
	if err != nil {
		t.Fatalf("Identify %s: %v", path, err)
	}
	deadline := time.Now().Add(extractionGuard)
	for {
		_, hash, found, err := store.Find(t.Context(), id.Device, id.Inode)
		if err != nil {
			t.Fatalf("Find: %v", err)
		}
		if found {
			return hash
		}
		if time.Now().After(deadline) {
			t.Fatalf("no partial hash of %s was cached", path)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// newHashingRig is newRig with the background reading of the kept videos and the partial
// hash, over the rig's database; the library is the folder that holds the staging area.
func newHashingRig(t *testing.T, hasher application.PartialHasher) (*rig, application.HashStore) {
	t.Helper()

	r := newRig(t, realExtractor(), 1<<40, true, nil)
	store := ingestionsqlite.NewPartialHashRepository(r.conn)
	r.svc.WithPartialHashing(application.NewCachedPartialHasher(hasher, fileid.Identifier{}, store, nil), fileid.Links{}, filepath.Dir(r.staging))

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	go r.svc.RunProbing(ctx)
	return r, store
}

func TestAKeptVideoIsHashedInTheBackgroundAndItsJobDoesNotChange(t *testing.T) {
	t.Parallel()

	data := make([]byte, 13<<20)
	for i := range data {
		data[i] = byte(i*31 + i>>10)
	}
	r, store := newHashingRig(t, partialhash.Hasher{})

	id := r.drop(t, "Movie.mkv", data)
	job := r.reachedFilter(t, id)
	if job.Status != domain.StatusFiltering || !job.FilterDone {
		t.Fatalf("job = %s (filter done %v), want filtering with the filter done", job.Status, job.FilterDone)
	}
	if got, want := waitForHash(t, store, r.stagedPath(id)), partialHashOfBytes(data); got != want {
		t.Fatalf("cached hash = %q, want %q", got, want)
	}
	if after, err := r.svc.Job(t.Context(), id); err != nil || after.Status != domain.StatusFiltering {
		t.Fatalf("job = %+v, %v; the hash must not change it", after, err)
	}
}

func TestAFailingHashOnlyLogsAndTheJobStaysWhereItIs(t *testing.T) {
	t.Parallel()

	failing := &fakeHasher{err: application.ErrNotVideo}
	r, _ := newHashingRig(t, failing)

	id := r.drop(t, "Movie.mkv", []byte("a video that the hash cannot read"))
	job := r.reachedFilter(t, id)
	if job.Status != domain.StatusFiltering || !job.FilterDone {
		t.Fatalf("job = %s, want it kept in filtering: a failing hash does not change its state", job.Status)
	}
	deadline := time.Now().Add(extractionGuard)
	for failing.count() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the kept video was never hashed")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if after, err := r.svc.Job(t.Context(), id); err != nil || after.Status != domain.StatusFiltering {
		t.Fatalf("job = %+v, %v; want it still filtering", after, err)
	}
}

func TestPartialHashOfOnlyHashesFilesUnderTheLibrary(t *testing.T) {
	t.Parallel()

	library := t.TempDir()
	tests := []struct {
		name    string
		path    string
		wired   bool
		links   fakeLinks
		wantErr error
	}{
		{name: "a file behind a symbolic link is not read", path: filepath.Join(library, "Movies", "Link", "Movie.mkv"), wired: true, links: fakeLinks{err: application.ErrSymbolicLink}, wantErr: application.ErrSymbolicLink},
		{name: "a file of the library is hashed", path: filepath.Join(library, "Movies", "Movie.mkv"), wired: true},
		{name: "a file of the staging area is hashed", path: filepath.Join(library, ".videoexplorer", "staging", "id"), wired: true},
		{name: "a file next to the library is refused", path: library + "-other/Movie.mkv", wired: true, wantErr: application.ErrOutsideLibrary},
		{name: "a path that climbs out of the library is refused", path: filepath.Join(library, "..", "Movie.mkv"), wired: true, wantErr: application.ErrOutsideLibrary},
		{name: "the library folder itself is not a file under it", path: library, wired: true, wantErr: application.ErrOutsideLibrary},
		{name: "without partial hashing nothing is hashed", path: filepath.Join(library, "Movie.mkv"), wired: false, wantErr: application.ErrHashingDisabled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := application.NewService(nil, nil, nil, nil, time.Now)
			hasher := &fakeHasher{hash: "ph1:x"}
			if tt.wired {
				svc.WithPartialHashing(hasher, tt.links, library)
			}
			hash, err := svc.PartialHashOf(t.Context(), tt.path)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				if hasher.count() != 0 {
					t.Fatal("a file outside the library was read")
				}
				return
			}
			if err != nil || hash != "ph1:x" {
				t.Fatalf("PartialHashOf = %q, %v; want the hasher's answer", hash, err)
			}
		})
	}
}
