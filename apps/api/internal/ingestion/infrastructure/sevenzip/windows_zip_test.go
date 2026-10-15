package sevenzip_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
)

// windowsZip is a zip made by Compress-Archive of PowerShell 5.1 (RF-10c): its entries are
// `Serie (2020)\Season 01\Serie.S01E01.mkv` (64 bytes) and `Serie (2020)\Serie.es.srt` (37 bytes), with the
// separator of Windows and no entries for the folders.
const windowsZip = "testdata/windows-compress-archive.zip"

func TestWindowsZipKeepsItsFolders(t *testing.T) {
	t.Parallel()
	e := require7zz(t)

	listing, err := e.List(t.Context(), windowsZip, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	dest := t.TempDir()
	warning, err := e.Extract(t.Context(), windowsZip, dest, "", listing, nil)
	if err != nil || warning != "" {
		t.Fatalf("Extract: %v (warning %q)", err, warning)
	}

	want := map[string]int64{
		"Serie (2020)/Season 01/Serie.S01E01.mkv": 64,
		"Serie (2020)/Serie.es.srt":               37,
	}
	for rel, size := range want {
		info, err := os.Stat(filepath.Join(dest, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("%s not extracted in its folders: %v", rel, err)
		}
		if info.Size() != size {
			t.Errorf("%s is %d bytes, want %d", rel, info.Size(), size)
		}
	}
	err = filepath.WalkDir(dest, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.Contains(d.Name(), `\`) {
			t.Errorf("%s keeps the Windows separator in its name", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWindowsZipPathsThatLeaveTheFolderAreUnsafe(t *testing.T) {
	t.Parallel()
	e := require7zz(t)

	tests := []struct {
		name string
		path string
	}{
		{name: "parent folder", path: `..\evil.mkv`},
		{name: "parent folder after a folder", path: `Movie\..\..\evil.mkv`},
		{name: "absolute", path: `\evil.mkv`},
		{name: "drive letter", path: `C:\evil.mkv`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			archive := makeZip(t, entry{name: tt.path, content: "evil", mode: 0o644})
			listing, err := e.List(t.Context(), archive, "")
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			dest := filepath.Join(t.TempDir(), "extract")
			if err := os.Mkdir(dest, 0o750); err != nil {
				t.Fatal(err)
			}
			if _, err := e.Extract(t.Context(), archive, dest, "", listing, nil); !errors.Is(err, application.ErrUnsafeEntry) {
				t.Fatalf("Extract: err = %v, want ErrUnsafeEntry", err)
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(dest), "evil.mkv")); err == nil {
				t.Fatal("an entry was written outside the extraction folder")
			}
		})
	}
}
