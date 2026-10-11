package domain_test

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"testing"

	"github.com/CarlosSV923/VideoExplorer/apps/api/internal/ingestion/domain"
)

const mib = int64(1 << 20)

// The partial hash reads, of a file of size bytes, the first 4 MiB, a 4 MiB chunk centred on
// the middle and the last 4 MiB (RF-24). Ranges that overlap or touch are one range, so a file
// of 12 MiB or less is read once, whole. The tests below pin that down for the sizes the spec
// names; they never allocate the files: content is generated on the fly.

func TestPartialHashRangesReadEachByteOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		size int64
		want []domain.ByteRange
	}{
		{name: "an empty file has no ranges", size: 0, want: nil},
		{name: "one byte is read whole", size: 1, want: []domain.ByteRange{{Offset: 0, Length: 1}}},
		{name: "4 MiB is read whole, once", size: 4 * mib, want: []domain.ByteRange{{Offset: 0, Length: 4 * mib}}},
		{
			name: "12 MiB: the three chunks touch and make one range, read whole once",
			size: 12 * mib,
			want: []domain.ByteRange{{Offset: 0, Length: 12 * mib}},
		},
		{
			// The middle chunk is [4 MiB, 8 MiB) and the last one starts at 8 MiB + 1: the byte
			// at 8 MiB is in no range (accepted limit, see the test of the bytes outside).
			name: "12 MiB + 1: first and middle merge, the last chunk starts one byte later",
			size: 12*mib + 1,
			want: []domain.ByteRange{{Offset: 0, Length: 8 * mib}, {Offset: 8*mib + 1, Length: 4 * mib}},
		},
		{
			name: "100 MiB: first, centred and last chunks, disjoint",
			size: 100 * mib,
			want: []domain.ByteRange{
				{Offset: 0, Length: 4 * mib},
				{Offset: 48 * mib, Length: 4 * mib},
				{Offset: 96 * mib, Length: 4 * mib},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := domain.PartialHashRanges(tt.size)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("PartialHashRanges(%d) = %v, want %v", tt.size, got, tt.want)
			}
		})
	}
}

func TestPartialHashRangesAreSortedInsideTheFileAndNeverOverlap(t *testing.T) {
	t.Parallel()

	sizes := []int64{0, 1, 2, 3, 4*mib - 1, 4 * mib, 4*mib + 1, 5 * mib, 8 * mib, 8*mib + 7, 12 * mib, 12*mib + 1, 100 * mib, 100*mib + 3}
	for _, size := range sizes {
		ranges := domain.PartialHashRanges(size)
		var read int64
		for i, r := range ranges {
			if r.Length <= 0 || r.Offset < 0 || r.Offset+r.Length > size {
				t.Fatalf("size %d: range %v is empty or outside the file", size, r)
			}
			if i > 0 && r.Offset < ranges[i-1].Offset+ranges[i-1].Length {
				t.Fatalf("size %d: ranges %v overlap", size, ranges)
			}
			read += r.Length
		}
		if read > size || read > 12*mib {
			t.Fatalf("size %d: %d bytes read, want at most the file and at most 12 MiB", size, read)
		}
	}
}

// pattern is the content of the test files: a byte that depends on its offset.
func pattern(off int64) byte {
	return byte(off*131 + off>>9)
}

// partialHashOf is the partial hash of a file of size bytes whose content is pattern, with
// the byte at flip (when flip is not negative) changed. It reads the ranges the way the
// adapter does, one buffer at a time, and never builds the file.
func partialHashOf(size, flip int64) string {
	h := domain.NewPartialDigest(size)
	buf := make([]byte, 64<<10)
	for _, r := range domain.PartialHashRanges(size) {
		end := r.Offset + r.Length
		for off := r.Offset; off < end; {
			n := min(int64(len(buf)), end-off)
			for i := range n {
				buf[i] = pattern(off + i)
				if off+i == flip {
					buf[i] ^= 0xFF
				}
			}
			_, _ = h.Write(buf[:n])
			off += n
		}
	}
	return domain.FormatPartialHash(h.Sum(nil))
}

func TestAChangedByteInsideARangeChangesThePartialHash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		size int64
		flip int64
	}{
		{name: "the only byte of a 1-byte file", size: 1, flip: 0},
		{name: "the first byte of a 4 MiB file", size: 4 * mib, flip: 0},
		{name: "the last byte of a 4 MiB file", size: 4 * mib, flip: 4*mib - 1},
		{name: "a byte in the middle of a 12 MiB file", size: 12 * mib, flip: 6 * mib},
		{name: "the last byte of a 12 MiB file", size: 12 * mib, flip: 12*mib - 1},
		{name: "the first byte of a 12 MiB + 1 file", size: 12*mib + 1, flip: 0},
		{name: "the first byte of the middle chunk of 12 MiB + 1", size: 12*mib + 1, flip: 4 * mib},
		{name: "the first byte of the last chunk of 12 MiB + 1", size: 12*mib + 1, flip: 8*mib + 1},
		{name: "the first byte of the first chunk of 100 MiB", size: 100 * mib, flip: 0},
		{name: "the last byte of the first chunk of 100 MiB", size: 100 * mib, flip: 4*mib - 1},
		{name: "the first byte of the centred chunk of 100 MiB", size: 100 * mib, flip: 48 * mib},
		{name: "the last byte of the centred chunk of 100 MiB", size: 100 * mib, flip: 52*mib - 1},
		{name: "the first byte of the last chunk of 100 MiB", size: 100 * mib, flip: 96 * mib},
		{name: "the last byte of the file", size: 100 * mib, flip: 100*mib - 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			same := partialHashOf(tt.size, -1)
			if got := partialHashOf(tt.size, tt.flip); got == same {
				t.Fatalf("hash of a changed byte %d equals the original, want a different hash", tt.flip)
			}
		})
	}
}

// Accepted limit (RF-24, documented in the spec and in the decision of the task): a byte that
// lies outside the ranges does not change the partial hash. Two files that differ only there
// get the same hash; the size and the full check of the copy are the only other tests.
func TestAChangedByteOutsideTheRangesKeepsThePartialHash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		size int64
		flip int64
	}{
		{name: "the byte at 8 MiB, between the centred and the last chunk of 12 MiB + 1", size: 12*mib + 1, flip: 8 * mib},
		{name: "a byte between the first and the centred chunk of 100 MiB", size: 100 * mib, flip: 10 * mib},
		{name: "a byte between the centred and the last chunk of 100 MiB", size: 100 * mib, flip: 60 * mib},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			same := partialHashOf(tt.size, -1)
			if got := partialHashOf(tt.size, tt.flip); got != same {
				t.Fatalf("hash of a byte outside the ranges changed: %s, want %s", got, same)
			}
		})
	}
}

func TestSameContentGivesTheSameHash(t *testing.T) {
	t.Parallel()

	for _, size := range []int64{1, 4 * mib, 12 * mib, 12*mib + 1, 100 * mib} {
		if a, b := partialHashOf(size, -1), partialHashOf(size, -1); a != b {
			t.Fatalf("size %d: two readings of the same content differ: %s and %s", size, a, b)
		}
	}
}

func TestPartialHashFormatIsVersionedHexOfTheSizeDigest(t *testing.T) {
	t.Parallel()

	// The digest of an empty file is SHA-256 of its size and a newline, nothing else (no ranges).
	sum := sha256.Sum256([]byte("0\n"))
	got := domain.FormatPartialHash(domain.NewPartialDigest(0).Sum(nil))
	want := "ph1:" + hex.EncodeToString(sum[:])
	if got != want {
		t.Fatalf("empty file hash = %q, want %q", got, want)
	}
	if !strings.HasPrefix(partialHashOf(1, -1), "ph1:") {
		t.Fatalf("the hash does not start with the version: %q", partialHashOf(1, -1))
	}
	if domain.PartialHashVersion != "ph1" {
		t.Fatalf("PartialHashVersion = %q, want ph1 (changing it invalidates every stored hash)", domain.PartialHashVersion)
	}
}
