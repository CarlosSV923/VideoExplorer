package tus_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/tus"
)

// The extraction works in folders of the staging area (phase 4). Those folders
// are not uploads: the staging listing must not take them for staged uploads,
// or the purge would try to remove them as orphans.
func TestIDsSkipsTheFoldersOfTheExtraction(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a, err := tus.New(dir, uploadsPath, nil, nil, &fakeUploads{})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"job1", "job1.info", "job1.lock"} {
		if err := os.WriteFile(filepath.Join(dir, p), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "extract", "job1"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "extract", "job1", "Movie.mkv"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	ids, err := a.IDs(t.Context())
	if err != nil {
		t.Fatalf("IDs: %v", err)
	}
	if want := []domain.JobID{"job1"}; !slices.Equal(ids, want) {
		t.Fatalf("IDs = %v, want %v", ids, want)
	}
}
