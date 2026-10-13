package application_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/infrastructure/libraryfs"
)

const (
	testMovieFolder  = "Movies/Iron Man (2008) [tmdbid-1726]"
	testSeriesFolder = "TV Shows/Breaking Bad (2008) [tmdbid-1396]"
	testIncoming     = "_unassigned/Iron Man/Iron Man (2008).mkv"
)

// libRepo is the registry as the finder reads it: one title and its files.
type libRepo struct {
	title domain.LibraryTitle
	files []domain.LibraryFile
}

func (r *libRepo) TitleByFolder(_ context.Context, folder string) (domain.LibraryTitle, bool, error) {
	if r.title.Folder == "" || r.title.Folder != folder {
		return domain.LibraryTitle{}, false, nil
	}
	return r.title, true, nil
}

func (r *libRepo) Titles(context.Context) ([]domain.LibraryTitle, error) {
	return []domain.LibraryTitle{r.title}, nil
}

func (r *libRepo) Files(context.Context, int64) ([]domain.LibraryFile, error) {
	return r.files, nil
}

// hashes answers the partial hash of a file from a table keyed by its path relative to the library,
// and records the reads. A file that is not in the table has no hash.
type hashes struct {
	mu     sync.Mutex
	root   string
	table  map[string]string
	called []string
}

func (h *hashes) PartialHash(_ context.Context, path string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.called = append(h.called, path)
	rel, err := filepath.Rel(h.root, path)
	if err != nil {
		return "", err
	}
	return h.table[filepath.ToSlash(rel)], nil
}

// writeLibrary creates the files of a library: rel → size bytes.
func writeLibrary(t *testing.T, root string, files map[string]int) {
	t.Helper()
	for rel, size := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, make([]byte, size), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDuplicateFinderFindsWhatHoldsTheTargetsPlace(t *testing.T) {
	t.Parallel()

	chapter2 := domain.Target{Kind: domain.KindSeries, Place: domain.Place{Episode: true, Season: 1, First: 2, Last: 2}}
	tests := []struct {
		name     string
		disk     map[string]int
		registry []domain.LibraryFile
		target   domain.Target
		folder   string
		// hashTable gives partial hashes by base name (see hashes).
		hashTable map[string]string
		wantFiles int
		wantExact bool
		// wantHashed is how many files are read for their partial hash, the new one included.
		wantHashed int
	}{
		{
			name:       "a video of the disk with the same size and hash is an exact copy",
			disk:       map[string]int{testMovieFolder + "/Iron Man (2008).mkv": 5},
			target:     domain.Target{Kind: domain.KindMovie},
			folder:     testMovieFolder,
			hashTable:  map[string]string{testMovieFolder + "/Iron Man (2008).mkv": "ph1:same", testIncoming: "ph1:same"},
			wantFiles:  1,
			wantExact:  true,
			wantHashed: 2,
		},
		{
			name:       "the same size with another hash is a duplicate, not an exact copy",
			disk:       map[string]int{testMovieFolder + "/Iron Man (2008).mkv": 5},
			target:     domain.Target{Kind: domain.KindMovie},
			folder:     testMovieFolder,
			hashTable:  map[string]string{testMovieFolder + "/Iron Man (2008).mkv": "ph1:one", testIncoming: "ph1:two"},
			wantFiles:  1,
			wantHashed: 2,
		},
		{
			name:       "a video of another size is a duplicate but is not read",
			disk:       map[string]int{testMovieFolder + "/Iron Man (2008) - 2160p.mkv": 9},
			target:     domain.Target{Kind: domain.KindMovie},
			folder:     testMovieFolder,
			wantFiles:  1,
			wantHashed: 0,
		},
		{
			name: "a registered video that the disk does not hold is still a duplicate",
			registry: []domain.LibraryFile{{
				Rel: testMovieFolder + "/Iron Man (2008).mkv", Kind: domain.FileVideo, Size: 9, PartialHash: "ph1:r",
			}},
			target:     domain.Target{Kind: domain.KindMovie},
			folder:     testMovieFolder,
			wantFiles:  1,
			wantHashed: 0,
		},
		{
			name: "a trashed video is not a duplicate",
			registry: []domain.LibraryFile{{
				Rel: testMovieFolder + "/Iron Man (2008).mkv", Kind: domain.FileVideo, Size: 5, TrashEntryID: 3,
			}},
			target:     domain.Target{Kind: domain.KindMovie},
			folder:     testMovieFolder,
			wantFiles:  0,
			wantHashed: 0,
		},
		{
			name: "a subtitle and a file that is not a video are not duplicates",
			disk: map[string]int{
				testMovieFolder + "/Iron Man (2008).en.srt": 5,
				testMovieFolder + "/Iron Man (2008).nfo":    5,
			},
			target:     domain.Target{Kind: domain.KindMovie},
			folder:     testMovieFolder,
			wantFiles:  0,
			wantHashed: 0,
		},
		{
			name:       "an empty title folder has no duplicate",
			target:     domain.Target{Kind: domain.KindMovie},
			folder:     testMovieFolder,
			wantFiles:  0,
			wantHashed: 0,
		},
		{
			name:       "parts against a whole movie in the folder are a duplicate of the movie",
			disk:       map[string]int{testMovieFolder + "/Iron Man (2008).mkv": 5},
			target:     domain.Target{Kind: domain.KindMovie, Parts: true},
			folder:     testMovieFolder,
			wantFiles:  1,
			wantHashed: 2,
		},
		{
			name:       "a chapter in its season folder is a duplicate of the chapter",
			disk:       map[string]int{testSeriesFolder + "/Season 01/Breaking Bad (2008) - S01E02.mkv": 5},
			target:     chapter2,
			folder:     testSeriesFolder,
			wantFiles:  1,
			wantHashed: 2,
		},
		{
			name: "another chapter of the same size is not read, a video at the root and a name without a chapter are not duplicates",
			disk: map[string]int{
				testSeriesFolder + "/Season 01/Breaking Bad (2008) - S01E01.mkv": 5,
				testSeriesFolder + "/Breaking Bad (2008) - S01E02.mkv":           5,
				testSeriesFolder + "/Season 01/notes.mkv":                        5,
			},
			target:     chapter2,
			folder:     testSeriesFolder,
			wantFiles:  0,
			wantHashed: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeLibrary(t, root, tt.disk)
			writeLibrary(t, root, map[string]int{testIncoming: 5})
			repo := &libRepo{files: tt.registry}
			if len(tt.registry) > 0 {
				repo.title = domain.LibraryTitle{ID: 1, Folder: tt.folder, Kind: tt.target.Kind}
			}
			h := &hashes{root: root, table: tt.hashTable}
			finder := application.NewDuplicateFinder(repo, libraryfs.New(root), h, []string{".mkv", ".mp4"})

			got, err := finder.Find(t.Context(), tt.folder, tt.target, testIncoming)
			if err != nil {
				t.Fatalf("Find: %v", err)
			}
			if len(got.Files) != tt.wantFiles {
				t.Errorf("files = %+v, want %d", got.Files, tt.wantFiles)
			}
			if got.ExactCopy != tt.wantExact {
				t.Errorf("exact copy = %v, want %v", got.ExactCopy, tt.wantExact)
			}
			if len(h.called) != tt.wantHashed {
				t.Errorf("hashed %d files (%v), want %d", len(h.called), h.called, tt.wantHashed)
			}
		})
	}
}

func TestDuplicateFinderRefusesALinkOnTheWay(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	writeLibrary(t, root, map[string]int{testIncoming: 5})
	if err := os.MkdirAll(filepath.Join(root, "Movies"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, filepath.FromSlash(testMovieFolder))); err != nil {
		t.Skipf("symbolic links are not available here: %v", err)
	}
	h := &hashes{root: root}
	finder := application.NewDuplicateFinder(&libRepo{}, libraryfs.New(root), h, []string{".mkv"})

	if _, err := finder.Find(t.Context(), testMovieFolder, domain.Target{Kind: domain.KindMovie}, testIncoming); err == nil {
		t.Fatal("Find listed a folder that is a symbolic link")
	}
	if len(h.called) != 0 {
		t.Errorf("hashed %v through a symbolic link", h.called)
	}
}
