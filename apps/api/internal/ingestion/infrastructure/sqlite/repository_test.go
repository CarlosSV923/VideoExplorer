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

func TestUploadJobWithoutBatchAndFailureReason(t *testing.T) {
	t.Parallel()

	conn := open(t, t.TempDir())
	jobs := sqlite.NewJobRepository(conn)
	ctx := t.Context()

	job, err := domain.NewUploadJob("job-2", domain.UploadMeta{FileName: "a.mkv"}, 10, created)
	if err != nil {
		t.Fatal(err)
	}
	if err := jobs.Create(ctx, job); err != nil {
		t.Fatalf("Create without batch: %v", err)
	}
	if err := job.Fail("disk full", created); err != nil {
		t.Fatal(err)
	}
	if err := jobs.Save(ctx, job); err != nil {
		t.Fatal(err)
	}

	got, err := jobs.Get(ctx, "job-2")
	if err != nil {
		t.Fatal(err)
	}
	if got.BatchID != "" {
		t.Errorf("batch = %q, want none", got.BatchID)
	}
	if got.Status != domain.StatusFailed || got.Error != "disk full" {
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
