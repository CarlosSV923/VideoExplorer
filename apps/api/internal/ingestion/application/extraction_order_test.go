package application_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
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

// TestARestartMovesAnOriginalThatStayedInStaging: the trash failed and the job went on, so the
// original is still in staging of a job that is filtering. The start moves it to the trash.
func TestARestartMovesAnOriginalThatStayedInStaging(t *testing.T) {
	t.Parallel()

	r := newRig(t, realExtractor(), 1<<40, true, nil)
	r.trash.setErr(errors.New("input/output error"))
	id := r.upload(t, "Movie.zip", videoZip(t))
	r.waitFor(t, id, "filtering", func(j domain.UploadJob) bool {
		return j.Status == domain.StatusFiltering
	})

	deadline := time.Now().Add(extractionGuard)
	for r.trash.calls() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the trash was never asked for the original")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !r.staged(id) {
		t.Fatal("the original left staging although the trash failed")
	}

	r.trash.setErr(nil)
	if _, err := r.svc.Recover(t.Context()); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if r.staged(id) {
		t.Error("the original is still in staging after the restart")
	}
	if n := len(r.trash.puts()); n != 1 {
		t.Errorf("trash entries = %d, want the original once", n)
	}
	if job, err := r.svc.Job(t.Context(), id); err != nil || job.Status != domain.StatusFiltering {
		t.Errorf("job after the restart = %+v, %v; want it still filtering", job, err)
	}
}

// TestASetCutInTheMiddleOfItsTrashIsFinishedAfterARestart: the first volume of a set reaches the
// trash, the second one fails to, and the job goes on with the set in staging. The restart moves
// the rest of the set to the trash, one volume at a time, and nothing is moved twice.
func TestASetCutInTheMiddleOfItsTrashIsFinishedAfterARestart(t *testing.T) {
	t.Parallel()

	names, data, _ := splitSet(t, "")
	r := newRig(t, realExtractor(), 1<<40, true, nil)
	r.trash.mu.Lock()
	r.trash.failName = names[1]
	r.trash.mu.Unlock()

	ids := r.volumeJobs(t, names, data, indexes(len(names)), len(names))
	for _, id := range ids {
		r.waitFor(t, id, "filtering", func(j domain.UploadJob) bool { return j.Status == domain.StatusFiltering })
	}
	deadline := time.Now().Add(extractionGuard)
	for r.trash.calls() < len(names) {
		if time.Now().After(deadline) {
			t.Fatalf("the trash was asked for %d volumes, want %d", r.trash.calls(), len(names))
		}
		time.Sleep(5 * time.Millisecond)
	}
	for _, p := range r.trash.puts() {
		if p.Name == names[1] {
			t.Fatalf("the second volume reached the trash although its move failed")
		}
	}
	if got := len(r.trash.puts()); got != len(names)-1 {
		t.Fatalf("trash entries before the restart = %d, want every volume but the second", got)
	}

	if _, err := r.svc.Recover(t.Context()); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	puts := r.trash.puts()
	if len(puts) != len(names) {
		t.Fatalf("trash entries after the restart = %d, want %d (one per volume)", len(puts), len(names))
	}
	seen := map[string]int{}
	for _, p := range puts {
		seen[p.Name]++
	}
	for _, n := range names {
		if seen[n] != 1 {
			t.Errorf("volume %s went to the trash %d times, want once", n, seen[n])
		}
	}
	for _, id := range ids {
		if got := r.statusOf(t, id); got.Status != domain.StatusFiltering {
			t.Errorf("job %s after the restart = %s, want it still filtering", id, got.Status)
		}
	}
}
