package application_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// recorder keeps the job changes the service publishes, in order.
type recorder struct {
	mu    sync.Mutex
	views []domain.JobView
}

func (r *recorder) Publish(v domain.JobView) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.views = append(r.views, v)
}

// published returns the changes announced for one job, in order.
func (r *recorder) published(id domain.JobID) []domain.JobView {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.JobView
	for _, v := range r.views {
		if v.Job.ID == id {
			out = append(out, v)
		}
	}
	return out
}

// fakeStaging is an in-memory staging area: an upload is staged while it is in files.
// inspectErr and removeErr make the operations of one upload fail, as a damaged file would.
type fakeStaging struct {
	mu         sync.Mutex
	files      map[domain.JobID]application.StagedUpload
	removed    []domain.JobID
	inspectErr map[domain.JobID]error
	removeErr  map[domain.JobID]error
}

func newFakeStaging() *fakeStaging {
	return &fakeStaging{
		files:      map[domain.JobID]application.StagedUpload{},
		inspectErr: map[domain.JobID]error{},
		removeErr:  map[domain.JobID]error{},
	}
}

// put stages an upload of 10 declared bytes, of which offset arrived.
func (f *fakeStaging) put(id domain.JobID, offset int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[id] = application.StagedUpload{Exists: true, Size: 10, Offset: offset}
}

func (f *fakeStaging) Inspect(_ context.Context, id domain.JobID) (application.StagedUpload, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.inspectErr[id]; err != nil {
		return application.StagedUpload{}, err
	}
	return f.files[id], nil
}

func (f *fakeStaging) Remove(_ context.Context, id domain.JobID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.removeErr[id]; err != nil {
		return err
	}
	delete(f.files, id)
	f.removed = append(f.removed, id)
	return nil
}

func (f *fakeStaging) IDs(context.Context) ([]domain.JobID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := make([]domain.JobID, 0, len(f.files))
	for id := range f.files {
		ids = append(ids, id)
	}
	return ids, nil
}

func (f *fakeStaging) staged(id domain.JobID) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.files[id]
	return ok
}

// admit starts an upload of 10 bytes in batch and returns its job id.
func admit(t *testing.T, svc *application.Service, batch domain.BatchID, name string) domain.JobID {
	t.Helper()

	id, err := svc.Admit(t.Context(), application.Admission{
		Meta: domain.UploadMeta{FileName: name, BatchID: string(batch)},
		Size: 10,
	})
	if err != nil {
		t.Fatalf("Admit %s: %v", name, err)
	}
	return id
}

func TestListReturnsTheHundredMostRecentJobsNewestFirst(t *testing.T) {
	t.Parallel()

	clock := start
	svc, _ := service(t, t.TempDir(), &clock)
	ctx := t.Context()
	batch, err := svc.CreateBatch(ctx, "Show", domain.RelationEpisodes)
	if err != nil {
		t.Fatal(err)
	}

	const total = 105
	ids := make([]domain.JobID, 0, total)
	for i := range total {
		clock = start.Add(time.Duration(i) * time.Second)
		ids = append(ids, admit(t, svc, batch.ID, fmt.Sprintf("e%03d.mkv", i)))
	}

	views, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(views) != 100 {
		t.Fatalf("List returned %d jobs, want 100", len(views))
	}
	for i, v := range views {
		want := ids[total-1-i]
		if v.Job.ID != want {
			t.Fatalf("position %d = %s, want %s (newest first)", i, v.Job.ID, want)
		}
		if v.Batch.ID != batch.ID || v.Batch.Name != "Show" || v.Batch.Relation != domain.RelationEpisodes {
			t.Fatalf("position %d batch = %+v, want the batch of the upload", i, v.Batch)
		}
	}
}

func TestListOrdersJobsOfTheSameInstantByInsertion(t *testing.T) {
	t.Parallel()

	clock := start
	svc, _ := service(t, t.TempDir(), &clock)
	batch, err := svc.CreateBatch(t.Context(), "", domain.RelationNone)
	if err != nil {
		t.Fatal(err)
	}
	first := admit(t, svc, batch.ID, "a.mkv")
	second := admit(t, svc, batch.ID, "b.mkv")

	views, err := svc.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 2 || views[0].Job.ID != second || views[1].Job.ID != first {
		t.Fatalf("order = %s, %s; want the later insert first", views[0].Job.ID, views[1].Job.ID)
	}
}

func TestProgressIsRecordedAndAnnouncedOnlyWhenItMoves(t *testing.T) {
	t.Parallel()

	clock := start
	pub := &recorder{}
	svc, _ := serviceWith(t, t.TempDir(), &clock, pub)
	ctx := t.Context()
	batch, err := svc.CreateBatch(ctx, "", domain.RelationNone)
	if err != nil {
		t.Fatal(err)
	}
	id := admit(t, svc, batch.ID, "a.mkv")

	clock = start.Add(time.Second)
	if err := svc.UploadProgress(ctx, id, 4); err != nil {
		t.Fatalf("UploadProgress: %v", err)
	}
	if err := svc.UploadProgress(ctx, id, 4); err != nil {
		t.Fatalf("repeated UploadProgress: %v", err)
	}
	if err := svc.UploadProgress(ctx, id, 2); err != nil {
		t.Fatalf("UploadProgress going back: %v", err)
	}
	job, err := svc.Job(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if job.Received != 4 || !job.UpdatedAt.Equal(clock) {
		t.Errorf("job received %d updated %v, want 4 updated %v", job.Received, job.UpdatedAt, clock)
	}

	// The admission and the one real change are announced; the repeats are not.
	if got := len(pub.published(id)); got != 2 {
		t.Errorf("announced %d changes, want 2 (admission and progress)", got)
	}
	if last := pub.published(id)[1]; last.Job.Received != 4 || last.Job.Status != domain.StatusUploading {
		t.Errorf("last announcement = received %d status %s", last.Job.Received, last.Job.Status)
	}

	// Progress that arrives after the upload ended is ignored, not refused.
	if err := svc.Completed(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := svc.UploadProgress(ctx, id, 9); err != nil {
		t.Errorf("late progress: err = %v, want nil", err)
	}
}

func TestCompletedIsIdempotentAndRefusedAfterCancel(t *testing.T) {
	t.Parallel()

	clock := start
	svc, _ := service(t, t.TempDir(), &clock)
	ctx := t.Context()
	batch, err := svc.CreateBatch(ctx, "", domain.RelationNone)
	if err != nil {
		t.Fatal(err)
	}
	done := admit(t, svc, batch.ID, "a.mkv")
	cancelled := admit(t, svc, batch.ID, "b.mkv")

	// The purge may record an upload as complete a moment before the endpoint does.
	for range 2 {
		if err := svc.Completed(ctx, done); err != nil {
			t.Fatalf("Completed: %v", err)
		}
	}
	if job, _ := svc.Job(ctx, done); job.Status != domain.StatusUploaded || job.Received != 10 {
		t.Errorf("job = %s received %d, want uploaded with all bytes", job.Status, job.Received)
	}

	if err := svc.Terminated(ctx, cancelled); err != nil {
		t.Fatal(err)
	}
	if err := svc.Completed(ctx, cancelled); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("Completed of a cancelled job: err = %v, want ErrInvalidTransition", err)
	}
}

func TestEveryChangeIsAnnouncedWithItsBatch(t *testing.T) {
	t.Parallel()

	clock := start
	pub := &recorder{}
	svc, _ := serviceWith(t, t.TempDir(), &clock, pub)
	ctx := t.Context()
	batch, err := svc.CreateBatch(ctx, "Iron Man", domain.RelationParts)
	if err != nil {
		t.Fatal(err)
	}
	id := admit(t, svc, batch.ID, "cd1.mkv")
	if err := svc.Completed(ctx, id); err != nil {
		t.Fatal(err)
	}

	got := pub.published(id)
	wantStatus := []domain.Status{domain.StatusUploading, domain.StatusUploaded}
	if len(got) != len(wantStatus) {
		t.Fatalf("announced %d changes, want %d", len(got), len(wantStatus))
	}
	for i, v := range got {
		if v.Job.Status != wantStatus[i] {
			t.Errorf("change %d status = %s, want %s", i, v.Job.Status, wantStatus[i])
		}
		if v.Batch.ID != batch.ID || v.Batch.Name != "Iron Man" || v.Batch.Relation != domain.RelationParts {
			t.Errorf("change %d batch = %+v", i, v.Batch)
		}
	}
}

func TestPurgeAbandonsIdleUploadsAndDeletesTheirFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		idleFor       time.Duration // how long before the purge the upload last changed
		wantAbandoned bool
	}{
		{name: "idle for a day and a second", idleFor: application.AbandonedAfter + time.Second, wantAbandoned: true},
		{name: "idle for less than a day", idleFor: 23 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clock := start
			pub := &recorder{}
			svc, _ := serviceWith(t, t.TempDir(), &clock, pub)
			staging := newFakeStaging()
			svc = svc.WithStaging(staging)
			ctx := t.Context()
			batch, err := svc.CreateBatch(ctx, "", domain.RelationNone)
			if err != nil {
				t.Fatal(err)
			}
			id := admit(t, svc, batch.ID, "a.mkv")
			staging.put(id, 4)

			clock = start.Add(tt.idleFor)
			res, err := svc.Purge(ctx)
			if err != nil {
				t.Fatalf("Purge: %v", err)
			}
			job, err := svc.Job(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if !tt.wantAbandoned {
				if res.Abandoned != 0 || job.Status != domain.StatusUploading || !staging.staged(id) {
					t.Errorf("result %+v, job %s; want the upload kept", res, job.Status)
				}
				return
			}
			if res.Abandoned != 1 || job.Status != domain.StatusFailed || job.Error != domain.FailureUploadAbandoned {
				t.Errorf("result %+v, job %s error %q; want abandoned", res, job.Status, job.Error)
			}
			if staging.staged(id) {
				t.Error("the partial file of the abandoned upload was kept")
			}
			if got := pub.published(id); len(got) == 0 || got[len(got)-1].Job.Error != domain.FailureUploadAbandoned {
				t.Error("the abandonment was not announced")
			}
		})
	}
}

func TestPurgeDeletesStagedFilesThatNoJobKnows(t *testing.T) {
	t.Parallel()

	clock := start
	svc, _ := service(t, t.TempDir(), &clock)
	staging := newFakeStaging()
	svc = svc.WithStaging(staging)
	ctx := t.Context()
	batch, err := svc.CreateBatch(ctx, "", domain.RelationNone)
	if err != nil {
		t.Fatal(err)
	}
	known := admit(t, svc, batch.ID, "a.mkv")
	staging.put(known, 10)
	const orphan domain.JobID = "no-job-behind-this"
	staging.put(orphan, 3)

	res, err := svc.Purge(ctx)
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if res.Orphans != 1 {
		t.Errorf("orphans = %d, want 1", res.Orphans)
	}
	if staging.staged(orphan) {
		t.Error("the orphan staged file was kept")
	}
	if !staging.staged(known) {
		t.Error("the staged file of a known job was deleted")
	}
}

func TestPurgeReconcilesBeforeItAbandons(t *testing.T) {
	t.Parallel()

	clock := start
	svc, _ := service(t, t.TempDir(), &clock)
	staging := newFakeStaging()
	svc = svc.WithStaging(staging)
	ctx := t.Context()
	batch, err := svc.CreateBatch(ctx, "", domain.RelationNone)
	if err != nil {
		t.Fatal(err)
	}
	// Every byte arrived, but the job was never recorded as uploaded: it is not abandoned.
	id := admit(t, svc, batch.ID, "a.mkv")
	staging.put(id, 10)

	clock = start.Add(application.AbandonedAfter + time.Hour)
	res, err := svc.Purge(ctx)
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	job, _ := svc.Job(ctx, id)
	if res.Completed != 1 || res.Abandoned != 0 || job.Status != domain.StatusUploaded {
		t.Errorf("result %+v, job %s; want the complete upload recorded as uploaded", res, job.Status)
	}
	if !staging.staged(id) {
		t.Error("the complete upload was deleted")
	}
}

func TestRecoverCompletesUploadsThatFinishedBeforeARestart(t *testing.T) {
	t.Parallel()

	clock := start
	dir := t.TempDir()
	svc, conn := service(t, dir, &clock)
	staging := newFakeStaging()
	svc = svc.WithStaging(staging)
	ctx := t.Context()
	batch, err := svc.CreateBatch(ctx, "", domain.RelationNone)
	if err != nil {
		t.Fatal(err)
	}
	finished := admit(t, svc, batch.ID, "a.mkv")
	partial := admit(t, svc, batch.ID, "b.mkv")
	staging.put(finished, 10)
	staging.put(partial, 5)
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	// After a restart the job still says uploading; Recover settles it.
	svc, _ = service(t, dir, &clock)
	svc = svc.WithStaging(staging)
	res, err := svc.Recover(ctx)
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if res.Completed != 1 || res.Missing != 0 {
		t.Errorf("result %+v, want one completed upload", res)
	}
	if job, _ := svc.Job(ctx, finished); job.Status != domain.StatusUploaded || job.Received != 10 {
		t.Errorf("finished job = %s received %d, want uploaded", job.Status, job.Received)
	}
	if job, _ := svc.Job(ctx, partial); job.Status != domain.StatusUploading || job.Received != 5 {
		t.Errorf("partial job = %s received %d, want still uploading with the 5 bytes of its file", job.Status, job.Received)
	}
}

func TestRecoverFailsUploadsWhoseFileIsGone(t *testing.T) {
	t.Parallel()

	clock := start
	svc, _ := service(t, t.TempDir(), &clock)
	staging := newFakeStaging()
	svc = svc.WithStaging(staging)
	ctx := t.Context()
	batch, err := svc.CreateBatch(ctx, "", domain.RelationNone)
	if err != nil {
		t.Fatal(err)
	}
	gone := admit(t, svc, batch.ID, "a.mkv")

	res, err := svc.Recover(ctx)
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	job, _ := svc.Job(ctx, gone)
	if res.Missing != 1 || job.Status != domain.StatusFailed || job.Error != domain.FailureUploadMissing {
		t.Errorf("result %+v, job %s error %q; want failed upload_missing", res, job.Status, job.Error)
	}
}

func TestPurgeLeavesAJustAdmittedUploadWithoutItsFileAlone(t *testing.T) {
	t.Parallel()

	clock := start
	svc, _ := service(t, t.TempDir(), &clock)
	staging := newFakeStaging()
	svc = svc.WithStaging(staging)
	ctx := t.Context()
	batch, err := svc.CreateBatch(ctx, "", domain.RelationNone)
	if err != nil {
		t.Fatal(err)
	}
	// Admitted, and tus has not written the file yet: a purge right now must not fail it.
	id := admit(t, svc, batch.ID, "a.mkv")
	if res, err := svc.Purge(ctx); err != nil || res.Missing != 0 {
		t.Fatalf("Purge = %+v, %v; want the job left alone", res, err)
	}
	if job, _ := svc.Job(ctx, id); job.Status != domain.StatusUploading {
		t.Fatalf("job = %s right after admission, want uploading", job.Status)
	}

	clock = start.Add(2 * time.Minute)
	res, err := svc.Purge(ctx)
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if job, _ := svc.Job(ctx, id); res.Missing != 1 || job.Error != domain.FailureUploadMissing {
		t.Errorf("result %+v, job error %q; want upload_missing once the grace has passed", res, job.Error)
	}
}

func TestRunPurgeReturnsWhenItsContextEnds(t *testing.T) {
	t.Parallel()

	clock := start
	svc, _ := service(t, t.TempDir(), &clock)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	done := make(chan struct{})
	go func() {
		svc.RunPurge(ctx, time.Hour)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(guard):
		t.Fatal("RunPurge did not return after its context ended")
	}
}

// guard bounds the waits of the tests; a healthy run never gets near it.
const guard = 10 * time.Second

func TestOneUnreadableUploadDoesNotStopTheOthersFromBeingSettled(t *testing.T) {
	t.Parallel()

	clock := start
	svc, _ := service(t, t.TempDir(), &clock)
	staging := newFakeStaging()
	svc = svc.WithStaging(staging)
	ctx := t.Context()
	batch, err := svc.CreateBatch(ctx, "", domain.RelationNone)
	if err != nil {
		t.Fatal(err)
	}
	damaged := admit(t, svc, batch.ID, "damaged.mkv")
	finished := admit(t, svc, batch.ID, "finished.mkv")
	staging.inspectErr[damaged] = errors.New("info file is damaged")
	staging.put(finished, 10)

	res, err := svc.Recover(ctx)
	if err != nil {
		t.Fatalf("Recover: %v, want the damaged upload skipped", err)
	}
	if res.Completed != 1 {
		t.Errorf("result %+v, want the finished upload settled", res)
	}
	if job, _ := svc.Job(ctx, finished); job.Status != domain.StatusUploaded {
		t.Errorf("finished job = %s, want uploaded", job.Status)
	}
	if job, _ := svc.Job(ctx, damaged); job.Status != domain.StatusUploading {
		t.Errorf("damaged job = %s, want left as it is", job.Status)
	}
}

func TestOneUploadThatCannotBeRemovedDoesNotStopThePurge(t *testing.T) {
	t.Parallel()

	clock := start
	svc, _ := service(t, t.TempDir(), &clock)
	staging := newFakeStaging()
	svc = svc.WithStaging(staging)
	ctx := t.Context()
	batch, err := svc.CreateBatch(ctx, "", domain.RelationNone)
	if err != nil {
		t.Fatal(err)
	}
	stuck := admit(t, svc, batch.ID, "stuck.mkv")
	idle := admit(t, svc, batch.ID, "idle.mkv")
	staging.put(stuck, 4)
	staging.put(idle, 4)
	staging.removeErr[stuck] = errors.New("permission denied")

	clock = start.Add(application.AbandonedAfter + time.Hour)
	res, err := svc.Purge(ctx)
	if err != nil {
		t.Fatalf("Purge: %v, want the stuck upload skipped", err)
	}
	if res.Abandoned != 1 {
		t.Errorf("result %+v, want the other idle upload abandoned", res)
	}
	if job, _ := svc.Job(ctx, idle); job.Error != domain.FailureUploadAbandoned {
		t.Errorf("idle job error = %q, want upload_abandoned", job.Error)
	}
	if job, _ := svc.Job(ctx, stuck); job.Status != domain.StatusUploading {
		t.Errorf("stuck job = %s, want uploading until the next purge retries", job.Status)
	}
}
