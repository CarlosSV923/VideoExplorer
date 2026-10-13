package application_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	ingestionsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/workspace"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/database"
)

// countingJobs is the job repository of the settlement tests. It counts the reads, and the
// decisions asked for each batch: a pass of the settler asks once for every batch it decides.
type countingJobs struct {
	domain.JobRepository

	mu        sync.Mutex
	reads     int
	unsettled map[domain.BatchID]int
}

func (c *countingJobs) read() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reads++
}

func (c *countingJobs) Get(ctx context.Context, id domain.JobID) (*domain.UploadJob, error) {
	c.read()
	return c.JobRepository.Get(ctx, id)
}

func (c *countingJobs) ByStatus(ctx context.Context, status domain.Status) ([]*domain.UploadJob, error) {
	c.read()
	return c.JobRepository.ByStatus(ctx, status)
}

func (c *countingJobs) ByBatch(ctx context.Context, batch domain.BatchID) ([]*domain.UploadJob, error) {
	c.read()
	return c.JobRepository.ByBatch(ctx, batch)
}

func (c *countingJobs) Exists(ctx context.Context, id domain.JobID) (bool, error) {
	c.read()
	return c.JobRepository.Exists(ctx, id)
}

func (c *countingJobs) Unsettled(ctx context.Context, batch domain.BatchID) (bool, error) {
	c.mu.Lock()
	c.unsettled[batch]++
	c.reads++
	c.mu.Unlock()
	return c.JobRepository.Unsettled(ctx, batch)
}

// decisions returns how many times a pass asked whether the batch could be decided.
func (c *countingJobs) decisions(batch domain.BatchID) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.unsettled[batch]
}

// readCount returns the reads made so far, of jobs and of the filter's records.
func (c *countingJobs) readCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reads
}

// countingEntries is the filter's record of the tests: its reads count as job reads.
type countingEntries struct {
	domain.FilterRepository
	jobs *countingJobs
}

func (e countingEntries) Entries(ctx context.Context, unit domain.JobID) ([]domain.FilterEntry, error) {
	e.jobs.read()
	return e.FilterRepository.Entries(ctx, unit)
}

// settleRig is a rig whose repositories count their reads. Its extraction workers run until the
// test ends, and its settler until stopSettler is called or the test ends; settlerDone closes when
// the settler returns.
type settleRig struct {
	*rig
	jobs        *countingJobs
	stopSettler context.CancelFunc
	settlerDone chan struct{}
}

// newSettleRig builds a service over a real database, with lib as No asignados. It runs the
// extraction workers and the settler in the background, as the process does.
func newSettleRig(t testing.TB, lib application.Library) *settleRig {
	t.Helper()

	conn, err := database.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	r := &rig{staging: filepath.Join(t.TempDir(), "staging"), trash: &recordingTrash{dir: t.TempDir()}, pub: &recorder{}}
	if err := os.MkdirAll(r.staging, 0o750); err != nil {
		t.Fatal(err)
	}
	space := func(string) (uint64, error) { return 1 << 40, nil }
	jobs := &countingJobs{JobRepository: ingestionsqlite.NewJobRepository(conn), unsettled: map[domain.BatchID]int{}}
	entries := countingEntries{FilterRepository: ingestionsqlite.NewFilterRepository(conn), jobs: jobs}
	svc := application.NewService(ingestionsqlite.NewBatchRepository(conn), jobs, r.pub, nil, time.Now).
		WithStaging(diskStaging{dir: r.staging}).
		WithExtraction(workspace.New(r.staging, space), realExtractor(), r.trash).
		WithFiltering(entries, filterRules()).
		WithLibrary(lib)
	r.svc, r.conn = svc, conn

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go svc.RunExtraction(ctx, 2)

	settling, stop := context.WithCancel(ctx)
	settlerDone := make(chan struct{})
	go func() {
		defer close(settlerDone)
		svc.RunSettler(settling)
	}()
	return &settleRig{rig: r, jobs: jobs, stopSettler: stop, settlerDone: settlerDone}
}

// receive waits for a signal of the test's own goroutines, or fails after the guard.
func receive(t *testing.T, what string, ch <-chan struct{}) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(extractionGuard):
		t.Fatalf("%s never happened", what)
	}
}

// terminatedWithoutWaiting deletes an upload and fails if the deletion waits for the settlement.
func terminatedWithoutWaiting(t *testing.T, svc *application.Service, id domain.JobID) {
	t.Helper()

	done := make(chan error, 1)
	go func() { done <- svc.Terminated(t.Context(), id) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Terminated: %v", err)
		}
	case <-time.After(extractionGuard):
		t.Fatal("the deletion of an upload waited for the settlement of its batch")
	}
}

// arriving admits n uploads of a batch that are still arriving: they are not completed.
func (r *settleRig) arriving(t *testing.T, batch domain.BatchID, n int) []domain.JobID {
	t.Helper()

	drops := make([]drop, 0, n)
	for i := range n {
		drops = append(drops, drop{relPath: fmt.Sprintf("Other/part%03d.mkv", i), data: []byte("x")})
	}
	return r.admitAll(t, batch, drops...)
}

// TestRequestsWhileAPassRunsCostOneExtraPass: while the settler is busy deciding one batch, any
// number of requests for another batch cost one decision of that batch afterwards, and nothing
// waits for them.
func TestRequestsWhileAPassRunsCostOneExtraPass(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		requests int
	}{
		{name: "one request", requests: 1},
		{name: "ten requests", requests: 10},
		{name: "a hundred requests", requests: 100},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			lib := newGatedLibrary(t)
			r := newSettleRig(t, lib)
			// The first batch is complete and its placement waits at the gate: the settler is busy.
			first := r.newBatch(t, "Movie", domain.RelationParts)
			r.dropAll(t, first, drop{relPath: "Movie/Movie.mkv", data: []byte("video")})
			receive(t, "the first batch is placed", lib.entered)

			second := r.newBatch(t, "Other", domain.RelationNone)
			ids := r.arriving(t, second, tc.requests)
			for _, id := range ids {
				terminatedWithoutWaiting(t, r.svc, id)
			}
			if n := r.jobs.decisions(second); n != 0 {
				t.Fatalf("the second batch was decided %d times while the settler was busy, want none", n)
			}

			close(lib.gate)
			if err := r.svc.WaitSettled(t.Context()); err != nil {
				t.Fatalf("WaitSettled: %v", err)
			}
			if n := r.jobs.decisions(second); n != 1 {
				t.Errorf("the second batch was decided %d times for %d requests, want once", n, tc.requests)
			}
			for _, id := range ids {
				if got := r.statusOf(t, id); got.Status != domain.StatusCancelled {
					t.Errorf("deleted upload is %s, want cancelled", got.Status)
				}
			}
		})
	}
}

// TestDeletingAnUploadDoesNotWaitForTheSettlement: a deletion returns while the decision of the
// batch is blocked, and the batch is decided later, when the settler goes on.
func TestDeletingAnUploadDoesNotWaitForTheSettlement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		uploads int
	}{
		{name: "one upload", uploads: 1},
		{name: "three uploads", uploads: 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			lib := newGatedLibrary(t)
			r := newSettleRig(t, lib)
			first := r.newBatch(t, "Movie", domain.RelationParts)
			firstIDs := r.dropAll(t, first, drop{relPath: "Movie/Movie.mkv", data: []byte("video")})
			receive(t, "the first batch is placed", lib.entered)

			ids := r.arriving(t, r.newBatch(t, "Other", domain.RelationNone), tc.uploads)
			for _, id := range ids {
				terminatedWithoutWaiting(t, r.svc, id)
			}

			close(lib.gate)
			if err := r.svc.WaitSettled(t.Context()); err != nil {
				t.Fatalf("WaitSettled: %v", err)
			}
			r.waitDone(t, firstIDs[0])
		})
	}
}

// TestStoppingDuringAPassLeavesItsBatchForTheNextStart: a stop during a decision returns the
// settler at once and leaves the batch undecided; the next start decides it.
func TestStoppingDuringAPassLeavesItsBatchForTheNextStart(t *testing.T) {
	t.Parallel()

	lib := newGatedLibrary(t)
	r := newSettleRig(t, lib)
	batch := r.newBatch(t, "Movie", domain.RelationParts)
	ids := r.dropAll(t, batch, drop{relPath: "Movie/Movie.mkv", data: []byte("video")})
	receive(t, "the batch is being placed", lib.entered)

	r.stopSettler()
	select {
	case <-r.settlerDone:
	case <-time.After(extractionGuard):
		t.Fatal("the settler did not return with its context")
	}
	stopped, cancel := context.WithCancel(t.Context())
	cancel()
	if err := r.svc.WaitSettled(stopped); !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitSettled after the stop = %v, want the context error: the batch was decided", err)
	}
	if job, _ := r.svc.Job(t.Context(), ids[0]); job.Status != domain.StatusFiltering || !job.FilterDone {
		t.Fatalf("job after the stop = %s, filter done %v; want filtered and waiting", job.Status, job.FilterDone)
	}

	next := r.service(ingestionsqlite.NewFilterRepository(r.conn), r.trash).WithLibrary(newFakeLibrary(t))
	settling, stop := context.WithCancel(t.Context())
	defer stop()
	go next.RunSettler(settling)
	if err := next.WaitSettled(t.Context()); err != nil {
		t.Fatalf("WaitSettled of the next start: %v", err)
	}
	if job, _ := next.Job(t.Context(), ids[0]); job.Status != domain.StatusDone {
		t.Errorf("job after the next start = %s, want done", job.Status)
	}
}

// TestBatchesMarkedTogetherAreAllSettled: batches whose last files arrive together are all
// decided, each as its own entry.
func TestBatchesMarkedTogetherAreAllSettled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		batches int
	}{
		{name: "one batch", batches: 1},
		{name: "three batches", batches: 3},
		{name: "five batches", batches: 5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			lib := newFakeLibrary(t)
			r := newSettleRig(t, lib)
			var ids []domain.JobID
			for i := range tc.batches {
				batch := r.newBatch(t, fmt.Sprintf("Movie %d", i), domain.RelationParts)
				ids = append(ids, r.admitAll(t, batch, drop{relPath: fmt.Sprintf("Movie %d/Movie.mkv", i), data: []byte("video")})...)
			}
			r.completeAll(t, ids...)

			for _, id := range ids {
				r.waitDone(t, id)
			}
			if got := len(lib.calls()); got != tc.batches {
				t.Errorf("entries moved = %d, want one per batch (%d)", got, tc.batches)
			}
		})
	}
}
