package sqlite_test

import (
	"errors"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
)

func TestFilterEntriesSurviveARestartAndAreNotDuplicated(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	ctx := t.Context()

	conn := open(t, dir)
	if err := sqlite.NewBatchRepository(conn).Create(ctx, &domain.Batch{ID: "b-1", Relation: domain.RelationNone, CreatedAt: created}); err != nil {
		t.Fatal(err)
	}
	job, err := domain.NewUploadJob("unit-1", domain.UploadMeta{FileName: "Movie.zip", BatchID: "b-1"}, 4096, created)
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlite.NewJobRepository(conn).Create(ctx, job); err != nil {
		t.Fatal(err)
	}
	filter := sqlite.NewFilterRepository(conn)

	if got, err := filter.Entries(ctx, "unit-1"); err != nil || len(got) != 0 {
		t.Fatalf("entries before Record = %+v, %v; want none", got, err)
	}
	first := []domain.FilterEntry{
		{Path: "Movie/Movie.mkv", Size: 4000, Kind: domain.KindVideo},
		{Path: "Movie/sample.mkv", Size: 1000, Kind: domain.KindDiscard},
		{Path: "Movie/Movie.nfo", Size: 10, Kind: domain.KindDiscard},
	}
	if err := filter.Record(ctx, "unit-1", first); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := filter.MarkTrashed(ctx, "unit-1", "Movie/sample.mkv"); err != nil {
		t.Fatalf("MarkTrashed: %v", err)
	}
	// Recording again, as a resumed filter does, changes nothing: no duplicates and the trashed flag stays.
	if err := filter.Record(ctx, "unit-1", first); err != nil {
		t.Fatalf("second Record: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	conn = open(t, dir)
	got, err := sqlite.NewFilterRepository(conn).Entries(ctx, "unit-1")
	if err != nil {
		t.Fatalf("Entries after restart: %v", err)
	}
	want := []domain.FilterEntry{
		{Path: "Movie/Movie.mkv", Size: 4000, Kind: domain.KindVideo},
		{Path: "Movie/Movie.nfo", Size: 10, Kind: domain.KindDiscard},
		{Path: "Movie/sample.mkv", Size: 1000, Kind: domain.KindDiscard, Trashed: true},
	}
	if len(got) != len(want) {
		t.Fatalf("entries = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestFilterEntriesNeedAKnownUnit(t *testing.T) {
	t.Parallel()

	conn := open(t, t.TempDir())
	filter := sqlite.NewFilterRepository(conn)
	err := filter.Record(t.Context(), "no-such-job", []domain.FilterEntry{{Path: "a.mkv", Kind: domain.KindVideo}})
	if err == nil {
		t.Fatal("entries of an unknown job were stored")
	}
	if errors.Is(err, domain.ErrJobNotFound) {
		t.Fatal("the foreign key must refuse, not the lookup")
	}
}

func TestFilteredMarkSurvivesTheJobRepository(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	ctx := t.Context()

	conn := open(t, dir)
	if err := sqlite.NewBatchRepository(conn).Create(ctx, &domain.Batch{ID: "b-1", Relation: domain.RelationNone, CreatedAt: created}); err != nil {
		t.Fatal(err)
	}
	jobs := sqlite.NewJobRepository(conn)
	job, err := domain.NewUploadJob("job-1", domain.UploadMeta{FileName: "Movie.mkv", BatchID: "b-1"}, 4096, created)
	if err != nil {
		t.Fatal(err)
	}
	if err := jobs.Create(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := job.MarkUploaded(created); err != nil {
		t.Fatal(err)
	}
	if err := job.StartFiltering(created); err != nil {
		t.Fatal(err)
	}
	if err := job.MarkFiltered(created); err != nil {
		t.Fatal(err)
	}
	if err := jobs.Save(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	conn = open(t, dir)
	got, err := sqlite.NewJobRepository(conn).Get(ctx, "job-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusFiltering || !got.FilterDone {
		t.Errorf("job after restart = %s, filter done %v; want filtering, done", got.Status, got.FilterDone)
	}
}
