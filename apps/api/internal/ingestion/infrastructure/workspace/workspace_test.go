package workspace_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/workspace"
)

func TestPathsOfAJobAreInsideStaging(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	area := workspace.New(root, nil)
	tests := []struct {
		name string
		id   domain.JobID
		want string
	}{
		{name: "archive is the upload file", id: "3kQ7Zr1xVb8mNcT2pL0aWq", want: filepath.Join(root, "3kQ7Zr1xVb8mNcT2pL0aWq")},
		{name: "a path in the id is reduced to its last part", id: "../../etc", want: filepath.Join(root, "etc")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := area.ArchivePath(tt.id); got != tt.want {
				t.Errorf("ArchivePath = %q, want %q", got, tt.want)
			}
		})
	}
	if got, want := area.ExtractionDir("job1"), filepath.Join(root, "extract", "job1"); got != want {
		t.Errorf("ExtractionDir = %q, want %q", got, want)
	}
}

func TestExtractionFolderIsResetAndRemoved(t *testing.T) {
	t.Parallel()

	area := workspace.New(t.TempDir(), nil)
	const id = domain.JobID("job1")
	if err := area.ResetExtraction(id); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(area.ExtractionDir(id), "old.mkv")
	if err := os.WriteFile(stale, []byte("left by a failed attempt"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := area.ResetExtraction(id); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reset kept the files of a failed attempt: %v", err)
	}

	if err := area.RemoveExtraction(id); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(area.ExtractionDir(id)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove kept the folder: %v", err)
	}
	if err := area.RemoveExtraction(id); err != nil {
		t.Fatalf("removing a folder that is not there: %v", err)
	}
}

func TestCheckExtractionRefusesLinks(t *testing.T) {
	t.Parallel()

	area := workspace.New(t.TempDir(), nil)
	const id = domain.JobID("job1")
	if err := area.ResetExtraction(id); err != nil {
		t.Fatal(err)
	}
	dir := area.ExtractionDir(id)
	if err := os.MkdirAll(filepath.Join(dir, "Movie"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Movie", "Movie.mkv"), []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := area.CheckExtraction(id); err != nil {
		t.Fatalf("regular files and folders refused: %v", err)
	}

	if err := os.Symlink("/etc/hostname", filepath.Join(dir, "Movie", "link.mkv")); err != nil {
		t.Skipf("symbolic links cannot be created here: %v", err)
	}
	if err := area.CheckExtraction(id); !errors.Is(err, application.ErrUnsafeEntry) {
		t.Fatalf("symlink: err = %v, want ErrUnsafeEntry", err)
	}
}

func TestArchiveExistsOnlyForAStagedFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	area := workspace.New(root, nil)
	if area.ArchiveExists("job1") {
		t.Fatal("an upload that is not staged exists")
	}
	if err := os.Mkdir(filepath.Join(root, "job2"), 0o750); err != nil {
		t.Fatal(err)
	}
	if area.ArchiveExists("job2") {
		t.Fatal("a folder counts as an archive")
	}
	if err := os.WriteFile(filepath.Join(root, "job1"), []byte("PK\x03\x04"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !area.ArchiveExists("job1") {
		t.Fatal("a staged file does not exist")
	}
}

func TestReadHeaderReturnsWhatTheFileHolds(t *testing.T) {
	t.Parallel()

	area := workspace.New(t.TempDir(), nil)
	p := filepath.Join(t.TempDir(), "short")
	if err := os.WriteFile(p, []byte("PK"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := area.ReadHeader(p, 8)
	if err != nil || string(got) != "PK" {
		t.Fatalf("ReadHeader = %q, %v; want the two bytes the file holds", got, err)
	}
	if _, err := area.ReadHeader(filepath.Join(t.TempDir(), "missing"), 8); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file: err = %v", err)
	}
}

func TestFreeSpaceMeasuresTheStagingArea(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	var measured string
	area := workspace.New(root, func(path string) (uint64, error) {
		measured = path
		return 42, nil
	})
	free, err := area.FreeSpace()
	if err != nil || free != 42 || measured != root {
		t.Fatalf("FreeSpace = %d, %v (measured %q); want 42 of %q", free, err, measured, root)
	}
}
