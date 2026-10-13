package application_test

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/ffprobe"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/fileid"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/partialhash"
	ingestionsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
)

// fakeLibrary is No asignados as the catalog keeps it (RF-08, RF-08a): each file is renamed
// into its entry's folder, the places are recorded first, and a repeated request goes on
// from what it recorded. interruptAfter stops a call after that many renames, as a restart
// does: the context is cancelled and the call returns the cancellation.
type fakeLibrary struct {
	root string

	mu       sync.Mutex
	requests []application.AsideRequest
	placed   map[string]map[string]string
	failNext int
	moves    int

	interruptAfter int
	cancel         context.CancelFunc
}

func newFakeLibrary(t *testing.T) *fakeLibrary {
	t.Helper()
	return &fakeLibrary{root: filepath.Join(t.TempDir(), "library", "_unassigned"), placed: map[string]map[string]string{}}
}

// PutAside implements application.Library.
func (l *fakeLibrary) PutAside(ctx context.Context, req application.AsideRequest) (map[string]string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.requests = append(l.requests, req)
	if l.failNext > 0 {
		l.failNext--
		return nil, errors.New("input/output error")
	}
	places := l.placed[req.Source]
	if places == nil {
		places = map[string]string{}
		l.placed[req.Source] = places
	}
	for _, f := range req.Files {
		if _, ok := places[f.Source]; !ok {
			places[f.Source] = l.place(req, f)
		}
	}
	moved := 0
	for _, f := range req.Files {
		if _, err := os.Stat(f.Source); err != nil {
			continue // in place already: moved before
		}
		dst := places[f.Source]
		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			return nil, err
		}
		if err := os.Rename(f.Source, dst); err != nil {
			return nil, err
		}
		l.moves++
		moved++
		if l.interruptAfter > 0 && moved == l.interruptAfter {
			l.interruptAfter = 0
			l.cancel()
			return nil, ctx.Err()
		}
	}
	out := make(map[string]string, len(places))
	for k, v := range places {
		out[k] = v
	}
	return out, nil
}

func (l *fakeLibrary) place(req application.AsideRequest, f application.AsideFile) string {
	if req.Loose {
		return filepath.Join(l.root, req.Name)
	}
	return filepath.Join(l.root, req.Name, filepath.FromSlash(f.Rel))
}

// calls returns the requests received so far.
func (l *fakeLibrary) calls() []application.AsideRequest {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]application.AsideRequest(nil), l.requests...)
}

// placeOf returns the place recorded for the file at source.
func (l *fakeLibrary) placeOf(source string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, places := range l.placed {
		if p, ok := places[source]; ok {
			return p
		}
	}
	return ""
}

// renames returns how many files were renamed so far.
func (l *fakeLibrary) renames() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.moves
}

// rels returns the places of the files of a request, sorted.
func rels(req application.AsideRequest) []string {
	out := make([]string, 0, len(req.Files))
	for _, f := range req.Files {
		out = append(out, f.Rel)
	}
	sort.Strings(out)
	return out
}

// placingRig is the rig with the unassigned section wired and its workers running.
func placingRig(t *testing.T, lib application.Library) *rig {
	t.Helper()

	r := newRig(t, realExtractor(), 1<<40, false, nil)
	r.svc.WithLibrary(lib)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	go r.svc.RunSettler(ctx)
	go r.svc.RunExtraction(ctx, 2)
	return r
}

// drop is a file of a batch: its path in the drop, and its bytes.
type drop struct {
	relPath string
	data    []byte
}

// dropAll admits every file of a batch before completing any of them, so that the batch
// is not complete, and decided, before its last file is in (RF-03).
func (r *rig) dropAll(t *testing.T, batch domain.BatchID, drops ...drop) []domain.JobID {
	t.Helper()

	ids := r.admitAll(t, batch, drops...)
	r.completeAll(t, ids...)
	return ids
}

// admitAll admits and stages the files of a batch, without completing them.
func (r *rig) admitAll(t *testing.T, batch domain.BatchID, drops ...drop) []domain.JobID {
	t.Helper()

	ids := make([]domain.JobID, 0, len(drops))
	for _, d := range drops {
		meta := domain.UploadMeta{FileName: path.Base(d.relPath), BatchID: string(batch)}
		if strings.Contains(d.relPath, "/") {
			meta.RelativePath = d.relPath
		}
		id, err := r.svc.Admit(t.Context(), application.Admission{Meta: meta, Size: int64(len(d.data))})
		if err != nil {
			t.Fatalf("Admit %s: %v", d.relPath, err)
		}
		if err := os.WriteFile(filepath.Join(r.staging, string(id)), d.data, 0o600); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

// completeAll tells the service that the files have all arrived (RF-02).
func (r *rig) completeAll(t *testing.T, ids ...domain.JobID) {
	t.Helper()

	for _, id := range ids {
		if err := r.svc.Completed(t.Context(), id); err != nil {
			t.Fatalf("Completed: %v", err)
		}
	}
}

// newBatch creates a batch with a name and a relation, as the browser does (RF-03, RF-01a).
func (r *rig) newBatch(t *testing.T, name string, relation domain.Relation) domain.BatchID {
	t.Helper()

	batch, err := r.svc.CreateBatch(t.Context(), name, relation)
	if err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	return batch.ID
}

// waitDone waits until the job's content is in No asignados.
func (r *rig) waitDone(t *testing.T, id domain.JobID) {
	t.Helper()

	r.waitFor(t, id, "done", func(j domain.UploadJob) bool { return j.Status == domain.StatusDone })
}

// eventually waits until ok holds, or fails the test after the guard.
func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()

	deadline := time.Now().Add(extractionGuard)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("%s never happened", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestContentOfABatchWithARelationMovesAsOneEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		relation  domain.Relation
		batchName string
		drops     func(t *testing.T) []drop
		wantName  string
		wantRels  []string
	}{
		{
			name:      "parts of a dropped folder: the folder is the entry and keeps its subfolders",
			relation:  domain.RelationParts,
			batchName: "Movie",
			drops: func(*testing.T) []drop {
				return []drop{{relPath: "Movie/Movie.mkv", data: []byte("video")}, {relPath: "Movie/sub/Movie.es.srt", data: []byte("subtitle")}}
			},
			wantName: "Movie",
			wantRels: []string{"Movie.mkv", "sub/Movie.es.srt"},
		},
		{
			name:     "chapters of two archives share one entry, named after the first",
			relation: domain.RelationEpisodes,
			drops: func(t *testing.T) []drop {
				return []drop{
					{relPath: "S01E01.zip", data: zipBytes(t, zipEntry{name: "S01E01.mkv", content: "video", mode: 0o644})},
					{relPath: "S01E02.zip", data: zipBytes(t, zipEntry{name: "S01E02.mkv", content: "video", mode: 0o644})},
				}
			},
			wantName: "S01E01",
			wantRels: []string{"S01E01.mkv", "S01E02.mkv"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lib := newFakeLibrary(t)
			r := placingRig(t, lib)
			batch := r.newBatch(t, tt.batchName, tt.relation)
			ids := r.dropAll(t, batch, tt.drops(t)...)
			for _, id := range ids {
				r.waitDone(t, id)
			}

			calls := lib.calls()
			if len(calls) != 1 {
				t.Fatalf("moves = %d, want one entry for the batch", len(calls))
			}
			if left := leftovers(t, r.staging); len(left) != 0 {
				t.Errorf("staging still holds %v after the move", left)
			}
			if calls[0].Name != tt.wantName || calls[0].Source != string(batch) || calls[0].Relation != string(tt.relation) {
				t.Errorf("request = %+v; want name %q for the batch", calls[0], tt.wantName)
			}
			if got := rels(calls[0]); strings.Join(got, ",") != strings.Join(tt.wantRels, ",") {
				t.Errorf("places = %v, want %v", got, tt.wantRels)
			}
		})
	}
}

func TestADiscardedFileOfAPartsBatchStaysOutOfItsEntry(t *testing.T) {
	t.Parallel()

	lib := newFakeLibrary(t)
	r := placingRig(t, lib)
	batch := r.newBatch(t, "Movie", domain.RelationParts)
	ids := r.dropAll(t, batch,
		drop{relPath: "Movie/Movie.mkv", data: []byte("video")},
		drop{relPath: "Movie/notes.txt", data: []byte("notes")},
	)

	r.waitDone(t, ids[0])
	r.waitFor(t, ids[1], "discarded", func(j domain.UploadJob) bool { return j.Status == domain.StatusDiscarded })
	calls := lib.calls()
	if len(calls) != 1 || strings.Join(rels(calls[0]), ",") != "Movie.mkv" {
		t.Fatalf("calls = %+v; want one entry with the video only", calls)
	}
	if len(r.trash.puts()) != 1 {
		t.Errorf("trashed = %d, want the text file in the trash", len(r.trash.puts()))
	}
}

func TestWithoutARelationEachUnitIsItsOwnEntry(t *testing.T) {
	t.Parallel()

	lib := newFakeLibrary(t)
	r := placingRig(t, lib)
	movie := r.drop(t, "Movie.mkv", []byte("video"))
	pack := r.upload(t, "Pack.zip", zipBytes(t,
		zipEntry{name: "Movie/Movie.mkv", content: "video", mode: 0o644},
		zipEntry{name: "Movie/Movie.es.srt", content: "subtitle", mode: 0o644},
	))
	r.waitDone(t, movie)
	r.waitDone(t, pack)

	calls := lib.calls()
	if len(calls) != 2 {
		t.Fatalf("entries = %d, want one per unit", len(calls))
	}
	byName := map[string]application.AsideRequest{}
	for _, c := range calls {
		byName[c.Name] = c
	}
	loose, ok := byName["Movie.mkv"]
	if !ok || !loose.Loose || len(loose.Files) != 1 || loose.Files[0].Rel != "" {
		t.Errorf("loose entry = %+v; want the video as it was uploaded", loose)
	}
	folder, ok := byName["Pack"]
	if !ok || folder.Loose || strings.Join(rels(folder), ",") != "Movie/Movie.es.srt,Movie/Movie.mkv" {
		t.Errorf("archive entry = %+v; want the archive's name and its content", folder)
	}
}

func TestAFailedMoveIsRetriedAndMovesEachFileOnce(t *testing.T) {
	t.Parallel()

	lib := newFakeLibrary(t)
	lib.failNext = 1
	r := placingRig(t, lib)
	batch := r.newBatch(t, "Movie", domain.RelationParts)
	ids := r.dropAll(t, batch,
		drop{relPath: "Movie/Movie.mkv", data: []byte("video")},
		drop{relPath: "Movie/Movie.es.srt", data: []byte("subtitle")},
	)

	failed := r.waitFor(t, ids[0], "failed with move_failed", func(j domain.UploadJob) bool {
		return j.Status == domain.StatusFailed
	})
	if failed.Error != domain.FailureMoveFailed {
		t.Fatalf("failed with %q, want move_failed", failed.Error)
	}
	other := r.waitFor(t, ids[1], "failed with move_failed too", func(j domain.UploadJob) bool {
		return j.Status == domain.StatusFailed && j.Error == domain.FailureMoveFailed
	})
	if other.Error != domain.FailureMoveFailed {
		t.Fatalf("the other job of the entry failed with %q, want move_failed", other.Error)
	}

	if _, err := r.svc.Retry(t.Context(), ids[1]); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	r.waitDone(t, ids[0])
	r.waitDone(t, ids[1])
	// The settler may still be deciding the batch, and each decision asks the library again: the
	// check waits until none is pending, then looks at every request it made (RF-08a).
	if err := r.svc.WaitSettled(t.Context()); err != nil {
		t.Fatalf("WaitSettled: %v", err)
	}
	if got := lib.renames(); got != 2 {
		t.Errorf("renames = %d, want 2: each file moved once", got)
	}
	calls := lib.calls()
	for _, c := range calls {
		if c.Source != calls[0].Source {
			t.Errorf("a request asked for entry %q, want every request for the same entry %q", c.Source, calls[0].Source)
		}
	}
	if len(calls) < 2 {
		t.Errorf("requests = %d; want the failed move asked again", len(calls))
	}
}

func TestAMoveInterruptedByARestartGoesOnAfterIt(t *testing.T) {
	t.Parallel()

	lib := newFakeLibrary(t)
	r := newRig(t, realExtractor(), 1<<40, false, nil)
	r.svc.WithLibrary(lib)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	lib.cancel = cancel
	lib.interruptAfter = 1
	go r.svc.RunSettler(ctx)
	go r.svc.RunExtraction(ctx, 1)

	batch := r.newBatch(t, "Movie", domain.RelationParts)
	ids := r.dropAll(t, batch,
		drop{relPath: "Movie/Movie.mkv", data: []byte("video")},
		drop{relPath: "Movie/Movie.es.srt", data: []byte("subtitle")},
	)
	eventually(t, "the first file moved before the stop", func() bool { return lib.renames() == 1 })

	// The restarted process finds one file in place and the other still in staging.
	after := r.service(ingestionsqlite.NewFilterRepository(r.conn), r.trash).WithLibrary(lib)
	resumeCtx, resumeCancel := context.WithCancel(t.Context())
	t.Cleanup(resumeCancel)
	go after.RunSettler(resumeCtx)
	go after.RunExtraction(resumeCtx, 1)
	r.svc = after

	r.waitDone(t, ids[0])
	r.waitDone(t, ids[1])
	if got := lib.renames(); got != 2 {
		t.Errorf("renames = %d, want 2: the file moved before the restart is not moved again", got)
	}
	if calls := lib.calls(); len(calls) != 2 {
		t.Errorf("requests = %d, want the first call and the resumed one", len(calls))
	}
}

// leftovers lists the regular files that remain under dir.
func leftovers(t *testing.T, dir string) []string {
	t.Helper()

	var out []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			out = append(out, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// countingStore counts the lookups of a probe cache, to see a video answered from it.
type countingStore struct {
	application.ProbeStore
	mu    sync.Mutex
	finds int
}

func (c *countingStore) Find(ctx context.Context, device, inode uint64) (domain.FileIdentity, domain.MediaInfo, bool, error) {
	c.mu.Lock()
	c.finds++
	c.mu.Unlock()
	return c.ProbeStore.Find(ctx, device, inode)
}

func (c *countingStore) lookups() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.finds
}

// TestAVideoMovedIntoUnassignedIsReadAgainFromItsCache: the reading of a kept video is
// queued again at its new place (RF-08a), and the cache answers it, since a rename keeps
// the inode (RF-24).
func TestAVideoMovedIntoUnassignedIsReadAgainFromItsCache(t *testing.T) {
	t.Parallel()
	needFFprobe(t)

	lib := newFakeLibrary(t)
	r := newRig(t, realExtractor(), 1<<40, false, nil)
	r.svc.WithLibrary(lib)
	counting := &countingProber{inner: ffprobe.New("")}
	store := &countingStore{ProbeStore: ingestionsqlite.NewMediaProbeRepository(r.conn)}
	r.svc.WithProbing(application.NewCachedProber(counting, fileid.Identifier{}, store, nil))
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	go r.svc.RunSettler(ctx)
	go r.svc.RunExtraction(ctx, 1)
	go r.svc.RunProbing(ctx)

	data, err := os.ReadFile(fixtureVideo)
	if err != nil {
		t.Fatal(err)
	}
	batch := r.newBatch(t, "Movie", domain.RelationParts)
	ids := r.admitAll(t, batch,
		drop{relPath: "Movie/Movie.mkv", data: data},
		drop{relPath: "Movie/notes.txt", data: []byte("notes")},
	)
	// The video is read where it was staged first; the batch is decided only then.
	r.completeAll(t, ids[0])
	staged := filepath.Join(r.staging, string(ids[0]))
	cachedProbeOf(t, store, staged)
	r.completeAll(t, ids[1])
	r.waitDone(t, ids[0])

	placed := lib.placeOf(staged)
	if placed == "" {
		t.Fatal("the video has no recorded place")
	}
	eventually(t, "the video looked up at its new place", func() bool { return store.lookups() >= 2 })
	cachedProbeOf(t, store, placed)
	if got := counting.finished(); got != 1 {
		t.Errorf("ffprobe read the video %d times, want once: its new place is answered by the cache", got)
	}
}

// countingHashStore counts the lookups of the partial hash cache.
type countingHashStore struct {
	application.HashStore
	mu    sync.Mutex
	finds int
}

func (c *countingHashStore) Find(ctx context.Context, device, inode uint64) (domain.FileIdentity, string, bool, error) {
	c.mu.Lock()
	c.finds++
	c.mu.Unlock()
	return c.HashStore.Find(ctx, device, inode)
}

func (c *countingHashStore) lookups() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.finds
}

// TestAVideoMovedIntoUnassignedIsHashedFromItsCache: the partial hash of a kept video is
// queued again at its new place (RF-08a, RF-24), where the cache answers it by its inode.
func TestAVideoMovedIntoUnassignedIsHashedFromItsCache(t *testing.T) {
	t.Parallel()

	lib := newFakeLibrary(t)
	r := newRig(t, realExtractor(), 1<<40, false, nil)
	r.svc.WithLibrary(lib)
	hasher := &counting{inner: partialhash.Hasher{}}
	store := &countingHashStore{HashStore: ingestionsqlite.NewPartialHashRepository(r.conn)}
	r.svc.WithPartialHashing(application.NewCachedPartialHasher(hasher, fileid.Identifier{}, store, nil), fileid.Links{}, filepath.Dir(r.staging))
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	go r.svc.RunSettler(ctx)
	go r.svc.RunExtraction(ctx, 1)
	go r.svc.RunProbing(ctx)

	batch := r.newBatch(t, "Movie", domain.RelationParts)
	ids := r.admitAll(t, batch,
		drop{relPath: "Movie/Movie.mkv", data: []byte("video content")},
		drop{relPath: "Movie/notes.txt", data: []byte("notes")},
	)
	r.completeAll(t, ids[0])
	staged := filepath.Join(r.staging, string(ids[0]))
	waitForHash(t, store, staged)
	r.completeAll(t, ids[1])
	r.waitDone(t, ids[0])

	placed := lib.placeOf(staged)
	if placed == "" {
		t.Fatal("the video has no recorded place")
	}
	eventually(t, "the video looked up at its new place", func() bool { return store.lookups() >= 2 })
	waitForHash(t, store, placed)
	if got := hasher.count(); got != 1 {
		t.Errorf("the partial hash was computed %d times, want once: its new place is answered by the cache", got)
	}
}
