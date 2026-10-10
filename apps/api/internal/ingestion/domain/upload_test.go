package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

func TestNewUploadMeta(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		fileName string
		relPath  string
		batchID  string
		want     domain.UploadMeta
		wantErr  bool
	}{
		{
			name:     "file on its own",
			fileName: "Iron Man (2008).mkv",
			want:     domain.UploadMeta{FileName: "Iron Man (2008).mkv"},
		},
		{
			name:     "names are trimmed",
			fileName: "  Iron Man.mkv  ",
			relPath:  " Iron Man / Iron Man.mkv ",
			batchID:  "lote-1_A",
			want:     domain.UploadMeta{FileName: "Iron Man.mkv", RelativePath: "Iron Man/Iron Man.mkv", BatchID: "lote-1_A"},
		},
		{
			name:     "folder drop keeps the relative path",
			fileName: "E01.mkv",
			relPath:  "Show/Season 01/E01.mkv",
			batchID:  "b1",
			want:     domain.UploadMeta{FileName: "E01.mkv", RelativePath: "Show/Season 01/E01.mkv", BatchID: "b1"},
		},
		{
			name:     "characters that later naming rules sanitize are kept",
			fileName: "Mission: Impossible?.mkv",
			want:     domain.UploadMeta{FileName: "Mission: Impossible?.mkv"},
		},
		{name: "empty file name", fileName: "   ", wantErr: true},
		{name: "dot", fileName: ".", wantErr: true},
		{name: "dot dot", fileName: "..", wantErr: true},
		{name: "slash in file name", fileName: "a/b.mkv", wantErr: true},
		{name: "backslash in file name", fileName: `a\b.mkv`, wantErr: true},
		{name: "absolute unix path", fileName: "/etc/passwd", wantErr: true},
		{name: "control character", fileName: "a\x00b.mkv", wantErr: true},
		{name: "tab in file name", fileName: "a\tb.mkv", wantErr: true},
		{name: "invalid UTF-8", fileName: "a\xffb.mkv", wantErr: true},
		{name: "name longer than 255 bytes", fileName: strings.Repeat("a", 256), wantErr: true},
		{name: "name of exactly 255 bytes", fileName: strings.Repeat("a", 255), want: domain.UploadMeta{FileName: strings.Repeat("a", 255)}},
		{name: "relative path with parent", fileName: "a.mkv", relPath: "../a.mkv", wantErr: true},
		{name: "relative path with parent in the middle", fileName: "a.mkv", relPath: "x/../a.mkv", wantErr: true},
		{name: "absolute relative path", fileName: "a.mkv", relPath: "/x/a.mkv", wantErr: true},
		{name: "double slash", fileName: "a.mkv", relPath: "x//a.mkv", wantErr: true},
		{name: "backslash path", fileName: "a.mkv", relPath: `x\a.mkv`, wantErr: true},
		{name: "relative path not ending in file name", fileName: "a.mkv", relPath: "x/b.mkv", wantErr: true},
		{name: "relative path of folder only", fileName: "a.mkv", relPath: "x", wantErr: true},
		{name: "relative path longer than 4096 bytes", fileName: "a.mkv", relPath: strings.Repeat("d/", 2100) + "a.mkv", wantErr: true},
		{name: "batch id with spaces", fileName: "a.mkv", batchID: "a b", wantErr: true},
		{name: "batch id with slash", fileName: "a.mkv", batchID: "a/b", wantErr: true},
		{name: "batch id longer than 64", fileName: "a.mkv", batchID: strings.Repeat("a", 65), wantErr: true},
		{name: "batch id of exactly 64", fileName: "a.mkv", batchID: strings.Repeat("a", 64), want: domain.UploadMeta{FileName: "a.mkv", BatchID: strings.Repeat("a", 64)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := domain.NewUploadMeta(tt.fileName, tt.relPath, tt.batchID)
			if tt.wantErr {
				if !errors.Is(err, domain.ErrInvalidMetadata) {
					t.Fatalf("err = %v, want ErrInvalidMetadata", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("meta = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCheckFreeSpace(t *testing.T) {
	t.Parallel()

	const gib = uint64(1) << 30
	tests := []struct {
		name    string
		size    int64
		free    uint64
		wantErr error
	}{
		{name: "plenty of room", size: 10, free: 50 * gib},
		{name: "exactly size plus the 1 GiB margin", size: 10, free: 10 + gib},
		{name: "one byte short", size: 10, free: 10 + gib - 1, wantErr: domain.ErrInsufficientSpace},
		{name: "empty file still keeps the margin", size: 0, free: gib - 1, wantErr: domain.ErrInsufficientSpace},
		{name: "huge file that does not fit", size: 49<<30 + 512<<20, free: 50 << 30, wantErr: domain.ErrInsufficientSpace},
		{name: "negative size", size: -1, free: 50 * gib, wantErr: domain.ErrInvalidMetadata},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := domain.CheckFreeSpace(tt.size, tt.free)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
