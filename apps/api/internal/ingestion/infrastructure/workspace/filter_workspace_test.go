package workspace_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/infrastructure/workspace"
)

func TestExtractedContentIsListedAsRegularFilesWithSlashes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	area := workspace.New(root, nil)
	const id domain.JobID = "unit-1"
	if err := area.ResetExtraction(id); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"Movie/Movie.mkv":        "video",
		"Movie/Samples/clip.mkv": "clip",
		"Movie.nfo":              "nfo",
	}
	for rel, body := range files {
		p := filepath.Join(area.ExtractionDir(id), filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	got, err := area.ExtractedFiles(id)
	if err != nil {
		t.Fatalf("ExtractedFiles: %v", err)
	}
	var names []string
	for _, f := range got {
		names = append(names, f.Path)
		if f.Size != int64(len(files[f.Path])) {
			t.Errorf("%s: size %d, want %d", f.Path, f.Size, len(files[f.Path]))
		}
	}
	slices.Sort(names)
	want := []string{"Movie.nfo", "Movie/Movie.mkv", "Movie/Samples/clip.mkv"}
	if !slices.Equal(names, want) {
		t.Errorf("ExtractedFiles = %v, want %v (folders are not files)", names, want)
	}
	if got := area.ExtractedPath(id, "Movie/Movie.mkv"); got != filepath.Join(area.ExtractionDir(id), "Movie", "Movie.mkv") {
		t.Errorf("ExtractedPath = %q", got)
	}
}

func TestExtractionExistsOnlyForAFolderAndFileExistsOnlyForARegularFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	area := workspace.New(root, nil)
	const archived domain.JobID = "unit-2"
	if area.ExtractionExists(archived) {
		t.Error("an upload without an extraction folder reports one")
	}
	if err := area.ResetExtraction(archived); err != nil {
		t.Fatal(err)
	}
	if !area.ExtractionExists(archived) {
		t.Error("an extracted archive, even an empty one, has its folder")
	}

	staged := filepath.Join(root, "upload-1")
	if err := os.WriteFile(staged, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "a staged file", path: staged, want: true},
		{name: "a folder is not a file", path: area.ExtractionDir(archived), want: false},
		{name: "a missing file", path: filepath.Join(root, "gone"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := area.FileExists(tt.path); got != tt.want {
				t.Errorf("FileExists(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
