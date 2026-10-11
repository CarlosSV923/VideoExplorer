package sqlite_test

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/database"
)

var created = time.Date(2026, time.October, 10, 12, 30, 15, 123456789, time.UTC)

// open opens the database in dir, which the test can reopen to simulate a restart.
func open(t *testing.T, dir string) *sql.DB {
	t.Helper()

	conn, err := database.Open(t.Context(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestBatchRoundTrip(t *testing.T) {
	t.Parallel()

	conn := open(t, t.TempDir())
	batches := sqlite.NewBatchRepository(conn)
	ctx := t.Context()

	if _, err := batches.Get(ctx, "missing"); !errors.Is(err, domain.ErrBatchNotFound) {
		t.Fatalf("Get of a missing batch: err = %v, want ErrBatchNotFound", err)
	}

	want := domain.Batch{ID: "b-1", Name: "Iron Man", Relation: domain.RelationParts, CreatedAt: created}
	if err := batches.Create(ctx, &want); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := batches.Get(ctx, "b-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != want.ID || got.Name != want.Name || got.Relation != want.Relation || !got.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("batch = %+v, want %+v", *got, want)
	}
}

func TestBatchRejectsAnUnknownRelation(t *testing.T) {
	t.Parallel()

	conn := open(t, t.TempDir())
	batches := sqlite.NewBatchRepository(conn)

	// The table only holds the three relations of RF-03.
	err := batches.Create(t.Context(), &domain.Batch{ID: "b-1", Relation: "r00", CreatedAt: created})
	if err == nil {
		t.Fatal("a batch with an unknown relation was stored")
	}
}

func TestUploadJobRoundTripAndStateAfterRestart(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	ctx := t.Context()

	conn := open(t, dir)
	batches := sqlite.NewBatchRepository(conn)
	jobs := sqlite.NewJobRepository(conn)
	if err := batches.Create(ctx, &domain.Batch{ID: "b-1", Relation: domain.RelationNone, CreatedAt: created}); err != nil {
		t.Fatal(err)
	}
	job, err := domain.NewUploadJob("job-1", domain.UploadMeta{FileName: "E01.mkv", RelativePath: "Show/E01.mkv", BatchID: "b-1"}, 4096, created)
	if err != nil {
		t.Fatal(err)
	}
	if err := jobs.Create(ctx, job); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := job.MarkUploaded(created.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := jobs.Save(ctx, job); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	// A restart: the database is opened again and the job still is uploaded.
	conn = open(t, dir)
	got, err := sqlite.NewJobRepository(conn).Get(ctx, "job-1")
	if err != nil {
		t.Fatalf("Get after restart: %v", err)
	}
	if got.Status != domain.StatusUploaded {
		t.Errorf("status after restart = %s, want uploaded", got.Status)
	}
	if got.BatchID != "b-1" || got.FileName != "E01.mkv" || got.RelativePath != "Show/E01.mkv" || got.Size != 4096 {
		t.Errorf("job after restart = %+v, lost declared values", *got)
	}
	if !got.UpdatedAt.Equal(created.Add(time.Second)) {
		t.Errorf("updated at = %v, want the time of the change", got.UpdatedAt)
	}
}

func TestUploadJobFailureReasonIsKept(t *testing.T) {
	t.Parallel()

	conn := open(t, t.TempDir())
	jobs := sqlite.NewJobRepository(conn)
	ctx := t.Context()
	if err := sqlite.NewBatchRepository(conn).Create(ctx, &domain.Batch{ID: "b-2", Relation: domain.RelationNone, CreatedAt: created}); err != nil {
		t.Fatal(err)
	}

	job, err := domain.NewUploadJob("job-2", domain.UploadMeta{FileName: "a.mkv", BatchID: "b-2"}, 10, created)
	if err != nil {
		t.Fatal(err)
	}
	if err := jobs.Create(ctx, job); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := job.Fail(domain.FailureUploadMissing, created); err != nil {
		t.Fatal(err)
	}
	if err := jobs.Save(ctx, job); err != nil {
		t.Fatal(err)
	}

	got, err := jobs.Get(ctx, "job-2")
	if err != nil {
		t.Fatal(err)
	}
	if got.BatchID != "b-2" {
		t.Errorf("batch = %q, want b-2", got.BatchID)
	}
	if got.Status != domain.StatusFailed || got.Error != domain.FailureUploadMissing {
		t.Errorf("job = %s %q, want failed with its reason", got.Status, got.Error)
	}
}

func TestUploadJobRejectsAMissingBatch(t *testing.T) {
	t.Parallel()

	// The foreign key keeps a job from naming a batch that does not exist.
	conn := open(t, t.TempDir())
	job, err := domain.NewUploadJob("job-3", domain.UploadMeta{FileName: "a.mkv", BatchID: "nope"}, 10, created)
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlite.NewJobRepository(conn).Create(t.Context(), job); err == nil {
		t.Fatal("a job with a missing batch was stored")
	}
}

func TestUploadJobGetMissing(t *testing.T) {
	t.Parallel()

	conn := open(t, t.TempDir())
	if _, err := sqlite.NewJobRepository(conn).Get(t.Context(), "missing"); !errors.Is(err, domain.ErrJobNotFound) {
		t.Fatalf("err = %v, want ErrJobNotFound", err)
	}
}

func TestUploadJobProgressIsKept(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	conn := open(t, dir)
	jobs := sqlite.NewJobRepository(conn)
	ctx := t.Context()
	if err := sqlite.NewBatchRepository(conn).Create(ctx, &domain.Batch{ID: "b-p", Relation: domain.RelationNone, CreatedAt: created}); err != nil {
		t.Fatal(err)
	}
	job, err := domain.NewUploadJob("job-p", domain.UploadMeta{FileName: "a.mkv", BatchID: "b-p"}, 10, created)
	if err != nil {
		t.Fatal(err)
	}
	if err := jobs.Create(ctx, job); err != nil {
		t.Fatal(err)
	}
	job.RecordProgress(7, created.Add(time.Second))
	if err := jobs.Save(ctx, job); err != nil {
		t.Fatal(err)
	}

	got, err := jobs.Get(ctx, "job-p")
	if err != nil {
		t.Fatal(err)
	}
	if got.Received != 7 || got.Status != domain.StatusUploading {
		t.Errorf("job = %s received %d, want uploading with 7 bytes", got.Status, got.Received)
	}
}

func TestRecentJobsCarryTheirBatchNewestFirst(t *testing.T) {
	t.Parallel()

	conn := open(t, t.TempDir())
	batches := sqlite.NewBatchRepository(conn)
	jobs := sqlite.NewJobRepository(conn)
	ctx := t.Context()
	if err := batches.Create(ctx, &domain.Batch{ID: "b-r", Name: "Show", Relation: domain.RelationEpisodes, CreatedAt: created}); err != nil {
		t.Fatal(err)
	}
	// Same instant for the first two: the insertion order decides.
	for i, name := range []string{"E01.mkv", "E02.mkv", "E03.mkv"} {
		at := created
		if i == 2 {
			at = created.Add(time.Minute)
		}
		job, err := domain.NewUploadJob(domain.JobID("job-r"+name), domain.UploadMeta{FileName: name, BatchID: "b-r"}, 10, at)
		if err != nil {
			t.Fatal(err)
		}
		if err := jobs.Create(ctx, job); err != nil {
			t.Fatal(err)
		}
	}

	got, err := jobs.Recent(ctx, 2)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Recent(2) returned %d jobs", len(got))
	}
	if got[0].Job.FileName != "E03.mkv" || got[1].Job.FileName != "E02.mkv" {
		t.Errorf("order = %s, %s; want E03 then E02", got[0].Job.FileName, got[1].Job.FileName)
	}
	if b := got[0].Batch; b.ID != "b-r" || b.Name != "Show" || b.Relation != domain.RelationEpisodes || !b.CreatedAt.Equal(created) {
		t.Errorf("batch = %+v", b)
	}
}

func TestJobQueriesByStatusAgeAndExistence(t *testing.T) {
	t.Parallel()

	conn := open(t, t.TempDir())
	batches := sqlite.NewBatchRepository(conn)
	jobs := sqlite.NewJobRepository(conn)
	ctx := t.Context()
	if err := batches.Create(ctx, &domain.Batch{ID: "b-q", Relation: domain.RelationNone, CreatedAt: created}); err != nil {
		t.Fatal(err)
	}
	old, err := domain.NewUploadJob("job-old", domain.UploadMeta{FileName: "old.mkv", BatchID: "b-q"}, 10, created)
	if err != nil {
		t.Fatal(err)
	}
	if err := jobs.Create(ctx, old); err != nil {
		t.Fatal(err)
	}
	fresh, err := domain.NewUploadJob("job-fresh", domain.UploadMeta{FileName: "fresh.mkv", BatchID: "b-q"}, 10, created.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := jobs.Create(ctx, fresh); err != nil {
		t.Fatal(err)
	}

	uploading, err := jobs.ByStatus(ctx, domain.StatusUploading)
	if err != nil || len(uploading) != 2 || uploading[0].ID != "job-old" {
		t.Fatalf("ByStatus(uploading) = %d jobs, err %v; want both, oldest first", len(uploading), err)
	}
	stale, err := jobs.Stale(ctx, domain.StatusUploading, created.Add(time.Hour))
	if err != nil || len(stale) != 1 || stale[0].ID != "job-old" {
		t.Fatalf("Stale = %d jobs, err %v; want only job-old", len(stale), err)
	}
	for id, want := range map[domain.JobID]bool{"job-old": true, "job-missing": false} {
		ok, err := jobs.Exists(ctx, id)
		if err != nil || ok != want {
			t.Errorf("Exists(%s) = %v, %v; want %v", id, ok, err, want)
		}
	}
}
