package http_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/libraryfs"
	catalogsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite"
	catalogshttp "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/interfaces/http"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/database"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/platform/httpapi"
)

// sectionHTTP is No asignados with a real library, database and clock, behind both handlers.
type sectionHTTP struct {
	section *catalogshttp.SectionHandler
	assign  *catalogshttp.Handler
	svc     *application.Section
	root    string
	now     time.Time
}

func setupSection(t *testing.T) *sectionHTTP {
	t.Helper()
	conn, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	root := t.TempDir()
	for _, dir := range []string{"Movies", "TV Shows", domain.UnassignedDir} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	env := &sectionHTTP{root: root, now: time.Date(2026, time.October, 13, 9, 0, 0, 0, time.UTC)}
	clock := func() time.Time { return env.now }
	files := libraryfs.New(root)
	engine := application.NewEngine(files, catalogsqlite.NewJournal(conn), clock, nil)
	repo := catalogsqlite.NewUnassignedRepository(conn)
	library := catalogsqlite.NewLibraryRepository(conn)
	section := application.NewSection(application.SectionDeps{
		Unassigned: repo, Files: files, Engine: engine, VideoExts: []string{".mkv"}, SubtitleExts: []string{".srt"}, Now: clock,
	})
	env.svc = section
	env.section = catalogshttp.NewSectionHandler(section, nil, nil)
	env.assign = catalogshttp.NewHandler(application.NewAssignments(application.AssignDeps{
		Unassigned: repo, Library: library, Files: files, Engine: engine,
		Finder: application.NewDuplicateFinder(library, files, contentHash{}, []string{".mkv"}),
		TMDB:   movieTMDB{}, Names: noNames{}, Roots: application.Roots{Movies: "Movies", Series: "TV Shows"},
		VideoExts: []string{".mkv"}, SubtitleExts: []string{".srt"}, Now: clock,
		NewToken: func() string { return "tab-s" },
	}), nil)
	return env
}

// put writes a file in _unassigned/ last modified at mtime.
func (env *sectionHTTP) put(t *testing.T, rel string, mtime time.Time) {
	t.Helper()
	p := filepath.Join(env.root, domain.UnassignedDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

// idOf returns the id of the section entry called name, read from the section now.
func (env *sectionHTTP) idOf(t *testing.T, name string) int64 {
	t.Helper()
	got, err := env.section.GetUnassigned(t.Context(), httpapi.GetUnassignedRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range got.(httpapi.GetUnassigned200JSONResponse).Entries {
		if e.Name == name {
			return e.Id
		}
	}
	t.Fatalf("no entry %s in the section", name)
	return 0
}

func TestTheSectionAnswersWhatSambaCopiedAndWhatIsCopying(t *testing.T) {
	t.Parallel()
	env := setupSection(t)
	env.put(t, "Copy/movie.mkv", env.now.Add(-10*time.Second))
	env.put(t, "Done/a.mkv", env.now.Add(-2*time.Minute))
	env.put(t, "Done/Thumbs.db", env.now.Add(-2*time.Minute))

	got, err := env.section.GetUnassigned(t.Context(), httpapi.GetUnassignedRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	entries := got.(httpapi.GetUnassigned200JSONResponse).Entries
	if len(entries) != 2 {
		t.Fatalf("entries = %+v, want two", entries)
	}
	copying, done := entries[0], entries[1]
	if copying.Name != "Copy" || copying.Status != "copying" || copying.Origin != "samba" || copying.Relation != "none" ||
		len(copying.Files) != 1 || !copying.Files[0].Copying || copying.Files[0].Path != "movie.mkv" {
		t.Errorf("Copy = %+v, want a samba entry copying its movie", copying)
	}
	if done.Name != "Done" || done.Status != "ready" || len(done.Files) != 1 || done.Files[0].Copying || done.Size != 7 {
		t.Errorf("Done = %+v, want a ready entry with its file and without the junk", done)
	}
}

func TestAChangeOfACopyingOrAssignedEntryIsRefused(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	env := setupSection(t)
	env.put(t, "Copy/movie.mkv", env.now.Add(-5*time.Second))
	id := env.idOf(t, "Copy")

	trashed, err := env.section.TrashUnassignedEntry(ctx, httpapi.TrashUnassignedEntryRequestObject{Id: id})
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := trashed.(httpapi.TrashUnassignedEntry409ApplicationProblemPlusJSONResponse); !ok || r.Code != "copying" || r.Status != 409 {
		t.Errorf("trash of a copying entry = %#v, want 409 with code copying (RF-27)", trashed)
	}
	deleted, err := env.section.DeleteUnassignedFile(ctx, httpapi.DeleteUnassignedFileRequestObject{Id: id, Body: &httpapi.DeleteUnassignedFileJSONRequestBody{Path: "movie.mkv"}})
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := deleted.(httpapi.DeleteUnassignedFile409ApplicationProblemPlusJSONResponse); !ok || r.Code != "copying" {
		t.Errorf("delete of a copying file = %#v, want 409 with code copying", deleted)
	}
	taken, err := env.assign.TakeAssignLock(ctx, httpapi.TakeAssignLockRequestObject{Id: id})
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := taken.(httpapi.TakeAssignLock409ApplicationProblemPlusJSONResponse); !ok || r.Code != "copying" {
		t.Errorf("assignment of a copying entry = %#v, want 409 with code copying (RF-27)", taken)
	}

	// Once it has settled, the entry takes the action; while another tab holds it, it refuses (RF-26).
	env.now = env.now.Add(time.Minute)
	lock, err := env.assign.TakeAssignLock(ctx, httpapi.TakeAssignLockRequestObject{Id: id})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := lock.(httpapi.TakeAssignLock200JSONResponse); !ok {
		t.Fatalf("assignment once settled = %#v, want the lock", lock)
	}
	held, err := env.section.TrashUnassignedEntry(ctx, httpapi.TrashUnassignedEntryRequestObject{Id: id})
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := held.(httpapi.TrashUnassignedEntry409ApplicationProblemPlusJSONResponse); !ok || r.Code != "assigning" {
		t.Errorf("trash of an assigned entry = %#v, want 409 with code assigning", held)
	}
}

func TestAChangeAnswersNotFoundAndBadRequest(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	env := setupSection(t)
	env.put(t, "Done/a.mkv", env.now.Add(-2*time.Minute))
	id := env.idOf(t, "Done")

	if r, err := env.section.TrashUnassignedEntry(ctx, httpapi.TrashUnassignedEntryRequestObject{Id: id + 100}); err != nil {
		t.Fatal(err)
	} else if _, ok := r.(httpapi.TrashUnassignedEntry404ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("trash of an unknown entry = %#v, want 404", r)
	}
	if r, err := env.section.TrashUnassignedFile(ctx, httpapi.TrashUnassignedFileRequestObject{Id: id, Body: &httpapi.TrashUnassignedFileJSONRequestBody{Path: "missing.mkv"}}); err != nil {
		t.Fatal(err)
	} else if _, ok := r.(httpapi.TrashUnassignedFile404ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("trash of a missing file = %#v, want 404", r)
	}
	if r, err := env.section.TrashUnassignedFile(ctx, httpapi.TrashUnassignedFileRequestObject{Id: id, Body: &httpapi.TrashUnassignedFileJSONRequestBody{Path: "../escape.mkv"}}); err != nil {
		t.Fatal(err)
	} else if _, ok := r.(httpapi.TrashUnassignedFile400ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("trash of a path outside the entry = %#v, want 400", r)
	}
	if r, err := env.section.DeleteUnassignedFile(ctx, httpapi.DeleteUnassignedFileRequestObject{Id: id}); err != nil {
		t.Fatal(err)
	} else if _, ok := r.(httpapi.DeleteUnassignedFile400ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("delete without a body = %#v, want 400", r)
	}
}

func TestTheSectionTrashesAndDeletesThroughTheContract(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	env := setupSection(t)
	env.put(t, "Done/a.mkv", env.now.Add(-2*time.Minute))
	env.put(t, "Done/b.mkv", env.now.Add(-2*time.Minute))
	id := env.idOf(t, "Done")

	if r, err := env.section.DeleteUnassignedFile(ctx, httpapi.DeleteUnassignedFileRequestObject{Id: id, Body: &httpapi.DeleteUnassignedFileJSONRequestBody{Path: "a.mkv"}}); err != nil {
		t.Fatal(err)
	} else if _, ok := r.(httpapi.DeleteUnassignedFile204Response); !ok {
		t.Fatalf("delete of a file = %#v, want 204", r)
	}
	if _, err := os.Stat(filepath.Join(env.root, domain.UnassignedDir, "Done", "a.mkv")); err == nil {
		t.Error("the deleted file is still on the disk")
	}
	if r, err := env.section.TrashUnassignedEntry(ctx, httpapi.TrashUnassignedEntryRequestObject{Id: id}); err != nil {
		t.Fatal(err)
	} else if _, ok := r.(httpapi.TrashUnassignedEntry204Response); !ok {
		t.Fatalf("trash of the entry = %#v, want 204", r)
	}
	got, err := env.section.GetUnassigned(ctx, httpapi.GetUnassignedRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	if entries := got.(httpapi.GetUnassigned200JSONResponse).Entries; entries == nil || len(entries) != 0 {
		t.Errorf("section after the trash = %#v, want an empty list (never null)", entries)
	}
}

// recordingArrivals records the order of the reads of a section and answers one job.
type recordingArrivals struct {
	calls []string
	jobs  []httpapi.UploadJob
	err   error
}

func (a *recordingArrivals) Adopt(context.Context) error {
	a.calls = append(a.calls, "adopt")
	return a.err
}

func (a *recordingArrivals) Jobs(context.Context) ([]httpapi.UploadJob, error) {
	a.calls = append(a.calls, "jobs")
	return a.jobs, nil
}

func TestTheSectionTakesTheSettledCopiesBeforeItReadsAndListsTheirJobs(t *testing.T) {
	t.Parallel()

	env := setupSection(t)
	arrivals := &recordingArrivals{jobs: []httpapi.UploadJob{{Id: "job-samba-1", FileName: "movie.zip"}}, err: errors.New("disk busy")}
	env.section = catalogshttp.NewSectionHandler(env.svc, arrivals, nil)

	resp, err := env.section.GetUnassigned(t.Context(), httpapi.GetUnassignedRequestObject{})
	if err != nil {
		t.Fatalf("GetUnassigned: %v", err)
	}
	got, ok := resp.(httpapi.GetUnassigned200JSONResponse)
	if !ok {
		t.Fatalf("answer = %T, want 200", resp)
	}
	if !slices.Equal(arrivals.calls, []string{"adopt", "jobs"}) {
		t.Fatalf("calls = %v, want the copies taken, then the jobs listed", arrivals.calls)
	}
	if len(got.Jobs) != 1 || got.Jobs[0].Id != "job-samba-1" {
		t.Fatalf("jobs = %+v, want the Samba job with its id", got.Jobs)
	}
	if got.Entries == nil {
		t.Fatal("entries are null, want an empty list")
	}
}
