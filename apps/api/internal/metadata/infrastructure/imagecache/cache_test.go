package imagecache

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/application"
	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/metadata/domain"
)

// jpegHead starts with the JPEG signature, which is what the type check sniffs.
var jpegHead = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{0x22}, 508)...)

var pngImage = append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0x33}, 504)...)

const testFile = "9ZmWNo1wq1y9vSIt7iGlVAtsgXB.jpg"

// fakeSource answers Fetch with a fixed body or error and counts the calls.
type fakeSource struct {
	mu    sync.Mutex
	calls int
	body  func() io.Reader
	err   error
}

var _ application.ImageSource = (*fakeSource)(nil)

func (f *fakeSource) Fetch(_ context.Context, _ domain.ImageRef) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return io.NopCloser(f.body()), nil
}

func (f *fakeSource) fetches() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func bodyOf(data []byte) func() io.Reader {
	return func() io.Reader { return bytes.NewReader(data) }
}

// failingReader yields the head of an image and then breaks, like a dropped connection.
type failingReader struct{ head io.Reader }

func (r failingReader) Read(p []byte) (int, error) {
	n, err := r.head.Read(p)
	if errors.Is(err, io.EOF) {
		return n, errors.New("connection reset")
	}
	return n, err
}

func ref(t *testing.T, kind, file string) domain.ImageRef {
	t.Helper()

	r, err := domain.NewImageRef(kind, file)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// names lists the files under dir, recursively, relative to dir.
func names(t *testing.T, dir string) []string {
	t.Helper()

	var out []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return out
}

func readAll(t *testing.T, img application.StoredImage) []byte {
	t.Helper()

	defer img.File.Close()
	data, err := io.ReadAll(img.File)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestOpenServesTheCachedFileWithoutTMDB(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "poster"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "poster", testFile), jpegHead, 0o600); err != nil {
		t.Fatal(err)
	}
	source := &fakeSource{err: domain.ErrUpstream}

	img, err := New(dir, source).Open(t.Context(), ref(t, "poster", testFile))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got := readAll(t, img); !bytes.Equal(got, jpegHead) {
		t.Errorf("content differs from the cached file")
	}
	if source.fetches() != 0 {
		t.Errorf("TMDB was asked %d times for a cached image, want 0", source.fetches())
	}
}

func TestOpenWithoutSourceAndNotCachedIsNotConfigured(t *testing.T) {
	t.Parallel()

	_, err := New(t.TempDir(), nil).Open(t.Context(), ref(t, "logo", "a.png"))
	if !errors.Is(err, domain.ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}

func TestOpenDownloadsOnceAndKeepsTheFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	source := &fakeSource{body: bodyOf(jpegHead)}
	c := New(dir, source)
	r := ref(t, "backdrop", testFile)

	for i := range 2 {
		img, err := c.Open(t.Context(), r)
		if err != nil {
			t.Fatalf("Open %d: %v", i+1, err)
		}
		if got := readAll(t, img); !bytes.Equal(got, jpegHead) {
			t.Fatalf("Open %d: content differs", i+1)
		}
	}
	if source.fetches() != 1 {
		t.Errorf("TMDB fetches = %d, want 1", source.fetches())
	}
	want := []string{"backdrop/" + testFile}
	if got := names(t, dir); len(got) != 1 || got[0] != want[0] {
		t.Errorf("files = %v, want %v (no temporary files left)", got, want)
	}
}

func TestOpenFailuresLeaveNothingVisible(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source *fakeSource
		file   string
		want   error
	}{
		{
			name:   "TMDB has no such image",
			source: &fakeSource{err: domain.ErrNotFound},
			file:   testFile,
			want:   domain.ErrNotFound,
		},
		{
			name:   "connection drops in the middle of the image",
			source: &fakeSource{body: func() io.Reader { return failingReader{head: bytes.NewReader(jpegHead)} }},
			file:   testFile,
			want:   domain.ErrUpstream,
		},
		{
			name:   "TMDB answers something that is not an image",
			source: &fakeSource{body: bodyOf([]byte("<html><body>rate limited</body></html>"))},
			file:   testFile,
			want:   domain.ErrUpstream,
		},
		{
			name:   "PNG bytes under a .jpg name",
			source: &fakeSource{body: bodyOf(pngImage)},
			file:   testFile,
			want:   domain.ErrUpstream,
		},
		{
			name: "body above the size limit",
			source: &fakeSource{body: func() io.Reader {
				return io.MultiReader(bytes.NewReader(jpegHead), io.LimitReader(zeros{}, maxImageBytes))
			}},
			file: testFile,
			want: domain.ErrUpstream,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			_, err := New(dir, tt.source).Open(t.Context(), ref(t, "poster", tt.file))
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if got := names(t, dir); len(got) != 0 {
				t.Errorf("files after the failure = %v, want none: a partial image must never be visible", got)
			}
		})
	}
}

// zeros is an endless reader of zero bytes, so the size test does not hold a large buffer.
type zeros struct{}

func (zeros) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func TestPruneRemovesOnlyUnreferencedImages(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	files := map[string]time.Time{
		"poster/keep.jpg":        now,
		"poster/drop.jpg":        now,
		"logo/keep.png":          now,
		"still/drop.jpg":         now,
		"poster/.download-old":   now.Add(-2 * time.Hour), // an abandoned download: removed
		"poster/.download-fresh": now.Add(-time.Minute),   // maybe still in progress: kept
	}
	for name, mod := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, mod, mod); err != nil {
			t.Fatal(err)
		}
	}

	c := New(dir, nil)
	c.now = func() time.Time { return now }
	keep := []domain.ImageRef{ref(t, "poster", "keep.jpg"), ref(t, "logo", "keep.png")}
	removed, err := c.Prune(t.Context(), keep)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if removed != 3 {
		t.Errorf("removed = %d, want 3 (drop.jpg, still/drop.jpg and the abandoned download)", removed)
	}
	want := []string{"logo/keep.png", "poster/.download-fresh", "poster/keep.jpg"}
	got := names(t, dir)
	if len(got) != len(want) {
		t.Fatalf("files = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("files = %v, want %v", got, want)
			break
		}
	}
}

func TestPruneWithoutImagesIsNotAnError(t *testing.T) {
	t.Parallel()

	removed, err := New(filepath.Join(t.TempDir(), "missing"), nil).Prune(t.Context(), nil)
	if err != nil || removed != 0 {
		t.Fatalf("Prune = (%d, %v), want (0, nil) for a cache that does not exist yet", removed, err)
	}
}
