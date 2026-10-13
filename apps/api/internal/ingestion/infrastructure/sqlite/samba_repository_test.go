package sqlite_test

import (
	"errors"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
)

// sambaJob stores a samba batch with one job in the given status, and returns both.
func sambaJob(t *testing.T, r *repos, batchID, jobID string, status domain.Status) (*domain.Batch, *domain.UploadJob) {
	t.Helper()
	now := time.Date(2026, time.October, 13, 9, 0, 0, 0, time.UTC)
	batch, err := domain.NewSambaBatch(domain.BatchID(batchID), "movie", now)
	if err != nil {
		t.Fatal(err)
	}
	job, err := domain.NewSambaJob(domain.JobID(jobID), batch.ID, "movie.zip", 1024, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.batches.Create(t.Context(), batch); err != nil {
		t.Fatalf("Create batch: %v", err)
	}
	if err := r.jobs.Create(t.Context(), job); err != nil {
		t.Fatalf("Create job: %v", err)
	}
	if status != domain.StatusUploading {
		job.Status = status
		job.UpdatedAt = now.Add(time.Minute)
		if err := r.jobs.Save(t.Context(), job); err != nil {
			t.Fatalf("Save job: %v", err)
		}
	}
	return batch, job
}

// repos are the batch and job repositories over one database.
type repos struct {
	batches *sqlite.BatchRepository
	jobs    *sqlite.JobRepository
}

func TestSambaBatchesAndJobsKeepTheirOrigin(t *testing.T) {
	t.Parallel()

	conn := open(t, t.TempDir())
	r := &repos{batches: sqlite.NewBatchRepository(conn), jobs: sqlite.NewJobRepository(conn)}
	batch, job := sambaJob(t, r, "samba-batch", "samba-job", domain.StatusUploading)

	got, err := r.batches.Get(t.Context(), batch.ID)
	if err != nil || got.Origin != domain.OriginSamba {
		t.Fatalf("Get batch = %+v, %v; want origin samba", got, err)
	}
	gotJob, err := r.jobs.Get(t.Context(), job.ID)
	if err != nil || !gotJob.IsSamba() {
		t.Fatalf("Get job = %+v, %v; want origin samba", gotJob, err)
	}
	recent, err := r.jobs.Recent(t.Context(), 10)
	if err != nil || len(recent) != 1 || recent[0].Batch.Origin != domain.OriginSamba || !recent[0].Job.IsSamba() {
		t.Fatalf("Recent = %+v, %v; want the job with its samba batch", recent, err)
	}
}

func TestOpenSambaListsOnlyTheUnitsThatHaveNotEnded(t *testing.T) {
	t.Parallel()

	conn := open(t, t.TempDir())
	r := &repos{batches: sqlite.NewBatchRepository(conn), jobs: sqlite.NewJobRepository(conn)}
	sambaJob(t, r, "b-open", "j-open", domain.StatusUploaded)
	sambaJob(t, r, "b-failed", "j-failed", domain.StatusFailed)
	sambaJob(t, r, "b-done", "j-done", domain.StatusDone)
	sambaJob(t, r, "b-discarded", "j-discarded", domain.StatusDiscarded)
	sambaJob(t, r, "b-cancelled", "j-cancelled", domain.StatusCancelled)
	upload, err := domain.NewBatch("b-upload", "", domain.RelationNone, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := r.batches.Create(t.Context(), upload); err != nil {
		t.Fatal(err)
	}
	uj, err := domain.NewUploadJob("j-upload", domain.UploadMeta{FileName: "x.zip", BatchID: "b-upload"}, 10, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := r.jobs.Create(t.Context(), uj); err != nil {
		t.Fatal(err)
	}

	open, err := r.jobs.OpenSamba(t.Context())
	if err != nil {
		t.Fatalf("OpenSamba: %v", err)
	}
	var ids []domain.JobID
	for _, j := range open {
		ids = append(ids, j.ID)
	}
	want := []domain.JobID{"j-open", "j-failed"}
	if len(ids) != len(want) || ids[0] != want[0] || ids[1] != want[1] {
		t.Fatalf("OpenSamba = %v, want %v (no done, discarded or cancelled copy, no upload)", ids, want)
	}
}

func TestDeletingAJobAndItsBatchForgetsBoth(t *testing.T) {
	t.Parallel()

	conn := open(t, t.TempDir())
	r := &repos{batches: sqlite.NewBatchRepository(conn), jobs: sqlite.NewJobRepository(conn)}
	batch, job := sambaJob(t, r, "b-gone", "j-gone", domain.StatusUploading)

	if err := r.jobs.Delete(t.Context(), job.ID); err != nil {
		t.Fatalf("Delete job: %v", err)
	}
	if err := r.batches.Delete(t.Context(), batch.ID); err != nil {
		t.Fatalf("Delete batch: %v", err)
	}
	if _, err := r.jobs.Get(t.Context(), job.ID); !errors.Is(err, domain.ErrJobNotFound) {
		t.Fatalf("Get deleted job: %v, want ErrJobNotFound", err)
	}
	if _, err := r.batches.Get(t.Context(), batch.ID); !errors.Is(err, domain.ErrBatchNotFound) {
		t.Fatalf("Get deleted batch: %v, want ErrBatchNotFound", err)
	}
}
