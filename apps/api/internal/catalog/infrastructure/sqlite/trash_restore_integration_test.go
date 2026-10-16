package sqlite_test

import (
	"database/sql"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/libraryfs"
	catalogsqlite "github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/sqlite"
)

type restoreFixture struct {
	conn    *sql.DB
	root    string
	files   *libraryfs.FS
	engine  *application.Engine
	library *catalogsqlite.LibraryRepository
	trash   *application.Trash
	title   domain.LibraryTitle
	video   domain.LibraryFile
	sub     domain.LibraryFile
	entry   domain.TrashEntry
	clock   time.Time
}

func newRestoreFixture(t *testing.T, withSubtitle bool) *restoreFixture {
	t.Helper()
	conn := openDB(t)
	root := t.TempDir()
	for _, d := range []string{"Movies", "TV Shows", domain.UnassignedDir, ".videoexplorer/trash", ".videoexplorer/ops"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(d)), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	files := libraryfs.New(root)
	eng := application.NewEngine(files, catalogsqlite.NewJournal(conn), func() time.Time { return now }, nil)
	lib := catalogsqlite.NewLibraryRepository(conn)
	repo := catalogsqlite.NewTrashRepository(conn)
	title, err := domain.NewLibraryTitle(domain.KindMovie, "Dune", "Dune", 2021, 123, true, "Movies/Dune (2021) [tmdbid-123]", now)
	if err != nil {
		t.Fatal(err)
	}
	video, err := domain.NewVideo(domain.KindMovie, domain.VideoSpec{Rel: title.Folder + "/Dune (2021) [tmdbid-123].mkv", Size: 10}, now)
	if err != nil {
		t.Fatal(err)
	}
	var sub domain.LibraryFile
	rows := []domain.LibraryFile{video}
	if withSubtitle {
		sub, err = domain.NewSubtitle(domain.SubtitleSpec{Rel: title.Folder + "/Dune (2021) [tmdbid-123].en.srt", VideoRel: video.Rel, Language: "en", Size: 3}, now)
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, sub)
	}
	for _, row := range rows {
		p := filepath.Join(root, filepath.FromSlash(row.Rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		content := []byte(strings.Repeat("v", int(row.Size)))
		if err := os.WriteFile(p, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var titleID, videoID, subID int64
	if err := commitLibrary(t.Context(), conn, func(rec application.Records) error {
		var err error
		titleID, err = rec.InsertLibraryTitle(t.Context(), title)
		if err != nil {
			return err
		}
		if err = rec.InsertLibraryFiles(t.Context(), titleID, rows); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	title.ID = titleID
	gotFiles, err := lib.Files(t.Context(), titleID)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range gotFiles {
		if f.Kind == domain.FileVideo {
			videoID = f.ID
			video.ID = f.ID
			video.TitleID = f.TitleID
		} else {
			subID = f.ID
			sub.ID = f.ID
			sub.TitleID = f.TitleID
		}
	}
	entryDir := "entry-1"
	if err := os.Mkdir(files.TrashPath(entryDir), 0o750); err != nil {
		t.Fatal(err)
	}
	trashFiles := []domain.TrashFile{{Name: filepath.Base(video.Rel), Size: video.Size}}
	if withSubtitle {
		trashFiles = append(trashFiles, domain.TrashFile{Name: filepath.Base(sub.Rel), Size: sub.Size})
	}
	entry, err := domain.NewTrashEntryOf(domain.TrashOriginLibrary, domain.TrashReasonRemoved, entryDir, domain.Provenance{RelativePath: video.Rel}, trashFiles, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := commitLibrary(t.Context(), conn, func(rec application.Records) error {
		id, err := rec.InsertTrash(t.Context(), entry)
		if err != nil {
			return err
		}
		entry.ID = id
		ids := []int64{videoID}
		if withSubtitle {
			ids = append(ids, subID)
		}
		return rec.TrashLibraryFiles(t.Context(), ids, id)
	}); err != nil {
		t.Fatal(err)
	}
	for _, name := range trashFiles {
		source := filepath.Join(root, filepath.FromSlash(video.Rel))
		if strings.EqualFold(name.Name, filepath.Base(sub.Rel)) {
			source = filepath.Join(root, filepath.FromSlash(sub.Rel))
		}
		if err := os.Rename(source, files.TrashPath(entryDir, name.Name)); err != nil {
			t.Fatal(err)
		}
	}
	trash := application.NewTrash(repo, files, eng, func() time.Time { return now })
	trash.ConfigureRestore(application.TrashRestoreDeps{Repository: repo, Library: lib, Files: files, Engine: eng, Finder: application.NewDuplicateFinder(lib, files, nil, []string{".mkv"}), Roots: application.Roots{Movies: "Movies", Series: "TV Shows"}, SubtitleExts: []string{".srt"}, Now: func() time.Time { return now }})
	return &restoreFixture{conn: conn, root: root, files: files, engine: eng, library: lib, trash: trash, title: title, video: video, sub: sub, entry: entry, clock: now}
}

func TestTrashRestoreToOriginalAndRenamedLibraryTitle(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		rename bool
	}{{name: "same folder"}, {name: "renamed title", rename: true}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fx := newRestoreFixture(t, true)
			expected := fx.video.Rel
			if tc.rename {
				fx.title.DiscTitle = "Dune Recut"
				fx.title.Folder = "Movies/Dune Recut (2021) [tmdbid-123]"
				if err := commitLibrary(t.Context(), fx.conn, func(rec application.Records) error { return rec.SaveLibraryTitle(t.Context(), fx.title) }); err != nil {
					t.Fatal(err)
				}
				expected = "Movies/Dune Recut (2021) [tmdbid-123]/Dune Recut (2021) [tmdbid-123].mkv"
			}
			plan, err := fx.trash.PlanRestore(t.Context(), fx.entry.ID)
			if err != nil {
				t.Fatalf("preview: %v", err)
			}
			if len(plan.Files) != 1 || plan.Files[0].File != fx.video.Rel || plan.Files[0].Destinations[0] != expected {
				t.Fatalf("preview = %+v, want %s", plan, expected)
			}
			paths, err := fx.trash.Restore(t.Context(), fx.entry.ID, nil)
			if err != nil {
				t.Fatalf("restore: %v", err)
			}
			if len(paths) != 2 || paths[0] != expected {
				t.Fatalf("restored paths = %v", paths)
			}
			if _, err := os.Stat(filepath.Join(fx.root, filepath.FromSlash(expected))); err != nil {
				t.Fatalf("video not restored: %v", err)
			}
			rows, err := fx.library.Files(t.Context(), 1)
			if err != nil {
				t.Fatal(err)
			}
			var live int
			for _, f := range rows {
				if f.Live() {
					live++
				}
			}
			if live != 2 {
				t.Fatalf("live registry rows=%d, rows=%+v", live, rows)
			}
			entries, err := fx.trash.Entries(t.Context())
			if err != nil || len(entries) != 0 {
				t.Fatalf("trash after restore=%+v, %v", entries, err)
			}
		})
	}
}

func TestTrashRestoreConflictChoicesReplaceKeepBothAndCancelPerVideo(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		decision     domain.Decision
		version      string
		wantErr      error
		wantOldTrash bool
	}{{name: "replace", decision: domain.DecisionReplace, wantOldTrash: true}, {name: "keep both", decision: domain.DecisionKeepBoth, version: "Restored", wantOldTrash: false}, {name: "cancel", decision: domain.Decision("cancel"), wantErr: application.ErrTrashRestoreCancelled}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fx := newRestoreFixture(t, false)
			occupant := "Movies/Dune (2021) [tmdbid-123]/Dune (2021) [tmdbid-123] - BluRay.mkv"
			if err := os.WriteFile(filepath.Join(fx.root, filepath.FromSlash(occupant)), []byte("occupant has a different size"), 0o600); err != nil {
				t.Fatal(err)
			}
			plan, err := fx.trash.PlanRestore(t.Context(), fx.entry.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Files) != 1 || len(plan.Files[0].Duplicates) != 1 {
				t.Fatalf("plan did not show the video duplicate: %+v", plan)
			}
			paths, err := fx.trash.Restore(t.Context(), fx.entry.ID, []application.TrashRestoreDecision{{File: fx.video.Rel, Decision: tc.decision, Version: tc.version}})
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("restore err=%v want %v", err, tc.wantErr)
				}
				if _, statErr := os.Stat(fx.files.TrashPath(fx.entry.Dir, filepath.Base(fx.video.Rel))); statErr != nil {
					t.Fatalf("cancel moved source: %v", statErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("restore: %v", err)
			}
			if len(paths) == 0 {
				t.Fatal("no restored path")
			}
			if tc.wantOldTrash {
				entries, err := fx.trash.Entries(t.Context())
				if err != nil || len(entries) != 1 {
					t.Fatalf("replacement not recorded in trash: %+v, %v", entries, err)
				}
			} else if !strings.Contains(paths[0], "Restored") {
				t.Fatalf("keep-both path=%s", paths[0])
			}
		})
	}
}

func TestTrashRestoreUploadArchiveReturnsToUnassignedWithOccupiedFolderSuffix(t *testing.T) {
	t.Parallel()
	conn := openDB(t)
	root := t.TempDir()
	for _, dir := range []string{domain.UnassignedDir, ".videoexplorer/trash", ".videoexplorer/ops"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	files := libraryfs.New(root)
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	engine := application.NewEngine(files, catalogsqlite.NewJournal(conn), func() time.Time { return now }, nil)
	repo := catalogsqlite.NewTrashRepository(conn)
	lib := catalogsqlite.NewLibraryRepository(conn)
	const dir = "archive-entry"
	if err := os.Mkdir(files.TrashPath(dir), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.TrashPath(dir, "show.zip"), []byte("PK\x03\x04test"), 0o600); err != nil {
		t.Fatal(err)
	}
	entry, err := domain.NewTrashEntry(domain.TrashOriginUpload, domain.TrashReasonExtracted, dir, domain.Provenance{BatchID: "batch-1", RelativePath: "Pack/show.zip"}, domain.TrashFile{Name: "show.zip", Size: 8}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := commitLibrary(t.Context(), conn, func(rec application.Records) error { _, err := rec.InsertTrash(t.Context(), entry); return err }); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(files.UnassignedPath("Pack"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.UnassignedPath("Pack", "keep.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	trash := application.NewTrash(repo, files, engine, func() time.Time { return now })
	trash.ConfigureRestore(application.TrashRestoreDeps{Repository: repo, Library: lib, Files: files, Engine: engine, Now: func() time.Time { return now }})
	paths, err := trash.Restore(t.Context(), 1, nil)
	if err != nil {
		t.Fatalf("restore archive: %v", err)
	}
	want := path.Join(domain.UnassignedDir, "Pack (2)", "show.zip")
	if len(paths) != 1 || paths[0] != want {
		t.Fatalf("restored path = %v want %s", paths, want)
	}
	section := application.NewSection(application.SectionDeps{Unassigned: catalogsqlite.NewUnassignedRepository(conn), Files: files, Engine: engine, VideoExts: []string{".mkv"}, SubtitleExts: []string{".srt"}, Now: func() time.Time { return now }})
	entries, err := section.Sync(t.Context(), nil)
	if err != nil {
		t.Fatalf("scan restored archive as Samba: %v", err)
	}
	var found bool
	for _, e := range entries {
		if e.Name == "Pack (2)" {
			found = true
		}
	}
	if !found {
		t.Fatalf("restored archive was not adopted as a Samba entry: %+v", entries)
	}
}

func TestTrashRestoreUndoesMovedFilesWhenTheCatalogCommitFails(t *testing.T) {
	t.Parallel()
	fx := newRestoreFixture(t, true)
	// A stale registry path has no disk occupant, so the engine moves the video and subtitle
	// before SQLite refuses the unique path during its atomic record commit.
	if _, err := fx.conn.ExecContext(t.Context(), `INSERT INTO library_files (title_id,rel,kind,language,video_id,video_rel,size,created_at) VALUES (?,?,?,?,?,?,?,?)`, 1, fx.video.Rel, "subtitle", "en", fx.video.ID, fx.video.Rel, 1, fx.clock.UTC().Format("2006-01-02T15:04:05.000000000Z")); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.trash.Restore(t.Context(), fx.entry.ID, nil); err == nil {
		t.Fatal("restore succeeded with a conflicting catalog path")
	}
	for _, file := range fx.entry.Files {
		if _, err := os.Stat(fx.files.TrashPath(fx.entry.Dir, file.Name)); err != nil {
			t.Errorf("rollback did not return %s to trash: %v", file.Name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(fx.root, filepath.FromSlash(fx.video.Rel))); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("failed restore left video at target: %v", err)
	}
	entries, err := fx.trash.Entries(t.Context())
	if err != nil || len(entries) != 1 {
		t.Fatalf("failed commit changed trash records: %+v, %v", entries, err)
	}
}

func TestTrashedSubtitleRestoresAfterItsVideoRegistryRowWasDeleted(t *testing.T) {
	t.Parallel()
	fx := newRestoreFixture(t, false)
	// First put the video back, then trash only its subtitle so that an import can later
	// forget the video while preserving the subtitle's trash row.
	if err := os.MkdirAll(filepath.Dir(filepath.Join(fx.root, filepath.FromSlash(fx.video.Rel))), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(fx.files.TrashPath(fx.entry.Dir, filepath.Base(fx.video.Rel)), filepath.Join(fx.root, filepath.FromSlash(fx.video.Rel))); err != nil {
		t.Fatal(err)
	}
	video := fx.video
	video.TrashEntryID = 0
	video.TrashRel = ""
	if err := commitLibrary(t.Context(), fx.conn, func(rec application.Records) error {
		if err := rec.RestoreLibraryFile(t.Context(), video); err != nil {
			return err
		}
		return rec.DeleteTrashEntry(t.Context(), fx.entry.ID)
	}); err != nil {
		t.Fatal(err)
	}
	sub, err := domain.NewSubtitle(domain.SubtitleSpec{Rel: fx.video.Rel[:len(fx.video.Rel)-len(".mkv")] + ".en.srt", VideoRel: fx.video.Rel, Language: "en", Size: 3}, fx.clock)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fx.root, filepath.FromSlash(sub.Rel)), []byte("sub"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := commitLibrary(t.Context(), fx.conn, func(rec application.Records) error {
		return rec.InsertLibraryFiles(t.Context(), 1, []domain.LibraryFile{sub})
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := fx.library.Files(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	var subID int64
	for _, row := range rows {
		if row.Rel == sub.Rel {
			subID = row.ID
		}
	}
	if err := os.Mkdir(fx.files.TrashPath("subtitle-only"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(fx.root, filepath.FromSlash(sub.Rel)), fx.files.TrashPath("subtitle-only", filepath.Base(sub.Rel))); err != nil {
		t.Fatal(err)
	}
	if err := commitLibrary(t.Context(), fx.conn, func(rec application.Records) error {
		entry := domain.TrashEntry{Origin: domain.TrashOriginLibrary, Reason: domain.TrashReasonRemoved, Dir: "subtitle-only", TrashedAt: fx.clock, RelativePath: sub.Rel, Files: []domain.TrashFile{{Name: filepath.Base(sub.Rel), Size: 3}}}
		id, err := rec.InsertTrash(t.Context(), entry)
		if err != nil {
			return err
		}
		return rec.TrashLibraryFiles(t.Context(), []int64{subID}, id)
	}); err != nil {
		t.Fatal(err)
	}
	videoID := video.ID
	if err := commitLibrary(t.Context(), fx.conn, func(rec application.Records) error { return rec.DeleteLibraryFile(t.Context(), videoID) }); err != nil {
		t.Fatalf("forget video row: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fx.root, filepath.FromSlash(video.Rel))); err == nil {
		if err := os.Remove(filepath.Join(fx.root, filepath.FromSlash(video.Rel))); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := fx.trash.Entries(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var entryID int64
	for _, entry := range entries {
		if entry.Dir == "subtitle-only" {
			entryID = entry.ID
		}
	}
	plan, err := fx.trash.PlanRestore(t.Context(), entryID)
	if err != nil {
		t.Fatalf("subtitle restore preview: %v", err)
	}
	if len(plan.Files) != 1 || plan.Files[0].Destinations[0] != sub.Rel {
		t.Fatalf("subtitle restore plan = %+v", plan)
	}
	if _, err := fx.trash.Restore(t.Context(), entryID, nil); err != nil {
		t.Fatalf("restore subtitle: %v", err)
	}
	rows, err = fx.library.Files(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ID == subID {
			if !row.Live() || row.VideoID != videoID || row.VideoRel != video.Rel {
				t.Fatalf("restored dangling subtitle = %+v", row)
			}
			return
		}
	}
	t.Fatal("restored subtitle row disappeared")
}
