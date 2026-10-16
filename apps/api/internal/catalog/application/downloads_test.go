package application

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/catalog/domain"
)

type downloadRepo struct {
	titles []domain.LibraryTitle
	files  map[int64][]domain.LibraryFile
}

func (r downloadRepo) TitleByFolder(context.Context, string) (domain.LibraryTitle, bool, error) {
	return domain.LibraryTitle{}, false, nil
}

func (r downloadRepo) Titles(context.Context) ([]domain.LibraryTitle, error) { return r.titles, nil }

func (r downloadRepo) Files(_ context.Context, id int64) ([]domain.LibraryFile, error) {
	return r.files[id], nil
}

type testDownloadFiles struct{ root string }

func (f testDownloadFiles) LibraryPath(elem ...string) string {
	return filepath.Join(append([]string{f.root}, elem...)...)
}

func (f testDownloadFiles) UnassignedPath(elem ...string) string {
	return filepath.Join(append([]string{f.root, domain.UnassignedDir}, elem...)...)
}

func (f testDownloadFiles) Info(p string) (FileInfo, error) {
	info, err := os.Lstat(p)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		if err == nil {
			err = errors.New("not a regular file")
		}
		return FileInfo{}, err
	}
	return FileInfo{Size: info.Size(), Modified: info.ModTime()}, nil
}

func (f testDownloadFiles) Open(p string) (io.ReadSeekCloser, error) { return os.Open(p) }

func TestLibraryDownloadsPrepareLiveMovieSeriesAndSeason(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	fs := testDownloadFiles{root: root}
	mod := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	title := domain.LibraryTitle{ID: 1, Kind: domain.KindSeries, Folder: "TV Shows/Lizzie Borden (2014)"}
	v1 := domain.LibraryFile{ID: 11, TitleID: 1, Kind: domain.FileVideo, Rel: "TV Shows/Lizzie Borden (2014)/Season 01/Lizzie.S01E01.mkv", Place: domain.Place{Episode: true, Season: 1, First: 1, Last: 1}}
	s1 := domain.LibraryFile{ID: 12, TitleID: 1, Kind: domain.FileSubtitle, Rel: "TV Shows/Lizzie Borden (2014)/Season 01/Lizzie.S01E01.es.srt", VideoRel: v1.Rel}
	v2 := domain.LibraryFile{ID: 13, TitleID: 1, Kind: domain.FileVideo, Rel: "TV Shows/Lizzie Borden (2014)/Season 02/Lizzie.S02E01.mkv", Place: domain.Place{Episode: true, Season: 2, First: 1, Last: 1}}
	trashed := domain.LibraryFile{ID: 14, TitleID: 1, Kind: domain.FileVideo, Rel: "TV Shows/Lizzie Borden (2014)/Season 01/trash.mkv", TrashEntryID: 9}
	for rel, contents := range map[string]string{v1.Rel: "video-1", s1.Rel: "subtitle", v2.Rel: "video-2"} {
		path := fs.LibraryPath(filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, mod, mod); err != nil {
			t.Fatal(err)
		}
	}
	service := NewLibraryDownloads(downloadRepo{titles: []domain.LibraryTitle{title}, files: map[int64][]domain.LibraryFile{1: {v1, s1, v2, trashed}}}, fs)

	tests := []struct {
		name        string
		prepare     func() (Download, error)
		wantName    string
		wantEntries []string
	}{
		{name: "whole series", prepare: func() (Download, error) { return service.Title(t.Context(), 1) }, wantName: "Lizzie Borden (2014).zip", wantEntries: []string{
			"Lizzie Borden (2014)/Season 01/Lizzie.S01E01.es.srt", "Lizzie Borden (2014)/Season 01/Lizzie.S01E01.mkv", "Lizzie Borden (2014)/Season 02/Lizzie.S02E01.mkv",
		}},
		{name: "one season with subtitles", prepare: func() (Download, error) { return service.Season(t.Context(), 1, 1) }, wantName: "Lizzie Borden (2014) - Season 01.zip", wantEntries: []string{
			"Lizzie Borden (2014)/Season 01/Lizzie.S01E01.es.srt", "Lizzie Borden (2014)/Season 01/Lizzie.S01E01.mkv",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := tt.prepare()
			if err != nil {
				t.Fatal(err)
			}
			if got.Name != tt.wantName || len(got.Entries) != len(tt.wantEntries) {
				t.Fatalf("download = %+v, want name %q and %d entries", got, tt.wantName, len(tt.wantEntries))
			}
			for i, e := range got.Entries {
				if e.Name != tt.wantEntries[i] || e.Size <= 0 || e.Modified.IsZero() {
					t.Errorf("entry %d = %+v", i, e)
				}
			}
		})
	}
}

func TestLibraryDownloadsRejectMissingTrashAndSymlinkTargets(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	fs := testDownloadFiles{root: root}
	title := domain.LibraryTitle{ID: 1, Kind: domain.KindMovie, Folder: "Movies/Godzilla (2014)"}
	file := domain.LibraryFile{ID: 11, TitleID: 1, Kind: domain.FileVideo, Rel: "Movies/Godzilla (2014)/Godzilla.mkv"}
	service := NewLibraryDownloads(downloadRepo{titles: []domain.LibraryTitle{title}, files: map[int64][]domain.LibraryFile{1: {file}}}, fs)

	tests := []struct {
		name string
		call func() (Download, error)
		want error
	}{
		{name: "missing file", call: func() (Download, error) { return service.File(t.Context(), 1, 11) }, want: ErrDownloadUnavailable},
		{name: "unknown title", call: func() (Download, error) { return service.Title(t.Context(), 99) }, want: ErrTitleNotFound},
		{name: "unknown file", call: func() (Download, error) { return service.File(t.Context(), 1, 99) }, want: ErrLibraryFileNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := tt.call()
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}

	outside := filepath.Join(t.TempDir(), "outside.mkv")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := fs.LibraryPath(filepath.FromSlash(file.Rel))
	if err := os.MkdirAll(filepath.Dir(link), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symbolic links are not available here: %v", err)
	}
	if _, err := service.File(t.Context(), 1, 11); !errors.Is(err, ErrDownloadUnavailable) {
		t.Fatalf("symlink error = %v, want %v", err, ErrDownloadUnavailable)
	}
}

func TestDownloadPathsAreBuiltFromRegistryRel(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	fs := testDownloadFiles{root: root}
	rel := "Movies/Godzilla (2014)/Godzilla.mkv"
	path := fs.LibraryPath(filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewLibraryDownloads(downloadRepo{
		titles: []domain.LibraryTitle{{ID: 1, Kind: domain.KindMovie, Folder: "Movies/Godzilla (2014)"}},
		files:  map[int64][]domain.LibraryFile{1: {{ID: 7, TitleID: 1, Kind: domain.FileVideo, Rel: rel}}},
	}, fs)
	got, err := service.File(t.Context(), 1, 7)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got.File.Path, filepath.Join("Movies", "Godzilla (2014)", "Godzilla.mkv")) {
		t.Fatalf("path = %q", got.File.Path)
	}
}
