//go:build unix

package fileid_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/fileid"
)

func TestIdentityOfAFileSurvivesARenameAndChangesWithItsContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// change edits the file after its identity was read; nil leaves it alone.
		change func(t *testing.T, path string) string
		// same: the identity read after the change still matches the first one (Matches).
		same bool
	}{
		{
			name: "a rename inside the folder keeps the identity",
			change: func(t *testing.T, path string) string {
				t.Helper()
				renamed := filepath.Join(filepath.Dir(path), "renamed.mkv")
				if err := os.Rename(path, renamed); err != nil {
					t.Fatal(err)
				}
				return renamed
			},
			same: true,
		},
		{
			name: "a change of size changes the identity",
			change: func(t *testing.T, path string) string {
				t.Helper()
				appendBytes(t, path, []byte("more"))
				return path
			},
			same: false,
		},
		{
			name: "a new modification time changes the identity",
			change: func(t *testing.T, path string) string {
				t.Helper()
				later := time.Now().Add(time.Hour)
				if err := os.Chtimes(path, later, later); err != nil {
					t.Fatal(err)
				}
				return path
			},
			same: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "movie.mkv")
			if err := os.WriteFile(path, []byte("video bytes"), 0o600); err != nil {
				t.Fatal(err)
			}
			before, err := fileid.Identifier{}.Identify(path)
			if err != nil {
				t.Fatalf("Identify: %v", err)
			}

			after := tt.change(t, path)
			now, err := fileid.Identifier{}.Identify(after)
			if err != nil {
				t.Fatalf("Identify after the change: %v", err)
			}
			if got := before.Matches(now); got != tt.same {
				t.Fatalf("Matches = %v, want %v (before %+v, after %+v)", got, tt.same, before, now)
			}
		})
	}
}

func TestIdentifyRefusesFoldersAndMissingFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path func(t *testing.T) string
	}{
		{name: "a folder is not a file", path: func(t *testing.T) string { return t.TempDir() }},
		{name: "a missing file", path: func(t *testing.T) string { return filepath.Join(t.TempDir(), "gone.mkv") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := (fileid.Identifier{}).Identify(tt.path(t)); err == nil {
				t.Fatal("Identify succeeded, want an error")
			}
		})
	}
}

func appendBytes(t *testing.T, path string, data []byte) {
	t.Helper()

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
}
