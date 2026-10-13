package application_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sevenzip"
	ingestionsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/workspace"
)

// sambaEntry is the id of the one entry of the fake section.
const sambaEntry int64 = 7

// fakeSamba is the section of No asignados for the tests of the adoption: its files are real files
// on disk, and Take moves them with a rename, as the catalog does.
type fakeSamba struct {
	mu    sync.Mutex
	files []domain.SambaFile
	// takeErr makes Take fail before it moves anything.
	takeErr error
	// moveThenErr makes Take move the files and then fail, as a change whose answer got lost would.
	moveThenErr error
}

func (f *fakeSamba) Files(context.Context) ([]domain.SambaFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.files), nil
}

func (f *fakeSamba) Take(_ context.Context, entry int64, moves []application.SambaMove) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.takeErr != nil {
		return f.takeErr
	}
	for _, m := range moves {
		i := slices.IndexFunc(f.files, func(x domain.SambaFile) bool { return x.Entry == entry && x.Rel == m.Rel })
		if i < 0 {
			return application.ErrSambaChanged
		}
		if err := os.Rename(f.files[i].Source, m.Dst); err != nil {
			return err
		}
		f.files = slices.Delete(f.files, i, i+1)
	}
	return f.moveThenErr
}

// copy puts a file that Samba copied on disk and lists it in the section, as still being copied or
// settled (copying).
func (f *fakeSamba) copy(t *testing.T, dir string, name, rel string, data []byte, copying bool) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files = append(f.files, domain.SambaFile{
		Entry: sambaEntry, Rel: rel, Name: filepath.Base(filepath.FromSlash(name)), Size: int64(len(data)), Source: path, Copying: copying,
	})
}

// sambaRig is a rig whose extraction is wired with the fake section. Its workers do not run: a test
// reads the state a unit reaches, and runs the extraction itself when it needs to.
func sambaRig(t *testing.T) (*rig, *fakeSamba, string) {
	t.Helper()
	clk := &virtualClock{t: time.Date(2026, time.October, 13, 9, 0, 0, 0, time.UTC)}
	r := newRig(t, sevenzip.New(""), 1<<40, false, clk)
	f := &fakeSamba{}
	r.svc.WithSamba(f)
	return r, f, t.TempDir()
}

// sevenZipVolume is a volume whose first bytes are the magic of a 7z archive.
func sevenZipVolume(n int) []byte {
	return append([]byte("7z\xBC\xAF\x27\x1C"), bytes.Repeat([]byte{byte(n)}, 64)...)
}

func TestAZipCopiedBySambaIsTakenAsAnUpload(t *testing.T) {
	t.Parallel()

	r, f, lib := sambaRig(t)
	data := videoZip(t)
	f.copy(t, lib, "movie.zip", "", data, false)

	n, err := r.svc.AdoptSamba(t.Context())
	if err != nil || n != 1 {
		t.Fatalf("AdoptSamba = %d, %v; want 1 unit", n, err)
	}
	views, err := r.svc.SambaJobs(t.Context())
	if err != nil || len(views) != 1 {
		t.Fatalf("SambaJobs = %d jobs, %v; want 1", len(views), err)
	}
	v := views[0]
	if !v.Job.IsSamba() || v.Batch.Origin != domain.OriginSamba || v.Batch.Relation != domain.RelationNone {
		t.Fatalf("job %+v in batch %+v: want a samba job in a samba batch with no relation", v.Job, v.Batch)
	}
	if v.Job.Status != domain.StatusUploaded || v.Job.FileName != "movie.zip" {
		t.Fatalf("job is %s for %q, want uploaded for movie.zip", v.Job.Status, v.Job.FileName)
	}
	if !r.staged(v.Job.ID) {
		t.Fatal("the copy is not in staging under its job's id")
	}
	if _, err := os.Stat(filepath.Join(lib, "movie.zip")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the copy is still in the section: %v", err)
	}
}

func TestAReadingTwiceDoesNotTakeTheSameCopyTwice(t *testing.T) {
	t.Parallel()

	r, f, lib := sambaRig(t)
	f.copy(t, lib, "movie.zip", "", videoZip(t), false)

	for range 2 {
		if _, err := r.svc.AdoptSamba(t.Context()); err != nil {
			t.Fatalf("AdoptSamba: %v", err)
		}
	}
	views, err := r.svc.SambaJobs(t.Context())
	if err != nil || len(views) != 1 {
		t.Fatalf("SambaJobs = %d jobs, %v; want exactly one after two readings", len(views), err)
	}
	batches, err := ingestionsqlite.NewJobRepository(r.conn).ByBatch(t.Context(), views[0].Batch.ID)
	if err != nil || len(batches) != 1 {
		t.Fatalf("jobs of the batch = %d, %v; want 1", len(batches), err)
	}
}

func TestACopyStillBeingCopiedWaitsUntilItSettles(t *testing.T) {
	t.Parallel()

	r, f, lib := sambaRig(t)
	f.copy(t, lib, "movie.zip", "", videoZip(t), true)

	if n, err := r.svc.AdoptSamba(t.Context()); err != nil || n != 0 {
		t.Fatalf("AdoptSamba while copying = %d, %v; want nothing", n, err)
	}
	f.mu.Lock()
	f.files[0].Copying = false
	f.mu.Unlock()
	if n, err := r.svc.AdoptSamba(t.Context()); err != nil || n != 1 {
		t.Fatalf("AdoptSamba once settled = %d, %v; want 1", n, err)
	}
}

func TestFilesThatAreNotArchivesAreLeftInTheSection(t *testing.T) {
	t.Parallel()

	r, f, lib := sambaRig(t)
	f.copy(t, lib, "Pack/notes.txt", "Pack/notes.txt", []byte("not an archive"), false)
	f.copy(t, lib, "Pack/disguised.zip", "Pack/disguised.zip", []byte("plain text named as an archive"), false)

	if n, err := r.svc.AdoptSamba(t.Context()); err != nil || n != 0 {
		t.Fatalf("AdoptSamba = %d, %v; want no unit", n, err)
	}
	if views, _ := r.svc.SambaJobs(t.Context()); len(views) != 0 {
		t.Fatalf("SambaJobs = %d, want none", len(views))
	}
}

func TestACompleteSetOfCopiesIsOneWork(t *testing.T) {
	t.Parallel()

	r, f, lib := sambaRig(t)
	for i := 1; i <= 3; i++ {
		data := sevenZipVolume(i)
		if i > 1 {
			data = bytes.Repeat([]byte{byte(i)}, 128)
		}
		name := "Pack/movie.7z.00" + string(rune('0'+i))
		f.copy(t, lib, name, name, data, false)
	}

	if n, err := r.svc.AdoptSamba(t.Context()); err != nil || n != 1 {
		t.Fatalf("AdoptSamba = %d, %v; want one unit for the set", n, err)
	}
	views, err := r.svc.SambaJobs(t.Context())
	if err != nil || len(views) != 1 {
		t.Fatalf("SambaJobs = %d, %v; want the set listed once, by its first volume", len(views), err)
	}
	first := views[0].Job
	volumes, err := ingestionsqlite.NewJobRepository(r.conn).ByBatch(t.Context(), views[0].Batch.ID)
	if err != nil || len(volumes) != 3 {
		t.Fatalf("jobs of the set = %d, %v; want 3", len(volumes), err)
	}
	for _, v := range volumes {
		if v.PartsTotal != 3 || v.Status != domain.StatusExtracting || v.VolumeGroup != string(first.ID) {
			t.Fatalf("volume %s: total %d status %s group %q; want the claimed set of 3", v.FileName, v.PartsTotal, v.Status, v.VolumeGroup)
		}
	}
}

func TestASetWithAHoleFailsAsMissingParts(t *testing.T) {
	t.Parallel()

	r, f, lib := sambaRig(t)
	for _, i := range []int{1, 2, 4} {
		name := "movie.7z.00" + string(rune('0'+i))
		f.copy(t, lib, "Pack/"+name, "Pack/"+name, sevenZipVolume(i), false)
	}

	if _, err := r.svc.AdoptSamba(t.Context()); err != nil {
		t.Fatalf("AdoptSamba: %v", err)
	}
	views, err := r.svc.SambaJobs(t.Context())
	if err != nil || len(views) != 1 {
		t.Fatalf("SambaJobs = %d, %v; want the failed set listed", len(views), err)
	}
	if views[0].Job.Status != domain.StatusFailed || views[0].Job.Error != domain.FailurePartsMissing {
		t.Fatalf("set is %s (%q), want failed parts_missing", views[0].Job.Status, views[0].Job.Error)
	}
}

func TestATakeThatIsRefusedLeavesNoJobBehind(t *testing.T) {
	t.Parallel()

	r, f, lib := sambaRig(t)
	f.copy(t, lib, "movie.zip", "", videoZip(t), false)
	f.takeErr = application.ErrSambaChanged

	if n, err := r.svc.AdoptSamba(t.Context()); err != nil || n != 0 {
		t.Fatalf("AdoptSamba refused = %d, %v; want nothing taken and no error", n, err)
	}
	if views, _ := r.svc.SambaJobs(t.Context()); len(views) != 0 {
		t.Fatalf("SambaJobs = %d after a refused take, want none", len(views))
	}

	f.takeErr = nil
	if n, err := r.svc.AdoptSamba(t.Context()); err != nil || n != 1 {
		t.Fatalf("AdoptSamba after the refusal = %d, %v; want 1", n, err)
	}
}

func TestARestartFinishesACopyWhoseMoveWasCommitted(t *testing.T) {
	t.Parallel()

	r, f, lib := sambaRig(t)
	f.copy(t, lib, "movie.zip", "", videoZip(t), false)
	f.moveThenErr = errors.New("connection lost after the move")

	if n, err := r.svc.AdoptSamba(t.Context()); err != nil || n != 0 {
		t.Fatalf("AdoptSamba with a lost answer = %d, %v; want the unit not counted and the job kept", n, err)
	}
	f.moveThenErr = nil
	kept, err := r.svc.SambaJobs(t.Context())
	if err != nil || len(kept) != 1 || kept[0].Job.Status != domain.StatusUploading {
		t.Fatalf("before the restart the job is not kept uploading: %d jobs, %v", len(kept), err)
	}

	// The process stops here, and a new one starts on the same database and staging folder.
	restarted := application.NewService(ingestionsqlite.NewBatchRepository(r.conn), ingestionsqlite.NewJobRepository(r.conn), r.pub, nil, time.Now).
		WithStaging(diskStaging{dir: r.staging}).
		WithExtraction(workspace.New(r.staging, nil), sevenzip.New(""), r.trash)
	if _, err := restarted.Recover(t.Context()); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	views, err := restarted.SambaJobs(t.Context())
	if err != nil || len(views) != 1 {
		t.Fatalf("SambaJobs after the restart = %d, %v; want the copy's job once", len(views), err)
	}
	if views[0].Job.Status != domain.StatusUploaded {
		t.Fatalf("job is %s after the restart, want uploaded (its file is in staging)", views[0].Job.Status)
	}
}

func TestARestartForgetsACopyWhoseMoveWasUndone(t *testing.T) {
	t.Parallel()

	r, f, lib := sambaRig(t)
	f.copy(t, lib, "movie.zip", "", videoZip(t), false)

	// A job whose move never happened: the copy is still in the section, and the job has no file.
	batch, err := domain.NewSambaBatch("crashed-batch", "movie", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	job, err := domain.NewSambaJob("crashed-job", batch.ID, "movie.zip", 100, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := ingestionsqlite.NewBatchRepository(r.conn).Create(t.Context(), batch); err != nil {
		t.Fatal(err)
	}
	if err := ingestionsqlite.NewJobRepository(r.conn).Create(t.Context(), job); err != nil {
		t.Fatal(err)
	}

	if _, err := r.svc.Recover(t.Context()); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if _, err := ingestionsqlite.NewJobRepository(r.conn).Get(t.Context(), job.ID); !errors.Is(err, domain.ErrJobNotFound) {
		t.Fatalf("the job without its file survived the restart: %v", err)
	}
	if _, err := ingestionsqlite.NewBatchRepository(r.conn).Get(t.Context(), batch.ID); !errors.Is(err, domain.ErrBatchNotFound) {
		t.Fatalf("the batch without its jobs survived the restart: %v", err)
	}
	if n, err := r.svc.AdoptSamba(t.Context()); err != nil || n != 1 {
		t.Fatalf("AdoptSamba after the restart = %d, %v; want the copy taken once", n, err)
	}
}

func TestAPurgeDoesNotAbandonACopyOfSamba(t *testing.T) {
	t.Parallel()

	clk := &virtualClock{t: time.Date(2026, time.October, 13, 9, 0, 0, 0, time.UTC)}
	r := newRig(t, sevenzip.New(""), 1<<40, false, clk)
	old := clk.Now().Add(-48 * time.Hour)
	batch, err := domain.NewSambaBatch("old-batch", "movie", old)
	if err != nil {
		t.Fatal(err)
	}
	job, err := domain.NewSambaJob("old-job", batch.ID, "movie.zip", 100, 0, old)
	if err != nil {
		t.Fatal(err)
	}
	if err := ingestionsqlite.NewBatchRepository(r.conn).Create(t.Context(), batch); err != nil {
		t.Fatal(err)
	}
	if err := ingestionsqlite.NewJobRepository(r.conn).Create(t.Context(), job); err != nil {
		t.Fatal(err)
	}

	if _, err := r.svc.Purge(t.Context()); err != nil {
		t.Fatalf("Purge: %v", err)
	}
	got, err := ingestionsqlite.NewJobRepository(r.conn).Get(t.Context(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusUploading || got.Error != "" {
		t.Fatalf("a copy of Samba is %s (%q) after a purge; it must not be failed as an abandoned upload", got.Status, got.Error)
	}
}

func TestSambaJobsHidesWhatHasReachedTheSection(t *testing.T) {
	t.Parallel()

	r, _, _ := sambaRig(t)
	batch, err := domain.NewSambaBatch("ended-batch", "movie", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	job, err := domain.NewSambaJob("ended-job", batch.ID, "movie.zip", 100, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	job.Status = domain.StatusDone
	if err := ingestionsqlite.NewBatchRepository(r.conn).Create(t.Context(), batch); err != nil {
		t.Fatal(err)
	}
	if err := ingestionsqlite.NewJobRepository(r.conn).Create(t.Context(), job); err != nil {
		t.Fatal(err)
	}
	if err := ingestionsqlite.NewJobRepository(r.conn).Save(t.Context(), job); err != nil {
		t.Fatal(err)
	}

	if views, err := r.svc.SambaJobs(t.Context()); err != nil || len(views) != 0 {
		t.Fatalf("SambaJobs = %d, %v; a job that reached No asignados is listed by its entry, not here", len(views), err)
	}
}
