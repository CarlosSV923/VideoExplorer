package sevenzip

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/application"
)

func TestPlanSeparatorsReadsTheWindowsSeparatorOfEveryEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		entries []application.ArchiveEntry
		want    []separatorMove
		wantErr error
	}{
		{
			name:    "entries with the separator of Linux are not moved",
			entries: []application.ArchiveEntry{{Path: "Movie/Movie.mkv"}, {Path: "Movie", IsDir: true}},
		},
		{
			name:    "a Windows file name goes to its folders",
			entries: []application.ArchiveEntry{{Path: `Serie (2020)\Season 01\Serie.S01E01.mkv`}},
			want:    []separatorMove{{from: `Serie (2020)\Season 01\Serie.S01E01.mkv`, to: "Serie (2020)/Season 01/Serie.S01E01.mkv"}},
		},
		{
			name:    "a Windows folder entry is a folder",
			entries: []application.ArchiveEntry{{Path: `Serie\`, IsDir: true}},
			want:    []separatorMove{{from: `Serie\`, to: "Serie", dir: true}},
		},
		{
			name:    "a mixed name keeps its Linux folders and splits the Windows ones",
			entries: []application.ArchiveEntry{{Path: `a/b\c.mkv`}},
			want:    []separatorMove{{from: `a/b\c.mkv`, to: "a/b/c.mkv"}},
		},
		{
			name:    "a name that goes up leaves the folder",
			entries: []application.ArchiveEntry{{Path: `..\evil.mkv`}},
			wantErr: application.ErrUnsafeEntry,
		},
		{
			name:    "a name that goes up after a folder leaves it too",
			entries: []application.ArchiveEntry{{Path: `Movie\..\..\evil.mkv`}},
			wantErr: application.ErrUnsafeEntry,
		},
		{
			name:    "an absolute name is unsafe",
			entries: []application.ArchiveEntry{{Path: `\evil.mkv`}},
			wantErr: application.ErrUnsafeEntry,
		},
		{
			name:    "a drive letter is unsafe",
			entries: []application.ArchiveEntry{{Path: `C:\evil.mkv`}},
			wantErr: application.ErrUnsafeEntry,
		},
		{
			name:    "an empty component is unsafe",
			entries: []application.ArchiveEntry{{Path: `Movie\\evil.mkv`}},
			wantErr: application.ErrUnsafeEntry,
		},
		{
			name:    "two names that normalize to the same path are corrupt",
			entries: []application.ArchiveEntry{{Path: `a\b.mkv`}, {Path: "a/b.mkv"}},
			wantErr: application.ErrCorrupt,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := planSeparators(tt.entries)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("moves = %+v, want %+v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("move %d = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestIsZipReadsTheMagicBytesNotTheName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{name: "zip", content: "PK\x03\x04 entries", want: true},
		{name: "empty zip", content: "PK\x05\x06 end", want: true},
		{name: "7z", content: "7z\xBC\xAF\x27\x1C rest", want: false},
		{name: "rar", content: "Rar!\x1A\x07\x00 rest", want: false},
		{name: "not an archive", content: "PFS0 game", want: false},
		{name: "shorter than a header", content: "PK", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := filepath.Join(t.TempDir(), "game.zip")
			if err := os.WriteFile(p, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := isZip(p)
			if err != nil {
				t.Fatalf("isZip: %v", err)
			}
			if got != tt.want {
				t.Errorf("isZip = %v, want %v", got, tt.want)
			}
		})
	}
}
