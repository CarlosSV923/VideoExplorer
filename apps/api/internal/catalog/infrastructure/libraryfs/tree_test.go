package libraryfs

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

// TestTreeListsEverythingButTheLinks lists a folder recursively: files and folders with their paths
// relative to it, sizes and times, and no symbolic link (RNF-02). A missing folder lists nothing.
func TestTreeListsEverythingButTheLinks(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		setup   func(t *testing.T, dir string)
		want    []string // "rel" for a file, "rel/" for a folder
		noDir   bool     // the folder is not created
		wantErr bool
	}{
		{
			name: "nested files and folders",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				mustWrite(t, filepath.Join(dir, "Show", "S01", "e1.mkv"), "episode")
				mustWrite(t, filepath.Join(dir, "Movie.mkv"), "movie")
			},
			want: []string{"Movie.mkv", "Show/", "Show/S01/", "Show/S01/e1.mkv"},
		},
		{
			name: "an empty folder is listed",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(dir, "Empty"), 0o750); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"Empty/"},
		},
		{
			name: "a symbolic link is not listed",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				mustWrite(t, filepath.Join(dir, "real.mkv"), "real")
				if err := os.Symlink(filepath.Join(dir, "real.mkv"), filepath.Join(dir, "link.mkv")); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"real.mkv"},
		},
		{
			name:  "a folder that does not exist lists nothing",
			noDir: true,
			setup: func(t *testing.T, dir string) {
				t.Helper()
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			dir := filepath.Join(root, domain.UnassignedDir)
			if !tt.noDir {
				if err := os.MkdirAll(dir, 0o750); err != nil {
					t.Fatal(err)
				}
			}
			tt.setup(t, dir)

			got, err := New(root).Tree(dir)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Tree err = %v, want error %v", err, tt.wantErr)
			}
			var rels []string
			for _, it := range got {
				rel := it.Rel
				if it.IsDir {
					rel += "/"
				}
				rels = append(rels, rel)
			}
			if !reflect.DeepEqual(rels, tt.want) {
				t.Errorf("Tree = %v, want %v", rels, tt.want)
			}
		})
	}
}

// TestTreeReportsSizeAndTime gives the size and the modification time of each file, as the section
// reads them to tell a copy in progress (RF-27).
func TestTreeReportsSizeAndTime(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, domain.UnassignedDir)
	file := filepath.Join(dir, "Show", "e1.mkv")
	mustWrite(t, file, "12345")
	when := time.Date(2026, time.October, 13, 8, 30, 0, 0, time.UTC)
	if err := os.Chtimes(file, when, when); err != nil {
		t.Fatal(err)
	}

	got, err := New(root).Tree(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range got {
		if it.Rel == "Show/e1.mkv" {
			if it.Size != 5 || !it.ModTime.Equal(when) || it.IsDir {
				t.Errorf("item = %+v, want a 5-byte file at %v", it, when)
			}
			return
		}
	}
	t.Fatalf("Tree = %+v, want Show/e1.mkv", got)
}

// TestTreeRefusesPathsOutsideTheLibrary keeps the rule of every other read: a folder outside the
// library is refused, not listed.
func TestTreeRefusesPathsOutsideTheLibrary(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if _, err := New(root).Tree(t.TempDir()); err == nil {
		t.Error("Tree of a folder outside the library: no error")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
