package application_test

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sevenzip"
	ingestionsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/workspace"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/database"
)

// extractionGuard bounds every wait for the background workers. A healthy run never gets near it.
const extractionGuard = 20 * time.Second

// diskStaging is the staging folder of the tus adapter, on disk: Remove deletes an
// upload's files the way the adapter does (the data, its info and its lock).
type diskStaging struct{ dir string }

func (d diskStaging) Inspect(context.Context, domain.JobID) (application.StagedUpload, error) {
	return application.StagedUpload{}, nil
}

func (d diskStaging) Remove(_ context.Context, id domain.JobID) error {
	for _, p := range []string{"", ".info", ".lock"} {
		if err := os.Remove(filepath.Join(d.dir, string(id)+p)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (d diskStaging) IDs(context.Context) ([]domain.JobID, error) { return nil, nil }

// recordingTrash moves the files it is given into a folder of its own and
// keeps what it was asked to trash.
type recordingTrash struct {
	dir string
	mu  sync.Mutex
	put []application.TrashPut
	err error
	// attempts counts the calls, failed or not; before, when set, sees each call first.
	attempts int
	before   func(context.Context, application.TrashPut)
	// failName makes the next call for a file with this name fail, once.
	failName string
}

func (r *recordingTrash) Put(ctx context.Context, p application.TrashPut) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.attempts++
	if r.before != nil {
		r.before(ctx, p)
	}
	if r.err != nil {
		return r.err
	}
	if r.failName != "" && p.Name == r.failName {
		r.failName = ""
		return errors.New("input/output error")
	}
	dest := filepath.Join(r.dir, fmt.Sprint(len(r.put)), p.Name)
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return err
	}
	if err := os.Rename(p.Source, dest); err != nil {
		return err
	}
	r.put = append(r.put, p)
	return nil
}

func (r *recordingTrash) puts() []application.TrashPut {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]application.TrashPut(nil), r.put...)
}

// setErr makes the next calls fail with err (nil makes them work again).
func (r *recordingTrash) setErr(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.err = err
}

// calls returns how many times Put was called.
func (r *recordingTrash) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.attempts
}

// tamperingExtractor reports a CRC32 in the index that the file does not have, as
// an archive whose index and content disagree would (RF-05).
type tamperingExtractor struct{ application.Extractor }

func (x tamperingExtractor) List(ctx context.Context, archive, password string) (application.Listing, error) {
	l, err := x.Extractor.List(ctx, archive, password)
	for i := range l.Entries {
		if l.Entries[i].CRC != "" {
			l.Entries[i].CRC = "DEADBEEF"
		}
	}
	return l, err
}

// runSettler runs the settler of svc until the test ends: the batches are decided in the
// background, as in the process. A test that needs a decision waits for it with WaitSettled.
func runSettler(t *testing.T, svc *application.Service) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	go svc.RunSettler(ctx)
}

// rig is a service with the extraction wired to disk, a real database and a
// trash that records what it gets. Its workers run until the test ends.
type rig struct {
	svc     *application.Service
	conn    *sql.DB
	staging string
	trash   *recordingTrash
	pub     *recorder
	batch   domain.BatchID

	mu   sync.Mutex
	free uint64
}

// newRig builds the rig. free is the free space the workspace reports; the
// workers only run when run is true.
func newRig(t *testing.T, extractor application.Extractor, free uint64, run bool, clk *virtualClock) *rig {
	t.Helper()

	dataDir := t.TempDir()
	conn, err := database.Open(t.Context(), dataDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	r := &rig{staging: filepath.Join(t.TempDir(), "staging"), trash: &recordingTrash{dir: t.TempDir()}, pub: &recorder{}, free: free}
	if err := os.MkdirAll(r.staging, 0o750); err != nil {
		t.Fatal(err)
	}
	space := func(string) (uint64, error) {
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.free, nil
	}
	now := time.Now
	if clk != nil {
		now = clk.Now
	}
	svc := application.NewService(ingestionsqlite.NewBatchRepository(conn), ingestionsqlite.NewJobRepository(conn), r.pub, nil, now).
		WithStaging(diskStaging{dir: r.staging}).
		WithExtraction(workspace.New(r.staging, space), extractor, r.trash).
		WithFiltering(ingestionsqlite.NewFilterRepository(conn), filterRules())
	r.svc = svc
	r.conn = conn

	batch, err := svc.CreateBatch(t.Context(), "", domain.RelationNone)
	if err != nil {
		t.Fatal(err)
	}
	r.batch = batch.ID

	if run {
		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)
		go svc.RunSettler(ctx)
		go svc.RunExtraction(ctx, 2)
	}
	return r
}

func (r *rig) setFree(n uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.free = n
}

// upload admits a file of the rig's batch, stages its bytes and completes it, as
// the tus endpoint does (RF-02, RF-10a).
func (r *rig) upload(t *testing.T, name string, data []byte) domain.JobID {
	t.Helper()
	return r.uploadPart(t, name, data, 0)
}

// uploadPart is upload for a volume of a set of partsTotal volumes (RF-03).
func (r *rig) uploadPart(t *testing.T, name string, data []byte, partsTotal int) domain.JobID {
	t.Helper()

	id, err := r.svc.Admit(t.Context(), application.Admission{
		Meta: domain.UploadMeta{FileName: name, BatchID: string(r.batch), PartsTotal: partsTotal},
		Size: int64(len(data)),
	})
	if err != nil {
		t.Fatalf("Admit %s: %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(r.staging, string(id)), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Completed(t.Context(), id); err != nil {
		t.Fatalf("Completed %s: %v", name, err)
	}
	return id
}

// waitFor polls the job until ok holds and returns it.
func (r *rig) waitFor(t *testing.T, id domain.JobID, what string, ok func(domain.UploadJob) bool) domain.UploadJob {
	t.Helper()

	deadline := time.Now().Add(extractionGuard)
	for {
		job, err := r.svc.Job(t.Context(), id)
		if err != nil {
			t.Fatalf("Job: %v", err)
		}
		if ok(*job) {
			return *job
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: job is %s (%q), not reached", what, job.Status, job.Error)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// extractionDir is the folder the workspace extracts a job into (staging/extract/<id>).
func (r *rig) extractionDir(id domain.JobID) string {
	return filepath.Join(r.staging, "extract", string(id))
}

func (r *rig) staged(id domain.JobID) bool {
	_, err := os.Stat(filepath.Join(r.staging, string(id)))
	return err == nil
}

// waitTrashed waits until the trash has received n originals. A job is filtering before its
// original goes to the trash (finish saves the state first), so a test that waits for the
// filtering state waits for the trash too.
func (r *rig) waitTrashed(t *testing.T, n int) {
	t.Helper()

	deadline := time.Now().Add(extractionGuard)
	for len(r.trash.puts()) < n {
		if time.Now().After(deadline) {
			t.Fatalf("the trash holds %d originals, want %d", len(r.trash.puts()), n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// zipBytes builds a zip in memory with the given entries, in any mode.
func zipBytes(t *testing.T, entries ...zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, e := range entries {
		hdr := &zip.FileHeader{Name: e.name, Method: zip.Store}
		hdr.SetMode(e.mode)
		f, err := w.CreateHeader(hdr)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(e.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type zipEntry struct {
	name, content string
	mode          fs.FileMode
}

// videoZip is a zip with one fictitious video.
func videoZip(t *testing.T) []byte {
	return zipBytes(t, zipEntry{name: "Movie/Movie.mkv", content: "fictitious video bytes", mode: 0o644})
}

// need7zz skips a test without 7-Zip, except where REQUIRE_7ZZ is set.
func need7zz(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("7zz"); err != nil {
		if os.Getenv("REQUIRE_7ZZ") != "" {
			t.Fatal("7zz not found but REQUIRE_7ZZ is set")
		}
		t.Skip("7zz not installed")
	}
}

// encrypted7z packs one fictitious video with 7zz: header-encrypted with password.
func encrypted7z(t *testing.T, password string) []byte {
	t.Helper()
	need7zz(t)
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "Movie"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "Movie", "Movie.mkv"), []byte("fictitious video bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "secret.7z")
	cmd := exec.CommandContext(t.Context(), "7zz", "a", "-bso0", "-bsp0", "-t7z", "-p"+password, "-mhe=on", archive, "Movie")
	cmd.Dir = src
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("7zz a: %v\n%s", err, out)
	}
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// realExtractor is the 7-Zip adapter the application runs with.
func realExtractor() application.Extractor { return sevenzip.New("") }

func TestAnArchiveIsExtractedAndItsOriginalGoesToTheTrash(t *testing.T) {
	t.Parallel()

	r := newRig(t, realExtractor(), 1<<40, true, nil)
	data := videoZip(t)
	id := r.upload(t, "Movie.zip", data)

	job := r.waitFor(t, id, "extraction finished", func(j domain.UploadJob) bool {
		return j.Status == domain.StatusFiltering
	})
	if job.Progress != 100 || job.Error != "" {
		t.Errorf("job = %+v, want filtering at 100 with no reason", job)
	}
	got, err := os.ReadFile(filepath.Join(r.extractionDir(id), "Movie", "Movie.mkv"))
	if err != nil || string(got) != "fictitious video bytes" {
		t.Fatalf("extracted video = %q, %v", got, err)
	}
	r.waitTrashed(t, 1)
	if r.staged(id) {
		t.Error("the archive is still in staging")
	}
	puts := r.trash.puts()
	if len(puts) != 1 || puts[0].Name != "Movie.zip" || puts[0].Origin != application.TrashOriginUpload ||
		puts[0].BatchID != string(r.batch) || puts[0].RelativePath != "Movie.zip" ||
		puts[0].Reason != application.TrashReasonExtracted || puts[0].Size != int64(len(data)) {
		t.Fatalf("trash = %+v", puts)
	}

	// The change is announced right after it is saved: wait for the announcement itself.
	deadline := time.Now().Add(extractionGuard)
	for !slices.ContainsFunc(r.pub.published(id), func(v domain.JobView) bool { return v.Job.Status == domain.StatusFiltering }) {
		if time.Now().After(deadline) {
			t.Fatal("the filtering state was saved but not announced")
		}
		time.Sleep(5 * time.Millisecond)
	}
	var statuses []domain.Status
	for _, v := range r.pub.published(id) {
		if len(statuses) == 0 || statuses[len(statuses)-1] != v.Job.Status {
			statuses = append(statuses, v.Job.Status)
		}
	}
	if want := []domain.Status{domain.StatusUploading, domain.StatusUploaded, domain.StatusExtracting, domain.StatusFiltering}; !slices.Equal(statuses, want) {
		t.Errorf("announced states = %v, want %v", statuses, want)
	}
}

func TestAnEncryptedArchiveWaitsForAPasswordThatOpensIt(t *testing.T) {
	t.Parallel()

	r := newRig(t, realExtractor(), 1<<40, true, nil)
	id := r.upload(t, "secret.7z", encrypted7z(t, "S3cret"))

	r.waitFor(t, id, "asks for the password", func(j domain.UploadJob) bool {
		return j.Status == domain.StatusNeedsPassword && j.Error == ""
	})
	if !r.staged(id) {
		t.Fatal("the archive left staging while it waits for its password")
	}

	if _, err := r.svc.SubmitPassword(t.Context(), id, "nope"); err != nil {
		t.Fatalf("SubmitPassword: %v", err)
	}
	r.waitFor(t, id, "wrong password is reported", func(j domain.UploadJob) bool {
		return j.Status == domain.StatusNeedsPassword && j.Error == domain.FailureWrongPassword
	})

	if _, err := r.svc.SubmitPassword(t.Context(), id, "S3cret"); err != nil {
		t.Fatalf("SubmitPassword: %v", err)
	}
	job := r.waitFor(t, id, "extracted with the right password", func(j domain.UploadJob) bool {
		return j.Status == domain.StatusFiltering
	})
	if job.Error != "" {
		t.Errorf("error = %q after a right password", job.Error)
	}
	r.waitTrashed(t, 1)
	if len(r.trash.puts()) != 1 {
		t.Errorf("trash entries = %d, want 1", len(r.trash.puts()))
	}
}

func TestNoSpaceFailsAndRetryExtractsOnceTheSpaceIsThere(t *testing.T) {
	t.Parallel()

	r := newRig(t, realExtractor(), 0, true, nil)
	id := r.upload(t, "Movie.zip", videoZip(t))

	job := r.waitFor(t, id, "no space", func(j domain.UploadJob) bool { return j.Status == domain.StatusFailed })
	if job.Error != domain.FailureNoSpace {
		t.Fatalf("error = %q, want no_space", job.Error)
	}
	if !r.staged(id) {
		t.Fatal("the archive must stay for a retry (RF-10c)")
	}

	r.setFree(1 << 40)
	if _, err := r.svc.Retry(t.Context(), id); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	r.waitFor(t, id, "extracted after the retry", func(j domain.UploadJob) bool {
		return j.Status == domain.StatusFiltering
	})
}

func TestUnsafeEntriesFailAsArchiveUnsafeAndCannotBeRetried(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		entries []zipEntry
	}{
		{name: "zip-slip", entries: []zipEntry{{name: "../../evil.mkv", content: "x", mode: 0o644}}},
		{name: "symbolic link", entries: []zipEntry{{name: "Movie/link.mkv", content: "/etc/passwd", mode: fs.ModeSymlink | 0o777}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := newRig(t, realExtractor(), 1<<40, true, nil)
			id := r.upload(t, "bad.zip", zipBytes(t, tt.entries...))

			job := r.waitFor(t, id, "refused", func(j domain.UploadJob) bool { return j.Status == domain.StatusFailed })
			if job.Error != domain.FailureArchiveUnsafe {
				t.Fatalf("error = %q, want archive_unsafe", job.Error)
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(r.staging), "evil.mkv")); err == nil {
				t.Fatal("an entry was written outside the extraction folder")
			}
			if _, err := r.svc.Retry(t.Context(), id); !errors.Is(err, application.ErrCannotRetry) {
				t.Fatalf("Retry: err = %v, want ErrCannotRetry", err)
			}
		})
	}
}

func TestDamagedArchiveFailsAsArchiveCorruptAndRetryFailsAgainWhileItIsDamaged(t *testing.T) {
	t.Parallel()

	r := newRig(t, realExtractor(), 1<<40, true, nil)
	data := videoZip(t)
	id := r.upload(t, "Movie.zip", data[:len(data)/2])

	job := r.waitFor(t, id, "damaged", func(j domain.UploadJob) bool { return j.Status == domain.StatusFailed })
	if job.Error != domain.FailureArchiveCorrupt || !r.staged(id) {
		t.Fatalf("error = %q, staged = %v; want archive_corrupt with the archive kept", job.Error, r.staged(id))
	}
	if _, err := r.svc.Retry(t.Context(), id); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	job = r.waitFor(t, id, "damaged again", func(j domain.UploadJob) bool { return j.Status == domain.StatusFailed })
	if job.Error != domain.FailureArchiveCorrupt {
		t.Fatalf("after retry: error = %q, want archive_corrupt", job.Error)
	}
}

func TestAnIndexThatDisagreesWithTheFilesFailsAsCRCMismatch(t *testing.T) {
	t.Parallel()

	r := newRig(t, tamperingExtractor{realExtractor()}, 1<<40, true, nil)
	id := r.upload(t, "Movie.zip", videoZip(t))

	job := r.waitFor(t, id, "crc mismatch", func(j domain.UploadJob) bool { return j.Status == domain.StatusFailed })
	if job.Error != domain.FailureCRCMismatch {
		t.Fatalf("error = %q, want crc_mismatch", job.Error)
	}
	if _, err := r.svc.Retry(t.Context(), id); err != nil {
		t.Fatalf("crc_mismatch admits a retry: %v", err)
	}
}

func TestPlainFilesAreNotExtractedAndAVolumeWaitsForItsSet(t *testing.T) {
	t.Parallel()

	r := newRig(t, realExtractor(), 1<<40, true, nil)
	video := r.upload(t, "Movie.mkv", []byte("\x1A\x45\xDF\xA3 not an archive"))
	// A volume of a set of two, of which only this one arrived: it waits (RF-03, RF-10a).
	part := r.uploadPart(t, "Movie.part1.rar", []byte("Rar!\x1A\x07\x00 a volume"), 2)
	// The archive uploaded last finishes after the others were looked at: the workers take requests in order.
	zipID := r.upload(t, "Movie.zip", videoZip(t))
	r.waitFor(t, zipID, "the last archive", func(j domain.UploadJob) bool { return j.Status == domain.StatusFiltering })

	// A video on its own is not extracted: it is filtered as it was uploaded, and kept in staging (RF-07).
	r.waitFor(t, video, "the video filtered", func(j domain.UploadJob) bool { return j.FilterDone })
	want := map[domain.JobID]domain.Status{video: domain.StatusFiltering, part: domain.StatusWaitingParts}
	for id, status := range want {
		job, err := r.svc.Job(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		if job.Status != status || !r.staged(id) {
			t.Errorf("%s: status %s, staged %v; want %s and untouched", job.FileName, job.Status, r.staged(id), status)
		}
	}
	if n := len(r.trash.puts()); n != 1 {
		t.Errorf("trash entries = %d, want only the zip", n)
	}
}

func TestSubmitPasswordAcceptsOnlyAPasswordForAJobWaitingForIt(t *testing.T) {
	t.Parallel()

	r := newRig(t, realExtractor(), 1<<40, true, nil)
	waiting := r.upload(t, "secret.7z", encrypted7z(t, "S3cret"))
	r.waitFor(t, waiting, "waiting", func(j domain.UploadJob) bool { return j.Status == domain.StatusNeedsPassword })
	plain := r.upload(t, "Movie.zip", videoZip(t))

	tests := []struct {
		name     string
		id       domain.JobID
		password string
		wantErr  error
	}{
		{name: "an unknown job", id: "nope", password: "x", wantErr: domain.ErrJobNotFound},
		{name: "a job that does not wait for a password", id: plain, password: "x", wantErr: application.ErrNotWaitingForPassword},
		{name: "an empty password", id: waiting, password: "", wantErr: application.ErrInvalidPassword},
		{name: "a password with a NUL", id: waiting, password: "a\x00b", wantErr: application.ErrInvalidPassword},
		{name: "a password too long", id: waiting, password: strings.Repeat("p", 1025), wantErr: application.ErrInvalidPassword},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := r.svc.SubmitPassword(t.Context(), tt.id, tt.password); !errors.Is(err, tt.wantErr) {
				t.Fatalf("SubmitPassword: err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestDeletingAnUploadWaitingForItsPasswordCancelsIt(t *testing.T) {
	t.Parallel()

	r := newRig(t, realExtractor(), 1<<40, true, nil)
	id := r.upload(t, "secret.7z", encrypted7z(t, "S3cret"))
	r.waitFor(t, id, "waiting", func(j domain.UploadJob) bool { return j.Status == domain.StatusNeedsPassword })

	if err := r.svc.Terminated(t.Context(), id); err != nil {
		t.Fatalf("Terminated: %v", err)
	}
	job, err := r.svc.Job(t.Context(), id)
	if err != nil || job.Status != domain.StatusCancelled {
		t.Fatalf("job = %+v, %v; want cancelled", job, err)
	}
	if _, err := os.Stat(r.extractionDir(id)); err == nil {
		t.Error("the folder of the cancelled extraction is still there")
	}
}

func TestAnInterruptedExtractionIsExtractedAgainAfterARestart(t *testing.T) {
	t.Parallel()

	// No workers yet: the job is left in extracting, as a stop in the middle leaves it.
	r := newRig(t, realExtractor(), 1<<40, false, nil)
	id := r.upload(t, "Movie.zip", videoZip(t))
	job, err := r.svc.Job(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	job.Status = domain.StatusExtracting
	job.UpdatedAt = time.Now()
	if err := ingestionsqlite.NewJobRepository(r.conn).Save(t.Context(), job); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	go r.svc.RunSettler(ctx)
	go r.svc.RunExtraction(ctx, 1)
	r.waitFor(t, id, "resumed and extracted", func(j domain.UploadJob) bool { return j.Status == domain.StatusFiltering })
}
