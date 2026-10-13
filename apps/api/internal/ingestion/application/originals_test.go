package application_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	ingestionsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/workspace"
)

// trashFailed is the state of a job whose original did not reach the trash (RF-08a).
func trashFailed(j domain.UploadJob) bool {
	return j.Status == domain.StatusFailed && j.Error == domain.FailureTrashFailed
}

// TestAnOriginalThatDoesNotReachTheTrashKeepsItsJobFailedAndItsFiles: whatever the archive holds,
// its original stays in staging and its extraction stays too, and nothing is deleted: the batch
// settles without removing the archive, and the job waits for Reintentar (RF-08a).
func TestAnOriginalThatDoesNotReachTheTrashKeepsItsJobFailedAndItsFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		archive func(t *testing.T) []byte
	}{
		{name: "an archive with a video", archive: videoZip},
		{name: "an archive with nothing to keep", archive: func(t *testing.T) []byte {
			return zipBytes(t,
				zipEntry{name: "Extras/Movie.nfo", content: "metadata", mode: 0o644},
				zipEntry{name: "Extras/sample.mkv", content: "small", mode: 0o644},
			)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := newRig(t, realExtractor(), 1<<40, true, nil)
			r.trash.setErr(errors.New("input/output error"))
			id := r.upload(t, "Extras.zip", tt.archive(t))
			r.waitFor(t, id, "failed with trash_failed", trashFailed)

			// The settlement of the batch runs right after the failure: the checks give it a moment.
			time.Sleep(200 * time.Millisecond)
			if !r.staged(id) {
				t.Error("the original left staging although its move to the trash failed")
			}
			if _, err := os.Stat(r.extractionDir(id)); err != nil {
				t.Errorf("the extracted content is gone: %v", err)
			}
			if n := len(r.trash.puts()); n != 0 {
				t.Errorf("trash entries = %d, want none", n)
			}
			if n := r.trash.calls(); n != 1 {
				t.Errorf("the move was asked %d times, want once: nothing retries it on its own", n)
			}
			if job := r.statusOf(t, id); job.Error != domain.FailureTrashFailed {
				t.Errorf("job failed with %q, want trash_failed", job.Error)
			}
		})
	}
}

// TestAnEmptyArchiveKeepsItsOriginalUntilItsTrashMoveIsDone: the archive has nothing to keep and its
// move to the trash fails. The job waits with trash_failed; once Reintentar moves the original, the
// batch is decided as usual (no_video), and the original is in the trash once.
func TestAnEmptyArchiveKeepsItsOriginalUntilItsTrashMoveIsDone(t *testing.T) {
	t.Parallel()

	r := newRig(t, realExtractor(), 1<<40, true, nil)
	r.trash.mu.Lock()
	r.trash.failName = "Extras.zip"
	r.trash.mu.Unlock()
	id := r.upload(t, "Extras.zip", zipBytes(t,
		zipEntry{name: "Extras/Movie.nfo", content: "metadata", mode: 0o644},
		zipEntry{name: "Extras/sample.mkv", content: "small", mode: 0o644},
	))
	r.waitFor(t, id, "failed with trash_failed", trashFailed)

	if _, err := r.svc.Retry(t.Context(), id); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	job := r.reachedFilter(t, id)
	if job.Status != domain.StatusFailed || job.Error != domain.FailureNoVideo {
		t.Fatalf("job = %s (%q), want failed with no_video", job.Status, job.Error)
	}
	if n := len(r.trashedNamed("Extras.zip")); n != 1 {
		t.Errorf("the original went to the trash %d times, want once", n)
	}
	if r.staged(id) {
		t.Error("the original is still in staging")
	}
}

// TestAFailedTrashMoveIsRetriedAndTheJobGoesOnToTheLibrary: after Reintentar the content of the
// archive is kept, moved into No asignados and the job is done.
func TestAFailedTrashMoveIsRetriedAndTheJobGoesOnToTheLibrary(t *testing.T) {
	t.Parallel()

	lib := newFakeLibrary(t)
	r := placingRig(t, lib)
	r.trash.setErr(errors.New("input/output error"))
	id := r.upload(t, "Movie.zip", videoZip(t))
	r.waitFor(t, id, "failed with trash_failed", trashFailed)

	r.trash.setErr(nil)
	if _, err := r.svc.Retry(t.Context(), id); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	r.waitDone(t, id)
	if n := len(r.trash.puts()); n != 1 {
		t.Errorf("trash entries = %d, want the original once", n)
	}
	if got := lib.renames(); got != 1 {
		t.Errorf("renames = %d, want the video moved once", got)
	}
	if r.staged(id) {
		t.Error("the original is still in staging after the job is done")
	}
}

// TestAPurgeKeepsAnOriginalWaitingForItsTrash: the purge deletes what no job knows; an original
// that waits for its move to the trash is known to its job, and stays.
func TestAPurgeKeepsAnOriginalWaitingForItsTrash(t *testing.T) {
	t.Parallel()

	r := newRig(t, realExtractor(), 1<<40, true, nil)
	r.trash.setErr(errors.New("input/output error"))
	id := r.upload(t, "Movie.zip", videoZip(t))
	r.waitFor(t, id, "failed with trash_failed", trashFailed)

	purge := r.listingService()
	if _, err := purge.Purge(t.Context()); err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if !r.staged(id) {
		t.Error("the purge deleted an original waiting for its move to the trash")
	}
	if job, err := purge.Job(t.Context(), id); err != nil || !trashFailed(*job) {
		t.Errorf("job after the purge = %+v, %v; want failed with trash_failed", job, err)
	}
}

// TestADeletionIsRefusedWhileTheOriginalWaitsForItsTrash: the browser cannot delete an upload whose
// original still waits for its move to the trash; once the move is done, the deletion may go on.
func TestADeletionIsRefusedWhileTheOriginalWaitsForItsTrash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		retry bool
		want  error
	}{
		{name: "while its move to the trash failed", want: application.ErrOriginalPending},
		{name: "once the move is retried and done", retry: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := newRig(t, realExtractor(), 1<<40, true, nil)
			r.trash.setErr(errors.New("input/output error"))
			id := r.upload(t, "Movie.zip", videoZip(t))
			r.waitFor(t, id, "failed with trash_failed", trashFailed)

			if tt.retry {
				r.trash.setErr(nil)
				if _, err := r.svc.Retry(t.Context(), id); err != nil {
					t.Fatalf("Retry: %v", err)
				}
				r.reachedFilter(t, id)
			}
			if err := r.svc.Terminating(t.Context(), id); !errors.Is(err, tt.want) {
				t.Errorf("Terminating = %v, want %v", err, tt.want)
			}
			if tt.want == nil && r.staged(id) {
				t.Error("the original is still in staging although its move is done")
			}
		})
	}
}

// TestTheStartDoesNotWaitForTheSettlement: a batch that a previous run left complete is decided
// after the start. Recover returns while the library is still to take the batch, and the service
// answers meanwhile; the batch is decided once the settlement runs.
func TestTheStartDoesNotWaitForTheSettlement(t *testing.T) {
	t.Parallel()

	r, id := settlingRig(t)
	lib := newGatedLibrary(t)
	restarted := r.service(ingestionsqlite.NewFilterRepository(r.conn), r.trash).WithLibrary(lib)
	r.svc = restarted

	recovered := make(chan error, 1)
	go func() {
		_, err := restarted.Recover(t.Context())
		recovered <- err
	}()
	select {
	case err := <-recovered:
		if err != nil {
			t.Fatalf("Recover: %v", err)
		}
	case <-time.After(extractionGuard):
		t.Fatal("Recover waited for the settlement of the batch")
	}
	if n := len(lib.calls()); n != 0 {
		t.Fatalf("the library was asked for the batch during Recover: %d calls", n)
	}

	go restarted.RunSettler(t.Context())
	select {
	case <-lib.entered:
	case <-time.After(extractionGuard):
		t.Fatal("the batch was not settled in the background")
	}
	if job := r.statusOf(t, id); job.Status != domain.StatusFiltering {
		t.Errorf("job while the batch is being settled = %s, want filtering", job.Status)
	}
	if _, err := restarted.List(t.Context()); err != nil {
		t.Errorf("List while the batch is being settled: %v", err)
	}

	close(lib.gate)
	r.waitDone(t, id)
}

// TestShuttingDownStopsTheSettlementAndTheNextStartDecidesTheBatch: a stop during the settlement
// returns at once, the batch is not failed by it, and the next start decides it.
func TestShuttingDownStopsTheSettlementAndTheNextStartDecidesTheBatch(t *testing.T) {
	t.Parallel()

	r, id := settlingRig(t)
	lib := newGatedLibrary(t)
	restarted := r.service(ingestionsqlite.NewFilterRepository(r.conn), r.trash).WithLibrary(lib)
	r.svc = restarted
	if _, err := restarted.Recover(t.Context()); err != nil {
		t.Fatalf("Recover: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	settled := make(chan struct{})
	go func() {
		restarted.RunSettler(ctx)
		close(settled)
	}()
	select {
	case <-lib.entered:
	case <-time.After(extractionGuard):
		t.Fatal("the batch was not settled in the background")
	}
	cancel()
	select {
	case <-settled:
	case <-time.After(extractionGuard):
		t.Fatal("the settlement did not stop with its context")
	}

	if job := r.statusOf(t, id); job.Status != domain.StatusFiltering || !job.FilterDone {
		t.Errorf("job after the stop = %s (%q), want it still filtered and waiting", job.Status, job.Error)
	}
	next := r.service(ingestionsqlite.NewFilterRepository(r.conn), r.trash).WithLibrary(newFakeLibrary(t))
	r.svc = next
	settling, stop := context.WithCancel(t.Context())
	defer stop()
	go next.RunSettler(settling)
	if err := next.WaitSettled(t.Context()); err != nil {
		t.Fatalf("WaitSettled: %v", err)
	}
	r.waitDone(t, id)
}

// settlingRig is a batch of one archive with a video, filtered and waiting for its settlement, as a
// start finds it when no library was there to decide it.
func settlingRig(t *testing.T) (*rig, domain.JobID) {
	t.Helper()

	r := newRig(t, realExtractor(), 1<<40, true, nil)
	id := r.upload(t, "Movie.zip", videoZip(t))
	r.waitFor(t, id, "filtered, waiting for its batch", func(j domain.UploadJob) bool {
		return j.Status == domain.StatusFiltering && j.FilterDone
	})
	return r, id
}

// listingService is the service of the rig over a staging area that lists its uploads, as the tus
// adapter does: Purge looks for the staged files that no job knows.
func (r *rig) listingService() *application.Service {
	space := func(string) (uint64, error) { return 1 << 40, nil }
	return application.NewService(ingestionsqlite.NewBatchRepository(r.conn), ingestionsqlite.NewJobRepository(r.conn), r.pub, nil, time.Now).
		WithStaging(listingStaging{diskStaging{dir: r.staging}}).
		WithExtraction(workspace.New(r.staging, space), realExtractor(), r.trash).
		WithFiltering(ingestionsqlite.NewFilterRepository(r.conn), filterRules())
}

// listingStaging is the staging folder with its uploads listed, as the tus adapter lists them.
type listingStaging struct{ diskStaging }

func (l listingStaging) IDs(context.Context) ([]domain.JobID, error) {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return nil, err
	}
	var ids []domain.JobID
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ids = append(ids, domain.JobID(strings.SplitN(e.Name(), ".", 2)[0]))
	}
	return ids, nil
}

// gatedLibrary is No asignados with a gate: its moves wait until the gate opens, and a call stops
// with its context, as a shutdown stops it.
type gatedLibrary struct {
	*fakeLibrary
	entered chan struct{}
	gate    chan struct{}
}

func newGatedLibrary(t *testing.T) gatedLibrary {
	t.Helper()
	return gatedLibrary{fakeLibrary: newFakeLibrary(t), entered: make(chan struct{}, 8), gate: make(chan struct{})}
}

// PutAside implements application.Library.
func (g gatedLibrary) PutAside(ctx context.Context, req application.AsideRequest) (map[string]string, error) {
	g.entered <- struct{}{}
	select {
	case <-g.gate:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return g.fakeLibrary.PutAside(ctx, req)
}
