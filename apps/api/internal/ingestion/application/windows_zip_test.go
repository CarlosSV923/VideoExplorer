package application_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

// A zip made on Windows writes the separator of Windows into its names (RF-10c): it is extracted with its folders.
func TestAZipWithWindowsSeparatorsIsExtractedInItsFolders(t *testing.T) {
	t.Parallel()
	need7zz(t)

	r := newRig(t, realExtractor(), 1<<40, true, nil)
	id := r.upload(t, "Movie.zip", zipBytes(t, zipEntry{name: `Movie\Movie.mkv`, content: "fictitious video bytes", mode: 0o644}))

	r.waitFor(t, id, "extraction finished", func(j domain.UploadJob) bool {
		return j.Status == domain.StatusFiltering
	})
	got, err := os.ReadFile(filepath.Join(r.extractionDir(id), "Movie", "Movie.mkv"))
	if err != nil || string(got) != "fictitious video bytes" {
		t.Fatalf("extracted video = %q, %v", got, err)
	}
}

// A name with the Windows separator that goes up leaves the folder: it is refused as in any zip (RF-10c, RNF-02).
func TestAWindowsZipPathThatLeavesTheFolderIsUnsafe(t *testing.T) {
	t.Parallel()
	need7zz(t)

	tests := []struct {
		name  string
		entry string
	}{
		{name: "parent folder", entry: `..\..\evil.mkv`},
		{name: "parent folder after a folder", entry: `Movie\..\..\evil.mkv`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := newRig(t, realExtractor(), 1<<40, true, nil)
			id := r.upload(t, "bad.zip", zipBytes(t, zipEntry{name: tt.entry, content: "x", mode: 0o644}))

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
