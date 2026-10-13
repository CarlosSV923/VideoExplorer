package sqlite_test

import (
	"database/sql"
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
)

// sectionEnv is a library with the real file system, the real database and a clock the test moves.
type sectionEnv struct {
	t        *testing.T
	root     string
	conn     *sql.DB
	files    *libraryfs.FS
	engine   *application.Engine
	repo     *catalogsqlite.UnassignedRepository
	journal  *catalogsqlite.Journal
	section  *application.Section
	assign   *application.Assignments
	now      time.Time
	unassign *application.Unassigned
}

func newSectionEnv(t *testing.T) *sectionEnv {
	t.Helper()
	e := &sectionEnv{t: t, root: t.TempDir(), now: time.Date(2026, time.October, 13, 9, 0, 0, 0, time.UTC)}
	for _, dir := range []string{"Movies", "TV Shows", domain.UnassignedDir, ".videoexplorer/staging"} {
		if err := os.MkdirAll(filepath.Join(e.root, dir), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	e.conn = openDB(t)
	clock := func() time.Time { return e.now }
	e.files = libraryfs.New(e.root)
	e.journal = catalogsqlite.NewJournal(e.conn)
	e.engine = application.NewEngine(e.files, e.journal, clock, nil)
	e.repo = catalogsqlite.NewUnassignedRepository(e.conn)
	libraryRepo := catalogsqlite.NewLibraryRepository(e.conn)
	e.section = application.NewSection(application.SectionDeps{
		Unassigned: e.repo, Files: e.files, Engine: e.engine,
		VideoExts: []string{".mkv"}, SubtitleExts: []string{".srt"}, Now: clock,
	})
	e.unassign = application.NewUnassigned(e.repo, e.files, e.engine, clock)
	e.assign = application.NewAssignments(application.AssignDeps{
		Unassigned: e.repo, Library: libraryRepo, Files: e.files, Engine: e.engine,
		Finder: application.NewDuplicateFinder(libraryRepo, e.files, contentPartialHash{}, []string{".mkv"}),
		TMDB:   oneMovieTMDB{}, Names: noNames{}, Roots: application.Roots{Movies: "Movies", Series: "TV Shows"},
		VideoExts: []string{".mkv"}, SubtitleExts: []string{".srt"}, Now: clock,
		NewToken: func() string { return "tab-a" },
	})
	return e
}

// put writes a file in _unassigned/ with the given modification time, as Samba or an earlier change leaves it.
func (e *sectionEnv) put(rel, content string, mtime time.Time) {
	e.t.Helper()
	p := filepath.Join(e.root, domain.UnassignedDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		e.t.Fatal(err)
	}
	if err := os.Chtimes(p, mtime, mtime); err != nil {
		e.t.Fatal(err)
	}
}

// settled writes a file that was last modified two minutes ago (RF-27: it is not being copied).
func (e *sectionEnv) settled(rel, content string) {
	e.put(rel, content, e.now.Add(-2*time.Minute))
}

// stagedUpload puts a file in staging, as an upload leaves it, settled, and returns its path.
func (e *sectionEnv) stagedUpload(name, content string) string {
	e.t.Helper()
	p := filepath.Join(e.root, ".videoexplorer", "staging", name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		e.t.Fatal(err)
	}
	if err := os.Chtimes(p, e.now.Add(-2*time.Minute), e.now.Add(-2*time.Minute)); err != nil {
		e.t.Fatal(err)
	}
	return p
}

// exists reports whether a path inside _unassigned/ is on the disk.
func (e *sectionEnv) exists(rel string) bool {
	e.t.Helper()
	_, err := os.Lstat(filepath.Join(e.root, domain.UnassignedDir, filepath.FromSlash(rel)))
	return err == nil
}

// list reads the section now.
func (e *sectionEnv) list() []application.SectionEntry {
	e.t.Helper()
	got, err := e.section.List(e.t.Context())
	if err != nil {
		e.t.Fatalf("List: %v", err)
	}
	return got
}

// entry returns the entry called name, or the zero entry when the section does not list one.
func entry(entries []application.SectionEntry, name string) application.SectionEntry {
	for _, e := range entries {
		if e.Name == name {
			return e
		}
	}
	return application.SectionEntry{}
}

// filePaths returns the paths of the files of an entry, in order.
func filePaths(e application.SectionEntry) []string {
	var out []string
	for _, f := range e.Files {
		out = append(out, f.Path)
	}
	return out
}

// trashed returns the trash entries, oldest first.
func (e *sectionEnv) trashed() []domain.TrashEntry {
	e.t.Helper()
	got, err := catalogsqlite.NewTrashRepository(e.conn).Entries(e.t.Context())
	if err != nil {
		e.t.Fatalf("trash Entries: %v", err)
	}
	return got
}

// journalEmpty reports whether no change stays in the journal, and the scratch folder is empty.
func (e *sectionEnv) journalEmpty() {
	e.t.Helper()
	pending, err := e.journal.Pending(e.t.Context())
	if err != nil || len(pending) != 0 {
		e.t.Errorf("journal: %d pending, %v", len(pending), err)
	}
	names, err := os.ReadDir(filepath.Join(e.root, ".videoexplorer", "ops"))
	if err == nil && len(names) != 0 {
		e.t.Errorf("scratch folder keeps %d entries", len(names))
	}
}

func TestTheSectionListsUploadsAndSambaCopies(t *testing.T) {
	t.Parallel()
	e := newSectionEnv(t)
	ctx := t.Context()

	// An upload the ingestion moved in (RF-08a): a folder with two files, in a subfolder.
	src1 := e.stagedUpload("show-e1.mkv", "episode one")
	src2 := e.stagedUpload("show-e1.es.srt", "sub")
	if _, err := e.unassign.Put(ctx, application.AsideRequest{
		Source: "batch-1", Name: "Show Pack", BatchID: "b1", Relation: domain.RelationParts,
		Files: []application.AsideFile{{Source: src1, Rel: "Season 01/e1.mkv", Size: 11}, {Source: src2, Rel: "Season 01/e1.es.srt", Size: 3}},
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	// A folder and a loose video that Samba copied, with junk in them (RF-27, RF-28).
	e.settled("Copied/a.mkv", "aaaa")
	e.settled("Copied/sub/b.mkv", "bb")
	e.settled("Copied/Thumbs.db", "junk")
	e.settled("Copied/.DS_Store", "junk")
	e.settled("Copied/@eaDir/c.jpg", "thumb")
	e.settled("Movie.mkv", "loose")
	e.settled("Thumbs.db", "junk")
	e.settled("@eaDir/x.jpg", "junk")

	got := e.list()
	var names []string
	for _, s := range got {
		names = append(names, s.Name)
	}
	if want := []string{"Copied", "Movie.mkv", "Show Pack"}; !slices.Equal(names, want) {
		t.Fatalf("entries = %v, want %v", names, want)
	}

	copied := entry(got, "Copied")
	if copied.Origin != domain.OriginSamba || copied.Relation != domain.RelationNone || copied.Loose || copied.Status != domain.StatusReady {
		t.Errorf("Copied = %+v, want a samba folder with no relation, ready", copied)
	}
	if want := []string{"a.mkv", "sub/b.mkv"}; !slices.Equal(filePaths(copied), want) || copied.Size != 6 {
		t.Errorf("Copied files = %v (size %d), want %v (size 6): junk is not listed nor counted", filePaths(copied), copied.Size, want)
	}
	movie := entry(got, "Movie.mkv")
	if !movie.Loose || movie.Origin != domain.OriginSamba || !slices.Equal(filePaths(movie), []string{""}) || movie.Size != 5 {
		t.Errorf("Movie.mkv = %+v, want a loose samba file of 5 bytes", movie)
	}
	pack := entry(got, "Show Pack")
	if pack.Origin != domain.OriginUpload || pack.Relation != domain.RelationParts || pack.Size != 14 {
		t.Errorf("Show Pack = %+v, want an upload with the batch's relation and 14 bytes", pack)
	}
	if want := []string{"Season 01/e1.es.srt", "Season 01/e1.mkv"}; !slices.Equal(filePaths(pack), want) {
		t.Errorf("Show Pack files = %v, want %v", filePaths(pack), want)
	}

	// The Samba entries are in the registry, with their origin (RF-27).
	rows, err := e.repo.List(ctx)
	if err != nil || len(rows) != 3 {
		t.Fatalf("registry = %d entries, %v; want 3", len(rows), err)
	}
	for _, r := range rows {
		if r.Name == "Copied" && (r.Origin != domain.OriginSamba || r.Relation != domain.RelationNone) {
			t.Errorf("registry entry %s = origin %q, relation %q", r.Name, r.Origin, r.Relation)
		}
	}
}

func TestTheSectionCallsAFileCopyingBySizeTimeAndSettling(t *testing.T) {
	t.Parallel()

	t.Run("a file modified less than a minute ago is copying, and settles without changes", func(t *testing.T) {
		t.Parallel()
		e := newSectionEnv(t)
		e.put("Drop/movie.mkv", "half", e.now.Add(-30*time.Second))

		got := entry(e.list(), "Drop")
		if got.Status != domain.StatusCopying || !got.Files[0].Copying {
			t.Fatalf("right after the copy: %+v, want copying", got)
		}
		e.now = e.now.Add(61 * time.Second)
		got = entry(e.list(), "Drop")
		if got.Status != domain.StatusReady || got.Files[0].Copying {
			t.Errorf("a minute later, unchanged: %+v, want ready", got)
		}
	})

	t.Run("a file whose size changes between two readings is copying", func(t *testing.T) {
		t.Parallel()
		e := newSectionEnv(t)
		e.settled("Drop/movie.mkv", "1234")
		if got := entry(e.list(), "Drop"); got.Status != domain.StatusReady {
			t.Fatalf("first reading: %+v, want ready", got)
		}
		// A copy that keeps the old modification time, as a copy that preserves the date does.
		e.put("Drop/movie.mkv", "12345678", e.now.Add(-2*time.Minute))
		if got := entry(e.list(), "Drop"); got.Status != domain.StatusCopying || !got.Files[0].Copying {
			t.Errorf("size changed: %+v, want copying", got)
		}
		if got := entry(e.list(), "Drop"); got.Status != domain.StatusReady {
			t.Errorf("the next reading, unchanged: %+v, want ready", got)
		}
	})

	t.Run("a file whose modification time changes between two readings is copying", func(t *testing.T) {
		t.Parallel()
		e := newSectionEnv(t)
		e.settled("Drop/movie.mkv", "1234")
		e.list()
		e.put("Drop/movie.mkv", "1234", e.now.Add(-90*time.Second))
		if got := entry(e.list(), "Drop"); got.Status != domain.StatusCopying {
			t.Errorf("mtime changed: %+v, want copying", got)
		}
	})

	t.Run("the reading survives a restart of the section", func(t *testing.T) {
		t.Parallel()
		e := newSectionEnv(t)
		e.settled("Drop/movie.mkv", "1234")
		e.list()
		e.put("Drop/movie.mkv", "12345678", e.now.Add(-2*time.Minute))
		// A new section over the same database, as after a restart: the reading in SQLite is what it compares with.
		restarted := application.NewSection(application.SectionDeps{
			Unassigned: catalogsqlite.NewUnassignedRepository(e.conn), Files: e.files, Engine: e.engine,
			VideoExts: []string{".mkv"}, SubtitleExts: []string{".srt"}, Now: func() time.Time { return e.now },
		})
		got, err := restarted.List(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if g := entry(got, "Drop"); g.Status != domain.StatusCopying {
			t.Errorf("after a restart: %+v, want copying", g)
		}
	})
}

func TestTheSectionForgetsAnEntryThatLeftTheDisk(t *testing.T) {
	t.Parallel()
	e := newSectionEnv(t)
	e.settled("Gone/movie.mkv", "x")
	if got := e.list(); len(got) != 1 {
		t.Fatalf("entries = %d, want 1", len(got))
	}
	if err := os.RemoveAll(filepath.Join(e.root, domain.UnassignedDir, "Gone")); err != nil {
		t.Fatal(err)
	}
	if got := e.list(); len(got) != 0 {
		t.Errorf("entries after the folder was removed = %+v, want none", got)
	}
	if rows, err := e.repo.List(t.Context()); err != nil || len(rows) != 0 {
		t.Errorf("registry = %d entries, %v; want none", len(rows), err)
	}
}

func TestCopyingAndAssignedEntriesRefuseEveryAction(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	t.Run("an entry being copied refuses trash, deletion and assignment", func(t *testing.T) {
		t.Parallel()
		e := newSectionEnv(t)
		e.put("Copying/movie.mkv", "half", e.now.Add(-5*time.Second))
		e.list()
		id := heldID(t, e, "Copying")
		if err := e.section.TrashEntry(ctx, id); !errors.Is(err, application.ErrCopying) {
			t.Errorf("TrashEntry = %v, want ErrCopying", err)
		}
		if err := e.section.DeleteFile(ctx, id, "movie.mkv"); !errors.Is(err, application.ErrCopying) {
			t.Errorf("DeleteFile = %v, want ErrCopying", err)
		}
		if _, err := e.assign.Take(ctx, id, ""); !errors.Is(err, application.ErrCopying) {
			t.Errorf("Take = %v, want ErrCopying", err)
		}
		if !e.exists("Copying/movie.mkv") {
			t.Error("the file being copied was touched")
		}
		// A minute without changes: the file has settled, and the same actions go on.
		e.now = e.now.Add(time.Minute)
		if err := e.section.TrashEntry(ctx, id); err != nil {
			t.Errorf("TrashEntry once settled: %v", err)
		}
	})

	t.Run("an entry held by an assignment refuses trash and deletion", func(t *testing.T) {
		t.Parallel()
		e := newSectionEnv(t)
		e.settled("Held/movie.mkv", "x")
		entries := e.list()
		held := entry(entries, "Held")
		if _, err := e.assign.Take(ctx, heldID(t, e, held.Name), ""); err != nil {
			t.Fatalf("Take: %v", err)
		}
		if err := e.section.TrashEntry(ctx, heldID(t, e, held.Name)); !errors.Is(err, application.ErrAssigning) {
			t.Errorf("TrashEntry = %v, want ErrAssigning", err)
		}
		if err := e.section.DeleteEntry(ctx, heldID(t, e, held.Name)); !errors.Is(err, application.ErrAssigning) {
			t.Errorf("DeleteEntry = %v, want ErrAssigning", err)
		}
		if !e.exists("Held/movie.mkv") {
			t.Error("the held entry was touched")
		}
	})

	t.Run("an entry that does not exist is not found", func(t *testing.T) {
		t.Parallel()
		e := newSectionEnv(t)
		if err := e.section.TrashEntry(ctx, 99); !errors.Is(err, application.ErrEntryNotFound) {
			t.Errorf("TrashEntry = %v, want ErrEntryNotFound", err)
		}
		if err := e.section.DeleteFile(ctx, 99, "x.mkv"); !errors.Is(err, application.ErrEntryNotFound) {
			t.Errorf("DeleteFile = %v, want ErrEntryNotFound", err)
		}
	})
}

// heldID returns the id of the section entry called name.
func heldID(t *testing.T, e *sectionEnv, name string) int64 {
	t.Helper()
	rows, err := e.repo.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Name == name {
			return r.ID
		}
	}
	t.Fatalf("no entry %s in the registry", name)
	return 0
}

func TestTrashingAnEntryKeepsItsFilesAndDropsItsJunk(t *testing.T) {
	t.Parallel()
	e := newSectionEnv(t)
	e.settled("Show/S01/e1.mkv", "episode")
	e.settled("Show/S01/e1.es.srt", "sub")
	e.settled("Show/S01/other.mkv", "other")
	e.settled("Show/Thumbs.db", "junk")
	e.list()

	id := heldID(t, e, "Show")
	if err := e.section.TrashEntry(t.Context(), id); err != nil {
		t.Fatalf("TrashEntry: %v", err)
	}
	if e.exists("Show") {
		t.Error("the folder is still on the disk")
	}
	if rows, _ := e.repo.List(t.Context()); len(rows) != 0 {
		t.Errorf("registry after the trash = %d entries, want none", len(rows))
	}

	trash := e.trashed()
	if len(trash) != 3 {
		t.Fatalf("trash entries = %d, want 3 (one per file, junk not among them)", len(trash))
	}
	var rels []string
	for _, tr := range trash {
		if tr.Origin != domain.TrashOriginSamba || tr.Reason != domain.TrashReasonRemoved || tr.BatchID != "" {
			t.Errorf("trash entry = origin %q, reason %q, batch %q; want samba, removed, no batch", tr.Origin, tr.Reason, tr.BatchID)
		}
		rels = append(rels, tr.RelativePath)
		content, err := os.ReadFile(e.files.TrashPath(tr.Dir, tr.Files[0].Name))
		if err != nil || len(content) == 0 {
			t.Errorf("file of %s is not in the trash: %v", tr.RelativePath, err)
		}
	}
	slices.Sort(rels)
	if want := []string{"Show/S01/e1.es.srt", "Show/S01/e1.mkv", "Show/S01/other.mkv"}; !slices.Equal(rels, want) {
		t.Errorf("trashed paths = %v, want %v", rels, want)
	}
	e.journalEmpty()
}

func TestTrashingAVideoSendsItsSubtitlesAlong(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	e := newSectionEnv(t)
	src1 := e.stagedUpload("m.mkv", "movie")
	src2 := e.stagedUpload("m.es.srt", "sub")
	src3 := e.stagedUpload("other.mkv", "other")
	if _, err := e.unassign.Put(ctx, application.AsideRequest{
		Source: "batch-1", Name: "Movie Pack", BatchID: "b1", Relation: domain.RelationParts,
		Files: []application.AsideFile{
			{Source: src1, Rel: "Movie.mkv", Size: 5}, {Source: src2, Rel: "Movie.es.srt", Size: 3}, {Source: src3, Rel: "Other.mkv", Size: 5},
		},
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if err := e.section.TrashFile(ctx, heldID(t, e, "Movie Pack"), "Movie.mkv"); err != nil {
		t.Fatalf("TrashFile: %v", err)
	}
	if e.exists("Movie Pack/Movie.mkv") || e.exists("Movie Pack/Movie.es.srt") {
		t.Error("the video or its subtitle is still in the entry")
	}
	if !e.exists("Movie Pack/Other.mkv") {
		t.Error("the other video was touched")
	}
	trash := e.trashed()
	if len(trash) != 2 {
		t.Fatalf("trash entries = %d, want the video and its subtitle", len(trash))
	}
	for _, tr := range trash {
		if tr.Origin != domain.TrashOriginUpload || tr.BatchID != "b1" {
			t.Errorf("trash entry = origin %q, batch %q; want the upload and its batch", tr.Origin, tr.BatchID)
		}
	}
	got := entry(e.list(), "Movie Pack")
	if got.Status != domain.StatusReady || !slices.Equal(filePaths(got), []string{"Other.mkv"}) {
		t.Errorf("entry after the trash = %+v, want only Other.mkv", got)
	}
	if err := e.section.TrashFile(ctx, heldID(t, e, "Movie Pack"), "Missing.mkv"); !errors.Is(err, application.ErrFileNotFound) {
		t.Errorf("TrashFile of a missing file = %v, want ErrFileNotFound", err)
	}
	if err := e.section.TrashFile(ctx, heldID(t, e, "Movie Pack"), ""); !errors.Is(err, application.ErrInvalidPath) {
		t.Errorf("TrashFile with no path = %v, want ErrInvalidPath", err)
	}
}

func TestDeletingForGoodLeavesNothingBehind(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	e := newSectionEnv(t)
	e.settled("Drop/a.mkv", "aaaa")
	e.settled("Drop/b.mkv", "bbbb")
	e.settled("Drop/Thumbs.db", "junk")
	e.settled("Loose.mkv", "loose")
	e.list()

	if err := e.section.DeleteFile(ctx, heldID(t, e, "Drop"), "a.mkv"); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	if e.exists("Drop/a.mkv") {
		t.Error("the deleted file is still on the disk")
	}
	if len(e.trashed()) != 0 {
		t.Error("a file deleted for good went to the trash")
	}
	if got := entry(e.list(), "Drop"); !slices.Equal(filePaths(got), []string{"b.mkv"}) {
		t.Errorf("entry after the deletion = %+v, want b.mkv only", got)
	}

	if err := e.section.DeleteEntry(ctx, heldID(t, e, "Drop")); err != nil {
		t.Fatalf("DeleteEntry: %v", err)
	}
	if e.exists("Drop") {
		t.Error("the deleted entry is still on the disk")
	}
	if err := e.section.DeleteEntry(ctx, heldID(t, e, "Loose.mkv")); err != nil {
		t.Fatalf("DeleteEntry of a loose file: %v", err)
	}
	if e.exists("Loose.mkv") || len(e.list()) != 0 {
		t.Error("the loose file or the section still has something")
	}
	if len(e.trashed()) != 0 {
		t.Error("the deleted entries went to the trash")
	}
	e.journalEmpty()
}

func TestAssigningAnEntryWithJunkInItRemovesTheJunk(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	e := newSectionEnv(t)
	src := e.stagedUpload("Iron.Man.2008.1080p.mkv", "iron man")
	if _, err := e.unassign.Put(ctx, application.AsideRequest{
		Source: "batch-2", Name: "Iron Man Pack", BatchID: "b2", Relation: domain.RelationParts,
		Files: []application.AsideFile{{Source: src, Rel: "Iron.Man.2008.1080p.mkv", Size: 8}},
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	// Windows leaves a Thumbs.db in the folder once it has been opened: it used to block the commit.
	e.settled("Iron Man Pack/Thumbs.db", "junk")
	id := heldID(t, e, "Iron Man Pack")
	if _, err := e.section.List(ctx); err != nil {
		t.Fatal(err)
	}

	lock, err := e.assign.Take(ctx, id, "")
	if err != nil {
		t.Fatalf("Take: %v", err)
	}
	req := application.Request{Videos: []application.VideoChoice{
		{File: "Iron.Man.2008.1080p.mkv", Title: application.TitleChoice{Source: "tmdb", TMDBID: 1726, Kind: domain.KindMovie}},
	}}
	plan, err := e.assign.Plan(ctx, id, lock.Token, req)
	if err != nil || !plan.Valid {
		t.Fatalf("Plan: valid=%v err=%v errors=%+v", plan.Valid, err, plan.Errors)
	}
	res, err := e.assign.Commit(ctx, id, lock.Token, req)
	if err != nil {
		t.Fatalf("Commit with a Thumbs.db in the entry: %v", err)
	}
	if !res.EntryRemoved || len(res.Placed) != 1 {
		t.Errorf("result = %+v, want the entry removed and one video placed", res)
	}
	if e.exists("Iron Man Pack") {
		t.Error("the entry folder, junk included, is still in _unassigned/")
	}
	video := filepath.Join(e.root, "Movies", "Iron Man (2008) [tmdbid-1726]", "Iron Man (2008) [tmdbid-1726].mkv")
	if _, err := os.Stat(video); err != nil {
		t.Errorf("the video is not in the library: %v", err)
	}
	if len(e.trashed()) != 0 {
		t.Error("the junk went to the trash; it must be deleted without it")
	}
	e.journalEmpty()
}

func TestAssigningKeepsAFolderThatStillHoldsJunk(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	e := newSectionEnv(t)
	src1 := e.stagedUpload("disc1.mkv", "one")
	src2 := e.stagedUpload("disc2.mkv", "two")
	if _, err := e.unassign.Put(ctx, application.AsideRequest{
		Source: "batch-3", Name: "Iron Man Pack", BatchID: "b3", Relation: domain.RelationParts,
		Files: []application.AsideFile{
			{Source: src1, Rel: "Disc 1/Iron.Man.2008.mkv", Size: 3}, {Source: src2, Rel: "Disc 2/Iron.Man.2008.mkv", Size: 3},
		},
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	e.settled("Iron Man Pack/Disc 1/Thumbs.db", "junk")
	id := heldID(t, e, "Iron Man Pack")

	lock, err := e.assign.Take(ctx, id, "")
	if err != nil {
		t.Fatalf("Take: %v", err)
	}
	req := application.Request{Videos: []application.VideoChoice{
		{File: "Disc 1/Iron.Man.2008.mkv", Title: application.TitleChoice{Source: "tmdb", TMDBID: 1726, Kind: domain.KindMovie}},
		{File: "Disc 2/Iron.Man.2008.mkv", Keep: true},
	}}
	res, err := e.assign.Commit(ctx, id, lock.Token, req)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if res.EntryRemoved {
		t.Error("the entry was removed while a file stays in it")
	}
	if !e.exists("Iron Man Pack/Disc 1/Thumbs.db") {
		t.Error("the junk in a folder of a kept entry was deleted")
	}
	if !e.exists("Iron Man Pack/Disc 2/Iron.Man.2008.mkv") {
		t.Error("the file that stays in the entry is gone")
	}
}
