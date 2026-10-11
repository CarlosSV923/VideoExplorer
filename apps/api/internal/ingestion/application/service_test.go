package application_test

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/database"
)

var start = time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)

// service builds the ingestion service over a real database in dir, with a
// clock the test moves forward. Its job changes go to a discarded publisher.
func service(t *testing.T, dir string, clock *time.Time) (*application.Service, *sql.DB) {
	t.Helper()

	svc, conn := serviceWith(t, dir, clock, &recorder{})
	return svc, conn
}

// serviceWith is service with the publisher of the test, to read the events back.
func serviceWith(t *testing.T, dir string, clock *time.Time, pub application.Publisher) (*application.Service, *sql.DB) {
	t.Helper()

	conn, err := database.Open(t.Context(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	svc := application.NewService(sqlite.NewBatchRepository(conn), sqlite.NewJobRepository(conn), pub, nil, func() time.Time { return *clock })
	return svc, conn
}

func TestCreateBatch(t *testing.T) {
	t.Parallel()

	clock := start
	svc, _ := service(t, t.TempDir(), &clock)
	ctx := t.Context()

	tests := []struct {
		name     string
		batch    string
		relation domain.Relation
		wantErr  bool
	}{
		{name: "single file, no question", relation: domain.RelationNone},
		{name: "parts of a film with its folder name", batch: "Iron Man", relation: domain.RelationParts},
		{name: "chapters of a series", batch: "Breaking Bad", relation: domain.RelationEpisodes},
		{name: "unknown relation", relation: "multipart", wantErr: true},
		{name: "folder name with a separator", batch: "a/b", relation: domain.RelationParts, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			batch, err := svc.CreateBatch(ctx, tt.batch, tt.relation)
			if tt.wantErr {
				if !errors.Is(err, domain.ErrInvalidBatch) {
					t.Fatalf("err = %v, want ErrInvalidBatch", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateBatch: %v", err)
			}
			if batch.ID == "" || batch.Relation != tt.relation || batch.Name != tt.batch || !batch.CreatedAt.Equal(start) {
				t.Errorf("batch = %+v", *batch)
			}
		})
	}
}

func TestBatchIDsAreDifferentAndURLSafe(t *testing.T) {
	t.Parallel()

	clock := start
	svc, _ := service(t, t.TempDir(), &clock)

	seen := map[domain.BatchID]bool{}
	for range 20 {
		batch, err := svc.CreateBatch(t.Context(), "", domain.RelationNone)
		if err != nil {
			t.Fatal(err)
		}
		if seen[batch.ID] {
			t.Fatalf("batch id %q repeated", batch.ID)
		}
		seen[batch.ID] = true
		if _, err := domain.NewBatch(batch.ID, "", domain.RelationNone, start); err != nil {
			t.Errorf("batch id %q is not a valid batch id: %v", batch.ID, err)
		}
	}
}

func TestAdmitLinksTheUploadToItsBatch(t *testing.T) {
	t.Parallel()

	clock := start
	svc, _ := service(t, t.TempDir(), &clock)
	ctx := t.Context()

	batch, err := svc.CreateBatch(ctx, "Show", domain.RelationEpisodes)
	if err != nil {
		t.Fatal(err)
	}
	id, err := svc.Admit(ctx, application.Admission{
		Meta: domain.UploadMeta{FileName: "E01.mkv", RelativePath: "Show/E01.mkv", BatchID: string(batch.ID)},
		Size: 10,
	})
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}
	job, err := svc.Job(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if job.BatchID != batch.ID || job.Status != domain.StatusUploading || job.Size != 10 || job.RelativePath != "Show/E01.mkv" {
		t.Errorf("job = %+v, want uploading in the batch", *job)
	}
}

func TestAdmitRefusesAnUnknownBatch(t *testing.T) {
	t.Parallel()

	clock := start
	svc, _ := service(t, t.TempDir(), &clock)

	_, err := svc.Admit(t.Context(), application.Admission{
		Meta: domain.UploadMeta{FileName: "a.mkv", BatchID: "does-not-exist"},
		Size: 10,
	})
	if !errors.Is(err, domain.ErrBatchNotFound) {
		t.Fatalf("err = %v, want ErrBatchNotFound", err)
	}
}

func TestAdmitRefusesAnUploadWithoutBatch(t *testing.T) {
	t.Parallel()

	clock := start
	svc, _ := service(t, t.TempDir(), &clock)

	// Every upload belongs to a batch (RF-09a): an empty batch id names no batch.
	_, err := svc.Admit(t.Context(), application.Admission{Meta: domain.UploadMeta{FileName: "a.mkv"}, Size: 10})
	if !errors.Is(err, domain.ErrBatchNotFound) {
		t.Fatalf("err = %v, want ErrBatchNotFound", err)
	}
}

func TestAdmitRefusesAnEmptyFile(t *testing.T) {
	t.Parallel()

	clock := start
	svc, _ := service(t, t.TempDir(), &clock)
	batch, err := svc.CreateBatch(t.Context(), "", domain.RelationNone)
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.Admit(t.Context(), application.Admission{Meta: domain.UploadMeta{FileName: "a.mkv", BatchID: string(batch.ID)}, Size: 0})
	if !errors.Is(err, domain.ErrEmptyFile) {
		t.Fatalf("err = %v, want ErrEmptyFile", err)
	}
}

func TestUploadEndsAreRecordedAndSurviveARestart(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	clock := start
	svc, conn := service(t, dir, &clock)
	ctx := t.Context()
	batch, err := svc.CreateBatch(ctx, "", domain.RelationNone)
	if err != nil {
		t.Fatal(err)
	}

	uploaded, err := svc.Admit(ctx, application.Admission{Meta: domain.UploadMeta{FileName: "a.mkv", BatchID: string(batch.ID)}, Size: 10})
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := svc.Admit(ctx, application.Admission{Meta: domain.UploadMeta{FileName: "b.mkv", BatchID: string(batch.ID)}, Size: 10})
	if err != nil {
		t.Fatal(err)
	}

	clock = start.Add(time.Minute)
	if err := svc.Completed(ctx, uploaded); err != nil {
		t.Fatalf("Completed: %v", err)
	}
	if err := svc.Terminated(ctx, deleted); err != nil {
		t.Fatalf("Terminated: %v", err)
	}
	// A job that already ended keeps its state when the browser reports it again.
	if err := svc.Terminated(ctx, deleted); err != nil {
		t.Fatalf("second Terminated: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	// After a restart the states are the same (RF-10a).
	svc, _ = service(t, dir, &clock)
	gotUploaded, err := svc.Job(ctx, uploaded)
	if err != nil {
		t.Fatal(err)
	}
	if gotUploaded.Status != domain.StatusUploaded {
		t.Errorf("finished upload = %s after restart, want uploaded", gotUploaded.Status)
	}
	gotDeleted, err := svc.Job(ctx, deleted)
	if err != nil {
		t.Fatal(err)
	}
	if gotDeleted.Status != domain.StatusCancelled {
		t.Errorf("deleted upload = %s after restart, want cancelled", gotDeleted.Status)
	}
}

func TestUploadEndsOfUnknownJobsAreReported(t *testing.T) {
	t.Parallel()

	clock := start
	svc, _ := service(t, t.TempDir(), &clock)

	if err := svc.Completed(t.Context(), "nope"); !errors.Is(err, domain.ErrJobNotFound) {
		t.Errorf("Completed of an unknown job: err = %v, want ErrJobNotFound", err)
	}
	if err := svc.Terminated(t.Context(), "nope"); !errors.Is(err, domain.ErrJobNotFound) {
		t.Errorf("Terminated of an unknown job: err = %v, want ErrJobNotFound", err)
	}
}
