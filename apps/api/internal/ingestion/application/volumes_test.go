package application_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	ingestionsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/workspace"
)

// virtualClock is a clock the test moves forward by hand.
type virtualClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *virtualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *virtualClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// splitSet packs one fictitious video with 7zz into volumes of 256 KiB (RF-03):
// the names of the volumes in order, their contents, and the video itself. The
// archive is stored, not compressed, so the volumes are big enough to be several.
func splitSet(t *testing.T, password string) (names []string, data [][]byte, video []byte) {
	t.Helper()
	need7zz(t)

	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "Movie"), 0o750); err != nil {
		t.Fatal(err)
	}
	video = make([]byte, 1<<20)
	if _, err := rand.Read(video); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "Movie", "Movie.mkv"), video, 0o600); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	args := []string{"a", "-bso0", "-bsp0", "-t7z", "-mx0", "-v256k"}
	if password != "" {
		args = append(args, "-p"+password, "-mhe=on")
	}
	args = append(args, filepath.Join(out, "movie.7z"), "Movie")
	cmd := exec.CommandContext(t.Context(), "7zz", args...)
	cmd.Dir = src
	if msg, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("7zz a: %v\n%s", err, msg)
	}
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	if len(names) < 2 {
		t.Fatalf("7zz made %d volumes, want several: %v", len(names), names)
	}
	for _, n := range names {
		b, err := os.ReadFile(filepath.Join(out, n))
		if err != nil {
			t.Fatal(err)
		}
		data = append(data, b)
	}
	return names, data, video
}

// restart starts a new service over the rig's database and staging area, as a
// process does after a restart: its workers run, and what was in progress resumes.
func (r *rig) restart(t *testing.T, extractor application.Extractor) {
	t.Helper()

	svc := application.NewService(ingestionsqlite.NewBatchRepository(r.conn), ingestionsqlite.NewJobRepository(r.conn), r.pub, nil, time.Now).
		WithStaging(diskStaging{dir: r.staging}).
		WithExtraction(workspace.New(r.staging, func(string) (uint64, error) { return 1 << 40, nil }), extractor, r.trash)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	go svc.RunExtraction(ctx, 1)
	r.svc = svc
}

// blockingExtractor holds every listing until its context ends, so that a test
// can act on an extraction that is in progress.
type blockingExtractor struct {
	application.Extractor
	started chan struct{}
}

func (b blockingExtractor) List(ctx context.Context, _, _ string) (application.Listing, error) {
	select {
	case b.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return application.Listing{}, ctx.Err()
}

// corruptingExtractor lists and extracts like 7-Zip, but its extraction fails
// with ErrNoSpace, as a disk that fills up while the files are written does.
type corruptingExtractor struct {
	application.Extractor
}

func (c corruptingExtractor) Extract(context.Context, string, string, string, application.Listing, func(int)) (string, error) {
	return "", fmt.Errorf("%w: System ERROR: errno=28 : No space left on device", application.ErrNoSpace)
}

// volumeJobs uploads the volumes in the order given and returns their job ids
// in volume order: ids[0] is the first volume.
func (r *rig) volumeJobs(t *testing.T, names []string, data [][]byte, order []int, total int) []domain.JobID {
	t.Helper()

	ids := make([]domain.JobID, len(names))
	for _, i := range order {
		ids[i] = r.uploadPart(t, names[i], data[i], total)
	}
	return ids
}

// statusOf returns the status of a job.
func (r *rig) statusOf(t *testing.T, id domain.JobID) domain.UploadJob {
	t.Helper()
	job, err := r.svc.Job(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return *job
}

func TestASetIsExtractedFromItsFirstVolumeOnceItIsComplete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		order func(n int) []int // the order the volumes arrive in
	}{
		{name: "in order", order: func(n int) []int {
			o := make([]int, n)
			for i := range o {
				o[i] = i
			}
			return o
		}},
		{name: "last volume first", order: func(n int) []int {
			o := make([]int, n)
			for i := range o {
				o[i] = n - 1 - i
			}
			return o
		}},
		{name: "first volume last", order: func(n int) []int {
			o := []int{}
			for i := 1; i < n; i++ {
				o = append(o, i)
			}
			return append(o, 0)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			names, data, video := splitSet(t, "")
			r := newRig(t, realExtractor(), 1<<40, true, nil)
			order := tt.order(len(names))
			ids := map[int]domain.JobID{}
			for k, i := range order {
				ids[i+1] = r.uploadPart(t, names[i], data[i], len(names))
				if k < len(order)-1 {
					// Until the last volume arrives, every volume waits (RF-10a).
					if got := r.statusOf(t, ids[i+1]); got.Status != domain.StatusWaitingParts {
						t.Fatalf("volume %d is %s before the set is complete, want waiting_parts", i+1, got.Status)
					}
				}
			}

			for idx, id := range ids {
				r.waitFor(t, id, fmt.Sprintf("volume %d filtering", idx), func(j domain.UploadJob) bool {
					return j.Status == domain.StatusFiltering
				})
			}
			got, err := os.ReadFile(filepath.Join(r.extractionDir(ids[1]), "Movie", "Movie.mkv"))
			if err != nil || !bytes.Equal(got, video) {
				t.Fatalf("extracted video differs from the original: %v", err)
			}
			puts := r.trash.puts()
			if len(puts) != len(names) {
				t.Fatalf("trash entries = %d, want one per volume (%d)", len(puts), len(names))
			}
			for _, p := range puts {
				if p.Reason != application.TrashReasonExtracted {
					t.Errorf("trashed %s for %s, want extracted", p.Name, p.Reason)
				}
			}
			if _, err := os.Stat(filepath.Join(r.staging, "volumes", string(ids[1]))); !os.IsNotExist(err) {
				t.Errorf("the set folder is still in staging: %v", err)
			}
		})
	}
}

func TestASetThatDoesNotFitFailsAsPartsMismatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		files  []string // the names of the volumes, in arrival order
		totals []int    // the total each one declares
	}{
		{name: "volumes declare different totals", files: []string{"Movie.7z.001", "Movie.7z.002", "Movie.7z.003"}, totals: []int{3, 3, 2}},
		{name: "a position is repeated", files: []string{"Movie.7z.001", "Movie.7z.002", "Movie.7z.002"}, totals: []int{3, 3, 3}},
		{name: "a position is missing", files: []string{"Movie.7z.001", "Movie.7z.002", "Movie.7z.004"}, totals: []int{3, 3, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := newRig(t, realExtractor(), 1<<40, true, nil)
			var ids []domain.JobID
			for i, name := range tt.files {
				ids = append(ids, r.uploadPart(t, name, []byte("not really a volume"), tt.totals[i]))
			}
			for _, id := range ids {
				job := r.waitFor(t, id, "parts mismatch", func(j domain.UploadJob) bool { return j.Status == domain.StatusFailed })
				if job.Error != domain.FailurePartsMismatch {
					t.Errorf("%s: error = %q, want parts_mismatch", job.FileName, job.Error)
				}
				if r.staged(id) {
					t.Errorf("%s: its staged file was kept after parts_mismatch", job.FileName)
				}
			}
			if n := len(r.trash.puts()); n != 0 {
				t.Errorf("trash entries = %d, want none", n)
			}
		})
	}
}

func TestAVolumeWhoseSetNeverCompletesFailsAsPartsMissingInThePurge(t *testing.T) {
	t.Parallel()

	clk := &virtualClock{t: time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)}
	r := newRig(t, realExtractor(), 1<<40, true, clk)
	id := r.uploadPart(t, "Movie.7z.001", []byte("waiting for its sisters"), 3)
	if got := r.statusOf(t, id); got.Status != domain.StatusWaitingParts {
		t.Fatalf("status = %s, want waiting_parts", got.Status)
	}

	clk.Advance(application.AbandonedAfter - time.Hour)
	if res, err := r.svc.Purge(t.Context()); err != nil || res.PartsMissing != 0 {
		t.Fatalf("purge before 24 hours: %+v, %v; want no parts missing yet", res, err)
	}

	clk.Advance(2 * time.Hour)
	res, err := r.svc.Purge(t.Context())
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if res.PartsMissing != 1 {
		t.Fatalf("purge result %+v, want one volume with parts missing", res)
	}
	job := r.statusOf(t, id)
	if job.Status != domain.StatusFailed || job.Error != domain.FailurePartsMissing {
		t.Errorf("job = %s (%q), want failed parts_missing", job.Status, job.Error)
	}
	if r.staged(id) {
		t.Error("the file of the volume is still in staging")
	}
}

func TestDeletingAWaitingVolumeCancelsOnlyThatVolume(t *testing.T) {
	t.Parallel()

	r := newRig(t, realExtractor(), 1<<40, false, nil)
	deleted := r.uploadPart(t, "Movie.7z.001", []byte("x"), 3)
	if err := r.svc.Terminated(t.Context(), deleted); err != nil {
		t.Fatalf("Terminated: %v", err)
	}
	if got := r.statusOf(t, deleted); got.Status != domain.StatusCancelled {
		t.Errorf("deleted volume is %s, want cancelled", got.Status)
	}
	// The set is still incomplete when the others arrive: they wait, they are not cancelled.
	kept := r.uploadPart(t, "Movie.7z.002", []byte("y"), 3)
	if got := r.statusOf(t, kept); got.Status != domain.StatusWaitingParts {
		t.Errorf("the other volume is %s, want it to keep waiting", got.Status)
	}
}

func TestDeletingAClaimedSetCancelsAllItsVolumes(t *testing.T) {
	t.Parallel()

	names, data, _ := splitSet(t, "")
	blocker := blockingExtractor{Extractor: realExtractor(), started: make(chan struct{}, 1)}
	r := newRig(t, blocker, 1<<40, true, nil)
	ids := make([]domain.JobID, len(names))
	for i := range names {
		ids[i] = r.uploadPart(t, names[i], data[i], len(names))
	}
	select {
	case <-blocker.started:
	case <-time.After(guard):
		t.Fatal("the set was never extracted")
	}

	if err := r.svc.Terminated(t.Context(), ids[len(ids)-1]); err != nil {
		t.Fatalf("Terminated: %v", err)
	}
	for i, id := range ids {
		if got := r.statusOf(t, id); got.Status != domain.StatusCancelled {
			t.Errorf("volume %d is %s, want cancelled with its set", i+1, got.Status)
		}
	}
	if _, err := os.Stat(filepath.Join(r.staging, "volumes", string(ids[0]))); !os.IsNotExist(err) {
		t.Errorf("the set folder is still there: %v", err)
	}
}

func TestADamagedSetFailsAsArchiveCorruptAndRetryTriesTheSetAgain(t *testing.T) {
	t.Parallel()

	names, data, _ := splitSet(t, "")
	// Damage the stored data of the second volume: 7-Zip finds it when it extracts.
	data[1] = append([]byte(nil), data[1]...)
	data[1][len(data[1])/2] ^= 0xFF
	r := newRig(t, realExtractor(), 1<<40, true, nil)
	ids := r.volumeJobs(t, names, data, indexes(len(names)), len(names))

	for i, id := range ids {
		job := r.waitFor(t, id, fmt.Sprintf("volume %d fails with its set", i+1), func(j domain.UploadJob) bool {
			return j.Status == domain.StatusFailed
		})
		if job.Error != domain.FailureArchiveCorrupt {
			t.Errorf("volume %d: error = %q, want archive_corrupt", i+1, job.Error)
		}
	}
	// Retry on any volume retries the set; its volumes are still there (RF-10c).
	if _, err := r.svc.Retry(t.Context(), ids[len(ids)-1]); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	r.waitFor(t, ids[1], "the retried set fails again", func(j domain.UploadJob) bool {
		return j.Status == domain.StatusFailed && j.Error == domain.FailureArchiveCorrupt
	})
	if _, err := os.Stat(filepath.Join(r.staging, "volumes", string(ids[0]), names[0])); err != nil {
		t.Errorf("the first volume is gone after a failed retry: %v", err)
	}
}

// indexes returns 0..n-1, the arrival order of volumes in index order.
func indexes(n int) []int {
	o := make([]int, n)
	for i := range o {
		o[i] = i
	}
	return o
}

func TestAnEncryptedSetAsksOnceAndExtractsWithThePassword(t *testing.T) {
	t.Parallel()

	names, data, video := splitSet(t, "S3cret")
	r := newRig(t, realExtractor(), 1<<40, true, nil)
	ids := r.volumeJobs(t, names, data, indexes(len(names)), len(names))

	for i, id := range ids {
		r.waitFor(t, id, fmt.Sprintf("volume %d asks for the password", i), func(j domain.UploadJob) bool {
			return j.Status == domain.StatusNeedsPassword && j.Error == ""
		})
	}
	if _, err := r.svc.SubmitPassword(t.Context(), ids[len(ids)-1], "nope"); err != nil {
		t.Fatalf("SubmitPassword: %v", err)
	}
	for i, id := range ids {
		r.waitFor(t, id, fmt.Sprintf("volume %d reports the wrong password", i+1), func(j domain.UploadJob) bool {
			return j.Status == domain.StatusNeedsPassword && j.Error == domain.FailureWrongPassword
		})
	}
	if _, err := r.svc.SubmitPassword(t.Context(), ids[2], "S3cret"); err != nil {
		t.Fatalf("SubmitPassword: %v", err)
	}
	for _, id := range ids {
		r.waitFor(t, id, "extracted with the password", func(j domain.UploadJob) bool { return j.Status == domain.StatusFiltering })
	}
	got, err := os.ReadFile(filepath.Join(r.extractionDir(ids[0]), "Movie", "Movie.mkv"))
	if err != nil || !bytes.Equal(got, video) {
		t.Fatalf("extracted video differs from the original: %v", err)
	}
}

func TestAClaimedSetIsExtractedAgainAfterARestart(t *testing.T) {
	t.Parallel()

	names, data, video := splitSet(t, "")
	r := newRig(t, realExtractor(), 1<<40, false, nil) // no workers: the set is claimed and left in extracting
	ids := r.volumeJobs(t, names, data, indexes(len(names)), len(names))
	for i, id := range ids {
		if got := r.statusOf(t, id); got.Status != domain.StatusExtracting {
			t.Fatalf("volume %d is %s before the restart, want extracting", i+1, got.Status)
		}
	}

	r.restart(t, realExtractor())
	for _, id := range ids {
		r.waitFor(t, id, "extracted after the restart", func(j domain.UploadJob) bool { return j.Status == domain.StatusFiltering })
	}
	got, err := os.ReadFile(filepath.Join(r.extractionDir(ids[0]), "Movie", "Movie.mkv"))
	if err != nil || !bytes.Equal(got, video) {
		t.Fatalf("extracted video differs from the original: %v", err)
	}
}

func TestLegacyRARVolumesFailAsLegacyVolumeWithoutRetry(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"Movie.r00", "movie.R01"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			r := newRig(t, realExtractor(), 1<<40, true, nil)
			id := r.upload(t, name, []byte("Rar!\x1A\x07\x00 old volume"))
			job := r.waitFor(t, id, "refused", func(j domain.UploadJob) bool { return j.Status == domain.StatusFailed })
			if job.Error != domain.FailureLegacyVolume {
				t.Fatalf("error = %q, want legacy_volume", job.Error)
			}
			if r.staged(id) {
				t.Error("the old volume was kept after legacy_volume")
			}
			if _, err := r.svc.Retry(t.Context(), id); !errors.Is(err, application.ErrCannotRetry) {
				t.Fatalf("Retry: err = %v, want ErrCannotRetry", err)
			}
		})
	}
}

func TestNoSpaceDuringTheExtractionFailsAndCanBeRetried(t *testing.T) {
	t.Parallel()

	r := newRig(t, corruptingExtractor{realExtractor()}, 1<<40, true, nil)
	id := r.upload(t, "Movie.zip", videoZip(t))
	job := r.waitFor(t, id, "no space", func(j domain.UploadJob) bool { return j.Status == domain.StatusFailed })
	if job.Error != domain.FailureNoSpace {
		t.Fatalf("error = %q, want no_space (the disk filled while extracting)", job.Error)
	}
	if !r.staged(id) {
		t.Fatal("the archive must stay for a retry (RF-10c)")
	}
}
