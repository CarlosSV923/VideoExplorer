package application_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	ingestionsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
)

// TestTheStateIsSavedBeforeTheOriginalGoesToTheTrash: the job is filtering when its original
// moves, so a cut between the two leaves a job that the start can finish (finish, trashLeftovers).
func TestTheStateIsSavedBeforeTheOriginalGoesToTheTrash(t *testing.T) {
	t.Parallel()

	r := newRig(t, realExtractor(), 1<<40, true, nil)
	var mu sync.Mutex
	var seen []domain.Status
	r.trash.before = func(ctx context.Context, p application.TrashPut) {
		views, err := r.svc.List(ctx)
		if err != nil {
			return
		}
		for _, v := range views {
			if v.Job.FileName == p.Name {
				mu.Lock()
				seen = append(seen, v.Job.Status)
				mu.Unlock()
			}
		}
	}

	r.upload(t, "Movie.zip", videoZip(t))
	r.waitTrashed(t, 1)

	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(seen, []domain.Status{domain.StatusFiltering}) {
		t.Errorf("job status when the original went to the trash = %v, want [filtering]", seen)
	}
}

// TestATrashFailureSurvivesARestartUntilItIsRetried: the original did not reach the trash, so its
// job failed with trash_failed and the archive stays in staging. A restart does not move it on its
// own, and does not delete it: Reintentar moves it, once, and the job goes on.
func TestATrashFailureSurvivesARestartUntilItIsRetried(t *testing.T) {
	t.Parallel()

	r := newRig(t, realExtractor(), 1<<40, true, nil)
	r.trash.setErr(errors.New("input/output error"))
	id := r.upload(t, "Movie.zip", videoZip(t))
	r.waitFor(t, id, "failed with trash_failed", func(j domain.UploadJob) bool {
		return j.Status == domain.StatusFailed && j.Error == domain.FailureTrashFailed
	})
	if !r.staged(id) {
		t.Fatal("the original left staging although its move to the trash failed")
	}

	r.trash.setErr(nil)
	if _, err := r.svc.Recover(t.Context()); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if !r.staged(id) || len(r.trash.puts()) != 0 {
		t.Error("the restart moved the original on its own: it waits for Reintentar")
	}
	if job, err := r.svc.Job(t.Context(), id); err != nil || job.Status != domain.StatusFailed || job.Error != domain.FailureTrashFailed {
		t.Errorf("job after the restart = %+v, %v; want failed with trash_failed", job, err)
	}

	if _, err := r.svc.Retry(t.Context(), id); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	r.reachedFilter(t, id)
	if r.staged(id) {
		t.Error("the original is still in staging after its retry")
	}
	if n := len(r.trash.puts()); n != 1 {
		t.Errorf("trash entries = %d, want the original once", n)
	}
}

// TestASetWhoseVolumeCannotGoToTheTrashFailsAsAWhole: one volume of a set fails its move to the
// trash. The set fails together with trash_failed; the volumes that moved are in the trash, the
// one that did not stays in the set's folder, and a restart does not move it on its own. Reintentar
// moves only the volume that is left, and the set goes on with every volume in the trash once.
func TestASetWhoseVolumeCannotGoToTheTrashFailsAsAWhole(t *testing.T) {
	t.Parallel()

	names, data, _ := splitSet(t, "")
	r := newRig(t, realExtractor(), 1<<40, true, nil)
	r.trash.mu.Lock()
	r.trash.failName = names[1]
	r.trash.mu.Unlock()

	ids := r.volumeJobs(t, names, data, indexes(len(names)), len(names))
	for _, id := range ids {
		r.waitFor(t, id, "failed with trash_failed", func(j domain.UploadJob) bool {
			return j.Status == domain.StatusFailed && j.Error == domain.FailureTrashFailed
		})
	}
	if got := len(r.trash.puts()); got != len(names)-1 {
		t.Fatalf("trash entries = %d, want every volume but the one that failed", got)
	}
	set := filepath.Join(r.staging, "volumes", string(ids[0]))
	if _, err := os.Stat(filepath.Join(set, names[1])); err != nil {
		t.Fatalf("the volume that did not move is not in the set's folder: %v", err)
	}

	if _, err := r.svc.Recover(t.Context()); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if got := len(r.trash.puts()); got != len(names)-1 {
		t.Errorf("trash entries after the restart = %d: a restart moved a volume on its own", got)
	}

	if _, err := r.svc.Retry(t.Context(), ids[1]); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	for _, id := range ids {
		r.waitFor(t, id, "filtering with its content filtered", func(j domain.UploadJob) bool {
			return j.Status == domain.StatusFiltering && j.FilterDone
		})
	}
	seen := map[string]int{}
	for _, p := range r.trash.puts() {
		seen[p.Name]++
	}
	for _, n := range names {
		if seen[n] != 1 {
			t.Errorf("volume %s went to the trash %d times, want once", n, seen[n])
		}
	}
}

// TestAnOriginalCutBeforeItsMoveIsTrashedByTheRestart: the worker stops while the original is
// moving, after its job was saved as filtering. The restart moves the original to the trash, keeps
// the extracted content it still needs, and filters it: nothing is moved twice.
func TestAnOriginalCutBeforeItsMoveIsTrashedByTheRestart(t *testing.T) {
	t.Parallel()

	r := newRig(t, realExtractor(), 1<<40, false, nil)
	cut := &cuttingTrash{started: make(chan struct{}, 1)}
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	first := r.service(ingestionsqlite.NewFilterRepository(r.conn), cut)
	stopped := make(chan struct{})
	go func() {
		first.RunExtraction(ctx, 1)
		close(stopped)
	}()
	r.svc = first

	id := r.upload(t, "Movie.zip", videoZip(t))
	select {
	case <-cut.started:
	case <-time.After(extractionGuard):
		t.Fatal("the original was never asked for its move")
	}
	cancel()
	<-stopped

	restarted := r.service(ingestionsqlite.NewFilterRepository(r.conn), r.trash)
	if _, err := restarted.Recover(t.Context()); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if r.staged(id) {
		t.Error("the original is still in staging after the restart")
	}
	r.svc = restarted
	resumeCtx, resumeCancel := context.WithCancel(t.Context())
	t.Cleanup(resumeCancel)
	go restarted.RunExtraction(resumeCtx, 1)

	job := r.reachedFilter(t, id)
	if job.Status != domain.StatusFiltering || !job.FilterDone {
		t.Errorf("job after the restart = %s (%q), want it filtered and kept", job.Status, job.Error)
	}
	if n := len(r.trashedNamed("Movie.zip")); n != 1 {
		t.Errorf("the original went to the trash %d times, want once", n)
	}
}

// cuttingTrash stands for a move that a shutdown interrupts: it waits for the worker's context to end and
// then fails, as a move cut by the stop does.
type cuttingTrash struct {
	started chan struct{}
}

func (c *cuttingTrash) Put(ctx context.Context, _ application.TrashPut) error {
	c.started <- struct{}{}
	<-ctx.Done()
	return ctx.Err()
}
