package partialhash

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

const mib = int64(1 << 20)

// pattern is the content of the test files: a byte that depends on its offset.
func pattern(off int64) byte {
	return byte(off*131 + off>>9)
}

// fakeFile is a file of size bytes whose content is pattern. It records every read, and a
// read past its end returns io.EOF, as a file does.
type fakeFile struct {
	size int64

	mu    sync.Mutex
	reads []read
}

type read struct {
	off, n int64
}

func (f *fakeFile) ReadAt(p []byte, off int64) (int, error) {
	f.mu.Lock()
	f.reads = append(f.reads, read{off: off, n: int64(len(p))})
	f.mu.Unlock()

	if off >= f.size {
		return 0, io.EOF
	}
	n := min(int64(len(p)), f.size-off)
	for i := range n {
		p[i] = pattern(off + i)
	}
	if n < int64(len(p)) {
		return int(n), io.EOF
	}
	return int(n), nil
}

func (f *fakeFile) readRecords() []read {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]read(nil), f.reads...)
}

// referenceHash is the partial hash of a file of size bytes with pattern content, computed
// from the domain's ranges and without the buffered reads under test.
func referenceHash(size int64) string {
	h := domain.NewPartialDigest(size)
	for _, rg := range domain.PartialHashRanges(size) {
		content := make([]byte, rg.Length)
		for i := range content {
			content[i] = pattern(rg.Offset + int64(i))
		}
		_, _ = h.Write(content)
	}
	return domain.FormatPartialHash(h.Sum(nil))
}

func TestPartialHashReadsOnlyItsRangesThroughASmallBuffer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		size int64
	}{
		{name: "an empty file is not read", size: 0},
		{name: "one byte", size: 1},
		{name: "4 MiB is read whole, once", size: 4 * mib},
		{name: "12 MiB is read whole, once", size: 12 * mib},
		{name: "12 MiB + 1: two chunks and the last one, no byte twice", size: 12*mib + 1},
		{name: "100 MiB reads 12 MiB and never the rest", size: 100 * mib},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := &fakeFile{size: tt.size}
			got, err := hashRanges(t.Context(), f, tt.size, make([]byte, bufferSize))
			if err != nil {
				t.Fatalf("hashRanges: %v", err)
			}
			if want := referenceHash(tt.size); got != want {
				t.Fatalf("hash = %s, want %s", got, want)
			}

			// Every read lies inside one range, is at most one buffer long, and the reads
			// together cover each byte of the ranges exactly once.
			ranges := domain.PartialHashRanges(tt.size)
			var total int64
			for _, r := range f.readRecords() {
				if r.n > bufferSize {
					t.Fatalf("a read of %d bytes, want at most the buffer of %d", r.n, bufferSize)
				}
				if !insideOneRange(r, ranges) {
					t.Fatalf("a read of %d bytes at %d lies outside the ranges %v", r.n, r.off, ranges)
				}
				total += r.n
			}
			var wantTotal int64
			for _, rg := range ranges {
				wantTotal += rg.Length
			}
			if total != wantTotal {
				t.Fatalf("read %d bytes of a %d-byte file, want exactly its ranges (%d bytes)", total, tt.size, wantTotal)
			}
		})
	}
}

func TestPartialHashStopsWhenTheContextEnds(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	f := &fakeFile{size: 100 * mib}
	if _, err := hashRanges(ctx, f, f.size, make([]byte, bufferSize)); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if reads := f.readRecords(); len(reads) != 0 {
		t.Fatalf("a cancelled hash still read %d times", len(reads))
	}
}

func TestPartialHashFailsWhenTheFileIsShorterThanItsSize(t *testing.T) {
	t.Parallel()

	// The file shrank after its size was read: the range past its end cannot be read.
	f := &fakeFile{size: 1 * mib}
	_, err := hashRanges(t.Context(), f, 100*mib, make([]byte, bufferSize))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("err = %v, want io.ErrUnexpectedEOF", err)
	}
}

// writeFile writes a real file of size bytes with pattern content.
func writeFile(t *testing.T, path string, size int64) {
	t.Helper()

	content := make([]byte, size)
	for i := range content {
		content[i] = pattern(int64(i))
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPartialHashOfAFileOnDisk(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "Movie.mkv")
	const size = 13 * mib
	writeFile(t, path, size)

	got, err := Hasher{}.PartialHash(t.Context(), path)
	if err != nil {
		t.Fatalf("PartialHash: %v", err)
	}
	if want := referenceHash(size); got != want {
		t.Fatalf("PartialHash = %s, want %s", got, want)
	}
}

func TestPartialHashRefusesWhatIsNotAFileOrIsMissing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    func(t *testing.T) string
		wantErr error // nil: any error will do
	}{
		{name: "a folder is not a file", path: func(t *testing.T) string { return t.TempDir() }},
		{
			name:    "a missing file reports that it does not exist",
			path:    func(t *testing.T) string { return filepath.Join(t.TempDir(), "gone.mkv") },
			wantErr: fs.ErrNotExist,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := Hasher{}.PartialHash(t.Context(), tt.path(t))
			if err == nil {
				t.Fatal("PartialHash succeeded, want an error")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// insideOneRange reports whether the read lies entirely inside one of the ranges.
func insideOneRange(r read, ranges []domain.ByteRange) bool {
	for _, rg := range ranges {
		if r.off >= rg.Offset && r.off+r.n <= rg.Offset+rg.Length {
			return true
		}
	}
	return false
}
