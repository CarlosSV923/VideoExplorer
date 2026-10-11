package application_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// trashFixture is a library with a trash, its engine and a file staged to send there.
func trashFixture(t *testing.T) (*fixture, *application.Trash, string) {
	t.Helper()
	f := newFixture(t)
	source := f.staged(t, "job1", "PK\x03\x04 the original archive")
	now := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)
	trash := application.NewTrash(f.store, f.files, engine(f.files, f.journal), func() time.Time { return now })
	return f, trash, source
}

func TestPutMovesTheFileIntoTheTrashWithItsMetadata(t *testing.T) {
	t.Parallel()

	f, trash, source := trashFixture(t)
	now := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)

	entry, err := trash.Put(t.Context(), application.PutRequest{
		Origin: domain.TrashOriginUpload, Reason: domain.TrashReasonExtracted,
		BatchID: "b1", RelativePath: "Folder/Movie.zip", Name: "Movie.zip", Source: source, Size: 25,
	})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the file is still at its source: %v", err)
	}
	moved := filepath.Join(f.root, ".videoexplorer", "trash", entry.Dir, "Movie.zip")
	if got := readFile(t, moved); got != "PK\x03\x04 the original archive" {
		t.Fatalf("trashed file = %q", got)
	}
	if entry.ID == 0 || len(f.store.trash) != 1 || f.store.trash[0].Dir != entry.Dir {
		t.Fatalf("entry not recorded: %+v", f.store.trash)
	}
	got := f.store.trash[0]
	if got.BatchID != "b1" || got.RelativePath != "Folder/Movie.zip" {
		t.Errorf("provenance = %q %q, want the batch and the relative path it was dropped with", got.BatchID, got.RelativePath)
	}
	if got.Origin != domain.TrashOriginUpload || got.Reason != domain.TrashReasonExtracted ||
		!got.TrashedAt.Equal(now) || len(got.Files) != 1 || got.Files[0] != (domain.TrashFile{Name: "Movie.zip", Size: 25}) {
		t.Errorf("metadata = %+v", got)
	}
	if pending, _ := f.journal.Pending(t.Context()); len(pending) != 0 {
		t.Errorf("journal keeps %d operations after the commit", len(pending))
	}
}

func TestPutRefusesWhatIsNotAValidEntryAndMovesNothing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  func(source string) application.PutRequest
	}{
		{name: "an unknown reason", req: func(s string) application.PutRequest {
			return application.PutRequest{Origin: domain.TrashOriginUpload, Reason: "deleted", BatchID: "b1", RelativePath: "Movie.zip", Name: "Movie.zip", Source: s}
		}},
		{name: "no batch", req: func(s string) application.PutRequest {
			return application.PutRequest{Origin: domain.TrashOriginUpload, Reason: domain.TrashReasonExtracted, RelativePath: "Movie.zip", Name: "Movie.zip", Source: s}
		}},
		{name: "a relative path that leaves the drop", req: func(s string) application.PutRequest {
			return application.PutRequest{Origin: domain.TrashOriginUpload, Reason: domain.TrashReasonFiltered, BatchID: "b1", RelativePath: "../Movie.zip", Name: "Movie.zip", Source: s}
		}},
		{name: "a file name that is a path", req: func(s string) application.PutRequest {
			return application.PutRequest{Origin: domain.TrashOriginUpload, Reason: domain.TrashReasonExtracted, BatchID: "b1", RelativePath: "Movie.zip", Name: "../Movie.zip", Source: s}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, trash, source := trashFixture(t)
			if _, err := trash.Put(t.Context(), tt.req(source)); !errors.Is(err, domain.ErrInvalidTrash) {
				t.Fatalf("err = %v, want ErrInvalidTrash", err)
			}
			if got := readFile(t, source); got == "" {
				t.Fatal("the source was touched")
			}
			if len(f.store.trash) != 0 {
				t.Fatalf("an entry was recorded: %+v", f.store.trash)
			}
		})
	}
}

// TestPutRefusesSourcesOutsideTheLibraryOrThroughALink covers the paths the file system
// refuses: a source outside the library, and one that goes through a symbolic link.
func TestPutRefusesSourcesOutsideTheLibraryOrThroughALink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source func(t *testing.T, f *fixture) string
	}{
		{name: "a source outside the library", source: func(t *testing.T, f *fixture) string {
			return writeFile(t, filepath.Join(t.TempDir(), "outside.zip"), "outside")
		}},
		{name: "a source through a symbolic link", source: func(t *testing.T, f *fixture) string {
			real := writeFile(t, filepath.Join(f.root, "real", "movie.zip"), "real")
			link := filepath.Join(f.root, "link")
			if err := os.Symlink(filepath.Dir(real), link); err != nil {
				t.Fatal(err)
			}
			return filepath.Join(link, "movie.zip")
		}},
		{name: "a source that is itself a link", source: func(t *testing.T, f *fixture) string {
			real := writeFile(t, filepath.Join(f.root, "real.zip"), "real")
			link := filepath.Join(f.root, "job.zip")
			if err := os.Symlink(real, link); err != nil {
				t.Fatal(err)
			}
			return link
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			source := tt.source(t, f)
			trash := application.NewTrash(f.store, f.files, engine(f.files, f.journal), nil)
			if _, err := trash.Put(t.Context(), application.PutRequest{
				Origin: domain.TrashOriginUpload, Reason: domain.TrashReasonExtracted, BatchID: "b1", RelativePath: "movie.zip", Name: "movie.zip", Source: source,
			}); err == nil {
				t.Fatal("Put accepted the source")
			}
			if len(f.store.trash) != 0 {
				t.Errorf("an entry was recorded")
			}
			if _, err := os.Lstat(source); err != nil {
				t.Errorf("the source was moved: %v", err)
			}
		})
	}
}

func TestPutGivesTheFileBackWhenTheEntryCannotBeRecorded(t *testing.T) {
	t.Parallel()

	f, trash, source := trashFixture(t)
	f.store.failRecords = true

	if _, err := trash.Put(t.Context(), application.PutRequest{
		Origin: domain.TrashOriginUpload, Reason: domain.TrashReasonExtracted, BatchID: "b1", RelativePath: "Folder/Movie.zip", Name: "Movie.zip", Source: source, Size: 25,
	}); err == nil {
		t.Fatal("Put succeeded without a recorded entry")
	}
	if got := readFile(t, source); got != "PK\x03\x04 the original archive" {
		t.Fatalf("the file did not go back to its source: %q", got)
	}
	if entries, _ := os.ReadDir(filepath.Join(f.root, ".videoexplorer", "trash")); len(entries) != 0 {
		t.Fatalf("the trash keeps a folder without its entry: %v", entries)
	}
}

func TestPutKeepsFilteredFilesWithTheReasonAndTheirDropPath(t *testing.T) {
	t.Parallel()

	f, trash, source := trashFixture(t)
	if _, err := trash.Put(t.Context(), application.PutRequest{
		Origin: domain.TrashOriginUpload, Reason: domain.TrashReasonFiltered,
		BatchID: "b7", RelativePath: "Movie/Samples/sample.mkv", Name: "sample.mkv", Source: source, Size: 25,
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got := f.store.trash[0]
	if got.Reason != domain.TrashReasonFiltered || got.BatchID != "b7" || got.RelativePath != "Movie/Samples/sample.mkv" {
		t.Errorf("entry = %+v, want the filtered reason with its batch and drop path", got)
	}
	if _, err := os.Stat(filepath.Join(f.root, ".videoexplorer", "trash", got.Dir, "sample.mkv")); err != nil {
		t.Errorf("the discarded file is not in its trash entry: %v", err)
	}
}

// TestConcurrentPutsEachKeepTheirOwnEntry sends many files at once: the engine runs the
// changes one at a time, so every file is trashed and recorded exactly once.
func TestConcurrentPutsEachKeepTheirOwnEntry(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	trash := application.NewTrash(f.store, f.files, engine(f.files, f.journal), nil)
	const n = 12
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := range n {
		source := f.staged(t, fmt.Sprintf("job%d", i), fmt.Sprintf("content %d", i))
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := trash.Put(t.Context(), application.PutRequest{
				Origin: domain.TrashOriginUpload, Reason: domain.TrashReasonFiltered, BatchID: "b1",
				RelativePath: fmt.Sprintf("f%d.mkv", i), Name: fmt.Sprintf("f%d.mkv", i), Source: source, Size: 1,
			})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Put: %v", err)
		}
	}
	if len(f.store.trash) != n {
		t.Fatalf("entries = %d, want %d", len(f.store.trash), n)
	}
	if pending, _ := f.journal.Pending(t.Context()); len(pending) != 0 {
		t.Errorf("journal keeps %d operations", len(pending))
	}
}
