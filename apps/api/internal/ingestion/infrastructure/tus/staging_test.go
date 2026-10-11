package tus_test

import (
	"net/http"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// eventually polls cond until it holds, and fails the test after guard.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()

	deadline := time.NewTimer(guard)
	defer deadline.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		if cond() {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("%s did not happen", what)
		case <-tick.C:
		}
	}
}

func TestProgressOfAnUploadReachesTheIngestionContext(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 50<<30)
	created := f.create(t, 10, metadata("filename", "a.mkv", "batchId", testBatch))
	location := created.Header.Get("Location")

	if res := f.patch(t, location, 0, "0123"); res.StatusCode != http.StatusNoContent {
		t.Fatalf("first chunk = %d, want 204", res.StatusCode)
	}
	eventually(t, "progress of 4 bytes reported", func() bool { return slices.Contains(f.uploads.received(), 4) })

	if res := f.patch(t, location, 4, "456789"); res.StatusCode != http.StatusNoContent {
		t.Fatalf("second chunk = %d, want 204", res.StatusCode)
	}
	eventually(t, "progress of 10 bytes reported", func() bool { return slices.Contains(f.uploads.received(), 10) })
}

func TestInspectReportsTheBytesStaged(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 50<<30)
	ctx := t.Context()
	location := f.create(t, 10, metadata("filename", "a.mkv", "batchId", testBatch)).Header.Get("Location")
	id := domain.JobID(idOf(location))

	steps := []struct {
		name   string
		chunk  string
		offset int64
		want   application.StagedUpload
	}{
		{name: "created, nothing received", want: application.StagedUpload{Exists: true, Size: 10, Offset: 0}},
		{name: "four bytes received", chunk: "0123", offset: 0, want: application.StagedUpload{Exists: true, Size: 10, Offset: 4}},
		{name: "all bytes received", chunk: "456789", offset: 4, want: application.StagedUpload{Exists: true, Size: 10, Offset: 10}},
	}
	for _, s := range steps {
		t.Run(s.name, func(t *testing.T) {
			if s.chunk != "" {
				if res := f.patch(t, location, s.offset, s.chunk); res.StatusCode != http.StatusNoContent {
					t.Fatalf("chunk = %d, want 204", res.StatusCode)
				}
			}
			got, err := f.adapter.Inspect(ctx, id)
			if err != nil {
				t.Fatalf("Inspect: %v", err)
			}
			if got != s.want {
				t.Errorf("Inspect = %+v, want %+v", got, s.want)
			}
		})
	}

	unknown, err := f.adapter.Inspect(ctx, "no-such-upload")
	if err != nil || unknown.Exists {
		t.Errorf("Inspect of an unknown upload = %+v, %v; want nothing staged", unknown, err)
	}
}

func TestRemoveDeletesEverythingStagedForAnUpload(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 50<<30)
	location := f.create(t, 10, metadata("filename", "a.mkv", "batchId", testBatch)).Header.Get("Location")
	id := domain.JobID(idOf(location))
	if res := f.patch(t, location, 0, "0123"); res.StatusCode != http.StatusNoContent {
		t.Fatalf("chunk = %d, want 204", res.StatusCode)
	}

	if err := f.adapter.Remove(t.Context(), id); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if n := stagedUploads(t, f.dir); n != 0 {
		t.Errorf("%d staged upload(s) after Remove, want 0", n)
	}
	if entries, err := os.ReadDir(f.dir); err != nil || len(entries) != 0 {
		t.Errorf("staging holds %d file(s) after Remove (err %v), want none", len(entries), err)
	}
	if got, _ := f.adapter.Inspect(t.Context(), id); got.Exists {
		t.Error("Inspect still finds the removed upload")
	}
	if err := f.adapter.Remove(t.Context(), id); err != nil {
		t.Errorf("second Remove: %v, want nil", err)
	}
}

func TestRemoveRefusesAnythingThatIsNotAnUploadId(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 50<<30)
	for _, id := range []domain.JobID{"", ".", "..", "a/b", `a\b`} {
		if err := f.adapter.Remove(t.Context(), id); err == nil {
			t.Errorf("Remove(%q) succeeded, want an error", id)
		}
	}
}

func TestIDsListsEachStagedUploadOnce(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 50<<30)
	first := f.create(t, 10, metadata("filename", "a.mkv", "batchId", testBatch)).Header.Get("Location")
	second := f.create(t, 10, metadata("filename", "b.mkv", "batchId", testBatch)).Header.Get("Location")
	if res := f.patch(t, first, 0, "0123"); res.StatusCode != http.StatusNoContent {
		t.Fatalf("chunk = %d, want 204", res.StatusCode)
	}

	ids, err := f.adapter.IDs(t.Context())
	if err != nil {
		t.Fatalf("IDs: %v", err)
	}
	want := []domain.JobID{domain.JobID(idOf(first)), domain.JobID(idOf(second))}
	slices.Sort(ids)
	slices.Sort(want)
	if !slices.Equal(ids, want) {
		t.Errorf("IDs = %v, want %v (each upload once, though it has several files)", ids, want)
	}
}

func TestRemovingAPartialUploadReleasesItsReservation(t *testing.T) {
	t.Parallel()

	// Room for two uploads of 1000 bytes only once the first one is no longer in progress (RF-09a).
	const size = 1000
	f := newFixture(t, domain.FreeSpaceMargin+1500)
	first := f.create(t, size, metadata("filename", "first.mkv", "batchId", testBatch)).Header.Get("Location")
	if res := f.create(t, size, metadata("filename", "second.mkv", "batchId", testBatch)); res.StatusCode != http.StatusInsufficientStorage {
		t.Fatalf("second create while the first reserves its bytes = %d, want 507", res.StatusCode)
	}

	if err := f.adapter.Remove(t.Context(), domain.JobID(idOf(first))); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if res := f.create(t, size, metadata("filename", "second.mkv", "batchId", testBatch)); res.StatusCode != http.StatusCreated {
		t.Fatalf("second create after the purge removed the first = %d, want 201", res.StatusCode)
	}
}
