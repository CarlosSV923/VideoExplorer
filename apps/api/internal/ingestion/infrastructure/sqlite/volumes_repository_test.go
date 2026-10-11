package sqlite_test

import (
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
)

func TestVolumeFieldsAndTheirClaimSurviveTheDatabase(t *testing.T) {
	t.Parallel()

	conn := open(t, t.TempDir())
	ctx := t.Context()
	batches := sqlite.NewBatchRepository(conn)
	jobs := sqlite.NewJobRepository(conn)
	batch, err := domain.NewBatch("lote-vol", "", domain.RelationParts, created)
	if err != nil {
		t.Fatal(err)
	}
	if err := batches.Create(ctx, batch); err != nil {
		t.Fatal(err)
	}

	meta, err := domain.NewUploadMeta("Movie.7z.002", "", "lote-vol", "3")
	if err != nil {
		t.Fatal(err)
	}
	vol, err := domain.NewUploadJob("v2", meta, 10, created)
	if err != nil {
		t.Fatal(err)
	}
	if err := jobs.Create(ctx, vol); err != nil {
		t.Fatal(err)
	}
	got, err := jobs.Get(ctx, "v2")
	if err != nil {
		t.Fatal(err)
	}
	if got.PartsTotal != 3 || got.VolumeIndex != 2 || got.VolumeSet != vol.VolumeSet || got.VolumeGroup != "" {
		t.Fatalf("stored volume = %+v, want total 3, index 2, the set, no group yet", got)
	}

	if err := got.MarkUploaded(created); err != nil {
		t.Fatal(err)
	}
	if err := got.WaitForParts(created); err != nil {
		t.Fatal(err)
	}
	if err := got.ClaimSet("v1", created); err != nil {
		t.Fatal(err)
	}
	if err := jobs.Save(ctx, got); err != nil {
		t.Fatal(err)
	}
	claimed, err := jobs.ByVolumeGroup(ctx, "v1")
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 || claimed[0].ID != "v2" || claimed[0].VolumeGroup != "v1" || claimed[0].Status != domain.StatusExtracting {
		t.Fatalf("ByVolumeGroup = %+v, want the claimed volume", claimed)
	}
	other, err := jobs.ByVolumeGroup(ctx, "nobody")
	if err != nil || len(other) != 0 {
		t.Fatalf("ByVolumeGroup of an unknown group = %+v, %v", other, err)
	}
}

func TestByVolumeGroupReturnsTheVolumesInVolumeOrder(t *testing.T) {
	t.Parallel()

	conn := open(t, t.TempDir())
	ctx := t.Context()
	batch, err := domain.NewBatch("lote-ord", "", domain.RelationParts, created)
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlite.NewBatchRepository(conn).Create(ctx, batch); err != nil {
		t.Fatal(err)
	}
	jobs := sqlite.NewJobRepository(conn)
	for _, file := range []struct{ id, name string }{{"c", "Movie.zip.003"}, {"a", "Movie.zip.001"}, {"b", "Movie.zip.002"}} {
		meta, err := domain.NewUploadMeta(file.name, "", "lote-ord", "3")
		if err != nil {
			t.Fatal(err)
		}
		j, err := domain.NewUploadJob(domain.JobID(file.id), meta, 10, created)
		if err != nil {
			t.Fatal(err)
		}
		if err := jobs.Create(ctx, j); err != nil {
			t.Fatal(err)
		}
		if err := j.MarkUploaded(created); err != nil {
			t.Fatal(err)
		}
		if err := j.WaitForParts(created); err != nil {
			t.Fatal(err)
		}
		if err := j.ClaimSet("a", created); err != nil {
			t.Fatal(err)
		}
		if err := jobs.Save(ctx, j); err != nil {
			t.Fatal(err)
		}
	}
	set, err := jobs.ByVolumeGroup(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	var order []domain.JobID
	for _, j := range set {
		order = append(order, j.ID)
	}
	if len(order) != 3 || order[0] != "a" || order[1] != "b" || order[2] != "c" {
		t.Fatalf("order = %v, want a, b, c (by volume index)", order)
	}
}
