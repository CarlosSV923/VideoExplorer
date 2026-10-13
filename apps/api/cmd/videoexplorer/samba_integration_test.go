package main

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	catalogapplication "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	catalogshttp "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/interfaces/http"
	ingestionapplication "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	ingestiondomain "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sevenzip"
	ingestionsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/sqlite"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/workspace"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/config"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/database"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
)

// sambaGuard bounds every wait for the background work of an adoption. A healthy run never gets near it.
const sambaGuard = 30 * time.Second

// testStaging is the staging area of the tus adapter, on disk, for the tests: a removal deletes what the
// adapter would delete (the data, its info and its lock).
type testStaging struct{ dir string }

func (s testStaging) Inspect(context.Context, ingestiondomain.JobID) (ingestionapplication.StagedUpload, error) {
	return ingestionapplication.StagedUpload{}, nil
}

func (s testStaging) Remove(_ context.Context, id ingestiondomain.JobID) error {
	for _, p := range []string{"", ".info", ".lock"} {
		if err := os.Remove(filepath.Join(s.dir, string(id)+p)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (s testStaging) IDs(context.Context) ([]ingestiondomain.JobID, error) { return nil, nil }

// sambaEnv is the composition root's wiring over a real library folder, a real database and the real
// 7-Zip: the ingestion, the catalog's section and the section's HTTP handler, as main builds them.
type sambaEnv struct {
	t       *testing.T
	lib     string
	conn    *sql.DB
	catalog catalogWiring
	ingest  *ingestionapplication.Service
	section *catalogshttp.SectionHandler
}

// newSambaEnv builds the wiring and starts the workers, which stop when the test ends.
func newSambaEnv(t *testing.T) *sambaEnv {
	t.Helper()
	lib := t.TempDir()
	for _, dir := range []string{"Movies", "TV Shows", "_unassigned", ".videoexplorer/staging"} {
		if err := os.MkdirAll(filepath.Join(lib, dir), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.Config{
		LibraryPath: lib, DataPath: t.TempDir(), MoviesDir: "Movies", SeriesDir: "TV Shows",
		ExtractConcurrency: 1,
		VideoExtensions:    []string{".mkv", ".mp4"},
		SubtitleExtensions: []string{".srt"},
	}
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	conn, err := database.Open(ctx, cfg.DataPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	log := slog.New(slog.DiscardHandler)

	catalog := newCatalog(conn, cfg, log)
	ingest := newIngestion(conn, log, nil)
	svc := ingest.Service
	svc.WithStaging(testStaging{dir: stagingDir(lib)}).
		WithExtraction(workspace.New(stagingDir(lib), nil), sevenzip.New(""), trashForIngestion{trash: catalog.Trash})
	withFiltering(svc, conn, cfg)
	svc.WithLibrary(unassignedForIngestion{unassigned: catalog.Unassigned}).
		WithSamba(sambaOf{section: catalog.Section})
	if _, err := svc.Recover(ctx); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	go svc.RunExtraction(ctx, cfg.ExtractConcurrency)
	go svc.RunSettler(ctx)

	return &sambaEnv{
		t: t, lib: lib, conn: conn, catalog: catalog, ingest: svc,
		section: catalogshttp.NewSectionHandler(catalog.Section, arrivalsOf{ingest: svc}),
	}
}

// read reads the section now, as GET /unassigned does: it takes the settled copies first.
func (e *sambaEnv) read() httpapi.UnassignedSection {
	e.t.Helper()
	resp, err := e.section.GetUnassigned(e.t.Context(), httpapi.GetUnassignedRequestObject{})
	if err != nil {
		e.t.Fatalf("GetUnassigned: %v", err)
	}
	ok, isOK := resp.(httpapi.GetUnassigned200JSONResponse)
	if !isOK {
		e.t.Fatalf("GetUnassigned answered %T", resp)
	}
	return httpapi.UnassignedSection(ok)
}

// until reads the section until ok holds for it, and returns the reading.
func (e *sambaEnv) until(what string, ok func(httpapi.UnassignedSection) bool) httpapi.UnassignedSection {
	e.t.Helper()
	deadline := time.Now().Add(sambaGuard)
	for {
		got := e.read()
		if ok(got) {
			return got
		}
		if time.Now().After(deadline) {
			e.t.Fatalf("%s not reached; the section shows %s", what, describe(got))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// put writes a file of _unassigned/, last modified long ago so that it is settled (RF-27).
func (e *sambaEnv) put(rel string, data []byte, settled bool) string {
	e.t.Helper()
	path := filepath.Join(e.lib, "_unassigned", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		e.t.Fatal(err)
	}
	mtime := time.Now().Add(-2 * time.Minute)
	if !settled {
		mtime = time.Now()
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		e.t.Fatal(err)
	}
	return path
}

// inTrash reports whether a file of that name is in the trash (RF-30).
func (e *sambaEnv) inTrash(name string) bool {
	e.t.Helper()
	found := false
	_ = filepath.WalkDir(filepath.Join(e.lib, ".videoexplorer", "trash"), func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == name {
			found = true
		}
		return nil
	})
	return found
}

// entryNamed returns the entry of the section with that name.
func entryNamed(s httpapi.UnassignedSection, name string) (httpapi.UnassignedEntry, bool) {
	for _, e := range s.Entries {
		if e.Name == name {
			return e, true
		}
	}
	return httpapi.UnassignedEntry{}, false
}

// describe says what the section shows, for a failure message.
func describe(s httpapi.UnassignedSection) string {
	var parts []string
	for _, e := range s.Entries {
		parts = append(parts, fmt.Sprintf("entry %s (%s)", e.Name, e.Origin))
	}
	for _, j := range s.Jobs {
		parts = append(parts, fmt.Sprintf("job %s %s", j.FileName, j.Status))
	}
	return strings.Join(parts, ", ")
}

// movieZip is a zip with one fictitious video in a folder, as a release has it.
func movieZip(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create("Movie/Movie.mkv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("fictitious video bytes")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// need7zz skips a test that needs 7-Zip, except where REQUIRE_7ZZ is set (as the other tests do).
func need7zz(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("7zz"); err != nil {
		if os.Getenv("REQUIRE_7ZZ") != "" {
			t.Fatal("7zz not found but REQUIRE_7ZZ is set")
		}
		t.Skip("7zz not found")
	}
}

// makeArchive builds an archive of a fictitious video with the real 7-Zip, optionally split into
// volumes of 256 KiB and encrypted (header encryption), and returns the files it made, by name.
func makeArchive(t *testing.T, base string, volumes bool, password string) map[string][]byte {
	t.Helper()
	need7zz(t)

	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "Movie"), 0o750); err != nil {
		t.Fatal(err)
	}
	video := bytes.Repeat([]byte("video-bytes-"), 1<<16) // 768 KiB, so that a split makes several volumes
	if err := os.WriteFile(filepath.Join(src, "Movie", "Movie.mkv"), video, 0o600); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	args := []string{"a", "-bso0", "-bsp0", "-t7z", "-mx0"}
	if volumes {
		args = append(args, "-v256k")
	}
	if password != "" {
		args = append(args, "-p"+password, "-mhe=on")
	}
	args = append(args, filepath.Join(out, base+".7z"), "Movie")
	cmd := exec.CommandContext(t.Context(), "7zz", args...)
	cmd.Dir = src
	if msg, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("7zz a: %v\n%s", err, msg)
	}
	made, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, m := range made {
		data, err := os.ReadFile(filepath.Join(out, m.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files[m.Name()] = data
	}
	return files
}

func TestASambaZipIsTakenAndItsVideoBecomesAnEntry(t *testing.T) {
	t.Parallel()

	e := newSambaEnv(t)
	e.put("movie.zip", movieZip(t), true)

	got := e.until("the video entry", func(s httpapi.UnassignedSection) bool {
		entry, ok := entryNamed(s, "movie")
		return ok && len(s.Jobs) == 0 && string(entry.Origin) == "upload"
	})
	entry, _ := entryNamed(got, "movie")
	if len(entry.Files) != 1 || entry.Files[0].Path != "Movie/Movie.mkv" {
		t.Fatalf("entry files = %+v, want the video in its folder", entry.Files)
	}
	if _, err := os.Stat(filepath.Join(e.lib, "_unassigned", "movie.zip")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the archive is still in No asignados: %v", err)
	}
	if !e.inTrash("movie.zip") {
		t.Fatal("the original archive is not in the trash (RF-06)")
	}
}

func TestAnArchiveInsideASambaFolderKeepsTheFolder(t *testing.T) {
	t.Parallel()

	e := newSambaEnv(t)
	e.put("Pack/movie.zip", movieZip(t), true)
	e.put("Pack/readme.txt", []byte("not a video"), true)

	got := e.until("the video entry", func(s httpapi.UnassignedSection) bool {
		_, ok := entryNamed(s, "movie")
		return ok && len(s.Jobs) == 0
	})
	pack, ok := entryNamed(got, "Pack")
	if !ok || string(pack.Origin) != "samba" || len(pack.Files) != 1 || pack.Files[0].Path != "readme.txt" {
		t.Fatalf("Pack = %+v, want the Samba folder with its readme", pack)
	}
}

func TestAMultipartSetOfSambaIsOneEntryAndAnIncompleteOneFails(t *testing.T) {
	t.Parallel()

	t.Run("complete", func(t *testing.T) {
		t.Parallel()
		e := newSambaEnv(t)
		for name, data := range makeArchive(t, "movie", true, "") {
			e.put("Pack/"+name, data, true)
		}
		got := e.until("the set's entry", func(s httpapi.UnassignedSection) bool {
			_, ok := entryNamed(s, "movie")
			return ok && len(s.Jobs) == 0
		})
		if entry, _ := entryNamed(got, "movie"); len(entry.Files) != 1 || entry.Files[0].Path != "Movie/Movie.mkv" {
			t.Fatalf("set entry files = %+v, want the video", entry.Files)
		}
	})

	t.Run("incomplete", func(t *testing.T) {
		t.Parallel()
		e := newSambaEnv(t)
		for name, data := range makeArchive(t, "movie", true, "") {
			if strings.HasSuffix(name, ".002") {
				continue // a hole in the set
			}
			e.put("Pack/"+name, data, true)
		}
		got := e.until("the failed set", func(s httpapi.UnassignedSection) bool {
			return len(s.Jobs) == 1 && s.Jobs[0].Status == "failed"
		})
		if got.Jobs[0].Error == nil || string(*got.Jobs[0].Error) != "parts_missing" {
			t.Fatalf("incomplete set fails with %v, want parts_missing", got.Jobs[0].Error)
		}
		// The copies of the failed set are in the trash, not deleted (RF-27a).
		for _, name := range []string{"movie.7z.001", "movie.7z.003"} {
			if !e.inTrash(name) {
				t.Fatalf("%s is not in the trash after the set failed", name)
			}
		}
		// "Quitar" takes the set off the section; the job stays in the history of the jobs (RF-27a).
		if _, err := e.ingest.DismissSamba(e.t.Context(), ingestiondomain.JobID(got.Jobs[0].Id)); err != nil {
			t.Fatalf("DismissSamba: %v", err)
		}
		if s := e.read(); len(s.Jobs) != 0 {
			t.Fatalf("the section still lists the dismissed set: %s", describe(s))
		}
		history, err := e.ingest.List(e.t.Context())
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		found := false
		for _, v := range history {
			found = found || string(v.Job.ID) == got.Jobs[0].Id
		}
		if !found {
			t.Fatal("the dismissed set left the jobs history")
		}
	})
}

func TestAnEncryptedSambaArchiveAsksForItsPasswordOnce(t *testing.T) {
	t.Parallel()

	e := newSambaEnv(t)
	const password = "test-only-password-7f3a" // a value of this test, not a credential
	for _, data := range makeArchive(t, "secret", false, password) {
		e.put("secret.7z", data, true)
	}

	waiting := e.until("the password request", func(s httpapi.UnassignedSection) bool {
		return len(s.Jobs) == 1 && s.Jobs[0].Status == "needs_password"
	})
	id := ingestiondomain.JobID(waiting.Jobs[0].Id)
	if _, err := e.ingest.SubmitPassword(e.t.Context(), id, password); err != nil {
		t.Fatalf("SubmitPassword: %v", err)
	}
	e.until("the entry after the password", func(s httpapi.UnassignedSection) bool {
		_, ok := entryNamed(s, "secret")
		return ok && len(s.Jobs) == 0
	})
}

func TestACopyStillBeingCopiedWaitsUntilItSettles(t *testing.T) {
	t.Parallel()

	e := newSambaEnv(t)
	path := e.put("movie.zip", movieZip(t), false)

	if s := e.read(); len(s.Jobs) != 0 {
		t.Fatalf("a copy still being copied was taken: %s", describe(s))
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the copy being copied moved: %v", err)
	}
	old := time.Now().Add(-2 * time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	e.until("the settled copy's entry", func(s httpapi.UnassignedSection) bool {
		_, ok := entryNamed(s, "movie")
		return ok && len(s.Jobs) == 0
	})
}

func TestAnUploadEntryIsNotTouchedBySambaAdoption(t *testing.T) {
	t.Parallel()

	e := newSambaEnv(t)
	staged := filepath.Join(e.lib, ".videoexplorer", "staging", "upload-kept")
	if err := os.WriteFile(staged, []byte("uploaded video bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.catalog.Unassigned.Put(e.t.Context(), catalogapplication.AsideRequest{
		Source: "upload-batch", Name: "Uploaded", BatchID: "upload-batch", Relation: "none",
		Files: []catalogapplication.AsideFile{{Source: staged, Rel: "Uploaded.mkv", Size: 20}},
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	e.put("movie.zip", movieZip(t), true)

	got := e.until("the samba entry", func(s httpapi.UnassignedSection) bool {
		_, ok := entryNamed(s, "movie")
		return ok && len(s.Jobs) == 0
	})
	kept, ok := entryNamed(got, "Uploaded")
	if !ok || string(kept.Origin) != "upload" || len(kept.Files) != 1 || kept.Files[0].Path != "Uploaded.mkv" {
		t.Fatalf("the upload entry = %+v, want it unchanged", kept)
	}
}

func TestReadingTwiceTakesTheCopyOnce(t *testing.T) {
	t.Parallel()

	e := newSambaEnv(t)
	e.put("movie.zip", movieZip(t), true)
	e.read()
	e.read()
	e.until("the entry", func(s httpapi.UnassignedSection) bool {
		_, ok := entryNamed(s, "movie")
		return ok && len(s.Jobs) == 0
	})
	var batches int
	if err := e.conn.QueryRowContext(e.t.Context(), `SELECT COUNT(*) FROM upload_batches WHERE origin = 'samba'`).Scan(&batches); err != nil {
		t.Fatal(err)
	}
	if batches != 1 {
		t.Fatalf("samba batches = %d after two readings, want 1", batches)
	}
}

func TestARestartMidMoveNeitherLosesNorDuplicatesTheCopy(t *testing.T) {
	t.Parallel()

	t.Run("the move was committed", func(t *testing.T) {
		t.Parallel()
		e := newSambaEnv(t)
		data := movieZip(t)
		e.put("movie.zip", data, true)

		// The move reached staging, but the process stopped before the job was completed.
		batch, err := ingestiondomain.NewSambaBatch("cut-batch-1", "movie", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		job, err := ingestiondomain.NewSambaJob("cut-job-1", batch.ID, "movie.zip", "movie.zip", int64(len(data)), 0, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err := ingestionsqlite.NewBatchRepository(e.conn).Create(e.t.Context(), batch); err != nil {
			t.Fatal(err)
		}
		if err := ingestionsqlite.NewJobRepository(e.conn).Create(e.t.Context(), job); err != nil {
			t.Fatal(err)
		}
		src := filepath.Join(e.lib, "_unassigned", "movie.zip")
		if err := os.Rename(src, filepath.Join(stagingDir(e.lib), string(job.ID))); err != nil {
			t.Fatal(err)
		}

		if _, err := e.ingest.Recover(e.t.Context()); err != nil {
			t.Fatalf("Recover: %v", err)
		}
		e.until("the entry of the finished copy", func(s httpapi.UnassignedSection) bool {
			_, ok := entryNamed(s, "movie")
			return ok && len(s.Jobs) == 0
		})
		var batches int
		if err := e.conn.QueryRowContext(e.t.Context(), `SELECT COUNT(*) FROM upload_batches WHERE origin = 'samba'`).Scan(&batches); err != nil {
			t.Fatal(err)
		}
		if batches != 1 {
			t.Fatalf("samba batches = %d, want the one batch of the copy", batches)
		}
	})

	t.Run("the move was undone", func(t *testing.T) {
		t.Parallel()
		e := newSambaEnv(t)
		data := movieZip(t)
		e.put("movie.zip", data, true)

		// A job without its file: its move never happened, and the copy is still in the section.
		batch, err := ingestiondomain.NewSambaBatch("cut-batch-2", "movie", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		job, err := ingestiondomain.NewSambaJob("cut-job-2", batch.ID, "movie.zip", "movie.zip", int64(len(data)), 0, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err := ingestionsqlite.NewBatchRepository(e.conn).Create(e.t.Context(), batch); err != nil {
			t.Fatal(err)
		}
		if err := ingestionsqlite.NewJobRepository(e.conn).Create(e.t.Context(), job); err != nil {
			t.Fatal(err)
		}

		if _, err := e.ingest.Recover(e.t.Context()); err != nil {
			t.Fatalf("Recover: %v", err)
		}
		got := e.until("the entry of the copy taken again", func(s httpapi.UnassignedSection) bool {
			_, ok := entryNamed(s, "movie")
			return ok && len(s.Jobs) == 0
		})
		if _, ok := entryNamed(got, "movie"); !ok {
			t.Fatal("the copy was lost")
		}
		var batches int
		if err := e.conn.QueryRowContext(e.t.Context(), `SELECT COUNT(*) FROM upload_batches WHERE origin = 'samba'`).Scan(&batches); err != nil {
			t.Fatal(err)
		}
		if batches != 1 {
			t.Fatalf("samba batches = %d, want the forgotten batch gone and one new", batches)
		}
	})
}
