package application_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/libraryfs"
)

// fixture is a library on disk with its journal and its catalog records in memory.
type fixture struct {
	root    string
	files   *libraryfs.FS
	journal *memJournal
	store   *memStore
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	store := &memStore{}
	return &fixture{root: root, files: libraryfs.New(root), store: store, journal: &memJournal{store: store}}
}

// engine returns an engine over the given files and journal, as a process would build it.
func engine(files application.Files, journal application.Journal) *application.Engine {
	now := time.Date(2026, time.October, 11, 10, 0, 0, 0, time.UTC)
	return application.NewEngine(files, journal, func() time.Time { return now }, nil)
}

// staged writes a file in the staging area of the fixture and returns its path.
func (f *fixture) staged(t *testing.T, name, content string) string {
	t.Helper()
	return writeFile(t, filepath.Join(f.root, ".videoexplorer", "staging", name), content)
}

func TestDoPerformsThePlanAndRecordsItOnce(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	src := f.staged(t, "a.mkv", "video")
	dst := filepath.Join(f.root, "Movies", "Movie", "a.mkv")
	recorded := 0

	err := engine(f.files, f.journal).Do(t.Context(), "test.move", "a", func(b *application.Builder) error {
		if err := b.Mkdir(filepath.Dir(dst)); err != nil {
			return err
		}
		if err := b.Move(src, dst); err != nil {
			return err
		}
		b.Record(func(application.Records) error {
			recorded++
			return nil
		})
		return nil
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got := readFile(t, dst); got != "video" {
		t.Errorf("moved content = %q", got)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("the source is still there: %v", err)
	}
	if recorded != 1 {
		t.Errorf("records written = %d, want 1", recorded)
	}
	if pending, _ := f.journal.Pending(t.Context()); len(pending) != 0 {
		t.Errorf("journal keeps %d operations after the commit", len(pending))
	}
}

func TestDoRefusesToOverwriteAndChangesNothing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T, f *fixture) (src, dst string)
	}{
		{name: "a file is at the target", setup: func(t *testing.T, f *fixture) (string, string) {
			src := f.staged(t, "a.mkv", "new")
			dst := writeFile(t, filepath.Join(f.root, "Movies", "a.mkv"), "old")
			return src, dst
		}},
		{name: "a folder is at the target", setup: func(t *testing.T, f *fixture) (string, string) {
			src := f.staged(t, "a.mkv", "new")
			dst := filepath.Join(f.root, "Movies", "a.mkv")
			writeFile(t, filepath.Join(dst, "inside.txt"), "kept")
			return src, dst
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			src, dst := tt.setup(t, f)
			err := engine(f.files, f.journal).Do(t.Context(), "test.move", "a", func(b *application.Builder) error {
				return b.Move(src, dst)
			})
			if !errors.Is(err, application.ErrTargetTaken) {
				t.Fatalf("Do = %v, want ErrTargetTaken", err)
			}
			if got := readFile(t, src); got != "new" {
				t.Errorf("the source changed: %q", got)
			}
			if _, err := os.Stat(dst); err != nil {
				t.Errorf("the target was touched: %v", err)
			}
			if pending, _ := f.journal.Pending(t.Context()); len(pending) != 0 {
				t.Errorf("a refused change was journaled: %+v", pending)
			}
		})
	}
}

func TestAFailedStepUndoesTheStepsBeforeIt(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	a := f.staged(t, "a.mkv", "one")
	b := f.staged(t, "b.mkv", "two")
	files := &flakyFiles{FS: f.files, failOn: "b.mkv"}
	unit := filepath.Join(f.root, "Series")

	err := engine(files, f.journal).Do(t.Context(), "test.move", "series", func(bd *application.Builder) error {
		if err := bd.Mkdir(unit); err != nil {
			return err
		}
		if err := bd.Move(a, filepath.Join(unit, "a.mkv")); err != nil {
			return err
		}
		return bd.Move(b, filepath.Join(unit, "b.mkv"))
	})
	if err == nil {
		t.Fatal("Do succeeded with a failing rename")
	}
	if errors.Is(err, application.ErrUndoFailed) {
		t.Fatalf("the undo failed: %v", err)
	}
	if readFile(t, a) != "one" || readFile(t, b) != "two" {
		t.Errorf("the staged files are not back: %v", libraryFiles(t, f.root))
	}
	if _, err := os.Stat(unit); !os.IsNotExist(err) {
		t.Errorf("the folder created by the change is still there: %v", err)
	}
	if pending, _ := f.journal.Pending(t.Context()); len(pending) != 0 {
		t.Errorf("journal keeps %d operations after the undo", len(pending))
	}
}

func TestAFailedCommitUndoesEverything(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	src := f.staged(t, "a.mkv", "video")
	dst := filepath.Join(f.root, "Movies", "a.mkv")
	f.store.failRecords = true

	err := engine(f.files, f.journal).Do(t.Context(), "test.move", "a", func(b *application.Builder) error {
		if err := b.Mkdir(filepath.Dir(dst)); err != nil {
			return err
		}
		if err := b.Move(src, dst); err != nil {
			return err
		}
		b.Record(func(r application.Records) error {
			_, err := r.InsertUnassigned(t.Context(), domain.UnassignedEntry{Source: "a", Name: "a", CreatedAt: time.Now()})
			return err
		})
		return nil
	})
	if err == nil {
		t.Fatal("Do succeeded with a failing record")
	}
	if readFile(t, src) != "video" {
		t.Errorf("the file did not go back to its source")
	}
	if got := libraryFiles(t, f.root); len(got) != 1 {
		t.Errorf("library files = %v, want only the staged file", got)
	}
	if f.store.count() != 0 {
		t.Errorf("a record was kept: %d entries", f.store.count())
	}
	if pending, _ := f.journal.Pending(t.Context()); len(pending) != 0 {
		t.Errorf("journal keeps %d operations after the undo", len(pending))
	}
}

func TestAFailedCommitIsUndoneAndForgotten(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	src := f.staged(t, "a.mkv", "video")
	dst := filepath.Join(f.root, "Movies", "a.mkv")
	f.journal.failCommit = true

	err := engine(f.files, f.journal).Do(t.Context(), "test.move", "a", func(b *application.Builder) error {
		if err := b.Mkdir(filepath.Dir(dst)); err != nil {
			return err
		}
		return b.Move(src, dst)
	})
	if err == nil || errors.Is(err, application.ErrUndoFailed) {
		t.Fatalf("Do = %v, want the commit failure, with the undo done", err)
	}
	if readFile(t, src) != "video" {
		t.Errorf("the file did not go back")
	}
	if _, err := os.Stat(filepath.Dir(dst)); !os.IsNotExist(err) {
		t.Errorf("the folder created by the change is still there: %v", err)
	}
	if pending, _ := f.journal.Pending(t.Context()); len(pending) != 0 {
		t.Errorf("journal keeps %d operations after the undo", len(pending))
	}
}

func TestACaseOnlyRenameGoesThroughAScratchName(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	old := writeFile(t, filepath.Join(f.root, "Movies", "Movie", "movie.mkv"), "video")
	next := filepath.Join(f.root, "Movies", "Movie", "Movie.mkv")

	err := engine(f.files, f.journal).Do(t.Context(), "test.rename", "movie", func(b *application.Builder) error {
		return b.Move(old, next)
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	names, err := f.files.Names(filepath.Dir(next))
	if err != nil || len(names) != 1 || names[0] != "Movie.mkv" {
		t.Fatalf("names after the rename = %v, %v; want Movie.mkv only", names, err)
	}
	if scratch, err := f.files.Names(f.files.OpsPath()); err != nil || len(scratch) != 0 {
		t.Errorf("the scratch area keeps %v (%v); want nothing", scratch, err)
	}
}

// TestACutAtAnyPointIsUndoneByRecover stops the process before each of the changes of a
// use case (journal writes, moves, folders, the commit), restarts it and checks that the
// library and the catalog are as they were before the change, or completely changed.
func TestACutAtAnyPointIsUndoneByRecover(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		// setup stages the content and returns the change to run and its check once it is complete.
		setup func(t *testing.T, f *fixture) (run func(eng *application.Engine, d *dying) error, complete func(t *testing.T, f *fixture))
		// staged lists what is in staging before the change, which an undo must put back.
		staged []string
	}{
		{name: "trash of an archive", staged: []string{".videoexplorer/staging/job.zip"}, setup: func(t *testing.T, f *fixture) (func(*application.Engine, *dying) error, func(*testing.T, *fixture)) {
			src := f.staged(t, "job.zip", "archive")
			run := func(eng *application.Engine, d *dying) error {
				files := dyingFiles{Files: f.files, d: d}
				trash := application.NewTrash(f.store, files, eng, nil)
				_, err := trash.Put(t.Context(), application.PutRequest{
					Origin: domain.TrashOriginUpload, Reason: domain.TrashReasonExtracted,
					BatchID: "b1", RelativePath: "Movie.zip", Name: "job.zip", Source: src, Size: 7,
				})
				return err
			}
			complete := func(t *testing.T, f *fixture) {
				t.Helper()
				if len(f.store.trash) != 1 {
					t.Errorf("trash entries = %d, want 1", len(f.store.trash))
				}
				if got := libraryFiles(t, f.root); len(got) != 1 || !strings.HasPrefix(got[0], ".videoexplorer/trash/") {
					t.Errorf("library files = %v, want only the trashed archive", got)
				}
			}
			return run, complete
		}},
		{name: "two files into an entry of No asignados", staged: []string{".videoexplorer/staging/a.mkv", ".videoexplorer/staging/b.srt"}, setup: func(t *testing.T, f *fixture) (func(*application.Engine, *dying) error, func(*testing.T, *fixture)) {
			a := f.staged(t, "a.mkv", "video")
			b := f.staged(t, "b.srt", "subs")
			run := func(eng *application.Engine, d *dying) error {
				files := dyingFiles{Files: f.files, d: d}
				section := application.NewUnassigned(f.store, files, eng, nil)
				_, err := section.Put(t.Context(), application.AsideRequest{
					Source: "batch-1", Name: "Series", BatchID: "b1", Relation: domain.RelationParts,
					Files: []application.AsideFile{{Source: a, Rel: "Season 01/a.mkv", Size: 5}, {Source: b, Rel: "Season 01/b.srt", Size: 4}},
				})
				return err
			}
			complete := func(t *testing.T, f *fixture) {
				t.Helper()
				if f.store.count() != 1 {
					t.Errorf("entries = %d, want 1", f.store.count())
				}
				if got := readFile(t, filepath.Join(f.root, "_unassigned", "Series", "Season 01", "a.mkv")); got != "video" {
					t.Errorf("the video in its entry = %q", got)
				}
			}
			return run, complete
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			completed := 0
			for allow := 0; allow < 40; allow++ {
				f := newFixture(t)
				run, complete := tc.setup(t, f)
				d := &dying{allow: allow}
				err := run(engine(dyingFiles{Files: f.files, d: d}, dyingJournal{Journal: f.journal, d: d}), d)

				// The process restarts: nothing is stopped now, and Recover runs first.
				restarted := engine(f.files, f.journal)
				if _, err := restarted.Recover(t.Context()); err != nil {
					t.Fatalf("cut after %d changes: Recover: %v", allow, err)
				}
				if pending, _ := f.journal.Pending(t.Context()); len(pending) != 0 {
					t.Fatalf("cut after %d changes: journal keeps %d operations after Recover", allow, len(pending))
				}

				if err == nil {
					completed++
					complete(t, f)
					break
				}
				if got := libraryFiles(t, f.root); !sameStrings(got, tc.staged) {
					t.Errorf("cut after %d changes: library files = %v, want the staged files %v", allow, got, tc.staged)
				}
				if f.store.count() != 0 || len(f.store.trash) != 0 {
					t.Errorf("cut after %d changes: a catalog record was written", allow)
				}
				for _, dir := range []string{"_unassigned", ".videoexplorer/trash", ".videoexplorer/ops"} {
					if _, err := os.Stat(filepath.Join(f.root, dir)); !os.IsNotExist(err) {
						t.Errorf("cut after %d changes: %s was left behind", allow, dir)
					}
				}
			}
			if completed == 0 {
				t.Fatal("the change never completed: the cuts did not reach its end")
			}
		})
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// halfMoveFiles simulates a move across file systems that stops after its copy is complete
// and before the source is deleted: both places hold the file.
type halfMoveFiles struct {
	application.Files
}

func (h halfMoveFiles) Move(from, to string) error {
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	if err := os.WriteFile(to, data, 0o600); err != nil {
		return err
	}
	return errDead
}

func TestARecoverKeepsTheJournalWhenBothPlacesOfAMoveExist(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	src := f.staged(t, "a.mkv", "video")
	dst := filepath.Join(f.root, "Movies", "a.mkv")

	files := halfMoveFiles{Files: f.files}
	err := engine(files, f.journal).Do(t.Context(), "test.move", "a", func(b *application.Builder) error {
		if err := b.Mkdir(filepath.Dir(dst)); err != nil {
			return err
		}
		return b.Move(src, dst)
	})
	if !errors.Is(err, application.ErrUndoFailed) {
		t.Fatalf("Do = %v, want ErrUndoFailed", err)
	}
	if _, err := engine(f.files, f.journal).Recover(t.Context()); err == nil {
		t.Fatal("Recover reported nothing for a change it could not undo")
	}
	if pending, _ := f.journal.Pending(t.Context()); len(pending) != 1 {
		t.Errorf("journal = %d operations, want the one that could not be undone", len(pending))
	}
	if readFile(t, src) != "video" || readFile(t, dst) != "video" {
		t.Errorf("a copy was deleted: the library holds %v", libraryFiles(t, f.root))
	}
}
